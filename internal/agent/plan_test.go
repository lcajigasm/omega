package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/deepclean"
	"github.com/lcajigasm/omega/internal/runner"
)

func planned(id string, steps ...map[string]any) map[string]any {
	return map[string]any{"id": id, "title": "Do " + id, "why": "the audit found something worth it",
		"what": "this happens and it can be undone", "risk": "normal", "evidence": "the output that shows it is safe", "steps": steps}
}

// The audit changes nothing: a change is refused with a pointer to the
// plan, a finding omega verified as safe is routine, and plan_actions
// records each action, its steps checked, without running it.
func TestAuditPlansAndRunsNothing(t *testing.T) {
	sandbox(t)
	removed := fakeDeep(t)
	plan := NewPlan()
	opts := Options{Ultra: true, ReadOnly: true, Audit: plan, Task: TaskUltraDeep}
	res, ran, asked := drive(t, opts, Approve,
		call("1", "run_command", map[string]any{"command": "rm -rf ~/Library/Caches/x", "reason": "free space"}),
		call("2", "deep_scan", nil),
		call("3", "plan_actions", map[string]any{"actions": []any{
			planned("caches", map[string]any{"tool": "run_command", "input": map[string]any{"command": "rm -rf ~/Library/Caches/x"}}),
			planned("wt", map[string]any{"tool": "deep_clean", "input": map[string]any{"ids": []string{"wt-1"}}}),
			planned("kept", map[string]any{"tool": "deep_clean", "input": map[string]any{"ids": []string{"wt-2"}}}),
			planned("blocked", map[string]any{"tool": "run_command", "input": map[string]any{"command": "rm -rf /"}}),
			planned("reads", map[string]any{"tool": "omega_cmd", "input": map[string]any{"action": "git_status"}}),
			map[string]any{"id": "bare", "title": "No why", "why": "x", "what": "y", "risk": "normal",
				"steps": []any{map[string]any{"tool": "run_step", "input": map[string]any{"id": "brew"}}}},
		}}),
		call("4", "propose_actions", map[string]any{"question": "?", "options": []any{map[string]any{"id": "a", "label": "A"}}}),
	)
	if len(ran) != 0 || len(*removed) != 0 || asked != 0 {
		t.Fatalf("the audit ran %v, removed %v, asked %d", ran, *removed, asked)
	}
	if !res[0].IsError || !strings.Contains(res[0].Content, "plan_actions") {
		t.Errorf("a change during the audit points at the plan: %+v", res[0])
	}
	got := res[2].Content
	for _, want := range []string{"caches", "wt", "kept and never removed", "blocked: step 1", "reads: step 1", "bare: why must"} {
		if !strings.Contains(got, want) {
			t.Errorf("plan_actions answer lacks %q:\n%s", want, got)
		}
	}
	if !res[3].IsError || !strings.Contains(res[3].Content, "plan_actions") {
		t.Errorf("propose_actions is refused in the audit: %+v", res[3])
	}
	acts := plan.List()
	if len(acts) != 2 {
		t.Fatalf("plan: %+v", acts)
	}
	wt := acts[1]
	if wt.Stage != TaskUltraDeep || wt.Risk != "normal" || wt.Bytes != 2e9 || len(wt.Findings) != 1 || !strings.Contains(wt.Steps[0].Show, "git worktree remove wt-1") {
		t.Errorf("a deep_clean action carries its finding, size and risk: %+v", wt)
	}
	if acts[0].Risk != "dangerous" {
		t.Errorf("rm -rf is a dangerous step whatever the model said: %+v", acts[0])
	}
}

// The plan is carried out without a model and without a card: the tick is
// the approval. A step that fails ends its action, not the others.
func TestExecutorRunsExactlyThePlannedSteps(t *testing.T) {
	sandbox(t)
	removed := fakeDeep(t)
	finding := deepScan(context.Background(), deepclean.Options{}).Findings[0]
	var ran []string
	exec := func(_ context.Context, command string, _ time.Duration, _ func(string)) runner.Result {
		ran = append(ran, command)
		if strings.Contains(command, "fails") {
			return runner.Result{ExitCode: 3, Output: "boom"}
		}
		return runner.Result{Output: "ok"}
	}
	acts := []PlannedAction{
		{ID: "one", Title: "One", Steps: []PlanStep{{Tool: "run_command", Input: map[string]any{"command": "rm -rf ~/Library/Caches/x", "reason": "r"}}}},
		{ID: "two", Title: "Two", Steps: []PlanStep{
			{Tool: "run_command", Input: map[string]any{"command": "echo fails && rm -rf ~/x", "reason": "r"}},
			{Tool: "run_command", Input: map[string]any{"command": "never runs", "reason": "r"}},
		}},
		{ID: "three", Title: "Three", Findings: []deepclean.Finding{finding},
			Steps: []PlanStep{{Tool: "deep_clean", Input: map[string]any{"ids": []any{"wt-1"}}}}},
		{ID: "four", Title: "Four", Steps: []PlanStep{{Tool: "run_command", Input: map[string]any{"command": "rm -rf /", "reason": "r"}}}},
	}
	a := NewExecutor(Options{Config: config.Default(), Exec: exec, Timeout: 10 * time.Second})
	status := map[string]string{}
	go a.RunPlanned(context.Background(), acts, func(id, st, _ string) { status[id] = st })
	asked := 0
	for e := range a.Events() {
		if e.Kind == EvConfirm || e.Kind == EvChoice {
			asked++
		}
	}
	if asked != 0 {
		t.Errorf("the executor asked %d times", asked)
	}
	if strings.Join(ran, "|") != "rm -rf ~/Library/Caches/x|echo fails && rm -rf ~/x" {
		t.Errorf("ran %q", ran)
	}
	if strings.Join(*removed, ",") != "wt-1" {
		t.Errorf("removed %v", *removed)
	}
	want := map[string]string{"one": ActionDone, "two": ActionFailed, "three": ActionDone, "four": ActionFailed}
	for id, st := range want {
		if status[id] != st {
			t.Errorf("%s: %q, want %q", id, status[id], st)
		}
	}
	if a.ExitCode() != ExitError {
		t.Errorf("exit %d", a.ExitCode())
	}
}

func TestPlanIsKeptBetweenRuns(t *testing.T) {
	sandbox(t)
	p := NewPlan()
	p.add(PlannedAction{ID: "a", Title: "A", Bytes: 10})
	p.add(PlannedAction{ID: "b", Title: "B", Bytes: 5})
	p.add(PlannedAction{ID: "a", Title: "A again", Bytes: 7})
	if n, b := p.Count(); n != 2 || b != 12 {
		t.Fatalf("count %d %d", n, b)
	}
	p.Mark("b", ActionDone, "ok")
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	q, err := LoadPlan()
	if err != nil || len(q.Actions) != 2 || q.Actions[0].Title != "A again" || q.Actions[1].Status != ActionDone {
		t.Fatalf("%+v %v", q, err)
	}
	if !strings.Contains(q.PromptSummary(), "- a: A again") {
		t.Errorf("summary: %s", q.PromptSummary())
	}
}
