package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/runner"
)

// accessFixture is an agent on a Mac whose commands print what outputs
// holds, one per run, and the answers given to its cards.
func accessFixture(t *testing.T, interactive bool, outputs []string, answers ...string) (a *Agent, runs *int, opened *[]string, cards *[]Event) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	oldOS, oldHost, oldOpen, oldForget := goos, accessHost, accessOpen, accessForget
	t.Cleanup(func() { goos, accessHost, accessOpen, accessForget = oldOS, oldHost, oldOpen, oldForget })
	goos = "darwin"
	opened, cards, runs = &[]string{}, &[]Event{}, new(int)
	accessHost = func() (string, string) { return "com.example.term", "Term" }
	accessOpen = func(service string) error { *opened = append(*opened, "pane:"+service); return nil }
	accessForget = func(service, bundle string) error {
		*opened = append(*opened, "forget:"+service+":"+bundle)
		return nil
	}
	opts := exitOpts(interactive)
	opts.Exec = func(context.Context, string, time.Duration, func(string)) runner.Result {
		out := outputs[min(*runs, len(outputs)-1)]
		*runs++
		code := 0
		if strings.Contains(out, "not permitted") {
			code = 1
		}
		return runner.Result{Output: out, ExitCode: code}
	}
	a = New(failProvider{chat: &failChat{}}, opts)
	a.ctx = context.Background()
	go func() {
		for e := range a.events {
			if e.Kind != EvChoice {
				continue
			}
			*cards = append(*cards, e)
			pick := "skip"
			if len(answers) > 0 {
				pick, answers = answers[0], answers[1:]
			}
			e.Picks <- []string{pick}
		}
	}()
	// The transcript is closed before the temporary folder goes: Windows
	// does not delete a file that is still open.
	t.Cleanup(func() {
		close(a.events)
		a.closeLog()
	})
	return a, runs, opened, cards
}

func shellCall(command string) llm.ToolCall {
	in, _ := json.Marshal(map[string]string{"command": command, "reason": "test"})
	return llm.ToolCall{ID: "1", Name: "run_command", Input: in}
}

const mailDenied = "ls: /Users/ana/Library/Mail: Operation not permitted"

// A read-only command that privacy protection stopped: omega opens the pane,
// waits for the user and runs it again.
func TestAccessGrantedRetries(t *testing.T) {
	a, runs, opened, cards := accessFixture(t, true, []string{mailDenied, "V10"}, "settings", "retry")
	res := a.execute(context.Background(), shellCall("ls ~/Library/Mail"))
	if *runs != 2 || !strings.Contains(res.Content, "V10") || strings.Contains(res.Content, "NOTE") {
		t.Fatalf("runs %d, result %q", *runs, res.Content)
	}
	if len(*opened) != 1 || (*opened)[0] != "pane:SystemPolicyAllFiles" {
		t.Fatalf("opened %v", *opened)
	}
	if len(*cards) != 2 || !strings.Contains((*cards)[0].Text, "Term") || !strings.Contains((*cards)[0].Text, "Full Disk Access") {
		t.Fatalf("cards %+v", *cards)
	}
	// Full Disk Access has no dialog of its own: the card must not offer one.
	for _, c := range (*cards)[0].Choices {
		if c.ID == "dialog" {
			t.Error("a dialog was offered for Full Disk Access")
		}
	}
}

// Still blocked after the grant: the model is told the app must be reopened,
// and the permission is not asked for twice.
func TestAccessStillBlockedAsksOnce(t *testing.T) {
	a, runs, _, cards := accessFixture(t, true, []string{mailDenied}, "settings", "retry")
	res := a.execute(context.Background(), shellCall("ls ~/Library/Mail"))
	if *runs != 2 || !strings.Contains(res.Content, "quit and opened again") {
		t.Fatalf("runs %d, result %q", *runs, res.Content)
	}
	res = a.execute(context.Background(), shellCall("ls ~/Library/Mail"))
	if *runs != 3 || len(*cards) != 2 || !strings.Contains(res.Content, "already asked") {
		t.Fatalf("runs %d, cards %d, result %q", *runs, len(*cards), res.Content)
	}
}

// A folder macOS asks about by itself: its dialog is brought back.
func TestAccessDialogComesBack(t *testing.T) {
	a, runs, opened, _ := accessFixture(t, true, []string{"du: /Users/ana/Documents: Operation not permitted", "12K"}, "dialog")
	res := a.execute(context.Background(), shellCall("du -sh ~/Documents"))
	if *runs != 2 || !strings.Contains(res.Content, "12K") {
		t.Fatalf("runs %d, result %q", *runs, res.Content)
	}
	if len(*opened) != 1 || (*opened)[0] != "forget:SystemPolicyDocumentsFolder:com.example.term" {
		t.Fatalf("opened %v", *opened)
	}
}

func TestAccessDeclinedAndUnattended(t *testing.T) {
	a, runs, opened, _ := accessFixture(t, true, []string{mailDenied}, "skip")
	res := a.execute(context.Background(), shellCall("ls ~/Library/Mail"))
	if *runs != 1 || len(*opened) != 0 || !strings.Contains(res.Content, "go on without it") {
		t.Fatalf("declined: runs %d, opened %v, result %q", *runs, *opened, res.Content)
	}
	a, runs, opened, cards := accessFixture(t, false, []string{mailDenied})
	res = a.execute(context.Background(), shellCall("ls ~/Library/Mail"))
	if *runs != 1 || len(*opened) != 0 || len(*cards) != 0 || !strings.Contains(res.Content, "nobody is here") {
		t.Fatalf("unattended: runs %d, opened %v, result %q", *runs, *opened, res.Content)
	}
}

// Other systems have no such permissions, and an ordinary failure is not one.
func TestAccessOnlyOnMacAndOnlyPrivacy(t *testing.T) {
	a, runs, _, cards := accessFixture(t, true, []string{"rm: /System/x: Operation not permitted"})
	a.execute(context.Background(), shellCall("ls /System/x"))
	goos = "linux"
	*runs = 0
	a2 := a
	res := a2.execute(context.Background(), shellCall("ls ~/Library/Mail"))
	if len(*cards) != 0 || strings.Contains(res.Content, "NOTE") {
		t.Fatalf("cards %d, result %q", len(*cards), res.Content)
	}
}
