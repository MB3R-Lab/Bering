package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/MB3R-Lab/Bering/internal/connectors/traces"
	"github.com/MB3R-Lab/Bering/internal/discovery"
	"github.com/MB3R-Lab/Bering/internal/model"
	"github.com/MB3R-Lab/Bering/internal/overlay"
	"github.com/MB3R-Lab/Bering/internal/reconciliation"
	"github.com/MB3R-Lab/Bering/internal/schema"
	"github.com/MB3R-Lab/Bering/internal/snapshot"
)

type EngineConfig struct {
	WindowSize               time.Duration
	MaxInMemorySpans         int
	LateSpanPolicy           string
	Sink                     SnapshotSink
	Metrics                  *Metrics
	Logger                   *slog.Logger
	Now                      func() time.Time
	SourceRef                string
	Sources                  []snapshot.SourceSummary
	Overlays                 []overlay.File
	ReconciliationConfig     reconciliation.Config
	ReconciliationReportPath string
	RawWindowPath            string
	StableCorePath           string
}

type Engine struct {
	cfg            EngineConfig
	mu             sync.Mutex
	flushMu        sync.Mutex
	current        windowState
	previous       *snapshot.Envelope
	previousRaw    *snapshot.Envelope
	previousStable *snapshot.Envelope
	reconciler     *reconciliation.Reconciler
	latestReport   reconciliation.Report
}

type windowState struct {
	start   time.Time
	end     time.Time
	spans   []traces.Span
	traces  map[string]struct{}
	sources map[string]snapshot.SourceSummary
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
	var reconcilerInstance *reconciliation.Reconciler
	if cfg.ReconciliationConfig.Enabled {
		var err error
		reconcilerInstance, err = reconciliation.New(cfg.ReconciliationConfig)
		if err != nil {
			return nil, fmt.Errorf("create reconciler: %w", err)
		}
	}
	start, end := alignWindow(cfg.Now().UTC(), cfg.WindowSize)
	return &Engine{
		cfg:        cfg,
		reconciler: reconcilerInstance,
		current: windowState{
			start:   start,
			end:     end,
			traces:  map[string]struct{}{},
			sources: map[string]snapshot.SourceSummary{},
		},
	}, nil
}

func (e *Engine) Ingest(ctx context.Context, spans []traces.Span, receivedAt time.Time) error {
	return e.IngestWithSource(ctx, spans, receivedAt, snapshot.SourceSummary{})
}

func (e *Engine) IngestWithSource(ctx context.Context, spans []traces.Span, receivedAt time.Time, source snapshot.SourceSummary) error {
	if receivedAt.IsZero() {
		receivedAt = e.cfg.Now().UTC()
	}
	e.cfg.Metrics.AddIngested(len(spans))
	if err := e.flushDue(ctx, receivedAt.UTC()); err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	mergeWindowSourceSummary(&e.current, defaultSourceSummary(source, e.cfg.Sources), len(spans))

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
	e.flushMu.Lock()
	defer e.flushMu.Unlock()
	return e.flushDueLocked(ctx, e.cfg.Now().UTC())
}

func (e *Engine) Close(ctx context.Context) error {
	e.flushMu.Lock()
	defer e.flushMu.Unlock()
	return e.flushCurrent(ctx, e.cfg.Now().UTC(), true)
}

func (e *Engine) LatestReconciliationReport() (reconciliation.Report, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.latestReport.FormatVersion == 0 {
		return reconciliation.Report{}, false
	}
	return e.latestReport, true
}

func (e *Engine) flushDue(ctx context.Context, now time.Time) error {
	e.flushMu.Lock()
	defer e.flushMu.Unlock()
	return e.flushDueLocked(ctx, now)
}

func (e *Engine) flushDueLocked(ctx context.Context, now time.Time) error {
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
	if len(current.spans) == 0 && !force && e.reconciler == nil {
		e.current = newWindow(current.end, e.cfg.WindowSize)
		e.mu.Unlock()
		return nil
	}
	if len(current.spans) == 0 && force && e.reconciler == nil {
		e.mu.Unlock()
		return nil
	}
	e.current = newWindow(current.end, e.cfg.WindowSize)
	previous := e.previous
	previousRaw := e.previousRaw
	previousStable := e.previousStable
	e.mu.Unlock()

	started := e.cfg.Now().UTC()
	var (
		result discovery.Result
		err    error
	)
	if len(current.spans) > 0 {
		result, err = discovery.Discover(current.spans, discovery.Options{
			SourceRef:    e.cfg.SourceRef,
			DiscoveredAt: current.end.Format(time.RFC3339),
			Overlays:     e.cfg.Overlays,
			Sources:      currentSources(current, e.cfg.Sources),
			RuntimeMode:  true,
		})
		if err != nil {
			return fmt.Errorf("build snapshot discovery result: %w", err)
		}
	}

	if e.reconciler != nil {
		return e.flushReconciled(ctx, current, result, previous, previousRaw, previousStable, started)
	}
	env, err := e.buildSnapshotEnvelope(current, result, previous, started)
	if err != nil {
		return err
	}
	return e.writePrimarySnapshot(ctx, env, started)
}

func (e *Engine) flushReconciled(ctx context.Context, current windowState, raw discovery.Result, previous, previousRaw, previousStable *snapshot.Envelope, started time.Time) error {
	reconciliationResult, err := e.reconciler.Process(reconciliation.Observation{
		WindowStart: current.start,
		WindowEnd:   current.end,
		Result:      raw,
		Ingest: snapshot.IngestSummary{
			Spans:        len(current.spans),
			Traces:       len(current.traces),
			DroppedSpans: current.dropped,
			LateSpans:    current.late,
		},
	})
	if err != nil {
		return fmt.Errorf("run reconciliation: %w", err)
	}
	if err := reconciliation.WriteReport(e.cfg.ReconciliationReportPath, reconciliationResult.Report); err != nil {
		return err
	}
	e.cfg.Metrics.RecordReconciliation(reconciliationResult.Report)

	if len(reconciliationResult.RawWindow.Model.Services) > 0 {
		rawEnv, err := e.buildProjectionSnapshot(current, reconciliationResult.RawWindow, previousRaw, started)
		if err != nil {
			return err
		}
		if err := WriteProjectionView(e.cfg.RawWindowPath, ProjectionView{
			Name:              string(reconciliationResult.RawWindow.Name),
			Observation:       reconciliationResult.Report.Versions.ObservationVersion,
			StructuralVersion: reconciliationResult.Report.Versions.ObservationVersion,
			Available:         true,
			TopologyVersion:   rawEnv.TopologyVersion,
			Snapshot:          &rawEnv,
		}); err != nil {
			return err
		}
		copyRaw := rawEnv
		e.mu.Lock()
		e.previousRaw = &copyRaw
		e.mu.Unlock()
	} else if err := WriteProjectionView(e.cfg.RawWindowPath, ProjectionView{
		Name:              "raw_window",
		Observation:       reconciliationResult.Report.Versions.ObservationVersion,
		StructuralVersion: reconciliationResult.Report.Versions.ObservationVersion,
		Available:         false,
	}); err != nil {
		return err
	}

	if len(reconciliationResult.StableCore.Model.Services) > 0 {
		stableEnv, err := e.buildProjectionSnapshot(current, reconciliationResult.StableCore, previousStable, started)
		if err != nil {
			return err
		}
		if err := WriteProjectionView(e.cfg.StableCorePath, ProjectionView{
			Name:              string(reconciliationResult.StableCore.Name),
			Observation:       reconciliationResult.Report.Versions.ObservationVersion,
			StructuralVersion: reconciliationResult.Report.Versions.StableCoreVersion,
			Available:         true,
			TopologyVersion:   stableEnv.TopologyVersion,
			Snapshot:          &stableEnv,
		}); err != nil {
			return err
		}
		copyStable := stableEnv
		e.mu.Lock()
		e.previousStable = &copyStable
		e.mu.Unlock()
	} else if err := WriteProjectionView(e.cfg.StableCorePath, ProjectionView{
		Name:              "stable_core",
		Observation:       reconciliationResult.Report.Versions.ObservationVersion,
		StructuralVersion: reconciliationResult.Report.Versions.StableCoreVersion,
		Available:         false,
	}); err != nil {
		return err
	}

	if len(reconciliationResult.GuardrailUnion.Model.Services) == 0 {
		e.mu.Lock()
		e.latestReport = reconciliationResult.Report
		e.mu.Unlock()
		return nil
	}
	env, err := e.buildProjectionSnapshot(current, reconciliationResult.GuardrailUnion, previous, started)
	if err != nil {
		return err
	}
	e.mu.Lock()
	e.latestReport = reconciliationResult.Report
	e.mu.Unlock()
	return e.writePrimarySnapshot(ctx, env, started)
}

func (e *Engine) buildSnapshotEnvelope(current windowState, result discovery.Result, previous *snapshot.Envelope, started time.Time) (snapshot.Envelope, error) {
	topologyVersion, err := snapshot.TopologyDigest(result.Model)
	if err != nil {
		return snapshot.Envelope{}, fmt.Errorf("compute topology digest: %w", err)
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
	return env, nil
}

func (e *Engine) buildProjectionSnapshot(current windowState, projection reconciliation.Projection, previous *snapshot.Envelope, started time.Time) (snapshot.Envelope, error) {
	env := snapshot.Envelope{
		SnapshotID:      snapshot.BuildSnapshotID(current.start.Format(time.RFC3339), current.end.Format(time.RFC3339), projection.TopologyVersion),
		TopologyVersion: projection.TopologyVersion,
		WindowStart:     current.start.Format(time.RFC3339),
		WindowEnd:       current.end.Format(time.RFC3339),
		Ingest: snapshot.IngestSummary{
			Spans:        len(current.spans),
			Traces:       len(current.traces),
			DroppedSpans: current.dropped,
			LateSpans:    current.late,
		},
		Counts:    projection.Counts,
		Coverage:  projection.Coverage,
		Sources:   projection.Sources,
		Discovery: projection.Discovery,
		Model:     projection.Model,
		Metadata: snapshot.Metadata{
			SourceType: discovery.SourceTypeBering,
			SourceRef:  e.cfg.SourceRef,
			EmittedAt:  started.Format(time.RFC3339),
			Confidence: projection.Model.Metadata.Confidence,
			Schema: model.SchemaRef{
				Name:    schema.ExpectedSnapshotSchemaName,
				Version: schema.ExpectedSnapshotSchemaVersion,
				URI:     schema.ExpectedSnapshotSchemaURI,
				Digest:  schema.ExpectedSnapshotSchemaDigest,
			},
		},
	}
	env.Diff = snapshot.ComputeDiff(previous, env)
	env.SortDeterministic()
	return env, nil
}

func (e *Engine) writePrimarySnapshot(ctx context.Context, env snapshot.Envelope, started time.Time) error {
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
	return windowState{
		start:   start,
		end:     start.Add(size),
		traces:  map[string]struct{}{},
		sources: map[string]snapshot.SourceSummary{},
	}
}

func currentSources(current windowState, fallback []snapshot.SourceSummary) []snapshot.SourceSummary {
	if len(current.sources) == 0 {
		return fallback
	}
	out := make([]snapshot.SourceSummary, 0, len(current.sources))
	for _, item := range current.sources {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		left, right := out[i], out[j]
		if left.Type != right.Type {
			return left.Type < right.Type
		}
		if left.Connector != right.Connector {
			return left.Connector < right.Connector
		}
		return left.Ref < right.Ref
	})
	return out
}

func defaultSourceSummary(source snapshot.SourceSummary, fallback []snapshot.SourceSummary) snapshot.SourceSummary {
	if strings.TrimSpace(source.Type) != "" {
		return source
	}
	if len(fallback) > 0 {
		return fallback[0]
	}
	return snapshot.SourceSummary{Type: "traces", Connector: "trace_file"}
}

func mergeWindowSourceSummary(current *windowState, source snapshot.SourceSummary, observations int) {
	if current == nil || observations <= 0 {
		return
	}
	if current.sources == nil {
		current.sources = map[string]snapshot.SourceSummary{}
	}
	source.Type = strings.TrimSpace(source.Type)
	if source.Type == "" {
		source.Type = "traces"
	}
	key := sourceSummaryKey(source)
	merged := current.sources[key]
	if merged.Type == "" {
		merged = source
	}
	merged.Observations += observations
	current.sources[key] = merged
}

func sourceSummaryKey(source snapshot.SourceSummary) string {
	return strings.TrimSpace(source.Type) + "|" + strings.TrimSpace(source.Connector) + "|" + strings.TrimSpace(source.Ref)
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
