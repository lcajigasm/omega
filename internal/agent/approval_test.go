package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/devtools"
	"github.com/lcajigasm/omega/internal/safety"
	"github.com/lcajigasm/omega/internal/steps"
)

// A7 and A8 of the fifth review: what changes the shell's startup file,
// omega's settings or its notes is asked every time, also under --yes,
// and refused in a run nobody watches.
func TestLastingChangesAlwaysNeedAPerson(t *testing.T) {
	sandbox(t)
	old := addInit
	addInit = func(tools []devtools.Tool, _ time.Time) ([]devtools.Tool, string, error) { return tools, "", nil }
	t.Cleanup(func() { addInit = old })
	writeNotes(t, "- 2026-01-01: brew needs a token\n- 2026-01-02: docker is off\n")

	calls := map[string]map[string]any{
		"dev_activate":   {"tools": []string{"fzf"}},
		"omega_settings": {"action": "set", "key": "language", "value": "es"},
		"notes":          {"action": "forget", "match": "docker"},
	}
	for tool, in := range calls {
		res, _, asked := drive(t, Options{AutoYes: true, Unattended: true}, Approve, call("1", tool, in))
		if asked != 0 || !res[0].IsError || !strings.Contains(res[0].Content, "Unattended run") {
			t.Errorf("%s unattended: asked %d, result %+v; it must be refused", tool, asked, res[0])
		}
		res, _, asked = drive(t, Options{AutoYes: true}, Approve, call("1", tool, in))
		if asked != 1 || res[0].IsError {
			t.Errorf("%s under --yes: asked %d times, result %+v; it must be asked every time", tool, asked, res[0])
		}
	}
}

// A9: a catalogue step whose command the policy always asks about is
// asked about, whatever the step declares.
func TestRunStepKeepsTheCommandsAlwaysConfirm(t *testing.T) {
	sandbox(t)
	var step *steps.Step
	for i, s := range steps.All {
		if !s.AlwaysConfirm && safety.AlwaysConfirm(s.Command) && safety.Classify(s.Command) != safety.Blocked {
			step = &steps.All[i]
			break
		}
	}
	if step == nil {
		t.Skip("no step on this system relies on its command for always-confirm")
	}
	res, ran, asked := drive(t, Options{AutoYes: true, Unattended: true, SkipDetect: true}, Approve, call("1", "run_step", map[string]any{"id": step.ID}))
	if asked != 0 || len(ran) != 0 || !res[0].IsError || !strings.Contains(res[0].Content, "Unattended run") {
		t.Fatalf("%s ran unattended: asked %d, ran %v, %+v", step.ID, asked, ran, res[0])
	}
}

// A10: an unattended run takes only what the model recommended and said
// only reads or makes a routine change.
func TestUnattendedTakesOnlyRecommendedRoutineOptions(t *testing.T) {
	s := runScript(t, Options{Unattended: true}, Deny, nil, nil,
		proposal(true,
			opt("cache", "Prune the build cache", "normal", true),
			opt("look", "Look at the logs", "read-only", true),
			opt("norisk", "Something without a risk", "", true),
			opt("later", "Not recommended", "normal", false),
			opt("wipe", "Delete the volumes", "dangerous", true)))
	got := s.p.results[1][0].Content
	for _, want := range []string{"- cache:", "- look:"} {
		if !strings.Contains(got, want) {
			t.Errorf("%s should be taken: %q", want, got)
		}
	}
	for _, not := range []string{"- norisk:", "- later:", "- wipe:"} {
		if strings.Contains(got, not) {
			t.Errorf("%s must not be taken: %q", not, got)
		}
	}
}

// A10: removing an app, turning off a login item or resetting every
// permission is refused in a run nobody watches, before anything is read.
func TestMacChangesAPersonMustMake(t *testing.T) {
	sandbox(t)
	for _, id := range []string{"app-remove:com.x.app", "disable:com.google.keystone.agent", "perm-reset-all:com.x.app"} {
		a := New(&scriptedProvider{}, Options{Unattended: true, Config: config.Default()})
		t.Cleanup(a.closeLog)
		go func() {
			for range a.events {
			}
		}()
		_, err := toolMacChange(context.Background(), a, map[string]any{"change": id, "reason": "r"})
		if err == nil || !strings.Contains(err.Error(), "Unattended run") {
			t.Errorf("%s: %v", id, err)
		}
	}
}
