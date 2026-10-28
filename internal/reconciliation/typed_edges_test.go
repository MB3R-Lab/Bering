package reconciliation

import (
	"testing"
	"time"

	"github.com/MB3R-Lab/Bering/internal/connectors/traces"
	"github.com/MB3R-Lab/Bering/internal/discovery"
	"github.com/MB3R-Lab/Bering/internal/model"
	"github.com/MB3R-Lab/Bering/internal/snapshot"
)

func TestDiscoveredParallelTypedEdgesSurviveReconciliation(t *testing.T) {
	for _, sourceKind := range []string{"", "ebpf"} {
		t.Run("source="+sourceKind, func(t *testing.T) {
			start := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
			spans := []traces.Span{
				{TraceID: "request", SpanID: "root", Service: "edge-api", Kind: "server", SourceKind: sourceKind,
					Attributes: map[string]any{"http.request.method": "GET", "http.route": "/call"}},
				{TraceID: "request", SpanID: "read", ParentSpanID: "root", Service: "lookup-api", Kind: "server", SourceKind: sourceKind,
					Attributes: map[string]any{"http.request.method": "GET", "http.route": "/items"}},
				{TraceID: "request", SpanID: "write", ParentSpanID: "root", Service: "lookup-api", Kind: "server", SourceKind: sourceKind,
					Attributes: map[string]any{"http.request.method": "POST", "http.route": "/items"}},
			}
			discovered, err := discovery.Discover(spans, discovery.Options{DiscoveredAt: start.Format(time.RFC3339)})
			if err != nil {
				t.Fatal(err)
			}
			if len(discovered.Model.Edges) != 2 || discovered.Model.Edges[0].ID == discovered.Model.Edges[1].ID {
				t.Fatalf("fixture must have two distinct observed calls: %+v", discovered.Model.Edges)
			}
			result := mustProcess(t, newTestReconciler(t, nil), Observation{
				WindowStart: start, WindowEnd: start.Add(time.Minute), Result: discovered,
				Ingest: snapshot.IngestSummary{Spans: len(spans), Traces: 1},
			})
			if len(result.GuardrailUnion.Model.Edges) != 2 {
				t.Fatalf("published graph lost parallel calls: %+v; raw=%+v", result.GuardrailUnion.Model.Edges, discovered.Model.Edges)
			}
			for _, edge := range discovered.Model.Edges {
				if lifecycleFor(result.Report, KindEdge, edge.ID) == "" {
					t.Fatalf("typed edge %q missing from reconciliation evidence", edge.ID)
				}
			}
		})
	}
}

func TestLegacyEdgeWithoutExplicitIDStillReconciles(t *testing.T) {
	start := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	obs := observation(start, []serviceSpec{{id: "a", support: 1}, {id: "b", support: 1}}, nil, nil,
		snapshot.IngestSummary{Spans: 2, Traces: 1})
	edge := model.Edge{From: "a", To: "b", Kind: model.EdgeKindSync, Blocking: true}
	id := model.EdgeID(edge.From, edge.To, edge.Kind, edge.Blocking)
	obs.Result.Model.Edges = []model.Edge{edge}
	obs.Result.Discovery.Edges = []snapshot.EdgeRecord{{ID: id, From: "a", To: "b", Kind: model.EdgeKindSync, Blocking: true,
		Support: snapshot.SupportSummary{Observations: 1, TraceCount: 1}}}
	if _, ok := buildObservationIndex(obs).edges[id]; !ok {
		t.Fatal("legacy edge without an explicit ID was lost")
	}
}
