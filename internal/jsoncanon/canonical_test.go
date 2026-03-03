package jsoncanon

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestMarshalIndent_MapInsertionOrderDeterministic(t *testing.T) {
	t.Parallel()

	first := map[string]any{
		"metadata": map[string]any{
			"attributes": map[string]any{
				"zeta":  1,
				"alpha": "x",
			},
		},
	}

	second := map[string]any{
		"metadata": map[string]any{
			"attributes": map[string]any{
				"alpha": "x",
				"zeta":  1,
			},
		},
	}

	rawA, err := MarshalIndent(first)
	if err != nil {
		t.Fatalf("marshal first: %v", err)
	}
	rawB, err := MarshalIndent(second)
	if err != nil {
		t.Fatalf("marshal second: %v", err)
	}

	if !bytes.Equal(rawA, rawB) {
		t.Fatalf("canonical output mismatch:\nA=%s\nB=%s", rawA, rawB)
	}
}

func TestMarshalIndent_ProducesValidJSON(t *testing.T) {
	t.Parallel()

	raw, err := MarshalIndent(map[string]any{
		"b": 2,
		"a": []any{map[string]any{"z": "v", "a": "w"}},
	})
	if err != nil {
		t.Fatalf("MarshalIndent returned error: %v", err)
	}

	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("output must be valid json: %v", err)
	}
}
