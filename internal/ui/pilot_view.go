package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/lcajigasm/omega/internal/i18n"
)

// ---------- view ----------
//
// The screen, top to bottom:
//
//	the title, the model and the clock            elapsed · cost
//	CPU ━━━─── 23%   RAM ━━━━── 62%   DISK ━━── 45%   NET ↓1 MB/s ↑4 KB/s
//	━━━━━━━ ━━━━━━━ ━━━━━━━ ───────    progress, one segment per stage
//
//	  the mascot   ╭─ Stage 2 of 4 · Update ───── omega update · 21s ─╮
//	               ┤ what it says, in the model's own words             │
//	               ╰────────────────────────────────────────────────────╯
//	                 ✻ the command that is running · 3s
//	                   └ the last line it printed
//
//	ROUTE ─────── 1/4   ACTIVITY ──────────────── 7 done   FOR YOU LATER ── 1
//	✓ Check omega  2s   ── Update                          ✗ Step(pipx)
//	▌✻ Update     21s    ✓ Step(brew)              4s        exit 1
//	· Clean up            …
//	────────────────────────────────────────────────────────────────────────────
//	  ctrl+c stop the run · s skip this stage · ↑↓ scroll    nobody needs to …

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

// fit pads or cuts a coloured line to exactly w columns.
func fit(s string, w int) string {
	s = truncateANSI(s, w)
	return s + strings.Repeat(" ", max(0, w-lipgloss.Width(s)))
}

func (m *pilotModel) spin() string { return titleStyle.Render(spinFrame(m.frame)) }

func (m *pilotModel) mark(i int) string {
	if m.res[i].Status == StageRunning {
		return m.spin()
	}
	return statusMark(m.res[i].Status)
}

// ---------- the top: title and progress ----------

func (m *pilotModel) headLine(w int) string {
	cost := ""
	if c, ok := m.cost(); ok {
		cost = FormatCost(c)
	}
	// What runs how (a dry run, the demo) matters most, then the model;
	// the version goes first when there is no room. Nothing is cut in half.
	var info []string
	if m.info.Version != "" {
		info = append(info, m.info.Version)
	}
	if m.header.Model != "" {
		info = append(info, m.header.Model)
	}
	info = append(info, m.info.Flags...)
	return topBar(i18n.T("pilot.title"), info, rightInfo(humanDuration(time.Since(m.began)), cost), w)
}

// barLine is the progress of the run: one segment per stage, coloured by
// how it went, a pulse running along the one that is running; then how
// many have ended and about how long the rest will take.
func (m *pilotModel) barLine(w int) string {
	total, settled := m.progress()
	label := fmt.Sprintf("%d/%d", settled, total)
	if est, ok := m.eta(); ok && est >= time.Second {
		label += " " + gl.Sep + " " + i18n.T("pilot.eta", humanDuration(est))
	}
	label = mutedStyle.Render(label)
	room := w - 3 - lipgloss.Width(label) - 2
	var idx []int
	for i, s := range m.stages {
		if s.Selected {
			idx = append(idx, i)
		}
	}
	if total == 0 || room < 8 {
		return truncateANSI(" "+label, w)
	}
	seg := (room - (total - 1)) / total
	var b strings.Builder
	if seg < 2 {
		// Too many stages for segments: one bar filled to the share done.
		fill := room * settled / total
		b.WriteString(accentStyle.Render(strings.Repeat(gl.Heavy, fill)) + faintStyle.Render(strings.Repeat(gl.Rule, room-fill)))
	} else {
		extra := room - (total - 1) - seg*total // the last segment takes what is left
		for n, i := range idx {
			sw := seg
			if n == len(idx)-1 {
				sw += extra
			}
			if n > 0 {
				b.WriteString(" ")
			}
			b.WriteString(m.segment(i, sw))
		}
	}
	return " " + b.String() + "  " + label
}

// segment is one stage on the progress bar, sw columns wide.
func (m *pilotModel) segment(i, sw int) string {
	switch m.res[i].Status {
	case StageDone:
		return okStyle.Render(strings.Repeat(gl.Heavy, sw))
	case StageFailed:
		return errStyle.Render(strings.Repeat(gl.Heavy, sw))
	case StageStopped:
		return warnPlainStyle.Render(strings.Repeat(gl.Heavy, sw))
	case StageRunning:
		if reducedMotion() {
			return accentStyle.Render(strings.Repeat(gl.Heavy, sw))
		}
		at := m.frame % (sw + 6)
		var b strings.Builder
		for x := 0; x < sw; x++ {
			switch d := at - x; {
			case d == 0:
				b.WriteString(th.shimBright.Render(gl.Heavy))
			case d > 0 && d <= 3:
				b.WriteString(accentStyle.Render(gl.Heavy))
			default:
				b.WriteString(dimStyle.Render(gl.Heavy))
			}
		}
		return b.String()
	}
	return faintStyle.Render(strings.Repeat(gl.Rule, sw))
}

// ---------- the mascot and what it says ----------

// bubbleHead is the title on the bubble's frame: the stage it is on, or
// how the run is going between stages.
func (m *pilotModel) bubbleHead() (left, right string) {
	switch {
	case m.finished:
		return titleStyle.Render(gl.Brand), ""
	case m.stopping:
		return warnStyle.Render(i18n.T("full.stopping")), ""
	case m.cur >= 0:
		s := m.stages[m.cur]
		return titleStyle.Render(i18n.T("full.stage", m.ord, m.of)) + dimStyle.Render(" "+gl.Sep+" ") + cmdStyle.Render(s.Title),
			faintStyle.Render(s.Command + " " + gl.Sep + " " + humanDuration(time.Since(m.st[m.cur].began)))
	case m.last >= 0:
		return mutedStyle.Render(i18n.T("pilot.waiting")), ""
	}
	return mutedStyle.Render(i18n.T("pilot.title")), ""
}

// bubble is what the mascot says, in a frame w columns wide (its tail
// included) with up to lines lines of text. The tail, on row tail, points
// back at the mascot.
func (m *pilotModel) bubble(w, lines, tail int) []string {
	b := border()
	frame := dimStyle
	switch {
	case m.stopping || m.skipping:
		frame = warnPlainStyle
	case m.finished && m.mood() == moodSad:
		frame = errStyle
	case m.finished:
		frame = okStyle
	}
	bw := max(14, w-2) // the frame, without the tail
	inner := bw - 4
	left, right := m.bubbleHead()
	// ╭─ left ───────── right ─╮: the frame takes 6 columns, the right
	// title 2 more, and the rule fills the rest.
	room := bw - 6
	if right != "" && lipgloss.Width(left)+lipgloss.Width(right)+2+3 > room {
		right = ""
	}
	if right != "" {
		room -= lipgloss.Width(right) + 2
	}
	left = truncateANSI(left, room-1)
	top := frame.Render(b.TopLeft+b.Top+" ") + left + " " + frame.Render(strings.Repeat(b.Top, max(0, room-lipgloss.Width(left))))
	if right != "" {
		top += " " + right + " "
	}
	top += frame.Render(b.Top + b.TopRight)

	text := wrapText(plainSay(m.say), inner)
	if len(text) > lines {
		text = text[:lines]
		last := []rune(text[lines-1])
		text[lines-1] = string(last[:min(len(last), inner-1)]) + gl.Ellipsis
	}
	for len(text) < min(lines, 2) {
		text = append(text, "")
	}
	ink := lipgloss.NewStyle()
	if m.finished {
		ink = cmdStyle
	}
	out := []string{"  " + top}
	for i, t := range text {
		lead, edge := "  ", frame.Render(b.Left)
		if i+1 == tail {
			l, e := tailGlyphs()
			lead, edge = frame.Render(l), frame.Render(e)
		}
		out = append(out, lead+edge+" "+ink.Render(fit(t, inner))+" "+frame.Render(b.Right))
	}
	out = append(out, "  "+frame.Render(b.BottomLeft+strings.Repeat(b.Bottom, bw-2)+b.BottomRight))
	return out
}

// tailGlyphs is the bubble's tail and the side of the frame it joins.
func tailGlyphs() (string, string) {
	if asciiMode() {
		return " <", "|"
	}
	return "╶─", "┤"
}

// nowLines are the command that is running and the last thing it printed.
func (m *pilotModel) nowLines(w int) []string {
	now, out := "", ""
	run := m.running()
	switch {
	case m.skipping:
		now = "  " + warnStyle.Render(gl.Stopped) + " " + warnPlainStyle.Render(i18n.T("uix.pilot_skipping"))
	case run >= 0:
		a := m.acts[run]
		label := cmdStyle.Render(a.name)
		if a.arg != "" {
			label += mutedStyle.Render("(" + a.arg + ")")
		}
		took := faintStyle.Render("  " + humanDuration(time.Since(a.began)))
		now = "  " + m.spin() + " " + truncateANSI(label, max(10, w-6-lipgloss.Width(took))) + took
	case m.thinking:
		now = "  " + m.spin() + " " + shimmerAt(i18n.T("pilot.thinking"), m.frame) + faintStyle.Render("  "+humanDuration(time.Since(m.thinkAt)))
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

// hero is the mascot with what it says beside it, sized for the terminal.
func (m *pilotModel) hero(w, h int) []string {
	frame := m.frame
	if reducedMotion() {
		frame = 7 // a still mascot, eyes open
	}
	mood := m.mood()
	var rows []string
	switch {
	case h >= 26 && w >= 80:
		// The big Ω, with the bubble at the height of its eyes and what is
		// running under it.
		art := avatar(mood, frame)
		rw := w - avatarWidth - 2
		side := append([]string{"", ""}, m.bubble(rw, 3, 1)...)
		side = append(side, "")
		side = append(side, m.nowLines(rw)...)
		for i := 0; i < max(len(art), len(side)); i++ {
			left := strings.Repeat(" ", avatarWidth)
			if i < len(art) {
				left = art[i]
			}
			right := ""
			if i < len(side) {
				right = side[i]
			}
			rows = append(rows, " "+left+" "+right)
		}
	case h >= 16 && w >= 50:
		art := mascot(mood, frame)
		bubble := m.bubble(w-mascotWidth-3, 2, 1)
		for i := 0; i < max(len(art), len(bubble)); i++ {
			left := strings.Repeat(" ", mascotWidth)
			if i < len(art) {
				left = art[i]
			}
			right := ""
			if i < len(bubble) {
				right = bubble[i]
			}
			rows = append(rows, " "+left+" "+right)
		}
		rows = append(rows, m.nowLines(w)...)
	default:
		// A short or narrow terminal: the stage on one line, no mascot.
		left, right := m.bubbleHead()
		rows = append(rows, spread(" "+left, right+" ", w))
		rows = append(rows, m.nowLines(w)...)
	}
	return rows
}

// ---------- the panels ----------

// panelHead is the title of a panel: its name, a rule and what it counts.
func panelHead(title, count string, w int) string {
	left := titleStyle.Render(title)
	right := ""
	if count != "" {
		right = " " + mutedStyle.Render(count)
	}
	rule := w - lipgloss.Width(left) - lipgloss.Width(right) - 1
	if rule < 2 {
		return spread(left, mutedStyle.Render(count), w)
	}
	return left + " " + faintStyle.Render(strings.Repeat(gl.Rule, rule)) + right
}

// routeLines is every stage and how it went; under the one that is
// running, its command and what it has done so far.
func (m *pilotModel) routeLines(w, h int) []string {
	total, settled := m.progress()
	lines := []string{panelHead(i18n.T("full.route"), fmt.Sprintf("%d/%d", settled, total), w)}
	cur := 0 // row of the stage that ran last
	for i, s := range m.stages {
		if !s.Selected {
			continue // left out of this run: the summary names it
		}
		if i == m.last {
			cur = len(lines) - 1
		}
		r := m.res[i]
		tail, tailStyle := "", faintStyle
		switch r.Status {
		case StageRunning:
			tail = humanDuration(time.Since(m.st[i].began))
			tailStyle = mutedStyle
		case StageDone:
			if r.Duration >= time.Second {
				tail = humanDuration(r.Duration)
			}
		case StageFailed:
			tail, tailStyle = StatusWord(r.Status), errStyle
		case StageStopped:
			tail, tailStyle = StatusWord(r.Status), warnPlainStyle
		}
		lead := " "
		name := s.Title
		switch r.Status {
		case StageRunning:
			lead = accentStyle.Render("▌")
			if asciiMode() {
				lead = accentStyle.Render(">")
			}
			name = cmdStyle.Render(truncate(name, max(8, w-4-lipgloss.Width(tail))))
		case StagePending:
			name = mutedStyle.Render(truncate(name, max(8, w-4-lipgloss.Width(tail))))
		default:
			name = truncate(name, max(8, w-4-lipgloss.Width(tail)))
		}
		lines = append(lines, spread(lead+m.mark(i)+" "+name, tailStyle.Render(tail), w))
		if r.Status == StageRunning {
			ok, failed, denied := m.tally(i)
			detail := s.Command
			if ok+failed+denied > 0 {
				detail += fmt.Sprintf("  %s%d", gl.Check, ok)
				if failed > 0 {
					detail += fmt.Sprintf(" %s%d", gl.Cross, failed)
				}
				if denied > 0 {
					detail += fmt.Sprintf(" %s%d", gl.Denied, denied)
				}
			}
			lines = append(lines, faintStyle.Render(truncate("   "+detail, w)))
		}
	}
	if len(lines) > h && h > 1 {
		// No room for every stage: keep the ones around the running one.
		rows := lines[1:]
		from := max(0, min(cur-(h-1)/2, len(rows)-(h-1)))
		lines = append(lines[:1], rows[from:from+h-1]...)
	}
	return lines
}

// actLabel names an action: the tool, what it ran, and why it did not go
// through.
func actLabel(a pilotAct, w int, withNote bool) string {
	name, arg := a.name, ""
	if a.arg != "" {
		arg = "(" + a.arg + ")"
	}
	note := ""
	if withNote && a.note != "" {
		note = " " + gl.Sep + " " + a.note
	}
	plain := truncate(name+arg+note, max(8, w))
	// Colour what is left after the cut, part by part.
	switch {
	case len([]rune(plain)) <= len([]rune(name)):
		return plain
	case len([]rune(plain)) <= len([]rune(name+arg)):
		return name + mutedStyle.Render(string([]rune(plain)[len([]rune(name)):]))
	}
	return name + mutedStyle.Render(arg) + faintStyle.Render(string([]rune(plain)[len([]rune(name+arg)):]))
}

// actLine is one line of the list of what has run.
func (m *pilotModel) actLine(a pilotAct, w int) string {
	if a.kind == actStage {
		head := titleStyle.Render(gl.Rule + " " + truncate(a.name, max(8, w-6)))
		return head + " " + faintStyle.Render(strings.Repeat(gl.Rule, max(0, w-lipgloss.Width(head)-1)))
	}
	mark, tail := mutedStyle.Render(gl.Pending), ""
	tailStyle := faintStyle
	switch a.kind {
	case actRunning:
		mark, tail = m.spin(), humanDuration(time.Since(a.began))
		tailStyle = mutedStyle
	case actOK:
		mark = okStyle.Render(gl.Check)
		if a.dur >= time.Second {
			tail = humanDuration(a.dur)
		}
	case actFailed:
		mark, tailStyle = errStyle.Render(gl.Cross), errStyle
		if a.exit > 0 {
			tail = fmt.Sprintf("exit %d", a.exit)
		}
	case actDenied:
		mark, tail, tailStyle = warnStyle.Render(gl.Denied), i18n.T("ui.denied"), warnPlainStyle
	case actError:
		mark = errStyle.Render(gl.Error)
	case actNote:
		mark = dimStyle.Render(gl.Note)
	}
	room := w - 4 - lipgloss.Width(tail)
	label := actLabel(a, room, true)
	if a.kind == actNote || a.kind == actError {
		label = mutedStyle.Render(truncate(a.name, max(8, room)))
	}
	return spread("  "+mark+" "+label, tailStyle.Render(tail), w)
}

// doneLines is the list of what has run, the latest at the bottom.
func (m *pilotModel) doneLines(w, h int) []string {
	ok, failed, denied := m.tally(-1)
	head, count := i18n.T("pilot.done_head"), i18n.T("pilot.tally", ok, failed, denied)
	if lipgloss.Width(head)+lipgloss.Width(count)+6 > w {
		// No room for the words: the marks say the same.
		count = fmt.Sprintf("%s%d %s%d %s%d", gl.Check, ok, gl.Cross, failed, gl.Denied, denied)
	}
	lines := []string{panelHead(head, count, w)}
	if len(m.acts) == 0 {
		return append(lines, mutedStyle.Render("  "+i18n.T("pilot.nothing_yet")))
	}
	acts := m.acts
	room := max(1, h-1)
	// Scrolled back with the arrows: the list ends that many lines higher.
	m.scroll = min(m.scroll, max(0, len(acts)-room))
	end := len(acts) - m.scroll
	if len(acts) > room {
		acts = acts[max(0, end-room):end]
	}
	for _, a := range acts {
		lines = append(lines, m.actLine(a, w))
	}
	if m.scroll > 0 && len(lines) > 1 {
		// The last line says there is more below, and how to get back.
		lines[len(lines)-1] = faintStyle.Render("  " + gl.Down + " " + i18n.T("pilot.more_below", m.scroll))
	}
	return lines
}

// attention is what did not go through: what the user may want to look at
// when they come back.
func (m *pilotModel) attention() []pilotAct {
	var out []pilotAct
	for _, a := range m.acts {
		if a.kind == actFailed || a.kind == actDenied || a.kind == actError {
			out = append(out, a)
		}
	}
	return out
}

// attentionLines is the panel of what did not go through, the latest at
// the bottom, each with the stage it was in and why.
func (m *pilotModel) attentionLines(w, h int) []string {
	items := m.attention()
	count := ""
	if len(items) > 0 {
		count = fmt.Sprint(len(items))
	}
	lines := []string{panelHead(i18n.T("pilot.attention_head"), count, w)}
	if len(items) == 0 {
		for _, l := range wrapText(i18n.T("pilot.attention_none"), max(10, w-2)) {
			lines = append(lines, mutedStyle.Render(" "+l))
		}
		return lines
	}
	var rows []string
	for _, a := range items {
		mark := errStyle.Render(gl.Cross)
		switch a.kind {
		case actDenied:
			mark = warnStyle.Render(gl.Denied)
		case actError:
			mark = errStyle.Render(gl.Error)
		}
		label := actLabel(a, w-3, false)
		if a.kind == actError {
			label = truncate(a.name, max(8, w-3))
		}
		rows = append(rows, " "+mark+" "+label)
		why := a.note
		if why == "" && a.exit > 0 {
			why = fmt.Sprintf("exit %d", a.exit)
		}
		where := m.stages[a.stage].Title
		if why != "" {
			where += " " + gl.Sep + " " + why
		}
		wrapped := wrapText(where, max(10, w-3))
		if len(wrapped) > 2 {
			wrapped = wrapped[:2]
			wrapped[1] = truncate(wrapped[1]+" "+gl.Ellipsis, max(10, w-3))
		}
		for _, l := range wrapped {
			rows = append(rows, faintStyle.Render("   "+l))
		}
	}
	if room := max(1, h-1); len(rows) > room {
		rows = rows[len(rows)-room:]
	}
	return append(lines, rows...)
}

// ---------- the bottom ----------

func (m *pilotModel) footer(w int) []string {
	var keys []hint
	var note string
	switch {
	case m.finished:
		keys = []hint{hk("ctrl+c", "uix.leave_now").must()}
	case m.stopping:
		keys, note = []hint{hk("ctrl+c", "uix.leave_now").must()}, warnPlainStyle.Render(i18n.T("uix.pilot_stopping"))
	default:
		keys = []hint{hk("ctrl+c", "uix.stop_run").must()}
		if m.cur >= 0 && m.cancel != nil && !m.skipping {
			keys = append(keys, hk("s", "uix.skip_stage").must())
		}
		if len(m.acts) > 1 {
			keys = append(keys, hk("↑↓", "uix.scroll"))
		}
		note = mutedStyle.Render(i18n.T("uix.pilot_alone"))
	}
	return chromeFooter(w, note, keys...)
}

func (m *pilotModel) View() string {
	w, h := m.width, m.height
	if w <= 0 || h <= 0 {
		return ""
	}
	w = max(w, 20)
	top := []string{m.headLine(w), statsLine(w), m.barLine(w), ""}
	top = append(top, m.hero(w, h)...)
	foot := m.footer(w)

	bodyH := max(2, h-len(top)-len(foot)-1)
	var body []string
	join := func(cols [][]string, widths []int, gap int) {
		for i := 0; i < bodyH; i++ {
			var row strings.Builder
			row.WriteString(" ")
			for c, col := range cols {
				if c > 0 {
					row.WriteString(strings.Repeat(" ", gap))
				}
				line := ""
				if i < len(col) {
					line = col[i]
				}
				if c == len(cols)-1 {
					row.WriteString(line)
				} else {
					row.WriteString(fit(line, widths[c]))
				}
			}
			body = append(body, row.String())
		}
	}
	switch {
	case w >= 120:
		// Three columns: the route, what has run, and what needs the user.
		routeW, attW := 30, 36
		doneW := w - routeW - attW - 2 - 6
		join([][]string{m.routeLines(routeW, bodyH), m.doneLines(doneW, bodyH), m.attentionLines(attW, bodyH)},
			[]int{routeW, doneW, attW}, 3)
	case w >= 84:
		// Two: the route with what needs the user under it, and what has run.
		routeW := 32
		left := m.routeLines(routeW, bodyH)
		if room := bodyH - len(left) - 1; room >= 3 {
			left = append(append(left, ""), m.attentionLines(routeW, room)...)
		}
		join([][]string{left, m.doneLines(w-routeW-2-3, bodyH)}, []int{routeW, w - routeW - 5}, 3)
	default:
		join([][]string{m.doneLines(w-2, bodyH)}, []int{w - 2}, 0)
	}
	lines := append(append(top, ""), body...)
	lines = append(lines, foot...)
	if len(lines) > h {
		lines = lines[:h]
	}
	for i := range lines {
		lines[i] = truncateANSI(strings.TrimRight(lines[i], " "), w)
	}
	return strings.Join(lines, "\n")
}
