package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/runner"
)

type noopChat struct{}

func (noopChat) Send(context.Context, string, []llm.ToolResult) (*llm.Reply, error) {
	return &llm.Reply{Text: "ok"}, nil
}
func (noopChat) SetModel(string) {}

type noopProvider struct{}

func (noopProvider) Name() string                             { return "test" }
func (noopProvider) Models(context.Context) ([]string, error) { return nil, nil }
func (noopProvider) NewChat(string, string, []llm.ToolSpec) llm.Chat {
	return noopChat{}
}

func testModel(t *testing.T) *model {
	t.Helper()
	t.Setenv("OMEGA_LANG", "")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	i18n.Init("en")
	a := agent.New(noopProvider{}, agent.Options{
		Interactive: true,
		Config:      config.Default(),
		Timeout:     time.Second,
		Exec: func(context.Context, string, time.Duration, func(string)) runner.Result {
			return runner.Result{}
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		a.Run(ctx, "test")
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	m := &model{
		ag: a, cancel: func() {}, state: stRunning,
		input: textinput.New(), started: time.Now(),
		header: Header{Model: "test-model", Models: []string{"test-model"}},
	}
	m.password = textinput.New()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	return m
}

func runeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func TestHelpOverlay(t *testing.T) {
	m := testModel(t)
	m.Update(runeKey('?'))
	if !m.help {
		t.Fatal("? did not open help")
	}
	view := m.View()
	if !strings.Contains(view, "Keyboard shortcuts") || !strings.Contains(view, "/mode") {
		t.Fatalf("help view missing shortcuts:\n%s", view)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.help {
		t.Fatal("esc did not close help")
	}

	reply := make(chan agent.Decision, 1)
	m.state = stConfirm
	m.pending = &agent.Event{Reply: reply, Tool: "run_command", Command: "rm x"}
	m.Update(runeKey('?'))
	if !m.help {
		t.Fatal("? during an approval should open help")
	}
	select {
	case d := <-reply:
		t.Fatalf("help key sent decision %v", d)
	default:
	}
}

func TestQuestionMarkStaysInPassword(t *testing.T) {
	m := testModel(t)
	m.state = stPassword
	m.password.Focus()
	m.Update(runeKey('?'))
	if m.help {
		t.Fatal("? inside the password card opened help")
	}
	if !strings.Contains(m.password.Value(), "?") {
		t.Fatalf("password field = %q", m.password.Value())
	}
}

func TestEscClearsInputBeforeQuit(t *testing.T) {
	m := testModel(t)
	m.state = stIdle
	m.input.Focus()
	m.input.SetValue("hello")
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.input.Value() != "" {
		t.Fatalf("input = %q, want empty", m.input.Value())
	}
	if m.state != stIdle {
		t.Fatalf("first esc left state %v", m.state)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.state != stRunning {
		t.Fatalf("second esc left state %v, want the session to end", m.state)
	}
}
