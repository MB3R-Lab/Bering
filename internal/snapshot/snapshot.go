package snapshot

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/MB3R-Lab/Bering/internal/model"
)

type Envelope struct {
	SnapshotID      string                `json:"snapshot_id"`
	TopologyVersion string                `json:"topology_version"`
	WindowStart     string                `json:"window_start"`
	WindowEnd       string                `json:"window_end"`
	Ingest          IngestSummary         `json:"ingest"`
	Counts          Counts                `json:"counts"`
	Coverage        CoverageSummary       `json:"coverage"`
	Sources         []SourceSummary       `json:"sources"`
	Diff            DiffSummary           `json:"diff"`
	Discovery       DiscoveryDetails      `json:"discovery"`
	Model           model.ResilienceModel `json:"model"`
	Metadata        Metadata              `json:"metadata"`
}

type IngestSummary struct {
	Spans        int `json:"spans"`
	Traces       int `json:"traces"`
	DroppedSpans int `json:"dropped_spans"`
	LateSpans    int `json:"late_spans"`
}

type Counts struct {
	Services  int `json:"services"`
	Edges     int `json:"edges"`
	Endpoints int `json:"endpoints"`
}

type CoverageSummary struct {
	Confidence         float64 `json:"confidence"`
	ServiceSupportMin  int     `json:"service_support_min"`
	EdgeSupportMin     int     `json:"edge_support_min"`
	EndpointSupportMin int     `json:"endpoint_support_min"`
}

type SourceSummary struct {
	Type         string `json:"type"`
	Connector    string `json:"connector,omitempty"`
	Ref          string `json:"ref,omitempty"`
	Observations int    `json:"observations,omitempty"`
}

type DiffSummary struct {
	AddedServices    int `json:"added_services"`
	RemovedServices  int `json:"removed_services"`
	ChangedServices  int `json:"changed_services"`
	AddedEdges       int `json:"added_edges"`
	RemovedEdges     int `json:"removed_edges"`
	ChangedEdges     int `json:"changed_edges"`
	AddedEndpoints   int `json:"added_endpoints"`
	RemovedEndpoints int `json:"removed_endpoints"`
	ChangedEndpoints int `json:"changed_endpoints"`
}

type DiscoveryDetails struct {
	Services  []ServiceRecord      `json:"services"`
	Edges     []EdgeRecord         `json:"edges"`
	Endpoints []EndpointRecord     `json:"endpoints"`
	Overlays  []OverlayApplication `json:"overlays,omitempty"`
}

type OverlayApplication struct {
	Name       string `json:"name"`
	Ref        string `json:"ref,omitempty"`
	Precedence int    `json:"precedence"`
}

type Provenance struct {
	Type       string `json:"type"`
	Connector  string `json:"connector,omitempty"`
	Name       string `json:"name,omitempty"`
	Ref        string `json:"ref,omitempty"`
	Precedence int    `json:"precedence,omitempty"`
}

type SupportSummary struct {
	Observations int      `json:"observations"`
	TraceCount   int      `json:"trace_count"`
	Evidence     []string `json:"evidence,omitempty"`
}

type ServiceRecord struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Replicas   int             `json:"replicas"`
	Support    SupportSummary  `json:"support"`
	FirstSeen  string          `json:"first_seen,omitempty"`
	LastSeen   string          `json:"last_seen,omitempty"`
	Provenance []Provenance    `json:"provenance,omitempty"`
	Metadata   ServiceMetadata `json:"metadata,omitempty"`
}

type EdgeRecord struct {
	ID         string         `json:"id"`
	From       string         `json:"from"`
	To         string         `json:"to"`
	Kind       model.EdgeKind `json:"kind"`
	Blocking   bool           `json:"blocking"`
	Support    SupportSummary `json:"support"`
	FirstSeen  string         `json:"first_seen,omitempty"`
	LastSeen   string         `json:"last_seen,omitempty"`
	Provenance []Provenance   `json:"provenance,omitempty"`
	Metadata   EdgeMetadata   `json:"metadata,omitempty"`
}

type EndpointRecord struct {
	ID           string           `json:"id"`
	EntryService string           `json:"entry_service"`
	Method       string           `json:"method,omitempty"`
	Path         string           `json:"path,omitempty"`
	Support      SupportSummary   `json:"support"`
	FirstSeen    string           `json:"first_seen,omitempty"`
	LastSeen     string           `json:"last_seen,omitempty"`
	Provenance   []Provenance     `json:"provenance,omitempty"`
	Metadata     EndpointMetadata `json:"metadata,omitempty"`
}

type CommonMetadata struct {
	Labels  map[string]string `json:"labels,omitempty"`
	Tags    []string          `json:"tags,omitempty"`
	SLORefs []string          `json:"slo_refs,omitempty"`
}

type ServiceMetadata struct {
	CommonMetadata
	FailureEligible  *bool             `json:"failure_eligible,omitempty"`
	ReplicasOverride *int              `json:"replicas_override,omitempty"`
	Attributes       map[string]string `json:"attributes,omitempty"`
}

type EdgeMetadata struct {
	CommonMetadata
	Weight     *float64          `json:"weight,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

type EndpointMetadata struct {
	CommonMetadata
	Weight       *float64          `json:"weight,omitempty"`
	PredicateRef string            `json:"predicate_ref,omitempty"`
	Attributes   map[string]string `json:"attributes,omitempty"`
}

type Metadata struct {
	SourceType string          `json:"source_type"`
	SourceRef  string          `json:"source_ref"`
	EmittedAt  string          `json:"emitted_at"`
	Confidence float64         `json:"confidence"`
	Schema     model.SchemaRef `json:"schema"`
}

func (e *Envelope) SortDeterministic() {
	sort.Slice(e.Sources, func(i, j int) bool {
		left, right := e.Sources[i], e.Sources[j]
		if left.Type != right.Type {
			return left.Type < right.Type
		}
		if left.Connector != right.Connector {
			return left.Connector < right.Connector
		}
		return left.Ref < right.Ref
	})
	sort.Slice(e.Discovery.Services, func(i, j int) bool { return e.Discovery.Services[i].ID < e.Discovery.Services[j].ID })
	sort.Slice(e.Discovery.Edges, func(i, j int) bool { return e.Discovery.Edges[i].ID < e.Discovery.Edges[j].ID })
	sort.Slice(e.Discovery.Endpoints, func(i, j int) bool { return e.Discovery.Endpoints[i].ID < e.Discovery.Endpoints[j].ID })
	sort.Slice(e.Discovery.Overlays, func(i, j int) bool {
		if e.Discovery.Overlays[i].Precedence != e.Discovery.Overlays[j].Precedence {
			return e.Discovery.Overlays[i].Precedence < e.Discovery.Overlays[j].Precedence
		}
		return e.Discovery.Overlays[i].Name < e.Discovery.Overlays[j].Name
	})
	e.Model.SortDeterministic()
}

func (e Envelope) ValidateSemantic() error {
	if strings.TrimSpace(e.SnapshotID) == "" {
		return errors.New("snapshot_id cannot be empty")
	}
	if strings.TrimSpace(e.TopologyVersion) == "" {
		return errors.New("topology_version cannot be empty")
	}
	if _, err := time.Parse(time.RFC3339, e.WindowStart); err != nil {
		return fmt.Errorf("window_start must be RFC3339: %w", err)
	}
	if _, err := time.Parse(time.RFC3339, e.WindowEnd); err != nil {
		return fmt.Errorf("window_end must be RFC3339: %w", err)
	}
	if err := e.Model.ValidateSemantic(); err != nil {
		return fmt.Errorf("model validation failed: %w", err)
	}
	if strings.TrimSpace(e.Metadata.SourceType) == "" {
		return errors.New("metadata.source_type cannot be empty")
	}
	if strings.TrimSpace(e.Metadata.SourceRef) == "" {
		return errors.New("metadata.source_ref cannot be empty")
	}
	if _, err := time.Parse(time.RFC3339, e.Metadata.EmittedAt); err != nil {
		return fmt.Errorf("metadata.emitted_at must be RFC3339: %w", err)
	}
	if e.Metadata.Confidence < 0 || e.Metadata.Confidence > 1 {
		return errors.New("metadata.confidence must be in [0,1]")
	}
	if e.Ingest.Spans < 0 || e.Ingest.Traces < 0 || e.Ingest.DroppedSpans < 0 || e.Ingest.LateSpans < 0 {
		return errors.New("ingest counts must be >= 0")
	}
	if e.Counts.Services != len(e.Model.Services) {
		return fmt.Errorf("counts.services mismatch: got=%d want=%d", e.Counts.Services, len(e.Model.Services))
	}
	if e.Counts.Edges != len(e.Model.Edges) {
		return fmt.Errorf("counts.edges mismatch: got=%d want=%d", e.Counts.Edges, len(e.Model.Edges))
	}
	if e.Counts.Endpoints != len(e.Model.Endpoints) {
		return fmt.Errorf("counts.endpoints mismatch: got=%d want=%d", e.Counts.Endpoints, len(e.Model.Endpoints))
	}
	if len(e.Discovery.Services) != len(e.Model.Services) {
		return fmt.Errorf("discovery.services mismatch: got=%d want=%d", len(e.Discovery.Services), len(e.Model.Services))
	}
	if len(e.Discovery.Edges) != len(e.Model.Edges) {
		return fmt.Errorf("discovery.edges mismatch: got=%d want=%d", len(e.Discovery.Edges), len(e.Model.Edges))
	}
	if len(e.Discovery.Endpoints) != len(e.Model.Endpoints) {
		return fmt.Errorf("discovery.endpoints mismatch: got=%d want=%d", len(e.Discovery.Endpoints), len(e.Model.Endpoints))
	}
	return nil
}

func TopologyDigest(mdl model.ResilienceModel) (string, error) {
	raw, err := model.MarshalCanonical(mdl)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func BuildSnapshotID(windowStart, windowEnd, topologyVersion string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(windowStart) + "|" + strings.TrimSpace(windowEnd) + "|" + strings.TrimSpace(topologyVersion)))
	return "snap-" + hex.EncodeToString(sum[:12])
}

func ComputeDiff(previous *Envelope, current Envelope) DiffSummary {
	if previous == nil {
		return DiffSummary{
			AddedServices:  len(current.Discovery.Services),
			AddedEdges:     len(current.Discovery.Edges),
			AddedEndpoints: len(current.Discovery.Endpoints),
		}
	}
	return DiffSummary{
		AddedServices:    diffAdded(serviceMap(previous.Discovery.Services), serviceMap(current.Discovery.Services)),
		RemovedServices:  diffAdded(serviceMap(current.Discovery.Services), serviceMap(previous.Discovery.Services)),
		ChangedServices:  diffChanged(serviceMap(previous.Discovery.Services), serviceMap(current.Discovery.Services)),
		AddedEdges:       diffAdded(edgeMap(previous.Discovery.Edges), edgeMap(current.Discovery.Edges)),
		RemovedEdges:     diffAdded(edgeMap(current.Discovery.Edges), edgeMap(previous.Discovery.Edges)),
		ChangedEdges:     diffChanged(edgeMap(previous.Discovery.Edges), edgeMap(current.Discovery.Edges)),
		AddedEndpoints:   diffAdded(endpointMap(previous.Discovery.Endpoints), endpointMap(current.Discovery.Endpoints)),
		RemovedEndpoints: diffAdded(endpointMap(current.Discovery.Endpoints), endpointMap(previous.Discovery.Endpoints)),
		ChangedEndpoints: diffChanged(endpointMap(previous.Discovery.Endpoints), endpointMap(current.Discovery.Endpoints)),
	}
}

func serviceMap(items []ServiceRecord) map[string]ServiceRecord {
	out := make(map[string]ServiceRecord, len(items))
	for _, item := range items {
		out[item.ID] = item
	}
	return out
}

func edgeMap(items []EdgeRecord) map[string]EdgeRecord {
	out := make(map[string]EdgeRecord, len(items))
	for _, item := range items {
		out[item.ID] = item
	}
	return out
}

func endpointMap(items []EndpointRecord) map[string]EndpointRecord {
	out := make(map[string]EndpointRecord, len(items))
	for _, item := range items {
		out[item.ID] = item
	}
	return out
}

func diffAdded[T any](left, right map[string]T) int {
	count := 0
	for key := range right {
		if _, ok := left[key]; !ok {
			count++
		}
	}
	return count
}

func diffChanged[T any](left, right map[string]T) int {
	count := 0
	for key, leftValue := range left {
		rightValue, ok := right[key]
		if !ok {
			continue
		}
		if !reflect.DeepEqual(leftValue, rightValue) {
			count++
		}
	}
	return count
}
