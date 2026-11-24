// Package ui renders an agent run, either as a full-screen Bubble Tea TUI or
// as plain line-oriented output for pipes and CI.
package ui

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"time"

	kb "github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/i18n"
)

const (
	tailLines = 5
	maxKept   = 400
	tick      = 100 * time.Millisecond
	// silentAfter is how long a running tool may print nothing before its
	// card says so: a long quiet step is told apart from a stuck one.
	silentAfter = 30 * time.Second
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
	lastOut  time.Time // when the running tool last printed
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

	// interrupted is set by the first ctrl+c: it cancelled what was running,
	// and the next ctrl+c leaves.
	interrupted bool
	selfCancel  atomic.Bool // the session's context was cancelled from here
	unseen      int         // lines that arrived while the user was scrolled up

	// The transcript as last painted, and the search over it: both are
	// reused while nothing they depend on changed.
	tparts  []string
	tjoined string
	sc      searchCache
}

// searchCache is the highlighted transcript and the lines that match.
type searchCache struct {
	content, query string
	painted        string
	lines          []int
}

// RunTUI runs the agent inside a full-screen terminal UI.
func RunTUI(ctx context.Context, cancel context.CancelFunc, ag *agent.Agent, h Header) error {
	// A guided run already has the screen: the session is one of its stages.
	if g := host.Load(); g != nil {
		return g.session(cancel, ag, h)
	}
	m := newModel(ag, cancel, h)
	opts := []tea.ProgramOption{tea.WithAltScreen()}
	if mouseWanted() {
		opts = append(opts, tea.WithMouseCellMotion())
	}
	p := tea.NewProgram(m, opts...)
	// The run's context ends the screen when it is cancelled from outside
	// (a signal). A cancel from the screen itself (the first ctrl+c) only
	// stops the agent: the transcript stays up until the user leaves.
	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			if !m.selfCancel.Load() {
				p.Kill()
			}
		case <-stop:
		}
	}()
	_, err := p.Run()
	close(stop)
	// Cancelling (a signal) stops the program through its context.
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
	ti.Prompt = gl.Prompt + " "
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
		m.input.Width = max(10, msg.Width-8)
		if !m.ready {
			m.vp = viewport.New(m.tw(), 1)
			m.vp.MouseWheelEnabled = true
			m.ready = true
		}
		m.vp.Width = m.tw()
		m.refresh()
		m.vp.GotoBottom()
		m.unseen = 0

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
		m.thinking = false
		if m.g != nil {
			m.g.release()
		}
		if m.quitting {
			return m, m.leave()
		}
		m.refresh()

	case tea.KeyMsg:
		return m, m.key(msg)

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

// key handles a key press in the session.
func (m *model) key(msg tea.KeyMsg) tea.Cmd {
	switch {
	case kb.Matches(msg, km.Interrupt):
		return m.interrupt()
	case kb.Matches(msg, km.Search):
		if m.state != stPassword {
			m.help = false
			m.searchOn = !m.searchOn
			if !m.searchOn {
				m.searchQ = ""
				m.searchAt = -1
			}
			m.refresh()
		}
		return nil
	case kb.Matches(msg, km.Copy):
		if m.state != stPassword {
			m.copyFocused()
		}
		return nil
	case kb.Matches(msg, km.FocusPrev):
		m.moveFocus(-1)
		return nil
	case kb.Matches(msg, km.FocusNext):
		m.moveFocus(1)
		return nil
	}
	if m.searchOn {
		m.handleSearchKey(msg)
		return nil
	}
	if m.help {
		switch msg.String() {
		case "?", "esc", "q":
			m.help = false
			m.refresh()
			return nil
		case "up", "down", "pgup", "pgdown", "ctrl+u", "ctrl+d":
			var cmd tea.Cmd
			m.vp, cmd = m.vp.Update(msg)
			return cmd
		}
		return nil
	}
	switch {
	case kb.Matches(msg, km.Expand):
		m.expanded = !m.expanded
		m.refresh()
		return nil
	case kb.Matches(msg, km.Panel):
		m.togglePanel()
		return nil
	case kb.Matches(msg, km.Mode):
		if m.state != stDone {
			m.setMode(m.mode.Next())
			m.refresh()
		}
		return nil
	case kb.Matches(msg, km.Model):
		if m.state != stDone {
			m.cycleModel()
			m.refresh()
		}
		return nil
	case kb.Matches(msg, km.Help):
		if m.helpAllowed() {
			m.help = true
			m.refresh()
			return nil
		}
	}
	switch m.state {
	case stConfirm:
		if scrollKey(msg) {
			var cmd tea.Cmd
			m.vp, cmd = m.vp.Update(msg)
			return cmd
		}
		d := m.confirmDecision(msg)
		if d >= 0 {
			m.decide(d)
		}
		return nil
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
			return nil
		}
		var cmd tea.Cmd
		m.password, cmd = m.password.Update(msg)
		return cmd
	case stChoice:
		if scrollKey(msg) {
			var cmd tea.Cmd
			m.vp, cmd = m.vp.Update(msg)
			return cmd
		}
		if done, cancelled := m.picker.HandleKey(msg); done {
			return m.finishChoice(cancelled)
		}
		m.refresh()
		return nil
	case stIdle:
		return m.idleKey(msg)
	case stDone:
		if msg.String() == "q" || msg.Type == tea.KeyEnter || msg.Type == tea.KeyEsc {
			return tea.Quit
		}
	}
	if m.jumpKey(msg, true) {
		return nil
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return cmd
}

// idleKey handles a key while the session waits for the user to type.
func (m *model) idleKey(msg tea.KeyMsg) tea.Cmd {
	if m.handleMenuKey(msg) {
		return nil
	}
	empty := strings.TrimSpace(m.input.Value()) == ""
	switch msg.Type {
	case tea.KeyEnter:
		text := strings.TrimSpace(m.input.Value())
		if strings.HasPrefix(text, "/") {
			return m.submitCommand(text)
		}
		m.input.Blur()
		m.state = stRunning
		m.interrupted = false
		if text != "" {
			m.add(&entry{kind: enUser, text: text})
		}
		ch := m.ag.Input()
		go func() { ch <- text }()
		m.refresh()
		return nil
	case tea.KeyEsc:
		// First Esc clears a typed line; the next one ends the session.
		if !empty {
			m.input.Reset()
			m.menuSel = 0
			return nil
		}
		m.endSession()
		return nil
	case tea.KeyPgUp, tea.KeyPgDown, tea.KeyUp, tea.KeyDown:
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return cmd
	}
	// With nothing typed, home and end move the transcript, not the cursor.
	if empty && m.jumpKey(msg, false) {
		return nil
	}
	before := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != before {
		m.menuSel = 0
	}
	return cmd
}

// jumpKey takes the transcript to its top or bottom: home and end, and g
// and G when letters are not being typed.
func (m *model) jumpKey(msg tea.KeyMsg, letters bool) bool {
	s := msg.String()
	switch {
	case s == "home" || (letters && s == "g"):
		m.vp.GotoTop()
	case s == "end" || (letters && s == "G"):
		m.vp.GotoBottom()
		m.unseen = 0
	default:
		return false
	}
	return true
}

// interrupt is ctrl+c. The first one stops what runs (the agent and the
// command it ran) and answers any card it was waiting on; the transcript
// stays so the user can read what happened. The second one leaves.
func (m *model) interrupt() tea.Cmd {
	if m.interrupted || m.state == stDone {
		m.cancel()
		return tea.Quit
	}
	m.interrupted = true
	if m.pending != nil {
		// The agent waits for an answer: give it one so it can end.
		switch {
		case m.pending.Reply != nil:
			m.pending.Reply <- agent.Deny
		case m.pending.Picks != nil:
			m.pending.Picks <- nil
		case m.pending.Secret != nil:
			m.pending.Secret <- ""
		}
		m.pending = nil
	}
	m.picker, m.onPick = nil, nil
	m.help = false
	if m.state != stIdle {
		m.thinking = false
		// Only the turn stops when the agent can do that: the session
		// stays, waiting for what the user types next. Otherwise the whole
		// run stops and the transcript stays up until the user leaves.
		if m.ag != nil && m.ag.CancelTurn() {
			m.note(i18n.T("uix.turn_cancelled"), mutedStyle)
			m.follow = true
			m.refresh()
			return nil
		}
		m.selfCancel.Store(true)
		m.cancel()
		m.state = stRunning
	}
	m.note(i18n.T("uix.cancelled"), mutedStyle)
	m.follow = true
	m.refresh()
	return nil
}

// endSession tells the agent the user is done.
func (m *model) endSession() {
	m.input.Blur()
	m.state = stRunning
	ch := m.ag.Input()
	go func() { ch <- "" }()
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
