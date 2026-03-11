package topology

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFileYAMLNormalizesDefaults(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "topology.yaml")
	raw := []byte(`services:
  - id: frontend
edges:
  - from: frontend
    to: frontend
    kind: sync
endpoints:
  - entry_service: frontend
    method: get
    path: health
`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write topology fixture: %v", err)
	}

	doc, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile returned error: %v", err)
	}
	if got, want := doc.Source.Type, SourceTypeTopologyAPI; got != want {
		t.Fatalf("source type mismatch: got=%s want=%s", got, want)
	}
	if got, want := doc.Services[0].Name, "frontend"; got != want {
		t.Fatalf("service name mismatch: got=%s want=%s", got, want)
	}
	if got, want := *doc.Services[0].Replicas, 1; got != want {
		t.Fatalf("replicas mismatch: got=%d want=%d", got, want)
	}
	if got, want := doc.Services[0].Support.Observations, 1; got != want {
		t.Fatalf("service support mismatch: got=%d want=%d", got, want)
	}
	if got, want := doc.Edges[0].ID, "frontend|frontend|sync|true"; got != want {
		t.Fatalf("edge id mismatch: got=%s want=%s", got, want)
	}
	if got, want := *doc.Edges[0].Blocking, true; got != want {
		t.Fatalf("edge blocking mismatch: got=%t want=%t", got, want)
	}
	if got, want := doc.Endpoints[0].ID, "frontend:GET /health"; got != want {
		t.Fatalf("endpoint id mismatch: got=%s want=%s", got, want)
	}
	if got, want := doc.Endpoints[0].PredicateRef, "frontend:GET /health"; got != want {
		t.Fatalf("endpoint predicate mismatch: got=%s want=%s", got, want)
	}
	if got, want := doc.Endpoints[0].Support.Evidence[0], SourceTypeTopologyAPI; got != want {
		t.Fatalf("endpoint evidence mismatch: got=%s want=%s", got, want)
	}
}
