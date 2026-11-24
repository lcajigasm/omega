package ui

import (
	"bufio"
	"context"
	"errors"
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
