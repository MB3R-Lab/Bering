package otlp

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	collecttracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/MB3R-Lab/Bering/internal/connectors/traces"
)

const (
	HTTPConnectorName = "otlp_http"
	GRPCConnectorName = "otlp_grpc"
)

func DecodeHTTPRequest(r *http.Request, maxBytes int64) ([]traces.Span, error) {
	body, err := readBody(r, maxBytes)
	if err != nil {
		return nil, err
	}
	return DecodePayload(r.Header.Get("Content-Type"), body)
}

func DecodePayload(contentType string, body []byte) ([]traces.Span, error) {
	var req collecttracev1.ExportTraceServiceRequest
	if isJSONContentType(contentType) {
		if err := protojson.Unmarshal(body, &req); err != nil {
			return nil, fmt.Errorf("decode otlp json: %w", err)
		}
	} else {
		if err := proto.Unmarshal(body, &req); err != nil {
			return nil, fmt.Errorf("decode otlp protobuf: %w", err)
		}
	}
	return NormalizeRequest(&req), nil
}

func NormalizeRequest(req *collecttracev1.ExportTraceServiceRequest) []traces.Span {
	out := []traces.Span{}
	for _, resourceSpans := range req.GetResourceSpans() {
		resourceAttrs := resourceAttributes(resourceSpans.GetResource())
		for _, scopeSpans := range resourceSpans.GetScopeSpans() {
			for _, span := range scopeSpans.GetSpans() {
				attrs := attributeMap(span.GetAttributes())
				merged := map[string]any{}
				for key, value := range resourceAttrs {
					merged[key] = value
				}
				for key, value := range attrs {
					merged[key] = value
				}
				service, _ := merged["service.name"].(string)
				out = append(out, traces.Span{
					TraceID:      traceIDHex(span.GetTraceId()),
					SpanID:       traceIDHex(span.GetSpanId()),
					ParentSpanID: traceIDHex(span.GetParentSpanId()),
					Service:      strings.TrimSpace(service),
					Name:         strings.TrimSpace(span.GetName()),
					Kind:         mapSpanKind(span.GetKind()),
					StartTime:    time.Unix(0, int64(span.GetStartTimeUnixNano())).UTC(),
					EndTime:      time.Unix(0, int64(span.GetEndTimeUnixNano())).UTC(),
					Attributes:   merged,
				})
			}
		}
	}
	return out
}

func resourceAttributes(resource *resourcev1.Resource) map[string]any {
	if resource == nil {
		return map[string]any{}
	}
	return attributeMap(resource.GetAttributes())
}

func attributeMap(attrs []*commonv1.KeyValue) map[string]any {
	out := make(map[string]any, len(attrs))
	for _, item := range attrs {
		out[item.GetKey()] = anyValue(item.GetValue())
	}
	return out
}

func anyValue(value *commonv1.AnyValue) any {
	if value == nil {
		return nil
	}
	switch typed := value.Value.(type) {
	case *commonv1.AnyValue_StringValue:
		return typed.StringValue
	case *commonv1.AnyValue_BoolValue:
		return typed.BoolValue
	case *commonv1.AnyValue_IntValue:
		return typed.IntValue
	case *commonv1.AnyValue_DoubleValue:
		return typed.DoubleValue
	case *commonv1.AnyValue_ArrayValue:
		out := make([]any, 0, len(typed.ArrayValue.GetValues()))
		for _, item := range typed.ArrayValue.GetValues() {
			out = append(out, anyValue(item))
		}
		return out
	case *commonv1.AnyValue_KvlistValue:
		out := map[string]any{}
		for _, item := range typed.KvlistValue.GetValues() {
			out[item.GetKey()] = anyValue(item.GetValue())
		}
		return out
	case *commonv1.AnyValue_BytesValue:
		return typed.BytesValue
	default:
		return nil
	}
}

func readBody(r *http.Request, maxBytes int64) ([]byte, error) {
	var reader io.Reader = r.Body
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("Content-Encoding")), "gzip") {
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			return nil, fmt.Errorf("open gzip body: %w", err)
		}
		defer gz.Close()
		reader = gz
	}
	if maxBytes > 0 {
		reader = io.LimitReader(reader, maxBytes+1)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}
	if maxBytes > 0 && int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("request body exceeds %d bytes", maxBytes)
	}
	return body, nil
}

func isJSONContentType(contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	return strings.Contains(contentType, "json")
}

func traceIDHex(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	return fmt.Sprintf("%x", raw)
}

func mapSpanKind(kind tracev1.Span_SpanKind) string {
	switch kind {
	case tracev1.Span_SPAN_KIND_SERVER:
		return "server"
	case tracev1.Span_SPAN_KIND_CLIENT:
		return "client"
	case tracev1.Span_SPAN_KIND_PRODUCER:
		return "producer"
	case tracev1.Span_SPAN_KIND_CONSUMER:
		return "consumer"
	default:
		return "internal"
	}
}
