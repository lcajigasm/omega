package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/models"
)

func TestPlanModel(t *testing.T) {
	info, _ := llm.Lookup("anthropic")
	cfg := config.Default()

	p, _ := planModel(cfg, info, "", "update")
	if p.Model != "claude-haiku-5-5" || p.Escalate != "claude-sonnet-5-5" {
		t.Fatalf("auto routine: %+v", p)
	}
	if len(p.Choices) != 2 {
		t.Fatalf("auto routine choices: %+v", p.Choices)
	}
	p, _ = planModel(cfg, info, "", "fix")
	if p.Model != "claude-sonnet-5-5" || p.Escalate != "" {
		t.Fatalf("auto fix: %+v", p)
	}
	p, _ = planModel(cfg, info, "claude-opus-5-5", "update")
	if p.Model != "claude-opus-5-5" || p.Escalate != "" {
		t.Fatalf("flag: %+v", p)
	}
	if len(p.Choices) != 3 || p.Choices[0] != "claude-opus-5-5" {
		t.Fatalf("flag choices: %+v", p.Choices)
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

func liveStore(t *testing.T) {
	t.Helper()
	old := modelStore
	t.Cleanup(func() { modelStore = old })
	modelStore = &models.Store{
		Fetched: time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC),
		Models: []models.Model{
			{ID: "anthropic/claude-new-6", Score: 60, In: 2, Out: 10, Tools: true, Text: true},
			{ID: "anthropic/claude-small-6", Score: 40, In: 0.1, Out: 0.5, Tools: true, Text: true},
			{ID: "anthropic/claude-guess-5", Score: 30, Estimated: true, In: 1, Out: 5, Tools: true, Text: true},
		},
		Picks: map[string]models.Picks{"anthropic": {Fast: "claude-small-6", Smart: "claude-new-6"}},
	}
}

func TestAutoUsesTheLivePicksAndOverridesStillWin(t *testing.T) {
	liveStore(t)
	info, _ := llm.Lookup("anthropic")
	cfg := config.Default()

	p, _ := planModel(cfg, info, "", "update")
	if p.Model != "claude-small-6" || p.Escalate != "claude-new-6" {
		t.Fatalf("live picks: %+v", p)
	}
	if p, _ = planModel(cfg, info, "", "fix"); p.Model != "claude-new-6" {
		t.Fatalf("fix: %+v", p)
	}
	cfg.Providers["anthropic"] = config.ProviderConfig{SmartModel: "my-smart"}
	if p, _ = planModel(cfg, info, "", "update"); p.Model != "claude-small-6" || p.Escalate != "my-smart" {
		t.Fatalf("override: %+v", p)
	}
	cfg = config.Default()
	cfg.Catalog = config.CatalogOff
	if p, _ = planModel(cfg, info, "", "update"); p.Model != info.FastModel || p.Escalate != info.SmartModel {
		t.Fatalf("catalogue off: %+v", p)
	}
	// A provider the catalogue has no picks for keeps the built-in names.
	other, _ := llm.Lookup("openai")
	if p, _ = planModel(config.Default(), other, "", "update"); p.Model != other.FastModel {
		t.Fatalf("no picks: %+v", p)
	}
}

func TestModelsReportMarksThePicksAndListsUnknownModels(t *testing.T) {
	liveStore(t)
	info, _ := llm.Lookup("anthropic")
	var b strings.Builder
	modelsReport(&b, config.Default(), info, []string{"claude-new-6", "claude-small-6", "claude-guess-5", "claude-mystery"}, "")
	out := b.String()
	for _, want := range []string{"▸ claude-small-6", "▸ claude-new-6", "60.0", "$0.1", "~30.0", "claude-mystery", "fast", "smart", "OpenRouter", "claude-small-6 for routine work"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Index(out, "claude-new-6") > strings.Index(out, "claude-small-6") {
		t.Errorf("best score should come first:\n%s", out)
	}
	b.Reset()
	modelsReport(&b, config.Default(), info, []string{"claude-new-6", "claude-small-6"}, "small")
	if strings.Contains(b.String(), "▸ claude-new-6") {
		t.Errorf("filter ignored:\n%s", b.String())
	}
	b.Reset()
	cfg := config.Default()
	cfg.Model = "claude-mystery"
	modelsReport(&b, cfg, info, nil, "")
	if !strings.Contains(b.String(), "fixed to claude-mystery") {
		t.Errorf("fixed model not reported:\n%s", b.String())
	}
}

func TestCatalogPriceIsUsedForModelsOmegaWasNotBuiltWith(t *testing.T) {
	liveStore(t)
	old := llm.LivePrice
	t.Cleanup(func() { llm.LivePrice = old })
	llm.LivePrice = func(provider, model string) (llm.Price, bool) {
		in, out, ok := modelStore.Price(provider, model)
		return llm.Price{Input: in, Output: out}, ok
	}
	if p, ok := llm.PriceFor("anthropic", "claude-new-6"); !ok || p.Input != 2 || p.Output != 10 {
		t.Fatalf("%+v %v", p, ok)
	}
	if _, ok := llm.PriceFor("anthropic", "claude-never-heard-of"); ok {
		t.Fatal("unknown model should have no price")
	}
}
