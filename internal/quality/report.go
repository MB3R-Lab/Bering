package quality

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MB3R-Lab/Bering/internal/atomicfile"
	"github.com/MB3R-Lab/Bering/internal/jsoncanon"
	"github.com/MB3R-Lab/Bering/internal/model"
	"github.com/MB3R-Lab/Bering/internal/snapshot"
)

const SchemaVersion = "1.0.0"

type ArtifactRef struct {
	Type            string          `json:"type"`
	Path            string          `json:"path,omitempty"`
	Schema          model.SchemaRef `json:"schema"`
	SnapshotID      string          `json:"snapshot_id,omitempty"`
	TopologyVersion string          `json:"topology_version,omitempty"`
	WindowStart     string          `json:"window_start,omitempty"`
	WindowEnd       string          `json:"window_end,omitempty"`
}

type Report struct {
	SchemaVersion string                   `json:"schema_version"`
	GeneratedAt   string                   `json:"generated_at"`
	Artifact      ArtifactRef              `json:"artifact"`
	Counts        snapshot.Counts          `json:"counts"`
	Coverage      snapshot.CoverageSummary `json:"coverage"`
	SourceMix     SourceMix                `json:"source_mix"`
	Dimensions    []Dimension              `json:"dimensions"`
	Warnings      []string                 `json:"warnings,omitempty"`
}

type SourceMix struct {
	Records                  int            `json:"records"`
	RecordsWithProvenance    int            `json:"records_with_provenance"`
	SourcesByType            map[string]int `json:"sources_by_type,omitempty"`
	SourceObservationsByType map[string]int `json:"source_observations_by_type,omitempty"`
	ProvenanceRecordsByType  map[string]int `json:"provenance_records_by_type,omitempty"`
	ProvenanceMentionsByType map[string]int `json:"provenance_mentions_by_type,omitempty"`
	RecordsWithoutProvenance int            `json:"records_without_provenance"`
}

type Dimension struct {
	Name         string   `json:"name"`
	Status       string   `json:"status"`
	Present      int      `json:"present"`
	Total        int      `json:"total"`
	Ratio        float64  `json:"ratio"`
	MissingCount int      `json:"missing_count"`
	Missing      []string `json:"missing,omitempty"`
}

func FromModel(mdl model.ResilienceModel, details snapshot.DiscoveryDetails, sources []snapshot.SourceSummary, coverage snapshot.CoverageSummary, generatedAt, path string) Report {
	return Build(mdl, details, sources, coverage, snapshot.Counts{
		Services:  len(mdl.Services),
		Edges:     len(mdl.Edges),
		Endpoints: len(mdl.Endpoints),
	}, ArtifactRef{
		Type:   "model",
		Path:   path,
		Schema: mdl.Metadata.Schema,
	}, generatedAt)
}

func FromSnapshot(env snapshot.Envelope, path string) Report {
	return FromSnapshotArtifact(env, "snapshot", path)
}

func FromSnapshotArtifact(env snapshot.Envelope, artifactType, path string) Report {
	return Build(env.Model, env.Discovery, env.Sources, env.Coverage, env.Counts, ArtifactRef{
		Type:            artifactType,
		Path:            path,
		Schema:          env.Metadata.Schema,
		SnapshotID:      env.SnapshotID,
		TopologyVersion: env.TopologyVersion,
		WindowStart:     env.WindowStart,
		WindowEnd:       env.WindowEnd,
	}, env.Metadata.EmittedAt)
}

func Build(mdl model.ResilienceModel, details snapshot.DiscoveryDetails, sources []snapshot.SourceSummary, coverage snapshot.CoverageSummary, counts snapshot.Counts, artifact ArtifactRef, generatedAt string) Report {
	dimensions := buildDimensions(mdl, details)
	sourceMix := buildSourceMix(details, sources)
	warnings := buildWarnings(dimensions, sourceMix)
	return Report{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   generatedAt,
		Artifact:      artifact,
		Counts:        counts,
		Coverage:      coverage,
		SourceMix:     sourceMix,
		Dimensions:    dimensions,
		Warnings:      warnings,
	}
}

func WriteFile(path string, report Report) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	raw, err := jsoncanon.MarshalIndent(report)
	if err != nil {
		return fmt.Errorf("marshal signal quality report: %w", err)
	}
	if err := atomicfile.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write signal quality report: %w", err)
	}
	return nil
}

func SidecarPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	ext := filepath.Ext(trimmed)
	if ext == "" {
		return trimmed + ".signal-quality.json"
	}
	return strings.TrimSuffix(trimmed, ext) + ".signal-quality.json"
}

func buildDimensions(mdl model.ResilienceModel, details snapshot.DiscoveryDetails) []Dimension {
	serviceIDs := make([]string, 0, len(mdl.Services))
	servicePresent := map[string]map[string]bool{}
	for _, svc := range mdl.Services {
		id := strings.TrimSpace(svc.ID)
		serviceIDs = append(serviceIDs, id)
		servicePresent[id] = map[string]bool{
			"services.failure_eligible":     svc.Metadata != nil && svc.Metadata.FailureEligible != nil,
			"services.placements":           svc.Metadata != nil && len(svc.Metadata.Placements) > 0,
			"services.shared_resource_refs": svc.Metadata != nil && len(svc.Metadata.SharedResourceRefs) > 0,
		}
	}

	edgeIDs := make([]string, 0, len(mdl.Edges))
	edgePresent := map[string]map[string]bool{}
	for _, edge := range mdl.Edges {
		id := edgeID(edge)
		edgeIDs = append(edgeIDs, id)
		edgePresent[id] = map[string]bool{
			"edges.weight":              edge.Metadata != nil && edge.Metadata.Weight != nil,
			"edges.request_timeout_ms":  edge.Resilience != nil && edge.Resilience.RequestTimeoutMS != nil,
			"edges.retry_policy":        edge.Resilience != nil && edge.Resilience.Retry != nil,
			"edges.circuit_breaker":     edge.Resilience != nil && edge.Resilience.CircuitBreaker != nil,
			"edges.observed_latency":    hasLatency(edge.Observed),
			"edges.observed_error_rate": edge.Observed != nil && edge.Observed.ErrorRate != nil,
			"edges.policy_scope":        hasPolicyScope(edge.PolicyScope),
		}
	}

	endpointIDs := make([]string, 0, len(mdl.Endpoints))
	endpointPresent := map[string]map[string]bool{}
	for _, endpoint := range mdl.Endpoints {
		id := strings.TrimSpace(endpoint.ID)
		endpointIDs = append(endpointIDs, id)
		endpointPresent[id] = map[string]bool{
			"endpoints.route":                    strings.TrimSpace(endpoint.Method) != "" && strings.TrimSpace(endpoint.Path) != "",
			"endpoints.weight":                   endpoint.Metadata != nil && endpoint.Metadata.Weight != nil,
			"endpoints.custom_success_predicate": strings.TrimSpace(endpoint.SuccessPredicateRef) != "" && endpoint.SuccessPredicateRef != endpoint.ID,
		}
	}

	recordIDs := discoveryRecordIDs(details)
	provenancePresent := provenancePresence(details)

	return []Dimension{
		dimensionFromPresence("services.failure_eligible", serviceIDs, servicePresent),
		dimensionFromPresence("services.placements", serviceIDs, servicePresent),
		dimensionFromPresence("services.shared_resource_refs", serviceIDs, servicePresent),
		dimensionFromPresence("edges.weight", edgeIDs, edgePresent),
		dimensionFromPresence("edges.request_timeout_ms", edgeIDs, edgePresent),
		dimensionFromPresence("edges.retry_policy", edgeIDs, edgePresent),
		dimensionFromPresence("edges.circuit_breaker", edgeIDs, edgePresent),
		dimensionFromPresence("edges.observed_latency", edgeIDs, edgePresent),
		dimensionFromPresence("edges.observed_error_rate", edgeIDs, edgePresent),
		dimensionFromPresence("edges.policy_scope", edgeIDs, edgePresent),
		dimensionFromPresence("endpoints.route", endpointIDs, endpointPresent),
		dimensionFromPresence("endpoints.weight", endpointIDs, endpointPresent),
		dimensionFromPresence("endpoints.custom_success_predicate", endpointIDs, endpointPresent),
		dimensionFromPresence("provenance.traces", recordIDs, provenancePresent["traces"]),
		dimensionFromPresence("provenance.topology_api", recordIDs, provenancePresent["topology_api"]),
		dimensionFromPresence("provenance.overlay", recordIDs, provenancePresent["overlay"]),
	}
}

func dimensionFromPresence(name string, ids []string, presentByID map[string]map[string]bool) Dimension {
	sort.Strings(ids)
	seen := map[string]struct{}{}
	missing := make([]string, 0)
	present := 0
	total := 0
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		total++
		if presentByID[id] != nil && presentByID[id][name] {
			present++
			continue
		}
		missing = append(missing, id)
	}
	return makeDimension(name, present, total, missing)
}

func makeDimension(name string, present, total int, missing []string) Dimension {
	sort.Strings(missing)
	ratio := 0.0
	status := "not_applicable"
	if total > 0 {
		ratio = round(float64(present) / float64(total))
		switch {
		case present == total:
			status = "complete"
		case present == 0:
			status = "missing"
		default:
			status = "partial"
		}
	}
	return Dimension{
		Name:         name,
		Status:       status,
		Present:      present,
		Total:        total,
		Ratio:        ratio,
		MissingCount: total - present,
		Missing:      trimMissing(missing),
	}
}

func buildSourceMix(details snapshot.DiscoveryDetails, sources []snapshot.SourceSummary) SourceMix {
	mix := SourceMix{
		SourcesByType:            map[string]int{},
		SourceObservationsByType: map[string]int{},
		ProvenanceRecordsByType:  map[string]int{},
		ProvenanceMentionsByType: map[string]int{},
	}
	for _, source := range sources {
		sourceType := strings.TrimSpace(source.Type)
		if sourceType == "" {
			sourceType = "unknown"
		}
		mix.SourcesByType[sourceType]++
		mix.SourceObservationsByType[sourceType] += source.Observations
	}

	for _, provenance := range allRecordProvenance(details) {
		mix.Records++
		if len(provenance) == 0 {
			continue
		}
		mix.RecordsWithProvenance++
		perRecord := map[string]struct{}{}
		for _, item := range provenance {
			provType := strings.TrimSpace(item.Type)
			if provType == "" {
				provType = "unknown"
			}
			mix.ProvenanceMentionsByType[provType]++
			perRecord[provType] = struct{}{}
		}
		for provType := range perRecord {
			mix.ProvenanceRecordsByType[provType]++
		}
	}
	mix.RecordsWithoutProvenance = mix.Records - mix.RecordsWithProvenance
	if len(mix.SourcesByType) == 0 {
		mix.SourcesByType = nil
	}
	if len(mix.SourceObservationsByType) == 0 {
		mix.SourceObservationsByType = nil
	}
	if len(mix.ProvenanceRecordsByType) == 0 {
		mix.ProvenanceRecordsByType = nil
	}
	if len(mix.ProvenanceMentionsByType) == 0 {
		mix.ProvenanceMentionsByType = nil
	}
	return mix
}

func buildWarnings(dimensions []Dimension, sourceMix SourceMix) []string {
	warnings := make([]string, 0)
	if sourceMix.Records > 0 {
		_, hasTopology := sourceMix.ProvenanceRecordsByType["topology_api"]
		_, hasOverlay := sourceMix.ProvenanceRecordsByType["overlay"]
		if !hasTopology && !hasOverlay {
			warnings = append(warnings, "no topology_api or overlay provenance; typed placement and policy coverage depends on trace-derived evidence only")
		}
	}
	for _, dimension := range dimensions {
		if strings.HasPrefix(dimension.Name, "provenance.") || dimension.Total == 0 || dimension.Ratio >= 0.5 {
			continue
		}
		warnings = append(warnings, fmt.Sprintf("%s coverage below 50%% (%d/%d)", dimension.Name, dimension.Present, dimension.Total))
	}
	sort.Strings(warnings)
	return warnings
}

func hasLatency(observed *model.ObservedEdge) bool {
	if observed == nil || observed.LatencyMS == nil {
		return false
	}
	latency := observed.LatencyMS
	return latency.P50 != nil || latency.P90 != nil || latency.P95 != nil || latency.P99 != nil
}

func hasPolicyScope(scope *model.PolicyScope) bool {
	if scope == nil {
		return false
	}
	return strings.TrimSpace(scope.SourceEndpointID) != "" ||
		strings.TrimSpace(scope.SourceRoute) != "" ||
		strings.TrimSpace(scope.Method) != "" ||
		strings.TrimSpace(scope.Operation) != ""
}

func edgeID(edge model.Edge) string {
	if strings.TrimSpace(edge.ID) != "" {
		return strings.TrimSpace(edge.ID)
	}
	return model.EdgeID(edge.From, edge.To, edge.Kind, edge.Blocking)
}

func discoveryRecordIDs(details snapshot.DiscoveryDetails) []string {
	out := make([]string, 0, len(details.Services)+len(details.Edges)+len(details.Endpoints))
	for _, item := range details.Services {
		out = append(out, "service:"+item.ID)
	}
	for _, item := range details.Edges {
		out = append(out, "edge:"+item.ID)
	}
	for _, item := range details.Endpoints {
		out = append(out, "endpoint:"+item.ID)
	}
	return out
}

func provenancePresence(details snapshot.DiscoveryDetails) map[string]map[string]map[string]bool {
	out := map[string]map[string]map[string]bool{
		"traces":       {},
		"topology_api": {},
		"overlay":      {},
	}
	add := func(id string, provenance []snapshot.Provenance) {
		for _, item := range provenance {
			provType := strings.TrimSpace(item.Type)
			if out[provType] == nil {
				out[provType] = map[string]map[string]bool{}
			}
			if out[provType][id] == nil {
				out[provType][id] = map[string]bool{}
			}
			out[provType][id]["provenance."+provType] = true
		}
	}
	for _, item := range details.Services {
		add("service:"+item.ID, item.Provenance)
	}
	for _, item := range details.Edges {
		add("edge:"+item.ID, item.Provenance)
	}
	for _, item := range details.Endpoints {
		add("endpoint:"+item.ID, item.Provenance)
	}
	return out
}

func allRecordProvenance(details snapshot.DiscoveryDetails) [][]snapshot.Provenance {
	out := make([][]snapshot.Provenance, 0, len(details.Services)+len(details.Edges)+len(details.Endpoints))
	for _, item := range details.Services {
		out = append(out, item.Provenance)
	}
	for _, item := range details.Edges {
		out = append(out, item.Provenance)
	}
	for _, item := range details.Endpoints {
		out = append(out, item.Provenance)
	}
	return out
}

func trimMissing(items []string) []string {
	const limit = 20
	if len(items) <= limit {
		return items
	}
	return append([]string(nil), items[:limit]...)
}

func round(value float64) float64 {
	return math.Round(value*1000) / 1000
}
