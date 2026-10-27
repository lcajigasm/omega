package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func keys(p *Picker, ks ...tea.KeyMsg) (done, cancelled bool) {
	for _, k := range ks {
		done, cancelled = p.HandleKey(k)
	}
	return
}

func TestPickerMulti(t *testing.T) {
	p := NewPicker("t", []Option{{ID: "a"}, {ID: "b"}, {ID: "c"}}, true)
	done, cancelled := keys(p,
		tea.KeyMsg{Type: tea.KeySpace}, tea.KeyMsg{Type: tea.KeyDown}, tea.KeyMsg{Type: tea.KeyDown},
		tea.KeyMsg{Type: tea.KeySpace}, tea.KeyMsg{Type: tea.KeyEnter})
	if !done || cancelled {
		t.Fatal("expected confirmation")
	}
	if got := p.Selected(); len(got) != 2 || got[0] != "a" || got[1] != "c" {
		t.Fatalf("selected %v", got)
	}
}

func TestPickerSingleFilter(t *testing.T) {
	p := NewPicker("t", []Option{{ID: "claude-haiku", Label: "claude-haiku"}, {ID: "gpt-5.5", Label: "gpt-5.5"}}, false)
	keys(p, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("gpt")})
	if got := p.Selected(); len(got) != 1 || got[0] != "gpt-5.5" {
		t.Fatalf("filter selected %v", got)
	}
	if done, cancelled := keys(p, tea.KeyMsg{Type: tea.KeyEsc}, tea.KeyMsg{Type: tea.KeyEsc}); !done || !cancelled {
		t.Fatal("esc twice should clear the filter, then cancel")
	}
}
