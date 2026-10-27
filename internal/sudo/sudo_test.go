package sudo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
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
	os.WriteFile(helper, []byte(strings.Replace(string(data), shellQuote(exe), shellQuote(bin), 1)), 0o700)

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
