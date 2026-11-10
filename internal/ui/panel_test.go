package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lcajigasm/omega/internal/agent"
)

func TestSessionPanel(t *testing.T) {
	m := testModel(t)
	m.panel = true
	m.header.Task, m.header.Model = "full", "fast-model"
	m.diskStart, m.diskNow, m.diskTotal = 100<<30, 109<<30, 500<<30
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 34})
	if !m.panelShown() {
		t.Fatal("a wide terminal shows the panel")
	}
	if m.vp.Width != 130-panelWidth {
		t.Fatalf("the transcript is %d wide, want %d", m.vp.Width, 130-panelWidth)
	}
	m.handleEvent(agent.Event{Kind: agent.EvToolStart, Tool: "run_step:brew", Command: "brew upgrade"})
	m.handleEvent(agent.Event{Kind: agent.EvToolEnd, Tool: "run_step:brew", Command: "brew upgrade", Duration: 41 * time.Second})
	m.handleEvent(agent.Event{Kind: agent.EvToolStart, Tool: "run_step:pipx", Command: "pipx upgrade-all"})
	m.handleEvent(agent.Event{Kind: agent.EvToolEnd, Tool: "run_step:pipx", Command: "pipx upgrade-all", Failed: true, ExitCode: 1})
	m.handleEvent(agent.Event{Kind: agent.EvDenied, Tool: "run_command", Command: "rm -rf ~/x", Reason: "denied"})
	m.handleEvent(agent.Event{Kind: agent.EvToolStart, Tool: "run_step:npm", Command: "npm update -g"})
	m.diskStart, m.diskNow = 100<<30, 109<<30 // the test machine's disk moved it
	if got := m.tally(); got != (tally{ok: 1, failed: 1, denied: 1, running: 1}) {
		t.Fatalf("tally %+v", got)
	}
	m.refresh()
	view := stripANSI(m.View())
	for _, want := range []string{"SESSION", "full", "fast-model", "✓ 1", "✗ 1", "⊘ 1", "● 1", "DISK", "109.0 GB free of 500.0 GB", "9.0 GB freed",
		"ACTIVITY", "Step brew", "41s", "Step pipx", "Step npm"} {
		if !strings.Contains(view, want) {
			t.Errorf("panel lacks %q:\n%s", want, view)
		}
	}
	for i, line := range strings.Split(view, "\n") {
		if w := len([]rune(line)); w > 130 {
			t.Errorf("line %d is %d columns wide, over the terminal's 130", i, w)
		}
	}
	if rows := strings.Count(view, "\n") + 1; rows > 34 {
		t.Errorf("the view is %d rows, over the terminal's 34", rows)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	if m.panelShown() || m.vp.Width != 130 {
		t.Fatal("ctrl+b hides the panel and gives the width back")
	}
	if strings.Contains(stripANSI(m.View()), "ACTIVITY") {
		t.Fatal("the hidden panel is still drawn")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	m.Update(tea.WindowSizeMsg{Width: 90, Height: 34})
	if m.panelShown() || m.vp.Width != 90 {
		t.Fatal("a narrow terminal has no room for the panel")
	}
}

func TestTaskCommandStartsTheNextJob(t *testing.T) {
	m := testModel(t)
	m.Update(tea.WindowSizeMsg{Width: 90, Height: 30})
	m.state = stIdle
	m.submitCommand("/dev")
	if m.state != stRunning || m.header.Task != "dev" {
		t.Fatalf("state %v task %q", m.state, m.header.Task)
	}
	last := m.entries[len(m.entries)-1]
	if last.kind != enUser || last.text != "/dev" {
		t.Fatalf("the transcript shows what was asked: %+v", last)
	}
	tasks := 0
	for _, c := range slashCommands {
		if !strings.HasPrefix(c.desc, "slash.task_") {
			continue
		}
		tasks++
		task := strings.TrimPrefix(c.name, "/")
		if _, ok := taskPrompts[task]; !ok {
			if _, ok := agent.TaskPrompts[task]; !ok {
				t.Errorf("%s has no prompt", c.name)
			}
		}
	}
	if tasks != 28 {
		t.Errorf("%d task commands, want 28", tasks)
	}
}

func TestHelpCardFitsASmallTerminal(t *testing.T) {
	m := testModel(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m.help = true
	if h := strings.Count(m.helpView(), "\n") + 1; h > 27 {
		t.Fatalf("the help card is %d rows; it must leave room on a 30-row terminal", h)
	}
}

func TestMenu(t *testing.T) {
	testModel(t) // language and state dirs
	key := func(m *menuModel, k tea.KeyType) tea.Cmd { _, cmd := m.Update(tea.KeyMsg{Type: k}); return cmd }
	typeIn := func(m *menuModel, text string) {
		for _, r := range text {
			m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}
	}
	items := func() []MenuItem {
		return []MenuItem{
			{Group: "MAINTENANCE", Label: "omega", Detail: "Full maintenance"},
			{Group: "MAINTENANCE", Label: "omega update", Detail: "Update all installed software", Args: []string{"update"},
				Options: []MenuItem{{Label: "omega update --dry-run", Detail: "read-only", Args: []string{"update", "--dry-run"}}}},
			{Group: "MAINTENANCE", Label: `omega -p "…"`, Detail: "Ask in words", Args: []string{"-p"}, Ask: "what you want"},
			{Group: "YOUR MAC", Label: "omega mac", Detail: "Review your Mac", Children: []MenuItem{
				{Label: "omega mac permissions", Detail: "Remove privacy permissions", Args: []string{"mac", "permissions"}},
				{Label: "omega mac undo", Detail: "Revert changes", Args: []string{"mac", "undo"}},
			}},
			{Group: "SETTINGS", Label: "omega config set language", Detail: "Language of the interface", Children: []MenuItem{
				{Label: "omega config set language es", Args: []string{"config", "set", "language", "es"}},
			}},
		}
	}
	m := newMenu(MenuInfo{Version: "1.0", Provider: "anthropic", Model: "auto", LastRun: "2 days ago"}, items())
	m.total, m.free = 500<<30, 100<<30
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	view := stripANSI(m.View())
	for _, want := range []string{"omega 1.0", "100.0 GB free of 500.0 GB", "2 days ago", "MAINTENANCE", "YOUR MAC", "❯ omega ", "omega mac ›", "omega update +"} {
		if !strings.Contains(view, want) {
			t.Errorf("menu lacks %q:\n%s", want, view)
		}
	}

	// Enter on a row with a list opens it; Enter on a row of that list runs it.
	key(m, tea.KeyDown)
	key(m, tea.KeyDown)
	key(m, tea.KeyDown)
	if cmd := key(m, tea.KeyEnter); cmd != nil || len(m.stack) != 2 {
		t.Fatalf("enter on a command with subcommands opens them: depth %d", len(m.stack))
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "omega mac permissions") || strings.Contains(view, "omega update") {
		t.Fatalf("the submenu shows only its rows:\n%s", view)
	}
	key(m, tea.KeyDown)
	if cmd := key(m, tea.KeyEnter); cmd == nil || !m.started || strings.Join(m.picked, " ") != "mac undo" {
		t.Fatalf("enter runs the highlighted row: started=%v picked=%v", m.started, m.picked)
	}

	// Left and Esc go back one level; Esc at the top leaves.
	m = newMenu(MenuInfo{}, items())
	key(m, tea.KeyDown)
	key(m, tea.KeyDown)
	key(m, tea.KeyDown)
	key(m, tea.KeyEnter)
	key(m, tea.KeyLeft)
	if len(m.stack) != 1 || m.top().cursor != 3 {
		t.Fatalf("left goes back to where the user was: depth %d cursor %d", len(m.stack), m.top().cursor)
	}
	key(m, tea.KeyEnter)
	if cmd := key(m, tea.KeyEsc); cmd != nil || len(m.stack) != 1 {
		t.Fatal("esc in a submenu goes back, it does not quit")
	}
	if cmd := key(m, tea.KeyEsc); cmd == nil || m.started {
		t.Fatal("esc at the top leaves without starting anything")
	}

	// The right arrow opens the other ways to run a row.
	m = newMenu(MenuInfo{}, items())
	key(m, tea.KeyDown)
	key(m, tea.KeyRight)
	if len(m.stack) != 2 {
		t.Fatal("right opens the run options")
	}
	if cmd := key(m, tea.KeyEnter); cmd == nil || strings.Join(m.picked, " ") != "update --dry-run" {
		t.Fatalf("picked %v", m.picked)
	}

	// Typing searches every row at any depth, also by what a setting is for.
	m = newMenu(MenuInfo{}, items())
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	typeIn(m, "perm")
	if len(m.hits) != 1 || m.hits[0].Label != "omega mac permissions" {
		t.Fatalf("typing finds a row inside a submenu: %+v", m.hits)
	}
	key(m, tea.KeyEsc)
	if m.filter != "" || m.started {
		t.Fatal("the first Esc clears the search")
	}
	typeIn(m, "interface")
	if len(m.hits) != 1 || m.hits[0].Label != "omega config set language es" {
		t.Fatalf("a value is found by what its setting is for: %+v", m.hits)
	}
	typeIn(m, "zzz")
	if view := stripANSI(m.View()); !strings.Contains(view, "Nothing matches") {
		t.Fatalf("no match is said:\n%s", view)
	}

	// A row that needs a value asks for it and appends it.
	m = newMenu(MenuInfo{}, items())
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	key(m, tea.KeyDown)
	key(m, tea.KeyDown)
	if cmd := key(m, tea.KeyEnter); cmd != nil || m.asking == nil {
		t.Fatal("enter on a row that needs a value asks for it")
	}
	if cmd := key(m, tea.KeyEnter); cmd != nil || m.started {
		t.Fatal("an empty value runs nothing")
	}
	typeIn(m, "why is the disk full")
	if view := stripANSI(m.View()); !strings.Contains(view, "what you want: why is the disk full") {
		t.Fatalf("the value is shown while typing:\n%s", view)
	}
	if cmd := key(m, tea.KeyEnter); cmd == nil || len(m.picked) != 2 || m.picked[0] != "-p" || m.picked[1] != "why is the disk full" {
		t.Fatalf("picked %q", m.picked)
	}

	// q leaves only at the top with nothing typed.
	m = newMenu(MenuInfo{}, items())
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}); cmd == nil {
		t.Fatal("q leaves the menu")
	}
}
