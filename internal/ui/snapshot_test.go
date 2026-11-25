package ui

import (
	"bytes"
	"flag"
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/safety"
	"github.com/lcajigasm/omega/internal/sysstats"
)

// update rewrites the snapshots under testdata instead of comparing them:
// go test ./internal/ui -run Snap -update (OMEGA_UPDATE_SNAPS=1 does the same).
var update = flag.Bool("update", false, "rewrite the snapshots under testdata")

// snapWidths are the terminal widths every screen is checked at.
var snapWidths = []int{60, 80, 120}

// stableLook turns off what would make a snapshot depend on the machine:
// the ASCII set, reduced motion.
func stableLook(t *testing.T) {
	t.Helper()
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("OMEGA_ASCII", "")
	t.Setenv("OMEGA_REDUCED_MOTION", "")
	t.Setenv("NO_COLOR", "")
	setGlyphs(false)
	// The snapshots are of the Mac texts; elsewhere a few say "machine".
	os := i18n.OS
	i18n.OS = "darwin"
	t.Cleanup(func() { i18n.OS = os })
	i18n.Init("en")
	pinStats(t)
}

// pinStats draws a fixed machine on the stats line.
func pinStats(t *testing.T) {
	t.Helper()
	was := statsNow
	statsNow = func() sysstats.Sample {
		return sysstats.Sample{
			CPU: 0.23, CPUKnown: true, Cores: 8,
			MemUsed: 10 << 30, MemTotal: 16 << 30, MemKnown: true,
			Disks:  []sysstats.Disk{{Mount: "/", Used: 400 << 30, Total: 500 << 30}, {Mount: "/Volumes/Backup", Used: 100 << 30, Total: 1000 << 30}},
			RxRate: 1.5 * (1 << 20), TxRate: 40 << 10, NetKnown: true,
		}
	}
	t.Cleanup(func() { statsNow = was })
}

func TestMenuSnapshots(t *testing.T) {
	testModel(t)
	stableLook(t)
	items := []MenuItem{
		{Group: "MAINTENANCE", Label: "omega", Detail: "Full maintenance"},
		{Group: "MAINTENANCE", Label: "omega update", Detail: "Update all installed software", Args: []string{"update"},
			Options: []MenuItem{{Label: "omega update --dry-run", Args: []string{"update", "--dry-run"}}}},
		{Group: "MAINTENANCE", Label: `omega -p "…"`, Detail: "Ask in words", Args: []string{"-p"}, Ask: "what you want"},
		{Group: "YOUR MAC", Label: "omega mac", Detail: "Review your Mac", Children: []MenuItem{
			{Label: "omega mac permissions", Detail: "Remove privacy permissions", Args: []string{"mac", "permissions"}},
		}},
	}
	for _, w := range snapWidths {
		m := newMenu(MenuInfo{Version: "1.0", Provider: "anthropic", Model: "auto", LastRun: "2 days ago"}, items)
		m.Update(tea.WindowSizeMsg{Width: w, Height: 30})
		assertSnap(t, fmt.Sprintf("menu_%d", w), m.View())
	}
}

func TestSessionSnapshots(t *testing.T) {
	for _, w := range snapWidths {
		m := testModel(t)
		stableLook(t)
		m.started = time.Now()
		m.panel = true
		m.header.Task = "update"
		m.handleEvent(agent.Event{Kind: agent.EvToolStart, Tool: "run_step:brew", Command: "brew upgrade"})
		m.handleEvent(agent.Event{Kind: agent.EvToolEnd, Tool: "run_step:brew", Command: "brew upgrade", Duration: 41 * time.Second, Lines: 3})
		m.handleEvent(agent.Event{Kind: agent.EvDenied, Tool: "run_command", Command: "rm -rf ~/x", Reason: "outside what omega may delete. More."})
		m.state = stDone
		m.Update(tea.WindowSizeMsg{Width: w, Height: 24})
		m.refresh()
		assertSnap(t, fmt.Sprintf("session_%d", w), m.View())
	}
}

func TestGuidePlanSnapshots(t *testing.T) {
	for _, w := range snapWidths {
		m := testGuide(t, w, 30, nil)
		stableLook(t)
		assertSnap(t, fmt.Sprintf("guide_plan_%d", w), m.View())
	}
}

func TestRoutineEditorSnapshots(t *testing.T) {
	testModel(t)
	stableLook(t)
	for _, w := range snapWidths {
		e := NewRoutineEditor(RoutineSetup{Steps: routineSteps(), Current: []string{"brew", "disk"}, StopOnFail: []string{"brew"}, OnlyBelowFreeGB: 30})
		e.Update(tea.WindowSizeMsg{Width: w, Height: 24})
		assertSnap(t, fmt.Sprintf("routine_%d", w), e.View())
	}
}

func TestReportSnapshots(t *testing.T) {
	testModel(t)
	stableLook(t)
	for _, w := range snapWidths {
		var buf bytes.Buffer
		r := NewReport(&buf)
		r.width = w
		r.Title("omega doctor", "v1.2.3")
		r.Section("Setup", 3)
		r.Check(MarkOK, "config", 8, "~/.config/omega/config.toml")
		r.Check(MarkWarn, "key", 8, "the key for anthropic is in the environment, not in the keychain; omega config set-key moves it")
		r.Check(MarkErr, "brew", 8, "not installed")
		r.Section("Steps", -1)
		r.Table([]Column{{Title: "STEP"}, {Title: "SIZE", Right: true}, {Title: "WHAT", Wrap: true}},
			[][]string{
				{"brew", Bytes(3 << 30), "Updates Homebrew formulae and casks, then cleans the old versions it leaves behind"},
				{"npm", Bytes(12 << 20), "Updates global npm packages"},
			}, []Mark{MarkOK, MarkOff})
		r.Blank()
		r.Hint("Run omega doctor --fix to repair what can be repaired without asking.")
		assertSnap(t, fmt.Sprintf("report_%d", w), buf.String())
	}
}

// The approval card's frame and key line follow the risk: a dangerous
// command gets the warning frame and a line saying Enter does not approve.
func TestDangerousConfirmSnapshot(t *testing.T) {
	m := testModel(t)
	stableLook(t)
	m.width = 72
	m.state = stConfirm
	m.pending = &agent.Event{Tool: "run_command", Command: "rm -rf ~/Library/Caches/old", Risk: safety.Dangerous}
	assertSnap(t, "card_confirm_dangerous", m.confirmView())
}
