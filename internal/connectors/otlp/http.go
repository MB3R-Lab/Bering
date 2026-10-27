package otlp

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"sort"
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
		normalized, err := normalizeOTLPJSONIDs(body)
		if err != nil {
			return nil, fmt.Errorf("decode otlp json: %w", err)
		}
		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(normalized, &req); err != nil {
			return nil, fmt.Errorf("decode otlp json: %w", err)
		}
	} else {
		if err := proto.Unmarshal(body, &req); err != nil {
			return nil, fmt.Errorf("decode otlp protobuf: %w", err)
		}
	}
	return NormalizeRequest(&req), nil
}

// OTLP's JSON encoding uses hex IDs. Protobuf JSON expects bytes as base64, so
// convert only canonical IDs in span fields before handing the rest to protojson.
func normalizeOTLPJSONIDs(body []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var doc any
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("multiple JSON values")
		}
		return nil, err
	}
	root, _ := doc.(map[string]any)
	resourceSpans, _ := firstJSONField(root, "resourceSpans", "resource_spans").([]any)
	for _, rawResource := range resourceSpans {
		resource, _ := rawResource.(map[string]any)
		scopeSpans, _ := firstJSONField(resource, "scopeSpans", "scope_spans", "instrumentationLibrarySpans").([]any)
		for _, rawScope := range scopeSpans {
			scope, _ := rawScope.(map[string]any)
			spans, _ := scope["spans"].([]any)
			for _, rawSpan := range spans {
				span, _ := rawSpan.(map[string]any)
				convertHexJSONID(span, 16, "traceId", "trace_id")
				convertHexJSONID(span, 8, "spanId", "span_id")
				convertHexJSONID(span, 8, "parentSpanId", "parent_span_id")
			}
		}
	}
	return json.Marshal(doc)
}

func firstJSONField(obj map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := obj[key]; ok {
			return value
		}
	}
	return nil
}

func convertHexJSONID(obj map[string]any, size int, keys ...string) {
	for _, key := range keys {
		raw, ok := obj[key].(string)
		if !ok || len(raw) != size*2 {
			continue
		}
		decoded, err := hex.DecodeString(raw)
		if err == nil {
			obj[key] = base64.StdEncoding.EncodeToString(decoded)
		}
	}
}

func NormalizeRequest(req *collecttracev1.ExportTraceServiceRequest) []traces.Span {
	out := []traces.Span{}
	for _, resourceSpans := range req.GetResourceSpans() {
		resourceAttrs, resourceConflicts := resourceAttributes(resourceSpans.GetResource())
		for _, scopeSpans := range resourceSpans.GetScopeSpans() {
			scope := scopeSpans.GetScope()
			for _, span := range scopeSpans.GetSpans() {
				attrs, spanConflicts := attributeMap(span.GetAttributes())
				merged := map[string]any{}
				for key, value := range resourceAttrs {
					merged[key] = value
				}
				for key, value := range attrs {
					merged[key] = value
				}
				service, _ := resourceAttrs["service.name"].(string)
				if strings.TrimSpace(service) == "" {
					service, _ = attrs["service.name"].(string)
				}
				out = append(out, traces.Span{
					TraceID:            traceIDHex(span.GetTraceId(), 16),
					SpanID:             traceIDHex(span.GetSpanId(), 8),
					ParentSpanID:       traceIDHex(span.GetParentSpanId(), 8),
					Service:            strings.TrimSpace(service),
					Name:               strings.TrimSpace(span.GetName()),
					Kind:               mapSpanKind(span.GetKind()),
					ScopeName:          strings.TrimSpace(scope.GetName()),
					ScopeVersion:       strings.TrimSpace(scope.GetVersion()),
					StartTime:          time.Unix(0, int64(span.GetStartTimeUnixNano())).UTC(),
					EndTime:            time.Unix(0, int64(span.GetEndTimeUnixNano())).UTC(),
					Attributes:         merged,
					ResourceAttributes: cloneAttributes(resourceAttrs),
					AttributeConflicts: append(prefixedConflicts("resource:", resourceConflicts), prefixedConflicts("span:", spanConflicts)...),
				})
			}
		}
	}
	return out
}

func cloneAttributes(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func resourceAttributes(resource *resourcev1.Resource) (map[string]any, []string) {
	if resource == nil {
		return map[string]any{}, nil
	}
	return attributeMap(resource.GetAttributes())
}

func attributeMap(attrs []*commonv1.KeyValue) (map[string]any, []string) {
	out := make(map[string]any, len(attrs))
	conflicted := map[string]bool{}
	for _, item := range attrs {
		key := strings.TrimSpace(item.GetKey())
		if key == "" || conflicted[key] {
			continue
		}
		decoded, nestedConflict := anyValue(item.GetValue())
		if nestedConflict {
			delete(out, key)
			conflicted[key] = true
			continue
		}
		if previous, exists := out[key]; exists && !reflect.DeepEqual(previous, decoded) {
			delete(out, key)
			conflicted[key] = true
			continue
		}
		out[key] = decoded
	}
	return out, sortedConflictKeys(conflicted)
}

func anyValue(value *commonv1.AnyValue) (any, bool) {
	if value == nil {
		return nil, false
	}
	switch typed := value.Value.(type) {
	case *commonv1.AnyValue_StringValue:
		return typed.StringValue, false
	case *commonv1.AnyValue_BoolValue:
		return typed.BoolValue, false
	case *commonv1.AnyValue_IntValue:
		return typed.IntValue, false
	case *commonv1.AnyValue_DoubleValue:
		return typed.DoubleValue, false
	case *commonv1.AnyValue_ArrayValue:
		out := make([]any, 0, len(typed.ArrayValue.GetValues()))
		for _, item := range typed.ArrayValue.GetValues() {
			decoded, conflict := anyValue(item)
			if conflict {
				return nil, true
			}
			out = append(out, decoded)
		}
		return out, false
	case *commonv1.AnyValue_KvlistValue:
		out := map[string]any{}
		for _, item := range typed.KvlistValue.GetValues() {
			key := strings.TrimSpace(item.GetKey())
			if key == "" {
				continue
			}
			decoded, conflict := anyValue(item.GetValue())
			if conflict {
				return nil, true
			}
			if previous, exists := out[key]; exists && !reflect.DeepEqual(previous, decoded) {
				return nil, true
			}
			out[key] = decoded
		}
		return out, false
	case *commonv1.AnyValue_BytesValue:
		return typed.BytesValue, false
	default:
		return nil, false
	}
}

func sortedConflictKeys(values map[string]bool) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func prefixedConflicts(prefix string, keys []string) []string {
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, prefix+key)
	}
	return out
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

func traceIDHex(raw []byte, size int) string {
	if len(raw) != size {
		return ""
	}
	allZero := true
	for _, b := range raw {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
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
