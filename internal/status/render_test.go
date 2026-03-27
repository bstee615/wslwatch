package status

import (
	"bytes"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/stretchr/testify/assert"

	"github.com/bstee615/wslwatch/internal/watchdog"
)

func init() {
	// Disable color for consistent test output
	color.NoColor = true
}

func TestRender_Running(t *testing.T) {
	ws := watchdog.WatchdogStatus{
		Running:   true,
		StartedAt: time.Now().Add(-3 * 24 * time.Hour),
		Distros: []watchdog.DistroStatusSnapshot{
			{
				Name:         "Ubuntu-22.04",
				Healthy:      true,
				RestartCount: 1,
				StartedAt:    time.Now().Add(-2 * 24 * time.Hour),
			},
			{
				Name:   "Ubuntu-20.04",
				Paused: true,
			},
		},
	}

	var buf bytes.Buffer
	Render(&buf, ws, []string{"docker-desktop"})
	output := buf.String()

	assert.Contains(t, output, "wslwatch")
	assert.Contains(t, output, "running")
	assert.Contains(t, output, "Ubuntu-22.04")
	assert.Contains(t, output, "healthy")
	assert.Contains(t, output, "Ubuntu-20.04")
	assert.Contains(t, output, "paused")
	assert.Contains(t, output, "docker-desktop")
	assert.Contains(t, output, "ignored")
}

func TestRender_Stopped(t *testing.T) {
	ws := watchdog.WatchdogStatus{
		Running: false,
	}

	var buf bytes.Buffer
	Render(&buf, ws, nil)
	output := buf.String()

	assert.Contains(t, output, "stopped")
}

func TestRender_Exhausted(t *testing.T) {
	ws := watchdog.WatchdogStatus{
		Running:   true,
		StartedAt: time.Now(),
		Distros: []watchdog.DistroStatusSnapshot{
			{
				Name:      "Ubuntu",
				Exhausted: true,
			},
		},
	}

	var buf bytes.Buffer
	Render(&buf, ws, nil)
	output := buf.String()

	assert.Contains(t, output, "exhausted")
}

func TestRender_WithFailures(t *testing.T) {
	ws := watchdog.WatchdogStatus{
		Running:   true,
		StartedAt: time.Now(),
		Distros: []watchdog.DistroStatusSnapshot{
			{
				Name:         "Ubuntu",
				Healthy:      false,
				FailureCount: 3,
				RestartCount: 2,
			},
		},
	}

	var buf bytes.Buffer
	Render(&buf, ws, nil)
	output := buf.String()

	assert.Contains(t, output, "Failure history")
	assert.Contains(t, output, "Ubuntu")
}

func TestRenderNotRunning(t *testing.T) {
	var buf bytes.Buffer
	RenderNotRunning(&buf)
	output := buf.String()

	assert.Contains(t, output, "not running")
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		dur      time.Duration
		expected string
	}{
		{0, "0m"},
		{30 * time.Second, "0m"},
		{90 * time.Minute, "1h 30m"},
		{25 * time.Hour, "1d 1h 0m"},
		{3*24*time.Hour + 14*time.Hour + 22*time.Minute, "3d 14h 22m"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, FormatDuration(tt.dur))
		})
	}
}
