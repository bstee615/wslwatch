package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// ---------- TestDefault ----------

func TestDefault(t *testing.T) {
	cfg := Default()
	require.NotNil(t, cfg)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, 15*time.Second, cfg.CheckInterval.Duration)
	assert.Equal(t, 8*time.Second, cfg.ProbeTimeout.Duration)
	assert.Equal(t, 3*time.Second, cfg.RestartDelay.Duration)
	assert.Equal(t, 60*time.Second, cfg.FailureWindow.Duration)
	assert.Equal(t, 5, cfg.FailureThreshold)
	assert.Equal(t, time.Duration(0), cfg.BackoffDuration.Duration)
	assert.Equal(t, []string{"docker-desktop", "docker-desktop-data"}, cfg.IgnoredDistros)
}

// ---------- TestLoad ----------

func TestLoad(t *testing.T) {
	t.Run("valid file", func(t *testing.T) {
		content := `
log_level: debug
check_interval: 30s
probe_timeout: 5s
restart_delay: 2s
failure_window: 120s
failure_threshold: 3
backoff_duration: 10s
distros:
  - name: Ubuntu
    enabled: true
    max_restarts: 5
ignored_distros:
  - my-ignored
`
		path := writeTempFile(t, content)
		cfg, err := Load(path)
		require.NoError(t, err)

		assert.Equal(t, "debug", cfg.LogLevel)
		assert.Equal(t, 30*time.Second, cfg.CheckInterval.Duration)
		assert.Equal(t, 5*time.Second, cfg.ProbeTimeout.Duration)
		assert.Equal(t, 2*time.Second, cfg.RestartDelay.Duration)
		assert.Equal(t, 120*time.Second, cfg.FailureWindow.Duration)
		assert.Equal(t, 3, cfg.FailureThreshold)
		assert.Equal(t, 10*time.Second, cfg.BackoffDuration.Duration)
		require.Len(t, cfg.Distros, 1)
		assert.Equal(t, "Ubuntu", cfg.Distros[0].Name)
		assert.True(t, cfg.Distros[0].Enabled)
		assert.Equal(t, 5, cfg.Distros[0].MaxRestarts)
		assert.Equal(t, []string{"my-ignored"}, cfg.IgnoredDistros)
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := Load("/nonexistent/path/wslwatch.yaml")
		require.Error(t, err)
	})

	t.Run("invalid yaml", func(t *testing.T) {
		path := writeTempFile(t, "log_level: [this is: invalid yaml\n")
		_, err := Load(path)
		require.Error(t, err)
	})

	t.Run("zero values get defaults", func(t *testing.T) {
		// File with only log_level set; everything else should get default.
		path := writeTempFile(t, "log_level: warn\n")
		cfg, err := Load(path)
		require.NoError(t, err)
		assert.Equal(t, "warn", cfg.LogLevel)
		assert.Equal(t, 15*time.Second, cfg.CheckInterval.Duration)
		assert.Equal(t, 5, cfg.FailureThreshold)
	})
}

// ---------- TestValidate ----------

func TestValidate(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		cfg := Default()
		assert.NoError(t, cfg.Validate())
	})

	t.Run("invalid log level", func(t *testing.T) {
		cfg := Default()
		cfg.LogLevel = "verbose"
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "log_level")
	})

	t.Run("short check_interval", func(t *testing.T) {
		cfg := Default()
		cfg.CheckInterval = Duration{500 * time.Millisecond}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "check_interval")
	})

	t.Run("short probe_timeout", func(t *testing.T) {
		cfg := Default()
		cfg.ProbeTimeout = Duration{0}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "probe_timeout")
	})

	t.Run("failure_threshold zero", func(t *testing.T) {
		cfg := Default()
		cfg.FailureThreshold = 0
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failure_threshold")
	})

	t.Run("empty distro name", func(t *testing.T) {
		cfg := Default()
		cfg.Distros = []DistroConfig{{Name: ""}}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name must not be empty")
	})

	t.Run("duplicate distros", func(t *testing.T) {
		cfg := Default()
		cfg.Distros = []DistroConfig{
			{Name: "Ubuntu"},
			{Name: "Ubuntu"},
		}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate distro name")
	})

	t.Run("multiple errors joined", func(t *testing.T) {
		cfg := Default()
		cfg.LogLevel = "bad"
		cfg.FailureThreshold = 0
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "log_level")
		assert.Contains(t, err.Error(), "failure_threshold")
	})
}

// ---------- TestSave (roundtrip) ----------

func TestSave(t *testing.T) {
	original := Default()
	original.LogLevel = "debug"
	original.CheckInterval = Duration{20 * time.Second}
	original.Distros = []DistroConfig{
		{Name: "Ubuntu", Enabled: true, MaxRestarts: 3},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "wslwatch.yaml")

	err := original.Save(path)
	require.NoError(t, err)

	loaded, err := Load(path)
	require.NoError(t, err)

	assert.Equal(t, original.LogLevel, loaded.LogLevel)
	assert.Equal(t, original.CheckInterval.Duration, loaded.CheckInterval.Duration)
	require.Len(t, loaded.Distros, 1)
	assert.Equal(t, original.Distros[0].Name, loaded.Distros[0].Name)
	assert.Equal(t, original.Distros[0].MaxRestarts, loaded.Distros[0].MaxRestarts)
}

// ---------- TestSetByKey ----------

func TestSetByKey(t *testing.T) {
	t.Run("log_level", func(t *testing.T) {
		cfg := Default()
		require.NoError(t, cfg.SetByKey("log_level", "warn"))
		assert.Equal(t, "warn", cfg.LogLevel)
	})

	t.Run("log_file", func(t *testing.T) {
		cfg := Default()
		require.NoError(t, cfg.SetByKey("log_file", "/var/log/wslwatch.log"))
		assert.Equal(t, "/var/log/wslwatch.log", cfg.LogFile)
	})

	t.Run("check_interval", func(t *testing.T) {
		cfg := Default()
		require.NoError(t, cfg.SetByKey("check_interval", "30s"))
		assert.Equal(t, 30*time.Second, cfg.CheckInterval.Duration)
	})

	t.Run("probe_timeout", func(t *testing.T) {
		cfg := Default()
		require.NoError(t, cfg.SetByKey("probe_timeout", "10s"))
		assert.Equal(t, 10*time.Second, cfg.ProbeTimeout.Duration)
	})

	t.Run("restart_delay", func(t *testing.T) {
		cfg := Default()
		require.NoError(t, cfg.SetByKey("restart_delay", "5s"))
		assert.Equal(t, 5*time.Second, cfg.RestartDelay.Duration)
	})

	t.Run("failure_window", func(t *testing.T) {
		cfg := Default()
		require.NoError(t, cfg.SetByKey("failure_window", "2m"))
		assert.Equal(t, 2*time.Minute, cfg.FailureWindow.Duration)
	})

	t.Run("failure_threshold", func(t *testing.T) {
		cfg := Default()
		require.NoError(t, cfg.SetByKey("failure_threshold", "10"))
		assert.Equal(t, 10, cfg.FailureThreshold)
	})

	t.Run("failure_threshold invalid", func(t *testing.T) {
		cfg := Default()
		err := cfg.SetByKey("failure_threshold", "notanumber")
		require.Error(t, err)
	})

	t.Run("backoff_duration", func(t *testing.T) {
		cfg := Default()
		require.NoError(t, cfg.SetByKey("backoff_duration", "1m"))
		assert.Equal(t, time.Minute, cfg.BackoffDuration.Duration)
	})

	t.Run("distros.<name>.enabled new distro", func(t *testing.T) {
		cfg := Default()
		require.NoError(t, cfg.SetByKey("distros.Ubuntu.enabled", "false"))
		require.Len(t, cfg.Distros, 1)
		assert.Equal(t, "Ubuntu", cfg.Distros[0].Name)
		assert.False(t, cfg.Distros[0].Enabled)
	})

	t.Run("distros.<name>.max_restarts", func(t *testing.T) {
		cfg := Default()
		cfg.Distros = []DistroConfig{{Name: "Ubuntu", Enabled: true}}
		require.NoError(t, cfg.SetByKey("distros.Ubuntu.max_restarts", "7"))
		assert.Equal(t, 7, cfg.Distros[0].MaxRestarts)
	})

	t.Run("distros.<name>.max_restarts invalid", func(t *testing.T) {
		cfg := Default()
		err := cfg.SetByKey("distros.Ubuntu.max_restarts", "bad")
		require.Error(t, err)
	})

	t.Run("distros.<name>.pause", func(t *testing.T) {
		cfg := Default()
		cfg.Distros = []DistroConfig{{Name: "Ubuntu"}}
		require.NoError(t, cfg.SetByKey("distros.Ubuntu.pause", "true"))
		assert.True(t, cfg.Distros[0].Pause)
	})

	t.Run("distros.<name>.start_command", func(t *testing.T) {
		cfg := Default()
		cfg.Distros = []DistroConfig{{Name: "Ubuntu"}}
		require.NoError(t, cfg.SetByKey("distros.Ubuntu.start_command", "wsl -d Ubuntu"))
		assert.Equal(t, "wsl -d Ubuntu", cfg.Distros[0].StartCommand)
	})

	t.Run("distros.<name>.unknown_field", func(t *testing.T) {
		cfg := Default()
		err := cfg.SetByKey("distros.Ubuntu.nonexistent", "value")
		require.Error(t, err)
	})

	t.Run("distro name with dots", func(t *testing.T) {
		cfg := Default()
		cfg.Distros = []DistroConfig{{Name: "Ubuntu.22.04"}}
		require.NoError(t, cfg.SetByKey("distros.Ubuntu.22.04.max_restarts", "3"))
		assert.Equal(t, 3, cfg.Distros[0].MaxRestarts)
	})

	t.Run("distro name with dots pause", func(t *testing.T) {
		cfg := Default()
		cfg.Distros = []DistroConfig{{Name: "My.Distro.Name"}}
		require.NoError(t, cfg.SetByKey("distros.My.Distro.Name.pause", "true"))
		assert.True(t, cfg.Distros[0].Pause)
	})

	t.Run("distros key missing field", func(t *testing.T) {
		cfg := Default()
		err := cfg.SetByKey("distros.Ubuntu", "value")
		require.Error(t, err)
	})

	t.Run("unknown top-level key", func(t *testing.T) {
		cfg := Default()
		err := cfg.SetByKey("nonexistent_key", "value")
		require.Error(t, err)
	})
}

// ---------- TestDurationYAML ----------

func TestDurationYAML(t *testing.T) {
	t.Run("unmarshal string", func(t *testing.T) {
		var d Duration
		node := &yaml.Node{Kind: yaml.ScalarNode, Value: "15s"}
		require.NoError(t, d.UnmarshalYAML(node))
		assert.Equal(t, 15*time.Second, d.Duration)
	})

	t.Run("unmarshal minutes", func(t *testing.T) {
		var d Duration
		node := &yaml.Node{Kind: yaml.ScalarNode, Value: "3m"}
		require.NoError(t, d.UnmarshalYAML(node))
		assert.Equal(t, 3*time.Minute, d.Duration)
	})

	t.Run("unmarshal hours", func(t *testing.T) {
		var d Duration
		node := &yaml.Node{Kind: yaml.ScalarNode, Value: "1h"}
		require.NoError(t, d.UnmarshalYAML(node))
		assert.Equal(t, time.Hour, d.Duration)
	})

	t.Run("unmarshal numeric nanoseconds", func(t *testing.T) {
		var d Duration
		node := &yaml.Node{Kind: yaml.ScalarNode, Value: "1000000000"}
		require.NoError(t, d.UnmarshalYAML(node))
		assert.Equal(t, time.Second, d.Duration)
	})

	t.Run("unmarshal invalid", func(t *testing.T) {
		var d Duration
		node := &yaml.Node{Kind: yaml.ScalarNode, Value: "notaduration"}
		require.Error(t, d.UnmarshalYAML(node))
	})

	t.Run("unmarshal wrong node kind", func(t *testing.T) {
		var d Duration
		node := &yaml.Node{Kind: yaml.MappingNode}
		require.Error(t, d.UnmarshalYAML(node))
	})

	t.Run("marshal roundtrip", func(t *testing.T) {
		d := Duration{30 * time.Second}
		v, err := d.MarshalYAML()
		require.NoError(t, err)
		assert.Equal(t, "30s", v)

		// Roundtrip via yaml.Marshal / yaml.Unmarshal on a struct.
		type wrapper struct {
			D Duration `yaml:"d"`
		}
		orig := wrapper{D: Duration{5 * time.Minute}}
		data, err := yaml.Marshal(orig)
		require.NoError(t, err)

		var got wrapper
		require.NoError(t, yaml.Unmarshal(data, &got))
		assert.Equal(t, orig.D.Duration, got.D.Duration)
	})

	t.Run("zero duration marshals as 0s", func(t *testing.T) {
		d := Duration{0}
		v, err := d.MarshalYAML()
		require.NoError(t, err)
		assert.Equal(t, "0s", v)
	})
}

// ---------- TestDefaultPath ----------

func TestDefaultPath(t *testing.T) {
	path := DefaultPath()
	assert.NotEmpty(t, path)
	assert.Contains(t, path, "wslwatch")
}

// ---------- helpers ----------

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "wslwatch.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}
