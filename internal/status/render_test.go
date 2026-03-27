package status_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/bstee615/wslwatch/internal/ipc"
	"github.com/bstee615/wslwatch/internal/status"
)

// TestFormatUptime verifies various duration formatting cases.
func TestFormatUptime(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "30s"},
		{59 * time.Second, "59s"},
		{1 * time.Minute, "1m"},
		{90 * time.Second, "1m"},
		{1*time.Hour + 30*time.Minute, "1h 30m"},
		{24 * time.Hour, "1d"},
		{3*24*time.Hour + 14*time.Hour + 22*time.Minute, "3d 14h 22m"},
		{0, "0s"},
	}

	for _, tc := range cases {
		got := status.FormatUptime(tc.d)
		if got != tc.want {
			t.Errorf("FormatUptime(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

// TestRenderStatus verifies that RenderStatus produces output containing key
// strings such as distro names and states.
func TestRenderStatus(t *testing.T) {
	data := &ipc.StatusData{
		Running:   true,
		Uptime:    "3d 14h 22m",
		StartedAt: time.Now().Add(-3 * 24 * time.Hour),
		Distros: []ipc.DistroData{
			{
				Name:         "Ubuntu-22.04",
				State:        "healthy",
				Uptime:       "2d 4h 11m",
				RestartCount: 1,
				FailureTimes: []time.Time{time.Now().Add(-10 * time.Minute)},
			},
			{
				Name:      "Ubuntu-20.04",
				State:     "healthy",
				InBackoff: true,
			},
			{
				Name:  "docker-desktop",
				State: "ignored",
			},
		},
	}

	var buf bytes.Buffer
	status.RenderStatus(&buf, data)
	out := buf.String()

	// Header must mention "running" and the overall uptime.
	if !strings.Contains(out, "running") {
		t.Errorf("output missing 'running': %s", out)
	}
	if !strings.Contains(out, "3d 14h 22m") {
		t.Errorf("output missing uptime '3d 14h 22m': %s", out)
	}

	// Each distro name must appear.
	for _, name := range []string{"Ubuntu-22.04", "Ubuntu-20.04", "docker-desktop"} {
		if !strings.Contains(out, name) {
			t.Errorf("output missing distro %q: %s", name, out)
		}
	}

	// State indicators.
	if !strings.Contains(out, "healthy") {
		t.Errorf("output missing 'healthy': %s", out)
	}
	if !strings.Contains(out, "paused") {
		t.Errorf("output missing 'paused': %s", out)
	}
	if !strings.Contains(out, "ignored") {
		t.Errorf("output missing 'ignored': %s", out)
	}

	// Restart count for Ubuntu-22.04.
	if !strings.Contains(out, "restarts 1") {
		t.Errorf("output missing 'restarts 1': %s", out)
	}

	// Uptime history bar should appear for all non-ignored distros.
	if !strings.Contains(out, "Uptime history") {
		t.Errorf("output missing uptime history section: %s", out)
	}

	// Axis labels.
	if !strings.Contains(out, "60m") || !strings.Contains(out, "15m") {
		t.Errorf("output missing axis labels: %s", out)
	}

	// Legend.
	if !strings.Contains(out, "Legend") {
		t.Errorf("output missing legend: %s", out)
	}
}

// TestRenderNotRunning verifies the not-running message is present.
func TestRenderNotRunning(t *testing.T) {
	var buf bytes.Buffer
	status.RenderNotRunning(&buf)
	out := buf.String()

	if !strings.Contains(out, "not running") {
		t.Errorf("output missing 'not running': %s", out)
	}
	if !strings.Contains(out, "wslwatch") {
		t.Errorf("output missing 'wslwatch': %s", out)
	}
}
