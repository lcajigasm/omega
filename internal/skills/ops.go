package skills

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/lcajigasm/omega/internal/config"
)

// ---- where a skill came from ----

// Origin is where an installed skill came from, so it can be updated.
type Origin struct {
	Source string `json:"source"`           // as it was given to install
	Git    string `json:"git,omitempty"`    // repository URL
	Ref    string `json:"ref,omitempty"`    // branch or tag; empty is the default branch
	Subdir string `json:"subdir,omitempty"` // the skill's folder inside the repository
	Dir    string `json:"dir,omitempty"`    // the skill's folder, when it came from this disk
	Commit string `json:"commit,omitempty"`
	Hash   string `json:"hash"` // content as installed, to notice local edits
	Time   string `json:"time"`
}

func originsPath() string { return filepath.Join(config.StateDir(), "skills.json") }

func loadOrigins() map[string]Origin {
	out := map[string]Origin{}
	if data, err := os.ReadFile(originsPath()); err == nil {
		_ = json.Unmarshal(data, &out)
	}
	return out
}

func saveOrigins(o map[string]Origin) error {
	data, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(originsPath()), 0o755); err != nil {
		return err
	}
	return os.WriteFile(originsPath(), append(data, '\n'), 0o644)
}

// ---- files ----

// copyDir copies a skill's files. Version control and links are left out: a
// link in a repository could point anywhere on this disk.
func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	return walkFiles(src, func(rel, path string) error {
		target := filepath.Join(dst, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		mode := os.FileMode(0o644)
		if fi, err := in.Stat(); err == nil && fi.Mode()&0o111 != 0 {
			mode = 0o755
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
}

// trash moves a folder out of the way without deleting it: to the Trash on
// a Mac, to omega's state folder elsewhere. It returns where it went.
func trash(path string) (string, error) {
	dir := filepath.Join(config.StateDir(), "removed-skills")
	if runtime.GOOS == "darwin" {
		dir = filepath.Join(Home, ".Trash")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(dir, filepath.Base(path))
	if _, err := os.Lstat(dest); err == nil {
		dest += " " + time.Now().Format("2006-01-02 15.04.05.000")
	}
	if err := os.Rename(path, dest); err != nil {
		// Another volume: copy everything, then delete. copyDir would not
		// do: it leaves out version control and links, and what is being
		// put away may be the only copy of them.
		if err := copyAll(path, dest); err != nil {
			os.RemoveAll(dest)
			return "", err
		}
		if err := os.RemoveAll(path); err != nil {
			return "", err
		}
	}
	return dest, nil
}

// copyAll copies a folder as it is: every file, hidden ones and .git
// included, and links as links.
func copyAll(src, dst string) error {
	return filepath.Walk(src, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case fi.Mode()&os.ModeSymlink != 0:
			to, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(to, target)
		case fi.IsDir():
			return os.MkdirAll(target, fi.Mode().Perm()|0o700)
		case !fi.Mode().IsRegular():
			return nil
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fi.Mode().Perm())
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
}

// ---- sharing ----

// Action puts a skill in a location, as a link to where it already is or
// as a copy.
type Action struct {
	Skill    string `json:"skill"`
	Location string `json:"location"`
	Path     string `json:"path"`
	Source   string `json:"source"`
}

// Skipped is a skill sync left alone, and why.
type Skipped struct {
	Skill  string `json:"skill"`
	Reason string `json:"reason"`
}

// cover picks the locations that make a skill visible to agents, the fewest
// possible: the one most of them read first.
func cover(agents []Agent) []string {
	var out []string
	left := append([]Agent(nil), agents...)
	for len(left) > 0 {
		best, bestN := "", 0
		for _, loc := range Locations {
			n := 0
			for _, a := range left {
				for _, r := range a.Reads {
					if r == loc.ID {
						n++
					}
				}
			}
			if n > bestN {
				best, bestN = loc.ID, n
			}
		}
		if best == "" {
			break
		}
		out = append(out, best)
		var rest []Agent
		for _, a := range left {
			reads := false
			for _, r := range a.Reads {
				reads = reads || r == best
			}
			if !reads {
				rest = append(rest, a)
			}
		}
		left = rest
	}
	return out
}

// PlanSync says what has to be created for every installed agent to see the
// named skills, or all of them when no name is given. Skills whose copies
// differ are left alone: which copy is right is not omega's to decide.
func PlanSync(inv *Inventory, names []string) ([]Action, []Skipped, error) {
	var skills []*Skill
	if len(names) == 0 {
		for i := range inv.Skills {
			skills = append(skills, &inv.Skills[i])
		}
	}
	for _, n := range names {
		s, ok := inv.Find(n)
		if !ok {
			return nil, nil, fmt.Errorf("no skill called %q is installed", n)
		}
		skills = append(skills, s)
	}
	var actions []Action
	var skipped []Skipped
	for _, s := range skills {
		if len(s.Hidden) == 0 {
			continue
		}
		src, ok := s.source()
		if !ok {
			skipped = append(skipped, Skipped{s.Name, "every installation is a broken link"})
			continue
		}
		differs := false
		for _, f := range s.Findings {
			differs = differs || f.Code == "differs"
		}
		if differs {
			skipped = append(skipped, Skipped{s.Name, "its copies differ; remove the ones that are wrong first"})
			continue
		}
		var hidden []Agent
		for _, id := range s.Hidden {
			a, _ := AgentByID(id)
			hidden = append(hidden, a)
		}
		for _, loc := range cover(hidden) {
			l, _ := LocationByID(loc)
			actions = append(actions, Action{Skill: s.Name, Location: loc, Path: filepath.Join(Expand(l.Path), s.Name), Source: src.Real})
		}
	}
	return actions, skipped, nil
}

// Apply carries out an action. It links unless copy is set or this system
// refuses the link (Windows without developer mode), and reports which.
func Apply(a Action, copy bool) (linked bool, err error) {
	if li, err := os.Lstat(a.Path); err == nil {
		// A link to nowhere is in the way of nothing.
		if _, statErr := os.Stat(a.Path); li.Mode()&os.ModeSymlink == 0 || statErr == nil {
			return false, fmt.Errorf("%s already exists", Tilde(a.Path))
		}
		if err := os.Remove(a.Path); err != nil {
			return false, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(a.Path), 0o755); err != nil {
		return false, err
	}
	if !copy {
		if err := os.Symlink(a.Source, a.Path); err == nil {
			return true, nil
		}
	}
	return false, copyDir(a.Source, a.Path)
}

// ---- removing ----

// PlanRemove lists what removing a skill takes away: every installation,
// or with from the one in that location and the links that point to it.
func PlanRemove(inv *Inventory, name, from string) ([]Install, error) {
	s, ok := inv.Find(name)
	if !ok {
		return nil, fmt.Errorf("no skill called %q is installed", name)
	}
	if from == "" {
		return s.Installs, nil
	}
	if _, ok := LocationByID(from); !ok {
		return nil, fmt.Errorf("unknown location %q (available: %s)", from, locationIDs())
	}
	var target *Install
	for i := range s.Installs {
		if s.Installs[i].Location == from {
			target = &s.Installs[i]
		}
	}
	if target == nil {
		return nil, fmt.Errorf("%s is not installed in %s", name, from)
	}
	out := []Install{*target}
	if target.Link == "" {
		for _, in := range s.Installs {
			if in.Location != from && in.Link != "" && !in.Broken && in.Real == target.Real {
				out = append(out, in)
			}
		}
	}
	return out, nil
}

func locationIDs() string {
	var ids []string
	for _, l := range Locations {
		ids = append(ids, l.ID)
	}
	return strings.Join(ids, ", ")
}

// Remove takes installations away: links are deleted, folders are moved to
// the trash. It returns where the folders went.
func Remove(name string, installs []Install) ([]string, error) {
	sort.SliceStable(installs, func(i, j int) bool { return installs[i].Link != "" && installs[j].Link == "" })
	var kept []string
	for _, in := range installs {
		if _, err := os.Lstat(in.Path); err != nil {
			continue // already gone with a folder it was inside
		}
		if in.Link != "" {
			if err := os.Remove(in.Path); err != nil {
				return kept, err
			}
			continue
		}
		dest, err := trash(in.Path)
		if err != nil {
			return kept, err
		}
		kept = append(kept, dest)
	}
	// Forget where it came from once nothing of it is left.
	if inv, err := Scan(); err == nil {
		if _, still := inv.Find(name); !still {
			if o := loadOrigins(); o[name] != (Origin{}) {
				delete(o, name)
				_ = saveOrigins(o)
			}
		}
	}
	return kept, nil
}

// ---- installing ----

// git runs git and returns what it printed. It never waits for a password.
var git = func(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if _, lookErr := LookPath("git"); lookErr != nil {
			return "", errors.New("git is not installed")
		}
		return "", fmt.Errorf("git %s: %s", args[0], strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

var (
	githubTree = regexp.MustCompile(`^https://github\.com/([^/]+/[^/]+)/tree/([^/]+)(?:/(.+))?$`)
	shorthand  = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
)

// parseSource reads what install was given: a folder of this disk, a
// repository URL, a GitHub page of a folder in a repository, or owner/repo.
func parseSource(src string) (Origin, error) {
	src = strings.TrimSpace(src)
	o := Origin{Source: src}
	switch {
	case src == "":
		return o, errors.New("say where the skill is: a folder, a git URL or owner/repo")
	case strings.HasPrefix(src, "-"):
		return o, fmt.Errorf("%q is not a folder or a repository", src)
	case githubTree.MatchString(src):
		m := githubTree.FindStringSubmatch(src)
		o.Git, o.Ref, o.Subdir = "https://github.com/"+m[1], m[2], strings.Trim(m[3], "/")
	case strings.Contains(src, "://") || strings.HasPrefix(src, "git@"):
		o.Git = src
	default:
		dir := Expand(src)
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			abs, err := filepath.Abs(dir)
			if err != nil {
				return o, err
			}
			o.Dir = abs
			break
		}
		if !shorthand.MatchString(src) {
			return o, fmt.Errorf("%q is not a folder, a git URL or owner/repo", src)
		}
		o.Git = "https://github.com/" + src
	}
	return o, nil
}

// found is a skill inside a folder or a clone.
type found struct {
	name string
	dir  string
	sub  string // dir relative to the root searched
}

// findSkills lists the skills under root: root itself, or the folders one
// level down and those in the places repositories keep them.
func findSkills(root, fallbackName string) []found {
	hasSkill := func(dir string) bool {
		fi, err := os.Stat(filepath.Join(dir, "SKILL.md"))
		return err == nil && fi.Mode().IsRegular()
	}
	if hasSkill(root) {
		name := fallbackName
		if data, err := os.ReadFile(filepath.Join(root, "SKILL.md")); err == nil {
			if n, _, _ := frontmatter(string(data)); ValidName(n) {
				name = n
			}
		}
		return []found{{name: name, dir: root}}
	}
	var out []found
	seen := map[string]bool{}
	for _, base := range []string{"", "skills", ".agents/skills", ".claude/skills"} {
		entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(base)))
		if err != nil {
			continue
		}
		for _, e := range entries {
			sub := filepath.Join(filepath.FromSlash(base), e.Name())
			if strings.HasPrefix(e.Name(), ".") || seen[e.Name()] || !hasSkill(filepath.Join(root, sub)) {
				continue
			}
			seen[e.Name()] = true
			out = append(out, found{name: e.Name(), dir: filepath.Join(root, sub), sub: sub})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// fetch makes the source's files available on disk and returns the folder,
// the commit when it is a repository, and what to call to clean up.
func fetch(ctx context.Context, o Origin) (root, commit string, done func(), err error) {
	if o.Git == "" {
		return o.Dir, "", func() {}, nil
	}
	tmp, err := os.MkdirTemp("", "omega-skill-")
	if err != nil {
		return "", "", nil, err
	}
	done = func() { os.RemoveAll(tmp) }
	args := []string{"clone", "--quiet", "--depth", "1"}
	if o.Ref != "" {
		args = append(args, "--branch", o.Ref)
	}
	if _, err := git(ctx, append(args, "--", o.Git, tmp)...); err != nil {
		done()
		return "", "", nil, err
	}
	commit, err = git(ctx, "-C", tmp, "rev-parse", "HEAD")
	if err != nil {
		done()
		return "", "", nil, err
	}
	return tmp, commit, done, nil
}

// repoName is the last word of a repository URL or a path.
func repoName(s string) string {
	s = strings.TrimSuffix(strings.TrimRight(filepath.ToSlash(s), "/"), ".git")
	if i := strings.LastIndexAny(s, "/:"); i >= 0 {
		s = s[i+1:]
	}
	return strings.ToLower(s)
}

// InstallOptions say what to install from a source and for whom.
type InstallOptions struct {
	Only  []string // names of the skills wanted; empty is all the source has
	To    []string // agent IDs; empty is every agent installed
	Copy  bool     // copy where a link would do
	Force bool     // replace a skill that is already installed
}

// Installed is a skill install put in place.
type Installed struct {
	Skill string   `json:"skill"`
	Paths []string `json:"paths"`
}

// InstallFrom installs the skills of a source: a folder, a git URL, a
// GitHub link to a folder, or owner/repo. The files are copied once; the
// other locations the agents need get a link to that copy.
func InstallFrom(ctx context.Context, source string, opts InstallOptions) ([]Installed, error) {
	origin, err := parseSource(source)
	if err != nil {
		return nil, err
	}
	var targets []Agent
	for _, id := range opts.To {
		a, ok := AgentByID(id)
		if !ok {
			return nil, fmt.Errorf("unknown agent %q (available: %s)", id, agentIDs())
		}
		targets = append(targets, a)
	}
	inv, err := Scan()
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		targets = inv.installed()
	}
	locs := cover(targets)
	if len(locs) == 0 {
		locs = []string{Shared} // no agent yet: where the next one will look
	}

	root, commit, done, err := fetch(ctx, origin)
	if err != nil {
		return nil, err
	}
	defer done()
	top := filepath.Join(root, filepath.FromSlash(origin.Subdir))
	if rel, err := filepath.Rel(root, top); err != nil || strings.HasPrefix(rel, "..") {
		return nil, fmt.Errorf("%q is outside the repository", origin.Subdir)
	}
	fallback := repoName(top)
	if origin.Subdir == "" && origin.Git != "" {
		fallback = repoName(origin.Git)
	}
	all := findSkills(top, fallback)
	if len(all) == 0 {
		return nil, fmt.Errorf("no skill found in %s: no SKILL.md there, one folder down or under skills/", source)
	}
	var picked []found
	if len(opts.Only) == 0 {
		picked = all
	}
	for _, want := range opts.Only {
		ok := false
		for _, f := range all {
			if f.name == want {
				picked, ok = append(picked, f), true
			}
		}
		if !ok {
			var names []string
			for _, f := range all {
				names = append(names, f.name)
			}
			return nil, fmt.Errorf("%s has no skill %q (it has: %s)", source, want, strings.Join(names, ", "))
		}
	}
	for _, f := range picked {
		if !ValidName(f.name) {
			return nil, fmt.Errorf("%q is not a valid skill name: lowercase letters, digits and hyphens", f.name)
		}
		if _, exists := inv.Find(f.name); exists && !opts.Force {
			return nil, fmt.Errorf("%s is already installed; `omega skills update %s` brings it up to date, --force replaces it", f.name, f.name)
		}
	}

	origins := loadOrigins()
	var out []Installed
	for _, f := range picked {
		res := Installed{Skill: f.name}
		// The copy goes where the skill already is, when it is being replaced.
		dest := ""
		if s, exists := inv.Find(f.name); exists {
			if src, ok := s.source(); ok {
				dest = src.Real
				if err := replaceAll(s, src, f.dir); err != nil {
					return out, err
				}
			} else {
				_, _ = Remove(f.name, s.Installs)
			}
		}
		if dest == "" {
			l, _ := LocationByID(locs[0])
			dest = filepath.Join(Expand(l.Path), f.name)
			if err := replace(dest, f.dir); err != nil {
				return out, err
			}
		}
		res.Paths = append(res.Paths, dest)

		// Whoever among the targets still cannot see it gets a link.
		after, err := Scan()
		if err != nil {
			return out, err
		}
		if s, ok := after.Find(f.name); ok {
			var hidden []Agent
			for _, a := range targets {
				sees := false
				for _, in := range s.Installs {
					for _, r := range a.Reads {
						sees = sees || (!in.Broken && r == in.Location)
					}
				}
				if !sees {
					hidden = append(hidden, a)
				}
			}
			for _, loc := range cover(hidden) {
				l, _ := LocationByID(loc)
				a := Action{Skill: f.name, Location: loc, Path: filepath.Join(Expand(l.Path), f.name), Source: dest}
				if _, err := Apply(a, opts.Copy); err != nil {
					return out, err
				}
				res.Paths = append(res.Paths, a.Path)
			}
		}

		o := origin
		o.Commit, o.Time = commit, time.Now().UTC().Format(time.RFC3339)
		if origin.Git != "" {
			o.Subdir = filepath.ToSlash(filepath.Join(filepath.FromSlash(origin.Subdir), f.sub))
			if o.Subdir == "." {
				o.Subdir = ""
			}
		} else {
			o.Dir = f.dir
		}
		o.Hash, _ = HashDir(dest)
		origins[f.name] = o
		out = append(out, res)
	}
	return out, saveOrigins(origins)
}

func agentIDs() string {
	var ids []string
	for _, a := range Agents {
		ids = append(ids, a.ID)
	}
	return strings.Join(ids, ", ")
}

// replace puts the files of src at dest, and only once they are complete
// beside it. What was at dest always goes to the trash, never away: a copy
// in one location may hold notes or a .git that the others do not, and
// comparing hashes does not show it (HashDir leaves out version control and
// links).
func replace(dest, src string) error {
	fresh := dest + ".omega-new"
	_ = os.RemoveAll(fresh)
	if err := copyDir(src, fresh); err != nil {
		os.RemoveAll(fresh)
		return err
	}
	if _, err := os.Lstat(dest); err == nil {
		if _, err := trash(dest); err != nil {
			os.RemoveAll(fresh)
			return err
		}
	}
	return os.Rename(fresh, dest)
}

// replaceAll does it for a skill's source and for its copies in other
// locations, so they stay the same.
func replaceAll(s *Skill, src Install, from string) error {
	done := map[string]bool{}
	for _, in := range append([]Install{src}, s.Installs...) {
		if in.Broken || done[in.Real] {
			continue
		}
		done[in.Real] = true
		if err := replace(in.Real, from); err != nil {
			return err
		}
	}
	return nil
}

// ---- updating ----

// States of an update.
const (
	Updated   = "updated"
	Current   = "current"
	Available = "available" // newer at the source; only checked
	Modified  = "modified"  // edited here since it was installed: left alone
	Failed    = "failed"
	Untracked = "untracked" // omega did not install it, so it has no source
)

// UpdateResult is what update did with a skill.
type UpdateResult struct {
	Skill  string `json:"skill"`
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
}

// Update brings the named skills, or every skill omega installed, up to
// date with where they came from. check only looks. A skill edited on this
// machine is left alone unless force is set.
func Update(ctx context.Context, names []string, check, force bool) ([]UpdateResult, error) {
	inv, err := Scan()
	if err != nil {
		return nil, err
	}
	origins := loadOrigins()
	if len(names) == 0 {
		for _, s := range inv.Skills {
			if s.Origin != nil {
				names = append(names, s.Name)
			}
		}
	}
	type clone struct {
		root, commit string
		err          error
	}
	clones := map[string]*clone{}
	defer func() {
		for _, c := range clones {
			if c.root != "" {
				os.RemoveAll(c.root)
			}
		}
	}()

	var out []UpdateResult
	changed := false
	for _, name := range names {
		s, ok := inv.Find(name)
		if !ok {
			return out, fmt.Errorf("no skill called %q is installed", name)
		}
		res := UpdateResult{Skill: name}
		src, hasSrc := s.source()
		o, tracked := origins[name]
		switch {
		case !tracked:
			res.State, res.Detail = Untracked, "omega did not install it"
		case !hasSrc:
			res.State, res.Detail = Failed, "every installation is a broken link"
		default:
			res.Detail = o.Source
			if src.Hash != o.Hash && !force {
				res.State, res.Detail = Modified, "edited here since it was installed; --force replaces it"
				break
			}
			var from string // the folder holding the new files
			if o.Git == "" {
				from = o.Dir
			} else {
				ref := o.Ref
				if ref == "" {
					ref = "HEAD"
				}
				remote, err := git(ctx, "ls-remote", "--", o.Git, ref)
				if err != nil {
					res.State, res.Detail = Failed, err.Error()
					break
				}
				if fields := strings.Fields(remote); len(fields) > 0 && fields[0] == o.Commit {
					res.State = Current
					break
				}
				if check {
					res.State = Available
					break
				}
				key := o.Git + "\x00" + o.Ref
				c := clones[key]
				if c == nil {
					c = &clone{}
					c.root, c.commit, _, c.err = fetch(ctx, Origin{Git: o.Git, Ref: o.Ref})
					clones[key] = c
				}
				if c.err != nil {
					res.State, res.Detail = Failed, c.err.Error()
					break
				}
				from = filepath.Join(c.root, filepath.FromSlash(o.Subdir))
				o.Commit = c.commit
			}
			if fi, err := os.Stat(filepath.Join(from, "SKILL.md")); err != nil || !fi.Mode().IsRegular() {
				res.State, res.Detail = Failed, "the source no longer has this skill: "+o.Source
				break
			}
			hash, err := HashDir(from)
			if err != nil {
				res.State, res.Detail = Failed, err.Error()
				break
			}
			if hash == src.Hash {
				res.State = Current
				if origins[name] != o { // a new commit that did not touch it
					o.Hash = hash
					origins[name], changed = o, true
				}
				break
			}
			if check {
				res.State = Available
				break
			}
			if err := replaceAll(s, src, from); err != nil {
				res.State, res.Detail = Failed, err.Error()
				break
			}
			o.Hash, o.Time = hash, time.Now().UTC().Format(time.RFC3339)
			origins[name], changed = o, true
			res.State = Updated
		}
		out = append(out, res)
	}
	if changed {
		return out, saveOrigins(origins)
	}
	return out, nil
}
