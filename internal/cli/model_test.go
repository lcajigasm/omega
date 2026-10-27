package cli

import (
	"testing"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/llm"
)

func TestPlanModel(t *testing.T) {
	info, _ := llm.Lookup("anthropic")
	cfg := config.Default()

	p, _ := planModel(cfg, info, "", "update")
	if p.Model != "claude-haiku-5-5" || p.Escalate != "claude-sonnet-5-5" {
		t.Fatalf("auto routine: %+v", p)
	}
	p, _ = planModel(cfg, info, "", "fix")
	if p.Model != "claude-sonnet-5-5" || p.Escalate != "" {
		t.Fatalf("auto fix: %+v", p)
	}
	p, _ = planModel(cfg, info, "claude-opus-5-5", "update")
	if p.Model != "claude-opus-5-5" || p.Escalate != "" {
		t.Fatalf("flag: %+v", p)
	}
	cfg.Providers["anthropic"] = config.ProviderConfig{FastModel: "my-fast"}
	p, _ = planModel(cfg, info, "", "clean")
	if p.Model != "my-fast" {
		t.Fatalf("override: %+v", p)
	}
	local, _ := llm.Lookup("ollama")
	if _, err := planModel(config.Default(), local, "", "update"); err == nil {
		t.Fatal("local provider without a model should ask the user to pick one")
	}
}

func TestParseSize(t *testing.T) {
	for in, want := range map[string]float64{"1.2GB": 1.2e9, "512MB (40%)": 512e6, "0B": 0, "3kB": 3e3} {
		if got := parseSize(in); got != want {
			t.Errorf("parseSize(%q) = %v, want %v", in, got, want)
		}
	}
}
