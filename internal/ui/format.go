package ui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/lcajigasm/omega/internal/i18n"
)

func toolLabel(tool, command string) (string, string) {
	name, arg := "Bash", command
	switch {
	case strings.HasPrefix(tool, "run_step:"):
		name, arg = "Step", strings.TrimPrefix(tool, "run_step:")
	case tool == "disk_usage":
		name = "Disk"
		if i, j := strings.Index(command, "'"), strings.LastIndex(command, "'"); i >= 0 && j > i {
			arg = command[i+1 : j]
		}
	case tool == "read_file":
		name = "Read"
	case tool == "write_file":
		name = "Write"
	case tool == "system_info":
		name, arg = "System", ""
	case tool == "remember":
		name = "Remember"
	case tool == "web_search":
		name = "Search"
	case tool == "fetch_url":
		name = "Fetch"
	case tool == "mac_inventory":
		name, arg = "Mac", strings.TrimSpace(strings.TrimPrefix(command, "mac_inventory"))
	case tool == "mac_change":
		name = "Change"
	case tool == "mac_policy":
		name = "Rule"
	case tool == "dev_inventory":
		name, arg = "Dev", ""
	case tool == "dev_install":
		name = "Install"
	case tool == "dev_activate":
		name = "Activate"
	case tool == "mac_history":
		name, arg = "History", "mac"
	case tool == "mac_undo":
		name = "Undo"
	case tool == "run_history":
		name, arg = "History", strings.TrimSpace(strings.TrimPrefix(command, "run_history"))
	case tool == "notes":
		name = "Notes"
	case tool == "omega_settings":
		name = "Settings"
	case tool == "omega_doctor":
		name, arg = "Doctor", ""
	case tool == "model_ranking":
		name, arg = "Models", ""
	}
	return name, homeTilde(arg)
}

// ToolLabel names a tool call the way the interface shows it: a short name
// and its argument. The desktop app uses the same names as the terminal.
func ToolLabel(tool, command string) (name, arg string) { return toolLabel(tool, command) }

// homeTilde shortens paths under the home directory to ~.
func homeTilde(s string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return strings.ReplaceAll(s, home, "~")
	}
	return s
}

func humanTokens(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1000:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	}
	return fmt.Sprintf("%d", n)
}

func humanDuration(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}

// Bytes writes a size the way people read it: 812 B, 34 KB, 512 MB, 3.4 GB,
// with the decimal comma of the languages that use one (3,4 GB). It is the
// one size formatter of the interface and of the reports.
func Bytes(n int64) string {
	var s string
	switch {
	case n >= 1<<40:
		s = fmt.Sprintf("%.1f TB", float64(n)/(1<<40))
	case n >= 1<<30:
		s = fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		s = fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		s = fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
	if decimalComma() {
		s = strings.Replace(s, ".", ",", 1)
	}
	return s
}

// decimalComma reports whether the active language writes 3,4 for 3.4.
func decimalComma() bool {
	switch i18n.Lang() {
	case "es", "fr", "de", "pt":
		return true
	}
	return false
}

// Duration writes how long something took: 4s, 1m05s.
func Duration(d time.Duration) string { return humanDuration(d) }

func (m *model) costText() string {
	if !m.stats.CostKnown {
		return ""
	}
	return FormatCost(m.stats.Cost)
}

// FormatCost writes a session's cost the same way on every screen: four
// decimals for the cents a cheap model costs, two from ten cents up.
func FormatCost(c float64) string {
	if c < 0.10 {
		return fmt.Sprintf("$%.4f", c)
	}
	return fmt.Sprintf("$%.2f", c)
}

func firstSentence(s string) string {
	if i := strings.Index(s, ". "); i > 0 {
		return s[:i+1]
	}
	return s
}

func tail(lines []string, n int) []string {
	if len(lines) <= n {
		return lines
	}
	return lines[len(lines)-n:]
}

// truncate cuts s to n terminal columns, ending in "…". It counts what the
// terminal shows: a wide character takes two columns, colour takes none.
func truncate(s string, n int) string {
	if n < 10 {
		n = 10
	}
	if ansi.StringWidth(s) <= n {
		return s
	}
	return ansi.Truncate(s, n, gl.Ellipsis)
}

// truncateANSI cuts a coloured line to n columns without an ellipsis.
func truncateANSI(s string, n int) string {
	if lipgloss.Width(s) <= n {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(n).Render(s)
}

// summaryLine is the closing line of a session: time, tokens and, when it
// is known, the cost. Nothing is left dangling when the cost is not known.
func summaryLine(elapsed time.Duration, tokens int64, cost string) string {
	parts := []string{i18n.T("ui.summary_time", humanDuration(elapsed)), i18n.T("ui.summary_tokens", humanTokens(tokens))}
	if cost != "" {
		parts = append(parts, cost)
	}
	return strings.Join(parts, " · ")
}

// markdownStyle is glamour's style for the background. Colour itself
// follows the terminal through WithColorProfile: with NO_COLOR or piped
// output the same layout is drawn without escapes.
func markdownStyle(style string) glamour.TermRendererOption {
	return glamour.WithStandardStyle(style)
}
