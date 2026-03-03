package discovery

import (
	"testing"

	"github.com/MB3R-Lab/Bering/internal/connectors/traces"
	"github.com/MB3R-Lab/Bering/internal/model"
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
