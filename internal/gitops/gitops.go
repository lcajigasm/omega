// Package gitops provides repository hygiene tools for scanning dev folders,
// detecting uncommitted changes, unpushed commits, merged branches, bloated .git
// directories, and large files.
package gitops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lcajigasm/omega/internal/runner"
)

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

// CheckRepo inspects a single repository for hygiene metrics.
func CheckRepo(ctx context.Context, repoPath string) (*RepoStatus, error) {
	st := &RepoStatus{
		Path: repoPath,
		Name: filepath.Base(repoPath),
	}

	// Git size
	gitDir := filepath.Join(repoPath, ".git")
	st.GitSizeMB = dirSizeMB(gitDir)

	// Current branch
	res := runner.Run(ctx, fmt.Sprintf("git -C %s rev-parse --abbrev-ref HEAD 2>/dev/null", shellQuote(repoPath)), 3*time.Second, nil)
	if res.ExitCode == 0 {
		st.Branch = strings.TrimSpace(res.Output)
	}

	// Uncommitted changes
	res = runner.Run(ctx, fmt.Sprintf("git -C %s status --porcelain 2>/dev/null", shellQuote(repoPath)), 3*time.Second, nil)
	if res.ExitCode == 0 && len(strings.TrimSpace(res.Output)) > 0 {
		st.HasUncommitted = true
	}

	// Unpushed commits
	res = runner.Run(ctx, fmt.Sprintf("git -C %s log @{u}..HEAD --oneline 2>/dev/null", shellQuote(repoPath)), 3*time.Second, nil)
	if res.ExitCode == 0 && len(strings.TrimSpace(res.Output)) > 0 {
		lines := strings.Split(strings.TrimSpace(res.Output), "\n")
		st.UnpushedCount = len(lines)
	}

	// Merged branches (excluding current branch, master, main)
	res = runner.Run(ctx, fmt.Sprintf("git -C %s branch --merged 2>/dev/null", shellQuote(repoPath)), 3*time.Second, nil)
	if res.ExitCode == 0 {
		for _, b := range strings.Split(res.Output, "\n") {
			b = strings.TrimSpace(b)
			if b == "" || strings.HasPrefix(b, "*") || b == "main" || b == "master" || b == "develop" {
				continue
			}
			st.MergedBranches = append(st.MergedBranches, b)
		}
	}

	// Find large files (>50MB) in repo
	res = runner.Run(ctx, fmt.Sprintf("find %s -maxdepth 4 -type f -size +50M ! -path '*/.git/*' 2>/dev/null", shellQuote(repoPath)), 3*time.Second, nil)
	if res.ExitCode == 0 && len(strings.TrimSpace(res.Output)) > 0 {
		for _, f := range strings.Split(strings.TrimSpace(res.Output), "\n") {
			if f != "" {
				rel, _ := filepath.Rel(repoPath, f)
				st.LargeFiles = append(st.LargeFiles, rel)
			}
		}
	}

	return st, nil
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

// DeleteMergedBranches removes the local merged branches.
func DeleteMergedBranches(ctx context.Context, repoPath string, branches []string) ([]string, error) {
	var deleted []string
	for _, b := range branches {
		if b == "main" || b == "master" || b == "develop" {
			continue
		}
		cmd := fmt.Sprintf("git -C %s branch -d %s 2>/dev/null", shellQuote(repoPath), shellQuote(b))
		res := runner.Run(ctx, cmd, 3*time.Second, nil)
		if res.ExitCode == 0 {
			deleted = append(deleted, b)
		}
	}
	return deleted, nil
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
