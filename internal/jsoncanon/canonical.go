package jsoncanon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
)

const indent = "  "

// MarshalIndent serializes JSON deterministically by sorting all object keys
// recursively, including nested map fields.
func MarshalIndent(v any) ([]byte, error) {
	doc, err := canonicalize(v)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := writeValue(&buf, doc, 0); err != nil {
		return nil, err
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

func canonicalize(v any) (any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal value: %w", err)
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()

	var doc any
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("decode canonical json: %w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("unexpected trailing tokens")
	}
	return doc, nil
}

func writeValue(buf *bytes.Buffer, value any, depth int) error {
	switch v := value.(type) {
	case nil:
		buf.WriteString("null")
		return nil
	case bool:
		if v {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
		return nil
	case string:
		encoded, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("encode string: %w", err)
		}
		buf.Write(encoded)
		return nil
	case json.Number:
		if _, err := strconv.ParseFloat(v.String(), 64); err != nil {
			return fmt.Errorf("invalid json number %q: %w", v.String(), err)
		}
		buf.WriteString(v.String())
		return nil
	case float64, float32, int, int64, int32, uint, uint64, uint32:
		encoded, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("encode number: %w", err)
		}
		buf.Write(encoded)
		return nil
	case []any:
		return writeArray(buf, v, depth)
	case map[string]any:
		return writeObject(buf, v, depth)
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("encode scalar value: %w", err)
		}
		buf.Write(encoded)
		return nil
	}
}

func writeArray(buf *bytes.Buffer, arr []any, depth int) error {
	if len(arr) == 0 {
		buf.WriteString("[]")
		return nil
	}

	buf.WriteString("[\n")
	for i, item := range arr {
		writeIndent(buf, depth+1)
		if err := writeValue(buf, item, depth+1); err != nil {
			return err
		}
		if i < len(arr)-1 {
			buf.WriteString(",")
		}
		buf.WriteString("\n")
	}
	writeIndent(buf, depth)
	buf.WriteString("]")
	return nil
}

func writeObject(buf *bytes.Buffer, obj map[string]any, depth int) error {
	if len(obj) == 0 {
		buf.WriteString("{}")
		return nil
	}

	keys := make([]string, 0, len(obj))
	for key := range obj {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	buf.WriteString("{\n")
	for i, key := range keys {
		writeIndent(buf, depth+1)
		encodedKey, err := json.Marshal(key)
		if err != nil {
			return fmt.Errorf("encode object key: %w", err)
		}
		buf.Write(encodedKey)
		buf.WriteString(": ")
		if err := writeValue(buf, obj[key], depth+1); err != nil {
			return err
		}
		if i < len(keys)-1 {
			buf.WriteString(",")
		}
		buf.WriteString("\n")
	}
	writeIndent(buf, depth)
	buf.WriteString("}")
	return nil
}

func writeIndent(buf *bytes.Buffer, depth int) {
	for i := 0; i < depth; i++ {
		buf.WriteString(indent)
	}
}
