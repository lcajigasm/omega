package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/i18n"
)

// cardFrame is the border plus the horizontal padding of a card, in columns.
const cardFrame = 2

// clickCard selects or confirms the card under a left click. Other mouse
// events, including the wheel, stay with the transcript.
func (m *model) clickCard(msg tea.MouseMsg) bool {
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
		return false
	}
	if m.help || !m.ready || (m.state != stChoice && m.state != stConfirm) {
		return false
	}
	top := m.vp.Height
	relY := msg.Y - top
	relX := msg.X - cardFrame
	if relY < 0 || relX < 0 {
		return false
	}
	switch m.state {
	case stChoice:
		if m.picker == nil {
			return false
		}
		done, cancelled := m.picker.Click(msg.X, relY, m.width)
		if done {
			m.finishChoice(cancelled)
			return true
		}
		// A click on the card's border or a blank row still belongs to the card.
		_, lines := m.picker.layout(m.width)
		if relY <= len(lines)+1 {
			m.refresh()
			return true
		}
		return false
	case stConfirm:
		_, keyLine, buttons := m.confirmLayout()
		if relY-1 != keyLine {
			if relY <= keyLine+2 {
				return true
			}
			return false
		}
		for _, b := range buttons {
			if relX >= b.start && relX < b.end {
				m.decide(b.decision)
				return true
			}
		}
		return true
	}
	return false
}

func (m *model) decide(d agent.Decision) {
	if m.pending == nil || m.pending.Reply == nil {
		return
	}
	if d == agent.ApproveAll {
		if m.pending.Always {
			return
		}
		m.mode = agent.ModeAuto
	}
	m.pending.Reply <- d
	m.pending = nil
	m.state = stRunning
	m.refresh()
}

func (m *model) finishChoice(cancelled bool) tea.Cmd {
	if m.onPick != nil {
		ids := m.picker.Selected()
		pick := m.onPick
		m.picker, m.onPick = nil, nil
		m.state = stIdle
		if !cancelled && len(ids) > 0 {
			pick(ids[0])
		}
		m.refresh()
		return m.input.Focus()
	}
	var ids []string
	if m.picker != nil && !cancelled {
		ids = m.picker.Selected()
	}
	var labels []string
	if m.picker != nil {
		for _, id := range ids {
			for _, o := range m.picker.Options {
				if o.ID == id {
					labels = append(labels, o.Label)
				}
			}
		}
	}
	if len(labels) == 0 {
		m.note(mutedStyle.Render("› "+i18n.T("ui.chosen_none")), lipgloss.NewStyle())
	} else {
		m.note(titleStyle.Render("› ")+i18n.T("ui.chosen", strings.Join(labels, ", ")), lipgloss.NewStyle())
	}
	if m.pending != nil && m.pending.Picks != nil {
		m.pending.Picks <- ids
	}
	m.pending, m.picker = nil, nil
	m.state = stRunning
	m.refresh()
	return nil
}
