package discovery

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/MB3R-Lab/Bering/internal/connectors/traces"
	"github.com/MB3R-Lab/Bering/internal/evidence"
	"github.com/MB3R-Lab/Bering/internal/model"
	"github.com/MB3R-Lab/Bering/internal/overlay"
	"github.com/MB3R-Lab/Bering/internal/snapshot"
)

func TestServiceNamespaceConflictWithoutInstanceHints(t *testing.T) {
	t.Parallel()
	result := discoverObservations(t, []traces.Span{
		{TraceID: "one", SpanID: "a", Service: "api", Attributes: map[string]any{"service.namespace": "blue"}},
		{TraceID: "two", SpanID: "b", Service: "api", Attributes: map[string]any{"service.namespace": "green"}},
	}, Options{})
	record := serviceEvidence(t, result, "api")
	if len(record.Instances) != 0 || !containsString(record.Warnings, "service_name_has_multiple_observed_namespaces") {
		t.Fatalf("namespace-only identity conflict was lost: %+v", record)
	}
}

func TestObservedInstancesAreHistoryNotReplicaCount(t *testing.T) {
	t.Parallel()

	first := observedSpan("trace-1", "one", "worker", "host-a", "41", "100")
	first.SourceRef = "synthetic://collector-a"
	duplicateExport := first
	duplicateExport.SourceRef = "synthetic://collector-b"
	spans := []traces.Span{
		first,
		duplicateExport,
		observedSpan("trace-2", "two", "worker", "host-b", "41", "100"),
		// A restarted process reusing the PID remains a distinct observation.
		observedSpan("trace-3", "three", "worker", "host-a", "41", "200"),
	}
	result := discoverObservations(t, spans, Options{})
	service := modelService(t, result, "worker")
	if service.Replicas != 1 {
		t.Fatalf("observed history changed compatibility replica count: got=%d want=1", service.Replicas)
	}
	record := serviceEvidence(t, result, "worker")
	if record.Replicas == nil || record.Replicas.ModelValue != 1 || record.Replicas.ObservedIdentities != 3 || record.Replicas.Configured != nil {
		t.Fatalf("unexpected replica evidence: %+v", record.Replicas)
	}
	if len(record.Instances) != 3 {
		t.Fatalf("PID reuse/restart or cross-host observations collapsed: %+v", record.Instances)
	}
	if len(record.Instances[0].Sources)+len(record.Instances[1].Sources)+len(record.Instances[2].Sources) != 4 {
		t.Fatalf("duplicate export provenance was not retained on the shared identity: %+v", record.Instances)
	}
	if !containsString(record.Warnings, "observed_instances_are_not_a_concurrent_replica_inventory") {
		t.Fatalf("missing historical-instance warning: %+v", record.Warnings)
	}
}

func TestSuppliedReplicaCountHasExplicitEvidenceBasis(t *testing.T) {
	t.Parallel()

	result := discoverObservations(t, []traces.Span{
		observedSpan("trace-1", "one", "worker", "host-a", "9", "100"),
	}, Options{ReplicasOverride: map[string]int{"worker": 4}})
	record := serviceEvidence(t, result, "worker")
	if record.Replicas == nil || record.Replicas.Configured == nil || *record.Replicas.Configured != 4 ||
		record.Replicas.ModelValue != 4 || record.Replicas.Basis != "supplied_overlay" || record.Replicas.ObservedIdentities != 1 {
		t.Fatalf("supplied count provenance was not preserved: %+v", record.Replicas)
	}
	if containsString(record.Warnings, "configured_replica_count_unknown") {
		t.Fatalf("configured count is still reported unknown: %+v", record.Warnings)
	}
	if len(result.Discovery.Overlays) != 1 || result.Discovery.Overlays[0].Name != "replicas" || result.Discovery.Overlays[0].Ref != "legacy:replicas" {
		t.Fatalf("supplied replica overlay provenance is missing: %+v", result.Discovery.Overlays)
	}
}

func TestClientOnlyDatabaseObservationsShareExplicitScopeAndMergeNamespaces(t *testing.T) {
	t.Parallel()

	spans := []traces.Span{
		databaseClient("trace-a", "db-a", "reader", "orders", "db.internal", "5432", "shared-fabric", "container-a"),
		databaseClient("trace-b", "db-b", "writer", "billing", "db.internal", "5432", "shared-fabric", "container-b"),
	}
	result := discoverObservations(t, spans, Options{})
	databases := databaseServices(result)
	if len(databases) != 1 {
		t.Fatalf("client-only observations in one explicit scope did not form one logical database: %+v", databases)
	}
	record := serviceEvidence(t, result, databases[0].ID)
	if record.Database == nil || record.Database.IdentityKind != "logical_endpoint" || record.Database.Address != "db.internal" ||
		record.Database.Port == nil || *record.Database.Port != 5432 ||
		!reflect.DeepEqual(record.Database.Namespaces, []string{"billing", "orders"}) {
		t.Fatalf("unexpected logical database evidence: %+v", record.Database)
	}
	if len(databaseEdges(result)) != 2 {
		t.Fatalf("both client dependencies must remain visible: %+v", databaseEdges(result))
	}
	for _, edge := range databaseEdges(result) {
		relation := edgeEvidence(t, result, edge.ID).Relation
		if relation == nil || relation.Kind != "client_dependency" || relation.Basis != "database_client_attributes" || len(relation.FromInstances) != 1 {
			t.Fatalf("client-only relation evidence is incomplete: edge=%s evidence=%+v", edge.ID, relation)
		}
	}
}

func TestDatabaseScopeIsolationAndLoopbackCallerContexts(t *testing.T) {
	t.Parallel()

	spans := []traces.Span{
		databaseClient("trace-a", "a", "reader-a", "main", "10.0.0.8", "5432", "scope-a", "container-a"),
		databaseClient("trace-b", "b", "reader-b", "main", "10.0.0.8", "5432", "scope-b", "container-b"),
		databaseClient("trace-c", "c", "local-a", "main", "localhost", "5432", "", "container-a"),
		databaseClient("trace-d", "d", "local-b", "main", "127.0.0.1", "5432", "", "container-b"),
	}
	result := discoverObservations(t, spans, Options{})
	if got := len(databaseServices(result)); got != 4 {
		t.Fatalf("isolated declared scopes or loopback caller contexts were merged: got=%d databases=%+v", got, databaseServices(result))
	}
}

func TestInvalidPortsStayUnresolvedAndCallerScoped(t *testing.T) {
	t.Parallel()

	spans := []traces.Span{
		databaseClient("trace-a", "a", "reader-a", "main", "db.internal", "", "shared-fabric", "container-a"),
		databaseClient("trace-b", "b", "reader-b", "main", "db.internal", "0", "shared-fabric", "container-b"),
		databaseClient("trace-c", "c", "reader-c", "main", "db.internal", "70000", "shared-fabric", "container-c"),
	}
	result := discoverObservations(t, spans, Options{})
	databases := databaseServices(result)
	if len(databases) != 3 {
		t.Fatalf("unresolved endpoints from different callers were merged: %+v", databases)
	}
	for _, service := range databases {
		record := serviceEvidence(t, result, service.ID)
		if record.Database == nil || record.Database.IdentityKind != "unresolved_alias" || record.Database.Port != nil {
			t.Fatalf("invalid port became a resolved endpoint: %+v", record.Database)
		}
		if service.Metadata == nil || service.Metadata.Labels["server.port"] != "" ||
			!strings.HasPrefix(service.Metadata.Labels["bering.network.scope"], "caller:") {
			t.Fatalf("unresolved database labels are unsafe: %+v", service.Metadata)
		}
	}
}

func TestServerDatabaseSpanDoesNotCreateSelfDependency(t *testing.T) {
	t.Parallel()

	span := observedSpan("trace-1", "server", "database-server", "host-a", "8", "100")
	span.Kind = "server"
	span.Attributes["db.system.name"] = "sql"
	span.Attributes["server.address"] = "db.internal"
	span.Attributes["server.port"] = "5432"
	result := discoverObservations(t, []traces.Span{span}, Options{})
	if len(databaseServices(result)) != 0 || len(result.Model.Edges) != 0 {
		t.Fatalf("database server span synthesized a client/self dependency: services=%+v edges=%+v", result.Model.Services, result.Model.Edges)
	}
}

func TestMalformedDatabaseAddressIsNotPublished(t *testing.T) {
	t.Parallel()

	const malformed = "sql://user:secret@db.internal/data"
	result := discoverObservations(t, []traces.Span{
		databaseClient("trace-a", "a", "reader", "main", malformed, "5432", "shared-fabric", "container-a"),
	}, Options{})
	databases := databaseServices(result)
	if len(databases) != 1 {
		t.Fatalf("expected one unresolved database observation: %+v", databases)
	}
	record := serviceEvidence(t, result, databases[0].ID)
	if record.Database == nil || record.Database.Address != "" || record.Database.IdentityKind != "unresolved_alias" {
		t.Fatalf("malformed address was published: %+v", record.Database)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	if strings.Contains(string(raw), malformed) || strings.Contains(string(raw), "user:secret") {
		t.Fatalf("credentials or malformed address leaked into discovered output: %s", raw)
	}
}

func TestConflictingSpanIdentityNeverChoosesArbitraryEdgeAndInputOrderIsDeterministic(t *testing.T) {
	t.Parallel()

	spans := []traces.Span{
		{TraceID: "trace-1", SpanID: "root", Service: "upstream", Name: "root", Kind: "client"},
		{TraceID: "trace-1", SpanID: "shared", Service: "parent-a", Name: "call", Kind: "client"},
		{TraceID: "trace-1", SpanID: "shared", ParentSpanID: "root", Service: "parent-b", Name: "handle", Kind: "server"},
		{TraceID: "trace-1", SpanID: "child", ParentSpanID: "shared", Service: "worker", Name: "handle", Kind: "server"},
	}
	forward := discoverObservations(t, spans, Options{})
	reversedInput := append([]traces.Span(nil), spans...)
	for left, right := 0, len(reversedInput)-1; left < right; left, right = left+1, right-1 {
		reversedInput[left], reversedInput[right] = reversedInput[right], reversedInput[left]
	}
	reversed := discoverObservations(t, reversedInput, Options{})
	if len(forward.Model.Edges) != 0 || len(reversed.Model.Edges) != 0 {
		t.Fatalf("conflicting duplicate identity produced an arbitrary edge: forward=%+v reversed=%+v", forward.Model.Edges, reversed.Model.Edges)
	}
	if !reflect.DeepEqual(forward, reversed) {
		t.Fatalf("reversing input changed discovery result:\nforward=%+v\nreversed=%+v", forward, reversed)
	}
	worker := serviceEvidence(t, forward, "worker")
	if !containsString(worker.Warnings, "conflicting_parent_identity") {
		t.Fatalf("conflicting parent was not disclosed: %+v", worker.Warnings)
	}
}

func TestMissingChildIdentityBlocksExecutionEdgeButKeepsDatabaseObservation(t *testing.T) {
	t.Parallel()

	client := databaseClient("trace-1", "", "worker", "main", "db.internal", "5432", "shared-fabric", "container-a")
	client.ParentSpanID = "parent"
	result := discoverObservations(t, []traces.Span{
		{TraceID: "trace-1", SpanID: "parent", Service: "gateway", Name: "call", Kind: "client"},
		client,
	}, Options{})
	for _, edge := range result.Model.Edges {
		if edge.From == "gateway" && edge.To == "worker" {
			t.Fatalf("span without a child identity created an execution edge: %+v", edge)
		}
	}
	if got := databaseEdges(result); len(got) != 1 || got[0].From != "worker" {
		t.Fatalf("valid structural database observation was lost with missing span identity: %+v", got)
	}
	if record := serviceEvidence(t, result, "worker"); !containsString(record.Warnings, "context_identity_incomplete") {
		t.Fatalf("missing context identity was not disclosed: %+v", record.Warnings)
	}
}

func TestNilAndEmptyAttributeMapsAreEquivalentDuplicateExports(t *testing.T) {
	t.Parallel()

	parent := traces.Span{TraceID: "trace-1", SpanID: "parent", Service: "gateway", Name: "call", Kind: "client"}
	parentWithEmptyMaps := parent
	parentWithEmptyMaps.Attributes = map[string]any{}
	parentWithEmptyMaps.ResourceAttributes = map[string]any{}
	child := traces.Span{TraceID: "trace-1", SpanID: "child", ParentSpanID: "parent", Service: "worker", Name: "handle", Kind: "server"}
	result := discoverObservations(t, []traces.Span{parent, parentWithEmptyMaps, child}, Options{})
	if len(result.Model.Edges) != 1 || result.Model.Edges[0].From != "gateway" || result.Model.Edges[0].To != "worker" {
		t.Fatalf("nil/empty duplicate attributes suppressed a valid edge: %+v", result.Model.Edges)
	}
}

func TestInvalidDatabaseHostAddressesAreUnresolvedAndSanitized(t *testing.T) {
	t.Parallel()

	addresses := []string{"0.0.0.0", "::", "[bad]", "db.internal:5432"}
	spans := make([]traces.Span, 0, len(addresses))
	for index, address := range addresses {
		service := "reader-" + string(rune('a'+index))
		spans = append(spans, databaseClient("trace-"+service, "span-"+service, service, "main", address, "5432", "shared-fabric", "container-"+service))
	}
	result := discoverObservations(t, spans, Options{})
	databases := databaseServices(result)
	if len(databases) != len(addresses) {
		t.Fatalf("invalid caller-scoped addresses were merged or dropped: %+v", databases)
	}
	for _, service := range databases {
		record := serviceEvidence(t, result, service.ID)
		if record.Database == nil || record.Database.IdentityKind != "unresolved_alias" || record.Database.Address != "" {
			t.Fatalf("invalid host address was resolved or published: %+v", record.Database)
		}
		if service.Metadata == nil || service.Metadata.Labels["server.address"] != "" {
			t.Fatalf("invalid address leaked into model labels: %+v", service.Metadata)
		}
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal discovered result: %v", err)
	}
	for _, address := range addresses {
		if strings.Contains(string(raw), address) {
			t.Fatalf("invalid database address %q leaked into discovered output", address)
		}
	}
}

func TestPrivateAliasObservationsRemainCallerScopedAcrossHostsAndPods(t *testing.T) {
	t.Parallel()

	first := databaseClient("trace-a", "span-a", "reader", "main", "10.20.30.40", "5432", "", "")
	first.ResourceAttributes["host.name"] = "node-a"
	first.ResourceAttributes["k8s.pod.uid"] = "pod-a"
	second := databaseClient("trace-b", "span-b", "reader", "main", "10.20.30.40", "5432", "", "")
	second.ResourceAttributes["host.name"] = "node-b"
	second.ResourceAttributes["k8s.pod.uid"] = "pod-b"
	result := discoverObservations(t, []traces.Span{first, second}, Options{})
	databases := databaseServices(result)
	if len(databases) != 2 {
		t.Fatalf("private aliases from distinct caller placements were merged: %+v", databases)
	}
	for _, service := range databases {
		record := serviceEvidence(t, result, service.ID)
		if record.Database == nil || record.Database.IdentityKind != "unresolved_alias" ||
			!strings.HasPrefix(service.Metadata.Labels["bering.network.scope"], "caller:") {
			t.Fatalf("private alias was treated as globally shared: evidence=%+v labels=%+v", record.Database, service.Metadata.Labels)
		}
	}
}

func TestCGNATAddressWithoutDeclaredScopeRemainsUnresolved(t *testing.T) {
	t.Parallel()

	span := databaseClient("trace-a", "span-a", "reader", "main", "100.64.12.34", "5432", "", "container-a")
	result := discoverObservations(t, []traces.Span{span}, Options{})
	databases := databaseServices(result)
	if len(databases) != 1 {
		t.Fatalf("expected one CGNAT database observation: %+v", databases)
	}
	record := serviceEvidence(t, result, databases[0].ID)
	if record.Database == nil || record.Database.IdentityKind != "unresolved_alias" || record.Database.Address != "100.64.12.34" ||
		!strings.HasPrefix(databases[0].Metadata.Labels["bering.network.scope"], "caller:") {
		t.Fatalf("CGNAT address was treated as globally scoped: evidence=%+v labels=%+v", record.Database, databases[0].Metadata.Labels)
	}
}

func TestReusedServiceInstanceIDAcrossPlacementsPreservesHistoryAndWarns(t *testing.T) {
	t.Parallel()

	first := traces.Span{
		TraceID: "trace-a", SpanID: "span-a", Service: "worker", Name: "work", Kind: "server",
		ResourceAttributes: map[string]any{
			"service.instance.id": "worker-1", "host.name": "node-a", "k8s.pod.uid": "pod-a",
		},
	}
	second := traces.Span{
		TraceID: "trace-b", SpanID: "span-b", Service: "worker", Name: "work", Kind: "server",
		ResourceAttributes: map[string]any{
			"service.instance.id": "worker-1", "host.name": "node-b", "k8s.pod.uid": "pod-b",
		},
	}
	result := discoverObservations(t, []traces.Span{first, second}, Options{})
	record := serviceEvidence(t, result, "worker")
	if record.Replicas == nil || record.Replicas.ObservedIdentities != 2 || len(record.Instances) != 2 {
		t.Fatalf("reused instance ID collapsed distinct placement history: replicas=%+v instances=%+v", record.Replicas, record.Instances)
	}
	if !containsString(record.Warnings, "instance_identity_has_multiple_observed_placements") {
		t.Fatalf("reused instance ID placement conflict was not disclosed: %+v", record.Warnings)
	}
}

func TestPairedDatabaseReceiverPreventsDuplicateLogicalNode(t *testing.T) {
	t.Parallel()

	client := databaseClient("trace-a", "client-a", "caller", "main", "db.internal", "5432", "shared-fabric", "caller-container")
	receiver := traces.Span{TraceID: "trace-a", SpanID: "server-a", ParentSpanID: "client-a", Service: "database-service", Name: "query", Kind: "server"}
	result := discoverObservations(t, []traces.Span{client, receiver}, Options{})
	if databases := databaseServices(result); len(databases) != 1 || databases[0].ID != "database-service" {
		t.Fatalf("paired receiver did not remain the sole database node: %+v", databases)
	}
	if len(result.Model.Services) != 2 {
		t.Fatalf("paired invocation synthesized a duplicate logical database: %+v", result.Model.Services)
	}
	edge := modelEdgeBetween(t, result, "caller", "database-service")
	record := edgeEvidence(t, result, edge.ID)
	if len(record.DatabaseAliases) != 1 || record.DatabaseAliases[0].Address != "db.internal" ||
		record.DatabaseAliases[0].NetworkScope != "declared:shared-fabric" {
		t.Fatalf("paired edge lost its logical endpoint alias: %+v", record.DatabaseAliases)
	}
}

func TestClientOnlyObservationReusesUniqueObservedDatabaseReceiver(t *testing.T) {
	t.Parallel()

	pairedClient := databaseClient("trace-a", "client-a", "caller", "main", "db.internal", "5432", "shared-fabric", "caller-container")
	receiver := traces.Span{TraceID: "trace-a", SpanID: "server-a", ParentSpanID: "client-a", Service: "database-service", Name: "query", Kind: "server"}
	clientOnly := databaseClient("trace-b", "client-b", "second-caller", "audit", "db.internal", "5432", "shared-fabric", "second-container")
	result := discoverObservations(t, []traces.Span{pairedClient, receiver, clientOnly}, Options{})
	if databases := databaseServices(result); len(databases) != 1 || databases[0].ID != "database-service" {
		t.Fatalf("unique observed receiver was not reused: %+v", databases)
	}
	edge := modelEdgeBetween(t, result, "second-caller", "database-service")
	record := edgeEvidence(t, result, edge.ID)
	if record.Relation == nil || record.Relation.Kind != "client_dependency" || len(record.DatabaseAliases) != 1 {
		t.Fatalf("reused receiver dependency evidence is incomplete: %+v", record)
	}
}

func TestAmbiguousDatabaseReceiversNeverGuessThirdCallerTarget(t *testing.T) {
	t.Parallel()

	firstClient := databaseClient("trace-a", "client-a", "caller-a", "main", "db.internal", "5432", "shared-fabric", "container-a")
	firstReceiver := traces.Span{TraceID: "trace-a", SpanID: "server-a", ParentSpanID: "client-a", Service: "database-a", Name: "query", Kind: "server"}
	secondClient := databaseClient("trace-b", "client-b", "caller-b", "main", "db.internal", "5432", "shared-fabric", "container-b")
	secondReceiver := traces.Span{TraceID: "trace-b", SpanID: "server-b", ParentSpanID: "client-b", Service: "database-b", Name: "query", Kind: "server"}
	thirdClient := databaseClient("trace-c", "client-c", "caller-c", "main", "db.internal", "5432", "shared-fabric", "container-c")
	result := discoverObservations(t, []traces.Span{firstClient, firstReceiver, secondClient, secondReceiver, thirdClient}, Options{})
	if databases := databaseServices(result); len(databases) != 2 {
		t.Fatalf("ambiguous receivers synthesized a third database node: %+v", databases)
	}
	modelEdgeBetween(t, result, "caller-a", "database-a")
	modelEdgeBetween(t, result, "caller-b", "database-b")
	for _, edge := range result.Model.Edges {
		if edge.From == "caller-c" {
			t.Fatalf("ambiguous endpoint guessed a target for the unpaired caller: %+v", edge)
		}
	}
	record := serviceEvidence(t, result, "caller-c")
	if len(record.DatabaseAliases) != 1 || !containsString(record.Warnings, "database_endpoint_has_multiple_observed_receivers") {
		t.Fatalf("ambiguous endpoint evidence was not retained on the caller: %+v", record)
	}
}

func TestGeneratedDatabaseReplicaOverrideAndUnknownIDValidation(t *testing.T) {
	t.Parallel()

	spans := []traces.Span{databaseClient("trace-a", "client-a", "caller", "main", "db.internal", "5432", "shared-fabric", "container-a")}
	initial := discoverObservations(t, spans, Options{})
	databases := databaseServices(initial)
	if len(databases) != 1 {
		t.Fatalf("expected one generated database node: %+v", databases)
	}
	configured := discoverObservations(t, spans, Options{ReplicasOverride: map[string]int{databases[0].ID: 3}})
	service := modelService(t, configured, databases[0].ID)
	record := serviceEvidence(t, configured, databases[0].ID)
	if service.Replicas != 3 || record.Replicas == nil || record.Replicas.Configured == nil ||
		*record.Replicas.Configured != 3 || record.Replicas.Basis != "supplied_overlay" {
		t.Fatalf("generated database replica override was not applied with provenance: service=%+v evidence=%+v", service, record.Replicas)
	}

	_, err := Discover(spans, Options{
		DiscoveredAt: "2026-01-01T00:00:00Z", SourceRef: "synthetic://observations",
		ReplicasOverride: map[string]int{"unknown-database": 2},
	})
	if err == nil || !strings.Contains(err.Error(), "unknown service") {
		t.Fatalf("unknown replica override was not rejected precisely: %v", err)
	}
}

func TestObservationEvidenceAttributeIsReservedFromOverlays(t *testing.T) {
	t.Parallel()

	spans := []traces.Span{
		{TraceID: "trace-a", SpanID: "parent", Service: "caller", Name: "call", Kind: "client"},
		{TraceID: "trace-a", SpanID: "child", ParentSpanID: "parent", Service: "worker", Name: "work", Kind: "server"},
	}
	initial := discoverObservations(t, spans, Options{})
	edge := modelEdgeBetween(t, initial, "caller", "worker")
	cases := []struct {
		name    string
		overlay overlay.File
	}{
		{
			name: "service",
			overlay: overlay.File{Name: "reserved-service", Services: []overlay.ServiceOverlay{{
				ID: "worker", CommonMetadata: overlay.CommonMetadata{Attributes: map[string]string{evidence.AttributeKey: "replacement"}},
			}}},
		},
		{
			name: "edge",
			overlay: overlay.File{Name: "reserved-edge", Edges: []overlay.EdgeOverlay{{
				ID: edge.ID, CommonMetadata: overlay.CommonMetadata{Attributes: map[string]string{evidence.AttributeKey: "replacement"}},
			}}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Discover(spans, Options{
				DiscoveredAt: "2026-01-01T00:00:00Z", SourceRef: "synthetic://observations", Overlays: []overlay.File{tc.overlay},
			})
			if err == nil || !strings.Contains(err.Error(), "reserved generated evidence attribute") || !strings.Contains(err.Error(), evidence.AttributeKey) {
				t.Fatalf("reserved evidence overwrite was not rejected precisely: %v", err)
			}
		})
	}
}

func discoverObservations(t *testing.T, spans []traces.Span, opts Options) Result {
	t.Helper()
	opts.DiscoveredAt = "2026-01-01T00:00:00Z"
	if opts.SourceRef == "" {
		opts.SourceRef = "synthetic://observations"
	}
	result, err := Discover(spans, opts)
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	return result
}

func observedSpan(traceID, spanID, service, hostID, pid, processStart string) traces.Span {
	return traces.Span{
		TraceID: traceID, SpanID: spanID, Service: service, Name: "work", Kind: "server",
		ScopeName: "generic-instrumentation", ScopeVersion: "1.0.0",
		Attributes: map[string]any{},
		ResourceAttributes: map[string]any{
			"host.id": hostID, "process.pid": pid, "process.creation.time": processStart,
		},
	}
}

func databaseClient(traceID, spanID, service, namespace, address, port, networkScope, container string) traces.Span {
	span := traces.Span{
		TraceID: traceID, SpanID: spanID, Service: service, Name: "query", Kind: "client",
		ScopeName: "generic-database-instrumentation", ScopeVersion: "1.0.0",
		Attributes: map[string]any{
			"db.system.name": "sql", "db.namespace": namespace,
			"server.address": address, "server.port": port,
		},
		ResourceAttributes: map[string]any{
			"service.instance.id": service + "-instance", "container.id": container,
		},
	}
	if networkScope != "" {
		span.ResourceAttributes["bering.network.scope"] = networkScope
	}
	return span
}

func modelService(t *testing.T, result Result, id string) model.Service {
	t.Helper()
	for _, item := range result.Model.Services {
		if item.ID == id {
			return item
		}
	}
	t.Fatalf("model service %q not found", id)
	return model.Service{}
}

func modelEdgeBetween(t *testing.T, result Result, from, to string) model.Edge {
	t.Helper()
	for _, item := range result.Model.Edges {
		if item.From == from && item.To == to {
			return item
		}
	}
	t.Fatalf("model edge %q -> %q not found: %+v", from, to, result.Model.Edges)
	return model.Edge{}
}

func serviceEvidence(t *testing.T, result Result, id string) evidence.Record {
	t.Helper()
	for _, item := range result.Discovery.Services {
		if item.ID == id {
			return decodeObservationEvidence(t, item.Metadata)
		}
	}
	t.Fatalf("service discovery record %q not found", id)
	return evidence.Record{}
}

func edgeEvidence(t *testing.T, result Result, id string) evidence.Record {
	t.Helper()
	for _, item := range result.Discovery.Edges {
		if item.ID == id {
			if item.Metadata == nil {
				t.Fatalf("edge %q has no metadata", id)
			}
			return decodeEncodedEvidence(t, item.Metadata.Attributes[evidence.AttributeKey])
		}
	}
	t.Fatalf("edge discovery record %q not found", id)
	return evidence.Record{}
}

func decodeObservationEvidence(t *testing.T, metadata *snapshot.ServiceMetadata) evidence.Record {
	t.Helper()
	if metadata == nil {
		t.Fatal("service has no metadata")
	}
	return decodeEncodedEvidence(t, metadata.Attributes[evidence.AttributeKey])
}

func decodeEncodedEvidence(t *testing.T, value string) evidence.Record {
	t.Helper()
	if value == "" {
		t.Fatal("observation evidence attribute is missing")
	}
	record, err := evidence.Decode(value)
	if err != nil {
		t.Fatalf("decode observation evidence: %v", err)
	}
	return record
}

func databaseServices(result Result) []model.Service {
	out := make([]model.Service, 0)
	for _, item := range result.Model.Services {
		if item.Metadata != nil && item.Metadata.Labels["bering.node.kind"] == "database" {
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func databaseEdges(result Result) []model.Edge {
	out := make([]model.Edge, 0)
	for _, item := range result.Model.Edges {
		if item.Metadata != nil && item.Metadata.Labels["bering.relation.kind"] == "client_dependency" {
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
