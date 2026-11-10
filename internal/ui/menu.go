package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lcajigasm/omega/internal/i18n"
)

// MenuItem is one thing the home screen can start.
type MenuItem struct {
	ID     string
	Group  string // heading the item sits under
	Label  string // the command it runs, as the user would type it
	Detail string
}

// MenuInfo is the state shown above the list.
type MenuInfo struct {
	Version  string
	Provider string
	Model    string
	LastRun  string // "" when omega has not run yet
	Notes    []string
}

type menuModel struct {
	info          MenuInfo
	items         []MenuItem
	shown         []int // indexes into items that match the filter
	cursor        int   // index into shown
	filter        string
	width, height int
	picked        string
	started       bool
	free, total   uint64
}

func (m *menuModel) Init() tea.Cmd { return nil }

func (m *menuModel) refilter() {
	m.shown = m.shown[:0]
	q := strings.ToLower(m.filter)
	for i, it := range m.items {
		if q == "" || strings.Contains(strings.ToLower(it.Label+" "+it.Detail), q) {
			m.shown = append(m.shown, i)
		}
	}
	if m.cursor >= len(m.shown) {
		m.cursor = max(0, len(m.shown)-1)
	}
}

func (m *menuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.filter != "" {
				m.filter = ""
				m.refilter()
				return m, nil
			}
			return m, tea.Quit
		case "up", "ctrl+p":
			if len(m.shown) > 0 {
				m.cursor = (m.cursor - 1 + len(m.shown)) % len(m.shown)
			}
		case "down", "ctrl+n", "tab":
			if len(m.shown) > 0 {
				m.cursor = (m.cursor + 1) % len(m.shown)
			}
		case "home", "pgup":
			m.cursor = 0
		case "end", "pgdown":
			m.cursor = max(0, len(m.shown)-1)
		case "enter":
			if len(m.shown) > 0 {
				m.picked, m.started = m.items[m.shown[m.cursor]].ID, true
				return m, tea.Quit
			}
		case "backspace":
			if r := []rune(m.filter); len(r) > 0 {
				m.filter = string(r[:len(r)-1])
				m.refilter()
			}
		default:
			if msg.Type == tea.KeyRunes && !msg.Alt {
				if m.filter == "" && string(msg.Runes) == "q" {
					return m, tea.Quit
				}
				m.filter += string(msg.Runes)
				m.refilter()
			}
		}
	}
	return m, nil
}

func (m *menuModel) View() string {
	w := min(max(m.width-2, 40), 108)
	var b strings.Builder
	b.WriteString(titleStyle.Render("✻ omega") + mutedStyle.Render(" "+m.info.Version) + "  " + mutedStyle.Render(i18n.T("ui.welcome_tagline")) + "\n\n")
	row := func(k, v string) {
		b.WriteString(faintStyle.Render(lipgloss.NewStyle().Width(10).Render(k)) + v + "\n")
	}
	if m.total > 0 {
		bar := min(28, w-44)
		if bar < 8 {
			bar = 8
		}
		row(i18n.T("menu.disk"), gauge(1-float64(m.free)/float64(m.total), bar)+"  "+
			mutedStyle.Render(i18n.T("panel.free", humanBytes(m.free), humanBytes(m.total))))
	}
	if m.info.Model != "" {
		row(i18n.T("ui.label_model"), m.info.Provider+mutedStyle.Render(" · ")+cmdStyle.Render(m.info.Model))
	}
	if m.info.LastRun != "" {
		row(i18n.T("menu.last_run"), mutedStyle.Render(m.info.LastRun))
	}
	for _, n := range m.info.Notes {
		b.WriteString(warnStyle.Render("! ") + mutedStyle.Render(truncate(n, w-4)) + "\n")
	}
	head := welcomeStyle.Width(w).Render(strings.TrimRight(b.String(), "\n"))

	labelW := 0
	for _, it := range m.items {
		labelW = max(labelW, lipgloss.Width(it.Label))
	}
	labelW += 2
	var lines []string
	cursorLine := 0
	group := ""
	for n, i := range m.shown {
		it := m.items[i]
		if it.Group != group {
			group = it.Group
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, "  "+faintStyle.Render(group))
		}
		label := lipgloss.NewStyle().Width(labelW).Render(it.Label)
		detail := truncate(it.Detail, max(10, w-labelW-6))
		if n == m.cursor {
			cursorLine = len(lines)
			lines = append(lines, titleStyle.Render("❯ "+label)+detail)
		} else {
			lines = append(lines, "  "+cmdStyle.Render(label)+mutedStyle.Render(detail))
		}
	}
	if len(m.shown) == 0 {
		lines = append(lines, "  "+mutedStyle.Render(i18n.T("menu.no_match", m.filter)))
	}
	// Keep the highlighted row on screen when the list is taller than the terminal.
	room := m.height - lipgloss.Height(head) - 4
	if room < 3 {
		room = 3
	}
	if len(lines) > room {
		start := min(max(0, cursorLine-room/2), len(lines)-room)
		lines = lines[start : start+room]
	}
	hint := i18n.T("menu.hint")
	if m.filter != "" {
		hint = fmt.Sprintf("/%s  ·  %s", m.filter, i18n.T("menu.hint_filter"))
	}
	return head + "\n\n" + strings.Join(lines, "\n") + "\n\n" + faintStyle.Render("  "+hint)
}

// MenuQuit is what RunMenuOrQuit returns when the user left without
// starting anything; "" is a valid item id.
const MenuQuit = "\x00"

// RunMenuOrQuit shows the home screen and returns the id of the item the
// user started, or MenuQuit.
func RunMenuOrQuit(info MenuInfo, items []MenuItem) (string, error) {
	m := &menuModel{info: info, items: items}
	m.free, m.total = diskSpace("/")
	m.refilter()
	final, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		return "", err
	}
	fm := final.(*menuModel)
	if !fm.started {
		return MenuQuit, nil
	}
	return fm.picked, nil
}
