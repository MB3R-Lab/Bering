package discovery

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/MB3R-Lab/Bering/internal/connectors/traces"
	"github.com/MB3R-Lab/Bering/internal/evidence"
	"github.com/MB3R-Lab/Bering/internal/model"
	"github.com/MB3R-Lab/Bering/internal/snapshot"
)

// enrichEBPFPeers adds structural dependencies from client observations when
// there is no exact server receipt. Address or timing never creates ancestry.
func enrichEBPFPeers(mdl *model.ResilienceModel, details *snapshot.DiscoveryDetails, spans []traces.Span, index map[string]traces.Span, ambiguous map[string]bool, opts Options, sourceRef string) error {
	receivers := map[string]bool{}
	for _, span := range spans {
		if !strings.EqualFold(span.Kind, "server") || span.TraceID == "" || span.ParentSpanID == "" {
			continue
		}
		key := traceSpanKey(span.TraceID, span.ParentSpanID, span.SourceDomain)
		if span.SourceDomain != "" && strings.HasPrefix(span.SourceDomain, "ebpf:") {
			receivers[key] = true
		}
	}
	serviceIndex := map[string]int{}
	edgeIndex := map[string]int{}
	for i, service := range mdl.Services {
		serviceIndex[service.ID] = i
	}
	for i, edge := range mdl.Edges {
		edgeIndex[edge.ID] = i
	}
	serviceStats := map[string]*serviceAccumulator{}
	edgeStats := map[string]*edgeAccumulator{}
	peerRecords := map[string]*evidence.Record{}
	relationRecords := map[string]*evidence.Record{}
	observedScopes := observedServiceScopes(spans)
	for _, span := range spans {
		if span.SourceKind != "ebpf" || !strings.EqualFold(span.Kind, "client") || strings.TrimSpace(span.Service) == "" {
			continue
		}
		if firstAttr(span.Attributes, "db.system.name", "db.system") != "" {
			continue // Database dependencies are handled by enrichObservations.
		}
		identity, ok := ebpfOperation(span)
		if !ok {
			continue // TCP and unclassified transport are not application calls.
		}
		key := traceSpanKey(span.TraceID, span.SpanID, span.SourceDomain)
		if len(span.AttributeConflicts) > 0 || (span.TraceID != "" && span.SpanID != "" && ambiguous[key]) {
			if err := warnEBPFCaller(details, span.Service, "conflicting_client_identity"); err != nil {
				return err
			}
			continue
		}
		if receivers[key] && span.TraceID != "" && span.SpanID != "" {
			// A receiver exists for this exact context. Its existing execution
			// edge, or its ambiguity, must not be replaced by a guessed peer.
			continue
		}
		peer, target, labels, ok := ebpfPeer(span, sourceRef)
		if !ok {
			if err := warnEBPFCaller(details, span.Service, "peer_identity_unresolved"); err != nil {
				return err
			}
			continue
		}
		if peer.Service == span.Service {
			continue
		}
		if peer.Service != "" {
			if scopes := observedScopes[peer.Service]; len(scopes) == 1 {
				if scopes[peerScope{domain: span.SourceDomain, namespace: ebpfPeerNamespace(span), network: peer.NetworkScope}] {
					if _, exists := serviceIndex[peer.Service]; exists {
						target = peer.Service
					}
				}
			}
		}
		if i, exists := serviceIndex[target]; exists {
			if meta := mdl.Services[i].Metadata; meta != nil && meta.Labels["bering.node.kind"] == "database" {
				if err := warnEBPFCaller(details, span.Service, "peer_name_collides_with_database"); err != nil {
					return err
				}
				continue
			}
		} else {
			metadata := model.ServiceMetadata{CommonMetadata: model.CommonMetadata{Labels: labels}}
			name := target
			if peer.Service != "" {
				name = peer.Service
			}
			serviceIndex[target] = len(mdl.Services)
			mdl.Services = append(mdl.Services, model.Service{ID: target, Name: name, Replicas: 1, Metadata: &metadata})
			details.Services = append(details.Services, snapshot.ServiceRecord{
				ID: target, Name: name, Replicas: 1,
				Metadata:   &snapshot.ServiceMetadata{ServiceMetadata: metadata},
				Provenance: []snapshot.Provenance{ebpfProvenance(span, sourceRef)},
			})
			peerRecords[target] = &evidence.Record{
				Peer:     &peer,
				Replicas: &evidence.ReplicaCount{ModelValue: 1, Basis: "logical_peer_placeholder"},
				Warnings: []string{"configured_replica_count_unknown", "physical_peer_identity_unknown"},
			}
		}
		if record := peerRecords[target]; record != nil {
			record.Peer.Sources = appendUniqueSources(record.Peer.Sources, peer.Sources...)
			serviceStats[target] = updateServiceAccumulator(serviceStats[target], span, opts.RuntimeMode)
		}
		if equivalentEBPFEdge(mdl.Edges, span.Service, target, identity) {
			continue
		}
		id := edgeKey(span.Service, target, model.EdgeKindSync, true, identity)
		if _, exists := edgeIndex[id]; exists {
			continue
		}
		child := span
		child.Service = target
		edgeStats[id] = updateEdgeAccumulator(edgeStats[id], id, span, child,
			model.EdgeKindSync, true, identity,
			[]string{"ebpf_client_dependency", "sync_client_span_hint"}, opts.RuntimeMode)
		if relationRecords[id] == nil {
			relationRecords[id] = &evidence.Record{
				Peer:     &peer,
				Relation: &evidence.Relation{Kind: "client_dependency", Basis: "ebpf_client_attributes"},
				Warnings: []string{"server_receipt_unobserved", "execution_context_unconfirmed", "blocking_is_client_span_hint"},
			}
		}
		relation := relationRecords[id]
		relation.Relation.FromInstances = appendUniqueStrings(relation.Relation.FromInstances, instanceIDs(span, sourceRef)...)
		relation.Relation.Sources = appendUniqueSources(relation.Relation.Sources, observationSource(span, sourceRef))
		relation.Peer.Sources = appendUniqueSources(relation.Peer.Sources, peer.Sources...)
	}
	for id, record := range peerRecords {
		for i := range details.Services {
			if details.Services[i].ID != id {
				continue
			}
			if stat := serviceStats[id]; stat != nil {
				details.Services[i].Support = stat.supportSummary()
				details.Services[i].Support.Evidence = []string{"ebpf_client_attributes"}
				details.Services[i].FirstSeen = formatOptionalTime(stat.firstSeen, opts.RuntimeMode)
				details.Services[i].LastSeen = formatOptionalTime(stat.lastSeen, opts.RuntimeMode)
			}
			if err := putEvidence(&ensureSnapshotServiceMetadata(&details.Services[i]).Attributes, *record); err != nil {
				return err
			}
		}
	}
	for id, stat := range edgeStats {
		provenance := snapshot.Provenance{Type: "ebpf", Connector: "obi", Ref: sourceRef}
		if sources := relationRecords[id].Relation.Sources; len(sources) > 0 {
			provenance = evidenceSourceProvenance(sources[0], sourceRef)
		}
		stat.edge.Observed = stat.observedSummary()
		stat.edge.Metadata = &model.EdgeMetadata{CommonMetadata: model.CommonMetadata{Labels: map[string]string{"bering.relation.kind": "client_dependency"}}}
		mdl.Edges = append(mdl.Edges, stat.edge)
		record := snapshot.EdgeRecord{
			ID: id, From: stat.edge.From, To: stat.edge.To,
			Kind: stat.edge.Kind, Blocking: stat.edge.Blocking,
			Identity: stat.edge.Identity, Metadata: &snapshot.EdgeMetadata{EdgeMetadata: *stat.edge.Metadata},
			Support: stat.supportSummary(), Observed: stat.edge.Observed,
			FirstSeen:  formatOptionalTime(stat.firstSeen, opts.RuntimeMode),
			LastSeen:   formatOptionalTime(stat.lastSeen, opts.RuntimeMode),
			Provenance: []snapshot.Provenance{provenance},
		}
		if err := putEvidence(&record.Metadata.Attributes, *relationRecords[id]); err != nil {
			return err
		}
		details.Edges = append(details.Edges, record)
	}
	sort.Slice(details.Services, func(i, j int) bool { return details.Services[i].ID < details.Services[j].ID })
	sort.Slice(details.Edges, func(i, j int) bool { return details.Edges[i].ID < details.Edges[j].ID })
	return nil
}

func ebpfProvenance(span traces.Span, fallback string) snapshot.Provenance {
	return evidenceSourceProvenance(observationSource(span, fallback), fallback)
}

func evidenceSourceProvenance(source evidence.Source, fallback string) snapshot.Provenance {
	connector := source.Connector
	if connector == "" {
		connector = "obi"
	}
	if source.Ref == "" {
		source.Ref = fallback
	}
	return snapshot.Provenance{Type: "ebpf", Connector: connector, Ref: source.Ref}
}

func ebpfOperation(span traces.Span) (*model.EdgeIdentity, bool) {
	method := strings.ToUpper(firstAttr(span.Attributes, "http.request.method", "http.method"))
	route := firstAttr(span.Attributes, "http.route", "url.path", "http.target")
	if route == "" {
		if full := firstAttr(span.Attributes, "url.full", "http.url"); full != "" {
			parsed, err := url.Parse(full)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
				return nil, false
			}
			route = parsed.Path
			if route == "" {
				route = "/"
			}
		}
	}
	if method == "" || route == "" {
		fromName, path := parseMethodAndPathFromSpanName(span.Name)
		if method == "" {
			method = fromName
		}
		if route == "" {
			route = path
		}
	}
	if isHTTPMethod(method) && normalizePath(route) != "" {
		return &model.EdgeIdentity{Protocol: "http", Operation: method, Route: normalizePath(route), SpanKind: "client"}, true
	}
	protocol, protocolOK := oneObservedValue(span.Attributes, true, "rpc.system", "rpc.system.name")
	operation := firstAttr(span.Attributes, "rpc.method")
	if protocolOK && protocol != "" && operation != "" {
		return &model.EdgeIdentity{Protocol: protocol, Operation: operation, SpanKind: "client"}, true
	}
	return nil, false
}

func ebpfPeer(span traces.Span, fallback string) (evidence.Peer, string, map[string]string, bool) {
	for _, key := range []string{
		"bering.network.scope", "host.id", "host.name", "container.id",
		"service.instance.id", "k8s.cluster.uid", "k8s.namespace.name", "k8s.pod.uid",
	} {
		resource, merged := firstAttr(span.ResourceAttributes, key), firstAttr(span.Attributes, key)
		if resource != "" && merged != "" && resource != merged {
			return evidence.Peer{}, "", nil, false
		}
	}
	service, consistent := oneObservedValue(span.Attributes, false, "peer.service", "service.peer.name")
	if !consistent {
		return evidence.Peer{}, "", nil, false
	}
	// A DNS name and observed socket IP can both describe one request. Prefer
	// the explicit server.address name, without asserting physical equivalence.
	address := strings.ToLower(firstAttr(span.Attributes, "server.address", "net.peer.name", "network.peer.address", "net.peer.ip"))
	portText, portOK := oneObservedValue(span.Attributes, false, "server.port", "network.peer.port", "net.peer.port")
	if !portOK {
		return evidence.Peer{}, "", nil, false
	}
	source := observationSource(span, fallback)
	if service != "" {
		if strings.ContainsAny(service, "\x00\r\n\t") {
			return evidence.Peer{}, "", nil, false
		}
		namespace, consistent := oneObservedValue(span.Attributes, false, "peer.service.namespace", "service.peer.namespace")
		if !consistent {
			return evidence.Peer{}, "", nil, false
		}
		scope := explicitEBPFNetworkScope(span)
		if scope == "" && address != "" {
			if inferred, known := databaseNetworkScope(span, address); known {
				scope = inferred
			}
		}
		fields := map[string]string{"service": service, "domain": span.SourceDomain, "namespace": namespace, "network_scope": scope}
		id := stableObservationID("peer", fields)
		peer := evidence.Peer{Service: service, NetworkScope: scope, IdentityKind: "reported_service", Sources: []evidence.Source{source}}
		labels := map[string]string{"bering.node.kind": "peer", "bering.peer.identity": "reported_service", "bering.peer.service": service}
		if scope != "" {
			labels["bering.network.scope"] = scope
		}
		if namespace != "" {
			labels["service.namespace"] = namespace
		}
		return peer, id, labels, true
	}
	address, valid := canonicalDatabaseAddress(strings.TrimSuffix(address, "."))
	portValue, err := strconv.Atoi(portText)
	if !valid || err != nil || portValue < 1 || portValue > 65535 {
		return evidence.Peer{}, "", nil, false
	}
	scope, scoped := databaseNetworkScope(span, address)
	identityKind := "logical_endpoint"
	if !scoped {
		identityKind = "unresolved_alias"
		caller := map[string]string{"service": span.Service, "domain": span.SourceDomain}
		for _, key := range instanceAttributeKeys {
			if value := resourceAttr(span, key); value != "" {
				caller[key] = value
			}
		}
		scope = "caller:" + stableObservationID("scope", caller)
	}
	fields := map[string]string{"address": address, "port": strconv.Itoa(portValue), "scope": scope, "domain": span.SourceDomain}
	id := stableObservationID("peer", fields)
	peer := evidence.Peer{Address: address, Port: &portValue, NetworkScope: scope, IdentityKind: identityKind, Sources: []evidence.Source{source}}
	labels := map[string]string{"bering.node.kind": "peer", "bering.peer.identity": identityKind,
		"bering.network.scope": scope, "server.address": address, "server.port": strconv.Itoa(portValue)}
	return peer, id, labels, true
}

type peerScope struct {
	domain, namespace, network string
}

func observedServiceScopes(spans []traces.Span) map[string]map[peerScope]bool {
	out := map[string]map[peerScope]bool{}
	for _, span := range spans {
		name := strings.TrimSpace(span.Service)
		if name == "" {
			continue
		}
		if out[name] == nil {
			out[name] = map[peerScope]bool{}
		}
		scope := peerScope{
			domain:    span.SourceDomain,
			namespace: resourceAttr(span, "service.namespace"),
			network:   explicitEBPFNetworkScope(span),
		}
		if len(span.AttributeConflicts) > 0 {
			scope.network = "conflicting_attributes"
		}
		out[name][scope] = true
	}
	return out
}

func explicitEBPFNetworkScope(span traces.Span) string {
	if scope := resourceAttr(span, "bering.network.scope"); scope != "" {
		return "declared:" + scope
	}
	return ""
}

func ebpfPeerNamespace(span traces.Span) string {
	value, _ := oneObservedValue(span.Attributes, false, "peer.service.namespace", "service.peer.namespace")
	return value
}

func oneObservedValue(attrs map[string]any, fold bool, keys ...string) (string, bool) {
	value := ""
	for _, key := range keys {
		current := strings.TrimSpace(attrString(attrs, key))
		if fold {
			current = strings.ToLower(current)
		}
		if current == "" {
			continue
		}
		if value != "" && value != current {
			return "", false
		}
		value = current
	}
	return value, true
}

func equivalentEBPFEdge(edges []model.Edge, from, to string, identity *model.EdgeIdentity) bool {
	for _, edge := range edges {
		if edge.From != from || edge.To != to || edge.Kind != model.EdgeKindSync || !edge.Blocking || edge.Identity == nil {
			continue
		}
		if strings.EqualFold(edge.Identity.Protocol, identity.Protocol) && edge.Identity.Operation == identity.Operation && edge.Identity.Route == identity.Route {
			return true
		}
	}
	return false
}

func warnEBPFCaller(details *snapshot.DiscoveryDetails, serviceID, warning string) error {
	for i := range details.Services {
		if details.Services[i].ID != serviceID {
			continue
		}
		metadata := ensureSnapshotServiceMetadata(&details.Services[i])
		record, err := evidence.Decode(metadata.Attributes[evidence.AttributeKey])
		if err != nil {
			return fmt.Errorf("service %q observation evidence: %w", serviceID, err)
		}
		record.Warnings = appendUniqueStrings(record.Warnings, warning)
		return putEvidence(&metadata.Attributes, record)
	}
	return nil
}
