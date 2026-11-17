package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/claudecode"
)

func fakeClaude(t *testing.T) *claudecode.Report {
	t.Helper()
	now := time.Now()
	r := &claudecode.Report{
		Taken: now,
		Leftovers: []claudecode.Group{
			{Session: "aaaa1111-2222", Project: "shop", Title: "Fix checkout", Procs: []claudecode.Proc{
				{PID: 9101, Command: "node vite --port 5173", Path: "node", Ports: []string{"5173"}, Memory: 200e6, Age: 7200},
				{PID: 9102, Command: "node watcher", Path: "node", Memory: 50e6, Age: 7000},
			}},
			{Session: "bbbb3333-4444", Project: "api", Procs: []claudecode.Proc{
				{PID: 9201, Command: "python3 -m http.server", Path: "python3", Memory: 10e6, Age: 300},
			}},
		},
		Sessions: []claudecode.Session{
			{PID: 9300, ID: "cccc5555", Project: "omega", Title: "Here", Status: claudecode.Busy, Since: now.Add(-time.Minute), Current: true},
			{PID: 9400, ID: "dddd6666", Project: "web", Status: claudecode.Idle, Since: now.Add(-3 * time.Hour), Memory: 300e6,
				Procs: []claudecode.Proc{{PID: 9401, Command: "next dev", Path: "next", Memory: 100e6, Age: 9000}}},
		},
	}
	old := claudeScan
	claudeScan = func() (*claudecode.Report, error) { return r, nil }
	t.Cleanup(func() { claudeScan = old })
	return r
}

func TestClaudeList(t *testing.T) {
	r := fakeClaude(t)
	var out bytes.Buffer
	writeClaude(&out, r)
	for _, want := range []string{"3 processes from 2 closed sessions", "shop · aaaa1111 · Fix checkout", ":5173",
		"omega · cccc5555 · pid 9300", "this one", "web · dddd6666 · pid 9400 · idle 3 h", "400 MB"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("%q is missing from:\n%s", want, out.String())
		}
	}
}

func TestLeftoverTargets(t *testing.T) {
	r := fakeClaude(t)
	all, _, err := leftoverTargets(r, nil)
	if err != nil || len(all) != 3 {
		t.Fatalf("all = %v, %v", all, err)
	}
	one, _, err := leftoverTargets(r, []string{"9102", "bbbb"})
	if err != nil || len(one) != 2 || one[0].PID != 9102 || one[1].PID != 9201 {
		t.Fatalf("named = %+v, %v", one, err)
	}
	// Only what was left behind can be ended this way: not an open
	// session's process, not any PID, not a prefix too short to be sure.
	for _, args := range [][]string{{"9401"}, {"1"}, {"aaa"}, {"zzzz"}} {
		if _, _, err := leftoverTargets(r, args); err == nil {
			t.Errorf("%v was accepted", args)
		}
	}
}

func TestOpenSession(t *testing.T) {
	r := fakeClaude(t)
	if s, err := openSession(r, "dddd"); err != nil || s.PID != 9400 {
		t.Fatalf("by ID: %+v, %v", s, err)
	}
	if s, err := openSession(r, "9400"); err != nil || s.ID != "dddd6666" {
		t.Fatalf("by PID: %+v, %v", s, err)
	}
	if _, err := openSession(r, "9300"); err == nil || !strings.Contains(err.Error(), "omega runs in") {
		t.Errorf("the current session could be ended: %v", err)
	}
	if got := len(claudecode.SessionTargets(r.Sessions[1])); got != 2 {
		t.Errorf("targets of a session = %d, want the claude process and its one process", got)
	}
}

func TestClaudeKillNeedsYesWithoutTerminal(t *testing.T) {
	fakeClaude(t)
	old := stdinIsTerminal
	stdinIsTerminal = func() bool { return false }
	t.Cleanup(func() { stdinIsTerminal = old })
	for _, args := range [][]string{{"claude", "kill"}, {"claude", "end", "9400"}, {"claude", "idle"}} {
		var f runFlags
		root := newRoot(&f)
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs(args)
		err := root.Execute()
		if err == nil || !strings.Contains(err.Error(), "--yes") {
			t.Errorf("%v without a terminal: %v", args, err)
		}
	}
}

func TestClaudeIdleMinimum(t *testing.T) {
	fakeClaude(t)
	var f runFlags
	root := newRoot(&f)
	root.SetArgs([]string{"claude", "idle", "--after", "10m", "--yes"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "at least") {
		t.Errorf("a 10 minute limit was accepted: %v", err)
	}
	if _, err := ClaudeEnd("idle", "10"); err == nil {
		t.Error("the app could end sessions idle for 10 minutes")
	}
}

func TestClaudeView(t *testing.T) {
	fakeClaude(t)
	v, err := ClaudeList()
	if err != nil || !v.Supported || v.Count != 3 || len(v.Leftovers) != 2 || len(v.Sessions) != 2 {
		t.Fatalf("view = %+v, %v", v, err)
	}
	if s := v.Sessions[1]; s.IdleMin != 180 || s.Status != claudecode.Idle || s.Current {
		t.Errorf("idle session row = %+v", s)
	}
	if !v.Sessions[0].Current {
		t.Error("the current session is not marked")
	}
	for _, c := range [][2]string{{"proc", "9401"}, {"proc", "-1"}, {"group", "aaaa"}, {"session", "9300"}, {"session", "x"}, {"nope", ""}} {
		if _, err := ClaudeEnd(c[0], c[1]); err == nil {
			t.Errorf("ClaudeEnd(%q, %q) was accepted", c[0], c[1])
		}
	}
}
