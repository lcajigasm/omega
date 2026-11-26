package deepclean

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// recentWork is how recent a worktree's last commit may be before it is
// offered only for review: it may still be the one somebody works in.
const recentWork = 48 * time.Hour

// worktree is one entry of `git worktree list --porcelain`.
type worktree struct {
	path     string
	head     string
	branch   string // without refs/heads/; "" when detached
	locked   bool
	prunable bool
}

func parseWorktrees(out string) []worktree {
	var list []worktree
	var cur *worktree
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(line, "worktree "):
			list = append(list, worktree{path: strings.TrimPrefix(line, "worktree ")})
			cur = &list[len(list)-1]
		case cur == nil:
		case strings.HasPrefix(line, "HEAD "):
			cur.head = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			cur.branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		case line == "locked" || strings.HasPrefix(line, "locked "):
			cur.locked = true
		case line == "prunable" || strings.HasPrefix(line, "prunable "):
			cur.prunable = true
		}
	}
	return list
}

// changes is what `git status` says of a worktree, split by whose it is.
type changes struct {
	tracked []string // modified, added, deleted or renamed tracked files
	scratch []string // untracked files an agent's run wrote
	others  []string // other untracked files and folders: possibly the user's
}

func parseStatus(out string) changes {
	var c changes
	for _, entry := range strings.Split(out, "\x00") {
		if len(entry) < 4 {
			continue
		}
		code, path := entry[:2], entry[3:]
		switch code {
		case "??":
			if IsAgentScratch(path) {
				c.scratch = append(c.scratch, path)
			} else {
				c.others = append(c.others, path)
			}
		case "!!":
		default:
			c.tracked = append(c.tracked, path)
		}
	}
	return c
}

// scanWorktrees looks at the linked worktrees of one repository.
func scanWorktrees(ctx context.Context, repo string, busy []string, now time.Time) ([]Finding, error) {
	out, err := command(ctx, repo, "git", "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("%s: %v", repo, err)
	}
	list := parseWorktrees(out)
	if len(list) <= 1 {
		return nil, nil
	}
	def := defaultBranch(ctx, repo)
	self := selfDir()
	var found []Finding
	var prunable []string
	// The first entry is the main checkout: never offered.
	for _, wt := range list[1:] {
		if wt.prunable {
			prunable = append(prunable, wt.path)
			continue
		}
		found = append(found, inspectWorktree(ctx, repo, def, wt, busy, self, now))
	}
	if len(prunable) > 0 {
		found = append(found, Finding{
			ID: id(KindWorktreeMeta, repo), Category: CatGit, Kind: KindWorktreeMeta,
			Title: fmt.Sprintf("%d worktree records whose folder is gone", len(prunable)), Path: repo, Repo: repo,
			Verdict: Safe, Reasons: []string{"git still lists them, but their folders were deleted: " + strings.Join(prunable, ", ")},
			Action: "git worktree prune in " + repo,
		})
	}
	return found, nil
}

func inspectWorktree(ctx context.Context, repo, def string, wt worktree, busy []string, self string, now time.Time) Finding {
	name := filepath.Base(wt.path)
	f := Finding{
		ID: id(KindWorktree, wt.path), Category: CatAgents, Kind: KindWorktree,
		Path: wt.path, Repo: repo, Ref: wt.branch, Size: sizeOf(wt.path),
		Action: "git worktree remove --force " + wt.path + " (the branch and its commits stay in " + filepath.Base(repo) + ")",
	}
	if wt.branch == "" {
		f.Ref = "detached at " + short(wt.head)
	}
	f.Title = filepath.Base(repo) + " · " + name + " · " + f.Ref

	keep := func(why string) Finding {
		f.Verdict = Keep
		f.Reasons = append(f.Reasons, why)
		f.Action = "nothing: " + why
		return f
	}
	if wt.locked {
		return keep("the worktree is locked (git worktree lock)")
	}
	if within(self, wt.path) {
		return keep("omega is running from inside it")
	}
	for _, d := range busy {
		if within(d, wt.path) {
			return keep("a running process works in it (" + d + "): an agent session or a dev server")
		}
	}

	st, err := command(ctx, wt.path, "git", "status", "--porcelain=v1", "-z", "--untracked-files=normal")
	if err != nil {
		return keep("git could not read its state: " + err.Error())
	}
	ch := parseStatus(st)
	f.Owner = owner(wt.path, ch.scratch)
	if f.Owner == "" && f.Category == CatAgents {
		f.Category = CatGit
	}

	if t, err := command(ctx, wt.path, "git", "log", "-1", "--format=%ct"); err == nil {
		if sec, err := strconv.ParseInt(strings.TrimSpace(t), 10, 64); err == nil {
			f.LastUsed = time.Unix(sec, 0)
		}
	}
	// The last commit, not the index: git status itself rewrites the index.
	recent := false
	if !f.LastUsed.IsZero() {
		f.Reasons = append(f.Reasons, "last commit "+ago(now.Sub(f.LastUsed)))
		recent = now.Sub(f.LastUsed) < recentWork
	}

	// Where its commits are: a branch keeps them after the worktree goes.
	reachable := wt.branch != ""
	if !reachable {
		if out, err := command(ctx, repo, "git", "for-each-ref", "--contains", wt.head, "--count=1", "--format=%(refname)", "refs/heads", "refs/remotes"); err == nil && strings.TrimSpace(out) != "" {
			reachable = true
		}
	}
	switch {
	case !reachable:
		f.Rescue = append(f.Rescue, "commit "+short(wt.head)+" is on no branch: saved as branch omega/rescued/"+name)
	case wt.branch != "":
		if up, err := command(ctx, wt.path, "git", "rev-parse", "--abbrev-ref", "@{u}"); err == nil {
			ahead, _ := command(ctx, wt.path, "git", "rev-list", "--count", "@{u}..HEAD")
			if n := strings.TrimSpace(ahead); n != "" && n != "0" {
				f.Reasons = append(f.Reasons, n+" commits not pushed to "+strings.TrimSpace(up)+" (they stay on branch "+wt.branch+")")
			} else {
				f.Reasons = append(f.Reasons, "pushed: "+wt.branch+" matches "+strings.TrimSpace(up))
			}
		} else {
			f.Reasons = append(f.Reasons, "branch "+wt.branch+" has no upstream (it stays in the repository)")
		}
	}
	if def != "" {
		if _, err := command(ctx, repo, "git", "merge-base", "--is-ancestor", wt.head, def); err == nil {
			f.Reasons = append(f.Reasons, "already merged into "+def)
		}
	}

	if len(ch.scratch) > 0 {
		f.Reasons = append(f.Reasons, fmt.Sprintf("%d agent files only (%s)", len(ch.scratch), list(ch.scratch, 4)))
	}
	if len(ch.tracked) > 0 {
		f.Rescue = append(f.Rescue, fmt.Sprintf("%d changed tracked files (%s), saved as a patch", len(ch.tracked), list(ch.tracked, 4)))
	}
	if len(ch.others) > 0 {
		f.Rescue = append(f.Rescue, fmt.Sprintf("%d untracked files or folders (%s), copied", len(ch.others), list(ch.others, 4)))
	}
	if len(f.Rescue) == 0 && recent {
		f.Verdict = Review
		f.Reasons = append(f.Reasons, "nothing in it would be lost, but it was worked on in the last two days: it may still be in use")
	} else if len(f.Rescue) == 0 {
		f.Verdict = Safe
		f.Reasons = append(f.Reasons, "nothing in it is lost: no changes of anyone's but the agent's own files")
	} else {
		f.Verdict = Review
		f.Action += "; first " + strings.Join(f.Rescue, "; ") + " under " + RescueRoot()
	}
	return f
}

// defaultBranch is the branch work is merged into, as gitops finds it.
func defaultBranch(ctx context.Context, repo string) string {
	if out, err := command(ctx, repo, "git", "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err == nil && strings.TrimSpace(out) != "" {
		return strings.TrimSpace(out)
	}
	for _, ref := range []string{"origin/main", "origin/master", "main", "master"} {
		if _, err := command(ctx, repo, "git", "rev-parse", "--verify", "--quiet", ref+"^{commit}"); err == nil {
			return ref
		}
	}
	return ""
}

// gitDirOf is the private git folder of a linked worktree (its .git file
// points to it).
func gitDirOf(wt string) string {
	data, err := os.ReadFile(filepath.Join(wt, ".git"))
	if err != nil {
		return filepath.Join(wt, ".git")
	}
	d := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(data)), "gitdir:"))
	if !filepath.IsAbs(d) {
		d = filepath.Join(wt, d)
	}
	return d
}

func short(sha string) string {
	if len(sha) > 10 {
		return sha[:10]
	}
	return sha
}

func list(items []string, max int) string {
	if len(items) <= max {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:max], ", ") + fmt.Sprintf(" and %d more", len(items)-max)
}

func ago(d time.Duration) string {
	switch {
	case d < time.Hour:
		return "less than an hour ago"
	case d < 48*time.Hour:
		return fmt.Sprintf("%d hours ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	}
}
