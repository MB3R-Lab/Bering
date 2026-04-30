package snapshot

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/MB3R-Lab/Bering/internal/atomicfile"
	"github.com/MB3R-Lab/Bering/internal/jsoncanon"
)

func ParseJSON(raw []byte) (Envelope, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var env Envelope
	if err := dec.Decode(&env); err != nil {
		return Envelope{}, fmt.Errorf("decode snapshot: %w", err)
	}
	if err := env.ValidateSemantic(); err != nil {
		return Envelope{}, fmt.Errorf("semantic validation failed: %w", err)
	}
	return env, nil
}

func MarshalCanonical(env Envelope) ([]byte, error) {
	if err := env.ValidateSemantic(); err != nil {
		return nil, fmt.Errorf("semantic validation failed: %w", err)
	}
	env.SortDeterministic()
	raw, err := jsoncanon.MarshalIndent(env)
	if err != nil {
		return nil, fmt.Errorf("canonical json marshal failed: %w", err)
	}
	return raw, nil
}

func WriteToFile(path string, env Envelope) error {
	raw, err := MarshalCanonical(env)
	if err != nil {
		return err
	}
	if err := atomicfile.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write snapshot file: %w", err)
	}
	return nil
}
