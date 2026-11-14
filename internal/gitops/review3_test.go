package gitops

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// A repository that names a program in its own .git/config (a folder that
// was unpacked, not cloned) does not get it run by omega looking at it.
func TestCheckRepoDoesNotRunTheRepositorysMonitor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	repo := t.TempDir()
	marker := filepath.Join(t.TempDir(), "ran")
	for _, args := range [][]string{{"init", "-q"}, {"config", "core.fsmonitor", "touch " + marker + "; false"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	os.WriteFile(filepath.Join(repo, "f"), []byte("x"), 0o644)
	if _, err := CheckRepo(context.Background(), repo); err != nil {
		t.Logf("CheckRepo: %v", err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("looking at the repository ran its core.fsmonitor")
	}
}
