package ui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/i18n"
)

// runningEntry finds the tool card a line belongs to. Read-only calls from
// one answer run together, so output is matched by tool and command rather
// than assumed to belong to whichever card started last.
func (m *model) runningEntry(tool, command string) *entry {
	for i := len(m.entries) - 1; i >= 0; i-- {
		e := m.entries[i]
		if e.kind != enTool || !e.running || e.tool != tool {
			continue
		}
		if command != "" && e.command != command {
			continue
		}
		return e
	}
	return nil
}

func (m *model) add(e *entry) {
	m.entries = append(m.entries, e)
	if e.kind == enTool {
		m.focus = len(m.entries) - 1
	}
}

func (m *model) note(text string, style lipgloss.Style) {
	m.add(&entry{kind: enNote, text: text, style: style})
}

// handleEvent takes one event of the agent.
func (m *model) handleEvent(e agent.Event) tea.Cmd {
	if e.Stats.Model != "" {
		m.stats = e.Stats
	}
	switch e.Kind {
	case agent.EvThinking:
		m.thinking = true
		m.thinkStart = time.Now()
		m.verb = m.verbs[int(time.Now().UnixNano()/7)%len(m.verbs)]
	case agent.EvAssistant:
		m.thinking = false
		m.add(&entry{kind: enAssistant, text: e.Text})
	case agent.EvToolStart:
		m.thinking = false
		now := time.Now()
		ent := &entry{kind: enTool, tool: e.Tool, command: e.Command, running: true, started: now, lastOut: now}
		m.add(ent)
		m.current = ent
	case agent.EvOutput:
		if ent := m.runningEntry(e.Tool, e.Command); ent != nil {
			ent.output = append(ent.output, e.Text)
			ent.lastOut = time.Now()
			if len(ent.output) > maxKept {
				ent.output = ent.output[len(ent.output)-maxKept:]
			}
		}
	case agent.EvToolEnd:
		cur := m.runningEntry(e.Tool, e.Command)
		if cur == nil {
			cur = &entry{kind: enTool, tool: e.Tool, command: e.Command}
			m.add(cur)
		}
		cur.running, cur.failed, cur.exitCode, cur.duration, cur.lines = false, e.Failed, e.ExitCode, e.Duration, e.Lines
		if free, _ := diskSpace("/"); free > 0 {
			m.diskNow = free
		}
		if e.Reason != "" {
			cur.errText = e.Reason
			m.follow = true
		}
		m.current = nil
		for i := len(m.entries) - 1; i >= 0; i-- {
			if m.entries[i].kind == enTool && m.entries[i].running {
				m.current = m.entries[i]
				break
			}
		}
	case agent.EvDenied:
		m.add(&entry{kind: enTool, tool: e.Tool, command: e.Command, denied: e.Reason})
	case agent.EvConfirm:
		m.thinking = false
		ev := e
		m.pending = &ev
		m.state = stConfirm
	case agent.EvChoice:
		m.thinking = false
		opts := make([]Option, len(e.Choices))
		for i, c := range e.Choices {
			opts[i] = Option{ID: c.ID, Label: c.Label, Detail: c.Detail, Badge: c.Impact, Risk: c.Risk}
		}
		m.picker = NewPicker(e.Text, opts, e.Multi)
		ev := e
		m.pending = &ev
		m.state = stChoice
	case agent.EvPassword:
		m.thinking = false
		ev := e
		m.pending = &ev
		m.state = stPassword
		m.password.Reset()
		return m.password.Focus()
	case agent.EvModel:
		m.header.Model = e.Text
		m.note(titleStyle.Render(gl.Up+" ")+mutedStyle.Render(i18n.T("ui.model_switch", e.Text)), lipgloss.NewStyle())
	case agent.EvMode:
		m.mode = e.Mode
		m.note(okStyle.Render(gl.Lifted+" ")+mutedStyle.Render(i18n.T("ui.mode_lifted")), lipgloss.NewStyle())
	case agent.EvIdle:
		m.thinking = false
		m.state = stIdle
		m.input.Reset()
		return m.input.Focus()
	case agent.EvError:
		m.thinking = false
		text := strings.TrimSpace(e.Text)
		if text == "" {
			break
		}
		m.note(m.wrapPlain(gl.Error+" "+text), errStyle)
		m.follow = true
	case agent.EvDone:
		m.thinking = false
		m.state = stDone
		if e.Text != "" {
			m.note(e.Text, mutedStyle)
		}
		m.note(summaryLine(m.stats.Elapsed, m.stats.Usage.Input()+m.stats.Usage.OutputTokens, m.costText()), mutedStyle)
		if m.quitting {
			return m.leave()
		}
	}
	return nil
}
