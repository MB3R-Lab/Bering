package model

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type EdgeKind string

const (
	EdgeKindSync  EdgeKind = "sync"
	EdgeKindAsync EdgeKind = "async"
)

type Service struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Replicas int    `json:"replicas"`
}

type Edge struct {
	From     string   `json:"from"`
	To       string   `json:"to"`
	Kind     EdgeKind `json:"kind"`
	Blocking bool     `json:"blocking"`
}

type Endpoint struct {
	ID                  string `json:"id"`
	EntryService        string `json:"entry_service"`
	SuccessPredicateRef string `json:"success_predicate_ref"`
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

type ResilienceModel struct {
	Services  []Service  `json:"services"`
	Edges     []Edge     `json:"edges"`
	Endpoints []Endpoint `json:"endpoints"`
	Metadata  Metadata   `json:"metadata"`
}

func (m *ResilienceModel) SortDeterministic() {
	sort.Slice(m.Services, func(i, j int) bool {
		return m.Services[i].ID < m.Services[j].ID
	})

	sort.Slice(m.Edges, func(i, j int) bool {
		left, right := m.Edges[i], m.Edges[j]
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
		serviceSet[svc.ID] = struct{}{}
	}

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
