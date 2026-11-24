package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/lcajigasm/omega/internal/i18n"
)

// ---------- view ----------

// spread puts left and right at the two ends of a line of width w.
func spread(left, right string, w int) string {
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		if room := w - lipgloss.Width(right) - 1; room >= 12 {
			return truncateANSI(left, room) + " " + right
		}
		return truncateANSI(left, w) // no room for both: the left one stays
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m *pilotModel) spin() string { return titleStyle.Render(spinFrame(m.frame)) }

func (m *pilotModel) mark(i int) string {
	if m.res[i].Status == StageRunning {
		return m.spin()
	}
	return statusMark(m.res[i].Status)
}

func (m *pilotModel) headLine(w int) string {
	left := " " + titleStyle.Render(gl.Brand) + " " + cmdStyle.Render(i18n.T("pilot.title"))
	if m.info.Version != "" {
		left += mutedStyle.Render("  " + m.info.Version)
	}
	if m.header.Model != "" {
		left += faintStyle.Render(" " + gl.Sep + " " + m.header.Model)
	}
	if len(m.info.Flags) > 0 {
		left += faintStyle.Render(" " + gl.Sep + " " + strings.Join(m.info.Flags, ", "))
	}
	right := humanDuration(time.Since(m.began))
	if c, ok := m.cost(); ok {
		right += " " + gl.Sep + " " + FormatCost(c)
	}
	return spread(left, mutedStyle.Render(right)+" ", w)
}

func (m *pilotModel) progressLine(w int) string {
	total, settled := m.progress()
	var marks strings.Builder
	for i, s := range m.stages {
		if s.Selected {
			marks.WriteString(m.mark(i))
		}
	}
	count := fmt.Sprintf("%d/%d", settled, total)
	barW := min(40, w-total-len(count)-8)
	bar := ""
	if barW >= 8 && total > 0 {
		fill := barW * settled / total
		bar = "  " + accentStyle.Render(strings.Repeat(gl.Heavy, fill)) + faintStyle.Render(strings.Repeat(gl.Heavy, barW-fill))
	}
	return truncateANSI(" "+marks.String()+"  "+mutedStyle.Render(count)+bar, w)
}

// bubble is what the mascot says: the stage it is on and what it is doing.
func (m *pilotModel) bubble(w int) []string {
	inner := max(10, w-4)
	head := mutedStyle.Render(i18n.T("pilot.title"))
	switch {
	case m.cur >= 0:
		s := m.stages[m.cur]
		head = titleStyle.Render(i18n.T("full.stage", m.ord, m.of)) + mutedStyle.Render(" "+gl.Sep+" ") + cmdStyle.Render(s.Title) +
			faintStyle.Render("  "+s.Command+" "+gl.Sep+" "+humanDuration(time.Since(m.st[m.cur].began)))
	case m.stopping:
		head = warnStyle.Render(i18n.T("full.stopping"))
	}
	say := strings.Split(lipgloss.NewStyle().Width(inner).Render(m.say), "\n")
	for i := range say {
		say[i] = strings.TrimRight(say[i], " ")
	}
	if len(say) > 2 {
		last := []rune(say[1])
		say = append(say[:1], string(last[:min(len(last), inner-1)])+gl.Ellipsis)
	}
	for len(say) < 2 {
		say = append(say, "")
	}
	body := truncateANSI(head, inner) + "\n" + strings.Join(say, "\n")
	return strings.Split(cardStyle.Width(w-2).Render(body), "\n")
}

// nowLines are the command that is running and the last thing it printed.
func (m *pilotModel) nowLines(w int) []string {
	now, out := "", ""
	run := m.running()
	switch {
	case run >= 0:
		a := m.acts[run]
		label := cmdStyle.Render(a.name)
		if a.arg != "" {
			label += "(" + a.arg + ")"
		}
		took := faintStyle.Render(" " + humanDuration(time.Since(a.began)))
		now = "  " + m.spin() + " " + truncateANSI(label, max(10, w-6-lipgloss.Width(took))) + took
	case m.thinking:
		now = "  " + m.spin() + " " + mutedStyle.Render(i18n.T("pilot.thinking")) + faintStyle.Render(" "+humanDuration(time.Since(m.thinkAt)))
	case m.cur >= 0:
		now = "  " + m.spin() + " " + mutedStyle.Render(m.stages[m.cur].Title+gl.Ellipsis)
	}
	// What a command printed is shown while it runs; a stage without a
	// session (a report, the built-in steps) prints by itself.
	if m.lastOut != "" && (run >= 0 || (m.cur >= 0 && m.cancel == nil)) {
		out = faintStyle.Render("    " + gl.Result + " " + truncate(m.lastOut, max(10, w-8)))
	}
	return []string{now, out}
}

// routeLines is every stage and how it went.
func (m *pilotModel) routeLines(w, h int) []string {
	total, settled := m.progress()
	lines := []string{spread(titleStyle.Render(i18n.T("full.route")), mutedStyle.Render(fmt.Sprintf("%d/%d", settled, total)), w)}
	cur := 0 // row of the stage that ran last
	for i, s := range m.stages {
		if !s.Selected {
			continue // left out of this run: the summary names it
		}
		if i == m.last {
			cur = len(lines) - 1
		}
		r := m.res[i]
		tail := ""
		switch {
		case r.Status == StageRunning:
			tail = humanDuration(time.Since(m.st[i].began))
		case r.Status == StageDone && r.Duration >= time.Second:
			tail = humanDuration(r.Duration)
		case r.Status == StageFailed || r.Status == StageStopped:
			tail = StatusWord(r.Status)
		}
		name := truncate(s.Title, max(8, w-4-lipgloss.Width(tail)))
		switch r.Status {
		case StageRunning:
			name = cmdStyle.Render(name)
		case StagePending:
			name = mutedStyle.Render(name)
		}
		lines = append(lines, spread(m.mark(i)+" "+name, faintStyle.Render(tail), w))
	}
	if len(lines) > h && h > 1 {
		// No room for every stage: keep the ones around the running one.
		rows := lines[1:]
		from := max(0, min(cur-(h-1)/2, len(rows)-(h-1)))
		lines = append(lines[:1], rows[from:from+h-1]...)
	}
	return lines
}

// actLine is one line of the list of what has run.
func (m *pilotModel) actLine(a pilotAct, w int) string {
	if a.kind == actStage {
		return titleStyle.Render(strings.Repeat(gl.Rule, 2) + " " + truncate(a.name, max(8, w-4)))
	}
	mark, tail := mutedStyle.Render(gl.Pending), ""
	switch a.kind {
	case actRunning:
		mark, tail = m.spin(), humanDuration(time.Since(a.began))
	case actOK:
		mark = okStyle.Render(gl.Check)
		if a.dur >= time.Second {
			tail = humanDuration(a.dur)
		}
	case actFailed:
		mark = errStyle.Render(gl.Cross)
		if a.exit > 0 {
			tail = fmt.Sprintf("exit %d", a.exit)
		}
	case actDenied:
		mark, tail = warnStyle.Render(gl.Denied), i18n.T("ui.denied")
	case actError:
		mark = errStyle.Render(gl.Error)
	}
	label := a.name
	if a.arg != "" {
		label += "(" + a.arg + ")"
	}
	if a.note != "" {
		label += " " + gl.Sep + " " + a.note
	}
	label = truncate(label, max(8, w-4-lipgloss.Width(tail)))
	if a.kind == actNote || a.kind == actError {
		label = mutedStyle.Render(label)
	}
	return spread(" "+mark+" "+label, faintStyle.Render(tail), w)
}

// doneLines is the list of what has run, the latest at the bottom.
func (m *pilotModel) doneLines(w, h int) []string {
	ok, failed, denied := m.tally(-1)
	lines := []string{spread(titleStyle.Render(i18n.T("pilot.done_head")), mutedStyle.Render(i18n.T("pilot.tally", ok, failed, denied)), w)}
	if len(m.acts) == 0 {
		return append(lines, mutedStyle.Render(" "+i18n.T("pilot.nothing_yet")))
	}
	acts := m.acts
	if room := max(1, h-1); len(acts) > room {
		acts = acts[len(acts)-room:]
	}
	for _, a := range acts {
		lines = append(lines, m.actLine(a, w))
	}
	return lines
}

func (m *pilotModel) View() string {
	w, h := m.width, m.height
	if w <= 0 || h <= 0 {
		return ""
	}
	w = max(w, 20)
	frame := m.frame
	if reducedMotion() {
		frame = 6 // a still mascot, eyes open
	}
	top := []string{m.headLine(w), m.progressLine(w), ""}
	big := h >= 26 && w >= 72
	switch {
	case big:
		// Room for the big head: what it says and what is running beside it.
		art := avatar(m.mood(), frame)
		side := append(m.bubble(w-avatarWidth-3), "")
		side = append(side, m.nowLines(w-avatarWidth-3)...)
		for i, line := range art {
			tip := "  "
			if i == 2 {
				tip = dimStyle.Render(gl.Pointing) + " "
			}
			row := " " + accentStyle.Render(line) + tip
			if i < len(side) {
				row += side[i]
			}
			top = append(top, row)
		}
	case h >= 18 && w >= compactWidth:
		art := mascot(m.mood(), frame)
		bubble := m.bubble(w - mascotWidth - 3)
		for i, line := range art {
			tip := "  "
			if i == 1 {
				tip = dimStyle.Render(gl.Pointing) + " "
			}
			row := " " + accentStyle.Render(line) + tip
			if i < len(bubble) {
				row += bubble[i]
			}
			top = append(top, row)
		}
	case m.cur >= 0:
		// A short or narrow terminal: the stage on one line, without the mascot.
		top = append(top, truncateANSI(" "+titleStyle.Render(i18n.T("full.stage", m.ord, m.of))+mutedStyle.Render(" "+gl.Sep+" ")+cmdStyle.Render(m.stages[m.cur].Title), w))
	}
	if !big {
		top = append(top, m.nowLines(w)...)
	}
	keys := hintLine(w, hint{action: i18n.T("uix.pilot_alone")}, hk("ctrl+c", "uix.stop_run").must())
	if m.stopping {
		keys = hintLine(w, hint{action: i18n.T("uix.pilot_stopping")}, hk("ctrl+c", "uix.leave_now").must())
	}
	foot := strings.Split(keys, "\n")

	bodyH := max(2, h-len(top)-len(foot)-2)
	var body []string
	if w >= 84 {
		routeW := 30
		route := m.routeLines(routeW, bodyH)
		done := m.doneLines(w-routeW-5, bodyH)
		sep := faintStyle.Render(" " + gl.VBar + " ")
		for i := 0; i < max(len(route), len(done)) && i < bodyH; i++ {
			left, right := "", ""
			if i < len(route) {
				left = route[i]
			}
			if i < len(done) {
				right = done[i]
			}
			body = append(body, " "+left+strings.Repeat(" ", max(0, routeW-lipgloss.Width(left)))+sep+right)
		}
	} else {
		for _, l := range m.doneLines(w-2, bodyH) {
			body = append(body, " "+l)
		}
	}
	for len(body) < bodyH {
		body = append(body, "")
	}
	lines := append(append(append(top, ""), body...), "")
	lines = append(lines, foot...)
	if len(lines) > h {
		lines = lines[:h]
	}
	return strings.Join(lines, "\n")
}
