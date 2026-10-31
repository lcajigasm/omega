package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/safety"
)

func TestCardSnapshots(t *testing.T) {
	i18n.Init("en")
	m := testModel(t)
	m.width = 72

	m.state = stConfirm
	m.pending = &agent.Event{
		Tool: "run_command", Command: "brew upgrade", Risk: safety.Normal,
		Reason: "upgrade outdated formulae\n+ curl 8.10\n- curl 8.9",
	}
	assertSnap(t, "card_confirm", m.confirmView())

	m.state = stPassword
	m.pending = &agent.Event{Text: "softwareupdate wants sudo"}
	m.password.Reset()
	m.password.Prompt = "🔒 "
	m.password.EchoMode = textinput.EchoPassword
	m.password.EchoCharacter = '•'
	m.password.Focus()
	m.password.Cursor.Blink = false
	assertSnap(t, "card_password", m.passwordView())

	choice := NewPicker("Choose a model", []Option{
		{ID: "haiku", Label: "Haiku", Detail: "fast and cheap"},
		{ID: "sonnet", Label: "Sonnet", Detail: "stronger reasoning"},
		{ID: "opus", Label: "Opus", Badge: "current"},
	}, false)
	assertSnap(t, "card_choice", choice.View(72))

	multi := NewPicker("Apply these changes", []Option{
		{ID: "brew", Label: "Upgrade Homebrew", Detail: "formulae and casks", Risk: "normal", Selected: true},
		{ID: "cache", Label: "Delete caches", Detail: "user caches only", Risk: "dangerous"},
	}, true)
	assertSnap(t, "card_choice_multi", multi.View(72))
}

func assertSnap(t *testing.T, name, view string) {
	t.Helper()
	got := screen(view, lipgloss.Width(view), lipgloss.Height(view))
	if !strings.HasSuffix(got, "\n") {
		got += "\n"
	}
	path := filepath.Join("testdata", name+".snap")
	if os.Getenv("OMEGA_UPDATE_SNAPS") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v\nrendered card:\n%s", path, err, got)
	}
	if string(want) != got {
		t.Errorf("snapshot %s does not match the rendered card\n--- got ---\n%s--- want ---\n%s", name, got, want)
	}
}

func TestScreenIgnoresANSIAndTerminalSize(t *testing.T) {
	raw := "\x1b[1mHi\x1b[0m\nthere"
	got := screen(raw, 10, 4)
	if got != "Hi\nthere" {
		t.Fatalf("screen = %q", got)
	}
	// A wider grid still shows the same text; the extra columns are blank.
	wide := screen(raw, 40, 8)
	if wide != "Hi\nthere" {
		t.Fatalf("wide screen = %q", wide)
	}
}
