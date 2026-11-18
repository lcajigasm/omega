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
		Short:   "Find and end what AI coding agents left running",
		Long: "Lists the processes that closed sessions of AI coding agents left behind (dev servers, watchers,\n" +
			"emulators, browsers opened by Playwright), grouped by agent and session with their ports, memory\n" +
			"and age, and the agent sessions still open and what they hold.\n\n" +
			"  omega agents                 what is left behind and what is open\n" +
			"  omega agents kill            end everything closed sessions left behind\n" +
			"  omega agents kill <pid|id>   end one process, or what one session left\n" +
			"  omega agents end <pid|id>    end an open session and what it runs\n" +
			"  omega agents idle            end the sessions idle for more than two hours (--after)\n\n" +
			"Known agents: " + knownAgents() + ".\n" +
			"What an agent leaves behind is found by the variable it puts in the environment of the commands it\n" +
			"runs; only Claude Code records whether a session is idle. Processes get two seconds to exit before\n" +
			"they are forced, and a PID that now runs something other than what was listed is left alone.\n" +
			"Nothing leaves your machine.",
		Args: cobra.NoArgs,
		RunE: list,
	}
	c.Flags().BoolVar(&asJSON, "json", false, "print the report as JSON")
	listCmd := &cobra.Command{Use: "list", Short: "List what closed agent sessions left running and the open sessions", Args: cobra.NoArgs, RunE: list}
	listCmd.Flags().BoolVar(&asJSON, "json", false, "print the report as JSON")

	var yes bool
	confirm := func(cmd *cobra.Command, q string) (bool, error) {
		if yes {
			return true, nil
		}
		if !stdinIsTerminal() {
			return false, errors.New("nothing ended: pass --yes to end processes without a terminal")
		}
		return askYesNo(q), nil
	}

	kill := &cobra.Command{
		Use:   "kill [pid | session-id]...",
		Short: "End what closed sessions left running: everything, one process or one session's worth",
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
				fmt.Fprintln(w, "Nothing left behind: every process an agent started belongs to an open session.")
				return nil
			}
			if ok, err := confirm(cmd, fmt.Sprintf("End %s (%s)?", what, humanBytes(float64(memoryOf(targets))))); !ok || err != nil {
				return err
			}
			writeEnded(w, aiagents.Kill(targets, aiagents.Grace))
			return nil
		},
	}
	kill.Flags().BoolVarP(&yes, "yes", "y", false, "do not ask")

	end := &cobra.Command{
		Use:   "end <pid | session-id>",
		Short: "End an open agent session and what it runs",
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
			q := fmt.Sprintf("End the %s session %q%s and %s?", aiagents.NameOf(s.Agent), sessionName(s), dotted(s.Project), plural(len(targets)-1, "process", "processes"))
			if ok, err := confirm(cmd, q); !ok || err != nil {
				return err
			}
			writeEnded(cmd.OutOrStdout(), aiagents.Kill(targets, aiagents.Grace))
			return nil
		},
	}
	end.Flags().BoolVarP(&yes, "yes", "y", false, "do not ask")

	var after time.Duration
	idle := &cobra.Command{
		Use:   "idle",
		Short: "End the open sessions that have waited for input longer than --after",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if after < aiagents.MinIdle {
				return fmt.Errorf("--after must be at least %s: a shorter wait ends sessions that only paused", aiagents.MinIdle)
			}
			r, err := agentScan()
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			sessions := r.IdleSessions(after)
			if len(sessions) == 0 {
				fmt.Fprintf(w, "No session has been idle for %s.\n", ageText(int(after.Seconds())))
				return nil
			}
			var targets []aiagents.Target
			for _, s := range sessions {
				fmt.Fprintf(w, "  %s%s · %s · idle %s\n", aiagents.NameOf(s.Agent), dotted(s.Project), sessionName(s), ageText(int(s.IdleFor(r.Taken).Seconds())))
				targets = append(targets, aiagents.SessionTargets(s)...)
			}
			if ok, err := confirm(cmd, fmt.Sprintf("End %s?", plural(len(sessions), "idle session", "idle sessions"))); !ok || err != nil {
				return err
			}
			writeEnded(w, aiagents.Kill(targets, aiagents.Grace))
			return nil
		},
	}
	idle.Flags().DurationVar(&after, "after", 2*time.Hour, "how long a session must have waited (at least 30m)")
	idle.Flags().BoolVarP(&yes, "yes", "y", false, "do not ask")

	c.AddCommand(listCmd, kill, end, idle)
	return c
}

// leftoverTargets resolves what to end: every leftover with no arguments,
// else each PID or session ID (or its first eight characters) named.
func leftoverTargets(r *aiagents.Report, args []string) ([]aiagents.Target, string, error) {
	var out []aiagents.Target
	if len(args) == 0 {
		for _, g := range r.Leftovers {
			out = append(out, aiagents.GroupTargets(g)...)
		}
		return out, fmt.Sprintf("%s from %s", plural(len(out), "process", "processes"),
			plural(len(r.Leftovers), "closed session", "closed sessions")), nil
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
				return nil, "", fmt.Errorf("%d is not a process a closed session left behind (see omega agents)", pid)
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
			return nil, "", fmt.Errorf("no closed session %q left anything behind (see omega agents)", a)
		case 1:
			for _, p := range match[0].Procs {
				if !p.Shared {
					add(p)
				}
			}
		default:
			return nil, "", fmt.Errorf("%q matches %d sessions: give more of the ID", a, len(match))
		}
	}
	return out, plural(len(out), "process", "processes"), nil
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
		return aiagents.Session{}, fmt.Errorf("no open agent session is %q (see omega agents)", arg)
	case len(match) > 1:
		return aiagents.Session{}, fmt.Errorf("%q matches %d sessions: give more of the ID", arg, len(match))
	case match[0].Current:
		return aiagents.Session{}, fmt.Errorf("that is the session omega runs in: end it from %s", aiagents.NameOf(match[0].Agent))
	case match[0].Service:
		return aiagents.Session{}, fmt.Errorf("that is %s's background service, which other sessions use: stop it from %s", aiagents.NameOf(match[0].Agent), aiagents.NameOf(match[0].Agent))
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

func writeProc(w io.Writer, p aiagents.Proc, indent string) {
	extra := ""
	if len(p.Ports) > 0 {
		extra = " · :" + strings.Join(p.Ports, " :")
	}
	if len(p.Helpers) > 0 {
		extra += " · +" + plural(len(p.Helpers), "helper", "helpers")
	}
	if p.Shared {
		extra += " · shared daemon: kept unless named"
	}
	fmt.Fprintf(w, "%s%-7d %-9s %-6s %s%s\n", indent, p.PID, humanBytes(float64(p.Memory)), ageText(p.Age), clip(p.Short(), 70), extra)
	if p.Cwd != "" {
		fmt.Fprintf(w, "%s        in %s\n", indent, p.Cwd)
	}
}

func writeAgents(w io.Writer, r *aiagents.Report) {
	n, kept := r.LeftoverCount(), r.SharedCount()
	if n+kept == 0 {
		fmt.Fprintln(w, "\nLeft behind: nothing. Every process an agent started belongs to an open session.")
	} else {
		also := ""
		if kept > 0 {
			also = fmt.Sprintf(", and %s that stay unless named", plural(kept, "shared daemon", "shared daemons"))
		}
		fmt.Fprintf(w, "\nLeft behind: %s from %s, holding %s%s\n", plural(n, "process", "processes"),
			plural(len(r.Leftovers), "closed session", "closed sessions"), humanBytes(float64(r.LeftoverMemory())), also)
		for _, g := range r.Leftovers {
			title := ""
			if g.Title != "" {
				title = " · " + clip(g.Title, 60)
			}
			id := ""
			if g.Session != "" {
				id = " · " + short(g.Session)
			}
			fmt.Fprintf(w, "\n  %s%s%s%s · %s\n", aiagents.NameOf(g.Agent), dotted(g.Project), id, title, humanBytes(float64(g.Memory())))
			for _, p := range g.Procs {
				writeProc(w, p, "    ")
			}
		}
		fmt.Fprintln(w, "\n  `omega agents kill` ends them all; give a PID or a session ID to end less.")
	}

	fmt.Fprintf(w, "\nOpen sessions (%d)\n", len(r.Sessions))
	for _, s := range r.Sessions {
		state := s.Status
		if d := s.IdleFor(r.Taken); d >= 0 {
			state = "idle " + ageText(int(d.Seconds()))
		} else if s.Status == aiagents.Busy && !s.Since.IsZero() {
			state = "working " + ageText(int(r.Taken.Sub(s.Since).Seconds()))
		}
		if s.Current {
			state += " · this one"
		}
		if s.Service {
			state += " · background service"
		}
		id := ""
		if s.ID != "" {
			id = " · " + short(s.ID)
		}
		fmt.Fprintf(w, "\n  %s%s%s · pid %d · %s · %s\n", aiagents.NameOf(s.Agent), dotted(s.Project), id, s.PID, state, humanBytes(float64(s.Total())))
		if s.Title != "" {
			fmt.Fprintf(w, "    %s\n", clip(s.Title, 90))
		}
		for _, p := range s.Procs {
			writeProc(w, p, "    ")
		}
	}
	fmt.Fprintln(w)
}

func writeEnded(w io.Writer, res aiagents.Result) {
	fmt.Fprintf(w, "Ended %s, freed %s.", plural(res.Ended, "process", "processes"), humanBytes(float64(res.Freed)))
	if res.Forced > 0 {
		fmt.Fprintf(w, " %d had to be forced.", res.Forced)
	}
	if res.Skipped > 0 {
		fmt.Fprintf(w, " %d had already exited or now run something else, and were left alone.", res.Skipped)
	}
	fmt.Fprintln(w)
	if len(res.Failed) > 0 {
		var pids []string
		for _, p := range res.Failed {
			pids = append(pids, strconv.Itoa(p))
		}
		fmt.Fprintf(w, "Still running: %s.\n", strings.Join(pids, ", "))
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
			Memory: humanBytes(float64(p.Memory)), Age: ageText(p.Age), Helpers: len(p.Helpers), Shared: p.Shared})
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
	v.Count, v.Shared, v.Memory = r.LeftoverCount(), r.SharedCount(), humanBytes(float64(r.LeftoverMemory()))
	for _, g := range r.Leftovers {
		v.Leftovers = append(v.Leftovers, AgentGroupRow{ID: g.Key(), Agent: aiagents.NameOf(g.Agent), Short: short(g.Session), Project: g.Project, Title: g.Title,
			Memory: humanBytes(float64(g.Memory())), Age: ageText(g.Oldest()), Procs: procRows(g.Procs)})
	}
	for _, s := range r.Sessions {
		row := AgentSessionRow{PID: s.PID, Agent: aiagents.NameOf(s.Agent), ID: s.ID, Project: s.Project, Title: s.Title, Status: s.Status,
			Memory: humanBytes(float64(s.Total())), Age: ageText(s.Age), Current: s.Current, Service: s.Service, Procs: procRows(s.Procs)}
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
	msg := i18n.T("gui.agents.ended", res.Ended, humanBytes(float64(res.Freed)))
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
