package runtime

import (
	"math"
	"path"
	"path/filepath"
	"time"

	"github.com/MB3R-Lab/Bering/internal/config"
	"github.com/MB3R-Lab/Bering/internal/reconciliation"
)

func buildRuntimeReconciliationConfig(cfg config.ServeConfig) reconciliation.Config {
	rcfg := cfg.Runtime.Reconciliation
	statePath, _, _, _ := resolveReconciliationPaths(cfg)
	out := reconciliation.DefaultConfig()
	out.Enabled = rcfg.Enabled
	out.StatePath = statePath
	out.SoftGapMultiplier = rcfg.SoftGapMultiplier
	out.HardGapMultiplier = rcfg.HardGapMultiplier
	out.MinSoftWindows = rcfg.MinimumSoftWindows
	out.MinHardWindows = rcfg.MinimumHardWindows
	out.MinimumOpportunityObservations = rcfg.MinimumOpportunityWindows
	out.TelemetryFreezeThreshold = rcfg.TelemetryHealthFreezeThreshold
	out.GapEMAAlpha = halfLifeToAlpha(rcfg.DecayHalfLife.Duration(), cfg.Runtime.WindowSize.Duration(), out.GapEMAAlpha)
	out.StableCoreExistenceThreshold = rcfg.StableCoreMinBelief
	out.StableCoreActivityThreshold = rcfg.StableCoreMinActivity
	out.GuardrailUnionExistenceThreshold = rcfg.GuardrailUnionMinBelief
	out.RetirementExistenceThreshold = rcfg.RetirementMinBelief
	out.RetiredTTL = rcfg.RetiredTTL.Duration()
	out.MaxRetiredOverall = rcfg.MaxRetainedRetiredEntities
	out.CompactionCadence = durationToWindows(rcfg.CompactionInterval.Duration(), cfg.Runtime.WindowSize.Duration(), out.CompactionCadence)
	return out.WithDefaults()
}

func resolveReconciliationPaths(cfg config.ServeConfig) (string, string, string, string) {
	defaults := config.DefaultServeConfig().Runtime.Reconciliation
	baseDir := reconciliationBaseDirectory(cfg)
	if baseDir == "." || baseDir == "" {
		baseDir = "out"
	}
	resolve := func(current, defaultValue, fileName string) string {
		if current == "" || current == defaultValue {
			return filepath.Join(baseDir, fileName)
		}
		return current
	}
	return resolve(cfg.Runtime.Reconciliation.StatePath, defaults.StatePath, "reconciliation-state.json"),
		resolve(cfg.Runtime.Reconciliation.ReportPath, defaults.ReportPath, "reconciliation-report.json"),
		resolve(cfg.Runtime.Reconciliation.RawWindowPath, defaults.RawWindowPath, "latest-raw-window.json"),
		resolve(cfg.Runtime.Reconciliation.StableCorePath, defaults.StableCorePath, "latest-stable-core.json")
}

func reconciliationBaseDirectory(cfg config.ServeConfig) string {
	if latestDir := stringsTrimDir(cfg.Sink.LatestPath); latestDir != "" {
		return latestDir
	}
	sinkDir := filepath.Clean(cfg.Sink.Directory)
	if sinkDir == "." || sinkDir == "" {
		return ""
	}
	if path.Base(filepath.ToSlash(sinkDir)) == "snapshots" {
		parent := filepath.Dir(sinkDir)
		if parent != "." && parent != "" {
			return parent
		}
	}
	return sinkDir
}

func stringsTrimDir(value string) string {
	if value == "" {
		return ""
	}
	dir := filepath.Dir(value)
	if dir == "." || dir == "" {
		return ""
	}
	return dir
}

func halfLifeToAlpha(halfLife, windowSize time.Duration, fallback float64) float64 {
	if halfLife <= 0 || windowSize <= 0 {
		return fallback
	}
	windows := halfLife.Seconds() / windowSize.Seconds()
	if windows <= 0 {
		return fallback
	}
	alpha := 1 - math.Exp(-math.Ln2/windows)
	if alpha <= 0 || alpha > 1 {
		return fallback
	}
	return alpha
}

func durationToWindows(interval, windowSize time.Duration, fallback int) int {
	if interval <= 0 || windowSize <= 0 {
		return fallback
	}
	windows := int(math.Round(interval.Seconds() / windowSize.Seconds()))
	if windows < 1 {
		return 1
	}
	return windows
}
