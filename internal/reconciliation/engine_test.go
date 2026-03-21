package reconciliation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MB3R-Lab/Bering/internal/discovery"
	"github.com/MB3R-Lab/Bering/internal/model"
	"github.com/MB3R-Lab/Bering/internal/schema"
	"github.com/MB3R-Lab/Bering/internal/snapshot"
)

type serviceSpec struct {
	id         string
	support    int
	provenance []snapshot.Provenance
}

type endpointSpec struct {
	id         string
	service    string
	support    int
	provenance []snapshot.Provenance
}

func TestNoOpportunityDoesNotCreateNegativeEvidence(t *testing.T) {
	t.Parallel()

	r := newTestReconciler(t, func(cfg *Config) {
		cfg.StableCoreExistenceThreshold = 0.5
		cfg.CompactionCadence = 1
	})
	start := time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC)

	first := mustProcess(t, r, observation(start, []serviceSpec{{id: "frontend", support: 8, provenance: traceProvenance()}}, nil, []endpointSpec{{id: "frontend:GET /checkout", service: "frontend", support: 8, provenance: traceProvenance()}}, snapshot.IngestSummary{Spans: 8, Traces: 2}))
	if got := lifecycleFor(first.Report, KindEndpoint, "frontend:GET /checkout"); got != LifecycleActive {
		t.Fatalf("endpoint lifecycle after positive evidence = %s, want %s", got, LifecycleActive)
	}

	second := mustProcess(t, r, observation(start.Add(time.Minute), nil, nil, nil, snapshot.IngestSummary{}))
	if got := lifecycleFor(second.Report, KindEndpoint, "frontend:GET /checkout"); got != LifecycleActive {
		t.Fatalf("endpoint lifecycle with no opportunity = %s, want %s", got, LifecycleActive)
	}
	if got := missStreakFor(second.Report, KindEndpoint, "frontend:GET /checkout"); got != 0 {
		t.Fatalf("endpoint miss streak with no opportunity = %d, want 0", got)
	}
}

func TestQualifiedMissesAdvanceToStaleThenRetired(t *testing.T) {
	t.Parallel()

	r := newTestReconciler(t, func(cfg *Config) {
		cfg.StableCoreExistenceThreshold = 0.5
		cfg.RetirementExistenceThreshold = 0.45
		cfg.SoftGapMultiplier = 1
		cfg.HardGapMultiplier = 1
		cfg.MinSoftWindows = 2
		cfg.MinHardWindows = 3
		cfg.SourceResistanceSoftWindows = 0
		cfg.SourceResistanceHardWindows = 0
		cfg.CompactionCadence = 1
	})
	start := time.Date(2026, 3, 21, 13, 0, 0, 0, time.UTC)

	mustProcess(t, r, observation(start, []serviceSpec{{id: "frontend", support: 8, provenance: traceProvenance()}}, nil, []endpointSpec{{id: "frontend:GET /checkout", service: "frontend", support: 8, provenance: traceProvenance()}}, snapshot.IngestSummary{Spans: 12, Traces: 2}))
	stale := mustProcess(t, r, observation(start.Add(time.Minute), []serviceSpec{{id: "frontend", support: 8, provenance: traceProvenance()}}, nil, nil, snapshot.IngestSummary{Spans: 6, Traces: 1}))
	if got := lifecycleFor(stale.Report, KindEndpoint, "frontend:GET /checkout"); got != LifecycleActive {
		t.Fatalf("endpoint lifecycle after first qualified miss = %s, want %s", got, LifecycleActive)
	}
	stale = mustProcess(t, r, observation(start.Add(2*time.Minute), []serviceSpec{{id: "frontend", support: 8, provenance: traceProvenance()}}, nil, nil, snapshot.IngestSummary{Spans: 6, Traces: 1}))
	if got := lifecycleFor(stale.Report, KindEndpoint, "frontend:GET /checkout"); got != LifecycleStale {
		t.Fatalf("endpoint lifecycle after repeated misses = %s, want %s", got, LifecycleStale)
	}
	retired := mustProcess(t, r, observation(start.Add(3*time.Minute), []serviceSpec{{id: "frontend", support: 8, provenance: traceProvenance()}}, nil, nil, snapshot.IngestSummary{Spans: 6, Traces: 1}))
	if got := lifecycleFor(retired.Report, KindEndpoint, "frontend:GET /checkout"); got != LifecycleRetired {
		t.Fatalf("endpoint lifecycle after hard misses = %s, want %s", got, LifecycleRetired)
	}
}

func TestTelemetryHealthFreezesRetirement(t *testing.T) {
	t.Parallel()

	r := newTestReconciler(t, func(cfg *Config) {
		cfg.StableCoreExistenceThreshold = 0.5
		cfg.SoftGapMultiplier = 1
		cfg.HardGapMultiplier = 1
		cfg.MinSoftWindows = 1
		cfg.MinHardWindows = 2
		cfg.SourceResistanceSoftWindows = 0
		cfg.SourceResistanceHardWindows = 0
		cfg.RetirementExistenceThreshold = 0.45
		cfg.CompactionCadence = 1
	})
	start := time.Date(2026, 3, 21, 14, 0, 0, 0, time.UTC)

	mustProcess(t, r, observation(start, []serviceSpec{{id: "frontend", support: 8, provenance: traceProvenance()}}, nil, []endpointSpec{{id: "frontend:GET /checkout", service: "frontend", support: 8, provenance: traceProvenance()}}, snapshot.IngestSummary{Spans: 12, Traces: 2}))
	firstMiss := mustProcess(t, r, observation(start.Add(time.Minute), []serviceSpec{{id: "frontend", support: 6, provenance: traceProvenance()}}, nil, nil, snapshot.IngestSummary{Spans: 6, Traces: 1, DroppedSpans: 20, LateSpans: 20}))
	if got := lifecycleFor(firstMiss.Report, KindEndpoint, "frontend:GET /checkout"); got != LifecycleStale {
		t.Fatalf("endpoint lifecycle with degraded telemetry = %s, want %s", got, LifecycleStale)
	}
	secondMiss := mustProcess(t, r, observation(start.Add(2*time.Minute), []serviceSpec{{id: "frontend", support: 6, provenance: traceProvenance()}}, nil, nil, snapshot.IngestSummary{Spans: 6, Traces: 1, DroppedSpans: 20, LateSpans: 20}))
	if got := lifecycleFor(secondMiss.Report, KindEndpoint, "frontend:GET /checkout"); got != LifecycleStale {
		t.Fatalf("endpoint should remain stale while retirement frozen, got %s", got)
	}
	if !secondMiss.Report.TelemetryHealth.RetirementFrozen {
		t.Fatal("telemetry health should freeze retirement")
	}
}

func TestRareEntityDecaysMoreSlowly(t *testing.T) {
	t.Parallel()

	r := newTestReconciler(t, func(cfg *Config) {
		cfg.StableCoreExistenceThreshold = 0.5
		cfg.CompactionCadence = 1
	})
	start := time.Date(2026, 3, 21, 15, 0, 0, 0, time.UTC)

	mustProcess(t, r, observation(start, []serviceSpec{{id: "frontend", support: 8, provenance: traceProvenance()}}, nil, []endpointSpec{{id: "frontend:GET /checkout", service: "frontend", support: 8, provenance: traceProvenance()}}, snapshot.IngestSummary{Spans: 10, Traces: 2}))
	for i := 1; i <= 4; i++ {
		mustProcess(t, r, observation(start.Add(time.Duration(i)*time.Minute), nil, nil, nil, snapshot.IngestSummary{}))
	}
	mustProcess(t, r, observation(start.Add(5*time.Minute), []serviceSpec{{id: "frontend", support: 8, provenance: traceProvenance()}}, nil, []endpointSpec{{id: "frontend:GET /checkout", service: "frontend", support: 8, provenance: traceProvenance()}}, snapshot.IngestSummary{Spans: 10, Traces: 2}))

	firstMiss := mustProcess(t, r, observation(start.Add(6*time.Minute), []serviceSpec{{id: "frontend", support: 8, provenance: traceProvenance()}}, nil, nil, snapshot.IngestSummary{Spans: 5, Traces: 1}))
	if got := hardThresholdFor(firstMiss.Report, KindEndpoint, "frontend:GET /checkout"); got <= 2 {
		t.Fatalf("hard threshold for rare endpoint = %d, want > 2", got)
	}
	secondMiss := mustProcess(t, r, observation(start.Add(7*time.Minute), []serviceSpec{{id: "frontend", support: 8, provenance: traceProvenance()}}, nil, nil, snapshot.IngestSummary{Spans: 5, Traces: 1}))
	if got := lifecycleFor(secondMiss.Report, KindEndpoint, "frontend:GET /checkout"); got == LifecycleRetired {
		t.Fatalf("rare endpoint retired too early")
	}
}

func TestStrongSourceResistsRetirementAndGuardrailDiverges(t *testing.T) {
	t.Parallel()

	r := newTestReconciler(t, func(cfg *Config) {
		cfg.StableCoreExistenceThreshold = 0.5
		cfg.RetirementExistenceThreshold = 0.45
		cfg.SoftGapMultiplier = 1
		cfg.HardGapMultiplier = 1
		cfg.MinSoftWindows = 2
		cfg.MinHardWindows = 3
		cfg.CompactionCadence = 1
	})
	start := time.Date(2026, 3, 21, 16, 0, 0, 0, time.UTC)

	mustProcess(t, r, observation(start,
		[]serviceSpec{{id: "frontend", support: 6, provenance: traceProvenance()}},
		nil,
		[]endpointSpec{
			{id: "frontend:GET /trace", service: "frontend", support: 8, provenance: traceProvenance()},
			{id: "frontend:GET /pinned", service: "frontend", support: 8, provenance: overlayProvenance()},
		},
		snapshot.IngestSummary{Spans: 12, Traces: 2},
	))
	for i := 1; i <= 4; i++ {
		mustProcess(t, r, observation(start.Add(time.Duration(i)*time.Minute), []serviceSpec{{id: "frontend", support: 8, provenance: traceProvenance()}}, nil, nil, snapshot.IngestSummary{Spans: 6, Traces: 1}))
	}

	latest := mustProcess(t, r, observation(start.Add(5*time.Minute), []serviceSpec{{id: "frontend", support: 8, provenance: traceProvenance()}}, nil, nil, snapshot.IngestSummary{Spans: 6, Traces: 1}))
	if got := lifecycleFor(latest.Report, KindEndpoint, "frontend:GET /trace"); got != LifecycleRetired {
		t.Fatalf("trace endpoint lifecycle = %s, want %s", got, LifecycleRetired)
	}
	if got := lifecycleFor(latest.Report, KindEndpoint, "frontend:GET /pinned"); got == LifecycleRetired {
		t.Fatalf("strong-source endpoint should resist retirement")
	}
	if containsEndpoint(latest.StableCore, "frontend:GET /pinned") {
		t.Fatalf("stale strong endpoint should not remain in stable core")
	}
	if !containsEndpoint(latest.GuardrailUnion, "frontend:GET /pinned") {
		t.Fatalf("stale strong endpoint should remain in guardrail union")
	}
}

func TestVersionsOnlyBumpOnStructuralChange(t *testing.T) {
	t.Parallel()

	r := newTestReconciler(t, func(cfg *Config) {
		cfg.StableCoreExistenceThreshold = 0.5
		cfg.CompactionCadence = 1
	})
	start := time.Date(2026, 3, 21, 17, 0, 0, 0, time.UTC)

	first := mustProcess(t, r, observation(start, []serviceSpec{{id: "frontend", support: 8, provenance: traceProvenance()}}, nil, []endpointSpec{{id: "frontend:GET /checkout", service: "frontend", support: 8, provenance: traceProvenance()}}, snapshot.IngestSummary{Spans: 8, Traces: 2}))
	second := mustProcess(t, r, observation(start.Add(time.Minute), nil, nil, nil, snapshot.IngestSummary{}))
	if first.Report.Versions.StableCoreVersion != second.Report.Versions.StableCoreVersion {
		t.Fatalf("stable core version changed without structural change: first=%d second=%d", first.Report.Versions.StableCoreVersion, second.Report.Versions.StableCoreVersion)
	}
	if first.Report.Versions.GuardrailUnionVersion != second.Report.Versions.GuardrailUnionVersion {
		t.Fatalf("guardrail version changed without structural change: first=%d second=%d", first.Report.Versions.GuardrailUnionVersion, second.Report.Versions.GuardrailUnionVersion)
	}
	if first.Report.Versions.ObservationVersion == second.Report.Versions.ObservationVersion {
		t.Fatal("observation version should advance every window")
	}
}

func TestRestartLoadsPersistedState(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.StatePath = filepath.Join(dir, "reconciliation-state.json")
	cfg.StableCoreExistenceThreshold = 0.5
	cfg.CompactionCadence = 1

	start := time.Date(2026, 3, 21, 18, 0, 0, 0, time.UTC)
	r, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	mustProcess(t, r, observation(start, []serviceSpec{{id: "frontend", support: 8, provenance: traceProvenance()}}, nil, []endpointSpec{{id: "frontend:GET /checkout", service: "frontend", support: 8, provenance: traceProvenance()}}, snapshot.IngestSummary{Spans: 8, Traces: 2}))

	reloaded, err := New(cfg)
	if err != nil {
		t.Fatalf("New() after persistence error = %v", err)
	}
	result := mustProcess(t, reloaded, observation(start.Add(time.Minute), []serviceSpec{{id: "frontend", support: 4, provenance: traceProvenance()}}, nil, nil, snapshot.IngestSummary{Spans: 4, Traces: 1}))
	if got := missStreakFor(result.Report, KindEndpoint, "frontend:GET /checkout"); got != 1 {
		t.Fatalf("miss streak after reload = %d, want 1", got)
	}
}

func TestCompactionEvictsExpiredRetiredStateAndDiskRepresentation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.StatePath = filepath.Join(dir, "reconciliation-state.json")
	cfg.StableCoreExistenceThreshold = 0.5
	cfg.SoftGapMultiplier = 1
	cfg.HardGapMultiplier = 1
	cfg.MinSoftWindows = 1
	cfg.MinHardWindows = 1
	cfg.SourceResistanceSoftWindows = 0
	cfg.SourceResistanceHardWindows = 0
	cfg.RetirementExistenceThreshold = 0.6
	cfg.RetiredTTL = time.Minute
	cfg.CompactionCadence = 1

	r, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	start := time.Date(2026, 3, 21, 19, 0, 0, 0, time.UTC)
	mustProcess(t, r, observation(start, []serviceSpec{{id: "frontend", support: 8, provenance: traceProvenance()}}, nil, []endpointSpec{{id: "frontend:GET /checkout", service: "frontend", support: 8, provenance: traceProvenance()}}, snapshot.IngestSummary{Spans: 10, Traces: 2}))
	mustProcess(t, r, observation(start.Add(time.Minute), []serviceSpec{{id: "frontend", support: 8, provenance: traceProvenance()}}, nil, nil, snapshot.IngestSummary{Spans: 5, Traces: 1}))
	evicted := mustProcess(t, r, observation(start.Add(3*time.Minute), nil, nil, nil, snapshot.IngestSummary{}))

	if _, ok := r.state.Endpoints["frontend:GET /checkout"]; ok {
		t.Fatal("retired endpoint should be evicted from in-memory state")
	}
	if !evicted.Report.Compaction.Performed || evicted.Report.Compaction.ExpiredRetired == 0 {
		t.Fatalf("expected compaction to evict expired retired entities, got %+v", evicted.Report.Compaction)
	}
	raw, err := os.ReadFile(cfg.StatePath)
	if err != nil {
		t.Fatalf("read persisted state: %v", err)
	}
	if strings.Contains(string(raw), "frontend:GET /checkout") {
		t.Fatalf("persisted state still contains evicted endpoint: %s", raw)
	}
}

func TestBoundedRetentionPreventsRetiredGrowth(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.StatePath = filepath.Join(dir, "reconciliation-state.json")
	cfg.StableCoreExistenceThreshold = 0.5
	cfg.SoftGapMultiplier = 1
	cfg.HardGapMultiplier = 1
	cfg.MinSoftWindows = 1
	cfg.MinHardWindows = 1
	cfg.SourceResistanceSoftWindows = 0
	cfg.SourceResistanceHardWindows = 0
	cfg.RetirementExistenceThreshold = 0.6
	cfg.MaxRetiredPerKind = 2
	cfg.MaxRetiredOverall = 2
	cfg.CompactionCadence = 1

	r, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	start := time.Date(2026, 3, 21, 20, 0, 0, 0, time.UTC)
	endpoints := []endpointSpec{
		{id: "frontend:GET /one", service: "frontend", support: 4, provenance: traceProvenance()},
		{id: "frontend:GET /two", service: "frontend", support: 4, provenance: traceProvenance()},
		{id: "frontend:GET /three", service: "frontend", support: 4, provenance: traceProvenance()},
		{id: "frontend:GET /four", service: "frontend", support: 4, provenance: traceProvenance()},
	}
	mustProcess(t, r, observation(start, []serviceSpec{{id: "frontend", support: 8, provenance: traceProvenance()}}, nil, endpoints, snapshot.IngestSummary{Spans: 16, Traces: 4}))
	latest := mustProcess(t, r, observation(start.Add(time.Minute), []serviceSpec{{id: "frontend", support: 8, provenance: traceProvenance()}}, nil, nil, snapshot.IngestSummary{Spans: 8, Traces: 2}))

	if got := retiredCount(r.state.Endpoints); got > 2 {
		t.Fatalf("retired endpoint count = %d, want <= 2", got)
	}
	if latest.Report.Compaction.TrimmedPerKind == 0 && latest.Report.Compaction.TrimmedOverall == 0 {
		t.Fatalf("expected bounded-retention trimming, got %+v", latest.Report.Compaction)
	}
	raw, err := os.ReadFile(cfg.StatePath)
	if err != nil {
		t.Fatalf("read persisted state: %v", err)
	}
	if strings.Count(string(raw), "\"endpoint_record\"") > 2 {
		t.Fatalf("persisted state retained too many retired endpoints: %s", raw)
	}
}

func newTestReconciler(t *testing.T, mutate func(*Config)) *Reconciler {
	t.Helper()
	cfg := DefaultConfig()
	cfg.StatePath = filepath.Join(t.TempDir(), "reconciliation-state.json")
	if mutate != nil {
		mutate(&cfg)
	}
	r, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return r
}

func mustProcess(t *testing.T, r *Reconciler, obs Observation) Result {
	t.Helper()
	result, err := r.Process(obs)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	return result
}

func observation(start time.Time, services []serviceSpec, _ []struct{}, endpoints []endpointSpec, ingest snapshot.IngestSummary) Observation {
	windowEnd := start.Add(time.Minute)
	mdl := model.ResilienceModel{
		Services:  make([]model.Service, 0, len(services)),
		Edges:     nil,
		Endpoints: make([]model.Endpoint, 0, len(endpoints)),
		Metadata: model.Metadata{
			SourceType:   discovery.SourceTypeBering,
			SourceRef:    "bering://serve?listen=:4318",
			DiscoveredAt: windowEnd.Format(time.RFC3339),
			Confidence:   0.8,
			Schema: model.SchemaRef{
				Name:    schema.ExpectedSchemaName,
				Version: schema.ExpectedSchemaVersion,
				URI:     schema.ExpectedSchemaURI,
				Digest:  schema.ExpectedSchemaDigest,
			},
		},
	}
	details := snapshot.DiscoveryDetails{
		Services:  make([]snapshot.ServiceRecord, 0, len(services)),
		Edges:     nil,
		Endpoints: make([]snapshot.EndpointRecord, 0, len(endpoints)),
	}
	for _, svc := range services {
		mdl.Services = append(mdl.Services, model.Service{ID: svc.id, Name: svc.id, Replicas: 1})
		details.Services = append(details.Services, snapshot.ServiceRecord{
			ID:         svc.id,
			Name:       svc.id,
			Replicas:   1,
			Support:    snapshot.SupportSummary{Observations: svc.support, TraceCount: max(1, svc.support/2)},
			FirstSeen:  start.Format(time.RFC3339),
			LastSeen:   windowEnd.Format(time.RFC3339),
			Provenance: svc.provenance,
		})
	}
	for _, ep := range endpoints {
		mdl.Endpoints = append(mdl.Endpoints, model.Endpoint{ID: ep.id, EntryService: ep.service, SuccessPredicateRef: ep.id})
		details.Endpoints = append(details.Endpoints, snapshot.EndpointRecord{
			ID:           ep.id,
			EntryService: ep.service,
			Method:       "GET",
			Path:         strings.TrimPrefix(strings.SplitN(ep.id, " ", 2)[1], ""),
			Support:      snapshot.SupportSummary{Observations: ep.support, TraceCount: max(1, ep.support/2)},
			FirstSeen:    start.Format(time.RFC3339),
			LastSeen:     windowEnd.Format(time.RFC3339),
			Provenance:   ep.provenance,
		})
	}
	mdl.SortDeterministic()
	return Observation{
		WindowStart: start,
		WindowEnd:   windowEnd,
		Result: discovery.Result{
			Model:      mdl,
			Discovery:  details,
			Coverage:   snapshot.CoverageSummary{Confidence: 0.8, ServiceSupportMin: 1},
			Sources:    []snapshot.SourceSummary{{Type: "traces", Connector: "otlp_http", Ref: "bering://serve?listen=:4318", Observations: ingest.Spans}},
			SpanCount:  ingest.Spans,
			TraceCount: ingest.Traces,
		},
		Ingest: ingest,
	}
}

func traceProvenance() []snapshot.Provenance {
	return []snapshot.Provenance{{Type: "traces", Connector: "otlp_http", Ref: "bering://serve?listen=:4318"}}
}

func overlayProvenance() []snapshot.Provenance {
	return []snapshot.Provenance{
		{Type: "traces", Connector: "otlp_http", Ref: "bering://serve?listen=:4318"},
		{Type: "overlay", Name: "pinned-endpoints", Ref: "configs/discovery.overlay.sample.yaml", Precedence: 1},
	}
}

func lifecycleFor(report Report, kind Kind, id string) Lifecycle {
	for _, item := range report.Entities {
		if item.Kind == kind && item.ID == id {
			return item.Lifecycle
		}
	}
	return ""
}

func missStreakFor(report Report, kind Kind, id string) int {
	for _, item := range report.Entities {
		if item.Kind == kind && item.ID == id {
			return item.MissStreak
		}
	}
	return -1
}

func hardThresholdFor(report Report, kind Kind, id string) int {
	for _, item := range report.Entities {
		if item.Kind == kind && item.ID == id {
			return item.HardThreshold
		}
	}
	return 0
}

func containsEndpoint(projection Projection, id string) bool {
	for _, item := range projection.Model.Endpoints {
		if item.ID == id {
			return true
		}
	}
	return false
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}
