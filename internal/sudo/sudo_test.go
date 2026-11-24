package sudo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/runner"
)

// TestBridgeAsksThroughOmega builds omega, runs `sudo -n true` the way a tool
// such as Mole would, and checks the prompt reaches omega's ask function
// instead of the terminal, and that a cancel makes sudo fail fast.
func TestBridgeAsksThroughOmega(t *testing.T) {
	if _, err := os.Stat("/usr/bin/sudo"); err != nil {
		t.Skip("no sudo")
	}
	bin := filepath.Join(t.TempDir(), "omega")
	build := exec.Command("go", "build", "-o", bin, "../../cmd/omega")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	var asked atomic.Int32
	b, err := Start(func(prompt string, retry bool) (string, bool) {
		asked.Add(1)
		return "", false
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	// Point the helper at the test binary instead of the test runner.
	helper := filepath.Join(b.dir, "askpass")
	data, _ := os.ReadFile(helper)
	exe, _ := os.Executable()
	os.WriteFile(helper, []byte(strings.Replace(string(data), runner.ShellQuote(exe), runner.ShellQuote(bin), 1)), 0o700)

	runner.Configure(b.Env(), b.PathPrefix())
	defer runner.Configure(nil, "")
	exec.Command("/usr/bin/sudo", "-k").Run()

	start := time.Now()
	res := runner.Run(context.Background(), "sudo -n true; echo exit=$?; command -v sudo", 20*time.Second, nil)
	if time.Since(start) > 15*time.Second {
		t.Fatalf("sudo hung: %s", res.Output)
	}
	if !strings.Contains(res.Output, b.PathPrefix()) {
		t.Fatalf("wrapper not first on PATH:\n%s", res.Output)
	}
	if strings.Contains(res.Output, "exit=0") {
		t.Skip("sudo did not need a password on this machine")
	}
	if asked.Load() == 0 {
		t.Fatalf("omega was not asked for the password:\n%s", res.Output)
	}
	// After a cancel, later sudo calls must not ask again.
	runner.Run(context.Background(), "sudo -n true", 20*time.Second, nil)
	if asked.Load() != 1 {
		t.Fatalf("asked %d times, want 1", asked.Load())
	}
}

// TestHelperRunByHandGetsNothing runs the askpass helper the way a command
// could (its path is in every command's environment) while omega holds a
// password. Only sudo is answered: a run by hand gets no password, and the
// user is not asked for one.
func TestHelperRunByHandGetsNothing(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "omega")
	build := exec.Command("go", "build", "-o", bin, "../../cmd/omega")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	const secret = "Tr0ub4dor-3xyz"
	var asked atomic.Int32
	b, err := Start(func(string, bool) (string, bool) {
		asked.Add(1)
		return "", false
	}, secret)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	helper := filepath.Join(b.dir, "askpass")
	data, _ := os.ReadFile(helper)
	exe, _ := os.Executable()
	os.WriteFile(helper, []byte(strings.Replace(string(data), runner.ShellQuote(exe), runner.ShellQuote(bin), 1)), 0o700)

	for _, cmd := range []string{
		helper + " x | rev",
		"find " + runner.ShellQuote(b.dir) + " -maxdepth 1 -name 'askp*' -type f -exec {} x \\; | rev",
	} {
		out, _ := exec.Command("/bin/sh", "-c", cmd).CombinedOutput()
		if strings.Contains(string(out), "zyx3-rod4bu0rT") || strings.Contains(string(out), secret) {
			t.Errorf("%s printed the password: %q", cmd, out)
		}
	}
	if asked.Load() != 0 {
		t.Errorf("the user was asked for a password %d times by a command that is not sudo", asked.Load())
	}
}

// M7: the wrapper finds bash and sudo where the system keeps them.
func TestWrapperFindsSudoOnPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sudo and its POSIX paths")
	}
	old := lookPath
	defer func() { lookPath = old }()
	lookPath = func(string) (string, error) { return "/run/wrappers/bin/sudo", nil }
	if got := realSudo(); got != "/run/wrappers/bin/sudo" {
		t.Errorf("realSudo = %q", got)
	}
	lookPath = func(string) (string, error) { return "/tmp/omega-sudo-123/bin/sudo", nil }
	if got := realSudo(); got != "/usr/bin/sudo" {
		t.Errorf("another wrapper was taken for sudo: %q", got)
	}
	lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	if got := realSudo(); got != "/usr/bin/sudo" {
		t.Errorf("fallback = %q", got)
	}
	w := wrapper("/run/wrappers/bin/sudo")
	if !strings.HasPrefix(w, "#!/usr/bin/env bash\n") || !strings.Contains(w, "exec '/run/wrappers/bin/sudo' -A ") {
		t.Errorf("wrapper:\n%s", w)
	}
}
