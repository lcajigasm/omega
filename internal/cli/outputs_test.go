package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/pending"
	"github.com/lcajigasm/omega/internal/steps"
)

func writeRun(t *testing.T, name, task string, failed int, ended bool) {
	t.Helper()
	body := "omega run 2026-11-23T09:33:15+01:00 — anthropic / claude-haiku-5-5\nversion: dev\ncommand: omega (task: " + task + ")\n\n" +
		"> the prompt\n\nSome prose of the model.\n» run_step brew\n$ brew upgrade\n  output line\n[run_step:brew exit=0 1s]\nMore prose.\n"
	if ended {
		body += "\n== tools: 3 succeeded, " + string(rune('0'+failed)) + " failed\n\n== end. tokens: 1 in, 1 out, $0.0100\n"
	}
	if err := os.MkdirAll(agent.RunsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agent.RunsDir(), name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// N5: logs filters, --json and view --tools.
func TestLogsFiltersAndTools(t *testing.T) {
	isolate(t, nil)
	writeRun(t, "2026-11-01_100000.log", "clean", 0, true)
	writeRun(t, "2026-11-02_100000.log", "update", 2, true)
	writeRun(t, "2026-11-03_100000.log", "clean", 0, false)
	out, err := execRoot(t, "logs", "--json")
	var runs []logEntry
	if err != nil || json.Unmarshal([]byte(out), &runs) != nil || len(runs) != 3 {
		t.Fatalf("--json: %v\n%s", err, out)
	}
	out, _ = execRoot(t, "logs", "--json", "--task", "clean", "-n", "1")
	if json.Unmarshal([]byte(out), &runs) != nil || len(runs) != 1 || runs[0].Task != "clean" || runs[0].N != 1 {
		t.Fatalf("--task -n: %s", out)
	}
	out, _ = execRoot(t, "logs", "--json", "--failed")
	if json.Unmarshal([]byte(out), &runs) != nil || len(runs) != 2 {
		t.Fatalf("--failed keeps the failed and the unfinished: %s", out)
	}
	out, err = execRoot(t, "logs", "view", "--tools")
	if err != nil || !strings.Contains(out, "» run_step brew") || !strings.Contains(out, "  output line") || strings.Contains(out, "prose") || strings.Contains(out, "the prompt") {
		t.Fatalf("view --tools: %v\n%s", err, out)
	}
}

// F11: --json on pending, notes, auth status, config show and model list.
func TestJSONAnswers(t *testing.T) {
	isolate(t, nil)
	now := time.Now()
	if err := pending.Save([]pending.Item{{ID: "time-machine", Text: "set it up", Status: pending.Open, FirstSeen: now, LastSeen: now, Seen: 1}}); err != nil {
		t.Fatal(err)
	}
	writeFile(t, config.NotesPath(), "- 2026-11-01: first\n- 2026-11-02: second\n")
	for _, args := range [][]string{{"pending", "--json"}, {"notes", "--json"}, {"auth", "status", "--json"}, {"config", "show", "--json"}, {"history", "--json"}} {
		out, err := execRoot(t, args...)
		if err != nil || !json.Valid([]byte(out)) {
			t.Errorf("omega %v: %v\n%s", args, err, out)
		}
	}
	out, _ := execRoot(t, "notes", "--json")
	if !strings.Contains(out, `"day": "2026-11-02"`) {
		t.Errorf("notes --json: %s", out)
	}
	out, _ = execRoot(t, "config", "show", "--json")
	if !strings.Contains(out, `"approval"`) {
		t.Errorf("config show --json does not use the keys of config.toml: %s", out)
	}
	out, _ = execRoot(t, "auth", "status", "--json")
	if !strings.Contains(out, `"provider": "anthropic"`) {
		t.Errorf("auth status --json: %s", out)
	}
	// model list is the canonical name; models still answers.
	c, _, _ := testRoot().Find([]string{"model", "list"})
	if c.Name() != "list" || c.Flags().Lookup("json") == nil {
		t.Error("omega model list --json is missing")
	}
	if m, _, _ := testRoot().Find([]string{"models"}); m.Name() != "models" || !m.Hidden {
		t.Error("omega models should still work, out of the list")
	}
}

// N3: history gathers runs, pending items and notes, and filters by task
// and date.
func TestHistory(t *testing.T) {
	isolate(t, nil)
	writeRun(t, "2026-11-01_100000.log", "clean", 0, true)
	writeRun(t, "2026-11-02_100000.log", "update", 0, true)
	writeFile(t, config.NotesPath(), "- 2026-10-01: old note\n- 2026-11-02: new note\n")
	h, err := loadHistory(5, "clean", time.Time{}, time.Now())
	if err != nil || len(h.Runs) != 1 || h.Runs[0].Task != "clean" || len(h.Notes) != 2 {
		t.Fatalf("%+v %v", h, err)
	}
	since, _ := parseSince("2026-10-15", time.Now())
	h, _ = loadHistory(5, "", since, time.Now())
	if len(h.Notes) != 1 || h.Notes[0].Text != "new note" {
		t.Fatalf("--since: %+v", h.Notes)
	}
	if _, err := parseSince("yesterday", time.Now()); err == nil {
		t.Error("a --since nobody can read was taken")
	}
	if d, _ := parseSince("7d", time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)); d.Day() != 17 {
		t.Errorf("7d: %v", d)
	}
	out, err := execRoot(t, "history")
	if err != nil || !strings.Contains(strings.ToLower(out), "recent runs") || !strings.Contains(out, "new note") {
		t.Errorf("history: %v\n%s", err, out)
	}
}

// N4: notes compact removes the older repeats, keeps a backup and asks.
func TestNotesCompact(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.md")
	body := "- 2026-10-01: Worktree agents-v3.7 pending.\n" +
		"- 2026-10-02: Disk tight: 22 GB free.\n" +
		"- 2026-10-03: 2026-10-03 worktree AGENTS-v3.7 pending\n" +
		"- 2026-10-04: something else\n"
	writeFile(t, path, body)
	var out strings.Builder
	// Nobody at a terminal and no --yes: nothing is written.
	if err := compactNotes(&out, path, false, false, false, nil); err == nil {
		t.Fatal("removed without a yes")
	}
	if b, _ := os.ReadFile(path); string(b) != body {
		t.Fatal("the file changed without a yes")
	}
	if err := compactNotes(&out, path, true, false, false, nil); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), "2026-10-01") || !strings.Contains(string(b), "AGENTS-v3.7") || !strings.Contains(string(b), "something else") {
		t.Errorf("compacted:\n%s", b)
	}
	if bak, _ := os.ReadFile(path + ".bak"); string(bak) != body {
		t.Error("no backup of the notes")
	}
	out.Reset()
	if err := compactNotes(&out, path, true, false, false, nil); err != nil || !strings.Contains(out.String(), "No repeated notes") {
		t.Errorf("second pass: %v %s", err, out.String())
	}
}

// F21/N2: steps filters and steps show.
func TestStepsShowAndFilters(t *testing.T) {
	isolate(t, nil)
	out, err := execRoot(t, "steps", "--json", "--category", "check")
	var list []StepInfo
	if err != nil || json.Unmarshal([]byte(out), &list) != nil || len(list) == 0 {
		t.Fatalf("--category: %v %s", err, out)
	}
	for _, s := range list {
		if s.CategoryID != "check" {
			t.Errorf("%s is not a check", s.ID)
		}
	}
	if _, err := execRoot(t, "steps", "--category", "bogus"); usageCode(err) != exitUsage {
		t.Errorf("a category that does not exist: %v", err)
	}
	id := steps.All[0].ID
	out, err = execRoot(t, "steps", "show", id)
	if err != nil || !strings.Contains(out, steps.All[0].Command[:min(20, len(steps.All[0].Command))]) {
		t.Errorf("steps show: %v\n%s", err, out)
	}
	out, _ = execRoot(t, "steps", "show", id, "--json")
	if !strings.Contains(out, `"always_confirm"`) {
		t.Errorf("steps show --json: %s", out)
	}
	if _, err := execRoot(t, "steps", "show", "nosuch"); err == nil {
		t.Error("an unknown step was shown")
	}
}

// D12: an unknown key of config.toml is said by config check and doctor.
func TestUnknownConfigKeysAreSaid(t *testing.T) {
	isolate(t, nil)
	writeFile(t, config.Path(), "aproval = \"all\"\n")
	out, err := execRoot(t, "config", "check")
	if err != nil || !strings.Contains(out, "aproval") {
		t.Errorf("config check: %v\n%s", err, out)
	}
	var b strings.Builder
	_ = runDoctor(&b)
	if !strings.Contains(b.String(), "aproval") {
		t.Errorf("doctor:\n%s", b.String())
	}
}

// E1: the run without the AI speaks the user's language, its JSON summary
// too; the field names stay.
func TestDeterministicIsTranslated(t *testing.T) {
	t.Cleanup(func() { i18n.Init("en") })
	i18n.Init("es")
	d, _, _, out := newDet(runFlags{dryRun: true}, false, true)
	d.cfg.SkipSteps = nil
	_ = d.run(context.Background(), agent.TaskClean)
	if !strings.Contains(out.String(), "omega clean  · sin IA") || !strings.Contains(out.String(), "LIMPIEZA") || strings.Contains(out.String(), "Deterministic Mode") {
		t.Errorf("clean in Spanish:\n%s", out.String())
	}
	if !strings.Contains(d.report.Summary, "Limpieza") {
		t.Errorf("summary: %q", d.report.Summary)
	}
	b, _ := json.Marshal(d.report)
	if !strings.Contains(string(b), `"summary"`) || !strings.Contains(string(b), `"steps_run"`) {
		t.Errorf("JSON field names changed: %s", b)
	}
}

// E4: every step has a name in the five languages, but for proper names.
func TestEveryStepHasATranslatedName(t *testing.T) {
	proper := map[string]bool{"brew": true, "mas": true, "bun": true, "pipx": true, "gem": true, "deno": true, "poetry": true,
		"conda": true, "flutter": true, "claude-code": true, "topgrade": true, "macos": true, "omz": true}
	for _, goos := range []string{"darwin", "linux", "windows"} {
		for _, s := range steps.For(goos) {
			if !proper[s.ID] && steps.Builtin(s.ID) && i18n.TOr("step."+s.ID, "") == "" {
				t.Errorf("%s/%s (%q) has no step.%s", goos, s.ID, s.Name, s.ID)
			}
		}
	}
}

// E5: the plurals of the texts this package counts with.
func TestPluralForms(t *testing.T) {
	i18n.Init("es")
	t.Cleanup(func() { i18n.Init("en") })
	if got := i18n.N("steps.failed_summary", 1); !strings.HasPrefix(got, "1 paso falló") {
		t.Errorf("one: %q", got)
	}
	if got := i18n.N("steps.failed_summary", 3); !strings.HasPrefix(got, "3 pasos fallaron") {
		t.Errorf("three: %q", got)
	}
	if got := i18n.N("doctor.failed", 1); got != "1 comprobación fallida" {
		t.Errorf("doctor: %q", got)
	}
}
