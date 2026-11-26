package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/i18n"
)

func reviewActs() []agent.PlannedAction {
	return []agent.PlannedAction{
		{ID: "brew-cache", Stage: "clean", Category: "Cleanup", Title: "Clear the Homebrew cache", Why: "8.9 GB of old downloads",
			What: "Removes old versions; nothing installed changes.", Evidence: "brew cleanup --dry-run: 8.9 GB", Risk: "normal",
			Recommended: true, Bytes: 8_900_000_000, Steps: []agent.PlanStep{{Tool: "run_step", Show: "brew cleanup --prune=all"}}},
		{ID: "derived", Stage: "ultra-deep", Category: "Cleanup", Title: "Delete Xcode DerivedData", Why: "4.1 GB of build products",
			What: "Xcode rebuilds it.", Evidence: "no xcodebuild running", Risk: "dangerous", Bytes: 4_100_000_000,
			Steps: []agent.PlanStep{{Tool: "run_command", Show: "rm -rf ~/Library/Developer/Xcode/DerivedData/*"}}},
		{ID: "agents", Stage: "ultra-care", Category: "AI agents", Title: "End 3 orphaned processes", Why: "1.2 GB of RAM held",
			What: "Ends them.", Evidence: "omega agents --json", Risk: "normal", Recommended: true, Impact: "~1.2 GB RAM",
			Steps: []agent.PlanStep{{Tool: "omega_cmd", Show: "omega agents kill --yes"}}},
	}
}

func reviewPress(m *reviewModel, keys ...string) {
	for _, k := range keys {
		m.key(k)
	}
}

func TestReviewStartsWithTheRecommendedAndGroups(t *testing.T) {
	i18n.Init("en")
	m := newReviewModel(PlanReviewInfo{}, reviewActs())
	if len(m.groups) != 2 || m.groups[0].name != "Cleanup" || len(m.groups[0].acts) != 2 {
		t.Fatalf("groups: %+v", m.groups)
	}
	if got := strings.Join(m.picked(), ","); got != "brew-cache,agents" {
		t.Fatalf("recommended start ticked: %s", got)
	}
	// The cursor starts on the first action; space unticks it.
	reviewPress(m, " ")
	if got := strings.Join(m.picked(), ","); got != "agents" {
		t.Fatalf("after space: %s", got)
	}
	// On a header, space ticks the whole group.
	reviewPress(m, "up", " ")
	if got := strings.Join(m.picked(), ","); got != "brew-cache,derived,agents" {
		t.Fatalf("group toggle: %s", got)
	}
	reviewPress(m, "n")
	if len(m.picked()) != 0 {
		t.Fatal("n clears")
	}
	reviewPress(m, "enter")
	if m.confirming || m.warn == "" {
		t.Fatal("enter with nothing ticked warns instead of confirming")
	}
	reviewPress(m, "r", "enter")
	if !m.confirming {
		t.Fatal("enter asks to confirm")
	}
	reviewPress(m, "esc")
	if m.confirming || m.ok {
		t.Fatal("esc goes back")
	}
	reviewPress(m, "enter", "enter")
	if !m.ok || !m.done {
		t.Fatal("enter twice runs")
	}
}

func TestReviewFoldsAndLeaves(t *testing.T) {
	i18n.Init("en")
	m := newReviewModel(PlanReviewInfo{}, reviewActs())
	before := len(m.rows())
	reviewPress(m, "left")
	if len(m.rows()) != before-2 {
		t.Fatalf("fold: %d rows", len(m.rows()))
	}
	reviewPress(m, "right")
	if len(m.rows()) != before {
		t.Fatal("unfold")
	}
	reviewPress(m, "q")
	if !m.quitting || m.done {
		t.Fatal("q with something ticked asks first")
	}
	reviewPress(m, "enter")
	if !m.done || m.ok {
		t.Fatal("leaving runs nothing")
	}
}

// Every width draws: side by side, one above the other, a modal, ASCII.
func TestReviewViewFitsEveryWidth(t *testing.T) {
	i18n.Init("en")
	for _, ascii := range []bool{false, true} {
		setGlyphs(ascii)
		for _, w := range []int{40, 72, 96, 140} {
			m := newReviewModel(PlanReviewInfo{Version: "v1", DryRun: w == 72}, reviewActs())
			m.Update(tea.WindowSizeMsg{Width: w, Height: 30})
			for _, step := range [][]string{nil, {"down"}, {"enter"}} {
				reviewPress(m, step...)
				v := m.View()
				lines := strings.Split(v, "\n")
				if len(lines) > 30 {
					t.Errorf("w=%d: %d lines", w, len(lines))
				}
				for _, l := range lines {
					if lipgloss.Width(l) > w {
						t.Errorf("w=%d ascii=%v: line too wide (%d): %q", w, ascii, lipgloss.Width(l), ansi.Strip(l))
					}
				}
			}
			plain := ansi.Strip(m.View())
			if !strings.Contains(plain, "Run the plan") && !strings.Contains(plain, "Show what would run") {
				t.Errorf("w=%d: no confirmation card:\n%s", w, plain)
			}
		}
	}
	setGlyphs(false)
	m := newReviewModel(PlanReviewInfo{}, reviewActs())
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	plain := ansi.Strip(m.View())
	for _, want := range []string{"WHAT THE AUDIT FOUND", "WHAT WILL HAPPEN", "VERIFIED", "brew cleanup --prune=all", "3 actions", "2 ticked"} {
		if !strings.Contains(plain, want) {
			t.Errorf("view lacks %q:\n%s", want, plain)
		}
	}
}
