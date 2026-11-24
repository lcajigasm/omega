package ui

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func routineSteps() []RoutineStep {
	return []RoutineStep{
		{ID: "disk", Label: "Disk space", Category: "check", Command: "df -h", Risk: "read-only"},
		{ID: "brew", Label: "Homebrew", Category: "update", Command: "brew upgrade", Risk: "normal", LastRun: 65 * time.Second, Freed: 2_254_857_830},
		{ID: "npm", Label: "npm", Category: "update", Command: "npm update -g", Risk: "normal"},
		{ID: "brew-cleanup", Label: "Homebrew cleanup", Category: "clean", Command: "brew cleanup", Risk: "dangerous"},
	}
}

func rpress(e *RoutineEditor, keys ...string) {
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "up":
			msg = tea.KeyMsg{Type: tea.KeyUp}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		case "shift+up":
			msg = tea.KeyMsg{Type: tea.KeyShiftUp}
		case "shift+down":
			msg = tea.KeyMsg{Type: tea.KeyShiftDown}
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case " ":
			msg = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		e.Update(msg)
	}
}

func TestRoutineEditorAddsInOrderAndReorders(t *testing.T) {
	e := NewRoutineEditor(RoutineSetup{Steps: routineSteps()})
	// Empty routine: the cursor starts on the first step to add (disk).
	rpress(e, " ")         // add disk
	rpress(e, "down", " ") // cursor moved to brew-cleanup? no: list is brew, npm, brew-cleanup
	if got := e.Routine(); len(got) != 2 {
		t.Fatalf("routine = %v, want two steps", got)
	}
	first := e.Routine()
	// Go to the top (the routine) and move the first step down.
	rpress(e, "g", "shift+down")
	got := e.Routine()
	if got[0] != first[1] || got[1] != first[0] {
		t.Fatalf("after moving down: %v, want %v reversed", got, first)
	}
	rpress(e, "K")
	if !slices.Equal(e.Routine(), first) {
		t.Fatalf("after moving back up: %v, want %v", e.Routine(), first)
	}
}

func TestRoutineEditorKeepsSavedOrderAndDropsUnknown(t *testing.T) {
	e := NewRoutineEditor(RoutineSetup{Steps: routineSteps(), Current: []string{"brew-cleanup", "gone", "disk", "disk"}})
	if got := e.Routine(); !slices.Equal(got, []string{"brew-cleanup", "disk"}) {
		t.Fatalf("routine = %v", got)
	}
	if e.dirty() {
		t.Fatal("a routine as loaded is not a change")
	}
	rpress(e, " ") // remove brew-cleanup (the cursor is on the first routine entry)
	if got := e.Routine(); !slices.Equal(got, []string{"disk"}) {
		t.Fatalf("after removing: %v", got)
	}
	rpress(e, "u")
	if got := e.Routine(); !slices.Equal(got, []string{"brew-cleanup", "disk"}) {
		t.Fatalf("after undo: %v", got)
	}
}

func TestRoutineEditorWillNotSaveEmpty(t *testing.T) {
	e := NewRoutineEditor(RoutineSetup{Steps: routineSteps()})
	rpress(e, "enter")
	if e.done {
		t.Fatal("an empty routine was saved")
	}
	if e.status == "" {
		t.Fatal("no word on why nothing was saved")
	}
}

func TestRoutineEditorAsksBeforeDroppingChanges(t *testing.T) {
	e := NewRoutineEditor(RoutineSetup{Steps: routineSteps(), Current: []string{"disk"}})
	rpress(e, "down", " ") // add something
	rpress(e, "esc")
	if e.done || !e.confirm {
		t.Fatalf("esc with changes left at once (done=%v confirm=%v)", e.done, e.confirm)
	}
	rpress(e, "n")
	if !e.done || !e.cancelled {
		t.Fatal("n did not discard")
	}
}

func TestRoutineEditorRecommendation(t *testing.T) {
	var asked []string
	rec := func(_ context.Context, current []string, _ bool) (Recommendation, error) {
		asked = current
		return Recommendation{Steps: []string{"disk", "brew", "brew-cleanup"}, Why: map[string]string{"brew": "keeps packages current"}, Summary: "A weekly routine."}, nil
	}
	e := NewRoutineEditor(RoutineSetup{Steps: routineSteps(), Current: []string{"npm"}, Recommend: rec})
	_, cmd := e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	if cmd == nil || !e.asking {
		t.Fatal("i did not ask")
	}
	e.Update(cmd())
	if !slices.Equal(asked, []string{"npm"}) {
		t.Fatalf("the model was sent %v, want the current routine", asked)
	}
	view := e.View()
	for _, want := range []string{"A weekly routine.", "keeps packages current", "npm"} {
		if !strings.Contains(view, want) {
			t.Errorf("recommendation view lacks %q:\n%s", want, view)
		}
	}
	rpress(e, "a")
	if got := e.Routine(); !slices.Equal(got, []string{"disk", "brew", "brew-cleanup"}) {
		t.Fatalf("after applying: %v", got)
	}
	if e.why["brew"] == "" {
		t.Fatal("the reasons were not kept")
	}
}

func TestRoutineEditorRecommendationError(t *testing.T) {
	e := NewRoutineEditor(RoutineSetup{Steps: routineSteps(), Recommend: func(context.Context, []string, bool) (Recommendation, error) {
		return Recommendation{}, errors.New("no key")
	}})
	_, cmd := e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	e.Update(cmd())
	if !strings.Contains(e.View(), "no key") {
		t.Fatal("the error is not shown")
	}
}

func TestRoutineEditorViewFitsNarrowAndShort(t *testing.T) {
	e := NewRoutineEditor(RoutineSetup{Steps: routineSteps(), Current: []string{"disk"}})
	e.Update(tea.WindowSizeMsg{Width: 60, Height: 14})
	view := e.View()
	lines := strings.Split(view, "\n")
	if len(lines) > 14 {
		t.Fatalf("%d lines for a 14-line terminal:\n%s", len(lines), view)
	}
	for _, l := range lines {
		if w := len([]rune(stripANSI(l))); w > 60 {
			t.Errorf("line of %d columns: %q", w, stripANSI(l))
		}
	}
}

func TestRoutineEditorStopOnFailAndFreeSpace(t *testing.T) {
	e := NewRoutineEditor(RoutineSetup{Steps: routineSteps(), Current: []string{"brew", "disk"}, StopOnFail: []string{"disk", "npm"}, OnlyBelowFreeGB: 30})
	if c := e.Choice(); !slices.Equal(c.StopOnFail, []string{"disk"}) || c.OnlyBelowFreeGB != 30 {
		t.Fatalf("as loaded: %+v", c)
	}
	if e.dirty() {
		t.Fatal("options as loaded are not a change")
	}
	rpress(e, "f") // the cursor is on brew
	if c := e.Choice(); !slices.Equal(c.StopOnFail, []string{"brew", "disk"}) {
		t.Fatalf("after f: %+v", c)
	}
	if !e.dirty() {
		t.Fatal("a new stop is a change")
	}
	if !strings.Contains(stripANSI(e.View()), "stop") {
		t.Errorf("the stop tag is not shown:\n%s", stripANSI(e.View()))
	}
	// A step that is not in the routine cannot stop it.
	rpress(e, "G", "f")
	if c := e.Choice(); len(c.StopOnFail) != 2 || e.status == "" {
		t.Fatalf("f outside the routine: %+v status %q", c, e.status)
	}
	// b opens the field with the value; typing replaces it.
	rpress(e, "b")
	if !e.editBelow || e.belowIn != "30" {
		t.Fatalf("field: %v %q", e.editBelow, e.belowIn)
	}
	e.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	e.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	rpress(e, "1", "2", ",", "5", "x", "enter")
	if e.editBelow || e.Choice().OnlyBelowFreeGB != 12.5 {
		t.Fatalf("after typing: %v %+v", e.editBelow, e.Choice())
	}
	rpress(e, "b")
	for range 4 {
		e.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	rpress(e, "enter")
	if e.Choice().OnlyBelowFreeGB != 0 {
		t.Fatalf("empty field: %+v", e.Choice())
	}
	rpress(e, "b", "9", "esc")
	if e.Choice().OnlyBelowFreeGB != 0 {
		t.Fatal("esc kept the typed value")
	}
	rpress(e, "u")
	if c := e.Choice(); !slices.Equal(c.StopOnFail, []string{"disk"}) || c.OnlyBelowFreeGB != 30 || e.dirty() {
		t.Fatalf("after undo: %+v", c)
	}
}

func TestRoutineEditorShowsLastRunFigures(t *testing.T) {
	testModel(t)
	e := NewRoutineEditor(RoutineSetup{Steps: routineSteps(), Current: []string{"brew"}})
	e.Update(tea.WindowSizeMsg{Width: 110, Height: 24})
	view := stripANSI(e.View())
	if !strings.Contains(view, "1m05s · 2.1 GB") {
		t.Fatalf("no figures of the last run:\n%s", view)
	}
	if strings.Contains(view, "Homebrew ·") {
		t.Fatal("the figures are in the label")
	}
}

func TestRoutineEditorSharesDataOnlyWhenTurnedOn(t *testing.T) {
	var sent []bool
	rec := func(_ context.Context, _ []string, with bool) (Recommendation, error) {
		sent = append(sent, with)
		return Recommendation{}, errors.New("stop here")
	}
	e := NewRoutineEditor(RoutineSetup{Steps: routineSteps(), Recommend: rec, MachineData: "42 GB free · 3 days since the last omega custom"})
	if !strings.Contains(stripANSI(e.View()), "send machine data: off") {
		t.Fatalf("the key line does not say it is off:\n%s", stripANSI(e.View()))
	}
	_, cmd := e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	e.Update(cmd())
	rpress(e, "d")
	if !e.SharesData() || !strings.Contains(e.status, "42 GB free · 3 days since the last omega custom") {
		t.Fatalf("d did not say what is sent: %q", e.status)
	}
	view := stripANSI(e.View())
	if !strings.Contains(view, "send machine data: on") || !strings.Contains(view, "42 GB free") {
		t.Fatalf("view after d:\n%s", view)
	}
	_, cmd = e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	e.Update(cmd())
	if !slices.Equal(sent, []bool{false, true}) {
		t.Fatalf("sent %v, want off then on", sent)
	}
	// Without a description there is no key d.
	e = NewRoutineEditor(RoutineSetup{Steps: routineSteps(), Recommend: rec})
	rpress(e, "d")
	if e.SharesData() || strings.Contains(stripANSI(e.View()), "machine data") {
		t.Fatal("d works with nothing to send")
	}
}

// w opens the days field with the days in the user's words; enter keeps
// what is typed, in any language, and an empty field means every day.
func TestRoutineEditorDays(t *testing.T) {
	e := NewRoutineEditor(RoutineSetup{Steps: routineSteps(), Current: []string{"brew"}, Days: []string{"fri", "mon"}})
	if c := e.Choice(); !slices.Equal(c.Days, []string{"mon", "fri"}) || e.dirty() {
		t.Fatalf("as loaded: %+v", c)
	}
	if !strings.Contains(stripANSI(e.View()), "only on mon, fri") {
		t.Errorf("the days are not in the header:\n%s", stripANSI(e.View()))
	}
	rpress(e, "w")
	if !e.editDays || e.daysIn != "mon, fri" {
		t.Fatalf("field: %v %q", e.editDays, e.daysIn)
	}
	for range len([]rune(e.daysIn)) {
		e.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	rpress(e, "mié", ",", " ", "sáb", "enter")
	if e.editDays || !slices.Equal(e.Choice().Days, []string{"wed", "sat"}) || !e.dirty() {
		t.Fatalf("after typing: %v %+v", e.editDays, e.Choice())
	}
	rpress(e, "w", "x", "y", "enter")
	if !e.editDays || !strings.Contains(e.status, "is not a day") {
		t.Fatalf("a bad day: %v %q", e.editDays, e.status)
	}
	rpress(e, "esc")
	if e.editDays || !slices.Equal(e.Choice().Days, []string{"wed", "sat"}) {
		t.Fatal("esc changed the days")
	}
	rpress(e, "w")
	for range len([]rune(e.daysIn)) {
		e.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	rpress(e, "enter")
	if len(e.Choice().Days) != 0 {
		t.Fatalf("empty field: %+v", e.Choice())
	}
	rpress(e, "u")
	if !slices.Equal(e.Choice().Days, []string{"mon", "fri"}) || e.dirty() {
		t.Fatalf("after undo: %+v", e.Choice())
	}
}
