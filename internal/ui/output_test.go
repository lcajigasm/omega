package ui

import (
	"strings"
	"testing"

	"github.com/lcajigasm/omega/internal/agent"
)

func TestOutputFollowsItsOwnTool(t *testing.T) {
	m := testModel(t)
	m.handleEvent(agent.Event{Kind: agent.EvToolStart, Tool: "run_command", Command: "echo one"})
	m.handleEvent(agent.Event{Kind: agent.EvToolStart, Tool: "run_command", Command: "echo two"})
	m.handleEvent(agent.Event{Kind: agent.EvOutput, Tool: "run_command", Command: "echo two", Text: "TWO"})
	m.handleEvent(agent.Event{Kind: agent.EvOutput, Tool: "run_command", Command: "echo one", Text: "ONE"})
	m.handleEvent(agent.Event{Kind: agent.EvToolEnd, Tool: "run_command", Command: "echo two"})
	m.handleEvent(agent.Event{Kind: agent.EvToolEnd, Tool: "run_command", Command: "echo one"})

	var one, two *entry
	for _, e := range m.entries {
		if e.kind != enTool {
			continue
		}
		switch e.command {
		case "echo one":
			one = e
		case "echo two":
			two = e
		}
	}
	if one == nil || two == nil {
		t.Fatal("missing tool cards")
	}
	if strings.Join(one.output, "") != "ONE" || strings.Join(two.output, "") != "TWO" {
		t.Fatalf("output crossed cards: one=%q two=%q", one.output, two.output)
	}
	if one.running || two.running || m.current != nil {
		t.Fatal("cards still marked running")
	}
}
