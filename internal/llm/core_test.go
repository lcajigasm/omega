package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// D1: the history is compacted before the prompt reaches the size from
// which the price rises, and otherwise at 70% of the window.
func TestCompactAtStaysUnderTheLongPromptPrice(t *testing.T) {
	old := LiveContext
	defer func() { LiveContext = old }()
	LiveContext = nil
	if got := CompactAt("anthropic", "claude-haiku-5-5"); got != 90_000 {
		t.Errorf("haiku 5.5: %d", got)
	}
	if got := CompactAt("openrouter", "anthropic/claude-haiku-5.5"); got != 90_000 {
		t.Errorf("haiku 5.5 on OpenRouter: %d", got)
	}
	if got := CompactAt("anthropic", "claude-opus-5-5"); got != 140_000 {
		t.Errorf("opus: %d", got)
	}
	LiveContext = func(string, string) (int64, bool) { return 100_000, true }
	if got := CompactAt("anthropic", "claude-haiku-5-5"); got != 70_000 {
		t.Errorf("a window smaller than the threshold: %d", got)
	}
}

// D2: an Anthropic request that hangs ends at the time limit, and is not
// tried again.
func TestAnthropicRequestTimeout(t *testing.T) {
	old := requestTimeout
	requestTimeout = 300 * time.Millisecond
	defer func() { requestTimeout = old }()
	var hits atomic.Int32
	release := make(chan struct{})
	defer close(release)
	chat, _ := anthropicTestChat(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	var waits []time.Duration
	chat.after = noWait(0, &waits)
	start := time.Now()
	if _, err := chat.Send(context.Background(), "hi", nil); err == nil {
		t.Fatal("a hung request succeeded")
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("took %s", d)
	}
	if hits.Load() != 1 {
		t.Errorf("a timed-out request was tried %d times", hits.Load())
	}
}

// D4: cached prompt tokens count as cache reads, and Claude through
// OpenRouter gets cache breakpoints on the system prompt and the last
// message; other models do not, and the history keeps plain strings.
func TestOpenAICompatCache(t *testing.T) {
	var last map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		last = nil
		json.Unmarshal(b, &last)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],` +
			`"usage":{"prompt_tokens":1000,"completion_tokens":5,"prompt_tokens_details":{"cached_tokens":800,"cache_write_tokens":150}}}`))
	}))
	defer srv.Close()

	p := newOpenAICompat("openrouter", Options{APIKey: "k", BaseURL: srv.URL})
	chat := p.NewChat("anthropic/claude-sonnet-5.5", "system prompt", nil).(*openAIChat)
	reply, err := chat.Send(context.Background(), "hello", nil)
	if err != nil {
		t.Fatal(err)
	}
	if u := reply.Usage; u.InputTokens != 50 || u.CacheRead != 800 || u.CacheWrite != 150 || u.Input() != 1000 {
		t.Errorf("usage %+v", u)
	}
	msgs := last["messages"].([]any)
	for _, i := range []int{0, len(msgs) - 1} {
		parts, ok := msgs[i].(map[string]any)["content"].([]any)
		if !ok || !strings.Contains(mustJSON(parts), `"cache_control":{"type":"ephemeral"}`) {
			t.Errorf("message %d has no breakpoint: %v", i, msgs[i])
		}
	}
	if _, ok := chat.messages[0].(map[string]any)["content"].(string); !ok {
		t.Error("the history was changed")
	}

	for _, c := range []struct{ provider, model string }{{"openrouter", "openai/gpt-5.5"}, {"openai", "gpt-5.5"}} {
		p := newOpenAICompat(c.provider, Options{APIKey: "k", BaseURL: srv.URL})
		if _, err := p.NewChat(c.model, "sys", nil).Send(context.Background(), "hi", nil); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(mustJSON(last), "cache_control") {
			t.Errorf("%s %s got cache_control", c.provider, c.model)
		}
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
