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

// RemoteState says how much of a worktree is already on the remote: its
// commits, checked against the server itself, and its uncommitted files,
// compared by content with what the remote branches hold.
type RemoteState struct {
	// Remote is the remote compared with ("origin").
	Remote string `json:"remote,omitempty"`
	// Server is "checked" when the server answered just now; otherwise
	// why not, and the comparison is with the last fetch.
	Server string `json:"server"`
	// Branch is the branch on the remote the worktree's branch goes to.
	Branch string `json:"branch,omitempty"`
	// OnServer: the worktree's last commit is on the server (the tip of its
	// branch, behind it, or merged into the default branch).
	OnServer bool `json:"on_server"`
	// FoundOn is the server's branch that holds the last commit.
	FoundOn string `json:"found_on,omitempty"`
	// BranchGone: the branch was on the remote and the server no longer
	// has it, which usually means it was merged and deleted.
	BranchGone bool `json:"branch_gone,omitempty"`
	// Unpushed is how many commits are on no remote branch.
	Unpushed int `json:"unpushed_commits"`
	// FilesOnRemote are uncommitted files whose exact content a remote
	// branch already holds, as "path (origin/branch)".
	FilesOnRemote []string `json:"files_on_remote,omitempty"`
	// FilesOnlyHere are uncommitted files no remote branch holds as they
	// are here: what removing the worktree would lose without a rescue.
	FilesOnlyHere []string `json:"files_only_here,omitempty"`
}

// Pushed reports whether everything the worktree holds is on the remote.
func (r RemoteState) Pushed() bool {
	return r.OnServer && r.Unpushed == 0 && len(r.FilesOnlyHere) == 0
}

// serverTimeout is how long the server may take to list its branches.
var serverTimeout = 20 * time.Second

// maxCompared is how many uncommitted files are compared by content; the
// others count as only here.
const maxCompared = 400

// server is what the remote's server said, once per repository and scan.
type server struct {
	remote string
	heads  map[string]string // branch -> commit, from git ls-remote
	err    error
	// fetched is when the remote-tracking branches were last updated.
	fetched time.Time
}

// askServer lists the branches the server has now. It reads only: no
// fetch, nothing written in the repository.
func askServer(ctx context.Context, repo string) server {
	s := server{}
	out, err := command(ctx, repo, "git", "remote")
	if err != nil {
		s.err = err
		return s
	}
	names := strings.Fields(out)
	switch {
	case len(names) == 0:
		s.err = fmt.Errorf("the repository has no remote")
		return s
	case contains(names, "origin"):
		s.remote = "origin"
	default:
		s.remote = names[0]
	}
	if fi, err := os.Stat(filepath.Join(commonDir(ctx, repo), "FETCH_HEAD")); err == nil {
		s.fetched = fi.ModTime()
	}
	tctx, cancel := context.WithTimeout(ctx, serverTimeout)
	defer cancel()
	out, err = command(tctx, repo, "git", "ls-remote", "--heads", s.remote)
	if err != nil {
		s.err = err
		return s
	}
	s.heads = map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		sha, ref, ok := strings.Cut(line, "\t")
		if ok {
			s.heads[strings.TrimPrefix(ref, "refs/heads/")] = sha
		}
	}
	return s
}

// commonDir is the repository's shared git folder.
func commonDir(ctx context.Context, repo string) string {
	out, err := command(ctx, repo, "git", "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return filepath.Join(repo, ".git")
	}
	return strings.TrimSpace(out)
}

// remoteState compares a worktree with the remote. ch is what git status
// reported; def is the default branch (origin/main).
func remoteState(ctx context.Context, wt worktree, ch changes, def string, srv server) RemoteState {
	st := RemoteState{Remote: srv.remote, Server: "checked"}
	if srv.err != nil {
		st.Server = "not reached (" + firstLineOf(srv.err.Error()) + ")"
		if !srv.fetched.IsZero() {
			st.Server += "; compared with the last fetch, " + srv.fetched.Format("2006-01-02 15:04")
		}
	}
	has := func(sha string) bool {
		_, err := command(ctx, wt.path, "git", "cat-file", "-e", sha+"^{commit}")
		return err == nil
	}
	contains := func(ancestor, tip string) bool {
		_, err := command(ctx, wt.path, "git", "merge-base", "--is-ancestor", ancestor, tip)
		return err == nil
	}

	// Commits on no remote branch, as this clone knows them.
	if srv.remote != "" {
		if out, err := command(ctx, wt.path, "git", "rev-list", "--count", "HEAD", "--not", "--remotes="+srv.remote); err == nil {
			st.Unpushed, _ = strconv.Atoi(strings.TrimSpace(out))
		}
	}

	// The branch on the remote: the upstream's, else one of the same name.
	st.Branch = wt.branch
	if wt.branch != "" {
		if up, err := command(ctx, wt.path, "git", "rev-parse", "--abbrev-ref", "@{u}"); err == nil {
			if r, b, ok := strings.Cut(strings.TrimSpace(up), "/"); ok && r == srv.remote {
				st.Branch = b
			}
		}
	}

	if srv.heads != nil {
		tip, onServer := srv.heads[st.Branch]
		switch {
		case st.Branch != "" && onServer && tip == wt.head:
			st.OnServer, st.FoundOn = true, st.Branch
		case st.Branch != "" && onServer && has(tip) && contains(wt.head, tip):
			st.OnServer, st.FoundOn = true, st.Branch // the server's branch went further
		case st.Branch != "" && onServer && !has(tip):
			// The server's branch moved since the last fetch: its new
			// commits are not here to compare with.
			st.Server += "; " + srv.remote + "/" + st.Branch + " moved since the last fetch"
			st.OnServer = st.Unpushed == 0
		}
		if !onServer && st.Branch != "" {
			if _, err := command(ctx, wt.path, "git", "rev-parse", "--verify", "--quiet", "refs/remotes/"+srv.remote+"/"+st.Branch); err == nil {
				st.BranchGone = true
			}
		}
		// Merged: the default branch on the server holds the commit.
		if !st.OnServer && def != "" {
			b := strings.TrimPrefix(def, srv.remote+"/")
			if t, ok := srv.heads[b]; ok && has(t) && contains(wt.head, t) {
				st.OnServer, st.FoundOn = true, b
			}
		}
		// Any other branch of the server that holds the commit: the one a
		// detached worktree was made from, or another after a rename.
		if !st.OnServer && srv.remote != "" {
			out, _ := command(ctx, wt.path, "git", "for-each-ref", "--contains", wt.head, "--count=50",
				"--format=%(refname:lstrip=3)", "refs/remotes/"+srv.remote)
			for _, b := range strings.Fields(out) {
				if t, ok := srv.heads[b]; ok && (t == wt.head || (has(t) && contains(wt.head, t))) {
					st.OnServer, st.FoundOn = true, b
					break
				}
			}
		}
	} else {
		// No answer from the server: the remote branches of the last
		// fetch are all there is.
		st.OnServer = srv.remote != "" && st.Unpushed == 0
		if st.OnServer {
			st.FoundOn = st.Branch
		}
	}

	st.FilesOnRemote, st.FilesOnlyHere = compareFiles(ctx, wt.path, ch, refsToCompare(ctx, wt.path, srv, st.Branch, def))
	return st
}

// refsToCompare are the remote branches whose files the uncommitted ones
// are compared with: the worktree's own branch on the remote and the
// default branch, those this clone has.
func refsToCompare(ctx context.Context, dir string, srv server, branch, def string) []string {
	var refs []string
	add := func(ref string) {
		if ref == "" || contains(refs, ref) {
			return
		}
		if _, err := command(ctx, dir, "git", "rev-parse", "--verify", "--quiet", ref+"^{commit}"); err == nil {
			refs = append(refs, ref)
		}
	}
	if srv.remote != "" && branch != "" {
		// The server's tip when this clone has it, newer than the
		// remote-tracking branch.
		if tip, ok := srv.heads[branch]; ok {
			if _, err := command(ctx, dir, "git", "cat-file", "-e", tip+"^{commit}"); err == nil {
				refs = append(refs, tip)
			}
		}
		add(srv.remote + "/" + branch)
	}
	add(def)
	return refs
}

// compareFiles compares each uncommitted file of a worktree, by content,
// with the same path in each ref. A file deleted here is on the remote
// when the ref does not have it either.
func compareFiles(ctx context.Context, dir string, ch changes, refs []string) (onRemote, onlyHere []string) {
	var paths []string
	paths = append(paths, ch.tracked...)
	if len(ch.others) > 0 {
		// Untracked folders, file by file.
		out, _ := command(ctx, dir, "git", "ls-files", "--others", "--exclude-standard", "-z")
		for _, p := range strings.Split(out, "\x00") {
			if p != "" && !IsAgentScratch(p) {
				paths = append(paths, p)
			}
		}
	}
	if len(paths) == 0 {
		return nil, nil
	}
	if len(refs) == 0 || len(paths) > maxCompared {
		return nil, paths
	}

	// What each ref holds at those paths.
	trees := make([]map[string]string, len(refs))
	for i, ref := range refs {
		trees[i] = map[string]string{}
		out, err := command(ctx, dir, "git", append([]string{"ls-tree", "-z", "--full-tree", ref, "--"}, paths...)...)
		if err != nil {
			continue
		}
		for _, entry := range strings.Split(out, "\x00") {
			meta, path, ok := strings.Cut(entry, "\t")
			if f := strings.Fields(meta); ok && len(f) == 3 && f[1] == "blob" {
				trees[i][path] = f[2]
			}
		}
	}

	// What each file holds here.
	here := map[string]string{}
	var present []string
	for _, p := range paths {
		if fi, err := os.Lstat(filepath.Join(dir, p)); err == nil && fi.Mode().IsRegular() {
			present = append(present, p)
		}
	}
	for start := 0; start < len(present); start += 100 {
		chunk := present[start:min(start+100, len(present))]
		out, err := command(ctx, dir, "git", append([]string{"hash-object", "--"}, chunk...)...)
		if err != nil {
			continue
		}
		for i, sha := range strings.Fields(out) {
			if i < len(chunk) {
				here[chunk[i]] = sha
			}
		}
	}

	for _, p := range paths {
		sha, exists := here[p]
		found := ""
		for i, ref := range refs {
			theirs, has := trees[i][p]
			if (exists && has && theirs == sha) || (!exists && !has) {
				found = refName(ref)
				break
			}
		}
		if found != "" {
			onRemote = append(onRemote, p+" ("+found+")")
		} else {
			onlyHere = append(onlyHere, p)
		}
	}
	return onRemote, onlyHere
}

// refName shortens a commit id given as a ref; branch names stay.
func refName(ref string) string {
	if len(ref) == 40 && strings.Trim(ref, "0123456789abcdef") == "" {
		return "the server's branch tip " + ref[:10]
	}
	return ref
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func firstLineOf(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	if len(line) > 120 {
		line = line[:120] + "…"
	}
	return line
}

// Summary is the remote state in a few lines, as the person reads it.
func (r RemoteState) Summary() string {
	if r.Remote == "" {
		return "no remote: this is the only copy of everything in it"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "server: %s\n", r.Server)
	switch {
	case r.OnServer && r.Unpushed == 0:
		fmt.Fprintf(&b, "commits: all pushed (last one on %s/%s)\n", r.Remote, r.FoundOn)
	case r.Unpushed > 0:
		fmt.Fprintf(&b, "commits: %d on no remote branch\n", r.Unpushed)
	default:
		fmt.Fprintf(&b, "commits: the last one is not on the server\n")
	}
	if r.BranchGone {
		fmt.Fprintf(&b, "branch: %s/%s was deleted on the server\n", r.Remote, r.Branch)
	}
	if len(r.FilesOnRemote) > 0 {
		fmt.Fprintf(&b, "uncommitted, already on the remote (same content): %s\n", strings.Join(r.FilesOnRemote, ", "))
	}
	if len(r.FilesOnlyHere) > 0 {
		fmt.Fprintf(&b, "uncommitted, only on this machine: %s\n", strings.Join(r.FilesOnlyHere, ", "))
	}
	if r.Pushed() {
		b.WriteString("verdict: everything in it is on the remote\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// Label is the remote state in a few words, for a card.
func (r RemoteState) Label() string {
	switch {
	case r.Remote == "":
		return "no remote"
	case r.Pushed() && r.Server == "checked":
		return "all on the remote (checked with the server)"
	case r.Pushed():
		return "all on the remote (as of the last fetch)"
	}
	var parts []string
	if r.Unpushed > 0 {
		parts = append(parts, fmt.Sprintf("%d commits not pushed", r.Unpushed))
	} else if !r.OnServer {
		parts = append(parts, "last commit not on the server")
	}
	if n := len(r.FilesOnlyHere); n > 0 {
		parts = append(parts, fmt.Sprintf("%d files only here", n))
	}
	return strings.Join(parts, ", ")
}
