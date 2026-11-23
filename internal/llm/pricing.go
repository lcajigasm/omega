package llm

import "strings"

// Price is the cost in US dollars per million tokens.
type Price struct {
	Input, Output float64
	// LongAbove and LongFactor describe a price that changes with the size
	// of the prompt: a request whose input passes LongAbove tokens pays
	// every rate times LongFactor (Claude Haiku 5.5: five times, above
	// 100,000 tokens). Zero means one price for every size.
	LongAbove  int64
	LongFactor float64
}

// Cost returns the dollar cost of one request's usage at this price.
func (p Price) Cost(u Usage) float64 {
	// Cache reads bill at a tenth of the input price and cache writes at 1.25x.
	in := float64(u.InputTokens) + 0.1*float64(u.CacheRead) + 1.25*float64(u.CacheWrite)
	cost := (in*p.Input + float64(u.OutputTokens)*p.Output) / 1e6
	if p.LongAbove > 0 && p.LongFactor > 0 && u.Input() > p.LongAbove {
		cost *= p.LongFactor
	}
	return cost
}

// longPrompt are the models whose price rises with the prompt's size. The
// live catalogue gives one price per model, so the tier is added here.
var longPrompt = map[string]Price{
	"claude-haiku-5-5": {LongAbove: 100_000, LongFactor: 5},
}

func withTier(id string, p Price) Price {
	if t, ok := longPrompt[strings.ReplaceAll(id, ".", "-")]; ok && p.LongAbove == 0 {
		p.LongAbove, p.LongFactor = t.LongAbove, t.LongFactor
	}
	return p
}

// prices lists list prices for the models omega suggests. Keys are model ids
// without a vendor prefix; OpenRouter ids ("anthropic/claude-haiku-5.5") are
// normalised before lookup.
var prices = map[string]Price{
	"claude-opus-5-5":   {Input: 4, Output: 20},
	"claude-opus-5":     {Input: 5, Output: 25},
	"claude-fable-5-1":  {Input: 10, Output: 50},
	"claude-sonnet-5-5": {Input: 2, Output: 10},
	"claude-sonnet-5":   {Input: 2, Output: 10},
	"claude-sonnet-4-6": {Input: 3, Output: 15},
	"claude-haiku-5-5":  {Input: 0.10, Output: 0.50}, // up to 100K tokens of prompt; see longPrompt
	"claude-haiku-4-5":  {Input: 1, Output: 5},
	"gpt-5.5":           {Input: 5, Output: 30},
	"gpt-5.5-pro":       {Input: 30, Output: 180},
	"gpt-5.4-mini":      {Input: 0.75, Output: 4.5},
	"gpt-5-mini":        {Input: 0.25, Output: 2},
}

// LivePrice, when set, answers with the current list price of a model from
// the live catalogue. It is asked before the table above, which only knows
// the models that existed when omega was built.
var LivePrice func(provider, model string) (Price, bool)

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
	if LivePrice != nil {
		if p, ok := LivePrice(provider, model); ok {
			return withTier(id, p), true
		}
	}
	if p, ok := prices[id]; ok {
		return withTier(id, p), true
	}
	// OpenRouter writes Claude versions with dots: claude-haiku-5.5.
	if strings.HasPrefix(id, "claude-") {
		if p, ok := prices[strings.ReplaceAll(id, ".", "-")]; ok {
			return withTier(id, p), true
		}
	}
	return Price{}, false
}
