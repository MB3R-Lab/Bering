package reconciliation

import (
	"fmt"
	"strings"
	"time"
)

const (
	defaultFormatVersion              = 1
	defaultSoftGapMultiplier          = 1.5
	defaultHardGapMultiplier          = 3.0
	defaultMinSoftWindows             = 2
	defaultMinHardWindows             = 5
	defaultStableCoreExistence        = 0.7
	defaultStableCoreActivity         = 0.35
	defaultGuardrailUnionExistence    = 0.3
	defaultRetirementExistence        = 0.18
	defaultTelemetryFreezeThreshold   = 0.55
	defaultMinimumOpportunity         = 2
	defaultGapEMAAlpha                = 0.35
	defaultPositiveExistenceBoost     = 0.42
	defaultPositiveActivityBoost      = 0.55
	defaultPassiveActivityDecay       = 0.04
	defaultMissActivityPenalty        = 0.12
	defaultMissExistencePenalty       = 0.08
	defaultAdditionalMissPenalty      = 0.05
	defaultSourceResistanceSoftWindow = 2.0
	defaultSourceResistanceHardWindow = 4.0
	defaultRetiredTTL                 = 24 * time.Hour
	defaultCompactionCadence          = 5
	defaultMaxRetiredPerKind          = 200
	defaultMaxRetiredOverall          = 500
)

type Config struct {
	Enabled                          bool
	StatePath                        string
	SoftGapMultiplier                float64
	HardGapMultiplier                float64
	MinSoftWindows                   int
	MinHardWindows                   int
	MinimumOpportunityObservations   int
	TelemetryFreezeThreshold         float64
	GapEMAAlpha                      float64
	PositiveExistenceBoost           float64
	PositiveActivityBoost            float64
	PassiveActivityDecay             float64
	MissActivityPenalty              float64
	MissExistencePenalty             float64
	AdditionalMissPenalty            float64
	SourceResistanceSoftWindows      float64
	SourceResistanceHardWindows      float64
	StableCoreExistenceThreshold     float64
	StableCoreActivityThreshold      float64
	GuardrailUnionExistenceThreshold float64
	RetirementExistenceThreshold     float64
	RetiredTTL                       time.Duration
	CompactionCadence                int
	MaxRetiredPerKind                int
	MaxRetiredOverall                int
}

func DefaultConfig() Config {
	return Config{
		Enabled:                          true,
		SoftGapMultiplier:                defaultSoftGapMultiplier,
		HardGapMultiplier:                defaultHardGapMultiplier,
		MinSoftWindows:                   defaultMinSoftWindows,
		MinHardWindows:                   defaultMinHardWindows,
		MinimumOpportunityObservations:   defaultMinimumOpportunity,
		TelemetryFreezeThreshold:         defaultTelemetryFreezeThreshold,
		GapEMAAlpha:                      defaultGapEMAAlpha,
		PositiveExistenceBoost:           defaultPositiveExistenceBoost,
		PositiveActivityBoost:            defaultPositiveActivityBoost,
		PassiveActivityDecay:             defaultPassiveActivityDecay,
		MissActivityPenalty:              defaultMissActivityPenalty,
		MissExistencePenalty:             defaultMissExistencePenalty,
		AdditionalMissPenalty:            defaultAdditionalMissPenalty,
		SourceResistanceSoftWindows:      defaultSourceResistanceSoftWindow,
		SourceResistanceHardWindows:      defaultSourceResistanceHardWindow,
		StableCoreExistenceThreshold:     defaultStableCoreExistence,
		StableCoreActivityThreshold:      defaultStableCoreActivity,
		GuardrailUnionExistenceThreshold: defaultGuardrailUnionExistence,
		RetirementExistenceThreshold:     defaultRetirementExistence,
		RetiredTTL:                       defaultRetiredTTL,
		CompactionCadence:                defaultCompactionCadence,
		MaxRetiredPerKind:                defaultMaxRetiredPerKind,
		MaxRetiredOverall:                defaultMaxRetiredOverall,
	}
}

func (c Config) WithDefaults() Config {
	merged := c
	defaults := DefaultConfig()
	if merged.SoftGapMultiplier <= 0 {
		merged.SoftGapMultiplier = defaults.SoftGapMultiplier
	}
	if merged.HardGapMultiplier <= 0 {
		merged.HardGapMultiplier = defaults.HardGapMultiplier
	}
	if merged.MinSoftWindows <= 0 {
		merged.MinSoftWindows = defaults.MinSoftWindows
	}
	if merged.MinHardWindows <= 0 {
		merged.MinHardWindows = defaults.MinHardWindows
	}
	if merged.MinimumOpportunityObservations <= 0 {
		merged.MinimumOpportunityObservations = defaults.MinimumOpportunityObservations
	}
	if merged.TelemetryFreezeThreshold <= 0 {
		merged.TelemetryFreezeThreshold = defaults.TelemetryFreezeThreshold
	}
	if merged.GapEMAAlpha <= 0 {
		merged.GapEMAAlpha = defaults.GapEMAAlpha
	}
	if merged.PositiveExistenceBoost <= 0 {
		merged.PositiveExistenceBoost = defaults.PositiveExistenceBoost
	}
	if merged.PositiveActivityBoost <= 0 {
		merged.PositiveActivityBoost = defaults.PositiveActivityBoost
	}
	if merged.PassiveActivityDecay <= 0 {
		merged.PassiveActivityDecay = defaults.PassiveActivityDecay
	}
	if merged.MissActivityPenalty <= 0 {
		merged.MissActivityPenalty = defaults.MissActivityPenalty
	}
	if merged.MissExistencePenalty <= 0 {
		merged.MissExistencePenalty = defaults.MissExistencePenalty
	}
	if merged.AdditionalMissPenalty <= 0 {
		merged.AdditionalMissPenalty = defaults.AdditionalMissPenalty
	}
	if merged.SourceResistanceSoftWindows < 0 {
		merged.SourceResistanceSoftWindows = defaults.SourceResistanceSoftWindows
	}
	if merged.SourceResistanceHardWindows < 0 {
		merged.SourceResistanceHardWindows = defaults.SourceResistanceHardWindows
	}
	if merged.StableCoreExistenceThreshold <= 0 {
		merged.StableCoreExistenceThreshold = defaults.StableCoreExistenceThreshold
	}
	if merged.StableCoreActivityThreshold <= 0 {
		merged.StableCoreActivityThreshold = defaults.StableCoreActivityThreshold
	}
	if merged.GuardrailUnionExistenceThreshold <= 0 {
		merged.GuardrailUnionExistenceThreshold = defaults.GuardrailUnionExistenceThreshold
	}
	if merged.RetirementExistenceThreshold <= 0 {
		merged.RetirementExistenceThreshold = defaults.RetirementExistenceThreshold
	}
	if merged.RetiredTTL <= 0 {
		merged.RetiredTTL = defaults.RetiredTTL
	}
	if merged.CompactionCadence <= 0 {
		merged.CompactionCadence = defaults.CompactionCadence
	}
	if merged.MaxRetiredPerKind <= 0 {
		merged.MaxRetiredPerKind = defaults.MaxRetiredPerKind
	}
	if merged.MaxRetiredOverall <= 0 {
		merged.MaxRetiredOverall = defaults.MaxRetiredOverall
	}
	return merged
}

func (c Config) Validate() error {
	c = c.WithDefaults()
	switch {
	case c.SoftGapMultiplier <= 0:
		return fmt.Errorf("soft gap multiplier must be > 0")
	case c.HardGapMultiplier < c.SoftGapMultiplier:
		return fmt.Errorf("hard gap multiplier must be >= soft gap multiplier")
	case c.MinHardWindows < c.MinSoftWindows:
		return fmt.Errorf("min hard windows must be >= min soft windows")
	case c.MinimumOpportunityObservations <= 0:
		return fmt.Errorf("minimum opportunity observations must be > 0")
	case c.TelemetryFreezeThreshold < 0 || c.TelemetryFreezeThreshold > 1:
		return fmt.Errorf("telemetry freeze threshold must be in [0,1]")
	case c.GapEMAAlpha <= 0 || c.GapEMAAlpha > 1:
		return fmt.Errorf("gap ema alpha must be in (0,1]")
	case c.StableCoreExistenceThreshold < 0 || c.StableCoreExistenceThreshold > 1:
		return fmt.Errorf("stable core existence threshold must be in [0,1]")
	case c.StableCoreActivityThreshold < 0 || c.StableCoreActivityThreshold > 1:
		return fmt.Errorf("stable core activity threshold must be in [0,1]")
	case c.GuardrailUnionExistenceThreshold < 0 || c.GuardrailUnionExistenceThreshold > 1:
		return fmt.Errorf("guardrail union existence threshold must be in [0,1]")
	case c.RetirementExistenceThreshold < 0 || c.RetirementExistenceThreshold > 1:
		return fmt.Errorf("retirement existence threshold must be in [0,1]")
	case c.RetiredTTL <= 0:
		return fmt.Errorf("retired ttl must be > 0")
	case c.CompactionCadence <= 0:
		return fmt.Errorf("compaction cadence must be > 0")
	case c.MaxRetiredPerKind <= 0:
		return fmt.Errorf("max retired per kind must be > 0")
	case c.MaxRetiredOverall <= 0:
		return fmt.Errorf("max retired overall must be > 0")
	}
	return nil
}

func sanitizeStatePath(path string) string {
	return strings.TrimSpace(path)
}
