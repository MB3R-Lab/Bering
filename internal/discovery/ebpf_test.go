package discovery

import (
	"testing"

	"github.com/MB3R-Lab/Bering/internal/connectors/traces"
	"github.com/MB3R-Lab/Bering/internal/model"
)

func ebpfClient(traceID, spanID, service string) traces.Span {
	return traces.Span{
		TraceID: traceID, SpanID: spanID, Service: service,
		Kind: "client", Name: "GET /items",
		SourceKind: "ebpf", SourceDomain: "ebpf:installation", SourceRef: "synthetic://collector",
		Attributes:         map[string]any{"http.request.method": "GET", "url.path": "/items"},
		ResourceAttributes: map[string]any{"service.name": service},
	}
}

func peerEdges(result Result) []model.Edge {
	out := []model.Edge{}
	for _, edge := range result.Model.Edges {
		if edge.Metadata != nil && edge.Metadata.Labels["bering.relation.kind"] == "client_dependency" && edge.Identity != nil && edge.Identity.Protocol == "http" {
			out = append(out, edge)
		}
	}
	return out
}

func TestEBPFClientOnlyNamedPeerHasStructuralEvidence(t *testing.T) {
	client := ebpfClient("trace", "client", "reader")
	client.Attributes["peer.service"] = "catalog"
	result := discoverObservations(t, []traces.Span{client}, Options{})
	if len(peerEdges(result)) != 1 || peerEdges(result)[0].To == "catalog" {
		t.Fatalf("observed named peer dependency absent: %+v", result.Model.Edges)
	}
	edge := peerEdges(result)[0]
	record := edgeEvidence(t, result, edge.ID)
	if record.Relation == nil || record.Relation.Kind != "client_dependency" || record.Peer == nil || record.Peer.Service != "catalog" ||
		!containsString(record.Warnings, "server_receipt_unobserved") || !containsString(record.Warnings, "execution_context_unconfirmed") {
		t.Fatalf("structural evidence was mistaken for execution: %+v", record)
	}
	if len(record.Relation.Sources) != 1 || record.Relation.Sources[0].Kind != "ebpf" || record.Relation.Sources[0].Domain != "ebpf:installation" {
		t.Fatalf("source kind/domain provenance was lost: %+v", record.Relation.Sources)
	}
	service := serviceEvidence(t, result, edge.To)
	if service.Peer == nil || service.Peer.IdentityKind != "reported_service" || service.Replicas == nil || service.Replicas.Basis != "logical_peer_placeholder" {
		t.Fatalf("peer node identity or replica uncertainty missing: %+v", service)
	}
}

func TestNamedCollectorProvenanceReachesPeerEdgeAndEvidence(t *testing.T) {
	client := ebpfClient("trace", "client", "reader")
	client.SourceConnector = "collector-b"
	client.SourceDomain = "ebpf:collector-b:installation"
	client.SourceRef = "receiver-b"
	client.Attributes["peer.service"] = "catalog"
	result := discoverObservations(t, []traces.Span{client}, Options{})
	edges := peerEdges(result)
	if len(edges) != 1 {
		t.Fatalf("expected one structural edge: %+v", result.Model.Edges)
	}
	var edgeConnector, peerConnector string
	for _, record := range result.Discovery.Edges {
		if record.ID == edges[0].ID && len(record.Provenance) == 1 {
			edgeConnector = record.Provenance[0].Connector
		}
	}
	for _, record := range result.Discovery.Services {
		if record.ID == edges[0].To && len(record.Provenance) == 1 {
			peerConnector = record.Provenance[0].Connector
		}
	}
	if edgeConnector != "collector-b" || peerConnector != "collector-b" {
		t.Fatalf("named collector missing from graph provenance: edge=%q peer=%q", edgeConnector, peerConnector)
	}
	record := edgeEvidence(t, result, edges[0].ID)
	if record.Relation == nil || len(record.Relation.Sources) != 1 || record.Relation.Sources[0].Connector != "collector-b" {
		t.Fatalf("named collector missing from warning/evidence record: %+v", record)
	}
	if !containsString(record.Warnings, "server_receipt_unobserved") {
		t.Fatalf("structural warning missing: %+v", record)
	}
}

func TestEBPFHTTPFullURLProducesPathOnlyPeerEdge(t *testing.T) {
	client := ebpfClient("trace", "client", "reader")
	client.Name = "outbound request"
	client.Attributes = map[string]any{
		"http.request.method": "GET",
		"url.full":            "https://person:secret@api.internal:8443/items?token=hidden",
		"server.address":      "api.internal", "network.peer.address": "192.0.2.10",
		"server.port": 8443,
	}
	client.ResourceAttributes["bering.network.scope"] = "network-a"
	client.Attributes["bering.network.scope"] = "network-a"
	result := discoverObservations(t, []traces.Span{client}, Options{})
	edges := peerEdges(result)
	if len(edges) != 1 || edges[0].Identity.Route != "/items" || edges[0].Identity.Operation != "GET" {
		t.Fatalf("full URL did not produce a path-only HTTP dependency: %+v", edges)
	}
	peer := serviceEvidence(t, result, edges[0].To).Peer
	if peer == nil || peer.Address != "api.internal" || peer.Port == nil || *peer.Port != 8443 {
		t.Fatalf("compatible DNS and socket IP fields were rejected or merged as physical identity: %+v", peer)
	}
	client.Attributes["url.full"] = "https://person:secret@api.internal/%zz?token=hidden"
	result = discoverObservations(t, []traces.Span{client}, Options{})
	if len(peerEdges(result)) != 0 {
		t.Fatalf("malformed full URL leaked into route: %+v", result.Model.Edges)
	}
}

func TestEBPFAddressPeerRequiresValidPortAndScope(t *testing.T) {
	first := ebpfClient("trace-a", "a", "reader")
	second := ebpfClient("trace-b", "b", "writer")
	for _, span := range []*traces.Span{&first, &second} {
		span.Attributes["server.address"] = "api.internal"
		span.Attributes["server.port"] = int64(8080)
		span.ResourceAttributes["bering.network.scope"] = "network-a"
		span.Attributes["bering.network.scope"] = "network-a"
	}
	result := discoverObservations(t, []traces.Span{first, second}, Options{})
	edges := peerEdges(result)
	if len(edges) != 2 || edges[0].To != edges[1].To {
		t.Fatalf("same scoped endpoint did not merge: %+v", edges)
	}
	peer := serviceEvidence(t, result, edges[0].To).Peer
	if peer == nil || peer.IdentityKind != "logical_endpoint" || peer.NetworkScope != "declared:network-a" || peer.Port == nil || *peer.Port != 8080 {
		t.Fatalf("scoped endpoint evidence missing: %+v", peer)
	}

	third := ebpfClient("trace-c", "c", "writer")
	third.Attributes["server.address"] = "api.internal"
	third.Attributes["server.port"] = int64(8080)
	result = discoverObservations(t, []traces.Span{first, third}, Options{})
	edges = peerEdges(result)
	if len(edges) != 2 || edges[0].To == edges[1].To {
		t.Fatalf("unscoped private endpoint merged with scoped endpoint: %+v", edges)
	}
	for _, edge := range edges {
		if edge.From == "writer" && serviceEvidence(t, result, edge.To).Peer.IdentityKind != "unresolved_alias" {
			t.Fatalf("unscoped endpoint not caller-scoped: %+v", edge)
		}
	}
}

func TestEBPFExactReceiverOrConflictingReceiverPreventsPeerSynthesis(t *testing.T) {
	client := ebpfClient("trace", "client", "reader")
	client.Attributes["peer.service"] = "reported-peer"
	receiver := traces.Span{
		TraceID: "trace", SpanID: "server", ParentSpanID: "client", Service: "actual-server",
		Kind: "server", SourceKind: "ebpf", SourceDomain: client.SourceDomain,
		Attributes: map[string]any{"http.request.method": "GET", "url.path": "/items"},
	}
	result := discoverObservations(t, []traces.Span{client, receiver}, Options{})
	if len(peerEdges(result)) != 0 {
		t.Fatalf("peer node duplicated an exact server receipt: %+v", result.Model.Edges)
	}
	if len(result.Model.Edges) != 1 || result.Model.Edges[0].To != "actual-server" {
		t.Fatalf("exact execution edge missing: %+v", result.Model.Edges)
	}

	other := receiver
	other.Service = "conflicting-server"
	result = discoverObservations(t, []traces.Span{client, receiver, other}, Options{})
	if len(peerEdges(result)) != 0 {
		t.Fatalf("ambiguous receiver was replaced by a selected peer: %+v", result.Model.Edges)
	}
}

func TestEBPFMissingContextIsStructuralAndConflictingPortsDoNotSelect(t *testing.T) {
	client := ebpfClient("", "", "reader")
	client.Attributes["peer.service"] = "catalog"
	result := discoverObservations(t, []traces.Span{client}, Options{})
	if len(peerEdges(result)) != 1 || !containsString(edgeEvidence(t, result, peerEdges(result)[0].ID).Warnings, "execution_context_unconfirmed") {
		t.Fatalf("missing context was certified or structural evidence lost: %+v", result.Model.Edges)
	}
	client = ebpfClient("trace", "client", "reader")
	client.Attributes["server.address"] = "api.internal"
	client.Attributes["server.port"] = 8080
	client.Attributes["net.peer.port"] = 9090
	result = discoverObservations(t, []traces.Span{client}, Options{})
	if len(peerEdges(result)) != 0 || !containsString(serviceEvidence(t, result, "reader").Warnings, "peer_identity_unresolved") {
		t.Fatalf("conflicting port variants selected a peer: %+v", result)
	}
}

func TestEBPFDoesNotInventFromRawTCPOrNativePeer(t *testing.T) {
	native := ebpfClient("trace-a", "a", "reader")
	native.SourceKind, native.SourceDomain = "", ""
	native.Attributes["peer.service"] = "catalog"
	tcp := ebpfClient("trace-b", "b", "reader")
	tcp.Name = "TCP connect"
	tcp.Attributes = map[string]any{"network.protocol.name": "tcp", "peer.service": "catalog"}
	result := discoverObservations(t, []traces.Span{native, tcp}, Options{})
	if len(peerEdges(result)) != 0 {
		t.Fatalf("native or raw TCP peer became an eBPF application dependency: %+v", result.Model.Edges)
	}
}

func TestEBPFReusesEquivalentExecutionEdgeWithoutDuplicate(t *testing.T) {
	paired := ebpfClient("trace-a", "client-a", "reader")
	paired.Attributes["peer.service"] = "catalog"
	receiver := traces.Span{
		TraceID: "trace-a", SpanID: "server-a", ParentSpanID: "client-a", Service: "catalog",
		Kind: "server", SourceKind: "ebpf", SourceDomain: paired.SourceDomain,
		Attributes: map[string]any{"http.request.method": "GET", "url.path": "/items"},
	}
	unpaired := ebpfClient("trace-b", "client-b", "reader")
	unpaired.Attributes["peer.service"] = "catalog"
	result := discoverObservations(t, []traces.Span{paired, receiver, unpaired}, Options{})
	if len(result.Model.Edges) != 1 || len(peerEdges(result)) != 0 {
		t.Fatalf("existing execution edge was duplicated by client-only peer evidence: %+v", result.Model.Edges)
	}
}

func TestEBPFConflictingIdentityAttributesAndSourceDomains(t *testing.T) {
	first := ebpfClient("same-trace", "same-span", "reader")
	first.Attributes["server.address"] = "api.internal"
	first.Attributes["server.port"] = 8080
	first.ResourceAttributes["bering.network.scope"] = "network-a"
	first.Attributes["bering.network.scope"] = "network-a"
	second := first
	second.SourceDomain = "ebpf:other-installation"
	result := discoverObservations(t, []traces.Span{first, second}, Options{})
	if len(peerEdges(result)) != 2 || peerEdges(result)[0].To == peerEdges(result)[1].To {
		t.Fatalf("same opaque ID or endpoint address crossed collection domains: %+v", result.Model.Edges)
	}

	conflicted := first
	conflicted.AttributeConflicts = []string{"span:server.address"}
	result = discoverObservations(t, []traces.Span{conflicted}, Options{})
	if len(peerEdges(result)) != 0 {
		t.Fatalf("conflicted address generated peer: %+v", result.Model.Edges)
	}
	conflicted = first
	conflicted.Attributes = map[string]any{}
	for key, value := range first.Attributes {
		conflicted.Attributes[key] = value
	}
	conflicted.Attributes["bering.network.scope"] = "network-b"
	result = discoverObservations(t, []traces.Span{conflicted}, Options{})
	if len(peerEdges(result)) != 0 {
		t.Fatalf("conflicting resource and span scope selected a peer: %+v", result.Model.Edges)
	}
}

func TestEBPFNamedPeersAreScopedByCollectionDomain(t *testing.T) {
	first := ebpfClient("trace-a", "a", "reader")
	first.Attributes["peer.service"] = "catalog"
	second := ebpfClient("trace-b", "b", "writer")
	second.SourceDomain = "ebpf:other-installation"
	second.Attributes["peer.service"] = "catalog"
	result := discoverObservations(t, []traces.Span{first, second}, Options{})
	edges := peerEdges(result)
	if len(edges) != 2 || edges[0].To == edges[1].To || edges[0].To == "catalog" || edges[1].To == "catalog" {
		t.Fatalf("same reported name crossed collection domains: %+v", edges)
	}
	for _, edge := range edges {
		if peer := serviceEvidence(t, result, edge.To).Peer; peer == nil || peer.Service != "catalog" {
			t.Fatalf("reported name lost from scoped alias evidence: %+v", edge)
		}
	}
}

func TestEBPFNamedPeerReusesOnlyUniqueMatchingObservedService(t *testing.T) {
	client := ebpfClient("trace-a", "a", "reader")
	client.Attributes["peer.service"] = "catalog"
	client.Attributes["peer.service.namespace"] = "shop"
	client.Attributes["bering.network.scope"] = "network-a"
	client.ResourceAttributes["bering.network.scope"] = "network-a"
	observed := traces.Span{
		TraceID: "trace-b", SpanID: "b", Service: "catalog", Kind: "server",
		SourceKind: "ebpf", SourceDomain: client.SourceDomain,
		ResourceAttributes: map[string]any{
			"service.name": "catalog", "service.namespace": "shop", "bering.network.scope": "network-a",
		},
	}
	result := discoverObservations(t, []traces.Span{client, observed}, Options{})
	if edges := peerEdges(result); len(edges) != 1 || edges[0].To != "catalog" {
		t.Fatalf("unique matching observed service was not reused: %+v", edges)
	}

	wrongDomain := observed
	wrongDomain.SourceDomain = "ebpf:other-installation"
	result = discoverObservations(t, []traces.Span{client, wrongDomain}, Options{})
	if edges := peerEdges(result); len(edges) != 1 || edges[0].To == "catalog" {
		t.Fatalf("same name from wrong domain was reused: %+v", edges)
	}

	wrongNamespace := observed
	wrongNamespace.ResourceAttributes = map[string]any{
		"service.name": "catalog", "service.namespace": "other", "bering.network.scope": "network-a",
	}
	result = discoverObservations(t, []traces.Span{client, wrongNamespace}, Options{})
	if edges := peerEdges(result); len(edges) != 1 || edges[0].To == "catalog" {
		t.Fatalf("same name from wrong namespace was reused: %+v", edges)
	}

	secondScope := observed
	secondScope.TraceID, secondScope.SpanID = "trace-c", "c"
	secondScope.ResourceAttributes = map[string]any{
		"service.name": "catalog", "service.namespace": "shop", "bering.network.scope": "network-b",
	}
	result = discoverObservations(t, []traces.Span{client, observed, secondScope}, Options{})
	if edges := peerEdges(result); len(edges) != 1 || edges[0].To == "catalog" {
		t.Fatalf("multiply scoped raw service was reused arbitrarily: %+v", edges)
	}
}
