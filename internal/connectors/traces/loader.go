package traces

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Span struct {
	TraceID      string
	SpanID       string
	ParentSpanID string
	Service      string
	Name         string
	Kind         string
	Attributes   map[string]any
}

func Load(inputPath string) ([]Span, error) {
	paths, err := collectInputFiles(inputPath)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no trace json files found at %q", inputPath)
	}

	out := make([]Span, 0, len(paths)*8)
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read trace input %q: %w", path, err)
		}
		spans, err := parseTracePayload(raw)
		if err != nil {
			return nil, fmt.Errorf("parse trace input %q: %w", path, err)
		}
		out = append(out, spans...)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("discovered zero spans from %q", inputPath)
	}
	return out, nil
}

func LoadReplicasOverride(path string) (map[string]int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read replicas override file: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		return parseReplicasJSON(raw)
	case ".yaml", ".yml":
		return parseReplicasYAML(raw)
	default:
		// Attempt JSON first, then YAML for extension-less files.
		if replicas, err := parseReplicasJSON(raw); err == nil {
			return replicas, nil
		}
		return parseReplicasYAML(raw)
	}
}

func collectInputFiles(inputPath string) ([]string, error) {
	info, err := os.Stat(inputPath)
	if err != nil {
		return nil, fmt.Errorf("stat input path: %w", err)
	}
	if !info.IsDir() {
		return []string{inputPath}, nil
	}

	files := []string{}
	err = filepath.WalkDir(inputPath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".json") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk input directory: %w", err)
	}
	sort.Strings(files)
	return files, nil
}

func parseTracePayload(raw []byte) ([]Span, error) {
	doc, err := decodeAnyJSON(raw)
	if err != nil {
		return nil, err
	}
	if spans, ok := parseNormalized(doc); ok {
		return spans, nil
	}
	if spans, ok := parseOTLP(doc); ok {
		return spans, nil
	}
	return nil, fmt.Errorf("unsupported trace format: expected normalized {\"spans\": [...]} or OTel resourceSpans payload")
}

func parseNormalized(doc any) ([]Span, bool) {
	root, ok := doc.(map[string]any)
	if !ok {
		return nil, false
	}

	rawSpans, ok := root["spans"].([]any)
	if !ok {
		return nil, false
	}

	out := make([]Span, 0, len(rawSpans))
	for _, rawSpan := range rawSpans {
		obj, ok := rawSpan.(map[string]any)
		if !ok {
			continue
		}

		attrs := map[string]any{}
		if rawAttrs, ok := obj["attributes"]; ok {
			attrs = toMap(rawAttrs)
		}
		mergeNonEmpty(attrs, "http.request.method", firstNonEmptyString(obj["http.request.method"], obj["http.method"], obj["method"]))
		mergeNonEmpty(attrs, "http.route", firstNonEmptyString(obj["http.route"], obj["route"]))
		mergeNonEmpty(attrs, "http.target", firstNonEmptyString(obj["http.target"], obj["target"]))
		mergeNonEmpty(attrs, "url.path", firstNonEmptyString(obj["url.path"], obj["path"]))
		mergeNonEmpty(attrs, "messaging.system", firstNonEmptyString(obj["messaging.system"]))

		out = append(out, Span{
			TraceID:      firstNonEmptyString(obj["trace_id"], obj["traceId"]),
			SpanID:       firstNonEmptyString(obj["span_id"], obj["spanId"]),
			ParentSpanID: firstNonEmptyString(obj["parent_span_id"], obj["parentSpanId"]),
			Service:      firstNonEmptyString(obj["service"], obj["service_name"], obj["service.name"]),
			Name:         firstNonEmptyString(obj["name"]),
			Kind:         strings.ToLower(strings.TrimSpace(firstNonEmptyString(obj["kind"]))),
			Attributes:   attrs,
		})
	}
	return out, true
}

func parseOTLP(doc any) ([]Span, bool) {
	root, ok := doc.(map[string]any)
	if !ok {
		return nil, false
	}

	resourceSpans, ok := root["resourceSpans"].([]any)
	if !ok {
		return nil, false
	}

	out := []Span{}
	for _, rawResourceSpan := range resourceSpans {
		resourceSpan, ok := rawResourceSpan.(map[string]any)
		if !ok {
			continue
		}

		resourceAttrs := attributeMap(nil)
		if resourceObj, ok := resourceSpan["resource"].(map[string]any); ok {
			resourceAttrs = attributeMap(resourceObj["attributes"])
		}

		rawScopeSpans := firstNonNil(resourceSpan["scopeSpans"], resourceSpan["instrumentationLibrarySpans"])
		scopeSpans, ok := rawScopeSpans.([]any)
		if !ok {
			continue
		}

		for _, rawScopeSpan := range scopeSpans {
			scopeSpan, ok := rawScopeSpan.(map[string]any)
			if !ok {
				continue
			}
			rawSpans, ok := scopeSpan["spans"].([]any)
			if !ok {
				continue
			}

			for _, rawSpan := range rawSpans {
				spanObj, ok := rawSpan.(map[string]any)
				if !ok {
					continue
				}
				attrs := attributeMap(spanObj["attributes"])

				mergedAttrs := map[string]any{}
				for k, v := range resourceAttrs {
					mergedAttrs[k] = v
				}
				for k, v := range attrs {
					mergedAttrs[k] = v
				}

				service := strings.TrimSpace(firstNonEmptyString(
					mergedAttrs["service.name"],
					spanObj["service"],
				))
				kind := mapSpanKind(firstNonEmptyString(spanObj["kind"]))

				out = append(out, Span{
					TraceID:      firstNonEmptyString(spanObj["traceId"], spanObj["trace_id"]),
					SpanID:       firstNonEmptyString(spanObj["spanId"], spanObj["span_id"]),
					ParentSpanID: firstNonEmptyString(spanObj["parentSpanId"], spanObj["parent_span_id"]),
					Service:      service,
					Name:         firstNonEmptyString(spanObj["name"]),
					Kind:         kind,
					Attributes:   mergedAttrs,
				})
			}
		}
	}
	return out, true
}

func parseReplicasJSON(raw []byte) (map[string]int, error) {
	wrapper := struct {
		Replicas map[string]int `json:"replicas"`
	}{}
	if err := json.Unmarshal(raw, &wrapper); err == nil && len(wrapper.Replicas) > 0 {
		return validateReplicas(wrapper.Replicas)
	}

	direct := map[string]int{}
	if err := json.Unmarshal(raw, &direct); err != nil {
		return nil, fmt.Errorf("decode replicas json: %w", err)
	}
	return validateReplicas(direct)
}

func parseReplicasYAML(raw []byte) (map[string]int, error) {
	wrapper := struct {
		Replicas map[string]int `yaml:"replicas"`
	}{}
	if err := yaml.Unmarshal(raw, &wrapper); err == nil && len(wrapper.Replicas) > 0 {
		return validateReplicas(wrapper.Replicas)
	}

	direct := map[string]int{}
	if err := yaml.Unmarshal(raw, &direct); err != nil {
		return nil, fmt.Errorf("decode replicas yaml: %w", err)
	}
	return validateReplicas(direct)
}

func validateReplicas(values map[string]int) (map[string]int, error) {
	out := make(map[string]int, len(values))
	for key, value := range values {
		id := strings.TrimSpace(key)
		if id == "" {
			return nil, fmt.Errorf("replicas override contains empty service id")
		}
		if value < 0 {
			return nil, fmt.Errorf("replicas override for %q must be >= 0", id)
		}
		out[id] = value
	}
	return out, nil
}

func decodeAnyJSON(raw []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var out any
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func attributeMap(raw any) map[string]any {
	switch typed := raw.(type) {
	case map[string]any:
		return typed
	case []any:
		out := map[string]any{}
		for _, entry := range typed {
			obj, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			key := firstNonEmptyString(obj["key"])
			if key == "" {
				continue
			}
			if value, ok := obj["value"]; ok {
				out[key] = otelValue(value)
			}
		}
		return out
	default:
		return map[string]any{}
	}
}

func otelValue(v any) any {
	obj, ok := v.(map[string]any)
	if !ok {
		return v
	}
	for _, key := range []string{"stringValue", "intValue", "doubleValue", "boolValue", "value"} {
		if value, exists := obj[key]; exists {
			return otelValue(value)
		}
	}
	return v
}

func mapSpanKind(rawKind string) string {
	rawKind = strings.TrimSpace(strings.ToLower(rawKind))
	switch rawKind {
	case "2", "server":
		return "server"
	case "3", "client":
		return "client"
	case "4", "producer":
		return "producer"
	case "5", "consumer":
		return "consumer"
	case "1", "internal", "":
		return "internal"
	default:
		// Preserve textual kind values if they are already meaningful.
		if _, err := strconv.Atoi(rawKind); err == nil {
			return "internal"
		}
		return rawKind
	}
}

func toMap(v any) map[string]any {
	switch typed := v.(type) {
	case map[string]any:
		return typed
	default:
		return map[string]any{}
	}
}

func mergeNonEmpty(dst map[string]any, key, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	if _, exists := dst[key]; exists {
		return
	}
	dst[key] = value
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return strings.TrimSpace(typed)
			}
		case json.Number:
			if typed.String() != "" {
				return typed.String()
			}
		case float64:
			return strconv.FormatFloat(typed, 'f', -1, 64)
		case int:
			return strconv.Itoa(typed)
		}
	}
	return ""
}
