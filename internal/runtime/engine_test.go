package runtime

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/MB3R-Lab/Bering/internal/connectors/traces"
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
