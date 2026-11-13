// Package gitops provides repository hygiene tools for scanning dev folders,
// detecting uncommitted changes, unpushed commits, merged branches, bloated .git
// directories, and large files.
package gitops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lcajigasm/omega/internal/runner"
)

// gitTimeout is how long one git command may take. A large repository on a
// cold disk needs more than a few seconds for `git status`.
var gitTimeout = 30 * time.Second

// ErrGitTimeout is returned when git did not answer in time.
var ErrGitTimeout = errors.New("git took too long")

// git runs git in a repository and returns what it printed. It is run as
// it is, not through the user's shell: starting a login shell for every
// command took longer than git itself. Tests replace it.
var git = func(ctx context.Context, repo string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
	// Looking must not write, and git must never stop to ask.
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("git %s: %w (over %s)", args[0], ErrGitTimeout, gitTimeout)
	}
	if err != nil {
		return "", fmt.Errorf("git %s: %s", args[0], firstLine(string(out), err))
	}
	return string(out), nil
}

// firstLine is the first line git printed, or the error when it printed
// nothing.
func firstLine(out string, err error) string {
	if line, _, _ := strings.Cut(strings.TrimSpace(out), "\n"); line != "" {
		return line
	}
	return err.Error()
}

// RepoStatus holds hygiene metrics for a single git repository.
type RepoStatus struct {
	Path           string   `json:"path"`
	Name           string   `json:"name"`
	Branch         string   `json:"branch"`
	HasUncommitted bool     `json:"has_uncommitted"`
	UnpushedCount  int      `json:"unpushed_count"`
	MergedBranches []string `json:"merged_branches,omitempty"`
	GitSizeMB      int64    `json:"git_size_mb"`
	LargeFiles     []string `json:"large_files,omitempty"`
}

// DefaultRoots returns common directories where users keep git repositories.
func DefaultRoots() []string {
	home, _ := os.UserHomeDir()
	candidates := []string{
		"Dev", "Projects", "Code", "src", "work", "Sites", "repos", "git",
		"Documents/GitHub", "Developer",
	}
	var existing []string
	for _, c := range candidates {
		p := filepath.Join(home, c)
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			existing = append(existing, p)
		}
	}
	return existing
}

// FindRepos walks the root directories up to depth 4 to locate git repositories.
func FindRepos(roots []string) []string {
	var repos []string
	for _, root := range roots {
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() && info.Name() == ".git" {
				repos = append(repos, filepath.Dir(path))
				return filepath.SkipDir
			}
			// Skip descending too deeply into node_modules, vendor, etc.
			if info.IsDir() && (info.Name() == "node_modules" || info.Name() == "vendor" || info.Name() == ".venv" || info.Name() == "target") {
				return filepath.SkipDir
			}
			return nil
		})
	}
	return repos
}

// CheckRepo inspects a single repository for hygiene metrics. When git
// cannot say whether there is uncommitted work (it failed, or took too
// long), that is an error: a repository that could not be checked is not a
// clean one, and the caller has to show it.
func CheckRepo(ctx context.Context, repoPath string) (*RepoStatus, error) {
	st := &RepoStatus{
		Path: repoPath,
		Name: filepath.Base(repoPath),
	}

	// Git size
	gitDir := filepath.Join(repoPath, ".git")
	st.GitSizeMB = dirSizeMB(gitDir)

	// Uncommitted changes
	out, err := git(ctx, repoPath, "status", "--porcelain")
	if err != nil {
		return st, fmt.Errorf("could not check: %w", err)
	}
	st.HasUncommitted = strings.TrimSpace(out) != ""

	// Current branch. A repository with no commit yet has none: only a
	// timeout is an error from here on.
	out, err = git(ctx, repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	if errors.Is(err, ErrGitTimeout) {
		return st, fmt.Errorf("could not check: %w", err)
	}
	if err == nil {
		st.Branch = strings.TrimSpace(out)
	}

	// Unpushed commits. A branch with no upstream has nothing to compare.
	out, err = git(ctx, repoPath, "log", "@{u}..HEAD", "--oneline")
	if errors.Is(err, ErrGitTimeout) {
		return st, fmt.Errorf("could not check: %w", err)
	}
	if err == nil && strings.TrimSpace(out) != "" {
		st.UnpushedCount = len(strings.Split(strings.TrimSpace(out), "\n"))
	}

	// Merged branches. Left out: the current one ("*"), the ones checked
	// out in another worktree ("+", which git refuses to delete) and the
	// main ones.
	out, err = git(ctx, repoPath, "branch", "--merged")
	if errors.Is(err, ErrGitTimeout) {
		return st, fmt.Errorf("could not check: %w", err)
	}
	if err == nil {
		for _, b := range strings.Split(out, "\n") {
			b = strings.TrimSpace(b)
			if b == "" || strings.HasPrefix(b, "*") || strings.HasPrefix(b, "+") || b == "main" || b == "master" || b == "develop" {
				continue
			}
			st.MergedBranches = append(st.MergedBranches, b)
		}
	}

	st.LargeFiles = largeFiles(repoPath)
	return st, nil
}

// largeFiles lists the files over 50 MB in a repository, four folders deep
// at most, .git left out. Paths are relative to the repository.
func largeFiles(repoPath string) []string {
	var out []string
	_ = filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, relErr := filepath.Rel(repoPath, path)
		if relErr != nil {
			return nil
		}
		if info.IsDir() {
			if info.Name() == ".git" || strings.Count(rel, string(filepath.Separator)) >= 3 {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Mode().IsRegular() && info.Size() > 50<<20 {
			out = append(out, rel)
		}
		return nil
	})
	return out
}

// ErrRepoBusy is returned for a repository another git process is using.
var ErrRepoBusy = errors.New("another git process is using the repository")

// OptimizeRepo runs git gc and repacks loose objects to reclaim space.
//
// It is the gc git runs by itself: objects nothing points to are kept for
// two weeks. --prune=now would delete at once what can still be recovered
// (a dropped stash, the commits of a deleted branch, a file that was staged
// and never committed), and git documents it as unsafe while anything else
// writes to the repository. A repository with a lock is left alone.
func OptimizeRepo(ctx context.Context, repoPath string) (int64, error) {
	for _, lock := range []string{"index.lock", "gc.pid", "shallow.lock", "packed-refs.lock"} {
		if _, err := os.Lstat(filepath.Join(repoPath, ".git", lock)); err == nil {
			return 0, ErrRepoBusy
		}
	}
	before := dirSizeMB(filepath.Join(repoPath, ".git"))
	cmd := fmt.Sprintf("git -C %s gc --quiet 2>&1", shellQuote(repoPath))
	res := runner.Run(ctx, cmd, 120*time.Second, nil)
	if res.ExitCode != 0 {
		return 0, fmt.Errorf("git gc failed: %s", strings.TrimSpace(res.Output))
	}
	after := dirSizeMB(filepath.Join(repoPath, ".git"))
	freed := before - after
	if freed < 0 {
		freed = 0
	}
	return freed, nil
}

// DeleteMergedBranches removes the local merged branches. It returns the
// ones it deleted, and an error that names every one git refused to
// delete and why.
func DeleteMergedBranches(ctx context.Context, repoPath string, branches []string) ([]string, error) {
	var deleted []string
	var failed []error
	for _, b := range branches {
		if b == "main" || b == "master" || b == "develop" {
			continue
		}
		// -d, never -D: git itself refuses a branch that is not merged.
		if _, err := git(ctx, repoPath, "branch", "-d", "--", b); err != nil {
			failed = append(failed, fmt.Errorf("%s: %w", b, err))
			continue
		}
		deleted = append(deleted, b)
	}
	return deleted, errors.Join(failed...)
}

func dirSizeMB(path string) int64 {
	var total int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total / (1024 * 1024)
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
