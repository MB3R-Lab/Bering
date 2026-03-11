package runtime

import (
	"context"
	"fmt"
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

func sanitizeFilename(ts string) string {
	if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
		return parsed.UTC().Format("20060102T150405Z")
	}
	replacer := strings.NewReplacer(":", "", "/", "-", "?", "", "&", "", "=", "", "%", "")
	return replacer.Replace(ts)
}
