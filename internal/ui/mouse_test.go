package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/safety"
)

func clickAt(view, needle string) (tea.MouseMsg, bool) {
	for y, line := range strings.Split(stripANSI(view), "\n") {
		if x := strings.Index(line, needle); x >= 0 {
			return tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}, true
		}
	}
	return tea.MouseMsg{}, false
}

func TestClickSelectsChoiceAndKeyLineOpensIt(t *testing.T) {
	m := testModel(t)
	picked := ""
	m.openPicker("Model", []Option{
		{ID: "haiku", Label: "Haiku"},
		{ID: "sonnet", Label: "Sonnet", Detail: "stronger"},
	}, func(id string) { picked = id })

	view := m.View()
	msg, ok := clickAt(view, "Sonnet")
	if !ok {
		t.Fatalf("Sonnet not on screen:\n%s", stripANSI(view))
	}
	m.Update(msg)
	if m.picker == nil || m.picker.cursor != 1 {
		t.Fatalf("cursor = %v, want the Sonnet row", m.picker)
	}
	selected := stripANSI(m.View())
	if !strings.Contains(selected, "❯") || strings.Index(selected, "❯") > strings.Index(selected, "Sonnet") {
		t.Fatalf("Sonnet is not the selected row:\n%s", selected)
	}

	msg, ok = clickAt(m.View(), "enter choose")
	if !ok {
		t.Fatalf("confirm line missing:\n%s", stripANSI(m.View()))
	}
	m.Update(msg)
	if picked != "sonnet" {
		t.Fatalf("picked %q, want sonnet", picked)
	}
	if m.state != stIdle {
		t.Fatalf("state %v after opening the card", m.state)
	}
}

func TestSecondClickOnARowOpensIt(t *testing.T) {
	p := NewPicker("Model", []Option{{ID: "a", Label: "Alpha"}, {ID: "b", Label: "Beta"}}, false)
	_, lines := p.layout(60)
	y := -1
	for i, line := range lines {
		if line.role == roleRow && line.row == 1 {
			y = i + 1 // content row sits under the top border
		}
	}
	if y < 0 {
		t.Fatal("Beta row missing")
	}
	if done, _ := p.Click(4, y, 60); done || p.cursor != 1 {
		t.Fatalf("first click done=%v cursor=%d", done, p.cursor)
	}
	if done, _ := p.Click(4, y, 60); !done || p.Selected()[0] != "b" {
		t.Fatalf("second click did not open Beta: done=%v selected=%v", done, p.Selected())
	}
}

func TestClickTogglesACheckbox(t *testing.T) {
	p := NewPicker("Apply", []Option{
		{ID: "brew", Label: "Upgrade"},
		{ID: "cache", Label: "Caches"},
	}, true)
	_, lines := p.layout(60)
	y := -1
	keys := -1
	for i, line := range lines {
		if line.role == roleRow && line.row == 1 {
			y = i + 1
		}
		if line.role == roleKeys {
			keys = i + 1
		}
	}
	if done, _ := p.Click(4, y, 60); done || !p.Options[1].Selected {
		t.Fatalf("click did not tick Caches: done=%v selected=%v", done, p.Options[1].Selected)
	}
	if done, cancelled := p.Click(4, keys, 60); !done || cancelled {
		t.Fatalf("key line click = done %v cancelled %v", done, cancelled)
	}
	if got := p.Selected(); len(got) != 1 || got[0] != "cache" {
		t.Fatalf("selected %v", got)
	}
}

func TestClickApprovalButtons(t *testing.T) {
	m := testModel(t)
	reply := make(chan agent.Decision, 1)
	m.state = stConfirm
	m.pending = &agent.Event{
		Reply: reply, Tool: "run_command", Command: "brew upgrade", Risk: safety.Normal,
	}
	m.refresh()
	view := m.View()
	msg, ok := clickAt(view, "[n]")
	if !ok {
		t.Fatalf("deny button missing:\n%s", stripANSI(view))
	}
	m.Update(msg)
	select {
	case d := <-reply:
		if d != agent.Deny {
			t.Fatalf("decision %v, want deny", d)
		}
	default:
		t.Fatalf("click on [n] sent no decision\n%s", stripANSI(m.View()))
	}
	if m.state != stRunning {
		t.Fatalf("state %v after deny", m.state)
	}

	reply = make(chan agent.Decision, 1)
	m.state = stConfirm
	m.pending = &agent.Event{
		Reply: reply, Tool: "run_command", Command: "brew upgrade", Risk: safety.Dangerous,
	}
	m.refresh()
	msg, ok = clickAt(m.View(), "[y]")
	if !ok {
		t.Fatal("[y] missing")
	}
	m.Update(msg)
	select {
	case d := <-reply:
		if d != agent.Approve {
			t.Fatalf("decision %v, want approve", d)
		}
	default:
		t.Fatal("click on [y] sent no decision")
	}
}
