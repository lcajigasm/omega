package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/i18n"
)

// testPilot is an unattended run on a terminal of the given size.
func testPilot(t *testing.T, width, height int) *pilotModel {
	t.Helper()
	i18n.Init("en")
	stages := append([]GuideStage(nil), guideFixture...)
	stages[3].Selected = false
	m := newPilotModel(PilotInfo{Version: "v1.2.3"}, stages)
	m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return m
}

func ev(m *pilotModel, e agent.Event) { m.Update(pilotEventMsg{e}) }

// The screen says, at every moment, what the run is doing, the stage it is
// on and what has run so far.
func TestPilotScreen(t *testing.T) {
	m := testPilot(t, 100, 30)
	if got := screen(m.View(), 100, 30); !strings.Contains(got, "unattended run") || !strings.Contains(got, "0/3") || !strings.Contains(got, "Nothing yet.") {
		t.Fatalf("before the first stage:\n%s", got)
	}

	m.Update(pilotStageMsg{idx: 0, ord: 1, of: 3})
	m.Update(stageOutMsg("omega is set up"))
	m.Update(pilotEndMsg{idx: 0, dur: 2 * time.Second})
	m.Update(pilotStageMsg{idx: 1, ord: 2, of: 3})
	cancelled := false
	m.Update(pilotAttachMsg{cancel: func() { cancelled = true }, header: Header{Model: "claude-haiku-5-5"}, log: "/tmp/run.md"})
	ev(m, agent.Event{Kind: agent.EvThinking})
	if got := screen(m.View(), 100, 30); !strings.Contains(got, "thinking about the next step") {
		t.Errorf("thinking is not shown:\n%s", got)
	}
	ev(m, agent.Event{Kind: agent.EvAssistant, Text: "## Plan\n\nI will **update Homebrew** first, then the `npm` globals."})
	// Two commands side by side, ending in the other order, and a tool
	// that only reports its end.
	ev(m, agent.Event{Kind: agent.EvToolStart, Tool: "bash", Command: "brew update"})
	ev(m, agent.Event{Kind: agent.EvToolStart, Tool: "bash", Command: "softwareupdate -ia"})
	ev(m, agent.Event{Kind: agent.EvToolEnd, Tool: "bash", Command: "softwareupdate -ia", Failed: true, ExitCode: 2})
	ev(m, agent.Event{Kind: agent.EvToolEnd, Tool: "bash", Command: "brew update", Duration: 3 * time.Second})
	ev(m, agent.Event{Kind: agent.EvToolEnd, Tool: "remember", Command: "pipx needs Python 3.13"})
	ev(m, agent.Event{Kind: agent.EvDenied, Tool: "bash", Command: "docker volume prune -f", Reason: "Unattended run: this needs a person. More."})
	ev(m, agent.Event{Kind: agent.EvToolStart, Tool: "bash", Command: "brew upgrade"})
	ev(m, agent.Event{Kind: agent.EvOutput, Text: "\x1b[32m==> Upgrading 4 outdated packages\x1b[0m", Stats: agent.Stats{Model: "claude-haiku-5-5", Cost: 0.0123, CostKnown: true}})

	got := screen(m.View(), 100, 30)
	for _, want := range []string{
		"Stage 2 of 3", "Update", "omega update", // the stage it is on
		"Plan I will update Homebrew first, then the npm globals.", // what it says, without the Markdown
		"Bash(brew upgrade)", "⎿ ==> Upgrading 4 outdated packages", // what is running now
		"ROUTE", "1/3", "✓ Check omega", "Clean up", // the route
		"DONE SO FAR", "actions: 2 done · 1 failed · 1 denied", "✓ Remember(pipx needs Python 3.13)", // the summary so far
		"✓ Bash(brew update)", "✗ Bash(softwareupdate -ia)", "exit 2", "⊘ Bash(docker volume prune -f) · Unattended run: this ne…", "denied",
		"── Check omega", "── Update", "$0.0123", "claude-haiku-5-5", "( > < )", `/| Ω |\`, "ctrl+c stops the run",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the screen does not show %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "What runs on the Mac") {
		t.Errorf("a stage left out of the run is on the route:\n%s", got)
	}
	for i, line := range strings.Split(m.View(), "\n") {
		if w := len([]rune(stripANSI(line))); w > 100 {
			t.Errorf("line %d is %d columns wide: %q", i, w, stripANSI(line))
		}
	}
	if n := len(strings.Split(m.View(), "\n")); n > 30 {
		t.Errorf("the view is %d lines high on a terminal of 30", n)
	}

	// ctrl+c stops the session and the run; asked twice, it leaves now.
	if _, cmd := m.Update(key("ctrl+c")); cmd != nil || !cancelled || !m.host.stop.Load() {
		t.Errorf("ctrl+c: cancelled %v, stop %v, cmd %v", cancelled, m.host.stop.Load(), cmd)
	}
	if got := screen(m.View(), 100, 30); !strings.Contains(got, "ctrl+c again leaves now") || !strings.Contains(got, "( - - )") {
		t.Errorf("stopping is not shown:\n%s", got)
	}
	if _, cmd := m.Update(key("ctrl+c")); cmd == nil {
		t.Error("a second ctrl+c must leave")
	}
	if _, cmd := m.Update(key("q")); cmd != nil {
		t.Error("only ctrl+c is a key of this screen")
	}
	m.Update(pilotEndMsg{idx: 1, dur: time.Minute, stopped: true})
	res := m.results()
	if res[0].Status != StageDone || res[1].Status != StageStopped || res[2].Status != StagePending || res[3].Status != StageSkipped {
		t.Errorf("results: %+v", res)
	}
	if !res[1].CostKnown || res[1].Cost != 0.0123 {
		t.Errorf("the cost of the stage is lost: %+v", res[1])
	}
}

// Narrow and short terminals still get a screen that fits.
func TestPilotSmallTerminal(t *testing.T) {
	for _, size := range [][2]int{{60, 24}, {80, 12}, {40, 8}, {200, 50}} {
		m := testPilot(t, size[0], size[1])
		m.Update(pilotStageMsg{idx: 1, ord: 2, of: 3})
		for i := 0; i < 40; i++ {
			ev(m, agent.Event{Kind: agent.EvToolStart, Tool: "bash", Command: fmt.Sprintf("echo %d %s", i, strings.Repeat("long ", 30))})
			ev(m, agent.Event{Kind: agent.EvToolEnd, Tool: "bash", Command: fmt.Sprintf("echo %d %s", i, strings.Repeat("long ", 30))})
		}
		lines := strings.Split(m.View(), "\n")
		if len(lines) > size[1] {
			t.Errorf("%v: %d lines", size, len(lines))
		}
		for _, line := range lines {
			if w := len([]rune(stripANSI(line))); w > max(size[0], 40) {
				t.Errorf("%v: a line is %d columns wide: %q", size, w, stripANSI(line))
			}
		}
		if got := screen(m.View(), size[0], size[1]); !strings.Contains(got, "Update") {
			t.Errorf("%v: the stage is not shown:\n%s", size, got)
		}
	}
}

// The mascot's face follows the run.
func TestMascot(t *testing.T) {
	faces := map[int]string{moodWork: "( o o )", moodOops: "( > < )", moodHappy: "( ^ ^ )", moodSad: "( ; ; )", moodStop: "( - - )"}
	for mood, face := range faces {
		art := mascot(mood, 10)
		if !strings.Contains(strings.Join(art, "\n"), face) {
			t.Errorf("mood %d: %q", mood, art)
		}
		for _, line := range art {
			if len([]rune(line)) != mascotWidth {
				t.Errorf("mood %d: line %q is not %d wide", mood, line, mascotWidth)
			}
		}
	}
	m := testPilot(t, 100, 30)
	m.Update(pilotStageMsg{idx: 0, ord: 1, of: 3})
	m.Update(pilotAttachMsg{cancel: func() {}})
	if m.mood() != moodIdle {
		t.Errorf("mood %d before anything runs", m.mood())
	}
	ev(m, agent.Event{Kind: agent.EvThinking})
	if m.mood() != moodThink {
		t.Errorf("mood %d while thinking", m.mood())
	}
	ev(m, agent.Event{Kind: agent.EvToolStart, Tool: "bash", Command: "true"})
	if m.mood() != moodWork {
		t.Errorf("mood %d while a command runs", m.mood())
	}
	ev(m, agent.Event{Kind: agent.EvToolEnd, Tool: "bash", Command: "true", Failed: true, ExitCode: 1})
	if m.mood() != moodOops {
		t.Errorf("mood %d after a failure", m.mood())
	}
	for i := 0; i < 25; i++ {
		m.Update(tickMsg(time.Now()))
	}
	if m.mood() != moodIdle {
		t.Errorf("mood %d once the failure is old", m.mood())
	}
}

// What is left on the terminal: what each stage concluded and could not do.
func TestPilotReport(t *testing.T) {
	m := testPilot(t, 100, 30)
	m.Update(pilotStageMsg{idx: 0, ord: 1, of: 3})
	m.Update(stageOutMsg("omega is set up"))
	m.Update(pilotEndMsg{idx: 0, dur: 2 * time.Second})
	m.Update(pilotStageMsg{idx: 1, ord: 2, of: 3})
	m.Update(pilotAttachMsg{cancel: func() {}, log: "/tmp/run.md"})
	ev(m, agent.Event{Kind: agent.EvToolStart, Tool: "bash", Command: "softwareupdate -ia"})
	ev(m, agent.Event{Kind: agent.EvToolEnd, Tool: "bash", Command: "softwareupdate -ia", Failed: true, ExitCode: 2})
	ev(m, agent.Event{Kind: agent.EvDenied, Tool: "bash", Command: "docker volume prune -f", Reason: "It needs a person."})
	ev(m, agent.Event{Kind: agent.EvToolStart, Tool: "bash", Command: "sleep 100"})
	ev(m, agent.Event{Kind: agent.EvAssistant, Text: "**Everything** is up to date.\n\n- macOS could not be updated."})
	m.Update(pilotEndMsg{idx: 1, dur: time.Minute, err: errors.New("the model stopped answering")})
	m.Update(pilotStageMsg{idx: 2, ord: 3, of: 3})
	m.Update(pilotEndMsg{idx: 2, dur: time.Second})
	if _, cmd := m.Update(pilotFinishMsg{}); cmd == nil {
		t.Error("the screen must close when the run ends")
	}
	got := stripANSI(strings.Join(m.report(), "\n"))
	for _, want := range []string{
		"━━ Check omega · done", "omega is set up",
		"━━ Update · ended with an error · 1m00s", "Everything is up to date.", "macOS could not be updated.", "! the model stopped answering", "✗ Bash(sleep 100)",
		"✗ Bash(softwareupdate -ia) · exit 2", "⊘ Bash(docker volume prune -f) · denied · It needs a person.", "Run log: /tmp/run.md",
		"━━ Clean up · done", "( ; ; )", "Finished, but 1 of the 3 stages that ran ended with an error.", "actions: 0 done · 2 failed · 1 denied",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the report does not say %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "What runs on the Mac") {
		t.Errorf("a stage that did not run is in the report:\n%s", got)
	}

	ok := testPilot(t, 100, 30)
	ok.Update(pilotStageMsg{idx: 0, ord: 1, of: 1})
	ok.Update(pilotEndMsg{idx: 0})
	ok.Update(pilotFinishMsg{})
	if got := stripANSI(strings.Join(ok.report(), "\n")); !strings.Contains(got, "( ^ ^ )") || !strings.Contains(got, "All done: no stage failed.") {
		t.Errorf("a clean run:\n%s", got)
	}
}

// What prints while the screen is up is read instead of drawn over it.
func TestCaptureOutput(t *testing.T) {
	stdout, stderr := os.Stdout, os.Stderr
	var lines []string
	release := captureOutput(func(l string) { lines = append(lines, l) })
	fmt.Println("to stdout")
	fmt.Fprintln(os.Stderr, "to stderr")
	release()
	release()
	if os.Stdout != stdout || os.Stderr != stderr {
		t.Fatal("the standard output was not put back")
	}
	if got := strings.Join(lines, "|"); got != "to stdout|to stderr" {
		t.Errorf("captured %q", got)
	}
}

// A session on the pilot's screen refuses what would need a person.
func TestPilotSessionWithoutHost(t *testing.T) {
	if Piloted() {
		t.Fatal("no unattended run is on screen")
	}
	done := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		reply(ctx, make(chan agent.Decision, 1), agent.Deny)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("a refusal did not return")
	}
}
