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

// size is a disk size as the panel and the home screen write it.
func size(n uint64) string { return Bytes(int64(n)) }

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
		st = warnPlainStyle
	}
	// Used and free are drawn differently, not only in different colours,
	// so the bar still reads without colour.
	return st.Render(strings.Repeat(gl.Heavy, full)) + faintStyle.Render(strings.Repeat(gl.Rule, width-full))
}

// diskDelta is the change in free space worth showing: small moves are noise
// from the system itself.
const diskDelta = 50 << 20

// panelView is the session panel: where the run stands at a glance, so the
// transcript does not have to be scrolled to know what happened.
func (m *model) panelView(height int) string {
	inner := panelWidth - 4 // the margins on both sides
	var lines []string
	add := func(s string) { lines = append(lines, s) }
	row := func(k, v string) {
		add(faintStyle.Render(lipgloss.NewStyle().Width(9).Render(k)) + v)
	}
	if m.g != nil {
		lines = append(lines, m.routeLines(inner)...)
	}
	add(panelHead(i18n.T("panel.session"), "", inner))
	row(i18n.T("ui.label_task"), truncate(m.header.Task, inner-9))
	row(i18n.T("panel.mode"), truncate(m.modeShort(), inner-9))
	// A stage that asks no model (omega doctor, the built-in steps) has
	// no model to name and spends no tokens: the rows are left out.
	if m.header.Model != "" {
		row(i18n.T("ui.label_model"), truncate(m.header.Model, inner-9))
	}

	t := m.tally()
	steps := okStyle.Render(fmt.Sprintf("%s %d", gl.Check, t.ok))
	if t.failed > 0 {
		steps += "  " + errStyle.Render(fmt.Sprintf("%s %d", gl.Cross, t.failed))
	}
	if t.denied > 0 {
		steps += "  " + warnStyle.Render(fmt.Sprintf("%s %d", gl.Denied, t.denied))
	}
	if t.running > 0 {
		steps += "  " + titleStyle.Render(fmt.Sprintf("%s %d", gl.Running, t.running))
	}
	if t.ok+t.failed+t.denied+t.running > 0 {
		row(i18n.T("panel.steps"), steps)
	}

	if m.diskTotal > 0 {
		add("")
		add(panelHead(i18n.T("panel.disk"), "", inner))
		add(gauge(1-float64(m.diskNow)/float64(m.diskTotal), inner))
		add(mutedStyle.Render(truncate(i18n.T("panel.free", size(m.diskNow), size(m.diskTotal)), inner)))
		switch {
		case m.diskNow > m.diskStart+diskDelta:
			add(okStyle.Render(gl.Freed + " " + i18n.T("panel.freed", size(m.diskNow-m.diskStart))))
		case m.diskStart > m.diskNow+diskDelta:
			add(faintStyle.Render(gl.Used + " " + i18n.T("panel.used", size(m.diskStart-m.diskNow))))
		}
	}

	if m.stats.Usage.Input()+m.stats.Usage.OutputTokens > 0 {
		add("")
		add(panelHead(i18n.T("panel.usage"), "", inner))
		usage := gl.Up + humanTokens(m.stats.Usage.Input()) + " " + gl.Down + humanTokens(m.stats.Usage.OutputTokens)
		if c := m.costText(); c != "" {
			usage += " " + gl.Sep + " " + c
		}
		add(mutedStyle.Render(usage))
	}

	add("")
	add(panelHead(i18n.T("panel.activity"), fmt.Sprint(t.ok+t.failed+t.denied+t.running), inner))
	var acts []string
	for _, e := range m.entries {
		if e.kind != enTool {
			continue
		}
		icon := okStyle.Render(gl.Check)
		dur := e.duration
		switch {
		case e.running:
			icon = titleStyle.Render(spinFrame(m.frame))
			dur = time.Since(e.started)
		case e.denied != "":
			icon = warnStyle.Render(gl.Denied)
		case e.failed:
			icon = errStyle.Render(gl.Cross)
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
	room := height - len(lines)
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
	// No frame: a column beside the transcript, as the unattended run's
	// panels sit beside each other.
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	for i, l := range lines {
		lines[i] = "  " + fit(l, inner) + "  "
	}
	return strings.Join(lines, "\n")
}
