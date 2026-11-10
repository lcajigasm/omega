package cli

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/steps"
	"github.com/lcajigasm/omega/internal/ui"
)

// fullFixture is the stages of a machine: its system, the tools it has and
// whether omega ran there before.
func fullFixture(t *testing.T, os string, earlierRuns bool, tools ...string) []fullStage {
	t.Helper()
	i18n.Init("en")
	oldOS, oldHas, oldRuns := goos, hasTool, hasRuns
	t.Cleanup(func() { goos, hasTool, hasRuns = oldOS, oldHas, oldRuns })
	goos = os
	hasTool = func(bin string) bool {
		for _, t := range tools {
			if t == bin {
				return true
			}
		}
		return false
	}
	hasRuns = func() bool { return earlierRuns }
	return stagesHere()
}

func stageIDs(stages []fullStage) string {
	var ids []string
	for _, s := range stages {
		ids = append(ids, s.id)
	}
	return strings.Join(ids, " ")
}

func guideStages(stages []fullStage, chosen map[string]bool) []ui.GuideStage {
	var out []ui.GuideStage
	for _, s := range stages {
		out = append(out, guideStage(s, chosen == nil || chosen[s.id]))
	}
	return out
}

func TestFullStagesFitTheMachine(t *testing.T) {
	if got := stageIDs(fullFixture(t, "darwin", true, "docker", "topgrade", "mo")); got != "doctor analyze health update clean docker tune dev mac fix history" {
		t.Errorf("a Mac with Docker, topgrade and Mole that omega has run on: %s", got)
	}
	if got := stageIDs(fullFixture(t, "linux", false)); got != "doctor analyze health update clean tune dev" {
		t.Errorf("a new Linux machine without Docker: %s", got)
	}
	// Mole is a Mac tool: finding a binary called mo elsewhere repairs nothing.
	if got := stageIDs(fullFixture(t, "windows", true, "docker", "mo")); got != "doctor analyze health update clean docker tune dev history" {
		t.Errorf("Windows with Docker: %s", got)
	}
	if got := stageIDs(fullFixture(t, "linux", false, "topgrade")); !strings.HasSuffix(got, "dev fix") {
		t.Errorf("Linux with topgrade: %s", got)
	}
}

// Every stage starts something omega has, says how to start it alone, and is
// explained in every language.
func TestFullStagesAreRealAndExplained(t *testing.T) {
	root := testRoot()
	for _, s := range fullStages {
		if s.task == "" && s.report == nil {
			t.Errorf("%s starts nothing", s.id)
		}
		if s.task != "" {
			if _, ok := agent.TaskPrompts[s.task]; !ok {
				t.Errorf("%s starts the task %q, which has no prompt", s.id, s.task)
			}
		}
		switch {
		case strings.HasPrefix(s.command, "omega "):
			words := strings.Fields(s.command)[1:]
			if c, rest, err := root.Find(words); err != nil {
				t.Errorf("%s: %v", s.id, err)
			} else if c == root {
				if _, ok := taskFor(rest[0]); !ok {
					t.Errorf("%s shows `%s`, which is neither a command nor a task", s.id, s.command)
				}
			}
		case strings.HasPrefix(s.command, "/"):
			found := false
			for _, name := range ui.SlashCommands() {
				found = found || name == s.command
			}
			if !found {
				t.Errorf("%s shows %s, which no session understands", s.id, s.command)
			}
		default:
			t.Errorf("%s: %q is not something the user can type", s.id, s.command)
		}
		for _, lang := range i18n.Supported {
			i18n.Init(lang)
			g := guideStage(s, true)
			for name, text := range map[string]string{"title": g.Title, "what": g.What, "why": g.Why} {
				if text == "" || strings.HasPrefix(text, "full.s.") {
					t.Errorf("%s has no %s in %s", s.id, name, lang)
				}
			}
		}
	}
	i18n.Init("en")
}

// The guided run covers every kind of maintenance the catalogue has: what
// the health stage asks for exists as steps on every system.
func TestFullCoversHealthOnEverySystem(t *testing.T) {
	for _, goos := range []string{"darwin", "linux", "windows"} {
		have := map[string]bool{}
		for _, s := range steps.For(goos) {
			have[s.ID] = true
		}
		for _, id := range []string{"health", "security", "system-logs", "disk"} {
			if !have[id] {
				t.Errorf("%s has no %s step", goos, id)
			}
		}
	}
	for _, task := range []string{agent.TaskHealth, agent.TaskDocker, agent.TaskHistory, agent.TaskFix} {
		if _, ok := taskFor(task); !ok && (task == agent.TaskHealth || task == agent.TaskFix) {
			t.Errorf("`omega %s` is not a task", task)
		}
	}
}

func TestFullOnlyAndSkip(t *testing.T) {
	stages := fullFixture(t, "darwin", true, "docker")
	chosen, err := pickStages(stages, nil, []string{"tune,dev", " Mac "})
	if err != nil {
		t.Fatal(err)
	}
	if chosen["tune"] || chosen["dev"] || chosen["mac"] || !chosen["update"] || !chosen["health"] {
		t.Errorf("--skip: %v", chosen)
	}
	chosen, err = pickStages(stages, []string{"update", "clean"}, []string{"clean"})
	if err != nil {
		t.Fatal(err)
	}
	for id, on := range chosen {
		if on != (id == "update") {
			t.Errorf("--only update,clean --skip clean: %s is %v", id, on)
		}
	}
	// A typo must not run everything.
	if _, err := pickStages(stages, nil, []string{"updaet"}); err == nil || !strings.Contains(err.Error(), "updaet") || !strings.Contains(err.Error(), "update") {
		t.Errorf("unknown stage: %v", err)
	}
	// A stage this machine lacks is a name omega knows: skipping it is fine.
	if _, err := pickStages(fullFixture(t, "linux", false), nil, []string{"mac"}); err != nil {
		t.Errorf("skipping a stage of another system: %v", err)
	}
}

func TestFullRunsInOrderAndCanSkipRepeatOrStop(t *testing.T) {
	stages := fullFixture(t, "darwin", true, "docker")
	const order = "doctor analyze health update clean docker tune dev mac history"
	var ran []string
	run := func(id string) error { ran = append(ran, id); return nil }
	status := func(res []ui.StageResult) map[string]string {
		out := map[string]string{}
		for _, r := range res {
			out[r.ID] = r.Status
		}
		return out
	}
	var b strings.Builder

	// Enter every time: everything runs, in order, each stage explained.
	res := runFull(&b, guideStages(stages, nil), run, func() string { return "\n" }, nil)
	if strings.Join(ran, " ") != order {
		t.Fatalf("ran %v", ran)
	}
	for id, st := range status(res) {
		if st != ui.StageDone {
			t.Errorf("%s: %s", id, st)
		}
	}
	out := b.String()
	for _, want := range []string{"Stage 1 of 10 · Check omega · omega doctor", "Stage 10 of 10 · Loose ends", "What it does: ", "Why: ",
		"3 of 10 stages done. Still to come: Update, Clean up", "Next: Update (omega update)", "[s] skip it"} {
		if !strings.Contains(out, want) {
			t.Errorf("the output lacks %q:\n%s", want, out)
		}
	}

	// Unticked: never run, reported as skipped, and not counted.
	ran = nil
	b.Reset()
	res = runFull(&b, guideStages(stages, map[string]bool{"doctor": true, "update": true, "clean": true}), run, func() string { return "" }, nil)
	if st := status(res); strings.Join(ran, " ") != "doctor update clean" || st["analyze"] != ui.StageSkipped || st["mac"] != ui.StageSkipped {
		t.Fatalf("ran %v result %v", ran, st)
	}
	if !strings.Contains(b.String(), "Stage 2 of 3 · Update") {
		t.Errorf("stages are numbered among the ones that run:\n%s", b.String())
	}

	// "s" skips the next stage (twice in a row skips two), "q" stops.
	ran = nil
	answers := []string{"s", "S ", "", "q"}
	res = runFull(io.Discard, guideStages(stages, nil), run, func() string { a := answers[0]; answers = answers[1:]; return a }, nil)
	if strings.Join(ran, " ") != "doctor update" {
		t.Fatalf("ran %v", ran)
	}
	want := map[string]string{"doctor": ui.StageDone, "analyze": ui.StageSkipped, "health": ui.StageSkipped, "update": ui.StageDone,
		"clean": ui.StagePending, "docker": ui.StagePending, "mac": ui.StagePending}
	for id, st := range want {
		if got := status(res)[id]; got != st {
			t.Errorf("%s: %s, want %s", id, got, st)
		}
	}

	// A stage that ends with an error is recorded, "r" repeats it, and the
	// run goes on.
	ran = nil
	fails := 1
	run = func(id string) error {
		ran = append(ran, id)
		if id == "update" && fails > 0 {
			fails--
			return errors.New("no API key")
		}
		return nil
	}
	b.Reset()
	asked := 0
	res = runFull(&b, guideStages(stages, nil), run, func() string {
		asked++
		if asked == 4 { // after update failed
			return "r"
		}
		return ""
	}, nil)
	if got := strings.Join(ran, " "); got != strings.Replace(order, "update", "update update", 1) {
		t.Fatalf("ran %s", got)
	}
	if status(res)["update"] != ui.StageDone {
		t.Errorf("the repeated stage ended well: %v", status(res))
	}
	if out := b.String(); !strings.Contains(out, "✗ Update · ended with an error · no API key") || !strings.Contains(out, "[r] repeat the last one") {
		t.Errorf("failure:\n%s", out)
	}

	// Unattended: nobody is asked, a failure does not stop the run, ctrl+c does.
	ran = nil
	run = func(id string) error {
		ran = append(ran, id)
		if id == "analyze" {
			return exitCode(1)
		}
		return nil
	}
	res = runFull(io.Discard, guideStages(stages, nil), run, nil, func() bool { return len(ran) == 4 })
	if st := status(res); strings.Join(ran, " ") != "doctor analyze health update" || st["analyze"] != ui.StageFailed || st["update"] != ui.StageStopped || st["clean"] != ui.StagePending {
		t.Fatalf("ran %v result %v", ran, st)
	}
	if !fullFailed(res) {
		t.Error("an unattended run with a failed stage must not exit 0")
	}
	b.Reset()
	fullSummary(&b, guideStages(stages, nil), res)
	for _, want := range []string{"✓ Check omega", "✗ State report", "ended with an error", "→ omega analyze", "■ Update", "stopped by you", "not run",
		"2 done · 2 failed or stopped · 0 skipped · 6 not run"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("the summary lacks %q:\n%s", want, b.String())
		}
	}
}

// A stage is a task started in this process, with the flags of the run: the
// guided run and the stage share one interface instead of starting omega again.
func TestFullStartsTheTaskOfEachStage(t *testing.T) {
	stages := fullFixture(t, "darwin", true, "docker", "topgrade")
	old := startTask
	t.Cleanup(func() { startTask = old })
	var started []string
	startTask = func(task string, f runFlags) error {
		if !f.dryRun || f.model != "m1" {
			t.Errorf("%s did not get the flags of the run: %+v", task, f)
		}
		started = append(started, task)
		return nil
	}
	for _, s := range stages {
		if s.id == "doctor" {
			continue // the doctor reads the real configuration
		}
		if err := runStage(s, runFlags{dryRun: true, model: "m1"}, io.Discard); err != nil {
			t.Errorf("%s: %v", s.id, err)
		}
	}
	if got := strings.Join(started, " "); got != "analyze health update clean docker tune dev mac fix history" {
		t.Errorf("started %s", got)
	}
}

func TestMenuKnowsWhatTakesTheWholeScreen(t *testing.T) {
	for line, want := range map[string]bool{
		"": true, "update": true, "full": true, "full --dry-run": true, "mac": true, "dev --auto": true, "-p why is it slow": true, "health": true,
		"doctor": false, "mac status": false, "dev install": false, "update --no-tui": false, "--help": false, "--version": false,
		"docker": false, "steps": false, "config set language es": false,
	} {
		if got := fullScreen(strings.Fields(line)); got != want {
			t.Errorf("omega %s: full screen %v, want %v", line, got, want)
		}
	}
}
