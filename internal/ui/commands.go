package ui

import (
	"runtime"
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
	{"/panel", "", "slash.panel"},
	{"/full", "", "slash.task_full"},
	{"/update", "", "slash.task_update"},
	{"/clean", "", "slash.task_clean"},
	{"/analyze", "", "slash.task_analyze"},
	{"/health", "", "slash.task_health"},
	{"/tune", "", "slash.task_tune"},
	{"/mac", "", "slash.task_mac"},
	{"/permissions", "", "slash.task_permissions"},
	{"/apps", "", "slash.task_apps"},
	{"/dev", "", "slash.task_dev"},
	{"/install", "", "slash.task_install"},
	{"/docker", "", "slash.task_docker"},
	{"/fix", "", "slash.task_fix"},
	{"/undo", "", "slash.task_undo"},
	{"/history", "", "slash.task_history"},
	{"/doctor", "", "slash.task_doctor"},
	{"/disk", "", "slash.task_disk"},
	{"/logs", "", "slash.task_logs"},
	{"/shell", "", "slash.task_shell"},
	{"/steps", "", "slash.task_steps"},
	{"/login", "", "slash.task_login"},
	{"/background", "", "slash.task_background"},
	{"/menubar", "", "slash.task_menubar"},
	{"/dock", "", "slash.task_dock"},
	{"/shortcuts", "", "slash.task_shortcuts"},
	{"/models", "", "slash.task_models"},
	{"/settings", "", "slash.task_settings"},
	{"/notes", "", "slash.task_notes"},
	{"/exit", "", "slash.exit"},
	{"/quit", "", "slash.exit"},
}

// goos is the operating system omega runs on. Tests replace it.
var goos = runtime.GOOS

// macOnlyCommands start the parts of omega that drive macOS itself.
var macOnlyCommands = map[string]bool{"/mac": true, "/permissions": true, "/apps": true, "/undo": true,
	"/login": true, "/background": true, "/menubar": true, "/dock": true, "/shortcuts": true}

// offered reports whether a command is shown on this operating system. The
// Mac-only ones are hidden elsewhere, and typing one says why.
func offered(c slashCommand) bool { return goos == "darwin" || !macOnlyCommands[c.name] }

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
			if offered(c) && strings.HasPrefix(c.name, strings.ToLower(name)) {
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
	case "/panel":
		m.togglePanel()
		return nil
	default:
		if c, ok := findCommand(name); ok && !offered(c) {
			m.note(errStyle.Render("! ")+mutedStyle.Render(i18n.T("mac.only_slash", c.name)), lipgloss.NewStyle())
			break
		}
		if c, ok := findCommand(name); ok && strings.HasPrefix(c.desc, "slash.task_") {
			return m.startTask(strings.TrimPrefix(c.name, "/"))
		}
		m.note(errStyle.Render("! ")+mutedStyle.Render(i18n.T("slash.unknown", name)), lipgloss.NewStyle())
	case "/exit", "/quit":
		m.quitting = true
		m.endSession()
		return nil
	}
	m.refresh()
	return nil
}

func findCommand(name string) (slashCommand, bool) {
	for _, c := range slashCommands {
		if strings.EqualFold(c.name, name) {
			return c, true
		}
	}
	return slashCommand{}, false
}

// taskPrompts are the follow-up tasks a slash command starts in the running
// session; the rest come from the agent's presets.
var taskPrompts = map[string]string{
	"disk": "Tell me where the disk space on this machine goes. Start with the disk step and disk_usage on the biggest folders of my " +
		"home, then say in plain words what the largest items are and which are safe to remove. Offer the safe cleanups with " +
		"propose_actions and change nothing I do not pick.",
	"logs": "Review the system logs with the system-logs step: crash loops, services or launch agents that keep failing, what " +
		"prevents sleep, and logs that grow too fast. Explain what matters, ignore the noise, and offer a fix for each real problem.",
	"shell": "Find out why my shell takes the time it does to start. Run the shell-startup step, read my shell startup files, name " +
		"what costs the most and propose changes with their effect. Apply only what I pick, showing the diff.",
	"steps": "List the maintenance steps omega detects on this machine with list_steps, grouped by category. Say which are " +
		"disabled or not installed, what each of those would add, and how to run one directly.",
	"login": "Review what opens when I log in to this Mac with mac_inventory (section login). Say what each item is, suggest " +
		"what to stop from opening and apply only what I pick with mac_change.",
	"background": "Review what is allowed to run in the background on this Mac with mac_inventory (section background), including " +
		"leftovers of apps that are gone. Explain what you do not recognise and apply only what I pick with mac_change.",
	"menubar": "Review the apps in the menu bar of this Mac with mac_inventory (section menubar). Offer to quit, stop at login or " +
		"hide the ones I do not need and apply only what I pick with mac_change.",
	"dock": "Review the Dock of this Mac with mac_inventory (section dock): apps that are no longer installed or that I do not " +
		"open, and the Dock settings. Apply only what I pick with mac_change.",
	"shortcuts": "Find global keyboard shortcuts that several apps want on this Mac with mac_inventory (section shortcuts). For " +
		"each conflict ask me who keeps it, move the others to a free shortcut with mac_change and remember my choice.",
	"models": "Show me the models my provider offers with model_ranking: their score and price, which two automatic mode uses " +
		"and why, and roughly what a run costs with each. Change the model only if I ask, with omega_settings.",
	"settings": "Show omega's settings with omega_settings (show) and explain each one in a line. Ask me what I want to change and " +
		"apply it with omega_settings (set); for the ones only I can change, give me the exact command.",
	"notes": "Show what omega has noted about this machine (notes, list). Point out notes that look wrong or out of date and " +
		"offer to forget them; forget only what I confirm.",
	"permissions": "Review the privacy permissions of this Mac with mac_inventory (section permissions). Offer to remove the entries left " +
		"behind by binaries whose path no longer exists and by apps that are no longer installed, explain any binary listed several times, " +
		"and apply only what I pick with mac_change.",
	"apps": "Review the apps installed on this Mac with mac_inventory (section apps): what I have not opened in months, what still runs " +
		"under Rosetta and what removed apps left in ~/Library. Tell me what each one is, let me choose with propose_actions (nothing " +
		"preselected) and apply only what I pick with mac_change.",
}

// startTask sends a task preset to the agent as the next message, so a
// session can move from one job to another without leaving the interface.
func (m *model) startTask(task string) tea.Cmd {
	prompt, ok := taskPrompts[task]
	if !ok {
		prompt, ok = agent.TaskPrompts[task]
	}
	if !ok {
		return nil
	}
	m.input.Blur()
	m.state = stRunning
	m.header.Task = task
	m.add(&entry{kind: enUser, text: "/" + task})
	ch := m.ag.Input()
	go func() { ch <- prompt }()
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
	{"ctrl+f", "key.search"},
	{"shift+tab", "key.mode"},
	{"ctrl+p", "key.model"},
	{"ctrl+o /expand", "slash.expand"},
	{"ctrl+b /panel", "slash.panel"},
	{"ctrl+y", "key.copy"},
	{"alt+up/down", "key.focus"},
	{"/", "key.slash"},
	{"tab", "key.complete"},
	{"y / n / a", "key.decide"},
	{"esc", "key.esc"},
	{"ctrl+c", "key.cancel"},
	{"↑ ↓", "key.scroll"},
	{"pgup/pgdown", "key.card_scroll"},
}

func (m *model) helpView() string {
	w := m.width - 2
	if w < 24 {
		w = 24
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render(i18n.T("slash.help_title")) + "\n")
	// The task commands share one row: listed one by one, the card would
	// not fit a 24-line terminal.
	var tasks []string
	for _, c := range slashCommands {
		if c.name == "/quit" || !offered(c) {
			continue
		}
		if strings.HasPrefix(c.desc, "slash.task_") {
			tasks = append(tasks, c.name)
			continue
		}
		// /expand and /panel are listed once, under their keys below.
		if c.name == "/expand" || c.name == "/panel" {
			continue
		}
		label := lipgloss.NewStyle().Width(27).Render(strings.TrimSpace(c.name + " " + c.args))
		b.WriteString("  " + cmdStyle.Render(label) + mutedStyle.Render(truncate(i18n.T(c.desc), max(12, w-33))) + "\n")
	}
	line := ""
	for _, t := range tasks {
		if line != "" && lipgloss.Width(line)+1+lipgloss.Width(t) > w-6 {
			b.WriteString("  " + cmdStyle.Render(line) + "\n")
			line = ""
		}
		line = strings.TrimSpace(line + " " + t)
	}
	b.WriteString("  " + cmdStyle.Render(line) + "\n")
	b.WriteString("  " + lipgloss.NewStyle().Width(27).Render("") + mutedStyle.Render(truncate(i18n.T("slash.tasks"), max(12, w-33))) + "\n")
	b.WriteString(titleStyle.Render(i18n.T("slash.keys_title")) + "\n")
	// Two columns when there is room, so the card fits a short terminal.
	cols := 1
	if w >= 108 {
		cols = 2
	}
	colW := (w - 4) / cols
	rows := (len(helpKeys) + cols - 1) / cols
	for r := 0; r < rows; r++ {
		line := " "
		for c := 0; c < cols; c++ {
			i := r + c*rows
			if i >= len(helpKeys) {
				break
			}
			k := helpKeys[i]
			label := lipgloss.NewStyle().Width(16).Render(k[0])
			cell := " " + cmdStyle.Render(label) + mutedStyle.Render(truncate(i18n.T(k[1]), max(12, colW-19)))
			if c < cols-1 {
				cell = lipgloss.NewStyle().Width(colW).Render(cell)
			}
			line += cell
		}
		b.WriteString(line + "\n")
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

// SlashCommands names the commands a session understands.
func SlashCommands() []string {
	out := make([]string, len(slashCommands))
	for i, c := range slashCommands {
		out[i] = c.name
	}
	return out
}
