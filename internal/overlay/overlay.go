package overlay

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/MB3R-Lab/Bering/internal/model"
)

type File struct {
	Name      string            `json:"name" yaml:"name"`
	Services  []ServiceOverlay  `json:"services" yaml:"services"`
	Edges     []EdgeOverlay     `json:"edges" yaml:"edges"`
	Endpoints []EndpointOverlay `json:"endpoints" yaml:"endpoints"`
	Ref       string            `json:"-" yaml:"-"`
}

type CommonMetadata struct {
	model.CommonMetadata `json:",inline" yaml:",inline"`
	Attributes           map[string]string `json:"attributes" yaml:"attributes"`
}

type ServiceOverlay struct {
	ID                 string            `json:"id" yaml:"id"`
	Replicas           *int              `json:"replicas" yaml:"replicas"`
	FailureEligible    *bool             `json:"failure_eligible" yaml:"failure_eligible"`
	Placements         []model.Placement `json:"placements" yaml:"placements"`
	SharedResourceRefs []string          `json:"shared_resource_refs" yaml:"shared_resource_refs"`
	CommonMetadata     `json:",inline" yaml:",inline"`
}

type EdgeOverlay struct {
	ID             string                  `json:"id" yaml:"id"`
	From           string                  `json:"from" yaml:"from"`
	To             string                  `json:"to" yaml:"to"`
	Kind           string                  `json:"kind" yaml:"kind"`
	Blocking       *bool                   `json:"blocking" yaml:"blocking"`
	Weight         *float64                `json:"weight" yaml:"weight"`
	Resilience     *model.ResiliencePolicy `json:"resilience" yaml:"resilience"`
	Observed       *model.ObservedEdge     `json:"observed" yaml:"observed"`
	PolicyScope    *model.PolicyScope      `json:"policy_scope" yaml:"policy_scope"`
	CommonMetadata `json:",inline" yaml:",inline"`
}

type EndpointOverlay struct {
	ID             string   `json:"id" yaml:"id"`
	EntryService   string   `json:"entry_service" yaml:"entry_service"`
	Method         string   `json:"method" yaml:"method"`
	Path           string   `json:"path" yaml:"path"`
	PredicateRef   string   `json:"predicate_ref" yaml:"predicate_ref"`
	Weight         *float64 `json:"weight" yaml:"weight"`
	CommonMetadata `json:",inline" yaml:",inline"`
}

func (f *File) Normalize(path string) error {
	f.Ref = filepath.ToSlash(filepath.Clean(path))
	f.Name = strings.TrimSpace(f.Name)
	if f.Name == "" {
		f.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	for i := range f.Services {
		f.Services[i].ID = strings.TrimSpace(f.Services[i].ID)
		if f.Services[i].ID == "" {
			return fmt.Errorf("service overlay at index %d has empty id", i)
		}
		normalizeCommonMetadata(&f.Services[i].CommonMetadata)
		normalizePlacements(f.Services[i].Placements)
		f.Services[i].SharedResourceRefs = dedupeNonEmpty(f.Services[i].SharedResourceRefs)
	}
	for i := range f.Edges {
		item := &f.Edges[i]
		item.ID = strings.TrimSpace(item.ID)
		item.From = strings.TrimSpace(item.From)
		item.To = strings.TrimSpace(item.To)
		item.Kind = strings.TrimSpace(item.Kind)
		if item.ID == "" {
			if item.From == "" || item.To == "" || item.Kind == "" || item.Blocking == nil {
				return fmt.Errorf("edge overlay at index %d requires id or from/to/kind/blocking", i)
			}
			item.ID = fmt.Sprintf("%s|%s|%s|%t", item.From, item.To, item.Kind, *item.Blocking)
		}
		normalizeCommonMetadata(&item.CommonMetadata)
		if item.Resilience != nil {
			item.Resilience.Normalize()
		}
		if item.Observed != nil {
			item.Observed.Normalize()
		}
		if item.PolicyScope != nil {
			item.PolicyScope.Normalize()
		}
	}
	for i := range f.Endpoints {
		item := &f.Endpoints[i]
		item.ID = strings.TrimSpace(item.ID)
		item.EntryService = strings.TrimSpace(item.EntryService)
		item.Method = strings.ToUpper(strings.TrimSpace(item.Method))
		item.Path = normalizePath(item.Path)
		item.PredicateRef = strings.TrimSpace(item.PredicateRef)
		if item.ID == "" {
			if item.EntryService == "" || item.Method == "" || item.Path == "" {
				return fmt.Errorf("endpoint overlay at index %d requires id or entry_service/method/path", i)
			}
			item.ID = fmt.Sprintf("%s:%s %s", item.EntryService, item.Method, item.Path)
		}
		normalizeCommonMetadata(&item.CommonMetadata)
	}
	return nil
}

func FromReplicas(values map[string]int, ref string) File {
	out := File{Name: "replicas", Ref: ref}
	for id, replicas := range values {
		value := replicas
		out.Services = append(out.Services, ServiceOverlay{ID: id, Replicas: &value})
	}
	return out
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

func normalizeCommonMetadata(meta *CommonMetadata) {
	if meta == nil {
		return
	}
	meta.CommonMetadata.Normalize()
	meta.Attributes = trimStringMap(meta.Attributes)
}

func normalizePlacements(items []model.Placement) {
	for i := range items {
		items[i].Labels = trimStringMap(items[i].Labels)
	}
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
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
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
	return out
}
