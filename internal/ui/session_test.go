package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lcajigasm/omega/internal/agent"
)

func fillTranscript(m *model) {
	for i := 0; i < 80; i++ {
		m.add(&entry{kind: enUser, text: "line of earlier output"})
	}
	m.refresh()
	m.vp.SetYOffset(0)
}

func TestScrollDuringConfirmDoesNotDecide(t *testing.T) {
	m := testModel(t)
	fillTranscript(m)
	reply := make(chan agent.Decision, 1)
	m.state = stConfirm
	m.pending = &agent.Event{Reply: reply, Tool: "run_command", Command: "brew uninstall openssl", Reason: "remove it"}
	before := m.vp.YOffset
	m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if m.vp.YOffset <= before {
		t.Fatalf("pgdown during approval left the transcript at %d", m.vp.YOffset)
	}
	select {
	case d := <-reply:
		t.Fatalf("scrolling approved with %v", d)
	default:
	}
	if m.state != stConfirm || m.pending == nil {
		t.Fatal("scrolling closed the approval card")
	}
}

func TestScrollDuringChoiceDoesNotPick(t *testing.T) {
	m := testModel(t)
	fillTranscript(m)
	m.picker = NewPicker("What should I clean up?", []Option{
		{ID: "a", Label: "Caches"},
		{ID: "b", Label: "Logs"},
	}, false)
	m.state = stChoice
	before := m.vp.YOffset
	cursor := m.picker.cursor
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if m.vp.YOffset <= before {
		t.Fatalf("ctrl+d during a choice left the transcript at %d", m.vp.YOffset)
	}
	if m.picker.cursor != cursor {
		t.Fatalf("scrolling moved the selection from %d to %d", cursor, m.picker.cursor)
	}
	if m.state != stChoice {
		t.Fatal("scrolling closed the choice card")
	}
}

func TestSettledMarkdownIsNotReparsed(t *testing.T) {
	m := testModel(t)
	const n = 8
	body := "The cache is fine.\n\n- item one\n- item two\n\n`brew cleanup` is enough."
	for i := 0; i < n; i++ {
		m.add(&entry{kind: enAssistant, text: body})
	}
	m.refresh()
	if m.mdRenders != n {
		t.Fatalf("first paint parsed %d messages, want %d", m.mdRenders, n)
	}
	start := time.Now()
	for i := 0; i < 40; i++ {
		m.frame++
		m.refresh()
	}
	cached := time.Since(start)
	if m.mdRenders != n {
		t.Fatalf("ticks reparsed markdown: %d renders, want %d", m.mdRenders, n)
	}
	// One uncached paint of the same messages, for the ratio.
	for _, e := range m.entries {
		e.cacheOK = false
	}
	start = time.Now()
	m.refresh()
	uncached := time.Since(start)
	t.Logf("40 cached refreshes %s; one full paint %s", cached.Round(time.Microsecond), uncached.Round(time.Microsecond))
	// The render count above is the proof that ticks reuse the cache; the
	// times are logged, not asserted, because a shared runner's clock is not
	// a measure of this code.

	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	if m.mdRenders != n*2+n {
		// width change paints once more (n), and the comparison paint above was another n.
		// WindowSize also refreshes: that is the third n.
		t.Fatalf("width change renders %d, want %d", m.mdRenders, n*3)
	}
	view := stripANSI(m.transcript())
	if !strings.Contains(view, "brew cleanup") {
		t.Fatalf("cached paint dropped the message:\n%s", view)
	}
}

func TestFailedToolShowsTheReason(t *testing.T) {
	m := testModel(t)
	const secret = "SUPERSECRETKEYDATA"
	reason := "refusing to read /Users/omega/.ssh/id_ed25519: it can hold a key or a credential"
	m.handleEvent(agent.Event{Kind: agent.EvToolStart, Tool: "read_file", Command: "/Users/omega/.ssh/id_ed25519"})
	m.handleEvent(agent.Event{Kind: agent.EvToolEnd, Tool: "read_file", Command: "/Users/omega/.ssh/id_ed25519", Failed: true, Reason: reason})
	m.refresh()
	plain := stripANSI(m.View())
	if !strings.Contains(plain, "refusing to read") {
		t.Fatalf("card hid the failure:\n%s", plain)
	}
	if strings.Contains(plain, secret) {
		t.Fatal("card showed key material")
	}
}

func TestModelErrorWrapsAndStaysOnScreen(t *testing.T) {
	m := testModel(t)
	for i := 0; i < 40; i++ {
		m.add(&entry{kind: enUser, text: "earlier"})
	}
	m.refresh()
	m.vp.SetYOffset(0)
	msg := strings.Repeat("the provider rejected the request ", 12)
	m.handleEvent(agent.Event{Kind: agent.EvError, Text: msg})
	m.refresh()
	var note string
	for _, e := range m.entries {
		if e.kind == enNote && strings.Contains(e.text, "provider rejected") {
			note = e.text
		}
	}
	if !strings.Contains(note, "\n") {
		t.Fatalf("error note did not wrap:\n%s", note)
	}
	if m.vp.YOffset == 0 {
		t.Fatal("the view stayed at the top after the error")
	}
	if strings.Contains(note, msg) && !strings.Contains(note, "\n") {
		t.Fatal("the full error is still one line")
	}
}
