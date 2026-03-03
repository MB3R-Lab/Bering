package traces

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_Normalized(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "trace.json")
	raw := `{
  "spans": [
    {
      "trace_id": "t1",
      "span_id": "s1",
      "service": "frontend",
      "kind": "server",
      "name": "GET /checkout",
      "attributes": {
        "http.request.method": "GET",
        "http.route": "/checkout"
      }
    }
  ]
}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	spans, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got, want := len(spans), 1; got != want {
		t.Fatalf("span count mismatch: got=%d want=%d", got, want)
	}
	if got := spans[0].Service; got != "frontend" {
		t.Fatalf("service mismatch: got=%s", got)
	}
}

func TestLoad_OTLP(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "trace.json")
	raw := `{
  "resourceSpans": [
    {
      "resource": {
        "attributes": [
          { "key": "service.name", "value": { "stringValue": "frontend" } }
        ]
      },
      "scopeSpans": [
        {
          "spans": [
            {
              "traceId": "abc",
              "spanId": "def",
              "name": "GET /health",
              "kind": 2,
              "attributes": [
                { "key": "http.request.method", "value": { "stringValue": "GET" } },
                { "key": "http.route", "value": { "stringValue": "/health" } }
              ]
            }
          ]
        }
      ]
    }
  ]
}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	spans, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got, want := len(spans), 1; got != want {
		t.Fatalf("span count mismatch: got=%d want=%d", got, want)
	}
	if got := spans[0].Kind; got != "server" {
		t.Fatalf("kind mismatch: got=%s", got)
	}
}

func TestLoadReplicasOverride(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "replicas.json")
	yamlPath := filepath.Join(dir, "replicas.yaml")

	if err := os.WriteFile(jsonPath, []byte(`{"frontend":2,"checkout":3}`), 0o644); err != nil {
		t.Fatalf("write json fixture: %v", err)
	}
	if err := os.WriteFile(yamlPath, []byte("replicas:\n  frontend: 2\n  checkout: 3\n"), 0o644); err != nil {
		t.Fatalf("write yaml fixture: %v", err)
	}

	fromJSON, err := LoadReplicasOverride(jsonPath)
	if err != nil {
		t.Fatalf("LoadReplicasOverride(json) error: %v", err)
	}
	if fromJSON["frontend"] != 2 || fromJSON["checkout"] != 3 {
		t.Fatalf("unexpected json replicas: %+v", fromJSON)
	}

	fromYAML, err := LoadReplicasOverride(yamlPath)
	if err != nil {
		t.Fatalf("LoadReplicasOverride(yaml) error: %v", err)
	}
	if fromYAML["frontend"] != 2 || fromYAML["checkout"] != 3 {
		t.Fatalf("unexpected yaml replicas: %+v", fromYAML)
	}
}
