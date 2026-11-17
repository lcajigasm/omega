// Package aiagents finds what AI coding agents leave running: the dev
// servers, watchers, emulators and browsers a session of Claude Code, Codex,
// Gemini CLI and the others started and that kept running after the session
// closed. It also lists the agent sessions that are still open and ends
// them.
//
// Most agents put a variable in the environment of every command they run
// (Claude Code sets CLAUDECODE=1, with the session's ID and its own PID). A
// process carrying one is a leftover once the agent process that started it
// is gone. Some programs keep their environment unreadable (Chrome does), so
// whatever a marked process started counts as part of its session too. An
// agent that marks nothing still shows as an open session, with what runs
// under it, but what it leaves behind cannot be told from any other process.
// Nothing here leaves the machine.
package aiagents

import (
	"errors"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ErrUnsupported is what Scan returns where the environment of other
// processes cannot be read: Windows.
var ErrUnsupported = errors.New("finding what AI agents left running works on macOS and Linux")

// Proc is a process started by an agent session.
type Proc struct {
	PID   int    `json:"pid"`
	PPID  int    `json:"ppid"`
	Age   int    `json:"age"` // seconds since it started
	Agent string `json:"agent"`
	// Session is the agent's session ID, "pid-<n>" when only the agent's
	// process is known, or "" when not even that.
	Session string `json:"session"`
	// Alive is true while the agent process that started it runs.
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
	// Shared is a daemon the whole machine uses (git's fsmonitor, mutagen,
	// op): it is ended only when named by its PID.
	Shared bool `json:"shared,omitempty"`
	// agentPID is the running agent process it belongs to, when known.
	agentPID int
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
	Agent   string `json:"agent"`
	Session string `json:"session"`
	Project string `json:"project"`
	Title   string `json:"title,omitempty"`
	Procs   []Proc `json:"procs"`
}

// Key names the group among all the others: an agent and a session.
func (g Group) Key() string { return g.Agent + "/" + g.Session }

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

// Session statuses as Claude Code records them; Unknown for agents that
// record none, or when there is no record for the process.
const (
	Busy    = "busy"
	Idle    = "idle"
	Unknown = "unknown"
)

// Session is an agent session that is still open: the agent's process and
// what it has had running for more than a minute.
type Session struct {
	PID     int    `json:"pid"`
	Agent   string `json:"agent"`
	ID      string `json:"id,omitempty"`
	Project string `json:"project"`
	Title   string `json:"title,omitempty"`
	Cwd     string `json:"cwd,omitempty"`
	Age     int    `json:"age"`
	Command string `json:"command"`
	// Status is busy, idle, unknown or whatever newer status the agent
	// records. Since is when it entered it; zero when not known.
	Status string    `json:"status"`
	Since  time.Time `json:"since"`
	Memory uint64    `json:"memory"` // its own, without Procs
	Procs  []Proc    `json:"procs"`
	// Current is the session omega itself runs in: ending it would end
	// omega too, so it is never ended.
	Current bool `json:"current,omitempty"`
	// Service is an agent's background service rather than a session: it
	// runs the commands of the sessions that use it, and is not ended here.
	Service bool `json:"service,omitempty"`
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

// LeftoverMemory is what the leftovers that would be ended together hold,
// in bytes: shared daemons are not counted.
func (r *Report) LeftoverMemory() uint64 {
	var n uint64
	for _, g := range r.Leftovers {
		for _, p := range g.Procs {
			if !p.Shared {
				n += p.Memory
			}
		}
	}
	return n
}

// LeftoverCount is the number of leftover rows that would be ended
// together, and SharedCount the number of shared daemons kept.
func (r *Report) LeftoverCount() int {
	n := 0
	for _, g := range r.Leftovers {
		for _, p := range g.Procs {
			if !p.Shared {
				n++
			}
		}
	}
	return n
}

// SharedCount is the number of shared daemons among the leftovers.
func (r *Report) SharedCount() int {
	n := 0
	for _, g := range r.Leftovers {
		for _, p := range g.Procs {
			if p.Shared {
				n++
			}
		}
	}
	return n
}

// IdleSessions are the open sessions that have waited at least limit,
// without the one omega runs in. Only agents that record their status can
// be idle.
func (r *Report) IdleSessions(limit time.Duration) []Session {
	var out []Session
	for _, s := range r.Sessions {
		if d := s.IdleFor(r.Taken); !s.Current && !s.Service && d >= 0 && d >= limit {
			out = append(out, s)
		}
	}
	return out
}

// MinIdle is the shortest wait after which a session may be ended as idle:
// under it a session that only paused for a reply would go.
const MinIdle = 30 * time.Minute

// Scan looks at the running processes and at the agents' own records.
func Scan() (*Report, error) {
	snap, err := snapshot()
	if err != nil {
		return nil, err
	}
	r := analyze(snap, selfPID(), time.Now())
	details(r)
	describe(r, snap, homeDir())
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

type snap struct {
	procs map[int]*raw
	// agents caches what agent each process is, nil for none.
	agents map[int]*Agent
}

// agentOf is the agent a process is, or nil.
func (s *snap) agentOf(pid int) *Agent {
	if a, ok := s.agents[pid]; ok {
		return a
	}
	if s.agents == nil {
		s.agents = map[int]*Agent{}
	}
	var a *Agent
	if p := s.procs[pid]; p != nil {
		a = identify(p.exe, p.command)
	}
	s.agents[pid] = a
	return a
}

// agentAncestor is the nearest ancestor of pid that is the agent a (any
// agent when a is nil) and how many levels up it is, or 0.
func (s *snap) agentAncestor(pid int, a *Agent) (int, int) {
	seen := map[int]bool{}
	depth := 0
	for at := pid; at > 1 && !seen[at]; {
		seen[at] = true
		p := s.procs[at]
		if p == nil {
			return 0, 0
		}
		at = p.ppid
		depth++
		if b := s.agentOf(at); b != nil && (a == nil || b == a) {
			return at, depth
		}
	}
	return 0, 0
}

// owner says which agent session a marked process belongs to. Agents run
// inside agents (Codex started from Claude Code carries both markers): of
// the agents still running, the nearest ancestor wins, then one known only
// by the PID it left in the environment; when none runs, any other agent
// still running above it, and else the innermost, taken to be the one
// latest in the catalogue.
func (s *snap) owner(p *raw) (a *Agent, session string, agentPID int) {
	best := 1 << 40
	for i := range Catalogue {
		c := &Catalogue[i]
		if !c.marks(p.env) {
			continue
		}
		found, depth := s.agentAncestor(p.pid, c)
		score := depth
		if found == 0 {
			if n := atoi(p.env[c.PIDVar]); n > 0 && s.agentOf(n) == c {
				found, score = n, 1<<20
			} else {
				score = 1<<30 - i
			}
		}
		if score >= best {
			continue
		}
		best = score
		a, agentPID = c, found
		switch {
		case c.SessionVar != "" && p.env[c.SessionVar] != "":
			session = p.env[c.SessionVar]
		case found > 0:
			session = "pid-" + strconv.Itoa(found)
		case atoi(p.env[c.PIDVar]) > 0:
			session = "pid-" + p.env[c.PIDVar]
		default:
			session = ""
		}
	}
	// Whatever runs under an agent that is still running is not left
	// behind, whatever markers it inherited: a Codex started from a Cursor
	// session that has closed still owns its helpers.
	if a != nil && agentPID == 0 {
		if up, _ := s.agentAncestor(p.pid, nil); up > 0 {
			a, session, agentPID = s.agentOf(up), "pid-"+strconv.Itoa(up), up
		}
	}
	return a, session, agentPID
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
	skip := func(p *raw) bool { return ancestors[p.pid] || p.ppid == self || s.agentOf(p.pid) != nil }

	pids := make([]int, 0, len(s.procs))
	for pid := range s.procs {
		pids = append(pids, pid)
	}
	sort.Ints(pids)

	var procs []Proc
	marked := map[int]int{}
	for _, pid := range pids {
		p := s.procs[pid]
		if !p.mine || skip(p) {
			continue
		}
		a, session, agentPID := s.owner(p)
		if a == nil {
			continue
		}
		marked[pid] = len(procs)
		procs = append(procs, Proc{PID: pid, PPID: p.ppid, Age: p.age, Agent: a.ID, Session: session, Alive: agentPID > 0,
			Command: p.command, Path: p.exe, Memory: p.rss, Ports: []string{}, agentPID: agentPID})
	}

	// A process started by a marked one belongs to its session, marker or
	// not. A process under an agent that marks nothing belongs to that
	// agent's open session.
	for _, pid := range pids {
		p := s.procs[pid]
		if !p.mine || skip(p) {
			continue
		}
		if _, ok := marked[pid]; ok {
			continue
		}
		owner := -1
		var agent *Agent
		agentPID := 0
		seen := map[int]bool{}
		for at := pid; at > 1 && !seen[at]; {
			seen[at] = true
			up := s.procs[at]
			if up == nil {
				break
			}
			at = up.ppid
			if at == self {
				break
			}
			if a := s.agentOf(at); a != nil {
				if a.Marker == "" {
					agent, agentPID = a, at
				}
				break
			}
			if i, ok := marked[at]; ok {
				owner = i
				break
			}
		}
		switch {
		case owner >= 0:
			o := procs[owner]
			procs = append(procs, Proc{PID: pid, PPID: p.ppid, Age: p.age, Agent: o.Agent, Session: o.Session,
				Alive: o.Alive, Command: p.command, Path: p.exe, Memory: p.rss, Ports: []string{}, agentPID: o.agentPID})
		case agent != nil:
			procs = append(procs, Proc{PID: pid, PPID: p.ppid, Age: p.age, Agent: agent.ID, Session: "pid-" + strconv.Itoa(agentPID),
				Alive: true, Command: p.command, Path: p.exe, Memory: p.rss, Ports: []string{}, agentPID: agentPID})
		}
	}
	procs = fold(procs)
	for i := range procs {
		procs[i].Shared = shared(procs[i])
	}

	// Leftovers: grouped by agent and session, oldest first.
	by := map[[2]string][]Proc{}
	for _, p := range procs {
		if !p.Alive {
			k := [2]string{p.Agent, p.Session}
			by[k] = append(by[k], p)
		}
	}
	for k, members := range by {
		sort.Slice(members, func(i, j int) bool { return members[i].Age > members[j].Age })
		r.Leftovers = append(r.Leftovers, Group{Agent: k[0], Session: k[1], Procs: members})
	}
	sort.Slice(r.Leftovers, func(i, j int) bool {
		a, b := r.Leftovers[i], r.Leftovers[j]
		if a.Oldest() != b.Oldest() {
			return a.Oldest() > b.Oldest()
		}
		return a.Key() < b.Key()
	})

	// Open sessions: agent processes not started by the same agent (its
	// subagents and helpers are part of the session).
	for _, pid := range pids {
		p := s.procs[pid]
		a := s.agentOf(pid)
		if !p.mine || a == nil {
			continue
		}
		if up, _ := s.agentAncestor(pid, a); up != 0 {
			continue
		}
		r.Sessions = append(r.Sessions, Session{PID: pid, Agent: a.ID, Age: p.age, Command: p.command, Memory: p.rss,
			Status: Unknown, Procs: []Proc{}, Current: ancestors[pid], Service: a.service(p.command)})
	}
	for _, p := range procs {
		if p.Alive {
			r.live = append(r.live, p)
		}
	}
	r.attach(r.live)
	return r
}

// attach gives each open session its live processes older than a minute.
// A session is found by its ID once its records are read, else by its
// process.
func (r *Report) attach(procs []Proc) {
	for i := range r.Sessions {
		s := &r.Sessions[i]
		s.Procs = []Proc{}
		for _, p := range procs {
			mine := p.agentPID == s.PID || (s.ID != "" && p.Session == s.ID)
			if p.Alive && p.Age >= 60 && p.Agent == s.Agent && mine {
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
	if s == "" {
		return 0
	}
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
