package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/lcajigasm/omega/internal/i18n"
)

// openAICompat speaks the Chat Completions wire format, shared by OpenAI,
// OpenRouter and OpenCode Zen's OpenAI-compatible models.
type openAICompat struct {
	name      string
	opts      Options
	http      *http.Client
	modelsURL string
}

func newOpenAICompat(name string, opts Options) *openAICompat {
	opts.BaseURL = strings.TrimRight(opts.BaseURL, "/")
	return &openAICompat{name: name, opts: opts, http: &http.Client{Timeout: 10 * time.Minute}}
}

func (p *openAICompat) Name() string { return p.name }

func (p *openAICompat) headers(req *http.Request) {
	if p.opts.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.opts.APIKey)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.name == "openrouter" {
		req.Header.Set("HTTP-Referer", "https://github.com/lcajigasm/omega")
		req.Header.Set("X-Title", "omega")
	}
}

func (p *openAICompat) Models(ctx context.Context) ([]string, error) {
	url := p.modelsURL
	if url == "" {
		url = p.opts.BaseURL + "/models"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	p.headers(req)
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, apiError(resp.StatusCode, body)
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("invalid models response: %w", err)
	}
	ids := make([]string, 0, len(out.Data))
	for _, m := range out.Data {
		ids = append(ids, m.ID)
	}
	return sortedUnique(ids), nil
}

func (p *openAICompat) verify(ctx context.Context) error {
	if p.name != "openrouter" {
		_, err := p.Models(ctx)
		return err
	}
	// OpenRouter lists models without auth; ask about the key itself.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.opts.BaseURL+"/key", nil)
	if err != nil {
		return err
	}
	p.headers(req)
	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return apiError(resp.StatusCode, body)
	}
	return nil
}

func (p *openAICompat) NewChat(model, system string, tools []ToolSpec) Chat {
	c := &openAIChat{p: p, model: model}
	c.messages = append(c.messages, map[string]any{"role": "system", "content": system})
	for _, t := range tools {
		params := map[string]any{"type": "object", "properties": t.Properties}
		if len(t.Required) > 0 {
			params["required"] = t.Required
		}
		c.tools = append(c.tools, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  params,
			},
		})
	}
	return c
}

func (c *openAIChat) SetModel(model string) { c.model = model }

type openAIChat struct {
	p        *openAICompat
	model    string
	messages []any
	tools    []any
}

type oaToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type oaResponse struct {
	Choices []struct {
		Message      json.RawMessage `json:"message"`
		FinishReason string          `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int64 `json:"prompt_tokens"`
		CompletionTokens int64 `json:"completion_tokens"`
	} `json:"usage"`
}

func (c *openAIChat) Send(ctx context.Context, text string, results []ToolResult) (*Reply, error) {
	added := 0
	for _, r := range results {
		content := r.Content
		if r.IsError {
			content = "ERROR: " + content
		}
		c.messages = append(c.messages, map[string]any{"role": "tool", "tool_call_id": r.CallID, "content": content})
		added++
	}
	if text != "" {
		c.messages = append(c.messages, map[string]any{"role": "user", "content": text})
		added++
	}

	body := map[string]any{"model": c.model, "messages": c.messages}
	if len(c.tools) > 0 {
		body["tools"] = c.tools
	}
	raw, err := c.post(ctx, body)
	if err != nil {
		c.messages = c.messages[:len(c.messages)-added]
		return nil, err
	}

	var resp oaResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("invalid response: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty response: %s", truncate(string(raw), 300))
	}
	choice := resp.Choices[0]
	var msg struct {
		Content   *string      `json:"content"`
		Refusal   *string      `json:"refusal"`
		ToolCalls []oaToolCall `json:"tool_calls"`
	}
	if err := json.Unmarshal(choice.Message, &msg); err != nil {
		return nil, fmt.Errorf("invalid message: %w", err)
	}

	// Keep only the fields every compatible API accepts back in history.
	hist := map[string]any{"role": "assistant", "content": msg.Content}
	if len(msg.ToolCalls) > 0 {
		hist["tool_calls"] = msg.ToolCalls
	}
	c.messages = append(c.messages, hist)

	reply := &Reply{Usage: Usage{InputTokens: resp.Usage.PromptTokens, OutputTokens: resp.Usage.CompletionTokens}}
	if msg.Content != nil {
		reply.Text = *msg.Content
	}
	if msg.Refusal != nil && *msg.Refusal != "" {
		reply.Refused = true
		reply.Text = *msg.Refusal
		return reply, nil
	}
	for _, tc := range msg.ToolCalls {
		args := tc.Function.Arguments
		if strings.TrimSpace(args) == "" {
			args = "{}"
		}
		reply.Calls = append(reply.Calls, ToolCall{ID: tc.ID, Name: tc.Function.Name, Input: json.RawMessage(args)})
	}
	if choice.FinishReason == "length" {
		reply.Calls = nil
		reply.Text += "\n[response truncated by length]"
	}
	return reply, nil
}

func (c *openAIChat) post(ctx context.Context, body map[string]any) ([]byte, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(1<<attempt) * time.Second):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.p.opts.BaseURL+"/chat/completions", bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		c.p.headers(req)
		resp, err := c.p.http.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = err
			continue
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return raw, nil
		}
		lastErr = apiError(resp.StatusCode, raw)
		if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 500 {
			return nil, lastErr
		}
	}
	return nil, lastErr
}

func apiError(status int, body []byte) error {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	msg := truncate(strings.TrimSpace(string(body)), 400)
	if json.Unmarshal(body, &e) == nil && e.Error.Message != "" {
		msg = e.Error.Message
	}
	hint := ""
	var base error = errors.New("request failed")
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		base = ErrUnauthorized
	}
	switch status {
	case http.StatusUnauthorized:
		hint = " (" + i18n.T("llm.invalid_key") + ")"
	case http.StatusNotFound:
		hint = " (" + i18n.T("llm.not_found") + ")"
	case http.StatusTooManyRequests:
		hint = " (" + i18n.T("llm.rate_limit") + ")"
	}
	return fmt.Errorf("%w: HTTP %d%s: %s", base, status, hint, msg)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
