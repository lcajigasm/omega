package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/aiagents"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/ui"
)

// What AI coding agents leave running: the dev servers, watchers, emulators
// and browsers of Claude Code, Codex, Gemini CLI and other sessions that have
// closed, and the sessions still open with what they hold. The desktop app
// shows the same through the functions at the end of the file.

var agentScan = aiagents.Scan

func agentsCmd() *cobra.Command {
	var asJSON bool
	list := func(cmd *cobra.Command, _ []string) error {
		r, err := agentScan()
		if err != nil {
			return err
		}
		if asJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(r)
		}
		writeAgents(cmd.OutOrStdout(), r)
		return nil
	}
	c := &cobra.Command{
		Use:     "agents",
		Aliases: []string{"claude", "leftovers"},
		Short:   i18n.T("cmd.agents"),
		Long:    i18n.T("cmd.agents_long", knownAgents()),
		Example: "  omega agents\n" +
			"  omega agents kill\n" +
			"  omega agents kill 41234\n" +
			"  omega agents end 3f2a9c1b\n" +
			"  omega agents idle --after 3h --yes\n" +
			"  omega agents --json",
		Args: cobra.NoArgs,
		RunE: list,
	}
	c.Flags().BoolVar(&asJSON, "json", false, i18n.T("flag.tools_json"))
	listCmd := &cobra.Command{Use: "list", Short: i18n.T("cmd.agents_list"), Args: cobra.NoArgs, RunE: list}
	listCmd.Flags().BoolVar(&asJSON, "json", false, i18n.T("flag.tools_json"))

	var yes bool
	confirm := func(cmd *cobra.Command, q string) (bool, error) {
		if yes {
			return true, nil
		}
		if !stdinIsTerminal() {
			return false, errors.New(i18n.T("agents.need_yes"))
		}
		return askYesNo(q), nil
	}

	kill := &cobra.Command{
		Use:   "kill [pid | session-id]...",
		Short: i18n.T("cmd.agents_kill"),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := agentScan()
			if err != nil {
				return err
			}
			targets, what, err := leftoverTargets(r, args)
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			if len(targets) == 0 {
				fmt.Fprintln(w, ui.NewReport(w).Glyph(ui.MarkOK)+" "+i18n.T("agents.nothing_left"))
				return nil
			}
			if ok, err := confirm(cmd, i18n.T("agents.confirm_kill", what, ui.Bytes(int64(memoryOf(targets))))); !ok || err != nil {
				return err
			}
			writeEnded(w, aiagents.Kill(targets, aiagents.Grace))
			return nil
		},
	}
	kill.Flags().BoolVarP(&yes, "yes", "y", false, i18n.T("flag.tools_yes"))

	end := &cobra.Command{
		Use:   "end <pid | session-id>",
		Short: i18n.T("cmd.agents_end"),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := agentScan()
			if err != nil {
				return err
			}
			s, err := openSession(r, args[0])
			if err != nil {
				return err
			}
			targets := aiagents.SessionTargets(s)
			q := i18n.T("agents.confirm_end", aiagents.NameOf(s.Agent), sessionName(s)+dotted(s.Project), procCount(len(targets)-1))
			if ok, err := confirm(cmd, q); !ok || err != nil {
				return err
			}
			writeEnded(cmd.OutOrStdout(), aiagents.Kill(targets, aiagents.Grace))
			return nil
		},
	}
	end.Flags().BoolVarP(&yes, "yes", "y", false, i18n.T("flag.tools_yes"))

	var after time.Duration
	idle := &cobra.Command{
		Use:   "idle",
		Short: i18n.T("cmd.agents_idle"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if after < aiagents.MinIdle {
				return fmt.Errorf("%s", i18n.T("agents.min_idle", aiagents.MinIdle))
			}
			r, err := agentScan()
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			sessions := r.IdleSessions(after)
			if len(sessions) == 0 {
				fmt.Fprintln(w, ui.NewReport(w).Glyph(ui.MarkOK)+" "+i18n.T("agents.none_idle", ageText(int(after.Seconds()))))
				return nil
			}
			rep := ui.NewReport(w)
			var targets []aiagents.Target
			for _, s := range sessions {
				rep.Check(ui.MarkInfo, aiagents.NameOf(s.Agent)+dotted(s.Project), 0,
					sessionName(s)+" · "+i18n.T("agents.idle_for", ageText(int(s.IdleFor(r.Taken).Seconds()))))
				targets = append(targets, aiagents.SessionTargets(s)...)
			}
			if ok, err := confirm(cmd, i18n.N("agents.confirm_idle", len(sessions))); !ok || err != nil {
				return err
			}
			writeEnded(w, aiagents.Kill(targets, aiagents.Grace))
			return nil
		},
	}
	idle.Flags().DurationVar(&after, "after", 2*time.Hour, i18n.T("flag.agents_after"))
	idle.Flags().BoolVarP(&yes, "yes", "y", false, i18n.T("flag.tools_yes"))

	c.AddCommand(listCmd, kill, end, idle)
	return c
}

// procCount is "1 process" or "N processes", translated.
func procCount(n int) string {
	if n == 1 {
		return i18n.T("agents.one_proc")
	}
	return i18n.N("agents.n_procs", n)
}

// leftoverTargets resolves what to end: every leftover with no arguments,
// else each PID or session ID (or its first eight characters) named.
func leftoverTargets(r *aiagents.Report, args []string) ([]aiagents.Target, string, error) {
	var out []aiagents.Target
	if len(args) == 0 {
		for _, g := range r.Leftovers {
			out = append(out, aiagents.GroupTargets(g)...)
		}
		return out, i18n.N("agents.what_all", len(r.Leftovers), procCount(len(out)), len(r.Leftovers)), nil
	}
	seen := map[int]bool{}
	add := func(p aiagents.Proc) {
		if !seen[p.PID] {
			seen[p.PID] = true
			out = append(out, aiagents.TargetOf(p))
		}
	}
	for _, a := range args {
		found := false
		if pid, ok := aiagents.ParsePID(a); ok {
			for _, g := range r.Leftovers {
				for _, p := range g.Procs {
					if p.PID == pid {
						add(p)
						found = true
					}
				}
			}
			if !found {
				return nil, "", fmt.Errorf("%s", i18n.T("agents.not_leftover", pid))
			}
			continue
		}
		var match []aiagents.Group
		for _, g := range r.Leftovers {
			if len(a) >= 4 && (strings.HasPrefix(g.Session, a) || strings.HasPrefix(g.Key(), a)) {
				match = append(match, g)
			}
		}
		switch len(match) {
		case 0:
			return nil, "", fmt.Errorf("%s", i18n.T("agents.no_closed", a))
		case 1:
			for _, p := range match[0].Procs {
				if !p.Shared {
					add(p)
				}
			}
		default:
			return nil, "", fmt.Errorf("%s", i18n.T("agents.ambiguous", a, len(match)))
		}
	}
	return out, procCount(len(out)), nil
}

// openSession finds an open session by the PID of its agent process or by
// its session ID (or the start of it).
func openSession(r *aiagents.Report, arg string) (aiagents.Session, error) {
	var match []aiagents.Session
	pid, isPID := aiagents.ParsePID(arg)
	for _, s := range r.Sessions {
		if (isPID && s.PID == pid) || (len(arg) >= 4 && s.ID != "" && strings.HasPrefix(s.ID, arg)) {
			match = append(match, s)
		}
	}
	switch {
	case len(match) == 0:
		return aiagents.Session{}, fmt.Errorf("%s", i18n.T("agents.no_open", arg))
	case len(match) > 1:
		return aiagents.Session{}, fmt.Errorf("%s", i18n.T("agents.ambiguous", arg, len(match)))
	case match[0].Current:
		return aiagents.Session{}, fmt.Errorf("%s", i18n.T("agents.is_current", aiagents.NameOf(match[0].Agent)))
	case match[0].Service:
		return aiagents.Session{}, fmt.Errorf("%s", i18n.T("agents.is_service", aiagents.NameOf(match[0].Agent)))
	}
	return match[0], nil
}

func memoryOf(ts []aiagents.Target) uint64 {
	var n uint64
	for _, t := range ts {
		n += t.Memory
	}
	return n
}

func sessionName(s aiagents.Session) string {
	if s.Title != "" {
		return s.Title
	}
	if s.ID != "" {
		return short(s.ID)
	}
	return "pid " + strconv.Itoa(s.PID)
}

var uuidRE = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

// dotted is " · s", or "" when s is empty.
func dotted(s string) string {
	if s == "" {
		return ""
	}
	return " · " + s
}

func short(id string) string {
	if strings.HasPrefix(id, "pid-") {
		return "pid " + id[4:]
	}
	// "sand-subagent-c7779cda-…": the UUID is what tells one from another.
	if m := uuidRE.FindString(id); m != "" {
		return m[:8]
	}
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// ageText is a duration as people say it: 45 s, 12 min, 3 h, 2 d.
func ageText(sec int) string {
	switch {
	case sec < 60:
		return fmt.Sprintf("%d s", sec)
	case sec < 3600:
		return fmt.Sprintf("%d min", sec/60)
	case sec < 48*3600:
		return fmt.Sprintf("%d h", sec/3600)
	}
	return fmt.Sprintf("%d d", sec/86400)
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// groupMemory is what a group holds without its shared daemons: the same
// measure as the header's, so the numbers add up.
func groupMemory(g aiagents.Group) uint64 {
	var n uint64
	for _, p := range g.Procs {
		if !p.Shared {
			n += p.Memory
		}
	}
	return n
}

// procLine is one process of the list: PID, memory, age, command, ports.
func procLine(r *ui.Report, p aiagents.Proc) {
	extra := ""
	if len(p.Ports) > 0 {
		extra = " · :" + strings.Join(p.Ports, " :")
	}
	if len(p.Helpers) > 0 {
		extra += " · " + i18n.N("agents.helpers", len(p.Helpers))
	}
	r.Line(fmt.Sprintf("%-7d %8s  %-6s %s%s", p.PID, ui.Bytes(int64(p.Memory)), ageText(p.Age), r.Clip(p.Short(), 34+len(extra)), extra))
	if p.Cwd != "" {
		r.Detail(i18n.T("agents.in", tildePath(p.Cwd)))
	}
}

// sharedLine folds the identical shared daemons into one line:
// "30 × git fsmonitor--daemon run, 2–5 MB".
type sharedLine struct {
	command  string
	count    int
	min, max uint64
	pids     []int
}

func sharedLines(groups []aiagents.Group) []sharedLine {
	var out []sharedLine
	index := map[string]int{}
	for _, g := range groups {
		for _, p := range g.Procs {
			if !p.Shared {
				continue
			}
			key := p.Short()
			i, ok := index[key]
			if !ok {
				i = len(out)
				index[key] = i
				out = append(out, sharedLine{command: key, min: p.Memory, max: p.Memory})
			}
			l := &out[i]
			l.count++
			l.min, l.max = min(l.min, p.Memory), max(l.max, p.Memory)
			l.pids = append(l.pids, p.PID)
		}
	}
	return out
}

func (l sharedLine) text(r *ui.Report) string {
	size := memRange(l.min, l.max)
	if l.count == 1 {
		return fmt.Sprintf("pid %d · %s, %s", l.pids[0], r.Clip(l.command, 40), size)
	}
	return fmt.Sprintf("%d × %s, %s", l.count, r.Clip(l.command, 40), size)
}

// memRange is "4 MB", "2–5 MB" or "900 KB–2 MB".
func memRange(lo, hi uint64) string {
	a, b := ui.Bytes(int64(lo)), ui.Bytes(int64(hi))
	if a == b {
		return b
	}
	na, ua, _ := strings.Cut(a, " ")
	if _, ub, _ := strings.Cut(b, " "); ua == ub {
		return na + "–" + b
	}
	return a + "–" + b
}

func writeAgents(w io.Writer, r *aiagents.Report) {
	rep := ui.NewReport(w)
	n, kept := r.LeftoverCount(), r.SharedCount()
	rep.Title("omega agents", "")
	if n == 0 {
		rep.Section(i18n.T("agents.left_section"), 0)
		rep.Check(ui.MarkOK, i18n.T("agents.nothing_left"), 0, "")
	} else {
		rep.Section(i18n.T("agents.left_section"), n)
		rep.Line(i18n.T("agents.left_summary", procCount(n), len(r.Leftovers), ui.Bytes(int64(r.LeftoverMemory()))))
		for _, g := range r.Leftovers {
			if groupMemory(g) == 0 && len(aiagents.GroupTargets(g)) == 0 {
				continue // only shared daemons: listed below
			}
			head := aiagents.NameOf(g.Agent) + dotted(g.Project)
			if g.Session != "" {
				head += " · " + short(g.Session)
			}
			if g.Title != "" {
				head += " · " + clip(g.Title, 60)
			}
			rep.Blank()
			rep.Check(ui.MarkWarn, head, 0, ui.Bytes(int64(groupMemory(g))))
			for _, p := range g.Procs {
				if !p.Shared {
					procLine(rep, p)
				}
			}
		}
	}
	if kept > 0 {
		rep.Section(i18n.T("agents.shared_section"), kept)
		for _, l := range sharedLines(r.Leftovers) {
			rep.Check(ui.MarkOff, l.text(rep), 0, "")
		}
		rep.Detail(i18n.T("agents.shared_note"))
	}

	rep.Section(i18n.T("agents.open_section"), len(r.Sessions))
	for i, s := range r.Sessions {
		state := i18n.T("gui.agents.unknown")
		if s.Status == aiagents.Busy {
			state = i18n.T("gui.agents.working")
		}
		if d := s.IdleFor(r.Taken); d >= 0 {
			state = i18n.T("agents.idle_for", ageText(int(d.Seconds())))
		} else if s.Status == aiagents.Busy && !s.Since.IsZero() {
			state = i18n.T("agents.working_for", ageText(int(r.Taken.Sub(s.Since).Seconds())))
		}
		if s.Current {
			state += " · " + i18n.T("agents.this_one")
		}
		if s.Service {
			state += " · " + i18n.T("agents.service")
		}
		head := aiagents.NameOf(s.Agent) + dotted(s.Project)
		if s.ID != "" {
			head += " · " + short(s.ID)
		}
		mark := ui.MarkInfo
		if s.Current {
			mark = ui.MarkCurrent
		}
		if i > 0 {
			rep.Blank()
		}
		rep.Check(mark, fmt.Sprintf("%s · pid %d", head, s.PID), 0, state+" · "+ui.Bytes(int64(s.Total())))
		if s.Title != "" {
			rep.Detail(clip(s.Title, 90))
		}
		for _, p := range s.Procs {
			procLine(rep, p)
		}
	}
	if n > 0 {
		rep.Blank()
		rep.Hint(i18n.T("agents.kill_hint"))
	}
}

func writeEnded(w io.Writer, res aiagents.Result) {
	r := ui.NewReport(w)
	mark := ui.MarkOK
	if len(res.Failed) > 0 {
		mark = ui.MarkWarn
	}
	r.Check(mark, i18n.T("agents.ended", procCount(res.Ended), ui.Bytes(int64(res.Freed))), 0, "")
	if res.Forced > 0 {
		r.Detail(i18n.T("agents.forced", res.Forced))
	}
	if res.Skipped > 0 {
		r.Detail(i18n.T("agents.skipped", res.Skipped))
	}
	if len(res.Failed) > 0 {
		var pids []string
		for _, p := range res.Failed {
			pids = append(pids, strconv.Itoa(p))
		}
		r.Check(ui.MarkErr, i18n.T("agents.still_running", strings.Join(pids, ", ")), 0, "")
	}
}

// ---- the desktop app ----

// AgentProcRow is a process as the app shows it.
type AgentProcRow struct {
	PID     int      `json:"pid"`
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Cwd     string   `json:"cwd"`
	Ports   []string `json:"ports"`
	Memory  string   `json:"memory"`
	Age     string   `json:"age"`
	Helpers int      `json:"helpers"`
	Shared  bool     `json:"shared"`
}

// AgentGroupRow is what one closed session left.
type AgentGroupRow struct {
	ID      string         `json:"id"` // the agent and the session
	Agent   string         `json:"agent"`
	Short   string         `json:"short"`
	Project string         `json:"project"`
	Title   string         `json:"title"`
	Memory  string         `json:"memory"`
	Age     string         `json:"age"`
	Procs   []AgentProcRow `json:"procs"`
}

// AgentSessionRow is an open session.
type AgentSessionRow struct {
	PID     int            `json:"pid"`
	Agent   string         `json:"agent"`
	ID      string         `json:"id"`
	Project string         `json:"project"`
	Title   string         `json:"title"`
	Status  string         `json:"status"` // busy, idle or unknown
	State   string         `json:"state"`  // translated, with how long
	IdleMin int            `json:"idleMin"`
	Memory  string         `json:"memory"`
	Age     string         `json:"age"`
	Current bool           `json:"current"`
	Service bool           `json:"service"`
	Procs   []AgentProcRow `json:"procs"`
}

// AgentsView is the app's AI agents screen.
type AgentsView struct {
	Supported bool              `json:"supported"`
	Known     string            `json:"known"` // the agents omega knows
	Count     int               `json:"count"`
	Shared    int               `json:"shared"` // shared daemons, kept by "end all"
	Memory    string            `json:"memory"`
	Leftovers []AgentGroupRow   `json:"leftovers"`
	Sessions  []AgentSessionRow `json:"sessions"`
}

func procRows(ps []aiagents.Proc) []AgentProcRow {
	out := []AgentProcRow{}
	for _, p := range ps {
		out = append(out, AgentProcRow{PID: p.PID, Name: p.Name(), Command: p.Short(), Cwd: p.Cwd, Ports: p.Ports,
			Memory: ui.Bytes(int64(p.Memory)), Age: ageText(p.Age), Helpers: len(p.Helpers), Shared: p.Shared})
	}
	return out
}

// AgentList looks at what the agents left running and what is open.
func AgentList() (AgentsView, error) {
	v := AgentsView{Supported: true, Known: knownAgents(), Leftovers: []AgentGroupRow{}, Sessions: []AgentSessionRow{}}
	r, err := agentScan()
	if errors.Is(err, aiagents.ErrUnsupported) {
		v.Supported = false
		return v, nil
	}
	if err != nil {
		return v, err
	}
	v.Count, v.Shared, v.Memory = r.LeftoverCount(), r.SharedCount(), ui.Bytes(int64(r.LeftoverMemory()))
	for _, g := range r.Leftovers {
		v.Leftovers = append(v.Leftovers, AgentGroupRow{ID: g.Key(), Agent: aiagents.NameOf(g.Agent), Short: short(g.Session), Project: g.Project, Title: g.Title,
			Memory: ui.Bytes(int64(groupMemory(g))), Age: ageText(g.Oldest()), Procs: procRows(g.Procs)})
	}
	for _, s := range r.Sessions {
		row := AgentSessionRow{PID: s.PID, Agent: aiagents.NameOf(s.Agent), ID: s.ID, Project: s.Project, Title: s.Title, Status: s.Status,
			Memory: ui.Bytes(int64(s.Total())), Age: ageText(s.Age), Current: s.Current, Service: s.Service, Procs: procRows(s.Procs)}
		switch d := s.IdleFor(r.Taken); {
		case d >= 0:
			row.IdleMin = int(d.Minutes())
			row.State = i18n.T("gui.agents.idle_for", ageText(int(d.Seconds())))
		case s.Status == aiagents.Busy:
			row.State = i18n.T("gui.agents.working")
		default:
			row.State = i18n.T("gui.agents.unknown")
		}
		v.Sessions = append(v.Sessions, row)
	}
	return v, nil
}

// AgentEnd ends what the app asks, looked up again in a fresh scan so the
// page can only end what is there now: "proc" (id is a PID left behind),
// "group" (id is a closed session), "all" (every leftover), "session" (id
// is the PID of an open session) or "idle" (id is a number of minutes, at
// least 30). It returns what was done, translated.
func AgentEnd(what, id string) (string, error) {
	r, err := agentScan()
	if err != nil {
		return "", err
	}
	var targets []aiagents.Target
	switch what {
	case "proc", "all":
		var args []string
		if what == "proc" {
			if _, ok := aiagents.ParsePID(id); !ok {
				return "", errors.New("nothing to end")
			}
			args = []string{id}
		}
		targets, _, err = leftoverTargets(r, args)
	case "group":
		for _, g := range r.Leftovers {
			if g.Key() == id {
				targets = append(targets, aiagents.GroupTargets(g)...)
			}
		}
		if len(targets) == 0 {
			return "", errors.New("nothing to end")
		}
	case "session":
		pid, ok := aiagents.ParsePID(id)
		if !ok {
			return "", errors.New("nothing to end")
		}
		var s aiagents.Session
		if s, err = openSession(r, strconv.Itoa(pid)); err == nil {
			targets = aiagents.SessionTargets(s)
		}
	case "idle":
		min, convErr := strconv.Atoi(id)
		if convErr != nil || time.Duration(min)*time.Minute < aiagents.MinIdle {
			return "", fmt.Errorf("%s", i18n.T("gui.agents.min_idle"))
		}
		for _, s := range r.IdleSessions(time.Duration(min) * time.Minute) {
			targets = append(targets, aiagents.SessionTargets(s)...)
		}
	default:
		return "", errors.New("unknown action: " + what)
	}
	if err != nil {
		return "", err
	}
	if len(targets) == 0 {
		return i18n.T("gui.agents.nothing"), nil
	}
	res := aiagents.Kill(targets, aiagents.Grace)
	msg := i18n.T("gui.agents.ended", res.Ended, ui.Bytes(int64(res.Freed)))
	if len(res.Failed) > 0 {
		return msg, fmt.Errorf("%s", i18n.T("gui.agents.failed", len(res.Failed)))
	}
	return msg, nil
}

// knownAgents lists the agents of the catalogue.
func knownAgents() string {
	var names []string
	for _, a := range aiagents.Catalogue {
		names = append(names, a.Name)
	}
	return strings.Join(names, ", ")
}
