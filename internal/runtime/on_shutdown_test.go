package runtime

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/MB3R-Lab/Bering/internal/config"
	"github.com/MB3R-Lab/Bering/internal/connectors/traces"
	"github.com/MB3R-Lab/Bering/internal/overlay"
	"github.com/MB3R-Lab/Bering/internal/snapshot"
)

func testBatchEngine(t *testing.T, clock *fakeClock, sink SnapshotSink, maximum int, overlays ...overlay.File) *Engine {
	t.Helper()
	engine, err := NewEngine(EngineConfig{
		PublicationMode:  config.PublicationModeOnShutdown,
		MaxInMemorySpans: maximum,
		LateSpanPolicy:   "drop",
		Sink:             sink,
		Now:              clock.Now,
		Overlays:         overlays,
	})
	if err != nil {
		t.Fatal(err)
	}
	return engine
}

func testSpan(service, trace, id, kind string, event time.Time) traces.Span {
	return traces.Span{
		Service: service, TraceID: trace, SpanID: id, Kind: kind,
		Name: "GET /items", StartTime: event, EndTime: event.Add(time.Millisecond),
		Attributes: map[string]any{"http.request.method": "GET", "url.path": "/items"},
	}
}

func TestOnShutdownRetainsMixedSourcesAcrossMidnightAndPublishesOnce(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 1, 1, 23, 59, 58, 0, time.UTC)}
	sink := &sinkRecorder{}
	engine := testBatchEngine(t, clock, sink, 3)
	old := testSpan("frontend", "trace-a", "span-a", "server", clock.Now().Add(-48*time.Hour))
	old.SourceKind, old.SourceDomain = "ebpf", "ebpf:cluster-a"
	if err := engine.IngestWithSource(context.Background(), []traces.Span{old}, clock.Now(),
		snapshot.SourceSummary{Type: "ebpf", Connector: "obi", Ref: "collector-a"}); err != nil {
		t.Fatal(err)
	}
	clock.Advance(3 * time.Hour)
	// The same opaque context ID in two source domains remains two traces.
	future := testSpan("backend", "trace-a", "span-a", "server", clock.Now().Add(48*time.Hour))
	if err := engine.IngestWithSource(context.Background(), []traces.Span{future}, clock.Now(),
		snapshot.SourceSummary{Type: "traces", Connector: "otlp_http", Ref: "receiver-b"}); err != nil {
		t.Fatal(err)
	}
	engine.mu.Lock()
	retained := append([]traces.Span(nil), engine.current.spans...)
	engine.mu.Unlock()
	if len(retained) != 2 || retained[0].SourceKind != "ebpf" || retained[0].SourceDomain != "ebpf:cluster-a" ||
		retained[0].SourceRef != "collector-a" || retained[1].SourceRef != "receiver-b" ||
		!retained[0].StartTime.Equal(old.StartTime) || !retained[1].StartTime.Equal(future.StartTime) {
		t.Fatalf("source provenance or original event times changed: %+v", retained)
	}
	if err := engine.FlushDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := sink.Count(); got != 0 {
		t.Fatalf("published before Close: %d", got)
	}
	if err := engine.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := engine.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := sink.Count(); got != 1 {
		t.Fatalf("wanted one publication, got %d", got)
	}
	env := sink.First()
	if env.Ingest.Spans != 2 || env.Ingest.Traces != 2 || env.Ingest.DroppedSpans != 0 || env.Ingest.LateSpans != 0 || len(env.Sources) != 2 {
		t.Fatalf("batch/source accounting changed: ingest=%+v sources=%+v", env.Ingest, env.Sources)
	}
	if env.WindowStart != "2026-01-01T23:59:58Z" || env.WindowEnd != "2026-01-02T02:59:58Z" {
		t.Fatalf("batch interval is not actual process interval: %s - %s", env.WindowStart, env.WindowEnd)
	}
	if err := engine.IngestWithSource(context.Background(), []traces.Span{old}, clock.Now(), snapshot.SourceSummary{}); err == nil {
		t.Fatal("ingest after Close accepted")
	}
}

func TestOnShutdownOverflowPoisonPreventsPrefixPublication(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	sink := &sinkRecorder{}
	engine := testBatchEngine(t, clock, sink, 1)
	first := testSpan("frontend", "t", "s", "server", clock.Now())
	if err := engine.Ingest(context.Background(), []traces.Span{first}, clock.Now()); err != nil {
		t.Fatal(err)
	}
	if err := engine.Ingest(context.Background(), []traces.Span{first}, clock.Now()); err == nil || !strings.Contains(err.Error(), "capacity exceeded") {
		t.Fatalf("expected terminal overflow, got %v", err)
	}
	if err := engine.Close(context.Background()); err == nil || !strings.Contains(err.Error(), "capacity exceeded") {
		t.Fatalf("Close hid overflow: %v", err)
	}
	if err := engine.Close(context.Background()); err == nil || !strings.Contains(err.Error(), "capacity exceeded") {
		t.Fatalf("duplicate Close changed outcome: %v", err)
	}
	if got := sink.Count(); got != 0 {
		t.Fatalf("published accepted prefix after overflow: %d", got)
	}
}

func TestOnShutdownUsesExistingOverlayAndOneWindowGraph(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	baseSink := &sinkRecorder{}
	base := testBatchEngine(t, clock, baseSink, 2)
	span := testSpan("frontend", "t", "s", "server", clock.Now())
	if err := base.Ingest(context.Background(), []traces.Span{span}, clock.Now()); err != nil {
		t.Fatal(err)
	}
	if err := base.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	serviceID := baseSink.First().Model.Services[0].ID
	replicas := 2
	withOverlay := &sinkRecorder{}
	engine := testBatchEngine(t, clock, withOverlay, 2, overlay.File{Name: "replica-supply", Services: []overlay.ServiceOverlay{{ID: serviceID, Replicas: &replicas}}})
	if err := engine.Ingest(context.Background(), []traces.Span{span}, clock.Now()); err != nil {
		t.Fatal(err)
	}
	if err := engine.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if withOverlay.First().Model.Services[0].Replicas != replicas {
		t.Fatal("existing overlay was not applied")
	}

	windowSink := &sinkRecorder{}
	window, err := NewEngine(EngineConfig{WindowSize: time.Hour, MaxInMemorySpans: 2, LateSpanPolicy: "drop", Sink: windowSink, Now: clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	if err := window.Ingest(context.Background(), []traces.Span{span}, clock.Now()); err != nil {
		t.Fatal(err)
	}
	if err := window.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(baseSink.First().Model.Services, windowSink.First().Model.Services) ||
		!reflect.DeepEqual(baseSink.First().Model.Edges, windowSink.First().Model.Edges) ||
		!reflect.DeepEqual(baseSink.First().Model.Endpoints, windowSink.First().Model.Endpoints) {
		t.Fatal("one-window graph differs from existing discovery path")
	}
}

type failingSink struct{ calls int }

func (s *failingSink) Write(context.Context, snapshot.Envelope) error {
	s.calls++
	return errors.New("sink unavailable")
}

func TestOnShutdownCloseFailureDoesNotRetry(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	sink := &failingSink{}
	engine := testBatchEngine(t, clock, sink, 1)
	span := testSpan("frontend", "t", "s", "server", clock.Now())
	if err := engine.Ingest(context.Background(), []traces.Span{span}, clock.Now()); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := engine.Close(context.Background()); err == nil || !strings.Contains(err.Error(), "sink unavailable") {
			t.Fatalf("close %d: %v", i, err)
		}
	}
	if sink.calls != 1 {
		t.Fatalf("sink retried after failure: %d", sink.calls)
	}
}

func TestOnShutdownDiscoveryFailureDoesNotPublish(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	sink := &sinkRecorder{}
	engine := testBatchEngine(t, clock, sink, 1, overlay.File{Name: "unknown", Services: []overlay.ServiceOverlay{{ID: "nonexistent"}}})
	span := testSpan("frontend", "t", "s", "server", clock.Now())
	if err := engine.Ingest(context.Background(), []traces.Span{span}, clock.Now()); err != nil {
		t.Fatal(err)
	}
	if err := engine.Close(context.Background()); err == nil || !strings.Contains(err.Error(), "unknown service") {
		t.Fatalf("expected discovery error: %v", err)
	}
	if sink.Count() != 0 {
		t.Fatal("published despite discovery failure")
	}
}
