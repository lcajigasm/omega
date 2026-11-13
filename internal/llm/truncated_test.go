package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A reply cut by length keeps no tool call in the conversation: the next
// request would be refused for the tool message that never comes.
func TestOpenAITruncatedReplyKeepsHistoryValid(t *testing.T) {
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "application/json")
		if len(bodies) == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"length","message":{"role":"assistant","content":null,
				"tool_calls":[{"id":"c1","type":"function","function":{"name":"run_command","arguments":"{\"comm"}}]}}],
				"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
			return
		}
		_, _ = w.Write([]byte(openAIChatOK))
	}))
	defer srv.Close()

	chat := newOpenAICompat("openai", Options{APIKey: "sk-test", BaseURL: srv.URL}).NewChat("m", "sys", nil)
	reply, err := chat.Send(context.Background(), "hi", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reply.Truncated || len(reply.Calls) != 0 {
		t.Fatalf("truncated %v, calls %v", reply.Truncated, reply.Calls)
	}
	if _, err := chat.Send(context.Background(), "go on", nil); err != nil {
		t.Fatal(err)
	}
	for _, m := range bodies[1]["messages"].([]any) {
		msg := m.(map[string]any)
		if _, has := msg["tool_calls"]; has {
			t.Errorf("the conversation still holds the cut tool call: %v", msg)
		}
		if msg["role"] == "assistant" && msg["content"] == nil {
			t.Errorf("an assistant message without content: %v", msg)
		}
	}
}

// A reply that cannot be read does not leave the user's message behind.
func TestOpenAIUnreadableReplyRollsBack(t *testing.T) {
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "application/json")
		if len(bodies) == 1 {
			_, _ = w.Write([]byte(`{"choices":[]}`))
			return
		}
		_, _ = w.Write([]byte(openAIChatOK))
	}))
	defer srv.Close()

	chat := newOpenAICompat("openai", Options{APIKey: "sk-test", BaseURL: srv.URL}).NewChat("m", "sys", nil)
	if _, err := chat.Send(context.Background(), "hi", nil); err == nil {
		t.Fatal("an empty reply gave no error")
	}
	if _, err := chat.Send(context.Background(), "hi", nil); err != nil {
		t.Fatal(err)
	}
	if n := len(bodies[1]["messages"].([]any)); n != len(bodies[0]["messages"].([]any)) {
		t.Errorf("the retry sent %d messages, the first try %d", n, len(bodies[0]["messages"].([]any)))
	}
}
