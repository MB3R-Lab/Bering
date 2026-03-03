package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/MB3R-Lab/Bering/internal/jsoncanon"
)

func ParseJSON(raw []byte) (ResilienceModel, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var mdl ResilienceModel
	if err := dec.Decode(&mdl); err != nil {
		return ResilienceModel{}, fmt.Errorf("decode model: %w", err)
	}
	if err := mdl.ValidateSemantic(); err != nil {
		return ResilienceModel{}, fmt.Errorf("semantic validation failed: %w", err)
	}
	return mdl, nil
}

func LoadFromFile(path string) (ResilienceModel, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ResilienceModel{}, fmt.Errorf("read model file: %w", err)
	}
	return ParseJSON(raw)
}

func MarshalCanonical(mdl ResilienceModel) ([]byte, error) {
	if err := mdl.ValidateSemantic(); err != nil {
		return nil, fmt.Errorf("semantic validation failed: %w", err)
	}
	mdl.SortDeterministic()
	raw, err := jsoncanon.MarshalIndent(mdl)
	if err != nil {
		return nil, fmt.Errorf("canonical json marshal failed: %w", err)
	}
	return raw, nil
}

func WriteToFile(path string, mdl ResilienceModel) error {
	raw, err := MarshalCanonical(mdl)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write model file: %w", err)
	}
	return nil
}
