package repl

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestREPLRun(t *testing.T) {
	input := strings.NewReader("help\ntest\nexit\n")
	output := &bytes.Buffer{}

	r := New(input, output, "dev")
	called := false
	r.Register("test", func(ctx context.Context, args []string) error {
		called = true
		return nil
	})

	err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("REPL.Run failed: %v", err)
	}
	if !called {
		t.Errorf("expected 'test' command to be called")
	}
	if !strings.Contains(output.String(), "Available commands:") {
		t.Errorf("expected output to contain help text")
	}
}
