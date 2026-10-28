package discovery

import (
	"testing"

	"github.com/MB3R-Lab/Bering/internal/connectors/traces"
)

func TestCollectionDomainSeparatesAncestryButNotAgents(t *testing.T) {
	for _, test := range []struct {
		name, parentDomain, childDomain string
		edges                           int
	}{
		{"legacy", "", "", 1},
		{"two-agents-one-deployment", "ebpf:shared", "ebpf:shared", 1},
		{"different-deployments", "ebpf:one", "ebpf:two", 0},
		{"native-and-ebpf", "", "ebpf:one", 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			spans := []traces.Span{
				{TraceID: "00000000000000000000000000000001", SpanID: "0000000000000001", Service: "edge-api", Kind: "server", SourceDomain: test.parentDomain,
					Attributes:         map[string]any{"http.request.method": "GET", "http.route": "/call"},
					ResourceAttributes: map[string]any{"host.id": "host-a", "service.instance.id": "edge-1"}},
				{TraceID: "00000000000000000000000000000001", SpanID: "0000000000000002", ParentSpanID: "0000000000000001", Service: "lookup-api", Kind: "server", SourceDomain: test.childDomain,
					Attributes:         map[string]any{"http.request.method": "GET", "http.route": "/lookup"},
					ResourceAttributes: map[string]any{"host.id": "host-b", "service.instance.id": "lookup-1"}},
			}
			result, err := Discover(spans, Options{DiscoveredAt: "2026-01-01T00:00:00Z"})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Model.Edges) != test.edges {
				t.Fatalf("edges=%v", result.Model.Edges)
			}
			if spans[1].ParentSpanID != "0000000000000001" {
				t.Fatal("source ID rewritten")
			}
		})
	}
}

func TestDomainKeyCannotCollideWithOpaqueNativeID(t *testing.T) {
	trace := "00000000000000000000000000000001"
	if traceSpanKey("ebpf:prod|"+trace, "child") == traceSpanKey(trace, "child", "ebpf:prod") {
		t.Fatal("opaque native ID collides with source domain")
	}
	spans := []traces.Span{
		{TraceID: "ebpf:prod|" + trace, SpanID: "parent", Service: "native-api", Kind: "server"},
		{TraceID: trace, SpanID: "child", ParentSpanID: "parent", SourceKind: "ebpf", SourceDomain: "ebpf:prod", Service: "agent-api", Kind: "server"},
	}
	result, err := Discover(spans, Options{DiscoveredAt: "2026-01-01T00:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Model.Edges) != 0 {
		t.Fatalf("cross-domain edge=%+v", result.Model.Edges)
	}
}

func TestEBPFInboundEndpointsRequireServerObservation(t *testing.T) {
	for _, kind := range []string{"client", ""} {
		span := traces.Span{Service: "edge-api", SourceKind: "ebpf", Kind: kind, Attributes: map[string]any{"http.request.method": "GET", "http.route": "/outgoing"}}
		if _, ok := inferEndpoint(span, traces.Span{}, false); ok {
			t.Fatalf("%q creates inbound endpoint", kind)
		}
	}
	span := traces.Span{Service: "lookup-api", SourceKind: "ebpf", Kind: "server", Attributes: map[string]any{"rpc.system": "grpc", "rpc.service": "demo.Lookup", "rpc.method": "Lookup"}}
	endpoint, ok := inferEndpoint(span, traces.Span{}, false)
	if !ok || endpoint.Method != "GRPC" || endpoint.Path != "/demo.Lookup/Lookup" {
		t.Fatalf("endpoint=%+v", endpoint)
	}
}

func TestEndpointProvenanceIsNotServiceWide(t *testing.T) {
	spans := []traces.Span{
		{Service: "api", Kind: "server", SourceKind: "ebpf", SourceDomain: "ebpf:one", SourceRef: "agent", Attributes: map[string]any{"http.request.method": "GET", "http.route": "/a"}},
		{Service: "api", Kind: "server", SourceRef: "native", Attributes: map[string]any{"http.request.method": "GET", "http.route": "/b"}},
	}
	result, err := Discover(spans, Options{DiscoveredAt: "2026-01-01T00:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range result.Discovery.Endpoints {
		if len(endpoint.Provenance) != 1 {
			t.Fatalf("mixed endpoint provenance %+v", endpoint)
		}
		if endpoint.Path == "/a" && endpoint.Provenance[0].Type != "ebpf" {
			t.Fatalf("missing agent provenance %+v", endpoint)
		}
		if endpoint.Path == "/b" && endpoint.Provenance[0].Ref != "native" {
			t.Fatalf("wrong native provenance %+v", endpoint)
		}
	}
}

func TestUnresolvedDatabaseDoesNotMergeCollectionDomains(t *testing.T) {
	spans := []traces.Span{}
	for _, domain := range []string{"ebpf:one", "ebpf:two"} {
		spans = append(spans, traces.Span{Service: "api", Kind: "client", SourceKind: "ebpf", SourceDomain: domain,
			Attributes: map[string]any{"db.system.name": "postgresql", "server.address": "10.0.0.8", "server.port": 5432}})
	}
	result, err := Discover(spans, Options{DiscoveredAt: "2026-01-01T00:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Model.Services) != 3 || len(result.Model.Edges) != 2 {
		t.Fatalf("unexpected merged graph %+v", result.Model)
	}
	for _, service := range result.Discovery.Services {
		if service.ID != "api" && (len(service.Provenance) != 1 || service.Provenance[0].Type != "ebpf") {
			t.Fatalf("wrong generated DB provenance %+v", service)
		}
	}
}
