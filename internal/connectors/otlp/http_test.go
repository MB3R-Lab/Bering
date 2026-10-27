package otlp

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	collecttracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/MB3R-Lab/Bering/internal/connectors/traces"
	"github.com/MB3R-Lab/Bering/internal/discovery"
)

func TestDecodePayload_Protobuf(t *testing.T) {
	t.Parallel()

	req := sampleRequest(time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC))
	raw, err := proto.Marshal(req)
	if err != nil {
		t.Fatalf("marshal protobuf request: %v", err)
	}

	spans, err := DecodePayload("application/x-protobuf", raw)
	if err != nil {
		t.Fatalf("DecodePayload returned error: %v", err)
	}
	if got, want := len(spans), 1; got != want {
		t.Fatalf("span count mismatch: got=%d want=%d", got, want)
	}
	if spans[0].Service != "frontend" {
		t.Fatalf("service mismatch: %s", spans[0].Service)
	}
	if spans[0].ResourceAttributes["service.name"] != "frontend" || spans[0].Attributes["service.name"] != "span-alias" {
		t.Fatalf("resource identity or merged compatibility lost: resource=%+v merged=%+v", spans[0].ResourceAttributes, spans[0].Attributes)
	}
	if spans[0].Kind != "server" {
		t.Fatalf("kind mismatch: %s", spans[0].Kind)
	}
	if spans[0].StartTime.IsZero() || spans[0].EndTime.IsZero() {
		t.Fatal("expected timestamps to be populated")
	}
	if spans[0].ScopeName != "http.instrumentation" || spans[0].ScopeVersion != "1.2.3" || spans[0].ResourceAttributes["host.name"] != "node-a" {
		t.Fatalf("missing protobuf metadata: %+v", spans[0])
	}
	if spans[0].SourceRef != "" {
		t.Fatalf("runtime source ref should be empty: %q", spans[0].SourceRef)
	}
}

func TestDecodePayload_StandardHexJSONIDsAndUnknownFields(t *testing.T) {
	t.Parallel()
	req := sampleRequest(time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC))
	protobufRaw, err := proto.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	binary, err := DecodePayload("application/x-protobuf", protobufRaw)
	if err != nil || len(binary) != 1 {
		t.Fatalf("binary decode: spans=%d err=%v", len(binary), err)
	}
	protoJSON, err := protojson.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(protoJSON, &doc); err != nil {
		t.Fatal(err)
	}
	doc["futureTopLevelField"] = "ignored"
	resource := doc["resourceSpans"].([]any)[0].(map[string]any)
	scope := resource["scopeSpans"].([]any)[0].(map[string]any)
	span := scope["spans"].([]any)[0].(map[string]any)
	span["traceId"] = binary[0].TraceID
	span["spanId"] = binary[0].SpanID
	span["parentSpanId"] = binary[0].ParentSpanID
	span["futureSpanField"] = true
	hexJSON, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	fromJSON, err := DecodePayload("application/json", hexJSON)
	if err != nil || len(fromJSON) != 1 {
		t.Fatalf("hex JSON decode: spans=%d err=%v", len(fromJSON), err)
	}
	if fromJSON[0].TraceID != binary[0].TraceID || fromJSON[0].SpanID != binary[0].SpanID || fromJSON[0].ParentSpanID != binary[0].ParentSpanID {
		t.Fatalf("standard hex JSON IDs differ from binary: json=%+v binary=%+v", fromJSON[0], binary[0])
	}
}

func TestDecodePayload_AllZeroOTLPIDsAreAbsent(t *testing.T) {
	t.Parallel()
	for _, contentType := range []string{"application/x-protobuf", "application/json"} {
		t.Run(contentType, func(t *testing.T) {
			req := sampleRequest(time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC))
			span := req.ResourceSpans[0].ScopeSpans[0].Spans[0]
			span.TraceId = make([]byte, 16)
			span.SpanId = make([]byte, 8)
			span.ParentSpanId = make([]byte, 8)
			var raw []byte
			var err error
			if contentType == "application/json" {
				raw, err = protojson.Marshal(req)
			} else {
				raw, err = proto.Marshal(req)
			}
			if err != nil {
				t.Fatal(err)
			}
			spans, err := DecodePayload(contentType, raw)
			if err != nil || len(spans) != 1 {
				t.Fatalf("decode: spans=%d err=%v", len(spans), err)
			}
			if spans[0].TraceID != "" || spans[0].SpanID != "" || spans[0].ParentSpanID != "" {
				t.Fatalf("all-zero OTLP IDs must be absent: %+v", spans[0])
			}
		})
	}
}

func TestDecodePayload_InvalidLengthOTLPIDsAreAbsent(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                string
		trace, span, parent []byte
	}{
		{"short", []byte{1}, []byte{2}, []byte{3}},
		{"overlong", bytes.Repeat([]byte{1}, 17), bytes.Repeat([]byte{2}, 9), bytes.Repeat([]byte{3}, 9)},
	} {
		for _, contentType := range []string{"application/x-protobuf", "application/json"} {
			t.Run(tc.name+"/"+contentType, func(t *testing.T) {
				req := sampleRequest(time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC))
				span := req.ResourceSpans[0].ScopeSpans[0].Spans[0]
				span.TraceId, span.SpanId, span.ParentSpanId = tc.trace, tc.span, tc.parent
				var raw []byte
				var err error
				if contentType == "application/json" {
					raw, err = protojson.Marshal(req)
				} else {
					raw, err = proto.Marshal(req)
				}
				if err != nil {
					t.Fatal(err)
				}
				spans, err := DecodePayload(contentType, raw)
				if err != nil || len(spans) != 1 {
					t.Fatalf("decode: spans=%d err=%v", len(spans), err)
				}
				if spans[0].TraceID != "" || spans[0].SpanID != "" || spans[0].ParentSpanID != "" {
					t.Fatalf("invalid-length OTLP IDs must be absent: %+v", spans[0])
				}
			})
		}
	}
}

func TestBatchAndRuntimeOTLPMetadataParity(t *testing.T) {
	t.Parallel()
	req := sampleRequest(time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC))
	stringValue := func(value string) *commonv1.AnyValue {
		return &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: value}}
	}
	intValue := &commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 42}}
	arrayValue := &commonv1.AnyValue{Value: &commonv1.AnyValue_ArrayValue{ArrayValue: &commonv1.ArrayValue{
		Values: []*commonv1.AnyValue{stringValue("42"), intValue},
	}}}
	nestedValue := &commonv1.AnyValue{Value: &commonv1.AnyValue_KvlistValue{KvlistValue: &commonv1.KeyValueList{
		Values: []*commonv1.KeyValue{{Key: "items", Value: arrayValue}},
	}}}
	req.ResourceSpans[0].Resource.Attributes = append(req.ResourceSpans[0].Resource.Attributes,
		&commonv1.KeyValue{Key: "process.pid", Value: intValue},
		&commonv1.KeyValue{Key: "resource.bytes", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_BytesValue{BytesValue: []byte{0, 1, 255}}}},
	)
	req.ResourceSpans[0].ScopeSpans[0].Spans[0].Attributes = append(req.ResourceSpans[0].ScopeSpans[0].Spans[0].Attributes,
		&commonv1.KeyValue{Key: "numeric.string", Value: stringValue("42")},
		&commonv1.KeyValue{Key: "flag", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_BoolValue{BoolValue: true}}},
		&commonv1.KeyValue{Key: "double", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_DoubleValue{DoubleValue: 1.25}}},
		&commonv1.KeyValue{Key: "nested", Value: nestedValue},
	)
	raw, err := protojson.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "trace.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	batch, err := traces.Load(path)
	if err != nil || len(batch) != 1 {
		t.Fatalf("batch load: spans=%d err=%v", len(batch), err)
	}
	runtime := NormalizeRequest(req)
	if len(runtime) != 1 {
		t.Fatalf("runtime spans=%d", len(runtime))
	}
	if batch[0].SourceRef != path || runtime[0].SourceRef != "" {
		t.Fatalf("source attribution mismatch: batch=%q runtime=%q", batch[0].SourceRef, runtime[0].SourceRef)
	}
	if batch[0].ScopeName != runtime[0].ScopeName || batch[0].ScopeVersion != runtime[0].ScopeVersion {
		t.Fatalf("scope metadata differs: batch=%+v runtime=%+v", batch[0], runtime[0])
	}
	if !reflect.DeepEqual(batch[0].ResourceAttributes, runtime[0].ResourceAttributes) {
		t.Fatalf("resource metadata differs: batch=%+v runtime=%+v", batch[0].ResourceAttributes, runtime[0].ResourceAttributes)
	}
	if !reflect.DeepEqual(batch[0].Attributes, runtime[0].Attributes) {
		t.Fatalf("merged attributes differ: batch=%+v runtime=%+v", batch[0].Attributes, runtime[0].Attributes)
	}
	if _, ok := batch[0].ResourceAttributes["process.pid"].(int64); !ok {
		t.Fatalf("process.pid lost int64 type: %#v", batch[0].ResourceAttributes["process.pid"])
	}
	if got, ok := batch[0].Attributes["numeric.string"].(string); !ok || got != "42" {
		t.Fatalf("numeric-looking string changed type: %#v", batch[0].Attributes["numeric.string"])
	}
	if batch[0].Service != runtime[0].Service {
		t.Fatalf("service differs: batch=%q runtime=%q", batch[0].Service, runtime[0].Service)
	}
	if batch[0].TraceID != runtime[0].TraceID || batch[0].SpanID != runtime[0].SpanID || batch[0].ParentSpanID != runtime[0].ParentSpanID || batch[0].Kind != runtime[0].Kind {
		t.Fatalf("identity/kind differ: batch=%+v runtime=%+v", batch[0], runtime[0])
	}
}

func TestMixedBatchRuntimeDuplicateKeepsExecutionLink(t *testing.T) {
	t.Parallel()
	req := sampleRequest(time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC))
	parent := req.ResourceSpans[0].ScopeSpans[0].Spans[0]
	parent.ParentSpanId = nil
	childResource := proto.Clone(req.ResourceSpans[0]).(*tracev1.ResourceSpans)
	childResource.Resource.Attributes[0].Value = &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "backend"}}
	child := childResource.ScopeSpans[0].Spans[0]
	child.SpanId = []byte{9, 8, 7, 6, 5, 4, 3, 2}
	child.ParentSpanId = append([]byte(nil), parent.SpanId...)
	child.Name = "GET /items"
	req.ResourceSpans = append(req.ResourceSpans, childResource)
	raw, err := protojson.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "trace.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	batch, err := traces.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	runtime := NormalizeRequest(req)
	result, err := discovery.Discover(append(batch, runtime...), discovery.Options{DiscoveredAt: "2026-03-11T12:00:00Z", SourceRef: "mixed-input"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Model.Edges) != 1 || result.Model.Edges[0].From != "frontend" || result.Model.Edges[0].To != "backend" {
		t.Fatalf("duplicate spans suppressed execution edge: %+v", result.Model.Edges)
	}
}

func TestAttributeDuplicateConflictsAcrossBatchRuntime(t *testing.T) {
	t.Parallel()
	stringKV := func(key, value string) *commonv1.KeyValue {
		return &commonv1.KeyValue{Key: key, Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: value}}}
	}
	for _, reverse := range []bool{false, true} {
		t.Run(map[bool]string{false: "forward", true: "reverse"}[reverse], func(t *testing.T) {
			req := sampleRequest(time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC))
			resource := req.ResourceSpans[0].Resource
			span := req.ResourceSpans[0].ScopeSpans[0].Spans[0]
			scopeValues := []string{"network-a", "network-b", "network-a"}
			addressValues := []string{"db-a.internal", "db-b.internal", "db-a.internal"}
			if reverse {
				scopeValues[0], scopeValues[1] = scopeValues[1], scopeValues[0]
				addressValues[0], addressValues[1] = addressValues[1], addressValues[0]
			}
			resource.Attributes = append(resource.Attributes, stringKV("host.name", "node-a"))
			for _, value := range scopeValues {
				resource.Attributes = append(resource.Attributes, stringKV("bering.network.scope", value))
			}
			span.Attributes = append(span.Attributes, stringKV("http.route", "/checkout"))
			for _, value := range addressValues {
				span.Attributes = append(span.Attributes, stringKV("server.address", value))
			}
			span.Attributes = append(span.Attributes, &commonv1.KeyValue{Key: "nested", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_KvlistValue{KvlistValue: &commonv1.KeyValueList{Values: []*commonv1.KeyValue{
				stringKV("key", "one"), stringKV("key", "two"),
			}}}}})
			raw, err := protojson.Marshal(req)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "trace.json")
			if err := os.WriteFile(path, raw, 0o644); err != nil {
				t.Fatal(err)
			}
			batch, err := traces.Load(path)
			if err != nil || len(batch) != 1 {
				t.Fatalf("batch: spans=%d err=%v", len(batch), err)
			}
			runtime := NormalizeRequest(req)
			if !reflect.DeepEqual(batch[0].AttributeConflicts, runtime[0].AttributeConflicts) || !reflect.DeepEqual(batch[0].Attributes, runtime[0].Attributes) || !reflect.DeepEqual(batch[0].ResourceAttributes, runtime[0].ResourceAttributes) {
				t.Fatalf("batch/runtime conflict mismatch: batch=%+v runtime=%+v", batch[0], runtime[0])
			}
			if want := []string{"resource:bering.network.scope", "span:nested", "span:server.address"}; !reflect.DeepEqual(batch[0].AttributeConflicts, want) {
				t.Fatalf("conflict keys: got=%v want=%v", batch[0].AttributeConflicts, want)
			}
			if _, exists := batch[0].ResourceAttributes["bering.network.scope"]; exists {
				t.Fatal("conflicting resource scope retained")
			}
			if _, exists := batch[0].Attributes["server.address"]; exists {
				t.Fatal("conflicting span address retained")
			}
			if _, exists := batch[0].Attributes["nested"]; exists {
				t.Fatal("conflicting nested value retained")
			}
			if batch[0].ResourceAttributes["host.name"] != "node-a" || batch[0].Attributes["http.route"] != "/checkout" {
				t.Fatal("identical duplicates were discarded")
			}
		})
	}
}

func TestConflictingResourceServiceNameDoesNotInventIdentity(t *testing.T) {
	t.Parallel()
	req := sampleRequest(time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC))
	span := req.ResourceSpans[0].ScopeSpans[0].Spans[0]
	span.Attributes = span.Attributes[1:] // Remove the span-level service fallback.
	req.ResourceSpans[0].Resource.Attributes = append(req.ResourceSpans[0].Resource.Attributes,
		&commonv1.KeyValue{Key: "service.name", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "other"}}},
		&commonv1.KeyValue{Key: "service.name", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "frontend"}}},
	)
	raw, err := protojson.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "trace.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	batch, err := traces.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	runtime := NormalizeRequest(req)
	for _, got := range []traces.Span{batch[0], runtime[0]} {
		if got.Service != "" || len(got.AttributeConflicts) != 1 || got.AttributeConflicts[0] != "resource:service.name" {
			t.Fatalf("conflicting service identity was selected: %+v", got)
		}
		if _, exists := got.ResourceAttributes["service.name"]; exists {
			t.Fatal("conflicting resource service.name retained")
		}
	}
}

func TestDecodeHTTPRequest_JSONGzip(t *testing.T) {
	t.Parallel()

	req := sampleRequest(time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC))
	raw, err := protojson.Marshal(req)
	if err != nil {
		t.Fatalf("marshal json request: %v", err)
	}
	var buf bytes.Buffer
	gz := gzipWriter(t, &buf)
	if _, err := gz.Write(raw); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}

	httpReq := httptest.NewRequest("POST", "/v1/traces", &buf)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Content-Encoding", "gzip")

	spans, err := DecodeHTTPRequest(httpReq, 1<<20)
	if err != nil {
		t.Fatalf("DecodeHTTPRequest returned error: %v", err)
	}
	if got, want := len(spans), 1; got != want {
		t.Fatalf("span count mismatch: got=%d want=%d", got, want)
	}
	if spans[0].Attributes["http.route"] != "/checkout" {
		t.Fatalf("route attribute mismatch: %+v", spans[0].Attributes)
	}
	if spans[0].TraceID != "aabbccddeeff0102030405060708090a" || spans[0].SpanID != "0102030405060708" || spans[0].ParentSpanID != "1020304050607080" {
		t.Fatalf("protojson base64 ID compatibility lost: %+v", spans[0])
	}
}

func sampleRequest(start time.Time) *collecttracev1.ExportTraceServiceRequest {
	end := start.Add(50 * time.Millisecond)
	return &collecttracev1.ExportTraceServiceRequest{
		ResourceSpans: []*tracev1.ResourceSpans{{
			Resource: &resourcev1.Resource{Attributes: []*commonv1.KeyValue{{
				Key:   "service.name",
				Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "frontend"}},
			}, {
				Key:   "host.name",
				Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "node-a"}},
			}}},
			ScopeSpans: []*tracev1.ScopeSpans{{
				Scope: &commonv1.InstrumentationScope{Name: "http.instrumentation", Version: "1.2.3"},
				Spans: []*tracev1.Span{{
					TraceId:           []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a},
					SpanId:            []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
					ParentSpanId:      []byte{0x10, 0x20, 0x30, 0x40, 0x50, 0x60, 0x70, 0x80},
					Name:              "GET /checkout",
					Kind:              tracev1.Span_SPAN_KIND_SERVER,
					StartTimeUnixNano: uint64(start.UnixNano()),
					EndTimeUnixNano:   uint64(end.UnixNano()),
					Attributes: []*commonv1.KeyValue{
						{Key: "service.name", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "span-alias"}}},
						{Key: "http.request.method", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "GET"}}},
						{Key: "http.route", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "/checkout"}}},
					},
				}},
			}},
		}},
	}
}

func gzipWriter(t *testing.T, buf *bytes.Buffer) *gzip.Writer {
	t.Helper()
	gz := gzip.NewWriter(buf)
	return gz
}
