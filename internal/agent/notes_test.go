package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/devtools"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/pending"
)

var devtoolsMac = devtools.Platform{OS: "darwin", Brew: true, Shell: "zsh"}

// writeNotes replaces the notes file of the sandbox.
func writeNotes(t *testing.T, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(config.NotesPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.NotesPath(), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// D6 of the fifth review: a run gets the newest notes, newest first, up to
// a size, and is told how many older ones were left out.
func TestPromptNotesKeepsTheNewest(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 100; i++ {
		b.WriteString("- 2026-01-01: note number " + strings.Repeat("x", 60) + string(rune('A'+i%26)) + "\n")
	}
	b.WriteString("- 2026-02-01: the newest note\n")
	got := promptNotes(b.String(), 1024)
	if len(got) > 1024+120 {
		t.Fatalf("%d bytes, more than the limit", len(got))
	}
	if !strings.HasPrefix(got, "- 2026-02-01: the newest note\n") {
		t.Fatalf("the newest note must come first: %q", got[:60])
	}
	if !strings.Contains(got, "older notes are not shown here") {
		t.Fatalf("the run must be told what was left out: %q", got)
	}
	if short := promptNotes("- a\n- b\n", 1024); short != "- b\n- a\n" {
		t.Fatalf("short notes: %q", short)
	}
}

// D6: a note equal to one already kept (date, case, spacing aside) is not
// saved again.
func TestRememberSkipsAnEqualNote(t *testing.T) {
	sandbox(t)
	writeNotes(t, "- 2026-01-01: Worktree agents-v3.7 pendiente.\n")
	res, _, _ := drive(t, Options{}, Approve,
		call("1", "remember", map[string]any{"note": "2026-03-04: worktree   agents-v3.7 PENDIENTE"}),
		call("2", "remember", map[string]any{"note": "something new"}))
	if !strings.Contains(res[0].Content, "already kept") {
		t.Fatalf("an equal note was saved again: %+v", res[0])
	}
	if b, _ := os.ReadFile(config.NotesPath()); strings.Count(string(b), "\n") != 2 || !strings.Contains(string(b), "something new") {
		t.Fatalf("notes: %q", b)
	}
}

// M4: a note is measured in characters, not bytes.
func TestNoteLimitCountsCharacters(t *testing.T) {
	sandbox(t)
	res, _, _ := drive(t, Options{}, Approve,
		call("1", "remember", map[string]any{"note": strings.Repeat("ñ", maxNote)}),
		call("2", "remember", map[string]any{"note": strings.Repeat("ñ", maxNote+1)}))
	if res[0].IsError {
		t.Fatalf("%d characters must fit: %+v", maxNote, res[0])
	}
	if !res[1].IsError || !strings.Contains(res[1].Content, "this one has 401") {
		t.Fatalf("over the limit: %+v", res[1])
	}
}

// D9: a dry run saves no note and records no pending item.
func TestDryRunPersistsNothing(t *testing.T) {
	sandbox(t)
	res, _, _ := drive(t, Options{ReadOnly: true, DryRun: true}, Approve,
		call("1", "remember", map[string]any{"note": "a fact"}),
		call("2", "pending", map[string]any{"action": "add", "id": "tm", "text": "set up Time Machine"}),
		call("3", "pending", map[string]any{"action": "list"}))
	if !strings.Contains(res[0].Content, "read-only") || !strings.Contains(res[1].Content, "read-only") {
		t.Fatalf("the model must be told nothing was saved: %+v %+v", res[0], res[1])
	}
	if _, err := os.Stat(config.NotesPath()); err == nil {
		t.Fatal("a dry run wrote a note")
	}
	if items, _ := pending.Load(); len(items) != 0 {
		t.Fatalf("a dry run recorded %v", items)
	}
	if !strings.Contains(res[2].Content, "No pending items") {
		t.Fatalf("list still works: %+v", res[2])
	}
}

// D10: an earlier transcript and what curl downloaded are as untrusted as
// a page: a note after them needs a yes.
func TestUntrustedSourcesTaintTheSession(t *testing.T) {
	for name, first := range map[string]llm.ToolCall{
		"run_history": call("1", "run_history", map[string]any{}),
		"curl":        call("1", "run_command", map[string]any{"command": "curl -s https://example.com/x", "reason": "r"}),
	} {
		sandbox(t)
		_, _, asked := drive(t, Options{AutoYes: true}, Approve, first, call("2", "remember", map[string]any{"note": "fact from " + name}))
		// --yes lets curl through without a question; the note is asked.
		if asked != 1 {
			t.Errorf("%s: the note was not put to the user (asked %d)", name, asked)
		}
	}
	for _, cmd := range []string{"curl -s https://x", "sudo wget -qO- https://x", "gh api repos/x/y", "Invoke-WebRequest https://x"} {
		if !fetchesNetwork(cmd) {
			t.Errorf("%q fetches from the network", cmd)
		}
	}
	if fetchesNetwork("brew update") {
		t.Error("brew update is not a download the model reads")
	}
}
