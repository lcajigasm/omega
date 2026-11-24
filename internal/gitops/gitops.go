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

// ErrUntrusted is a repository whose own configuration names a program git
// would run while omega only looks (a filter, a diff driver): it is left
// out instead.
var ErrUntrusted = errors.New("its .git/config names a program git would run (a filter or diff driver); not inspected")

// git runs git in a repository and returns what it printed. It is run as
// it is, not through the user's shell: starting a login shell for every
// command took longer than git itself. Tests replace it.
var git = func(ctx context.Context, repo string, args ...string) (string, error) {
	if runner.UntrustedGitConfig(repo) {
		return "", fmt.Errorf("git %s: %w", args[0], ErrUntrusted)
	}
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
	// Looking must not write, and git must never stop to ask.
	// Nor run a program a repository's own .git/config names for it
	// (core.fsmonitor): every repository under the project folders is
	// looked at, also one that was only unpacked there.
	cmd.Env = append(append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0"), runner.GitEnv()...)
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
	Path           string `json:"path"`
	Name           string `json:"name"`
	Branch         string `json:"branch"`
	HasUncommitted bool   `json:"has_uncommitted"`
	UnpushedCount  int    `json:"unpushed_count"`
	// DefaultBranch is what merged branches are compared with: origin/HEAD,
	// else main or master, else HEAD. OnDefault says whether HEAD is on it.
	DefaultBranch  string   `json:"default_branch,omitempty"`
	OnDefault      bool     `json:"on_default"`
	MergedBranches []string `json:"merged_branches,omitempty"`
	GitSize        int64    `json:"git_size"` // bytes
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
	st.GitSize = dirSize(gitDir)

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

	// Merged branches: merged into the default branch, not into whatever
	// HEAD is on. With HEAD on a feature branch, "merged into HEAD" took
	// develop, release and backup branches for leftovers.
	def, err := defaultBranch(ctx, repoPath)
	if errors.Is(err, ErrGitTimeout) {
		return st, fmt.Errorf("could not check: %w", err)
	}
	st.DefaultBranch = def
	st.OnDefault = def == "" || localName(def) == st.Branch
	merged, err := mergedBranches(ctx, repoPath, def, st.Branch)
	if errors.Is(err, ErrGitTimeout) {
		return st, fmt.Errorf("could not check: %w", err)
	}
	st.MergedBranches = merged

	st.LargeFiles = largeFiles(repoPath)
	return st, nil
}

// defaultBranch is the branch a repository's work is merged into:
// origin/HEAD when the clone knows it, else a local or remote main or
// master. "" when there is none (merged branches are then measured
// against HEAD).
func defaultBranch(ctx context.Context, repo string) (string, error) {
	out, err := git(ctx, repo, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
	if errors.Is(err, ErrGitTimeout) {
		return "", err
	}
	if b := strings.TrimSpace(out); err == nil && b != "" {
		return b, nil
	}
	for _, ref := range []string{"main", "master", "origin/main", "origin/master"} {
		_, err := git(ctx, repo, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
		if errors.Is(err, ErrGitTimeout) {
			return "", err
		}
		if err == nil {
			return ref, nil
		}
	}
	return "", nil
}

// localName is a branch name without its remote: origin/main is main.
func localName(ref string) string {
	if i := strings.Index(ref, "/"); i >= 0 && strings.HasPrefix(ref, "origin/") {
		return ref[i+1:]
	}
	return ref
}

// Protected reports whether a branch is one omega never offers to delete
// whatever git says of it: the main lines of work (main, master, develop…,
// release/…) and the backups somebody made on purpose.
func Protected(branch string) bool {
	switch {
	case branch == "main", branch == "master", branch == "trunk",
		strings.HasPrefix(branch, "develop"), strings.HasPrefix(branch, "release"),
		strings.HasPrefix(branch, "backup"):
		return true
	}
	return false
}

// keptBranches are the branches of a repository that must stay whatever
// is merged: the default one, the one HEAD is on, those checked out in any
// worktree, and the upstream of each of those (a worktree on feature
// tracking origin/develop2 keeps develop2).
func keptBranches(ctx context.Context, repo, def, current string) (map[string]bool, error) {
	keep := map[string]bool{}
	if def != "" {
		keep[localName(def)] = true
	}
	if current != "" && current != "HEAD" {
		keep[current] = true
	}
	out, err := git(ctx, repo, "worktree", "list", "--porcelain")
	if errors.Is(err, ErrGitTimeout) {
		return nil, err
	}
	for _, line := range strings.Split(out, "\n") {
		if b, ok := strings.CutPrefix(strings.TrimSpace(line), "branch refs/heads/"); ok {
			keep[b] = true
		}
	}
	out, err = git(ctx, repo, "for-each-ref", "--format=%(refname:short) %(upstream:short)", "refs/heads")
	if errors.Is(err, ErrGitTimeout) {
		return nil, err
	}
	for _, line := range strings.Split(out, "\n") {
		branch, upstream, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok || upstream == "" || !keep[branch] {
			continue
		}
		// The upstream as a local branch name: origin/develop2 is develop2.
		if _, name, ok := strings.Cut(upstream, "/"); ok {
			keep[name] = true
		} else {
			keep[upstream] = true
		}
	}
	return keep, nil
}

// mergedBranches lists the local branches merged into def (HEAD when def
// is ""), without the protected and kept ones.
func mergedBranches(ctx context.Context, repo, def, current string) ([]string, error) {
	args := []string{"branch", "--format=%(refname:short)", "--merged"}
	if def != "" {
		args = append(args, def)
	}
	out, err := git(ctx, repo, args...)
	if err != nil {
		return nil, err
	}
	keep, err := keptBranches(ctx, repo, def, current)
	if err != nil {
		return nil, err
	}
	var merged []string
	for _, b := range strings.Split(out, "\n") {
		b = strings.TrimSpace(b)
		// "(HEAD detached at …)" is not a branch.
		if b == "" || strings.HasPrefix(b, "(") || keep[b] || Protected(b) {
			continue
		}
		merged = append(merged, b)
	}
	return merged, nil
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

// OptimizeRepo runs git gc and repacks loose objects to reclaim space. It
// returns the bytes freed.
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
	before := dirSize(filepath.Join(repoPath, ".git"))
	cmd := fmt.Sprintf("git -C %s gc --quiet 2>&1", shellQuote(repoPath))
	res := runner.Run(ctx, cmd, 120*time.Second, nil)
	if res.ExitCode != 0 {
		return 0, fmt.Errorf("git gc failed: %s", strings.TrimSpace(res.Output))
	}
	after := dirSize(filepath.Join(repoPath, ".git"))
	freed := before - after
	if freed < 0 {
		freed = 0
	}
	return freed, nil
}

// DeleteMergedBranches removes the local merged branches. It returns the
// ones it deleted, and an error that names every one git refused to
// delete and why. A protected or kept branch (see CheckRepo) is never
// deleted, whatever the caller passed.
func DeleteMergedBranches(ctx context.Context, repoPath string, branches []string) ([]string, error) {
	var deleted []string
	var failed []error
	current := ""
	if out, err := git(ctx, repoPath, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		current = strings.TrimSpace(out)
	}
	def, err := defaultBranch(ctx, repoPath)
	if err != nil {
		return nil, err
	}
	keep, err := keptBranches(ctx, repoPath, def, current)
	if err != nil {
		return nil, err
	}
	for _, b := range branches {
		if Protected(b) {
			continue
		}
		if keep[b] {
			failed = append(failed, fmt.Errorf("%s: checked out or the upstream of a worktree; kept", b))
			continue
		}
		// -d: git refuses a branch that is not merged into its upstream
		// or HEAD. One merged into the default branch while HEAD is
		// elsewhere is checked here, and only then deleted with -D.
		_, err := git(ctx, repoPath, "branch", "-d", "--", b)
		if err != nil && def != "" {
			if _, aerr := git(ctx, repoPath, "merge-base", "--is-ancestor", "refs/heads/"+b, def); aerr == nil {
				_, err = git(ctx, repoPath, "branch", "-D", "--", b)
			}
		}
		if err != nil {
			failed = append(failed, fmt.Errorf("%s: %w", b, err))
			continue
		}
		deleted = append(deleted, b)
	}
	return deleted, errors.Join(failed...)
}

func dirSize(path string) int64 {
	var total int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
