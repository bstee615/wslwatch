package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration wraps time.Duration to support YAML marshal/unmarshal as human-readable strings.
type Duration struct {
	time.Duration
}

// UnmarshalYAML parses a duration from a YAML scalar. Strings like "15s", "3m",
// "1h" are parsed with time.ParseDuration. Plain integers are treated as nanoseconds
// for backward compatibility.
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.ScalarNode {
		return fmt.Errorf("duration must be a scalar, got %v", value.Kind)
	}
	raw := value.Value

	// Try numeric first (nanoseconds, backward compat).
	if ns, err := strconv.ParseInt(raw, 10, 64); err == nil {
		d.Duration = time.Duration(ns)
		return nil
	}

	dur, err := time.ParseDuration(raw)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", raw, err)
	}
	d.Duration = dur
	return nil
}

// MarshalYAML encodes the duration as a human-readable string (e.g. "15s").
func (d Duration) MarshalYAML() (interface{}, error) {
	return d.Duration.String(), nil
}

// DistroConfig holds per-distro watchdog configuration.
type DistroConfig struct {
	Name         string `yaml:"name"`
	Enabled      bool   `yaml:"enabled"`
	MaxRestarts  int    `yaml:"max_restarts"` // 0 = unlimited
	Pause        bool   `yaml:"pause"`
	StartCommand string `yaml:"start_command"`
}

// Config is the top-level wslwatch configuration structure.
type Config struct {
	LogLevel         string        `yaml:"log_level"`         // debug|info|warn|error
	LogFile          string        `yaml:"log_file"`          // empty = stdout
	CheckInterval    Duration      `yaml:"check_interval"`    // default 15s
	ProbeTimeout     Duration      `yaml:"probe_timeout"`     // default 8s
	RestartDelay     Duration      `yaml:"restart_delay"`     // default 3s
	FailureWindow    Duration      `yaml:"failure_window"`    // default 60s
	FailureThreshold int           `yaml:"failure_threshold"` // default 5
	BackoffDuration  Duration      `yaml:"backoff_duration"`  // default 0s
	Distros          []DistroConfig `yaml:"distros"`
	IgnoredDistros   []string      `yaml:"ignored_distros"`
}

// Default returns a Config populated with sane defaults.
func Default() *Config {
	return &Config{
		LogLevel:         "info",
		CheckInterval:    Duration{15 * time.Second},
		ProbeTimeout:     Duration{8 * time.Second},
		RestartDelay:     Duration{3 * time.Second},
		FailureWindow:    Duration{60 * time.Second},
		FailureThreshold: 5,
		BackoffDuration:  Duration{0},
		IgnoredDistros:   []string{"docker-desktop", "docker-desktop-data"},
	}
}

// DefaultPath returns the default path for the wslwatch configuration file.
func DefaultPath() string {
	programData := os.Getenv("ProgramData")
	if programData == "" {
		programData = `C:\ProgramData`
	}
	return filepath.Join(programData, "wslwatch", "wslwatch.yaml")
}

// Load reads and parses the YAML configuration file at path. If path is empty
// the default path is used. Zero values in the loaded file are replaced with
// the defaults from Default().
func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultPath()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %q: %w", path, err)
	}

	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %q: %w", path, err)
	}

	// Apply defaults for zero values.
	defaults := Default()
	if cfg.LogLevel == "" {
		cfg.LogLevel = defaults.LogLevel
	}
	if cfg.CheckInterval.Duration == 0 {
		cfg.CheckInterval = defaults.CheckInterval
	}
	if cfg.ProbeTimeout.Duration == 0 {
		cfg.ProbeTimeout = defaults.ProbeTimeout
	}
	if cfg.RestartDelay.Duration == 0 {
		cfg.RestartDelay = defaults.RestartDelay
	}
	if cfg.FailureWindow.Duration == 0 {
		cfg.FailureWindow = defaults.FailureWindow
	}
	if cfg.FailureThreshold == 0 {
		cfg.FailureThreshold = defaults.FailureThreshold
	}
	if cfg.IgnoredDistros == nil {
		cfg.IgnoredDistros = defaults.IgnoredDistros
	}

	return cfg, nil
}

// Validate checks that the configuration values are within acceptable ranges.
func (c *Config) Validate() error {
	var errs []error

	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[c.LogLevel] {
		errs = append(errs, fmt.Errorf("log_level must be one of debug|info|warn|error, got %q", c.LogLevel))
	}

	if c.CheckInterval.Duration < time.Second {
		errs = append(errs, fmt.Errorf("check_interval must be >= 1s, got %s", c.CheckInterval.Duration))
	}

	if c.ProbeTimeout.Duration < time.Second {
		errs = append(errs, fmt.Errorf("probe_timeout must be >= 1s, got %s", c.ProbeTimeout.Duration))
	}

	if c.RestartDelay.Duration < 0 {
		errs = append(errs, fmt.Errorf("restart_delay must be >= 0, got %s", c.RestartDelay.Duration))
	}

	if c.FailureThreshold < 1 {
		errs = append(errs, fmt.Errorf("failure_threshold must be >= 1, got %d", c.FailureThreshold))
	}

	seen := make(map[string]bool)
	for i, d := range c.Distros {
		if d.Name == "" {
			errs = append(errs, fmt.Errorf("distros[%d]: name must not be empty", i))
			continue
		}
		if seen[d.Name] {
			errs = append(errs, fmt.Errorf("duplicate distro name %q", d.Name))
		}
		seen[d.Name] = true
	}

	return errors.Join(errs...)
}

// Save writes the configuration to path as YAML, creating parent directories
// as needed.
func (c *Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing config file %q: %w", path, err)
	}
	return nil
}

// SetByKey sets a single configuration field identified by a dotted key path.
// Supported keys:
//
//	log_level, log_file
//	check_interval, probe_timeout, restart_delay
//	failure_window, failure_threshold, backoff_duration
//	distros.<name>.enabled, distros.<name>.max_restarts,
//	distros.<name>.pause, distros.<name>.start_command
func (c *Config) SetByKey(key string, value string) error {
	switch key {
	case "log_level":
		c.LogLevel = value
	case "log_file":
		c.LogFile = value
	case "check_interval":
		d, err := parseDuration(key, value)
		if err != nil {
			return err
		}
		c.CheckInterval = d
	case "probe_timeout":
		d, err := parseDuration(key, value)
		if err != nil {
			return err
		}
		c.ProbeTimeout = d
	case "restart_delay":
		d, err := parseDuration(key, value)
		if err != nil {
			return err
		}
		c.RestartDelay = d
	case "failure_window":
		d, err := parseDuration(key, value)
		if err != nil {
			return err
		}
		c.FailureWindow = d
	case "failure_threshold":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("failure_threshold: expected integer, got %q", value)
		}
		c.FailureThreshold = n
	case "backoff_duration":
		d, err := parseDuration(key, value)
		if err != nil {
			return err
		}
		c.BackoffDuration = d
	default:
		// Handle distros.<name>.<field>
		parts := strings.SplitN(key, ".", 3)
		if len(parts) == 3 && parts[0] == "distros" {
			return c.setDistroField(parts[1], parts[2], value)
		}
		return fmt.Errorf("unknown config key %q", key)
	}
	return nil
}

// setDistroField sets a field on the named distro entry, creating it if absent.
func (c *Config) setDistroField(name, field, value string) error {
	idx := -1
	for i, d := range c.Distros {
		if d.Name == name {
			idx = i
			break
		}
	}
	if idx == -1 {
		c.Distros = append(c.Distros, DistroConfig{Name: name, Enabled: true})
		idx = len(c.Distros) - 1
	}

	switch field {
	case "enabled":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("distros.%s.enabled: expected bool, got %q", name, value)
		}
		c.Distros[idx].Enabled = b
	case "max_restarts":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("distros.%s.max_restarts: expected integer, got %q", name, value)
		}
		c.Distros[idx].MaxRestarts = n
	case "pause":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("distros.%s.pause: expected bool, got %q", name, value)
		}
		c.Distros[idx].Pause = b
	case "start_command":
		c.Distros[idx].StartCommand = value
	default:
		return fmt.Errorf("unknown distro field %q", field)
	}
	return nil
}

// parseDuration is a helper that returns a Duration from a string value.
func parseDuration(key, value string) (Duration, error) {
	d, err := time.ParseDuration(value)
	if err != nil {
		return Duration{}, fmt.Errorf("%s: invalid duration %q: %w", key, value, err)
	}
	return Duration{d}, nil
}
