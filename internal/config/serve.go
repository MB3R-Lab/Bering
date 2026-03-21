package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Duration time.Duration

func (d *Duration) UnmarshalJSON(data []byte) error {
	var asString string
	if err := json.Unmarshal(data, &asString); err == nil {
		parsed, err := time.ParseDuration(asString)
		if err != nil {
			return fmt.Errorf("parse duration %q: %w", asString, err)
		}
		*d = Duration(parsed)
		return nil
	}
	var asInt int64
	if err := json.Unmarshal(data, &asInt); err == nil {
		*d = Duration(time.Duration(asInt))
		return nil
	}
	return fmt.Errorf("duration must be a string or integer nanoseconds")
}

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	if value == nil {
		return nil
	}
	if value.Kind == yaml.ScalarNode {
		var asString string
		if err := value.Decode(&asString); err == nil {
			parsed, err := time.ParseDuration(asString)
			if err != nil {
				return fmt.Errorf("parse duration %q: %w", asString, err)
			}
			*d = Duration(parsed)
			return nil
		}
		var asInt int64
		if err := value.Decode(&asInt); err == nil {
			*d = Duration(time.Duration(asInt))
			return nil
		}
	}
	return fmt.Errorf("duration must be a scalar string or integer")
}

func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}

type ServeConfig struct {
	Server   ServerConfig  `json:"server" yaml:"server"`
	Runtime  RuntimeConfig `json:"runtime" yaml:"runtime"`
	Sink     SinkConfig    `json:"sink" yaml:"sink"`
	Logging  LoggingConfig `json:"logging" yaml:"logging"`
	Overlays []string      `json:"overlays" yaml:"overlays"`
}

type ServerConfig struct {
	ListenAddress     string `json:"listen_address" yaml:"listen_address"`
	GRPCListenAddress string `json:"grpc_listen_address" yaml:"grpc_listen_address"`
	MaxRequestBytes   int64  `json:"max_request_bytes" yaml:"max_request_bytes"`
}

type RuntimeConfig struct {
	FlushInterval    Duration                    `json:"flush_interval" yaml:"flush_interval"`
	WindowSize       Duration                    `json:"window_size" yaml:"window_size"`
	MaxInMemorySpans int                         `json:"max_in_memory_spans" yaml:"max_in_memory_spans"`
	LateSpanPolicy   string                      `json:"late_span_policy" yaml:"late_span_policy"`
	Reconciliation   RuntimeReconciliationConfig `json:"reconciliation" yaml:"reconciliation"`
}

type RuntimeReconciliationConfig struct {
	Enabled                        bool     `json:"enabled" yaml:"enabled"`
	StatePath                      string   `json:"state_path" yaml:"state_path"`
	ReportPath                     string   `json:"report_path" yaml:"report_path"`
	RawWindowPath                  string   `json:"raw_window_path" yaml:"raw_window_path"`
	StableCorePath                 string   `json:"stable_core_path" yaml:"stable_core_path"`
	DecayHalfLife                  Duration `json:"decay_half_life" yaml:"decay_half_life"`
	MinimumOpportunityWindows      int      `json:"minimum_opportunity_windows" yaml:"minimum_opportunity_windows"`
	TelemetryHealthFreezeThreshold float64  `json:"telemetry_health_freeze_threshold" yaml:"telemetry_health_freeze_threshold"`
	SoftGapMultiplier              float64  `json:"soft_gap_multiplier" yaml:"soft_gap_multiplier"`
	HardGapMultiplier              float64  `json:"hard_gap_multiplier" yaml:"hard_gap_multiplier"`
	MinimumSoftWindows             int      `json:"minimum_soft_windows" yaml:"minimum_soft_windows"`
	MinimumHardWindows             int      `json:"minimum_hard_windows" yaml:"minimum_hard_windows"`
	StableCoreMinBelief            float64  `json:"stable_core_min_belief" yaml:"stable_core_min_belief"`
	StableCoreMinActivity          float64  `json:"stable_core_min_activity" yaml:"stable_core_min_activity"`
	GuardrailUnionMinBelief        float64  `json:"guardrail_union_min_belief" yaml:"guardrail_union_min_belief"`
	RetirementMinBelief            float64  `json:"retirement_min_belief" yaml:"retirement_min_belief"`
	RetiredTTL                     Duration `json:"retired_ttl" yaml:"retired_ttl"`
	MaxRetainedRetiredEntities     int      `json:"max_retained_retired_entities" yaml:"max_retained_retired_entities"`
	CompactionInterval             Duration `json:"compaction_interval" yaml:"compaction_interval"`
}

type SinkConfig struct {
	Directory  string `json:"directory" yaml:"directory"`
	LatestPath string `json:"latest_path" yaml:"latest_path"`
}

type LoggingConfig struct {
	Structured bool `json:"structured" yaml:"structured"`
}

func DefaultServeConfig() ServeConfig {
	return ServeConfig{
		Server: ServerConfig{
			ListenAddress:     ":8080",
			GRPCListenAddress: "",
			MaxRequestBytes:   5 << 20,
		},
		Runtime: RuntimeConfig{
			FlushInterval:    Duration(5 * time.Second),
			WindowSize:       Duration(30 * time.Second),
			MaxInMemorySpans: 10000,
			LateSpanPolicy:   "drop",
			Reconciliation: RuntimeReconciliationConfig{
				Enabled:                        true,
				StatePath:                      "out/reconciliation-state.json",
				ReportPath:                     "out/reconciliation-report.json",
				RawWindowPath:                  "out/latest-raw-window.json",
				StableCorePath:                 "out/latest-stable-core.json",
				DecayHalfLife:                  Duration(10 * time.Minute),
				MinimumOpportunityWindows:      2,
				TelemetryHealthFreezeThreshold: 0.55,
				SoftGapMultiplier:              1.5,
				HardGapMultiplier:              3.0,
				MinimumSoftWindows:             2,
				MinimumHardWindows:             5,
				StableCoreMinBelief:            0.70,
				StableCoreMinActivity:          0.35,
				GuardrailUnionMinBelief:        0.30,
				RetirementMinBelief:            0.18,
				RetiredTTL:                     Duration(24 * time.Hour),
				MaxRetainedRetiredEntities:     500,
				CompactionInterval:             Duration(150 * time.Second),
			},
		},
		Sink: SinkConfig{
			Directory:  "out/snapshots",
			LatestPath: "out/latest-snapshot.json",
		},
	}
}

func LoadServeConfig(path string) (ServeConfig, error) {
	cfg := DefaultServeConfig()
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return cfg, cfg.Validate()
	}
	raw, err := os.ReadFile(trimmed)
	if err != nil {
		return ServeConfig{}, fmt.Errorf("read config file: %w", err)
	}
	switch strings.ToLower(filepath.Ext(trimmed)) {
	case ".json":
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return ServeConfig{}, fmt.Errorf("decode config json: %w", err)
		}
	default:
		if err := yaml.Unmarshal(raw, &cfg); err != nil {
			return ServeConfig{}, fmt.Errorf("decode config yaml: %w", err)
		}
	}
	return cfg, cfg.Validate()
}

func (c ServeConfig) Validate() error {
	if strings.TrimSpace(c.Server.ListenAddress) == "" {
		return fmt.Errorf("server.listen_address cannot be empty")
	}
	if grpcAddress := strings.TrimSpace(c.Server.GRPCListenAddress); grpcAddress != "" && grpcAddress == strings.TrimSpace(c.Server.ListenAddress) {
		return fmt.Errorf("server.grpc_listen_address must differ from server.listen_address")
	}
	if c.Server.MaxRequestBytes <= 0 {
		return fmt.Errorf("server.max_request_bytes must be > 0")
	}
	if c.Runtime.FlushInterval.Duration() <= 0 {
		return fmt.Errorf("runtime.flush_interval must be > 0")
	}
	if c.Runtime.WindowSize.Duration() <= 0 {
		return fmt.Errorf("runtime.window_size must be > 0")
	}
	if c.Runtime.MaxInMemorySpans <= 0 {
		return fmt.Errorf("runtime.max_in_memory_spans must be > 0")
	}
	switch strings.ToLower(strings.TrimSpace(c.Runtime.LateSpanPolicy)) {
	case "drop", "current_window":
	default:
		return fmt.Errorf("runtime.late_span_policy must be one of: drop, current_window")
	}
	if err := c.Runtime.Reconciliation.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(c.Sink.Directory) == "" {
		return fmt.Errorf("sink.directory cannot be empty")
	}
	return nil
}

func (c RuntimeReconciliationConfig) Validate() error {
	if c.DecayHalfLife.Duration() <= 0 {
		return fmt.Errorf("runtime.reconciliation.decay_half_life must be > 0")
	}
	if c.MinimumOpportunityWindows <= 0 {
		return fmt.Errorf("runtime.reconciliation.minimum_opportunity_windows must be > 0")
	}
	if c.TelemetryHealthFreezeThreshold < 0 || c.TelemetryHealthFreezeThreshold > 1 {
		return fmt.Errorf("runtime.reconciliation.telemetry_health_freeze_threshold must be in [0,1]")
	}
	if c.SoftGapMultiplier <= 0 {
		return fmt.Errorf("runtime.reconciliation.soft_gap_multiplier must be > 0")
	}
	if c.HardGapMultiplier < c.SoftGapMultiplier {
		return fmt.Errorf("runtime.reconciliation.hard_gap_multiplier must be >= runtime.reconciliation.soft_gap_multiplier")
	}
	if c.MinimumSoftWindows <= 0 {
		return fmt.Errorf("runtime.reconciliation.minimum_soft_windows must be > 0")
	}
	if c.MinimumHardWindows < c.MinimumSoftWindows {
		return fmt.Errorf("runtime.reconciliation.minimum_hard_windows must be >= runtime.reconciliation.minimum_soft_windows")
	}
	for _, item := range []struct {
		name  string
		value float64
	}{
		{"runtime.reconciliation.stable_core_min_belief", c.StableCoreMinBelief},
		{"runtime.reconciliation.stable_core_min_activity", c.StableCoreMinActivity},
		{"runtime.reconciliation.guardrail_union_min_belief", c.GuardrailUnionMinBelief},
		{"runtime.reconciliation.retirement_min_belief", c.RetirementMinBelief},
	} {
		if item.value < 0 || item.value > 1 {
			return fmt.Errorf("%s must be in [0,1]", item.name)
		}
	}
	if c.StableCoreMinBelief < c.GuardrailUnionMinBelief {
		return fmt.Errorf("runtime.reconciliation.stable_core_min_belief must be >= runtime.reconciliation.guardrail_union_min_belief")
	}
	if c.RetiredTTL.Duration() <= 0 {
		return fmt.Errorf("runtime.reconciliation.retired_ttl must be > 0")
	}
	if c.MaxRetainedRetiredEntities <= 0 {
		return fmt.Errorf("runtime.reconciliation.max_retained_retired_entities must be > 0")
	}
	if c.CompactionInterval.Duration() <= 0 {
		return fmt.Errorf("runtime.reconciliation.compaction_interval must be > 0")
	}
	return nil
}
