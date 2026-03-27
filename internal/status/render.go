package status

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/bstee615/wslwatch/internal/watchdog"
	"github.com/fatih/color"
)

var (
	green  = color.New(color.FgGreen)
	yellow = color.New(color.FgYellow)
	red    = color.New(color.FgRed)
	gray   = color.New(color.FgHiBlack)
	bold   = color.New(color.Bold)
	cyan   = color.New(color.FgCyan)
)

// Render writes the formatted watchdog status to the given writer.
func Render(w io.Writer, ws watchdog.WatchdogStatus, ignoredDistros []string) {
	// Header
	if ws.Running {
		fmt.Fprintf(w, "wslwatch  ")
		green.Fprintf(w, "● running")
		fmt.Fprintf(w, "  uptime %s\n", formatDuration(time.Since(ws.StartedAt)))
	} else {
		fmt.Fprintf(w, "wslwatch  ")
		red.Fprintf(w, "● stopped")
		fmt.Fprintln(w)
	}

	// Separator
	fmt.Fprintln(w, strings.Repeat("─", 58))

	// Distro statuses
	for _, d := range ws.Distros {
		fmt.Fprintf(w, "  %-20s", d.Name)
		if d.Paused {
			yellow.Fprintf(w, " ⏸ paused")
		} else if d.Exhausted {
			red.Fprintf(w, " ✗ exhausted")
		} else if d.Healthy {
			green.Fprintf(w, " ● healthy")
			fmt.Fprintf(w, "   uptime %s", formatDuration(time.Since(d.StartedAt)))
			fmt.Fprintf(w, "   restarts %d", d.RestartCount)
		} else {
			red.Fprintf(w, " ● unhealthy")
			fmt.Fprintf(w, "   restarts %d", d.RestartCount)
		}
		fmt.Fprintln(w)
	}

	// Ignored distros
	for _, name := range ignoredDistros {
		fmt.Fprintf(w, "  %-20s", name)
		gray.Fprintf(w, " ─ ignored")
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w)

	// Failure history for distros with failures
	for _, d := range ws.Distros {
		if d.FailureCount > 0 {
			renderFailureBar(w, d)
		}
	}

	// Legend
	fmt.Fprintln(w)
	fmt.Fprint(w, "Legend: ")
	red.Fprint(w, "█")
	fmt.Fprint(w, " failure  ")
	green.Fprint(w, "░")
	fmt.Fprint(w, " healthy  ")
	gray.Fprint(w, "─")
	fmt.Fprintln(w, " no data")
}

// RenderNotRunning writes a message when the watchdog is not running.
func RenderNotRunning(w io.Writer) {
	red.Fprintln(w, "wslwatch is not running")
	fmt.Fprintln(w, "Start it with: wslwatch")
	fmt.Fprintln(w, "Or install as service: wslwatch --install")
}

func renderFailureBar(w io.Writer, d watchdog.DistroStatusSnapshot) {
	cyan.Fprintf(w, "Failure history (last 60m) — %s\n", d.Name)
	barLen := 60
	failRatio := float64(d.FailureCount) / float64(barLen)
	if failRatio > 1 {
		failRatio = 1
	}
	failBars := int(failRatio * float64(barLen))
	fmt.Fprint(w, "  ")
	for i := 0; i < barLen; i++ {
		if i < failBars {
			red.Fprint(w, "█")
		} else {
			green.Fprint(w, "░")
		}
	}
	fmt.Fprintln(w)
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}

	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	parts := []string{}
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 || days > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	parts = append(parts, fmt.Sprintf("%dm", minutes))

	return strings.Join(parts, " ")
}

// FormatDuration formats a duration for display (exported for tests).
func FormatDuration(d time.Duration) string {
	return formatDuration(d)
}
