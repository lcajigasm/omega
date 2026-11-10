package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"

	"github.com/lcajigasm/omega/internal/i18n"
)

// onOS makes the interface see one operating system.
func onOS(t *testing.T, os string) {
	t.Helper()
	old := goos
	goos = os
	t.Cleanup(func() { goos = old })
}

// The Mac-only commands are hidden off macOS, and typing one says why
// instead of starting a task.
func TestMacCommandsOffMacOS(t *testing.T) {
	i18n.Init("en")
	onOS(t, "linux")
	m := &model{input: textinput.New()}
	m.input.SetValue("/")
	for _, s := range m.suggestions() {
		if macOnlyCommands[s.value] {
			t.Errorf("%s is suggested on Linux", s.value)
		}
	}
	if len(m.suggestions()) != len(slashCommands)-len(macOnlyCommands) {
		t.Errorf("only the Mac commands are hidden: %d suggestions", len(m.suggestions()))
	}
	for name := range macOnlyCommands {
		before := len(m.entries)
		if cmd := m.submitCommand(name); cmd != nil || m.header.Task != "" {
			t.Errorf("%s must not start a task on Linux", name)
		}
		if len(m.entries) != before+1 || !strings.Contains(m.entries[len(m.entries)-1].text, "only works on macOS") {
			t.Errorf("%s must say it is Mac-only", name)
		}
	}
}

func TestSuggestions(t *testing.T) {
	onOS(t, "darwin")
	m := &model{input: textinput.New(), header: Header{Model: "fast", Models: []string{"fast", "smart"}}}
	labels := func() []string {
		var out []string
		for _, s := range m.suggestions() {
			out = append(out, s.value)
		}
		return out
	}
	for in, want := range map[string][]string{
		"hello": nil,
		"/": {"/help", "/mode", "/model", "/expand", "/panel", "/full", "/update", "/clean", "/analyze", "/health", "/tune", "/mac", "/permissions", "/apps", "/dev",
			"/install", "/docker", "/fix", "/undo", "/history", "/doctor", "/disk", "/logs", "/shell", "/steps", "/login", "/background",
			"/menubar", "/dock", "/shortcuts", "/models", "/settings", "/notes", "/exit", "/quit"},
		"/mo":       {"/mode", "/model", "/models"},
		"/model ":   {"/model fast", "/model smart"},
		"/model sm": {"/model smart"},
		"/model a":  {"/model fast", "/model smart"},
		"/mode r":   {"/mode readonly"},
		"/zz":       nil,
	} {
		m.input.SetValue(in)
		got := labels()
		if len(got) != len(want) {
			t.Errorf("%q: got %v, want %v", in, got, want)
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%q: got %v, want %v", in, got, want)
				break
			}
		}
	}
	m.input.SetValue("/mo")
	m.complete(m.suggestions()[1])
	if m.input.Value() != "/model " {
		t.Fatalf("completing a command with arguments leaves room for one: %q", m.input.Value())
	}
}
