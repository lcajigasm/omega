package repl

import (
	"bytes"
	"context"
	"errors"
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

// exitStatus is an error with a status and no text, as a run that already
// showed its failure returns.
type exitStatus int

func (e exitStatus) Error() string { return "" }
func (e exitStatus) ExitCode() int { return int(e) }

// A command that failed without a message does not print a bare "Error: ",
// and the help lists every built-in command, status included.
func TestREPLFailureAndHelp(t *testing.T) {
	output := &bytes.Buffer{}
	r := New(strings.NewReader("quiet\nloud\nhelp\nexit\n"), output, "dev")
	r.Register("quiet", func(context.Context, []string) error { return exitStatus(2) })
	r.Register("loud", func(context.Context, []string) error { return errors.New("no network") })
	if err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	out := output.String()
	if strings.Contains(out, "Error: \n") || !strings.Contains(out, "failed (exit 2)") {
		t.Errorf("a failure without text: %q", out)
	}
	if !strings.Contains(out, "Error: no network") {
		t.Errorf("a failure with text: %q", out)
	}
	if !strings.Contains(out, "  status ") {
		t.Errorf("help does not list status: %q", out)
	}
}
