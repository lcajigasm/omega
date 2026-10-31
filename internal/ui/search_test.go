package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lcajigasm/omega/internal/agent"
)

func TestTranscriptSearch(t *testing.T) {
	m := testModel(t)
	m.add(&entry{kind: enUser, text: "alpha cache one"})
	for i := 0; i < 30; i++ {
		m.add(&entry{kind: enUser, text: "filler line"})
	}
	m.add(&entry{kind: enUser, text: "omega cache two"})
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 14})

	m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	if !m.searchOn {
		t.Fatal("ctrl+f did not open search")
	}
	for _, r := range "cache" {
		m.Update(runeKey(r))
	}
	view := m.View()
	if !strings.Contains(view, "\x1b[7mcache\x1b[27m") {
		t.Fatalf("match is not highlighted:\n%s", view)
	}
	plain := stripANSI(view)
	if !strings.Contains(plain, "search: cache") || !strings.Contains(plain, "1/2") {
		t.Fatalf("search bar:\n%s", plain)
	}
	first := m.vp.YOffset

	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.vp.YOffset <= first {
		t.Fatalf("next match did not move down: %d -> %d", first, m.vp.YOffset)
	}
	if !strings.Contains(stripANSI(m.View()), "2/2") {
		t.Fatalf("search count after next:\n%s", stripANSI(m.View()))
	}
	second := m.vp.YOffset

	m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	if m.vp.YOffset != first {
		t.Fatalf("previous match offset %d, want %d (second was %d)", m.vp.YOffset, first, second)
	}
	if m.header.Model != "test-model" {
		t.Fatalf("ctrl+p during search switched model to %s", m.header.Model)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.searchOn {
		t.Fatal("esc did not close search")
	}
	if strings.Contains(m.View(), "\x1b[7m") {
		t.Fatal("highlight remained after closing search")
	}
}

func TestSearchDoesNotApproveACard(t *testing.T) {
	m := testModel(t)
	m.add(&entry{kind: enUser, text: "brew upgrade"})
	reply := make(chan agent.Decision, 1)
	m.state = stConfirm
	m.pending = &agent.Event{Reply: reply, Tool: "run_command", Command: "brew upgrade"}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	m.Update(runeKey('y'))
	select {
	case d := <-reply:
		t.Fatalf("typing in search approved with %v", d)
	default:
	}
	if m.searchQ != "y" {
		t.Fatalf("query %q", m.searchQ)
	}
}
