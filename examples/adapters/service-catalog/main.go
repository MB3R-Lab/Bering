package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type catalog struct {
	SourceRef    string              `json:"source_ref"`
	Services     []catalogService    `json:"services"`
	Dependencies []catalogDependency `json:"dependencies"`
	Endpoints    []catalogEndpoint   `json:"endpoints"`
}

type catalogService struct {
	ID              string            `json:"id"`
	Replicas        int               `json:"replicas"`
	Team            string            `json:"team"`
	Tier            string            `json:"tier"`
	FailureEligible *bool             `json:"failure_eligible,omitempty"`
	Placements      []placement       `json:"placements,omitempty"`
	SharedResources []string          `json:"shared_resources,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
}

type catalogDependency struct {
	From                  string   `json:"from"`
	To                    string   `json:"to"`
	Mode                  string   `json:"mode"`
	Blocking              bool     `json:"blocking"`
	Weight                *float64 `json:"weight,omitempty"`
	RequestTimeoutMS      *int     `json:"request_timeout_ms,omitempty"`
	RetryMaxAttempts      *int     `json:"retry_max_attempts,omitempty"`
	CircuitBreakerEnabled *bool    `json:"circuit_breaker_enabled,omitempty"`
	SourceEndpointID      string   `json:"source_endpoint_id,omitempty"`
	Method                string   `json:"method,omitempty"`
	Route                 string   `json:"route,omitempty"`
}

type catalogEndpoint struct {
	Service      string   `json:"service"`
	Method       string   `json:"method"`
	Path         string   `json:"path"`
	PredicateRef string   `json:"predicate_ref"`
	Weight       *float64 `json:"weight,omitempty"`
}

type topologyDocument struct {
	Source    topologySource     `json:"source"`
	Services  []topologyService  `json:"services"`
	Edges     []topologyEdge     `json:"edges"`
	Endpoints []topologyEndpoint `json:"endpoints"`
}

type topologySource struct {
	Type string `json:"type"`
	Ref  string `json:"ref,omitempty"`
}

type topologyService struct {
	ID                 string            `json:"id"`
	Replicas           int               `json:"replicas,omitempty"`
	FailureEligible    *bool             `json:"failure_eligible,omitempty"`
	Labels             map[string]string `json:"labels,omitempty"`
	Placements         []placement       `json:"placements,omitempty"`
	SharedResourceRefs []string          `json:"shared_resource_refs,omitempty"`
}

type placement struct {
	Replicas int               `json:"replicas"`
	Labels   map[string]string `json:"labels,omitempty"`
}

type topologyEdge struct {
	ID          string            `json:"id,omitempty"`
	From        string            `json:"from"`
	To          string            `json:"to"`
	Kind        string            `json:"kind"`
	Blocking    bool              `json:"blocking"`
	Weight      *float64          `json:"weight,omitempty"`
	Resilience  *resiliencePolicy `json:"resilience,omitempty"`
	PolicyScope *policyScope      `json:"policy_scope,omitempty"`
}

type resiliencePolicy struct {
	RequestTimeoutMS *int                  `json:"request_timeout_ms,omitempty"`
	Retry            *retryPolicy          `json:"retry,omitempty"`
	CircuitBreaker   *circuitBreakerPolicy `json:"circuit_breaker,omitempty"`
}

type retryPolicy struct {
	MaxAttempts *int `json:"max_attempts,omitempty"`
}

type circuitBreakerPolicy struct {
	Enabled *bool `json:"enabled,omitempty"`
}

type policyScope struct {
	SourceEndpointID string `json:"source_endpoint_id,omitempty"`
	Method           string `json:"method,omitempty"`
	SourceRoute      string `json:"source_route,omitempty"`
}

type topologyEndpoint struct {
	EntryService string   `json:"entry_service"`
	Method       string   `json:"method"`
	Path         string   `json:"path"`
	PredicateRef string   `json:"predicate_ref,omitempty"`
	Weight       *float64 `json:"weight,omitempty"`
}

func main() {
	input := flag.String("input", "", "Path to service catalog JSON")
	out := flag.String("out", "", "Path to write topology_api JSON; stdout when empty")
	flag.Parse()

	if strings.TrimSpace(*input) == "" {
		exitf("missing --input")
	}
	doc, err := loadCatalog(*input)
	if err != nil {
		exitf("%v", err)
	}
	topology := convert(doc)
	raw, err := json.MarshalIndent(topology, "", "  ")
	if err != nil {
		exitf("marshal topology_api json: %v", err)
	}
	raw = append(raw, '\n')
	if strings.TrimSpace(*out) == "" {
		_, _ = os.Stdout.Write(raw)
		return
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		exitf("create output directory: %v", err)
	}
	if err := os.WriteFile(*out, raw, 0o644); err != nil {
		exitf("write topology_api json: %v", err)
	}
}

func loadCatalog(path string) (catalog, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return catalog{}, fmt.Errorf("read catalog: %w", err)
	}
	var doc catalog
	if err := json.Unmarshal(raw, &doc); err != nil {
		return catalog{}, fmt.Errorf("decode catalog json: %w", err)
	}
	if len(doc.Services) == 0 {
		return catalog{}, fmt.Errorf("catalog.services cannot be empty")
	}
	return doc, nil
}

func convert(doc catalog) topologyDocument {
	out := topologyDocument{
		Source: topologySource{
			Type: "topology_api",
			Ref:  strings.TrimSpace(doc.SourceRef),
		},
		Services:  make([]topologyService, 0, len(doc.Services)),
		Edges:     make([]topologyEdge, 0, len(doc.Dependencies)),
		Endpoints: make([]topologyEndpoint, 0, len(doc.Endpoints)),
	}
	for _, service := range doc.Services {
		labels := cloneLabels(service.Labels)
		if strings.TrimSpace(service.Team) != "" {
			labels["team"] = strings.TrimSpace(service.Team)
		}
		if strings.TrimSpace(service.Tier) != "" {
			labels["tier"] = strings.TrimSpace(service.Tier)
		}
		out.Services = append(out.Services, topologyService{
			ID:                 strings.TrimSpace(service.ID),
			Replicas:           service.Replicas,
			FailureEligible:    service.FailureEligible,
			Labels:             nilIfEmpty(labels),
			Placements:         service.Placements,
			SharedResourceRefs: service.SharedResources,
		})
	}
	for _, dependency := range doc.Dependencies {
		edge := topologyEdge{
			ID:       edgeID(dependency),
			From:     strings.TrimSpace(dependency.From),
			To:       strings.TrimSpace(dependency.To),
			Kind:     normalizeKind(dependency.Mode),
			Blocking: dependency.Blocking,
			Weight:   dependency.Weight,
		}
		edge.Resilience = buildResilience(dependency)
		edge.PolicyScope = buildPolicyScope(dependency)
		out.Edges = append(out.Edges, edge)
	}
	for _, endpoint := range doc.Endpoints {
		out.Endpoints = append(out.Endpoints, topologyEndpoint{
			EntryService: strings.TrimSpace(endpoint.Service),
			Method:       strings.ToUpper(strings.TrimSpace(endpoint.Method)),
			Path:         normalizePath(endpoint.Path),
			PredicateRef: strings.TrimSpace(endpoint.PredicateRef),
			Weight:       endpoint.Weight,
		})
	}
	return out
}

func buildResilience(dependency catalogDependency) *resiliencePolicy {
	policy := &resiliencePolicy{RequestTimeoutMS: dependency.RequestTimeoutMS}
	if dependency.RetryMaxAttempts != nil {
		policy.Retry = &retryPolicy{MaxAttempts: dependency.RetryMaxAttempts}
	}
	if dependency.CircuitBreakerEnabled != nil {
		policy.CircuitBreaker = &circuitBreakerPolicy{Enabled: dependency.CircuitBreakerEnabled}
	}
	if policy.RequestTimeoutMS == nil && policy.Retry == nil && policy.CircuitBreaker == nil {
		return nil
	}
	return policy
}

func buildPolicyScope(dependency catalogDependency) *policyScope {
	scope := &policyScope{
		SourceEndpointID: strings.TrimSpace(dependency.SourceEndpointID),
		Method:           strings.ToUpper(strings.TrimSpace(dependency.Method)),
		SourceRoute:      normalizePath(dependency.Route),
	}
	if scope.SourceEndpointID == "" && scope.Method == "" && scope.SourceRoute == "" {
		return nil
	}
	return scope
}

func cloneLabels(labels map[string]string) map[string]string {
	out := make(map[string]string, len(labels)+2)
	for key, value := range labels {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	return out
}

func nilIfEmpty(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	return labels
}

func normalizeKind(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "async":
		return "async"
	default:
		return "sync"
	}
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "/") {
		return path
	}
	return "/" + path
}

func edgeID(dependency catalogDependency) string {
	kind := normalizeKind(dependency.Mode)
	return fmt.Sprintf("%s|%s|%s|%t", strings.TrimSpace(dependency.From), strings.TrimSpace(dependency.To), kind, dependency.Blocking)
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
