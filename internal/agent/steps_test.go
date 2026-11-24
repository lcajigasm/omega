package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/safety"
	"github.com/lcajigasm/omega/internal/steps"
)

// D5 of the fifth review: list_steps is short by default, and gives one
// step in full on request.
func TestListStepsIsShortByDefault(t *testing.T) {
	sandbox(t)
	a := New(&scriptedProvider{}, Options{SkipDetect: true, Config: config.Default()})
	t.Cleanup(a.closeLog)
	short, _ := toolListSteps(context.Background(), a, map[string]any{})
	longest := steps.All[0]
	for _, s := range steps.All {
		if len(s.Command) > len(longest.Command) {
			longest = s
		}
	}
	if strings.Contains(short, longest.Command) {
		t.Errorf("the default list must not carry the commands (%s)", longest.ID)
	}
	full, _ := toolListSteps(context.Background(), a, map[string]any{"detail": true})
	if len(short) >= len(full) {
		t.Errorf("default %d bytes, with detail %d", len(short), len(full))
	}
	s := steps.All[0]
	one, err := toolListSteps(context.Background(), a, map[string]any{"id": s.ID})
	if err != nil || !strings.Contains(one, "command: "+s.Command) {
		t.Fatalf("detail of %s: %q %v", s.ID, one, err)
	}
}

// M6: omega's own read-only steps join the parallel batch, although their
// commands read as changes to the policy.
func TestBuiltinSafeStepsAreReadOnlyCalls(t *testing.T) {
	found := false
	for _, s := range steps.All {
		if !steps.Builtin(s.ID) || s.Risk != safety.Safe || s.AlwaysConfirm || safety.Classify(s.Command) == safety.Safe {
			continue
		}
		found = true
		in, _ := json.Marshal(map[string]any{"id": s.ID})
		if !readOnlyCall(llm.ToolCall{Name: "run_step", Input: in}) {
			t.Errorf("%s is declared read-only but cannot join the batch", s.ID)
		}
	}
	if !found {
		t.Skip("no built-in read-only step here reads as a change")
	}
}

// M5: two identical calls of one batch have their own place in the
// transcript.
func TestIdenticalCallsHaveTheirOwnLogKey(t *testing.T) {
	a, b := Event{Tool: "run_command", Command: "ls", call: 1}, Event{Tool: "run_command", Command: "ls", call: 2}
	if callKey(a) == callKey(b) {
		t.Fatal("two calls share a key")
	}
}
