package cli

import (
	"strings"
	"testing"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/llm"
)

// Ultra does everything omega full does, plus its two own stages.
func TestUltraHasEveryStageOfFullAndItsOwn(t *testing.T) {
	ids := map[string]int{}
	for i, s := range ultraStages() {
		ids[s.id] = i
	}
	for _, s := range fullStages {
		if _, ok := ids[s.id]; !ok {
			t.Errorf("ultra leaves out the %s stage of omega full", s.id)
		}
	}
	if ids["deep"] != ids["docker"]+1 || ids["care"] != ids["tune"]+1 {
		t.Errorf("deep goes after docker and care after tune: %v", ids)
	}
}

func TestUltraModels(t *testing.T) {
	cfg := config.Default()
	info, _ := llm.Lookup("anthropic")
	got := ultraPlan(cfg, info, "", agent.TaskClean)
	if got.Model != "claude-sonnet-5-5" || got.Escalate != "claude-opus-5-5" {
		t.Errorf("a stage starts on the smart model and moves to the ultra one: %+v", got)
	}
	if got := ultraPlan(cfg, info, "", agent.TaskUltraDeep); got.Model != "claude-opus-5-5" || got.Escalate != "" {
		t.Errorf("the deep clean starts on the ultra model: %+v", got)
	}
	if got := ultraPlan(cfg, info, "gpt-x", agent.TaskUltraDeep); got.Model != "gpt-x" {
		t.Errorf("--model wins: %+v", got)
	}
	cfg.Providers = map[string]config.ProviderConfig{"anthropic": {UltraModel: "claude-fable-5-2"}}
	if got := ultraPlan(cfg, info, "", agent.TaskUltraDeep); got.Model != "claude-fable-5-2" {
		t.Errorf("ultra_model is the user's: %+v", got)
	}
	// A provider without an ultra tier stays on its smart one.
	oi, _ := llm.Lookup("openai")
	if got := ultraPlan(config.Default(), oi, "", agent.TaskClean); got.Model != oi.SmartModel || got.Escalate != "" {
		t.Errorf("openai: %+v", got)
	}
}

func TestUltraEffortNeverLowersTheUsers(t *testing.T) {
	for in, want := range map[string]string{"": "high", "low": "high", "medium": "high", "high": "high", "xhigh": "xhigh", "max": "max"} {
		if got := ultraEffort(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestUltraRefusesTheFlagsThatAnswerForThePerson(t *testing.T) {
	for _, flag := range []string{"--yes", "--auto", "--no-ai"} {
		c := ultraCmd()
		c.SetArgs([]string{flag})
		c.SilenceUsage, c.SilenceErrors = true, true
		err := c.Execute()
		if err == nil || !strings.Contains(err.Error(), flag) {
			t.Errorf("%s: %v", flag, err)
		}
	}
}

func TestUltraStagesFlags(t *testing.T) {
	chosen, err := pickUltraStages(ultraStages(), []string{"deep,care"}, nil)
	if err != nil || !chosen["deep"] || !chosen["care"] || chosen["update"] {
		t.Fatalf("%v %v", chosen, err)
	}
	if _, err := pickUltraStages(ultraStages(), []string{"nope"}, nil); err == nil {
		t.Errorf("an unknown stage must be an error")
	}
}
