package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/MB3R-Lab/Bering/internal/connectors/traces"
	"github.com/MB3R-Lab/Bering/internal/discovery"
	"github.com/MB3R-Lab/Bering/internal/model"
	"github.com/MB3R-Lab/Bering/internal/overlay"
	"github.com/MB3R-Lab/Bering/internal/schema"
	"github.com/MB3R-Lab/Bering/internal/snapshot"
)

type EngineConfig struct {
	WindowSize       time.Duration
	MaxInMemorySpans int
	LateSpanPolicy   string
	Sink             SnapshotSink
	Metrics          *Metrics
	Logger           *slog.Logger
	Now              func() time.Time
	SourceRef        string
	Sources          []snapshot.SourceSummary
	Overlays         []overlay.File
}

type Engine struct {
	cfg      EngineConfig
	mu       sync.Mutex
	current  windowState
	previous *snapshot.Envelope
}

type windowState struct {
	start   time.Time
	end     time.Time
	spans   []traces.Span
	traces  map[string]struct{}
	dropped int
	late    int
}

func NewEngine(cfg EngineConfig) (*Engine, error) {
	if cfg.WindowSize <= 0 {
		return nil, fmt.Errorf("window size must be > 0")
	}
	if cfg.MaxInMemorySpans <= 0 {
		return nil, fmt.Errorf("max in-memory spans must be > 0")
	}
	switch strings.ToLower(strings.TrimSpace(cfg.LateSpanPolicy)) {
	case "drop", "current_window":
	default:
		return nil, fmt.Errorf("unsupported late span policy: %s", cfg.LateSpanPolicy)
	}
	if cfg.Sink == nil {
		return nil, fmt.Errorf("snapshot sink is required")
	}
	if cfg.Metrics == nil {
		cfg.Metrics = NewMetrics()
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.NewTextHandler(ioDiscard{}, nil))
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	start, end := alignWindow(cfg.Now().UTC(), cfg.WindowSize)
	return &Engine{
		cfg: cfg,
		current: windowState{
			start:  start,
			end:    end,
			traces: map[string]struct{}{},
		},
	}, nil
}

func (e *Engine) Ingest(ctx context.Context, spans []traces.Span, receivedAt time.Time) error {
	if receivedAt.IsZero() {
		receivedAt = e.cfg.Now().UTC()
	}
	e.cfg.Metrics.AddIngested(len(spans))
	if err := e.flushDue(ctx, receivedAt.UTC()); err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	dropped := 0
	lateDropped := 0
	lateAccepted := 0
	for _, span := range spans {
		eventTime := span.EventTime(receivedAt)
		if eventTime.Before(e.current.start) {
			e.current.late++
			if strings.EqualFold(e.cfg.LateSpanPolicy, "drop") {
				e.current.dropped++
				dropped++
				lateDropped++
				continue
			}
			lateAccepted++
		}
		if len(e.current.spans) >= e.cfg.MaxInMemorySpans {
			e.current.dropped++
			dropped++
			continue
		}
		e.current.spans = append(e.current.spans, span)
		if span.TraceID != "" {
			e.current.traces[span.TraceID] = struct{}{}
		}
	}
	if dropped > 0 {
		e.cfg.Metrics.AddDropped(dropped)
	}
	if lateDropped > 0 || lateAccepted > 0 {
		e.cfg.Logger.Warn("late spans observed",
			slog.Int("dropped", lateDropped),
			slog.Int("accepted_current_window", lateAccepted),
			slog.String("policy", e.cfg.LateSpanPolicy),
		)
	}
	if dropped-lateDropped > 0 {
		e.cfg.Logger.Warn("spans dropped due to in-memory buffer limit", slog.Int("dropped", dropped-lateDropped), slog.Int("max_in_memory_spans", e.cfg.MaxInMemorySpans))
	}
	return nil
}

func (e *Engine) FlushDue(ctx context.Context) error {
	return e.flushDue(ctx, e.cfg.Now().UTC())
}

func (e *Engine) Close(ctx context.Context) error {
	return e.flushCurrent(ctx, e.cfg.Now().UTC(), true)
}

func (e *Engine) flushDue(ctx context.Context, now time.Time) error {
	for {
		e.mu.Lock()
		currentEnd := e.current.end
		e.cfg.Metrics.SetWindowLag(now, currentEnd)
		shouldFlush := !now.Before(currentEnd)
		e.mu.Unlock()
		if !shouldFlush {
			return nil
		}
		if err := e.flushCurrent(ctx, now, false); err != nil {
			return err
		}
	}
}

func (e *Engine) flushCurrent(ctx context.Context, now time.Time, force bool) error {
	e.mu.Lock()
	current := e.current
	if len(current.spans) == 0 && !force {
		e.current = newWindow(current.end, e.cfg.WindowSize)
		e.mu.Unlock()
		return nil
	}
	if len(current.spans) == 0 && force {
		e.mu.Unlock()
		return nil
	}
	e.current = newWindow(current.end, e.cfg.WindowSize)
	previous := e.previous
	e.mu.Unlock()

	started := e.cfg.Now().UTC()
	result, err := discovery.Discover(current.spans, discovery.Options{
		SourceRef:    e.cfg.SourceRef,
		DiscoveredAt: current.end.Format(time.RFC3339),
		Overlays:     e.cfg.Overlays,
		Sources:      e.cfg.Sources,
		RuntimeMode:  true,
	})
	if err != nil {
		return fmt.Errorf("build snapshot discovery result: %w", err)
	}
	topologyVersion, err := snapshot.TopologyDigest(result.Model)
	if err != nil {
		return fmt.Errorf("compute topology digest: %w", err)
	}
	env := snapshot.Envelope{
		SnapshotID:      snapshot.BuildSnapshotID(current.start.Format(time.RFC3339), current.end.Format(time.RFC3339), topologyVersion),
		TopologyVersion: topologyVersion,
		WindowStart:     current.start.Format(time.RFC3339),
		WindowEnd:       current.end.Format(time.RFC3339),
		Ingest: snapshot.IngestSummary{
			Spans:        len(current.spans),
			Traces:       len(current.traces),
			DroppedSpans: current.dropped,
			LateSpans:    current.late,
		},
		Counts: snapshot.Counts{
			Services:  len(result.Model.Services),
			Edges:     len(result.Model.Edges),
			Endpoints: len(result.Model.Endpoints),
		},
		Coverage:  result.Coverage,
		Sources:   result.Sources,
		Discovery: result.Discovery,
		Model:     result.Model,
		Metadata: snapshot.Metadata{
			SourceType: discovery.SourceTypeBering,
			SourceRef:  e.cfg.SourceRef,
			EmittedAt:  started.Format(time.RFC3339),
			Confidence: result.Coverage.Confidence,
			Schema: model.SchemaRef{
				Name:    schema.ExpectedSnapshotSchemaName,
				Version: schema.ExpectedSnapshotSchemaVersion,
				URI:     schema.ExpectedSnapshotSchemaURI,
				Digest:  schema.ExpectedSnapshotSchemaDigest,
			},
		},
	}
	carryForwardObservationTimes(previous, &env)
	env.Diff = snapshot.ComputeDiff(previous, env)
	env.SortDeterministic()
	if err := e.cfg.Sink.Write(ctx, env); err != nil {
		return err
	}
	finished := e.cfg.Now().UTC()
	e.cfg.Metrics.RecordSnapshot(env, finished.Sub(started), finished)
	e.cfg.Logger.Info("snapshot emitted",
		slog.String("snapshot_id", env.SnapshotID),
		slog.String("topology_version", env.TopologyVersion),
		slog.String("window_start", env.WindowStart),
		slog.String("window_end", env.WindowEnd),
		slog.Int("services", env.Counts.Services),
		slog.Int("edges", env.Counts.Edges),
		slog.Int("endpoints", env.Counts.Endpoints),
		slog.Int("dropped_spans", env.Ingest.DroppedSpans),
		slog.Int("late_spans", env.Ingest.LateSpans),
	)
	copyEnv := env
	e.mu.Lock()
	e.previous = &copyEnv
	e.mu.Unlock()
	return nil
}

func alignWindow(now time.Time, size time.Duration) (time.Time, time.Time) {
	start := now.UTC().Truncate(size)
	return start, start.Add(size)
}

func newWindow(start time.Time, size time.Duration) windowState {
	start = start.UTC()
	return windowState{start: start, end: start.Add(size), traces: map[string]struct{}{}}
}

func carryForwardObservationTimes(previous *snapshot.Envelope, current *snapshot.Envelope) {
	if previous == nil || current == nil {
		return
	}

	serviceIndex := make(map[string]snapshot.ServiceRecord, len(previous.Discovery.Services))
	for _, item := range previous.Discovery.Services {
		serviceIndex[item.ID] = item
	}
	for i := range current.Discovery.Services {
		if prev, ok := serviceIndex[current.Discovery.Services[i].ID]; ok {
			current.Discovery.Services[i].FirstSeen = mergeFirstSeen(prev.FirstSeen, current.Discovery.Services[i].FirstSeen)
			current.Discovery.Services[i].LastSeen = mergeLastSeen(prev.LastSeen, current.Discovery.Services[i].LastSeen)
		}
	}

	edgeIndex := make(map[string]snapshot.EdgeRecord, len(previous.Discovery.Edges))
	for _, item := range previous.Discovery.Edges {
		edgeIndex[item.ID] = item
	}
	for i := range current.Discovery.Edges {
		if prev, ok := edgeIndex[current.Discovery.Edges[i].ID]; ok {
			current.Discovery.Edges[i].FirstSeen = mergeFirstSeen(prev.FirstSeen, current.Discovery.Edges[i].FirstSeen)
			current.Discovery.Edges[i].LastSeen = mergeLastSeen(prev.LastSeen, current.Discovery.Edges[i].LastSeen)
		}
	}

	endpointIndex := make(map[string]snapshot.EndpointRecord, len(previous.Discovery.Endpoints))
	for _, item := range previous.Discovery.Endpoints {
		endpointIndex[item.ID] = item
	}
	for i := range current.Discovery.Endpoints {
		if prev, ok := endpointIndex[current.Discovery.Endpoints[i].ID]; ok {
			current.Discovery.Endpoints[i].FirstSeen = mergeFirstSeen(prev.FirstSeen, current.Discovery.Endpoints[i].FirstSeen)
			current.Discovery.Endpoints[i].LastSeen = mergeLastSeen(prev.LastSeen, current.Discovery.Endpoints[i].LastSeen)
		}
	}
}

func mergeFirstSeen(previous, current string) string {
	prevTime, prevOK := parseSeenTime(previous)
	currTime, currOK := parseSeenTime(current)
	switch {
	case prevOK && currOK:
		if prevTime.Before(currTime) {
			return prevTime.Format(time.RFC3339)
		}
		return currTime.Format(time.RFC3339)
	case prevOK:
		return prevTime.Format(time.RFC3339)
	case currOK:
		return currTime.Format(time.RFC3339)
	default:
		return ""
	}
}

func mergeLastSeen(previous, current string) string {
	prevTime, prevOK := parseSeenTime(previous)
	currTime, currOK := parseSeenTime(current)
	switch {
	case prevOK && currOK:
		if prevTime.After(currTime) {
			return prevTime.Format(time.RFC3339)
		}
		return currTime.Format(time.RFC3339)
	case prevOK:
		return prevTime.Format(time.RFC3339)
	case currOK:
		return currTime.Format(time.RFC3339)
	default:
		return ""
	}
}

func parseSeenTime(value string) (time.Time, bool) {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, false
	}
	return parsed.UTC(), true
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}
