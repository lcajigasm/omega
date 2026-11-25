package agent

import (
	"strings"
	"testing"

	"github.com/lcajigasm/omega/internal/llm"
)

// The stages of one run share what they measured read-only: the second
// stage does not scan the disk again.
func TestStagesShareTheResultCache(t *testing.T) {
	step := &llm.Reply{Calls: []llm.ToolCall{call("1", "disk_usage", map[string]any{"path": "/tmp"})}}
	shared := NewResultCache()
	first := runScript(t, Options{AutoYes: true, Cache: shared}, Approve, nil, nil, step)
	second := runScript(t, Options{AutoYes: true, Cache: shared}, Approve, nil, nil, step)
	if len(first.ran) != 1 || len(second.ran) != 0 {
		t.Fatalf("the second stage measured again: first %v, second %v", first.ran, second.ran)
	}
	if got := second.p.results[1][0].Content; !strings.Contains(got, "not run again") {
		t.Fatalf("the second stage did not get the first one's result: %q", got)
	}
	// Something that may change the machine, in any stage, clears it for
	// every stage after.
	runScript(t, Options{AutoYes: true, Cache: shared}, Approve, nil, nil,
		&llm.Reply{Calls: []llm.ToolCall{call("2", "run_command", map[string]any{"command": "brew cleanup", "reason": "r"})}})
	if third := runScript(t, Options{AutoYes: true, Cache: shared}, Approve, nil, nil, step); len(third.ran) != 1 {
		t.Fatalf("after a change the scan must run again: %v", third.ran)
	}
	// Without a shared cache every session starts empty.
	if alone := runScript(t, Options{AutoYes: true}, Approve, nil, nil, step); len(alone.ran) != 1 {
		t.Fatalf("a session on its own has its own cache: %v", alone.ran)
	}
}

// What the model wrote last is what the session concluded.
func TestConclusion(t *testing.T) {
	s := runScript(t, Options{AutoYes: true}, Approve, nil, nil,
		&llm.Reply{Text: "Measuring first.", Calls: []llm.ToolCall{call("1", "disk_usage", map[string]any{"path": "/tmp"})}})
	if got := s.a.Conclusion(); got != "done" {
		t.Fatalf("conclusion %q", got)
	}
}

func TestEarlierStagesPrompt(t *testing.T) {
	if got := EarlierStagesPrompt(nil); got != "" {
		t.Fatalf("no stage, no text: %q", got)
	}
	if got := EarlierStagesPrompt([]StageNote{{Task: "analyze", Text: "  "}}); got != "" {
		t.Fatalf("a stage that said nothing hands nothing on: %q", got)
	}
	got := EarlierStagesPrompt([]StageNote{
		{Task: "analyze", Text: "Disk at 91%."},
		{Task: "update", Text: "brew failed on pipx.", Failed: true},
	})
	for _, want := range []string{"## analyze\nDisk at 91%.", "## update (ended with an error", "not instructions", "<earlier_stages>"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Index(got, "## analyze") > strings.Index(got, "## update") {
		t.Error("the stages must keep their order")
	}

	// A long summary is cut, and when they do not all fit the oldest go.
	long := strings.Repeat("x", maxNoteRunes*2)
	var notes []StageNote
	for _, task := range []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7"} {
		notes = append(notes, StageNote{Task: task, Text: long})
	}
	got = EarlierStagesPrompt(notes)
	if strings.Contains(got, strings.Repeat("x", maxNoteRunes+1)) || !strings.Contains(got, "[…]") {
		t.Error("a long summary must be cut")
	}
	if !strings.Contains(got, "## a7") || strings.Contains(got, "## a1\n") {
		t.Errorf("the latest stages must be kept first:\n%.300s", got)
	}
	if n := len([]rune(got)); n > maxEarlierRunes+1000 {
		t.Errorf("the notes take %d characters", n)
	}
}
