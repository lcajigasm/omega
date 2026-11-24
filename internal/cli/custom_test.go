package cli

import (
	"bytes"
	"context"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/runner"
	"github.com/lcajigasm/omega/internal/safety"
	"github.com/lcajigasm/omega/internal/steps"
)

func TestParseRecommendation(t *testing.T) {
	installed := map[string]bool{"disk": true, "brew": true, "npm-cache": true}
	text := "Here you are:\n```json\n" +
		`{"summary": "Checks, then updates, then cleanup.", "steps": [` +
		`{"id": "disk", "why": "see the space first"}, {"id": "made-up", "why": "x"},` +
		`{"id": "brew", "why": "updates"}, {"id": "disk", "why": "again"}, {"id": "npm-cache", "why": "frees space"}]}` +
		"\n```"
	rec, err := parseRecommendation(text, installed)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(rec.Steps, []string{"disk", "brew", "npm-cache"}) {
		t.Fatalf("steps = %v: unknown ids and repeats must go", rec.Steps)
	}
	if rec.Why["disk"] != "see the space first" || rec.Summary == "" {
		t.Fatalf("reasons or summary lost: %+v", rec)
	}
	for _, bad := range []string{"no json here", `{"steps": [{"id": "made-up"}]}`, `{"steps": [}`} {
		if _, err := parseRecommendation(bad, installed); err == nil {
			t.Errorf("%q gave a routine", bad)
		}
	}
}

func TestRoutineFileRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, ok, err := config.LoadRoutine(); ok || err != nil {
		t.Fatalf("no file: ok=%v err=%v", ok, err)
	}
	if err := config.SaveRoutine(config.Routine{Steps: []string{" brew ", "", "npm", "brew"}}); err != nil {
		t.Fatal(err)
	}
	r, ok, err := config.LoadRoutine()
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if !slices.Equal(r.Steps, []string{"brew", "npm"}) {
		t.Fatalf("steps = %v", r.Steps)
	}
	if err := config.SaveRoutine(config.Routine{}); err == nil {
		t.Fatal("an empty routine was saved")
	}
}

func fakeCustomRun(out *bytes.Buffer, tty bool, answer bool, exits map[string]int) (customRun, *[]string) {
	var ran []string
	cat := map[string]steps.Step{
		"look":  {ID: "look", Name: "Look", Command: "echo look", Risk: safety.Safe},
		"up":    {ID: "up", Name: "Update", Command: "echo up", Risk: safety.Normal},
		"wipe":  {ID: "wipe", Name: "Wipe", Command: "echo wipe", Risk: safety.Dangerous},
		"break": {ID: "break", Name: "Break", Command: "false", Risk: safety.Normal},
	}
	cfg := config.Default()
	return customRun{
		cfg: cfg, out: out, tty: tty,
		find:    func(id string) (steps.Step, bool) { s, ok := cat[id]; return s, ok },
		confirm: func(string) bool { return answer },
		sudo:    func(string) error { return nil },
		exec: func(_ context.Context, cmd string, _ time.Duration, onLine func(string)) runner.Result {
			ran = append(ran, cmd)
			onLine("output of " + cmd)
			return runner.Result{ExitCode: exits[cmd], Duration: 2 * time.Second}
		},
	}, &ran
}

func TestCustomRunRunsInOrderAndSummarises(t *testing.T) {
	var out bytes.Buffer
	c, ran := fakeCustomRun(&out, true, true, map[string]int{"false": 3})
	err := c.run(context.Background(), []string{"look", "up", "gone", "break", "wipe"})
	if err == nil {
		t.Fatal("a failed step must fail the command")
	}
	if !slices.Equal(*ran, []string{"echo look", "echo up", "false", "echo wipe"}) {
		t.Fatalf("ran %v", *ran)
	}
	s := out.String()
	for _, want := range []string{"output of echo up", "exit 3", "gone", "3 done · 1 failed"} {
		if !strings.Contains(s, want) {
			t.Errorf("output lacks %q:\n%s", want, s)
		}
	}
}

func TestCustomRunWithoutTerminalSkipsWhatNeedsAYes(t *testing.T) {
	var out bytes.Buffer
	c, ran := fakeCustomRun(&out, false, false, nil)
	err := c.run(context.Background(), []string{"look", "wipe"})
	if err == nil {
		t.Fatal("a skipped step must fail the command, so a script does not read it as done")
	}
	if slices.Contains(*ran, "echo wipe") {
		t.Fatal("a dangerous step ran with nobody to approve it")
	}
}

func TestCustomRunDryRunRunsNothing(t *testing.T) {
	var out bytes.Buffer
	c, ran := fakeCustomRun(&out, true, true, nil)
	c.dryRun = true
	if err := c.run(context.Background(), []string{"look", "wipe"}); err != nil {
		t.Fatal(err)
	}
	if len(*ran) != 0 {
		t.Fatalf("dry run ran %v", *ran)
	}
}

func TestSummarizeLog(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/2026-11-23_093315.log"
	body := "omega run 2026-11-23T09:33:15+01:00 — anthropic / claude-haiku-5-5\n" +
		"version: dev · host: mac\n" +
		"command: ./omega full --unattended (task: history)\n" +
		strings.Repeat("output line\n", 2000) +
		"\n== tools: 5 succeeded, 2 failed: a, b\n\n== end. tokens: 10 in (0 cached), 4 out, $0.0092\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	s := summarizeLog(path)
	if s.task != "history" || s.model != "claude-haiku-5-5" || !s.ended || s.failed != 2 || s.cost != 0.0092 {
		t.Fatalf("summary = %+v", s)
	}
	if s.when.Hour() != 9 {
		t.Fatalf("when = %v", s.when)
	}
	cut := dir + "/cut.log"
	os.WriteFile(cut, []byte("omega run 2026-11-23T09:33:15+01:00 — anthropic / m\nversion: x\n"), 0o600)
	if s := summarizeLog(cut); s.ended {
		t.Fatal("a log with no closing line counted as finished")
	}
}
