package deepclean

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAgentScratchNames(t *testing.T) {
	for _, name := range []string{".claude-run.json", ".claude-run-3.json", "PLAN.md", ".pr-body.md", ".pr-body2.md",
		".pr-body-master.md", ".issue-body.md", ".review.diff", ".review2.diff", ".spec-2495.md", "wt-10316.codex.txt",
		"wt-2622-canary.codex1.txt", ".codex-review-07c37840ba.txt", ".aider.chat.history.md"} {
		if !IsAgentScratch(name) {
			t.Errorf("%s is an agent's file", name)
		}
	}
	for _, name := range []string{"README.md", "main.go", "plan.go", "notes.md", "CHANGELOG.md", ".env", "review.go", ".c2899.md"} {
		if IsAgentScratch(name) {
			t.Errorf("%s may be the user's: it is not an agent's file", name)
		}
	}
}

func TestParseStatusSplitsWhoseFilesTheyAre(t *testing.T) {
	ch := parseStatus(" M src/a.go\x00?? PLAN.md\x00?? notes/\x00A  b.go\x00?? .claude-run.json\x00!! dist/\x00")
	if strings.Join(ch.tracked, ",") != "src/a.go,b.go" || strings.Join(ch.scratch, ",") != "PLAN.md,.claude-run.json" ||
		strings.Join(ch.others, ",") != "notes/" {
		t.Fatalf("got %+v", ch)
	}
}

func TestParseWorktrees(t *testing.T) {
	out := "worktree /r\nHEAD aaa\nbranch refs/heads/main\n\nworktree /r/wt\nHEAD bbb\ndetached\nlocked\n\nworktree /gone\nHEAD ccc\nbranch refs/heads/x\nprunable gitdir file points to non-existent location\n"
	wts := parseWorktrees(out)
	if len(wts) != 3 || wts[0].branch != "main" || wts[1].branch != "" || !wts[1].locked || !wts[2].prunable {
		t.Fatalf("got %+v", wts)
	}
}

// repo makes a repository with one commit pushed to a bare origin.
func repo(t *testing.T) (string, func(dir string, args ...string) string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	base, _ := filepath.EvalSymlinks(t.TempDir())
	run := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t",
			"GIT_COMMITTER_EMAIL=t@t", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	origin := filepath.Join(base, "origin.git")
	main := filepath.Join(base, "work", "app")
	os.MkdirAll(main, 0o755)
	run(base, "init", "-q", "--bare", "-b", "main", origin)
	run(main, "init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(main, "a.txt"), []byte("a\n"), 0o644)
	run(main, "add", ".")
	run(main, "commit", "-q", "-m", "one")
	run(main, "remote", "add", "origin", origin)
	run(main, "push", "-q", "-u", "origin", "main")
	run(main, "remote", "set-head", "origin", "main")
	return main, run
}

func fakeEnv(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	oldBusy, oldSelf, oldClock, oldTrash := busyDirs, selfDir, clock, trash
	busyDirs = func(context.Context) []string { return nil }
	selfDir = func() string { return "/nowhere" }
	// The commits were made a minute ago: look from a week later, so
	// they are not "recent work".
	later := time.Now().Add(7 * 24 * time.Hour)
	clock = func() time.Time { return later }
	trash = func(p string) (string, error) { return "", os.Remove(p) }
	t.Cleanup(func() { busyDirs, selfDir, clock, trash = oldBusy, oldSelf, oldClock, oldTrash })
}

func scan(t *testing.T, main string) Report {
	t.Helper()
	return Scan(context.Background(), Options{Roots: []string{filepath.Dir(main)}, Repos: []string{main}, Now: clock()})
}

func finding(t *testing.T, r Report, path string) Finding {
	t.Helper()
	for _, f := range r.Findings {
		if f.Path == path {
			return f
		}
	}
	t.Fatalf("no finding for %s in %+v", path, r.Findings)
	return Finding{}
}

func TestAWorktreeWithOnlyTheAgentsFilesIsSafeAndGoesKeepingItsBranch(t *testing.T) {
	fakeEnv(t)
	main, run := repo(t)
	wt := filepath.Join(filepath.Dir(main), ".claude", "worktrees", "feature")
	run(main, "worktree", "add", "-q", "-b", "feature", wt)
	run(wt, "push", "-q", "-u", "origin", "feature")
	os.WriteFile(filepath.Join(wt, "PLAN.md"), []byte("plan"), 0o644)
	os.WriteFile(filepath.Join(wt, ".claude-run.json"), []byte("{}"), 0o644)

	f := finding(t, scan(t, main), wt)
	if f.Verdict != Safe || f.Owner != "Claude Code" || len(f.Rescue) != 0 {
		t.Fatalf("got %+v", f)
	}
	res, err := Apply(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Errorf("the worktree is still there")
	}
	if res.Rescued != "" {
		t.Errorf("nothing needed rescuing, got %s", res.Rescued)
	}
	if run(main, "branch", "--list", "feature") == "" {
		t.Errorf("the branch went with the worktree")
	}
}

func TestAWorktreeWithWorkHasItSavedFirst(t *testing.T) {
	fakeEnv(t)
	main, run := repo(t)
	wt := filepath.Join(filepath.Dir(main), "wt-fix")
	run(main, "worktree", "add", "-q", "-b", "fix", wt)
	os.WriteFile(filepath.Join(wt, "a.txt"), []byte("changed\n"), 0o644)
	os.MkdirAll(filepath.Join(wt, "notes"), 0o755)
	os.WriteFile(filepath.Join(wt, "notes", "idea.txt"), []byte("mine"), 0o644)

	f := finding(t, scan(t, main), wt)
	if f.Verdict != Review || f.Remote == nil || strings.Join(f.Remote.FilesOnlyHere, ",") != "a.txt,notes/idea.txt" {
		t.Fatalf("got %+v", f)
	}
	res, err := Apply(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	patch, err := os.ReadFile(filepath.Join(res.Rescued, "changes.patch"))
	if err != nil || !strings.Contains(string(patch), "+changed") {
		t.Errorf("the patch was not kept: %v %q", err, patch)
	}
	if b, err := os.ReadFile(filepath.Join(res.Rescued, "files", "notes", "idea.txt")); err != nil || string(b) != "mine" {
		t.Errorf("the untracked file was not copied: %v", err)
	}
}

func TestACommitOnNoBranchGetsOne(t *testing.T) {
	fakeEnv(t)
	main, run := repo(t)
	wt := filepath.Join(filepath.Dir(main), "wt-detached")
	run(main, "worktree", "add", "-q", "--detach", wt)
	os.WriteFile(filepath.Join(wt, "b.txt"), []byte("b\n"), 0o644)
	run(wt, "add", ".")
	run(wt, "commit", "-q", "-m", "lost")
	head := run(wt, "rev-parse", "HEAD")

	f := finding(t, scan(t, main), wt)
	if f.Verdict != Review {
		t.Fatalf("got %+v", f)
	}
	if _, err := Apply(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if got := run(main, "rev-parse", "omega/rescued/wt-detached"); got != head {
		t.Errorf("rescue branch points to %s, want %s", got, head)
	}
}

func TestApplyRefusesWhatChangedSinceTheScan(t *testing.T) {
	fakeEnv(t)
	main, run := repo(t)
	wt := filepath.Join(filepath.Dir(main), "wt-busy")
	run(main, "worktree", "add", "-q", "-b", "busy", wt)
	run(wt, "push", "-q", "-u", "origin", "busy")
	f := finding(t, scan(t, main), wt)
	if f.Verdict != Safe {
		t.Fatalf("got %+v", f)
	}
	// Somebody starts working in it after the scan.
	os.WriteFile(filepath.Join(wt, "a.txt"), []byte("new work\n"), 0o644)
	if _, err := Apply(context.Background(), f); err == nil || !strings.Contains(err.Error(), "changed since") {
		t.Fatalf("want a refusal, got %v", err)
	}
	if _, err := os.Stat(wt); err != nil {
		t.Errorf("the worktree was removed: %v", err)
	}
}

func TestAWorktreeInUseOrOmegasOwnIsKept(t *testing.T) {
	fakeEnv(t)
	main, run := repo(t)
	wt := filepath.Join(filepath.Dir(main), "wt-open")
	run(main, "worktree", "add", "-q", "-b", "open", wt)
	busyDirs = func(context.Context) []string { return []string{filepath.Join(wt, "src")} }
	f := finding(t, scan(t, main), wt)
	if f.Verdict != Keep {
		t.Fatalf("a worktree a process works in must be kept, got %+v", f)
	}
	if _, err := Apply(context.Background(), f); err == nil {
		t.Errorf("a kept finding was removed")
	}
	busyDirs = func(context.Context) []string { return nil }
	selfDir = func() string { return wt }
	if f := finding(t, scan(t, main), wt); f.Verdict != Keep {
		t.Fatalf("omega's own folder must be kept, got %+v", f)
	}
}

func TestRecentWorkIsOnlyForReview(t *testing.T) {
	fakeEnv(t)
	clock = time.Now
	main, run := repo(t)
	wt := filepath.Join(filepath.Dir(main), "wt-today")
	run(main, "worktree", "add", "-q", "-b", "today", wt)
	run(wt, "push", "-q", "-u", "origin", "today")
	if f := finding(t, scan(t, main), wt); f.Verdict != Review {
		t.Fatalf("a worktree committed to today is not safe to remove, got %+v", f)
	}
}

func TestLooseAgentFilesGoToTheTrash(t *testing.T) {
	fakeEnv(t)
	root, _ := filepath.EvalSymlinks(t.TempDir())
	dir := filepath.Join(root, "company")
	os.MkdirAll(dir, 0o755)
	old := clock().Add(-10 * 24 * time.Hour)
	for _, n := range []string{"wt-1.codex.txt", "wt-2.codex1.txt", "budget.txt"} {
		p := filepath.Join(dir, n)
		os.WriteFile(p, []byte("x"), 0o644)
		os.Chtimes(p, old, old)
	}
	r := Scan(context.Background(), Options{Roots: []string{root}, Now: clock()})
	f := finding(t, r, dir)
	if f.Kind != KindAgentScratch || f.Verdict != Safe || f.Owner != "Codex" {
		t.Fatalf("got %+v", f)
	}
	if _, err := Apply(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "budget.txt")); err != nil {
		t.Errorf("the user's file went too")
	}
	if _, err := os.Stat(filepath.Join(dir, "wt-1.codex.txt")); !os.IsNotExist(err) {
		t.Errorf("the agent's file is still there")
	}
}

func TestDockerSizes(t *testing.T) {
	for in, want := range map[string]int64{"119.5MB": 119_500_000, "2GB": 2_000_000_000, "0B": 0, "512kB": 512_000, "junk": 0} {
		if got := ParseSize(in); got != want {
			t.Errorf("%s: got %d want %d", in, got, want)
		}
	}
}
