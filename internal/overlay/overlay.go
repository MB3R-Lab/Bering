package overlay

import (
	"fmt"
	"path/filepath"
	"strings"
)

type File struct {
	Name      string            `json:"name" yaml:"name"`
	Services  []ServiceOverlay  `json:"services" yaml:"services"`
	Edges     []EdgeOverlay     `json:"edges" yaml:"edges"`
	Endpoints []EndpointOverlay `json:"endpoints" yaml:"endpoints"`
	Ref       string            `json:"-" yaml:"-"`
}

type CommonMetadata struct {
	Labels     map[string]string `json:"labels" yaml:"labels"`
	Tags       []string          `json:"tags" yaml:"tags"`
	SLORefs    []string          `json:"slo_refs" yaml:"slo_refs"`
	Attributes map[string]string `json:"attributes" yaml:"attributes"`
}

type ServiceOverlay struct {
	ID              string `json:"id" yaml:"id"`
	Replicas        *int   `json:"replicas" yaml:"replicas"`
	FailureEligible *bool  `json:"failure_eligible" yaml:"failure_eligible"`
	CommonMetadata  `json:",inline" yaml:",inline"`
}

type EdgeOverlay struct {
	ID             string   `json:"id" yaml:"id"`
	From           string   `json:"from" yaml:"from"`
	To             string   `json:"to" yaml:"to"`
	Kind           string   `json:"kind" yaml:"kind"`
	Blocking       *bool    `json:"blocking" yaml:"blocking"`
	Weight         *float64 `json:"weight" yaml:"weight"`
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
