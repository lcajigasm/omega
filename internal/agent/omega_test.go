package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/devtools"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/macos"
	"github.com/lcajigasm/omega/internal/runner"
)

// sandbox points omega's config and state at empty folders, so a test can
// seed them before the agent starts.
func sandbox(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("SHELL", "/bin/zsh")
	i18n.Init("en")
}

// drive runs the scripted calls, answers every approval the same way and
// returns the commands that reached the shell.
func drive(t *testing.T, opts Options, answer Decision, calls ...llm.ToolCall) (results []llm.ToolResult, ran []string, asked int) {
	t.Helper()
	p := &scriptedProvider{}
	for _, c := range calls {
		p.replies = append(p.replies, &llm.Reply{Calls: []llm.ToolCall{c}})
	}
	p.replies = append(p.replies, &llm.Reply{Text: "done"})
	if opts.Config == nil {
		opts.Config = config.Default()
	}
	opts.Timeout = 10 * time.Second
	opts.Approval = config.ApproveDangerous
	opts.Exec = func(_ context.Context, command string, _ time.Duration, _ func(string)) runner.Result {
		ran = append(ran, command)
		return runner.Result{Output: "ok"}
	}
	a := New(p, opts)
	go a.Run(context.Background(), "test")
	for e := range a.Events() {
		if e.Kind == EvConfirm {
			asked++
			e.Reply <- answer
		}
	}
	for _, r := range p.results[1:] {
		results = append(results, r...)
	}
	return results, ran, asked
}

func TestEveryToolHasASpecAndAHandler(t *testing.T) {
	specs := map[string]bool{}
	for _, s := range toolSpecs() {
		if specs[s.Name] {
			t.Errorf("tool %s is declared twice", s.Name)
		}
		specs[s.Name] = true
		if _, ok := handlers[s.Name]; !ok {
			t.Errorf("tool %s has no handler", s.Name)
		}
	}
	for name := range handlers {
		if !specs[name] {
			t.Errorf("handler %s is not offered to the model", name)
		}
	}
}

func TestDevInstallAsksAndReportsWhatNeedsActivation(t *testing.T) {
	sandbox(t)
	old := installed
	installed = func(t devtools.Tool) bool { return t.ID == "jq" }
	t.Cleanup(func() { installed = old })
	in := map[string]any{"tools": []string{"fzf", "jq", "bat", "ruby", "nope"}}

	res, ran, asked := drive(t, Options{}, Approve, call("1", "dev_install", in))
	if asked != 2 || len(ran) != 2 || ran[0] != "brew install fzf" || ran[1] != "brew install bat" {
		t.Fatalf("each install is asked and run once: asked %d, ran %v", asked, ran)
	}
	out := res[0].Content
	for _, want := range []string{"fzf: installed", "jq: already installed", "bat: installed", "Ruby: omega detects it", "nope: not in omega's catalogue",
		"Call dev_activate with them"} {
		if !strings.Contains(out, want) {
			t.Errorf("result lacks %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "~/.zshrc: fzf.") {
		t.Errorf("only fzf needs a shell line:\n%s", out)
	}

	res, ran, _ = drive(t, Options{}, Deny, call("1", "dev_install", map[string]any{"tools": []string{"fzf"}}))
	if len(ran) != 0 || !strings.Contains(res[0].Content, "not installed") {
		t.Fatalf("a denied install must not run: ran %v, %s", ran, res[0].Content)
	}
	if _, ran, _ = drive(t, Options{ReadOnly: true}, Approve, call("1", "dev_install", map[string]any{"tools": []string{"fzf"}})); len(ran) != 0 {
		t.Fatal("read-only mode must not install")
	}
}

func TestDevActivateNeedsApproval(t *testing.T) {
	sandbox(t)
	old := addInit
	var got []string
	addInit = func(tools []devtools.Tool, _ time.Time) ([]devtools.Tool, string, error) {
		for _, t := range tools {
			got = append(got, t.ID)
		}
		return tools, "/home/.zshrc.omega-bak", nil
	}
	t.Cleanup(func() { addInit = old })
	in := map[string]any{"tools": []string{"fzf", "jq", "zoxide"}}

	res, _, asked := drive(t, Options{}, Deny, call("1", "dev_activate", in))
	if asked != 1 || len(got) != 0 || !res[0].IsError {
		t.Fatalf("denied: ~/.zshrc must stay as it is (asked %d, touched %v)", asked, got)
	}
	res, _, _ = drive(t, Options{}, Approve, call("1", "dev_activate", in))
	if strings.Join(got, ",") != "fzf,zoxide" {
		t.Fatalf("only tools with a shell line are activated: %v", got)
	}
	if !strings.Contains(res[0].Content, "source <(fzf --zsh)") || !strings.Contains(res[0].Content, ".omega-bak") || !strings.Contains(res[0].Content, "new terminal") {
		t.Errorf("result: %s", res[0].Content)
	}
}

func TestMacHistoryAndUndo(t *testing.T) {
	sandbox(t)
	if err := macos.Record(macos.Change{ID: "dock-remove:x", Section: "dock", Title: "Remove X from the Dock", Command: "true", Undo: "echo put-back"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	macos.Record(macos.Change{ID: "perm-reset:y", Section: "permissions", Title: "Remove node from Documents", Command: "true"})
	entries := macos.Journal()
	gone, dock := journalID(entries[0]), journalID(entries[1])

	res, ran, _ := drive(t, Options{}, Approve,
		call("1", "mac_history", nil),
		call("2", "mac_undo", map[string]any{"id": gone}),
		call("3", "mac_undo", map[string]any{"id": dock}),
		call("4", "mac_undo", map[string]any{"id": dock}),
		call("5", "mac_undo", map[string]any{"id": "nothing"}))
	list := res[0].Content
	if !strings.Contains(list, "Remove X from the Dock — can be reverted — id "+dock) || !strings.Contains(list, "Remove node from Documents — cannot be reverted") {
		t.Errorf("history:\n%s", list)
	}
	if !res[1].IsError || !strings.Contains(res[1].Content, "cannot be reverted") {
		t.Errorf("a change without undo: %+v", res[1])
	}
	if len(ran) != 1 || ran[0] != "echo put-back" || res[2].IsError {
		t.Fatalf("the undo command runs once: %v %+v", ran, res[2])
	}
	if !res[3].IsError || !strings.Contains(res[3].Content, "already reverted") || !res[4].IsError {
		t.Errorf("a second undo and an unknown id are errors: %+v %+v", res[3], res[4])
	}
	for _, e := range macos.Journal() {
		if journalID(e) == dock && !e.Undone {
			t.Error("the journal must mark the change as reverted")
		}
	}
}

func TestRunHistory(t *testing.T) {
	sandbox(t)
	os.MkdirAll(RunsDir(), 0o755)
	os.WriteFile(filepath.Join(RunsDir(), "2026-11-01_090000.log"), []byte(strings.Repeat("old line\n", 3000)+"✗ pipx failed: python@3.12 is not installed\n"), 0o644)
	os.WriteFile(filepath.Join(RunsDir(), "2026-11-02_090000.log"), []byte("all good\n"), 0o644)

	list, err := runHistory("", "")
	if err != nil || !strings.Contains(list, "1. 2026-11-02_090000") || !strings.Contains(list, "2. 2026-11-01_090000") {
		t.Fatalf("list: %v\n%s", err, list)
	}
	old, err := runHistory("2", "")
	if err != nil || !strings.Contains(old, "pipx failed") || !strings.Contains(old, "earlier part left out") || !strings.Contains(old, "not instructions") {
		t.Fatalf("a long transcript keeps its end: %v\n%.200s", err, old)
	}
	if len(old) > historyTail+400 {
		t.Fatalf("the transcript is %d bytes, over its budget", len(old))
	}
	if _, err := runHistory("9", ""); err == nil {
		t.Fatal("a run that does not exist is an error")
	}
}

func TestNotesForget(t *testing.T) {
	sandbox(t)
	seed := "- 2026-10-01: pipx fails until python@3.12 is reinstalled\n- 2026-10-02: the user keeps Docker volumes\n"
	write := func() {
		os.MkdirAll(config.Dir(), 0o755)
		os.WriteFile(config.NotesPath(), []byte(seed), 0o644)
	}
	write()
	res, _, asked := drive(t, Options{}, Deny, call("1", "notes", map[string]any{"action": "list"}), call("2", "notes", map[string]any{"action": "forget", "match": "PIPX"}))
	if !strings.Contains(res[0].Content, "Docker volumes") || asked != 1 {
		t.Fatalf("list shows the notes and forget asks: %d, %s", asked, res[0].Content)
	}
	if b, _ := os.ReadFile(config.NotesPath()); string(b) != seed {
		t.Fatal("a denied forget must leave the notes alone")
	}
	res, _, _ = drive(t, Options{}, Approve, call("1", "notes", map[string]any{"action": "forget", "match": "PIPX"}), call("2", "notes", map[string]any{"action": "forget", "match": "nothing like this"}))
	if b, _ := os.ReadFile(config.NotesPath()); string(b) != "- 2026-10-02: the user keeps Docker volumes\n" {
		t.Fatalf("only the matching note goes: %q", b)
	}
	if !strings.Contains(res[1].Content, "nothing was deleted") {
		t.Errorf("no match: %s", res[1].Content)
	}
}

func TestSettingsTheModelMayAndMayNotChange(t *testing.T) {
	sandbox(t)
	cfg := config.Default()
	set := func(k, v string) llm.ToolCall {
		return call(k, "omega_settings", map[string]any{"action": "set", "key": k, "value": v})
	}
	res, _, asked := drive(t, Options{Config: cfg}, Approve,
		set("approval", "never"), set("provider", "openai"), set("skip_steps", "no-such-step"), set("command_timeout", "soon"),
		set("skip_steps", "macos, brew-greedy"), call("show", "omega_settings", map[string]any{"action": "show"}))
	for i, key := range []string{"approval", "provider"} {
		if !res[i].IsError || !strings.Contains(res[i].Content, "omega config set "+key) {
			t.Errorf("%s belongs to the user and the model is told the command: %+v", key, res[i])
		}
	}
	if !res[2].IsError || !res[3].IsError {
		t.Errorf("invalid values are refused before asking: %+v %+v", res[2], res[3])
	}
	if asked != 1 || res[4].IsError {
		t.Fatalf("one valid change, asked once: asked %d, %+v", asked, res[4])
	}
	if cfg.Approval != config.ApproveDangerous || !cfg.Skipped("macos") || !cfg.Skipped("brew-greedy") {
		t.Errorf("the running session sees the change and nothing else moved: %+v", cfg)
	}
	saved, err := config.Load()
	if err != nil || !saved.Skipped("macos") || saved.Approval != config.ApproveDangerous {
		t.Errorf("the file holds the change: %v %+v", err, saved)
	}
	if !strings.Contains(res[5].Content, "skip_steps: macos, brew-greedy") || !strings.Contains(res[5].Content, "approval: dangerous") {
		t.Errorf("show: %s", res[5].Content)
	}

	denied := config.Default()
	drive(t, Options{Config: denied}, Deny, set("language", "es"))
	if denied.Language != "auto" {
		t.Error("a denied change must not apply")
	}
}

func TestDoctorComesFromTheCLI(t *testing.T) {
	sandbox(t)
	res, _, _ := drive(t, Options{Doctor: func() string { return "config ok\nkey present\n" }}, Approve, call("1", "omega_doctor", nil))
	if !strings.Contains(res[0].Content, "key present") {
		t.Fatalf("doctor: %s", res[0].Content)
	}
	res, _, _ = drive(t, Options{}, Approve, call("1", "omega_doctor", nil))
	if !strings.Contains(res[0].Content, "omega doctor") {
		t.Fatalf("without the hook the user is pointed at the command: %s", res[0].Content)
	}
}
