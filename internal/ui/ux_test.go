package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/safety"
)

// Enter approves a normal command but not a dangerous one: that takes y,
// and the card says so.
func TestEnterDoesNotApproveADangerousCommand(t *testing.T) {
	m := testModel(t)
	stableLook(t)
	reply := make(chan agent.Decision, 1)
	m.state = stConfirm
	m.pending = &agent.Event{Reply: reply, Tool: "run_command", Command: "rm -rf ~/x", Risk: safety.Dangerous}
	if view := stripANSI(m.View()); !strings.Contains(view, "enter does not approve") || !strings.Contains(view, gl.Warn+" Run this?") {
		t.Fatalf("the dangerous card does not warn:\n%s", view)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	select {
	case d := <-reply:
		t.Fatalf("enter answered %v", d)
	default:
	}
	m.Update(runeKey('y'))
	if d := <-reply; d != agent.Approve {
		t.Fatalf("y answered %v", d)
	}

	reply = make(chan agent.Decision, 1)
	m.state = stConfirm
	m.pending = &agent.Event{Reply: reply, Tool: "run_command", Command: "brew upgrade", Risk: safety.Normal}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if d := <-reply; d != agent.Approve {
		t.Fatalf("enter on a normal command answered %v", d)
	}
}

// The approval keys are shown with the yes letter of the user's language;
// y still works.
func TestConfirmKeysFollowTheLanguage(t *testing.T) {
	m := testModel(t)
	stableLook(t)
	i18n.Init("es")
	t.Cleanup(func() { i18n.Init("en") })
	reply := make(chan agent.Decision, 2)
	m.state = stConfirm
	m.pending = &agent.Event{Reply: reply, Tool: "run_command", Command: "brew upgrade", Risk: safety.Normal}
	if view := stripANSI(m.View()); !strings.Contains(view, "s sí · a sí a todo · n no") {
		t.Fatalf("the keys are not in Spanish:\n%s", view)
	}
	m.Update(runeKey('s'))
	if d := <-reply; d != agent.Approve {
		t.Fatalf("s answered %v", d)
	}
	m.state = stConfirm
	m.pending = &agent.Event{Reply: reply, Tool: "run_command", Command: "brew upgrade", Risk: safety.Normal}
	m.Update(runeKey('y'))
	if d := <-reply; d != agent.Approve {
		t.Fatalf("y answered %v", d)
	}
}

// The first ctrl+c stops what runs and answers the card; the second leaves.
func TestCtrlCCancelsThenLeaves(t *testing.T) {
	m := testModel(t)
	stableLook(t)
	cancelled := 0
	m.cancel = func() { cancelled++ }
	reply := make(chan agent.Decision, 1)
	m.state = stConfirm
	m.pending = &agent.Event{Reply: reply, Tool: "run_command", Command: "brew upgrade", Risk: safety.Normal}
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC}); cmd != nil {
		t.Fatal("the first ctrl+c left")
	}
	if d := <-reply; d != agent.Deny || cancelled != 1 {
		t.Fatalf("first ctrl+c: decision %v, cancels %d", d, cancelled)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "ctrl+c again to leave") {
		t.Fatalf("the screen does not say how to leave:\n%s", view)
	}
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC}); cmd == nil {
		t.Fatal("the second ctrl+c did not leave")
	}
}

// A key line too wide for the terminal drops hints from the end, never
// help or quit, and wraps rather than cut what must stay.
func TestHintLineKeepsWhatMustStay(t *testing.T) {
	stableLook(t)
	hs := []hint{hk("↑↓", "uix.move"), hk("enter", "uix.open_run"), hk("→", "uix.options"),
		hint{action: i18n.T("uix.type_search")}, hk("?", "uix.help").must(), hk("q", "uix.quit").must()}
	for _, w := range []int{120, 50, 30, 14} {
		got := stripANSI(hintLine(w, hs...))
		if !strings.Contains(got, "? help") || !strings.Contains(got, "q quit") {
			t.Errorf("width %d lost help or quit: %q", w, got)
		}
		for _, l := range strings.Split(got, "\n") {
			if lipgloss.Width(l) > max(w, 12) {
				t.Errorf("width %d: line %q is %d wide", w, l, lipgloss.Width(l))
			}
		}
	}
	if got := stripANSI(hintLine(120, hs...)); !strings.Contains(got, "↑↓ move · enter open or run") {
		t.Errorf("a wide line keeps every hint: %q", got)
	}
}

// A narrow status bar gives up the mode's key, then the model, then the
// tokens; the cost and the time stay.
func TestStatusBarDegrades(t *testing.T) {
	m := testModel(t)
	stableLook(t)
	m.header.Model = "claude-sonnet-very-long-model-name"
	m.stats.Cost, m.stats.CostKnown = 0.25, true
	for _, w := range []int{120, 70, 50, 40} {
		m.width = w
		bar := stripANSI(m.statusBar())
		if !strings.Contains(bar, "$0.25") || !strings.Contains(bar, "0s") {
			t.Errorf("width %d dropped the cost or the time: %q", w, bar)
		}
		if lipgloss.Width(bar) > w {
			t.Errorf("width %d: the bar is %d wide: %q", w, lipgloss.Width(bar), bar)
		}
	}
	m.width = 120
	if bar := stripANSI(m.statusBar()); !strings.Contains(bar, "claude-sonnet") || !strings.Contains(bar, "(shift+tab)") {
		t.Errorf("a wide bar shows everything: %q", bar)
	}
}

// Lines that arrive while the user reads further up are counted, and the
// notice says how to get to them.
func TestNewLinesNotice(t *testing.T) {
	m := testModel(t)
	stableLook(t)
	fillTranscript(m)
	m.state = stRunning
	m.add(&entry{kind: enUser, text: "new one"})
	m.add(&entry{kind: enUser, text: "new two"})
	m.refresh()
	if m.unseen == 0 {
		t.Fatal("new lines were not counted")
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "new lines") || !strings.Contains(view, "end go to the end") {
		t.Fatalf("no notice of the new lines:\n%s", view)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	if !m.vp.AtBottom() || m.unseen != 0 {
		t.Fatal("end did not go to the bottom")
	}
}

// A running tool that has printed nothing for a while says so.
func TestSilentToolIsShown(t *testing.T) {
	m := testModel(t)
	stableLook(t)
	m.handleEvent(agent.Event{Kind: agent.EvToolStart, Tool: "run_command", Command: "softwareupdate -ia"})
	e := m.entries[len(m.entries)-1]
	if strings.Contains(stripANSI(m.paintEntry(e)), "no output") {
		t.Fatal("a tool that just started is not silent")
	}
	e.lastOut = time.Now().Add(-45 * time.Second)
	if got := stripANSI(m.paintEntry(e)); !strings.Contains(got, "no output for 45s") {
		t.Fatalf("a silent tool is not shown:\n%s", got)
	}
}

// Lists go round at both ends and say what is out of sight.
func TestPickerWrapsAndShowsMore(t *testing.T) {
	stableLook(t)
	var opts []Option
	for i := 0; i < 20; i++ {
		opts = append(opts, Option{ID: string(rune('a' + i)), Label: "Option " + string(rune('A'+i))})
	}
	p := NewPicker("Pick", opts, false)
	p.HandleKey(tea.KeyMsg{Type: tea.KeyUp})
	if p.cursor != 19 {
		t.Fatalf("up from the top went to %d", p.cursor)
	}
	p.HandleKey(tea.KeyMsg{Type: tea.KeyDown})
	if p.cursor != 0 {
		t.Fatalf("down from the bottom went to %d", p.cursor)
	}
	p.setHeight(14)
	view := stripANSI(p.View(80))
	if !strings.Contains(view, "↓ 14 more") {
		t.Fatalf("the rows below are not counted:\n%s", view)
	}
	p.cursor = 10
	if view = stripANSI(p.View(80)); !strings.Contains(view, "↑ 5 more") {
		t.Fatalf("the rows above are not counted:\n%s", view)
	}
}

// Below compactWidth a card loses its border and nothing runs off the edge.
func TestNarrowScreensFit(t *testing.T) {
	m := testModel(t)
	stableLook(t)
	p := NewPicker("Choose a model for this session", []Option{
		{ID: "a", Label: "claude-haiku-5-5", Badge: "$1/M", Detail: "fast and cheap"},
		{ID: "b", Label: "claude-sonnet-5-5", Badge: "current"},
	}, false)
	for _, l := range strings.Split(stripANSI(p.View(36)), "\n") {
		if lipgloss.Width(l) > 36 {
			t.Errorf("picker line %q is wider than 36", l)
		}
	}
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 24})
	m.state = stDone
	for _, l := range strings.Split(stripANSI(m.View()), "\n") {
		if lipgloss.Width(l) > 40 {
			t.Errorf("session line %q is wider than 40", l)
		}
	}
	g := testGuide(t, 40, 24, nil)
	for _, l := range strings.Split(stripANSI(g.View()), "\n") {
		if lipgloss.Width(l) > 40 {
			t.Errorf("plan line %q is wider than 40", l)
		}
	}
}

// TERM=dumb and OMEGA_ASCII=1 draw with ASCII only.
func TestASCIIGlyphs(t *testing.T) {
	t.Setenv("OMEGA_ASCII", "1")
	if !asciiMode() || pickGlyphs().Tool != "*" {
		t.Fatal("OMEGA_ASCII=1 does not pick the ASCII set")
	}
	t.Setenv("OMEGA_ASCII", "")
	t.Setenv("TERM", "dumb")
	if !asciiMode() {
		t.Fatal("TERM=dumb does not pick the ASCII set")
	}
	setGlyphs(true)
	defer setGlyphs(false)
	m := testModel(t)
	m.handleEvent(agent.Event{Kind: agent.EvToolStart, Tool: "run_command", Command: "ls"})
	m.handleEvent(agent.Event{Kind: agent.EvToolEnd, Tool: "run_command", Command: "ls"})
	m.state = stDone
	m.refresh()
	for _, r := range stripANSI(m.View()) {
		if r > 126 && r != '…' {
			t.Fatalf("non-ASCII %q on an ASCII screen:\n%s", r, stripANSI(m.View()))
		}
	}
}

// One size formatter, with the decimal comma where the language uses one.
func TestBytes(t *testing.T) {
	t.Cleanup(func() { i18n.Init("en") })
	i18n.Init("en")
	for n, want := range map[int64]string{512: "512 B", 34 << 10: "34 KB", 512 << 20: "512 MB", 3<<30 + 400<<20: "3.4 GB"} {
		if got := Bytes(n); got != want {
			t.Errorf("Bytes(%d) = %q, want %q", n, got, want)
		}
	}
	i18n.Init("es")
	if got := Bytes(3<<30 + 400<<20); got != "3,4 GB" {
		t.Errorf("in Spanish: %q", got)
	}
}

// The transcript and the search over it are reused while nothing changed.
func TestSearchIsCached(t *testing.T) {
	m := testModel(t)
	m.add(&entry{kind: enUser, text: "alpha cache one"})
	m.add(&entry{kind: enUser, text: "omega cache two"})
	m.searchOn, m.searchQ = true, "cache"
	m.refresh()
	first := m.sc
	m.View()
	m.refresh()
	if m.sc.painted != first.painted || len(m.sc.lines) != 2 {
		t.Fatalf("search cache: %+v", m.sc.lines)
	}
	m.add(&entry{kind: enUser, text: "third cache"})
	m.refresh()
	if len(m.sc.lines) != 3 {
		t.Fatalf("a new entry was not searched: %v", m.sc.lines)
	}
}

func TestWantTUIRespectsTheFlagAndTERM(t *testing.T) {
	if WantTUI(true) {
		t.Error("--no-tui still wants the screen")
	}
	t.Setenv("TERM", "dumb")
	if WantTUI(false) {
		t.Error("TERM=dumb wants the screen")
	}
}
