package deepclean

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lcajigasm/omega/internal/cleaner"
	"github.com/lcajigasm/omega/internal/config"
)

// rescueLimit is the most Apply copies out of a worktree before removing it.
// More than that is not a leftover: the user decides by hand.
const rescueLimit = 1 << 30

// RescueRoot is where Apply keeps what it saved from a worktree before
// removing it.
func RescueRoot() string { return filepath.Join(config.StateDir(), "rescued") }

// ErrChanged is a finding that is no longer what the user approved: it was
// looked at again just before acting, and something changed.
var ErrChanged = errors.New("it changed since it was scanned")

// Seams the tests replace.
var (
	trash = cleaner.Remove
	clock = time.Now
)

// Result is what Apply did.
type Result struct {
	ID    string `json:"id"`
	Freed int64  `json:"freed"`
	// Rescued is the folder that holds what was saved first, if anything.
	Rescued string `json:"rescued,omitempty"`
	Note    string `json:"note,omitempty"`
}

// Verify looks at a finding again, as Apply does before acting, and says
// whether it is still the same: the same verdict or a safer one, and not
// kept. The second finding is the fresh look.
func Verify(ctx context.Context, f Finding) (Finding, error) {
	switch f.Kind {
	case KindWorktree:
		out, err := command(ctx, f.Repo, "git", "worktree", "list", "--porcelain")
		if err != nil {
			return f, err
		}
		for i, wt := range parseWorktrees(out) {
			if i == 0 || wt.path != f.Path {
				continue
			}
			now := inspectWorktree(ctx, f.Repo, defaultBranch(ctx, f.Repo), wt, busyDirs(ctx), selfDir(), clock(), askServer(ctx, f.Repo))
			switch {
			case now.Verdict == Keep:
				return now, fmt.Errorf("%w: %s", ErrChanged, strings.Join(now.Reasons, "; "))
			case f.Verdict == Safe && now.Verdict != Safe:
				return now, fmt.Errorf("%w: it now has work to rescue (%s); scan again and decide", ErrChanged, strings.Join(now.Rescue, "; "))
			}
			return now, nil
		}
		return f, fmt.Errorf("%w: git no longer lists the worktree", ErrChanged)
	case KindAgentScratch:
		files := scratchFiles(f.Path)
		if len(files) == 0 {
			return f, fmt.Errorf("%w: the files are gone", ErrChanged)
		}
		if f.Verdict == Safe {
			for _, name := range files {
				if fi, err := os.Stat(filepath.Join(f.Path, name)); err == nil && clock().Sub(fi.ModTime()) < scratchFresh {
					return f, fmt.Errorf("%w: %s was written in the last day", ErrChanged, name)
				}
			}
		}
	}
	return f, nil
}

// Apply removes one finding the user approved. It looks at it again
// first and refuses when it changed; a worktree with work in it has that
// work saved under RescueRoot before it goes. A finding kept is never
// removed.
func Apply(ctx context.Context, f Finding) (Result, error) {
	res := Result{ID: f.ID}
	if f.Verdict == Keep {
		return res, fmt.Errorf("%s is kept: %s", f.Title, strings.Join(f.Reasons, "; "))
	}
	fresh, err := Verify(ctx, f)
	if err != nil {
		return res, err
	}
	switch f.Kind {
	case KindWorktree:
		if len(fresh.Rescue) > 0 {
			dir, err := rescueWorktree(ctx, fresh)
			if err != nil {
				return res, fmt.Errorf("saving its work first failed, so nothing was removed: %w", err)
			}
			res.Rescued = dir
		}
		before := sizeOf(f.Path)
		if _, err := command(ctx, f.Repo, "git", "worktree", "remove", "--force", f.Path); err != nil {
			return res, err
		}
		res.Freed = before - sizeOf(f.Path)
		if f.Ref != "" && !strings.HasPrefix(f.Ref, "detached") {
			res.Note = "branch " + f.Ref + " is still in " + filepath.Base(f.Repo)
		}
	case KindWorktreeMeta:
		if _, err := command(ctx, f.Repo, "git", "worktree", "prune"); err != nil {
			return res, err
		}
	case KindAgentScratch:
		var failed []string
		for _, name := range scratchFiles(f.Path) {
			p := filepath.Join(f.Path, name)
			fi, err := os.Stat(p)
			if err != nil {
				continue
			}
			if _, err := trash(p); err != nil {
				failed = append(failed, name+": "+err.Error())
				continue
			}
			res.Freed += fi.Size()
		}
		res.Note = "moved to the Trash"
		if len(failed) > 0 {
			return res, fmt.Errorf("not moved: %s", strings.Join(failed, "; "))
		}
	case KindDockerImage:
		if _, err := command(ctx, "", "docker", "image", "rm", f.Ref); err != nil {
			return res, err
		}
		res.Freed = f.Size
	case KindDockerContainer:
		if _, err := command(ctx, "", "docker", "container", "rm", f.Ref); err != nil {
			return res, err
		}
		res.Freed = f.Size
	case KindDockerVolume:
		if _, err := command(ctx, "", "docker", "volume", "rm", f.Ref); err != nil {
			return res, err
		}
		res.Freed = f.Size
	default:
		return res, fmt.Errorf("omega does not know how to remove a %s", f.Kind)
	}
	return res, nil
}

// scratchFiles are the agent files in a folder now.
func scratchFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.Type().IsRegular() && IsAgentScratch(e.Name()) {
			out = append(out, e.Name())
		}
	}
	return out
}

// rescueWorktree saves what removing a worktree would lose: the changes
// to tracked files as a patch, the untracked files as copies, a commit on
// no branch as a branch. It returns the folder it wrote.
func rescueWorktree(ctx context.Context, f Finding) (string, error) {
	name := filepath.Base(f.Path)
	dir := filepath.Join(RescueRoot(), filepath.Base(f.Repo)+"-"+name+"-"+clock().Format("20060102-150405"))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	var readme strings.Builder
	fmt.Fprintf(&readme, "omega ultra removed the worktree %s of %s on %s.\nWhat it held that was not committed is here.\n\n",
		f.Path, f.Repo, clock().Format(time.RFC1123))

	// Changes to tracked files, staged or not, against HEAD.
	patch, err := command(ctx, f.Path, "git", "diff", "--binary", "HEAD")
	if err != nil {
		return dir, err
	}
	if strings.TrimSpace(patch) != "" {
		if err := os.WriteFile(filepath.Join(dir, "changes.patch"), []byte(patch), 0o600); err != nil {
			return dir, err
		}
		readme.WriteString("changes.patch: the changes to tracked files. In a checkout of the same commit: git apply changes.patch\n")
	}

	// Untracked files that are not an agent's own.
	out, err := command(ctx, f.Path, "git", "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return dir, err
	}
	var copied int
	var total int64
	for _, rel := range strings.Split(out, "\x00") {
		if rel == "" || IsAgentScratch(rel) {
			continue
		}
		src := filepath.Join(f.Path, rel)
		fi, err := os.Lstat(src)
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		if total += fi.Size(); total > rescueLimit {
			return dir, fmt.Errorf("its untracked files are over %d MB: decide about them by hand", rescueLimit>>20)
		}
		if err := copyFile(src, filepath.Join(dir, "files", rel), fi.Mode()); err != nil {
			return dir, err
		}
		copied++
	}
	if copied > 0 {
		fmt.Fprintf(&readme, "files/: the %d untracked files, at their paths in the worktree\n", copied)
	}

	// A commit no branch holds would be lost to git gc: give it a branch.
	for _, r := range f.Rescue {
		if strings.Contains(r, "is on no branch") {
			head, err := command(ctx, f.Path, "git", "rev-parse", "HEAD")
			if err != nil {
				return dir, err
			}
			branch := "omega/rescued/" + name
			if _, err := command(ctx, f.Repo, "git", "branch", branch, strings.TrimSpace(head)); err != nil {
				return dir, err
			}
			fmt.Fprintf(&readme, "branch %s in %s: the commit that was on no branch\n", branch, f.Repo)
		}
	}
	return dir, os.WriteFile(filepath.Join(dir, "README.txt"), []byte(readme.String()), 0o600)
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_EXCL, mode.Perm()|0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
