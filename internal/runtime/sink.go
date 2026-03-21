package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MB3R-Lab/Bering/internal/snapshot"
)

type SnapshotSink interface {
	Write(context.Context, snapshot.Envelope) error
}

type FileSink struct {
	Directory  string
	LatestPath string
}

type ProjectionView struct {
	Name              string             `json:"name"`
	Observation       int64              `json:"observation"`
	StructuralVersion int64              `json:"structural_version"`
	Available         bool               `json:"available"`
	TopologyVersion   string             `json:"topology_version,omitempty"`
	Snapshot          *snapshot.Envelope `json:"snapshot,omitempty"`
}

func (s FileSink) Write(_ context.Context, env snapshot.Envelope) error {
	name := sanitizeFilename(env.WindowEnd) + "-" + env.SnapshotID + ".json"
	path := filepath.Join(s.Directory, name)
	if err := snapshot.WriteToFile(path, env); err != nil {
		return fmt.Errorf("write snapshot sink file: %w", err)
	}
	if strings.TrimSpace(s.LatestPath) != "" {
		if err := snapshot.WriteToFile(s.LatestPath, env); err != nil {
			return fmt.Errorf("write latest snapshot file: %w", err)
		}
	}
	return nil
}

func WriteProjectionView(path string, view ProjectionView) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	raw, err := json.MarshalIndent(view, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal projection view: %w", err)
	}
	return writeJSONAtomically(path, raw)
}

func writeJSONAtomically(path string, raw []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tempPath := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".tmp")
	if err := os.WriteFile(tempPath, raw, 0o644); err != nil {
		return err
	}
	_ = os.Remove(path)
	return os.Rename(tempPath, path)
}

func sanitizeFilename(ts string) string {
	if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
		return parsed.UTC().Format("20060102T150405Z")
	}
	replacer := strings.NewReplacer(":", "", "/", "-", "?", "", "&", "", "=", "", "%", "")
	return replacer.Replace(ts)
}
