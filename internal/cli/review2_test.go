package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/hardware"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/runner"
	"github.com/lcajigasm/omega/internal/safety"
	"github.com/lcajigasm/omega/internal/steps"
)

// Regression tests for the second review (13 November).

// execRoot runs a command line through the real command tree and returns
// what it wrote to its own output and the error.
func execRoot(t *testing.T, args ...string) (string, error) {
	t.Helper()
	i18n.Init("en")
	root := testRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ---- bootstrap ----

// A blueprint whose "package" is a piece of shell is refused whole: nothing
// runs and the configuration is not replaced, even under --yes.
func TestBootstrapRefusesShellInAPackageName(t *testing.T) {
	isolate(t, nil)
	writeFile(t, config.Path(), "approval = \"dangerous\"\n")
	file := filepath.Join(t.TempDir(), "bp.json")
	marker := filepath.Join(t.TempDir(), "INJECTED")
	// Forward slashes: a Windows path would not be valid inside JSON.
	writeFile(t, file, `{"homebrew":["--help >/dev/null; touch `+filepath.ToSlash(marker)+`"],"config":{"Provider":"anthropic","Approval":"never","Language":"auto","Timeout":"45m"}}`)

	var out bytes.Buffer
	err := bootstrapRun{yes: true}.run(&out, file)
	if err == nil || !strings.Contains(err.Error(), "not a package name") {
		t.Fatalf("the blueprint was accepted: %v\n%s", err, out.String())
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Error("the shell text ran")
	}
	if b, _ := os.ReadFile(config.Path()); !strings.Contains(string(b), `"dangerous"`) {
		t.Errorf("the configuration was replaced: %s", b)
	}
	for _, name := range []string{"-rf", "a b", "$(x)", "a;b", "a|b", "`x`", ""} {
		file := filepath.Join(t.TempDir(), "bp.json")
		b, _ := json.Marshal(map[string]any{"npm_global": []string{name}})
		writeFile(t, file, string(b))
		if err := (bootstrapRun{yes: true}).run(io.Discard, file); err == nil {
			t.Errorf("%q was accepted as a package name", name)
		}
	}
}

// Nothing in a blueprint happens unasked, a dry run changes nothing, and
// the settings it restores never widen what runs without asking.
func TestBootstrapAsksAndKeepsApproval(t *testing.T) {
	isolate(t, nil)
	writeFile(t, config.Path(), "approval = \"all\"\nlanguage = \"en\"\n")
	file := filepath.Join(t.TempDir(), "bp.json")
	writeFile(t, file, `{"config":{"Provider":"anthropic","Approval":"never","Language":"es","Timeout":"30m",
		"Providers":{"custom":{"BaseURL":"http://elsewhere.example/v1"}}}}`)
	unchanged := func(when string) {
		t.Helper()
		if b, _ := os.ReadFile(config.Path()); !strings.Contains(string(b), `language = "en"`) {
			t.Fatalf("%s: the configuration changed: %s", when, b)
		}
	}

	// Nobody at a terminal and no --yes: an error, and nothing done.
	var out bytes.Buffer
	if err := (bootstrapRun{}).run(&out, file); err == nil {
		t.Error("ran without a terminal and without --yes")
	}
	unchanged("no terminal")
	// --dry-run shows the plan only.
	out.Reset()
	if err := (bootstrapRun{dryRun: true, yes: true}).run(&out, file); err != nil {
		t.Fatal(err)
	}
	unchanged("dry run")
	if !strings.Contains(out.String(), "approval stays") || !strings.Contains(out.String(), "base_url") {
		t.Errorf("the plan does not say what is never restored:\n%s", out.String())
	}
	// At a terminal the answer decides.
	asked := 0
	no := bootstrapRun{tty: true, confirm: func(string) bool { asked++; return false }}
	if err := no.run(io.Discard, file); err != nil || asked != 1 {
		t.Errorf("answered no: %v, asked %d", err, asked)
	}
	unchanged("answered no")

	// Yes: the settings come over, approval and base_url do not, and the
	// file that was there is kept.
	if err := (bootstrapRun{yes: true}).run(io.Discard, file); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Language != "es" || cfg.Timeout != "30m" {
		t.Errorf("the settings were not restored: %+v", cfg)
	}
	if cfg.Approval != config.ApproveAll {
		t.Errorf("approval came from the blueprint: %q", cfg.Approval)
	}
	if cfg.BaseURLFor("custom") != "" {
		t.Errorf("base_url came from the blueprint: %q", cfg.BaseURLFor("custom"))
	}
	if b, err := os.ReadFile(config.Path() + ".bak"); err != nil || !strings.Contains(string(b), `language = "en"`) {
		t.Errorf("no backup of the previous configuration: %v %s", err, b)
	}
}

// ---- runs without the model ----

// A request in the user's own words is not answered with the full
// maintenance, and a task only the model can do fails before printing.
func TestDeterministicRefusesWhatNeedsTheModel(t *testing.T) {
	isolate(t, nil)
	i18n.Init("en")
	for _, f := range []runFlags{
		{noAI: true, prompt: "what is using my disk"},
		{offline: true, prompt: "what is using my disk"},
		{json: true, prompt: "what is using my disk"},
		{format: "json", prompt: "what is using my disk"},
	} {
		err := runTask(agent.TaskFull, f)
		if err == nil || !strings.Contains(err.Error(), "needs the AI") {
			t.Errorf("%+v: %v", f, err)
		}
	}
	// The words after `omega` reach runTask as a prompt with no task.
	if _, err := execRoot(t, "--json", "what", "is", "using", "my", "disk"); err == nil || !strings.Contains(err.Error(), "needs the AI") {
		t.Errorf("omega --json <sentence>: %v", err)
	}
	needAI := [][]string{{"dev", "--no-ai"}, {"fix", "--json"}, {"tune", "--offline"}}
	if runtime.GOOS == "darwin" { // elsewhere `omega mac` says it is for macOS
		needAI = append(needAI, []string{"mac", "--json"})
	}
	for _, args := range needAI {
		out, err := execRoot(t, args...)
		if err == nil || !strings.Contains(err.Error(), "needs the AI") {
			t.Errorf("omega %v: %v", args, err)
		}
		if strings.TrimSpace(out) != "" {
			t.Errorf("omega %v printed before failing: %q", args, out)
		}
	}
	if !hasDeterministic(agent.TaskFull) || hasDeterministic("") || hasDeterministic(agent.TaskFix) {
		t.Error("the list of tasks that run without the model is wrong")
	}
}

// A configuration that cannot be read or is not valid stops the run: the
// defaults would run what skip_steps turned off.
func TestDeterministicNeedsAReadableConfig(t *testing.T) {
	isolate(t, nil)
	i18n.Init("en")
	writeFile(t, config.Path(), "skip_steps = [\"trash\"\n")
	if err := runTask(agent.TaskClean, runFlags{noAI: true, dryRun: true, quiet: true}); err == nil || !strings.Contains(err.Error(), "config.toml") {
		t.Errorf("unreadable config: %v", err)
	}
	// With no key the routine tasks fall back to the built-in steps, but
	// not past a configuration error.
	if err := runTask(agent.TaskClean, runFlags{dryRun: true, quiet: true}); err == nil || !strings.Contains(err.Error(), "config.toml") {
		t.Errorf("unreadable config, AI mode: %v", err)
	}
	writeFile(t, config.Path(), "approval = \"sometimes\"\n")
	if err := runTask(agent.TaskClean, runFlags{noAI: true, dryRun: true, quiet: true}); err == nil || !strings.Contains(err.Error(), "approval") {
		t.Errorf("invalid config: %v", err)
	}
}

// Only a missing key falls back to the built-in steps. A provider that does
// not exist is an error.
func TestNoKeyFallbackIsOnlyForAMissingKey(t *testing.T) {
	isolate(t, nil)
	i18n.Init("en")
	err := runTask(agent.TaskClean, runFlags{provider: "bogus", dryRun: true, quiet: true})
	if err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Errorf("--provider bogus: %v", err)
	}
	// No key: the dry run of the built-in steps, which is not an error.
	if err := runTask(agent.TaskClean, runFlags{dryRun: true, quiet: true}); err != nil {
		t.Errorf("missing key: %v", err)
	}
}

// approval = "all" asks before anything that is not read-only, and skips it
// (saying so, and in the report) when nobody can answer.
func TestDeterministicApprovalAll(t *testing.T) {
	normal := steps.Step{ID: "brew", Name: "Homebrew", Category: steps.Update, Command: "brew upgrade", Risk: safety.Normal}

	d, ran, asked, _ := newDet(runFlags{}, true, true)
	d.change(context.Background(), normal, time.Second)
	if len(*ran) != 1 || *asked != 0 {
		t.Errorf("approval dangerous: ran %v, asked %d", *ran, *asked)
	}

	d, ran, asked, out := newDet(runFlags{}, false, true)
	d.cfg.Approval = config.ApproveAll
	d.change(context.Background(), normal, time.Second)
	if len(*ran) != 0 || *asked != 0 || !strings.Contains(out.String(), "skipped") || len(d.report.Skipped) != 1 {
		t.Errorf("approval all, no terminal: ran %v, asked %d, said %q, report %+v", *ran, *asked, out.String(), d.report.Skipped)
	}

	d, ran, asked, _ = newDet(runFlags{}, true, false)
	d.cfg.Approval = config.ApproveAll
	d.change(context.Background(), normal, time.Second)
	if len(*ran) != 0 || *asked != 1 {
		t.Errorf("approval all, answered no: ran %v, asked %d", *ran, *asked)
	}

	// --yes answers, as it does for the agent; a read-only step never asks.
	d, ran, asked, _ = newDet(runFlags{yes: true}, false, true)
	d.cfg.Approval = config.ApproveAll
	d.change(context.Background(), normal, time.Second)
	if len(*ran) != 1 || *asked != 0 {
		t.Errorf("approval all with --yes: ran %v, asked %d", *ran, *asked)
	}
	cfg := config.Default()
	cfg.Approval = config.ApproveAll
	if needsAnswer(cfg, steps.Step{Risk: safety.Safe, Command: "df -h"}, false) {
		t.Error("a read-only step asks")
	}
}

// A step that needs sudo and cannot get the password fails without running.
func TestDeterministicSudoRefused(t *testing.T) {
	d, ran, _, out := newDet(runFlags{yes: true}, false, true)
	d.sudo = func(string) error { return errors.New("no password") }
	d.change(context.Background(), steps.Step{ID: "x", Name: "x", Command: "sudo true", Risk: safety.Normal}, time.Second)
	if len(*ran) != 0 || d.failed != 1 || !strings.Contains(out.String(), "sudo") {
		t.Errorf("ran %v, failed %d, said %q", *ran, d.failed, out.String())
	}
}

// The report is one document a program can rely on: steps_run is a list
// even when empty, a dry run says so and says what it would run, and every
// step has a duration, also in milliseconds.
func TestDeterministicReportShape(t *testing.T) {
	d, _, _, _ := newDet(runFlags{dryRun: true}, false, true)
	d.report = &DeterministicReport{DryRun: true, StepsRun: []StepRunRecord{}}
	d.change(context.Background(), trash, time.Second)
	b, err := json.Marshal(d.report)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"steps_run":[]`, `"dry_run":true`, `"would_run":[{"id":"trash"`, `"total_duration_ms":`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("%s is missing from %s", want, b)
		}
	}

	d, _, _, _ = newDet(runFlags{}, false, true)
	d.exec = func(context.Context, string, time.Duration, func(string)) runner.Result {
		time.Sleep(2 * time.Millisecond)
		return runner.Result{}
	}
	check := steps.Step{ID: "c", Name: "c", Category: steps.Check, Command: "true", Risk: safety.Safe}
	if _, ran := d.check(context.Background(), check, time.Second); !ran {
		t.Fatal("the check did not run")
	}
	if r := d.report.StepsRun[0]; r.Duration <= 0 || r.DurationMS < 1 {
		t.Errorf("a check has no duration: %v, %d ms", r.Duration, r.DurationMS)
	}
}

// Only the lines that name a package are counted as outdated.
func TestOutdatedCount(t *testing.T) {
	for _, c := range []struct {
		id, out string
		want    int
	}{
		{"brew-outdated", "Warning: Some taps failed to update\ngit (2.44.0) < 2.45.0\nmise (2026.11.7) < 2026.11.8\n", 2},
		{"brew-outdated", "==> Auto-updating Homebrew...\nError: something\n", 0},
		{"brew-outdated", "", 0},
		{"apt-outdated", "Listing... Done\ngit/stable 1:2.45 amd64 [upgradable from: 1:2.44]\n", 1},
		{"dnf-outdated", "Last metadata expiration check: 0:10:01 ago.\n\ngit.x86_64  2.45.0-1.fc40  updates\n", 1},
		{"pacman-outdated", "git 2.44.0-1 -> 2.45.0-1\n", 1},
		{"zypper-outdated", "Loading repository data...\nReading installed packages...\nS | Repository | Name | Current | Available | Arch\n--+------------+------+---------+-----------+-----\nv | repo-oss   | git  | 2.44    | 2.45      | x86_64\n", 1},
		{"zypper-outdated", "Loading repository data...\nReading installed packages...\nNo updates found.\n", 0},
		{"snap-outdated", "Name  Version  Rev  Size  Publisher  Notes\ncore22  2024  1380  77MB  canonical  base\n", 1},
		{"snap-outdated", "All snaps up to date.\n", 0},
		{"choco-outdated", "Chocolatey v2.2.2\nOutdated Packages\n Output is package name | current version | available version | pinned?\n\ngit|2.44.0|2.45.0|false\n\nChocolatey has determined 1 package(s) are outdated.\n", 1},
		{"winget-outdated", "Name  Id       Version  Available  Source\n-----------------------------------------\nGit   Git.Git  2.44.0   2.45.0     winget\n1 upgrades available.\n", 1},
	} {
		if got := outdatedCount(c.id, c.out); got != c.want {
			t.Errorf("%s: counted %d, want %d in %q", c.id, got, c.want, c.out)
		}
	}
}

// quick leaves out the checks skip_steps turned off, counts packages and
// not lines, and its last line says what happened instead of "healthy".
func TestQuickHonoursSkipAndSaysWhatItDid(t *testing.T) {
	var outdated []steps.Step
	for _, s := range steps.For(goos) {
		if strings.HasSuffix(s.ID, "-outdated") && s.Risk == safety.Safe && s.Available() {
			outdated = append(outdated, s)
		}
	}
	if len(outdated) == 0 {
		t.Skip("no package manager with an outdated check is installed")
	}
	fake := func(_ context.Context, command string, _ time.Duration, _ func(string)) runner.Result {
		if command == outdated[0].Command {
			return runner.Result{Output: "Warning: a tap is broken\n"}
		}
		return runner.Result{}
	}
	oldBattery := batteryInfo
	t.Cleanup(func() { batteryInfo = oldBattery })
	batteryInfo = func(context.Context) (*hardware.BatteryHealth, error) { return nil, nil }
	d, _, _, out := newDet(runFlags{dryRun: true}, false, true)
	var ran []string
	d.exec = func(ctx context.Context, command string, timeout time.Duration, onLine func(string)) runner.Result {
		ran = append(ran, command)
		return fake(ctx, command, timeout, onLine)
	}
	d.cfg.SkipSteps = []string{outdated[0].ID}
	d.quick(context.Background())
	for _, c := range ran {
		if c == outdated[0].Command {
			t.Errorf("%s ran although skip_steps names it", outdated[0].ID)
		}
	}
	if strings.Contains(out.String(), "healthy") {
		t.Errorf("quick still calls the machine healthy:\n%s", out.String())
	}

	d, _, _, out = newDet(runFlags{dryRun: true}, false, true)
	d.exec = fake
	d.quick(context.Background())
	if d.report.OutdatedCount != 0 {
		t.Errorf("a warning was counted as %d outdated package(s)", d.report.OutdatedCount)
	}
	// A check that fails is said in the last line.
	d, _, _, out = newDet(runFlags{dryRun: true}, false, true)
	d.exec = func(context.Context, string, time.Duration, func(string)) runner.Result {
		return runner.Result{ExitCode: 1}
	}
	d.quick(context.Background())
	if !strings.Contains(d.report.Summary, "could not be run") || strings.Contains(d.report.Summary, "No problems") {
		t.Errorf("failed checks are not in the summary: %q", d.report.Summary)
	}
}

// ---- steps run ----

func newStepsRun(tty, answer bool, list ...steps.Step) (*stepsRun, *[]string, *int, *bytes.Buffer) {
	var ran []string
	asked := 0
	out := &bytes.Buffer{}
	r := &stepsRun{
		cfg: config.Default(), out: out, tty: tty,
		find: func(id string) (steps.Step, bool) {
			for _, s := range list {
				if s.ID == id {
					return s, true
				}
			}
			return steps.Step{}, false
		},
		confirm: func(string) bool { asked++; return answer },
		sudo:    func(string) error { return nil },
		exec: func(_ context.Context, command string, _ time.Duration, _ func(string)) runner.Result {
			ran = append(ran, command)
			return runner.Result{}
		},
	}
	return r, &ran, &asked, out
}

// `omega steps run <id> --yes` does not answer for a step that always asks,
// and a step nobody could confirm makes the command fail.
func TestStepsRunAlwaysConfirm(t *testing.T) {
	i18n.Init("en")
	// --yes, no terminal: skipped, said, and not exit 0.
	r, ran, asked, out := newStepsRun(false, true, volumes, trash)
	r.yes = true
	err := r.run([]string{"docker-volumes"})
	if len(*ran) != 0 || *asked != 0 || err == nil || !strings.Contains(out.String(), "needs confirmation") {
		t.Errorf("--yes without a terminal: ran %v, asked %d, err %v, said %q", *ran, *asked, err, out.String())
	}
	// --yes at a terminal: still asked.
	r, ran, asked, _ = newStepsRun(true, false, volumes)
	r.yes = true
	if err := r.run([]string{"docker-volumes"}); err != nil || len(*ran) != 0 || *asked != 1 {
		t.Errorf("--yes at a terminal, answered no: ran %v, asked %d, err %v", *ran, *asked, err)
	}
	r, ran, asked, _ = newStepsRun(true, true, volumes)
	r.yes = true
	if err := r.run([]string{"docker-volumes"}); err != nil || len(*ran) != 1 || *asked != 1 {
		t.Errorf("--yes at a terminal, answered yes: ran %v, asked %d, err %v", *ran, *asked, err)
	}
	// An ordinary dangerous step: --yes is enough; without it and without
	// a terminal it is skipped and the command fails.
	r, ran, asked, _ = newStepsRun(false, true, trash)
	r.yes = true
	if err := r.run([]string{"trash"}); err != nil || len(*ran) != 1 || *asked != 0 {
		t.Errorf("dangerous with --yes: ran %v, asked %d, err %v", *ran, *asked, err)
	}
	r, ran, _, _ = newStepsRun(false, true, trash)
	if err := r.run([]string{"trash"}); err == nil || len(*ran) != 0 {
		t.Errorf("dangerous without a terminal: ran %v, err %v", *ran, err)
	}
	// --dry-run shows the command and runs nothing.
	r, ran, asked, out = newStepsRun(true, true, trash)
	r.dryRun, r.yes = true, true
	if err := r.run([]string{"trash"}); err != nil || len(*ran) != 0 || *asked != 0 || !strings.Contains(out.String(), "would run trash") {
		t.Errorf("--dry-run: ran %v, asked %d, err %v, said %q", *ran, *asked, err, out.String())
	}
	if _, _, err := testRoot().Find([]string{"steps", "run"}); err != nil {
		t.Fatal(err)
	}
	c, _, _ := testRoot().Find([]string{"steps", "run"})
	if c.Flags().Lookup("dry-run") == nil {
		t.Error("steps run has no --dry-run")
	}
}

// ---- hooks ----

func TestHooksRunAroundATask(t *testing.T) {
	isolate(t, nil)
	i18n.Init("en")
	writeFile(t, filepath.Join(config.Dir(), "steps.d", "hooks.toml"), "[hooks]\npre_clean = \"echo before\"\npost_clean = \"echo after\"\n")
	var names []string
	oldRun, oldOut := hookRun, hookOut
	t.Cleanup(func() { hookRun, hookOut = oldRun, oldOut })
	fail := ""
	hookRun = func(_ context.Context, name string) error {
		names = append(names, name)
		if name == fail {
			return errors.New("hook " + name + " failed")
		}
		return nil
	}
	var out bytes.Buffer
	hookOut = &out

	// Under --dry-run a hook is named, not run; and the run without the
	// model has hooks too.
	if err := runDeterministic(agent.TaskClean, runFlags{noAI: true, dryRun: true}); err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Errorf("hooks ran under --dry-run: %v", names)
	}
	if !strings.Contains(out.String(), "would run hook pre_clean: echo before") || !strings.Contains(out.String(), "would run hook post_clean: echo after") {
		t.Errorf("the dry run does not name the hooks:\n%s", out.String())
	}

	// A real run calls them in order, around the task.
	ranTask := false
	task := func() error { ranTask = true; return nil }
	if err := withHooks(agent.TaskClean, runFlags{}, task); err != nil || !ranTask {
		t.Fatalf("err %v, task ran %v", err, ranTask)
	}
	if strings.Join(names, " ") != "pre_clean post_clean" {
		t.Errorf("hooks: %v", names)
	}
	// A pre hook that fails stops the task and is returned.
	names, ranTask, fail = nil, false, "pre_clean"
	if err := withHooks(agent.TaskClean, runFlags{}, task); err == nil || ranTask {
		t.Errorf("failed pre hook: err %v, task ran %v", err, ranTask)
	}
	// A post hook that fails makes the run fail.
	names, fail = nil, "post_clean"
	if err := withHooks(agent.TaskClean, runFlags{}, task); err == nil {
		t.Error("a failed post hook left the run successful")
	}
	// A task that fails runs on_error, then the post hook.
	names, fail = nil, ""
	boom := errors.New("boom")
	if err := withHooks(agent.TaskClean, runFlags{}, func() error { return boom }); err != boom {
		t.Errorf("the task's error was lost: %v", err)
	}
	if strings.Join(names, " ") != "pre_clean on_error post_clean" {
		t.Errorf("hooks after a failure: %v", names)
	}
	// A request in the user's words and the demo have none.
	names = nil
	withHooks(agent.TaskFull, runFlags{prompt: "why"}, task)
	withHooks(agent.TaskFull, runFlags{demo: true}, task)
	if len(names) != 0 {
		t.Errorf("hooks ran for a prompt or the demo: %v", names)
	}
}

// ---- commands, flags and arguments ----

// `omega full --json` is refused instead of printing several documents.
func TestFullRefusesJSON(t *testing.T) {
	isolate(t, nil)
	for _, args := range [][]string{{"full", "--json", "--yes"}, {"full", "--format", "json", "--unattended"}} {
		out, err := execRoot(t, args...)
		if err == nil || !strings.Contains(err.Error(), "JSON") || strings.TrimSpace(out) != "" {
			t.Errorf("omega %v: %v, printed %q", args, err, out)
		}
	}
}

// A word that is not a subcommand is an error on every command that only
// holds subcommands; on its own such a command shows its help.
func TestGroupCommandsRejectUnknownWords(t *testing.T) {
	isolate(t, nil)
	for _, name := range []string{"net", "audit", "git", "disk", "schedule", "completion", "auth", "config"} {
		// "zzz" is not the start of any subcommand: a unique start of one
		// is accepted, as everywhere in omega.
		if _, err := execRoot(t, name, "zzz"); err == nil || !strings.Contains(err.Error(), "unknown command") {
			t.Errorf("omega %s zzz: %v", name, err)
		}
		out, err := execRoot(t, name)
		if err != nil || !strings.Contains(out, "omega "+name) {
			t.Errorf("omega %s: %v\n%s", name, err, out)
		}
	}
}

func TestRunFlagsAreValidated(t *testing.T) {
	i18n.Init("en")
	for _, f := range []runFlags{{format: "yaml"}, {format: "xml"}, {effort: "bogus"}, {maxTurns: -3}, {maxCost: -1}} {
		if err := f.validate(); err == nil {
			t.Errorf("%+v was accepted", f)
		}
		// The task does not start.
		if err := runTask(agent.TaskClean, f); err == nil {
			t.Errorf("%+v started a task", f)
		}
	}
	for _, f := range []runFlags{{}, {format: "json"}, {format: "JSON"}, {format: "text"}, {effort: "xhigh"}, {maxTurns: 5, maxCost: 0.5}} {
		if err := f.validate(); err != nil {
			t.Errorf("%+v: %v", f, err)
		}
	}
	if _, err := execRoot(t, "full", "--format", "yaml"); err == nil {
		t.Error("omega full --format yaml was accepted")
	}
}

// A number that is not one, and a folder that is not there, are errors.
func TestBadArgumentsAreErrors(t *testing.T) {
	isolate(t, nil)
	for _, args := range [][]string{
		{"net", "ports", "abc"}, {"net", "ports", "0"}, {"net", "ports", "70000"},
		{"disk", "large", filepath.Join(t.TempDir(), "missing"), "50"},
		{"disk", "large", t.TempDir(), "xx"}, {"disk", "large", t.TempDir(), "-5"},
		{"disk", "artifacts", "abc"}, {"disk", "artifacts", "0"},
	} {
		if _, err := execRoot(t, args...); err == nil {
			t.Errorf("omega %v succeeded", args)
		}
	}
}

// The help of --model names its value "string", not "omega models", and
// the man page describes the flags as they behave.
func TestHelpText(t *testing.T) {
	for _, lang := range i18n.Supported {
		i18n.Init(lang)
		name, usage := flagValueName(t, "model")
		if name != "string" || strings.Contains(usage, "`") {
			t.Errorf("%s: --model shows the value as %q (%s)", lang, name, usage)
		}
		long := i18n.T("cli.long")
		if !strings.Contains(long, "\n  quick ") || !strings.Contains(long, "\n  health ") {
			t.Errorf("%s: the long help leaves out quick or health", lang)
		}
	}
	i18n.Init("en")
	man := generateManPage(testRoot())
	for _, wrong := range []string{"YAML", "safe non-destructive", "only print errors"} {
		if strings.Contains(man, wrong) {
			t.Errorf("the man page still says %q", wrong)
		}
	}
	if c, _, _ := testRoot().Find([]string{"quick"}); strings.Contains(c.Long, "disk health") {
		t.Errorf("quick still promises a disk health check: %s", c.Long)
	}
}

func flagValueName(t *testing.T, flag string) (name, usage string) {
	t.Helper()
	f := testRoot().Flags().Lookup(flag)
	if f == nil {
		t.Fatalf("no --%s", flag)
	}
	// What cobra prints after the flag: a back-quoted word of the usage,
	// or the type.
	if i := strings.Index(f.Usage, "`"); i >= 0 {
		if j := strings.Index(f.Usage[i+1:], "`"); j >= 0 {
			return f.Usage[i+1 : i+1+j], f.Usage
		}
	}
	return f.Value.Type(), f.Usage
}

// A yes-or-no question shows its answers once.
func TestYesNoPrompt(t *testing.T) {
	i18n.Init("en")
	if got := yesNoPrompt("Delete these branches?"); got != "Delete these branches? [y/N]: " {
		t.Errorf("%q", got)
	}
	// A translated question that brings its own answers keeps them.
	for _, q := range []string{i18n.T("docker.confirm", 2), i18n.T("steps.confirm", "Trash", "rm"), "Run it? [y/N]"} {
		got := yesNoPrompt(q)
		if strings.Count(got, "/N") != 1 || strings.Contains(got, "[n]") {
			t.Errorf("the answers are shown twice: %q", got)
		}
	}
	i18n.Init("es")
	if got := yesNoPrompt("¿Borrar?"); got != "¿Borrar? [s/N]: " {
		t.Errorf("%q", got)
	}
	i18n.Init("en")
}

// $VISUAL comes before $EDITOR, either may hold arguments, and the fallback
// exists on the system it is used on.
func TestEditorArgv(t *testing.T) {
	old := goos
	t.Cleanup(func() { goos = old })
	for _, c := range []struct {
		visual, editor, os string
		want               []string
	}{
		{"", "code -w", "darwin", []string{"code", "-w", "f.toml"}},
		{"vim", "nano", "linux", []string{"vim", "f.toml"}},
		{"", `"/Applications/Sublime Text.app/Contents/SharedSupport/bin/subl" -w`, "darwin", []string{"/Applications/Sublime Text.app/Contents/SharedSupport/bin/subl", "-w", "f.toml"}},
		{"", "", "linux", []string{"nano", "f.toml"}},
		{"  ", "", "windows", []string{"notepad", "f.toml"}},
	} {
		t.Setenv("VISUAL", c.visual)
		t.Setenv("EDITOR", c.editor)
		goos = c.os
		if got := editorArgv("f.toml"); strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("VISUAL=%q EDITOR=%q on %s: %q, want %q", c.visual, c.editor, c.os, got, c.want)
		}
	}
}

// ---- setup, the doctor's short report, completion ----

func TestSetupNeedsATerminal(t *testing.T) {
	isolate(t, nil)
	old := stdinIsTerminal
	t.Cleanup(func() { stdinIsTerminal = old })
	stdinIsTerminal = func() bool { return false }
	if _, err := execRoot(t, "setup"); err == nil || !strings.Contains(err.Error(), "terminal") {
		t.Errorf("omega setup without a terminal: %v", err)
	}
	if _, err := os.Stat(config.Path()); err == nil {
		t.Error("setup wrote a configuration")
	}
}

// The guided run's doctor stage leaves out every step line, also the ones
// of steps the user turned off.
func TestDoctorBriefDropsSkippedSteps(t *testing.T) {
	isolate(t, nil)
	i18n.Init("en")
	writeFile(t, config.Path(), "skip_steps = [\""+alwaysThere+"\"]\n")
	var full, brief bytes.Buffer
	runDoctor(&full)
	doctorBrief(&brief)
	if !strings.Contains(full.String(), alwaysThere+" (skipped)") {
		t.Fatalf("the doctor does not mark the skipped step:\n%s", full.String())
	}
	if strings.Contains(brief.String(), "(skipped)") || strings.Contains(brief.String(), alwaysThere) {
		t.Errorf("the short report keeps a step line:\n%s", brief.String())
	}
	if !strings.Contains(brief.String(), "steps:") || !strings.Contains(brief.String(), "config:") {
		t.Errorf("the short report lost its own lines:\n%s", brief.String())
	}
}

func TestCompletionTarget(t *testing.T) {
	i18n.Init("en")
	for _, c := range []struct{ os, shell, want string }{
		{"darwin", "/bin/zsh", "zsh"}, {"linux", "/usr/bin/fish", "fish"}, {"linux", "/bin/bash", "bash"},
		{"windows", "", "powershell"}, {"windows", `C:\Program Files\PowerShell\7\pwsh.exe`, "powershell"},
		{"windows", "/usr/bin/bash", "bash"}, // Git Bash sets SHELL
		{"linux", "", ""}, {"linux", "/usr/bin/nu", ""},
	} {
		kind, path, err := completionTarget(c.os, c.shell, "/home/u")
		if kind != c.want || (c.want == "") != (err != nil) || (c.want != "" && path == "") {
			t.Errorf("%s with SHELL=%q: %q %q %v", c.os, c.shell, kind, path, err)
		}
	}
}

// ---- custom steps ----

// A steps.d file omega cannot use is listed by `config check` and the
// doctor, by name.
func TestCustomStepProblemsAreReported(t *testing.T) {
	isolate(t, nil)
	i18n.Init("en")
	writeFile(t, filepath.Join(config.Dir(), "steps.d", "broken.toml"), "[[steps]\nid = ")
	writeFile(t, filepath.Join(config.Dir(), "steps.d", "odd.toml"), "[[steps]]\nid = \"mine\"\ncategory = \"maintenance\"\ncommand = \"echo hi\"\n")
	_, err := execRoot(t, "config", "check")
	if err == nil || !strings.Contains(err.Error(), "broken.toml") || !strings.Contains(err.Error(), "odd.toml") {
		t.Errorf("config check: %v", err)
	}
	var b bytes.Buffer
	if err := runDoctor(&b); err == nil {
		t.Error("the doctor passed")
	}
	if !strings.Contains(b.String(), "broken.toml") || !strings.Contains(b.String(), `category "maintenance"`) {
		t.Errorf("the doctor does not list the files:\n%s", b.String())
	}
	if info := doctorInfo(); len(info.CustomStepProblems) != 2 || info.OK {
		t.Errorf("doctor --json: %+v", info.CustomStepProblems)
	}
}

// ---- json ----

func TestStepsAndDoctorJSON(t *testing.T) {
	isolate(t, nil)
	i18n.Init("es") // the stable fields do not follow the language
	t.Cleanup(func() { i18n.Init("en") })
	root := testRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"steps", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var list []map[string]any
	if err := json.Unmarshal(out.Bytes(), &list); err != nil {
		t.Fatalf("steps --json is not one JSON document: %v\n%s", err, out.String())
	}
	if len(list) != len(steps.All) {
		t.Fatalf("%d steps, want %d", len(list), len(steps.All))
	}
	cats := map[string]bool{}
	for _, s := range list {
		for _, key := range []string{"id", "name", "category", "risk", "installed", "skipped", "category_id", "risk_id", "command", "available"} {
			if _, ok := s[key]; !ok {
				t.Fatalf("step %v has no %q", s["id"], key)
			}
		}
		cats[s["category_id"].(string)] = true
		if r := s["risk_id"].(string); r != "read-only" && r != "normal" && r != "dangerous" {
			t.Errorf("%v: risk_id %q", s["id"], r)
		}
	}
	for c := range cats {
		if c != steps.Check && c != steps.Update && c != steps.Clean && c != steps.Backend {
			t.Errorf("category_id %q is not a category", c)
		}
	}

	// No key: the doctor fails, says so in the document and by its status.
	root = testRoot()
	out.Reset()
	root.SetOut(&out)
	root.SetArgs([]string{"doctor", "--json"})
	err := root.Execute()
	var code interface{ ExitCode() int }
	if !errors.As(err, &code) || code.ExitCode() != 1 || err.Error() != "" {
		t.Errorf("doctor --json with no key: %v", err)
	}
	var doc struct {
		OK      *bool    `json:"ok"`
		Lines   []string `json:"lines"`
		Version *string  `json:"version"`
		Config  struct {
			Path string `json:"path"`
			OK   bool   `json:"ok"`
		} `json:"config"`
		Provider struct {
			Name string `json:"name"`
			OK   bool   `json:"ok"`
		} `json:"provider"`
		Steps []map[string]any `json:"steps"`
	}
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("doctor --json is not one JSON document: %v\n%s", err, out.String())
	}
	if doc.OK == nil || *doc.OK || doc.Version == nil || len(doc.Lines) == 0 || len(doc.Steps) != len(steps.All) {
		t.Errorf("doctor --json: %s", out.String())
	}
	if !doc.Config.OK || doc.Config.Path != config.Path() || doc.Provider.Name != "anthropic" || doc.Provider.OK {
		t.Errorf("doctor --json facts: %+v %+v", doc.Config, doc.Provider)
	}
}

// The Homebrew formula asks for the completion scripts as
// `omega completion completion <shell>`. v0.26.0 refused that and
// `brew upgrade omega` failed; v0.25.0 printed the help instead of a script.
func TestCompletionAsHomebrewAsksForIt(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		stdout := os.Stdout
		os.Stdout = w
		_, runErr := execRoot(t, "completion", "completion", shell)
		os.Stdout = stdout
		w.Close()
		script, _ := io.ReadAll(r)
		if runErr != nil {
			t.Errorf("%s: %v", shell, runErr)
		}
		if len(script) < 200 || strings.Contains(string(script), "Usage:") {
			t.Errorf("%s: not a completion script:\n%.200s", shell, script)
		}
	}
	for _, args := range [][]string{{"completion", "bogus"}, {"completion", "completion", "install"}, {"completion", "completion", "nope"}} {
		if _, err := execRoot(t, args...); err == nil {
			t.Errorf("omega %v: accepted", args)
		}
	}
}
