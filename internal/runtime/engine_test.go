package runtime

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MB3R-Lab/Bering/internal/connectors/traces"
	"github.com/MB3R-Lab/Bering/internal/reconciliation"
	"github.com/MB3R-Lab/Bering/internal/schema"
	"github.com/MB3R-Lab/Bering/internal/snapshot"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type sinkRecorder struct {
	mu   sync.Mutex
	envs []snapshot.Envelope
}

func (s *sinkRecorder) Write(_ context.Context, env snapshot.Envelope) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.envs = append(s.envs, env)
	return nil
}

func (s *sinkRecorder) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.envs)
}

func (s *sinkRecorder) First() snapshot.Envelope {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.envs[0]
}

func (s *sinkRecorder) Last() snapshot.Envelope {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.envs[len(s.envs)-1]
}

func TestEngineFlushesWindowIntoSnapshot(t *testing.T) {
	t.Parallel()

	clock := &fakeClock{now: time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC)}
	sink := &sinkRecorder{}
	engine, err := NewEngine(EngineConfig{
		WindowSize:       time.Minute,
		MaxInMemorySpans: 10,
		LateSpanPolicy:   "drop",
		Sink:             sink,
		Metrics:          NewMetrics(),
		Logger:           slog.New(slog.NewTextHandler(ioDiscard{}, nil)),
		Now:              clock.Now,
		SourceRef:        "bering://serve?listen=:4318",
	})
	if err != nil {
		t.Fatalf("NewEngine returned error: %v", err)
	}

	span := traces.Span{
		TraceID:    "trace-1",
		SpanID:     "span-1",
		Service:    "frontend",
		Name:       "GET /checkout",
		Kind:       "server",
		StartTime:  clock.Now(),
		EndTime:    clock.Now().Add(100 * time.Millisecond),
		Attributes: map[string]any{"http.request.method": "GET", "http.route": "/checkout"},
	}
	if err := engine.Ingest(context.Background(), []traces.Span{span}, clock.Now()); err != nil {
		t.Fatalf("Ingest returned error: %v", err)
	}
	clock.Advance(61 * time.Second)
	if err := engine.FlushDue(context.Background()); err != nil {
		t.Fatalf("FlushDue returned error: %v", err)
	}
	if got, want := sink.Count(), 1; got != want {
		t.Fatalf("snapshot count mismatch: got=%d want=%d", got, want)
	}
	env := sink.First()
	if env.Counts.Services != 1 || env.Counts.Endpoints != 1 {
		t.Fatalf("unexpected snapshot counts: %+v", env.Counts)
	}
	if env.Ingest.Spans != 1 {
		t.Fatalf("unexpected ingest summary: %+v", env.Ingest)
	}
}

func TestEngineDropsLateSpanWhenConfigured(t *testing.T) {
	t.Parallel()

	clock := &fakeClock{now: time.Date(2026, 3, 11, 12, 1, 0, 0, time.UTC)}
	sink := &sinkRecorder{}
	engine, err := NewEngine(EngineConfig{
		WindowSize:       time.Minute,
		MaxInMemorySpans: 10,
		LateSpanPolicy:   "drop",
		Sink:             sink,
		Metrics:          NewMetrics(),
		Logger:           slog.New(slog.NewTextHandler(ioDiscard{}, nil)),
		Now:              clock.Now,
		SourceRef:        "bering://serve?listen=:4318",
	})
	if err != nil {
		t.Fatalf("NewEngine returned error: %v", err)
	}

	lateSpan := traces.Span{
		TraceID:    "trace-1",
		SpanID:     "span-1",
		Service:    "frontend",
		Name:       "GET /checkout",
		Kind:       "server",
		StartTime:  clock.Now().Add(-2 * time.Minute),
		EndTime:    clock.Now().Add(-2*time.Minute + 100*time.Millisecond),
		Attributes: map[string]any{"http.request.method": "GET", "http.route": "/checkout"},
	}
	if err := engine.Ingest(context.Background(), []traces.Span{lateSpan}, clock.Now()); err != nil {
		t.Fatalf("Ingest returned error: %v", err)
	}
	clock.Advance(61 * time.Second)
	if err := engine.FlushDue(context.Background()); err != nil {
		t.Fatalf("FlushDue returned error: %v", err)
	}
	if got, want := sink.Count(), 0; got != want {
		t.Fatalf("expected no snapshot for dropped late spans, got=%d", got)
	}
}

func TestEngineCarriesForwardObservationTimesAcrossSnapshots(t *testing.T) {
	t.Parallel()

	clock := &fakeClock{now: time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC)}
	sink := &sinkRecorder{}
	engine, err := NewEngine(EngineConfig{
		WindowSize:       time.Minute,
		MaxInMemorySpans: 10,
		LateSpanPolicy:   "drop",
		Sink:             sink,
		Metrics:          NewMetrics(),
		Logger:           slog.New(slog.NewTextHandler(ioDiscard{}, nil)),
		Now:              clock.Now,
		SourceRef:        "bering://serve?listen=:4318",
	})
	if err != nil {
		t.Fatalf("NewEngine returned error: %v", err)
	}

	firstSpan := traces.Span{
		TraceID:    "trace-1",
		SpanID:     "span-1",
		Service:    "frontend",
		Name:       "GET /checkout",
		Kind:       "server",
		StartTime:  clock.Now().Add(10 * time.Second),
		EndTime:    clock.Now().Add(11 * time.Second),
		Attributes: map[string]any{"http.request.method": "GET", "http.route": "/checkout"},
	}
	if err := engine.Ingest(context.Background(), []traces.Span{firstSpan}, clock.Now()); err != nil {
		t.Fatalf("Ingest returned error: %v", err)
	}
	clock.Advance(61 * time.Second)
	if err := engine.FlushDue(context.Background()); err != nil {
		t.Fatalf("FlushDue returned error: %v", err)
	}
	firstEnv := sink.Last()
	firstSeen := firstEnv.Discovery.Services[0].FirstSeen
	lastSeen := firstEnv.Discovery.Services[0].LastSeen

	secondSpan := traces.Span{
		TraceID:    "trace-2",
		SpanID:     "span-2",
		Service:    "frontend",
		Name:       "GET /checkout",
		Kind:       "server",
		StartTime:  clock.Now().Add(20 * time.Second),
		EndTime:    clock.Now().Add(21 * time.Second),
		Attributes: map[string]any{"http.request.method": "GET", "http.route": "/checkout"},
	}
	if err := engine.Ingest(context.Background(), []traces.Span{secondSpan}, clock.Now()); err != nil {
		t.Fatalf("second Ingest returned error: %v", err)
	}
	clock.Advance(61 * time.Second)
	if err := engine.FlushDue(context.Background()); err != nil {
		t.Fatalf("second FlushDue returned error: %v", err)
	}

	if got, want := sink.Count(), 2; got != want {
		t.Fatalf("snapshot count mismatch: got=%d want=%d", got, want)
	}
	secondEnv := sink.Last()
	if got := secondEnv.Discovery.Services[0].FirstSeen; got != firstSeen {
		t.Fatalf("first_seen was not carried forward: got=%s want=%s", got, firstSeen)
	}
	if got := secondEnv.Discovery.Services[0].LastSeen; got == lastSeen {
		t.Fatalf("last_seen was not advanced: got=%s", got)
	}
}

func TestEngineReconciliationRetainsMissingTopologyAndWritesViews(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	clock := &fakeClock{now: time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC)}
	sink := &sinkRecorder{}
	engine, err := NewEngine(EngineConfig{
		WindowSize:               time.Minute,
		MaxInMemorySpans:         20,
		LateSpanPolicy:           "drop",
		Sink:                     sink,
		Metrics:                  NewMetrics(),
		Logger:                   slog.New(slog.NewTextHandler(ioDiscard{}, nil)),
		Now:                      clock.Now,
		SourceRef:                "bering://serve?listen=:4318",
		ReconciliationConfig:     buildRuntimeReconciliationConfigForTest(dir),
		ReconciliationReportPath: filepath.Join(dir, "reconciliation-report.json"),
		RawWindowPath:            filepath.Join(dir, "raw-window.json"),
		StableCorePath:           filepath.Join(dir, "stable-core.json"),
	})
	if err != nil {
		t.Fatalf("NewEngine returned error: %v", err)
	}

	firstSpans := []traces.Span{
		{TraceID: "trace-1", SpanID: "frontend-server", Service: "frontend", Name: "GET /checkout", Kind: "server", StartTime: clock.Now().Add(10 * time.Second), EndTime: clock.Now().Add(11 * time.Second), Attributes: map[string]any{"http.request.method": "GET", "http.route": "/checkout"}},
		{TraceID: "trace-1", SpanID: "frontend-client", ParentSpanID: "frontend-server", Service: "frontend", Name: "GET checkout", Kind: "client", StartTime: clock.Now().Add(12 * time.Second), EndTime: clock.Now().Add(13 * time.Second)},
		{TraceID: "trace-1", SpanID: "checkout-server", ParentSpanID: "frontend-client", Service: "checkout", Name: "GET /checkout", Kind: "server", StartTime: clock.Now().Add(14 * time.Second), EndTime: clock.Now().Add(15 * time.Second), Attributes: map[string]any{"http.request.method": "GET", "http.route": "/checkout"}},
	}
	if err := engine.Ingest(context.Background(), firstSpans, clock.Now()); err != nil {
		t.Fatalf("first Ingest returned error: %v", err)
	}
	clock.Advance(61 * time.Second)
	if err := engine.FlushDue(context.Background()); err != nil {
		t.Fatalf("first FlushDue returned error: %v", err)
	}

	secondSpan := traces.Span{
		TraceID: "trace-2", SpanID: "frontend-only", Service: "frontend", Name: "GET /checkout", Kind: "server",
		StartTime: clock.Now().Add(10 * time.Second), EndTime: clock.Now().Add(11 * time.Second),
		Attributes: map[string]any{"http.request.method": "GET", "http.route": "/checkout"},
	}
	if err := engine.Ingest(context.Background(), []traces.Span{secondSpan}, clock.Now()); err != nil {
		t.Fatalf("second Ingest returned error: %v", err)
	}
	clock.Advance(61 * time.Second)
	if err := engine.FlushDue(context.Background()); err != nil {
		t.Fatalf("second FlushDue returned error: %v", err)
	}

	env := sink.Last()
	if env.Counts.Services < 2 {
		t.Fatalf("guardrail snapshot dropped retained topology too early: %+v", env.Counts)
	}
	rawLatest, err := os.ReadFile(filepath.Join(dir, "raw-window.json"))
	if err != nil {
		t.Fatalf("read raw window view: %v", err)
	}
	var rawView ProjectionView
	if err := json.Unmarshal(rawLatest, &rawView); err != nil {
		t.Fatalf("decode raw window view: %v", err)
	}
	if rawView.Snapshot == nil || rawView.Snapshot.Counts.Services != 1 {
		t.Fatalf("unexpected raw window view: %+v", rawView)
	}
	stableLatest, err := os.ReadFile(filepath.Join(dir, "stable-core.json"))
	if err != nil {
		t.Fatalf("read stable core view: %v", err)
	}
	var stableView ProjectionView
	if err := json.Unmarshal(stableLatest, &stableView); err != nil {
		t.Fatalf("decode stable core view: %v", err)
	}
	if stableView.Name != "stable_core" {
		t.Fatalf("unexpected stable core view metadata: %+v", stableView)
	}
	reportRaw, err := os.ReadFile(filepath.Join(dir, "reconciliation-report.json"))
	if err != nil {
		t.Fatalf("read reconciliation report: %v", err)
	}
	if !strings.Contains(string(reportRaw), "guardrail_union") {
		t.Fatalf("reconciliation report missing projection summary: %s", reportRaw)
	}
	latestRaw, err := snapshot.MarshalCanonical(env)
	if err != nil {
		t.Fatalf("marshal guardrail snapshot: %v", err)
	}
	if err := schema.ValidateSnapshotJSON(latestRaw); err != nil {
		t.Fatalf("guardrail snapshot failed schema validation: %v", err)
	}
}

func TestEngineReconciliationReloadsStateAcrossRestartAndKeepsGuardrailVersion(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	start := time.Date(2026, 3, 21, 14, 0, 0, 0, time.UTC)
	sinkFirst := &sinkRecorder{}
	clockFirst := &fakeClock{now: start}

	engineFirst, err := NewEngine(EngineConfig{
		WindowSize:               time.Minute,
		MaxInMemorySpans:         20,
		LateSpanPolicy:           "drop",
		Sink:                     sinkFirst,
		Metrics:                  NewMetrics(),
		Logger:                   slog.New(slog.NewTextHandler(ioDiscard{}, nil)),
		Now:                      clockFirst.Now,
		SourceRef:                "bering://serve?listen=:4318",
		ReconciliationConfig:     buildRuntimeReconciliationConfigForTest(dir),
		ReconciliationReportPath: filepath.Join(dir, "reconciliation-report.json"),
		RawWindowPath:            filepath.Join(dir, "raw-window.json"),
		StableCorePath:           filepath.Join(dir, "stable-core.json"),
	})
	if err != nil {
		t.Fatalf("NewEngine returned error: %v", err)
	}

	firstSpans := []traces.Span{
		{TraceID: "trace-r1", SpanID: "frontend-server", Service: "frontend", Name: "GET /checkout", Kind: "server", StartTime: clockFirst.Now().Add(10 * time.Second), EndTime: clockFirst.Now().Add(11 * time.Second), Attributes: map[string]any{"http.request.method": "GET", "http.route": "/checkout"}},
		{TraceID: "trace-r1", SpanID: "frontend-client", ParentSpanID: "frontend-server", Service: "frontend", Name: "GET checkout", Kind: "client", StartTime: clockFirst.Now().Add(12 * time.Second), EndTime: clockFirst.Now().Add(13 * time.Second)},
		{TraceID: "trace-r1", SpanID: "checkout-server", ParentSpanID: "frontend-client", Service: "checkout", Name: "GET /checkout", Kind: "server", StartTime: clockFirst.Now().Add(14 * time.Second), EndTime: clockFirst.Now().Add(15 * time.Second), Attributes: map[string]any{"http.request.method": "GET", "http.route": "/checkout"}},
	}
	if err := engineFirst.Ingest(context.Background(), firstSpans, clockFirst.Now()); err != nil {
		t.Fatalf("first Ingest returned error: %v", err)
	}
	clockFirst.Advance(61 * time.Second)
	if err := engineFirst.FlushDue(context.Background()); err != nil {
		t.Fatalf("first FlushDue returned error: %v", err)
	}
	firstReport, ok := engineFirst.LatestReconciliationReport()
	if !ok {
		t.Fatal("expected first reconciliation report")
	}

	sinkSecond := &sinkRecorder{}
	clockSecond := &fakeClock{now: start.Add(61 * time.Second)}
	engineSecond, err := NewEngine(EngineConfig{
		WindowSize:               time.Minute,
		MaxInMemorySpans:         20,
		LateSpanPolicy:           "drop",
		Sink:                     sinkSecond,
		Metrics:                  NewMetrics(),
		Logger:                   slog.New(slog.NewTextHandler(ioDiscard{}, nil)),
		Now:                      clockSecond.Now,
		SourceRef:                "bering://serve?listen=:4318",
		ReconciliationConfig:     buildRuntimeReconciliationConfigForTest(dir),
		ReconciliationReportPath: filepath.Join(dir, "reconciliation-report.json"),
		RawWindowPath:            filepath.Join(dir, "raw-window.json"),
		StableCorePath:           filepath.Join(dir, "stable-core.json"),
	})
	if err != nil {
		t.Fatalf("NewEngine after restart returned error: %v", err)
	}

	secondSpan := traces.Span{
		TraceID: "trace-r2", SpanID: "frontend-only", Service: "frontend", Name: "GET /checkout", Kind: "server",
		StartTime: clockSecond.Now().Add(10 * time.Second), EndTime: clockSecond.Now().Add(11 * time.Second),
		Attributes: map[string]any{"http.request.method": "GET", "http.route": "/checkout"},
	}
	if err := engineSecond.Ingest(context.Background(), []traces.Span{secondSpan}, clockSecond.Now()); err != nil {
		t.Fatalf("second Ingest returned error: %v", err)
	}
	clockSecond.Advance(61 * time.Second)
	if err := engineSecond.FlushDue(context.Background()); err != nil {
		t.Fatalf("second FlushDue returned error: %v", err)
	}

	secondReport, ok := engineSecond.LatestReconciliationReport()
	if !ok {
		t.Fatal("expected second reconciliation report")
	}
	if got, want := secondReport.Versions.GuardrailUnionVersion, firstReport.Versions.GuardrailUnionVersion; got != want {
		t.Fatalf("guardrail version changed across restart without structural change: got=%d want=%d", got, want)
	}
	if secondReport.Versions.ObservationVersion <= firstReport.Versions.ObservationVersion {
		t.Fatalf("observation version did not advance across restart: first=%d second=%d", firstReport.Versions.ObservationVersion, secondReport.Versions.ObservationVersion)
	}

	env := sinkSecond.Last()
	if env.Counts.Services < 2 {
		t.Fatalf("restarted engine did not retain persisted topology: %+v", env.Counts)
	}
	latestRaw, err := snapshot.MarshalCanonical(env)
	if err != nil {
		t.Fatalf("marshal restarted guardrail snapshot: %v", err)
	}
	if err := schema.ValidateSnapshotJSON(latestRaw); err != nil {
		t.Fatalf("restarted guardrail snapshot failed schema validation: %v", err)
	}
}

func buildRuntimeReconciliationConfigForTest(dir string) reconciliation.Config {
	cfg := reconciliation.DefaultConfig()
	cfg.StatePath = filepath.Join(dir, "reconciliation-state.json")
	cfg.CompactionCadence = 1
	return cfg
}
