// Package llm is a small provider-neutral layer over the chat APIs omega
// supports. Each Chat keeps its own provider-native history so details such
// as Claude's thinking blocks round-trip unchanged.
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ToolSpec describes a tool the model may call. Schema is a JSON Schema object
// with "properties" and "required".
type ToolSpec struct {
	Name        string
	Description string
	Properties  map[string]any
	Required    []string
}

// ToolCall is a request from the model to run a tool.
type ToolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}

// ToolResult is the outcome of a ToolCall, sent back on the next turn.
type ToolResult struct {
	CallID  string
	Content string
	IsError bool
}

// Usage is the token accounting for one model call.
type Usage struct {
	InputTokens  int64 // uncached input
	OutputTokens int64
	CacheRead    int64 // input served from the prompt cache
	CacheWrite   int64 // input written to the prompt cache
}

// Input is every input token the request processed, cached or not.
func (u Usage) Input() int64 { return u.InputTokens + u.CacheRead + u.CacheWrite }

// Add sums two usages.
func (u Usage) Add(o Usage) Usage {
	return Usage{u.InputTokens + o.InputTokens, u.OutputTokens + o.OutputTokens, u.CacheRead + o.CacheRead, u.CacheWrite + o.CacheWrite}
}

// Reply is one model turn.
type Reply struct {
	Text    string
	Calls   []ToolCall
	Refused bool
	Usage   Usage
}

// Chat is a running conversation with one model.
type Chat interface {
	// Send appends either a user message (text) or the results of the
	// previous turn's tool calls, then asks the model for its next turn.
	Send(ctx context.Context, text string, results []ToolResult) (*Reply, error)
	// SetModel switches the model used for the following turns, keeping the
	// conversation.
	SetModel(model string)
}

// Provider creates chats and lists models.
type Provider interface {
	Name() string
	NewChat(model, system string, tools []ToolSpec) Chat
	Models(ctx context.Context) ([]string, error)
}

// ErrUnauthorized means the provider rejected the API key.
var ErrUnauthorized = errors.New("unauthorized")

// Verify checks that the provider accepts its API key.
func Verify(ctx context.Context, p Provider) error {
	if v, ok := p.(interface{ verify(context.Context) error }); ok {
		return v.verify(ctx)
	}
	_, err := p.Models(ctx)
	return err
}

// Options configure a provider instance.
type Options struct {
	APIKey  string
	BaseURL string
	Effort  string
}

// Info describes a supported provider.
type Info struct {
	Name       string
	Label      string
	FastModel  string // cheap model for routine maintenance
	SmartModel string // stronger model for diagnosing failures
	KeyURL     string
	Local      bool   // runs on this Mac; no API key needed
	BaseURL    string // default endpoint for local and custom providers
}

// Providers lists every supported provider, in display order.
var Providers = []Info{
	{Name: "anthropic", Label: "Anthropic (Claude)", FastModel: "claude-haiku-5-5", SmartModel: "claude-sonnet-5-5", KeyURL: "https://console.anthropic.com/settings/keys"},
	{Name: "openai", Label: "OpenAI", FastModel: "gpt-5.4-mini", SmartModel: "gpt-5.5", KeyURL: "https://platform.openai.com/api-keys"},
	{Name: "openrouter", Label: "OpenRouter", FastModel: "anthropic/claude-haiku-5.5", SmartModel: "anthropic/claude-sonnet-5.5", KeyURL: "https://openrouter.ai/settings/keys"},
	{Name: "opencode", Label: "OpenCode Zen", FastModel: "claude-sonnet-4-6", SmartModel: "claude-sonnet-4-6", KeyURL: "https://opencode.ai/auth"},
	{Name: "ollama", Label: "Ollama (local)", Local: true, BaseURL: "http://localhost:11434/v1", KeyURL: "https://ollama.com/download"},
	{Name: "lmstudio", Label: "LM Studio (local)", Local: true, BaseURL: "http://localhost:1234/v1", KeyURL: "https://lmstudio.ai"},
	{Name: "custom", Label: "OpenAI-compatible endpoint", Local: true, KeyURL: "https://github.com/lcajigasm/omega#local-models"},
}

// Lookup returns the Info for a provider name.
func Lookup(name string) (Info, bool) {
	for _, p := range Providers {
		if p.Name == name {
			return p, true
		}
	}
	return Info{}, false
}

// New builds a provider by name.
func New(name string, opts Options) (Provider, error) {
	info, ok := Lookup(name)
	if !ok {
		names := make([]string, 0, len(Providers))
		for _, p := range Providers {
			names = append(names, p.Name)
		}
		return nil, fmt.Errorf("unknown provider %q (available: %s)", name, strings.Join(names, ", "))
	}
	switch name {
	case "anthropic":
		return newAnthropic("anthropic", opts), nil
	case "openai":
		if opts.BaseURL == "" {
			opts.BaseURL = "https://api.openai.com/v1"
		}
	case "openrouter":
		if opts.BaseURL == "" {
			opts.BaseURL = "https://openrouter.ai/api/v1"
		}
	case "opencode":
		return newOpenCode(opts), nil
	default:
		if opts.BaseURL == "" {
			opts.BaseURL = info.BaseURL
		}
		if opts.BaseURL == "" {
			return nil, fmt.Errorf("provider %q needs a base_url in the configuration", name)
		}
	}
	return newOpenAICompat(name, opts), nil
}

func sortedUnique(in []string) []string {
	seen := map[string]bool{}
	out := in[:0]
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
