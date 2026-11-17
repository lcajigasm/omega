package aiagents

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func marked(session, claudePID string) map[string]string {
	return map[string]string{"CLAUDECODE": "1", "CLAUDE_CODE_SESSION_ID": session, "CLAUDE_PID": claudePID}
}

// machine is a process table: launchd, a live session (claude 100) with a
// dev server, and what a closed session (its claude was 200) left: a vite
// server, a Playwright driver and the Chrome it opened with two helpers.
// Chrome's environment is unreadable, as it is on a real Mac.
func machine() snap {
	chrome := "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	helper := "/Applications/Google Chrome.app/Contents/Frameworks/Google Chrome Framework.framework/Helpers/Google Chrome Helper.app/Contents/MacOS/Google Chrome Helper"
	ps := []*raw{
		{pid: 1, exe: "/sbin/launchd", command: "/sbin/launchd"},
		{pid: 90, ppid: 1, exe: "/bin/zsh", command: "-zsh", mine: true},
		{pid: 100, ppid: 90, exe: "/usr/local/bin/claude", command: "claude", mine: true, age: 5000},
		{pid: 101, ppid: 100, exe: "/bin/zsh", command: "/bin/zsh -c npm run dev", mine: true, env: marked("live", "100"), age: 4000},
		{pid: 102, ppid: 101, exe: "/usr/local/bin/node", command: "node vite", mine: true, env: marked("live", "100"), age: 4000, rss: 100 << 20},
		{pid: 103, ppid: 100, exe: "/usr/local/bin/claude", command: "claude --sub", mine: true, age: 30},
		// The closed session: reparented to launchd.
		{pid: 300, ppid: 1, exe: "/usr/local/bin/node", command: "node vite --port 5173", mine: true, env: marked("gone", "200"), age: 9000, rss: 200 << 20},
		{pid: 301, ppid: 1, exe: "/usr/local/bin/node", command: "node playwright", mine: true, env: marked("gone", "200"), age: 8000, rss: 50 << 20},
		{pid: 302, ppid: 301, exe: chrome, command: chrome + " --remote-debugging-pipe", mine: true, age: 8000, rss: 300 << 20},
		{pid: 303, ppid: 302, exe: helper, command: helper + " --type=renderer", mine: true, age: 8000, rss: 100 << 20},
		{pid: 304, ppid: 302, exe: helper, command: helper + " --type=gpu", mine: true, age: 8000, rss: 100 << 20},
		// Someone else's process with the marker is not ours to touch.
		{pid: 400, ppid: 1, exe: "/usr/bin/python3", command: "python3 x", mine: false, env: marked("other", "200"), age: 10},
		// A marked process with no session ID.
		{pid: 500, ppid: 1, exe: "/usr/bin/sleep", command: "sleep 999", mine: true, env: map[string]string{"CLAUDECODE": "1"}, age: 70},
		// Not marked and not started by anything marked.
		{pid: 600, ppid: 90, exe: "/usr/bin/vim", command: "vim", mine: true, age: 10},
		// omega, run from the live session, and what it started.
		{pid: 700, ppid: 101, exe: "/usr/local/bin/omega", command: "omega claude", mine: true, env: marked("live", "100"), age: 1},
		{pid: 701, ppid: 700, exe: "/bin/ps", command: "ps", mine: true, env: marked("live", "100"), age: 0},
	}
	s := snap{procs: map[int]*raw{}}
	for _, p := range ps {
		s.procs[p.pid] = p
	}
	return s
}

func TestAnalyze(t *testing.T) {
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	r := analyze(machine(), 700, now)

	if len(r.Leftovers) != 2 {
		t.Fatalf("leftover groups = %d, want 2: %+v", len(r.Leftovers), r.Leftovers)
	}
	g := r.Leftovers[0]
	if g.Session != "gone" {
		t.Fatalf("oldest group = %q, want gone", g.Session)
	}
	var pids []int
	for _, p := range g.Procs {
		pids = append(pids, p.PID)
	}
	if len(pids) != 3 || pids[0] != 300 || pids[1] != 301 || pids[2] != 302 {
		t.Fatalf("gone procs = %v, want [300 301 302] (Chrome adopted, its helpers folded)", pids)
	}
	chrome := g.Procs[2]
	if len(chrome.Helpers) != 2 || chrome.Memory != 500<<20 {
		t.Errorf("chrome helpers = %d, memory = %d; want 2 and 500 MiB", len(chrome.Helpers), chrome.Memory>>20)
	}
	if chrome.Name() != "Google Chrome" || !strings.HasPrefix(chrome.Short(), "Google Chrome --remote") {
		t.Errorf("name %q, short %q", chrome.Name(), chrome.Short())
	}
	if r.Leftovers[1].Session != "" || r.Leftovers[1].Procs[0].PID != 500 {
		t.Errorf("second group = %+v, want the marked process without a session", r.Leftovers[1])
	}
	if got := r.LeftoverMemory(); got != 750<<20 {
		t.Errorf("leftover memory = %d MiB, want 750", got>>20)
	}

	if len(r.Sessions) != 1 || r.Sessions[0].PID != 100 {
		t.Fatalf("sessions = %+v, want only claude 100 (103 is its subagent)", r.Sessions)
	}
	if !r.Sessions[0].Current {
		t.Error("the session omega runs in is not marked current")
	}
	for _, p := range r.live {
		if p.PID == 700 || p.PID == 701 {
			t.Errorf("omega or its child %d was counted", p.PID)
		}
	}
	// Processes reach the session once its ID is known, if older than a minute.
	r.Sessions[0].ID = "live"
	r.attach(r.live)
	if n := len(r.Sessions[0].Procs); n != 1 { // 102; 101 is omega's parent
		t.Errorf("live session procs = %d, want 1", n)
	}
}

func TestAliveThroughAncestorWithoutClaudePID(t *testing.T) {
	s := machine()
	s.procs[102].env = map[string]string{"CLAUDECODE": "1", "CLAUDE_CODE_SESSION_ID": "live"} // no CLAUDE_PID
	r := analyze(s, 0, time.Now())
	for _, g := range r.Leftovers {
		if g.Session == "live" {
			t.Fatalf("a process under a running claude is a leftover: %+v", g)
		}
	}
}

func TestIdleSessions(t *testing.T) {
	now := time.Now()
	r := &Report{Taken: now, Sessions: []Session{
		{PID: 1, Status: Idle, Since: now.Add(-3 * time.Hour)},
		{PID: 2, Status: Idle, Since: now.Add(-10 * time.Minute)},
		{PID: 3, Status: Busy, Since: now.Add(-5 * time.Hour)},
		{PID: 4, Status: Idle, Since: now.Add(-5 * time.Hour), Current: true},
		{PID: 5, Status: Idle},
	}}
	got := r.IdleSessions(2 * time.Hour)
	if len(got) != 1 || got[0].PID != 1 {
		t.Fatalf("idle = %+v, want only pid 1", got)
	}
}

func TestFieldsAndElapsed(t *testing.T) {
	f := fields("  12   34 /Applications/Google Chrome.app/x --flag", 3)
	if len(f) != 3 || f[2] != "/Applications/Google Chrome.app/x --flag" {
		t.Fatalf("fields = %q", f)
	}
	if fields("12", 3) != nil {
		t.Error("a short line gave fields")
	}
	for in, want := range map[string]int{"05": 5, "01:05": 65, "02:01:05": 7265, "3-02:01:05": 3*86400 + 7265} {
		if got := elapsed(in); got != want {
			t.Errorf("elapsed(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestRecordMatchesProcess(t *testing.T) {
	p := &raw{lstart: "Tue Nov  3 16:25:01 2026"}
	if !(&record{ProcStart: "Tue Nov 3 16:25:01 2026"}).sameProcess(p) {
		t.Error("the same start, padded differently, did not match")
	}
	if (&record{ProcStart: "Tue Nov 3 16:25:02 2026"}).sameProcess(p) {
		t.Error("a reused PID matched")
	}
	start := time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)
	q := &raw{start: start}
	ok := &record{StartedAt: "1794932702301"}
	if !ok.sameProcess(&raw{start: time.UnixMilli(1794932702301).Add(-time.Second)}) {
		t.Error("startedAt a second after the start did not match")
	}
	if (&record{StartedAt: "1794932702301"}).sameProcess(q) {
		t.Error("startedAt far from the start matched")
	}
	if (&record{}).sameProcess(q) {
		t.Error("a record without a start matched")
	}
}

func TestDescribe(t *testing.T) {
	home := t.TempDir()
	write := func(path, text string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(home, ".claude/sessions/100.json"),
		`{"pid":100,"sessionId":"live","cwd":"/src/shop","procStart":"Tue Nov 17 10:00:00 2026","status":"idle","statusUpdatedAt":1794900000000}`)
	write(filepath.Join(home, ".claude/projects/-src-api/gone.jsonl"),
		`{"type":"user","cwd":"/src/api"}`+"\n"+`{"type":"user","cwd":"/src/api/sub"}`+"\n"+`{"type":"ai-title","aiTitle":"Fix the login"}`+"\n")

	s := machine()
	s.procs[100].lstart = "Tue Nov 17 10:00:00 2026"
	r := analyze(s, 700, time.UnixMilli(1794900000000).Add(3*time.Hour))
	describe(r, s, home)

	ss := r.Sessions[0]
	if ss.ID != "live" || ss.Project != "shop" || ss.Status != Idle || ss.IdleFor(r.Taken) != 3*time.Hour {
		t.Errorf("session = %+v", ss)
	}
	if len(ss.Procs) != 1 {
		t.Errorf("session procs = %d, want 1", len(ss.Procs))
	}
	g := r.Leftovers[0]
	if g.Project != "api" || g.Title != "Fix the login" {
		t.Errorf("group project %q title %q, want api and Fix the login", g.Project, g.Title)
	}

	// A stale record, of an earlier process with the same PID, is ignored.
	s.procs[100].lstart = "Wed Nov 18 10:00:00 2026"
	r = analyze(s, 700, time.Now())
	describe(r, s, home)
	if r.Sessions[0].ID != "" || r.Sessions[0].Status != Unknown {
		t.Errorf("a stale record was read: %+v", r.Sessions[0])
	}
}

func TestLookupRefusesPaths(t *testing.T) {
	for _, id := range []string{"../x", "a/b", `a\b`, ""} {
		if got := lookup(t.TempDir(), id); got != (info{}) {
			t.Errorf("lookup(%q) = %+v", id, got)
		}
	}
}

func TestKill(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("nothing is ended on Windows")
	}
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go cmd.Wait() // reap it, or it stays a zombie that looks alive
	pid := cmd.Process.Pid
	var seen string
	// Until exec the child still shows the test binary's command line.
	wait(func() bool { seen = commandOf(pid); return strings.Contains(seen, "sleep") }, 2*time.Second)
	if !strings.Contains(seen, "sleep") {
		t.Fatal("the test process cannot be read")
	}

	// A PID that now runs something else is left alone.
	if res := Kill([]Target{{PID: pid, Command: "something else"}}, Grace); res.Skipped != 1 || res.Ended != 0 || !alive(pid) {
		t.Fatalf("a changed command was ended: %+v", res)
	}
	res := Kill([]Target{{PID: pid, Command: seen, Memory: 42}}, Grace)
	if res.Ended != 1 || res.Freed != 42 || len(res.Failed) != 0 {
		t.Fatalf("result = %+v", res)
	}
	if res := Kill([]Target{{PID: pid, Command: seen}}, Grace); res.Skipped != 1 {
		t.Errorf("a process already gone was not skipped: %+v", res)
	}
	if res := Kill([]Target{{PID: os.Getpid(), Command: commandOf(os.Getpid())}}, Grace); res.Skipped != 1 {
		t.Errorf("omega would end itself: %+v", res)
	}
}

func TestSnapshotFindsThisProcess(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		if _, err := snapshot(); err != ErrUnsupported {
			t.Fatalf("err = %v, want ErrUnsupported", err)
		}
		return
	}
	s, err := snapshot()
	if err != nil {
		t.Fatal(err)
	}
	me := s.procs[selfPID()]
	if me == nil || !me.mine || me.command == "" || me.age < 0 {
		t.Fatalf("this process: %+v", me)
	}
	if me.start.IsZero() {
		t.Error("the start time of this process is not known")
	}
}

// withCatalogue swaps the catalogue for a test.
func withCatalogue(t *testing.T, agents ...Agent) {
	t.Helper()
	old := Catalogue
	Catalogue = agents
	t.Cleanup(func() { Catalogue = old })
}

var (
	testClaude = Agent{ID: "claude", Name: "Claude Code", Exes: []string{"claude"}, Marker: "CLAUDECODE", Value: "1",
		SessionVar: "CLAUDE_CODE_SESSION_ID", PIDVar: "CLAUDE_PID", Records: true}
	testCodex  = Agent{ID: "codex", Name: "Codex", Exes: []string{"codex"}, Marker: "CODEX_THREAD", SessionVar: "CODEX_THREAD"}
	testGemini = Agent{ID: "gemini", Name: "Gemini CLI", Scripts: []string{"@google/gemini-cli/", "/bin/gemini"}, Marker: "GEMINI_CLI", Value: "1"}
	testAider  = Agent{ID: "aider", Name: "Aider", Exes: []string{"aider"}, Scripts: []string{"/bin/aider"}}
)

func table(ps ...*raw) snap {
	s := snap{procs: map[int]*raw{}}
	for _, p := range ps {
		s.procs[p.pid] = p
	}
	return s
}

func TestIdentify(t *testing.T) {
	withCatalogue(t, testClaude, testCodex, testGemini, testAider)
	for _, c := range []struct{ exe, cmd, want string }{
		{"/opt/homebrew/bin/codex", "codex", "codex"},
		{"/usr/local/bin/node", "node --no-warnings /usr/local/lib/node_modules/@google/gemini-cli/dist/index.js", "gemini"},
		{"node", "node /Users/x/.nvm/versions/node/v22/bin/gemini -p hi", "gemini"},
		{"/usr/bin/python3.12", "/usr/bin/python3.12 /home/x/.local/bin/aider --model y", "aider"},
		{"/Applications/Claude.app/Contents/MacOS/Claude", "/Applications/Claude.app/Contents/MacOS/Claude", ""},
		{"/usr/local/bin/node", "node server.js", ""},
		{"/usr/local/bin/node", "node", ""},
	} {
		got := ""
		if a := identify(c.exe, c.cmd); a != nil {
			got = a.ID
		}
		if got != c.want {
			t.Errorf("identify(%q, %q) = %q, want %q", c.exe, c.cmd, got, c.want)
		}
	}
}

func TestSeveralAgents(t *testing.T) {
	withCatalogue(t, testClaude, testCodex, testGemini, testAider)
	codex := map[string]string{"CODEX_THREAD": "th-1"}
	both := map[string]string{"CLAUDECODE": "1", "CLAUDE_CODE_SESSION_ID": "cl-1", "CLAUDE_PID": "100", "CODEX_THREAD": "th-2"}
	s := table(
		&raw{pid: 1, exe: "/sbin/launchd", command: "/sbin/launchd"},
		&raw{pid: 100, ppid: 1, exe: "/bin/claude", command: "claude", mine: true},
		// Codex started from Claude Code, still running, with a server.
		&raw{pid: 110, ppid: 100, exe: "/bin/codex", command: "codex exec", mine: true, env: marked("cl-1", "100")},
		&raw{pid: 111, ppid: 110, exe: "/bin/node", command: "node dev", mine: true, env: both, age: 120},
		// What a Codex session that closed left: the inner agent's.
		&raw{pid: 200, ppid: 1, exe: "/bin/node", command: "node watch", mine: true, env: both, age: 500},
		// A plain Codex session that closed.
		&raw{pid: 300, ppid: 1, exe: "/bin/vite", command: "vite", mine: true, env: codex, age: 900},
		// Gemini running as a node script, with a server it marked.
		&raw{pid: 400, ppid: 1, exe: "/bin/node", command: "node /usr/lib/node_modules/@google/gemini-cli/dist/index.js", mine: true},
		&raw{pid: 401, ppid: 400, exe: "/bin/python3", command: "python3 -m http.server", mine: true, env: map[string]string{"GEMINI_CLI": "1"}, age: 90},
		// Gemini's marker with another value is not Gemini's.
		&raw{pid: 402, ppid: 1, exe: "/bin/sleep", command: "sleep 9", mine: true, env: map[string]string{"GEMINI_CLI": "0"}},
		// Aider marks nothing: what runs under it is its session's.
		&raw{pid: 500, ppid: 1, exe: "/bin/aider", command: "aider", mine: true},
		&raw{pid: 501, ppid: 500, exe: "/bin/npm", command: "npm test --watch", mine: true, age: 300},
	)
	r := analyze(s, 0, time.Now())

	keys := map[string]int{}
	for _, g := range r.Leftovers {
		keys[g.Key()] = len(g.Procs)
	}
	// 200 is not left behind: the Claude Code session around the Codex that
	// started it is still open.
	if len(keys) != 1 || keys["codex/th-1"] != 1 {
		t.Fatalf("leftover groups = %v, want only codex/th-1", keys)
	}

	got := map[string]int{}
	for _, ss := range r.Sessions {
		got[ss.Agent] = len(ss.Procs)
	}
	// Codex 110 runs under Claude Code but is a session of its own agent,
	// and 111 under it is Codex's, not Claude Code's. 200 is the Claude Code
	// session's, through the PID in its environment.
	want := map[string]int{"claude": 1, "codex": 1, "gemini": 1, "aider": 1}
	for k, v := range want {
		if n, ok := got[k]; !ok || n != v {
			t.Errorf("sessions = %v, want %v", got, want)
			break
		}
	}
}

func TestCatalogue(t *testing.T) {
	seen := map[string]bool{}
	for _, a := range Catalogue {
		if a.ID == "" || a.Name == "" || seen[a.ID] || (len(a.Exes) == 0 && len(a.Scripts) == 0) {
			t.Errorf("bad entry %+v", a)
		}
		seen[a.ID] = true
		if a.Marker == "" && (a.SessionVar != "" || a.PIDVar != "") {
			t.Errorf("%s has session variables but no marker", a.ID)
		}
	}
	for _, c := range []struct{ exe, cmd, want string }{
		{"/opt/homebrew/bin/codex", "/opt/homebrew/bin/codex", "codex"},
		{"/Users/x/.local/share/cursor-agent/versions/1/node", "/Users/x/.local/bin/cursor-agent --use-system-ca /Users/x/.local/share/cursor-agent/versions/1/index.js", "cursor"},
		{"/usr/local/bin/node", "node /usr/local/lib/node_modules/@qwen-code/qwen-code/dist/index.js", "qwen"},
		{"/usr/local/bin/node", "node /usr/local/lib/node_modules/@github/copilot/npm-loader.js", "copilot"},
		{"/usr/local/bin/goosed", "goosed agent", "goose"},
		{"/Applications/Claude.app/Contents/MacOS/Claude", "Claude", ""},
	} {
		got := ""
		if a := identify(c.exe, c.cmd); a != nil {
			got = a.ID
		}
		if got != c.want {
			t.Errorf("identify(%q) = %q, want %q", c.cmd, got, c.want)
		}
	}
	codex, _ := ByID("codex")
	if !codex.service("/x/bin/codex app-server --listen unix:///tmp/s --managed-daemon") || codex.service("codex exec fix it") {
		t.Error("codex's service is not told from a session")
	}
}

func TestServicesAndForks(t *testing.T) {
	s := table(
		&raw{pid: 1, exe: "/sbin/launchd", command: "/sbin/launchd"},
		&raw{pid: 10, ppid: 1, exe: "/bin/codex", command: "codex app-server --managed-daemon", mine: true},
		&raw{pid: 11, ppid: 10, exe: "/bin/node", command: "node dev", mine: true, env: map[string]string{"CODEX_THREAD_ID": "t1"}, age: 100},
		// Kilo sets OPENCODE=1 too: a command of a Kilo that is gone is Kilo's.
		&raw{pid: 20, ppid: 1, exe: "/bin/vite", command: "vite", mine: true, env: map[string]string{"OPENCODE": "1", "KILO": "1", "KILO_PID": "999"}, age: 100},
	)
	r := analyze(s, 0, time.Now())
	if len(r.Sessions) != 1 || !r.Sessions[0].Service || len(r.Sessions[0].Procs) != 1 {
		t.Fatalf("sessions = %+v, want codex's service holding its command", r.Sessions)
	}
	if len(r.Leftovers) != 1 || r.Leftovers[0].Key() != "kilo/pid-999" {
		t.Fatalf("leftovers = %+v, want kilo/pid-999", r.Leftovers)
	}
	r.Sessions[0].Status, r.Sessions[0].Since = Idle, time.Now().Add(-5*time.Hour)
	if len(r.IdleSessions(time.Hour)) != 0 {
		t.Error("a service can be ended as idle")
	}
}

func TestUnderAnotherLiveAgent(t *testing.T) {
	cursor := map[string]string{"CURSOR_AGENT": "1", "CURSOR_CONVERSATION_ID": "c1"}
	s := table(
		&raw{pid: 1, exe: "/sbin/launchd", command: "/sbin/launchd"},
		// A script a closed Cursor session left, still running Codex.
		&raw{pid: 10, ppid: 1, exe: "/bin/zsh", command: "/bin/zsh chain.sh", mine: true, env: cursor, age: 600},
		&raw{pid: 11, ppid: 10, exe: "/bin/codex", command: "codex review", mine: true, env: cursor, age: 100},
		&raw{pid: 12, ppid: 11, exe: "/bin/codex-code-mode-host", command: "codex-code-mode-host --listen grpc+stdio://", mine: true, env: cursor, age: 90},
	)
	r := analyze(s, 0, time.Now())
	if len(r.Leftovers) != 1 || len(r.Leftovers[0].Procs) != 1 || r.Leftovers[0].Procs[0].PID != 10 {
		t.Fatalf("leftovers = %+v, want only the script", r.Leftovers)
	}
	if len(r.Sessions) != 1 || r.Sessions[0].Agent != "codex" || len(r.Sessions[0].Procs) != 1 {
		t.Fatalf("sessions = %+v, want codex holding its helper", r.Sessions)
	}
}
