package reconciliation

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/MB3R-Lab/Bering/internal/discovery"
	"github.com/MB3R-Lab/Bering/internal/model"
	"github.com/MB3R-Lab/Bering/internal/snapshot"
)

type Kind string

const (
	KindService  Kind = "service"
	KindEdge     Kind = "edge"
	KindEndpoint Kind = "endpoint"
)

type Lifecycle string

const (
	LifecycleCandidate Lifecycle = "candidate"
	LifecycleActive    Lifecycle = "active"
	LifecycleStale     Lifecycle = "stale"
	LifecycleRetired   Lifecycle = "retired"
)

type Observation struct {
	WindowStart time.Time
	WindowEnd   time.Time
	Result      discovery.Result
	Ingest      snapshot.IngestSummary
}

func (o Observation) Validate() error {
	if o.WindowStart.IsZero() || o.WindowEnd.IsZero() {
		return fmt.Errorf("window boundaries must be set")
	}
	if !o.WindowEnd.After(o.WindowStart) {
		return fmt.Errorf("window_end must be after window_start")
	}
	return nil
}

type Versions struct {
	ObservationVersion    int64 `json:"observation_version"`
	StableCoreVersion     int64 `json:"stable_core_version"`
	GuardrailUnionVersion int64 `json:"guardrail_union_version"`
}

type ProjectionName string

const (
	ProjectionRawWindow      ProjectionName = "raw_window"
	ProjectionStableCore     ProjectionName = "stable_core"
	ProjectionGuardrailUnion ProjectionName = "guardrail_union"
)

type Projection struct {
	Name            ProjectionName            `json:"name"`
	Model           model.ResilienceModel     `json:"model"`
	Discovery       snapshot.DiscoveryDetails `json:"discovery"`
	Coverage        snapshot.CoverageSummary  `json:"coverage"`
	Sources         []snapshot.SourceSummary  `json:"sources"`
	Counts          snapshot.Counts           `json:"counts"`
	TopologyVersion string                    `json:"topology_version"`
	Changed         bool                      `json:"changed"`
	Diff            snapshot.DiffSummary      `json:"diff"`
}

type ProjectionSummary struct {
	Name            ProjectionName       `json:"name"`
	Counts          snapshot.Counts      `json:"counts"`
	TopologyVersion string               `json:"topology_version"`
	Version         int64                `json:"version"`
	Changed         bool                 `json:"changed"`
	Diff            snapshot.DiffSummary `json:"diff"`
}

type TelemetryHealth struct {
	Score            float64  `json:"score"`
	DropRatio        float64  `json:"drop_ratio"`
	LateRatio        float64  `json:"late_ratio"`
	VolumeEMA        float64  `json:"volume_ema"`
	VolumeRatio      float64  `json:"volume_ratio"`
	RetirementFrozen bool     `json:"retirement_frozen"`
	Reasons          []string `json:"reasons,omitempty"`
}

type EntityStateSummary struct {
	Kind                Kind      `json:"kind"`
	ID                  string    `json:"id"`
	Lifecycle           Lifecycle `json:"lifecycle"`
	ExistenceConfidence float64   `json:"existence_confidence"`
	ActivityConfidence  float64   `json:"activity_confidence"`
	ObservedWindows     int       `json:"observed_windows"`
	OpportunityWindows  int       `json:"opportunity_windows"`
	MissStreak          int       `json:"miss_streak"`
	LastSeenWindow      int64     `json:"last_seen_window"`
	GapEMA              float64   `json:"gap_ema"`
	SourceStrength      float64   `json:"source_strength"`
	ObservedCurrent     bool      `json:"observed_current_window"`
	OpportunityCurrent  bool      `json:"opportunity_current_window"`
	QualifiedMiss       bool      `json:"qualified_miss"`
	SoftThreshold       int       `json:"soft_threshold"`
	HardThreshold       int       `json:"hard_threshold"`
	Reason              string    `json:"reason,omitempty"`
}

type Transition struct {
	Kind   Kind      `json:"kind"`
	ID     string    `json:"id"`
	From   Lifecycle `json:"from"`
	To     Lifecycle `json:"to"`
	Reason string    `json:"reason"`
}

type CompactionSummary struct {
	Performed      bool `json:"performed"`
	ExpiredRetired int  `json:"expired_retired"`
	TrimmedPerKind int  `json:"trimmed_per_kind"`
	TrimmedOverall int  `json:"trimmed_overall"`
}

type Report struct {
	FormatVersion   int                  `json:"format_version"`
	WindowStart     string               `json:"window_start"`
	WindowEnd       string               `json:"window_end"`
	Versions        Versions             `json:"versions"`
	TelemetryHealth TelemetryHealth      `json:"telemetry_health"`
	Transitions     []Transition         `json:"transitions,omitempty"`
	Projections     []ProjectionSummary  `json:"projections"`
	Entities        []EntityStateSummary `json:"entities"`
	Compaction      CompactionSummary    `json:"compaction"`
	LoadWarnings    []string             `json:"load_warnings,omitempty"`
}

type Result struct {
	RawWindow      Projection `json:"raw_window"`
	StableCore     Projection `json:"stable_core"`
	GuardrailUnion Projection `json:"guardrail_union"`
	Report         Report     `json:"report"`
}

type entitySnapshot struct {
	Kind     Kind
	ID       string
	Service  *model.Service
	Edge     *model.Edge
	Endpoint *model.Endpoint

	ServiceRecord  *snapshot.ServiceRecord
	EdgeRecord     *snapshot.EdgeRecord
	EndpointRecord *snapshot.EndpointRecord
}

type entityState struct {
	Kind                Kind      `json:"kind"`
	ID                  string    `json:"id"`
	Lifecycle           Lifecycle `json:"lifecycle"`
	ExistenceConfidence float64   `json:"existence_confidence"`
	ActivityConfidence  float64   `json:"activity_confidence"`
	ObservedWindows     int       `json:"observed_windows"`
	OpportunityWindows  int       `json:"opportunity_windows"`
	MissStreak          int       `json:"miss_streak"`
	LastSeenWindow      int64     `json:"last_seen_window"`
	LastOpportunity     int64     `json:"last_opportunity_window"`
	GapEMA              float64   `json:"gap_ema"`
	SourceStrength      float64   `json:"source_strength"`
	RetiredAt           string    `json:"retired_at,omitempty"`
	UpdatedAt           string    `json:"updated_at,omitempty"`
	Reason              string    `json:"reason,omitempty"`

	Service        *model.Service           `json:"service,omitempty"`
	Edge           *model.Edge              `json:"edge,omitempty"`
	Endpoint       *model.Endpoint          `json:"endpoint,omitempty"`
	ServiceRecord  *snapshot.ServiceRecord  `json:"service_record,omitempty"`
	EdgeRecord     *snapshot.EdgeRecord     `json:"edge_record,omitempty"`
	EndpointRecord *snapshot.EndpointRecord `json:"endpoint_record,omitempty"`
}

type persistedState struct {
	FormatVersion              int                     `json:"format_version"`
	Versions                   Versions                `json:"versions"`
	StableCoreTopologyHash     string                  `json:"stable_core_topology_hash,omitempty"`
	GuardrailUnionTopologyHash string                  `json:"guardrail_union_topology_hash,omitempty"`
	VolumeEMA                  float64                 `json:"volume_ema"`
	LastCompactionVersion      int64                   `json:"last_compaction_version"`
	Services                   map[string]*entityState `json:"services"`
	Edges                      map[string]*entityState `json:"edges"`
	Endpoints                  map[string]*entityState `json:"endpoints"`
}

func newPersistedState() persistedState {
	return persistedState{
		FormatVersion: defaultFormatVersion,
		Services:      map[string]*entityState{},
		Edges:         map[string]*entityState{},
		Endpoints:     map[string]*entityState{},
	}
}

func buildRawProjection(obs Observation) (Projection, error) {
	result := obs.Result
	mdl := result.Model
	if len(mdl.Services) > 0 {
		if err := mdl.ValidateSemantic(); err != nil {
			return Projection{}, err
		}
	}
	if mdl.Metadata.DiscoveredAt == "" {
		mdl.Metadata = obs.Result.Model.Metadata
	}
	hash, err := topologyFingerprint(mdl)
	if err != nil {
		return Projection{}, err
	}
	return Projection{
		Name:            ProjectionRawWindow,
		Model:           mdl,
		Discovery:       result.Discovery,
		Coverage:        result.Coverage,
		Sources:         cloneSources(result.Sources),
		Counts:          countModel(mdl),
		TopologyVersion: hash,
		Changed:         true,
		Diff: snapshot.DiffSummary{
			AddedServices:  len(result.Discovery.Services),
			AddedEdges:     len(result.Discovery.Edges),
			AddedEndpoints: len(result.Discovery.Endpoints),
		},
	}, nil
}

func countModel(mdl model.ResilienceModel) snapshot.Counts {
	return snapshot.Counts{
		Services:  len(mdl.Services),
		Edges:     len(mdl.Edges),
		Endpoints: len(mdl.Endpoints),
	}
}

func cloneSources(items []snapshot.SourceSummary) []snapshot.SourceSummary {
	out := make([]snapshot.SourceSummary, len(items))
	copy(out, items)
	return out
}

func gapThresholds(cfg Config, state *entityState) (int, int) {
	gap := math.Max(1, state.GapEMA)
	soft := int(math.Ceil(gap * cfg.SoftGapMultiplier))
	hard := int(math.Ceil(gap * cfg.HardGapMultiplier))
	soft = maxInt(soft, cfg.MinSoftWindows)
	hard = maxInt(hard, cfg.MinHardWindows)
	soft += int(math.Round(state.SourceStrength * cfg.SourceResistanceSoftWindows))
	hard += int(math.Round(state.SourceStrength * cfg.SourceResistanceHardWindows))
	if hard < soft {
		hard = soft
	}
	return soft, hard
}

func sourceStrengthFromProvenance(items []snapshot.Provenance) float64 {
	score := 0.2
	for _, item := range items {
		switch strings.TrimSpace(strings.ToLower(item.Type)) {
		case "overlay":
			score = math.Max(score, 1.0)
		case "topology_api":
			score = math.Max(score, 0.95)
		case "traces":
			score = math.Max(score, 0.35)
		default:
			score = math.Max(score, 0.5)
		}
	}
	return score
}

func buildObservationIndex(obs Observation) observationIndex {
	index := observationIndex{
		services:            map[string]entitySnapshot{},
		edges:               map[string]entitySnapshot{},
		endpoints:           map[string]entitySnapshot{},
		serviceSupport:      map[string]int{},
		endpointOpportunity: map[string]int{},
		edgeOpportunity:     map[string]int{},
		serviceOpportunity:  map[string]int{},
	}
	serviceRecords := make(map[string]snapshot.ServiceRecord, len(obs.Result.Discovery.Services))
	for _, item := range obs.Result.Discovery.Services {
		serviceRecords[item.ID] = item
	}
	for _, item := range obs.Result.Model.Services {
		record, ok := serviceRecords[item.ID]
		if !ok {
			continue
		}
		copyItem := item
		copyRecord := record
		index.services[item.ID] = entitySnapshot{Kind: KindService, ID: item.ID, Service: &copyItem, ServiceRecord: &copyRecord}
		index.serviceSupport[item.ID] = record.Support.Observations
	}
	modelEndpoints := make(map[string]model.Endpoint, len(obs.Result.Model.Endpoints))
	for _, item := range obs.Result.Model.Endpoints {
		modelEndpoints[item.ID] = item
	}
	for _, record := range obs.Result.Discovery.Endpoints {
		endpoint, ok := modelEndpoints[record.ID]
		if !ok {
			continue
		}
		copyEndpoint := endpoint
		copyRecord := record
		index.endpoints[record.ID] = entitySnapshot{Kind: KindEndpoint, ID: record.ID, Endpoint: &copyEndpoint, EndpointRecord: &copyRecord}
		index.endpointOpportunity[record.ID] = record.Support.Observations
		index.serviceOpportunity[record.EntryService] += record.Support.Observations
	}
	modelEdges := make(map[string]model.Edge, len(obs.Result.Model.Edges))
	for _, item := range obs.Result.Model.Edges {
		modelEdges[edgeIdentity(item)] = item
	}
	for _, record := range obs.Result.Discovery.Edges {
		modelEdge, ok := modelEdges[record.ID]
		if !ok {
			continue
		}
		copyEdge := modelEdge
		copyRecord := record
		index.edges[record.ID] = entitySnapshot{Kind: KindEdge, ID: record.ID, Edge: &copyEdge, EdgeRecord: &copyRecord}
		index.edgeOpportunity[record.ID] = index.serviceSupport[record.From]
		index.serviceOpportunity[record.From] += record.Support.Observations
		index.serviceOpportunity[record.To] += record.Support.Observations
	}
	for id, count := range index.serviceSupport {
		index.serviceOpportunity[id] += count
	}
	return index
}

type observationIndex struct {
	services            map[string]entitySnapshot
	edges               map[string]entitySnapshot
	endpoints           map[string]entitySnapshot
	serviceSupport      map[string]int
	serviceOpportunity  map[string]int
	edgeOpportunity     map[string]int
	endpointOpportunity map[string]int
}

func edgeIdentity(edge model.Edge) string {
	return fmt.Sprintf("%s|%s|%s|%t", edge.From, edge.To, edge.Kind, edge.Blocking)
}

func cloneModelService(item *model.Service) *model.Service {
	if item == nil {
		return nil
	}
	copyItem := *item
	return &copyItem
}

func cloneModelEdge(item *model.Edge) *model.Edge {
	if item == nil {
		return nil
	}
	copyItem := *item
	return &copyItem
}

func cloneModelEndpoint(item *model.Endpoint) *model.Endpoint {
	if item == nil {
		return nil
	}
	copyItem := *item
	return &copyItem
}

func cloneServiceRecord(item *snapshot.ServiceRecord) *snapshot.ServiceRecord {
	if item == nil {
		return nil
	}
	copyItem := *item
	return &copyItem
}

func cloneEdgeRecord(item *snapshot.EdgeRecord) *snapshot.EdgeRecord {
	if item == nil {
		return nil
	}
	copyItem := *item
	return &copyItem
}

func cloneEndpointRecord(item *snapshot.EndpointRecord) *snapshot.EndpointRecord {
	if item == nil {
		return nil
	}
	copyItem := *item
	return &copyItem
}

func sortEntitySummaries(items []EntityStateSummary) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		return items[i].ID < items[j].ID
	})
}

func clamp01(value float64) float64 {
	switch {
	case value < 0:
		return 0
	case value > 1:
		return 1
	default:
		return math.Round(value*1000) / 1000
	}
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
