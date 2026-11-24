package agent

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/runner"
	"github.com/lcajigasm/omega/internal/safety"
)

func machineFactsSpec() llm.ToolSpec {
	return llm.ToolSpec{Name: "machine_facts", Description: "Facts that runs used to work out with improvised commands, measured " +
		"the same way every time. worktrees: the git worktrees of the repositories under the usual project folders, each with " +
		"uncommitted changes, commits not pushed (only when it has an upstream), whether git can prune it, its size and the command " +
		"to remove the clean ones. caches: where each installed tool really keeps its cache (go env, uv cache dir, npm config get " +
		"cache, brew --cache…) and its size. configs: which of the usual configuration files exist. path: PATH entries of the login " +
		"shell that do not exist, with the ones built from an empty variable. Read-only; results are kept for the session.",
		Properties: map[string]any{"section": map[string]any{"type": "string", "enum": []string{"worktrees", "caches", "configs", "path", "all"}}}}
}

func toolMachineFacts(ctx context.Context, a *Agent, in map[string]any) (string, error) {
	section := getString(in, "section")
	if section == "" {
		section = "all"
	}
	key := "machine_facts " + section
	if out, ok := a.cached("run_step:"+key, key, safety.Safe); ok {
		return out, nil
	}
	a.emit(Event{Kind: EvToolStart, Tool: "machine_facts", Command: key, Risk: safety.Safe})
	start := time.Now()
	var b strings.Builder
	home, _ := os.UserHomeDir()
	all := section == "all"
	if all || section == "worktrees" {
		b.WriteString(worktreeFacts(ctx, home))
	}
	if all || section == "caches" {
		b.WriteString(cacheFacts(ctx, home))
	}
	if all || section == "configs" {
		b.WriteString(configFacts(home))
	}
	if all || section == "path" {
		b.WriteString(pathFacts(ctx))
	}
	out := b.String()
	a.quiet("machine_facts", key, start, out, nil)
	a.remember("run_step:"+key, key, out)
	return out, nil
}

// devRoots are the folders people keep projects in.
var devRoots = []string{"Dev", "Developer", "dev", "Projects", "projects", "Code", "code", "src", "work", "Work", "Sites", "repos", "git", "Documents/GitHub"}

// uniqueDirs keeps one path per folder: on a disk that ignores case, ~/Dev
// and ~/dev are the same folder, and one run counted everything twice.
func uniqueDirs(paths []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range paths {
		fi, err := os.Stat(p)
		if err != nil || !fi.IsDir() {
			continue
		}
		real, err := filepath.EvalSymlinks(p)
		if err != nil {
			real = p
		}
		id := fileID(fi, real)
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, p)
	}
	return out
}

// gitOut runs git without letting a repository's own configuration run a
// program: fsmonitor is switched off, and a repository that names a filter
// or a diff driver is not looked into.
func gitOut(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if runner.UntrustedGitConfig(dir) {
		return "", fmt.Errorf("not inspected: its .git/config names a program git would run")
	}
	full := append([]string{"-c", "core.fsmonitor=false", "-C", dir}, args...)
	out, err := exec.CommandContext(ctx, "git", full...).Output()
	return strings.TrimSpace(string(out)), err
}

// findRepos lists the repositories (folders with a .git folder, not a
// worktree's .git file) down to three levels under the project roots.
func findRepos(home string) []string {
	var roots []string
	for _, r := range devRoots {
		roots = append(roots, filepath.Join(home, r))
	}
	var repos []string
	for _, root := range uniqueDirs(roots) {
		base := strings.Count(root, string(os.PathSeparator))
		filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			name := d.Name()
			if p != root && (strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "target") {
				return filepath.SkipDir
			}
			if fi, err := os.Stat(filepath.Join(p, ".git")); err == nil && fi.IsDir() {
				repos = append(repos, p)
				return filepath.SkipDir
			}
			if strings.Count(p, string(os.PathSeparator))-base >= 3 {
				return filepath.SkipDir
			}
			return nil
		})
	}
	return repos
}

type worktree struct {
	path, branch       string
	dirty              int
	ahead              string
	prunable, detached bool
	sizeKB             int64
}

func worktreeFacts(ctx context.Context, home string) string {
	if !runner.Has("git") {
		return "## worktrees\ngit is not installed.\n\n"
	}
	var b strings.Builder
	b.WriteString("## worktrees\n")
	found := 0
	deadline := time.Now().Add(45 * time.Second) // sizes are measured while time allows
	for _, repo := range findRepos(home) {
		list, err := gitOut(ctx, repo, "worktree", "list", "--porcelain")
		if err != nil {
			continue
		}
		var wts []worktree
		var cur *worktree
		for _, l := range strings.Split(list, "\n") {
			switch {
			case strings.HasPrefix(l, "worktree "):
				wts = append(wts, worktree{path: strings.TrimPrefix(l, "worktree ")})
				cur = &wts[len(wts)-1]
			case cur == nil:
			case strings.HasPrefix(l, "branch "):
				cur.branch = strings.TrimPrefix(strings.TrimPrefix(l, "branch "), "refs/heads/")
			case l == "detached":
				cur.detached = true
			case strings.HasPrefix(l, "prunable"):
				cur.prunable = true
			}
		}
		if len(wts) < 2 {
			continue
		}
		var wg sync.WaitGroup
		sem := make(chan struct{}, 6)
		for i := range wts[1:] {
			w := &wts[i+1]
			if w.prunable {
				continue
			}
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				if st, err := gitOut(ctx, w.path, "status", "--porcelain"); err == nil && st != "" {
					w.dirty = len(strings.Split(st, "\n"))
				}
				if _, err := gitOut(ctx, w.path, "rev-parse", "--abbrev-ref", "@{u}"); err != nil {
					w.ahead = "no upstream"
				} else if n, err := gitOut(ctx, w.path, "rev-list", "--count", "@{u}..HEAD"); err == nil {
					w.ahead = n
				}
				if time.Now().Before(deadline) {
					w.sizeKB = duKB(ctx, w.path)
				}
			}()
		}
		wg.Wait()
		found += len(wts) - 1
		fmt.Fprintf(&b, "repository %s: %d worktrees besides the main one\n", repo, len(wts)-1)
		var clean []string
		for _, w := range wts[1:] {
			state := []string{}
			switch {
			case w.prunable:
				state = append(state, "prunable (its folder is gone: git worktree prune)")
			case w.dirty > 0:
				state = append(state, fmt.Sprintf("%d uncommitted changes", w.dirty))
			default:
				state = append(state, "no uncommitted changes")
			}
			if w.ahead == "no upstream" {
				state = append(state, "no upstream (unpushed commits unknown)")
			} else if w.ahead != "" && w.ahead != "0" {
				state = append(state, w.ahead+" commits not pushed")
			}
			if w.detached {
				state = append(state, "detached HEAD")
			}
			size := "size not measured"
			if w.sizeKB > 0 {
				size = fmt.Sprintf("%.1f GB", float64(w.sizeKB)*1024/1e9)
			}
			fmt.Fprintf(&b, "- %s (%s): %s; %s\n", w.path, orNone(w.branch), strings.Join(state, ", "), size)
			if !w.prunable && w.dirty == 0 && (w.ahead == "0") {
				clean = append(clean, w.path)
			}
		}
		if len(clean) > 0 {
			b.WriteString("  clean and pushed (safe to remove, one option per worktree on a card):\n")
			for _, p := range clean {
				fmt.Fprintf(&b, "    git -C %s worktree remove %s\n", safety.ShellQuote(repo), safety.ShellQuote(p))
			}
		}
	}
	if found == 0 {
		b.WriteString("No repository under the project folders has extra worktrees.\n")
	}
	b.WriteString("\n")
	return b.String()
}

func orNone(s string) string {
	if s == "" {
		return "no branch"
	}
	return s
}

// duKB is the size of a folder in KB, or 0 when it takes too long.
func duKB(ctx context.Context, path string) int64 {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "du", "-sk", path).Output()
	if err != nil && len(out) == 0 {
		return 0
	}
	n, _ := strconv.ParseInt(strings.Fields(string(out) + " 0")[0], 10, 64)
	return n
}

// cacheQueries ask each tool where its cache is, instead of guessing paths
// that do not exist on this machine.
var cacheQueries = []struct {
	tool, bin string
	args      []string
}{
	{"Go build cache", "go", []string{"env", "GOCACHE"}},
	{"Go module cache", "go", []string{"env", "GOMODCACHE"}},
	{"uv", "uv", []string{"cache", "dir"}},
	{"npm", "npm", []string{"config", "get", "cache"}},
	{"pnpm store", "pnpm", []string{"store", "path"}},
	{"Homebrew downloads", "brew", []string{"--cache"}},
	{"pip", "pip3", []string{"cache", "dir"}},
	{"Yarn", "yarn", []string{"cache", "dir"}},
}

func cacheFacts(ctx context.Context, home string) string {
	type row struct {
		name, path string
		kb         int64
	}
	var rows []row
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, q := range cacheQueries {
		if !runner.Has(q.bin) {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			out, err := exec.CommandContext(c, q.bin, q.args...).Output()
			p := strings.TrimSpace(strings.Split(string(out), "\n")[0])
			if err != nil || p == "" || p == "undefined" {
				return
			}
			r := row{name: q.tool, path: p}
			if _, err := os.Stat(p); err == nil {
				r.kb = duKB(ctx, p)
			}
			mu.Lock()
			rows = append(rows, r)
			mu.Unlock()
		}()
	}
	for _, fixed := range []struct{ name, rel string }{{"Cargo registry", ".cargo/registry"}, {"Bun", ".bun/install/cache"}, {"Gradle", ".gradle/caches"}} {
		p := filepath.Join(home, fixed.rel)
		if _, err := os.Stat(p); err == nil {
			wg.Add(1)
			go func() {
				defer wg.Done()
				kb := duKB(ctx, p)
				mu.Lock()
				rows = append(rows, row{fixed.name, p, kb})
				mu.Unlock()
			}()
		}
	}
	wg.Wait()
	sort.Slice(rows, func(i, j int) bool { return rows[i].kb > rows[j].kb })
	var b strings.Builder
	b.WriteString("## caches (where each tool says it keeps them)\n")
	for _, r := range rows {
		size := "does not exist"
		if _, err := os.Stat(r.path); err == nil {
			size = fmt.Sprintf("%.2f GB", float64(r.kb)*1024/1e9)
		}
		fmt.Fprintf(&b, "- %s: %s — %s\n", r.name, r.path, size)
	}
	if len(rows) == 0 {
		b.WriteString("No known cache.\n")
	}
	b.WriteString("\n")
	return b.String()
}

// knownConfigs are configuration files runs looked for by hand.
var knownConfigs = []string{
	".config/topgrade.toml", ".config/topgrade.d", ".config/mole", ".gitconfig", ".config/git/config", ".npmrc",
	".config/uv/uv.toml", ".config/pip/pip.conf", "Library/Application Support/pip/pip.conf", ".gemrc", ".bunfig.toml",
	".config/mise/config.toml", ".tool-versions", ".ssh/config", ".docker/daemon.json", ".docker/config.json",
	".homebrew/brew.env", ".config/homebrew/brew.env", ".zshrc", ".zprofile", ".zshenv", ".bashrc", ".bash_profile",
	".config/starship.toml", ".oh-my-zsh", ".config/atuin/config.toml", ".config/gh/config.yml", ".cargo/config.toml",
	".config/go/env", "Library/Application Support/go/env",
}

func configFacts(home string) string {
	var have, missing []string
	for _, rel := range knownConfigs {
		if _, err := os.Stat(filepath.Join(home, rel)); err == nil {
			have = append(have, "~/"+rel)
		} else {
			missing = append(missing, "~/"+rel)
		}
	}
	return "## configs\nexist: " + strings.Join(have, ", ") + "\ndo not exist (do not read them): " + strings.Join(missing, ", ") + "\n\n"
}

// standardRoots are where PATH entries normally start; one that starts
// anywhere else and does not exist was most likely built from an empty
// variable ($PNPM_HOME/bin with PNPM_HOME unset gives /bin… or /pkg/…).
var standardRoots = map[string]bool{"usr": true, "opt": true, "bin": true, "sbin": true, "Library": true, "Applications": true,
	"System": true, "Users": true, "home": true, "nix": true, "snap": true, "var": true, "private": true, "etc": true, "tmp": true,
	"Volumes": true, "Developer": true, "root": true, "run": true, "mnt": true, "Applications (Parallels)": true}

func pathFacts(ctx context.Context) string {
	c, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	res := runner.Run(c, `printf '%s' "$PATH"`, 20*time.Second, nil)
	var b strings.Builder
	b.WriteString("## path (login shell)\n")
	seen := map[string]int{}
	var bad []string
	sc := bufio.NewScanner(strings.NewReader(strings.ReplaceAll(strings.TrimSpace(res.Output), ":", "\n")))
	for sc.Scan() {
		p := sc.Text()
		if p == "" {
			bad = append(bad, "(empty entry: the current folder)")
			continue
		}
		seen[p]++
		if seen[p] == 2 {
			bad = append(bad, p+": listed more than once")
		}
		if _, err := os.Stat(p); err != nil && seen[p] == 1 {
			first := strings.SplitN(strings.TrimPrefix(p, "/"), "/", 2)[0]
			why := "does not exist"
			if strings.HasPrefix(p, "/") && !standardRoots[first] {
				why += "; probably built from an empty variable (look for $SOMETHING" + p + " in the shell's startup files)"
			}
			bad = append(bad, p+": "+why)
		}
	}
	if len(bad) == 0 {
		b.WriteString("Every entry exists and appears once.\n")
	}
	for _, l := range bad {
		b.WriteString("- " + l + "\n")
	}
	b.WriteString("\n")
	return b.String()
}
