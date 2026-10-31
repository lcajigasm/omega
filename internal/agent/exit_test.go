package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/runner"
)

type failChat struct {
	calls int
	err   error
}

func (f *failChat) Send(context.Context, string, []llm.ToolResult) (*llm.Reply, error) {
	f.calls++
	if f.err != nil && f.calls == 1 {
		return nil, f.err
	}
	return &llm.Reply{Text: "ok"}, nil
}

func (f *failChat) SetModel(string) {}

type failProvider struct{ chat *failChat }

func (p failProvider) Name() string                             { return "test" }
func (p failProvider) Models(context.Context) ([]string, error) { return nil, nil }
func (p failProvider) NewChat(string, string, []llm.ToolSpec) llm.Chat {
	return p.chat
}

func exitOpts(interactive bool) Options {
	return Options{
		Interactive: interactive,
		Config:      config.Default(),
		Timeout:     time.Second,
		Approval:    config.ApproveDangerous,
		Exec: func(context.Context, string, time.Duration, func(string)) runner.Result {
			return runner.Result{}
		},
	}
}

func TestModelErrorExitsNonZero(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New(failProvider{chat: &failChat{err: errors.New("boom")}}, exitOpts(false))
	a.Run(context.Background(), "test")
	if a.ExitCode() != ExitError {
		t.Fatalf("exit %d, want %d", a.ExitCode(), ExitError)
	}
}

func TestModelErrorRecoveredExitsZero(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New(failProvider{chat: &failChat{err: errors.New("boom")}}, exitOpts(true))
	go a.Run(context.Background(), "test")
	idle := 0
	for e := range a.Events() {
		if e.Kind != EvIdle {
			continue
		}
		idle++
		if idle == 1 {
			a.Input() <- "retry"
			continue
		}
		a.Input() <- ""
	}
	if a.ExitCode() != ExitOK {
		t.Fatalf("exit %d, want 0 after a recovered turn", a.ExitCode())
	}
}

func TestCancelExits130(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New(failProvider{chat: &failChat{}}, exitOpts(false))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a.Run(ctx, "test")
	if a.ExitCode() != ExitCancelled {
		t.Fatalf("exit %d, want %d", a.ExitCode(), ExitCancelled)
	}
}
