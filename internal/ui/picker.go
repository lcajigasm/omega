package ui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lcajigasm/omega/internal/i18n"
)

// Option is one entry of a selection card.
type Option struct {
	ID       string
	Label    string
	Detail   string // second line, muted
	Badge    string // short tag shown on the right, e.g. "~9 GB" or "$0.10/M"
	Risk     string // "read-only", "normal" or "dangerous"; colours the marker
	Selected bool
}

// Picker is a selection card: one choice (radio) or several (checkboxes).
// In single mode, typing filters the list, which suits long model lists.
type Picker struct {
	Title   string
	Hint    string
	Options []Option
	Multi   bool

	cursor  int
	filter  string
	visible []int
}

// ErrCancelled is returned when the user closes a card without choosing.
var ErrCancelled = errors.New("cancelled")

const pickerRows = 10

// NewPicker builds a card.
func NewPicker(title string, opts []Option, multi bool) *Picker {
	p := &Picker{Title: title, Options: opts, Multi: multi}
	p.refilter()
	return p
}

func (p *Picker) refilter() {
	p.visible = p.visible[:0]
	f := strings.ToLower(p.filter)
	for i, o := range p.Options {
		if f == "" || strings.Contains(strings.ToLower(o.Label+" "+o.ID), f) {
			p.visible = append(p.visible, i)
		}
	}
	if p.cursor >= len(p.visible) {
		p.cursor = max(0, len(p.visible)-1)
	}
}

// Selected returns the chosen option IDs.
func (p *Picker) Selected() []string {
	var ids []string
	if !p.Multi {
		if len(p.visible) > 0 {
			ids = append(ids, p.Options[p.visible[p.cursor]].ID)
		}
		return ids
	}
	for _, o := range p.Options {
		if o.Selected {
			ids = append(ids, o.ID)
		}
	}
	return ids
}

// HandleKey updates the card. done is true when the user confirmed or
// cancelled; cancelled tells which.
func (p *Picker) HandleKey(k tea.KeyMsg) (done, cancelled bool) {
	switch k.Type {
	case tea.KeyUp:
		if p.cursor > 0 {
			p.cursor--
		}
		return
	case tea.KeyDown, tea.KeyTab:
		if p.cursor < len(p.visible)-1 {
			p.cursor++
		}
		return
	case tea.KeyEnter:
		if p.Multi || len(p.visible) > 0 {
			return true, false
		}
		return
	case tea.KeyEsc:
		if p.filter != "" {
			p.filter = ""
			p.refilter()
			return
		}
		return true, true
	case tea.KeyBackspace:
		if !p.Multi && p.filter != "" {
			p.filter = p.filter[:len(p.filter)-1]
			p.refilter()
		}
		return
	case tea.KeySpace:
		if p.Multi && len(p.visible) > 0 {
			o := &p.Options[p.visible[p.cursor]]
			o.Selected = !o.Selected
		}
		return
	case tea.KeyRunes:
		r := string(k.Runes)
		if p.Multi {
			switch r {
			case "k":
				return p.HandleKey(tea.KeyMsg{Type: tea.KeyUp})
			case "j":
				return p.HandleKey(tea.KeyMsg{Type: tea.KeyDown})
			case "a":
				all := true
				for _, o := range p.Options {
					all = all && o.Selected
				}
				for i := range p.Options {
					p.Options[i].Selected = !all
				}
			case "x", " ":
				return p.HandleKey(tea.KeyMsg{Type: tea.KeySpace})
			}
			return
		}
		p.filter += r
		p.refilter()
	}
	return
}

func riskStyle(risk string) lipgloss.Style {
	switch risk {
	case "dangerous":
		return warnStyle
	case "blocked":
		return errStyle
	case "read-only":
		return okStyle
	}
	return lipgloss.NewStyle().Foreground(accent)
}

// View renders the card at the given width.
func (p *Picker) View(width int) string {
	if width < 40 {
		width = 40
	}
	inner := width - 6
	var b strings.Builder
	b.WriteString(titleStyle.Render(p.Title))
	if !p.Multi && p.filter != "" {
		b.WriteString(mutedStyle.Render("  " + i18n.T("pick.filter", p.filter)))
	}
	b.WriteString("\n")
	if p.Hint != "" {
		b.WriteString(mutedStyle.Render(truncate(p.Hint, inner)) + "\n")
	}
	b.WriteString("\n")

	start := 0
	if p.cursor >= pickerRows {
		start = p.cursor - pickerRows + 1
	}
	end := min(len(p.visible), start+pickerRows)
	if len(p.visible) == 0 {
		b.WriteString(mutedStyle.Render("  "+i18n.T("pick.no_match")) + "\n")
	}
	for row := start; row < end; row++ {
		o := p.Options[p.visible[row]]
		active := row == p.cursor
		pointer := "  "
		if active {
			pointer = titleStyle.Render("❯ ")
		}
		mark := ""
		if p.Multi {
			if o.Selected {
				mark = riskStyle(o.Risk).Render("◼") + " "
			} else {
				mark = mutedStyle.Render("◻") + " "
			}
		}
		label := o.Label
		if active {
			label = cmdStyle.Render(label)
		}
		badge := ""
		if o.Badge != "" {
			badge = "  " + riskStyle(o.Risk).Render(o.Badge)
		}
		line := pointer + mark + label + badge
		b.WriteString(truncateANSI(line, inner) + "\n")
		if o.Detail != "" && (active || p.Multi) {
			b.WriteString(mutedStyle.Render("    "+truncate(o.Detail, inner-4)) + "\n")
		}
	}
	if len(p.visible) > end {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("    … %d", len(p.visible)-end)) + "\n")
	}
	b.WriteString("\n")
	if p.Multi {
		n := len(p.Selected())
		b.WriteString(mutedStyle.Render(i18n.T("pick.keys_multi", n)))
	} else {
		b.WriteString(mutedStyle.Render(i18n.T("pick.keys_single")))
	}
	return cardStyle.Width(width - 2).Render(b.String())
}

func truncateANSI(s string, n int) string {
	if lipgloss.Width(s) <= n {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(n).Render(s)
}

// pickerProgram runs a Picker on its own, inline in the terminal.
type pickerProgram struct {
	p         *Picker
	width     int
	cancelled bool
	done      bool
}

func (m *pickerProgram) Init() tea.Cmd { return nil }

func (m *pickerProgram) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.cancelled, m.done = true, true
			return m, tea.Quit
		}
		if done, cancelled := m.p.HandleKey(msg); done {
			m.done, m.cancelled = true, cancelled
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *pickerProgram) View() string {
	if m.done {
		return ""
	}
	w := m.width
	if w == 0 || w > 100 {
		w = 100
	}
	return m.p.View(w) + "\n"
}

// Pick shows a card in the terminal and returns the chosen IDs. Without a
// terminal it falls back to a numbered prompt on in/out.
func Pick(title, hint string, opts []Option, multi bool, tty bool, in io.Reader, out io.Writer) ([]string, error) {
	p := NewPicker(title, opts, multi)
	p.Hint = hint
	if tty {
		// Resolve the terminal background query before Bubble Tea reads stdin;
		// otherwise its reply (which starts with ESC) arrives as an Esc key
		// press and closes the card at once.
		lipgloss.HasDarkBackground()
		m := &pickerProgram{p: p}
		if _, err := tea.NewProgram(m).Run(); err != nil {
			return nil, err
		}
		if m.cancelled {
			return nil, ErrCancelled
		}
		return p.Selected(), nil
	}
	return pickPlain(p, bufio.NewReader(in), out)
}

func pickPlain(p *Picker, in *bufio.Reader, out io.Writer) ([]string, error) {
	fmt.Fprintln(out, p.Title)
	for i, o := range p.Options {
		fmt.Fprintf(out, "  %2d) %s", i+1, o.Label)
		if o.Badge != "" {
			fmt.Fprintf(out, "  [%s]", o.Badge)
		}
		fmt.Fprintln(out)
		if o.Detail != "" {
			fmt.Fprintf(out, "      %s\n", o.Detail)
		}
	}
	if p.Multi {
		fmt.Fprint(out, i18n.T("pick.plain_multi"))
	} else {
		fmt.Fprint(out, i18n.T("pick.plain_single"))
	}
	line, _ := in.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, ErrCancelled
	}
	if p.Multi && strings.EqualFold(line, "all") {
		ids := make([]string, len(p.Options))
		for i, o := range p.Options {
			ids[i] = o.ID
		}
		return ids, nil
	}
	var ids []string
	for _, f := range strings.FieldsFunc(line, func(r rune) bool { return r == ',' || r == ' ' }) {
		n, err := strconv.Atoi(f)
		if err != nil || n < 1 || n > len(p.Options) {
			continue
		}
		ids = append(ids, p.Options[n-1].ID)
		if !p.Multi {
			break
		}
	}
	if len(ids) == 0 {
		return nil, ErrCancelled
	}
	return ids, nil
}
