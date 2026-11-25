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
	skip atomic.Bool // s: the stage that is running ends and the run goes on
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
	skipped bool // the user ended this stage alone: the run goes on
}

type pilotAttachMsg struct {
	cancel context.CancelFunc
	header Header
	log    string
}

type pilotEventMsg struct{ e agent.Event }

type pilotFinishMsg struct{}

// pilotQuitMsg closes the screen once the mascot has shown how the run went.
type pilotQuitMsg struct{}

// finale is how long the screen stays up when the run has ended, so the
// last state can be seen before the report replaces it.
const finale = 1800 * time.Millisecond

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
	cheer     int // frames the mascot still cheers a stage that ended well
	header    Header
	cancel    context.CancelFunc

	width, height, frame int
	began                time.Time
	stopping, finished   bool
	skipping             bool // the running stage is being ended with s
	scroll               int  // lines of "done so far" hidden below, when scrolled back
	title                string
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
	// Resolve the background query before Bubble Tea reads stdin, or its
	// reply arrives as an Esc key press (see Pick).
	lipgloss.HasDarkBackground()
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
			m.host.skip.Store(false)
			p.Send(pilotStageMsg{idx: i, ord: n + 1, of: len(todo)})
			w := &lineWriter{send: p.Send}
			began := time.Now()
			err := run(stages[i].ID, w)
			w.flush()
			p.Send(pilotEndMsg{idx: i, err: err, dur: time.Since(began), stopped: m.host.stop.Load(), skipped: m.host.skip.Swap(false)})
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

// windowTitle is the terminal's title while the run goes on: the run is
// often left in a tab of its own, and the title says how it is going
// without opening it. The last one stays when the screen closes, so the
// tab still says whether the run went well.
func (m *pilotModel) windowTitle() string {
	total, settled := m.progress()
	count := fmt.Sprintf("%d/%d", settled, total)
	switch {
	case total > 0 && settled == total:
		mark := gl.Check
		for _, r := range m.res {
			if r.Status == StageFailed {
				mark = gl.Cross
			}
		}
		return gl.Brand + " omega " + mark + " " + count
	case m.cur >= 0:
		return gl.Brand + " omega " + count + " " + gl.Sep + " " + m.stages[m.cur].Title
	}
	return gl.Brand + " omega " + count
}

func (m *pilotModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	acts := len(m.acts)
	model, cmd := m.update(msg)
	if m.scroll > 0 && len(m.acts) > acts {
		// Scrolled back: what is on the screen stays where it is.
		m.scroll += len(m.acts) - acts
	}
	if t := m.windowTitle(); t != m.title {
		m.title = t
		cmd = tea.Batch(cmd, tea.SetWindowTitle(t))
	}
	return model, cmd
}

func (m *pilotModel) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case tickMsg:
		m.frame++
		if m.oops > 0 {
			m.oops--
		}
		if m.cheer > 0 {
			m.cheer--
		}
		return m, doTick()

	case tea.KeyMsg:
		switch msg.String() {
		case "s":
			// End the stage that is running and go on with the next. A
			// stage without a session (a report, the built-in steps) has
			// nothing to stop.
			if m.cur >= 0 && m.cancel != nil && !m.stopping && !m.skipping {
				m.skipping = true
				m.host.skip.Store(true)
				m.cancel()
			}
			return m, nil
		case "up", "k":
			m.scroll++
			return m, nil
		case "down", "j":
			m.scroll = max(0, m.scroll-1)
			return m, nil
		case "pgup":
			m.scroll += max(1, m.height/2)
			return m, nil
		case "pgdown":
			m.scroll = max(0, m.scroll-max(1, m.height/2))
			return m, nil
		case "end", "G":
			m.scroll = 0
			return m, nil
		case "ctrl+c":
		default:
			return m, nil
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
		m.lastOut, m.thinking, m.cancel, m.skipping = "", false, nil, false
		m.header = Header{}
		m.say = m.stages[i].What
		m.acts = append(m.acts, pilotAct{kind: actStage, stage: i, name: m.stages[i].Title})

	case pilotAttachMsg:
		m.cancel, m.header = msg.cancel, msg.header
		if m.cur >= 0 {
			m.st[m.cur].log = msg.log
		}
		if m.stopping || m.skipping {
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
		case msg.skipped:
			r.Status, r.Note = StageStopped, i18n.T("pilot.skipped_note")
		case msg.err != nil:
			r.Status, r.Note = StageFailed, strings.TrimSpace(msg.err.Error())
			m.oops = 20
		default:
			r.Status = StageDone
			m.cheer = 18
		}
		for j := range m.acts {
			// The session ended under a command: it did not finish.
			if a := &m.acts[j]; a.kind == actRunning {
				a.kind, a.dur = actFailed, time.Since(a.began)
			}
		}
		m.cur, m.thinking, m.cancel, m.lastOut, m.skipping = -1, false, nil, "", false

	case pilotFinishMsg:
		m.finished, m.cur, m.cheer, m.oops = true, -1, 0, 0
		m.say = m.verdict()
		if reducedMotion() {
			return m, tea.Quit
		}
		return m, tea.Tick(finale, func(time.Time) tea.Msg { return pilotQuitMsg{} })

	case pilotQuitMsg:
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
		m.acts = append(m.acts, pilotAct{kind: actNote, stage: i, name: gl.Up + " " + i18n.T("ui.model_switch", e.Text)})
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

// verdict is how the run went, in a sentence.
func (m *pilotModel) verdict() string {
	failed, ran := 0, 0
	for i, r := range m.results() {
		if !m.stages[i].Selected {
			continue
		}
		switch r.Status {
		case StageFailed:
			failed++
			ran++
		case StageDone, StageStopped:
			ran++
		}
	}
	switch {
	case m.stopping:
		return i18n.T("pilot.bye_stopped")
	case failed > 0:
		return i18n.T("pilot.bye_failed", failed, ran)
	}
	return i18n.T("pilot.bye_ok")
}

// eta is about how long the stages left will take, from how long the ones
// that ran took; false until a stage has ended.
func (m *pilotModel) eta() (time.Duration, bool) {
	var sum time.Duration
	ended, left := 0, 0
	for i, s := range m.stages {
		if !s.Selected {
			continue
		}
		switch r := m.res[i]; r.Status {
		case StageDone, StageFailed:
			sum += r.Duration
			ended++
		case StagePending:
			left++
		}
	}
	if ended == 0 || (left == 0 && m.cur < 0) {
		return 0, false
	}
	avg := sum / time.Duration(ended)
	est := avg * time.Duration(left)
	if m.cur >= 0 {
		est += max(0, avg-time.Since(m.st[m.cur].began))
	}
	return est, true
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
