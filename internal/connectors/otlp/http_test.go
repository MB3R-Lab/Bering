package otlp

import (
	"bytes"
	"compress/gzip"
	"net/http/httptest"
	"testing"
	"time"

	collecttracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
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
	if spans[0].Kind != "server" {
		t.Fatalf("kind mismatch: %s", spans[0].Kind)
	}
	if spans[0].StartTime.IsZero() || spans[0].EndTime.IsZero() {
		t.Fatal("expected timestamps to be populated")
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
}

func sampleRequest(start time.Time) *collecttracev1.ExportTraceServiceRequest {
	end := start.Add(50 * time.Millisecond)
	return &collecttracev1.ExportTraceServiceRequest{
		ResourceSpans: []*tracev1.ResourceSpans{{
			Resource: &resourcev1.Resource{Attributes: []*commonv1.KeyValue{{
				Key:   "service.name",
				Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "frontend"}},
			}}},
			ScopeSpans: []*tracev1.ScopeSpans{{
				Spans: []*tracev1.Span{{
					TraceId:           []byte{0xaa, 0xbb},
					SpanId:            []byte{0x01},
					Name:              "GET /checkout",
					Kind:              tracev1.Span_SPAN_KIND_SERVER,
					StartTimeUnixNano: uint64(start.UnixNano()),
					EndTimeUnixNano:   uint64(end.UnixNano()),
					Attributes: []*commonv1.KeyValue{
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
