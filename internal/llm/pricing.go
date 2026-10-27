package llm

import "strings"

// Price is the cost in US dollars per million tokens.
type Price struct {
	Input, Output float64
}

// Cost returns the dollar cost of a usage at this price.
func (p Price) Cost(u Usage) float64 {
	// Cache reads bill at a tenth of the input price and cache writes at 1.25x.
	in := float64(u.InputTokens) + 0.1*float64(u.CacheRead) + 1.25*float64(u.CacheWrite)
	return (in*p.Input + float64(u.OutputTokens)*p.Output) / 1e6
}

// prices lists list prices for the models omega suggests. Keys are model ids
// without a vendor prefix; OpenRouter ids ("anthropic/claude-haiku-5.5") are
// normalised before lookup.
var prices = map[string]Price{
	"claude-opus-5-5":   {4, 20},
	"claude-opus-5":     {5, 25},
	"claude-fable-5-1":  {10, 50},
	"claude-sonnet-5-5": {2, 10},
	"claude-sonnet-5":   {2, 10},
	"claude-sonnet-4-6": {3, 15},
	"claude-haiku-5-5":  {0.10, 0.50},
	"claude-haiku-4-5":  {1, 5},
	"gpt-5.5":           {5, 30},
	"gpt-5.5-pro":       {30, 180},
	"gpt-5.4-mini":      {0.75, 4.5},
	"gpt-5-mini":        {0.25, 2},
}

// PriceFor returns the known price of a model. Local providers are free.
func PriceFor(provider, model string) (Price, bool) {
	if info, ok := Lookup(provider); ok && info.Local {
		return Price{}, true
	}
	id := model
	if i := strings.LastIndex(id, "/"); i >= 0 {
		id = id[i+1:]
	}
	id = strings.TrimSuffix(id, ":batch")
	if p, ok := prices[id]; ok {
		return p, true
	}
	// OpenRouter writes Claude versions with dots: claude-haiku-5.5.
	if strings.HasPrefix(id, "claude-") {
		if p, ok := prices[strings.ReplaceAll(id, ".", "-")]; ok {
			return p, true
		}
	}
	return Price{}, false
}
