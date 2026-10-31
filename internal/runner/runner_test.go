package runner

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestShellExists(t *testing.T) {
	t.Setenv("SHELL", "/no/such/omega-shell")
	sh := Shell()
	if _, err := os.Stat(sh); err != nil && sh != "cmd.exe" {
		t.Fatalf("Shell() = %q, not a usable shell", sh)
	}
}

func TestExtendPathUsesListSeparator(t *testing.T) {
	t.Setenv("PATH", string(os.PathListSeparator)+"usr-bin-only")
	ExtendPath()
	got := os.Getenv("PATH")
	if !strings.Contains(got, "/opt/homebrew/bin") {
		t.Fatalf("PATH missing homebrew dir: %s", got)
	}
	if !strings.Contains(got, string(os.PathListSeparator)) {
		t.Fatalf("PATH %q has no list separator", got)
	}
}

func TestRunEcho(t *testing.T) {
	res := Run(context.Background(), "echo omega-ok", 15*time.Second, nil)
	if res.ExitCode != 0 || !strings.Contains(res.Output, "omega-ok") {
		t.Fatalf("echo failed: exit %d output %q", res.ExitCode, res.Output)
	}
}
