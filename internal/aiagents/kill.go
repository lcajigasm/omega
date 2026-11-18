package aiagents

import (
	"strconv"
	"strings"
	"time"
)

// Target is a process to end, with the command line it was seen running:
// PIDs get reused, and a PID that now runs something else is left alone.
type Target struct {
	PID     int      `json:"pid"`
	Command string   `json:"command"`
	Memory  uint64   `json:"memory"`
	Helpers []Helper `json:"helpers,omitempty"`
}

// TargetOf is the target for a row of the report.
func TargetOf(p Proc) Target {
	return Target{PID: p.PID, Command: p.Command, Memory: p.Memory, Helpers: p.Helpers}
}

// SessionTargets are the agent process of an open session and its
// processes, without the shared daemons.
func SessionTargets(s Session) []Target {
	out := []Target{{PID: s.PID, Command: s.Command, Memory: s.Memory}}
	for _, p := range s.Procs {
		if !p.Shared {
			out = append(out, TargetOf(p))
		}
	}
	return out
}

// GroupTargets are what a closed session left, without the shared daemons.
func GroupTargets(g Group) []Target {
	var out []Target
	for _, p := range g.Procs {
		if !p.Shared {
			out = append(out, TargetOf(p))
		}
	}
	return out
}

// Result says what an end did.
type Result struct {
	Ended   int    `json:"ended"`   // processes that are gone
	Forced  int    `json:"forced"`  // of them, the ones that ignored the request to exit
	Skipped int    `json:"skipped"` // gone already, or now running something else
	Failed  []int  `json:"failed"`  // still running
	Freed   uint64 `json:"freed"`   // bytes the ended ones held
}

// Grace is how long a process gets to exit before it is forced to.
const Grace = 2 * time.Second

// Kill asks each process to exit, then forces whatever ignored the request
// after grace. An app's helpers exit with it; any that did not are forced.
func Kill(targets []Target, grace time.Duration) Result {
	res := Result{Failed: []int{}}
	var live []Target
	for _, t := range targets {
		if t.PID <= 1 || t.PID == selfPID() || !sameCommand(commandOf(t.PID), t.Command) {
			res.Skipped++
			continue
		}
		live = append(live, t)
	}
	for _, t := range live {
		terminate(t.PID)
	}
	running := func(ts []Target) []Target {
		var out []Target
		for _, t := range ts {
			if alive(t.PID) {
				out = append(out, t)
			}
		}
		return out
	}
	wait(func() bool { return len(running(live)) == 0 }, grace)
	stubborn := running(live)
	for _, t := range stubborn {
		forceKill(t.PID)
	}
	wait(func() bool { return len(running(stubborn)) == 0 }, time.Second)

	var helpers []Helper
	for _, t := range live {
		if alive(t.PID) {
			res.Failed = append(res.Failed, t.PID)
			continue
		}
		res.Ended++
		res.Freed += t.Memory
		helpers = append(helpers, t.Helpers...)
	}
	for _, t := range stubborn {
		if !alive(t.PID) {
			res.Forced++
		}
	}
	if len(helpers) > 0 {
		wait(func() bool {
			for _, h := range helpers {
				if alive(h.PID) {
					return false
				}
			}
			return true
		}, time.Second)
		for _, h := range helpers {
			if alive(h.PID) && sameCommand(commandOf(h.PID), h.Command) {
				forceKill(h.PID)
			}
		}
	}
	return res
}

func wait(done func() bool, d time.Duration) {
	deadline := time.Now().Add(d)
	for !done() && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
}

// sameCommand compares command lines as ps and /proc print them, which
// may differ in trailing spaces.
func sameCommand(now, seen string) bool {
	now, seen = strings.TrimSpace(now), strings.TrimSpace(seen)
	return now != "" && now == seen
}

// ParsePID reads a PID typed by a person.
func ParsePID(s string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	return n, err == nil && n > 1
}
