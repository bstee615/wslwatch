package status

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/fatih/color"

	"github.com/bstee615/wslwatch/internal/ipc"
)

// FormatUptime formats a duration as "Xd Xh Xm" (or "Xs" for sub-minute durations).
func FormatUptime(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}

	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if len(parts) == 0 {
		return "0m"
	}
	return strings.Join(parts, " ")
}

const divider = "──────────────────────────────────────────────────────"

// barWidth is the number of characters in the uptime history bar.
const barWidth = 60

// barWindow is the time window the bar represents.
const barWindow = 60 * time.Minute

// RenderStatus renders the full status output to w.
func RenderStatus(w io.Writer, data *ipc.StatusData) {
	cyan := color.New(color.FgCyan).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()

	// Header line.
	fmt.Fprintf(w, "%s  %s  uptime %s\n",
		cyan("wslwatch"),
		green("● running"),
		data.Uptime,
	)
	fmt.Fprintln(w, divider)

	// Per-distro lines.
	for _, d := range data.Distros {
		renderDistroLine(w, d)
	}

	// Uptime history bars for all non-ignored distros.
	for _, d := range data.Distros {
		if strings.EqualFold(d.State, "ignored") {
			continue
		}
		fmt.Fprintln(w)
		gray := color.New(color.FgWhite).SprintFunc()
		fmt.Fprintf(w, "Uptime history (last 60m) — %s\n", gray(d.Name))
		fmt.Fprintf(w, "  %s\n", buildTimeBar(d.FailureTimes, data.StartedAt, time.Now()))
		fmt.Fprintf(w, "  %-15s%-15s%-15s%-15s\n", "60m", "45m", "30m", "15m")
	}

	// Legend.
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Legend: █ down  ░ healthy  ─ no data")
}

// renderDistroLine writes a single distro status line.
func renderDistroLine(w io.Writer, d ipc.DistroData) {
	green := color.New(color.FgGreen).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()
	gray := color.New(color.FgWhite).SprintFunc()

	switch {
	case d.Exhausted:
		fmt.Fprintf(w, "  %-20s %s\n", d.Name, red("✗ exhausted"))
	case d.InBackoff:
		fmt.Fprintf(w, "  %-20s %s\n", d.Name, yellow("⏸ paused"))
	case strings.EqualFold(d.State, "ignored"):
		fmt.Fprintf(w, "  %-20s %s\n", d.Name, gray("─ ignored"))
	case strings.EqualFold(d.State, "stopped"):
		fmt.Fprintf(w, "  %-20s %s\n", d.Name, red("✗ stopped"))
	case strings.EqualFold(d.State, "starting"):
		fmt.Fprintf(w, "  %-20s %s\n", d.Name, yellow("◌ starting"))
	case strings.EqualFold(d.State, "healthy"):
		line := fmt.Sprintf("  %-20s %s", d.Name, green("● healthy"))
		if d.Uptime != "" {
			line += fmt.Sprintf("   uptime %s", d.Uptime)
		}
		if d.RestartCount > 0 {
			line += fmt.Sprintf("   restarts %d", d.RestartCount)
		}
		fmt.Fprintln(w, line)
	default:
		fmt.Fprintf(w, "  %-20s %s\n", d.Name, d.State)
	}
}

// buildTimeBar produces a barWidth-character bar spanning the last 60 minutes.
// Each cell represents 1 minute. Cells before startedAt are shown as "─" (no data),
// cells with a failure are "█" (red), and healthy cells are "░".
func buildTimeBar(failureTimes []time.Time, startedAt time.Time, now time.Time) string {
	windowStart := now.Add(-barWindow)
	cellDur := barWindow / time.Duration(barWidth) // 1 minute per cell

	red := color.New(color.FgRed).SprintFunc()

	// Build a set of which cells have failures.
	failureCells := make(map[int]bool)
	for _, ft := range failureTimes {
		if ft.Before(windowStart) || ft.After(now) {
			continue
		}
		cell := int(ft.Sub(windowStart) / cellDur)
		if cell >= barWidth {
			cell = barWidth - 1
		}
		failureCells[cell] = true
	}

	var buf strings.Builder
	for i := 0; i < barWidth; i++ {
		if failureCells[i] {
			buf.WriteString(red("█"))
		} else {
			cellEnd := windowStart.Add(time.Duration(i+1) * cellDur)
			if cellEnd.Before(startedAt) {
				buf.WriteString("─")
			} else {
				buf.WriteString("░")
			}
		}
	}
	return buf.String()
}

// RenderNotRunning renders the "not running" message to w.
func RenderNotRunning(w io.Writer) {
	yellow := color.New(color.FgYellow).SprintFunc()
	fmt.Fprintln(w, yellow("wslwatch  ○ not running"))
}
