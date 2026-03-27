package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// DistroConfig holds configuration for a single WSL distro.
type DistroConfig struct {
	Name         string `yaml:"name"`
	Enabled      bool   `yaml:"enabled"`
	MaxRestarts  int    `yaml:"max_restarts"`
	Pause        bool   `yaml:"pause"`
	StartCommand string `yaml:"start_command"`
}

// Config holds the full wslwatch configuration.
type Config struct {
	LogLevel    string `yaml:"log_level"`
	LogFile     string `yaml:"log_file"`

	CheckInterval    time.Duration `yaml:"check_interval"`
	ProbeTimeout     time.Duration `yaml:"probe_timeout"`
	RestartDelay     time.Duration `yaml:"restart_delay"`

	FailureWindow    time.Duration `yaml:"failure_window"`
	FailureThreshold int           `yaml:"failure_threshold"`
	BackoffDuration  time.Duration `yaml:"backoff_duration"`

	Distros         []DistroConfig `yaml:"distros"`
	IgnoredDistros  []string       `yaml:"ignored_distros"`
}

// DefaultConfigPath returns the default config file path.
func DefaultConfigPath() string {
	programData := os.Getenv("ProgramData")
	if programData == "" {
		programData = `C:\ProgramData`
	}
	return filepath.Join(programData, "wslwatch", "wslwatch.yaml")
}

// DefaultConfig returns a Config with sane defaults.
func DefaultConfig() *Config {
	return &Config{
		LogLevel:         "info",
		LogFile:          "",
		CheckInterval:    15 * time.Second,
		ProbeTimeout:     8 * time.Second,
		RestartDelay:     3 * time.Second,
		FailureWindow:    60 * time.Second,
		FailureThreshold: 5,
		BackoffDuration:  0,
		Distros:          []DistroConfig{},
		IgnoredDistros:   []string{"docker-desktop", "docker-desktop-data"},
	}
}

// Load reads and parses a config file from the given path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}
	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return cfg, nil
}

// Validate checks the config for errors.
func (c *Config) Validate() error {
	validLogLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLogLevels[c.LogLevel] {
		return fmt.Errorf("invalid log_level %q: must be one of debug, info, warn, error", c.LogLevel)
	}
	if c.CheckInterval <= 0 {
		return fmt.Errorf("check_interval must be positive, got %s", c.CheckInterval)
	}
	if c.ProbeTimeout <= 0 {
		return fmt.Errorf("probe_timeout must be positive, got %s", c.ProbeTimeout)
	}
	if c.RestartDelay < 0 {
		return fmt.Errorf("restart_delay must be non-negative, got %s", c.RestartDelay)
	}
	if c.FailureWindow <= 0 {
		return fmt.Errorf("failure_window must be positive, got %s", c.FailureWindow)
	}
	if c.FailureThreshold <= 0 {
		return fmt.Errorf("failure_threshold must be positive, got %d", c.FailureThreshold)
	}
	if c.BackoffDuration < 0 {
		return fmt.Errorf("backoff_duration must be non-negative, got %s", c.BackoffDuration)
	}

	seen := make(map[string]bool)
	for i, d := range c.Distros {
		if d.Name == "" {
			return fmt.Errorf("distros[%d].name must not be empty", i)
		}
		if seen[d.Name] {
			return fmt.Errorf("duplicate distro name %q", d.Name)
		}
		seen[d.Name] = true
		if d.MaxRestarts < 0 {
			return fmt.Errorf("distros[%d].max_restarts must be non-negative, got %d", i, d.MaxRestarts)
		}
	}
	return nil
}

// Save writes the config to the given path.
func Save(cfg *Config, path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating config directory %s: %w", dir, err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing config file %s: %w", path, err)
	}
	return nil
}

// SetByKey sets a config property by dotted key path.
func (c *Config) SetByKey(key, value string) error {
	parts := strings.SplitN(key, ".", 3)
	switch parts[0] {
	case "log_level":
		c.LogLevel = value
	case "log_file":
		c.LogFile = value
	case "check_interval":
		d, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("invalid duration for check_interval: %w", err)
		}
		c.CheckInterval = d
	case "probe_timeout":
		d, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("invalid duration for probe_timeout: %w", err)
		}
		c.ProbeTimeout = d
	case "restart_delay":
		d, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("invalid duration for restart_delay: %w", err)
		}
		c.RestartDelay = d
	case "failure_window":
		d, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("invalid duration for failure_window: %w", err)
		}
		c.FailureWindow = d
	case "failure_threshold":
		var n int
		if _, err := fmt.Sscanf(value, "%d", &n); err != nil {
			return fmt.Errorf("invalid integer for failure_threshold: %w", err)
		}
		c.FailureThreshold = n
	case "backoff_duration":
		d, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("invalid duration for backoff_duration: %w", err)
		}
		c.BackoffDuration = d
	case "distros":
		// Key format: distros.<name>.<field>
		// Since distro names can contain dots, we find the field by matching
		// known distro names against the remainder after "distros."
		remainder := strings.TrimPrefix(key, "distros.")
		var matched *DistroConfig
		var field string
		for i := range c.Distros {
			prefix := c.Distros[i].Name + "."
			if strings.HasPrefix(remainder, prefix) {
				matched = &c.Distros[i]
				field = strings.TrimPrefix(remainder, prefix)
				break
			}
		}
		if matched == nil {
			return fmt.Errorf("distro not found in config (key: %q)", key)
		}
		if field == "" {
			return fmt.Errorf("distros key requires format distros.<name>.<field>")
		}
		return c.setDistroField(matched, field, value)
	default:
		return fmt.Errorf("unknown config key %q", parts[0])
	}
	return nil
}

func (c *Config) setDistroField(d *DistroConfig, field, value string) error {
	switch field {
	case "enabled":
		d.Enabled = value == "true"
	case "max_restarts":
		var n int
		if _, err := fmt.Sscanf(value, "%d", &n); err != nil {
			return fmt.Errorf("invalid integer for max_restarts: %w", err)
		}
		d.MaxRestarts = n
	case "pause":
		d.Pause = value == "true"
	case "start_command":
		d.StartCommand = value
	default:
		return fmt.Errorf("unknown distro field %q", field)
	}
	return nil
}

// IsIgnored checks whether a distro name matches the ignored list.
func (c *Config) IsIgnored(name string) bool {
	lower := strings.ToLower(name)
	for _, pattern := range c.IgnoredDistros {
		if strings.ToLower(pattern) == lower {
			return true
		}
	}
	return false
}

// FindDistro returns the DistroConfig for the given name, or nil if not found.
func (c *Config) FindDistro(name string) *DistroConfig {
	for i := range c.Distros {
		if c.Distros[i].Name == name {
			return &c.Distros[i]
		}
	}
	return nil
}
