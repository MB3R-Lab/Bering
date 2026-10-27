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
	if got := spans[0].SourceRef; got != path {
		t.Fatalf("source ref mismatch: got=%q want=%q", got, path)
	}
}

func TestLoad_NormalizedMetadataAndMergedAttributes(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "spans.json")
	raw := `{"spans":[{"trace_id":"abc","span_id":"01","source_ref":"embedded-hint","service":"span-alias","scope_name":"http.instrumentation","scope_version":"1.2.3","resource_attributes":{"service.name":"api","host.name":"node-a","shared":"resource"},"attributes":{"http.route":"/items","shared":"span"}}]}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	spans, err := Load(path)
	if err != nil || len(spans) != 1 {
		t.Fatalf("Load: spans=%d err=%v", len(spans), err)
	}
	span := spans[0]
	if span.SourceRef != path || span.ScopeName != "http.instrumentation" || span.ScopeVersion != "1.2.3" {
		t.Fatalf("metadata mismatch: %+v", span)
	}
	if span.Service != "api" {
		t.Fatalf("resource service should take precedence: %q", span.Service)
	}
	if span.ResourceAttributes["host.name"] != "node-a" || span.ResourceAttributes["shared"] != "resource" {
		t.Fatalf("resource attributes mismatch: %+v", span.ResourceAttributes)
	}
	if span.Attributes["shared"] != "span" || span.Attributes["host.name"] != "node-a" || span.Attributes["http.route"] != "/items" {
		t.Fatalf("merged attributes mismatch: %+v", span.Attributes)
	}
	span.Attributes["host.name"] = "changed"
	if span.ResourceAttributes["host.name"] != "node-a" {
		t.Fatal("merged attributes unexpectedly alias resource attributes")
	}
}

func TestParseNormalizedSourceRefHint(t *testing.T) {
	t.Parallel()
	spans, err := parseTracePayload([]byte(`{"spans":[{"trace_id":"abc","span_id":"01","source_ref":"file-origin"}]}`))
	if err != nil || len(spans) != 1 {
		t.Fatalf("parse: spans=%d err=%v", len(spans), err)
	}
	if spans[0].SourceRef != "file-origin" {
		t.Fatalf("source_ref hint lost: %q", spans[0].SourceRef)
	}
}

func TestRawOTLPIdentifiersAndKinds(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, input, want string
		size              int
	}{
		{"trace base64", "qrvM3e7/AQIDBAUGBwgJCg==", "aabbccddeeff0102030405060708090a", 16},
		{"span base64", "AQIDBAUGBwg=", "0102030405060708", 8},
		{"hex canonicalized", "AABBCCDDEEFF0102030405060708090A", "aabbccddeeff0102030405060708090a", 16},
		{"opaque raw rejected", "fixture-span", "", 8},
		{"short base64 rejected", "qrs=", "", 16},
		{"short hex rejected", "aabb", "", 16},
		{"overlong hex rejected", "aabbccddeeff0102030405060708090a11", "", 16},
		{"malformed rejected", "not-an-otlp-id!", "", 8},
		{"zero trace hex absent", "00000000000000000000000000000000", "", 16},
		{"zero trace base64 absent", "AAAAAAAAAAAAAAAAAAAAAA==", "", 16},
		{"zero span hex absent", "0000000000000000", "", 8},
		{"zero span base64 absent", "AAAAAAAAAAA=", "", 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeOTLPID(tc.input, tc.size); got != tc.want {
				t.Fatalf("id mismatch: got=%q want=%q", got, tc.want)
			}
		})
	}
	if got := mapSpanKind("SPAN_KIND_SERVER"); got != "server" {
		t.Fatalf("enum kind mismatch: %q", got)
	}
}

func TestRawOTLPZeroIDsAbsentNormalizedOpaquePreserved(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"api"}}]},"scopeSpans":[{"spans":[{"traceId":"AAAAAAAAAAAAAAAAAAAAAA==","spanId":"0000000000000000","parentSpanId":"AAAAAAAAAAA=","name":"request","kind":2}]}]}]}`)
	spans, err := parseTracePayload(raw)
	if err != nil || len(spans) != 1 {
		t.Fatalf("parse raw OTLP: spans=%d err=%v", len(spans), err)
	}
	if spans[0].TraceID != "" || spans[0].SpanID != "" || spans[0].ParentSpanID != "" {
		t.Fatalf("all-zero OTLP IDs must be absent: %+v", spans[0])
	}
	normalized, err := parseTracePayload([]byte(`{"spans":[{"trace_id":"00000000000000000000000000000000","span_id":"0000000000000000","parent_span_id":"0000000000000000"}]}`))
	if err != nil || len(normalized) != 1 {
		t.Fatalf("parse normalized: spans=%d err=%v", len(normalized), err)
	}
	if normalized[0].TraceID == "" || normalized[0].SpanID == "" || normalized[0].ParentSpanID == "" {
		t.Fatalf("normalized opaque IDs must remain untouched: %+v", normalized[0])
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
				"traceId": "00000000000000000000000000000001",
				"spanId": "0000000000000001",
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
