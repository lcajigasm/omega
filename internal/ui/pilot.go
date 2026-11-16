package ui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/i18n"
)

// The pilot is the screen of an unattended run (omega full --unattended):
// nobody answers anything, so instead of a session to talk to it shows a
// mascot that says what it is doing, the stage it is on, the route, and
// everything that has run so far.

// PilotInfo is what an unattended run shows about itself.
type PilotInfo struct {
	Version string
	Flags   []string // how the stages run, e.g. "read-only"
}

// Kinds of line in the list of what has run.
const (
	actRunning = iota
	actOK
	actFailed
	actDenied
	actError // an error of the session, not of a command
	actNote
	actStage // a stage began
)

// pilotAct is one thing the run did: a command, a refusal, a note.
type pilotAct struct {
	kind  int
	stage int
	tool  string // the tool and its command, as the session names them
	cmd   string
	name  string
	arg   string
	note  string
	exit  int
	began time.Time
	dur   time.Duration
}

// pilotStage is what the screen knows about one stage beyond its outcome.
type pilotStage struct {
	began  time.Time
	said   string   // the last thing the model said: the stage's own summary
	out    []string // what the stage printed itself
	log    string
	cost   float64
	costOK bool
}

// pilotHost is the unattended run that owns the screen.
type pilotHost struct {
	send func(tea.Msg)
	stop atomic.Bool // ctrl+c: no stage starts after the one that is running
}

var pilot atomic.Pointer[pilotHost]

// Piloted reports whether an unattended run owns the screen: a session
// started now is shown on it with PilotSession, and nothing else may print
// to the terminal.
func Piloted() bool { return pilot.Load() != nil }

type pilotStageMsg struct{ idx, ord, of int }

type pilotEndMsg struct {
	idx     int
	err     error
	dur     time.Duration
	stopped bool
}

type pilotAttachMsg struct {
	cancel context.CancelFunc
	header Header
	log    string
}

type pilotEventMsg struct{ e agent.Event }

type pilotFinishMsg struct{}

// PilotSession shows an agent session on the unattended run's screen and
// returns when the session has ended. Nobody is there to answer: what would
// need a person is refused, as a run without a terminal refuses it.
func PilotSession(ctx context.Context, cancel context.CancelFunc, ag *agent.Agent, h Header) {
	host := pilot.Load()
	send := func(tea.Msg) {}
	if host != nil {
		send = host.send
	}
	send(pilotAttachMsg{cancel: cancel, header: h, log: ag.LogPath()})
	for e := range ag.Events() {
		switch e.Kind {
		case agent.EvConfirm:
			reply(ctx, e.Reply, agent.Deny)
		case agent.EvPassword:
			reply(ctx, e.Secret, "")
		case agent.EvChoice:
			reply(ctx, e.Picks, nil)
		case agent.EvIdle:
			// The session waits for a follow-up nobody will type.
			cancel()
		}
		e.Reply, e.Picks, e.Secret = nil, nil, nil
		send(pilotEventMsg{e})
	}
}

// pilotModel is the screen of an unattended run.
type pilotModel struct {
	info   PilotInfo
	stages []GuideStage
	res    []StageResult
	st     []pilotStage
	acts   []pilotAct
	host   *pilotHost

	cur, last int // stage that is running (-1 between two), and the last one that ran
	ord, of   int // its position among the stages of the run
	lastOut   string
	say       string // what the mascot says
	thinking  bool
	thinkAt   time.Time
	oops      int // frames the mascot still winces after a failure
	header    Header
	cancel    context.CancelFunc

	width, height, frame int
	began                time.Time
	stopping, finished   bool
}

func newPilotModel(info PilotInfo, stages []GuideStage) *pilotModel {
	m := &pilotModel{info: info, stages: stages, cur: -1, last: -1, began: time.Now(),
		host: &pilotHost{send: func(tea.Msg) {}}, st: make([]pilotStage, len(stages)), say: i18n.T("pilot.starting")}
	for _, s := range stages {
		status := StagePending
		if !s.Selected {
			status = StageSkipped
		}
		m.res = append(m.res, StageResult{ID: s.ID, Status: status})
	}
	return m
}

// RunPilot runs an unattended run on the full screen: every ticked stage,
// one after another, a failure not stopping the ones after it. ctrl+c stops
// the stage that is running and the run with it. It returns what happened
// to each stage and the report to leave on the terminal once the screen is
// gone: what every stage concluded and what it could not do.
func RunPilot(info PilotInfo, stages []GuideStage, run StageRunner) (results []StageResult, report []string, err error) {
	hostMu.Lock()
	defer hostMu.Unlock()
	m := newPilotModel(info, stages)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithOutput(os.Stdout))
	m.host.send = p.Send
	// Whatever prints while the screen is up (a stage without the model, a
	// hook) would draw over it: it is read and shown as the stage's output.
	release := captureOutput(func(line string) { p.Send(stageOutMsg(line)) })
	pilot.Store(m.host)
	go func() {
		var todo []int
		for i, s := range stages {
			if s.Selected {
				todo = append(todo, i)
			}
		}
		for n, i := range todo {
			if m.host.stop.Load() {
				break
			}
			p.Send(pilotStageMsg{idx: i, ord: n + 1, of: len(todo)})
			w := &lineWriter{send: p.Send}
			began := time.Now()
			err := run(stages[i].ID, w)
			w.flush()
			p.Send(pilotEndMsg{idx: i, err: err, dur: time.Since(began), stopped: m.host.stop.Load()})
		}
		release() // the last lines reach the screen before it closes
		p.Send(pilotFinishMsg{})
	}()
	_, err = p.Run()
	pilot.Store(nil)
	release()
	if errors.Is(err, tea.ErrProgramKilled) {
		err = nil
	}
	return m.results(), m.report(), err
}

// captureOutput sends what is written to the standard output and error to
// line, until the function it returns is called.
func captureOutput(line func(string)) (release func()) {
	r, w, err := os.Pipe()
	if err != nil {
		return func() {}
	}
	stdout, stderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = w, w
	done := make(chan struct{})
	go func() {
		defer close(done)
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			line(sc.Text())
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			os.Stdout, os.Stderr = stdout, stderr
			w.Close()
			<-done
			r.Close()
		})
	}
}

// results reports every stage; one cut short by leaving counts as stopped.
func (m *pilotModel) results() []StageResult {
	out := append([]StageResult(nil), m.res...)
	for i := range out {
		if out[i].Status == StageRunning {
			out[i].Status = StageStopped
		}
		out[i].Cost, out[i].CostKnown = m.st[i].cost, m.st[i].costOK
	}
	return out
}

func (m *pilotModel) Init() tea.Cmd { return doTick() }

func (m *pilotModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case tickMsg:
		m.frame++
		if m.oops > 0 {
			m.oops--
		}
		return m, doTick()

	case tea.KeyMsg:
		if msg.String() != "ctrl+c" {
			break
		}
		if m.stopping || m.finished {
			return m, tea.Quit // asked twice: leave now
		}
		m.stopping = true
		m.host.stop.Store(true)
		if m.cancel != nil {
			m.cancel()
		}

	case pilotStageMsg:
		i := msg.idx
		m.cur, m.last, m.ord, m.of = i, i, msg.ord, msg.of
		m.res[i].Status = StageRunning
		m.st[i].began = time.Now()
		m.lastOut, m.thinking, m.cancel = "", false, nil
		m.header = Header{}
		m.say = m.stages[i].What
		m.acts = append(m.acts, pilotAct{kind: actStage, stage: i, name: m.stages[i].Title})

	case pilotAttachMsg:
		m.cancel, m.header = msg.cancel, msg.header
		if m.cur >= 0 {
			m.st[m.cur].log = msg.log
		}
		if m.stopping {
			msg.cancel()
		}

	case stageOutMsg:
		line := strings.TrimRight(stripANSI(string(msg)), " \t\r")
		if i := m.last; i >= 0 {
			if st := &m.st[i]; len(st.out) < 400 {
				st.out = append(st.out, line)
			}
		}
		if strings.TrimSpace(line) != "" {
			m.lastOut = strings.TrimSpace(line)
		}

	case pilotEventMsg:
		m.event(msg.e)

	case pilotEndMsg:
		i := msg.idx
		r := &m.res[i]
		r.Duration = msg.dur
		switch {
		case msg.stopped:
			r.Status = StageStopped
		case msg.err != nil:
			r.Status, r.Note = StageFailed, strings.TrimSpace(msg.err.Error())
			m.oops = 20
		default:
			r.Status = StageDone
		}
		for j := range m.acts {
			// The session ended under a command: it did not finish.
			if a := &m.acts[j]; a.kind == actRunning {
				a.kind, a.dur = actFailed, time.Since(a.began)
			}
		}
		m.cur, m.thinking, m.cancel, m.lastOut = -1, false, nil, ""

	case pilotFinishMsg:
		m.finished = true
		return m, tea.Quit
	}
	return m, nil
}

// event takes one event of the session that is running.
func (m *pilotModel) event(e agent.Event) {
	i := m.last
	if i < 0 {
		return
	}
	st := &m.st[i]
	if e.Stats.Model != "" {
		st.cost, st.costOK = e.Stats.Cost, e.Stats.CostKnown
		m.header.Model = e.Stats.Model
	}
	switch e.Kind {
	case agent.EvThinking:
		if !m.thinking {
			m.thinking, m.thinkAt = true, time.Now()
		}
	case agent.EvAssistant:
		m.thinking = false
		if text := strings.TrimSpace(e.Text); text != "" {
			st.said = text
			m.say = plainSay(text)
		}
	case agent.EvToolStart:
		m.thinking = false
		name, arg := toolLabel(e.Tool, e.Command)
		m.acts = append(m.acts, pilotAct{kind: actRunning, stage: i, tool: e.Tool, cmd: e.Command, name: name, arg: oneLine(arg), began: time.Now()})
		m.lastOut = ""
	case agent.EvOutput:
		if line := strings.TrimSpace(stripANSI(e.Text)); line != "" {
			m.lastOut = line
		}
	case agent.EvToolEnd:
		// Commands may run side by side: the one that ended is found by
		// what it ran. Some tools only report their end.
		n := -1
		for j := len(m.acts) - 1; j >= 0 && n < 0; j-- {
			if a := m.acts[j]; a.kind == actRunning && a.tool == e.Tool && a.cmd == e.Command {
				n = j
			}
		}
		if n < 0 {
			name, arg := toolLabel(e.Tool, e.Command)
			m.acts = append(m.acts, pilotAct{stage: i, name: name, arg: oneLine(arg), began: time.Now()})
			n = len(m.acts) - 1
		}
		a := &m.acts[n]
		a.kind, a.dur, a.exit = actOK, e.Duration, e.ExitCode
		if e.Failed {
			a.kind, a.note = actFailed, firstSentence(oneLine(e.Reason))
			m.oops = 20
		}
		m.lastOut = ""
	case agent.EvDenied:
		name, arg := toolLabel(e.Tool, e.Command)
		m.acts = append(m.acts, pilotAct{kind: actDenied, stage: i, name: name, arg: oneLine(arg), note: firstSentence(oneLine(e.Reason))})
	case agent.EvModel:
		m.acts = append(m.acts, pilotAct{kind: actNote, stage: i, name: "↑ " + i18n.T("ui.model_switch", e.Text)})
	case agent.EvError:
		m.acts = append(m.acts, pilotAct{kind: actError, stage: i, name: oneLine(e.Text)})
		m.oops = 20
	case agent.EvDone:
		m.thinking = false
		if text := strings.TrimSpace(e.Text); text != "" {
			m.acts = append(m.acts, pilotAct{kind: actNote, stage: i, name: oneLine(text)})
		}
	}
}

// running is the command that is running, the latest when there are
// several; -1 when none is.
func (m *pilotModel) running() int {
	for j := len(m.acts) - 1; j >= 0; j-- {
		if m.acts[j].kind == actRunning {
			return j
		}
	}
	return -1
}

// oneLine puts a text on one line.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// plainSay turns what the model wrote (Markdown) into a sentence the mascot
// can say on a couple of lines.
func plainSay(s string) string {
	var parts []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimSpace(strings.TrimLeft(line, "#>-*• "))
		if line == "" || strings.Trim(line, "-=|: ") == "" {
			continue
		}
		parts = append(parts, line)
	}
	return oneLine(strings.NewReplacer("**", "", "__", "", "`", "").Replace(strings.Join(parts, " ")))
}

// tally counts what the run did, in one stage or (stage < 0) in all.
func (m *pilotModel) tally(stage int) (ok, failed, denied int) {
	for _, a := range m.acts {
		if stage >= 0 && a.stage != stage {
			continue
		}
		switch a.kind {
		case actOK:
			ok++
		case actFailed, actError:
			failed++
		case actDenied:
			denied++
		}
	}
	return
}

func (m *pilotModel) progress() (total, settled int) {
	for i, s := range m.stages {
		if !s.Selected {
			continue
		}
		total++
		if st := m.res[i].Status; st != StagePending && st != StageRunning {
			settled++
		}
	}
	return
}

func (m *pilotModel) cost() (float64, bool) {
	var sum float64
	known := false
	for _, st := range m.st {
		if st.costOK {
			sum += st.cost
			known = true
		}
	}
	return sum, known
}

// ---------- the mascot ----------

// Moods of the mascot.
const (
	moodIdle = iota
	moodWork
	moodThink
	moodOops
	moodHappy
	moodSad
	moodStop
)

// mascotWidth is the width of every line of the mascot.
const mascotWidth = 9

// mascot draws the mascot: a small round robot with an Ω on its chest. Its
// eyes and arms say what the run is doing.
func mascot(mood, frame int) []string {
	eyes, arms := " o o ", `/| Ω |\`
	switch mood {
	case moodIdle, moodWork:
		if frame%40 < 2 {
			eyes = " - - " // a blink
		}
		if mood == moodWork && frame/4%2 == 1 {
			arms = `\| Ω |/`
		}
	case moodThink:
		eyes = []string{" o o ", "o o  ", " o o ", "  o o"}[frame/6%4]
	case moodOops:
		eyes = " > < "
	case moodHappy:
		eyes, arms = " ^ ^ ", `\| Ω |/`
	case moodSad:
		eyes = " ; ; "
	case moodStop:
		eyes = " - - "
	}
	return []string{
		"  .---.  ",
		" (" + eyes + ") ",
		" " + arms + " ",
		"  '---'  ",
		`   " "   `,
	}
}

func (m *pilotModel) mood() int {
	switch {
	case m.finished:
		for _, r := range m.res {
			if r.Status == StageFailed {
				return moodSad
			}
		}
		if m.stopping {
			return moodStop
		}
		return moodHappy
	case m.stopping:
		return moodStop
	case m.oops > 0:
		return moodOops
	case m.thinking:
		return moodThink
	case m.running() >= 0 || (m.cur >= 0 && m.cancel == nil):
		return moodWork
	}
	return moodIdle
}

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

func (m *pilotModel) spin() string { return titleStyle.Render(spinFrames[m.frame%len(spinFrames)]) }

func (m *pilotModel) mark(i int) string {
	switch m.res[i].Status {
	case StageDone:
		return okStyle.Render("✓")
	case StageFailed:
		return errStyle.Render("✗")
	case StageStopped:
		return warnStyle.Render("■")
	case StageSkipped:
		return faintStyle.Render("↷")
	case StageRunning:
		return m.spin()
	}
	return mutedStyle.Render("·")
}

func (m *pilotModel) headLine(w int) string {
	left := " " + titleStyle.Render("✻ omega") + " " + cmdStyle.Render(i18n.T("pilot.title"))
	if m.info.Version != "" {
		left += mutedStyle.Render("  " + m.info.Version)
	}
	if m.header.Model != "" {
		left += faintStyle.Render(" · " + m.header.Model)
	}
	if len(m.info.Flags) > 0 {
		left += faintStyle.Render(" · " + strings.Join(m.info.Flags, ", "))
	}
	right := humanDuration(time.Since(m.began))
	if c, ok := m.cost(); ok {
		right += fmt.Sprintf(" · $%.4f", c)
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
		bar = "  " + lipgloss.NewStyle().Foreground(accent).Render(strings.Repeat("━", fill)) +
			lipgloss.NewStyle().Foreground(faint).Render(strings.Repeat("━", barW-fill))
	}
	return truncateANSI(" "+marks.String()+"  "+mutedStyle.Render(count)+bar, w)
}

// bubble is what the mascot says: the stage it is on and what it is doing.
func (m *pilotModel) bubble(w int) []string {
	inner := max(16, w-4)
	head := mutedStyle.Render(i18n.T("pilot.title"))
	switch {
	case m.cur >= 0:
		s := m.stages[m.cur]
		head = titleStyle.Render(i18n.T("full.stage", m.ord, m.of)) + mutedStyle.Render(" · ") + cmdStyle.Render(s.Title) +
			faintStyle.Render("  "+s.Command+" · "+humanDuration(time.Since(m.st[m.cur].began)))
	case m.stopping:
		head = warnStyle.Render(i18n.T("full.stopping"))
	}
	say := strings.Split(lipgloss.NewStyle().Width(inner).Render(m.say), "\n")
	for i := range say {
		say[i] = strings.TrimRight(say[i], " ")
	}
	if len(say) > 2 {
		last := []rune(say[1])
		say = append(say[:1], string(last[:min(len(last), inner-1)])+"…")
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
		now = "  " + m.spin() + " " + mutedStyle.Render(m.stages[m.cur].Title+"…")
	}
	// What a command printed is shown while it runs; a stage without a
	// session (a report, the built-in steps) prints by itself.
	if m.lastOut != "" && (run >= 0 || (m.cur >= 0 && m.cancel == nil)) {
		out = faintStyle.Render("    ⎿ " + truncate(m.lastOut, max(10, w-8)))
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
		return titleStyle.Render("── " + truncate(a.name, max(8, w-4)))
	}
	mark, tail := mutedStyle.Render("·"), ""
	switch a.kind {
	case actRunning:
		mark, tail = m.spin(), humanDuration(time.Since(a.began))
	case actOK:
		mark = okStyle.Render("✓")
		if a.dur >= time.Second {
			tail = humanDuration(a.dur)
		}
	case actFailed:
		mark = errStyle.Render("✗")
		if a.exit > 0 {
			tail = fmt.Sprintf("exit %d", a.exit)
		}
	case actDenied:
		mark, tail = warnStyle.Render("⊘"), i18n.T("ui.denied")
	case actError:
		mark = errStyle.Render("!")
	}
	label := a.name
	if a.arg != "" {
		label += "(" + a.arg + ")"
	}
	if a.note != "" {
		label += " · " + a.note
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
	w = max(w, 40)
	top := []string{m.headLine(w), m.progressLine(w), ""}
	if h >= 18 {
		art := mascot(m.mood(), m.frame)
		bubble := m.bubble(w - mascotWidth - 3)
		for i, line := range art {
			tip := "  "
			if i == 1 {
				tip = lipgloss.NewStyle().Foreground(accentDim).Render("◂") + " "
			}
			row := " " + lipgloss.NewStyle().Foreground(accent).Render(line) + tip
			if i < len(bubble) {
				row += bubble[i]
			}
			top = append(top, row)
		}
	} else if m.cur >= 0 {
		// A short terminal: the stage on one line, without the mascot.
		top = append(top, truncateANSI(" "+titleStyle.Render(i18n.T("full.stage", m.ord, m.of))+mutedStyle.Render(" · ")+cmdStyle.Render(m.stages[m.cur].Title), w))
	}
	top = append(top, m.nowLines(w)...)
	hint := i18n.T("pilot.hint")
	if m.stopping {
		hint = i18n.T("pilot.stopping")
	}
	foot := []string{faintStyle.Render(truncate(" "+hint, w))}

	bodyH := max(2, h-len(top)-len(foot)-2)
	var body []string
	if w >= 84 {
		routeW := 30
		route := m.routeLines(routeW, bodyH)
		done := m.doneLines(w-routeW-5, bodyH)
		sep := lipgloss.NewStyle().Foreground(faint).Render(" │ ")
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

// ---------- what is left on the terminal ----------

// reportMarkdown renders what a stage concluded for the terminal; the text
// as it was written when that cannot be done.
func reportMarkdown(s string, width int) string {
	style := "dark"
	if !lipgloss.HasDarkBackground() {
		style = "light"
	}
	r, err := glamour.NewTermRenderer(glamour.WithStandardStyle(style), glamour.WithWordWrap(min(max(width, 60), 110)-4), glamour.WithEmoji())
	if err != nil {
		return s
	}
	out, err := r.Render(s)
	if err != nil {
		return s
	}
	return strings.Trim(out, "\n")
}

// report is what an unattended run leaves on the terminal when its screen
// closes: for every stage that ran, what it concluded and what it could not
// do, then the mascot with the verdict.
func (m *pilotModel) report() []string {
	var lines []string
	res := m.results()
	failedStages, ran := 0, 0
	for i, s := range m.stages {
		r := res[i]
		if r.Status != StageDone && r.Status != StageFailed && r.Status != StageStopped {
			continue
		}
		ran++
		head := "━━ " + s.Title + " · " + StatusWord(r.Status)
		if r.Duration >= time.Second {
			head += " · " + humanDuration(r.Duration)
		}
		style := titleStyle
		if r.Status == StageFailed {
			style = errStyle
			failedStages++
		}
		lines = append(lines, style.Render(head))
		st := m.st[i]
		switch {
		case st.said != "":
			lines = append(lines, strings.Split(reportMarkdown(st.said, m.width), "\n")...)
		case len(st.out) > 0:
			lines = append(lines, strings.Split(strings.Trim(strings.Join(st.out, "\n"), "\n"), "\n")...)
		}
		if r.Note != "" {
			lines = append(lines, errStyle.Render("! "+r.Note))
		}
		for _, a := range m.acts {
			if a.stage != i || (a.kind != actFailed && a.kind != actDenied && a.kind != actError) {
				continue
			}
			label := a.name
			if a.arg != "" {
				label += "(" + a.arg + ")"
			}
			switch a.kind {
			case actDenied:
				lines = append(lines, warnStyle.Render("  ⊘ ")+label+mutedStyle.Render(" · "+i18n.T("ui.denied")+" · "+a.note))
			case actError:
				lines = append(lines, errStyle.Render("  ! ")+label)
			default:
				if a.exit > 0 {
					label += mutedStyle.Render(fmt.Sprintf(" · exit %d", a.exit))
				}
				lines = append(lines, errStyle.Render("  ✗ ")+label)
			}
		}
		if st.log != "" {
			lines = append(lines, faintStyle.Render("  "+i18n.T("run.log", homeTilde(st.log))))
		}
		lines = append(lines, "")
	}

	mood, verdict := moodHappy, okStyle.Render(i18n.T("pilot.bye_ok"))
	switch {
	case m.stopping:
		mood, verdict = moodStop, warnStyle.Render(i18n.T("pilot.bye_stopped"))
	case failedStages > 0:
		mood, verdict = moodSad, errStyle.Render(i18n.T("pilot.bye_failed", failedStages, ran))
	}
	ok, failed, denied := m.tally(-1)
	side := []string{"", verdict, mutedStyle.Render(i18n.T("pilot.tally", ok, failed, denied)), "", ""}
	for i, line := range mascot(mood, 10) {
		lines = append(lines, strings.TrimRight(" "+lipgloss.NewStyle().Foreground(accent).Render(line)+"  "+side[i], " "))
	}
	return lines
}
