package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/lcajigasm/omega/internal/i18n"
)

// ---------- views ----------

// stageMark is the sign of a stage on the route.
func (m *model) stageMark(i int) string {
	g := m.g
	switch g.res[i].Status {
	case StageRunning:
		return titleStyle.Render(spinFrame(m.frame))
	case StagePending:
		if !g.stages[i].Selected {
			return faintStyle.Render(gl.Skipped)
		}
	}
	return statusMark(g.res[i].Status)
}

// statusMark is the sign of a stage outcome.
func statusMark(status string) string {
	switch status {
	case StageDone:
		return okStyle.Render(gl.Check)
	case StageFailed:
		return errStyle.Render(gl.Cross)
	case StageStopped:
		return warnStyle.Render(gl.Stopped)
	case StageSkipped:
		return faintStyle.Render(gl.Skipped)
	}
	return mutedStyle.Render(gl.Pending)
}

// StatusWord is the localized name of a stage outcome.
func StatusWord(status string) string { return i18n.T("full.st_" + status) }

func riskWord(risk string) string {
	switch risk {
	case "read-only":
		return okStyle.Render(i18n.T("full.risk_readonly"))
	case "dangerous":
		return warnStyle.Render(i18n.T("full.risk_dangerous"))
	}
	return mutedStyle.Render(i18n.T("full.risk_normal"))
}

// fieldCol is the width of the label column of a field.
const fieldCol = 14

// field is a labelled paragraph: the label in a column, the text wrapped
// beside it. On a narrow terminal the label goes on a line of its own.
func field(label, text string, width int) string {
	const col = fieldCol
	if width < compactWidth {
		w := max(10, width)
		return faintStyle.Render(label) + "\n" + strings.TrimRight(lipgloss.NewStyle().Width(w).Render(text), " ")
	}
	w := width - col
	lines := strings.Split(lipgloss.NewStyle().Width(w).Render(text), "\n")
	var b strings.Builder
	for i, l := range lines {
		if i == 0 {
			b.WriteString(faintStyle.Render(pad(label, col)))
		} else {
			b.WriteString("\n" + strings.Repeat(" ", col))
		}
		b.WriteString(strings.TrimRight(l, " "))
	}
	return b.String()
}

// fieldRow is a label and a short value on one line, or on two when the
// terminal is narrow.
func fieldRow(label, value string, width int) string {
	if width < compactWidth {
		return faintStyle.Render(label) + "\n" + value
	}
	return faintStyle.Render(pad(label, fieldCol)) + value
}

// paintStage is the heading a stage opens in the transcript.
func (m *model) paintStage(e *entry) string {
	s := m.g.stages[e.stage]
	w := max(20, m.tw()-2)
	head := " " + i18n.T("full.stage", e.ord, e.of) + " " + gl.Sep + " " + s.Title + " "
	fill := w - lipgloss.Width(head) - 2
	if fill < 2 {
		fill = 2
	}
	var b strings.Builder
	b.WriteString(truncateANSI(titleStyle.Render(strings.Repeat(gl.Heavy, 2)+head)+dimStyle.Render(strings.Repeat(gl.Heavy, fill)), w) + "\n")
	b.WriteString(field(i18n.T("full.what"), s.What, w) + "\n")
	b.WriteString(field(i18n.T("full.why"), s.Why, w) + "\n")
	b.WriteString(fieldRow(i18n.T("full.alone"), cmdStyle.Render(s.Command)+"  "+riskWord(s.Risk), w))
	if e.text != "" {
		indent := fieldCol
		if w < compactWidth {
			indent = 0
		}
		b.WriteString("\n" + strings.Repeat(" ", indent) + faintStyle.Render(truncate(e.text, w-indent)))
	}
	return b.String()
}

// planView is the first screen: every stage, what it does and why, and
// which ones will run. The list scrolls around the cursor when the terminal
// is short; below compactWidth columns the card loses its border.
func (m *model) planView() string {
	g := m.g
	w := max(20, min(m.width, 110))
	compact := w < compactWidth
	inner := w - 4
	if compact {
		inner = w - 1
	}
	var top strings.Builder
	intro := i18n.T("full.plan_intro")
	if g.info.Ultra {
		intro = i18n.T("ultra.plan_intro")
	}
	if len(g.info.Flags) > 0 {
		intro += " " + i18n.T("full.plan_flags", strings.Join(g.info.Flags, ", "))
	}
	top.WriteString(mutedStyle.Width(w-2).PaddingLeft(1).Render(intro) + "\n")

	titleW := 0
	for _, s := range g.stages {
		titleW = max(titleW, lipgloss.Width(s.Title))
	}
	titleW = min(titleW, max(8, w-14))
	var rows []string
	for i, s := range g.stages {
		pointer := "  "
		if i == g.cursor {
			pointer = titleStyle.Render(gl.Pointer + " ")
		}
		box := mutedStyle.Render(gl.BoxOff)
		st := g.res[i].Status
		switch {
		case st != StagePending && st != StageSkipped:
			box = m.stageMark(i)
		case s.Selected && st == StagePending:
			box = riskStyle(s.Risk).Render(gl.BoxOn)
		}
		title := pad(truncate(s.Title, titleW), titleW)
		if i == g.cursor {
			title = cmdStyle.Render(title)
		}
		tail := faintStyle.Render(s.Command)
		if st != StagePending {
			tail = mutedStyle.Render(StatusWord(st))
		}
		row := fmt.Sprintf(" %s%s %s  %s   %s", pointer, box, faintStyle.Render(fmt.Sprintf("%2d", i+1)), title, tail)
		rows = append(rows, truncateANSI(row, w))
	}

	s := g.stages[g.cursor]
	var card strings.Builder
	card.WriteString(titleStyle.Render(s.Title) + mutedStyle.Render("  "+s.Command) + "\n")
	card.WriteString(field(i18n.T("full.what"), s.What, inner) + "\n")
	card.WriteString(field(i18n.T("full.why"), s.Why, inner) + "\n")
	card.WriteString(fieldRow(i18n.T("full.risk"), riskWord(s.Risk), inner))
	cardView := card.String()
	if !compact {
		cardView = cardStyle.Width(w - 2).Render(cardView)
	}

	total, settled := g.chosen()
	count := " " + mutedStyle.Render(i18n.T("full.plan_count", total-settled, len(g.stages)))
	hints := []hint{hk("↑↓", "uix.move"), hk("space", "uix.tick"), hk("a", "uix.all"), hk("enter", "uix.start").must(), hk("q", "uix.leave").must()}
	if g.planBack {
		hints = []hint{hk("↑↓", "uix.move"), hk("space", "uix.tick"), hk("a", "uix.all"), hk("enter", "uix.back_run").must()}
	}
	keyLine := strings.Join(chromeFooter(max(m.width, 20), "", hints...), "\n")

	// The list takes what the rest leaves, scrolled to keep the cursor in sight.
	room := len(rows)
	if m.height > 0 {
		used := chromeRows + lipgloss.Height(top.String()) + 1 + 1 + lipgloss.Height(cardView) + 1 + lipgloss.Height(keyLine)
		room = max(3, m.height-used)
	}
	if len(rows) > room {
		start := max(0, min(g.cursor-room/2, len(rows)-room))
		end := start + room
		view := append([]string(nil), rows[start:end]...)
		if start > 0 {
			view[0] = "    " + moreAbove(start)
		}
		if end < len(rows) {
			view[len(view)-1] = "    " + moreBelow(len(rows)-end)
		}
		rows = view
	}
	body := top.String() + "\n" + strings.Join(rows, "\n") + "\n\n" + cardView + "\n" + count
	// The keys sit on the last rows of the terminal, as on every other
	// screen, however short the list is.
	if m.height > 0 {
		if gap := m.height - chromeRows - lipgloss.Height(body) - lipgloss.Height(keyLine); gap > 0 {
			body += strings.Repeat("\n", gap)
		}
	}
	return body + "\n" + keyLine
}

// outcomeLine says how a stage ended, for the card between two stages.
func (m *model) outcomeLine(i int) string {
	g := m.g
	r, s := g.res[i], g.stages[i]
	var parts []string
	switch r.Status {
	case StageDone:
		parts = append(parts, okStyle.Render(gl.Check+" ")+cmdStyle.Render(s.Title)+" "+mutedStyle.Render(i18n.T("full.finished", humanDuration(r.Duration))))
	case StageFailed:
		line := errStyle.Render(gl.Cross+" ") + cmdStyle.Render(s.Title) + " " + errStyle.Render(StatusWord(StageFailed))
		if r.Note != "" {
			line += mutedStyle.Render(" " + gl.Sep + " " + truncate(firstSentence(r.Note), 60))
		}
		parts = append(parts, line)
	case StageStopped:
		parts = append(parts, warnStyle.Render(gl.Stopped+" ")+cmdStyle.Render(s.Title)+" "+mutedStyle.Render(StatusWord(StageStopped)))
	default:
		parts = append(parts, faintStyle.Render(gl.Skipped+" ")+cmdStyle.Render(s.Title)+" "+mutedStyle.Render(StatusWord(r.Status)))
	}
	if r.CostKnown && r.Cost > 0 {
		parts = append(parts, mutedStyle.Render(FormatCost(r.Cost)))
	}
	return strings.Join(parts, mutedStyle.Render(" "+gl.Sep+" "))
}

// betweenView is the card between two stages: how the last one ended,
// what comes next and why, and what the user can do about it.
func (m *model) betweenView() string {
	g := m.g
	compact := m.width < compactWidth
	inner := m.width - 6
	if compact {
		inner = m.width - 1
	}
	var b strings.Builder
	retry := false
	if g.last >= 0 {
		b.WriteString(m.outcomeLine(g.last) + "\n\n")
		retry = g.res[g.last].Status == StageFailed || g.res[g.last].Status == StageStopped
	}
	total, _ := g.chosen()
	s := g.stages[g.next]
	b.WriteString(titleStyle.Render(i18n.T("full.next")) + mutedStyle.Render(" "+gl.Sep+" "+i18n.T("full.stage", g.ordinal(g.next), total)+" "+gl.Sep+" ") +
		cmdStyle.Render(s.Title) + faintStyle.Render("  "+s.Command) + "\n")
	b.WriteString(field(i18n.T("full.what"), s.What, inner) + "\n")
	b.WriteString(field(i18n.T("full.why"), s.Why, inner) + "\n")
	b.WriteString(fieldRow(i18n.T("full.risk"), riskWord(s.Risk), inner) + "\n\n")
	hints := []hint{hk("enter", "uix.go_on").must(), hk("s", "uix.skip_it"), hk("p", "uix.change_plan"), hk("q", "uix.finish").must()}
	if retry {
		hints = []hint{hk("enter", "uix.go_on").must(), hk("r", "uix.repeat"), hk("s", "uix.skip_next"), hk("p", "uix.change_plan"), hk("q", "uix.finish").must()}
	}
	b.WriteString(cardHints(inner, hints...))
	if compact {
		return b.String()
	}
	return cardStyle.Width(m.width - 2).Render(b.String())
}

// guideFooter is the footer while the guided run, not a session, has the
// keyboard.
func (m *model) guideFooter() (string, bool) {
	switch m.state {
	case stBetween:
		return m.betweenView() + "\n" + m.statusBlock(), true
	case stSummary:
		return m.statusBlock() + "\n" + hintLine(m.width, hk("enter/q", "uix.leave").must(), hk("↑↓", "uix.scroll"),
			hk("ctrl+f", "uix.search"), hk("ctrl+o", "uix.expand")), true
	}
	if m.ag == nil || m.state == stDone {
		top := ""
		if m.g.cur >= 0 {
			glyph := titleStyle.Render(spinFrame(m.frame))
			top = truncateANSI(glyph+" "+m.shimmer(m.g.stages[m.g.cur].Title+gl.Ellipsis)+" "+faintStyle.Render(humanDuration(time.Since(m.g.began))), m.width)
		}
		return top + "\n" + m.statusBlock() + "\n" + hintLine(m.width, hk("ctrl+c", "uix.stop_stage").must(), hk("↑↓", "uix.scroll"), hk("ctrl+o", "uix.expand")), true
	}
	return "", false
}

// topRows is the height of what sits above the transcript: the chrome,
// and the route on one line when the panel, which shows it in full, has
// no room.
func (m *model) topRows() int {
	if m.g != nil && !m.panelShown() {
		return chromeRows + 1
	}
	return chromeRows
}

// chromeRows is the top bar, the stats line and a blank line under them.
const chromeRows = 3

// chromeTop is the top bar and the machine's stats, as every screen has.
func (m *model) chromeTop() string {
	w := max(m.width, 20)
	title, info := m.header.Task, []string{}
	if m.g != nil {
		title = i18n.T("full.title")
		if m.g.info.Ultra {
			title = i18n.T("ultra.title") + " " + ultraBadge.Render(" ULTRA ")
		}
		if m.g.info.Version != "" {
			info = append(info, m.g.info.Version)
		}
		if m.header.Model != "" {
			info = append(info, m.header.Model)
		} else if m.g.info.Model != "" {
			info = append(info, m.g.info.Model)
		}
		info = append(info, m.g.info.Flags...)
	} else {
		if m.header.Version != "" {
			info = append(info, m.header.Version)
		}
		if m.header.Provider != "" {
			info = append(info, m.header.Provider)
		}
	}
	return topBar(title, info, "", w) + "\n" + statsLine(w) + "\n"
}

// routeStrip is the route on one line.
func (m *model) routeStrip() string {
	g := m.g
	total, settled := g.chosen()
	var marks strings.Builder
	for i := range g.stages {
		if g.res[i].Status == StagePending && !g.stages[i].Selected {
			continue
		}
		marks.WriteString(m.stageMark(i))
	}
	s := " " + marks.String() + "  " + mutedStyle.Render(fmt.Sprintf("%d/%d", settled, total))
	if g.cur >= 0 {
		s += mutedStyle.Render(" "+gl.Sep+" ") + cmdStyle.Render(g.stages[g.cur].Title)
	}
	return truncateANSI(s, m.width)
}

// routeLines is the route for the session panel: every stage and how it went.
func (m *model) routeLines(inner int) []string {
	g := m.g
	total, settled := g.chosen()
	lines := []string{panelHead(i18n.T("full.route"), fmt.Sprintf("%d/%d", settled, total), inner)}
	for i, s := range g.stages {
		name := truncate(s.Title, max(10, inner-2))
		switch {
		case i == g.cur:
			name = cmdStyle.Render(name)
		case m.state == stBetween && i == g.next:
			name = titleStyle.Render(name)
		case g.res[i].Status == StageSkipped || (g.res[i].Status == StagePending && !s.Selected):
			name = faintStyle.Render(name)
		case g.res[i].Status == StagePending:
			name = mutedStyle.Render(name)
		}
		lines = append(lines, m.stageMark(i)+" "+name)
	}
	return append(lines, "")
}

// SummaryLines says what happened to every stage of a guided run, and what
// it adds up to.
func SummaryLines(stages []GuideStage, res []StageResult) []string {
	return summaryLines(stages, res, reportWidth)
}

// summaryLines is SummaryLines for a screen width columns wide: a long note
// wraps under its stage instead of being cut. Only a stage that failed or
// was stopped has its note in the error colour.
func summaryLines(stages []GuideStage, res []StageResult, width int) []string {
	lines := []string{titleStyle.Render(strings.Repeat(gl.Heavy, 2) + " " + i18n.T("full.summary"))}
	titleW := 0
	for _, s := range stages {
		titleW = max(titleW, lipgloss.Width(s.Title))
	}
	titleW = min(titleW, max(8, width/2))
	count := map[string]int{}
	var took time.Duration
	var cost float64
	costOK := false
	for i, s := range stages {
		r := res[i]
		count[r.Status]++
		took += r.Duration
		if r.CostKnown {
			cost += r.Cost
			costOK = true
		}
		line := fmt.Sprintf("  %s %s  %s", statusMark(r.Status), pad(truncate(s.Title, titleW), titleW), mutedStyle.Render(StatusWord(r.Status)))
		if r.Duration >= time.Second {
			line += faintStyle.Render(" " + gl.Sep + " " + humanDuration(r.Duration))
		}
		if r.Status == StageFailed || r.Status == StageStopped || r.Status == StagePending {
			line += faintStyle.Render("  → " + s.Command)
		}
		lines = append(lines, line)
		if r.Note != "" {
			st := mutedStyle
			if r.Status == StageFailed || r.Status == StageStopped {
				st = errStyle
			}
			for _, l := range wrapText(r.Note, max(20, width-6)) {
				lines = append(lines, "      "+st.Render(l))
			}
		}
	}
	total := i18n.T("full.total", count[StageDone], count[StageFailed]+count[StageStopped], count[StageSkipped], count[StagePending])
	if took >= time.Second {
		total += " " + gl.Sep + " " + humanDuration(took)
	}
	if costOK {
		total += " " + gl.Sep + " " + FormatCost(cost)
	}
	return append(lines, "", "  "+mutedStyle.Render(total))
}
