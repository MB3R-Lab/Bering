package runtime

import (
	"path/filepath"
	"testing"

	"github.com/MB3R-Lab/Bering/internal/config"
)

func TestResolveReconciliationPathsUsesLatestSnapshotDirectory(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultServeConfig()
	cfg.Sink.Directory = filepath.Join("var", "lib", "bering", "snapshots")
	cfg.Sink.LatestPath = filepath.Join("var", "lib", "bering", "latest-snapshot.json")

	statePath, reportPath, summaryPath, rawPath, stablePath := resolveReconciliationPaths(cfg)
	signalQualityPath := resolveSignalQualityPath(cfg)
	base := filepath.Join("var", "lib", "bering")
	assertEqualPath(t, statePath, filepath.Join(base, "reconciliation-state.json"))
	assertEqualPath(t, reportPath, filepath.Join(base, "reconciliation-report.json"))
	assertEqualPath(t, summaryPath, filepath.Join(base, "reconciliation-summary.md"))
	assertEqualPath(t, rawPath, filepath.Join(base, "latest-raw-window.json"))
	assertEqualPath(t, stablePath, filepath.Join(base, "latest-stable-core.json"))
	assertEqualPath(t, signalQualityPath, filepath.Join(base, "latest-signal-quality.json"))
}

func TestResolveReconciliationPathsFallsBackToSinkDirectory(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultServeConfig()
	cfg.Sink.Directory = filepath.Join("var", "lib", "bering")
	cfg.Sink.LatestPath = ""

	statePath, _, _, _, _ := resolveReconciliationPaths(cfg)
	assertEqualPath(t, statePath, filepath.Join("var", "lib", "bering", "reconciliation-state.json"))
}

func TestResolveReconciliationPathsPreservesExplicitOverrides(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultServeConfig()
	cfg.Runtime.Reconciliation.StatePath = filepath.Join("custom", "state.json")
	cfg.Runtime.Reconciliation.ReportPath = filepath.Join("custom", "report.json")
	cfg.Runtime.Reconciliation.SummaryPath = filepath.Join("custom", "summary.md")
	cfg.Runtime.Reconciliation.RawWindowPath = filepath.Join("custom", "raw.json")
	cfg.Runtime.Reconciliation.StableCorePath = filepath.Join("custom", "stable.json")
	cfg.Sink.SignalQualityPath = filepath.Join("custom", "quality.json")

	statePath, reportPath, summaryPath, rawPath, stablePath := resolveReconciliationPaths(cfg)
	signalQualityPath := resolveSignalQualityPath(cfg)
	assertEqualPath(t, statePath, filepath.Join("custom", "state.json"))
	assertEqualPath(t, reportPath, filepath.Join("custom", "report.json"))
	assertEqualPath(t, summaryPath, filepath.Join("custom", "summary.md"))
	assertEqualPath(t, rawPath, filepath.Join("custom", "raw.json"))
	assertEqualPath(t, stablePath, filepath.Join("custom", "stable.json"))
	assertEqualPath(t, signalQualityPath, filepath.Join("custom", "quality.json"))
}

func assertEqualPath(t *testing.T, got, want string) {
	t.Helper()
	if filepath.Clean(got) != filepath.Clean(want) {
		t.Fatalf("path mismatch: got=%s want=%s", got, want)
	}
}
