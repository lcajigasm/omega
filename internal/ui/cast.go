package ui

import (
	"bytes"
	"encoding/json"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/safety"
)

const (
	castWidth  = 100
	castHeight = 32
)

// shortcutsCast renders a short session: the shortcut card, Esc closing it,
// and a denied command. The bytes are an asciinema v2 recording.
func shortcutsCast(m *model) ([]byte, error) {
	lipgloss.SetHasDarkBackground(true)
	lipgloss.SetColorProfile(termenv.TrueColor)
	m.dark = true
	m.started = time.Now()
	m.header.Model = "claude-haiku-5-5"
	m.header.Provider = "anthropic"
	m.header.Task = "update"
	m.state = stRunning
	m.add(&entry{kind: enUser, text: "update what is outdated"})
	m.add(&entry{kind: enAssistant, text: "Checking Homebrew."})
	m.add(&entry{
		kind: enTool, tool: "run_step:brew-outdated", command: "brew outdated --verbose",
		output: []string{"node (24.9.0) < 24.11.1", "gh (2.81.0) < 2.83.0"}, lines: 2,
		duration: 2 * time.Second,
	})
	m.Update(tea.WindowSizeMsg{Width: castWidth, Height: castHeight})

	var frames []string
	shot := func() {
		frames = append(frames, "\x1b[?25l\x1b[H\x1b[2J"+m.View())
	}
	shot()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	shot()
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	shot()

	reply := make(chan agent.Decision, 1)
	m.Update(agent.Event{
		Kind: agent.EvConfirm, Tool: "run_command",
		Command: "pipx reinstall-all --python python3.13",
		Reason:  "Rebuild the pipx environments against the current Homebrew Python",
		Risk:    safety.Dangerous, Reply: reply,
	})
	shot()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m.Update(agent.Event{
		Kind: agent.EvDenied, Tool: "run_command",
		Command: "pipx reinstall-all --python python3.13",
		Reason:  "The user denied this action. Do not retry it or an equivalent; carry on with the rest.",
	})
	shot()

	type step struct {
		at   float64
		kind string
		data string
	}
	steps := []step{
		{0.0, "o", frames[0]},
		{1.2, "i", "?"},
		{1.4, "o", frames[1]},
		{3.0, "i", "\x1b"},
		{3.2, "o", frames[2]},
		{4.6, "o", frames[3]},
		{6.0, "i", "n"},
		{6.2, "o", frames[4]},
	}
	header, err := json.Marshal(map[string]any{
		"version":   2,
		"width":     castWidth,
		"height":    castHeight,
		"timestamp": 1762000000,
		"title":     "omega: help, Esc and a denial",
		"env":       map[string]string{"TERM": "xterm-256color", "SHELL": "/bin/zsh"},
	})
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.Write(header)
	buf.WriteByte('\n')
	for _, s := range steps {
		line, err := json.Marshal([]any{s.at, s.kind, s.data})
		if err != nil {
			return nil, err
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}
