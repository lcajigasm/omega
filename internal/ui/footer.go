package ui

import (
	"strings"
	"time"

	kb "github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/safety"
)

func riskLabel(r safety.Risk) string {
	switch r {
	case safety.Dangerous:
		return warnStyle.Render(RiskName(r))
	case safety.Blocked:
		return errStyle.Render(RiskName(r))
	}
	return RiskName(r)
}

// RiskName is the localized name of a risk level.
func RiskName(r safety.Risk) string {
	return i18n.T([...]string{"risk.safe", "risk.normal", "risk.dangerous", "risk.blocked"}[r])
}

// confirmButton is a clickable span on the approval card's key line.
// start and end are columns in that line's visible text.
type confirmButton struct {
	start, end int
	decision   agent.Decision
}

// confirmChoices are the answers of the approval card, in the user's
// language: y (or the yes letter of the language), a, n.
func (m *model) confirmChoices() ([]hint, []agent.Decision) {
	hs := []hint{{key: yesKey(), action: i18n.T("uix.yes"), keep: true}}
	ds := []agent.Decision{agent.Approve}
	if !m.pending.Always {
		hs = append(hs, hint{key: "a", action: i18n.T("uix.yes_all"), keep: true})
		ds = append(ds, agent.ApproveAll)
	}
	hs = append(hs, hint{key: "n", action: i18n.T("uix.no"), keep: true})
	ds = append(ds, agent.Deny)
	return hs, ds
}

// confirmDecision is the answer a key gives on the approval card, or -1.
// Enter approves a normal command, never a dangerous one: that takes the
// yes letter.
func (m *model) confirmDecision(msg tea.KeyMsg) agent.Decision {
	switch {
	case isYes(msg):
		return agent.Approve
	case msg.Type == tea.KeyEnter:
		if m.pending.Risk < safety.Dangerous {
			return agent.Approve
		}
	case kb.Matches(msg, km.ApproveAll):
		if !m.pending.Always {
			return agent.ApproveAll
		}
	case kb.Matches(msg, km.Deny):
		return agent.Deny
	}
	return -1
}

// confirmCard is the frame of the approval card: its colour says the risk.
func confirmCard(r safety.Risk) lipgloss.Style {
	switch r {
	case safety.Dangerous:
		return confirmWarnStyle
	case safety.Blocked:
		return confirmErrStyle
	}
	return confirmStyle
}

func (m *model) confirmLayout() (body string, keyLine int, buttons []confirmButton) {
	e := m.pending
	w := m.width - 2
	var b strings.Builder
	line := 0
	write := func(s string) {
		if line > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(s)
		line++
	}
	title := titleStyle.Render(i18n.T("ui.confirm"))
	switch e.Risk {
	case safety.Dangerous:
		title = warnStyle.Render(gl.Warn + " " + i18n.T("ui.confirm"))
	case safety.Blocked:
		title = errStyle.Render(gl.Cross + " " + i18n.T("uix.blocked"))
	}
	write(title + "  " + mutedStyle.Render(i18n.T("ui.risk", riskLabel(e.Risk))))
	write("")
	name, arg := toolLabel(e.Tool, e.Command)
	write(cmdStyle.Render(name) + mutedStyle.Render("(") + truncate(arg, max(10, w-12)) + mutedStyle.Render(")"))
	if strings.HasPrefix(e.Tool, "run_step:") {
		write(faintStyle.Render(truncate(e.Command, max(10, w-6))))
	}
	if e.Reason != "" {
		lines := strings.Split(e.Reason, "\n")
		if len(lines) > 14 {
			lines = append(lines[:14], gl.Ellipsis)
		}
		write("")
		for _, l := range lines {
			st := mutedStyle
			switch {
			case strings.HasPrefix(l, "+ "):
				st = okStyle
			case strings.HasPrefix(l, "- "):
				st = errStyle
			}
			write(st.Render(truncate(l, max(10, w-6))))
		}
	}
	if e.Always {
		write("")
		write(warnStyle.Render(gl.Warn+" ") + mutedStyle.Render(i18n.T("ui.always_note")))
	}
	write("")
	hs, ds := m.confirmChoices()
	// Columns, not bytes: "sí" and "à" take two bytes and one column. Each
	// button runs up to the next one, so its label is part of it.
	col := 0
	for i, h := range hs {
		if i > 0 {
			col += lipgloss.Width(hintSep())
		}
		buttons = append(buttons, confirmButton{start: col, decision: ds[i]})
		col += lipgloss.Width(h.plain())
	}
	for i := range buttons {
		buttons[i].end = col
		if i+1 < len(buttons) {
			buttons[i].end = buttons[i+1].start
		}
	}
	keyLine = line
	write(renderLines([][]hint{hs}, ""))
	if e.Risk >= safety.Dangerous {
		write(faintStyle.Render(i18n.T("uix.enter_not")))
	}
	return b.String(), keyLine, buttons
}

func (m *model) confirmView() string {
	body, _, _ := m.confirmLayout()
	return confirmCard(m.pending.Risk).Width(m.width - 2).Render(body)
}

// statusBar is the line under the transcript: the approval mode on the
// left; model, tokens, cost and time on the right. On a narrow terminal
// the parts go one by one (the mode's key, the model, the tokens) and the
// cost and the time stay.
func (m *model) statusBar() string {
	var left string
	switch m.mode {
	case agent.ModeReadOnly:
		left = okStyle.Render(gl.Mode + " " + i18n.T("ui.mode_readonly"))
	case agent.ModeAuto:
		left = warnStyle.Render(gl.Mode + " " + i18n.T("ui.mode_auto"))
	default:
		left = mutedStyle.Render(gl.Mode + " " + i18n.T("ui.mode_ask"))
	}
	modeKey := ""
	if m.state != stDone && (m.g == nil || m.ag != nil) {
		modeKey = faintStyle.Render(" " + i18n.T("ui.mode_hint"))
	}
	mdl := m.header.Model
	tokens := gl.Up + humanTokens(m.stats.Usage.Input()) + " " + gl.Down + humanTokens(m.stats.Usage.OutputTokens)
	elapsed := time.Since(m.started)
	if m.state == stDone {
		elapsed = m.stats.Elapsed
	}
	keep := []string{}
	if c := m.costText(); c != "" {
		keep = append(keep, c)
	}
	keep = append(keep, humanDuration(elapsed))
	right := func(withModel, withTokens bool) string {
		var parts []string
		if withModel && mdl != "" {
			parts = append(parts, mdl)
		}
		if withTokens {
			parts = append(parts, tokens)
		}
		return mutedStyle.Render(strings.Join(append(parts, keep...), " "+gl.Sep+" "))
	}
	for _, try := range []struct {
		key           bool
		model, tokens bool
	}{{true, true, true}, {false, true, true}, {false, false, true}, {false, false, false}} {
		l := left
		if try.key {
			l += modeKey
		}
		r := right(try.model, try.tokens)
		if gap := m.width - lipgloss.Width(l) - lipgloss.Width(r) - 2; gap >= 1 {
			return " " + l + strings.Repeat(" ", gap) + r
		}
	}
	r := right(false, false)
	return " " + truncateANSI(left, max(4, m.width-lipgloss.Width(r)-3)) + " " + r
}

// newLinesLine says how much arrived below while the user reads further
// up, and how to get there; "" when nothing did.
func (m *model) newLinesLine() string {
	if m.unseen <= 0 || !m.ready || m.vp.AtBottom() {
		m.unseen = 0
		return ""
	}
	return " " + titleStyle.Render(gl.Down+" "+i18n.N("uix.new_lines", m.unseen)) + faintStyle.Render(hintSep()) +
		hint{key: "end", action: i18n.T("uix.to_end")}.render()
}

// statusBlock is the status bar, with the new-lines notice above it.
func (m *model) statusBlock() string {
	if s := m.newLinesLine(); s != "" {
		return s + "\n" + m.statusBar()
	}
	return m.statusBar()
}

// helpAllowed is false while a password or a filter is being typed, and
// while the idle input already has text, so "?" stays a character.
func (m *model) helpAllowed() bool {
	switch m.state {
	case stPassword, stChoice:
		return false
	case stIdle:
		return strings.TrimSpace(m.input.Value()) == ""
	default:
		return true
	}
}

// sessionHints are the keys of the session in its current state.
func (m *model) sessionHints() []hint {
	help := hk("?", "uix.help").must()
	if m.interrupted {
		return []hint{hk("ctrl+c", "uix.leave_again").must(), help, hk("↑↓", "uix.scroll")}
	}
	switch m.state {
	case stIdle:
		if m.g != nil {
			return []hint{hk("enter", "uix.send"), hk("esc", "uix.end_stage").must(), hk("/", "uix.commands"), help, hk("ctrl+c", "uix.stop_stage")}
		}
		return []hint{hk("enter", "uix.send"), hk("/", "uix.commands"), help, hk("shift+tab", "uix.mode"), hk("esc", "uix.esc_idle").must()}
	case stDone:
		return []hint{help, hk("q", "uix.quit").must(), hk("↑↓", "uix.scroll"), hk("ctrl+f", "uix.search")}
	}
	return []hint{hk("ctrl+c", "uix.cancel").must(), help, hk("ctrl+f", "uix.search"), hk("shift+tab", "uix.mode"),
		hk("ctrl+p", "uix.model"), hk("↑↓", "uix.scroll")}
}

func (m *model) footerView() string {
	if m.g != nil && !m.help {
		if v, ok := m.guideFooter(); ok {
			return m.withSearchBar(v)
		}
	}
	if m.help {
		return m.withSearchBar(m.helpView() + "\n" + m.statusBlock() + "\n" +
			hintLine(m.width, hk("?/esc", "uix.close_help").must(), hk("↑↓", "uix.scroll")))
	}
	keyLine := hintLine(m.width, m.sessionHints()...)
	var top string
	switch m.state {
	case stConfirm:
		top = m.confirmView()
	case stChoice:
		m.picker.setHeight(m.height)
		top = m.picker.View(m.width)
	case stPassword:
		top = m.passwordView()
	case stIdle:
		top = inputStyle.Width(m.width - 2).Render(m.input.View())
		if menu := m.menuView(); menu != "" {
			return m.withSearchBar(top + "\n" + menu + "\n" + m.statusBlock())
		}
		return m.withSearchBar(top + "\n" + m.statusBlock() + "\n" + keyLine)
	case stDone:
		return m.withSearchBar(m.statusBlock() + "\n" + keyLine)
	default:
		if m.thinking {
			el := humanDuration(time.Since(m.thinkStart))
			glyph := titleStyle.Render(spinFrame(m.frame))
			top = glyph + " " + m.shimmer(m.verb+gl.Ellipsis) + " " +
				faintStyle.Render(i18n.T("ui.thinking_meta", el, humanTokens(m.stats.Usage.Input()+m.stats.Usage.OutputTokens)))
			top = truncateANSI(top, m.width)
		}
	}
	return m.withSearchBar(top + "\n" + m.statusBlock() + "\n" + keyLine)
}

func (m *model) passwordView() string {
	w := m.width - 6
	var b strings.Builder
	b.WriteString(warnStyle.Render(i18n.T("ui.password_title")) + "\n")
	if m.pending != nil && m.pending.Text != "" {
		b.WriteString(faintStyle.Render(truncate(m.pending.Text, m.width-8)) + "\n")
	}
	if m.pending != nil && m.pending.Failed {
		b.WriteString(errStyle.Render(i18n.T("ui.password_wrong")) + "\n")
	}
	b.WriteString("\n" + m.password.View() + "\n\n")
	b.WriteString(mutedStyle.Render(i18n.T("ui.password_hint")) + "\n\n")
	b.WriteString(cardHints(w, hk("enter", "uix.confirm").must(), hk("esc", "uix.skip_admin").must()))
	return confirmWarnStyle.Width(m.width - 2).Render(b.String())
}

func (m *model) withSearchBar(base string) string {
	if !m.searchOn {
		return base
	}
	count := i18n.T("ui.search_none")
	if m.searchQ != "" {
		n := len(m.matches())
		if n > 0 && m.searchAt >= 0 && m.searchAt < n {
			count = i18n.T("ui.search_count", m.searchAt+1, n)
		}
	}
	bar := inputStyle.Width(m.width - 2).Render(i18n.T("ui.search_bar", m.searchQ, count))
	return base + "\n" + bar + "\n" + hintLine(m.width, hk("enter", "uix.next"), hk("↑", "uix.previous"), hk("esc", "uix.close").must())
}

func (m *model) resize() {
	if !m.ready {
		return
	}
	h := m.height - lipgloss.Height(m.footerView()) - 1 - m.topRows()
	if h < 3 {
		h = 3
	}
	m.vp.Height = h
}

// startView is the screen before the terminal has said its size.
func (m *model) startView() string {
	s := titleStyle.Render(gl.Brand) + "   " + titleStyle.Render(spinFrame(m.frame)) + " " + mutedStyle.Render(i18n.T("ui.starting"))
	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, s)
	}
	return "\n   " + s
}

func (m *model) View() string {
	if !m.ready {
		return m.startView()
	}
	if m.g != nil && m.state == stPlan {
		return m.planView()
	}
	m.resize()
	if m.panelShown() {
		return lipgloss.JoinHorizontal(lipgloss.Top, m.vp.View(), m.panelView(m.vp.Height)) + "\n" + m.footerView()
	}
	if m.topRows() > 0 {
		return m.routeStrip() + "\n" + m.vp.View() + "\n" + m.footerView()
	}
	return m.vp.View() + "\n" + m.footerView()
}
