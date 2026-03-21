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
  grpc_listen_address: "127.0.0.1:4317"
  max_request_bytes: 1048576
runtime:
  flush_interval: 2s
  window_size: 15s
  max_in_memory_spans: 500
  late_span_policy: current_window
  reconciliation:
    enabled: true
    state_path: out/runtime/reconciliation-state.json
    report_path: out/runtime/reconciliation-report.json
    raw_window_path: out/runtime/raw-window.json
    stable_core_path: out/runtime/stable-core.json
    decay_half_life: 5m
    minimum_opportunity_windows: 3
    telemetry_health_freeze_threshold: 0.6
    soft_gap_multiplier: 1.25
    hard_gap_multiplier: 2.5
    minimum_soft_windows: 2
    minimum_hard_windows: 4
    stable_core_min_belief: 0.8
    stable_core_min_activity: 0.4
    guardrail_union_min_belief: 0.35
    retirement_min_belief: 0.2
    retired_ttl: 12h
    max_retained_retired_entities: 250
    compaction_interval: 90s
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
	if cfg.Server.GRPCListenAddress != "127.0.0.1:4317" {
		t.Fatalf("gRPC listen address mismatch: %s", cfg.Server.GRPCListenAddress)
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
	if !cfg.Runtime.Reconciliation.Enabled {
		t.Fatal("expected reconciliation to be enabled")
	}
	if got, want := cfg.Runtime.Reconciliation.StatePath, "out/runtime/reconciliation-state.json"; got != want {
		t.Fatalf("state path mismatch: got=%s want=%s", got, want)
	}
	if got, want := cfg.Runtime.Reconciliation.MinimumOpportunityWindows, 3; got != want {
		t.Fatalf("minimum opportunity windows mismatch: got=%d want=%d", got, want)
	}
	if got, want := cfg.Runtime.Reconciliation.TelemetryHealthFreezeThreshold, 0.6; got != want {
		t.Fatalf("telemetry health threshold mismatch: got=%v want=%v", got, want)
	}
	if got, want := cfg.Runtime.Reconciliation.CompactionInterval.Duration(), 90*time.Second; got != want {
		t.Fatalf("compaction interval mismatch: got=%s want=%s", got, want)
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

func TestServeConfigValidateRejectsMatchingHTTPAndGRPCAddresses(t *testing.T) {
	t.Parallel()

	cfg := DefaultServeConfig()
	cfg.Server.ListenAddress = "127.0.0.1:4318"
	cfg.Server.GRPCListenAddress = "127.0.0.1:4318"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for matching HTTP and gRPC addresses")
	}
}

func TestServeConfigValidateRejectsInvalidReconciliationThresholds(t *testing.T) {
	t.Parallel()

	cfg := DefaultServeConfig()
	cfg.Runtime.Reconciliation.MinimumHardWindows = cfg.Runtime.Reconciliation.MinimumSoftWindows - 1
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for invalid reconciliation thresholds")
	}
}
