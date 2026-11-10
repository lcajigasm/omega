//go:build gui

package main

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"time"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/cli"
	"github.com/lcajigasm/omega/internal/gui"
	"github.com/lcajigasm/omega/internal/i18n"
)

// App is what the window can ask of omega. Every exported method is callable
// from the frontend as main.App.<Method>.
type App struct {
	emit func(gui.Message)
	demo bool

	mu     sync.Mutex
	bridge *gui.Bridge
	cancel context.CancelFunc
	done   chan struct{}
}

// Task is a preset the home screen offers.
type Task struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

// Info is what the frontend needs before any session exists.
type Info struct {
	Version string `json:"version"`
	OS      string `json:"os"`
	Lang    string `json:"lang"`
	Demo    bool   `json:"demo"`
	Tagline string `json:"tagline"`
	Tasks   []Task `json:"tasks"`
	// Provider is the configured provider; Ready says it can be used as it
	// is (it has a key, or needs none).
	Provider string            `json:"provider"`
	Ready    bool              `json:"ready"`
	Text     map[string]string `json:"text"`
}

// SessionInfo describes a session that has just started.
type SessionInfo struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Task     string `json:"task"`
	Mode     string `json:"mode"`
}

var homeTasks = []string{agent.TaskFull, agent.TaskUpdate, agent.TaskClean, agent.TaskAnalyze, agent.TaskFix, agent.TaskTune}

// Info returns the texts and the tasks of the home screen.
func (a *App) Info() Info {
	in := Info{Version: cli.Version(), OS: runtime.GOOS, Lang: i18n.Lang(), Demo: a.demo,
		Tagline: i18n.T("ui.welcome_tagline"),
		Text:    i18n.WithPrefix("gui.", "setup.no_key", "ui.mode_", "ui.password_", "ui.always_note", "ui.denied", "ui.confirm", "ui.model_switch")}
	in.Provider, in.Ready = cli.CurrentProvider()
	for _, id := range homeTasks {
		in.Tasks = append(in.Tasks, Task{ID: id, Title: i18n.T("gui.task." + id), Detail: i18n.T("gui.task." + id + ".detail")})
	}
	return in
}

// Providers lists the providers for the setup screen.
func (a *App) Providers() ([]cli.ProviderStatus, error) { return cli.SetupProviders() }

// SaveSetup makes provider the one omega uses. key may be empty to keep the
// stored one; it is checked with the provider before it is stored.
func (a *App) SaveSetup(provider, key, baseURL string) (cli.SetupResult, error) {
	return cli.SaveSetup(provider, key, baseURL)
}

var errRunning = errors.New("a session is already running")

// Start begins a session: a preset task, or prompt as free text.
func (a *App) Start(task, prompt string, dryRun bool) (SessionInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.bridge != nil {
		return SessionInfo{}, errRunning
	}
	ctx, cancel := context.WithCancel(context.Background())
	s, err := cli.StartSession(ctx, cli.SessionRequest{Task: task, Prompt: prompt, DryRun: dryRun, Demo: a.demo})
	if err != nil {
		cancel()
		return SessionInfo{}, err
	}
	b := gui.New(ctx, s.Agent, a.emit)
	done := make(chan struct{})
	a.bridge, a.cancel, a.done = b, cancel, done
	go func() {
		b.Run() // returns when the agent has closed its transcript
		cancel()
		a.mu.Lock()
		if a.bridge == b {
			a.bridge, a.cancel = nil, nil
		}
		a.mu.Unlock()
		close(done)
	}()
	return SessionInfo{Provider: s.Provider, Model: s.Model, Task: s.Task, Mode: b.Mode()}, nil
}

func (a *App) session() (*gui.Bridge, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.bridge == nil {
		return nil, errors.New("no session is running")
	}
	return a.bridge, nil
}

// Send gives the agent a follow-up message.
func (a *App) Send(text string) error {
	b, err := a.session()
	if err != nil {
		return err
	}
	return b.Send(text)
}

// Confirm answers an approval card: "approve", "all" or "deny".
func (a *App) Confirm(id int, decision string) error {
	b, err := a.session()
	if err != nil {
		return err
	}
	return b.Confirm(id, decision)
}

// Choose answers a selection card with the picked IDs.
func (a *App) Choose(id int, picks []string) error {
	b, err := a.session()
	if err != nil {
		return err
	}
	return b.Choose(id, picks)
}

// Password answers a sudo prompt; "" skips what needs administrator access.
func (a *App) Password(id int, secret string) error {
	b, err := a.session()
	if err != nil {
		return err
	}
	return b.Password(id, secret)
}

// SetMode changes the approval mode and returns the one now in force.
func (a *App) SetMode(mode string) (string, error) {
	b, err := a.session()
	if err != nil {
		return "", err
	}
	if err := b.SetMode(mode); err != nil {
		return "", err
	}
	return b.Mode(), nil
}

// Stop cancels the running session. The frontend hears "closed" when the
// agent has finished.
func (a *App) Stop() {
	a.mu.Lock()
	cancel := a.cancel
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// shutdown stops the session and gives the agent a moment to close its log.
func (a *App) shutdown() {
	a.mu.Lock()
	cancel, done := a.cancel, a.done
	a.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}
}
