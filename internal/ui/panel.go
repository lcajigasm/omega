package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/lcajigasm/omega/internal/i18n"
)

const (
	panelWidth    = 34  // columns, border included
	panelMinWidth = 104 // terminal width from which the panel fits beside the transcript
	panelMinRows  = 12
)

// panelShown reports whether the session panel is drawn: the user can turn
// it off, and a narrow or short terminal has no room for it.
func (m *model) panelShown() bool {
	return m.panel && m.width >= panelMinWidth && m.height >= panelMinRows+6
}

// tw is the width of the transcript: the terminal's, less the panel.
func (m *model) tw() int {
	if m.panelShown() {
		return m.width - panelWidth
	}
	return m.width
}

// tally counts the tool calls of the session by outcome.
type tally struct{ ok, failed, denied, running int }

func (m *model) tally() tally {
	var t tally
	for _, e := range m.entries {
		if e.kind != enTool {
			continue
		}
		switch {
		case e.running:
			t.running++
		case e.denied != "":
			t.denied++
		case e.failed:
			t.failed++
		default:
			t.ok++
		}
	}
	return t
}

func humanBytes(n uint64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
	}
	return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
}

// gauge draws a bar of width cells with the used share filled.
func gauge(used float64, width int) string {
	if used < 0 {
		used = 0
	}
	if used > 1 {
		used = 1
	}
	full := int(used*float64(width) + 0.5)
	st := okStyle
	switch {
	case used >= 0.9:
		st = errStyle
	case used >= 0.75:
		st = lipgloss.NewStyle().Foreground(warnColor)
	}
	return st.Render(strings.Repeat("━", full)) + faintStyle.Render(strings.Repeat("━", width-full))
}

// diskDelta is the change in free space worth showing: small moves are noise
// from the system itself.
const diskDelta = 50 << 20

// panelView is the session panel: where the run stands at a glance, so the
// transcript does not have to be scrolled to know what happened.
func (m *model) panelView(height int) string {
	inner := panelWidth - 4 // border and padding
	var lines []string
	add := func(s string) { lines = append(lines, s) }
	row := func(k, v string) {
		add(faintStyle.Render(lipgloss.NewStyle().Width(9).Render(k)) + v)
	}
	add(titleStyle.Render(i18n.T("panel.session")))
	row(i18n.T("ui.label_task"), truncate(m.header.Task, inner-9))
	row(i18n.T("panel.mode"), truncate(m.modeText(), inner-9))
	row(i18n.T("ui.label_model"), truncate(m.header.Model, inner-9))

	t := m.tally()
	steps := okStyle.Render(fmt.Sprintf("✓ %d", t.ok))
	if t.failed > 0 {
		steps += "  " + errStyle.Render(fmt.Sprintf("✗ %d", t.failed))
	}
	if t.denied > 0 {
		steps += "  " + warnStyle.Render(fmt.Sprintf("⊘ %d", t.denied))
	}
	if t.running > 0 {
		steps += "  " + titleStyle.Render(fmt.Sprintf("● %d", t.running))
	}
	row(i18n.T("panel.steps"), steps)

	if m.diskTotal > 0 {
		add("")
		add(titleStyle.Render(i18n.T("panel.disk")))
		add(gauge(1-float64(m.diskNow)/float64(m.diskTotal), inner))
		add(mutedStyle.Render(truncate(i18n.T("panel.free", humanBytes(m.diskNow), humanBytes(m.diskTotal)), inner)))
		switch {
		case m.diskNow > m.diskStart+diskDelta:
			add(okStyle.Render("▲ " + i18n.T("panel.freed", humanBytes(m.diskNow-m.diskStart))))
		case m.diskStart > m.diskNow+diskDelta:
			add(faintStyle.Render("▼ " + i18n.T("panel.used", humanBytes(m.diskStart-m.diskNow))))
		}
	}

	add("")
	add(titleStyle.Render(i18n.T("panel.usage")))
	usage := "↑" + humanTokens(m.stats.Usage.Input()) + " ↓" + humanTokens(m.stats.Usage.OutputTokens)
	if c := m.costText(); c != "" {
		usage += " · " + c
	}
	add(mutedStyle.Render(usage))

	add("")
	add(titleStyle.Render(i18n.T("panel.activity")))
	var acts []string
	for _, e := range m.entries {
		if e.kind != enTool {
			continue
		}
		icon := okStyle.Render("✓")
		dur := e.duration
		switch {
		case e.running:
			icon = titleStyle.Render(spinFrames[m.frame%len(spinFrames)])
			dur = time.Since(e.started)
		case e.denied != "":
			icon = warnStyle.Render("⊘")
		case e.failed:
			icon = errStyle.Render("✗")
		}
		name, arg := toolLabel(e.tool, e.command)
		label := name
		if arg != "" {
			label += " " + arg
		}
		right := ""
		if dur >= time.Second {
			right = humanDuration(dur)
		}
		room := inner - 2 - lipgloss.Width(right)
		if right != "" {
			room--
		}
		label = truncate(strings.ReplaceAll(label, "\n", " "), max(10, room))
		gap := inner - 2 - lipgloss.Width(label) - lipgloss.Width(right)
		if gap < 1 {
			gap = 1
		}
		text := label
		if e.running {
			text = cmdStyle.Render(label)
		}
		acts = append(acts, icon+" "+text+strings.Repeat(" ", gap)+faintStyle.Render(right))
	}
	room := height - 2 - len(lines) // the border takes two rows
	if room < 1 {
		room = 1
	}
	if len(acts) == 0 {
		acts = []string{faintStyle.Render(i18n.T("panel.none"))}
	}
	if len(acts) > room {
		acts = acts[len(acts)-room:]
	}
	lines = append(lines, acts...)
	if len(lines) > height-2 {
		lines = lines[:height-2]
	}
	return cardStyle.Width(panelWidth - 2).Height(height - 2).Render(strings.Join(lines, "\n"))
}
