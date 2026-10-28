package traces

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Span struct {
	TraceID            string
	SpanID             string
	ParentSpanID       string
	SourceRef          string
	SourceKind         string
	SourceDomain       string
	Service            string
	Name               string
	Kind               string
	ScopeName          string
	ScopeVersion       string
	StartTime          time.Time
	EndTime            time.Time
	Attributes         map[string]any
	ResourceAttributes map[string]any
	AttributeConflicts []string
}

func (s Span) EventTime(fallback time.Time) time.Time {
	if !s.EndTime.IsZero() {
		return s.EndTime.UTC()
	}
	if !s.StartTime.IsZero() {
		return s.StartTime.UTC()
	}
	return fallback.UTC()
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
		for i := range spans {
			spans[i].SourceRef = path
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

		resourceAttrs := toMap(obj["resource_attributes"])
		conflicts := normalizedConflictKeys(obj["attribute_conflicts"])
		attrs := copyAttributes(resourceAttrs)
		if rawAttrs, ok := obj["attributes"]; ok {
			for key, value := range toMap(rawAttrs) {
				attrs[key] = value
			}
		}
		mergeNonEmpty(attrs, "http.request.method", firstNonEmptyString(obj["http.request.method"], obj["http.method"], obj["method"]))
		mergeNonEmpty(attrs, "http.route", firstNonEmptyString(obj["http.route"], obj["route"]))
		mergeNonEmpty(attrs, "http.target", firstNonEmptyString(obj["http.target"], obj["target"]))
		mergeNonEmpty(attrs, "url.path", firstNonEmptyString(obj["url.path"], obj["path"]))
		mergeNonEmpty(attrs, "messaging.system", firstNonEmptyString(obj["messaging.system"]))
		mergeNonEmpty(attrs, "messaging.destination", firstNonEmptyString(obj["messaging.destination"], obj["messaging.destination.name"], obj["topic"]))
		mergeNonEmpty(attrs, "messaging.operation", firstNonEmptyString(obj["messaging.operation"], obj["operation"]))
		mergeNonEmpty(attrs, "rpc.system", firstNonEmptyString(obj["rpc.system"], obj["protocol"]))
		mergeNonEmpty(attrs, "rpc.method", firstNonEmptyString(obj["rpc.method"]))

		out = append(out, Span{
			TraceID:            firstNonEmptyString(obj["trace_id"], obj["traceId"]),
			SpanID:             firstNonEmptyString(obj["span_id"], obj["spanId"]),
			ParentSpanID:       firstNonEmptyString(obj["parent_span_id"], obj["parentSpanId"]),
			SourceRef:          firstNonEmptyString(obj["source_ref"]),
			Service:            firstNonEmptyString(resourceAttrs["service.name"], obj["service"], obj["service_name"], obj["service.name"], attrs["service.name"]),
			Name:               firstNonEmptyString(obj["name"]),
			Kind:               strings.ToLower(strings.TrimSpace(firstNonEmptyString(obj["kind"]))),
			ScopeName:          firstNonEmptyString(obj["scope_name"]),
			ScopeVersion:       firstNonEmptyString(obj["scope_version"]),
			StartTime:          parseTimeValue(obj["start_time"], obj["startTime"]),
			EndTime:            parseTimeValue(obj["end_time"], obj["endTime"]),
			Attributes:         attrs,
			ResourceAttributes: copyAttributes(resourceAttrs),
			AttributeConflicts: conflicts,
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

		resourceAttrs, resourceConflicts := attributeMap(nil)
		if resourceObj, ok := resourceSpan["resource"].(map[string]any); ok {
			resourceAttrs, resourceConflicts = attributeMap(resourceObj["attributes"])
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
			scopeObj, _ := firstNonNil(scopeSpan["scope"], scopeSpan["instrumentationLibrary"]).(map[string]any)
			scopeName := firstNonEmptyString(scopeObj["name"])
			scopeVersion := firstNonEmptyString(scopeObj["version"])
			rawSpans, ok := scopeSpan["spans"].([]any)
			if !ok {
				continue
			}

			for _, rawSpan := range rawSpans {
				spanObj, ok := rawSpan.(map[string]any)
				if !ok {
					continue
				}
				attrs, spanConflicts := attributeMap(spanObj["attributes"])

				mergedAttrs := map[string]any{}
				for k, v := range resourceAttrs {
					mergedAttrs[k] = v
				}
				for k, v := range attrs {
					mergedAttrs[k] = v
				}

				service := strings.TrimSpace(firstNonEmptyString(
					resourceAttrs["service.name"],
					attrs["service.name"],
					spanObj["service"],
				))
				kind := mapSpanKind(firstNonEmptyString(spanObj["kind"]))

				out = append(out, Span{
					TraceID:            normalizeOTLPID(firstNonEmptyString(spanObj["traceId"], spanObj["trace_id"]), 16),
					SpanID:             normalizeOTLPID(firstNonEmptyString(spanObj["spanId"], spanObj["span_id"]), 8),
					ParentSpanID:       normalizeOTLPID(firstNonEmptyString(spanObj["parentSpanId"], spanObj["parent_span_id"]), 8),
					Service:            service,
					Name:               firstNonEmptyString(spanObj["name"]),
					Kind:               kind,
					ScopeName:          scopeName,
					ScopeVersion:       scopeVersion,
					StartTime:          parseTimeValue(spanObj["startTimeUnixNano"], spanObj["start_time_unix_nano"]),
					EndTime:            parseTimeValue(spanObj["endTimeUnixNano"], spanObj["end_time_unix_nano"]),
					Attributes:         mergedAttrs,
					ResourceAttributes: copyAttributes(resourceAttrs),
					AttributeConflicts: append(prefixedConflicts("resource:", resourceConflicts), prefixedConflicts("span:", spanConflicts)...),
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
		if value <= 0 {
			return nil, fmt.Errorf("replicas override for %q must be >= 1", id)
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

func attributeMap(raw any) (map[string]any, []string) {
	switch typed := raw.(type) {
	case map[string]any:
		return typed, nil
	case []any:
		out := map[string]any{}
		conflicted := map[string]bool{}
		for _, entry := range typed {
			obj, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			key := firstNonEmptyString(obj["key"])
			if key == "" {
				continue
			}
			if conflicted[key] {
				continue
			}
			decoded, nestedConflict := otelValue(obj["value"])
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
	default:
		return map[string]any{}, nil
	}
}

func otelValue(v any) (any, bool) {
	obj, ok := v.(map[string]any)
	if !ok {
		return v, false
	}
	if value, exists := obj["stringValue"]; exists {
		return value, false
	}
	if value, exists := obj["boolValue"]; exists {
		return value, false
	}
	if value, exists := obj["intValue"]; exists {
		n, err := strconv.ParseInt(firstNonEmptyString(value), 10, 64)
		if err == nil {
			return n, false
		}
		return nil, true
	}
	if value, exists := obj["doubleValue"]; exists {
		n, err := strconv.ParseFloat(firstNonEmptyString(value), 64)
		if err == nil {
			return n, false
		}
		return nil, true
	}
	if value, exists := obj["arrayValue"]; exists {
		array, _ := value.(map[string]any)
		values, _ := array["values"].([]any)
		out := make([]any, 0, len(values))
		for _, item := range values {
			decoded, conflict := otelValue(item)
			if conflict {
				return nil, true
			}
			out = append(out, decoded)
		}
		return out, false
	}
	if value, exists := obj["kvlistValue"]; exists {
		list, _ := value.(map[string]any)
		values, _ := list["values"].([]any)
		out := map[string]any{}
		for _, item := range values {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			key := firstNonEmptyString(entry["key"])
			if key == "" {
				continue
			}
			decoded, conflict := otelValue(entry["value"])
			if conflict {
				return nil, true
			}
			if previous, exists := out[key]; exists && !reflect.DeepEqual(previous, decoded) {
				return nil, true
			}
			out[key] = decoded
		}
		return out, false
	}
	if value, exists := obj["bytesValue"]; exists {
		encoded, _ := value.(string)
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err == nil {
			return decoded, false
		}
		return nil, true
	}
	if value, exists := obj["value"]; exists {
		return otelValue(value)
	}
	return v, false
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

func normalizedConflictKeys(raw any) []string {
	values, _ := raw.([]any)
	seen := map[string]bool{}
	for _, value := range values {
		if key, ok := value.(string); ok && strings.TrimSpace(key) != "" {
			seen[strings.TrimSpace(key)] = true
		}
	}
	return sortedConflictKeys(seen)
}

func mapSpanKind(rawKind string) string {
	rawKind = strings.TrimSpace(strings.ToLower(rawKind))
	rawKind = strings.TrimPrefix(rawKind, "span_kind_")
	switch rawKind {
	case "2", "server":
		return "server"
	case "3", "client":
		return "client"
	case "4", "producer":
		return "producer"
	case "5", "consumer":
		return "consumer"
	case "0", "1", "unspecified", "internal", "":
		return "internal"
	default:
		// Preserve textual kind values if they are already meaningful.
		if _, err := strconv.Atoi(rawKind); err == nil {
			return "internal"
		}
		return rawKind
	}
}

func normalizeOTLPID(raw string, size int) string {
	raw = strings.TrimSpace(raw)
	if len(raw) == size*2 {
		if decoded, err := hex.DecodeString(raw); err == nil {
			if allZeroOTLPID(decoded) {
				return ""
			}
			return strings.ToLower(raw)
		}
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err == nil && len(decoded) == size && base64.StdEncoding.EncodeToString(decoded) == raw {
		if allZeroOTLPID(decoded) {
			return ""
		}
		return hex.EncodeToString(decoded)
	}
	return ""
}

func allZeroOTLPID(raw []byte) bool {
	for _, b := range raw {
		if b != 0 {
			return false
		}
	}
	return true
}

func toMap(v any) map[string]any {
	switch typed := v.(type) {
	case map[string]any:
		return typed
	default:
		return map[string]any{}
	}
}

func copyAttributes(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
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

func parseTimeValue(values ...any) time.Time {
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) == "" {
				continue
			}
			if parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(typed)); err == nil {
				return parsed.UTC()
			}
			if n, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64); err == nil {
				return time.Unix(0, n).UTC()
			}
		case json.Number:
			if n, err := typed.Int64(); err == nil {
				return time.Unix(0, n).UTC()
			}
		case float64:
			return time.Unix(0, int64(typed)).UTC()
		case int64:
			return time.Unix(0, typed).UTC()
		case int:
			return time.Unix(0, int64(typed)).UTC()
		}
	}
	return time.Time{}
}
