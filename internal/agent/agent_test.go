package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/llm"
)

// scriptedProvider replays a fixed list of replies and records tool results.
type scriptedProvider struct {
	replies []*llm.Reply
	results [][]llm.ToolResult
}

func (p *scriptedProvider) Name() string                                    { return "test" }
func (p *scriptedProvider) Models(context.Context) ([]string, error)        { return nil, nil }
func (p *scriptedProvider) NewChat(string, string, []llm.ToolSpec) llm.Chat { return p }

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
	if r := p.results[1][0]; !r.IsError || !strings.Contains(r.Content, "rechazado") {
		t.Fatalf("expected a denial result, got %+v", r)
	}
}

func TestReadOnlyRejectsUpdates(t *testing.T) {
	p := &scriptedProvider{replies: []*llm.Reply{
		{Calls: []llm.ToolCall{call("1", "run_command", map[string]any{"command": "brew upgrade", "reason": "r"})}},
		{Text: "ok"},
	}}
	run(t, p, Options{ReadOnly: true, AutoYes: true}, Approve)
	if r := p.results[1][0]; !r.IsError || !strings.Contains(r.Content, "solo lectura") {
		t.Fatalf("expected read-only refusal, got %+v", r)
	}
}

func TestBlockedEvenWithYes(t *testing.T) {
	p := &scriptedProvider{replies: []*llm.Reply{
		{Calls: []llm.ToolCall{call("1", "run_command", map[string]any{"command": "rm -rf ~", "reason": "r"})}},
		{Text: "ok"},
	}}
	run(t, p, Options{AutoYes: true}, Approve)
	if r := p.results[1][0]; !r.IsError || !strings.Contains(r.Content, "bloqueado") {
		t.Fatalf("expected blocked, got %+v", r)
	}
}
