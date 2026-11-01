package ui

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/aymanbagabas/go-osc52/v2"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/lcajigasm/omega/internal/agent"
)

func useClipboard(t *testing.T, fn func(string) error) {
	t.Helper()
	prev := copyClipboard
	copyClipboard = fn
	t.Cleanup(func() { copyClipboard = prev })
}

func TestCopyFocusedTool(t *testing.T) {
	m := testModel(t)
	var got string
	useClipboard(t, func(s string) error {
		got = s
		return nil
	})
	m.add(&entry{kind: enTool, tool: "run_step:brew-outdated", command: "brew outdated", output: []string{"node (24.9.0) < 24.11.1", "gh (2.81.0) < 2.83.0"}})
	m.add(&entry{kind: enTool, tool: "run_command", command: "pipx reinstall-all", output: []string{"installed httpie"}})

	m.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	if got != "installed httpie" {
		t.Fatalf("copied the latest tool as %q", got)
	}
	plain := stripANSI(m.View())
	if !strings.Contains(plain, "Copied to the clipboard") {
		t.Fatal("copy confirmation missing")
	}
	if !strings.Contains(plain, "▸") {
		t.Fatalf("focused tool is not marked:\n%s", plain)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyUp, Alt: true})
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	if got != "node (24.9.0) < 24.11.1\ngh (2.81.0) < 2.83.0" {
		t.Fatalf("copied the previous tool as %q", got)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyDown, Alt: true})
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	if got != "installed httpie" {
		t.Fatalf("alt+down did not return to the latest tool, copied %q", got)
	}
}

func TestCopyDoesNotApprove(t *testing.T) {
	m := testModel(t)
	useClipboard(t, func(string) error { return nil })
	m.add(&entry{kind: enTool, tool: "run_command", command: "brew upgrade", output: []string{"ok"}})
	reply := make(chan agent.Decision, 1)
	m.state = stConfirm
	m.pending = &agent.Event{Reply: reply, Tool: "run_command", Command: "brew upgrade"}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	select {
	case d := <-reply:
		t.Fatalf("ctrl+y decided %v", d)
	default:
	}
}

func TestCopyEmptyAndFailure(t *testing.T) {
	m := testModel(t)
	useClipboard(t, func(string) error { return errors.New("no clipboard") })
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	if !strings.Contains(stripANSI(m.View()), "Nothing to copy") {
		t.Fatal("empty copy was silent")
	}
	m.add(&entry{kind: enTool, tool: "run_command", command: "true", output: []string{"ok"}})
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	if !strings.Contains(stripANSI(m.View()), "Could not copy") {
		t.Fatal("failed copy was silent")
	}
}

func TestCopyDeniedReason(t *testing.T) {
	m := testModel(t)
	var got string
	useClipboard(t, func(s string) error { got = s; return nil })
	m.add(&entry{kind: enTool, tool: "run_command", command: "rm -rf ~/Library/Caches", denied: "The user denied this action."})
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	if got != "The user denied this action." {
		t.Fatalf("copied %q", got)
	}
}

func TestHelpShowsCopyShortcut(t *testing.T) {
	m := testModel(t)
	m.Update(runeKey('?'))
	view := stripANSI(m.View())
	if !strings.Contains(view, "ctrl+y") || !strings.Contains(view, "Copy the focused tool output") {
		t.Fatalf("help is missing the copy shortcut:\n%s", view)
	}
	if !strings.Contains(view, "alt+up/down") {
		t.Fatal("help is missing focus movement")
	}
}

func TestOSC52CarriesTheText(t *testing.T) {
	seq := osc52.New("node 24.11.1\n").String()
	const prefix = "\x1b]52;c;"
	i := strings.Index(seq, prefix)
	if i < 0 {
		t.Fatalf("sequence %q", seq)
	}
	rest := seq[i+len(prefix):]
	end := strings.IndexAny(rest, "\x07\x1b")
	if end < 0 {
		t.Fatalf("no terminator in %q", seq)
	}
	raw, err := base64.StdEncoding.DecodeString(rest[:end])
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "node 24.11.1\n" {
		t.Fatalf("decoded %q", raw)
	}
}
