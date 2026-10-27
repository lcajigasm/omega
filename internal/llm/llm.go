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
	InputTokens  int64
	OutputTokens int64
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
	Name         string
	Label        string
	DefaultModel string
	KeyURL       string
}

// Providers lists every supported provider, in display order.
var Providers = []Info{
	{"anthropic", "Anthropic (Claude)", "claude-opus-5-5", "https://console.anthropic.com/settings/keys"},
	{"openai", "OpenAI", "gpt-5.5", "https://platform.openai.com/api-keys"},
	{"openrouter", "OpenRouter", "anthropic/claude-opus-5.5", "https://openrouter.ai/settings/keys"},
	{"opencode", "OpenCode Zen", "claude-sonnet-4-6", "https://opencode.ai/auth"},
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
	switch name {
	case "anthropic":
		return newAnthropic("anthropic", opts), nil
	case "openai":
		if opts.BaseURL == "" {
			opts.BaseURL = "https://api.openai.com/v1"
		}
		return newOpenAICompat("openai", opts), nil
	case "openrouter":
		if opts.BaseURL == "" {
			opts.BaseURL = "https://openrouter.ai/api/v1"
		}
		return newOpenAICompat("openrouter", opts), nil
	case "opencode":
		return newOpenCode(opts), nil
	}
	names := make([]string, 0, len(Providers))
	for _, p := range Providers {
		names = append(names, p.Name)
	}
	return nil, fmt.Errorf("unknown provider %q (available: %s)", name, strings.Join(names, ", "))
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
