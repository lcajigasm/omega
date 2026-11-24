package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/safety"
	"github.com/lcajigasm/omega/internal/steps"
	"github.com/lcajigasm/omega/internal/ui"
)

// A custom.toml from before routines had names is read as "default", and
// saving another routine keeps it.
func TestRoutinesMigrateAndKeepEachOther(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeFile(t, config.CustomPath(), "steps = [\"brew\", \"npm\"]\n")
	rs, ok, err := config.LoadRoutines()
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	if r, ok := rs.Get(""); !ok || !slices.Equal(r.Steps, []string{"brew", "npm"}) || rs.DefaultName() != config.DefaultRoutine {
		t.Fatalf("old file not read as the default routine: %+v", rs)
	}
	if err := config.SaveNamedRoutine("weekly", config.Routine{Steps: []string{"disk"}, StopOnFail: []string{"disk"}, OnlyBelowFreeGB: 30}); err != nil {
		t.Fatal(err)
	}
	rs, _, _ = config.LoadRoutines()
	if !slices.Equal(rs.Names(), []string{"default", "weekly"}) {
		t.Fatalf("names: %v", rs.Names())
	}
	w := rs.Routines["weekly"]
	if !w.Stops("disk") || w.OnlyBelowFreeGB != 30 {
		t.Fatalf("options lost: %+v", w)
	}
	b, _ := os.ReadFile(config.CustomPath())
	if !strings.Contains(string(b), "[routines.weekly]") || strings.Contains(string(b), "\nsteps = [\"brew\"") {
		t.Errorf("file not in the named form:\n%s", b)
	}
	if config.ValidRoutineName("a b") || config.ValidRoutineName("") || !config.ValidRoutineName("semanal-2") {
		t.Error("routine names are not checked")
	}
}

// The command line: --set with a name, --list, --default, --show --json.
func TestCustomNamedRoutinesCommandLine(t *testing.T) {
	isolate(t, nil)
	id := steps.All[0].ID
	if out, err := execRoot(t, "custom", "--set", "weekly", id, "--stop-on-fail", id); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if out, err := execRoot(t, "custom", "--set", id); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	out, err := execRoot(t, "custom", "--list")
	if err != nil || !strings.Contains(out, "weekly") || !strings.Contains(out, "default") {
		t.Fatalf("list: %v\n%s", err, out)
	}
	if _, err := execRoot(t, "custom", "weekly", "--default"); err != nil {
		t.Fatal(err)
	}
	rs, _, _ := config.LoadRoutines()
	if rs.DefaultName() != "weekly" {
		t.Fatalf("default is %q", rs.DefaultName())
	}
	out, err = execRoot(t, "custom", "--show", "--json")
	var doc struct {
		Name       string   `json:"name"`
		Steps      []string `json:"steps"`
		StopOnFail []string `json:"stop_on_fail"`
	}
	if err != nil || json.Unmarshal([]byte(out), &doc) != nil || doc.Name != "weekly" || !slices.Equal(doc.StopOnFail, []string{id}) {
		t.Fatalf("show --json: %v\n%s", err, out)
	}
	if _, err := execRoot(t, "custom", "nosuch"); err == nil || usageCode(err) != exitUsage {
		t.Errorf("an unknown routine: %v", err)
	}
	if _, err := execRoot(t, "custom", "--set", "weekly", "--only-below-free-gb", "-1"); err == nil {
		t.Error("a negative space limit was taken")
	}
}

// A step whose failure stops the routine stops it; the steps after it do
// not run. The run is recorded for the editor.
func TestCustomRunStopsOnFail(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	var out bytes.Buffer
	c, ran := fakeCustomRun(&out, true, true, map[string]int{"false": 2})
	c.routine = config.Routine{StopOnFail: []string{"break"}}
	c.history = customHistory{}
	if err := c.run(context.Background(), []string{"look", "break", "up"}); err == nil {
		t.Fatal("a failed step must fail the run")
	}
	if slices.Contains(*ran, "echo up") {
		t.Fatalf("a step ran after the one that stops the routine: %v", *ran)
	}
	if !strings.Contains(out.String(), "ends the routine") {
		t.Errorf("the stop is not said:\n%s", out.String())
	}
	h := loadCustomHistory()
	if r, ok := h.last("look"); !ok || r.Took != 2*time.Second {
		t.Errorf("the run of look was not recorded: %+v", h)
	}
	if rs := routineStep(steps.Step{ID: "look", Name: "Look"}, h); rs.LastRun != 2*time.Second || strings.Contains(rs.Label, "2s") {
		t.Errorf("the editor does not get the last duration in its own field: %+v", rs)
	}
}

// With more free space than the routine's limit, nothing runs and that is
// not a failure.
func TestCustomRunFreeSpaceGuard(t *testing.T) {
	var out bytes.Buffer
	c, ran := fakeCustomRun(&out, true, true, nil)
	c.routine = config.Routine{OnlyBelowFreeGB: 20}
	c.free = func() (uint64, bool) { return 50e9, true }
	if err := c.run(context.Background(), []string{"look", "up"}); err != nil || len(*ran) != 0 {
		t.Fatalf("err %v ran %v", err, *ran)
	}
	c.free = func() (uint64, bool) { return 5e9, true }
	if err := c.run(context.Background(), []string{"look", "up"}); err != nil || len(*ran) != 2 {
		t.Fatalf("below the limit: err %v ran %v", err, *ran)
	}
}

// A9: a step's own command decides too, for omega's steps as for the
// user's: mo purge --yes always asks, whatever the catalogue says. Only a
// command that always asks because of sudo alone is trusted, and only for
// omega's own steps.
func TestNeedsAnswerFollowsTheCommand(t *testing.T) {
	cfg := config.Default()
	purge := steps.Step{ID: "mole-purge", Command: "mo purge --yes", Risk: safety.Dangerous}
	if safety.AlwaysConfirm(purge.Command) && !needsAnswer(cfg, purge, true) {
		t.Error("mo purge --yes ran on --yes")
	}
	script := steps.Step{ID: "omz", Command: "sh ~/.oh-my-zsh/tools/upgrade.sh", Risk: safety.Normal}
	if safety.AlwaysConfirm(script.Command) && !needsAnswer(cfg, script, true) {
		t.Error("a script from $HOME ran on --yes")
	}
	// The macOS update: sudo is its only reason to always ask.
	update := steps.Step{ID: "brew", Command: "sudo softwareupdate --install --all", Risk: safety.Dangerous}
	if !onlySudoAsks(update.Command) && safety.AlwaysConfirm(update.Command) {
		t.Skip("the policy now asks for softwareupdate itself")
	}
	if needsAnswer(cfg, update, true) {
		t.Error("a built-in update that needs sudo always asks: the scheduled update would stop working")
	}
	if onlySudoAsks("mo purge --yes") || onlySudoAsks("sudo mo purge --yes") && safety.AlwaysConfirm("mo purge --yes") {
		t.Error("a command that asks without sudo too was taken for sudo only")
	}
	own := steps.Step{ID: "my-own-step", Command: update.Command, Risk: safety.Dangerous}
	if !needsAnswer(cfg, own, true) {
		t.Error("a step from steps.d that runs sudo must always ask")
	}
	// Every built-in step that always asks by its command is asked, unless
	// sudo is the only reason.
	for _, s := range steps.All {
		if safety.AlwaysConfirm(s.Command) && !onlySudoAsks(s.Command) && !needsAnswer(cfg, s, true) {
			t.Errorf("%s: its command always asks, but --yes answers for it", s.ID)
		}
	}
}

// The default routine is a task of the runs without the AI, for the job
// `omega schedule install --task custom` creates.
func TestCustomAsADeterministicTask(t *testing.T) {
	isolate(t, nil)
	if !hasDeterministic(taskCustom) {
		t.Fatal("custom is not a task without the AI")
	}
	d, ran, _, out := newDet(runFlags{yes: true, dryRun: true}, false, true)
	if err := d.run(context.Background(), taskCustom); err != nil {
		t.Fatal(err)
	}
	if len(*ran) != 0 || !strings.Contains(out.String(), "omega custom --set") {
		t.Errorf("no routine: ran %v\n%s", *ran, out.String())
	}
	// A scheduled job passes --no-ai and --no-tui; custom takes them.
	if _, err := execRoot(t, "custom", "--yes", "--no-tui", "--no-ai", "--scheduled"); err != nil {
		t.Errorf("the scheduled command line: %v", err)
	}
}

// Each step's run records how much free space the disk gained over it, and
// the editor gets it in its own field.
func TestCustomRunRecordsFreedSpace(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	var out bytes.Buffer
	c, _ := fakeCustomRun(&out, true, true, nil)
	c.history = customHistory{}
	// look: 10 GB -> 12 GB; up: 12 GB -> 11 GB.
	free := []uint64{10e9, 12e9, 12e9, 11e9}
	c.free = func() (uint64, bool) { v := free[0]; free = free[1:]; return v, true }
	if err := c.run(context.Background(), []string{"look", "up"}); err != nil {
		t.Fatal(err)
	}
	h := loadCustomHistory()
	if r, _ := h.last("look"); r.Freed != 2e9 {
		t.Errorf("look freed %d", r.Freed)
	}
	if r, _ := h.last("up"); r.Freed != -1e9 {
		t.Errorf("up freed %d", r.Freed)
	}
	if rs := routineStep(steps.Step{ID: "look"}, h); rs.Freed != 2e9 {
		t.Errorf("editor step: %+v", rs)
	}
	if rs := routineStep(steps.Step{ID: "up"}, h); rs.Freed != 0 {
		t.Errorf("a step that took space shows it freed some: %+v", rs)
	}
}

// The machine data is aggregated and goes to the model only when asked.
func TestRoutineDataAndPrompt(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	h := customHistory{
		"brew": {{When: now.Add(-72 * time.Hour), OK: false}, {When: now.Add(-200 * time.Hour), OK: true}},
		"npm":  {{When: now.Add(-100 * time.Hour), OK: true}},
		"disk": {{When: now.Add(-300 * time.Hour), OK: true}, {When: now.Add(-400 * time.Hour), OK: false}},
	}
	d := gatherRoutineData(h, func() (uint64, bool) { return 41_600_000_000, true }, now)
	if d.FreeGB != 42 || d.Days != 3 || !slices.Equal(d.Failed, []string{"brew", "disk"}) {
		t.Fatalf("data: %+v", d)
	}
	if w := d.words(); !strings.Contains(w, "42 GB free") || !strings.Contains(w, "3 days") || !strings.Contains(w, "brew, disk") {
		t.Errorf("words: %q", w)
	}
	none := gatherRoutineData(customHistory{}, nil, now)
	if none.FreeGB != -1 || none.Days != -1 || len(none.Failed) != 0 || !strings.Contains(none.words(), "never") {
		t.Errorf("no data: %+v %q", none, none.words())
	}
	all := []steps.Step{{ID: "look", Name: "Look", Category: "check", Risk: safety.Safe}}
	plain, installed := routinePrompt(all, []string{"look"}, nil)
	if !installed["look"] || strings.Contains(plain, "free space") || strings.Contains(plain, "failed") {
		t.Errorf("the plain prompt carries machine data:\n%s", plain)
	}
	with, _ := routinePrompt(all, []string{"look"}, &d)
	for _, want := range []string{"free space on the system disk: 42 GB", "days since omega custom last ran: 3", "failed in recent runs: brew, disk"} {
		if !strings.Contains(with, want) {
			t.Errorf("prompt lacks %q:\n%s", want, with)
		}
	}
}

// What the editor saved keeps the steps this machine lacks, with their
// stop_on_fail.
func TestKeepMissing(t *testing.T) {
	old := config.Routine{Steps: []string{"no-such-step", steps.All[0].ID}, StopOnFail: []string{"no-such-step"}}
	ids, stops, gb := keepMissing(old, ui.RoutineChoice{Steps: []string{steps.All[0].ID}, StopOnFail: []string{steps.All[0].ID}, OnlyBelowFreeGB: 20})
	if !slices.Equal(ids, []string{steps.All[0].ID, "no-such-step"}) || !slices.Equal(stops, []string{steps.All[0].ID, "no-such-step"}) || gb != 20 {
		t.Fatalf("ids %v stops %v gb %v", ids, stops, gb)
	}
}

// On a day the routine does not list, a scheduled run does nothing and is
// not a failure; a run started by hand warns and runs.
func TestCustomRunDays(t *testing.T) {
	wed := time.Date(2026, 11, 25, 10, 0, 0, 0, time.Local)
	var out bytes.Buffer
	c, ran := fakeCustomRun(&out, true, true, nil)
	c.routine = config.Routine{Days: []string{"mon", "fri"}}
	c.now = func() time.Time { return wed }
	c.scheduled = true
	if err := c.run(context.Background(), []string{"look"}); err != nil || len(*ran) != 0 {
		t.Fatalf("scheduled on another day: err %v ran %v", err, *ran)
	}
	if !strings.Contains(out.String(), "Today is wed") || !strings.Contains(out.String(), "mon, fri") {
		t.Errorf("the skip is not said:\n%s", out.String())
	}
	out.Reset()
	c.scheduled = false
	if err := c.run(context.Background(), []string{"look"}); err != nil || len(*ran) != 1 {
		t.Fatalf("by hand: err %v ran %v", err, *ran)
	}
	if !strings.Contains(out.String(), "it runs now because") {
		t.Errorf("no warning by hand:\n%s", out.String())
	}
	out.Reset()
	c.scheduled, c.routine.Days = true, []string{"wed"}
	if err := c.run(context.Background(), []string{"look"}); err != nil || len(*ran) != 2 || strings.Contains(out.String(), "Today is") {
		t.Fatalf("on its day: err %v ran %v\n%s", err, *ran, out.String())
	}
}

// --days sets and clears the days of a routine, alone or with the steps,
// and a word that is not a day is a usage error.
func TestCustomDaysFlag(t *testing.T) {
	isolate(t, nil)
	id := steps.All[0].ID
	if out, err := execRoot(t, "custom", "--set", "weekly", id, "--days", "fri,lun"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	rs, _, _ := config.LoadRoutines()
	if !slices.Equal(rs.Routines["weekly"].Days, []string{"mon", "fri"}) {
		t.Fatalf("days: %+v", rs.Routines["weekly"])
	}
	out, err := execRoot(t, "custom", "--list")
	if err != nil || !strings.Contains(out, "only on mon, fri") {
		t.Fatalf("list: %v\n%s", err, out)
	}
	if out, err := execRoot(t, "custom", "weekly", "--show"); err != nil || !strings.Contains(out, "only on mon, fri") {
		t.Fatalf("show: %v\n%s", err, out)
	}
	if _, err := execRoot(t, "custom", "--set", "weekly", "--days", "someday"); err == nil || usageCode(err) != exitUsage {
		t.Errorf("a bad day: %v", err)
	}
	if out, err := execRoot(t, "custom", "--set", "weekly", "--days", ""); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	rs, _, _ = config.LoadRoutines()
	if w := rs.Routines["weekly"]; len(w.Days) != 0 || !slices.Equal(w.Steps, []string{id}) {
		t.Fatalf("after clearing: %+v", w)
	}
}
