package overlay

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func LoadFile(path string) (File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return File{}, fmt.Errorf("read overlay file: %w", err)
	}
	var out File
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		if err := json.Unmarshal(raw, &out); err != nil {
			return File{}, fmt.Errorf("decode overlay json: %w", err)
		}
	default:
		if err := yaml.Unmarshal(raw, &out); err != nil {
			return File{}, fmt.Errorf("decode overlay yaml: %w", err)
		}
	}
	if err := out.Normalize(path); err != nil {
		return File{}, err
	}
	return out, nil
}

func LoadFiles(paths []string) ([]File, error) {
	out := make([]File, 0, len(paths))
	for _, path := range paths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			continue
		}
		item, err := LoadFile(trimmed)
		if err != nil {
			return nil, fmt.Errorf("load overlay %q: %w", trimmed, err)
		}
		out = append(out, item)
	}
	return out, nil
}
