// Package ui renders an agent run, either as a full-screen Bubble Tea TUI or
// as plain line-oriented output for pipes and CI.
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/safety"
)

var (
	accent    = lipgloss.AdaptiveColor{Light: "#5A4FCF", Dark: "#9D8CFF"}
	muted     = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#8B93A1"}
	okColor   = lipgloss.AdaptiveColor{Light: "#15803D", Dark: "#4ADE80"}
	errColor  = lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#F87171"}
	warnColor = lipgloss.AdaptiveColor{Light: "#B45309", Dark: "#FBBF24"}

	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(accent)
	mutedStyle   = lipgloss.NewStyle().Foreground(muted)
	okStyle      = lipgloss.NewStyle().Foreground(okColor)
	errStyle     = lipgloss.NewStyle().Foreground(errColor)
	warnStyle    = lipgloss.NewStyle().Foreground(warnColor).Bold(true)
	cmdStyle     = lipgloss.NewStyle().Bold(true)
	boxStyle     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(muted).Padding(0, 1)
	confirmStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(warnColor).Padding(0, 1)
)

const liveLines = 6

type state int

const (
	stRunning state = iota
	stConfirm
	stIdle
	stDone
)

type closedMsg struct{}

// Header is the information shown on the TUI's top line.
type Header struct {
	Provider, Model, Task string
	ReadOnly              bool
}

type model struct {
	ag     *agent.Agent
	cancel context.CancelFunc
	header Header

	vp       viewport.Model
	spin     spinner.Model
	input    textinput.Model
	width    int
	height   int
	ready    bool
	content  strings.Builder
	state    state
	thinking bool

	current string // command running now
	started time.Time
	live    []string // last lines of its output
	pending *agent.Event
}

// RunTUI runs the agent inside a full-screen terminal UI.
func RunTUI(ctx context.Context, cancel context.CancelFunc, ag *agent.Agent, h Header) error {
	ti := textinput.New()
	ti.Placeholder = "Pide algo más, o pulsa Enter vacío / Esc para salir"
	ti.CharLimit = 2000
	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	sp.Style = lipgloss.NewStyle().Foreground(accent)

	m := &model{ag: ag, cancel: cancel, header: h, spin: sp, input: ti}
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx))
	_, err := p.Run()
	if err == tea.ErrProgramKilled {
		err = nil
	}
	return err
}

func (m *model) waitEvent() tea.Cmd {
	ch := m.ag.Events()
	return func() tea.Msg {
		e, ok := <-ch
		if !ok {
			return closedMsg{}
		}
		return e
	}
}

func (m *model) Init() tea.Cmd { return tea.Batch(m.waitEvent(), m.spin.Tick) }

func (m *model) appendLine(s string) {
	m.content.WriteString(s)
	m.content.WriteString("\n")
	if !m.ready {
		return
	}
	atBottom := m.vp.AtBottom()
	m.vp.SetContent(m.content.String())
	if atBottom {
		m.vp.GotoBottom()
	}
}

func (m *model) wrap(s string, indent int) string {
	w := m.width - indent - 2
	if w < 20 {
		w = 20
	}
	return lipgloss.NewStyle().Width(w).PaddingLeft(indent).Render(s)
}

func riskLabel(r safety.Risk) string {
	switch r {
	case safety.Dangerous:
		return warnStyle.Render("peligroso")
	case safety.Blocked:
		return errStyle.Render("bloqueado")
	case safety.Normal:
		return "normal"
	}
	return "lectura"
}

func (m *model) handleEvent(e agent.Event) tea.Cmd {
	switch e.Kind {
	case agent.EvThinking:
		m.thinking = true
	case agent.EvAssistant:
		m.thinking = false
		m.appendLine("")
		m.appendLine(m.wrap(e.Text, 0))
	case agent.EvToolStart:
		m.thinking = false
		m.current = e.Command
		m.started = time.Now()
		m.live = nil
	case agent.EvOutput:
		m.live = append(m.live, e.Text)
		if len(m.live) > 200 {
			m.live = m.live[len(m.live)-liveLines:]
		}
	case agent.EvToolEnd:
		mark := okStyle.Render("✓")
		if e.Failed {
			mark = errStyle.Render(fmt.Sprintf("✗ exit %d", e.ExitCode))
		}
		dur := ""
		if e.Duration >= time.Second {
			dur = mutedStyle.Render(" · " + e.Duration.Round(time.Second).String())
		}
		m.appendLine(fmt.Sprintf("%s %s%s", mark, cmdStyle.Render(truncate(e.Command, m.width-20)), dur))
		if e.Failed {
			for _, l := range tail(m.live, 4) {
				m.appendLine(errStyle.Render("  │ ") + mutedStyle.Render(truncate(l, m.width-6)))
			}
		}
		m.current, m.live = "", nil
	case agent.EvConfirm:
		m.thinking = false
		ev := e
		m.pending = &ev
		m.state = stConfirm
	case agent.EvIdle:
		m.thinking = false
		m.state = stIdle
		m.input.Reset()
		return m.input.Focus()
	case agent.EvError:
		m.thinking = false
		m.appendLine(errStyle.Render("! " + e.Text))
	case agent.EvDone:
		m.thinking = false
		m.state = stDone
		if e.Text != "" {
			m.appendLine(mutedStyle.Render(e.Text))
		}
		u := m.ag.Usage()
		m.appendLine("")
		m.appendLine(mutedStyle.Render(fmt.Sprintf("Fin. Tokens: %d entrada / %d salida. Registro: %s", u.InputTokens, u.OutputTokens, m.ag.LogPath())))
		m.appendLine(mutedStyle.Render("Pulsa q para salir."))
	}
	return nil
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.input.Width = msg.Width - 6
		if !m.ready {
			m.vp = viewport.New(msg.Width, 1)
			m.ready = true
		}
		m.vp.Width = msg.Width
		m.resize()
		m.vp.SetContent(m.content.String())
		m.vp.GotoBottom()

	case agent.Event:
		if cmd := m.handleEvent(msg); cmd != nil {
			cmds = append(cmds, cmd)
		}
		m.resize()
		cmds = append(cmds, m.waitEvent())

	case closedMsg:
		if m.state != stDone {
			m.state = stDone
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		cmds = append(cmds, cmd)

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.cancel()
			return m, tea.Quit
		}
		switch m.state {
		case stConfirm:
			var d agent.Decision = -1
			switch strings.ToLower(msg.String()) {
			case "y", "s", "enter":
				d = agent.Approve
			case "a":
				d = agent.ApproveAll
			case "n", "esc":
				d = agent.Deny
			}
			if d >= 0 {
				answer := map[agent.Decision]string{agent.Approve: okStyle.Render("aprobado"), agent.ApproveAll: okStyle.Render("aprobado (todo)"), agent.Deny: errStyle.Render("rechazado")}[d]
				m.appendLine(fmt.Sprintf("? %s → %s", truncate(m.pending.Command, m.width-30), answer))
				m.pending.Reply <- d
				m.pending = nil
				m.state = stRunning
				m.resize()
			}
			return m, tea.Batch(cmds...)
		case stIdle:
			switch msg.Type {
			case tea.KeyEnter:
				text := strings.TrimSpace(m.input.Value())
				m.input.Blur()
				m.state = stRunning
				if text != "" {
					m.appendLine("")
					m.appendLine(titleStyle.Render("› ") + text)
				}
				ch := m.ag.Input()
				go func() { ch <- text }()
				m.resize()
				return m, tea.Batch(cmds...)
			case tea.KeyEsc:
				m.input.Blur()
				m.state = stRunning
				ch := m.ag.Input()
				go func() { ch <- "" }()
				return m, tea.Batch(cmds...)
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			cmds = append(cmds, cmd)
			return m, tea.Batch(cmds...)
		case stDone:
			if msg.String() == "q" || msg.Type == tea.KeyEnter || msg.Type == tea.KeyEsc {
				return m, tea.Quit
			}
		}
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		cmds = append(cmds, cmd)

	case tea.MouseMsg:
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

func (m *model) headerView() string {
	mode := ""
	if m.header.ReadOnly {
		mode = warnStyle.Render(" · solo lectura")
	}
	return titleStyle.Render("Ω omega") + mutedStyle.Render(fmt.Sprintf("  %s · %s · tarea: %s", m.header.Provider, m.header.Model, m.header.Task)) + mode
}

func (m *model) footerView() string {
	switch m.state {
	case stConfirm:
		e := m.pending
		var b strings.Builder
		fmt.Fprintf(&b, "%s  riesgo: %s\n", warnStyle.Render("¿Ejecutar?"), riskLabel(e.Risk))
		b.WriteString(cmdStyle.Render(truncate(e.Command, m.width-8)) + "\n")
		if e.Reason != "" {
			lines := strings.Split(e.Reason, "\n")
			if len(lines) > 14 {
				lines = append(lines[:14], "…")
			}
			for _, l := range lines {
				b.WriteString(mutedStyle.Render(truncate(l, m.width-8)) + "\n")
			}
		}
		b.WriteString("[y] sí   [a] sí a todo   [n] no")
		return confirmStyle.Width(m.width - 2).Render(b.String())
	case stIdle:
		return boxStyle.Width(m.width - 2).Render(m.input.View())
	case stDone:
		return mutedStyle.Render("q salir · ↑/↓ desplazar")
	}
	if m.current != "" {
		var b strings.Builder
		elapsed := time.Since(m.started).Round(time.Second)
		fmt.Fprintf(&b, "%s %s %s\n", m.spin.View(), cmdStyle.Render(truncate(m.current, m.width-20)), mutedStyle.Render(elapsed.String()))
		for _, l := range tail(m.live, liveLines) {
			b.WriteString(mutedStyle.Render(truncate(l, m.width-6)) + "\n")
		}
		return boxStyle.Width(m.width - 2).Render(strings.TrimRight(b.String(), "\n"))
	}
	if m.thinking {
		return m.spin.View() + mutedStyle.Render(" pensando…  (ctrl+c cancela)")
	}
	return mutedStyle.Render("ctrl+c cancela · ↑/↓ desplazar")
}

func (m *model) resize() {
	if !m.ready {
		return
	}
	h := m.height - lipgloss.Height(m.headerView()) - lipgloss.Height(m.footerView()) - 1
	if h < 3 {
		h = 3
	}
	atBottom := m.vp.AtBottom()
	m.vp.Height = h
	if atBottom {
		m.vp.GotoBottom()
	}
}

func (m *model) View() string {
	if !m.ready {
		return "iniciando…"
	}
	m.resize()
	return m.headerView() + "\n" + m.vp.View() + "\n" + m.footerView()
}

func tail(lines []string, n int) []string {
	if len(lines) <= n {
		return lines
	}
	return lines[len(lines)-n:]
}

func truncate(s string, n int) string {
	if n < 10 {
		n = 10
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
