package agent

import (
	"context"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/runner"
)

// D3 of the fifth review: a step that ran out of time or was cancelled
// does not switch to the stronger model.
func TestTimeoutsDoNotEscalate(t *testing.T) {
	for name, res := range map[string]runner.Result{
		"cancelled": {ExitCode: -1},
		"timed out": {ExitCode: 1, TimedOut: true},
	} {
		p := &modelRecorder{scriptedProvider: scriptedProvider{replies: []*llm.Reply{
			{Calls: []llm.ToolCall{call("1", "run_step", map[string]any{"id": "npm"})}},
			{Text: "ok"},
		}}}
		t.Setenv("XDG_STATE_HOME", t.TempDir())
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		a := New(p, Options{Model: "fast", Escalate: "smart", AutoYes: true, SkipDetect: true, Config: config.Default(), Timeout: 5 * time.Second,
			Exec: func(context.Context, string, time.Duration, func(string)) runner.Result { return res }})
		go a.Run(context.Background(), "test")
		for range a.Events() {
		}
		if len(p.models) != 0 {
			t.Errorf("%s: switched to %v", name, p.models)
		}
	}
}

// D3: after a few turns without a failure the run goes back to the model
// it started with.
func TestEscalationReturnsToTheFastModel(t *testing.T) {
	failing := &llm.Reply{Calls: []llm.ToolCall{call("1", "run_step", map[string]any{"id": "npm"})}}
	calm := &llm.Reply{Calls: []llm.ToolCall{call("2", "run_command", map[string]any{"command": "echo ok", "reason": "r"})}}
	p := &modelRecorder{scriptedProvider: scriptedProvider{replies: []*llm.Reply{failing, calm, calm, calm, calm, calm, {Text: "ok"}}}}
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New(p, Options{Model: "fast", Escalate: "smart", AutoYes: true, SkipDetect: true, Config: config.Default(), Timeout: 5 * time.Second,
		Exec: func(_ context.Context, cmd string, _ time.Duration, _ func(string)) runner.Result {
			if cmd == "echo ok" {
				return runner.Result{Output: "ok"}
			}
			return runner.Result{ExitCode: 1, Output: "Error: SHA256 mismatch"}
		}})
	go a.Run(context.Background(), "test")
	for range a.Events() {
	}
	if len(p.models) != 2 || p.models[0] != "smart" || p.models[1] != "fast" {
		t.Fatalf("expected smart then fast after %d calm turns, got %v", calmTurns, p.models)
	}
}
