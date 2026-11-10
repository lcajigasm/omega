package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lcajigasm/omega/internal/i18n"
)

// MenuItem is one row of the home screen. A row either runs a command
// (Args), opens another list (Children), or asks for a value first (Ask).
type MenuItem struct {
	Group  string // heading the item sits under
	Label  string // the command it runs, as the user would type it
	Detail string
	// Args is the command line after "omega". A row with no Children runs it.
	Args []string
	// Children opens a list instead of running: the subcommands of a
	// command, or the values a setting can take.
	Children []MenuItem
	// Options are other ways to run the same thing (read-only, automatic…),
	// opened with the right arrow.
	Options []MenuItem
	// Ask is the name of a value the user types; it is appended to Args.
	Ask string
}

// MenuInfo is the state shown above the list.
type MenuInfo struct {
	Version  string
	Provider string
	Model    string
	LastRun  string // "" when omega has not run yet
	Notes    []string
}

// menuLevel is one list the user has opened, with its place in it.
type menuLevel struct {
	title  string
	items  []MenuItem
	cursor int
}

type menuModel struct {
	info          MenuInfo
	stack         []menuLevel // the lists opened so far; the last is on screen
	all           []MenuItem  // every row that runs something, for the search
	hits          []MenuItem  // rows matching the filter
	hitCursor     int
	filter        string
	asking        *MenuItem // the row whose value is being typed
	value         string
	width, height int
	picked        []string
	started       bool
	free, total   uint64
}

func newMenu(info MenuInfo, items []MenuItem) *menuModel {
	m := &menuModel{info: info, stack: []menuLevel{{items: items}}}
	m.all = flattenMenu(items, "")
	return m
}

// flattenMenu lists every row that runs something, at any depth, each under
// the path that leads to it. Typing searches this list, so nothing is more
// than a few letters away.
func flattenMenu(items []MenuItem, path string) []MenuItem {
	var out []MenuItem
	seen := map[string]bool{}
	var walk func(items []MenuItem, path, about string)
	walk = func(items []MenuItem, path, about string) {
		for _, it := range items {
			here := it.Group
			if path != "" {
				here = path
			}
			if len(it.Children) == 0 {
				if key := it.Label + "\x00" + it.Ask; !seen[key] {
					seen[key] = true
					leaf := it
					leaf.Group = here
					// A bare value ("… language es") is found by what its
					// setting is for.
					if leaf.Detail == "" {
						leaf.Detail = about
					}
					out = append(out, leaf)
				}
			} else {
				walk(it.Children, it.Label, it.Detail)
			}
			walk(it.Options, it.Label, it.Detail)
		}
	}
	walk(items, path, "")
	return out
}

func (m *menuModel) Init() tea.Cmd { return nil }

func (m *menuModel) top() *menuLevel { return &m.stack[len(m.stack)-1] }

// rows are the rows on screen and the index of the highlighted one.
func (m *menuModel) rows() ([]MenuItem, *int) {
	if m.filter != "" {
		return m.hits, &m.hitCursor
	}
	return m.top().items, &m.top().cursor
}

func (m *menuModel) refilter() {
	m.hits = m.hits[:0]
	q := strings.ToLower(m.filter)
	for _, it := range m.all {
		if strings.Contains(strings.ToLower(it.Label+" "+it.Detail+" "+it.Group), q) {
			m.hits = append(m.hits, it)
		}
	}
	if m.hitCursor >= len(m.hits) {
		m.hitCursor = max(0, len(m.hits)-1)
	}
}

func (m *menuModel) open(title string, items []MenuItem) {
	m.filter, m.hitCursor = "", 0
	m.stack = append(m.stack, menuLevel{title: title, items: items})
}

func (m *menuModel) back() bool {
	if len(m.stack) > 1 {
		m.stack = m.stack[:len(m.stack)-1]
		return true
	}
	return false
}

// choose acts on the highlighted row: open its list, ask for its value, or
// run it.
func (m *menuModel) choose(it MenuItem) tea.Cmd {
	switch {
	case len(it.Children) > 0:
		m.open(it.Label, it.Children)
	case it.Ask != "":
		m.asking, m.value = &it, ""
	default:
		m.picked, m.started = it.Args, true
		return tea.Quit
	}
	return nil
}

func (m *menuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		if m.asking != nil {
			return m, m.updateAsking(msg)
		}
		rows, cursor := m.rows()
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.filter != "" {
				m.filter = ""
				return m, nil
			}
			if !m.back() {
				return m, tea.Quit
			}
		case "left":
			if m.filter == "" {
				m.back()
			}
		case "up", "ctrl+p":
			if len(rows) > 0 {
				*cursor = (*cursor - 1 + len(rows)) % len(rows)
			}
		case "down", "ctrl+n":
			if len(rows) > 0 {
				*cursor = (*cursor + 1) % len(rows)
			}
		case "home", "pgup":
			*cursor = 0
		case "end", "pgdown":
			*cursor = max(0, len(rows)-1)
		case "enter":
			if len(rows) > 0 {
				return m, m.choose(rows[*cursor])
			}
		case "right", "tab":
			if len(rows) == 0 {
				break
			}
			if it := rows[*cursor]; len(it.Options) > 0 {
				m.open(it.Label, it.Options)
			} else if len(it.Children) > 0 {
				m.open(it.Label, it.Children)
			}
		case "backspace":
			if r := []rune(m.filter); len(r) > 0 {
				m.filter = string(r[:len(r)-1])
				m.refilter()
			} else {
				m.back()
			}
		default:
			if msg.Type == tea.KeyRunes && !msg.Alt {
				if m.filter == "" && len(m.stack) == 1 && string(msg.Runes) == "q" {
					return m, tea.Quit
				}
				m.filter += string(msg.Runes)
				m.refilter()
			}
		}
	}
	return m, nil
}

// updateAsking handles the keys while a value is being typed.
func (m *menuModel) updateAsking(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+c":
		return tea.Quit
	case "esc":
		m.asking = nil
	case "enter":
		if v := strings.TrimSpace(m.value); v != "" {
			m.picked, m.started = append(append([]string(nil), m.asking.Args...), v), true
			return tea.Quit
		}
	case "backspace":
		if r := []rune(m.value); len(r) > 0 {
			m.value = string(r[:len(r)-1])
		}
	default:
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
			m.value += string(msg.Runes)
		}
	}
	return nil
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

	rows, cursor := m.rows()
	// Where the user is: omega › config › set language.
	var crumbs []string
	for _, l := range m.stack[1:] {
		crumbs = append(crumbs, l.title)
	}
	labelW := 0
	for _, it := range rows {
		labelW = max(labelW, lipgloss.Width(it.Label))
	}
	labelW = min(labelW+4, max(20, w/2))
	var lines []string
	if len(crumbs) > 0 && m.filter == "" {
		lines = append(lines, "  "+faintStyle.Render("‹ ")+mutedStyle.Render(strings.Join(crumbs, faintStyle.Render("  ›  "))), "")
	}
	cursorLine := 0
	group := ""
	for n, it := range rows {
		if it.Group != group {
			group = it.Group
			if len(lines) > 0 && lines[len(lines)-1] != "" {
				lines = append(lines, "")
			}
			if group != "" {
				lines = append(lines, "  "+faintStyle.Render(group))
			}
		}
		mark := "  "
		switch {
		case len(it.Children) > 0:
			mark = " ›"
		case it.Ask != "":
			mark = " …"
		case len(it.Options) > 0:
			mark = " +"
		}
		label := lipgloss.NewStyle().Width(labelW).Render(truncate(it.Label, labelW-3) + mark)
		detail := truncate(it.Detail, max(10, w-labelW-6))
		if n == *cursor {
			cursorLine = len(lines)
			lines = append(lines, titleStyle.Render("❯ "+label)+detail)
		} else {
			lines = append(lines, "  "+cmdStyle.Render(label)+mutedStyle.Render(detail))
		}
	}
	if len(rows) == 0 {
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
	switch {
	case m.asking != nil:
		ask := "  " + titleStyle.Render(m.asking.Label) + "\n  " + mutedStyle.Render(m.asking.Ask+": ") + m.value + "█"
		return head + "\n\n" + ask + "\n\n" + faintStyle.Render("  "+i18n.T("menu.hint_ask"))
	case m.filter != "":
		hint = fmt.Sprintf("/%s  ·  %s", m.filter, i18n.T("menu.hint_filter"))
	case len(m.stack) > 1:
		hint = i18n.T("menu.hint_sub")
	}
	return head + "\n\n" + strings.Join(lines, "\n") + "\n\n" + faintStyle.Render("  "+hint)
}

// RunMenu shows the home screen and returns the command line of the row
// the user started. ok is false when they left without starting anything.
func RunMenu(info MenuInfo, items []MenuItem) (args []string, ok bool, err error) {
	m := newMenu(info, items)
	m.free, m.total = diskSpace("/")
	final, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		return nil, false, err
	}
	fm := final.(*menuModel)
	return fm.picked, fm.started, nil
}
