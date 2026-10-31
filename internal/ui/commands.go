package ui

import (
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/i18n"
)

// slashCommand is a command typed in the input box, such as /model.
type slashCommand struct {
	name string
	args string // placeholder shown in the menu, e.g. "[name]"
	desc string // i18n key
}

var slashCommands = []slashCommand{
	{"/help", "", "slash.help"},
	{"/mode", "[ask|auto|readonly]", "slash.mode"},
	{"/model", "[name]", "slash.model"},
	{"/expand", "", "slash.expand"},
	{"/exit", "", "slash.exit"},
	{"/quit", "", "slash.exit"},
}

// modeArgs are the arguments of /mode, in cycling order.
var modeArgs = []string{"ask", "auto", "readonly"}

// suggestion is one row of the autocomplete menu under the input box.
type suggestion struct {
	value string // the input text once completed
	label string
	desc  string
}

const maxSuggestions = 6

// suggestions lists what completes the current input: commands while the
// name is being typed, then the arguments of /mode and /model.
func (m *model) suggestions() []suggestion {
	text := m.input.Value()
	if !strings.HasPrefix(text, "/") {
		return nil
	}
	name, arg, hasArg := strings.Cut(text, " ")
	var out []suggestion
	if !hasArg {
		for _, c := range slashCommands {
			if strings.HasPrefix(c.name, strings.ToLower(name)) {
				out = append(out, suggestion{value: c.name, label: strings.TrimSpace(c.name + " " + c.args), desc: i18n.T(c.desc)})
			}
		}
		return out
	}
	var args []string
	var descs func(string) string
	switch strings.ToLower(name) {
	case "/mode":
		args = modeArgs
		descs = func(a string) string { return modeLabel(parseMode(a)) }
	case "/model":
		args = m.header.Models
		descs = func(a string) string {
			if a == m.header.Model {
				return i18n.T("slash.current")
			}
			return ""
		}
	}
	// Model names share long prefixes (claude-…), so match anywhere.
	for _, a := range args {
		if strings.Contains(strings.ToLower(a), strings.ToLower(strings.TrimSpace(arg))) {
			out = append(out, suggestion{value: name + " " + a, label: a, desc: descs(a)})
		}
	}
	return out
}

// takesArgs reports whether a command name expects an argument.
func takesArgs(name string) bool {
	for _, c := range slashCommands {
		if c.name == name {
			return c.args != ""
		}
	}
	return false
}

func parseMode(s string) agent.Mode {
	switch strings.ToLower(s) {
	case "auto", "yes":
		return agent.ModeAuto
	case "readonly", "read-only", "ro":
		return agent.ModeReadOnly
	}
	return agent.ModeAsk
}

func modeLabel(md agent.Mode) string {
	return i18n.T([...]string{"ui.mode_ask", "ui.mode_auto", "ui.mode_readonly"}[md])
}

// handleMenuKey moves through the autocomplete menu while it is open. It
// returns false when the key is not for the menu.
func (m *model) handleMenuKey(msg tea.KeyMsg) (handled bool) {
	sugg := m.suggestions()
	if len(sugg) == 0 {
		return false
	}
	m.menuSel = min(m.menuSel, len(sugg)-1)
	switch msg.Type {
	case tea.KeyUp:
		m.menuSel = (m.menuSel - 1 + len(sugg)) % len(sugg)
	case tea.KeyDown:
		m.menuSel = (m.menuSel + 1) % len(sugg)
	case tea.KeyTab:
		m.complete(sugg[m.menuSel])
	case tea.KeyEsc:
		m.input.Reset()
	default:
		return false
	}
	return true
}

// complete puts a suggestion in the input box, leaving room for an argument.
func (m *model) complete(s suggestion) {
	v := s.value
	if !strings.Contains(v, " ") && takesArgs(v) {
		v += " "
	}
	m.input.SetValue(v)
	m.input.CursorEnd()
	m.menuSel = 0
}

// submitCommand runs a slash command typed in the input box. When the text
// is a prefix of a command, the highlighted suggestion runs instead.
func (m *model) submitCommand(text string) tea.Cmd {
	if sugg := m.suggestions(); len(sugg) > 0 && !m.isCommand(text) {
		text = sugg[min(m.menuSel, len(sugg)-1)].value
	}
	m.input.Reset()
	m.menuSel = 0
	name, arg, _ := strings.Cut(text, " ")
	arg = strings.TrimSpace(arg)
	switch strings.ToLower(name) {
	case "/help":
		m.help = true
	case "/mode":
		if arg == "" {
			m.openPicker(i18n.T("slash.pick_mode"), m.modeOptions(), func(id string) { m.setMode(parseMode(id)) })
			return nil
		}
		m.setMode(parseMode(arg))
	case "/model":
		if arg == "" {
			m.openPicker(i18n.T("slash.pick_model"), m.modelOptions(), m.setModel)
			return nil
		}
		if !slices.Contains(m.header.Models, arg) {
			m.note(errStyle.Render("! ")+mutedStyle.Render(i18n.T("slash.no_model", arg, strings.Join(m.header.Models, ", "))), lipgloss.NewStyle())
			break
		}
		m.setModel(arg)
	case "/expand":
		m.expanded = !m.expanded
	case "/exit", "/quit":
		m.quitting = true
		m.endSession()
		return nil
	default:
		m.note(errStyle.Render("! ")+mutedStyle.Render(i18n.T("slash.unknown", name)), lipgloss.NewStyle())
	}
	m.refresh()
	return nil
}

// isCommand reports whether text names a command exactly (with or without
// arguments), so Enter runs it as typed.
func (m *model) isCommand(text string) bool {
	name, _, _ := strings.Cut(text, " ")
	for _, c := range slashCommands {
		if strings.EqualFold(c.name, name) {
			return true
		}
	}
	return false
}

func (m *model) modeOptions() []Option {
	risks := []string{"normal", "dangerous", "read-only"}
	var opts []Option
	for i, a := range modeArgs {
		md := agent.Mode(i)
		opts = append(opts, Option{ID: a, Label: modeLabel(md), Detail: i18n.T("slash.mode_" + a), Risk: risks[i]})
	}
	return opts
}

func (m *model) modelOptions() []Option {
	var opts []Option
	for _, name := range m.header.Models {
		o := Option{ID: name, Label: name}
		if name == m.header.Model {
			o.Badge = i18n.T("slash.current")
		}
		opts = append(opts, o)
	}
	return opts
}

// openPicker shows a card whose answer stays in the interface instead of
// going to the agent.
func (m *model) openPicker(title string, opts []Option, onPick func(string)) {
	m.picker = NewPicker(title, opts, false)
	for i, o := range opts {
		if o.ID == m.header.Model || (o.ID == modeArgs[m.mode] && title == i18n.T("slash.pick_mode")) {
			m.picker.cursor = i
		}
	}
	m.onPick = onPick
	m.input.Blur()
	m.state = stChoice
	m.refresh()
}

func (m *model) setMode(md agent.Mode) {
	m.mode = md
	m.ag.SetMode(md)
}

func (m *model) setModel(name string) {
	if name == m.header.Model {
		return
	}
	m.header.Model = name
	m.ag.SetModel(name)
	m.note(titleStyle.Render("↻ ")+mutedStyle.Render(i18n.T("ui.model_picked", name)), lipgloss.NewStyle())
}

// cycleModel switches to the next model the run can use (ctrl+p).
func (m *model) cycleModel() {
	if len(m.header.Models) < 2 {
		m.note(mutedStyle.Render("↻ "+i18n.T("ui.model_only", m.header.Model)), lipgloss.NewStyle())
		return
	}
	next := m.header.Models[0]
	for i, name := range m.header.Models {
		if name == m.header.Model {
			next = m.header.Models[(i+1)%len(m.header.Models)]
		}
	}
	m.setModel(next)
}

// helpKeys is the on-screen shortcut card, in the order a session uses them.
var helpKeys = [][2]string{
	{"?", "key.help"},
	{"shift+tab", "key.mode"},
	{"ctrl+p", "key.model"},
	{"ctrl+o", "slash.expand"},
	{"/", "key.slash"},
	{"tab", "key.complete"},
	{"y / n / a", "key.decide"},
	{"esc", "key.esc"},
	{"ctrl+c", "key.cancel"},
	{"↑ ↓", "key.scroll"},
}

func (m *model) helpView() string {
	w := m.width - 2
	if w < 24 {
		w = 24
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render(i18n.T("slash.help_title")) + "\n")
	for _, c := range slashCommands {
		if c.name == "/quit" {
			continue
		}
		label := lipgloss.NewStyle().Width(22).Render(strings.TrimSpace(c.name + " " + c.args))
		b.WriteString("  " + cmdStyle.Render(label) + mutedStyle.Render(truncate(i18n.T(c.desc), max(12, w-28))) + "\n")
	}
	b.WriteString("\n" + titleStyle.Render(i18n.T("slash.keys_title")) + "\n")
	for _, k := range helpKeys {
		label := lipgloss.NewStyle().Width(14).Render(k[0])
		b.WriteString("  " + cmdStyle.Render(label) + mutedStyle.Render(truncate(i18n.T(k[1]), max(12, w-20))) + "\n")
	}
	return cardStyle.Width(w).Render(strings.TrimRight(b.String(), "\n"))
}

// menuView renders the autocomplete menu under the input box.
func (m *model) menuView() string {
	sugg := m.suggestions()
	if len(sugg) == 0 {
		return ""
	}
	sel := min(m.menuSel, len(sugg)-1)
	start := 0
	if sel >= maxSuggestions {
		start = sel - maxSuggestions + 1
	}
	end := min(len(sugg), start+maxSuggestions)
	width := 0
	for _, s := range sugg {
		width = max(width, lipgloss.Width(s.label))
	}
	width = min(width+3, 32)
	var b strings.Builder
	for i := start; i < end; i++ {
		s := sugg[i]
		label := lipgloss.NewStyle().Width(width).Render(truncate(s.label, width-1))
		desc := truncate(s.desc, max(10, m.width-width-8))
		if i == sel {
			b.WriteString("  " + titleStyle.Render("❯ "+label) + mutedStyle.Render(desc) + "\n")
		} else {
			b.WriteString("    " + label + faintStyle.Render(desc) + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
