package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadServeConfigYAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "serve.yaml")
	raw := []byte(`server:
  listen_address: "127.0.0.1:4318"
  max_request_bytes: 1048576
runtime:
  flush_interval: 2s
  window_size: 15s
  max_in_memory_spans: 500
  late_span_policy: current_window
sink:
  directory: out/runtime
  latest_path: out/runtime/latest.json
logging:
  structured: true
overlays:
  - configs/discovery.overlay.yaml
`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	cfg, err := LoadServeConfig(path)
	if err != nil {
		t.Fatalf("LoadServeConfig returned error: %v", err)
	}
	if cfg.Server.ListenAddress != "127.0.0.1:4318" {
		t.Fatalf("listen address mismatch: %s", cfg.Server.ListenAddress)
	}
	if got, want := cfg.Runtime.FlushInterval.Duration(), 2*time.Second; got != want {
		t.Fatalf("flush interval mismatch: got=%s want=%s", got, want)
	}
	if got, want := cfg.Runtime.WindowSize.Duration(), 15*time.Second; got != want {
		t.Fatalf("window size mismatch: got=%s want=%s", got, want)
	}
	if got, want := cfg.Runtime.LateSpanPolicy, "current_window"; got != want {
		t.Fatalf("late span policy mismatch: got=%s want=%s", got, want)
	}
	if !cfg.Logging.Structured {
		t.Fatal("expected structured logging to be enabled")
	}
	if got, want := len(cfg.Overlays), 1; got != want {
		t.Fatalf("overlay count mismatch: got=%d want=%d", got, want)
	}
}

func TestServeConfigValidateRejectsInvalidLatePolicy(t *testing.T) {
	t.Parallel()

	cfg := DefaultServeConfig()
	cfg.Runtime.LateSpanPolicy = "unknown"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for invalid late policy")
	}
}
