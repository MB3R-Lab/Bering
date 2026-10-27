package discovery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/MB3R-Lab/Bering/internal/connectors/traces"
	"github.com/MB3R-Lab/Bering/internal/evidence"
	"github.com/MB3R-Lab/Bering/internal/model"
	"github.com/MB3R-Lab/Bering/internal/overlay"
	"github.com/MB3R-Lab/Bering/internal/snapshot"
)

// Duplicate exports may add provenance, but conflicting identities must never
// choose an arbitrary parent based on input order.
func unambiguousSpanIndex(spans []traces.Span) (map[string]traces.Span, map[string]bool) {
	index := map[string]traces.Span{}
	ambiguous := map[string]bool{}
	signatures := map[string]string{}
	for _, span := range spans {
		if strings.TrimSpace(span.TraceID) == "" || strings.TrimSpace(span.SpanID) == "" {
			continue
		}
		key := traceSpanKey(span.TraceID, span.SpanID)
		if ambiguous[key] {
			continue
		}
		if len(span.AttributeConflicts) > 0 {
			ambiguous[key] = true
			delete(index, key)
			continue
		}
		signature, err := spanSignature(span)
		if err != nil {
			ambiguous[key] = true
			delete(index, key)
			continue
		}
		if previous, exists := signatures[key]; exists && signature != previous {
			ambiguous[key] = true
			delete(index, key)
			continue
		}
		if previous, exists := index[key]; !exists || canonicalSpan(span) < canonicalSpan(previous) {
			index[key] = span
		}
		signatures[key] = signature
	}
	return index, ambiguous
}

func canonicalSpan(span traces.Span) string {
	raw, _ := json.Marshal(span)
	return string(raw)
}

func spanSignature(span traces.Span) (string, error) {
	// Timing is descriptive, not part of context identity. Attribute conflicts
	// remain explicit even when only one export has a particular field.
	attrs, resource := span.Attributes, span.ResourceAttributes
	if len(attrs) == 0 {
		attrs = nil
	}
	if len(resource) == 0 {
		resource = nil
	}
	raw, err := json.Marshal(struct {
		Service, Parent, Kind, Name string
		Attributes, Resource        map[string]any
	}{span.Service, span.ParentSpanID, span.Kind, span.Name, attrs, resource})
	return string(raw), err
}

func observationSource(span traces.Span, fallback string) evidence.Source {
	ref := span.SourceRef
	if ref == "" {
		ref = fallback
	}
	return evidence.Source{Ref: ref, ScopeName: span.ScopeName, ScopeVersion: span.ScopeVersion}
}

var instanceAttributeKeys = []string{
	"service.instance.id", "service.namespace", "host.id", "host.name", "host.boot.id",
	"container.id", "process.pid", "process.creation.time", "process.start_time",
	"k8s.cluster.uid", "k8s.cluster.name", "k8s.namespace.name", "k8s.pod.uid", "k8s.node.name",
	"telemetry.sdk.name", "telemetry.sdk.language", "telemetry.sdk.version",
}

func resourceAttr(span traces.Span, key string) string {
	if value := firstAttr(span.ResourceAttributes, key); value != "" {
		return value
	}
	return firstAttr(span.Attributes, key)
}

func observedInstance(span traces.Span, fallback string) (evidence.Instance, []string, bool) {
	attrs := map[string]string{}
	warnings := []string{}
	for _, key := range instanceAttributeKeys {
		if value := resourceAttr(span, key); value != "" {
			attrs[key] = value
		}
		resource, merged := firstAttr(span.ResourceAttributes, key), firstAttr(span.Attributes, key)
		if resource != "" && merged != "" && resource != merged {
			warnings = append(warnings, "resource_attribute_conflict:"+key)
		}
	}
	identity := map[string]string{"service": span.Service}
	status := "partial"
	if id := attrs["service.instance.id"]; id != "" {
		identity["service.instance.id"] = id
		identity["service.namespace"] = attrs["service.namespace"]
		status = "observed"
	} else if attrs["host.id"] != "" && attrs["process.pid"] != "" && (attrs["process.creation.time"] != "" || attrs["process.start_time"] != "") {
		status = "observed"
		identity["host.id"], identity["host.boot.id"] = attrs["host.id"], attrs["host.boot.id"]
		identity["process.pid"], identity["container.id"] = attrs["process.pid"], attrs["container.id"]
	} else if attrs["host.id"] == "" && attrs["host.name"] == "" && attrs["container.id"] == "" && attrs["process.pid"] == "" && attrs["k8s.pod.uid"] == "" {
		return evidence.Instance{}, warnings, false
	} else {
		warnings = append(warnings, "instance_identity_incomplete")
		for key, value := range attrs {
			identity[key] = value
		}
	}
	// A new process generation stays distinct even when an exporter reuses an
	// instance ID. This is observed history, never a concurrent replica census.
	for _, key := range []string{"host.id", "host.name", "host.boot.id", "k8s.cluster.uid", "k8s.namespace.name", "k8s.pod.uid", "container.id", "process.pid", "process.creation.time", "process.start_time"} {
		if attrs[key] != "" {
			identity[key] = attrs[key]
		}
	}
	return evidence.Instance{
		ID: stableObservationID("instance", identity), IdentityStatus: status,
		Attributes: attrs, Sources: []evidence.Source{observationSource(span, fallback)},
	}, warnings, true
}

func stableObservationID(prefix string, fields any) string {
	raw, _ := json.Marshal(fields)
	digest := sha256.Sum256(raw)
	return prefix + ":" + hex.EncodeToString(digest[:])
}

func instanceIDs(span traces.Span, fallback string) []string {
	instance, _, ok := observedInstance(span, fallback)
	if !ok || instance.IdentityStatus != "observed" {
		return nil
	}
	return []string{instance.ID}
}

type databaseObservation struct {
	id       string
	evidence evidence.Database
	warnings []string
	labels   map[string]string
}

func databaseFromClient(span traces.Span, fallback string) (databaseObservation, bool) {
	if !strings.EqualFold(span.Kind, "client") {
		return databaseObservation{}, false
	}
	system := strings.ToLower(firstAttr(span.Attributes, "db.system.name", "db.system"))
	if system == "" {
		return databaseObservation{}, false
	}
	address := strings.TrimSuffix(strings.ToLower(firstAttr(span.Attributes, "server.address", "net.peer.name", "network.peer.address", "net.peer.ip")), ".")
	portText := firstAttr(span.Attributes, "server.port", "network.peer.port", "net.peer.port")
	portValue, portErr := strconv.Atoi(portText)
	var port *int
	if portErr == nil && portValue > 0 && portValue <= 65535 {
		port = &portValue
	}
	warnings := []string{"database_physical_identity_unknown"}
	scope, scoped := databaseNetworkScope(span, address)
	address, validAddress := canonicalDatabaseAddress(address)
	resolution := "logical_endpoint"
	if !validAddress || port == nil || !scoped {
		resolution = "unresolved_alias"
		warnings = append(warnings, "database_endpoint_identity_incomplete")
		// Unresolved aliases are caller-scoped; they never establish a shared
		// physical resource merely because an address or protocol is missing.
		caller := map[string]string{"service": span.Service, "network_scope": scope}
		for _, key := range instanceAttributeKeys {
			if value := resourceAttr(span, key); value != "" {
				caller[key] = value
			}
		}
		scope = "caller:" + stableObservationID("scope", caller)
	}
	if !validAddress {
		// Never publish a malformed address containing credentials or URL data.
		address = ""
	}
	fields := map[string]string{"system": system, "address": address, "scope": scope, "port": "unknown"}
	if port != nil {
		fields["port"] = strconv.Itoa(*port)
	}
	id := stableObservationID("database", fields)
	db := evidence.Database{System: system, Address: address, Port: port, NetworkScope: scope, IdentityKind: resolution, Sources: []evidence.Source{observationSource(span, fallback)}}
	if namespace := firstAttr(span.Attributes, "db.namespace", "db.name"); namespace != "" {
		db.Namespaces = []string{namespace}
	}
	labels := map[string]string{"bering.node.kind": "database", "bering.database.identity": resolution, "db.system.name": system, "bering.network.scope": scope}
	if address != "" {
		labels["server.address"] = address
	}
	if port != nil {
		labels["server.port"] = strconv.Itoa(*port)
	}
	return databaseObservation{id: id, evidence: db, warnings: warnings, labels: labels}, true
}

func canonicalDatabaseAddress(address string) (string, bool) {
	if address == "" || strings.ContainsAny(address, " /@?#\t\r\n") {
		return "", false
	}
	if ip := net.ParseIP(strings.Trim(address, "[]")); ip != nil {
		if ip.IsUnspecified() || (!ip.IsGlobalUnicast() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast()) {
			return "", false
		}
		return ip.String(), true
	}
	if len(address) > 253 || strings.ContainsAny(address, ":[]") {
		return "", false
	}
	allNumeric := true
	for _, label := range strings.Split(address, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", false
		}
		for _, c := range label {
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
				return "", false
			}
			if c < '0' || c > '9' {
				allNumeric = false
			}
		}
	}
	if allNumeric {
		return "", false
	}
	return address, true
}

func databaseNetworkScope(span traces.Span, address string) (string, bool) {
	ip := net.ParseIP(strings.Trim(address, "[]"))
	if address == "localhost" || (ip != nil && (ip.IsLoopback() || ip.IsLinkLocalUnicast())) {
		// Loopback belongs to a network namespace, not to the entire input.
		if container := resourceAttr(span, "container.id"); container != "" {
			return "container:" + stableObservationID("scope", []string{resourceAttr(span, "host.id"), resourceAttr(span, "host.name"), resourceAttr(span, "k8s.pod.uid"), container}), true
		}
		return "host:" + resourceAttr(span, "host.id"), false
	}
	if scope := resourceAttr(span, "bering.network.scope"); scope != "" {
		return "declared:" + scope, true
	}
	if cluster := resourceAttr(span, "k8s.cluster.uid"); cluster != "" {
		if ip != nil {
			return "cluster:" + cluster, true
		}
		if namespace := resourceAttr(span, "k8s.namespace.name"); namespace != "" {
			return "cluster:" + cluster + ":namespace:" + namespace, true
		}
	}
	sharedAddressSpace := false
	if ipv4 := ip.To4(); ipv4 != nil {
		sharedAddressSpace = ipv4[0] == 100 && ipv4[1]&0xc0 == 64
	}
	if ip != nil && ip.IsGlobalUnicast() && !ip.IsPrivate() && !sharedAddressSpace {
		return "global_ip", true
	}
	return "unresolved", false
}

func enrichObservations(mdl *model.ResilienceModel, details *snapshot.DiscoveryDetails, spans []traces.Span, index map[string]traces.Span, ambiguous map[string]bool, opts Options, sourceRef string) error {
	serviceIndex, edgeIndex := map[string]int{}, map[string]int{}
	records, relations := map[string]*evidence.Record{}, map[string]*evidence.Record{}
	instanceRecordIndex := map[string]map[string]int{}
	serviceNamespaces := map[string]map[string]bool{}
	for i, service := range mdl.Services {
		serviceIndex[service.ID] = i
		records[service.ID] = &evidence.Record{Replicas: &evidence.ReplicaCount{ModelValue: service.Replicas, Basis: "compatibility_default_unknown"}, Warnings: []string{"configured_replica_count_unknown"}}
	}
	for i, edge := range mdl.Edges {
		edgeIndex[edge.ID] = i
		relations[edge.ID] = &evidence.Record{Relation: &evidence.Relation{Kind: "execution_link", Basis: "trace_parent"}}
	}
	dbStats := map[string]*serviceAccumulator{}
	dbEdgeStats := map[string]*edgeAccumulator{}
	pairedReceivers, endpointReceivers := databaseReceivers(index, sourceRef)
	for _, span := range spans {
		record := records[span.Service]
		if record == nil {
			continue
		}
		if len(span.AttributeConflicts) > 0 {
			for _, key := range span.AttributeConflicts {
				record.Warnings = appendUniqueStrings(record.Warnings, "attribute_conflict:"+key)
			}
			continue
		}
		if namespace := resourceAttr(span, "service.namespace"); namespace != "" {
			if serviceNamespaces[span.Service] == nil {
				serviceNamespaces[span.Service] = map[string]bool{}
			}
			serviceNamespaces[span.Service][namespace] = true
		}
		instance, warnings, ok := observedInstance(span, sourceRef)
		record.Warnings = appendUniqueStrings(record.Warnings, warnings...)
		if ok {
			if instanceRecordIndex[span.Service] == nil {
				instanceRecordIndex[span.Service] = map[string]int{}
			}
			key := stableObservationID("observation", []any{instance.ID, instance.IdentityStatus, instance.Attributes})
			if i, exists := instanceRecordIndex[span.Service][key]; exists {
				record.Instances[i].Sources = appendUniqueSources(record.Instances[i].Sources, instance.Sources...)
			} else {
				instanceRecordIndex[span.Service][key] = len(record.Instances)
				record.Instances = append(record.Instances, instance)
			}
		}
		key := traceSpanKey(span.TraceID, span.SpanID)
		if ambiguous[key] {
			record.Warnings = appendUniqueStrings(record.Warnings, "conflicting_span_identity")
			continue
		}
		if ambiguous[traceSpanKey(span.TraceID, span.ParentSpanID)] {
			record.Warnings = appendUniqueStrings(record.Warnings, "conflicting_parent_identity")
		}
		_, hasChild := index[key]
		if !hasChild {
			record.Warnings = appendUniqueStrings(record.Warnings, "context_identity_incomplete")
		}
		if parent, exists := parentSpan(span, index); exists && hasChild && parent.Service != span.Service {
			kind, _ := edgeKind(parent, span)
			id := edgeKey(parent.Service, span.Service, kind, kind == model.EdgeKindSync, edgeIdentity(parent, span))
			if relation := relations[id]; relation != nil {
				relation.Relation.FromInstances = appendUniqueStrings(relation.Relation.FromInstances, instanceIDs(parent, sourceRef)...)
				relation.Relation.ToInstances = appendUniqueStrings(relation.Relation.ToInstances, instanceIDs(span, sourceRef)...)
				relation.Relation.Sources = appendUniqueSources(relation.Relation.Sources, observationSource(parent, sourceRef), observationSource(span, sourceRef))
			}
		}
		db, isDB := databaseFromClient(span, sourceRef)
		if !isDB {
			continue
		}
		if children := pairedReceivers[key]; len(children) > 0 {
			for _, child := range children {
				kind, _ := edgeKind(span, child)
				id := edgeKey(span.Service, child.Service, kind, kind == model.EdgeKindSync, edgeIdentity(span, child))
				if relation := relations[id]; relation != nil {
					relation.DatabaseAliases = appendDatabaseAlias(relation.DatabaseAliases, db.evidence)
					relation.Warnings = appendUniqueStrings(relation.Warnings, "database_physical_identity_unknown")
					markDatabaseReceiver(mdl, details, serviceIndex[child.Service])
				}
			}
			// The receiver is already a graph node for this exact invocation.
			// A second synthetic node would invent an independent resource.
			continue
		}
		reusedReceiver := false
		if receivers := endpointReceivers[db.id]; len(receivers) > 1 {
			record.DatabaseAliases = appendDatabaseAlias(record.DatabaseAliases, db.evidence)
			record.Warnings = appendUniqueStrings(record.Warnings, "database_endpoint_has_multiple_observed_receivers")
			continue
		} else if len(receivers) == 1 {
			db.id = receivers[0]
			reusedReceiver = true
			markDatabaseReceiver(mdl, details, serviceIndex[db.id])
			records[db.id].DatabaseAliases = appendDatabaseAlias(records[db.id].DatabaseAliases, db.evidence)
		}
		if existing, exists := serviceIndex[db.id]; !reusedReceiver && exists && (mdl.Services[existing].Metadata == nil || mdl.Services[existing].Metadata.Labels["bering.node.kind"] != "database") {
			return fmt.Errorf("database identity collides with service %q", db.id)
		}
		if _, exists := records[db.id]; !exists {
			metadata := model.ServiceMetadata{CommonMetadata: model.CommonMetadata{Labels: db.labels}}
			serviceIndex[db.id] = len(mdl.Services)
			mdl.Services = append(mdl.Services, model.Service{ID: db.id, Name: db.id, Replicas: 1, Metadata: &metadata})
			details.Services = append(details.Services, snapshot.ServiceRecord{
				ID: db.id, Name: db.id, Replicas: 1, Metadata: &snapshot.ServiceMetadata{ServiceMetadata: metadata},
				Provenance: []snapshot.Provenance{{Type: "traces", Connector: inferConnector(opts.Sources), Ref: sourceRef}},
			})
			records[db.id] = &evidence.Record{Database: &db.evidence, Replicas: &evidence.ReplicaCount{ModelValue: 1, Basis: "logical_endpoint_placeholder"}, Warnings: db.warnings}
		} else if !reusedReceiver {
			records[db.id].Database.Namespaces = appendUniqueStrings(records[db.id].Database.Namespaces, db.evidence.Namespaces...)
			records[db.id].Database.Sources = appendUniqueSources(records[db.id].Database.Sources, db.evidence.Sources...)
		}
		if !reusedReceiver {
			dbStats[db.id] = updateServiceAccumulator(dbStats[db.id], span, opts.RuntimeMode)
		}
		identity := &model.EdgeIdentity{Protocol: db.evidence.System, Operation: firstAttr(span.Attributes, "db.operation.name", "db.operation"), SpanKind: "client"}
		id := edgeKey(span.Service, db.id, model.EdgeKindSync, true, identity)
		child := span
		child.Service = db.id
		dbEdgeStats[id] = updateEdgeAccumulator(dbEdgeStats[id], id, span, child, model.EdgeKindSync, true, identity, []string{"client_database_dependency", "sync_client_span_hint"}, opts.RuntimeMode)
		if relations[id] == nil {
			relations[id] = &evidence.Record{Relation: &evidence.Relation{Kind: "client_dependency", Basis: "database_client_attributes"}, Warnings: []string{"server_receipt_unobserved", "blocking_is_client_span_hint"}}
		}
		relations[id].DatabaseAliases = appendDatabaseAlias(relations[id].DatabaseAliases, db.evidence)
		relations[id].Relation.FromInstances = appendUniqueStrings(relations[id].Relation.FromInstances, instanceIDs(span, sourceRef)...)
		relations[id].Relation.Sources = appendUniqueSources(relations[id].Relation.Sources, observationSource(span, sourceRef))
	}
	for id, stat := range dbStats {
		for i := range details.Services {
			if details.Services[i].ID == id {
				details.Services[i].Support = stat.supportSummary()
				details.Services[i].Support.Evidence = []string{"database_client_attributes"}
				details.Services[i].FirstSeen = formatOptionalTime(stat.firstSeen, opts.RuntimeMode)
				details.Services[i].LastSeen = formatOptionalTime(stat.lastSeen, opts.RuntimeMode)
			}
		}
	}
	for id, stat := range dbEdgeStats {
		if _, exists := edgeIndex[id]; exists {
			return fmt.Errorf("database dependency collides with edge %q", id)
		}
		stat.edge.Observed = stat.observedSummary()
		stat.edge.Metadata = &model.EdgeMetadata{CommonMetadata: model.CommonMetadata{Labels: map[string]string{"bering.relation.kind": "client_dependency"}}}
		mdl.Edges = append(mdl.Edges, stat.edge)
		details.Edges = append(details.Edges, snapshot.EdgeRecord{
			ID: id, From: stat.edge.From, To: stat.edge.To, Kind: stat.edge.Kind, Blocking: stat.edge.Blocking,
			Identity: stat.edge.Identity, Metadata: &snapshot.EdgeMetadata{EdgeMetadata: *stat.edge.Metadata},
			Support: stat.supportSummary(), Observed: stat.edge.Observed,
			FirstSeen: formatOptionalTime(stat.firstSeen, opts.RuntimeMode), LastSeen: formatOptionalTime(stat.lastSeen, opts.RuntimeMode),
			Provenance: []snapshot.Provenance{{Type: "traces", Connector: inferConnector(opts.Sources), Ref: sourceRef}},
		})
	}
	for i := range details.Services {
		record := records[details.Services[i].ID]
		*record = evidence.Normalize(*record)
		observedIDs := map[string]bool{}
		placements := map[string]map[string]bool{}
		namespaces := map[string]bool{}
		for namespace := range serviceNamespaces[details.Services[i].ID] {
			namespaces[namespace] = true
		}
		for _, instance := range record.Instances {
			if namespace := instance.Attributes["service.namespace"]; namespace != "" {
				namespaces[namespace] = true
			}
			if instance.IdentityStatus == "observed" {
				observedIDs[instance.ID] = true
			}
			if id := instance.Attributes["service.instance.id"]; id != "" {
				if placements[id] == nil {
					placements[id] = map[string]bool{}
				}
				host := stableObservationID("placement", []string{instance.Attributes["host.id"], instance.Attributes["host.name"], instance.Attributes["k8s.pod.uid"]})
				placements[id][host] = true
			}
		}
		record.Replicas.ObservedIdentities = len(observedIDs)
		if len(namespaces) > 1 {
			record.Warnings = appendUniqueStrings(record.Warnings, "service_name_has_multiple_observed_namespaces")
		}
		for _, hosts := range placements {
			if len(hosts) > 1 {
				record.Warnings = appendUniqueStrings(record.Warnings, "instance_identity_has_multiple_observed_placements")
			}
		}
		if len(record.Instances) > 0 {
			record.Warnings = append(record.Warnings, "observed_instances_are_not_a_concurrent_replica_inventory")
		}
		metadata := ensureSnapshotServiceMetadata(&details.Services[i])
		if err := putEvidence(&metadata.Attributes, *record); err != nil {
			return err
		}
	}
	for i := range details.Edges {
		if record := relations[details.Edges[i].ID]; record != nil {
			metadata := ensureSnapshotEdgeMetadata(&details.Edges[i])
			if err := putEvidence(&metadata.Attributes, *record); err != nil {
				return err
			}
		}
	}
	sort.Slice(details.Services, func(i, j int) bool { return details.Services[i].ID < details.Services[j].ID })
	sort.Slice(details.Edges, func(i, j int) bool { return details.Edges[i].ID < details.Edges[j].ID })
	return nil
}

func databaseReceivers(index map[string]traces.Span, sourceRef string) (map[string][]traces.Span, map[string][]string) {
	paired := map[string][]traces.Span{}
	endpoints := map[string][]string{}
	for _, child := range index {
		if !strings.EqualFold(child.Kind, "server") {
			continue
		}
		parent, ok := parentSpan(child, index)
		if !ok || child.Service == parent.Service || strings.TrimSpace(child.Service) == "" {
			continue
		}
		db, ok := databaseFromClient(parent, sourceRef)
		if !ok {
			continue
		}
		key := traceSpanKey(parent.TraceID, parent.SpanID)
		paired[key] = append(paired[key], child)
		if db.evidence.IdentityKind == "logical_endpoint" {
			endpoints[db.id] = appendUniqueStrings(endpoints[db.id], child.Service)
		}
	}
	for key := range paired {
		sort.Slice(paired[key], func(i, j int) bool { return canonicalSpan(paired[key][i]) < canonicalSpan(paired[key][j]) })
	}
	for key := range endpoints {
		sort.Strings(endpoints[key])
	}
	return paired, endpoints
}

func markDatabaseReceiver(mdl *model.ResilienceModel, details *snapshot.DiscoveryDetails, i int) {
	metadata := ensureModelServiceMetadata(&mdl.Services[i])
	if metadata.Labels == nil {
		metadata.Labels = map[string]string{}
	}
	metadata.Labels["bering.node.kind"] = "database"
	metadata.Labels["bering.database.identity"] = "observed_receiver_service"
	for j := range details.Services {
		if details.Services[j].ID == mdl.Services[i].ID {
			record := ensureSnapshotServiceMetadata(&details.Services[j])
			if record.Labels == nil {
				record.Labels = map[string]string{}
			}
			record.Labels["bering.node.kind"] = "database"
			record.Labels["bering.database.identity"] = "observed_receiver_service"
		}
	}
}

func appendDatabaseAlias(values []evidence.Database, addition evidence.Database) []evidence.Database {
	for i := range values {
		a, b := values[i], addition
		a.Sources, b.Sources, a.Namespaces, b.Namespaces = nil, nil, nil, nil
		left, _ := json.Marshal(a)
		right, _ := json.Marshal(b)
		if string(left) == string(right) {
			values[i].Sources = appendUniqueSources(values[i].Sources, addition.Sources...)
			values[i].Namespaces = appendUniqueStrings(values[i].Namespaces, addition.Namespaces...)
			return values
		}
	}
	return append(values, addition)
}

func validateObservationOverlay(file overlay.File) error {
	check := func(kind, id string, attrs map[string]string) error {
		if _, exists := attrs[evidence.AttributeKey]; exists {
			return fmt.Errorf("overlay %q %s %q writes reserved generated evidence attribute %q", file.Name, kind, id, evidence.AttributeKey)
		}
		return nil
	}
	for _, item := range file.Services {
		if err := check("service", item.ID, item.Attributes); err != nil {
			return err
		}
	}
	for _, item := range file.Edges {
		if err := check("edge", item.ID, item.Attributes); err != nil {
			return err
		}
	}
	for _, item := range file.Endpoints {
		if err := check("endpoint", item.ID, item.Attributes); err != nil {
			return err
		}
	}
	return nil
}

func appendUniqueStrings(values []string, additions ...string) []string {
	for _, addition := range additions {
		found := false
		for _, value := range values {
			if value == addition {
				found = true
				break
			}
		}
		if !found {
			values = append(values, addition)
		}
	}
	return values
}

func appendUniqueSources(values []evidence.Source, additions ...evidence.Source) []evidence.Source {
	for _, addition := range additions {
		found := false
		for _, value := range values {
			if value == addition {
				found = true
				break
			}
		}
		if !found {
			values = append(values, addition)
		}
	}
	return values
}

func putEvidence(attrs *map[string]string, record evidence.Record) error {
	encoded, err := evidence.Encode(record)
	if err != nil {
		return err
	}
	if *attrs == nil {
		*attrs = map[string]string{}
	}
	(*attrs)[evidence.AttributeKey] = encoded
	return nil
}

func reconcileReplicaEvidence(records []snapshot.ServiceRecord, overlays []overlay.File) error {
	supplied := map[string]int{}
	for _, file := range overlays {
		for _, item := range file.Services {
			if item.Replicas != nil {
				supplied[item.ID] = *item.Replicas
			}
		}
	}
	for i := range records {
		metadata := records[i].Metadata
		if metadata == nil || metadata.Attributes[evidence.AttributeKey] == "" {
			continue
		}
		record, err := evidence.Decode(metadata.Attributes[evidence.AttributeKey])
		if err != nil {
			return fmt.Errorf("service %q observation evidence: %w", records[i].ID, err)
		}
		if record.Replicas != nil {
			record.Replicas.ModelValue = records[i].Replicas
			if count, ok := supplied[records[i].ID]; ok {
				record.Replicas.Configured, record.Replicas.Basis = &count, "supplied_overlay"
				filtered := record.Warnings[:0]
				for _, warning := range record.Warnings {
					if warning != "configured_replica_count_unknown" {
						filtered = append(filtered, warning)
					}
				}
				record.Warnings = filtered
			}
		}
		if err := putEvidence(&metadata.Attributes, record); err != nil {
			return err
		}
	}
	return nil
}
