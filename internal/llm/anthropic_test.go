package llm

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const anthropicOK = `{"id":"msg_1","type":"message","role":"assistant","model":"claude-served","content":[{"type":"text","text":"hello"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`

func anthropicTestChat(t *testing.T, h http.HandlerFunc) (*anthropicChat, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	p := newAnthropic("anthropic", Options{APIKey: "sk-ant-test", BaseURL: srv.URL})
	return p.NewChat("claude-test", "sys", nil).(*anthropicChat), srv
}

func noWait(d time.Duration, got *[]time.Duration) func(time.Duration) <-chan time.Time {
	return func(wait time.Duration) <-chan time.Time {
		*got = append(*got, wait)
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	}
}

func TestAnthropicRetries429And500(t *testing.T) {
	var hits int
	var body string
	chat, _ := anthropicTestChat(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path %s", r.URL.Path)
		}
		if hits < 3 {
			code := http.StatusTooManyRequests
			if hits == 2 {
				code = http.StatusInternalServerError
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"slow"}}`))
			return
		}
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(anthropicOK))
	})
	var waits []time.Duration
	chat.after = noWait(0, &waits)

	reply, err := chat.Send(context.Background(), "hello there", nil)
	if err != nil {
		t.Fatal(err)
	}
	if hits != 3 {
		t.Fatalf("requests = %d, want 3", hits)
	}
	if reply.Text != "hello" {
		t.Fatalf("text %q", reply.Text)
	}
	if len(waits) != 2 || waits[0] != 2*time.Second || waits[1] != 4*time.Second {
		t.Fatalf("waits %v", waits)
	}
	if strings.Count(body, `"role":"user"`) != 1 {
		t.Fatalf("user messages in the successful request: %s", body)
	}
	if len(chat.params.Messages) != 2 {
		t.Fatalf("history length %d, want 2", len(chat.params.Messages))
	}
}

func TestAnthropicHonorsRetryAfter(t *testing.T) {
	var hits int
	chat, _ := anthropicTestChat(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "12")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"slow"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(anthropicOK))
	})
	var waits []time.Duration
	chat.after = noWait(0, &waits)
	reply, err := chat.Send(context.Background(), "hi", nil)
	if err != nil {
		t.Fatal(err)
	}
	if reply.Text != "hello" || hits != 2 {
		t.Fatalf("text %q hits %d", reply.Text, hits)
	}
	if len(waits) != 1 || waits[0] != 12*time.Second {
		t.Fatalf("waits %v, want 12s from Retry-After", waits)
	}
}

func TestAnthropicDoesNotRetryClientErrors(t *testing.T) {
	var hits int
	chat, _ := anthropicTestChat(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"no"}}`))
	})
	chat.after = func(time.Duration) <-chan time.Time {
		t.Error("backoff after a 400")
		ch := make(chan time.Time)
		return ch
	}
	if _, err := chat.Send(context.Background(), "first", nil); err == nil {
		t.Fatal("expected an error")
	}
	if hits != 1 {
		t.Fatalf("requests = %d, want 1", hits)
	}
	if len(chat.params.Messages) != 0 {
		t.Fatalf("failed send left %d messages", len(chat.params.Messages))
	}
}

func TestAnthropicRetryStopsWhenContextIsCancelled(t *testing.T) {
	var hits int
	chat, _ := anthropicTestChat(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"api_error","message":"down"}}`))
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	chat.after = func(time.Duration) <-chan time.Time {
		cancel()
		return nil
	}
	_, err := chat.Send(ctx, "hi", nil)
	if err != context.Canceled {
		t.Fatalf("err = %v, want context canceled", err)
	}
	if hits != 1 {
		t.Fatalf("requests = %d, want 1", hits)
	}
}

func TestAnthropicGivesUpAfterFourAttempts(t *testing.T) {
	var hits int
	chat, _ := anthropicTestChat(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"api_error","message":"down"}}`))
	})
	var waits []time.Duration
	chat.after = noWait(0, &waits)
	_, err := chat.Send(context.Background(), "hi", nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	if hits != 4 {
		t.Fatalf("requests = %d, want 4", hits)
	}
	if len(waits) != 3 || waits[0] != 2*time.Second || waits[1] != 4*time.Second || waits[2] != 8*time.Second {
		t.Fatalf("waits %v", waits)
	}
	if len(chat.params.Messages) != 0 {
		t.Fatalf("failed send left %d messages", len(chat.params.Messages))
	}
}

func TestAnthropicRetriesNetworkErrors(t *testing.T) {
	var hits int
	chat, _ := anthropicTestChat(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits == 1 {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("no hijacker")
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Fatal(err)
			}
			conn.Close()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(anthropicOK))
	})
	var waits []time.Duration
	chat.after = noWait(0, &waits)
	reply, err := chat.Send(context.Background(), "hi", nil)
	if err != nil {
		t.Fatal(err)
	}
	if hits != 2 || reply.Text != "hello" {
		t.Fatalf("hits %d text %q", hits, reply.Text)
	}
	if len(waits) != 1 || waits[0] != 2*time.Second {
		t.Fatalf("waits %v", waits)
	}
}

// A reply with no content, and a tool that printed nothing, used to leave an
// empty block in the history: the API then refused every later message.
func TestAnthropicEmptyReplyAndEmptyResultStayValid(t *testing.T) {
	var bodies []string
	chat, _ := anthropicTestChat(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		w.Header().Set("Content-Type", "application/json")
		switch len(bodies) {
		case 1:
			_, _ = w.Write([]byte(`{"id":"m1","type":"message","role":"assistant","model":"m","content":[{"type":"text","text":""}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":0}}`))
		case 2:
			_, _ = w.Write([]byte(`{"id":"m2","type":"message","role":"assistant","model":"m","content":[{"type":"tool_use","id":"tu_1","name":"run_command","input":{"command":"true"}}],"stop_reason":"tool_use","usage":{"input_tokens":3,"output_tokens":1}}`))
		default:
			_, _ = w.Write([]byte(anthropicOK))
		}
	})
	if _, err := chat.Send(context.Background(), "hi", nil); err != nil {
		t.Fatal(err)
	}
	reply, err := chat.Send(context.Background(), "go on", nil)
	if err != nil || len(reply.Calls) != 1 {
		t.Fatalf("calls %v, err %v", reply.Calls, err)
	}
	if !strings.Contains(bodies[1], emptyReply) || strings.Contains(bodies[1], `"text":""`) {
		t.Fatalf("the empty reply went back as it came: %s", bodies[1])
	}
	if _, err := chat.Send(context.Background(), "", []ToolResult{{CallID: "tu_1", Content: ""}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(bodies[2], emptyResult) || strings.Contains(bodies[2], `"text":""`) {
		t.Fatalf("the empty tool result was sent empty: %s", bodies[2])
	}
}

// Compact drops the old tool outputs and keeps every call paired with a
// result, so the conversation is still accepted.
func TestAnthropicCompactKeepsPairs(t *testing.T) {
	var last string
	turn := 0
	chat, _ := anthropicTestChat(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		last = string(b)
		turn++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"m","type":"message","role":"assistant","model":"m","content":[{"type":"tool_use","id":"tu_` + string(rune('0'+turn)) + `","name":"run_command","input":{"command":"ls"}}],"stop_reason":"tool_use","usage":{"input_tokens":3,"output_tokens":1}}`))
	})
	reply, err := chat.Send(context.Background(), "start", nil)
	for i := 1; i <= 5; i++ {
		if err != nil {
			t.Fatal(err)
		}
		out := "OUTPUT-" + string(rune('0'+i)) + strings.Repeat(" long output", 50)
		reply, err = chat.Send(context.Background(), "", []ToolResult{{CallID: reply.Calls[0].ID, Content: out}})
	}
	if err != nil {
		t.Fatal(err)
	}
	if n := chat.Compact(); n != 2 {
		t.Fatalf("Compact() = %d, want the 2 results older than the last %d turns", n, keepTurns)
	}
	if n := chat.Compact(); n != 0 {
		t.Fatalf("a second Compact() shortened %d more", n)
	}
	if _, err := chat.Send(context.Background(), "", []ToolResult{{CallID: reply.Calls[0].ID, Content: "OUTPUT-6"}}); err != nil {
		t.Fatal(err)
	}
	if strings.Count(last, Omitted) != 2 || strings.Contains(last, "OUTPUT-1") || strings.Contains(last, "OUTPUT-2") || !strings.Contains(last, "OUTPUT-3") {
		t.Fatalf("old outputs not replaced, or new ones lost: %s", last)
	}
	for i := 1; i <= 6; i++ {
		id := `"tu_` + string(rune('0'+i)) + `"`
		if strings.Count(last, `"id":`+id) != 1 || strings.Count(last, `"tool_use_id":`+id) != 1 {
			t.Fatalf("call %s lost its result (or the result its call): %s", id, last)
		}
	}
}

func TestIsContextLength(t *testing.T) {
	for msg, want := range map[string]bool{
		`400 Bad Request {"type":"error","error":{"type":"invalid_request_error","message":"prompt is too long: 210000 tokens > 200000 maximum"}}`: true,
		`request failed: HTTP 400: This model's maximum context length is 128000 tokens. However, your messages resulted in 130000 tokens.`:        true,
		`request failed: HTTP 400: context_length_exceeded`:                                 true,
		`request failed: HTTP 400: the request exceeds the available context size`:          true,
		`request failed: HTTP 400: Input is too long for requested model.`:                  true,
		`request failed: HTTP 429 (rate limit): slow down`:                                  false,
		`unauthorized: HTTP 401`:                                                            false,
		`request failed: HTTP 400: exceeds the context window of this model`:                true,
		`request failed: HTTP 413: Please reduce the length of the messages or completion.`: true,
	} {
		if got := IsContextLength(errors.New(msg)); got != want {
			t.Errorf("IsContextLength(%q) = %v, want %v", msg, got, want)
		}
	}
	if IsContextLength(nil) {
		t.Error("nil is not an error")
	}
}

// OpenCode Zen serves Claude on one wire format and the rest on another: a
// conversation cannot change sides, and says so instead of sending one
// format to the other endpoint.
func TestOpenCodeRefusesAModelOfTheOtherEndpoint(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "anthropic") {
			_, _ = w.Write([]byte(anthropicOK))
			return
		}
		_, _ = w.Write([]byte(openAIChatOK))
	}))
	defer srv.Close()
	p := newOpenCode(Options{APIKey: "k", BaseURL: srv.URL})

	chat := p.NewChat("claude-sonnet-4-6", "sys", nil)
	if err := SwitchModel(chat, "kimi-k2"); err == nil {
		t.Fatal("switching from Claude to an OpenAI-format model must be refused")
	}
	chat.SetModel("kimi-k2") // no way to report it: the switch is not made
	if err := SwitchModel(chat, "claude-haiku-4-5"); err != nil {
		t.Fatalf("another Claude model is fine: %v", err)
	}
	if _, err := chat.Send(context.Background(), "hi", nil); err != nil {
		t.Fatal(err)
	}
	other := p.NewChat("kimi-k2", "sys", nil)
	if err := SwitchModel(other, "claude-sonnet-4-6"); err == nil {
		t.Fatal("and the other way round")
	}
	if _, err := other.Send(context.Background(), "hi", nil); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || !strings.Contains(paths[0], "/anthropic/") || !strings.Contains(paths[1], "/openai/") {
		t.Fatalf("each chat stays on its endpoint: %v", paths)
	}
	if _, ok := chat.(Compactor); !ok {
		t.Error("an OpenCode chat can be compacted like the others")
	}
}
