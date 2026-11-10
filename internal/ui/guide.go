package ui

import (
	"context"
	"errors"
	"fmt"
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
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
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
	m.add(&entry{kind: enNote, text: strings.Join(SummaryLines(m.g.stages, m.g.results()), "\n"), style: lipgloss.NewStyle().MaxWidth(max(40, m.tw()-1))})
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
				m.note(m.wrapPlain("! "+r.Note), errStyle)
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
		if g.cursor > 0 {
			g.cursor--
		}
	case "down", "j", "tab":
		if g.cursor < len(g.stages)-1 {
			g.cursor++
		}
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

// ---------- views ----------

// stageMark is the sign of a stage on the route.
func (m *model) stageMark(i int) string {
	g := m.g
	switch g.res[i].Status {
	case StageDone:
		return okStyle.Render("✓")
	case StageFailed:
		return errStyle.Render("✗")
	case StageStopped:
		return warnStyle.Render("■")
	case StageSkipped:
		return faintStyle.Render("↷")
	case StageRunning:
		return titleStyle.Render(spinFrames[m.frame%len(spinFrames)])
	}
	if !g.stages[i].Selected {
		return faintStyle.Render("↷")
	}
	return mutedStyle.Render("·")
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
// beside it.
func field(label, text string, width int) string {
	const col = fieldCol
	w := width - col
	if w < 20 {
		w = 20
	}
	lines := strings.Split(lipgloss.NewStyle().Width(w).Render(text), "\n")
	var b strings.Builder
	for i, l := range lines {
		if i == 0 {
			b.WriteString(faintStyle.Render(lipgloss.NewStyle().Width(col).Render(label)))
		} else {
			b.WriteString("\n" + strings.Repeat(" ", col))
		}
		b.WriteString(strings.TrimRight(l, " "))
	}
	return b.String()
}

// paintStage is the heading a stage opens in the transcript.
func (m *model) paintStage(e *entry) string {
	s := m.g.stages[e.stage]
	w := max(40, m.tw()-2)
	head := " " + i18n.T("full.stage", e.ord, e.of) + " · " + s.Title + " "
	fill := w - lipgloss.Width(head) - 2
	if fill < 2 {
		fill = 2
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render("━━"+head) + lipgloss.NewStyle().Foreground(accentDim).Render(strings.Repeat("━", fill)) + "\n")
	b.WriteString(field(i18n.T("full.what"), s.What, w) + "\n")
	b.WriteString(field(i18n.T("full.why"), s.Why, w) + "\n")
	b.WriteString(faintStyle.Render(lipgloss.NewStyle().Width(fieldCol).Render(i18n.T("full.alone"))) + cmdStyle.Render(s.Command) + "  " + riskWord(s.Risk))
	if e.text != "" {
		b.WriteString("\n" + strings.Repeat(" ", fieldCol) + faintStyle.Render(truncate(e.text, w-fieldCol)))
	}
	return b.String()
}

// planView is the first screen: every stage, what it does and why, and
// which ones will run.
func (m *model) planView() string {
	g := m.g
	w := min(max(m.width, 60), 110)
	inner := w - 4
	var b strings.Builder
	b.WriteString(" " + titleStyle.Render("✻ omega full") + mutedStyle.Render(" "+g.info.Version+" · "+i18n.T("full.title")) + "\n")
	intro := i18n.T("full.plan_intro")
	if len(g.info.Flags) > 0 {
		intro += " " + i18n.T("full.plan_flags", strings.Join(g.info.Flags, ", "))
	}
	b.WriteString(lipgloss.NewStyle().Foreground(muted).Width(w-2).PaddingLeft(1).Render(intro) + "\n\n")

	titleW := 0
	for _, s := range g.stages {
		titleW = max(titleW, lipgloss.Width(s.Title))
	}
	for i, s := range g.stages {
		pointer := "  "
		if i == g.cursor {
			pointer = titleStyle.Render("❯ ")
		}
		box := mutedStyle.Render("◻")
		st := g.res[i].Status
		switch {
		case st != StagePending && st != StageSkipped:
			box = m.stageMark(i)
		case s.Selected && st == StagePending:
			box = riskStyle(s.Risk).Render("◼")
		}
		title := lipgloss.NewStyle().Width(titleW).Render(s.Title)
		if i == g.cursor {
			title = cmdStyle.Render(title)
		}
		tail := faintStyle.Render(s.Command)
		if st != StagePending {
			tail = mutedStyle.Render(StatusWord(st))
		}
		row := fmt.Sprintf(" %s%s %s  %s   %s", pointer, box, faintStyle.Render(fmt.Sprintf("%2d", i+1)), title, tail)
		b.WriteString(truncateANSI(row, w) + "\n")
	}

	s := g.stages[g.cursor]
	var card strings.Builder
	card.WriteString(titleStyle.Render(s.Title) + mutedStyle.Render("  "+s.Command) + "\n")
	card.WriteString(field(i18n.T("full.what"), s.What, inner) + "\n")
	card.WriteString(field(i18n.T("full.why"), s.Why, inner) + "\n")
	card.WriteString(faintStyle.Render(lipgloss.NewStyle().Width(fieldCol).Render(i18n.T("full.risk"))) + riskWord(s.Risk))
	b.WriteString("\n" + cardStyle.Width(w-2).Render(card.String()) + "\n")

	total, settled := g.chosen()
	count := i18n.T("full.plan_count", total-settled, len(g.stages))
	keys := i18n.T("full.plan_keys")
	if g.planBack {
		keys = i18n.T("full.plan_keys_back")
	}
	b.WriteString(" " + mutedStyle.Render(count) + "\n " + faintStyle.Render(keys))
	return b.String()
}

// outcomeLine says how a stage ended, for the card between two stages.
func (m *model) outcomeLine(i int) string {
	g := m.g
	r, s := g.res[i], g.stages[i]
	var parts []string
	switch r.Status {
	case StageDone:
		parts = append(parts, okStyle.Render("✓ ")+cmdStyle.Render(s.Title)+" "+mutedStyle.Render(i18n.T("full.finished", humanDuration(r.Duration))))
	case StageFailed:
		line := errStyle.Render("✗ ") + cmdStyle.Render(s.Title) + " " + errStyle.Render(StatusWord(StageFailed))
		if r.Note != "" {
			line += mutedStyle.Render(" · " + truncate(firstSentence(r.Note), 60))
		}
		parts = append(parts, line)
	case StageStopped:
		parts = append(parts, warnStyle.Render("■ ")+cmdStyle.Render(s.Title)+" "+mutedStyle.Render(StatusWord(StageStopped)))
	default:
		parts = append(parts, faintStyle.Render("↷ ")+cmdStyle.Render(s.Title)+" "+mutedStyle.Render(StatusWord(r.Status)))
	}
	if r.CostKnown && r.Cost > 0 {
		parts = append(parts, mutedStyle.Render(fmt.Sprintf("$%.4f", r.Cost)))
	}
	return strings.Join(parts, mutedStyle.Render(" · "))
}

// betweenView is the card between two stages: how the last one ended,
// what comes next and why, and what the user can do about it.
func (m *model) betweenView() string {
	g := m.g
	inner := m.width - 6
	var b strings.Builder
	retry := false
	if g.last >= 0 {
		b.WriteString(m.outcomeLine(g.last) + "\n\n")
		retry = g.res[g.last].Status == StageFailed || g.res[g.last].Status == StageStopped
	}
	total, _ := g.chosen()
	s := g.stages[g.next]
	b.WriteString(titleStyle.Render(i18n.T("full.next")) + mutedStyle.Render(" · "+i18n.T("full.stage", g.ordinal(g.next), total)+" · ") +
		cmdStyle.Render(s.Title) + faintStyle.Render("  "+s.Command) + "\n")
	b.WriteString(field(i18n.T("full.what"), s.What, inner) + "\n")
	b.WriteString(field(i18n.T("full.why"), s.Why, inner) + "\n")
	b.WriteString(faintStyle.Render(lipgloss.NewStyle().Width(fieldCol).Render(i18n.T("full.risk"))) + riskWord(s.Risk) + "\n\n")
	keys := i18n.T("full.between_keys")
	if retry {
		keys = i18n.T("full.between_keys_retry")
	}
	b.WriteString(truncateANSI(keys, inner))
	return cardStyle.Width(m.width - 2).Render(b.String())
}

// guideFooter is the footer while the guided run, not a session, has the
// keyboard.
func (m *model) guideFooter() (string, bool) {
	switch m.state {
	case stBetween:
		return m.betweenView() + "\n" + m.statusBar(), true
	case stSummary:
		return m.statusBar() + "\n" + faintStyle.Render("  "+i18n.T("full.hint_summary")), true
	}
	if m.ag == nil || m.state == stDone {
		top := ""
		if m.g.cur >= 0 {
			glyph := titleStyle.Render(spinFrames[m.frame%len(spinFrames)])
			top = glyph + " " + m.shimmer(m.g.stages[m.g.cur].Title+"…") + " " + faintStyle.Render(humanDuration(time.Since(m.g.began)))
		}
		return top + "\n" + m.statusBar() + "\n" + faintStyle.Render("  "+i18n.T("full.hint_stage")), true
	}
	return "", false
}

// topRows is the height of what sits above the transcript: the route on
// one line when the panel, which shows it in full, has no room.
func (m *model) topRows() int {
	if m.g != nil && !m.panelShown() {
		return 1
	}
	return 0
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
	s := " " + titleStyle.Render("✻ omega full") + "  " + marks.String() + "  " + mutedStyle.Render(fmt.Sprintf("%d/%d", settled, total))
	if g.cur >= 0 {
		s += mutedStyle.Render(" · ") + cmdStyle.Render(g.stages[g.cur].Title)
	}
	return truncateANSI(s, m.width)
}

// routeLines is the route for the session panel: every stage and how it went.
func (m *model) routeLines(inner int) []string {
	g := m.g
	total, settled := g.chosen()
	head := titleStyle.Render(i18n.T("full.route"))
	count := fmt.Sprintf("%d/%d", settled, total)
	gap := inner - lipgloss.Width(head) - lipgloss.Width(count)
	if gap < 1 {
		gap = 1
	}
	lines := []string{head + strings.Repeat(" ", gap) + mutedStyle.Render(count)}
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
	lines := []string{titleStyle.Render("━━ " + i18n.T("full.summary"))}
	titleW := 0
	for _, s := range stages {
		titleW = max(titleW, lipgloss.Width(s.Title))
	}
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
		mark := faintStyle.Render("·")
		switch r.Status {
		case StageDone:
			mark = okStyle.Render("✓")
		case StageFailed:
			mark = errStyle.Render("✗")
		case StageStopped:
			mark = warnStyle.Render("■")
		case StageSkipped:
			mark = faintStyle.Render("↷")
		}
		line := fmt.Sprintf("  %s %s  %s", mark, lipgloss.NewStyle().Width(titleW).Render(s.Title), mutedStyle.Render(StatusWord(r.Status)))
		if r.Duration >= time.Second {
			line += faintStyle.Render(" · " + humanDuration(r.Duration))
		}
		if r.Status == StageFailed || r.Status == StageStopped || r.Status == StagePending {
			line += faintStyle.Render("  → " + s.Command)
		}
		lines = append(lines, line)
		if r.Note != "" {
			lines = append(lines, "      "+errStyle.Render(truncate(firstSentence(r.Note), 90)))
		}
	}
	total := i18n.T("full.total", count[StageDone], count[StageFailed]+count[StageStopped], count[StageSkipped], count[StagePending])
	if took >= time.Second {
		total += " · " + humanDuration(took)
	}
	if costOK {
		total += fmt.Sprintf(" · $%.4f", cost)
	}
	return append(lines, "", "  "+mutedStyle.Render(total))
}
