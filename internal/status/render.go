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

// barWidth is the number of characters in the failure history bar.
const barWidth = 60

// maxRestarts is the reference maximum used to scale the bar.
const maxRestarts = 10

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

	// Failure history bars (only for distros that have had restarts).
	for _, d := range data.Distros {
		if d.RestartCount == 0 {
			continue
		}
		fmt.Fprintln(w)
		gray := color.New(color.FgWhite).SprintFunc()
		fmt.Fprintf(w, "Failure history (last 60m) — %s\n", gray(d.Name))
		fmt.Fprintf(w, "  %s\n", buildBar(d.RestartCount))
	}

	// Legend.
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Legend: █ failure  ░ healthy  ─ no data")
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

// buildBar produces a barWidth-character bar where filled blocks represent
// relative failure density based on restart_count.
func buildBar(restartCount int) string {
	filled := restartCount
	if filled > maxRestarts {
		filled = maxRestarts
	}
	// Scale filled to barWidth.
	filledCells := (filled * barWidth) / maxRestarts
	if filledCells < 1 {
		filledCells = 1
	}
	emptyCells := barWidth - filledCells

	red := color.New(color.FgRed).SprintFunc()
	return red(strings.Repeat("█", filledCells)) + strings.Repeat("░", emptyCells)
}

// RenderNotRunning renders the "not running" message to w.
func RenderNotRunning(w io.Writer) {
	yellow := color.New(color.FgYellow).SprintFunc()
	fmt.Fprintln(w, yellow("wslwatch  ○ not running"))
}
