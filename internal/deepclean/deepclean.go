// Package deepclean finds what omega ultra offers to remove beyond the
// catalogue steps: the git worktrees AI coding agents leave behind, the loose
// files their runs write next to the projects, and Docker's images,
// containers and volumes one by one. It never deletes on its own: Scan
// measures and explains, Apply removes one finding the user approved.
//
// What the run logs of a developer Mac showed is why it exists: the biggest
// item was always the agents' worktrees (13 × 1.7 GB), reported run after run
// as "uncommitted changes, left to the user" when the only uncommitted thing
// in each was a PLAN.md or a .claude-run.json the agent wrote; and a blanket
// `docker image prune -a` once took an image built on the machine that no
// registry can give back.
package deepclean

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/lcajigasm/omega/internal/runner"
)

// Category groups the findings as the user thinks of them.
type Category string

const (
	CatAgents Category = "ai-agents"
	CatGit    Category = "git"
	CatDocker Category = "docker"
)

// Kind is what a finding is, which says how Apply removes it.
type Kind string

const (
	KindWorktree        Kind = "worktree"         // a linked git worktree
	KindWorktreeMeta    Kind = "worktree-prune"   // git's record of a worktree whose folder is gone
	KindAgentScratch    Kind = "agent-scratch"    // loose files an agent's run wrote in a project folder
	KindDockerImage     Kind = "docker-image"     // an image no container uses
	KindDockerContainer Kind = "docker-container" // a stopped container
	KindDockerVolume    Kind = "docker-volume"    // a volume no container uses
)

// Verdict is how sure omega is that nothing is lost.
type Verdict string

const (
	// Safe: nothing the user made is lost; what goes can be made again.
	Safe Verdict = "safe"
	// Review: something would be lost or has to be rescued first; the
	// details say what. Apply keeps a copy where it can.
	Review Verdict = "review"
	// Keep: listed so the user knows about it, never removed by omega
	// (in use, locked, the folder omega runs from).
	Keep Verdict = "keep"
)

// Finding is one thing omega ultra can offer to remove.
type Finding struct {
	ID       string   `json:"id"`
	Category Category `json:"category"`
	Kind     Kind     `json:"kind"`
	Title    string   `json:"title"`
	Path     string   `json:"path,omitempty"`
	// Repo is the main checkout a worktree belongs to.
	Repo string `json:"repo,omitempty"`
	// Ref is the branch, image or container a finding names.
	Ref string `json:"ref,omitempty"`
	// Owner is who made it, when it can be told: Claude Code, Codex, Cursor.
	Owner   string  `json:"owner,omitempty"`
	Size    int64   `json:"size"`
	Verdict Verdict `json:"verdict"`
	// Reasons say why the verdict is what it is, in plain words.
	Reasons []string `json:"reasons,omitempty"`
	// Rescue lists what Apply copies away before removing (files changed
	// or added by hand, commits on no branch).
	Rescue []string `json:"rescue,omitempty"`
	// LastUsed is the last commit or change, when known.
	LastUsed time.Time `json:"last_used,omitempty"`
	// Action is what Apply does, as the user reads it on the approval card.
	Action string `json:"action"`
}

// Report is what Scan found.
type Report struct {
	Findings []Finding `json:"findings"`
	// Errors are the places that could not be looked at.
	Errors []string `json:"errors,omitempty"`
}

// Totals adds up the findings by verdict.
func (r Report) Totals() (safe, review, keep int64) {
	for _, f := range r.Findings {
		switch f.Verdict {
		case Safe:
			safe += f.Size
		case Review:
			review += f.Size
		default:
			keep += f.Size
		}
	}
	return
}

// Find returns a finding by id.
func (r Report) Find(id string) (Finding, bool) {
	for _, f := range r.Findings {
		if f.ID == id {
			return f, true
		}
	}
	return Finding{}, false
}

// Options says where to look.
type Options struct {
	// Roots are the folders with the user's projects (gitops.DefaultRoots).
	Roots []string
	// Repos are the main checkouts under Roots (gitops.FindRepos).
	Repos []string
	// Docker turns on the Docker part (it is slow when the daemon is cold).
	Docker bool
	// Now is the time ages are measured from.
	Now time.Time
}

// Seams the tests replace.
var (
	// command runs a program, not through a shell, and returns its output.
	command = func(ctx context.Context, dir, name string, args ...string) (string, error) {
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0")
		if name == "git" {
			cmd.Env = append(cmd.Env, runner.GitEnv()...)
		}
		out, err := cmd.Output()
		if err != nil {
			var ee *exec.ExitError
			if errors.As(err, &ee) && len(ee.Stderr) > 0 {
				return string(out), fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), strings.TrimSpace(string(ee.Stderr)))
			}
			return string(out), fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
		}
		return string(out), nil
	}
	has = runner.Has
	// busyDirs are the working folders of running processes: a worktree
	// one of them is in is in use (an agent session, a dev server).
	busyDirs = processDirs
	// selfDir is where omega was started.
	selfDir = func() string { d, _ := os.Getwd(); return d }
	// sizeOf measures a folder.
	sizeOf = dirSize
)

// Scan looks for everything omega ultra offers to remove. It never
// changes anything.
func Scan(ctx context.Context, opts Options) Report {
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	var r Report
	busy := busyDirs(ctx)
	for _, repo := range opts.Repos {
		if ctx.Err() != nil {
			break
		}
		fs, err := scanWorktrees(ctx, repo, busy, opts.Now)
		if err != nil {
			r.Errors = append(r.Errors, err.Error())
		}
		r.Findings = append(r.Findings, fs...)
	}
	r.Findings = append(r.Findings, scanScratch(opts.Roots, opts.Repos, opts.Now)...)
	if opts.Docker && has("docker") && ctx.Err() == nil {
		fs, err := scanDocker(ctx, opts.Now)
		if err != nil {
			r.Errors = append(r.Errors, err.Error())
		}
		r.Findings = append(r.Findings, fs...)
	}
	sort.SliceStable(r.Findings, func(i, j int) bool {
		a, b := r.Findings[i], r.Findings[j]
		if a.Category != b.Category {
			return a.Category < b.Category
		}
		return a.Size > b.Size
	})
	return r
}

// id is a short, stable name for a finding: the same thing gets the same
// id in the next scan, so the model can pass it back to Apply.
func id(kind Kind, key string) string {
	sum := sha1.Sum([]byte(string(kind) + "\x00" + key))
	return string(kind) + "-" + hex.EncodeToString(sum[:])[:8]
}

// agentScratch are the files an AI coding agent's run writes next to the
// code and never commits: run records, plans, PR bodies, review diffs,
// transcripts piped to a file. Found untracked in a worktree, they are not
// the user's work.
var agentScratch = regexp.MustCompile(`(?i)^(` +
	`\.(claude|codex|cursor|gemini)-run[\w.-]*\.json|\.(claude|codex|cursor|gemini)-[\w.-]*\.(txt|log|diff)|` +
	`plan\.md|\.plan\.md|\.?(pr|issue)-body[\w.-]*\.md|\.pr-title[\w.-]*\.txt|\.commit-msg[\w.-]*(\.txt)?|` +
	`\.?review\d*(\.[\w-]+)?\.(diff|md)|\.spec-[\w.-]*\.md|\.task-[\w.-]*\.md|` +
	`[\w.-]*\.codex\d*\.txt|[\w.-]*\.claude\d*\.txt|\.aider\.(chat|input)\.history\.md|\.aider\.tags\.cache\.v\d+/?` +
	`)$`)

// IsAgentScratch reports whether a file name is one an agent's run leaves.
func IsAgentScratch(name string) bool {
	return agentScratch.MatchString(strings.TrimSuffix(filepath.Base(strings.TrimSuffix(name, "/")), "/"))
}

// owner guesses which agent made a worktree from where it lives and what
// it left.
func owner(path string, scratch []string) string {
	p := filepath.ToSlash(path)
	switch {
	case strings.Contains(p, "/.claude/worktrees/"):
		return "Claude Code"
	case strings.Contains(p, "/.codex/") || strings.Contains(p, "/codex-worktrees/"):
		return "Codex"
	case strings.Contains(p, "/.cursor/") || strings.Contains(p, "/cursor-worktrees/"):
		return "Cursor"
	}
	for _, s := range scratch {
		l := strings.ToLower(s)
		switch {
		case strings.Contains(l, "claude"):
			return "Claude Code"
		case strings.Contains(l, "codex"):
			return "Codex"
		case strings.Contains(l, "cursor"):
			return "Cursor"
		}
	}
	return ""
}

// within reports whether p is root or inside it.
func within(p, root string) bool {
	if p == "" || root == "" {
		return false
	}
	// git and lsof give real paths; a folder may be reached through a
	// link (/var is /private/var on a Mac).
	p, root = realPath(p), realPath(root)
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func dirSize(path string) int64 {
	var total int64
	_ = filepath.WalkDir(path, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type().IsRegular() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

// processDirs lists the working folders of the running processes, from
// lsof. Empty where lsof is missing: then nothing is known to be in use,
// and omega still never removes the folder it runs from.
func processDirs(ctx context.Context) []string {
	if !has("lsof") {
		return nil
	}
	out, _ := command(ctx, "", "lsof", "-n", "-P", "-w", "-d", "cwd", "-Fn")
	var dirs []string
	seen := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		if d, ok := strings.CutPrefix(line, "n"); ok && strings.HasPrefix(d, "/") && d != "/" && !seen[d] {
			seen[d] = true
			dirs = append(dirs, d)
		}
	}
	return dirs
}

// realPath resolves the links in p, or returns it as it is.
func realPath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}
