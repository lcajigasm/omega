package ui

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/i18n"
)

// GuideStage is one stage of a guided run (omega full): a whole task, with
// what the user needs to decide whether to run it.
type GuideStage struct {
	ID       string
	Command  string // what starts it on its own, e.g. "omega update"
	Title    string
	What     string // what it does
	Why      string // why it is worth doing, and why at this point
	Risk     string // "read-only", "normal" or "dangerous"
	Selected bool
}

// Outcomes of a stage.
const (
	StagePending = "pending" // not run: still to come, unticked, or the run was ended before it
	StageRunning = "running"
	StageDone    = "done"
	StageFailed  = "failed"
	StageSkipped = "skipped"
	StageStopped = "stopped" // the user stopped it with ctrl+c
)

// StageResult is what happened to a stage.
type StageResult struct {
	ID        string
	Status    string
	Note      string // why it failed
	Duration  time.Duration
	Cost      float64
	CostKnown bool
}

// GuideInfo is what the guided run shows about itself.
type GuideInfo struct {
	Version string
	Flags   []string // how the stages run, e.g. "read-only"
}

// StageRunner runs one stage and returns when it has ended. A stage that
// starts an agent session does it the usual way: while a guided run is on
// screen, RunTUI shows the session inside it. What a stage writes to out is
// shown as its report.
type StageRunner func(id string, out io.Writer) error

// guide is the state of a guided run inside the session interface.
type guide struct {
	info   GuideInfo
	stages []GuideStage
	res    []StageResult
	run    StageRunner
	send   func(tea.Msg)
	quit   chan struct{} // closed when the interface has left the screen

	cur      int // stage that is running; -1 between stages
	last     int // stage that ran last; -1 before the first
	next     int // stage offered next
	cursor   int // row of the plan
	started  bool
	planBack bool // the plan was opened between two stages
	stopping bool // ctrl+c stopped the running stage
	began    time.Time
	att      *attachMsg
	report   *entry // what the running stage wrote
	cost     float64
	costOK   bool
	aborted  bool // left from the plan before anything ran
}

// attachMsg hands an agent session to the guided run on screen.
type attachMsg struct {
	ag     *agent.Agent
	cancel context.CancelFunc
	header Header
	done   chan struct{}
}

type stageOutMsg string

type stageEndMsg struct {
	idx int
	err error
}

// agEvent is an agent event with the agent it came from: a guided run goes
// through several agents, and a late event of an earlier one is dropped.
type agEvent struct {
	ag     *agent.Agent
	e      agent.Event
	closed bool
}

var (
	hostMu sync.Mutex
	host   atomic.Pointer[guide]
)

// Hosted reports whether a guided run owns the screen: a session started now
// is shown inside it, and nothing else may print to the terminal.
func Hosted() bool { return host.Load() != nil }

// session shows an agent session inside the guided run and returns when the
// session has ended or the user stopped the stage.
func (g *guide) session(cancel context.CancelFunc, ag *agent.Agent, h Header) error {
	done := make(chan struct{})
	g.send(attachMsg{ag: ag, cancel: cancel, header: h, done: done})
	select {
	case <-done:
	case <-g.quit:
		cancel()
	}
	return nil
}

func (g *guide) release() {
	if g.att != nil {
		close(g.att.done)
		g.att = nil
	}
}

// lineWriter turns what a stage writes into one message per line.
type lineWriter struct {
	send func(tea.Msg)
	buf  []byte
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		i := strings.IndexByte(string(w.buf), '\n')
		if i < 0 {
			break
		}
		w.send(stageOutMsg(strings.TrimRight(string(w.buf[:i]), "\r")))
		w.buf = w.buf[i+1:]
	}
	return len(p), nil
}

func (w *lineWriter) flush() {
	if len(w.buf) > 0 {
		w.send(stageOutMsg(string(w.buf)))
		w.buf = nil
	}
}

// newGuideModel builds the interface of a guided run, on the plan.
func newGuideModel(info GuideInfo, stages []GuideStage, run StageRunner) *model {
	m := newModel(nil, func() {}, Header{Version: info.Version, Task: "full"})
	m.entries = nil
	g := &guide{info: info, stages: stages, run: run, cur: -1, last: -1, next: -1, quit: make(chan struct{}), send: func(tea.Msg) {}}
	for _, s := range stages {
		g.res = append(g.res, StageResult{ID: s.ID, Status: StagePending})
	}
	m.g = g
	m.state = stPlan
	return m
}

// RunGuide runs a guided run on the full screen: the plan to choose the
// stages, every stage's session, the card between two stages and the
// summary, all in one interface. It returns what happened to each stage;
// ok is false when the user left the plan without running anything.
func RunGuide(info GuideInfo, stages []GuideStage, run StageRunner) (results []StageResult, ok bool, err error) {
	hostMu.Lock()
	defer hostMu.Unlock()
	m := newGuideModel(info, stages, run)
	opts := []tea.ProgramOption{tea.WithAltScreen()}
	if mouseWanted() {
		opts = append(opts, tea.WithMouseCellMotion())
	}
	p := tea.NewProgram(m, opts...)
	m.g.send = p.Send
	host.Store(m.g)
	_, err = p.Run()
	host.Store(nil)
	close(m.g.quit)
	if errors.Is(err, tea.ErrProgramKilled) {
		err = nil
	}
	return m.g.results(), !m.g.aborted, err
}

// results reports every stage; one that was unticked counts as skipped.
func (g *guide) results() []StageResult {
	out := append([]StageResult(nil), g.res...)
	for i := range out {
		if out[i].Status == StagePending && !g.stages[i].Selected {
			out[i].Status = StageSkipped
		}
		if out[i].Status == StageRunning {
			out[i].Status = StageStopped
		}
	}
	return out
}

// ---------- flow ----------

// chosen counts the stages of the run: everything but the unticked ones.
func (g *guide) chosen() (total, settled int) {
	for i, s := range g.stages {
		st := g.res[i].Status
		if st == StagePending && !s.Selected {
			continue
		}
		total++
		if st != StagePending && st != StageRunning {
			settled++
		}
	}
	return
}

// ordinal is the position of stage i among the stages of the run.
func (g *guide) ordinal(i int) int {
	n := 0
	for j := 0; j <= i && j < len(g.stages); j++ {
		if g.res[j].Status != StagePending || g.stages[j].Selected {
			n++
		}
	}
	return n
}

// upcoming is the first ticked stage that has not run; -1 when none is left.
func (g *guide) upcoming() int {
	for i, s := range g.stages {
		if s.Selected && g.res[i].Status == StagePending {
			return i
		}
	}
	return -1
}

func (m *model) startStage(i int) tea.Cmd {
	g := m.g
	g.started, g.stopping = true, false
	g.cur, g.last, g.report = i, i, nil
	g.began = time.Now()
	g.stages[i].Selected = true
	g.res[i] = StageResult{ID: g.stages[i].ID, Status: StageRunning}
	total, _ := g.chosen()
	m.state = stRunning
	m.thinking = false
	m.header.Task = g.stages[i].ID
	m.stats = agent.Stats{}
	m.add(&entry{kind: enStage, stage: i, ord: g.ordinal(i), of: total})
	m.follow = true
	m.refresh()
	run, send, id := g.run, g.send, g.stages[i].ID
	return func() tea.Msg {
		w := &lineWriter{send: send}
		err := run(id, w)
		w.flush()
		return stageEndMsg{idx: i, err: err}
	}
}

// afterStage offers the next stage, or ends the run when none is left.
func (m *model) afterStage() {
	g := m.g
	g.cur = -1
	if n := g.upcoming(); n >= 0 {
		g.next = n
		m.state = stBetween
	} else {
		m.finishGuide()
	}
	m.follow = true
	m.refresh()
}

func (m *model) finishGuide() {
	if m.state == stSummary {
		return
	}
	m.state = stSummary
	m.header.Task = "full"
	m.add(&entry{kind: enNote, text: strings.Join(summaryLines(m.g.stages, m.g.results(), m.tw()-1), "\n"), style: lipgloss.NewStyle()})
}

// guideAfter looks at an agent event once the session has handled it.
func (m *model) guideAfter(e agent.Event) {
	if e.Kind == agent.EvDone {
		m.g.release()
	}
}

// leave ends the interface, or only the stage inside a guided run.
func (m *model) leave() tea.Cmd {
	if m.g != nil {
		m.quitting = false
		return nil
	}
	return tea.Quit
}

// guideUpdate handles what belongs to the guided run. handled is false for
// what the session itself must see.
func (m *model) guideUpdate(msg tea.Msg) (cmd tea.Cmd, handled bool) {
	g := m.g
	switch msg := msg.(type) {
	case attachMsg:
		if g.cur < 0 || g.stopping {
			msg.cancel()
			close(msg.done)
			return nil, true
		}
		g.att = &msg
		m.ag, m.cancel, m.mode = msg.ag, msg.cancel, msg.ag.Mode()
		task := m.header.Task
		m.header = msg.header
		m.header.Task = task
		m.stats = agent.Stats{Model: msg.header.Model}
		m.state = stRunning
		line := msg.header.Provider + " · " + msg.header.Model
		if p := msg.ag.LogPath(); p != "" {
			line += " · " + i18n.T("ui.label_log") + " " + homeTilde(p)
		}
		for i := len(m.entries) - 1; i >= 0; i-- {
			if m.entries[i].kind == enStage {
				m.entries[i].text = line
				break
			}
		}
		m.refresh()
		return m.waitEvent(), true

	case stageOutMsg:
		if g.report == nil {
			g.report = &entry{kind: enNote, style: mutedStyle}
			m.add(g.report)
		} else {
			g.report.text += "\n"
		}
		g.report.text += string(msg)
		m.refresh()
		return nil, true

	case stageEndMsg:
		if msg.idx != g.cur {
			return nil, true
		}
		g.release()
		r := &g.res[msg.idx]
		r.Duration = time.Since(g.began)
		r.Cost, r.CostKnown = m.stats.Cost, m.stats.CostKnown
		if m.stats.CostKnown {
			g.cost += m.stats.Cost
			g.costOK = true
		}
		switch {
		case g.stopping:
			r.Status = StageStopped
		case msg.err != nil:
			r.Status = StageFailed
			r.Note = strings.TrimSpace(msg.err.Error())
			if r.Note != "" {
				m.note(m.wrapPlain(gl.Error+" "+r.Note), errStyle)
			}
		default:
			r.Status = StageDone
		}
		m.ag, m.pending, m.picker, m.current = nil, nil, nil, nil
		m.mode = agent.ModeAsk
		m.thinking = false
		m.input.Blur()
		for _, e := range m.entries {
			e.running = false
		}
		m.afterStage()
		return nil, true

	case tea.MouseMsg:
		return nil, m.state == stPlan

	case tea.KeyMsg:
		key := msg.String()
		if key == "ctrl+c" {
			return m.guideInterrupt(), true
		}
		switch m.state {
		case stPlan:
			return m.planKey(msg), true
		case stBetween, stSummary:
			if m.searchOn || m.help || guidePassKey(key) {
				return nil, false
			}
			if m.state == stSummary {
				if key == "q" || key == "enter" || key == "esc" {
					return tea.Quit, true
				}
				return nil, true
			}
			return m.betweenKey(key), true
		}
		if m.ag == nil || m.state == stDone {
			// A stage without a session, or one that is closing: only the
			// transcript can be looked at.
			if m.searchOn || m.help || guidePassKey(key) {
				return nil, false
			}
			return nil, true
		}
	}
	return nil, false
}

// guidePassKey says which keys keep their meaning when no session is on:
// the ones that look at the transcript.
func guidePassKey(key string) bool {
	switch key {
	case "ctrl+o", "ctrl+b", "ctrl+f", "ctrl+y", "alt+up", "alt+down", "up", "down", "pgup", "pgdown", "ctrl+u", "ctrl+d", "home", "end", "?":
		return true
	}
	return false
}

// guideInterrupt is ctrl+c: it stops the running stage, and outside a
// stage it ends the run.
func (m *model) guideInterrupt() tea.Cmd {
	g := m.g
	switch m.state {
	case stPlan:
		if g.planBack {
			g.planBack = false
			m.afterStage()
			return nil
		}
		g.aborted = true
		return tea.Quit
	case stBetween:
		m.finishGuide()
		m.follow = true
		m.refresh()
		return nil
	case stSummary:
		return tea.Quit
	}
	if g.cur >= 0 && !g.stopping {
		g.stopping = true
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
		if m.ag != nil {
			m.cancel()
		}
		g.release()
		m.thinking = false
		m.state = stDone
		m.note(i18n.T("full.stopping"), mutedStyle)
		m.refresh()
	}
	return nil
}

func (m *model) betweenKey(key string) tea.Cmd {
	g := m.g
	switch strings.ToLower(key) {
	case "enter", " ", "y":
		return m.startStage(g.next)
	case "s":
		g.res[g.next].Status = StageSkipped
		m.afterStage()
	case "r":
		if g.last >= 0 && (g.res[g.last].Status == StageFailed || g.res[g.last].Status == StageStopped) {
			return m.startStage(g.last)
		}
	case "p":
		g.planBack = true
		g.cursor = g.next
		m.state = stPlan
	case "q", "esc":
		m.finishGuide()
		m.follow = true
		m.refresh()
	}
	return nil
}

// togglable reports whether the plan may tick or untick a stage: one that
// ran keeps its outcome.
func (g *guide) togglable(i int) bool {
	st := g.res[i].Status
	return st == StagePending || st == StageSkipped
}

func (g *guide) toggle(i int, on bool) {
	if !g.togglable(i) {
		return
	}
	g.res[i].Status = StagePending
	g.stages[i].Selected = on
}

func (m *model) planKey(msg tea.KeyMsg) tea.Cmd {
	g := m.g
	switch msg.String() {
	case "up", "k", "shift+tab":
		g.cursor = wrap(g.cursor, -1, len(g.stages))
	case "down", "j", "tab":
		g.cursor = wrap(g.cursor, 1, len(g.stages))
	case "home", "g":
		g.cursor = 0
	case "end", "G":
		g.cursor = len(g.stages) - 1
	case " ", "x":
		g.toggle(g.cursor, !(g.stages[g.cursor].Selected && g.res[g.cursor].Status == StagePending))
	case "a":
		all := true
		for i, s := range g.stages {
			if g.togglable(i) {
				all = all && s.Selected && g.res[i].Status == StagePending
			}
		}
		for i := range g.stages {
			g.toggle(i, !all)
		}
	case "enter":
		if g.planBack {
			g.planBack = false
			m.afterStage()
			return nil
		}
		if n := g.upcoming(); n >= 0 {
			return m.startStage(n)
		}
	case "q", "esc":
		if g.planBack {
			g.planBack = false
			m.afterStage()
			return nil
		}
		g.aborted = true
		return tea.Quit
	}
	return nil
}
