package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/lcajigasm/omega/internal/i18n"
)

type anthropicProvider struct {
	name   string
	client anthropic.Client
	opts   Options
}

func newAnthropic(name string, opts Options) *anthropicProvider {
	// Retries belong to sendRetried, on the same schedule as the OpenAI client.
	// The SDK's own backoff would stack on top of that one.
	reqOpts := []option.RequestOption{option.WithAPIKey(opts.APIKey), option.WithMaxRetries(0)}
	if opts.BaseURL != "" {
		reqOpts = append(reqOpts, option.WithBaseURL(opts.BaseURL))
	}
	return &anthropicProvider{name: name, client: anthropic.NewClient(reqOpts...), opts: opts}
}

func (p *anthropicProvider) Name() string { return p.name }

func (p *anthropicProvider) Models(ctx context.Context) ([]string, error) {
	var ids []string
	iter := p.client.Models.ListAutoPaging(ctx, anthropic.ModelListParams{})
	for iter.Next() {
		ids = append(ids, iter.Current().ID)
	}
	return ids, iter.Err()
}

func (p *anthropicProvider) verify(ctx context.Context) error {
	_, err := p.client.Models.List(ctx, anthropic.ModelListParams{})
	var apierr *anthropic.Error
	if errors.As(err, &apierr) && (apierr.StatusCode == http.StatusUnauthorized || apierr.StatusCode == http.StatusForbidden) {
		return fmt.Errorf("%w: %s", ErrUnauthorized, i18n.T("llm.invalid_key"))
	}
	return err
}

func (p *anthropicProvider) NewChat(model, system string, tools []ToolSpec) Chat {
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: 16000,
		// Cache the conversation up to the latest message too: every turn
		// resends the history, including command output and fetched pages.
		CacheControl: anthropic.NewCacheControlEphemeralParam(),
		System: []anthropic.TextBlockParam{{
			Text:         system,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}},
	}
	for _, t := range tools {
		tp := anthropic.ToolParam{
			Name:        t.Name,
			Description: anthropic.String(t.Description),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: t.Properties,
				Required:   t.Required,
			},
		}
		params.Tools = append(params.Tools, anthropic.ToolUnionParam{OfTool: &tp})
	}
	if p.opts.Effort != "" {
		params.OutputConfig = anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffort(p.opts.Effort)}
	}

	c := &anthropicChat{client: p.client, params: params, firstParty: p.opts.BaseURL == ""}
	c.SetModel(model)
	return c
}

func (c *anthropicChat) SetModel(model string) {
	c.params.Model = anthropic.Model(model)
	c.reqOpts = nil
	// On the first-party API, let refusals on the newest models fall back
	// server-side instead of ending the run. Proxies may reject the field.
	if c.firstParty && supportsFallbacks(model) {
		c.reqOpts = append(c.reqOpts,
			option.WithHeaderAdd("anthropic-beta", "server-side-fallback-2026-07-01"),
			option.WithJSONSet("fallbacks", "default"))
	}
}

func supportsFallbacks(model string) bool {
	switch model {
	case "claude-fable-5-1", "claude-opus-5-5", "claude-opus-5", "claude-sonnet-5-5":
		return true
	}
	return false
}

type anthropicChat struct {
	client     anthropic.Client
	params     anthropic.MessageNewParams
	reqOpts    []option.RequestOption
	firstParty bool
	// after waits out a retry. Tests replace it; nil uses time.After.
	after func(time.Duration) <-chan time.Time
}

func (c *anthropicChat) Send(ctx context.Context, text string, results []ToolResult) (*Reply, error) {
	if len(results) > 0 {
		blocks := make([]anthropic.ContentBlockParamUnion, 0, len(results)+1)
		for _, r := range results {
			blocks = append(blocks, anthropic.NewToolResultBlock(r.CallID, r.Content, r.IsError))
		}
		if text != "" {
			blocks = append(blocks, anthropic.NewTextBlock(text))
		}
		c.params.Messages = append(c.params.Messages, anthropic.NewUserMessage(blocks...))
	} else {
		c.params.Messages = append(c.params.Messages, anthropic.NewUserMessage(anthropic.NewTextBlock(text)))
	}

	resp, err := c.sendRetried(ctx)
	if err != nil {
		// Drop the message we just appended so the chat can be retried.
		c.params.Messages = c.params.Messages[:len(c.params.Messages)-1]
		return nil, describeAnthropicError(err)
	}
	c.params.Messages = append(c.params.Messages, resp.ToParam())

	reply := &Reply{Usage: Usage{
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
		CacheRead:    resp.Usage.CacheReadInputTokens,
		CacheWrite:   resp.Usage.CacheCreationInputTokens,
	}}
	var sb strings.Builder
	for _, block := range resp.Content {
		switch b := block.AsAny().(type) {
		case anthropic.TextBlock:
			sb.WriteString(b.Text)
		case anthropic.ToolUseBlock:
			reply.Calls = append(reply.Calls, ToolCall{ID: b.ID, Name: b.Name, Input: []byte(b.JSON.Input.Raw())})
		}
	}
	reply.Text = sb.String()
	switch resp.StopReason {
	case anthropic.StopReasonRefusal:
		reply.Refused = true
		reply.Calls = nil
		if reply.Text == "" {
			reply.Text = "The model declined the request: " + resp.StopDetails.Explanation
		}
	case anthropic.StopReasonMaxTokens:
		// A tool call cut off mid-input is not safe to run.
		reply.Calls = nil
		reply.Text += "\n[response truncated at max_tokens]"
	}
	return reply, nil
}

// sendRetried calls the Messages API up to four times. A network error, a 429
// or a 5xx waits retryDelay and tries again. Any other status is returned at once.
func (c *anthropicChat) sendRetried(ctx context.Context) (*anthropic.Message, error) {
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			if err := c.pause(ctx, retryDelay(attempt)); err != nil {
				return nil, err
			}
		}
		resp, err := c.client.Messages.New(ctx, c.params, c.reqOpts...)
		if err == nil {
			return resp, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		lastErr = err
		if !retryableAnthropic(err) {
			return nil, err
		}
	}
	return nil, lastErr
}

func (c *anthropicChat) pause(ctx context.Context, d time.Duration) error {
	after := c.after
	if after == nil {
		after = time.After
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-after(d):
		return nil
	}
}

func retryableAnthropic(err error) bool {
	var apierr *anthropic.Error
	if errors.As(err, &apierr) {
		return apierr.StatusCode == http.StatusTooManyRequests || apierr.StatusCode >= 500
	}
	return true
}

func describeAnthropicError(err error) error {
	var apierr *anthropic.Error
	if errors.As(err, &apierr) {
		switch apierr.StatusCode {
		case http.StatusUnauthorized:
			return fmt.Errorf("%s (401): %w", i18n.T("llm.invalid_key"), err)
		case http.StatusNotFound:
			return fmt.Errorf("%s (404): %w", i18n.T("llm.not_found"), err)
		case http.StatusTooManyRequests:
			return fmt.Errorf("%s (429): %w", i18n.T("llm.rate_limit"), err)
		}
	}
	return err
}
