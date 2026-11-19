//go:build gui

package main

import (
	"sync"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/gui"
	"github.com/lcajigasm/omega/internal/i18n"
)

// recorder keeps what the app sends to the window.
type recorder struct {
	app  *App // answers the cards of the scripted sessions: none is picked
	mu   sync.Mutex
	msgs []gui.Message
	seen chan gui.Message
}

func newRecorder() *recorder { return &recorder{seen: make(chan gui.Message, 4096)} }

func (r *recorder) emit(m gui.Message) {
	r.mu.Lock()
	r.msgs = append(r.msgs, m)
	r.mu.Unlock()
	r.seen <- m
}

// wait returns the next message of kind, failing after a while.
func (r *recorder) wait(t *testing.T, kind string) gui.Message {
	t.Helper()
	deadline := time.After(60 * time.Second)
	for {
		select {
		case m := <-r.seen:
			if m.Kind == kind {
				return m
			}
			switch m.Kind {
			case "choice":
				go r.app.Choose(m.ID, nil)
			case "confirm":
				go r.app.Confirm(m.ID, "deny")
			case "password":
				go r.app.Password(m.ID, "")
			}
		case <-deadline:
			t.Fatalf("no %q message", kind)
		}
	}
}

func isolate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir+"/config")
	t.Setenv("XDG_STATE_HOME", dir+"/state")
	i18n.Init("en")
}

// A guided run in the window runs the ticked stages one after another,
// asks between two of them, and ends with the summary and one "closed".
func TestFullRunStagesAndChoices(t *testing.T) {
	isolate(t)
	r := newRecorder()
	a := &App{emit: r.emit, demo: true}
	r.app = a
	plan := a.FullPlan()
	if len(plan) < 3 || plan[0].ID != "doctor" || !plan[0].Report {
		t.Fatalf("plan = %+v", plan)
	}
	if _, err := a.StartFull([]string{"doctor", "analyze", "health"}, true); err != nil {
		t.Fatal(err)
	}
	if !a.Busy() {
		t.Fatal("the app is not busy during a guided run")
	}
	if _, err := a.Start("quick", "", false); err == nil {
		t.Fatal("a session started while a guided run was on")
	}

	// doctor is a report: its lines come as one call.
	if m := r.wait(t, "stage_start"); m.Stage != 1 || m.Text != "Stage 1 of 3" {
		t.Fatalf("first stage_start = %+v", m)
	}
	r.wait(t, "tool_end")
	if m := r.wait(t, "stage_end"); m.Stages[0].Status != "done" && m.Stages[0].Status != "failed" {
		t.Fatalf("doctor ended %q", m.Stages[0].Status)
	}

	// Between two stages the run waits; health is skipped from the plan
	// while it waits, and the choice is asked again.
	if m := r.wait(t, "between"); m.Stage != 2 {
		t.Fatalf("between = %+v", m)
	}
	if err := a.FullToggle("health", false); err != nil {
		t.Fatal(err)
	}
	r.wait(t, "between")
	if err := a.FullNext("go"); err != nil {
		t.Fatal(err)
	}
	if m := r.wait(t, "stage_start"); m.Stage != 2 || m.Text != "Stage 2 of 2" {
		t.Fatalf("second stage_start = %+v", m)
	}
	r.wait(t, "session")
	end := r.wait(t, "stage_end")
	if s := end.Stages[1]; s.Status != "done" {
		t.Fatalf("analyze ended %q (%s)", s.Status, s.Note)
	}
	sum := r.wait(t, "summary")
	if sum.Stages[2].Selected || sum.Stages[2].Status != "pending" {
		t.Fatalf("health in the summary = %+v", sum.Stages[2])
	}
	r.wait(t, "closed")
	time.Sleep(50 * time.Millisecond)
	if a.Busy() {
		t.Fatal("the app is still busy after the run")
	}
	closed := 0
	r.mu.Lock()
	for _, m := range r.msgs {
		if m.Kind == "closed" {
			closed++
		}
	}
	r.mu.Unlock()
	if closed != 1 {
		t.Fatalf("%d closed messages, want 1: a stage's own end must not end the run", closed)
	}
	// A window drawn again gets the whole run.
	if s := a.Session(); s.Running || len(s.Messages) == 0 || s.Info.Task != "full" {
		t.Fatalf("session after the run: running=%v, %d messages, task %q", s.Running, len(s.Messages), s.Info.Task)
	}
}

// Stop ends the running stage, not the run; "end" between two stages ends it.
func TestFullRunStopStageThenEnd(t *testing.T) {
	isolate(t)
	r := newRecorder()
	a := &App{emit: r.emit, demo: true}
	r.app = a
	if _, err := a.StartFull([]string{"analyze", "health"}, true); err != nil {
		t.Fatal(err)
	}
	r.wait(t, "stage_start")
	r.wait(t, "session")
	a.Stop()
	if m := r.wait(t, "stage_end"); m.Stages[m.Stage-1].Status != "stopped" {
		t.Fatalf("stopped stage = %+v", m.Stages[m.Stage-1])
	}
	if m := r.wait(t, "between"); !m.Retry {
		t.Fatal("a stopped stage may be repeated")
	}
	if err := a.FullNext("end"); err != nil {
		t.Fatal(err)
	}
	r.wait(t, "summary")
	r.wait(t, "closed")
}

func TestFullRunNeedsAStage(t *testing.T) {
	isolate(t)
	a := &App{emit: func(gui.Message) {}, demo: true}
	if _, err := a.StartFull([]string{"nothing-here"}, false); err == nil {
		t.Fatal("a run with no stage started")
	}
	if a.Busy() {
		t.Fatal("busy after a run that did not start")
	}
}
