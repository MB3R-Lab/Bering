package discovery

import (
	"fmt"
	"strings"
	"time"

	"github.com/MB3R-Lab/Bering/internal/connectors/topology"
	"github.com/MB3R-Lab/Bering/internal/model"
	"github.com/MB3R-Lab/Bering/internal/overlay"
	"github.com/MB3R-Lab/Bering/internal/schema"
	"github.com/MB3R-Lab/Bering/internal/snapshot"
)

func DiscoverTopology(doc topology.Document, opts Options) (Result, error) {
	discoveredAt := strings.TrimSpace(opts.DiscoveredAt)
	if discoveredAt == "" {
		discoveredAt = time.Now().UTC().Format(time.RFC3339)
	}
	if _, err := time.Parse(time.RFC3339, discoveredAt); err != nil {
		return Result{}, fmt.Errorf("discovered_at must be RFC3339: %w", err)
	}

	sourceRef := strings.TrimSpace(opts.SourceRef)
	if sourceRef == "" {
		sourceRef = "bering://discover"
	}
	sourceType := strings.TrimSpace(doc.Source.Type)
	if sourceType == "" {
		sourceType = topology.SourceTypeTopologyAPI
	}
	provenanceRef := strings.TrimSpace(doc.Source.Ref)
	if provenanceRef == "" {
		provenanceRef = sourceRef
	}

	allOverlays := make([]overlay.File, 0, len(opts.Overlays)+1)
	if len(opts.ReplicasOverride) > 0 {
		allOverlays = append(allOverlays, overlay.FromReplicas(opts.ReplicasOverride, "legacy:replicas"))
	}
	allOverlays = append(allOverlays, opts.Overlays...)

	serviceReplicas := make(map[string]int, len(doc.Services))
	for _, item := range doc.Services {
		serviceReplicas[item.ID] = valueOrDefault(item.Replicas, 1)
	}
	if err := applyReplicaOverlays(serviceReplicas, allOverlays); err != nil {
		return Result{}, err
	}

	provenance := snapshot.Provenance{Type: sourceType, Connector: topology.ConnectorName, Ref: provenanceRef}
	services := make([]model.Service, 0, len(doc.Services))
	serviceRecords := make([]snapshot.ServiceRecord, 0, len(doc.Services))
	for _, item := range doc.Services {
		replicas := serviceReplicas[item.ID]
		services = append(services, model.Service{
			ID:       item.ID,
			Name:     item.Name,
			Replicas: replicas,
			Metadata: buildServiceMetadataFromTopology(item),
		})
		serviceRecords = append(serviceRecords, snapshot.ServiceRecord{
			ID:         item.ID,
			Name:       item.Name,
			Replicas:   replicas,
			Support:    topologySupport(item.Support),
			FirstSeen:  strings.TrimSpace(item.FirstSeen),
			LastSeen:   strings.TrimSpace(item.LastSeen),
			Provenance: []snapshot.Provenance{provenance},
			Metadata:   buildSnapshotServiceMetadataFromTopology(item),
		})
	}

	edges := make([]model.Edge, 0, len(doc.Edges))
	edgeRecords := make([]snapshot.EdgeRecord, 0, len(doc.Edges))
	for _, item := range doc.Edges {
		kind := model.EdgeKind(strings.TrimSpace(item.Kind))
		blocking := valueOrDefault(item.Blocking, kind == model.EdgeKindSync)
		edges = append(edges, model.Edge{
			ID:          item.ID,
			From:        item.From,
			To:          item.To,
			Kind:        kind,
			Blocking:    blocking,
			Metadata:    buildEdgeMetadataFromTopology(item),
			Resilience:  cloneResiliencePolicy(item.Resilience),
			Observed:    cloneObservedEdge(item.Observed),
			PolicyScope: clonePolicyScope(item.PolicyScope),
		})
		edgeRecords = append(edgeRecords, snapshot.EdgeRecord{
			ID:          item.ID,
			From:        item.From,
			To:          item.To,
			Kind:        kind,
			Blocking:    blocking,
			Support:     topologySupport(item.Support),
			FirstSeen:   strings.TrimSpace(item.FirstSeen),
			LastSeen:    strings.TrimSpace(item.LastSeen),
			Provenance:  []snapshot.Provenance{provenance},
			Metadata:    buildSnapshotEdgeMetadataFromTopology(item),
			Resilience:  cloneResiliencePolicy(item.Resilience),
			Observed:    cloneObservedEdge(item.Observed),
			PolicyScope: clonePolicyScope(item.PolicyScope),
		})
	}

	endpoints := make([]model.Endpoint, 0, len(doc.Endpoints))
	endpointRecords := make([]snapshot.EndpointRecord, 0, len(doc.Endpoints))
	for _, item := range doc.Endpoints {
		predicateRef := strings.TrimSpace(item.PredicateRef)
		if predicateRef == "" {
			predicateRef = item.ID
		}
		endpoints = append(endpoints, model.Endpoint{
			ID:                  item.ID,
			EntryService:        item.EntryService,
			SuccessPredicateRef: predicateRef,
			Method:              strings.TrimSpace(item.Method),
			Path:                strings.TrimSpace(item.Path),
			Metadata:            buildEndpointMetadataFromTopology(item),
		})
		endpointRecords = append(endpointRecords, snapshot.EndpointRecord{
			ID:           item.ID,
			EntryService: item.EntryService,
			Method:       strings.TrimSpace(item.Method),
			Path:         strings.TrimSpace(item.Path),
			Support:      topologySupport(item.Support),
			FirstSeen:    strings.TrimSpace(item.FirstSeen),
			LastSeen:     strings.TrimSpace(item.LastSeen),
			Provenance:   []snapshot.Provenance{provenance},
			Metadata:     buildSnapshotEndpointMetadataFromTopology(item, predicateRef),
		})
	}

	confidence := 1.0
	mdl := model.ResilienceModel{
		Services:  services,
		Edges:     edges,
		Endpoints: endpoints,
		Metadata: model.Metadata{
			SourceType:   SourceTypeBering,
			SourceRef:    sourceRef,
			DiscoveredAt: discoveredAt,
			Confidence:   confidence,
			Schema: model.SchemaRef{
				Name:    schema.ExpectedSchemaName,
				Version: schema.ExpectedSchemaVersion,
				URI:     schema.ExpectedSchemaURI,
				Digest:  schema.ExpectedSchemaDigest,
			},
		},
	}
	if err := mdl.ValidateSemantic(); err != nil {
		return Result{}, err
	}
	mdl.SortDeterministic()

	applications, err := applyOverlays(&mdl, serviceRecords, edgeRecords, endpointRecords, allOverlays)
	if err != nil {
		return Result{}, err
	}
	mdl.SortDeterministic()
	serviceRecords = rebuildServiceRecords(serviceRecords, mdl.Services)

	coverage := snapshot.CoverageSummary{
		Confidence:         confidence,
		ServiceSupportMin:  minSupportServices(serviceRecords),
		EdgeSupportMin:     minSupportEdges(edgeRecords),
		EndpointSupportMin: minSupportEndpoints(endpointRecords),
	}

	result := Result{
		Model: mdl,
		Discovery: snapshot.DiscoveryDetails{
			Services:  serviceRecords,
			Edges:     edgeRecords,
			Endpoints: endpointRecords,
			Overlays:  applications,
		},
		Sources: []snapshot.SourceSummary{{
			Type:         sourceType,
			Connector:    topology.ConnectorName,
			Ref:          provenanceRef,
			Observations: len(doc.Services) + len(doc.Edges) + len(doc.Endpoints),
		}},
		Coverage: coverage,
	}
	return result, nil
}

func topologySupport(item topology.Support) snapshot.SupportSummary {
	return snapshot.SupportSummary{
		Observations: item.Observations,
		TraceCount:   0,
		Evidence:     append([]string(nil), item.Evidence...),
	}
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func cloneCommonMetadata(values topology.CommonMetadata) model.CommonMetadata {
	return model.CommonMetadata{
		Labels:  cloneStringMap(values.Labels),
		Tags:    cloneStringSlice(values.Tags),
		SLORefs: cloneStringSlice(values.SLORefs),
	}
}

func clonePlacements(values []model.Placement) []model.Placement {
	if len(values) == 0 {
		return nil
	}
	out := make([]model.Placement, len(values))
	for i, item := range values {
		out[i] = model.Placement{
			Replicas: item.Replicas,
			Labels:   cloneStringMap(item.Labels),
		}
	}
	return out
}

func buildServiceMetadataFromTopology(item topology.Service) *model.ServiceMetadata {
	meta := &model.ServiceMetadata{
		CommonMetadata:     cloneCommonMetadata(item.CommonMetadata),
		FailureEligible:    item.FailureEligible,
		Placements:         clonePlacements(item.Placements),
		SharedResourceRefs: cloneStringSlice(item.SharedResourceRefs),
	}
	meta.Normalize()
	if meta.IsZero() {
		return nil
	}
	return meta
}

func buildSnapshotServiceMetadataFromTopology(item topology.Service) *snapshot.ServiceMetadata {
	meta := &snapshot.ServiceMetadata{
		ServiceMetadata: model.ServiceMetadata{
			CommonMetadata:     cloneCommonMetadata(item.CommonMetadata),
			FailureEligible:    item.FailureEligible,
			Placements:         clonePlacements(item.Placements),
			SharedResourceRefs: cloneStringSlice(item.SharedResourceRefs),
		},
		Attributes: cloneStringMap(item.Attributes),
	}
	meta.Normalize()
	if meta.IsZero() {
		return nil
	}
	return meta
}

func buildEdgeMetadataFromTopology(item topology.Edge) *model.EdgeMetadata {
	meta := &model.EdgeMetadata{
		CommonMetadata: cloneCommonMetadata(item.CommonMetadata),
		Weight:         item.Weight,
	}
	meta.Normalize()
	if meta.IsZero() {
		return nil
	}
	return meta
}

func buildSnapshotEdgeMetadataFromTopology(item topology.Edge) *snapshot.EdgeMetadata {
	meta := &snapshot.EdgeMetadata{
		EdgeMetadata: model.EdgeMetadata{
			CommonMetadata: cloneCommonMetadata(item.CommonMetadata),
			Weight:         item.Weight,
		},
		Attributes: cloneStringMap(item.Attributes),
	}
	meta.Normalize()
	if meta.IsZero() {
		return nil
	}
	return meta
}

func buildEndpointMetadataFromTopology(item topology.Endpoint) *model.EndpointMetadata {
	meta := &model.EndpointMetadata{
		CommonMetadata: cloneCommonMetadata(item.CommonMetadata),
		Weight:         item.Weight,
	}
	meta.Normalize()
	if meta.IsZero() {
		return nil
	}
	return meta
}

func buildSnapshotEndpointMetadataFromTopology(item topology.Endpoint, predicateRef string) *snapshot.EndpointMetadata {
	meta := &snapshot.EndpointMetadata{
		EndpointMetadata: model.EndpointMetadata{
			CommonMetadata: cloneCommonMetadata(item.CommonMetadata),
			Weight:         item.Weight,
		},
		PredicateRef: predicateRef,
		Attributes:   cloneStringMap(item.Attributes),
	}
	meta.Normalize()
	if meta.IsZero() {
		return nil
	}
	return meta
}

func cloneResiliencePolicy(values *model.ResiliencePolicy) *model.ResiliencePolicy {
	if values == nil {
		return nil
	}
	out := &model.ResiliencePolicy{
		RequestTimeoutMS: cloneIntPointer(values.RequestTimeoutMS),
		PerTryTimeoutMS:  cloneIntPointer(values.PerTryTimeoutMS),
		Retry:            cloneRetryPolicy(values.Retry),
		CircuitBreaker:   cloneCircuitBreakerPolicy(values.CircuitBreaker),
	}
	out.Normalize()
	if out.IsZero() {
		return nil
	}
	return out
}

func cloneRetryPolicy(values *model.RetryPolicy) *model.RetryPolicy {
	if values == nil {
		return nil
	}
	out := &model.RetryPolicy{
		MaxAttempts: cloneIntPointer(values.MaxAttempts),
		BudgetCap:   cloneFloatPointer(values.BudgetCap),
		RetryOn:     cloneStringSlice(values.RetryOn),
		Backoff:     cloneBackoffPolicy(values.Backoff),
	}
	out.Normalize()
	if out.IsZero() {
		return nil
	}
	return out
}

func cloneBackoffPolicy(values *model.BackoffPolicy) *model.BackoffPolicy {
	if values == nil {
		return nil
	}
	out := &model.BackoffPolicy{
		InitialMS:  cloneIntPointer(values.InitialMS),
		MaxMS:      cloneIntPointer(values.MaxMS),
		Multiplier: cloneFloatPointer(values.Multiplier),
		Jitter:     values.Jitter,
	}
	out.Normalize()
	if out.IsZero() {
		return nil
	}
	return out
}

func cloneCircuitBreakerPolicy(values *model.CircuitBreakerPolicy) *model.CircuitBreakerPolicy {
	if values == nil {
		return nil
	}
	out := &model.CircuitBreakerPolicy{
		Enabled:            cloneBoolPointer(values.Enabled),
		MaxPendingRequests: cloneIntPointer(values.MaxPendingRequests),
		MaxRequests:        cloneIntPointer(values.MaxRequests),
		MaxConnections:     cloneIntPointer(values.MaxConnections),
		Consecutive5xx:     cloneIntPointer(values.Consecutive5xx),
		IntervalMS:         cloneIntPointer(values.IntervalMS),
		BaseEjectionTimeMS: cloneIntPointer(values.BaseEjectionTimeMS),
	}
	out.Normalize()
	if out.IsZero() {
		return nil
	}
	return out
}

func cloneObservedEdge(values *model.ObservedEdge) *model.ObservedEdge {
	if values == nil {
		return nil
	}
	out := &model.ObservedEdge{
		LatencyMS: cloneLatencySummary(values.LatencyMS),
		ErrorRate: cloneFloatPointer(values.ErrorRate),
	}
	out.Normalize()
	if out.IsZero() {
		return nil
	}
	return out
}

func cloneLatencySummary(values *model.LatencySummary) *model.LatencySummary {
	if values == nil {
		return nil
	}
	out := &model.LatencySummary{
		P50: cloneFloatPointer(values.P50),
		P90: cloneFloatPointer(values.P90),
		P95: cloneFloatPointer(values.P95),
		P99: cloneFloatPointer(values.P99),
	}
	out.Normalize()
	if out.IsZero() {
		return nil
	}
	return out
}

func clonePolicyScope(values *model.PolicyScope) *model.PolicyScope {
	if values == nil {
		return nil
	}
	out := &model.PolicyScope{
		SourceEndpointID: values.SourceEndpointID,
		SourceRoute:      values.SourceRoute,
		Method:           values.Method,
		Operation:        values.Operation,
	}
	out.Normalize()
	if out.IsZero() {
		return nil
	}
	return out
}

func cloneIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

func cloneFloatPointer(value *float64) *float64 {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

func cloneBoolPointer(value *bool) *bool {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

func valueOrDefault[T any](value *T, fallback T) T {
	if value == nil {
		return fallback
	}
	return *value
}
