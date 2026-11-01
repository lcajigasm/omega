package ui

import (
	"os"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/aymanbagabas/go-osc52/v2"
	"github.com/charmbracelet/lipgloss"

	"github.com/lcajigasm/omega/internal/i18n"
)

// copyClipboard writes text to the clipboard. Tests replace it.
var copyClipboard = writeClipboard

// writeClipboard copies text with OSC 52 (so a remote terminal receives it)
// and with the OS clipboard when one is available. Either path is enough.
func writeClipboard(text string) error {
	oscErr := writeOSC52(text)
	clipErr := clipboard.WriteAll(text)
	if oscErr == nil || clipErr == nil {
		return nil
	}
	return clipErr
}

func writeOSC52(text string) error {
	seq := osc52.New(text)
	f, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		_, err = seq.WriteTo(os.Stdout)
		return err
	}
	defer f.Close()
	_, err = seq.WriteTo(f)
	return err
}

func (m *model) toolIndexes() []int {
	var idx []int
	for i, e := range m.entries {
		if e.kind == enTool {
			idx = append(idx, i)
		}
	}
	return idx
}

func (m *model) focusedEntry() *entry {
	if m.focus < 0 || m.focus >= len(m.entries) {
		return nil
	}
	e := m.entries[m.focus]
	if e.kind != enTool {
		return nil
	}
	return e
}

// focusedText is what ctrl+y copies: the tool's output, or the reason it
// was denied.
func (m *model) focusedText() (string, bool) {
	e := m.focusedEntry()
	if e == nil {
		return "", false
	}
	if e.denied != "" {
		return e.denied, true
	}
	if len(e.output) == 0 {
		return "", false
	}
	return strings.Join(e.output, "\n"), true
}

func (m *model) moveFocus(delta int) {
	idx := m.toolIndexes()
	if len(idx) == 0 {
		return
	}
	pos := 0
	found := false
	for i, at := range idx {
		if at == m.focus {
			pos = i
			found = true
		}
	}
	if !found {
		if delta < 0 {
			pos = len(idx) - 1
		}
	} else {
		pos += delta
		if pos < 0 {
			pos = 0
		}
		if pos >= len(idx) {
			pos = len(idx) - 1
		}
	}
	m.focus = idx[pos]
	m.refresh()
}

func (m *model) copyFocused() {
	text, ok := m.focusedText()
	if !ok {
		m.note(mutedStyle.Render(i18n.T("ui.copy_empty")), lipgloss.NewStyle())
		m.refresh()
		return
	}
	if err := copyClipboard(text); err != nil {
		m.note(errStyle.Render("! ")+mutedStyle.Render(i18n.T("ui.copy_failed")), lipgloss.NewStyle())
		m.refresh()
		return
	}
	m.note(okStyle.Render("✓ ")+mutedStyle.Render(i18n.T("ui.copied")), lipgloss.NewStyle())
	m.refresh()
}
