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
	"github.com/MB3R-Lab/Bering/internal/schema"
)

const SourceTypeBering = "bering"

type Options struct {
	SourceRef        string
	DiscoveredAt     string
	ReplicasOverride map[string]int
}

func Build(spans []traces.Span, opts Options) (model.ResilienceModel, error) {
	if len(spans) == 0 {
		return model.ResilienceModel{}, fmt.Errorf("no spans provided for discovery")
	}

	discoveredAt := opts.DiscoveredAt
	if strings.TrimSpace(discoveredAt) == "" {
		discoveredAt = time.Now().UTC().Format(time.RFC3339)
	}
	if _, err := time.Parse(time.RFC3339, discoveredAt); err != nil {
		return model.ResilienceModel{}, fmt.Errorf("discovered_at must be RFC3339: %w", err)
	}

	sourceRef := strings.TrimSpace(opts.SourceRef)
	if sourceRef == "" {
		sourceRef = "bering://discover"
	}

	spanByKey := map[string]traces.Span{}
	serviceSet := map[string]int{}
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
	}

	if len(serviceSet) == 0 {
		return model.ResilienceModel{}, fmt.Errorf("discovery produced no services")
	}

	for serviceID, replicas := range opts.ReplicasOverride {
		if _, exists := serviceSet[serviceID]; !exists {
			return model.ResilienceModel{}, fmt.Errorf("replicas override references unknown service %q", serviceID)
		}
		serviceSet[serviceID] = replicas
	}

	edgeSet := map[string]model.Edge{}
	endpointSet := map[string]model.Endpoint{}
	linkedCrossServiceEdges := 0

	for _, span := range spans {
		if strings.TrimSpace(span.Service) == "" {
			continue
		}

		parent, hasParent := parentSpan(span, spanByKey)
		if hasParent && strings.TrimSpace(parent.Service) != "" && parent.Service != span.Service {
			kind := edgeKind(parent, span)
			blocking := kind == model.EdgeKindSync
			key := edgeKey(parent.Service, span.Service, kind, blocking)
			if _, exists := edgeSet[key]; !exists {
				edgeSet[key] = model.Edge{
					From:     parent.Service,
					To:       span.Service,
					Kind:     kind,
					Blocking: blocking,
				}
			}
			linkedCrossServiceEdges++
		}

		if endpoint, ok := inferEndpoint(span, parent, hasParent); ok {
			endpointSet[endpoint.ID] = endpoint
		}
	}

	services := make([]model.Service, 0, len(serviceSet))
	for id, replicas := range serviceSet {
		services = append(services, model.Service{
			ID:       id,
			Name:     id,
			Replicas: replicas,
		})
	}
	sort.Slice(services, func(i, j int) bool { return services[i].ID < services[j].ID })

	edges := make([]model.Edge, 0, len(edgeSet))
	for _, edge := range edgeSet {
		edges = append(edges, edge)
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

	endpoints := make([]model.Endpoint, 0, len(endpointSet))
	for _, endpoint := range endpointSet {
		endpoints = append(endpoints, endpoint)
	}
	sort.Slice(endpoints, func(i, j int) bool { return endpoints[i].ID < endpoints[j].ID })

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
		return model.ResilienceModel{}, err
	}
	mdl.SortDeterministic()
	return mdl, nil
}

func BuildSourceRef(input string) string {
	clean := strings.TrimSpace(input)
	if clean == "" {
		return "bering://discover"
	}
	clean = filepath.ToSlash(filepath.Clean(clean))
	return "bering://discover?input=" + url.QueryEscape(clean)
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

func edgeKind(parent, child traces.Span) model.EdgeKind {
	if isAsyncSpan(parent) || isAsyncSpan(child) {
		return model.EdgeKindAsync
	}
	return model.EdgeKindSync
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

func inferEndpoint(span, parent traces.Span, hasParent bool) (model.Endpoint, bool) {
	if strings.TrimSpace(span.Service) == "" {
		return model.Endpoint{}, false
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
		return model.Endpoint{}, false
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
		return model.Endpoint{}, false
	}

	isServer := strings.EqualFold(span.Kind, "server")
	if !isServer {
		// If span isn't marked as server, only keep it as entry when it has no same-service parent.
		if hasParent && parent.Service == span.Service {
			return model.Endpoint{}, false
		}
	}

	id := fmt.Sprintf("%s:%s %s", span.Service, method, path)
	return model.Endpoint{
		ID:                  id,
		EntryService:        span.Service,
		SuccessPredicateRef: id,
	}, true
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
	// Keep two decimals stable for deterministic output readability.
	return math.Round(score*100) / 100
}

func edgeKey(from, to string, kind model.EdgeKind, blocking bool) string {
	return fmt.Sprintf("%s|%s|%s|%t", from, to, kind, blocking)
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
