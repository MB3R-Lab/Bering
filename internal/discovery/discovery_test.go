package discovery

import (
	"testing"
	"time"

	"github.com/MB3R-Lab/Bering/internal/connectors/traces"
	"github.com/MB3R-Lab/Bering/internal/model"
	"github.com/MB3R-Lab/Bering/internal/overlay"
	"github.com/MB3R-Lab/Bering/internal/schema"
)

func TestBuild_BasicInference(t *testing.T) {
	t.Parallel()

	spans := []traces.Span{
		{
			TraceID: "t1", SpanID: "1", Service: "frontend", Name: "GET /checkout", Kind: "server",
			Attributes: map[string]any{"http.request.method": "GET", "http.route": "/checkout"},
		},
		{
			TraceID: "t1", SpanID: "2", ParentSpanID: "1", Service: "checkout", Kind: "server",
			Attributes: map[string]any{"http.request.method": "POST", "http.route": "/process"},
		},
		{
			TraceID: "t1", SpanID: "3", ParentSpanID: "2", Service: "inventory", Kind: "consumer",
			Attributes: map[string]any{"messaging.system": "kafka"},
		},
	}

	mdl, err := Build(spans, Options{
		SourceRef:    BuildSourceRef("examples/traces/normalized.sample.json"),
		DiscoveredAt: "2026-03-03T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if got, want := len(mdl.Services), 3; got != want {
		t.Fatalf("services mismatch: got=%d want=%d", got, want)
	}
	if got, want := len(mdl.Edges), 2; got != want {
		t.Fatalf("edges mismatch: got=%d want=%d", got, want)
	}
	if got, want := len(mdl.Endpoints), 2; got != want {
		t.Fatalf("endpoints mismatch: got=%d want=%d", got, want)
	}

	if mdl.Metadata.Schema.Name != schema.ExpectedSchemaName {
		t.Fatalf("schema.name mismatch: got=%s", mdl.Metadata.Schema.Name)
	}
	if mdl.Metadata.SourceType != SourceTypeBering {
		t.Fatalf("source_type mismatch: got=%s", mdl.Metadata.SourceType)
	}

	foundAsync := false
	for _, edge := range mdl.Edges {
		if edge.Kind == model.EdgeKindAsync {
			foundAsync = true
			if edge.Blocking {
				t.Fatalf("async edge must be non-blocking: %+v", edge)
			}
		}
	}
	if !foundAsync {
		t.Fatalf("expected at least one async edge")
	}
}

func TestBuild_UnknownReplicaOverrideFails(t *testing.T) {
	t.Parallel()

	spans := []traces.Span{
		{TraceID: "t1", SpanID: "1", Service: "frontend", Name: "GET /health", Kind: "server", Attributes: map[string]any{"http.request.method": "GET", "http.route": "/health"}},
	}

	_, err := Build(spans, Options{
		SourceRef:        BuildSourceRef("examples/traces/normalized.sample.json"),
		DiscoveredAt:     "2026-03-03T00:00:00Z",
		ReplicasOverride: map[string]int{"unknown": 2},
	})
	if err == nil {
		t.Fatal("expected unknown override service error, got nil")
	}
}

func TestDiscover_OverlayAppliesRuntimeMetadata(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC)
	replicas := 3
	weight := 0.7
	spans := []traces.Span{
		{
			TraceID: "t1", SpanID: "1", Service: "checkout", Name: "POST /process", Kind: "server",
			StartTime: now, EndTime: now.Add(50 * time.Millisecond),
			Attributes: map[string]any{"http.request.method": "POST", "http.route": "/process"},
		},
	}

	result, err := Discover(spans, Options{
		SourceRef:    BuildSourceRef("examples/traces/normalized.sample.json"),
		DiscoveredAt: "2026-03-03T00:00:00Z",
		RuntimeMode:  true,
		Overlays: []overlay.File{{
			Name:      "test",
			Services:  []overlay.ServiceOverlay{{ID: "checkout", Replicas: &replicas, FailureEligible: boolPtr(true), CommonMetadata: overlay.CommonMetadata{Attributes: map[string]string{"team": "commerce"}}}},
			Endpoints: []overlay.EndpointOverlay{{ID: "checkout:POST /process", PredicateRef: "catalog.checkout.success", Weight: &weight, CommonMetadata: overlay.CommonMetadata{Attributes: map[string]string{"verb": "write"}}}},
		}},
	})
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}

	if got := result.Model.Services[0].Replicas; got != 3 {
		t.Fatalf("replicas override mismatch: got=%d", got)
	}
	if got := result.Model.Services[0].Metadata.FailureEligible; got == nil || !*got {
		t.Fatal("failure_eligible was not propagated into the model")
	}
	if got := result.Model.Endpoints[0].SuccessPredicateRef; got != "catalog.checkout.success" {
		t.Fatalf("predicate ref mismatch: got=%s", got)
	}
	if got := result.Discovery.Services[0].FirstSeen; got == "" {
		t.Fatal("expected runtime first_seen to be populated")
	}
	if got := result.Discovery.Endpoints[0].Metadata.PredicateRef; got != "catalog.checkout.success" {
		t.Fatalf("endpoint metadata predicate mismatch: got=%s", got)
	}
	if got := result.Discovery.Services[0].Metadata.Attributes["team"]; got != "commerce" {
		t.Fatalf("service metadata attribute mismatch: got=%s", got)
	}
	if got := result.Discovery.Endpoints[0].Metadata.Attributes["verb"]; got != "write" {
		t.Fatalf("endpoint metadata attribute mismatch: got=%s", got)
	}
}

func TestDiscover_ComputesObservedLatencySummaryFromTraceTiming(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC)
	spans := []traces.Span{
		{
			TraceID: "t1", SpanID: "frontend-server", Service: "frontend", Name: "GET /checkout", Kind: "server",
			StartTime: start, EndTime: start.Add(5 * time.Millisecond),
			Attributes: map[string]any{"http.request.method": "GET", "http.route": "/checkout"},
		},
		{
			TraceID: "t1", SpanID: "checkout-server", ParentSpanID: "frontend-server", Service: "checkout", Kind: "server",
			StartTime: start.Add(5 * time.Millisecond), EndTime: start.Add(35 * time.Millisecond),
			Attributes: map[string]any{"http.request.method": "POST", "http.route": "/process"},
		},
		{
			TraceID: "t2", SpanID: "frontend-server-2", Service: "frontend", Name: "GET /checkout", Kind: "server",
			StartTime: start.Add(time.Second), EndTime: start.Add(time.Second + 5*time.Millisecond),
			Attributes: map[string]any{"http.request.method": "GET", "http.route": "/checkout"},
		},
		{
			TraceID: "t2", SpanID: "checkout-server-2", ParentSpanID: "frontend-server-2", Service: "checkout", Kind: "server",
			StartTime: start.Add(time.Second + 5*time.Millisecond), EndTime: start.Add(time.Second + 75*time.Millisecond),
			Attributes: map[string]any{"http.request.method": "POST", "http.route": "/process"},
		},
	}

	result, err := Discover(spans, Options{
		SourceRef:    BuildSourceRef("examples/traces/normalized.sample.json"),
		DiscoveredAt: "2026-03-11T00:00:00Z",
		RuntimeMode:  true,
	})
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}

	if got := result.Model.Edges[0].Observed; got == nil || got.LatencyMS == nil {
		t.Fatal("expected observed latency summary on discovered model edge")
	}
	if got, want := *result.Model.Edges[0].Observed.LatencyMS.P50, 30.0; got != want {
		t.Fatalf("p50 mismatch: got=%v want=%v", got, want)
	}
	if got, want := *result.Model.Edges[0].Observed.LatencyMS.P99, 70.0; got != want {
		t.Fatalf("p99 mismatch: got=%v want=%v", got, want)
	}
	if got := result.Discovery.Edges[0].Observed; got == nil || got.LatencyMS == nil {
		t.Fatal("expected observed latency summary on discovery edge record")
	}
}

func boolPtr(v bool) *bool {
	return &v
}
