package reconciliation

import (
	"time"

	"github.com/MB3R-Lab/Bering/internal/discovery"
	"github.com/MB3R-Lab/Bering/internal/model"
	"github.com/MB3R-Lab/Bering/internal/schema"
	"github.com/MB3R-Lab/Bering/internal/snapshot"
)

type Reconciler struct {
	cfg          Config
	state        persistedState
	loadWarnings []string
}

func New(cfg Config) (*Reconciler, error) {
	cfg = cfg.WithDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	state, warnings, err := loadState(sanitizeStatePath(cfg.StatePath))
	if err != nil {
		return nil, err
	}
	return &Reconciler{
		cfg:          cfg,
		state:        state,
		loadWarnings: warnings,
	}, nil
}

func (r *Reconciler) Process(obs Observation) (Result, error) {
	if err := obs.Validate(); err != nil {
		return Result{}, err
	}
	rawWindow, err := buildRawProjection(obs)
	if err != nil {
		return Result{}, err
	}
	r.ensureStateMaps()
	r.state.Versions.ObservationVersion++
	observationVersion := r.state.Versions.ObservationVersion
	index := buildObservationIndex(obs)
	health := r.evaluateTelemetryHealth(obs)

	transitions := make([]Transition, 0)
	summaries := make([]EntityStateSummary, 0, len(r.state.Services)+len(r.state.Edges)+len(r.state.Endpoints)+len(index.services)+len(index.edges)+len(index.endpoints))

	for id, snapshot := range index.services {
		state := ensureEntityState(r.state.Services, KindService, id)
		r.observeEntity(state, snapshot, observationVersion, obs.WindowEnd)
	}
	for id, snapshot := range index.edges {
		state := ensureEntityState(r.state.Edges, KindEdge, id)
		r.observeEntity(state, snapshot, observationVersion, obs.WindowEnd)
	}
	for id, snapshot := range index.endpoints {
		state := ensureEntityState(r.state.Endpoints, KindEndpoint, id)
		r.observeEntity(state, snapshot, observationVersion, obs.WindowEnd)
	}

	for _, bucket := range []struct {
		kind        Kind
		states      map[string]*entityState
		opportunity func(string) int
		observed    func(string) bool
	}{
		{
			kind:        KindService,
			states:      r.state.Services,
			opportunity: func(id string) int { return r.serviceOpportunity(id, index) },
			observed:    func(id string) bool { _, ok := index.services[id]; return ok },
		},
		{
			kind:        KindEdge,
			states:      r.state.Edges,
			opportunity: func(id string) int { return r.edgeOpportunity(id, index) },
			observed:    func(id string) bool { _, ok := index.edges[id]; return ok },
		},
		{
			kind:        KindEndpoint,
			states:      r.state.Endpoints,
			opportunity: func(id string) int { return r.endpointOpportunity(id, index) },
			observed:    func(id string) bool { _, ok := index.endpoints[id]; return ok },
		},
	} {
		for id, state := range bucket.states {
			opportunity := bucket.opportunity(id)
			observed := bucket.observed(id)
			transition := r.advanceLifecycle(state, observationVersion, obs.WindowEnd, opportunity, observed, health)
			if transition != nil {
				transitions = append(transitions, *transition)
			}
			soft, hard := gapThresholds(r.cfg, state)
			summaries = append(summaries, EntityStateSummary{
				Kind:                bucket.kind,
				ID:                  id,
				Lifecycle:           state.Lifecycle,
				ExistenceConfidence: state.ExistenceConfidence,
				ActivityConfidence:  state.ActivityConfidence,
				ObservedWindows:     state.ObservedWindows,
				OpportunityWindows:  state.OpportunityWindows,
				MissStreak:          state.MissStreak,
				LastSeenWindow:      state.LastSeenWindow,
				GapEMA:              state.GapEMA,
				SourceStrength:      state.SourceStrength,
				ObservedCurrent:     observed,
				OpportunityCurrent:  opportunity >= r.cfg.MinimumOpportunityObservations,
				QualifiedMiss:       !observed && opportunity >= r.cfg.MinimumOpportunityObservations,
				SoftThreshold:       soft,
				HardThreshold:       hard,
				Reason:              state.Reason,
			})
		}
	}

	sortEntitySummaries(summaries)
	compaction := r.compactIfDue(obs.WindowEnd)
	stableCore, err := r.buildProjection(ProjectionStableCore, obs, func(state *entityState) bool {
		return state.Lifecycle == LifecycleActive &&
			state.ExistenceConfidence >= r.cfg.StableCoreExistenceThreshold &&
			state.ActivityConfidence >= r.cfg.StableCoreActivityThreshold
	})
	if err != nil {
		return Result{}, err
	}
	guardrail, err := r.buildProjection(ProjectionGuardrailUnion, obs, func(state *entityState) bool {
		if state.Lifecycle == LifecycleRetired {
			return false
		}
		if state.LastSeenWindow == observationVersion {
			return true
		}
		if state.SourceStrength >= 0.9 {
			return true
		}
		return state.ExistenceConfidence >= r.cfg.GuardrailUnionExistenceThreshold
	})
	if err != nil {
		return Result{}, err
	}
	r.bumpProjectionVersion(ProjectionStableCore, &stableCore)
	r.bumpProjectionVersion(ProjectionGuardrailUnion, &guardrail)
	report := Report{
		FormatVersion:   defaultFormatVersion,
		WindowStart:     obs.WindowStart.UTC().Format(time.RFC3339),
		WindowEnd:       obs.WindowEnd.UTC().Format(time.RFC3339),
		Versions:        r.state.Versions,
		TelemetryHealth: health,
		Transitions:     transitions,
		Projections: []ProjectionSummary{
			{
				Name:            rawWindow.Name,
				Counts:          rawWindow.Counts,
				TopologyVersion: rawWindow.TopologyVersion,
				Version:         r.state.Versions.ObservationVersion,
				Changed:         rawWindow.Changed,
				Diff:            rawWindow.Diff,
			},
			{
				Name:            stableCore.Name,
				Counts:          stableCore.Counts,
				TopologyVersion: stableCore.TopologyVersion,
				Version:         r.state.Versions.StableCoreVersion,
				Changed:         stableCore.Changed,
				Diff:            stableCore.Diff,
			},
			{
				Name:            guardrail.Name,
				Counts:          guardrail.Counts,
				TopologyVersion: guardrail.TopologyVersion,
				Version:         r.state.Versions.GuardrailUnionVersion,
				Changed:         guardrail.Changed,
				Diff:            guardrail.Diff,
			},
		},
		Entities:     summaries,
		Compaction:   compaction,
		LoadWarnings: append([]string(nil), r.loadWarnings...),
	}
	r.loadWarnings = nil
	if err := saveState(r.cfg.StatePath, r.state); err != nil {
		return Result{}, err
	}
	return Result{
		RawWindow:      rawWindow,
		StableCore:     stableCore,
		GuardrailUnion: guardrail,
		Report:         report,
	}, nil
}

func (r *Reconciler) ensureStateMaps() {
	if r.state.Services == nil {
		r.state.Services = map[string]*entityState{}
	}
	if r.state.Edges == nil {
		r.state.Edges = map[string]*entityState{}
	}
	if r.state.Endpoints == nil {
		r.state.Endpoints = map[string]*entityState{}
	}
	if r.state.FormatVersion == 0 {
		r.state.FormatVersion = defaultFormatVersion
	}
}

func ensureEntityState(bucket map[string]*entityState, kind Kind, id string) *entityState {
	if item, ok := bucket[id]; ok {
		return item
	}
	state := &entityState{
		Kind:                kind,
		ID:                  id,
		Lifecycle:           LifecycleCandidate,
		ExistenceConfidence: 0.25,
		ActivityConfidence:  0.2,
		GapEMA:              1,
	}
	bucket[id] = state
	return state
}

func (r *Reconciler) observeEntity(state *entityState, snapshot entitySnapshot, observationVersion int64, observedAt time.Time) {
	previousLastSeen := state.LastSeenWindow
	state.LastSeenWindow = observationVersion
	state.ObservedWindows++
	state.MissStreak = 0
	state.ExistenceConfidence = clamp01(state.ExistenceConfidence + r.cfg.PositiveExistenceBoost*(0.4+0.6*snapshotSupportStrength(snapshot)))
	state.ActivityConfidence = clamp01(state.ActivityConfidence + r.cfg.PositiveActivityBoost)
	state.SourceStrength = clamp01(maxFloat(state.SourceStrength, snapshotSourceStrength(snapshot)))
	if previousLastSeen > 0 {
		gap := float64(observationVersion - previousLastSeen)
		state.GapEMA = clampGapEMA(state.GapEMA*(1-r.cfg.GapEMAAlpha) + gap*r.cfg.GapEMAAlpha)
	} else {
		state.GapEMA = 1
	}
	state.UpdatedAt = observedAt.UTC().Format(time.RFC3339)
	state.RetiredAt = ""
	state.Reason = "observed in current window"
	switch snapshot.Kind {
	case KindService:
		state.Service = cloneModelService(snapshot.Service)
		state.ServiceRecord = cloneServiceRecord(snapshot.ServiceRecord)
	case KindEdge:
		state.Edge = cloneModelEdge(snapshot.Edge)
		state.EdgeRecord = cloneEdgeRecord(snapshot.EdgeRecord)
	case KindEndpoint:
		state.Endpoint = cloneModelEndpoint(snapshot.Endpoint)
		state.EndpointRecord = cloneEndpointRecord(snapshot.EndpointRecord)
	}
	if state.ExistenceConfidence >= r.cfg.StableCoreExistenceThreshold {
		state.Lifecycle = LifecycleActive
	} else {
		state.Lifecycle = LifecycleCandidate
	}
}

func (r *Reconciler) advanceLifecycle(state *entityState, observationVersion int64, now time.Time, opportunity int, observed bool, health TelemetryHealth) *Transition {
	before := state.Lifecycle
	if observed {
		state.Reason = "observed in current window"
		return lifecycleTransition(state.Kind, state.ID, before, state.Lifecycle, state.Reason)
	}
	if opportunity < r.cfg.MinimumOpportunityObservations {
		state.ActivityConfidence = clamp01(state.ActivityConfidence - r.cfg.PassiveActivityDecay)
		state.Reason = "no qualified opportunity in current window"
		return lifecycleTransition(state.Kind, state.ID, before, state.Lifecycle, state.Reason)
	}
	state.OpportunityWindows++
	state.LastOpportunity = observationVersion
	state.MissStreak++
	soft, hard := gapThresholds(r.cfg, state)
	escalation := maxFloat(0, float64(state.MissStreak-soft+1))
	resistance := 1 - minFloat(0.8, state.SourceStrength*0.6)
	state.ActivityConfidence = clamp01(state.ActivityConfidence - (r.cfg.MissActivityPenalty+r.cfg.AdditionalMissPenalty*escalation)*resistance)
	if state.MissStreak >= soft {
		penalty := (r.cfg.MissExistencePenalty + r.cfg.AdditionalMissPenalty*escalation) * resistance
		if health.RetirementFrozen {
			penalty *= 0.35
		}
		state.ExistenceConfidence = clamp01(state.ExistenceConfidence - penalty)
	}
	state.UpdatedAt = now.UTC().Format(time.RFC3339)
	switch {
	case state.MissStreak >= hard && !health.RetirementFrozen && state.ExistenceConfidence <= r.cfg.RetirementExistenceThreshold:
		state.Lifecycle = LifecycleRetired
		state.RetiredAt = now.UTC().Format(time.RFC3339)
		state.Reason = "retired after repeated qualified misses"
	case state.MissStreak >= soft:
		state.Lifecycle = LifecycleStale
		if health.RetirementFrozen {
			state.Reason = "stale after qualified misses; retirement frozen by telemetry health"
		} else {
			state.Reason = "stale after repeated qualified misses"
		}
	case state.ExistenceConfidence >= r.cfg.StableCoreExistenceThreshold:
		state.Lifecycle = LifecycleActive
		state.Reason = "retained as active"
	default:
		state.Lifecycle = LifecycleCandidate
		state.Reason = "insufficient evidence to promote"
	}
	return lifecycleTransition(state.Kind, state.ID, before, state.Lifecycle, state.Reason)
}

func lifecycleTransition(kind Kind, id string, from, to Lifecycle, reason string) *Transition {
	if from == to {
		return nil
	}
	return &Transition{Kind: kind, ID: id, From: from, To: to, Reason: reason}
}

func snapshotSupportStrength(item entitySnapshot) float64 {
	switch item.Kind {
	case KindService:
		if item.ServiceRecord == nil {
			return 0.5
		}
		return clamp01(float64(item.ServiceRecord.Support.Observations) / 10)
	case KindEdge:
		if item.EdgeRecord == nil {
			return 0.5
		}
		return clamp01(float64(item.EdgeRecord.Support.Observations) / 10)
	case KindEndpoint:
		if item.EndpointRecord == nil {
			return 0.5
		}
		return clamp01(float64(item.EndpointRecord.Support.Observations) / 10)
	default:
		return 0.5
	}
}

func snapshotSourceStrength(item entitySnapshot) float64 {
	switch item.Kind {
	case KindService:
		if item.ServiceRecord == nil {
			return 0.35
		}
		return sourceStrengthFromProvenance(item.ServiceRecord.Provenance)
	case KindEdge:
		if item.EdgeRecord == nil {
			return 0.35
		}
		return sourceStrengthFromProvenance(item.EdgeRecord.Provenance)
	case KindEndpoint:
		if item.EndpointRecord == nil {
			return 0.35
		}
		return sourceStrengthFromProvenance(item.EndpointRecord.Provenance)
	default:
		return 0.35
	}
}

func clampGapEMA(value float64) float64 {
	if value < 1 {
		return 1
	}
	return value
}

func (r *Reconciler) serviceOpportunity(id string, index observationIndex) int {
	opportunity := index.serviceOpportunity[id]
	for _, edge := range r.state.Edges {
		if edge.Edge == nil {
			continue
		}
		if edge.Edge.From == id {
			opportunity += index.serviceSupport[id]
		}
		if edge.Edge.To == id {
			opportunity += index.serviceSupport[edge.Edge.From]
		}
	}
	return opportunity
}

func (r *Reconciler) edgeOpportunity(id string, index observationIndex) int {
	if count := index.edgeOpportunity[id]; count > 0 {
		return count
	}
	state := r.state.Edges[id]
	if state == nil || state.Edge == nil {
		return 0
	}
	return index.serviceSupport[state.Edge.From]
}

func (r *Reconciler) endpointOpportunity(id string, index observationIndex) int {
	if count := index.endpointOpportunity[id]; count > 0 {
		return count
	}
	state := r.state.Endpoints[id]
	if state == nil || state.EndpointRecord == nil {
		return 0
	}
	return index.serviceSupport[state.EndpointRecord.EntryService]
}

func (r *Reconciler) evaluateTelemetryHealth(obs Observation) TelemetryHealth {
	total := float64(obs.Ingest.Spans + obs.Ingest.DroppedSpans)
	if total <= 0 {
		total = 1
	}
	dropRatio := float64(obs.Ingest.DroppedSpans) / total
	lateRatio := float64(obs.Ingest.LateSpans) / total
	currentSpans := float64(obs.Ingest.Spans)
	if r.state.VolumeEMA == 0 {
		r.state.VolumeEMA = currentSpans
	} else {
		r.state.VolumeEMA = (r.state.VolumeEMA * 0.7) + (currentSpans * 0.3)
	}
	volumeRatio := 1.0
	if r.state.VolumeEMA > 0 {
		volumeRatio = currentSpans / r.state.VolumeEMA
	}
	score := 1.0 - dropRatio*0.55 - lateRatio*0.25
	if r.state.VolumeEMA >= float64(r.cfg.MinimumOpportunityObservations*2) && volumeRatio < 0.35 {
		score -= 0.25 * (0.35 - volumeRatio) / 0.35
	}
	score = clamp01(score)
	reasons := make([]string, 0)
	if dropRatio > 0.15 {
		reasons = append(reasons, "high dropped span ratio")
	}
	if lateRatio > 0.2 {
		reasons = append(reasons, "high late span ratio")
	}
	if r.state.VolumeEMA >= float64(r.cfg.MinimumOpportunityObservations*2) && volumeRatio < 0.35 {
		reasons = append(reasons, "ingest volume below recent baseline")
	}
	frozen := score < r.cfg.TelemetryFreezeThreshold
	if frozen {
		reasons = append(reasons, "retirement frozen")
	}
	return TelemetryHealth{
		Score:            score,
		DropRatio:        clamp01(dropRatio),
		LateRatio:        clamp01(lateRatio),
		VolumeEMA:        r.state.VolumeEMA,
		VolumeRatio:      clamp01(volumeRatio),
		RetirementFrozen: frozen,
		Reasons:          reasons,
	}
}

func (r *Reconciler) buildProjection(name ProjectionName, obs Observation, include func(*entityState) bool) (Projection, error) {
	mdl := model.ResilienceModel{
		Services:  make([]model.Service, 0),
		Edges:     make([]model.Edge, 0),
		Endpoints: make([]model.Endpoint, 0),
		Metadata:  obs.Result.Model.Metadata,
	}
	details := snapshot.DiscoveryDetails{
		Services:  make([]snapshot.ServiceRecord, 0),
		Edges:     make([]snapshot.EdgeRecord, 0),
		Endpoints: make([]snapshot.EndpointRecord, 0),
		Overlays:  obs.Result.Discovery.Overlays,
	}
	serviceSet := map[string]struct{}{}
	for _, state := range r.state.Services {
		if !include(state) || state.Service == nil || state.ServiceRecord == nil {
			continue
		}
		mdl.Services = append(mdl.Services, *cloneModelService(state.Service))
		details.Services = append(details.Services, *cloneServiceRecord(state.ServiceRecord))
		serviceSet[state.ID] = struct{}{}
	}
	for _, state := range r.state.Edges {
		if !include(state) || state.Edge == nil || state.EdgeRecord == nil {
			continue
		}
		if _, ok := serviceSet[state.Edge.From]; !ok {
			continue
		}
		if _, ok := serviceSet[state.Edge.To]; !ok {
			continue
		}
		mdl.Edges = append(mdl.Edges, *cloneModelEdge(state.Edge))
		details.Edges = append(details.Edges, *cloneEdgeRecord(state.EdgeRecord))
	}
	for _, state := range r.state.Endpoints {
		if !include(state) || state.Endpoint == nil || state.EndpointRecord == nil {
			continue
		}
		if _, ok := serviceSet[state.Endpoint.EntryService]; !ok {
			continue
		}
		mdl.Endpoints = append(mdl.Endpoints, *cloneModelEndpoint(state.Endpoint))
		details.Endpoints = append(details.Endpoints, *cloneEndpointRecord(state.EndpointRecord))
	}
	if mdl.Metadata.SourceType == "" {
		sourceRef := obs.Result.Model.Metadata.SourceRef
		if sourceRef == "" && len(obs.Result.Sources) > 0 {
			sourceRef = obs.Result.Sources[0].Ref
		}
		if sourceRef == "" {
			sourceRef = "bering://serve"
		}
		mdl.Metadata = model.Metadata{
			SourceType:   discovery.SourceTypeBering,
			SourceRef:    sourceRef,
			DiscoveredAt: obs.WindowEnd.UTC().Format(time.RFC3339),
			Confidence:   obs.Result.Coverage.Confidence,
			Schema: model.SchemaRef{
				Name:    schema.ExpectedSchemaName,
				Version: schema.ExpectedSchemaVersion,
				URI:     schema.ExpectedSchemaURI,
				Digest:  schema.ExpectedSchemaDigest,
			},
		}
	}
	mdl.SortDeterministic()
	hash, err := topologyFingerprint(mdl)
	if err != nil {
		return Projection{}, err
	}
	previousHash := ""
	switch name {
	case ProjectionStableCore:
		previousHash = r.state.StableCoreTopologyHash
	case ProjectionGuardrailUnion:
		previousHash = r.state.GuardrailUnionTopologyHash
	}
	return Projection{
		Name:            name,
		Model:           mdl,
		Discovery:       details,
		Coverage:        obs.Result.Coverage,
		Sources:         cloneSources(obs.Result.Sources),
		Counts:          countModel(mdl),
		TopologyVersion: hash,
		Changed:         hash != previousHash,
		Diff: snapshot.DiffSummary{
			AddedServices:  len(details.Services),
			AddedEdges:     len(details.Edges),
			AddedEndpoints: len(details.Endpoints),
		},
	}, nil
}

func (r *Reconciler) bumpProjectionVersion(name ProjectionName, projection *Projection) {
	if projection == nil {
		return
	}
	switch name {
	case ProjectionStableCore:
		if projection.Changed {
			r.state.Versions.StableCoreVersion++
		}
		r.state.StableCoreTopologyHash = projection.TopologyVersion
	case ProjectionGuardrailUnion:
		if projection.Changed {
			r.state.Versions.GuardrailUnionVersion++
		}
		r.state.GuardrailUnionTopologyHash = projection.TopologyVersion
	}
}

func maxFloat(left, right float64) float64 {
	if left > right {
		return left
	}
	return right
}

func minFloat(left, right float64) float64 {
	if left < right {
		return left
	}
	return right
}

func (r *Reconciler) compactIfDue(now time.Time) CompactionSummary {
	if r.state.Versions.ObservationVersion-r.state.LastCompactionVersion < int64(r.cfg.CompactionCadence) {
		return CompactionSummary{}
	}
	summary := r.compact(now)
	summary.Performed = true
	r.state.LastCompactionVersion = r.state.Versions.ObservationVersion
	return summary
}

func (r *Reconciler) compact(now time.Time) CompactionSummary {
	summary := CompactionSummary{}
	for _, bucket := range []map[string]*entityState{r.state.Services, r.state.Edges, r.state.Endpoints} {
		for id, state := range bucket {
			if state.Lifecycle != LifecycleRetired || state.RetiredAt == "" {
				continue
			}
			retiredAt, err := time.Parse(time.RFC3339, state.RetiredAt)
			if err != nil || now.Sub(retiredAt) > r.cfg.RetiredTTL {
				delete(bucket, id)
				summary.ExpiredRetired++
			}
		}
	}
	summary.TrimmedPerKind += trimRetiredBucket(r.state.Services, r.cfg.MaxRetiredPerKind)
	summary.TrimmedPerKind += trimRetiredBucket(r.state.Edges, r.cfg.MaxRetiredPerKind)
	summary.TrimmedPerKind += trimRetiredBucket(r.state.Endpoints, r.cfg.MaxRetiredPerKind)
	totalRetired := retiredCount(r.state.Services) + retiredCount(r.state.Edges) + retiredCount(r.state.Endpoints)
	if totalRetired > r.cfg.MaxRetiredOverall {
		summary.TrimmedOverall = trimRetiredOverall([]map[string]*entityState{r.state.Services, r.state.Edges, r.state.Endpoints}, totalRetired-r.cfg.MaxRetiredOverall)
	}
	return summary
}
