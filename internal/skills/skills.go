// Package skills looks after the skills of the AI coding agents installed
// on this machine: Claude Code, Codex, Gemini CLI, opencode, Cursor and
// GitHub Copilot. A skill is a folder with a SKILL.md; every agent reads its
// own folder of them and most also read the shared one, ~/.agents/skills.
// The package lists what is installed and who can see it, finds what is
// broken, and installs, shares, updates and removes skills.
package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Home is the home directory. Tests replace it.
var Home = func() string {
	h, _ := os.UserHomeDir()
	return h
}()

// LookPath finds a binary on PATH. Tests replace it.
var LookPath = exec.LookPath

// Location is a folder that holds skills, one folder per skill.
type Location struct {
	ID   string `json:"id"`
	Path string `json:"path"` // with ~ for the home
}

// Shared is the location most agents read, whoever made them.
const Shared = "shared"

// Locations are the personal skill folders, the shared one first.
var Locations = []Location{
	{Shared, "~/.agents/skills"},
	{"claude", "~/.claude/skills"},
	{"gemini", "~/.gemini/skills"},
	{"opencode", "~/.config/opencode/skills"},
	{"cursor", "~/.cursor/skills"},
	{"copilot", "~/.copilot/skills"},
}

// Agent is a coding agent and the locations it reads skills from, as its
// documentation states them.
type Agent struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Bins  []string `json:"-"`
	Home  string   `json:"-"` // its configuration folder
	Reads []string `json:"reads"`
}

// Agents are the agents omega knows, in display order.
var Agents = []Agent{
	{ID: "claude", Name: "Claude Code", Bins: []string{"claude"}, Home: "~/.claude", Reads: []string{"claude"}},
	{ID: "codex", Name: "Codex CLI", Bins: []string{"codex"}, Home: "~/.codex", Reads: []string{Shared}},
	{ID: "gemini", Name: "Gemini CLI", Bins: []string{"gemini"}, Home: "~/.gemini", Reads: []string{Shared, "gemini"}},
	{ID: "opencode", Name: "opencode", Bins: []string{"opencode"}, Home: "~/.config/opencode", Reads: []string{"opencode", "claude", Shared}},
	{ID: "cursor", Name: "Cursor", Bins: []string{"cursor-agent", "cursor"}, Home: "~/.cursor", Reads: []string{Shared, "cursor", "claude"}},
	{ID: "copilot", Name: "GitHub Copilot CLI", Bins: []string{"copilot"}, Home: "~/.copilot", Reads: []string{Shared, "copilot"}},
}

// Expand turns a leading ~ into the home directory.
func Expand(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		return filepath.Join(Home, filepath.FromSlash(strings.TrimPrefix(p, "~")))
	}
	return p
}

// Tilde writes a path under the home with ~, for display.
func Tilde(p string) string {
	if rel, err := filepath.Rel(Home, p); err == nil && !strings.HasPrefix(rel, "..") {
		return "~/" + filepath.ToSlash(rel)
	}
	return p
}

// LocationByID returns a location.
func LocationByID(id string) (Location, bool) {
	for _, l := range Locations {
		if l.ID == id {
			return l, true
		}
	}
	return Location{}, false
}

// AgentByID returns an agent.
func AgentByID(id string) (Agent, bool) {
	for _, a := range Agents {
		if a.ID == id {
			return a, true
		}
	}
	return Agent{}, false
}

// Installed says whether an agent is on this machine: its command is on
// PATH or its configuration folder exists.
func (a Agent) Installed() bool {
	for _, b := range a.Bins {
		if _, err := LookPath(b); err == nil {
			return true
		}
	}
	fi, err := os.Stat(Expand(a.Home))
	return err == nil && fi.IsDir()
}

// Levels of a finding.
const (
	Problem = "problem" // the skill does not work, or not as written
	Note    = "note"    // worth knowing
)

// Finding is something the audit found in a skill.
type Finding struct {
	Code   string `json:"code"`
	Level  string `json:"level"`
	Detail string `json:"detail,omitempty"`
}

// Text is the finding in English, as the command line prints it.
func (f Finding) Text() string {
	text := map[string]string{
		"broken-link":      "the link points to a folder that no longer exists",
		"no-frontmatter":   "SKILL.md does not start with a --- block holding name and description",
		"no-name":          "SKILL.md has no name",
		"bad-name":         "the name must be lowercase letters, digits and hyphens, 64 characters at most",
		"name-mismatch":    "the name in SKILL.md is not the folder's name",
		"no-description":   "SKILL.md has no description, so no agent knows when to use it",
		"long-description": "the description is longer than 1024 characters",
		"differs":          "the copies have different content",
		"hidden":           "not visible to",
	}[f.Code]
	if text == "" {
		text = f.Code
	}
	if f.Detail != "" {
		text += ": " + f.Detail
	}
	return text
}

// Install is a skill in one location.
type Install struct {
	Location    string    `json:"location"`
	Path        string    `json:"path"`
	Link        string    `json:"link,omitempty"` // where it points, when it is a link
	Broken      bool      `json:"broken,omitempty"`
	Real        string    `json:"-"` // the folder after following links
	Hash        string    `json:"-"`
	Name        string    `json:"-"` // from SKILL.md
	Description string    `json:"-"`
	Findings    []Finding `json:"-"`
}

// Skill is a skill and every place it is installed.
type Skill struct {
	Name        string    `json:"name"` // the folder's name
	Description string    `json:"description"`
	Installs    []Install `json:"installs"`
	Visible     []string  `json:"visible"` // installed agents that see it
	Hidden      []string  `json:"hidden"`  // installed agents that do not
	Origin      *Origin   `json:"origin,omitempty"`
	Findings    []Finding `json:"findings"`
}

// Problems counts the findings that are problems.
func (s Skill) Problems() int {
	n := 0
	for _, f := range s.Findings {
		if f.Level == Problem {
			n++
		}
	}
	return n
}

// source is the installation the others are compared with and linked to: a
// real folder before a link, the shared location before the rest.
func (s Skill) source() (Install, bool) {
	var best Install
	found := false
	for _, in := range s.Installs {
		if in.Broken {
			continue
		}
		if !found || (best.Link != "" && in.Link == "") {
			best, found = in, true
		}
	}
	return best, found
}

// AgentStatus is an agent as the inventory found it.
type AgentStatus struct {
	Agent
	Installed bool `json:"installed"`
	Skills    int  `json:"skills"` // how many it can see
}

// Inventory is every skill on this machine.
type Inventory struct {
	Agents []AgentStatus `json:"agents"`
	Skills []Skill       `json:"skills"`
}

// Find returns a skill by name.
func (inv *Inventory) Find(name string) (*Skill, bool) {
	for i := range inv.Skills {
		if inv.Skills[i].Name == name {
			return &inv.Skills[i], true
		}
	}
	return nil, false
}

// installed are the agents on this machine.
func (inv *Inventory) installed() []Agent {
	var out []Agent
	for _, a := range inv.Agents {
		if a.Installed {
			out = append(out, a.Agent)
		}
	}
	return out
}

var nameOK = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidName says whether a skill may be called name.
func ValidName(name string) bool { return len(name) <= 64 && nameOK.MatchString(name) }

// frontmatter reads name and description from the block that opens a
// SKILL.md. ok is false when the file has no such block.
func frontmatter(text string) (name, description string, ok bool) {
	text = strings.TrimPrefix(strings.ReplaceAll(text, "\r\n", "\n"), "\ufeff")
	if !strings.HasPrefix(text, "---\n") {
		return "", "", false
	}
	end := strings.Index(text[4:], "\n---")
	if end < 0 {
		return "", "", false
	}
	lines := strings.Split(text[4:4+end], "\n")
	for i := 0; i < len(lines); i++ {
		key, value, found := strings.Cut(lines[i], ":")
		if !found || strings.HasPrefix(key, " ") || strings.HasPrefix(key, "\t") {
			continue
		}
		value = strings.TrimSpace(value)
		// A block scalar or an empty value continues on the indented lines.
		if value == "" || strings.HasPrefix(value, ">") || strings.HasPrefix(value, "|") {
			var parts []string
			for i+1 < len(lines) && (strings.HasPrefix(lines[i+1], " ") || strings.HasPrefix(lines[i+1], "\t") || lines[i+1] == "") {
				i++
				if part := strings.TrimSpace(lines[i]); part != "" {
					parts = append(parts, part)
				}
			}
			value = strings.Join(parts, " ")
		} else if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			value = value[1 : len(value)-1]
		}
		switch strings.TrimSpace(key) {
		case "name":
			name = value
		case "description":
			description = value
		}
	}
	return name, description, true
}

// HashDir is a digest of a folder's files: their paths and what they hold.
// Version control and links are left out, as when a skill is copied.
func HashDir(dir string) (string, error) {
	h := sha256.New()
	err := walkFiles(dir, func(rel, path string) error {
		io.WriteString(h, rel+"\x00")
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(h, f)
		io.WriteString(h, "\x00")
		return err
	})
	return hex.EncodeToString(h.Sum(nil)), err
}

// walkFiles calls fn for every regular file under dir, in a fixed order,
// with its path relative to dir written with slashes.
func walkFiles(dir string, fn func(rel, path string) error) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		return fn(filepath.ToSlash(rel), path)
	})
}

// readInstall describes the entry called name in a location. ok is false
// when it is not a skill: a file, or a folder with no SKILL.md.
func readInstall(loc Location, name string) (Install, bool) {
	in := Install{Location: loc.ID, Path: filepath.Join(Expand(loc.Path), name)}
	li, err := os.Lstat(in.Path)
	if err != nil {
		return in, false
	}
	if li.Mode()&os.ModeSymlink != 0 {
		in.Link, _ = os.Readlink(in.Path)
		if !filepath.IsAbs(in.Link) {
			in.Link = filepath.Join(filepath.Dir(in.Path), in.Link)
		}
	}
	real, err := filepath.EvalSymlinks(in.Path)
	if err != nil {
		if in.Link == "" {
			return in, false
		}
		in.Broken = true
		in.Findings = []Finding{{Code: "broken-link", Level: Problem, Detail: Tilde(in.Link)}}
		return in, true
	}
	in.Real = real
	data, err := os.ReadFile(filepath.Join(real, "SKILL.md"))
	if err != nil {
		return in, false
	}
	in.Hash, _ = HashDir(real)
	var ok bool
	if in.Name, in.Description, ok = frontmatter(string(data)); !ok {
		in.Findings = append(in.Findings, Finding{Code: "no-frontmatter", Level: Problem})
		return in, true
	}
	switch {
	case in.Name == "":
		in.Findings = append(in.Findings, Finding{Code: "no-name", Level: Problem})
	case !ValidName(in.Name):
		in.Findings = append(in.Findings, Finding{Code: "bad-name", Level: Problem, Detail: in.Name})
	case in.Name != name:
		in.Findings = append(in.Findings, Finding{Code: "name-mismatch", Level: Note, Detail: in.Name})
	}
	switch {
	case in.Description == "":
		in.Findings = append(in.Findings, Finding{Code: "no-description", Level: Problem})
	case len(in.Description) > 1024:
		in.Findings = append(in.Findings, Finding{Code: "long-description", Level: Problem})
	}
	return in, true
}

// Scan reads every location and returns what is installed, who sees it and
// what is wrong with it.
func Scan() (*Inventory, error) {
	inv := &Inventory{}
	byName := map[string]*Skill{}
	var names []string
	for _, loc := range Locations {
		entries, err := os.ReadDir(Expand(loc.Path))
		if err != nil {
			continue // the folder does not exist: nothing is installed there
		}
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".") {
				continue
			}
			in, ok := readInstall(loc, e.Name())
			if !ok {
				continue
			}
			s := byName[e.Name()]
			if s == nil {
				s = &Skill{Name: e.Name()}
				byName[e.Name()] = s
				names = append(names, e.Name())
			}
			s.Installs = append(s.Installs, in)
		}
	}
	sort.Strings(names)

	for _, a := range Agents {
		inv.Agents = append(inv.Agents, AgentStatus{Agent: a, Installed: a.Installed()})
	}
	origins := loadOrigins()
	for _, name := range names {
		s := byName[name]
		if o, ok := origins[name]; ok {
			s.Origin = &o
		}
		audit(s, inv)
		inv.Skills = append(inv.Skills, *s)
	}
	return inv, nil
}

// audit fills in what is derived from a skill's installations: its
// description, who sees it and the findings.
func audit(s *Skill, inv *Inventory) {
	seen := map[string]bool{}
	add := func(f Finding) {
		if key := f.Code + "\x00" + f.Detail; !seen[key] {
			seen[key] = true
			s.Findings = append(s.Findings, f)
		}
	}
	if src, ok := s.source(); ok {
		s.Description = src.Description
	}
	in := map[string]bool{} // locations where it works
	hashes := map[string][]string{}
	for _, i := range s.Installs {
		for _, f := range i.Findings {
			add(f)
		}
		if i.Broken {
			continue
		}
		in[i.Location] = true
		hashes[i.Hash] = append(hashes[i.Hash], i.Location)
	}
	if len(hashes) > 1 {
		var groups []string
		for _, locs := range hashes {
			groups = append(groups, strings.Join(locs, " = "))
		}
		sort.Strings(groups)
		add(Finding{Code: "differs", Level: Problem, Detail: strings.Join(groups, " ≠ ")})
	}
	var hidden []string
	for i, a := range inv.Agents {
		if !a.Installed {
			continue
		}
		sees := false
		for _, loc := range a.Reads {
			sees = sees || in[loc]
		}
		if sees {
			s.Visible = append(s.Visible, a.ID)
			inv.Agents[i].Skills++
		} else {
			s.Hidden = append(s.Hidden, a.ID)
			hidden = append(hidden, a.Name)
		}
	}
	if len(hidden) > 0 && len(in) > 0 {
		add(Finding{Code: "hidden", Level: Note, Detail: strings.Join(hidden, ", ")})
	}
	if s.Findings == nil {
		s.Findings = []Finding{}
	}
}
