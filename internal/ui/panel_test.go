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
	if tasks != 15 {
		t.Errorf("%d task commands, want 15", tasks)
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
	items := []MenuItem{
		{ID: "", Group: "MAINTENANCE", Label: "omega", Detail: "Full maintenance"},
		{ID: "update", Group: "MAINTENANCE", Label: "omega update", Detail: "Update all installed software"},
		{ID: "mac permissions", Group: "YOUR MAC", Label: "omega mac permissions", Detail: "Remove privacy permissions"},
		{ID: "dev", Group: "DEVELOPMENT", Label: "omega dev", Detail: "Review languages"},
	}
	m := &menuModel{info: MenuInfo{Version: "1.0", Provider: "anthropic", Model: "auto", LastRun: "2 days ago"}, items: items, total: 500 << 30, free: 100 << 30}
	m.refilter()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	view := stripANSI(m.View())
	for _, want := range []string{"omega 1.0", "100.0 GB free of 500.0 GB", "2 days ago", "MAINTENANCE", "YOUR MAC", "❯ omega ", "omega mac permissions"} {
		if !strings.Contains(view, want) {
			t.Errorf("menu lacks %q:\n%s", want, view)
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd == nil || !m.started || m.picked != "mac permissions" {
		t.Fatalf("enter starts the highlighted entry: started=%v picked=%q", m.started, m.picked)
	}

	m = &menuModel{items: items}
	m.refilter()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	for _, r := range "perm" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if len(m.shown) != 1 || items[m.shown[0]].ID != "mac permissions" {
		t.Fatalf("typing filters the list: %v", m.shown)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.filter != "" || len(m.shown) != 4 || m.started {
		t.Fatal("the first Esc clears the filter")
	}
	// The first entry has an empty id (bare omega): starting it is not the same as leaving.
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.started || m.picked != "" {
		t.Fatalf("full maintenance is started=%v picked=%q", m.started, m.picked)
	}
}
