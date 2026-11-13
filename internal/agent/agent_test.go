package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/runner"
)

// scriptedProvider replays a fixed list of replies and records tool results.
type scriptedProvider struct {
	replies []*llm.Reply
	results [][]llm.ToolResult
}

func (p *scriptedProvider) Name() string                                    { return "test" }
func (p *scriptedProvider) Models(context.Context) ([]string, error)        { return nil, nil }
func (p *scriptedProvider) NewChat(string, string, []llm.ToolSpec) llm.Chat { return p }

func (p *scriptedProvider) SetModel(string) {}

func (p *scriptedProvider) Send(_ context.Context, _ string, results []llm.ToolResult) (*llm.Reply, error) {
	p.results = append(p.results, append([]llm.ToolResult(nil), results...))
	r := p.replies[0]
	p.replies = p.replies[1:]
	return r, nil
}

func call(id, name string, in map[string]any) llm.ToolCall {
	b, _ := json.Marshal(in)
	return llm.ToolCall{ID: id, Name: name, Input: b}
}

func run(t *testing.T, p *scriptedProvider, opts Options, answer Decision) []Event {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	opts.Config = config.Default()
	opts.Timeout = 10 * time.Second
	if opts.Approval == "" {
		opts.Approval = config.ApproveDangerous
	}
	a := New(p, opts)
	go a.Run(context.Background(), "test")
	var evs []Event
	for e := range a.Events() {
		if e.Kind == EvConfirm {
			e.Reply <- answer
		}
		evs = append(evs, e)
	}
	return evs
}

func TestSafeCommandRunsWithoutConfirmation(t *testing.T) {
	p := &scriptedProvider{replies: []*llm.Reply{
		{Calls: []llm.ToolCall{call("1", "run_command", map[string]any{"command": "echo hola", "reason": "r"})}},
		{Text: "listo"},
	}}
	evs := run(t, p, Options{}, Deny)
	for _, e := range evs {
		if e.Kind == EvConfirm {
			t.Fatal("safe command asked for confirmation")
		}
	}
	got := p.results[1][0]
	if got.IsError || !strings.Contains(got.Content, "hola") || !strings.Contains(got.Content, "exit_code: 0") {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestDangerousCommandDenied(t *testing.T) {
	p := &scriptedProvider{replies: []*llm.Reply{
		{Calls: []llm.ToolCall{call("1", "run_command", map[string]any{"command": "rm -rf /tmp/omega-test-nope", "reason": "r"})}},
		{Text: "vale"},
	}}
	evs := run(t, p, Options{}, Deny)
	asked := false
	for _, e := range evs {
		asked = asked || e.Kind == EvConfirm
	}
	if !asked {
		t.Fatal("dangerous command did not ask for confirmation")
	}
	if r := p.results[1][0]; !r.IsError || !strings.Contains(r.Content, "denied") {
		t.Fatalf("expected a denial result, got %+v", r)
	}
}

func flagExec(ran *bool) ExecFunc {
	return func(_ context.Context, _ string, _ time.Duration, _ func(string)) runner.Result {
		*ran = true
		return runner.Result{}
	}
}

func countingExec(pause time.Duration, cur, maxc *int, mu *sync.Mutex, onLine bool) ExecFunc {
	return func(_ context.Context, command string, _ time.Duration, line func(string)) runner.Result {
		mu.Lock()
		*cur++
		if *cur > *maxc {
			*maxc = *cur
		}
		mu.Unlock()
		time.Sleep(pause)
		if onLine && line != nil {
			line(command)
		}
		mu.Lock()
		*cur--
		mu.Unlock()
		return runner.Result{Output: command}
	}
}

func TestReadFileRefusesAPrivateKey(t *testing.T) {
	const secret = "SUPERSECRETKEYDATA"
	path := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(path, []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}
	p := &scriptedProvider{replies: []*llm.Reply{
		{Calls: []llm.ToolCall{call("1", "read_file", map[string]any{"path": path})}},
		{Text: "ok"},
	}}
	evs := run(t, p, Options{}, Deny)
	r := p.results[1][0]
	if !r.IsError || strings.Contains(r.Content, secret) {
		t.Fatalf("private key reached the model: %+v", r)
	}
	shown := false
	for _, e := range evs {
		if strings.Contains(e.Reason, secret) || strings.Contains(e.Text, secret) {
			t.Fatalf("event carried the key: %+v", e)
		}
		if e.Kind == EvToolEnd && e.Failed && strings.Contains(e.Reason, "refusing to read") {
			shown = true
		}
	}
	if !shown {
		t.Fatal("the refusal never reached the tool card")
	}
	if !strings.Contains(r.Content, "refusing to read") {
		t.Fatalf("result: %+v", r)
	}
}

func TestReadFileReturnsANormalFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "topgrade.toml")
	if err := os.WriteFile(path, []byte("assume_yes = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := &scriptedProvider{replies: []*llm.Reply{
		{Calls: []llm.ToolCall{call("1", "read_file", map[string]any{"path": path})}},
		{Text: "ok"},
	}}
	run(t, p, Options{}, Deny)
	r := p.results[1][0]
	if r.IsError || !strings.Contains(r.Content, "assume_yes = true") {
		t.Fatalf("result: %+v", r)
	}
}

func TestYesDoesNotCatAPrivateKey(t *testing.T) {
	ran := false
	p := &scriptedProvider{replies: []*llm.Reply{
		{Calls: []llm.ToolCall{call("1", "run_command", map[string]any{"command": "cat ~/.aws/credentials", "reason": "r"})}},
		{Text: "ok"},
	}}
	run(t, p, Options{AutoYes: true, Exec: flagExec(&ran)}, Approve)
	if ran {
		t.Fatal("cat of credentials ran under --yes")
	}
	if r := p.results[1][0]; !r.IsError || !strings.Contains(r.Content, "Blocked") {
		t.Fatalf("expected blocked, got %+v", r)
	}
}

func TestYesDoesNotRunADisguisedWipe(t *testing.T) {
	ran := false
	p := &scriptedProvider{replies: []*llm.Reply{
		{Calls: []llm.ToolCall{call("1", "run_command", map[string]any{"command": "rm -rf / --no-preserve-root", "reason": "r"})}},
		{Text: "ok"},
	}}
	run(t, p, Options{AutoYes: true, Exec: flagExec(&ran)}, Approve)
	if ran {
		t.Fatal("rm -rf / --no-preserve-root ran under --yes")
	}
	if r := p.results[1][0]; !r.IsError || !strings.Contains(r.Content, "Blocked") {
		t.Fatalf("expected blocked, got %+v", r)
	}
}

func TestReadOnlyBatchOverlaps(t *testing.T) {
	const n = 4
	const pause = 80 * time.Millisecond
	var mu sync.Mutex
	var cur, maxc int
	p := &scriptedProvider{replies: []*llm.Reply{{Calls: []llm.ToolCall{
		call("1", "run_command", map[string]any{"command": "echo a", "reason": "r"}),
		call("2", "run_command", map[string]any{"command": "echo b", "reason": "r"}),
		call("3", "run_command", map[string]any{"command": "echo c", "reason": "r"}),
		call("4", "run_command", map[string]any{"command": "echo d", "reason": "r"}),
	}}, {Text: "ok"}}}
	start := time.Now()
	run(t, p, Options{Exec: countingExec(pause, &cur, &maxc, &mu, true)}, Deny)
	elapsed := time.Since(start)
	if maxc != n {
		t.Fatalf("overlap %d, want %d running at once", maxc, n)
	}
	// The count above is the proof: all four were inside Exec at the same
	// moment. The time is only logged. A wall-clock limit failed on a busy
	// Windows runner (2.5s for four 80ms waits) with the overlap intact.
	t.Logf("%d read-only tools overlapped in %s (one after another would be about %s)", n, elapsed.Round(time.Millisecond), (pause * time.Duration(n)).Round(time.Millisecond))
	for i, id := range []string{"1", "2", "3", "4"} {
		got := p.results[1][i]
		if got.CallID != id || got.IsError {
			t.Fatalf("result %d: %+v", i, got)
		}
	}
}

func TestMixedBatchDoesNotOverlap(t *testing.T) {
	const pause = 60 * time.Millisecond
	var mu sync.Mutex
	var cur, maxc int
	p := &scriptedProvider{replies: []*llm.Reply{{Calls: []llm.ToolCall{
		call("1", "run_command", map[string]any{"command": "echo a", "reason": "r"}),
		call("2", "run_command", map[string]any{"command": "rm -rf /tmp/omega-test-nope", "reason": "r"}),
	}}, {Text: "ok"}}}
	run(t, p, Options{Exec: countingExec(pause, &cur, &maxc, &mu, false)}, Approve)
	if maxc != 1 {
		t.Fatalf("a read and a change overlapped %d-wide; they must stay in order", maxc)
	}
}

func TestReadOnlyRejectsUpdates(t *testing.T) {
	p := &scriptedProvider{replies: []*llm.Reply{
		{Calls: []llm.ToolCall{call("1", "run_command", map[string]any{"command": "brew upgrade", "reason": "r"})}},
		{Text: "ok"},
	}}
	run(t, p, Options{ReadOnly: true, AutoYes: true}, Approve)
	if r := p.results[1][0]; !r.IsError || !strings.Contains(r.Content, "Read-only") {
		t.Fatalf("expected read-only refusal, got %+v", r)
	}
}

func TestBlockedEvenWithYes(t *testing.T) {
	p := &scriptedProvider{replies: []*llm.Reply{
		{Calls: []llm.ToolCall{call("1", "run_command", map[string]any{"command": "rm -rf ~", "reason": "r"})}},
		{Text: "ok"},
	}}
	run(t, p, Options{AutoYes: true}, Approve)
	if r := p.results[1][0]; !r.IsError || !strings.Contains(r.Content, "Blocked") {
		t.Fatalf("expected blocked, got %+v", r)
	}
}

func TestProposeActionsLiftsReadOnly(t *testing.T) {
	p := &scriptedProvider{replies: []*llm.Reply{
		{Calls: []llm.ToolCall{call("1", "propose_actions", map[string]any{
			"question": "Clean?", "multiple": true,
			"options": []map[string]any{{"id": "a", "label": "A"}, {"id": "b", "label": "B"}},
		})}},
		{Calls: []llm.ToolCall{call("2", "run_command", map[string]any{"command": "touch /dev/null", "reason": "r"})}},
		{Text: "done"},
	}}
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New(p, Options{ReadOnly: true, Interactive: true, Config: config.Default(), Approval: config.ApproveDangerous, Timeout: 5 * time.Second})
	go a.Run(context.Background(), "test")
	sawMode := false
	for e := range a.Events() {
		switch e.Kind {
		case EvChoice:
			e.Picks <- []string{"b"}
		case EvMode:
			sawMode = true
		case EvConfirm:
			e.Reply <- Approve
		case EvIdle:
			a.Input() <- ""
		}
	}
	if !sawMode {
		t.Fatal("read-only mode was not lifted after a selection")
	}
	if got := p.results[1][0].Content; !strings.Contains(got, "- b: B") {
		t.Fatalf("selection not reported: %q", got)
	}
	if r := p.results[2][0]; r.IsError {
		t.Fatalf("command after selection was rejected: %+v", r)
	}
}

func TestDockerPruneAlwaysAsks(t *testing.T) {
	p := &scriptedProvider{replies: []*llm.Reply{
		{Calls: []llm.ToolCall{call("1", "run_command", map[string]any{"command": "docker volume prune -f", "reason": "r"})}},
		{Text: "ok"},
	}}
	asked := false
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New(p, Options{AutoYes: true, Config: config.Default(), Timeout: 5 * time.Second,
		Exec: func(context.Context, string, time.Duration, func(string)) runner.Result { return runner.Result{} }})
	go a.Run(context.Background(), "test")
	for e := range a.Events() {
		if e.Kind == EvConfirm {
			asked = e.Always
			e.Reply <- Deny
		}
	}
	if !asked {
		t.Fatal("docker prune ran without an always-confirm question under --yes")
	}
}

type modelRecorder struct {
	scriptedProvider
	models []string
}

func (m *modelRecorder) NewChat(string, string, []llm.ToolSpec) llm.Chat { return m }
func (m *modelRecorder) SetModel(s string)                               { m.models = append(m.models, s) }

func TestEscalatesOnStepFailure(t *testing.T) {
	p := &modelRecorder{scriptedProvider: scriptedProvider{replies: []*llm.Reply{
		{Calls: []llm.ToolCall{call("1", "run_step", map[string]any{"id": "npm"})}},
		{Text: "ok"},
	}}}
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := New(p, Options{Model: "fast", Escalate: "smart", AutoYes: true, SkipDetect: true, Config: config.Default(), Timeout: 5 * time.Second,
		Exec: func(context.Context, string, time.Duration, func(string)) runner.Result {
			return runner.Result{ExitCode: 1}
		}})
	go a.Run(context.Background(), "test")
	for range a.Events() {
	}
	if len(p.models) != 1 || p.models[0] != "smart" {
		t.Fatalf("expected one switch to smart, got %v", p.models)
	}
}

func TestFollowUpHints(t *testing.T) {
	for out, want := range map[string]string{
		"Cannot connect to the Docker daemon at unix:///x. Is the docker daemon running?": "docker-start",
		"zsh: command not found: mas":                                      "install",
		"✓ Skipping sudo-required optimizations: admin access not granted": "omega --sudo",
		"all good": "",
	} {
		got := followUpHint(out)
		if (want == "") != (got == "") || !strings.Contains(got, want) {
			t.Errorf("followUpHint(%q) = %q, want it to mention %q", out, got, want)
		}
	}
}

func TestUserChangesApplyBeforeNextTurn(t *testing.T) {
	p := &modelRecorder{}
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	a := New(p, Options{Model: "fast", Escalate: "smart", Config: config.Default()})
	// Run is not called, so the log file would stay open and Windows could not
	// delete the temp dir.
	t.Cleanup(a.closeLog)
	a.SetMode(ModeReadOnly)
	a.SetModel("other")
	if ok, _ := a.authorize(context.Background(), "run_command", "brew upgrade", "", 1, false); ok {
		t.Fatal("read-only mode set by the user must reject changes")
	}
	text := a.applyChanges("hello")
	if !strings.Contains(text, "read-only") || !strings.HasPrefix(text, "hello") {
		t.Fatalf("the model should be told about read-only mode: %q", text)
	}
	if a.model != "other" || len(p.models) != 1 || p.models[0] != "other" {
		t.Fatalf("picked model not applied: %s %v", a.model, p.models)
	}
	a.escalate()
	if a.model != "other" {
		t.Fatal("a model the user picked must not be replaced by escalation")
	}
	a.SetMode(ModeAuto)
	if ok, _ := a.authorize(context.Background(), "run_command", "brew upgrade", "", 2, false); !ok {
		t.Fatal("auto mode should approve without asking")
	}
	if text := a.applyChanges(""); !strings.Contains(text, "switched off read-only") {
		t.Fatalf("the model should be told read-only mode ended: %q", text)
	}
	if text := a.applyChanges("x"); text != "x" {
		t.Fatalf("notes must be sent once: %q", text)
	}
}

// An unattended run waits for nobody: every option offered is taken, and an
// action that always needs a person is skipped instead of asked.
func TestUnattendedTakesEveryOptionAndSkipsAlwaysAsk(t *testing.T) {
	p := &scriptedProvider{replies: []*llm.Reply{
		{Calls: []llm.ToolCall{call("1", "propose_actions", map[string]any{
			"question": "Clean?", "multiple": true,
			"options": []map[string]any{{"id": "a", "label": "A"}, {"id": "b", "label": "B"}},
		})}},
		{Calls: []llm.ToolCall{call("2", "run_command", map[string]any{"command": "brew cleanup", "reason": "r"})}},
		{Calls: []llm.ToolCall{call("3", "run_command", map[string]any{"command": "docker volume prune -f", "reason": "r"})}},
		{Text: "done"},
	}}
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var ran []string
	a := New(p, Options{ReadOnly: true, AutoYes: true, Unattended: true, Config: config.Default(), Approval: config.ApproveAll,
		Timeout: 5 * time.Second, Exec: func(_ context.Context, cmd string, _ time.Duration, _ func(string)) runner.Result {
			ran = append(ran, cmd)
			return runner.Result{}
		}})
	go a.Run(context.Background(), "test")
	for e := range a.Events() {
		switch e.Kind {
		case EvChoice, EvConfirm, EvPassword, EvIdle:
			t.Fatalf("an unattended run asked something: %+v", e)
		}
	}
	if got := p.results[1][0].Content; !strings.Contains(got, "- a: A") || !strings.Contains(got, "- b: B") {
		t.Fatalf("every option should be taken: %q", got)
	}
	if r := p.results[2][0]; r.IsError {
		t.Fatalf("an approved command was rejected: %+v", r)
	}
	if r := p.results[3][0]; !r.IsError || !strings.Contains(r.Content, "Unattended run") {
		t.Fatalf("an always-ask action must be skipped: %+v", r)
	}
	if strings.Join(ran, ";") != "brew cleanup" {
		t.Fatalf("ran %q", ran)
	}
}

func TestFetchRefusesPrivateAddresses(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:80", "[::1]:80", "10.0.0.5:80", "192.168.1.1:80", "172.16.3.4:443",
		"169.254.169.254:80", "100.100.100.100:80", "[fe80::1]:80", "[fd00::1]:80", "0.0.0.0:80", "[::ffff:127.0.0.1]:80"} {
		if err := publicOnly("tcp", addr, nil); err == nil {
			t.Errorf("%s was allowed", addr)
		}
	}
	for _, addr := range []string{"1.1.1.1:443", "140.82.112.3:443", "[2606:4700:4700::1111]:443"} {
		if err := publicOnly("tcp", addr, nil); err != nil {
			t.Errorf("%s was refused: %v", addr, err)
		}
	}
	if _, _, err := get(context.Background(), "http://127.0.0.1:9/"); err == nil || !strings.Contains(err.Error(), "public internet") {
		t.Errorf("a fetch from localhost: %v", err)
	}
}

func TestReadHead(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "big")
	os.WriteFile(big, bytes.Repeat([]byte("a"), 100), 0o644)
	data, cut, err := readHead(big, 10)
	if err != nil || !cut || len(data) != 10 {
		t.Errorf("big file: %d bytes, cut %v, %v", len(data), cut, err)
	}
	if _, _, err := readHead(dir, 10); err == nil {
		t.Error("a folder was read")
	}
	if runtime.GOOS != "windows" {
		if _, _, err := readHead("/dev/zero", 10); err == nil {
			t.Error("/dev/zero was read")
		}
	}
}

// At the turn limit the results of the last tools are not dropped: they go
// to the model with the user's next message, so every tool call it made
// has its answer.
func TestTurnLimitKeepsToolResults(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	p := &scriptedProvider{replies: []*llm.Reply{
		{Calls: []llm.ToolCall{call("1", "run_command", map[string]any{"command": "echo uno", "reason": "r"})}},
		{Text: "listo"},
	}}
	a := New(p, Options{Config: config.Default(), Timeout: 10 * time.Second, Approval: config.ApproveDangerous, Interactive: true, MaxTurns: 1})
	go a.Run(context.Background(), "test")
	idles := 0
	for e := range a.Events() {
		if e.Kind != EvIdle {
			continue
		}
		if idles++; idles == 1 {
			a.Input() <- "sigue"
		} else {
			a.Input() <- ""
		}
	}
	if len(p.results) != 2 || len(p.results[1]) != 1 || p.results[1][0].CallID != "1" {
		t.Fatalf("the second message carried %+v", p.results)
	}
}

// A run that ends on a reply cut at the output limit did not finish.
func TestTruncatedReplyIsNotSuccess(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	p := &scriptedProvider{replies: []*llm.Reply{{Text: "a medias", Truncated: true}}}
	a := New(p, Options{Config: config.Default(), Timeout: 10 * time.Second, Approval: config.ApproveDangerous})
	go a.Run(context.Background(), "test")
	for range a.Events() {
	}
	if a.ExitCode() == 0 {
		t.Error("exit status 0 after a truncated reply")
	}
}
