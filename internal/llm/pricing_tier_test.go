package llm

import "testing"

// Claude Haiku 5.5 costs five times as much for a request whose prompt
// passes 100,000 tokens; the runs that read most were costed at a fifth.
func TestHaikuLongPromptTier(t *testing.T) {
	p, ok := PriceFor("anthropic", "claude-haiku-5-5")
	if !ok {
		t.Fatal("no price")
	}
	short := p.Cost(Usage{InputTokens: 50_000, OutputTokens: 1_000})
	long := p.Cost(Usage{InputTokens: 150_000, OutputTokens: 1_000})
	if want := (50_000*0.10 + 1_000*0.50) / 1e6; short != want {
		t.Errorf("short prompt: %v, want %v", short, want)
	}
	if want := 5 * (150_000*0.10 + 1_000*0.50) / 1e6; long != want {
		t.Errorf("long prompt: %v, want %v", long, want)
	}
	if p, _ := PriceFor("openrouter", "anthropic/claude-haiku-5.5"); p.LongAbove != 100_000 {
		t.Errorf("the tier must follow the model through OpenRouter's name: %+v", p)
	}
	if p, _ := PriceFor("anthropic", "claude-sonnet-5-5"); p.LongAbove != 0 {
		t.Errorf("sonnet has one price: %+v", p)
	}
}
