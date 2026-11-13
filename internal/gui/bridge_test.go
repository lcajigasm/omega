package gui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/safety"
)

type fakeAgent struct {
	events chan agent.Event
	input  chan string
	mode   agent.Mode
}

func (f *fakeAgent) Events() <-chan agent.Event { return f.events }
func (f *fakeAgent) Input() chan<- string       { return f.input }
func (f *fakeAgent) Mode() agent.Mode           { return f.mode }
func (f *fakeAgent) SetMode(m agent.Mode)       { f.mode = m }

// start runs a bridge over a fake agent and returns a way to read the next
// message. The "answered" that follows an answer is left out; startAll
// keeps it.
func start(t *testing.T) (*fakeAgent, *Bridge, func() Message) {
	t.Helper()
	f, b, all := startAll(t)
	return f, b, func() Message {
		t.Helper()
		for {
			if m := all(); m.Kind != "answered" {
				return m
			}
		}
	}
}

func startAll(t *testing.T) (*fakeAgent, *Bridge, func() Message) {
	t.Helper()
	f := &fakeAgent{events: make(chan agent.Event, 16), input: make(chan string)}
	out := make(chan Message, 16)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	b := New(ctx, f, func(m Message) { out <- m })
	go b.Run()
	return f, b, func() Message {
		t.Helper()
		select {
		case m := <-out:
			return m
		case <-time.After(2 * time.Second):
			t.Fatal("no message")
			return Message{}
		}
	}
}

func TestToolCallsKeepTheirID(t *testing.T) {
	f, _, next := start(t)
	// Two read-only calls run together; output must reach the right one.
	f.events <- agent.Event{Kind: agent.EvToolStart, Tool: "run_command", Command: "df -h"}
	f.events <- agent.Event{Kind: agent.EvToolStart, Tool: "run_command", Command: "uptime"}
	f.events <- agent.Event{Kind: agent.EvOutput, Tool: "run_command", Command: "df -h", Text: "disk"}
	f.events <- agent.Event{Kind: agent.EvToolEnd, Tool: "run_command", Command: "uptime", Duration: 2 * time.Second}
	f.events <- agent.Event{Kind: agent.EvToolEnd, Tool: "run_command", Command: "df -h", Failed: true, ExitCode: 1}

	df, up := next(), next()
	if df.ID == 0 || up.ID == 0 || df.ID == up.ID {
		t.Fatalf("ids: %d %d", df.ID, up.ID)
	}
	if df.Tool != "Bash" || df.Arg != "df -h" {
		t.Errorf("label: %q %q", df.Tool, df.Arg)
	}
	if o := next(); o.Kind != "output" || o.ID != df.ID || o.Text != "disk" {
		t.Errorf("output: %+v", o)
	}
	if e := next(); e.Kind != "tool_end" || e.ID != up.ID || e.Seconds != 2 || e.Failed {
		t.Errorf("uptime end: %+v", e)
	}
	if e := next(); e.ID != df.ID || !e.Failed || e.ExitCode != 1 {
		t.Errorf("df end: %+v", e)
	}
}

func TestConfirm(t *testing.T) {
	cases := []struct {
		decision string
		always   bool
		want     agent.Decision
	}{
		{"approve", false, agent.Approve},
		{"all", false, agent.ApproveAll},
		{"all", true, agent.Approve}, // cannot be skipped: this one only
		{"deny", false, agent.Deny},
		{"", false, agent.Deny},
	}
	for _, c := range cases {
		f, b, next := start(t)
		reply := make(chan agent.Decision, 1)
		f.events <- agent.Event{Kind: agent.EvConfirm, Tool: "run_command", Command: "rm -rf x", Risk: safety.Dangerous, Always: c.always, Reply: reply}
		m := next()
		if m.Kind != "confirm" || m.Risk != "dangerous" || m.Always != c.always {
			t.Fatalf("message: %+v", m)
		}
		if err := b.Confirm(m.ID, c.decision); err != nil {
			t.Fatal(err)
		}
		if got := <-reply; got != c.want {
			t.Errorf("%q always=%v: got %v, want %v", c.decision, c.always, got, c.want)
		}
		// A second answer has nothing to answer.
		if err := b.Confirm(m.ID, "approve"); !errors.Is(err, ErrNoRequest) {
			t.Errorf("second answer: %v", err)
		}
	}
}

func TestAnswerOfTheWrongKindIsRefused(t *testing.T) {
	f, b, next := start(t)
	reply := make(chan agent.Decision, 1)
	f.events <- agent.Event{Kind: agent.EvConfirm, Command: "x", Reply: reply}
	m := next()
	if err := b.Password(m.ID, "secret"); !errors.Is(err, ErrNoRequest) {
		t.Fatalf("password on a confirm: %v", err)
	}
	if err := b.Confirm(m.ID, "deny"); err != nil {
		t.Fatalf("the request must still be pending: %v", err)
	}
}

func TestChooseKeepsOnlyOfferedIDs(t *testing.T) {
	f, b, next := start(t)
	picks := make(chan []string, 1)
	choices := []agent.Choice{{ID: "a", Label: "A", Impact: "~9 GB"}, {ID: "b", Label: "B"}}
	f.events <- agent.Event{Kind: agent.EvChoice, Text: "Which?", Choices: choices, Multi: true, Picks: picks}
	m := next()
	if len(m.Choices) != 2 || m.Choices[0].Impact != "~9 GB" || !m.Multi {
		t.Fatalf("message: %+v", m)
	}
	if err := b.Choose(m.ID, []string{"b", "zzz", "b", "a"}); err != nil {
		t.Fatal(err)
	}
	if got := <-picks; strings.Join(got, ",") != "b,a" {
		t.Errorf("picks: %v", got)
	}

	f.events <- agent.Event{Kind: agent.EvChoice, Choices: choices, Picks: picks}
	m = next()
	b.Choose(m.ID, []string{"a", "b"})
	if got := <-picks; len(got) != 1 || got[0] != "a" {
		t.Errorf("single choice: %v", got)
	}
}

func TestPassword(t *testing.T) {
	f, b, next := start(t)
	secret := make(chan string, 1)
	f.events <- agent.Event{Kind: agent.EvPassword, Text: "Password:", Failed: true, Secret: secret}
	m := next()
	if m.Kind != "password" || !m.Failed {
		t.Fatalf("message: %+v", m)
	}
	if err := b.Password(m.ID, "hunter2"); err != nil {
		t.Fatal(err)
	}
	if got := <-secret; got != "hunter2" {
		t.Errorf("secret: %q", got)
	}
}

func TestSendOnlyWhenIdle(t *testing.T) {
	f, b, next := start(t)
	if err := b.Send("hello"); !errors.Is(err, ErrBusy) {
		t.Fatalf("send while working: %v", err)
	}
	f.events <- agent.Event{Kind: agent.EvIdle}
	next()
	got := make(chan string, 1)
	go func() { got <- <-f.input }()
	if err := b.Send("  clean docker  "); err != nil {
		t.Fatal(err)
	}
	if s := <-got; s != "clean docker" {
		t.Errorf("input: %q", s)
	}
	if err := b.Send("again"); !errors.Is(err, ErrBusy) {
		t.Errorf("second send before the next idle: %v", err)
	}
}

func TestModeAndClose(t *testing.T) {
	f, b, next := start(t)
	if err := b.SetMode("readonly"); err != nil || f.mode != agent.ModeReadOnly || b.Mode() != "readonly" {
		t.Fatalf("mode: %v %v", err, f.mode)
	}
	if err := b.SetMode("root"); err == nil {
		t.Error("an unknown mode was accepted")
	}
	reply := make(chan agent.Decision, 1)
	f.events <- agent.Event{Kind: agent.EvConfirm, Command: "x", Reply: reply}
	m := next()
	close(f.events)
	if c := next(); c.Kind != "closed" {
		t.Fatalf("last message: %+v", c)
	}
	if err := b.Confirm(m.ID, "approve"); !errors.Is(err, ErrNoRequest) {
		t.Errorf("answer after the session ended: %v", err)
	}
}

func TestMarkdownDoesNotPassHTMLThrough(t *testing.T) {
	out := Markdown("**bold** <script>alert(1)</script>\n\n| a | b |\n|---|---|\n| 1 | 2 |\n")
	if !strings.Contains(out, "<strong>bold</strong>") || !strings.Contains(out, "<table>") {
		t.Errorf("markdown not rendered: %s", out)
	}
	if strings.Contains(out, "<script>") {
		t.Errorf("raw HTML passed through: %s", out)
	}
}

// An image is fetched as soon as it is drawn, with no click, and its
// address is the model's: it must never reach the window as an image.
func TestMarkdownShowsImagesAsText(t *testing.T) {
	for _, in := range []string{
		"![ok](/wails/runtime?object=0&method=0&args=%7B%7D)",
		"![status](https://example.com/p.png?d=secret)",
		"text ![a](x.png \"title\") more",
		"[![badge](https://example.com/b.svg)](https://example.com)",
		"![][ref]\n\n[ref]: https://example.com/i.png",
	} {
		out := Markdown(in)
		if strings.Contains(out, "<img") || strings.Contains(out, "src=") {
			t.Errorf("Markdown(%q) = %s: an image was drawn", in, out)
		}
	}
	if out := Markdown("![the alt](x.png)"); !strings.Contains(out, "the alt") {
		t.Errorf("the text of the image is lost: %s", out)
	}
}

// A window drawn again shows the session from its messages: an answer has
// to be one of them, and a password must not be.
func TestAnswersAreMessages(t *testing.T) {
	f, b, next := startAll(t)
	reply := make(chan agent.Decision, 1)
	f.events <- agent.Event{Kind: agent.EvConfirm, Tool: "run_command", Command: "rm x", Risk: safety.Dangerous, Reply: reply}
	m := next()
	if err := b.Confirm(m.ID, "deny"); err != nil {
		t.Fatal(err)
	}
	if a := next(); a.Kind != "answered" || a.ID != m.ID || a.Text != "deny" {
		t.Errorf("confirm: %+v", a)
	}
	secret := make(chan string, 1)
	f.events <- agent.Event{Kind: agent.EvPassword, Secret: secret}
	m = next()
	if err := b.Password(m.ID, "hunter2"); err != nil {
		t.Fatal(err)
	}
	if a := next(); a.Kind != "answered" || a.ID != m.ID || strings.Contains(a.Text, "hunter2") {
		t.Errorf("password: %+v", a)
	}
}
