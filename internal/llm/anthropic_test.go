package llm

import (
	"context"
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
