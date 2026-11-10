package runner

import (
	"context"
	"os"
	"runtime"
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

func TestCaptureBoundsAHugeLine(t *testing.T) {
	const raw = 200_000
	c := &capture{}
	c.add(strings.Repeat("x", raw) + "\n")
	got := c.String()
	const budget = headBytes + tailBytes + 64
	if len(got) > budget {
		t.Fatalf("kept %d bytes of a %d byte line, budget %d", len(got), raw, budget)
	}
	if !strings.Contains(got, "omitted") {
		t.Fatalf("huge line was not marked omitted: %d bytes kept", len(got))
	}
	t.Logf("a %d byte line is kept as %d bytes (head %d + tail %d)", raw, len(got), headBytes, tailBytes)
}

func TestRunEcho(t *testing.T) {
	res := Run(context.Background(), "echo omega-ok", 15*time.Second, nil)
	if res.ExitCode != 0 || !strings.Contains(res.Output, "omega-ok") {
		t.Fatalf("echo failed: exit %d output %q", res.ExitCode, res.Output)
	}
}

// The model providers' keys stay with omega: a command must not see them,
// because its output is sent to the model.
func TestWithheldVariablesDoNotReachCommands(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the command below is written for a POSIX shell")
	}
	t.Setenv("OMEGA_TEST_PROVIDER_KEY", "sk-must-not-leak-0001")
	t.Setenv("OMEGA_TEST_OTHER", "visible")
	Withhold("OMEGA_TEST_PROVIDER_KEY")
	t.Cleanup(func() { delete(withheld, "OMEGA_TEST_PROVIDER_KEY") })
	res := Run(context.Background(), `echo "key=[$OMEGA_TEST_PROVIDER_KEY] other=[$OMEGA_TEST_OTHER]"; env | grep -c OMEGA_TEST_PROVIDER_KEY`, 10*time.Second, nil)
	if strings.Contains(res.Output, "sk-must-not-leak") || !strings.Contains(res.Output, "key=[]") || !strings.Contains(res.Output, "other=[visible]") {
		t.Fatalf("output: %q", res.Output)
	}
}
