package gitops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lcajigasm/omega/internal/runner"
)

func TestDefaultRoots(t *testing.T) {
	roots := DefaultRoots()
	t.Logf("Found %d default dev roots", len(roots))
}

func TestCheckRepo(t *testing.T) {
	tmp := t.TempDir()
	// Init test git repo
	res := runner.Run(context.Background(), "git -C "+tmp+" init", 0, nil)
	if res.ExitCode != 0 {
		t.Skip("git not available in test environment")
	}

	testFile := filepath.Join(tmp, "test.txt")
	_ = os.WriteFile(testFile, []byte("hello world"), 0644)

	st, err := CheckRepo(context.Background(), tmp)
	if err != nil {
		t.Fatalf("CheckRepo failed: %v", err)
	}
	if !st.HasUncommitted {
		t.Errorf("expected repo to have uncommitted changes")
	}
}

// A repository git could not answer for in time is reported, not passed
// over as if it were clean.
func TestCheckRepoTimeoutIsAnError(t *testing.T) {
	old := git
	defer func() { git = old }()
	git = func(_ context.Context, _ string, args ...string) (string, error) {
		return "", fmt.Errorf("git %s: %w", args[0], ErrGitTimeout)
	}
	st, err := CheckRepo(context.Background(), t.TempDir())
	if !errors.Is(err, ErrGitTimeout) {
		t.Fatalf("CheckRepo = %+v, %v; want the timeout", st, err)
	}
	// Only the first question must be answered: a branch with no upstream
	// is not a repository that could not be checked.
	git = func(_ context.Context, _ string, args ...string) (string, error) {
		if args[0] == "status" {
			return " M file\n", nil
		}
		return "", errors.New("fatal: no upstream configured")
	}
	st, err = CheckRepo(context.Background(), t.TempDir())
	if err != nil || !st.HasUncommitted {
		t.Errorf("CheckRepo = %+v, %v", st, err)
	}
}

// A merged branch that is checked out in a worktree cannot be deleted: it
// is not offered, and when asked for anyway the failure is reported.
func TestMergedBranchesInAWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	repo := filepath.Join(dir, "repo")
	os.MkdirAll(repo, 0o755)
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	os.WriteFile(filepath.Join(repo, "file"), []byte("x"), 0o644)
	run("add", "-A")
	run("commit", "-q", "-m", "one")
	run("branch", "old")
	run("branch", "feature")
	run("worktree", "add", "-q", filepath.Join(dir, "wt"), "feature")

	ctx := context.Background()
	st, err := CheckRepo(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(st.MergedBranches, ",") != "old" {
		t.Errorf("merged branches offered: %v, want only old", st.MergedBranches)
	}
	deleted, err := DeleteMergedBranches(ctx, repo, []string{"old", "feature"})
	if strings.Join(deleted, ",") != "old" {
		t.Errorf("deleted: %v", deleted)
	}
	if err == nil || !strings.Contains(err.Error(), "feature") {
		t.Errorf("the branch git refused to delete was not reported: %v", err)
	}
}
