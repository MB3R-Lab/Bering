package model

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type EdgeKind string

const (
	EdgeKindSync  EdgeKind = "sync"
	EdgeKindAsync EdgeKind = "async"
)

type Service struct {
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	Replicas int              `json:"replicas"`
	Metadata *ServiceMetadata `json:"metadata,omitempty"`
}

type Edge struct {
	ID          string            `json:"id,omitempty"`
	From        string            `json:"from"`
	To          string            `json:"to"`
	Kind        EdgeKind          `json:"kind"`
	Blocking    bool              `json:"blocking"`
	Metadata    *EdgeMetadata     `json:"metadata,omitempty"`
	Resilience  *ResiliencePolicy `json:"resilience,omitempty"`
	Observed    *ObservedEdge     `json:"observed,omitempty"`
	PolicyScope *PolicyScope      `json:"policy_scope,omitempty"`
}

type Endpoint struct {
	ID                  string            `json:"id"`
	EntryService        string            `json:"entry_service"`
	SuccessPredicateRef string            `json:"success_predicate_ref"`
	Method              string            `json:"method,omitempty"`
	Path                string            `json:"path,omitempty"`
	Metadata            *EndpointMetadata `json:"metadata,omitempty"`
}

type Metadata struct {
	SourceType   string    `json:"source_type"`
	SourceRef    string    `json:"source_ref"`
	DiscoveredAt string    `json:"discovered_at"`
	Confidence   float64   `json:"confidence"`
	Schema       SchemaRef `json:"schema"`
}

type SchemaRef struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	URI     string `json:"uri"`
	Digest  string `json:"digest"`
}

type CommonMetadata struct {
	Labels  map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Tags    []string          `json:"tags,omitempty" yaml:"tags,omitempty"`
	SLORefs []string          `json:"slo_refs,omitempty" yaml:"slo_refs,omitempty"`
}

type Placement struct {
	Replicas int               `json:"replicas" yaml:"replicas"`
	Labels   map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
}

type ServiceMetadata struct {
	CommonMetadata
	FailureEligible    *bool       `json:"failure_eligible,omitempty" yaml:"failure_eligible,omitempty"`
	Placements         []Placement `json:"placements,omitempty" yaml:"placements,omitempty"`
	SharedResourceRefs []string    `json:"shared_resource_refs,omitempty" yaml:"shared_resource_refs,omitempty"`
}

type EdgeMetadata struct {
	CommonMetadata
	Weight *float64 `json:"weight,omitempty" yaml:"weight,omitempty"`
}

type EndpointMetadata struct {
	CommonMetadata
	Weight *float64 `json:"weight,omitempty" yaml:"weight,omitempty"`
}

type ResiliencePolicy struct {
	RequestTimeoutMS *int                  `json:"request_timeout_ms,omitempty" yaml:"request_timeout_ms,omitempty"`
	PerTryTimeoutMS  *int                  `json:"per_try_timeout_ms,omitempty" yaml:"per_try_timeout_ms,omitempty"`
	Retry            *RetryPolicy          `json:"retry,omitempty" yaml:"retry,omitempty"`
	CircuitBreaker   *CircuitBreakerPolicy `json:"circuit_breaker,omitempty" yaml:"circuit_breaker,omitempty"`
}

type RetryPolicy struct {
	MaxAttempts *int           `json:"max_attempts,omitempty" yaml:"max_attempts,omitempty"`
	BudgetCap   *float64       `json:"budget_cap,omitempty" yaml:"budget_cap,omitempty"`
	RetryOn     []string       `json:"retry_on,omitempty" yaml:"retry_on,omitempty"`
	Backoff     *BackoffPolicy `json:"backoff,omitempty" yaml:"backoff,omitempty"`
}

type BackoffPolicy struct {
	InitialMS  *int     `json:"initial_ms,omitempty" yaml:"initial_ms,omitempty"`
	MaxMS      *int     `json:"max_ms,omitempty" yaml:"max_ms,omitempty"`
	Multiplier *float64 `json:"multiplier,omitempty" yaml:"multiplier,omitempty"`
	Jitter     string   `json:"jitter,omitempty" yaml:"jitter,omitempty"`
}

type CircuitBreakerPolicy struct {
	Enabled            *bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	MaxPendingRequests *int  `json:"max_pending_requests,omitempty" yaml:"max_pending_requests,omitempty"`
	MaxRequests        *int  `json:"max_requests,omitempty" yaml:"max_requests,omitempty"`
	MaxConnections     *int  `json:"max_connections,omitempty" yaml:"max_connections,omitempty"`
	Consecutive5xx     *int  `json:"consecutive_5xx,omitempty" yaml:"consecutive_5xx,omitempty"`
	IntervalMS         *int  `json:"interval_ms,omitempty" yaml:"interval_ms,omitempty"`
	BaseEjectionTimeMS *int  `json:"base_ejection_time_ms,omitempty" yaml:"base_ejection_time_ms,omitempty"`
}

type ObservedEdge struct {
	LatencyMS *LatencySummary `json:"latency_ms,omitempty" yaml:"latency_ms,omitempty"`
	ErrorRate *float64        `json:"error_rate,omitempty" yaml:"error_rate,omitempty"`
}

type LatencySummary struct {
	P50 *float64 `json:"p50,omitempty" yaml:"p50,omitempty"`
	P90 *float64 `json:"p90,omitempty" yaml:"p90,omitempty"`
	P95 *float64 `json:"p95,omitempty" yaml:"p95,omitempty"`
	P99 *float64 `json:"p99,omitempty" yaml:"p99,omitempty"`
}

type PolicyScope struct {
	SourceEndpointID string `json:"source_endpoint_id,omitempty" yaml:"source_endpoint_id,omitempty"`
	SourceRoute      string `json:"source_route,omitempty" yaml:"source_route,omitempty"`
	Method           string `json:"method,omitempty" yaml:"method,omitempty"`
	Operation        string `json:"operation,omitempty" yaml:"operation,omitempty"`
}

type ResilienceModel struct {
	Services  []Service  `json:"services"`
	Edges     []Edge     `json:"edges"`
	Endpoints []Endpoint `json:"endpoints"`
	Metadata  Metadata   `json:"metadata"`
}

func (m *ResilienceModel) SortDeterministic() {
	for i := range m.Services {
		m.Services[i].normalizeForOutput()
	}
	for i := range m.Edges {
		m.Edges[i].normalizeForOutput()
	}
	for i := range m.Endpoints {
		m.Endpoints[i].normalizeForOutput()
	}

	sort.Slice(m.Services, func(i, j int) bool {
		return m.Services[i].ID < m.Services[j].ID
	})

	sort.Slice(m.Edges, func(i, j int) bool {
		left, right := m.Edges[i], m.Edges[j]
		if left.ID != right.ID {
			return left.ID < right.ID
		}
		if left.From != right.From {
			return left.From < right.From
		}
		if left.To != right.To {
			return left.To < right.To
		}
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if left.Blocking != right.Blocking {
			return !left.Blocking && right.Blocking
		}
		return false
	})

	sort.Slice(m.Endpoints, func(i, j int) bool {
		return m.Endpoints[i].ID < m.Endpoints[j].ID
	})
}

func (m ResilienceModel) ValidateSemantic() error {
	if len(m.Services) == 0 {
		return errors.New("services cannot be empty")
	}

	serviceSet := make(map[string]struct{}, len(m.Services))
	for _, svc := range m.Services {
		if strings.TrimSpace(svc.ID) == "" {
			return errors.New("service id cannot be empty")
		}
		if strings.TrimSpace(svc.Name) == "" {
			return fmt.Errorf("service %q has empty name", svc.ID)
		}
		if svc.Replicas < 0 {
			return fmt.Errorf("service %q replicas cannot be negative", svc.ID)
		}
		if _, exists := serviceSet[svc.ID]; exists {
			return fmt.Errorf("duplicate service id: %s", svc.ID)
		}
		if err := validateServiceMetadata(svc.Metadata); err != nil {
			return fmt.Errorf("service %q metadata: %w", svc.ID, err)
		}
		serviceSet[svc.ID] = struct{}{}
	}

	edgeIDSet := make(map[string]struct{}, len(m.Edges))
	for _, edge := range m.Edges {
		if _, exists := serviceSet[edge.From]; !exists {
			return fmt.Errorf("edge.from service not found: %s", edge.From)
		}
		if _, exists := serviceSet[edge.To]; !exists {
			return fmt.Errorf("edge.to service not found: %s", edge.To)
		}
		if edge.Kind != EdgeKindSync && edge.Kind != EdgeKindAsync {
			return fmt.Errorf("unsupported edge kind: %s", edge.Kind)
		}
		if trimmed := strings.TrimSpace(edge.ID); trimmed != "" {
			if _, exists := edgeIDSet[trimmed]; exists {
				return fmt.Errorf("duplicate edge id: %s", trimmed)
			}
			edgeIDSet[trimmed] = struct{}{}
		}
		if err := validateEdgeMetadata(edge.Metadata); err != nil {
			return fmt.Errorf("edge %q metadata: %w", edgeIdentifier(edge), err)
		}
		if err := validateResilience(edge.Resilience); err != nil {
			return fmt.Errorf("edge %q resilience: %w", edgeIdentifier(edge), err)
		}
		if err := validateObserved(edge.Observed); err != nil {
			return fmt.Errorf("edge %q observed: %w", edgeIdentifier(edge), err)
		}
	}

	endpointSet := make(map[string]struct{}, len(m.Endpoints))
	for _, endpoint := range m.Endpoints {
		if strings.TrimSpace(endpoint.ID) == "" {
			return errors.New("endpoint id cannot be empty")
		}
		if _, exists := endpointSet[endpoint.ID]; exists {
			return fmt.Errorf("duplicate endpoint id: %s", endpoint.ID)
		}
		endpointSet[endpoint.ID] = struct{}{}

		if _, exists := serviceSet[endpoint.EntryService]; !exists {
			return fmt.Errorf("endpoint %q references unknown entry service: %s", endpoint.ID, endpoint.EntryService)
		}
		if strings.TrimSpace(endpoint.SuccessPredicateRef) == "" {
			return fmt.Errorf("endpoint %q success_predicate_ref cannot be empty", endpoint.ID)
		}
		if err := validateEndpointMetadata(endpoint.Metadata); err != nil {
			return fmt.Errorf("endpoint %q metadata: %w", endpoint.ID, err)
		}
	}

	if strings.TrimSpace(m.Metadata.SourceType) == "" {
		return errors.New("metadata.source_type cannot be empty")
	}
	if strings.TrimSpace(m.Metadata.SourceRef) == "" {
		return errors.New("metadata.source_ref cannot be empty")
	}
	if strings.TrimSpace(m.Metadata.DiscoveredAt) == "" {
		return errors.New("metadata.discovered_at cannot be empty")
	}
	if _, err := time.Parse(time.RFC3339, m.Metadata.DiscoveredAt); err != nil {
		return fmt.Errorf("metadata.discovered_at must be RFC3339: %w", err)
	}
	if m.Metadata.Confidence < 0 || m.Metadata.Confidence > 1 {
		return errors.New("metadata.confidence must be in [0,1]")
	}

	return nil
}

func EdgeID(from, to string, kind EdgeKind, blocking bool) string {
	return fmt.Sprintf("%s|%s|%s|%t", strings.TrimSpace(from), strings.TrimSpace(to), kind, blocking)
}

func FormatMilliseconds(value float64) float64 {
	return roundFloat(value)
}

func roundFloat(value float64) float64 {
	text := strconv.FormatFloat(value, 'f', 3, 64)
	parsed, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return value
	}
	return parsed
}

func (s *Service) normalizeForOutput() {
	if s == nil {
		return
	}
	if s.Metadata != nil {
		s.Metadata.Normalize()
		if s.Metadata.IsZero() {
			s.Metadata = nil
		}
	}
}

func (e *Edge) normalizeForOutput() {
	if e == nil {
		return
	}
	if strings.TrimSpace(e.ID) == "" {
		e.ID = EdgeID(e.From, e.To, e.Kind, e.Blocking)
	}
	if e.Metadata != nil {
		e.Metadata.Normalize()
		if e.Metadata.IsZero() {
			e.Metadata = nil
		}
	}
	if e.Resilience != nil {
		e.Resilience.Normalize()
		if e.Resilience.IsZero() {
			e.Resilience = nil
		}
	}
	if e.Observed != nil {
		e.Observed.Normalize()
		if e.Observed.IsZero() {
			e.Observed = nil
		}
	}
	if e.PolicyScope != nil {
		e.PolicyScope.Normalize()
		if e.PolicyScope.IsZero() {
			e.PolicyScope = nil
		}
	}
}

func (e *Endpoint) normalizeForOutput() {
	if e == nil {
		return
	}
	if e.Metadata != nil {
		e.Metadata.Normalize()
		if e.Metadata.IsZero() {
			e.Metadata = nil
		}
	}
}

func (m *CommonMetadata) Normalize() {
	if m == nil {
		return
	}
	m.Labels = normalizeStringMap(m.Labels)
	m.Tags = normalizeStringSlice(m.Tags)
	m.SLORefs = normalizeStringSlice(m.SLORefs)
}

func (m CommonMetadata) IsZero() bool {
	return len(m.Labels) == 0 && len(m.Tags) == 0 && len(m.SLORefs) == 0
}

func (m *ServiceMetadata) Normalize() {
	if m == nil {
		return
	}
	m.CommonMetadata.Normalize()
	for i := range m.Placements {
		m.Placements[i].Labels = normalizeStringMap(m.Placements[i].Labels)
	}
	sort.Slice(m.Placements, func(i, j int) bool {
		left, right := m.Placements[i], m.Placements[j]
		if left.Replicas != right.Replicas {
			return left.Replicas < right.Replicas
		}
		return placementKey(left) < placementKey(right)
	})
	m.SharedResourceRefs = normalizeStringSlice(m.SharedResourceRefs)
}

func (m ServiceMetadata) IsZero() bool {
	return m.CommonMetadata.IsZero() && m.FailureEligible == nil && len(m.Placements) == 0 && len(m.SharedResourceRefs) == 0
}

func (m *EdgeMetadata) Normalize() {
	if m == nil {
		return
	}
	m.CommonMetadata.Normalize()
}

func (m EdgeMetadata) IsZero() bool {
	return m.CommonMetadata.IsZero() && m.Weight == nil
}

func (m *EndpointMetadata) Normalize() {
	if m == nil {
		return
	}
	m.CommonMetadata.Normalize()
}

func (m EndpointMetadata) IsZero() bool {
	return m.CommonMetadata.IsZero() && m.Weight == nil
}

func (p *ResiliencePolicy) Normalize() {
	if p == nil {
		return
	}
	if p.Retry != nil {
		p.Retry.Normalize()
		if p.Retry.IsZero() {
			p.Retry = nil
		}
	}
	if p.CircuitBreaker != nil {
		p.CircuitBreaker.Normalize()
		if p.CircuitBreaker.IsZero() {
			p.CircuitBreaker = nil
		}
	}
}

func (p ResiliencePolicy) IsZero() bool {
	return p.RequestTimeoutMS == nil && p.PerTryTimeoutMS == nil && p.Retry == nil && p.CircuitBreaker == nil
}

func (p *RetryPolicy) Normalize() {
	if p == nil {
		return
	}
	p.RetryOn = normalizeStringSlice(p.RetryOn)
	if p.Backoff != nil {
		p.Backoff.Normalize()
		if p.Backoff.IsZero() {
			p.Backoff = nil
		}
	}
}

func (p RetryPolicy) IsZero() bool {
	return p.MaxAttempts == nil && p.BudgetCap == nil && len(p.RetryOn) == 0 && p.Backoff == nil
}

func (p *BackoffPolicy) Normalize() {
	if p == nil {
		return
	}
	p.Jitter = strings.TrimSpace(p.Jitter)
}

func (p BackoffPolicy) IsZero() bool {
	return p.InitialMS == nil && p.MaxMS == nil && p.Multiplier == nil && p.Jitter == ""
}

func (p *CircuitBreakerPolicy) Normalize() {
	if p == nil {
		return
	}
}

func (p CircuitBreakerPolicy) IsZero() bool {
	return p.Enabled == nil &&
		p.MaxPendingRequests == nil &&
		p.MaxRequests == nil &&
		p.MaxConnections == nil &&
		p.Consecutive5xx == nil &&
		p.IntervalMS == nil &&
		p.BaseEjectionTimeMS == nil
}

func (o *ObservedEdge) Normalize() {
	if o == nil {
		return
	}
	if o.LatencyMS != nil {
		o.LatencyMS.Normalize()
		if o.LatencyMS.IsZero() {
			o.LatencyMS = nil
		}
	}
	if o.ErrorRate != nil {
		value := roundFloat(*o.ErrorRate)
		o.ErrorRate = &value
	}
}

func (o ObservedEdge) IsZero() bool {
	return o.LatencyMS == nil && o.ErrorRate == nil
}

func (s *LatencySummary) Normalize() {
	if s == nil {
		return
	}
	for _, value := range []*float64{s.P50, s.P90, s.P95, s.P99} {
		if value == nil {
			continue
		}
		normalized := roundFloat(*value)
		*value = normalized
	}
}

func (s LatencySummary) IsZero() bool {
	return s.P50 == nil && s.P90 == nil && s.P95 == nil && s.P99 == nil
}

func (s *PolicyScope) Normalize() {
	if s == nil {
		return
	}
	s.SourceEndpointID = strings.TrimSpace(s.SourceEndpointID)
	s.SourceRoute = strings.TrimSpace(s.SourceRoute)
	s.Method = strings.ToUpper(strings.TrimSpace(s.Method))
	s.Operation = strings.TrimSpace(s.Operation)
}

func (s PolicyScope) IsZero() bool {
	return s.SourceEndpointID == "" && s.SourceRoute == "" && s.Method == "" && s.Operation == ""
}

func validateServiceMetadata(meta *ServiceMetadata) error {
	if meta == nil {
		return nil
	}
	for i, placement := range meta.Placements {
		if placement.Replicas < 0 {
			return fmt.Errorf("placements[%d].replicas must be >= 0", i)
		}
	}
	return nil
}

func validateEdgeMetadata(meta *EdgeMetadata) error {
	if meta == nil || meta.Weight == nil {
		return nil
	}
	if *meta.Weight < 0 {
		return fmt.Errorf("weight must be >= 0")
	}
	return nil
}

func validateEndpointMetadata(meta *EndpointMetadata) error {
	if meta == nil || meta.Weight == nil {
		return nil
	}
	if *meta.Weight < 0 {
		return fmt.Errorf("weight must be >= 0")
	}
	return nil
}

func validateResilience(policy *ResiliencePolicy) error {
	if policy == nil {
		return nil
	}
	for _, item := range []struct {
		name  string
		value *int
	}{
		{"request_timeout_ms", policy.RequestTimeoutMS},
		{"per_try_timeout_ms", policy.PerTryTimeoutMS},
	} {
		if item.value != nil && *item.value < 0 {
			return fmt.Errorf("%s must be >= 0", item.name)
		}
	}
	if policy.Retry != nil {
		if policy.Retry.MaxAttempts != nil && *policy.Retry.MaxAttempts < 0 {
			return fmt.Errorf("retry.max_attempts must be >= 0")
		}
		if policy.Retry.BudgetCap != nil && *policy.Retry.BudgetCap < 0 {
			return fmt.Errorf("retry.budget_cap must be >= 0")
		}
		if policy.Retry.Backoff != nil {
			for _, item := range []struct {
				name  string
				value *int
			}{
				{"retry.backoff.initial_ms", policy.Retry.Backoff.InitialMS},
				{"retry.backoff.max_ms", policy.Retry.Backoff.MaxMS},
			} {
				if item.value != nil && *item.value < 0 {
					return fmt.Errorf("%s must be >= 0", item.name)
				}
			}
			if policy.Retry.Backoff.Multiplier != nil && *policy.Retry.Backoff.Multiplier < 0 {
				return fmt.Errorf("retry.backoff.multiplier must be >= 0")
			}
		}
	}
	if policy.CircuitBreaker != nil {
		for _, item := range []struct {
			name  string
			value *int
		}{
			{"circuit_breaker.max_pending_requests", policy.CircuitBreaker.MaxPendingRequests},
			{"circuit_breaker.max_requests", policy.CircuitBreaker.MaxRequests},
			{"circuit_breaker.max_connections", policy.CircuitBreaker.MaxConnections},
			{"circuit_breaker.consecutive_5xx", policy.CircuitBreaker.Consecutive5xx},
			{"circuit_breaker.interval_ms", policy.CircuitBreaker.IntervalMS},
			{"circuit_breaker.base_ejection_time_ms", policy.CircuitBreaker.BaseEjectionTimeMS},
		} {
			if item.value != nil && *item.value < 0 {
				return fmt.Errorf("%s must be >= 0", item.name)
			}
		}
	}
	return nil
}

func validateObserved(observed *ObservedEdge) error {
	if observed == nil {
		return nil
	}
	if observed.ErrorRate != nil && (*observed.ErrorRate < 0 || *observed.ErrorRate > 1) {
		return fmt.Errorf("error_rate must be in [0,1]")
	}
	if observed.LatencyMS == nil {
		return nil
	}
	for _, item := range []struct {
		name  string
		value *float64
	}{
		{"latency_ms.p50", observed.LatencyMS.P50},
		{"latency_ms.p90", observed.LatencyMS.P90},
		{"latency_ms.p95", observed.LatencyMS.P95},
		{"latency_ms.p99", observed.LatencyMS.P99},
	} {
		if item.value != nil && *item.value < 0 {
			return fmt.Errorf("%s must be >= 0", item.name)
		}
	}
	return validateMonotonicLatency(observed.LatencyMS)
}

func validateMonotonicLatency(summary *LatencySummary) error {
	if summary == nil {
		return nil
	}
	ordered := []struct {
		name  string
		value *float64
	}{
		{"p50", summary.P50},
		{"p90", summary.P90},
		{"p95", summary.P95},
		{"p99", summary.P99},
	}
	var (
		lastName  string
		lastValue float64
		haveLast  bool
	)
	for _, item := range ordered {
		if item.value == nil {
			continue
		}
		if haveLast && *item.value < lastValue {
			return fmt.Errorf("%s must be >= %s", item.name, lastName)
		}
		lastName = item.name
		lastValue = *item.value
		haveLast = true
	}
	return nil
}

func normalizeStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		out[trimmedKey] = strings.TrimSpace(value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
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
	if len(out) == 0 {
		return nil
	}
	return out
}

func placementKey(value Placement) string {
	keys := make([]string, 0, len(value.Labels))
	for key := range value.Labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+value.Labels[key])
	}
	return strings.Join(parts, "|")
}

func edgeIdentifier(edge Edge) string {
	if strings.TrimSpace(edge.ID) != "" {
		return edge.ID
	}
	return EdgeID(edge.From, edge.To, edge.Kind, edge.Blocking)
}
