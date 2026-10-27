package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/runner"
)

// scriptedProvider replays a fixed list of replies and records tool results.
type scriptedProvider struct {
	replies []*llm.Reply
	results [][]llm.ToolResult
}

func (p *scriptedProvider) Name() string                                    { return "test" }
func (p *scriptedProvider) Models(context.Context) ([]string, error)        { return nil, nil }
func (p *scriptedProvider) NewChat(string, string, []llm.ToolSpec) llm.Chat { return p }

func (p *scriptedProvider) SetModel(string) {}

func (p *scriptedProvider) Send(_ context.Context, _ string, results []llm.ToolResult) (*llm.Reply, error) {
	p.results = append(p.results, append([]llm.ToolResult(nil), results...))
	r := p.replies[0]
	p.replies = p.replies[1:]
	return r, nil
}

func call(id, name string, in map[string]any) llm.ToolCall {
	b, _ := json.Marshal(in)
	return llm.ToolCall{ID: id, Name: name, Input: b}
}

func run(t *testing.T, p *scriptedProvider, opts Options, answer Decision) []Event {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	opts.Config = config.Default()
	opts.Timeout = 10 * time.Second
	if opts.Approval == "" {
		opts.Approval = config.ApproveDangerous
	}
	a := New(p, opts)
	go a.Run(context.Background(), "test")
	var evs []Event
	for e := range a.Events() {
		if e.Kind == EvConfirm {
			e.Reply <- answer
		}
		evs = append(evs, e)
	}
	return evs
}

func TestSafeCommandRunsWithoutConfirmation(t *testing.T) {
	p := &scriptedProvider{replies: []*llm.Reply{
		{Calls: []llm.ToolCall{call("1", "run_command", map[string]any{"command": "echo hola", "reason": "r"})}},
		{Text: "listo"},
	}}
	evs := run(t, p, Options{}, Deny)
	for _, e := range evs {
		if e.Kind == EvConfirm {
			t.Fatal("safe command asked for confirmation")
		}
	}
	got := p.results[1][0]
	if got.IsError || !strings.Contains(got.Content, "hola") || !strings.Contains(got.Content, "exit_code: 0") {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestDangerousCommandDenied(t *testing.T) {
	p := &scriptedProvider{replies: []*llm.Reply{
		{Calls: []llm.ToolCall{call("1", "run_command", map[string]any{"command": "rm -rf /tmp/omega-test-nope", "reason": "r"})}},
		{Text: "vale"},
	}}
	evs := run(t, p, Options{}, Deny)
	asked := false
	for _, e := range evs {
		asked = asked || e.Kind == EvConfirm
	}
	if !asked {
		t.Fatal("dangerous command did not ask for confirmation")
	}
	if r := p.results[1][0]; !r.IsError || !strings.Contains(r.Content, "denied") {
		t.Fatalf("expected a denial result, got %+v", r)
	}
}

func TestReadOnlyRejectsUpdates(t *testing.T) {
	p := &scriptedProvider{replies: []*llm.Reply{
		{Calls: []llm.ToolCall{call("1", "run_command", map[string]any{"command": "brew upgrade", "reason": "r"})}},
		{Text: "ok"},
	}}
	run(t, p, Options{ReadOnly: true, AutoYes: true}, Approve)
	if r := p.results[1][0]; !r.IsError || !strings.Contains(r.Content, "Read-only") {
		t.Fatalf("expected read-only refusal, got %+v", r)
	}
}

func TestBlockedEvenWithYes(t *testing.T) {
	p := &scriptedProvider{replies: []*llm.Reply{
		{Calls: []llm.ToolCall{call("1", "run_command", map[string]any{"command": "rm -rf ~", "reason": "r"})}},
		{Text: "ok"},
	}}
	run(t, p, Options{AutoYes: true}, Approve)
	if r := p.results[1][0]; !r.IsError || !strings.Contains(r.Content, "Blocked") {
		t.Fatalf("expected blocked, got %+v", r)
	}
}

func TestProposeActionsLiftsReadOnly(t *testing.T) {
	p := &scriptedProvider{replies: []*llm.Reply{
		{Calls: []llm.ToolCall{call("1", "propose_actions", map[string]any{
			"question": "Clean?", "multiple": true,
			"options": []map[string]any{{"id": "a", "label": "A"}, {"id": "b", "label": "B"}},
		})}},
		{Calls: []llm.ToolCall{call("2", "run_command", map[string]any{"command": "touch /dev/null", "reason": "r"})}},
		{Text: "done"},
	}}
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New(p, Options{ReadOnly: true, Interactive: true, Config: config.Default(), Approval: config.ApproveDangerous, Timeout: 5 * time.Second})
	go a.Run(context.Background(), "test")
	sawMode := false
	for e := range a.Events() {
		switch e.Kind {
		case EvChoice:
			e.Picks <- []string{"b"}
		case EvMode:
			sawMode = true
		case EvConfirm:
			e.Reply <- Approve
		case EvIdle:
			a.Input() <- ""
		}
	}
	if !sawMode {
		t.Fatal("read-only mode was not lifted after a selection")
	}
	if got := p.results[1][0].Content; !strings.Contains(got, "- b: B") {
		t.Fatalf("selection not reported: %q", got)
	}
	if r := p.results[2][0]; r.IsError {
		t.Fatalf("command after selection was rejected: %+v", r)
	}
}

func TestDockerPruneAlwaysAsks(t *testing.T) {
	p := &scriptedProvider{replies: []*llm.Reply{
		{Calls: []llm.ToolCall{call("1", "run_command", map[string]any{"command": "docker volume prune -f", "reason": "r"})}},
		{Text: "ok"},
	}}
	asked := false
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New(p, Options{AutoYes: true, Config: config.Default(), Timeout: 5 * time.Second,
		Exec: func(context.Context, string, time.Duration, func(string)) runner.Result { return runner.Result{} }})
	go a.Run(context.Background(), "test")
	for e := range a.Events() {
		if e.Kind == EvConfirm {
			asked = e.Always
			e.Reply <- Deny
		}
	}
	if !asked {
		t.Fatal("docker prune ran without an always-confirm question under --yes")
	}
}

type modelRecorder struct {
	scriptedProvider
	models []string
}

func (m *modelRecorder) NewChat(string, string, []llm.ToolSpec) llm.Chat { return m }
func (m *modelRecorder) SetModel(s string)                               { m.models = append(m.models, s) }

func TestEscalatesOnStepFailure(t *testing.T) {
	p := &modelRecorder{scriptedProvider: scriptedProvider{replies: []*llm.Reply{
		{Calls: []llm.ToolCall{call("1", "run_step", map[string]any{"id": "npm"})}},
		{Text: "ok"},
	}}}
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New(p, Options{Model: "fast", Escalate: "smart", AutoYes: true, SkipDetect: true, Config: config.Default(), Timeout: 5 * time.Second,
		Exec: func(context.Context, string, time.Duration, func(string)) runner.Result {
			return runner.Result{ExitCode: 1}
		}})
	go a.Run(context.Background(), "test")
	for range a.Events() {
	}
	if len(p.models) != 1 || p.models[0] != "smart" {
		t.Fatalf("expected one switch to smart, got %v", p.models)
	}
}

func TestFollowUpHints(t *testing.T) {
	for out, want := range map[string]string{
		"Cannot connect to the Docker daemon at unix:///x. Is the docker daemon running?": "docker-start",
		"zsh: command not found: mas":                                      "install",
		"✓ Skipping sudo-required optimizations: admin access not granted": "omega --sudo",
		"all good": "",
	} {
		got := followUpHint(out)
		if (want == "") != (got == "") || !strings.Contains(got, want) {
			t.Errorf("followUpHint(%q) = %q, want it to mention %q", out, got, want)
		}
	}
}

func TestUserChangesApplyBeforeNextTurn(t *testing.T) {
	p := &modelRecorder{}
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	a := New(p, Options{Model: "fast", Escalate: "smart", Config: config.Default()})
	a.SetMode(ModeReadOnly)
	a.SetModel("other")
	if ok, _ := a.authorize(context.Background(), "run_command", "brew upgrade", "", 1, false); ok {
		t.Fatal("read-only mode set by the user must reject changes")
	}
	text := a.applyChanges("hello")
	if !strings.Contains(text, "read-only") || !strings.HasPrefix(text, "hello") {
		t.Fatalf("the model should be told about read-only mode: %q", text)
	}
	if a.model != "other" || len(p.models) != 1 || p.models[0] != "other" {
		t.Fatalf("picked model not applied: %s %v", a.model, p.models)
	}
	a.escalate()
	if a.model != "other" {
		t.Fatal("a model the user picked must not be replaced by escalation")
	}
	a.SetMode(ModeAuto)
	if ok, _ := a.authorize(context.Background(), "run_command", "brew upgrade", "", 2, false); !ok {
		t.Fatal("auto mode should approve without asking")
	}
	if text := a.applyChanges(""); !strings.Contains(text, "switched off read-only") {
		t.Fatalf("the model should be told read-only mode ended: %q", text)
	}
	if text := a.applyChanges("x"); text != "x" {
		t.Fatalf("notes must be sent once: %q", text)
	}
}
