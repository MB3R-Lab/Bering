package ebpf

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	collecttracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"

	"github.com/MB3R-Lab/Bering/internal/connectors/traces"
)

const validTrace = "00112233445566778899aabbccddeeff"
const validSpan = "1122334455667788"

func exportJSON(traceID, spanID string) []byte {
	return []byte(fmt.Sprintf(`{"resourceSpans":[{"resource":{"attributes":[
		{"key":"service.name","value":{"stringValue":"gateway"}},
		{"key":"service.instance.id","value":{"stringValue":"gateway-a"}},
		{"key":"host.id","value":{"stringValue":"host-a"}},
		{"key":"process.pid","value":{"intValue":"17"}}
	]},"scopeSpans":[{"scope":{"name":"obi","version":"1.2"},"spans":[{
		"traceId":"%s","spanId":"%s","name":"GET /items",
		"kind":"SPAN_KIND_SERVER","startTimeUnixNano":"1000000000",
		"endTimeUnixNano":"2000000000","attributes":[
			{"key":"http.request.method","value":{"stringValue":"GET"}}
		]
	}]}]}]}`, traceID, spanID))
}

func writeFile(t *testing.T, name string, body []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func compactJSON(t *testing.T, body []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := json.Compact(&out, body); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func gzipBytes(t *testing.T, body []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	writer := gzip.NewWriter(&out)
	if _, err := writer.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestLoadBareJSONPreservesOTLPEvidence(t *testing.T) {
	path := writeFile(t, "export.json", exportJSON(validTrace, validSpan))
	result, err := Load(path, Options{Collector: "obi", Domain: "cluster-a"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Spans) != 1 || len(result.Sources) != 1 {
		t.Fatalf("unexpected result counts: %+v", result)
	}
	span := result.Spans[0]
	if span.TraceID != validTrace || span.SpanID != validSpan || span.Service != "gateway" || span.Kind != "server" {
		t.Fatalf("identity was altered: %+v", span)
	}
	if span.ScopeName != "obi" || span.ScopeVersion != "1.2" || span.SourceKind != "ebpf" || span.SourceDomain != "ebpf:cluster-a" || span.SourceRef != path {
		t.Fatalf("scope or declared provenance was lost: %+v", span)
	}
	if span.ResourceAttributes["host.id"] != "host-a" || span.ResourceAttributes["service.instance.id"] != "gateway-a" || span.ResourceAttributes["process.pid"] != int64(17) {
		t.Fatalf("resource identity was lost: %+v", span.ResourceAttributes)
	}
	if source := result.Sources[0]; source.Type != "ebpf" || source.Connector != "obi" || source.Ref != path || source.Observations != 1 {
		t.Fatalf("source summary mismatch: %+v", source)
	}
}

func TestJSONLSuccessiveExportsAndTruncatedRecord(t *testing.T) {
	first := compactJSON(t, exportJSON(validTrace, validSpan))
	second := compactJSON(t, exportJSON("ffeeddccbbaa99887766554433221100", "8877665544332211"))
	path := writeFile(t, "exports.jsonl", append(append(append([]byte{}, first...), '\n'), second...))
	result, err := Load(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Spans) != 2 || result.Sources[0].Observations != 2 {
		t.Fatalf("JSONL exports not combined: %+v", result)
	}
	if result.Spans[0].SourceDomain != "ebpf:default" || result.Spans[1].SourceDomain != "ebpf:default" {
		t.Fatalf("default domain missing: %+v", result.Spans)
	}
	if err := os.WriteFile(path, append(append(append([]byte{}, first...), '\n'), []byte(`{"resourceSpans":[`)...), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path, Options{}); err == nil || !strings.Contains(err.Error(), "JSONL record 2") {
		t.Fatalf("truncated export was accepted or lacked line provenance: %v", err)
	}
}

func binaryExport(t *testing.T) []byte {
	t.Helper()
	request := &collecttracev1.ExportTraceServiceRequest{ResourceSpans: []*tracev1.ResourceSpans{{
		Resource: &resourcev1.Resource{Attributes: []*commonv1.KeyValue{{
			Key: "service.name", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "gateway"}},
		}}},
		ScopeSpans: []*tracev1.ScopeSpans{{Spans: []*tracev1.Span{{
			TraceId: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			SpanId:  []byte{1, 2, 3, 4, 5, 6, 7, 8},
			Name:    "GET /items", Kind: tracev1.Span_SPAN_KIND_SERVER,
		}}}},
	}}}
	body, err := proto.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestBinaryProtobufAndSortedDirectory(t *testing.T) {
	body := binaryExport(t)
	dir := t.TempDir()
	for name, data := range map[string][]byte{
		"b.pb": body, "a.json": exportJSON(validTrace, validSpan),
		"metadata.log": []byte("not OTLP"), "metadata.log.gz": gzipBytes(t, []byte("not OTLP")),
	} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := Load(dir, Options{Domain: "installation"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Spans) != 2 || len(result.Sources) != 2 || result.Sources[0].Ref != filepath.Join(dir, "a.json") || result.Sources[1].Ref != filepath.Join(dir, "b.pb") {
		t.Fatalf("directory order or protobuf decode mismatch: %+v", result)
	}
	if result.Spans[1].TraceID != "0102030405060708090a0b0c0d0e0f10" {
		t.Fatalf("protobuf ID changed: %+v", result.Spans[1])
	}
}

func TestGzipJSONJSONLAndProtobuf(t *testing.T) {
	jsonDoc := exportJSON(validTrace, validSpan)
	jsonlDoc := append(compactJSON(t, jsonDoc), '\n')
	for name, body := range map[string][]byte{
		"export.json.gz":  gzipBytes(t, jsonDoc),
		"export.jsonl.gz": gzipBytes(t, jsonlDoc),
		"export.pb.gz":    gzipBytes(t, binaryExport(t)),
	} {
		path := writeFile(t, name, body)
		result, err := Load(path, Options{})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(result.Spans) != 1 || result.Spans[0].SourceRef != path {
			t.Fatalf("%s: decoded span or source missing: %+v", name, result)
		}
	}
}

func TestGzipTruncationAndDecompressedBounds(t *testing.T) {
	compressed := gzipBytes(t, exportJSON(validTrace, validSpan))
	path := writeFile(t, "truncated.json.gz", compressed[:len(compressed)-5])
	if _, err := Load(path, Options{}); err == nil {
		t.Fatal("truncated gzip footer accepted")
	}

	path = writeFile(t, "large.json.gz", compressed)
	if _, err := loadFileLimited(path, 100, maxJSONLRecordBytes); err == nil || !strings.Contains(err.Error(), "decompressed OTLP document exceeds") {
		t.Fatalf("oversized decompressed document accepted: %v", err)
	}

	line := append(compactJSON(t, exportJSON(validTrace, validSpan)), '\n')
	path = writeFile(t, "large.jsonl.gz", gzipBytes(t, line))
	if _, err := loadFileLimited(path, maxDocumentBytes, 100); err == nil || !strings.Contains(err.Error(), "JSONL record 1") {
		t.Fatalf("oversized decompressed JSONL record accepted: %v", err)
	}
}

func TestMalformedIDsAndConflictingResourceKeysRemainConservative(t *testing.T) {
	path := writeFile(t, "bad-ids.json", exportJSON("abcd", "00"))
	result, err := Load(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Spans[0].TraceID != "" || result.Spans[0].SpanID != "" {
		t.Fatalf("malformed OTLP IDs were certified: %+v", result.Spans[0])
	}

	conflict := strings.Replace(string(exportJSON(validTrace, validSpan)),
		`{"key":"service.name","value":{"stringValue":"gateway"}},`,
		`{"key":"service.name","value":{"stringValue":"gateway"}},{"key":"service.name","value":{"stringValue":"other"}},`, 1)
	path = writeFile(t, "conflict.json", []byte(conflict))
	result, err = Load(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	span := result.Spans[0]
	if span.Service != "" || span.ResourceAttributes["service.name"] != nil || !reflect.DeepEqual(span.AttributeConflicts, []string{"resource:service.name"}) {
		t.Fatalf("conflicting resource identity was repaired or hidden: %+v", span)
	}
}

func TestNormalizeValidatesDeclarationWithoutChangingObservations(t *testing.T) {
	in := []traces.Span{{TraceID: "opaque", SourceRef: "old", Service: "reader", ScopeName: "scope", ResourceAttributes: map[string]any{"host.id": "actual"}}}
	result, err := Normalize(in, Options{}, "http://collector.local/v1/traces")
	if err != nil {
		t.Fatal(err)
	}
	if in[0].SourceKind != "" || result.Spans[0].SourceKind != "ebpf" || result.Spans[0].TraceID != "opaque" || result.Spans[0].ResourceAttributes["host.id"] != "actual" {
		t.Fatalf("input was mutated or observation altered: in=%+v out=%+v", in[0], result.Spans[0])
	}
	for _, opts := range []Options{{Collector: "bad collector"}, {Collector: "../collector"}, {Collector: "obi", Domain: "bad domain"}, {Collector: "obi", Domain: "scope/other"}} {
		if _, err := Normalize(in, opts, "endpoint"); err == nil {
			t.Fatalf("invalid declaration accepted: %+v", opts)
		}
	}
	if _, err := Normalize(in, Options{}, " "); err == nil {
		t.Fatal("missing source reference accepted")
	}
}

func TestNormalizeGenericCollectorPreservesIDsAndIsolatesDomain(t *testing.T) {
	in := []traces.Span{{TraceID: "shared-trace", SpanID: "shared-span", ParentSpanID: "shared-parent", Service: "worker"}}
	result, err := Normalize(in, Options{Collector: "collector-b", Domain: "shared"}, "receiver-b")
	if err != nil {
		t.Fatal(err)
	}
	span := result.Spans[0]
	if span.TraceID != in[0].TraceID || span.SpanID != in[0].SpanID || span.ParentSpanID != in[0].ParentSpanID {
		t.Fatalf("OTLP identity was rewritten: in=%+v out=%+v", in[0], span)
	}
	if span.SourceConnector != "collector-b" || span.SourceDomain != "ebpf-otlp:collector-b:shared" {
		t.Fatalf("generic source provenance mismatch: %+v", span)
	}
	if got := result.Sources[0]; got.Type != "ebpf" || got.Connector != "collector-b" || got.Ref != "receiver-b" {
		t.Fatalf("source summary mismatch: %+v", got)
	}
	legacy, err := Normalize(in, Options{Collector: "obi", Domain: "shared"}, "receiver-a")
	if err != nil {
		t.Fatal(err)
	}
	if legacy.Spans[0].SourceDomain != "ebpf:shared" || legacy.Sources[0].Connector != "obi" {
		t.Fatalf("legacy OBI provenance changed: %+v %+v", legacy.Spans[0], legacy.Sources)
	}
	collision, err := Normalize(in, Options{Collector: "obi", Domain: "collector-b:shared"}, "receiver-c")
	if err != nil {
		t.Fatal(err)
	}
	if collision.Spans[0].SourceDomain == span.SourceDomain {
		t.Fatalf("legacy OBI and named collector domains collide: legacy=%q generic=%q", collision.Spans[0].SourceDomain, span.SourceDomain)
	}
}
