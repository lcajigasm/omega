package ui

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/lcajigasm/omega/internal/i18n"
)

func (m *model) markdown(s string) string {
	w := m.tw() - 4
	if w < 30 {
		w = 30
	}
	if m.md == nil || m.mdWidth != w {
		style := "dark"
		if !m.dark {
			style = "light"
		}
		r, err := glamour.NewTermRenderer(markdownStyle(style), glamour.WithColorProfile(lipgloss.ColorProfile()), glamour.WithWordWrap(min(w, 110)), glamour.WithEmoji())
		if err != nil {
			return s
		}
		m.md, m.mdWidth = r, w
	}
	m.mdRenders++
	out, err := m.md.Render(s)
	if err != nil {
		return s
	}
	return strings.Trim(out, "\n")
}

// shimmer paints a soft highlight that sweeps across the text. With motion
// reduced the text stands still in one colour.
func (m *model) shimmer(s string) string {
	if reducedMotion() {
		return th.shimMid.Render(s)
	}
	r := []rune(s)
	pos := m.frame % (len(r) + 8)
	var b strings.Builder
	for i, c := range r {
		d := math.Abs(float64(i - pos + 4))
		switch {
		case d < 1.5:
			b.WriteString(th.shimBright.Render(string(c)))
		case d < 3.5:
			b.WriteString(th.shimMid.Render(string(c)))
		default:
			b.WriteString(th.shimDim.Render(string(c)))
		}
	}
	return b.String()
}

// leadingSpace matches the margin glamour puts before a line, past any
// colour codes it starts with.
var leadingSpace = regexp.MustCompile(`^((?:\x1b\[[0-9;]*m)*) +`)

func (m *model) modeText() string { return modeLabel(m.mode) }

// modeShort is the approval mode in a few words, for the narrow panel.
func (m *model) modeShort() string {
	return i18n.T([...]string{"uix.mode_ask_short", "uix.mode_auto_short", "uix.mode_ro_short"}[m.mode])
}

// renderWelcome is the first entry of the transcript. Beside the panel,
// which shows the task, mode and model, it is two lines: the version and
// where the log goes. Without the panel it is a box as wide as the
// transcript.
func (m *model) renderWelcome() string {
	w := max(20, m.tw()-2)
	logPath := ""
	if m.ag != nil {
		logPath = m.ag.LogPath()
	}
	head := titleStyle.Render(gl.Brand) + mutedStyle.Render(" "+m.header.Version)
	if m.panelShown() || m.tw() < compactWidth {
		lines := []string{truncateANSI(head+mutedStyle.Render(" "+gl.Sep+" "+i18n.T("ui.welcome_tagline")), w)}
		if logPath != "" {
			label := i18n.T("ui.label_log") + " "
			lines = append(lines, faintStyle.Render(label)+mutedStyle.Render(truncate(homeTilde(logPath), w-lipgloss.Width(label))))
		}
		return strings.Join(lines, "\n")
	}
	inner := w - 6 // border and padding
	var b strings.Builder
	b.WriteString(head + "\n")
	b.WriteString(mutedStyle.Render(i18n.T("ui.welcome_tagline")) + "\n\n")
	row := func(k, v string) {
		b.WriteString(faintStyle.Render(fmt.Sprintf("%-10s", k)) + truncateANSI(v, max(10, inner-10)) + "\n")
	}
	row(i18n.T("ui.label_provider"), m.header.Provider)
	mdl := cmdStyle.Render(m.header.Model)
	if m.header.Escalate != "" && m.header.Escalate != m.header.Model {
		mdl += mutedStyle.Render("  " + i18n.T("ui.escalates", m.header.Escalate))
	}
	row(i18n.T("ui.label_model"), mdl)
	row(i18n.T("ui.label_task"), m.header.Task+mutedStyle.Render(" "+gl.Sep+" "+m.modeText()))
	if logPath != "" {
		row(i18n.T("ui.label_log"), mutedStyle.Render(truncate(homeTilde(logPath), inner-12)))
	}
	return welcomeStyle.Width(w).Render(strings.TrimRight(b.String(), "\n"))
}

// renderKey is everything paintEntry looks at. A settled card keeps its
// painted form, so a tick or a new line does not run glamour again.
func (m *model) renderKey(e *entry) string {
	focused := 0
	if m.focusedEntry() == e {
		focused = 1
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d|%d|%d|%t|%d|%s|%s|%s|%t|%t|%s|%d|%d|%s",
		e.kind, m.tw(), focused, m.expanded, len(e.output),
		e.text, e.tool, e.command, e.running, e.failed, e.denied, e.exitCode, e.lines, e.errText)
	if e.kind == enWelcome {
		fmt.Fprintf(&b, "|%t|%s|%d", m.panelShown(), m.header.Model, m.mode)
	}
	if n := len(e.output); n > 0 {
		b.WriteByte('|')
		b.WriteString(e.output[n-1])
	}
	if e.running {
		fmt.Fprintf(&b, "|%d|%s|%s", (m.frame/4)%2, humanDuration(time.Since(e.started)), humanDuration(time.Since(e.lastOut)))
	}
	return b.String()
}

func (m *model) renderEntry(e *entry) string {
	key := m.renderKey(e)
	if e.cacheOK && e.cacheKey == key {
		return e.cached
	}
	s := m.paintEntry(e)
	e.cached, e.cacheKey, e.cacheOK = s, key, true
	return s
}

// resultGutter is the margin of the lines under a tool call: the result
// glyph, padded to five columns.
func resultGutter() string {
	return "  " + gl.Result + strings.Repeat(" ", max(1, 3-lipgloss.Width(gl.Result)))
}

func (m *model) paintEntry(e *entry) string {
	w := m.tw() - 2
	switch e.kind {
	case enWelcome:
		return m.renderWelcome()
	case enUser:
		return titleStyle.Render(gl.Prompt+" ") + e.text
	case enAssistant:
		lines := strings.Split(m.markdown(e.text), "\n")
		// glamour indents by two columns; put the bullet in that gutter.
		if len(lines) > 0 {
			lines[0] = titleStyle.Render(gl.Tool) + " " + leadingSpace.ReplaceAllString(lines[0], "$1")
		}
		return strings.Join(lines, "\n")
	case enNote:
		return e.style.Render(e.text)
	case enStage:
		return m.paintStage(e)
	}

	// Tool call.
	name, arg := toolLabel(e.tool, e.command)
	dot := okStyle.Render(gl.Tool)
	switch {
	case e.running:
		if (m.frame/4)%2 == 0 || reducedMotion() {
			dot = titleStyle.Render(gl.Tool)
		} else {
			dot = dimStyle.Render(gl.Tool)
		}
	case e.denied != "":
		dot = warnStyle.Render(gl.Tool)
	case e.failed:
		dot = errStyle.Render(gl.Tool)
	}
	mark := ""
	if m.focusedEntry() == e {
		mark = titleStyle.Render(gl.Focus + " ")
	}
	head := mark + dot + " " + cmdStyle.Render(name)
	if arg != "" {
		head += mutedStyle.Render("(") + truncate(arg, max(10, w-len(name)-6)) + mutedStyle.Render(")")
	}
	if strings.HasPrefix(e.tool, "run_step:") && e.command != "" {
		head += faintStyle.Render("  " + truncate(e.command, max(10, w-lipgloss.Width(head)-4)))
	}

	var b strings.Builder
	b.WriteString(head)
	gutter := faintStyle.Render(resultGutter())
	pad := "     "
	switch {
	case e.denied != "":
		b.WriteString("\n" + gutter + warnStyle.Render(i18n.T("ui.denied")) + mutedStyle.Render(" "+gl.Sep+" "+truncate(firstSentence(e.denied), max(10, w-20))))
	case e.running:
		line := gutter + mutedStyle.Render(i18n.T("ui.running", humanDuration(time.Since(e.started))))
		if quiet := time.Since(e.lastOut); !e.lastOut.IsZero() && quiet >= silentAfter {
			line += warnPlainStyle.Render(" " + gl.Sep + " " + i18n.T("uix.silent", humanDuration(quiet)))
		}
		b.WriteString("\n" + line)
		for _, l := range tail(e.output, tailLines) {
			b.WriteString("\n" + pad + faintStyle.Render(truncate(l, w-6)))
		}
	default:
		status := okStyle.Render(gl.Check)
		switch {
		case e.failed && e.exitCode != 0:
			status = errStyle.Render(fmt.Sprintf("%s exit %d", gl.Cross, e.exitCode))
		case e.failed:
			status = errStyle.Render(gl.Cross)
		}
		var meta []string
		if e.duration >= time.Second {
			meta = append(meta, humanDuration(e.duration))
		}
		switch {
		case e.lines == 1:
			meta = append(meta, i18n.T("ui.line_one"))
		case e.lines > 1:
			meta = append(meta, i18n.T("ui.lines", e.lines))
		}
		line := gutter + status
		if len(meta) > 0 {
			line += mutedStyle.Render(" " + gl.Sep + " " + strings.Join(meta, " "+gl.Sep+" "))
		}
		if !m.expanded && e.lines > 0 && !e.failed {
			line += faintStyle.Render("  (ctrl+o)")
		}
		b.WriteString("\n" + line)
		var show []string
		switch {
		case m.expanded:
			show = e.output
		case e.failed:
			show = tail(e.output, tailLines)
		}
		for _, l := range show {
			b.WriteString("\n" + pad + mutedStyle.Render(truncate(l, w-6)))
		}
		if e.errText != "" {
			for _, l := range strings.Split(m.wrapPlain(e.errText), "\n") {
				b.WriteString("\n" + pad + errStyle.Render(l))
			}
		}
	}
	return b.String()
}

// wrapPlain folds a long error onto the width of the transcript so one
// provider message cannot run off the right edge.
func (m *model) wrapPlain(s string) string {
	w := m.tw() - 8
	if w < 16 {
		w = 16
	}
	return lipgloss.NewStyle().Width(w).Render(s)
}

// transcript is every entry painted, one under another. Entries keep their
// painted form (renderEntry); the joined text is kept too, so a refresh
// where nothing changed costs no copy.
func (m *model) transcript() string {
	parts := make([]string, 0, len(m.entries))
	for _, e := range m.entries {
		parts = append(parts, m.renderEntry(e))
	}
	same := len(parts) == len(m.tparts)
	for i := 0; same && i < len(parts); i++ {
		same = parts[i] == m.tparts[i]
	}
	if !same {
		m.tparts = parts
		m.tjoined = strings.Join(parts, "\n\n") + "\n"
	}
	return m.tjoined
}

// searched is the transcript with the query highlighted and the lines that
// match it, reused while neither the transcript nor the query changed.
func (m *model) searched(content string) (string, []int) {
	if m.sc.content != content || m.sc.query != m.searchQ || m.sc.painted == "" {
		m.sc = searchCache{content: content, query: m.searchQ, painted: highlightQuery(content, m.searchQ), lines: matchLines(content, m.searchQ)}
	}
	return m.sc.painted, m.sc.lines
}

// matches are the transcript lines that hold the query.
func (m *model) matches() []int {
	_, lines := m.searched(m.transcript())
	return lines
}

func (m *model) refresh() {
	if !m.ready {
		return
	}
	atBottom := m.vp.AtBottom()
	before := m.vp.TotalLineCount()
	content := m.transcript()
	var lines []int
	if m.searchOn && m.searchQ != "" {
		content, lines = m.searched(content)
	}
	m.vp.SetContent(content)
	m.resize()
	if m.searchOn {
		if m.searchAt >= 0 && m.searchAt < len(lines) {
			m.vp.SetYOffset(lines[m.searchAt])
		}
		m.follow = false
		return
	}
	if m.follow || atBottom || m.state == stIdle {
		m.vp.GotoBottom()
		m.unseen = 0
	} else if grown := m.vp.TotalLineCount() - before; grown > 0 && before > 0 {
		// The user is reading further up: count what arrived below.
		m.unseen += grown
	}
	m.follow = false
}
