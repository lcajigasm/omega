package customsteps

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/runner"
	"github.com/lcajigasm/omega/internal/safety"
)

func stepsDir(t *testing.T, files map[string]string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	i18n.Init("en")
	dir := filepath.Join(config.Dir(), "steps.d")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// A file that does not parse and a step omega cannot place are reported by
// file name; the usable steps still load.
func TestLoadReportsWhatItLeavesOut(t *testing.T) {
	stepsDir(t, map[string]string{
		"a-good.toml": "[[steps]]\nid = \"mine\"\nname = \"Mine\"\ncategory = \"check\"\ncommand = \"echo hi\"\nrisk = \"read-only\"\n\n" +
			"[[steps]]\nid = \"plain\"\ncommand = \"echo\"\n",
		"b-syntax.toml":   "[[steps]\nid = ",
		"c-category.toml": "[[steps]]\nid = \"odd\"\ncategory = \"maintenance\"\ncommand = \"echo\"\n",
		"d-risk.toml":     "[[steps]]\nid = \"risky\"\ncommand = \"echo\"\nrisk = \"harmless\"\n",
		"e-empty.toml":    "[[steps]]\nname = \"no id\"\n",
	})
	loaded, problems := Load()
	if len(loaded) != 2 || loaded[0].ID != "mine" || loaded[0].Risk != safety.Safe || loaded[1].Category != "clean" || loaded[1].Risk != safety.Normal {
		t.Errorf("loaded: %+v", loaded)
	}
	if len(problems) != 4 {
		t.Fatalf("problems: %q", problems)
	}
	for i, want := range []string{"b-syntax.toml", "c-category.toml", "d-risk.toml", "e-empty.toml"} {
		if !strings.Contains(problems[i], want) {
			t.Errorf("problem %d does not name %s: %s", i, want, problems[i])
		}
	}
	if !strings.Contains(problems[1], `"odd"`) || !strings.Contains(problems[1], `"maintenance"`) || !strings.Contains(problems[1], "check, update, clean, backend") {
		t.Errorf("the category problem does not say what is wrong: %s", problems[1])
	}
}

func TestNoStepsDirIsNotAProblem(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if loaded, problems := Load(); len(loaded) != 0 || len(problems) != 0 {
		t.Errorf("%v %v", loaded, problems)
	}
	if err := RunHook(context.Background(), "pre_full"); err != nil {
		t.Error(err)
	}
}

// A hook that fails is an error that names it.
func TestRunHook(t *testing.T) {
	stepsDir(t, map[string]string{
		"a.toml": "[hooks]\npre_update = \"first\"\npost_update = \"fails\"\n",
		"b.toml": "[hooks]\npre_update = \"second\"\n",
	})
	old := run
	t.Cleanup(func() { run = old })
	var ran []string
	run = func(_ context.Context, command string, _ time.Duration, _ func(string)) runner.Result {
		ran = append(ran, command)
		if command == "fails" {
			return runner.Result{ExitCode: 3, Output: "no disk\n"}
		}
		return runner.Result{}
	}
	if hooks := Hooks("pre_update"); len(hooks) != 2 || hooks[0].Command != "first" || !strings.HasSuffix(hooks[1].File, "b.toml") {
		t.Errorf("hooks: %+v", hooks)
	}
	if len(ran) != 0 {
		t.Fatalf("listing the hooks ran them: %v", ran)
	}
	if err := RunHook(context.Background(), "pre_update"); err != nil || strings.Join(ran, ",") != "first,second" {
		t.Errorf("pre_update: %v, ran %v", err, ran)
	}
	err := RunHook(context.Background(), "post_update")
	if err == nil || !strings.Contains(err.Error(), "post_update") || !strings.Contains(err.Error(), "exit 3") || !strings.Contains(err.Error(), "no disk") {
		t.Errorf("post_update: %v", err)
	}
	if err := RunHook(context.Background(), "pre_clean"); err != nil {
		t.Errorf("a hook nobody configured: %v", err)
	}
}
