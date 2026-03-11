package topology

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	SourceTypeTopologyAPI = "topology_api"
	ConnectorName         = "topology_file"
)

type Document struct {
	Source    Source     `json:"source" yaml:"source"`
	Services  []Service  `json:"services" yaml:"services"`
	Edges     []Edge     `json:"edges" yaml:"edges"`
	Endpoints []Endpoint `json:"endpoints" yaml:"endpoints"`
}

type Source struct {
	Type string `json:"type" yaml:"type"`
	Ref  string `json:"ref" yaml:"ref"`
}

type Support struct {
	Observations int      `json:"observations" yaml:"observations"`
	Evidence     []string `json:"evidence" yaml:"evidence"`
}

type CommonMetadata struct {
	Labels     map[string]string `json:"labels" yaml:"labels"`
	Tags       []string          `json:"tags" yaml:"tags"`
	SLORefs    []string          `json:"slo_refs" yaml:"slo_refs"`
	Attributes map[string]string `json:"attributes" yaml:"attributes"`
}

type Service struct {
	ID              string  `json:"id" yaml:"id"`
	Name            string  `json:"name" yaml:"name"`
	Replicas        *int    `json:"replicas" yaml:"replicas"`
	FirstSeen       string  `json:"first_seen" yaml:"first_seen"`
	LastSeen        string  `json:"last_seen" yaml:"last_seen"`
	FailureEligible *bool   `json:"failure_eligible" yaml:"failure_eligible"`
	Support         Support `json:"support" yaml:"support"`
	CommonMetadata  `json:",inline" yaml:",inline"`
}

type Edge struct {
	ID             string   `json:"id" yaml:"id"`
	From           string   `json:"from" yaml:"from"`
	To             string   `json:"to" yaml:"to"`
	Kind           string   `json:"kind" yaml:"kind"`
	Blocking       *bool    `json:"blocking" yaml:"blocking"`
	FirstSeen      string   `json:"first_seen" yaml:"first_seen"`
	LastSeen       string   `json:"last_seen" yaml:"last_seen"`
	Weight         *float64 `json:"weight" yaml:"weight"`
	Support        Support  `json:"support" yaml:"support"`
	CommonMetadata `json:",inline" yaml:",inline"`
}

type Endpoint struct {
	ID             string   `json:"id" yaml:"id"`
	EntryService   string   `json:"entry_service" yaml:"entry_service"`
	Method         string   `json:"method" yaml:"method"`
	Path           string   `json:"path" yaml:"path"`
	PredicateRef   string   `json:"predicate_ref" yaml:"predicate_ref"`
	FirstSeen      string   `json:"first_seen" yaml:"first_seen"`
	LastSeen       string   `json:"last_seen" yaml:"last_seen"`
	Weight         *float64 `json:"weight" yaml:"weight"`
	Support        Support  `json:"support" yaml:"support"`
	CommonMetadata `json:",inline" yaml:",inline"`
}

func LoadFile(path string) (Document, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Document{}, fmt.Errorf("read topology input: %w", err)
	}

	var doc Document
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&doc); err != nil {
			return Document{}, fmt.Errorf("decode topology json: %w", err)
		}
	default:
		dec := yaml.NewDecoder(bytes.NewReader(raw))
		dec.KnownFields(true)
		if err := dec.Decode(&doc); err != nil {
			return Document{}, fmt.Errorf("decode topology yaml: %w", err)
		}
	}

	if err := doc.Normalize(); err != nil {
		return Document{}, err
	}
	return doc, nil
}

func (d *Document) Normalize() error {
	if d == nil {
		return fmt.Errorf("topology document cannot be nil")
	}

	d.Source.Type = strings.TrimSpace(strings.ToLower(d.Source.Type))
	if d.Source.Type == "" {
		d.Source.Type = SourceTypeTopologyAPI
	}
	d.Source.Ref = strings.TrimSpace(d.Source.Ref)
	if len(d.Services) == 0 {
		return fmt.Errorf("topology document must contain at least one service")
	}

	serviceIDs := map[string]struct{}{}
	for i := range d.Services {
		item := &d.Services[i]
		item.ID = strings.TrimSpace(item.ID)
		if item.ID == "" {
			return fmt.Errorf("service at index %d has empty id", i)
		}
		if _, exists := serviceIDs[item.ID]; exists {
			return fmt.Errorf("duplicate service id: %s", item.ID)
		}
		serviceIDs[item.ID] = struct{}{}
		item.Name = strings.TrimSpace(item.Name)
		if item.Name == "" {
			item.Name = item.ID
		}
		if item.Replicas == nil {
			replicas := 1
			item.Replicas = &replicas
		}
		if *item.Replicas < 0 {
			return fmt.Errorf("service %q replicas must be >= 0", item.ID)
		}
		if err := normalizeSupport(&item.Support, d.Source.Type); err != nil {
			return fmt.Errorf("service %q support: %w", item.ID, err)
		}
		if err := validateSeenRange(item.FirstSeen, item.LastSeen); err != nil {
			return fmt.Errorf("service %q seen range: %w", item.ID, err)
		}
		normalizeCommonMetadata(&item.CommonMetadata)
	}

	edgeIDs := map[string]struct{}{}
	for i := range d.Edges {
		item := &d.Edges[i]
		item.From = strings.TrimSpace(item.From)
		item.To = strings.TrimSpace(item.To)
		item.Kind = strings.TrimSpace(strings.ToLower(item.Kind))
		if item.From == "" || item.To == "" {
			return fmt.Errorf("edge at index %d requires from and to", i)
		}
		if item.Kind == "" {
			return fmt.Errorf("edge %q->%q requires kind", item.From, item.To)
		}
		switch item.Kind {
		case "sync", "async":
		default:
			return fmt.Errorf("edge %q->%q has unsupported kind %q", item.From, item.To, item.Kind)
		}
		if _, ok := serviceIDs[item.From]; !ok {
			return fmt.Errorf("edge %q->%q references unknown from service %q", item.From, item.To, item.From)
		}
		if _, ok := serviceIDs[item.To]; !ok {
			return fmt.Errorf("edge %q->%q references unknown to service %q", item.From, item.To, item.To)
		}
		if item.Blocking == nil {
			blocking := item.Kind == "sync"
			item.Blocking = &blocking
		}
		item.ID = strings.TrimSpace(item.ID)
		if item.ID == "" {
			item.ID = fmt.Sprintf("%s|%s|%s|%t", item.From, item.To, item.Kind, *item.Blocking)
		}
		if _, exists := edgeIDs[item.ID]; exists {
			return fmt.Errorf("duplicate edge id: %s", item.ID)
		}
		edgeIDs[item.ID] = struct{}{}
		if err := normalizeSupport(&item.Support, d.Source.Type); err != nil {
			return fmt.Errorf("edge %q support: %w", item.ID, err)
		}
		if err := validateSeenRange(item.FirstSeen, item.LastSeen); err != nil {
			return fmt.Errorf("edge %q seen range: %w", item.ID, err)
		}
		normalizeCommonMetadata(&item.CommonMetadata)
	}

	endpointIDs := map[string]struct{}{}
	for i := range d.Endpoints {
		item := &d.Endpoints[i]
		item.EntryService = strings.TrimSpace(item.EntryService)
		item.Method = strings.ToUpper(strings.TrimSpace(item.Method))
		item.Path = normalizePath(item.Path)
		item.ID = strings.TrimSpace(item.ID)
		item.PredicateRef = strings.TrimSpace(item.PredicateRef)
		if item.EntryService == "" {
			return fmt.Errorf("endpoint at index %d requires entry_service", i)
		}
		if _, ok := serviceIDs[item.EntryService]; !ok {
			return fmt.Errorf("endpoint at index %d references unknown entry service %q", i, item.EntryService)
		}
		if item.ID == "" {
			if item.Method == "" || item.Path == "" {
				return fmt.Errorf("endpoint at index %d requires id or entry_service/method/path", i)
			}
			item.ID = fmt.Sprintf("%s:%s %s", item.EntryService, item.Method, item.Path)
		}
		if _, exists := endpointIDs[item.ID]; exists {
			return fmt.Errorf("duplicate endpoint id: %s", item.ID)
		}
		endpointIDs[item.ID] = struct{}{}
		if item.PredicateRef == "" {
			item.PredicateRef = item.ID
		}
		if err := normalizeSupport(&item.Support, d.Source.Type); err != nil {
			return fmt.Errorf("endpoint %q support: %w", item.ID, err)
		}
		if err := validateSeenRange(item.FirstSeen, item.LastSeen); err != nil {
			return fmt.Errorf("endpoint %q seen range: %w", item.ID, err)
		}
		normalizeCommonMetadata(&item.CommonMetadata)
	}

	return nil
}

func normalizeSupport(support *Support, defaultEvidence string) error {
	if support == nil {
		return nil
	}
	if support.Observations < 0 {
		return fmt.Errorf("observations must be >= 0")
	}
	if support.Observations == 0 {
		support.Observations = 1
	}
	support.Evidence = dedupeNonEmpty(support.Evidence)
	if len(support.Evidence) == 0 && strings.TrimSpace(defaultEvidence) != "" {
		support.Evidence = []string{strings.TrimSpace(defaultEvidence)}
	}
	return nil
}

func validateSeenRange(firstSeen, lastSeen string) error {
	first, err := parseOptionalRFC3339(firstSeen)
	if err != nil {
		return fmt.Errorf("first_seen: %w", err)
	}
	last, err := parseOptionalRFC3339(lastSeen)
	if err != nil {
		return fmt.Errorf("last_seen: %w", err)
	}
	if !first.IsZero() && !last.IsZero() && first.After(last) {
		return fmt.Errorf("first_seen must be <= last_seen")
	}
	return nil
}

func parseOptionalRFC3339(value string) (time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return time.Time{}, fmt.Errorf("must be RFC3339: %w", err)
	}
	return parsed.UTC(), nil
}

func normalizeCommonMetadata(meta *CommonMetadata) {
	if meta == nil {
		return
	}
	meta.Tags = dedupeNonEmpty(meta.Tags)
	meta.SLORefs = dedupeNonEmpty(meta.SLORefs)
	meta.Labels = trimStringMap(meta.Labels)
	meta.Attributes = trimStringMap(meta.Attributes)
}

func trimStringMap(values map[string]string) map[string]string {
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

func dedupeNonEmpty(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizePath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	if !strings.HasPrefix(trimmed, "/") {
		return "/" + trimmed
	}
	return trimmed
}
