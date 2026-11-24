package agent

import (
	"context"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/llm"
)

// slowChat blocks its first reply until the turn is cancelled, then answers.
type slowChat struct {
	sends   int
	started chan struct{}
}

func (c *slowChat) Send(ctx context.Context, text string, _ []llm.ToolResult) (*llm.Reply, error) {
	c.sends++
	if c.sends == 1 {
		close(c.started)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return &llm.Reply{Text: "done: " + text}, nil
}
func (c *slowChat) SetModel(string) {}

type slowProvider struct{ chat *slowChat }

func (p slowProvider) Name() string                                    { return "test" }
func (p slowProvider) Models(context.Context) ([]string, error)        { return nil, nil }
func (p slowProvider) NewChat(string, string, []llm.ToolSpec) llm.Chat { return p.chat }

// ctrl+c in the session stops the turn, not the session: the agent waits for
// the user and goes on with what they type.
func TestCancelTurnKeepsTheSession(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	chat := &slowChat{started: make(chan struct{})}
	a := New(slowProvider{chat}, exitOpts(true))
	if a.CancelTurn() {
		t.Fatal("CancelTurn with no turn running said it stopped one")
	}
	done := make(chan struct{})
	go func() { a.Run(context.Background(), "first"); close(done) }()
	var idle, answered, ended bool
	go func() {
		<-chat.started
		if !a.CancelTurn() {
			t.Error("CancelTurn did not find the running turn")
		}
		// A second ctrl+c straight after is not another turn to stop: the
		// caller must be free to end the session.
		if a.CancelTurn() {
			t.Error("a second CancelTurn stopped the same turn again")
		}
	}()
	timeout := time.After(5 * time.Second)
	for !ended {
		select {
		case e, ok := <-a.Events():
			if !ok {
				ended = true
				break
			}
			switch e.Kind {
			case EvIdle:
				if !idle {
					idle = true
					a.Input() <- "second"
				} else {
					a.Input() <- ""
				}
			case EvAssistant:
				if e.Text == "done: second" {
					answered = true
				}
			case EvConfirm:
				e.Reply <- Deny
			}
		case <-timeout:
			t.Fatal("the session did not go on after the turn was cancelled")
		}
	}
	<-done
	if !idle || !answered {
		t.Fatalf("idle=%v answered=%v: the session should wait and then answer the next message", idle, answered)
	}
	if a.ExitCode() == ExitCancelled {
		t.Fatal("a cancelled turn ended the run as cancelled")
	}
}
