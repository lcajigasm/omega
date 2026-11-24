package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/steps"
)

func TestSetConfigLanguageTakesEffectAtOnce(t *testing.T) {
	isolate(t, nil)
	t.Setenv("OMEGA_LANG", "")
	t.Cleanup(func() { i18n.Init("en") })
	if err := SetConfig("language", "de"); err != nil {
		t.Fatal(err)
	}
	if i18n.Lang() != "de" {
		t.Errorf("language in force: %s", i18n.Lang())
	}
	if s, _ := GetSettings(); s.Language != "de" || s.Model != "auto" || !s.Catalog {
		t.Errorf("settings: %+v", s)
	}
	if err := SetConfig("language", "xx"); err == nil {
		t.Error("an unknown language was accepted")
	}
	if err := SetConfig("no_such_key", "1"); err == nil {
		t.Error("an unknown key was accepted")
	}
}

func TestSkipStep(t *testing.T) {
	isolate(t, nil)
	id := steps.All[0].ID
	for _, skip := range []bool{true, true, false} {
		if err := SkipStep(id, skip); err != nil {
			t.Fatal(err)
		}
		cfg, _ := config.Load()
		if n := len(cfg.SkipSteps); cfg.Skipped(id) != skip || n > 1 {
			t.Fatalf("skip=%v: skip_steps %v", skip, cfg.SkipSteps)
		}
	}
	if err := SkipStep("no-such-step", true); err == nil {
		t.Error("an unknown step was accepted")
	}
	list, err := StepList()
	if err != nil || len(list) != len(steps.All) {
		t.Fatalf("steps: %d, %v", len(list), err)
	}
}

func TestSetModel(t *testing.T) {
	isolate(t, nil)
	if err := SetModel("claude-opus-5-5"); err != nil {
		t.Fatal(err)
	}
	if s, _ := GetSettings(); s.Model != "claude-opus-5-5" {
		t.Errorf("fixed: %q", s.Model)
	}
	if err := SetModel("auto"); err != nil {
		t.Fatal(err)
	}
	if cfg, _ := config.Load(); cfg.Model != "" || cfg.Providers[cfg.Provider].Model != "" {
		t.Errorf("auto left a model: %+v", cfg)
	}
}

func TestRunsAndNotes(t *testing.T) {
	isolate(t, nil)
	if runs, err := RunList(); err != nil || len(runs) != 0 || LastRun() != "" {
		t.Fatalf("no runs yet: %v %v", runs, err)
	}
	if err := os.MkdirAll(agent.RunsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "omega run 2026-11-10T10:00:00Z — anthropic / claude-haiku-5-5\n\n$ df -h\n  disk\n"
	if err := os.WriteFile(filepath.Join(agent.RunsDir(), "2026-11-10_100000.log"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	runs, err := RunList()
	if err != nil || len(runs) != 1 || runs[0].Title != "anthropic / claude-haiku-5-5" || runs[0].Ago == "" {
		t.Fatalf("runs: %+v %v", runs, err)
	}
	if text, err := RunText(runs[0].Name); err != nil || text != body {
		t.Errorf("text: %q %v", text, err)
	}
	if _, err := RunText("../../etc/passwd"); err == nil {
		t.Error("a file outside the runs was read")
	}

	if Notes() != "" {
		t.Error("notes before any were written")
	}
	os.MkdirAll(config.Dir(), 0o755)
	os.WriteFile(config.NotesPath(), []byte("pipx needs Python 3.13\n"), 0o644)
	if Notes() != "pipx needs Python 3.13" {
		t.Errorf("notes: %q", Notes())
	}
	if err := ClearNotes(); err != nil || Notes() != "" {
		t.Errorf("clear: %v %q", err, Notes())
	}
	if err := ClearNotes(); err != nil {
		t.Errorf("clearing nothing: %v", err)
	}
}

// The app draws the steps itself, so its check leaves them out.
func TestRunCheckLeavesTheStepsOut(t *testing.T) {
	isolate(t, map[string]string{})
	c := RunCheck()
	if len(c.Lines) == 0 {
		t.Fatal("no lines")
	}
	if slices.ContainsFunc(c.Lines, func(l string) bool { return strings.Contains(l, steps.All[0].ID) }) {
		t.Errorf("the steps are listed: %v", c.Lines)
	}
}

func TestGUIRoutines(t *testing.T) {
	isolate(t, nil)
	a, b := steps.All[0].ID, steps.All[1].ID
	if rs, err := Routines(); err != nil || len(rs.Routines) != 0 || rs.Default != "" {
		t.Fatalf("no routines yet: %+v %v", rs, err)
	}
	if err := SaveRoutine("", []string{a, b}, []string{b, "x-not-in-routine"}, 0, nil); err == nil {
		t.Fatal("an unknown step was accepted")
	}
	if err := SaveRoutine("", []string{a, b}, []string{b}, 0, nil); err != nil {
		t.Fatal(err)
	}
	if err := SaveRoutine("weekly", []string{b}, []string{a}, 25, []string{"fri", "mon"}); err != nil {
		t.Fatal(err)
	}
	if err := SaveRoutine("weekly", []string{b}, nil, 0, []string{"someday"}); err == nil {
		t.Error("a bad day was saved")
	}
	for _, bad := range []func() error{
		func() error { return SaveRoutine("bad name", []string{a}, nil, 0, nil) },
		func() error { return SaveRoutine("x", nil, nil, 0, nil) },
		func() error { return SaveRoutine("x", []string{a}, nil, -1, nil) },
	} {
		if bad() == nil {
			t.Error("a bad routine was saved")
		}
	}
	h := customHistory{}
	h.add(a, customRecord{When: time.Now(), Took: 3 * time.Second, OK: true, Freed: 3 << 30})
	if err := h.save(); err != nil {
		t.Fatal(err)
	}
	rs, err := Routines()
	if err != nil {
		t.Fatal(err)
	}
	if rs.Default != config.DefaultRoutine || len(rs.Routines) != 2 || rs.Routines[0].Name != config.DefaultRoutine || !rs.Routines[0].Default {
		t.Fatalf("routines: %+v", rs)
	}
	def, weekly := rs.Routines[0], rs.Routines[1]
	if len(def.Steps) != 2 || def.Steps[0].ID != a || def.Steps[0].LastRun != "3s" || def.Steps[0].Freed != 3<<30 || def.Steps[0].RiskID == "" || !def.Steps[1].Stops {
		t.Errorf("default: %+v", def)
	}
	if weekly.OnlyBelowFreeGB != 25 || len(weekly.StopOnFail) != 0 || weekly.Options == "" || weekly.Default || !slices.Equal(weekly.Days, []string{"mon", "fri"}) || len(def.Days) != 0 {
		t.Errorf("weekly (a stop for a step outside the routine is dropped): %+v", weekly)
	}
}
