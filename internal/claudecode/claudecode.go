// Package claudecode finds what Claude Code sessions leave running: the dev
// servers, watchers, emulators and browsers a session started and that kept
// running after the session closed. It also lists the sessions that are
// still open, whether they are working or waiting, and ends them.
//
// Claude Code sets CLAUDECODE=1 in the environment of everything it starts,
// with the ID of the session (CLAUDE_CODE_SESSION_ID) and of its own process
// (CLAUDE_PID). A process carrying them is a leftover once that Claude Code
// process is gone. Some programs keep their environment unreadable (Chrome
// does), so whatever a marked process started counts as part of its session
// too. Nothing here leaves the machine.
package claudecode

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ErrUnsupported is what Scan returns where the environment of other
// processes cannot be read: Windows.
var ErrUnsupported = errors.New("finding what Claude Code left running works on macOS and Linux")

// Proc is a process started by a Claude Code session.
type Proc struct {
	PID     int    `json:"pid"`
	PPID    int    `json:"ppid"`
	Age     int    `json:"age"` // seconds since it started
	Session string `json:"session"`
	// Alive is true while the Claude Code process that started it runs.
	Alive   bool     `json:"alive"`
	Command string   `json:"command"`
	Path    string   `json:"path,omitempty"`
	Cwd     string   `json:"cwd,omitempty"`
	Ports   []string `json:"ports"`
	Memory  uint64   `json:"memory"` // bytes, resident
	// Helpers are the helper processes of the same app (a browser has one
	// per tab), folded into this row. Their memory and ports count here,
	// and they go when it goes.
	Helpers []Helper `json:"helpers,omitempty"`
}

// Helper is a process folded into the row of the app it belongs to.
type Helper struct {
	PID     int    `json:"pid"`
	Command string `json:"command"`
}

// Name is the program's name, without its folder.
func (p Proc) Name() string {
	exe := p.Path
	if exe == "" || !strings.HasPrefix(p.Command, exe) {
		exe, _, _ = strings.Cut(p.Command, " ")
	}
	name := filepath.Base(exe)
	if name == "Python" {
		return "python"
	}
	return name
}

// Short is the command line with the program's name in place of its path.
func (p Proc) Short() string {
	var rest string
	if p.Path != "" && strings.HasPrefix(p.Command, p.Path) {
		rest = p.Command[len(p.Path):]
	} else if _, after, ok := strings.Cut(p.Command, " "); ok {
		rest = " " + after
	}
	// ps prints a newline inside an argument as \012.
	return strings.ReplaceAll(p.Name()+rest, `\012`, " ")
}

// bundleRoot is the app this process is the main executable of, as
// "/Applications/Google Chrome.app/", or "" when it is not one.
func (p Proc) bundleRoot() string {
	i := strings.Index(p.Path, ".app/Contents/MacOS/")
	if i < 0 {
		return ""
	}
	return p.Path[:i] + ".app/"
}

// Group is what one closed session left running.
type Group struct {
	Session string `json:"session"`
	Project string `json:"project"`
	Title   string `json:"title,omitempty"`
	Procs   []Proc `json:"procs"`
}

// Memory is what the group holds, in bytes.
func (g Group) Memory() uint64 {
	var n uint64
	for _, p := range g.Procs {
		n += p.Memory
	}
	return n
}

// Oldest is the age of its oldest process, in seconds.
func (g Group) Oldest() int {
	o := 0
	for _, p := range g.Procs {
		o = max(o, p.Age)
	}
	return o
}

// Session statuses as Claude Code records them; Unknown when it left no
// record for the process.
const (
	Busy    = "busy"
	Idle    = "idle"
	Unknown = "unknown"
)

// Session is a Claude Code session that is still open: the claude process
// and what it has had running for more than a minute.
type Session struct {
	PID     int    `json:"pid"`
	ID      string `json:"id,omitempty"`
	Project string `json:"project"`
	Title   string `json:"title,omitempty"`
	Cwd     string `json:"cwd,omitempty"`
	Age     int    `json:"age"`
	Command string `json:"command"`
	// Status is busy, idle, unknown or whatever newer status Claude Code
	// records. Since is when it entered it; zero when not known.
	Status string    `json:"status"`
	Since  time.Time `json:"since"`
	Memory uint64    `json:"memory"` // its own, without Procs
	Procs  []Proc    `json:"procs"`
	// Current is the session omega itself runs in: ending it would end
	// omega too, so it is never ended.
	Current bool `json:"current,omitempty"`
}

// IdleFor is how long the session has waited for input, or -1 while it is
// working or its status is not known.
func (s Session) IdleFor(now time.Time) time.Duration {
	if s.Status != Idle || s.Since.IsZero() {
		return -1
	}
	return max(0, now.Sub(s.Since))
}

// Total is what the session holds with its processes, in bytes.
func (s Session) Total() uint64 {
	n := s.Memory
	for _, p := range s.Procs {
		n += p.Memory
	}
	return n
}

// Report is one look at the machine.
type Report struct {
	// Leftovers are the processes of closed sessions, oldest first.
	Leftovers []Group `json:"leftovers"`
	// Sessions are the open sessions, the longest idle first.
	Sessions []Session `json:"sessions"`
	Taken    time.Time `json:"taken"`
	// live are the processes of open sessions, given to them once their
	// IDs are known.
	live []Proc
}

// LeftoverMemory is what every leftover holds, in bytes.
func (r *Report) LeftoverMemory() uint64 {
	var n uint64
	for _, g := range r.Leftovers {
		n += g.Memory()
	}
	return n
}

// LeftoverCount is the number of leftover rows.
func (r *Report) LeftoverCount() int {
	n := 0
	for _, g := range r.Leftovers {
		n += len(g.Procs)
	}
	return n
}

// IdleSessions are the open sessions that have waited at least limit,
// without the one omega runs in.
func (r *Report) IdleSessions(limit time.Duration) []Session {
	var out []Session
	for _, s := range r.Sessions {
		if d := s.IdleFor(r.Taken); !s.Current && d >= 0 && d >= limit {
			out = append(out, s)
		}
	}
	return out
}

// MinIdle is the shortest wait after which a session may be ended as idle:
// under it a session that only paused for a reply would go.
const MinIdle = 30 * time.Minute

// Scan looks at the running processes and at Claude Code's own records.
func Scan() (*Report, error) {
	snap, err := snapshot()
	if err != nil {
		return nil, err
	}
	home := homeDir()
	r := analyze(snap, selfPID(), time.Now())
	details(r)
	describe(r, snap, home)
	return r, nil
}

// raw is a process as the system lists it.
type raw struct {
	pid, ppid int
	age       int       // seconds
	start     time.Time // zero when not known
	exe       string    // path of the executable, or its name
	command   string    // full command line
	mine      bool      // it runs as the current user
	env       map[string]string
	rss       uint64 // bytes
	// lstart is the start time as `ps -o lstart` prints it in UTC: Claude
	// Code records it in that form.
	lstart string
}

// marker variables, the only ones read from the environment.
const (
	envMarker  = "CLAUDECODE"
	envSession = "CLAUDE_CODE_SESSION_ID"
	envPID     = "CLAUDE_PID"
)

var envKeys = []string{envMarker, envSession, envPID}

type snap struct {
	procs map[int]*raw
}

func (s snap) isClaude(pid int) bool {
	p := s.procs[pid]
	if p == nil {
		return false
	}
	// Exactly "claude": the desktop app's own process is "Claude".
	return filepath.Base(p.exe) == "claude"
}

// analyze finds the leftovers and the open sessions in a snapshot. self is
// omega's own process: it, what it started and what started it are never
// counted, and the session it runs in is marked current.
func analyze(s snap, self int, now time.Time) *Report {
	r := &Report{Taken: now, Leftovers: []Group{}, Sessions: []Session{}}
	ancestors := map[int]bool{}
	for at, seen := self, map[int]bool{}; at > 1 && !seen[at]; {
		seen[at] = true
		ancestors[at] = true
		p := s.procs[at]
		if p == nil {
			break
		}
		at = p.ppid
	}
	skip := func(p *raw) bool { return ancestors[p.pid] || p.ppid == self || s.isClaude(p.pid) }

	hasClaudeAncestor := func(pid int) bool {
		seen := map[int]bool{}
		for at := pid; at > 1 && !seen[at]; {
			seen[at] = true
			p := s.procs[at]
			if p == nil {
				return false
			}
			at = p.ppid
			if s.isClaude(at) {
				return true
			}
		}
		return false
	}

	pids := make([]int, 0, len(s.procs))
	for pid := range s.procs {
		pids = append(pids, pid)
	}
	sort.Ints(pids)

	var procs []Proc
	marked := map[int]int{}
	for _, pid := range pids {
		p := s.procs[pid]
		if !p.mine || p.env[envMarker] != "1" || skip(p) {
			continue
		}
		session := p.env[envSession]
		if session == "" {
			session = "no-session"
		}
		alive := hasClaudeAncestor(pid)
		if cp := atoi(p.env[envPID]); cp > 0 && s.isClaude(cp) {
			alive = true
		}
		marked[pid] = len(procs)
		procs = append(procs, Proc{PID: pid, PPID: p.ppid, Age: p.age, Session: session, Alive: alive,
			Command: p.command, Path: p.exe, Memory: p.rss, Ports: []string{}})
	}

	// A process started by a marked one belongs to its session, marker or not.
	for _, pid := range pids {
		p := s.procs[pid]
		if !p.mine || skip(p) {
			continue
		}
		if _, ok := marked[pid]; ok {
			continue
		}
		owner := -1
		seen := map[int]bool{}
		for at := pid; at > 1 && !seen[at]; {
			seen[at] = true
			up := s.procs[at]
			if up == nil {
				break
			}
			at = up.ppid
			if at == self || s.isClaude(at) {
				break
			}
			if i, ok := marked[at]; ok {
				owner = i
				break
			}
		}
		if owner < 0 {
			continue
		}
		procs = append(procs, Proc{PID: pid, PPID: p.ppid, Age: p.age, Session: procs[owner].Session,
			Alive: procs[owner].Alive, Command: p.command, Path: p.exe, Memory: p.rss, Ports: []string{}})
	}
	procs = fold(procs)

	// Leftovers: grouped by session, oldest first.
	by := map[string][]Proc{}
	for _, p := range procs {
		if !p.Alive {
			by[p.Session] = append(by[p.Session], p)
		}
	}
	for id, members := range by {
		sort.Slice(members, func(i, j int) bool { return members[i].Age > members[j].Age })
		r.Leftovers = append(r.Leftovers, Group{Session: id, Procs: members})
	}
	sort.Slice(r.Leftovers, func(i, j int) bool {
		a, b := r.Leftovers[i], r.Leftovers[j]
		if a.Oldest() != b.Oldest() {
			return a.Oldest() > b.Oldest()
		}
		return a.Session < b.Session
	})

	// Open sessions: claude processes not started by another claude.
	for _, pid := range pids {
		p := s.procs[pid]
		if !p.mine || !s.isClaude(pid) || s.isClaude(p.ppid) {
			continue
		}
		r.Sessions = append(r.Sessions, Session{PID: pid, Age: p.age, Command: p.command, Memory: p.rss,
			Status: Unknown, Procs: []Proc{}, Current: ancestors[pid]})
	}
	for _, p := range procs {
		if p.Alive {
			r.live = append(r.live, p)
		}
	}
	return r
}

// attach gives each open session its live processes older than a minute,
// once the session's ID is known.
func (r *Report) attach(procs []Proc) {
	for i := range r.Sessions {
		s := &r.Sessions[i]
		s.Procs = []Proc{}
		if s.ID == "" {
			continue
		}
		for _, p := range procs {
			if p.Alive && p.Age >= 60 && p.Session == s.ID {
				s.Procs = append(s.Procs, p)
			}
		}
	}
}

// fold puts an app's helper processes into the app's row: one browser is
// one row, not twenty.
func fold(procs []Proc) []Proc {
	index := map[int]int{}
	for i, p := range procs {
		index[p.PID] = i
	}
	folded := map[int]bool{}
	for i := range procs {
		app := -1
		seen := map[int]bool{}
		for at := i; !seen[at]; {
			seen[at] = true
			up, ok := index[procs[at].PPID]
			if !ok {
				break
			}
			root := procs[up].bundleRoot()
			if root == "" || !strings.HasPrefix(procs[i].Path, root) {
				break
			}
			app, at = up, up
		}
		if app < 0 {
			continue
		}
		procs[app].Helpers = append(procs[app].Helpers, Helper{PID: procs[i].PID, Command: procs[i].Command})
		procs[app].Memory += procs[i].Memory
		folded[i] = true
	}
	out := procs[:0:0]
	for i, p := range procs {
		if !folded[i] {
			out = append(out, p)
		}
	}
	return out
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// fields splits a line into n fields at runs of spaces; the last one keeps
// its inner spaces.
func fields(line string, n int) []string {
	out := make([]string, 0, n)
	rest := line
	for len(out) < n-1 {
		rest = strings.TrimLeft(rest, " \t")
		if rest == "" {
			return nil
		}
		end := strings.IndexAny(rest, " \t")
		if end < 0 {
			end = len(rest)
		}
		out = append(out, rest[:end])
		rest = rest[end:]
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return nil
	}
	return append(out, rest)
}

// elapsed reads `[[dd-]hh:]mm:ss` as ps prints it for etime.
func elapsed(etime string) int {
	days := 0
	if d, rest, ok := strings.Cut(etime, "-"); ok {
		days, etime = atoi(d), rest
	}
	parts := strings.Split(etime, ":")
	for len(parts) < 3 {
		parts = append([]string{"0"}, parts...)
	}
	return days*86400 + atoi(parts[0])*3600 + atoi(parts[1])*60 + atoi(parts[2])
}

// squeeze collapses runs of spaces, as Claude Code and ps pad dates
// differently.
func squeeze(s string) string { return strings.Join(strings.Fields(s), " ") }
