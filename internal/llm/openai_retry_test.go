package llm

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
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

// A request that used up the client's whole time limit is not sent again:
// four attempts of ten minutes held a run for forty.
func TestOpenAIDoesNotRetryATimeout(t *testing.T) {
	var hits atomic.Int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		<-release
	}))
	defer srv.Close()
	defer close(release)

	p := newOpenAICompat("openai", Options{APIKey: "sk-test", BaseURL: srv.URL})
	p.http = &http.Client{Timeout: 100 * time.Millisecond}
	var waits []time.Duration
	p.after = noWait(0, &waits)
	_, err := p.NewChat("m", "sys", nil).Send(context.Background(), "hi", nil)
	if err == nil || hits.Load() != 1 || len(waits) != 0 {
		t.Fatalf("err %v, requests %d, waits %v: want one request and no retry", err, hits.Load(), waits)
	}
}

// A reply with neither text nor tool calls, and a tool that printed
// nothing, must not leave an empty message in the history.
func TestOpenAIEmptyReplyAndEmptyResultStayValid(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		w.Header().Set("Content-Type", "application/json")
		switch len(bodies) {
		case 1:
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":null},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":0}}`))
		case 2:
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"run_command","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
		default:
			_, _ = w.Write([]byte(openAIChatOK))
		}
	}))
	defer srv.Close()
	chat := newOpenAICompat("openai", Options{APIKey: "sk-test", BaseURL: srv.URL}).NewChat("m", "sys", nil)
	if _, err := chat.Send(context.Background(), "hi", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := chat.Send(context.Background(), "go on", nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(bodies[1], `"content":"`+emptyReply+`"`) {
		t.Fatalf("the empty reply went back without content: %s", bodies[1])
	}
	if _, err := chat.Send(context.Background(), "", []ToolResult{{CallID: "c1", Content: ""}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(bodies[2], `"content":"`+emptyResult+`"`) || !strings.Contains(bodies[2], `"tool_calls"`) {
		t.Fatalf("the empty tool result was sent empty: %s", bodies[2])
	}
}

// Compact drops the old tool outputs; every tool message keeps its call.
func TestOpenAICompactKeepsPairs(t *testing.T) {
	var last string
	turn := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		last = string(b)
		turn++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"c` + strconv.Itoa(turn) + `","type":"function","function":{"name":"run_command","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer srv.Close()
	chat := newOpenAICompat("openai", Options{APIKey: "sk-test", BaseURL: srv.URL}).NewChat("m", "sys", nil)
	reply, err := chat.Send(context.Background(), "start", nil)
	for i := 1; i <= 5; i++ {
		if err != nil {
			t.Fatal(err)
		}
		out := "OUTPUT-" + strconv.Itoa(i) + strings.Repeat(" long output", 50)
		reply, err = chat.Send(context.Background(), "", []ToolResult{{CallID: reply.Calls[0].ID, Content: out}})
	}
	if err != nil {
		t.Fatal(err)
	}
	c := chat.(Compactor)
	// Six assistant turns called tools; five have a result so far. The
	// results before the last three calls are old: those of calls 1, 2 and 3.
	if n := c.Compact(); n != 3 {
		t.Fatalf("Compact() = %d, want 3", n)
	}
	if n := c.Compact(); n != 0 {
		t.Fatalf("a second Compact() shortened %d more", n)
	}
	if _, err := chat.Send(context.Background(), "", []ToolResult{{CallID: reply.Calls[0].ID, Content: "OUTPUT-6"}}); err != nil {
		t.Fatal(err)
	}
	if strings.Count(last, Omitted) != 3 || strings.Contains(last, "OUTPUT-1") || !strings.Contains(last, "OUTPUT-4") || !strings.Contains(last, "OUTPUT-6") {
		t.Fatalf("old outputs not replaced, or new ones lost: %s", last)
	}
	for i := 1; i <= 6; i++ {
		id := `"c` + strconv.Itoa(i) + `"`
		if strings.Count(last, `"id":`+id) != 1 || strings.Count(last, `"tool_call_id":`+id) != 1 {
			t.Fatalf("call %s lost its result (or the result its call): %s", id, last)
		}
	}
}
