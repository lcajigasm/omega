package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/claudecode"
	"github.com/lcajigasm/omega/internal/i18n"
)

// What Claude Code sessions leave running: the dev servers, watchers,
// emulators and browsers of sessions that have closed, and the sessions
// still open with what they hold. The desktop app shows the same through
// the functions at the end of the file.

var claudeScan = claudecode.Scan

func claudeCmd() *cobra.Command {
	var asJSON bool
	list := func(cmd *cobra.Command, _ []string) error {
		r, err := claudeScan()
		if err != nil {
			return err
		}
		if asJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(r)
		}
		writeClaude(cmd.OutOrStdout(), r)
		return nil
	}
	c := &cobra.Command{
		Use:   "claude",
		Short: "Find and end what Claude Code sessions left running",
		Long: "Lists the processes that closed Claude Code sessions left behind (dev servers, watchers,\n" +
			"emulators, browsers opened by Playwright), grouped by session with their ports, memory and age,\n" +
			"and the sessions still open: working or idle, for how long, and what they hold.\n\n" +
			"  omega claude                 what is left behind and what is open\n" +
			"  omega claude kill            end everything closed sessions left behind\n" +
			"  omega claude kill <pid|id>   end one process, or what one session left\n" +
			"  omega claude end <pid|id>    end an open session and what it runs\n" +
			"  omega claude idle            end the sessions idle for more than two hours (--after)\n\n" +
			"Processes get two seconds to exit before they are forced. A PID that now runs something\n" +
			"other than what was listed is left alone. Nothing leaves your machine.",
		Args: cobra.NoArgs,
		RunE: list,
	}
	c.Flags().BoolVar(&asJSON, "json", false, "print the report as JSON")
	listCmd := &cobra.Command{Use: "list", Short: "List what closed sessions left running and the open sessions", Args: cobra.NoArgs, RunE: list}
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
			r, err := claudeScan()
			if err != nil {
				return err
			}
			targets, what, err := leftoverTargets(r, args)
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			if len(targets) == 0 {
				fmt.Fprintln(w, "Nothing left behind: every process Claude Code started belongs to an open session.")
				return nil
			}
			if ok, err := confirm(cmd, fmt.Sprintf("End %s (%s)?", what, humanBytes(float64(memoryOf(targets))))); !ok || err != nil {
				return err
			}
			writeEnded(w, claudecode.Kill(targets, claudecode.Grace))
			return nil
		},
	}
	kill.Flags().BoolVarP(&yes, "yes", "y", false, "do not ask")

	end := &cobra.Command{
		Use:   "end <pid | session-id>",
		Short: "End an open Claude Code session and what it runs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := claudeScan()
			if err != nil {
				return err
			}
			s, err := openSession(r, args[0])
			if err != nil {
				return err
			}
			targets := claudecode.SessionTargets(s)
			q := fmt.Sprintf("End the session %q in %s and %s?", sessionName(s), s.Project, plural(len(targets)-1, "process", "processes"))
			if ok, err := confirm(cmd, q); !ok || err != nil {
				return err
			}
			writeEnded(cmd.OutOrStdout(), claudecode.Kill(targets, claudecode.Grace))
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
			if after < claudecode.MinIdle {
				return fmt.Errorf("--after must be at least %s: a shorter wait ends sessions that only paused", claudecode.MinIdle)
			}
			r, err := claudeScan()
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			sessions := r.IdleSessions(after)
			if len(sessions) == 0 {
				fmt.Fprintf(w, "No session has been idle for %s.\n", ageText(int(after.Seconds())))
				return nil
			}
			var targets []claudecode.Target
			for _, s := range sessions {
				fmt.Fprintf(w, "  %s · %s · idle %s\n", s.Project, sessionName(s), ageText(int(s.IdleFor(r.Taken).Seconds())))
				targets = append(targets, claudecode.SessionTargets(s)...)
			}
			if ok, err := confirm(cmd, fmt.Sprintf("End %s?", plural(len(sessions), "idle session", "idle sessions"))); !ok || err != nil {
				return err
			}
			writeEnded(w, claudecode.Kill(targets, claudecode.Grace))
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
func leftoverTargets(r *claudecode.Report, args []string) ([]claudecode.Target, string, error) {
	var out []claudecode.Target
	if len(args) == 0 {
		for _, g := range r.Leftovers {
			for _, p := range g.Procs {
				out = append(out, claudecode.TargetOf(p))
			}
		}
		return out, fmt.Sprintf("%s from %s", plural(len(out), "process", "processes"),
			plural(len(r.Leftovers), "closed session", "closed sessions")), nil
	}
	seen := map[int]bool{}
	add := func(p claudecode.Proc) {
		if !seen[p.PID] {
			seen[p.PID] = true
			out = append(out, claudecode.TargetOf(p))
		}
	}
	for _, a := range args {
		found := false
		if pid, ok := claudecode.ParsePID(a); ok {
			for _, g := range r.Leftovers {
				for _, p := range g.Procs {
					if p.PID == pid {
						add(p)
						found = true
					}
				}
			}
			if !found {
				return nil, "", fmt.Errorf("%d is not a process a closed session left behind (see omega claude)", pid)
			}
			continue
		}
		var match []claudecode.Group
		for _, g := range r.Leftovers {
			if len(a) >= 4 && strings.HasPrefix(g.Session, a) {
				match = append(match, g)
			}
		}
		switch len(match) {
		case 0:
			return nil, "", fmt.Errorf("no closed session %q left anything behind (see omega claude)", a)
		case 1:
			for _, p := range match[0].Procs {
				add(p)
			}
		default:
			return nil, "", fmt.Errorf("%q matches %d sessions: give more of the ID", a, len(match))
		}
	}
	return out, plural(len(out), "process", "processes"), nil
}

// openSession finds an open session by the PID of its claude process or by
// its session ID (or the start of it).
func openSession(r *claudecode.Report, arg string) (claudecode.Session, error) {
	var match []claudecode.Session
	pid, isPID := claudecode.ParsePID(arg)
	for _, s := range r.Sessions {
		if (isPID && s.PID == pid) || (len(arg) >= 4 && s.ID != "" && strings.HasPrefix(s.ID, arg)) {
			match = append(match, s)
		}
	}
	switch {
	case len(match) == 0:
		return claudecode.Session{}, fmt.Errorf("no open Claude Code session is %q (see omega claude)", arg)
	case len(match) > 1:
		return claudecode.Session{}, fmt.Errorf("%q matches %d sessions: give more of the ID", arg, len(match))
	case match[0].Current:
		return claudecode.Session{}, errors.New("that is the session omega runs in: end it from Claude Code")
	}
	return match[0], nil
}

func memoryOf(ts []claudecode.Target) uint64 {
	var n uint64
	for _, t := range ts {
		n += t.Memory
	}
	return n
}

func sessionName(s claudecode.Session) string {
	if s.Title != "" {
		return s.Title
	}
	if s.ID != "" {
		return short(s.ID)
	}
	return "pid " + strconv.Itoa(s.PID)
}

func short(id string) string {
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

func writeProc(w io.Writer, p claudecode.Proc, indent string) {
	extra := ""
	if len(p.Ports) > 0 {
		extra = " · :" + strings.Join(p.Ports, " :")
	}
	if len(p.Helpers) > 0 {
		extra += " · +" + plural(len(p.Helpers), "helper", "helpers")
	}
	fmt.Fprintf(w, "%s%-7d %-9s %-6s %s%s\n", indent, p.PID, humanBytes(float64(p.Memory)), ageText(p.Age), clip(p.Short(), 70), extra)
	if p.Cwd != "" {
		fmt.Fprintf(w, "%s        in %s\n", indent, p.Cwd)
	}
}

func writeClaude(w io.Writer, r *claudecode.Report) {
	n := r.LeftoverCount()
	if n == 0 {
		fmt.Fprintln(w, "\nLeft behind: nothing. Every process Claude Code started belongs to an open session.")
	} else {
		fmt.Fprintf(w, "\nLeft behind: %s from %s, holding %s\n", plural(n, "process", "processes"),
			plural(len(r.Leftovers), "closed session", "closed sessions"), humanBytes(float64(r.LeftoverMemory())))
		for _, g := range r.Leftovers {
			title := ""
			if g.Title != "" {
				title = " · " + clip(g.Title, 60)
			}
			fmt.Fprintf(w, "\n  %s · %s%s · %s\n", g.Project, short(g.Session), title, humanBytes(float64(g.Memory())))
			for _, p := range g.Procs {
				writeProc(w, p, "    ")
			}
		}
		fmt.Fprintln(w, "\n  `omega claude kill` ends them all; give a PID or a session ID to end less.")
	}

	fmt.Fprintf(w, "\nOpen sessions (%d)\n", len(r.Sessions))
	for _, s := range r.Sessions {
		state := s.Status
		if d := s.IdleFor(r.Taken); d >= 0 {
			state = "idle " + ageText(int(d.Seconds()))
		} else if s.Status == claudecode.Busy && !s.Since.IsZero() {
			state = "working " + ageText(int(r.Taken.Sub(s.Since).Seconds()))
		}
		if s.Current {
			state += " · this one"
		}
		id := ""
		if s.ID != "" {
			id = " · " + short(s.ID)
		}
		fmt.Fprintf(w, "\n  %s%s · pid %d · %s · %s\n", s.Project, id, s.PID, state, humanBytes(float64(s.Total())))
		if s.Title != "" {
			fmt.Fprintf(w, "    %s\n", clip(s.Title, 90))
		}
		for _, p := range s.Procs {
			writeProc(w, p, "    ")
		}
	}
	fmt.Fprintln(w)
}

func writeEnded(w io.Writer, res claudecode.Result) {
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

// ClaudeProcRow is a process as the app shows it.
type ClaudeProcRow struct {
	PID     int      `json:"pid"`
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Cwd     string   `json:"cwd"`
	Ports   []string `json:"ports"`
	Memory  string   `json:"memory"`
	Age     string   `json:"age"`
	Helpers int      `json:"helpers"`
}

// ClaudeGroupRow is what one closed session left.
type ClaudeGroupRow struct {
	ID      string          `json:"id"`
	Short   string          `json:"short"`
	Project string          `json:"project"`
	Title   string          `json:"title"`
	Memory  string          `json:"memory"`
	Age     string          `json:"age"`
	Procs   []ClaudeProcRow `json:"procs"`
}

// ClaudeSessionRow is an open session.
type ClaudeSessionRow struct {
	PID     int             `json:"pid"`
	ID      string          `json:"id"`
	Project string          `json:"project"`
	Title   string          `json:"title"`
	Status  string          `json:"status"` // busy, idle or unknown
	State   string          `json:"state"`  // translated, with how long
	IdleMin int             `json:"idleMin"`
	Memory  string          `json:"memory"`
	Age     string          `json:"age"`
	Current bool            `json:"current"`
	Procs   []ClaudeProcRow `json:"procs"`
}

// ClaudeView is the app's Claude Code screen.
type ClaudeView struct {
	Supported bool               `json:"supported"`
	Count     int                `json:"count"`
	Memory    string             `json:"memory"`
	Leftovers []ClaudeGroupRow   `json:"leftovers"`
	Sessions  []ClaudeSessionRow `json:"sessions"`
}

func procRows(ps []claudecode.Proc) []ClaudeProcRow {
	out := []ClaudeProcRow{}
	for _, p := range ps {
		out = append(out, ClaudeProcRow{PID: p.PID, Name: p.Name(), Command: p.Short(), Cwd: p.Cwd, Ports: p.Ports,
			Memory: humanBytes(float64(p.Memory)), Age: ageText(p.Age), Helpers: len(p.Helpers)})
	}
	return out
}

// ClaudeList looks at what Claude Code left running and what is open.
func ClaudeList() (ClaudeView, error) {
	v := ClaudeView{Supported: true, Leftovers: []ClaudeGroupRow{}, Sessions: []ClaudeSessionRow{}}
	r, err := claudeScan()
	if errors.Is(err, claudecode.ErrUnsupported) {
		v.Supported = false
		return v, nil
	}
	if err != nil {
		return v, err
	}
	v.Count, v.Memory = r.LeftoverCount(), humanBytes(float64(r.LeftoverMemory()))
	for _, g := range r.Leftovers {
		v.Leftovers = append(v.Leftovers, ClaudeGroupRow{ID: g.Session, Short: short(g.Session), Project: g.Project, Title: g.Title,
			Memory: humanBytes(float64(g.Memory())), Age: ageText(g.Oldest()), Procs: procRows(g.Procs)})
	}
	for _, s := range r.Sessions {
		row := ClaudeSessionRow{PID: s.PID, ID: s.ID, Project: s.Project, Title: s.Title, Status: s.Status,
			Memory: humanBytes(float64(s.Total())), Age: ageText(s.Age), Current: s.Current, Procs: procRows(s.Procs)}
		switch d := s.IdleFor(r.Taken); {
		case d >= 0:
			row.IdleMin = int(d.Minutes())
			row.State = i18n.T("gui.claude.idle_for", ageText(int(d.Seconds())))
		case s.Status == claudecode.Busy:
			row.State = i18n.T("gui.claude.working")
		default:
			row.State = i18n.T("gui.claude.unknown")
		}
		v.Sessions = append(v.Sessions, row)
	}
	return v, nil
}

// ClaudeEnd ends what the app asks, looked up again in a fresh scan so the
// page can only end what is there now: "proc" (id is a PID left behind),
// "group" (id is a closed session), "all" (every leftover), "session" (id
// is the PID of an open session) or "idle" (id is a number of minutes, at
// least 30). It returns what was done, translated.
func ClaudeEnd(what, id string) (string, error) {
	r, err := claudeScan()
	if err != nil {
		return "", err
	}
	var targets []claudecode.Target
	switch what {
	case "proc", "group", "all":
		var args []string
		if what != "all" {
			if id == "" || (what == "group" && len(id) < 8) {
				return "", errors.New("nothing to end")
			}
			args = []string{id}
		}
		if what == "proc" {
			if _, ok := claudecode.ParsePID(id); !ok {
				return "", errors.New("nothing to end")
			}
		}
		targets, _, err = leftoverTargets(r, args)
	case "session":
		pid, ok := claudecode.ParsePID(id)
		if !ok {
			return "", errors.New("nothing to end")
		}
		var s claudecode.Session
		if s, err = openSession(r, strconv.Itoa(pid)); err == nil {
			targets = claudecode.SessionTargets(s)
		}
	case "idle":
		min, convErr := strconv.Atoi(id)
		if convErr != nil || time.Duration(min)*time.Minute < claudecode.MinIdle {
			return "", fmt.Errorf("%s", i18n.T("gui.claude.min_idle"))
		}
		for _, s := range r.IdleSessions(time.Duration(min) * time.Minute) {
			targets = append(targets, claudecode.SessionTargets(s)...)
		}
	default:
		return "", errors.New("unknown action: " + what)
	}
	if err != nil {
		return "", err
	}
	if len(targets) == 0 {
		return i18n.T("gui.claude.nothing"), nil
	}
	res := claudecode.Kill(targets, claudecode.Grace)
	msg := i18n.T("gui.claude.ended", res.Ended, humanBytes(float64(res.Freed)))
	if len(res.Failed) > 0 {
		return msg, fmt.Errorf("%s", i18n.T("gui.claude.failed", len(res.Failed)))
	}
	return msg, nil
}
