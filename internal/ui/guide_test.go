package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/i18n"
)

var guideFixture = []GuideStage{
	{ID: "doctor", Command: "omega doctor", Title: "Check omega", What: "Checks the setup.", Why: "Everything else needs it.", Risk: "read-only", Selected: true},
	{ID: "update", Command: "omega update", Title: "Update", What: "Updates everything installed.", Why: "Security fixes come this way.", Risk: "normal", Selected: true},
	{ID: "clean", Command: "omega clean", Title: "Clean up", What: "Frees disk space.", Why: "Old versions pile up.", Risk: "dangerous", Selected: true},
	{ID: "mac", Command: "omega mac", Title: "What runs on the Mac", What: "Reviews login items.", Why: "It slows the Mac down.", Risk: "normal", Selected: true},
}

// testGuide is a guided run on a terminal of the given size. What its
// stages do is set per test through run.
func testGuide(t *testing.T, width, height int, run StageRunner) *model {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	i18n.Init("en")
	if run == nil {
		run = func(string, io.Writer) error { return nil }
	}
	m := newGuideModel(GuideInfo{Version: "v1.2.3"}, append([]GuideStage(nil), guideFixture...), run)
	m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return m
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	}
	return runeKey([]rune(s)[0])
}

// press sends keys and returns the command of the last one.
func press(m *model, keys ...string) tea.Cmd {
	var cmd tea.Cmd
	for _, k := range keys {
		_, cmd = m.Update(key(k))
	}
	return cmd
}

// finish runs the command that started a stage, as Bubble Tea would, and
// hands its end to the interface.
func finish(t *testing.T, m *model, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("no stage was started")
	}
	msg := cmd()
	if _, ok := msg.(stageEndMsg); !ok {
		t.Fatalf("the stage ended with %T", msg)
	}
	m.Update(msg)
}

func statuses(m *model) string {
	var out []string
	for _, r := range m.g.results() {
		out = append(out, r.ID+"="+r.Status)
	}
	return strings.Join(out, " ")
}

func TestGuidePlanExplainsAndChooses(t *testing.T) {
	m := testGuide(t, 120, 40, nil)
	view := screen(m.View(), 120, 40)
	for _, want := range []string{"omega full v1.2.3", "Guided run", "Check omega", "omega doctor", "What runs on the Mac",
		"What it does", "Checks the setup.", "Why", "Everything else needs it.", "read-only: it changes nothing", "4 of 4 stages will run", "enter start"} {
		if !strings.Contains(view, want) {
			t.Errorf("the plan lacks %q:\n%s", want, view)
		}
	}
	// The card follows the cursor, and space unticks.
	press(m, "down", "down")
	if view = screen(m.View(), 120, 40); !strings.Contains(view, "Frees disk space.") || !strings.Contains(view, "deletes things") {
		t.Errorf("the card does not describe the highlighted stage:\n%s", view)
	}
	press(m, "space")
	if view = screen(m.View(), 120, 40); !strings.Contains(view, "3 of 4 stages will run") {
		t.Errorf("unticking is not counted:\n%s", view)
	}
	press(m, "a")
	if got := statuses(m); strings.Contains(got, "skipped") {
		t.Errorf("a ticks every stage when one is unticked: %s", got)
	}
	press(m, "a")
	if cmd := press(m, "enter"); cmd != nil || m.state != stPlan {
		t.Error("with nothing ticked, Enter has nothing to start")
	}
	// Leaving the plan runs nothing.
	if cmd := press(m, "q"); cmd == nil || !m.g.aborted {
		t.Error("q on the plan leaves")
	}
}

func TestGuideWalksTheStagesInOneInterface(t *testing.T) {
	var ran []string
	m := testGuide(t, 120, 40, func(id string, out io.Writer) error {
		ran = append(ran, id)
		if id == "doctor" {
			fmt.Fprintln(out, "config: ok")
			fmt.Fprint(out, "key: ok")
		}
		if id == "clean" {
			return errors.New("no API key for anthropic")
		}
		return nil
	})
	var sent []tea.Msg
	m.g.send = func(msg tea.Msg) { sent = append(sent, msg) }

	press(m, "down", "space") // untick update
	cmd := press(m, "enter")
	if m.state != stRunning || m.g.cur != 0 {
		t.Fatalf("Enter starts the first stage: state %v, stage %d", m.state, m.g.cur)
	}
	view := screen(m.View(), 120, 40)
	for _, want := range []string{"Stage 1 of 3 · Check omega", "Checks the setup.", "Everything else needs it.", "On its own", "omega doctor", "ROUTE", "0/3"} {
		if !strings.Contains(view, want) {
			t.Errorf("the running stage lacks %q:\n%s", want, view)
		}
	}
	msg := cmd()
	for _, s := range sent {
		m.Update(s)
	}
	m.Update(msg)

	// Between two stages: how the last one went, what comes next and why.
	if m.state != stBetween || m.g.next != 2 {
		t.Fatalf("after the first stage: state %v, next %d", m.state, m.g.next)
	}
	view = screen(m.View(), 120, 40)
	for _, want := range []string{"config: ok", "key: ok", "✓ Check omega finished in", "Next · Stage 2 of 3 · Clean up", "omega clean",
		"Frees disk space.", "Old versions pile up.", "[enter] go on", "[s] skip it", "1/3", "✓ Check omega", "↷ Update"} {
		if !strings.Contains(view, want) {
			t.Errorf("the card between stages lacks %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "[r] repeat") {
		t.Error("a stage that went well is not offered again")
	}

	// A stage that fails says why, and can be repeated.
	finish(t, m, press(m, "enter"))
	view = screen(m.View(), 120, 40)
	for _, want := range []string{"! no API key for anthropic", "✗ Clean up ended with an error", "[r] repeat the last one", "✗ Clean up", "Next · Stage 3 of 3"} {
		if !strings.Contains(view, want) {
			t.Errorf("after a failure the screen lacks %q:\n%s", want, view)
		}
	}
	finish(t, m, press(m, "r"))
	if got := strings.Join(ran, " "); got != "doctor clean clean" {
		t.Fatalf("ran %s", got)
	}

	// The plan can be changed in the middle: what ran keeps its outcome.
	press(m, "p")
	if m.state != stPlan {
		t.Fatal("p opens the plan")
	}
	m.g.cursor = 0
	press(m, "space") // doctor is done: nothing to toggle
	m.g.cursor = 1
	press(m, "space") // update is ticked again
	press(m, "enter")
	if m.state != stBetween || m.g.next != 1 {
		t.Fatalf("back from the plan: state %v, next %d", m.state, m.g.next)
	}
	if got := statuses(m); !strings.HasPrefix(got, "doctor=done update=pending clean=failed") {
		t.Errorf("after the plan: %s", got)
	}

	// s skips the next stage, q ends the run with the summary on screen.
	press(m, "s")
	if m.g.next != 3 {
		t.Fatalf("s skips to the stage after: next %d", m.g.next)
	}
	press(m, "q")
	if m.state != stSummary {
		t.Fatal("q between stages ends the run")
	}
	view = screen(m.View(), 120, 40)
	for _, want := range []string{"What happened", "✓ Check omega", "done", "↷ Update", "skipped", "✗ Clean up", "→ omega clean", "not run",
		"1 done · 1 failed or stopped · 1 skipped · 1 not run", "enter or q leaves"} {
		if !strings.Contains(view, want) {
			t.Errorf("the summary lacks %q:\n%s", want, view)
		}
	}
	if got := statuses(m); got != "doctor=done update=skipped clean=failed mac=pending" {
		t.Errorf("results: %s", got)
	}
	if cmd := press(m, "enter"); cmd == nil {
		t.Error("Enter on the summary leaves")
	} else if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("Enter on the summary must quit")
	}
}

// A stage with an agent is shown in the same interface: the session is
// attached, its events arrive, and when it ends the next stage is offered.
func TestGuideHostsTheSessionOfAStage(t *testing.T) {
	m := testGuide(t, 120, 40, nil)
	other := testModel(t) // gives two live agents
	ag, old := other.ag, testModel(t).ag
	press(m, "enter")

	// RunTUI, called by the stage, hands the session over and waits.
	sent := make(chan tea.Msg, 1)
	m.g.send = func(msg tea.Msg) { sent <- msg }
	host.Store(m.g)
	defer host.Store(nil)
	if !Hosted() {
		t.Fatal("a guided run on screen hosts the sessions")
	}
	cancelled := false
	ended := make(chan error, 1)
	go func() {
		ended <- RunTUI(context.Background(), func() { cancelled = true }, ag, Header{Provider: "anthropic", Model: "haiku", Models: []string{"haiku"}})
	}()
	m.Update(<-sent)
	if m.ag != ag || m.header.Model != "haiku" || m.header.Task != "doctor" {
		t.Fatalf("the session was not attached: %+v", m.header)
	}
	m.Update(agEvent{ag: ag, e: agent.Event{Kind: agent.EvToolStart, Tool: "run_step:brew", Command: "brew upgrade"}})
	m.Update(agEvent{ag: old, e: agent.Event{Kind: agent.EvAssistant, Text: "late words of an earlier stage"}})
	m.Update(agEvent{ag: ag, e: agent.Event{Kind: agent.EvToolEnd, Tool: "run_step:brew", Command: "brew upgrade", Lines: 3}})
	view := screen(m.View(), 120, 40)
	for _, want := range []string{"Stage 1 of 4", "anthropic · haiku", "Step(brew)", "ROUTE", "SESSION", "ACTIVITY"} {
		if !strings.Contains(view, want) {
			t.Errorf("the hosted session lacks %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "late words") {
		t.Error("an event of an earlier agent reached the screen")
	}
	select {
	case <-ended:
		t.Fatal("the session returned before it ended")
	default:
	}
	// The session ends: RunTUI returns, the interface stays.
	_, cmd := m.Update(agEvent{ag: ag, e: agent.Event{Kind: agent.EvDone}})
	if err := <-ended; err != nil || cancelled {
		t.Fatalf("session end: %v, cancelled %v", err, cancelled)
	}
	if cmd != nil {
		if _, quit := cmd().(tea.QuitMsg); quit {
			t.Fatal("the end of a stage must not close the interface")
		}
	}
	// Keys of a finished session do nothing until the stage reports back.
	if cmd := press(m, "q", "enter"); cmd != nil {
		t.Error("q after a stage's session must not quit the run")
	}
	m.Update(stageEndMsg{idx: 0})
	if m.state != stBetween || m.ag != nil || m.g.res[0].Status != StageDone {
		t.Fatalf("after the session: state %v, %s", m.state, statuses(m))
	}
}

func TestGuideCtrlCStopsTheStageNotTheRun(t *testing.T) {
	m := testGuide(t, 120, 40, nil)
	ag := testModel(t).ag
	press(m, "enter")
	cancelled := false
	done := make(chan struct{})
	m.Update(attachMsg{ag: ag, cancel: func() { cancelled = true }, header: Header{Model: "haiku"}, done: done})
	reply := make(chan agent.Decision, 1)
	m.Update(agEvent{ag: ag, e: agent.Event{Kind: agent.EvConfirm, Tool: "run_command", Command: "rm -rf x", Reply: reply}})
	if cmd := press(m, "ctrl+c"); cmd != nil {
		t.Fatal("ctrl+c in a stage must not quit")
	}
	select {
	case <-done:
	default:
		t.Fatal("ctrl+c did not release the session")
	}
	if !cancelled || <-reply != agent.Deny {
		t.Error("ctrl+c cancels the agent and denies what it was asking")
	}
	m.Update(stageEndMsg{idx: 0, err: errors.New("interrupted")})
	if m.g.res[0].Status != StageStopped || m.state != stBetween {
		t.Fatalf("a stopped stage: %s, state %v", statuses(m), m.state)
	}
	if view := screen(m.View(), 120, 40); !strings.Contains(view, "■ Check omega stopped by you") || !strings.Contains(view, "[r] repeat the last one") {
		t.Errorf("the card after a stop:\n%s", view)
	}
	// Between stages ctrl+c ends the run; on the summary it leaves.
	press(m, "ctrl+c")
	if m.state != stSummary {
		t.Fatal("ctrl+c between stages ends the run")
	}
	if cmd := press(m, "ctrl+c"); cmd == nil {
		t.Error("ctrl+c on the summary leaves")
	}
}

// A narrow terminal has no room for the panel: the route goes on one line
// above the transcript, and the cards still fit.
func TestGuideOnANarrowTerminal(t *testing.T) {
	m := testGuide(t, 80, 24, nil)
	if view := screen(m.View(), 80, 24); !strings.Contains(view, "Check omega") || !strings.Contains(view, "enter start") {
		t.Errorf("the plan on 80 columns:\n%s", view)
	}
	finish(t, m, press(m, "enter"))
	view := screen(m.View(), 80, 24)
	lines := strings.Split(view, "\n")
	if !strings.Contains(lines[0], "omega full") || !strings.Contains(lines[0], "1/4") {
		t.Errorf("the route is not on the first line: %q", lines[0])
	}
	if len(lines) > 24 || !strings.Contains(view, "Next · Stage 2 of 4 · Update") || strings.Contains(view, "ROUTE") {
		t.Errorf("the screen between stages on 80x24 (%d lines):\n%s", len(lines), view)
	}
	for _, l := range lines {
		if len([]rune(l)) > 80 {
			t.Errorf("a line is wider than the terminal: %q", l)
		}
	}
}

func TestStageWriterSendsWholeLines(t *testing.T) {
	var got []string
	w := &lineWriter{send: func(msg tea.Msg) { got = append(got, string(msg.(stageOutMsg))) }}
	fmt.Fprint(w, "one\ntw")
	fmt.Fprint(w, "o\r\nthree")
	w.flush()
	if strings.Join(got, "|") != "one|two|three" {
		t.Errorf("lines: %q", got)
	}
}
