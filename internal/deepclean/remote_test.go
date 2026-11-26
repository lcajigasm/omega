package deepclean

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Uncommitted files whose exact content is already on a remote branch
// need no rescue: the worktree is safe.
func TestUncommittedFilesAlreadyOnTheRemoteNeedNoRescue(t *testing.T) {
	fakeEnv(t)
	main, run := repo(t)
	wt := filepath.Join(filepath.Dir(main), "wt-done")
	run(main, "worktree", "add", "-q", "-b", "done", wt)
	run(wt, "push", "-q", "-u", "origin", "done")
	// The same change, committed and pushed to main from the main checkout.
	os.WriteFile(filepath.Join(main, "a.txt"), []byte("final\n"), 0o644)
	os.WriteFile(filepath.Join(main, "new.txt"), []byte("new\n"), 0o644)
	run(main, "add", ".")
	run(main, "commit", "-q", "-m", "landed")
	run(main, "push", "-q", "origin", "main")
	// The worktree still has it uncommitted.
	os.WriteFile(filepath.Join(wt, "a.txt"), []byte("final\n"), 0o644)
	os.WriteFile(filepath.Join(wt, "new.txt"), []byte("new\n"), 0o644)

	f := finding(t, scan(t, main), wt)
	if f.Verdict != Safe || len(f.Rescue) != 0 || f.Remote == nil || len(f.Remote.FilesOnRemote) != 2 || f.Remote.Server != "checked" {
		t.Fatalf("got %+v remote %+v", f, f.Remote)
	}
	if !strings.Contains(strings.Join(f.Reasons, "\n"), "already on the remote with the same content") {
		t.Errorf("the reasons do not say so: %v", f.Reasons)
	}

	// One byte different is only here.
	os.WriteFile(filepath.Join(wt, "new.txt"), []byte("new!\n"), 0o644)
	f = finding(t, scan(t, main), wt)
	if f.Verdict != Review || strings.Join(f.Remote.FilesOnlyHere, ",") != "new.txt" {
		t.Fatalf("got %+v remote %+v", f, f.Remote)
	}
}

func TestCommitsNotPushedAreForReview(t *testing.T) {
	fakeEnv(t)
	main, run := repo(t)
	wt := filepath.Join(filepath.Dir(main), "wt-local")
	run(main, "worktree", "add", "-q", "-b", "local", wt)
	run(wt, "push", "-q", "-u", "origin", "local")
	os.WriteFile(filepath.Join(wt, "b.txt"), []byte("b\n"), 0o644)
	run(wt, "add", ".")
	run(wt, "commit", "-q", "-m", "not pushed")

	f := finding(t, scan(t, main), wt)
	if f.Verdict != Review || f.Remote.Unpushed != 1 || f.Remote.OnServer {
		t.Fatalf("got %+v remote %+v", f, f.Remote)
	}
	if !strings.Contains(strings.Join(f.Reasons, "\n"), "1 commits are on no remote branch") {
		t.Errorf("reasons: %v", f.Reasons)
	}
}

// A branch merged and deleted on the server is done work.
func TestABranchMergedAndDeletedOnTheServerIsSafe(t *testing.T) {
	fakeEnv(t)
	main, run := repo(t)
	wt := filepath.Join(filepath.Dir(main), "wt-merged")
	run(main, "worktree", "add", "-q", "-b", "merged", wt)
	os.WriteFile(filepath.Join(wt, "c.txt"), []byte("c\n"), 0o644)
	run(wt, "add", ".")
	run(wt, "commit", "-q", "-m", "feature")
	run(wt, "push", "-q", "-u", "origin", "merged")
	run(main, "merge", "-q", "--no-ff", "-m", "merge", "merged")
	run(main, "push", "-q", "origin", "main")
	// The pull request's branch is deleted on the server only.
	origin := run(main, "remote", "get-url", "origin")
	run(origin, "branch", "-D", "merged")

	f := finding(t, scan(t, main), wt)
	if f.Verdict != Safe || !f.Remote.BranchGone || !f.Remote.OnServer || f.Remote.FoundOn != "main" {
		t.Fatalf("got %+v remote %+v", f, f.Remote)
	}
}

// Without the server the last fetch is used, and the scan says so.
func TestAnUnreachableServerFallsBackToTheLastFetch(t *testing.T) {
	fakeEnv(t)
	main, run := repo(t)
	wt := filepath.Join(filepath.Dir(main), "wt-offline")
	run(main, "worktree", "add", "-q", "-b", "offline", wt)
	run(wt, "push", "-q", "-u", "origin", "offline")
	run(main, "remote", "set-url", "origin", filepath.Join(filepath.Dir(main), "gone.git"))

	f := finding(t, scan(t, main), wt)
	if f.Remote == nil || !strings.HasPrefix(f.Remote.Server, "not reached") || !f.Remote.OnServer || f.Verdict != Safe {
		t.Fatalf("got %+v remote %+v", f, f.Remote)
	}
}

func TestARepositoryWithoutRemoteIsNeverSafe(t *testing.T) {
	fakeEnv(t)
	main, run := repo(t)
	run(main, "remote", "remove", "origin")
	wt := filepath.Join(filepath.Dir(main), "wt-alone")
	run(main, "worktree", "add", "-q", "-b", "alone", wt)
	f := finding(t, scan(t, main), wt)
	if f.Verdict != Review {
		t.Fatalf("a worktree of a repository with no remote is its only copy: %+v", f)
	}
}
