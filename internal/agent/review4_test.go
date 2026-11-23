package agent

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/pending"
	"github.com/lcajigasm/omega/internal/runner"
)

// session runs scripted replies against a fake shell and returns the
// provider (with the tool results), the commands that ran and the transcript.
type session struct {
	p      *scriptedProvider
	ran    []string
	log    string
	events []Event
}

func runScript(t *testing.T, opts Options, answer Decision, picks []string, output func(cmd string) runner.Result, replies ...*llm.Reply) *session {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s := &session{p: &scriptedProvider{replies: append(replies, &llm.Reply{Text: "done"})}}
	opts.Config = config.Default()
	opts.Timeout = 5 * time.Second
	if opts.Approval == "" {
		opts.Approval = config.ApproveDangerous
	}
	opts.Exec = func(_ context.Context, cmd string, _ time.Duration, onLine func(string)) runner.Result {
		s.ran = append(s.ran, cmd)
		res := runner.Result{}
		if output != nil {
			res = output(cmd)
		}
		if onLine != nil {
			for _, l := range strings.Split(res.Output, "\n") {
				if l != "" {
					onLine(l)
				}
			}
		}
		return res
	}
	a := New(s.p, opts)
	path := a.LogPath()
	go a.Run(context.Background(), "the task")
	for e := range a.Events() {
		switch e.Kind {
		case EvConfirm:
			e.Reply <- answer
		case EvChoice:
			e.Picks <- picks
		}
		s.events = append(s.events, e)
	}
	b, _ := os.ReadFile(path)
	s.log = string(b)
	return s
}

func proposal(multi bool, options ...map[string]any) *llm.Reply {
	return &llm.Reply{Calls: []llm.ToolCall{call("1", "propose_actions", map[string]any{
		"question": "What now?", "multiple": multi, "options": options})}}
}

func opt(id, label, risk string, rec bool) map[string]any {
	return map[string]any{"id": id, "label": label, "risk": risk, "recommended": rec}
}

// A1: an unattended run never takes two answers to a question that has
// one, never a "leave it" option, never a dangerous one, and says nobody chose.
func TestUnattendedSingleChoiceTakesOnlyTheRecommendedOption(t *testing.T) {
	s := runScript(t, Options{Unattended: true}, Deny, nil, nil,
		proposal(false, opt("apply", "Apply ControlMaster", "normal", true), opt("none", "No change", "read-only", false)))
	got := s.p.results[1][0].Content
	if !strings.Contains(got, "- apply: Apply ControlMaster") || strings.Contains(got, "- none:") || !strings.Contains(got, "nobody chose") {
		t.Fatalf("single choice: %q", got)
	}
	if !strings.Contains(s.log, "chosen by omega, automatically") {
		t.Fatalf("the transcript must say who chose: %s", s.log)
	}

	s = runScript(t, Options{Unattended: true}, Deny, nil, nil,
		proposal(false, opt("alfred", "Alfred keeps ⌥Space", "normal", false), opt("chatgpt", "ChatGPT keeps it", "normal", false)))
	if got := s.p.results[1][0].Content; !strings.Contains(got, "No option was taken") {
		t.Fatalf("without a recommendation nothing is taken: %q", got)
	}

	s = runScript(t, Options{Unattended: true}, Deny, nil, nil,
		proposal(true, opt("cache", "Prune the build cache", "normal", false), opt("volumes", "Delete the 80 volumes", "dangerous", false),
			opt("nothing", "Leave Docker as it is", "read-only", false)))
	got = s.p.results[1][0].Content
	if !strings.Contains(got, "- cache:") || strings.Contains(got, "- volumes:") || strings.Contains(got, "- nothing:") {
		t.Fatalf("multiple choice: %q", got)
	}
}

// A1: a person's card that comes back with several answers to a single
// question is not acted on.
func TestSeveralAnswersToOneQuestionAreRefused(t *testing.T) {
	s := runScript(t, Options{Interactive: true, OneShot: true}, Deny, []string{"a", "b"}, nil,
		proposal(false, opt("a", "A", "normal", true), opt("b", "B", "normal", false)))
	if got := s.p.results[1][0].Content; !strings.Contains(got, "nothing was chosen") {
		t.Fatalf("got %q", got)
	}
}

// A4: after a program asks for a person's confirmation, adding --yes to it
// is asked about, also in automatic mode.
func TestYesAfterARefusalIsAskedAbout(t *testing.T) {
	out := func(cmd string) runner.Result {
		if !strings.Contains(cmd, "--yes") {
			return runner.Result{ExitCode: 1, Output: "Purge requires confirmation. Run mo purge in a terminal, or --yes to confirm unattended cleanup"}
		}
		return runner.Result{}
	}
	s := runScript(t, Options{AutoYes: true}, Deny, nil, out,
		&llm.Reply{Calls: []llm.ToolCall{call("1", "run_command", map[string]any{"command": "cleaner purge 2>&1 | tail -4", "reason": "r"})}},
		&llm.Reply{Calls: []llm.ToolCall{call("2", "run_command", map[string]any{"command": "cleaner purge --yes", "reason": "r"})}})
	asked := false
	for _, e := range s.events {
		if e.Kind == EvConfirm && strings.Contains(e.Command, "--yes") {
			asked = true
		}
	}
	if !asked || len(s.ran) != 1 {
		t.Fatalf("asked %v, ran %q", asked, s.ran)
	}
}

// B6: an unattended run does not open System Settings; the link goes to the summary.
func TestUnattendedDoesNotOpenSettings(t *testing.T) {
	s := runScript(t, Options{Unattended: true}, Deny, nil, nil,
		&llm.Reply{Calls: []llm.ToolCall{call("1", "run_command", map[string]any{
			"command": `open "x-apple.systempreferences:com.apple.Time-Machine-Settings.extension"`, "reason": "r"})}})
	if len(s.ran) != 0 || !strings.Contains(s.p.results[1][0].Content, "x-apple.systempreferences:com.apple.Time-Machine-Settings.extension") {
		t.Fatalf("ran %q, result %q", s.ran, s.p.results[1][0].Content)
	}
	if !strings.Contains(s.log, "[run_command DENIED]") {
		t.Fatalf("C2: the refusal must be in the transcript:\n%s", s.log)
	}
}

// C1, C4, C6: the header says how the run was started; tools that are not
// shell commands do not read as commands and leave what they returned.
func TestTranscriptHeaderAndToolLines(t *testing.T) {
	s := runScript(t, Options{Unattended: true, Version: "v9.9.9", Task: "health", Command: "omega full --unattended"}, Deny, nil, nil,
		&llm.Reply{Calls: []llm.ToolCall{call("1", "read_file", map[string]any{"path": "/nonexistent/omega-test"})}},
		&llm.Reply{Calls: []llm.ToolCall{call("2", "pending", map[string]any{"action": "list"})}})
	for _, want := range []string{"version: v9.9.9", "command: omega full --unattended (task: health)", "mode: unattended",
		"> the task", "» read_file /nonexistent/omega-test", "[read_file FAILED", "[pending returned"} {
		if !strings.Contains(s.log, want) {
			t.Errorf("the transcript lacks %q:\n%s", want, s.log)
		}
	}
	if strings.Contains(s.log, "$ /nonexistent") {
		t.Error("a path must not read as a command")
	}
}

// C5: the output of calls that run together is written one call at a time.
func TestParallelOutputIsWrittenInBlocks(t *testing.T) {
	out := func(cmd string) runner.Result {
		var b strings.Builder
		for i := 0; i < 50; i++ {
			b.WriteString(cmd + " line\n")
		}
		return runner.Result{Output: b.String()}
	}
	s := runScript(t, Options{}, Deny, nil, out,
		&llm.Reply{Calls: []llm.ToolCall{
			call("1", "run_command", map[string]any{"command": "echo aaa", "reason": "r"}),
			call("2", "run_command", map[string]any{"command": "echo bbb", "reason": "r"}),
		}})
	a, b := strings.Index(s.log, "$ echo aaa"), strings.Index(s.log, "$ echo bbb")
	if a < 0 || b < 0 {
		t.Fatalf("headers missing:\n%s", s.log)
	}
	first, second := "echo aaa line", "echo bbb line"
	if a > b {
		first, second = second, first
	}
	end := strings.LastIndex(s.log, first)
	if strings.Index(s.log, second) < end {
		t.Fatalf("the two outputs are interleaved:\n%s", s.log)
	}
}

// C8: thousands of lines of the same error become a handful.
func TestRepeatedLinesAreFolded(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 5000; i++ {
		b.WriteString("awscli/2.41.1/libexec/lib/python3.15/site-packages/awscli/examples/x" + strings.Repeat("y", i%7) + ".rst: Failed to create dir 'awscli/2.41.1/x': No space left on device\n")
	}
	got := collapseRepeats(b.String())
	if strings.Count(got, "\n") > 10 || !strings.Contains(got, "more lines like the one above") {
		t.Fatalf("not folded (%d lines)", strings.Count(got, "\n"))
	}
	if lineShape("Warning: icu4c@78 has been deprecated") == lineShape("Error: naps2: no password was provided") {
		t.Fatal("two different messages share a shape")
	}
	out := func(string) runner.Result { return runner.Result{ExitCode: 1, Output: b.String()} }
	s := runScript(t, Options{}, Approve, nil, out, &llm.Reply{Calls: []llm.ToolCall{call("1", "run_command", map[string]any{"command": "echo x", "reason": "r"})}})
	if n := strings.Count(s.log, "No space left"); n > 10 {
		t.Fatalf("the transcript kept %d copies", n)
	}
}

// H1: ~/.ssh/config and git's settings reach the model without hosts, users
// or e-mail addresses.
func TestPersonalConfigIsMasked(t *testing.T) {
	ssh := "Host prod\n  HostName 203.0.113.7\n  User deploy\n  IdentityFile ~/.ssh/id_prod\n  ControlMaster auto"
	got := maskPersonal("cat ~/.ssh/config", ssh)
	if strings.Contains(got, "203.0.113.7") || strings.Contains(got, "deploy") || !strings.Contains(got, "Host prod") || !strings.Contains(got, "ControlMaster auto") {
		t.Fatalf("ssh: %q", got)
	}
	if got := maskPersonal("git config --global --list", "user.email=someone@example.com\ncore.pager=less"); strings.Contains(got, "someone@") {
		t.Fatalf("git: %q", got)
	}
	if got := maskPersonal("echo hi", "mail me at someone@example.com"); !strings.Contains(got, "someone@") {
		t.Fatal("only configuration is masked")
	}
}

// F1: a read-only step is not run twice in a session, and anything that
// changes the machine makes it run again.
func TestReadOnlyResultsAreKeptForTheSession(t *testing.T) {
	step := &llm.Reply{Calls: []llm.ToolCall{call("1", "disk_usage", map[string]any{"path": "/tmp"})}}
	s := runScript(t, Options{AutoYes: true}, Approve, nil, nil, step, step,
		&llm.Reply{Calls: []llm.ToolCall{call("2", "run_command", map[string]any{"command": "brew cleanup", "reason": "r"})}}, step)
	du := 0
	for _, c := range s.ran {
		if strings.HasPrefix(c, "du ") {
			du++
		}
	}
	if du != 2 || !strings.Contains(s.p.results[2][0].Content, "not run again") {
		t.Fatalf("du ran %d times; second result %q", du, s.p.results[2][0].Content)
	}
}

// F1: Mole's clean is not repeated within hours.
func TestMoleCleanCooldown(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	markClean("mole-clean")
	if _, ok := lastClean("mole-clean"); !ok {
		t.Fatal("a clean that just ran should be remembered")
	}
	if _, ok := lastClean("brew"); ok {
		t.Fatal("only cleaners have a cooldown")
	}
}

// G1: what is left for the user is recorded across runs and the next run
// is told about it.
func TestPendingItemsReachTheNextRun(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	now := time.Now()
	if out, err := pendingAction("add", "Time Machine", "Set up a Time Machine disk in System Settings", "backup", now); err != nil || !strings.Contains(out, "new") {
		t.Fatalf("add: %q %v", out, err)
	}
	if out, _ := pendingAction("add", "time-machine", "Set up a Time Machine disk in System Settings", "", now); !strings.Contains(out, "already known") {
		t.Fatalf("second add: %q", out)
	}
	if !strings.Contains(pendingPrompt(), "time-machine: Set up a Time Machine disk") {
		t.Fatalf("prompt: %q", pendingPrompt())
	}
	items, _ := pending.Load()
	items, _ = pending.Set(items, "time-machine", pending.Dismissed, time.Time{})
	pending.Save(items)
	if p := pendingPrompt(); strings.Contains(p, "Set up a Time Machine") || !strings.Contains(p, "do not mention them: time-machine") {
		t.Fatalf("dismissed: %q", p)
	}
}

// D3: a failure a stronger model cannot fix does not switch models.
func TestBenignFailuresDoNotEscalate(t *testing.T) {
	for _, out := range []string{"Cannot connect to the Docker daemon at unix:///var/run/docker.sock", "Error: no repositories found",
		"Please note that these warnings are just used to help the Homebrew maintainers", "No space left on device"} {
		if !benignFailure(out) {
			t.Errorf("%q should not escalate", out)
		}
	}
	if benignFailure("Error: SHA256 mismatch") {
		t.Error("a real failure must escalate")
	}
}

// A2: the prompt states who can answer, from omega's options.
func TestRunModeIsStated(t *testing.T) {
	if !strings.Contains(runModePrompt(Options{Unattended: true}), "UNATTENDED") ||
		!strings.Contains(runModePrompt(Options{Interactive: true, OneShot: true}), "GUIDED STAGE") ||
		!strings.Contains(runModePrompt(Options{}), "NON-INTERACTIVE") {
		t.Fatal("run mode")
	}
}

// Third review, P1: the ranges the address filter missed.
func TestFetchRefusesTunnelledAndReservedRanges(t *testing.T) {
	for _, addr := range []string{"[64:ff9b::7f00:1]:80", "[2002:7f00:1::1]:80", "198.18.0.1:80", "[2001:0:4136:e378::1]:80", "240.1.2.3:80"} {
		if err := publicOnly("tcp", addr, nil); err == nil {
			t.Errorf("%s was allowed", addr)
		}
	}
	if err := publicOnly("tcp", "93.184.216.34:443", nil); err != nil {
		t.Errorf("a public address was refused: %v", err)
	}
}

// Third review, P1: writing ~/.aws/config or ~/.kube/config is asked every time.
func TestCredentialRunnersAreSensitive(t *testing.T) {
	for _, p := range []string{"/Users/x/.aws/config", "/home/x/.kube/config"} {
		if !sensitiveWrite(p) {
			t.Errorf("%s", p)
		}
	}
}

// A6: a command that keeps failing on a full disk is stopped.
func TestFullDiskStopsTheCommand(t *testing.T) {
	stopped := false
	opts := Options{AutoYes: true}
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	p := &scriptedProvider{replies: []*llm.Reply{
		{Calls: []llm.ToolCall{call("1", "run_command", map[string]any{"command": "brew upgrade", "reason": "r"})}}, {Text: "done"}}}
	opts.Config, opts.Timeout = config.Default(), 5*time.Second
	opts.Exec = func(ctx context.Context, _ string, _ time.Duration, onLine func(string)) runner.Result {
		for i := 0; i < 1000; i++ {
			if ctx.Err() != nil {
				stopped = true
				break
			}
			onLine("x/y/z.rb: Failed to create dir: No space left on device")
		}
		return runner.Result{ExitCode: -1}
	}
	a := New(p, opts)
	go a.Run(context.Background(), "t")
	for e := range a.Events() {
		if e.Kind == EvConfirm {
			e.Reply <- Approve
		}
	}
	if !stopped || !strings.Contains(p.results[1][0].Content, "the disk is full") {
		t.Fatalf("stopped %v: %q", stopped, p.results[1][0].Content)
	}
}

// --dry-run stays read-only for the whole session: neither a pick on a
// card nor omega's own picks in an unattended run lift it.
func TestDryRunIsNeverLifted(t *testing.T) {
	for _, unattended := range []bool{true, false} {
		s := runScript(t, Options{ReadOnly: true, DryRun: true, AutoYes: true, Unattended: unattended, Interactive: !unattended, OneShot: true}, Approve, []string{"a"}, nil,
			proposal(true, opt("a", "Clean the cache", "normal", true)),
			&llm.Reply{Calls: []llm.ToolCall{call("2", "run_command", map[string]any{"command": "brew cleanup", "reason": "r"})}})
		if len(s.ran) != 0 || !strings.Contains(s.p.results[1][0].Content, "dry run") {
			t.Fatalf("unattended=%v: ran %q; %q", unattended, s.ran, s.p.results[1][0].Content)
		}
	}
}
