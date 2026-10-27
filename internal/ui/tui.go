// Package ui renders an agent run, either as a full-screen Bubble Tea TUI or
// as plain line-oriented output for pipes and CI.
package ui

import (
	"context"
	"fmt"
	"math"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/safety"
)

var (
	accent    = lipgloss.AdaptiveColor{Light: "#5A4FCF", Dark: "#9D8CFF"}
	accentDim = lipgloss.AdaptiveColor{Light: "#A49DE6", Dark: "#5B5196"}
	muted     = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#8B93A1"}
	faint     = lipgloss.AdaptiveColor{Light: "#9CA3AF", Dark: "#5C6370"}
	okColor   = lipgloss.AdaptiveColor{Light: "#15803D", Dark: "#4ADE80"}
	errColor  = lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#F87171"}
	warnColor = lipgloss.AdaptiveColor{Light: "#B45309", Dark: "#FBBF24"}

	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(accent)
	mutedStyle   = lipgloss.NewStyle().Foreground(muted)
	faintStyle   = lipgloss.NewStyle().Foreground(faint)
	okStyle      = lipgloss.NewStyle().Foreground(okColor)
	errStyle     = lipgloss.NewStyle().Foreground(errColor)
	warnStyle    = lipgloss.NewStyle().Foreground(warnColor).Bold(true)
	cmdStyle     = lipgloss.NewStyle().Bold(true)
	cardStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accentDim).Padding(0, 1)
	confirmStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(warnColor).Padding(0, 1)
	inputStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(faint).Padding(0, 1)
	welcomeStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accentDim).Padding(0, 2)
)

const (
	tailLines = 5
	maxKept   = 400
	tick      = 100 * time.Millisecond
)

// Header describes the session shown in the welcome box and status bar.
type Header struct {
	Version, Provider, Model, Escalate, Task string
	ReadOnly, AutoYes                        bool
}

type entryKind int

const (
	enWelcome entryKind = iota
	enUser
	enAssistant
	enTool
	enNote
)

type entry struct {
	kind     entryKind
	text     string
	style    lipgloss.Style
	tool     string
	command  string
	running  bool
	failed   bool
	denied   string
	exitCode int
	duration time.Duration
	started  time.Time
	output   []string
	lines    int
}

type state int

const (
	stRunning state = iota
	stConfirm
	stChoice
	stPassword
	stIdle
	stDone
)

type tickMsg time.Time
type closedMsg struct{}

type model struct {
	ag     *agent.Agent
	cancel context.CancelFunc
	header Header

	vp       viewport.Model
	input    textinput.Model
	width    int
	height   int
	ready    bool
	entries  []*entry
	current  *entry
	state    state
	expanded bool

	thinking   bool
	thinkStart time.Time
	frame      int
	verbs      []string
	verb       string
	stats      agent.Stats

	pending  *agent.Event
	picker   *Picker
	password textinput.Model

	md      *glamour.TermRenderer
	mdWidth int
	dark    bool
	started time.Time
}

// RunTUI runs the agent inside a full-screen terminal UI.
func RunTUI(ctx context.Context, cancel context.CancelFunc, ag *agent.Agent, h Header) error {
	ti := textinput.New()
	ti.Placeholder = i18n.T("ui.placeholder")
	ti.Prompt = "› "
	ti.CharLimit = 2000

	m := &model{
		ag: ag, cancel: cancel, header: h, input: ti,
		verbs:   strings.Split(i18n.T("ui.verbs"), "|"),
		dark:    lipgloss.HasDarkBackground(),
		started: time.Now(),
	}
	m.verb = m.verbs[0]
	m.password = textinput.New()
	m.password.EchoMode = textinput.EchoPassword
	m.password.EchoCharacter = '•'
	m.password.Prompt = "🔒 "
	m.password.CharLimit = 256
	m.stats.Model = h.Model
	m.entries = append(m.entries, &entry{kind: enWelcome})
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx), tea.WithMouseCellMotion())
	_, err := p.Run()
	if err == tea.ErrProgramKilled {
		err = nil
	}
	return err
}

func (m *model) waitEvent() tea.Cmd {
	ch := m.ag.Events()
	return func() tea.Msg {
		e, ok := <-ch
		if !ok {
			return closedMsg{}
		}
		return e
	}
}

func doTick() tea.Cmd { return tea.Tick(tick, func(t time.Time) tea.Msg { return tickMsg(t) }) }

func (m *model) Init() tea.Cmd { return tea.Batch(m.waitEvent(), doTick()) }

// ---------- rendering helpers ----------

func (m *model) markdown(s string) string {
	w := m.width - 4
	if w < 30 {
		w = 30
	}
	if m.md == nil || m.mdWidth != w {
		style := "dark"
		if !m.dark {
			style = "light"
		}
		r, err := glamour.NewTermRenderer(glamour.WithStandardStyle(style), glamour.WithWordWrap(w), glamour.WithEmoji())
		if err != nil {
			return s
		}
		m.md, m.mdWidth = r, w
	}
	out, err := m.md.Render(s)
	if err != nil {
		return s
	}
	return strings.Trim(out, "\n")
}

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
	}
	return name, homeTilde(arg)
}

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

func (m *model) costText() string {
	if !m.stats.CostKnown {
		return ""
	}
	if m.stats.Cost < 0.01 {
		return fmt.Sprintf("$%.4f", m.stats.Cost)
	}
	return fmt.Sprintf("$%.2f", m.stats.Cost)
}

// shimmer paints a soft highlight that sweeps across the text.
func (m *model) shimmer(s string) string {
	r := []rune(s)
	pos := m.frame % (len(r) + 8)
	bright := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#1E1B4B", Dark: "#FFFFFF"})
	mid := lipgloss.NewStyle().Foreground(accent)
	dim := lipgloss.NewStyle().Foreground(accentDim)
	var b strings.Builder
	for i, c := range r {
		d := math.Abs(float64(i - pos + 4))
		switch {
		case d < 1.5:
			b.WriteString(bright.Render(string(c)))
		case d < 3.5:
			b.WriteString(mid.Render(string(c)))
		default:
			b.WriteString(dim.Render(string(c)))
		}
	}
	return b.String()
}

// leadingSpace matches the margin glamour puts before a line, past any
// colour codes it starts with.
var leadingSpace = regexp.MustCompile(`^((?:\x1b\[[0-9;]*m)*) +`)

var spinFrames = []string{"·", "✢", "✳", "✶", "✻", "✽", "✻", "✶", "✳", "✢"}

func (m *model) modeText() string {
	switch {
	case m.header.ReadOnly:
		return i18n.T("ui.mode_readonly")
	case m.header.AutoYes:
		return i18n.T("ui.mode_auto")
	}
	return i18n.T("ui.mode_ask")
}

func (m *model) renderWelcome() string {
	w := min(m.width-2, 78)
	var b strings.Builder
	b.WriteString(titleStyle.Render("✻ omega") + mutedStyle.Render(" "+m.header.Version) + "\n")
	b.WriteString(mutedStyle.Render(i18n.T("ui.welcome_tagline")) + "\n\n")
	row := func(k, v string) { b.WriteString(faintStyle.Render(fmt.Sprintf("%-10s", k)) + v + "\n") }
	row(i18n.T("ui.label_provider"), m.header.Provider)
	mdl := cmdStyle.Render(m.header.Model)
	if m.header.Escalate != "" && m.header.Escalate != m.header.Model {
		mdl += mutedStyle.Render("  " + i18n.T("ui.escalates", m.header.Escalate))
	}
	row(i18n.T("ui.label_model"), mdl)
	row(i18n.T("ui.label_task"), m.header.Task+mutedStyle.Render(" · "+m.modeText()))
	if p := m.ag.LogPath(); p != "" {
		row(i18n.T("ui.label_log"), mutedStyle.Render(truncate(homeTilde(p), w-16)))
	}
	return welcomeStyle.Width(w).Render(strings.TrimRight(b.String(), "\n"))
}

func (m *model) renderEntry(e *entry) string {
	w := m.width - 2
	switch e.kind {
	case enWelcome:
		return m.renderWelcome()
	case enUser:
		return titleStyle.Render("› ") + e.text
	case enAssistant:
		lines := strings.Split(m.markdown(e.text), "\n")
		// glamour indents by two columns; put the bullet in that gutter.
		if len(lines) > 0 {
			lines[0] = titleStyle.Render("⏺") + " " + leadingSpace.ReplaceAllString(lines[0], "$1")
		}
		return strings.Join(lines, "\n")
	case enNote:
		return e.style.Render(e.text)
	}

	// Tool call.
	name, arg := toolLabel(e.tool, e.command)
	dot := okStyle.Render("⏺")
	switch {
	case e.running:
		if (m.frame/4)%2 == 0 {
			dot = titleStyle.Render("⏺")
		} else {
			dot = lipgloss.NewStyle().Foreground(accentDim).Render("⏺")
		}
	case e.denied != "":
		dot = warnStyle.Render("⏺")
	case e.failed:
		dot = errStyle.Render("⏺")
	}
	head := dot + " " + cmdStyle.Render(name)
	if arg != "" {
		head += mutedStyle.Render("(") + truncate(arg, max(10, w-len(name)-6)) + mutedStyle.Render(")")
	}
	if strings.HasPrefix(e.tool, "run_step:") && e.command != "" {
		head += faintStyle.Render("  " + truncate(e.command, max(10, w-lipgloss.Width(head)-4)))
	}

	var b strings.Builder
	b.WriteString(head)
	gutter := faintStyle.Render("  ⎿  ")
	pad := "     "
	switch {
	case e.denied != "":
		b.WriteString("\n" + gutter + warnStyle.Render(i18n.T("ui.denied")) + mutedStyle.Render(" · "+truncate(firstSentence(e.denied), max(10, w-20))))
	case e.running:
		b.WriteString("\n" + gutter + mutedStyle.Render(i18n.T("ui.running", humanDuration(time.Since(e.started)))))
		for _, l := range tail(e.output, tailLines) {
			b.WriteString("\n" + pad + faintStyle.Render(truncate(l, w-6)))
		}
	default:
		status := okStyle.Render("✓")
		switch {
		case e.failed && e.exitCode != 0:
			status = errStyle.Render(fmt.Sprintf("✗ exit %d", e.exitCode))
		case e.failed:
			status = errStyle.Render("✗")
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
			line += mutedStyle.Render(" · " + strings.Join(meta, " · "))
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
	}
	return b.String()
}

func firstSentence(s string) string {
	if i := strings.Index(s, ". "); i > 0 {
		return s[:i+1]
	}
	return s
}

func (m *model) refresh() {
	if !m.ready {
		return
	}
	atBottom := m.vp.AtBottom()
	parts := make([]string, 0, len(m.entries))
	for _, e := range m.entries {
		parts = append(parts, m.renderEntry(e))
	}
	m.vp.SetContent(strings.Join(parts, "\n\n") + "\n")
	m.resize()
	if atBottom || m.state == stIdle {
		m.vp.GotoBottom()
	}
}

func (m *model) add(e *entry) { m.entries = append(m.entries, e) }

func (m *model) note(text string, style lipgloss.Style) {
	m.add(&entry{kind: enNote, text: text, style: style})
}

// ---------- events ----------

func (m *model) handleEvent(e agent.Event) tea.Cmd {
	if e.Stats.Model != "" {
		m.stats = e.Stats
	}
	switch e.Kind {
	case agent.EvThinking:
		m.thinking = true
		m.thinkStart = time.Now()
		m.verb = m.verbs[int(time.Now().UnixNano()/7)%len(m.verbs)]
	case agent.EvAssistant:
		m.thinking = false
		m.add(&entry{kind: enAssistant, text: e.Text})
	case agent.EvToolStart:
		m.thinking = false
		m.current = &entry{kind: enTool, tool: e.Tool, command: e.Command, running: true, started: time.Now()}
		m.add(m.current)
	case agent.EvOutput:
		if m.current != nil {
			m.current.output = append(m.current.output, e.Text)
			if len(m.current.output) > maxKept {
				m.current.output = m.current.output[len(m.current.output)-maxKept:]
			}
		}
	case agent.EvToolEnd:
		cur := m.current
		if cur == nil || cur.tool != e.Tool {
			cur = &entry{kind: enTool, tool: e.Tool, command: e.Command}
			m.add(cur)
		}
		cur.running, cur.failed, cur.exitCode, cur.duration, cur.lines = false, e.Failed, e.ExitCode, e.Duration, e.Lines
		m.current = nil
	case agent.EvDenied:
		m.add(&entry{kind: enTool, tool: e.Tool, command: e.Command, denied: e.Reason})
	case agent.EvConfirm:
		m.thinking = false
		ev := e
		m.pending = &ev
		m.state = stConfirm
	case agent.EvChoice:
		m.thinking = false
		opts := make([]Option, len(e.Choices))
		for i, c := range e.Choices {
			opts[i] = Option{ID: c.ID, Label: c.Label, Detail: c.Detail, Badge: c.Impact, Risk: c.Risk}
		}
		m.picker = NewPicker(e.Text, opts, e.Multi)
		ev := e
		m.pending = &ev
		m.state = stChoice
	case agent.EvPassword:
		m.thinking = false
		ev := e
		m.pending = &ev
		m.state = stPassword
		m.password.Reset()
		return m.password.Focus()
	case agent.EvModel:
		m.header.Model = e.Text
		m.note(titleStyle.Render("↑ ")+mutedStyle.Render(i18n.T("ui.model_switch", e.Text)), lipgloss.NewStyle())
	case agent.EvMode:
		m.header.ReadOnly = false
		m.note(okStyle.Render("⏵ ")+mutedStyle.Render(i18n.T("ui.mode_lifted")), lipgloss.NewStyle())
	case agent.EvIdle:
		m.thinking = false
		m.state = stIdle
		m.input.Reset()
		return m.input.Focus()
	case agent.EvError:
		m.thinking = false
		m.note("! "+e.Text, errStyle)
	case agent.EvDone:
		m.thinking = false
		m.state = stDone
		if e.Text != "" {
			m.note(e.Text, mutedStyle)
		}
		m.note(i18n.T("ui.summary_done", humanDuration(m.stats.Elapsed),
			humanTokens(m.stats.Usage.Input()+m.stats.Usage.OutputTokens), m.costText()), mutedStyle)
	}
	return nil
}

// ---------- update ----------

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.input.Width = msg.Width - 8
		if !m.ready {
			m.vp = viewport.New(msg.Width, 1)
			m.vp.MouseWheelEnabled = true
			m.ready = true
		}
		m.vp.Width = msg.Width
		m.refresh()
		m.vp.GotoBottom()

	case tickMsg:
		m.frame++
		if m.thinking || m.current != nil {
			m.refresh()
		}
		cmds = append(cmds, doTick())

	case agent.Event:
		if cmd := m.handleEvent(msg); cmd != nil {
			cmds = append(cmds, cmd)
		}
		m.refresh()
		cmds = append(cmds, m.waitEvent())

	case closedMsg:
		m.state = stDone

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.cancel()
			return m, tea.Quit
		case "ctrl+o":
			m.expanded = !m.expanded
			m.refresh()
			return m, nil
		}
		switch m.state {
		case stConfirm:
			var d agent.Decision = -1
			switch strings.ToLower(msg.String()) {
			case "y", "s", "o", "j", "enter":
				d = agent.Approve
			case "a":
				if !m.pending.Always {
					d = agent.ApproveAll
				}
			case "n", "esc":
				d = agent.Deny
			}
			if d >= 0 {
				m.pending.Reply <- d
				m.pending = nil
				m.state = stRunning
				m.refresh()
			}
			return m, nil
		case stPassword:
			switch msg.Type {
			case tea.KeyEnter, tea.KeyEsc:
				pw := m.password.Value()
				if msg.Type == tea.KeyEsc {
					pw = ""
				}
				m.password.Reset()
				m.password.Blur()
				m.pending.Secret <- pw
				m.pending = nil
				m.state = stRunning
				m.refresh()
				return m, nil
			}
			var cmd tea.Cmd
			m.password, cmd = m.password.Update(msg)
			return m, cmd
		case stChoice:
			if done, cancelled := m.picker.HandleKey(msg); done {
				var ids []string
				if !cancelled {
					ids = m.picker.Selected()
				}
				var labels []string
				for _, id := range ids {
					for _, o := range m.picker.Options {
						if o.ID == id {
							labels = append(labels, o.Label)
						}
					}
				}
				if len(labels) == 0 {
					m.note(mutedStyle.Render("› "+i18n.T("ui.chosen_none")), lipgloss.NewStyle())
				} else {
					m.note(titleStyle.Render("› ")+i18n.T("ui.chosen", strings.Join(labels, ", ")), lipgloss.NewStyle())
				}
				m.pending.Picks <- ids
				m.pending, m.picker = nil, nil
				m.state = stRunning
			}
			m.refresh()
			return m, nil
		case stIdle:
			switch msg.Type {
			case tea.KeyEnter:
				text := strings.TrimSpace(m.input.Value())
				m.input.Blur()
				m.state = stRunning
				if text != "" {
					m.add(&entry{kind: enUser, text: text})
				}
				ch := m.ag.Input()
				go func() { ch <- text }()
				m.refresh()
				return m, nil
			case tea.KeyEsc:
				m.input.Blur()
				m.state = stRunning
				ch := m.ag.Input()
				go func() { ch <- "" }()
				return m, nil
			case tea.KeyPgUp, tea.KeyPgDown, tea.KeyUp, tea.KeyDown:
				var cmd tea.Cmd
				m.vp, cmd = m.vp.Update(msg)
				return m, cmd
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		case stDone:
			if msg.String() == "q" || msg.Type == tea.KeyEnter || msg.Type == tea.KeyEsc {
				return m, tea.Quit
			}
		}
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		cmds = append(cmds, cmd)

	case tea.MouseMsg:
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

// ---------- footer ----------

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

func (m *model) confirmView() string {
	e := m.pending
	w := m.width - 2
	name, arg := toolLabel(e.Tool, e.Command)
	var b strings.Builder
	b.WriteString(warnStyle.Render(i18n.T("ui.confirm")) + "  " + mutedStyle.Render(i18n.T("ui.risk", riskLabel(e.Risk))) + "\n\n")
	b.WriteString(cmdStyle.Render(name) + mutedStyle.Render("(") + truncate(arg, max(10, w-12)) + mutedStyle.Render(")") + "\n")
	if strings.HasPrefix(e.Tool, "run_step:") {
		b.WriteString(faintStyle.Render(truncate(e.Command, max(10, w-6))) + "\n")
	}
	if e.Reason != "" {
		lines := strings.Split(e.Reason, "\n")
		if len(lines) > 14 {
			lines = append(lines[:14], "…")
		}
		b.WriteString("\n")
		for _, l := range lines {
			st := mutedStyle
			switch {
			case strings.HasPrefix(l, "+ "):
				st = okStyle
			case strings.HasPrefix(l, "- "):
				st = errStyle
			}
			b.WriteString(st.Render(truncate(l, max(10, w-6))) + "\n")
		}
	}
	if e.Always {
		b.WriteString("\n" + warnStyle.Render("⚠ ") + mutedStyle.Render(i18n.T("ui.always_note")) + "\n")
	}
	b.WriteString("\n")
	if e.Always {
		b.WriteString(i18n.T("ui.confirm_keys_once"))
	} else {
		b.WriteString(i18n.T("ui.confirm_keys"))
	}
	return confirmStyle.Width(w).Render(b.String())
}

func (m *model) statusBar() string {
	var left string
	switch {
	case m.header.ReadOnly:
		left = okStyle.Render("⏵⏵ " + i18n.T("ui.mode_readonly"))
	case m.header.AutoYes:
		left = warnStyle.Render("⏵⏵ " + i18n.T("ui.mode_auto"))
	default:
		left = mutedStyle.Render("⏵⏵ " + i18n.T("ui.mode_ask"))
	}
	parts := []string{m.header.Model,
		"↑" + humanTokens(m.stats.Usage.Input()) + " ↓" + humanTokens(m.stats.Usage.OutputTokens)}
	if c := m.costText(); c != "" {
		parts = append(parts, c)
	}
	elapsed := time.Since(m.started)
	if m.state == stDone {
		elapsed = m.stats.Elapsed
	}
	parts = append(parts, humanDuration(elapsed))
	right := mutedStyle.Render(strings.Join(parts, " · "))
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		return " " + left
	}
	return " " + left + strings.Repeat(" ", gap) + right
}

func (m *model) footerView() string {
	var top string
	switch m.state {
	case stConfirm:
		top = m.confirmView()
	case stChoice:
		top = m.picker.View(m.width)
	case stPassword:
		var b strings.Builder
		b.WriteString(warnStyle.Render(i18n.T("ui.password_title")) + "\n")
		if m.pending.Text != "" {
			b.WriteString(faintStyle.Render(truncate(m.pending.Text, m.width-8)) + "\n")
		}
		if m.pending.Failed {
			b.WriteString(errStyle.Render(i18n.T("ui.password_wrong")) + "\n")
		}
		b.WriteString("\n" + m.password.View() + "\n\n")
		b.WriteString(mutedStyle.Render(i18n.T("ui.password_hint")) + "\n")
		b.WriteString(faintStyle.Render(i18n.T("ui.password_keys")))
		top = confirmStyle.Width(m.width - 2).Render(b.String())
	case stIdle:
		top = inputStyle.Width(m.width - 2).Render(m.input.View())
		return top + "\n" + m.statusBar() + "\n" + faintStyle.Render("  "+i18n.T("ui.hint_idle"))
	case stDone:
		return m.statusBar() + "\n" + faintStyle.Render("  "+i18n.T("ui.hint_done"))
	default:
		if m.thinking {
			el := humanDuration(time.Since(m.thinkStart))
			glyph := titleStyle.Render(spinFrames[m.frame%len(spinFrames)])
			top = glyph + " " + m.shimmer(m.verb+"…") + " " +
				faintStyle.Render(i18n.T("ui.thinking_meta", el, humanTokens(m.stats.Usage.Input()+m.stats.Usage.OutputTokens)))
		}
	}
	return top + "\n" + m.statusBar() + "\n" + faintStyle.Render("  "+i18n.T("ui.hint_running"))
}

func (m *model) resize() {
	if !m.ready {
		return
	}
	h := m.height - lipgloss.Height(m.footerView()) - 1
	if h < 3 {
		h = 3
	}
	m.vp.Height = h
}

func (m *model) View() string {
	if !m.ready {
		return i18n.T("ui.starting")
	}
	m.resize()
	return m.vp.View() + "\n" + m.footerView()
}

func tail(lines []string, n int) []string {
	if len(lines) <= n {
		return lines
	}
	return lines[len(lines)-n:]
}

func truncate(s string, n int) string {
	if n < 10 {
		n = 10
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
