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
	Server  ServerConfig  `json:"server" yaml:"server"`
	Runtime RuntimeConfig `json:"runtime" yaml:"runtime"`
	Sink    SinkConfig    `json:"sink" yaml:"sink"`
	Logging LoggingConfig `json:"logging" yaml:"logging"`
	Overlays []string     `json:"overlays" yaml:"overlays"`
}

type ServerConfig struct {
	ListenAddress   string `json:"listen_address" yaml:"listen_address"`
	MaxRequestBytes int64  `json:"max_request_bytes" yaml:"max_request_bytes"`
}

type RuntimeConfig struct {
	FlushInterval     Duration `json:"flush_interval" yaml:"flush_interval"`
	WindowSize        Duration `json:"window_size" yaml:"window_size"`
	MaxInMemorySpans  int      `json:"max_in_memory_spans" yaml:"max_in_memory_spans"`
	LateSpanPolicy    string   `json:"late_span_policy" yaml:"late_span_policy"`
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
			ListenAddress:   ":8080",
			MaxRequestBytes: 5 << 20,
		},
		Runtime: RuntimeConfig{
			FlushInterval:    Duration(5 * time.Second),
			WindowSize:       Duration(30 * time.Second),
			MaxInMemorySpans: 10000,
			LateSpanPolicy:   "drop",
		},
		Sink: SinkConfig{
			Directory: "out/snapshots",
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
	if strings.TrimSpace(c.Sink.Directory) == "" {
		return fmt.Errorf("sink.directory cannot be empty")
	}
	return nil
}
