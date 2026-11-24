package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/lcajigasm/omega/internal/llm"
)

// tokenChat answers with a big usage every turn, and never finishes.
type tokenChat struct{ sends int }

func (c *tokenChat) Send(context.Context, string, []llm.ToolResult) (*llm.Reply, error) {
	c.sends++
	return &llm.Reply{Text: "still working", Usage: llm.Usage{InputTokens: 100_000, OutputTokens: 10_000}}, nil
}
func (c *tokenChat) SetModel(string) {}

type tokenProvider struct{ chat *tokenChat }

func (p tokenProvider) Name() string                                    { return "test" }
func (p tokenProvider) Models(context.Context) ([]string, error)        { return nil, nil }
func (p tokenProvider) NewChat(string, string, []llm.ToolSpec) llm.Chat { return p.chat }

// --max-cost holds for a model omega has no price for: it counts at a
// cautious price, warns once, and stops.
func TestMaxCostHoldsWithUnknownPrice(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	opts := exitOpts(true)
	opts.Model = "some-unpriced-model"
	opts.MaxCost = 1
	chat := &tokenChat{}
	a := New(tokenProvider{chat}, opts)
	go a.Run(context.Background(), "go")
	var warned, stopped int
	for e := range a.Events() {
		switch e.Kind {
		case EvError:
			if strings.Contains(e.Text, "some-unpriced-model") {
				warned++
			}
		case EvDone:
			if strings.Contains(e.Text, "$1.00") {
				stopped++
			}
		case EvIdle:
			a.Input() <- "continue"
		}
	}
	if warned != 1 || stopped != 1 {
		t.Fatalf("warned %d times, stopped %d times; want 1 and 1", warned, stopped)
	}
	// 100k in + 10k out at $10/$50 per million is $1.50: the first turn stops it.
	if chat.sends != 1 {
		t.Fatalf("%d turns ran", chat.sends)
	}
}
