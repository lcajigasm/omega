package llm

import (
	"context"
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

func (p *openCode) NewChat(model, system string, tools []ToolSpec) Chat {
	if usesAnthropicEndpoint(model) {
		return p.anthropic.NewChat(model, system, tools)
	}
	return p.openai.NewChat(model, system, tools)
}

func usesAnthropicEndpoint(model string) bool {
	m := strings.ToLower(model)
	if strings.HasPrefix(m, "claude") {
		return true
	}
	return strings.HasPrefix(m, "qwen") && (strings.HasSuffix(m, "-plus") || strings.HasSuffix(m, "-flash"))
}
