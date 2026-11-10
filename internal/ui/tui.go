// Package ui renders an agent run, either as a full-screen Bubble Tea TUI or
// as plain line-oriented output for pipes and CI.
package ui

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
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
	Models                                   []string // models the user can switch to
}

type entryKind int

const (
	enWelcome entryKind = iota
	enUser
	enAssistant
	enTool
	enNote
	enStage // heading of a stage of a guided run
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
	errText  string // why a tool failed, shown on the card
	stage    int    // enStage: index of the stage,
	ord, of  int    // and its position among the stages of the run

	cached   string
	cacheKey string
	cacheOK  bool
}

type state int

const (
	stRunning state = iota
	stConfirm
	stChoice
	stPassword
	stIdle
	stDone
	stPlan    // guided run: choosing the stages
	stBetween // guided run: between two stages
	stSummary // guided run: what happened
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
	mode     agent.Mode
	menuSel  int  // highlighted row of the command menu
	help     bool // on-screen shortcut card
	quitting bool // /exit: leave as soon as the session ends
	focus    int  // index of the focused tool entry; -1 when there is none

	thinking   bool
	thinkStart time.Time
	frame      int
	verbs      []string
	verb       string
	stats      agent.Stats

	pending  *agent.Event
	picker   *Picker
	onPick   func(string) // set when a card answers the interface, not the agent
	password textinput.Model

	searchOn bool
	searchQ  string
	searchAt int // index of the current transcript match; -1 when there is none

	md      *glamour.TermRenderer
	mdWidth int
	dark    bool
	started time.Time

	panel     bool   // the session panel is wanted (it also needs a wide terminal)
	diskStart uint64 // free bytes when the session began
	diskNow   uint64
	diskTotal uint64

	g *guide // the guided run this session is a stage of; nil on its own

	follow    bool // next refresh jumps to the bottom (an error just arrived)
	mdRenders int  // times glamour parsed a message; tests read it
}

// RunTUI runs the agent inside a full-screen terminal UI.
func RunTUI(ctx context.Context, cancel context.CancelFunc, ag *agent.Agent, h Header) error {
	// A guided run already has the screen: the session is one of its stages.
	if g := host.Load(); g != nil {
		return g.session(cancel, ag, h)
	}
	m := newModel(ag, cancel, h)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx), tea.WithMouseCellMotion())
	_, err := p.Run()
	// Cancelling (ctrl+c, a signal) stops the program through its context.
	// Bubble Tea reports that as "program was killed: context canceled",
	// wrapping the sentinel: it is the user leaving, not a failure.
	if errors.Is(err, tea.ErrProgramKilled) || errors.Is(err, context.Canceled) {
		err = nil
	}
	return err
}

// newModel builds the session interface. ag is nil for a guided run, which
// gets its agents one stage at a time.
func newModel(ag *agent.Agent, cancel context.CancelFunc, h Header) *model {
	ti := textinput.New()
	ti.Placeholder = i18n.T("ui.placeholder")
	ti.Prompt = "› "
	ti.CharLimit = 2000

	m := &model{
		ag: ag, cancel: cancel, header: h, input: ti,
		verbs:   strings.Split(i18n.T("ui.verbs"), "|"),
		dark:    lipgloss.HasDarkBackground(),
		started: time.Now(),
		panel:   true,
	}
	if ag != nil {
		m.mode = ag.Mode()
	}
	m.diskNow, m.diskTotal = diskSpace("/")
	m.diskStart = m.diskNow
	m.verb = m.verbs[0]
	m.password = textinput.New()
	m.password.EchoMode = textinput.EchoPassword
	m.password.EchoCharacter = '•'
	m.password.Prompt = "🔒 "
	m.password.CharLimit = 256
	m.stats.Model = h.Model
	m.focus = -1
	m.entries = append(m.entries, &entry{kind: enWelcome})
	return m
}

func (m *model) waitEvent() tea.Cmd {
	ag := m.ag
	if ag == nil {
		return nil
	}
	ch := ag.Events()
	return func() tea.Msg {
		e, ok := <-ch
		return agEvent{ag: ag, e: e, closed: !ok}
	}
}

func doTick() tea.Cmd { return tea.Tick(tick, func(t time.Time) tea.Msg { return tickMsg(t) }) }

func (m *model) Init() tea.Cmd {
	if m.ag == nil {
		return doTick()
	}
	return tea.Batch(m.waitEvent(), doTick())
}

// ---------- rendering helpers ----------

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
		r, err := glamour.NewTermRenderer(glamour.WithStandardStyle(style), glamour.WithWordWrap(w), glamour.WithEmoji())
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

func (m *model) modeText() string { return modeLabel(m.mode) }

func (m *model) renderWelcome() string {
	w := min(m.tw()-2, 78)
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
	if n := len(e.output); n > 0 {
		b.WriteByte('|')
		b.WriteString(e.output[n-1])
	}
	if e.running {
		fmt.Fprintf(&b, "|%d|%s", (m.frame/4)%2, humanDuration(time.Since(e.started)))
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

func (m *model) paintEntry(e *entry) string {
	w := m.tw() - 2
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
	case enStage:
		return m.paintStage(e)
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
	mark := ""
	if m.focusedEntry() == e {
		mark = titleStyle.Render("▸ ")
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

func firstSentence(s string) string {
	if i := strings.Index(s, ". "); i > 0 {
		return s[:i+1]
	}
	return s
}

func (m *model) transcript() string {
	parts := make([]string, 0, len(m.entries))
	for _, e := range m.entries {
		parts = append(parts, m.renderEntry(e))
	}
	return strings.Join(parts, "\n\n") + "\n"
}

func (m *model) refresh() {
	if !m.ready {
		return
	}
	atBottom := m.vp.AtBottom()
	content := m.transcript()
	if m.searchOn && m.searchQ != "" {
		content = highlightQuery(content, m.searchQ)
	}
	m.vp.SetContent(content)
	m.resize()
	if m.searchOn {
		lines := matchLines(m.transcript(), m.searchQ)
		if m.searchAt >= 0 && m.searchAt < len(lines) {
			m.vp.SetYOffset(lines[m.searchAt])
		}
		m.follow = false
		return
	}
	if m.follow || atBottom || m.state == stIdle {
		m.vp.GotoBottom()
	}
	m.follow = false
}

// runningEntry finds the tool card a line belongs to. Read-only calls from
// one answer run together, so output is matched by tool and command rather
// than assumed to belong to whichever card started last.
func (m *model) runningEntry(tool, command string) *entry {
	for i := len(m.entries) - 1; i >= 0; i-- {
		e := m.entries[i]
		if e.kind != enTool || !e.running || e.tool != tool {
			continue
		}
		if command != "" && e.command != command {
			continue
		}
		return e
	}
	return nil
}

func (m *model) add(e *entry) {
	m.entries = append(m.entries, e)
	if e.kind == enTool {
		m.focus = len(m.entries) - 1
	}
}

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
		ent := &entry{kind: enTool, tool: e.Tool, command: e.Command, running: true, started: time.Now()}
		m.add(ent)
		m.current = ent
	case agent.EvOutput:
		if ent := m.runningEntry(e.Tool, e.Command); ent != nil {
			ent.output = append(ent.output, e.Text)
			if len(ent.output) > maxKept {
				ent.output = ent.output[len(ent.output)-maxKept:]
			}
		}
	case agent.EvToolEnd:
		cur := m.runningEntry(e.Tool, e.Command)
		if cur == nil {
			cur = &entry{kind: enTool, tool: e.Tool, command: e.Command}
			m.add(cur)
		}
		cur.running, cur.failed, cur.exitCode, cur.duration, cur.lines = false, e.Failed, e.ExitCode, e.Duration, e.Lines
		if free, _ := diskSpace("/"); free > 0 {
			m.diskNow = free
		}
		if e.Reason != "" {
			cur.errText = e.Reason
			m.follow = true
		}
		m.current = nil
		for i := len(m.entries) - 1; i >= 0; i-- {
			if m.entries[i].kind == enTool && m.entries[i].running {
				m.current = m.entries[i]
				break
			}
		}
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
		m.mode = e.Mode
		m.note(okStyle.Render("⏵ ")+mutedStyle.Render(i18n.T("ui.mode_lifted")), lipgloss.NewStyle())
	case agent.EvIdle:
		m.thinking = false
		m.state = stIdle
		m.input.Reset()
		return m.input.Focus()
	case agent.EvError:
		m.thinking = false
		text := strings.TrimSpace(e.Text)
		if text == "" {
			break
		}
		m.note(m.wrapPlain("! "+text), errStyle)
		m.follow = true
	case agent.EvDone:
		m.thinking = false
		m.state = stDone
		if e.Text != "" {
			m.note(e.Text, mutedStyle)
		}
		m.note(i18n.T("ui.summary_done", humanDuration(m.stats.Elapsed),
			humanTokens(m.stats.Usage.Input()+m.stats.Usage.OutputTokens), m.costText()), mutedStyle)
		if m.quitting {
			return m.leave()
		}
	}
	return nil
}

// ---------- update ----------

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	if ev, ok := msg.(agEvent); ok {
		if ev.ag != m.ag {
			return m, nil // a late event of a session that is over
		}
		if ev.closed {
			msg = closedMsg{}
		} else {
			msg = ev.e
		}
	}
	if m.g != nil {
		if cmd, handled := m.guideUpdate(msg); handled {
			return m, cmd
		}
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.input.Width = msg.Width - 8
		if !m.ready {
			m.vp = viewport.New(m.tw(), 1)
			m.vp.MouseWheelEnabled = true
			m.ready = true
		}
		m.vp.Width = m.tw()
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
		if m.g != nil {
			m.guideAfter(msg)
		}
		m.refresh()
		cmds = append(cmds, m.waitEvent())

	case closedMsg:
		m.state = stDone
		if m.g != nil {
			m.g.release()
		}
		if m.quitting {
			return m, m.leave()
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.cancel()
			return m, tea.Quit
		case "ctrl+f":
			if m.state != stPassword {
				m.help = false
				m.searchOn = !m.searchOn
				if !m.searchOn {
					m.searchQ = ""
					m.searchAt = -1
				}
				m.refresh()
			}
			return m, nil
		case "ctrl+y":
			if m.state != stPassword {
				m.copyFocused()
			}
			return m, nil
		case "alt+up":
			m.moveFocus(-1)
			return m, nil
		case "alt+down":
			m.moveFocus(1)
			return m, nil
		}
		if m.searchOn {
			m.handleSearchKey(msg)
			return m, nil
		}
		if m.help {
			switch msg.String() {
			case "?", "esc", "q":
				m.help = false
				m.refresh()
				return m, nil
			case "up", "down", "pgup", "pgdown", "ctrl+u", "ctrl+d":
				var cmd tea.Cmd
				m.vp, cmd = m.vp.Update(msg)
				return m, cmd
			}
			return m, nil
		}
		switch msg.String() {
		case "ctrl+o":
			m.expanded = !m.expanded
			m.refresh()
			return m, nil
		case "ctrl+b":
			m.togglePanel()
			return m, nil
		case "shift+tab":
			if m.state != stDone {
				m.setMode(m.mode.Next())
				m.refresh()
			}
			return m, nil
		case "ctrl+p", "alt+p":
			if m.state != stDone {
				m.cycleModel()
				m.refresh()
			}
			return m, nil
		case "?":
			if m.helpAllowed() {
				m.help = true
				m.refresh()
				return m, nil
			}
		}
		switch m.state {
		case stConfirm:
			if scrollKey(msg) {
				var cmd tea.Cmd
				m.vp, cmd = m.vp.Update(msg)
				return m, cmd
			}
			var d agent.Decision = -1
			switch strings.ToLower(msg.String()) {
			case "y", "s", "o", "j", "enter":
				d = agent.Approve
			case "a":
				if !m.pending.Always {
					d = agent.ApproveAll
					m.mode = agent.ModeAuto
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
			if scrollKey(msg) {
				var cmd tea.Cmd
				m.vp, cmd = m.vp.Update(msg)
				return m, cmd
			}
			if done, cancelled := m.picker.HandleKey(msg); done {
				return m, m.finishChoice(cancelled)
			}
			m.refresh()
			return m, nil
		case stIdle:
			if m.handleMenuKey(msg) {
				return m, nil
			}
			switch msg.Type {
			case tea.KeyEnter:
				text := strings.TrimSpace(m.input.Value())
				if strings.HasPrefix(text, "/") {
					return m, m.submitCommand(text)
				}
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
				// First Esc clears a typed line; the next one ends the session.
				if strings.TrimSpace(m.input.Value()) != "" {
					m.input.Reset()
					m.menuSel = 0
					return m, nil
				}
				m.endSession()
				return m, nil
			case tea.KeyPgUp, tea.KeyPgDown, tea.KeyUp, tea.KeyDown:
				var cmd tea.Cmd
				m.vp, cmd = m.vp.Update(msg)
				return m, cmd
			}
			before := m.input.Value()
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			if m.input.Value() != before {
				m.menuSel = 0
			}
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
		if m.clickCard(msg) {
			return m, nil
		}
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

// endSession tells the agent the user is done.
func (m *model) endSession() {
	m.input.Blur()
	m.state = stRunning
	ch := m.ag.Input()
	go func() { ch <- "" }()
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

// confirmButton is a clickable span on the approval card's key line.
// start and end are columns in that line's visible text.
type confirmButton struct {
	start, end int
	decision   agent.Decision
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
	write(warnStyle.Render(i18n.T("ui.confirm")) + "  " + mutedStyle.Render(i18n.T("ui.risk", riskLabel(e.Risk))))
	write("")
	name, arg := toolLabel(e.Tool, e.Command)
	write(cmdStyle.Render(name) + mutedStyle.Render("(") + truncate(arg, max(10, w-12)) + mutedStyle.Render(")"))
	if strings.HasPrefix(e.Tool, "run_step:") {
		write(faintStyle.Render(truncate(e.Command, max(10, w-6))))
	}
	if e.Reason != "" {
		lines := strings.Split(e.Reason, "\n")
		if len(lines) > 14 {
			lines = append(lines[:14], "…")
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
		write(warnStyle.Render("⚠ ") + mutedStyle.Render(i18n.T("ui.always_note")))
	}
	write("")
	keys := i18n.T("ui.confirm_keys")
	marks := []struct {
		mark string
		d    agent.Decision
	}{{"[y]", agent.Approve}, {"[a]", agent.ApproveAll}, {"[n]", agent.Deny}}
	if e.Always {
		keys = i18n.T("ui.confirm_keys_once")
		marks = []struct {
			mark string
			d    agent.Decision
		}{{"[y]", agent.Approve}, {"[n]", agent.Deny}}
	}
	for _, mark := range marks {
		at := strings.Index(keys, mark.mark)
		if at < 0 {
			continue
		}
		buttons = append(buttons, confirmButton{start: at, end: at + len(mark.mark), decision: mark.d})
	}
	sort.Slice(buttons, func(i, j int) bool { return buttons[i].start < buttons[j].start })
	// Extend each button up to the next one so the label next to [y] is included.
	for i := range buttons {
		end := len(keys)
		if i+1 < len(buttons) {
			end = buttons[i+1].start
		}
		buttons[i].end = end
	}
	keyLine = line
	write(keys)
	return b.String(), keyLine, buttons
}

func (m *model) confirmView() string {
	body, _, _ := m.confirmLayout()
	return confirmStyle.Width(m.width - 2).Render(body)
}

func (m *model) statusBar() string {
	var left string
	switch m.mode {
	case agent.ModeReadOnly:
		left = okStyle.Render("⏵⏵ " + i18n.T("ui.mode_readonly"))
	case agent.ModeAuto:
		left = warnStyle.Render("⏵⏵ " + i18n.T("ui.mode_auto"))
	default:
		left = mutedStyle.Render("⏵⏵ " + i18n.T("ui.mode_ask"))
	}
	if m.state != stDone && (m.g == nil || m.ag != nil) {
		left += faintStyle.Render(" " + i18n.T("ui.mode_hint"))
	}
	var parts []string
	if m.header.Model != "" {
		parts = append(parts, m.header.Model)
	}
	parts = append(parts, "↑"+humanTokens(m.stats.Usage.Input())+" ↓"+humanTokens(m.stats.Usage.OutputTokens))
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

func (m *model) footerView() string {
	if m.g != nil && !m.help {
		if v, ok := m.guideFooter(); ok {
			return m.withSearchBar(v)
		}
	}
	if m.help {
		return m.withSearchBar(m.helpView() + "\n" + m.statusBar() + "\n" + faintStyle.Render("  "+i18n.T("ui.hint_help")))
	}
	var top string
	switch m.state {
	case stConfirm:
		top = m.confirmView()
	case stChoice:
		top = m.picker.View(m.width)
	case stPassword:
		top = m.passwordView()
	case stIdle:
		top = inputStyle.Width(m.width - 2).Render(m.input.View())
		if menu := m.menuView(); menu != "" {
			return m.withSearchBar(top + "\n" + menu + "\n" + m.statusBar())
		}
		hint := i18n.T("ui.hint_idle")
		if m.g != nil {
			hint = i18n.T("full.hint_idle")
		}
		return m.withSearchBar(top + "\n" + m.statusBar() + "\n" + faintStyle.Render("  "+hint))
	case stDone:
		return m.withSearchBar(m.statusBar() + "\n" + faintStyle.Render("  "+i18n.T("ui.hint_done")))
	default:
		if m.thinking {
			el := humanDuration(time.Since(m.thinkStart))
			glyph := titleStyle.Render(spinFrames[m.frame%len(spinFrames)])
			top = glyph + " " + m.shimmer(m.verb+"…") + " " +
				faintStyle.Render(i18n.T("ui.thinking_meta", el, humanTokens(m.stats.Usage.Input()+m.stats.Usage.OutputTokens)))
		}
	}
	return m.withSearchBar(top + "\n" + m.statusBar() + "\n" + faintStyle.Render("  "+i18n.T("ui.hint_running")))
}

func (m *model) passwordView() string {
	var b strings.Builder
	b.WriteString(warnStyle.Render(i18n.T("ui.password_title")) + "\n")
	if m.pending != nil && m.pending.Text != "" {
		b.WriteString(faintStyle.Render(truncate(m.pending.Text, m.width-8)) + "\n")
	}
	if m.pending != nil && m.pending.Failed {
		b.WriteString(errStyle.Render(i18n.T("ui.password_wrong")) + "\n")
	}
	b.WriteString("\n" + m.password.View() + "\n\n")
	b.WriteString(mutedStyle.Render(i18n.T("ui.password_hint")) + "\n")
	b.WriteString(faintStyle.Render(i18n.T("ui.password_keys")))
	return confirmStyle.Width(m.width - 2).Render(b.String())
}

func (m *model) withSearchBar(base string) string {
	if !m.searchOn {
		return base
	}
	count := i18n.T("ui.search_none")
	if m.searchQ != "" {
		n := len(matchLines(m.transcript(), m.searchQ))
		if n > 0 && m.searchAt >= 0 && m.searchAt < n {
			count = i18n.T("ui.search_count", m.searchAt+1, n)
		}
	}
	bar := inputStyle.Width(m.width - 2).Render(i18n.T("ui.search_bar", m.searchQ, count))
	return base + "\n" + bar + "\n" + faintStyle.Render("  "+i18n.T("ui.search_hint"))
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

func (m *model) View() string {
	if !m.ready {
		return i18n.T("ui.starting")
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

// togglePanel shows or hides the session panel and gives the transcript
// the width that frees or takes.
func (m *model) togglePanel() {
	m.panel = !m.panel
	if m.ready {
		m.vp.Width = m.tw()
	}
	m.refresh()
}

// scrollKey is a transcript scroll that must keep working while a card
// is open. Up and down still move the selection on that card.
func scrollKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "pgup", "pgdown", "ctrl+u", "ctrl+d":
		return true
	}
	return false
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

// ToolLabel names a tool call the way the interface shows it: a short name
// and its argument. The desktop app uses the same names as the terminal.
func ToolLabel(tool, command string) (name, arg string) { return toolLabel(tool, command) }
