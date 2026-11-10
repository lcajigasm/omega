package cli

import (
	"errors"
	"strings"
	"testing"
)

func fullFixture(t *testing.T, os string, docker bool) []fullStage {
	t.Helper()
	oldOS, oldHas := goos, hasTool
	t.Cleanup(func() { goos, hasTool = oldOS, oldHas })
	goos = os
	hasTool = func(bin string) bool { return docker && bin == "docker" }
	return stagesHere()
}

func stageIDs(stages []fullStage) string {
	var ids []string
	for _, s := range stages {
		ids = append(ids, s.id)
	}
	return strings.Join(ids, " ")
}

func TestFullStagesFitTheMachine(t *testing.T) {
	if got := stageIDs(fullFixture(t, "darwin", true)); got != "doctor analyze update clean tune dev docker mac" {
		t.Errorf("a Mac with Docker: %s", got)
	}
	if got := stageIDs(fullFixture(t, "linux", false)); got != "doctor analyze update clean tune dev" {
		t.Errorf("Linux without Docker: %s", got)
	}
	if got := stageIDs(fullFixture(t, "windows", true)); got != "doctor analyze update clean tune dev docker" {
		t.Errorf("Windows with Docker: %s", got)
	}
	// Every stage is a command this binary has.
	root := testRoot()
	for _, s := range fullStages {
		c, rest, err := root.Find(s.args)
		if err != nil {
			t.Errorf("%s: %v", s.id, err)
			continue
		}
		if c == root {
			if _, ok := taskFor(rest[0]); !ok {
				t.Errorf("%s runs `omega %s`, which is neither a command nor a task", s.id, strings.Join(s.args, " "))
			}
		}
	}
}

func TestFullRunsInOrderAndCanSkipOrStop(t *testing.T) {
	stages := fullFixture(t, "darwin", true)
	all := map[string]bool{}
	for _, s := range stages {
		all[s.id] = true
	}
	var ran []string
	run := func(args []string) error { ran = append(ran, args[0]); return nil }
	var b strings.Builder

	// Enter every time: everything runs, in order.
	result := runFull(&b, stages, all, runFlags{}, run, func(string) string { return "\n" })
	if strings.Join(ran, " ") != "doctor analyze update clean tune dev docker mac" {
		t.Fatalf("ran %v", ran)
	}
	for _, s := range stages {
		if result[s.id] != stDone {
			t.Errorf("%s: %s", s.id, result[s.id])
		}
	}
	if out := b.String(); !strings.Contains(out, "Step 1 of 8 · omega doctor") || !strings.Contains(out, "Step 8 of 8 · omega mac") {
		t.Errorf("headers:\n%s", out)
	}

	// Unticked on the card: never run, reported as skipped.
	ran = nil
	chosen := map[string]bool{"doctor": true, "update": true, "clean": true}
	result = runFull(&b, stages, chosen, runFlags{}, run, func(string) string { return "" })
	if strings.Join(ran, " ") != "doctor update clean" || result["analyze"] != stSkipped || result["mac"] != stSkipped {
		t.Fatalf("ran %v result %v", ran, result)
	}

	// "s" skips the next step (twice in a row skips two), "q" stops.
	ran = nil
	answers := map[string]string{"omega analyze": "s", "omega update": "S ", "omega clean": "", "omega tune": "q"}
	result = runFull(&b, stages, all, runFlags{}, run, func(next string) string { return answers[next] })
	if strings.Join(ran, " ") != "doctor clean" {
		t.Fatalf("ran %v", ran)
	}
	want := map[string]string{"doctor": stDone, "analyze": stSkipped, "update": stSkipped, "clean": stDone, "tune": stPending, "dev": stPending, "mac": stPending}
	for id, st := range want {
		if result[id] != st {
			t.Errorf("%s: %s, want %s", id, result[id], st)
		}
	}

	// A step that ends with an error is recorded and the run goes on.
	ran = nil
	run = func(args []string) error {
		ran = append(ran, args[0])
		if args[0] == "update" {
			return errors.New("exit 1")
		}
		return nil
	}
	result = runFull(&b, stages, all, runFlags{}, run, func(string) string { return "" })
	if result["update"] != stFailed || result["clean"] != stDone || len(ran) != 8 {
		t.Fatalf("ran %v result %v", ran, result)
	}
	b.Reset()
	fullSummary(&b, stages, result)
	if out := b.String(); !strings.Contains(out, "✗ omega update") || !strings.Contains(out, "ended with an error") || !strings.Contains(out, "✓ omega clean") {
		t.Errorf("summary:\n%s", out)
	}
}

func TestFullPassesTheRunFlagsToTheAgentStepsOnly(t *testing.T) {
	stages := fullFixture(t, "linux", false)
	f := runFlags{dryRun: true, model: "m1", provider: "openai"}
	got := map[string]string{}
	for _, s := range stages {
		got[s.id] = strings.Join(stageArgs(s, f), " ")
	}
	if got["doctor"] != "doctor" {
		t.Errorf("doctor takes no run flags: %q", got["doctor"])
	}
	for _, want := range []string{"update", "--dry-run", "--model m1", "--provider openai"} {
		if !strings.Contains(got["update"], want) {
			t.Errorf("update: %q lacks %q", got["update"], want)
		}
	}
	if strings.Contains(got["update"], "--yes") || strings.Contains(got["update"], "--sudo") {
		t.Errorf("flags that were not given: %q", got["update"])
	}
}
