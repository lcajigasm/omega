package llm

import (
	"context"
	"fmt"
	"strings"
)

// OpenCode Zen serves Claude (and a few Qwen models) on an Anthropic-compatible
// endpoint and most other models on an OpenAI-compatible one. GPT models are
// only offered through the Responses API, which omega does not speak yet.
const openCodeBase = "https://opencode.ai/inference"

type openCode struct {
	anthropic *anthropicProvider
	openai    *openAICompat
}

func newOpenCode(opts Options) *openCode {
	base := strings.TrimRight(opts.BaseURL, "/")
	if base == "" {
		base = openCodeBase
	}
	aOpts := opts
	aOpts.BaseURL = base + "/anthropic"
	oOpts := opts
	oOpts.BaseURL = base + "/openai/v1"
	oOpts.Effort = ""
	oc := &openCode{anthropic: newAnthropic("opencode", aOpts), openai: newOpenAICompat("opencode", oOpts)}
	oc.openai.modelsURL = base + "/v1/models"
	return oc
}

func (p *openCode) Name() string { return "opencode" }

func (p *openCode) Models(ctx context.Context) ([]string, error) { return p.openai.Models(ctx) }

func (p *openCode) verify(ctx context.Context) error { return p.openai.verify(ctx) }

func (p *openCode) NewChat(model, system string, tools []ToolSpec) Chat {
	if usesAnthropicEndpoint(model) {
		return &openCodeChat{Chat: p.anthropic.NewChat(model, system, tools), anthropic: true}
	}
	return &openCodeChat{Chat: p.openai.NewChat(model, system, tools)}
}

// openCodeChat is a conversation on one of the two endpoints. The history
// is kept in that endpoint's own format, so the model can only be changed
// for another one served there.
type openCodeChat struct {
	Chat
	anthropic bool
}

// SwitchModel changes the model, or says why it cannot.
func (c *openCodeChat) SwitchModel(model string) error {
	if usesAnthropicEndpoint(model) != c.anthropic {
		return fmt.Errorf("OpenCode Zen serves %s on another endpoint: this conversation cannot continue with it; start a new one to use it", model)
	}
	c.Chat.SetModel(model)
	return nil
}

// SetModel is SwitchModel for callers that cannot take an error: a model
// of the other endpoint is not switched to.
func (c *openCodeChat) SetModel(model string) { _ = c.SwitchModel(model) }

// Compact shortens the history of the endpoint's chat.
func (c *openCodeChat) Compact() int {
	if cc, ok := c.Chat.(Compactor); ok {
		return cc.Compact()
	}
	return 0
}

func usesAnthropicEndpoint(model string) bool {
	m := strings.ToLower(model)
	if strings.HasPrefix(m, "claude") {
		return true
	}
	return strings.HasPrefix(m, "qwen") && (strings.HasSuffix(m, "-plus") || strings.HasSuffix(m, "-flash"))
}
