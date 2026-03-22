package discovery

import (
	"fmt"
	"math"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/MB3R-Lab/Bering/internal/connectors/traces"
	"github.com/MB3R-Lab/Bering/internal/model"
	"github.com/MB3R-Lab/Bering/internal/overlay"
	"github.com/MB3R-Lab/Bering/internal/schema"
	"github.com/MB3R-Lab/Bering/internal/snapshot"
)

const SourceTypeBering = "bering"

type Options struct {
	SourceRef        string
	DiscoveredAt     string
	ReplicasOverride map[string]int
	Overlays         []overlay.File
	Sources          []snapshot.SourceSummary
	RuntimeMode      bool
}

type Result struct {
	Model      model.ResilienceModel
	Discovery  snapshot.DiscoveryDetails
	Sources    []snapshot.SourceSummary
	Coverage   snapshot.CoverageSummary
	SpanCount  int
	TraceCount int
}

func Build(spans []traces.Span, opts Options) (model.ResilienceModel, error) {
	result, err := Discover(spans, opts)
	if err != nil {
		return model.ResilienceModel{}, err
	}
	return result.Model, nil
}

func Discover(spans []traces.Span, opts Options) (Result, error) {
	if len(spans) == 0 {
		return Result{}, fmt.Errorf("no spans provided for discovery")
	}

	discoveredAt := opts.DiscoveredAt
	if strings.TrimSpace(discoveredAt) == "" {
		discoveredAt = time.Now().UTC().Format(time.RFC3339)
	}
	if _, err := time.Parse(time.RFC3339, discoveredAt); err != nil {
		return Result{}, fmt.Errorf("discovered_at must be RFC3339: %w", err)
	}

	sourceRef := strings.TrimSpace(opts.SourceRef)
	if sourceRef == "" {
		sourceRef = "bering://discover"
	}

	allOverlays := make([]overlay.File, 0, len(opts.Overlays)+1)
	if len(opts.ReplicasOverride) > 0 {
		allOverlays = append(allOverlays, overlay.FromReplicas(opts.ReplicasOverride, "legacy:replicas"))
	}
	allOverlays = append(allOverlays, opts.Overlays...)

	spanByKey := map[string]traces.Span{}
	serviceSet := map[string]int{}
	serviceStats := map[string]*serviceAccumulator{}
	traceSet := map[string]struct{}{}

	for _, span := range spans {
		service := strings.TrimSpace(span.Service)
		if service == "" {
			continue
		}
		if _, exists := serviceSet[service]; !exists {
			serviceSet[service] = 1
		}
		if span.TraceID != "" && span.SpanID != "" {
			spanByKey[traceSpanKey(span.TraceID, span.SpanID)] = span
		}
		if span.TraceID != "" {
			traceSet[span.TraceID] = struct{}{}
		}
		serviceStats[service] = updateServiceAccumulator(serviceStats[service], span, opts.RuntimeMode)
	}

	if len(serviceSet) == 0 {
		return Result{}, fmt.Errorf("discovery produced no services")
	}

	if err := applyReplicaOverlays(serviceSet, allOverlays); err != nil {
		return Result{}, err
	}

	edgeStats := map[string]*edgeAccumulator{}
	endpointStats := map[string]*endpointAccumulator{}
	linkedCrossServiceEdges := 0

	for _, span := range spans {
		if strings.TrimSpace(span.Service) == "" {
			continue
		}

		parent, hasParent := parentSpan(span, spanByKey)
		if hasParent && strings.TrimSpace(parent.Service) != "" && parent.Service != span.Service {
			kind, evidence := edgeKind(parent, span)
			blocking := kind == model.EdgeKindSync
			key := edgeKey(parent.Service, span.Service, kind, blocking)
			edgeStats[key] = updateEdgeAccumulator(edgeStats[key], key, parent, span, kind, blocking, evidence, opts.RuntimeMode)
			linkedCrossServiceEdges++
		}

		if endpoint, ok := inferEndpoint(span, parent, hasParent); ok {
			endpointStats[endpoint.ID] = updateEndpointAccumulator(endpointStats[endpoint.ID], endpoint, span, opts.RuntimeMode)
		}
	}

	services := make([]model.Service, 0, len(serviceSet))
	serviceRecords := make([]snapshot.ServiceRecord, 0, len(serviceStats))
	for id, replicas := range serviceSet {
		services = append(services, model.Service{ID: id, Name: id, Replicas: replicas})
		stat := serviceStats[id]
		serviceRecords = append(serviceRecords, snapshot.ServiceRecord{
			ID:         id,
			Name:       id,
			Replicas:   replicas,
			Support:    stat.supportSummary(),
			FirstSeen:  formatOptionalTime(stat.firstSeen, opts.RuntimeMode),
			LastSeen:   formatOptionalTime(stat.lastSeen, opts.RuntimeMode),
			Provenance: []snapshot.Provenance{{Type: "traces", Connector: inferConnector(opts.Sources), Ref: sourceRef}},
		})
	}
	sort.Slice(services, func(i, j int) bool { return services[i].ID < services[j].ID })
	sort.Slice(serviceRecords, func(i, j int) bool { return serviceRecords[i].ID < serviceRecords[j].ID })

	edges := make([]model.Edge, 0, len(edgeStats))
	edgeRecords := make([]snapshot.EdgeRecord, 0, len(edgeStats))
	for _, stat := range edgeStats {
		stat.edge.Observed = stat.observedSummary()
		edges = append(edges, stat.edge)
		edgeRecords = append(edgeRecords, snapshot.EdgeRecord{
			ID:         stat.id,
			From:       stat.edge.From,
			To:         stat.edge.To,
			Kind:       stat.edge.Kind,
			Blocking:   stat.edge.Blocking,
			Support:    stat.supportSummary(),
			FirstSeen:  formatOptionalTime(stat.firstSeen, opts.RuntimeMode),
			LastSeen:   formatOptionalTime(stat.lastSeen, opts.RuntimeMode),
			Provenance: []snapshot.Provenance{{Type: "traces", Connector: inferConnector(opts.Sources), Ref: sourceRef}},
			Observed:   stat.observedSummary(),
		})
	}
	sort.Slice(edges, func(i, j int) bool {
		left, right := edges[i], edges[j]
		if left.From != right.From {
			return left.From < right.From
		}
		if left.To != right.To {
			return left.To < right.To
		}
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		return !left.Blocking && right.Blocking
	})
	sort.Slice(edgeRecords, func(i, j int) bool { return edgeRecords[i].ID < edgeRecords[j].ID })

	endpoints := make([]model.Endpoint, 0, len(endpointStats))
	endpointRecords := make([]snapshot.EndpointRecord, 0, len(endpointStats))
	for _, stat := range endpointStats {
		endpoints = append(endpoints, stat.endpoint)
		endpointRecords = append(endpointRecords, snapshot.EndpointRecord{
			ID:           stat.endpoint.ID,
			EntryService: stat.endpoint.EntryService,
			Method:       stat.endpoint.Method,
			Path:         stat.endpoint.Path,
			Support:      stat.supportSummary(),
			FirstSeen:    formatOptionalTime(stat.firstSeen, opts.RuntimeMode),
			LastSeen:     formatOptionalTime(stat.lastSeen, opts.RuntimeMode),
			Provenance:   []snapshot.Provenance{{Type: "traces", Connector: inferConnector(opts.Sources), Ref: sourceRef}},
		})
	}
	sort.Slice(endpoints, func(i, j int) bool { return endpoints[i].ID < endpoints[j].ID })
	sort.Slice(endpointRecords, func(i, j int) bool { return endpointRecords[i].ID < endpointRecords[j].ID })

	confidence := calculateConfidence(len(spans), len(services), len(edges), len(endpoints), linkedCrossServiceEdges)
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
		Sources:    defaultSources(opts.Sources, sourceRef, len(spans)),
		Coverage:   coverage,
		SpanCount:  len(spans),
		TraceCount: len(traceSet),
	}
	return result, nil
}

func BuildSourceRef(input string) string {
	clean := strings.TrimSpace(input)
	if clean == "" {
		return "bering://discover"
	}
	clean = filepath.ToSlash(filepath.Clean(clean))
	return "bering://discover?input=" + url.QueryEscape(clean)
}

func BuildServeSourceRef(listenAddr string) string {
	if strings.TrimSpace(listenAddr) == "" {
		return "bering://serve"
	}
	return "bering://serve?listen=" + url.QueryEscape(strings.TrimSpace(listenAddr))
}

func parentSpan(child traces.Span, index map[string]traces.Span) (traces.Span, bool) {
	if strings.TrimSpace(child.TraceID) == "" || strings.TrimSpace(child.ParentSpanID) == "" {
		return traces.Span{}, false
	}
	parent, ok := index[traceSpanKey(child.TraceID, child.ParentSpanID)]
	return parent, ok
}

func traceSpanKey(traceID, spanID string) string {
	return strings.TrimSpace(traceID) + "|" + strings.TrimSpace(spanID)
}

func edgeKind(parent, child traces.Span) (model.EdgeKind, []string) {
	evidence := []string{"default_sync"}
	if isAsyncSpan(parent) {
		evidence = append([]string{"parent_async_signal"}, asyncEvidence(parent)...)
		return model.EdgeKindAsync, dedupeStrings(evidence)
	}
	if isAsyncSpan(child) {
		evidence = append([]string{"child_async_signal"}, asyncEvidence(child)...)
		return model.EdgeKindAsync, dedupeStrings(evidence)
	}
	return model.EdgeKindSync, evidence
}

func isAsyncSpan(span traces.Span) bool {
	kind := strings.ToLower(strings.TrimSpace(span.Kind))
	if kind == "producer" || kind == "consumer" {
		return true
	}
	if attrString(span.Attributes, "messaging.system") != "" {
		return true
	}
	if attrString(span.Attributes, "messaging.destination") != "" {
		return true
	}
	if attrString(span.Attributes, "messaging.operation") != "" {
		return true
	}
	return false
}

func asyncEvidence(span traces.Span) []string {
	out := []string{}
	kind := strings.ToLower(strings.TrimSpace(span.Kind))
	if kind == "producer" || kind == "consumer" {
		out = append(out, "span.kind="+kind)
	}
	for _, key := range []string{"messaging.system", "messaging.destination", "messaging.operation"} {
		if attrString(span.Attributes, key) != "" {
			out = append(out, key)
		}
	}
	return out
}

func inferEndpoint(span, parent traces.Span, hasParent bool) (model.Endpoint, bool) {
	method, path, ok := inferEndpointDetails(span, parent, hasParent)
	if !ok {
		return model.Endpoint{}, false
	}
	id := fmt.Sprintf("%s:%s %s", span.Service, method, path)
	return model.Endpoint{ID: id, EntryService: span.Service, SuccessPredicateRef: id, Method: method, Path: path}, true
}

func inferEndpointDetails(span, parent traces.Span, hasParent bool) (string, string, bool) {
	if strings.TrimSpace(span.Service) == "" {
		return "", "", false
	}

	method := strings.ToUpper(strings.TrimSpace(firstAttr(
		span.Attributes,
		"http.request.method",
		"http.method",
	)))
	if method == "" {
		method, _ = parseMethodAndPathFromSpanName(span.Name)
	}
	if method == "" {
		return "", "", false
	}

	path := strings.TrimSpace(firstAttr(
		span.Attributes,
		"http.route",
		"url.path",
		"http.target",
	))
	if path == "" {
		_, path = parseMethodAndPathFromSpanName(span.Name)
	}
	path = normalizePath(path)
	if path == "" {
		return "", "", false
	}

	isServer := strings.EqualFold(span.Kind, "server")
	if !isServer {
		if hasParent && parent.Service == span.Service {
			return "", "", false
		}
	}

	return method, path, true
}

func calculateConfidence(totalSpans, serviceCount, edgeCount, endpointCount, linkedCrossEdges int) float64 {
	score := 0.35
	if serviceCount > 1 {
		score += 0.2
	}
	if edgeCount > 0 {
		score += 0.15
	}
	if endpointCount > 0 {
		score += 0.15
	}
	if totalSpans > 0 && linkedCrossEdges > 0 {
		score += math.Min(0.15, (float64(linkedCrossEdges)/float64(totalSpans))*0.15)
	}
	if score > 1 {
		score = 1
	}
	return math.Round(score*100) / 100
}

func edgeKey(from, to string, kind model.EdgeKind, blocking bool) string {
	return model.EdgeID(from, to, kind, blocking)
}

func firstAttr(attrs map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := attrString(attrs, key); strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func attrString(attrs map[string]any, key string) string {
	if attrs == nil {
		return ""
	}
	value, ok := attrs[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprintf("%v", typed)
	}
}

func parseMethodAndPathFromSpanName(name string) (string, string) {
	parts := strings.Fields(strings.TrimSpace(name))
	if len(parts) < 2 {
		return "", ""
	}
	method := strings.ToUpper(parts[0])
	if !isHTTPMethod(method) {
		return "", ""
	}
	return method, parts[1]
}

func isHTTPMethod(method string) bool {
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		return true
	default:
		return false
	}
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if idx := strings.Index(path, "?"); idx >= 0 {
		path = path[:idx]
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		if parsed, err := url.Parse(path); err == nil {
			path = parsed.Path
		}
	}
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

type serviceAccumulator struct {
	observations int
	traceIDs     map[string]struct{}
	firstSeen    time.Time
	lastSeen     time.Time
}

func updateServiceAccumulator(acc *serviceAccumulator, span traces.Span, runtimeMode bool) *serviceAccumulator {
	if acc == nil {
		acc = &serviceAccumulator{traceIDs: map[string]struct{}{}}
	}
	acc.observations++
	if span.TraceID != "" {
		acc.traceIDs[span.TraceID] = struct{}{}
	}
	acc.observeTime(span.EventTime(time.Time{}), runtimeMode)
	return acc
}

func (a *serviceAccumulator) observeTime(ts time.Time, runtimeMode bool) {
	if !runtimeMode || ts.IsZero() {
		return
	}
	if a.firstSeen.IsZero() || ts.Before(a.firstSeen) {
		a.firstSeen = ts
	}
	if a.lastSeen.IsZero() || ts.After(a.lastSeen) {
		a.lastSeen = ts
	}
}

func (a *serviceAccumulator) supportSummary() snapshot.SupportSummary {
	return snapshot.SupportSummary{Observations: a.observations, TraceCount: len(a.traceIDs)}
}

type edgeAccumulator struct {
	id           string
	edge         model.Edge
	observations int
	traceIDs     map[string]struct{}
	evidence     map[string]struct{}
	latencyMS    []float64
	firstSeen    time.Time
	lastSeen     time.Time
}

func updateEdgeAccumulator(acc *edgeAccumulator, id string, parent, child traces.Span, kind model.EdgeKind, blocking bool, evidence []string, runtimeMode bool) *edgeAccumulator {
	if acc == nil {
		acc = &edgeAccumulator{
			id:       id,
			edge:     model.Edge{ID: id, From: parent.Service, To: child.Service, Kind: kind, Blocking: blocking},
			traceIDs: map[string]struct{}{},
			evidence: map[string]struct{}{},
		}
	}
	acc.observations++
	if child.TraceID != "" {
		acc.traceIDs[child.TraceID] = struct{}{}
	}
	for _, item := range evidence {
		if strings.TrimSpace(item) != "" {
			acc.evidence[item] = struct{}{}
		}
	}
	if latency, ok := spanDurationMS(child); ok {
		acc.latencyMS = append(acc.latencyMS, latency)
	}
	acc.observeTime(child.EventTime(time.Time{}), runtimeMode)
	return acc
}

func (a *edgeAccumulator) observeTime(ts time.Time, runtimeMode bool) {
	if !runtimeMode || ts.IsZero() {
		return
	}
	if a.firstSeen.IsZero() || ts.Before(a.firstSeen) {
		a.firstSeen = ts
	}
	if a.lastSeen.IsZero() || ts.After(a.lastSeen) {
		a.lastSeen = ts
	}
}

func (a *edgeAccumulator) supportSummary() snapshot.SupportSummary {
	return snapshot.SupportSummary{Observations: a.observations, TraceCount: len(a.traceIDs), Evidence: sortStringSet(a.evidence)}
}

func (a *edgeAccumulator) observedSummary() *model.ObservedEdge {
	if len(a.latencyMS) == 0 {
		return nil
	}
	samples := append([]float64(nil), a.latencyMS...)
	sort.Float64s(samples)
	observed := &model.ObservedEdge{
		LatencyMS: &model.LatencySummary{
			P50: float64Ptr(percentile(samples, 0.50)),
			P90: float64Ptr(percentile(samples, 0.90)),
			P95: float64Ptr(percentile(samples, 0.95)),
			P99: float64Ptr(percentile(samples, 0.99)),
		},
	}
	observed.Normalize()
	if observed.IsZero() {
		return nil
	}
	return observed
}

type endpointAccumulator struct {
	endpoint     model.Endpoint
	observations int
	traceIDs     map[string]struct{}
	firstSeen    time.Time
	lastSeen     time.Time
}

func updateEndpointAccumulator(acc *endpointAccumulator, endpoint model.Endpoint, span traces.Span, runtimeMode bool) *endpointAccumulator {
	if acc == nil {
		acc = &endpointAccumulator{endpoint: endpoint, traceIDs: map[string]struct{}{}}
	}
	acc.observations++
	if span.TraceID != "" {
		acc.traceIDs[span.TraceID] = struct{}{}
	}
	acc.observeTime(span.EventTime(time.Time{}), runtimeMode)
	return acc
}

func (a *endpointAccumulator) observeTime(ts time.Time, runtimeMode bool) {
	if !runtimeMode || ts.IsZero() {
		return
	}
	if a.firstSeen.IsZero() || ts.Before(a.firstSeen) {
		a.firstSeen = ts
	}
	if a.lastSeen.IsZero() || ts.After(a.lastSeen) {
		a.lastSeen = ts
	}
}

func (a *endpointAccumulator) supportSummary() snapshot.SupportSummary {
	return snapshot.SupportSummary{Observations: a.observations, TraceCount: len(a.traceIDs)}
}

func applyReplicaOverlays(serviceSet map[string]int, overlays []overlay.File) error {
	for _, file := range overlays {
		for _, item := range file.Services {
			if item.Replicas == nil {
				continue
			}
			if _, exists := serviceSet[item.ID]; !exists {
				return fmt.Errorf("overlay %q references unknown service %q", file.Name, item.ID)
			}
			serviceSet[item.ID] = *item.Replicas
		}
	}
	return nil
}

func applyOverlays(mdl *model.ResilienceModel, services []snapshot.ServiceRecord, edges []snapshot.EdgeRecord, endpoints []snapshot.EndpointRecord, overlays []overlay.File) ([]snapshot.OverlayApplication, error) {
	serviceIndex := make(map[string]int, len(services))
	serviceModelIndex := make(map[string]int, len(mdl.Services))
	edgeIndex := make(map[string]int, len(edges))
	edgeModelIndex := make(map[string]int, len(mdl.Edges))
	endpointIndex := make(map[string]int, len(endpoints))
	endpointModelIndex := make(map[string]int, len(mdl.Endpoints))
	for i, item := range services {
		serviceIndex[item.ID] = i
	}
	for i, item := range mdl.Services {
		serviceModelIndex[item.ID] = i
	}
	for i, item := range edges {
		edgeIndex[item.ID] = i
	}
	for i, item := range mdl.Edges {
		edgeModelIndex[item.ID] = i
	}
	for i, item := range endpoints {
		endpointIndex[item.ID] = i
	}
	for i, item := range mdl.Endpoints {
		endpointModelIndex[item.ID] = i
	}
	applications := make([]snapshot.OverlayApplication, 0, len(overlays))
	for i, file := range overlays {
		precedence := i + 1
		applications = append(applications, snapshot.OverlayApplication{Name: file.Name, Ref: file.Ref, Precedence: precedence})
		prov := snapshot.Provenance{Type: "overlay", Name: file.Name, Ref: file.Ref, Precedence: precedence}
		for _, item := range file.Services {
			index, ok := serviceIndex[item.ID]
			if !ok {
				return nil, fmt.Errorf("overlay %q references unknown service %q", file.Name, item.ID)
			}
			modelIndex, ok := serviceModelIndex[item.ID]
			if !ok {
				return nil, fmt.Errorf("overlay %q references unknown model service %q", file.Name, item.ID)
			}
			mergeServiceMetadata(&services[index], &mdl.Services[modelIndex], item)
			services[index].Provenance = append(services[index].Provenance, prov)
			if item.Replicas != nil {
				services[index].Replicas = *item.Replicas
				mdl.Services[modelIndex].Replicas = *item.Replicas
			}
		}
		for _, item := range file.Edges {
			index, ok := edgeIndex[item.ID]
			if !ok {
				return nil, fmt.Errorf("overlay %q references unknown edge %q", file.Name, item.ID)
			}
			modelIndex, ok := edgeModelIndex[item.ID]
			if !ok {
				return nil, fmt.Errorf("overlay %q references unknown model edge %q", file.Name, item.ID)
			}
			mergeEdgeMetadata(&edges[index], &mdl.Edges[modelIndex], item)
			edges[index].Provenance = append(edges[index].Provenance, prov)
		}
		for _, item := range file.Endpoints {
			index, ok := endpointIndex[item.ID]
			if !ok {
				return nil, fmt.Errorf("overlay %q references unknown endpoint %q", file.Name, item.ID)
			}
			modelIndex, ok := endpointModelIndex[item.ID]
			if !ok {
				return nil, fmt.Errorf("overlay %q references unknown model endpoint %q", file.Name, item.ID)
			}
			mergeEndpointMetadata(&endpoints[index], &mdl.Endpoints[modelIndex], item)
			endpoints[index].Provenance = append(endpoints[index].Provenance, prov)
		}
	}
	return applications, nil
}

func mergeServiceMetadata(target *snapshot.ServiceRecord, modelTarget *model.Service, item overlay.ServiceOverlay) {
	recordMeta := ensureSnapshotServiceMetadata(target)
	modelMeta := ensureModelServiceMetadata(modelTarget)
	mergeCommonMetadata(&recordMeta.ServiceMetadata.CommonMetadata, item.CommonMetadata.CommonMetadata)
	mergeCommonMetadata(&modelMeta.CommonMetadata, item.CommonMetadata.CommonMetadata)
	mergeAttributes(&recordMeta.Attributes, item.Attributes)
	if item.FailureEligible != nil {
		recordMeta.FailureEligible = cloneBoolPointer(item.FailureEligible)
		modelMeta.FailureEligible = cloneBoolPointer(item.FailureEligible)
	}
	if item.Replicas != nil {
		recordMeta.ReplicasOverride = cloneIntPointer(item.Replicas)
	}
	if len(item.Placements) > 0 {
		recordMeta.Placements = clonePlacements(item.Placements)
		modelMeta.Placements = clonePlacements(item.Placements)
	}
	if len(item.SharedResourceRefs) > 0 {
		recordMeta.SharedResourceRefs = cloneStringSlice(item.SharedResourceRefs)
		modelMeta.SharedResourceRefs = cloneStringSlice(item.SharedResourceRefs)
	}
}

func mergeEdgeMetadata(target *snapshot.EdgeRecord, modelTarget *model.Edge, item overlay.EdgeOverlay) {
	recordMeta := ensureSnapshotEdgeMetadata(target)
	modelMeta := ensureModelEdgeMetadata(modelTarget)
	mergeCommonMetadata(&recordMeta.EdgeMetadata.CommonMetadata, item.CommonMetadata.CommonMetadata)
	mergeCommonMetadata(&modelMeta.CommonMetadata, item.CommonMetadata.CommonMetadata)
	mergeAttributes(&recordMeta.Attributes, item.Attributes)
	if item.Weight != nil {
		recordMeta.Weight = cloneFloatPointer(item.Weight)
		modelMeta.Weight = cloneFloatPointer(item.Weight)
	}
	mergeResiliencePolicy(&target.Resilience, item.Resilience)
	mergeResiliencePolicy(&modelTarget.Resilience, item.Resilience)
	mergeObservedEdge(&target.Observed, item.Observed)
	mergeObservedEdge(&modelTarget.Observed, item.Observed)
	mergePolicyScope(&target.PolicyScope, item.PolicyScope)
	mergePolicyScope(&modelTarget.PolicyScope, item.PolicyScope)
}

func mergeEndpointMetadata(target *snapshot.EndpointRecord, modelTarget *model.Endpoint, item overlay.EndpointOverlay) {
	recordMeta := ensureSnapshotEndpointMetadata(target)
	modelMeta := ensureModelEndpointMetadata(modelTarget)
	mergeCommonMetadata(&recordMeta.EndpointMetadata.CommonMetadata, item.CommonMetadata.CommonMetadata)
	mergeCommonMetadata(&modelMeta.CommonMetadata, item.CommonMetadata.CommonMetadata)
	mergeAttributes(&recordMeta.Attributes, item.Attributes)
	if item.Weight != nil {
		recordMeta.Weight = cloneFloatPointer(item.Weight)
		modelMeta.Weight = cloneFloatPointer(item.Weight)
	}
	if strings.TrimSpace(item.PredicateRef) != "" {
		recordMeta.PredicateRef = strings.TrimSpace(item.PredicateRef)
		modelTarget.SuccessPredicateRef = strings.TrimSpace(item.PredicateRef)
	}
	if strings.TrimSpace(item.Method) != "" {
		method := strings.ToUpper(strings.TrimSpace(item.Method))
		target.Method = method
		modelTarget.Method = method
	}
	if strings.TrimSpace(item.Path) != "" {
		path := normalizePath(item.Path)
		target.Path = path
		modelTarget.Path = path
	}
}

func mergeCommonMetadata(target *model.CommonMetadata, source model.CommonMetadata) {
	if len(source.Labels) > 0 {
		if target.Labels == nil {
			target.Labels = map[string]string{}
		}
		for key, value := range source.Labels {
			target.Labels[key] = value
		}
	}
	if len(source.Tags) > 0 {
		target.Tags = dedupeStrings(append(target.Tags, source.Tags...))
	}
	if len(source.SLORefs) > 0 {
		target.SLORefs = dedupeStrings(append(target.SLORefs, source.SLORefs...))
	}
}

func mergeAttributes(target *map[string]string, source map[string]string) {
	if len(source) == 0 {
		return
	}
	if *target == nil {
		*target = map[string]string{}
	}
	for key, value := range source {
		(*target)[key] = value
	}
}

func ensureSnapshotServiceMetadata(target *snapshot.ServiceRecord) *snapshot.ServiceMetadata {
	if target.Metadata == nil {
		target.Metadata = &snapshot.ServiceMetadata{}
	}
	return target.Metadata
}

func ensureModelServiceMetadata(target *model.Service) *model.ServiceMetadata {
	if target.Metadata == nil {
		target.Metadata = &model.ServiceMetadata{}
	}
	return target.Metadata
}

func ensureSnapshotEdgeMetadata(target *snapshot.EdgeRecord) *snapshot.EdgeMetadata {
	if target.Metadata == nil {
		target.Metadata = &snapshot.EdgeMetadata{}
	}
	return target.Metadata
}

func ensureModelEdgeMetadata(target *model.Edge) *model.EdgeMetadata {
	if target.Metadata == nil {
		target.Metadata = &model.EdgeMetadata{}
	}
	return target.Metadata
}

func ensureSnapshotEndpointMetadata(target *snapshot.EndpointRecord) *snapshot.EndpointMetadata {
	if target.Metadata == nil {
		target.Metadata = &snapshot.EndpointMetadata{}
	}
	return target.Metadata
}

func ensureModelEndpointMetadata(target *model.Endpoint) *model.EndpointMetadata {
	if target.Metadata == nil {
		target.Metadata = &model.EndpointMetadata{}
	}
	return target.Metadata
}

func mergeResiliencePolicy(target **model.ResiliencePolicy, source *model.ResiliencePolicy) {
	if source == nil {
		return
	}
	if *target == nil {
		*target = cloneResiliencePolicy(source)
		return
	}
	if source.RequestTimeoutMS != nil {
		(*target).RequestTimeoutMS = cloneIntPointer(source.RequestTimeoutMS)
	}
	if source.PerTryTimeoutMS != nil {
		(*target).PerTryTimeoutMS = cloneIntPointer(source.PerTryTimeoutMS)
	}
	mergeRetryPolicy(&(*target).Retry, source.Retry)
	mergeCircuitBreakerPolicy(&(*target).CircuitBreaker, source.CircuitBreaker)
}

func mergeRetryPolicy(target **model.RetryPolicy, source *model.RetryPolicy) {
	if source == nil {
		return
	}
	if *target == nil {
		*target = cloneRetryPolicy(source)
		return
	}
	if source.MaxAttempts != nil {
		(*target).MaxAttempts = cloneIntPointer(source.MaxAttempts)
	}
	if source.BudgetCap != nil {
		(*target).BudgetCap = cloneFloatPointer(source.BudgetCap)
	}
	if len(source.RetryOn) > 0 {
		(*target).RetryOn = dedupeStrings(append(cloneStringSlice((*target).RetryOn), source.RetryOn...))
	}
	if source.Backoff != nil {
		if (*target).Backoff == nil {
			(*target).Backoff = cloneBackoffPolicy(source.Backoff)
		} else {
			mergeBackoffPolicy((*target).Backoff, source.Backoff)
		}
	}
}

func mergeBackoffPolicy(target *model.BackoffPolicy, source *model.BackoffPolicy) {
	if target == nil || source == nil {
		return
	}
	if source.InitialMS != nil {
		target.InitialMS = cloneIntPointer(source.InitialMS)
	}
	if source.MaxMS != nil {
		target.MaxMS = cloneIntPointer(source.MaxMS)
	}
	if source.Multiplier != nil {
		target.Multiplier = cloneFloatPointer(source.Multiplier)
	}
	if strings.TrimSpace(source.Jitter) != "" {
		target.Jitter = strings.TrimSpace(source.Jitter)
	}
}

func mergeCircuitBreakerPolicy(target **model.CircuitBreakerPolicy, source *model.CircuitBreakerPolicy) {
	if source == nil {
		return
	}
	if *target == nil {
		*target = cloneCircuitBreakerPolicy(source)
		return
	}
	for _, item := range []struct {
		dst **int
		src *int
	}{
		{dst: &(*target).MaxPendingRequests, src: source.MaxPendingRequests},
		{dst: &(*target).MaxRequests, src: source.MaxRequests},
		{dst: &(*target).MaxConnections, src: source.MaxConnections},
		{dst: &(*target).Consecutive5xx, src: source.Consecutive5xx},
		{dst: &(*target).IntervalMS, src: source.IntervalMS},
		{dst: &(*target).BaseEjectionTimeMS, src: source.BaseEjectionTimeMS},
	} {
		if item.src != nil {
			*item.dst = cloneIntPointer(item.src)
		}
	}
	if source.Enabled != nil {
		(*target).Enabled = cloneBoolPointer(source.Enabled)
	}
}

func mergeObservedEdge(target **model.ObservedEdge, source *model.ObservedEdge) {
	if source == nil {
		return
	}
	if *target == nil {
		*target = cloneObservedEdge(source)
		return
	}
	if source.ErrorRate != nil {
		(*target).ErrorRate = cloneFloatPointer(source.ErrorRate)
	}
	if source.LatencyMS != nil {
		if (*target).LatencyMS == nil {
			(*target).LatencyMS = cloneLatencySummary(source.LatencyMS)
			return
		}
		for _, item := range []struct {
			dst **float64
			src *float64
		}{
			{dst: &(*target).LatencyMS.P50, src: source.LatencyMS.P50},
			{dst: &(*target).LatencyMS.P90, src: source.LatencyMS.P90},
			{dst: &(*target).LatencyMS.P95, src: source.LatencyMS.P95},
			{dst: &(*target).LatencyMS.P99, src: source.LatencyMS.P99},
		} {
			if item.src != nil {
				*item.dst = cloneFloatPointer(item.src)
			}
		}
	}
}

func mergePolicyScope(target **model.PolicyScope, source *model.PolicyScope) {
	if source == nil {
		return
	}
	if *target == nil {
		*target = clonePolicyScope(source)
		return
	}
	if strings.TrimSpace(source.SourceEndpointID) != "" {
		(*target).SourceEndpointID = strings.TrimSpace(source.SourceEndpointID)
	}
	if strings.TrimSpace(source.SourceRoute) != "" {
		(*target).SourceRoute = strings.TrimSpace(source.SourceRoute)
	}
	if strings.TrimSpace(source.Method) != "" {
		(*target).Method = strings.ToUpper(strings.TrimSpace(source.Method))
	}
	if strings.TrimSpace(source.Operation) != "" {
		(*target).Operation = strings.TrimSpace(source.Operation)
	}
}

func rebuildServiceRecords(records []snapshot.ServiceRecord, services []model.Service) []snapshot.ServiceRecord {
	index := make(map[string]model.Service, len(services))
	for _, item := range services {
		index[item.ID] = item
	}
	for i := range records {
		if svc, ok := index[records[i].ID]; ok {
			records[i].Replicas = svc.Replicas
			records[i].Name = svc.Name
		}
	}
	return records
}

func minSupportServices(items []snapshot.ServiceRecord) int {
	return minSupport(len(items), func(i int) int { return items[i].Support.Observations })
}

func minSupportEdges(items []snapshot.EdgeRecord) int {
	return minSupport(len(items), func(i int) int { return items[i].Support.Observations })
}

func minSupportEndpoints(items []snapshot.EndpointRecord) int {
	return minSupport(len(items), func(i int) int { return items[i].Support.Observations })
}

func minSupport(length int, value func(int) int) int {
	if length == 0 {
		return 0
	}
	min := value(0)
	for i := 1; i < length; i++ {
		if current := value(i); current < min {
			min = current
		}
	}
	return min
}

func formatOptionalTime(ts time.Time, include bool) string {
	if !include || ts.IsZero() {
		return ""
	}
	return ts.UTC().Format(time.RFC3339)
}

func sortStringSet(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func dedupeStrings(values []string) []string {
	set := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, exists := set[trimmed]; exists {
			continue
		}
		set[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	sort.Strings(out)
	return out
}

func inferConnector(sources []snapshot.SourceSummary) string {
	if len(sources) == 0 {
		return "trace_file"
	}
	return strings.TrimSpace(sources[0].Connector)
}

func spanDurationMS(span traces.Span) (float64, bool) {
	if span.StartTime.IsZero() || span.EndTime.IsZero() || span.EndTime.Before(span.StartTime) {
		return 0, false
	}
	return model.FormatMilliseconds(float64(span.EndTime.Sub(span.StartTime)) / float64(time.Millisecond)), true
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return model.FormatMilliseconds(sorted[0])
	}
	if p <= 0 {
		return model.FormatMilliseconds(sorted[0])
	}
	if p >= 1 {
		return model.FormatMilliseconds(sorted[len(sorted)-1])
	}
	index := int(math.Ceil(float64(len(sorted))*p)) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return model.FormatMilliseconds(sorted[index])
}

func float64Ptr(value float64) *float64 {
	return &value
}

func defaultSources(sources []snapshot.SourceSummary, ref string, observations int) []snapshot.SourceSummary {
	if len(sources) > 0 {
		out := append([]snapshot.SourceSummary(nil), sources...)
		if out[0].Ref == "" {
			out[0].Ref = ref
		}
		if out[0].Observations == 0 {
			out[0].Observations = observations
		}
		return out
	}
	return []snapshot.SourceSummary{{Type: "traces", Connector: "trace_file", Ref: ref, Observations: observations}}
}
