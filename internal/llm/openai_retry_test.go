package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const openAIChatOK = `{"choices":[{"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`

func TestOpenAIHonorsRetryAfter(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path %s", r.URL.Path)
		}
		if hits == 1 {
			w.Header().Set("Retry-After", "12")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"slow"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(openAIChatOK))
	}))
	defer srv.Close()

	p := newOpenAICompat("openai", Options{APIKey: "sk-test", BaseURL: srv.URL})
	var waits []time.Duration
	p.after = noWait(0, &waits)
	chat := p.NewChat("m", "sys", nil)

	reply, err := chat.Send(context.Background(), "hi", nil)
	if err != nil {
		t.Fatal(err)
	}
	if reply.Text != "hello" {
		t.Fatalf("text %q", reply.Text)
	}
	if hits != 2 {
		t.Fatalf("requests = %d, want 2", hits)
	}
	if len(waits) != 1 || waits[0] != 12*time.Second {
		t.Fatalf("waits %v, want 12s from Retry-After", waits)
	}
}

func TestOpenAICapsRetryAfter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"slow"}}`))
	}))
	defer srv.Close()

	p := newOpenAICompat("openai", Options{APIKey: "sk-test", BaseURL: srv.URL})
	var waits []time.Duration
	p.after = noWait(0, &waits)
	if _, err := p.NewChat("m", "sys", nil).Send(context.Background(), "hi", nil); err == nil {
		t.Fatal("expected an error")
	}
	if len(waits) != 3 || waits[0] != maxRetryAfter || waits[1] != maxRetryAfter || waits[2] != maxRetryAfter {
		t.Fatalf("waits %v, want three capped pauses", waits)
	}
}

func TestOpenAIKeepsTheScheduleWithoutRetryAfter(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"slow"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(openAIChatOK))
	}))
	defer srv.Close()

	p := newOpenAICompat("openai", Options{APIKey: "sk-test", BaseURL: srv.URL})
	var waits []time.Duration
	p.after = noWait(0, &waits)
	if _, err := p.NewChat("m", "sys", nil).Send(context.Background(), "hi", nil); err != nil {
		t.Fatal(err)
	}
	if len(waits) != 1 || waits[0] != 2*time.Second {
		t.Fatalf("waits %v, want the 2s schedule", waits)
	}
}
