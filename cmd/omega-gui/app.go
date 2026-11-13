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
	// starting is set while a session is being prepared: there is no
	// bridge yet, and a second Start must not begin another.
	starting bool
	cancel   context.CancelFunc
	done     chan struct{}
	prompt   string
	// The session on show, kept so that a window drawn again (a reload)
	// can show it and go on: its messages in order, and what it was asked.
	seq   int
	log   []gui.Message
	title SessionInfo
	// quitting is set once the user has agreed to end the session by
	// closing the window.
	quitting bool
	quit     func()
}

// maxLog is how many messages of a session are kept for a window drawn
// again. Past it, lines of output are no longer kept; the rest is.
const maxLog = 20000

// send numbers a message, keeps it and hands it to the window.
func (a *App) send(m gui.Message) {
	a.mu.Lock()
	a.seq++
	m.Seq = a.seq
	if len(a.log) < maxLog || m.Kind != "output" {
		a.log = append(a.log, m)
	}
	// Emitted under the lock: the window must hear them in order.
	a.emit(m)
	a.mu.Unlock()
}

// SessionState is the session the app holds, for a window that has just
// been drawn.
type SessionState struct {
	Running  bool          `json:"running"`
	Info     SessionInfo   `json:"info"`
	Prompt   string        `json:"prompt"`
	Messages []gui.Message `json:"messages"`
}

// Session returns the session on show: the running one, or the last one
// that ended. A window that was reloaded draws it again from here.
func (a *App) Session() SessionState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return SessionState{Running: a.bridge != nil || a.starting, Info: a.title, Prompt: a.prompt,
		Messages: append([]gui.Message{}, a.log...)}
}

// local puts an error of the session in the user's language.
func local(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, errRunning):
		return errors.New(i18n.T("gui.err.running"))
	case errors.Is(err, errNoSession):
		return errors.New(i18n.T("gui.err.no_session"))
	case errors.Is(err, gui.ErrNoRequest):
		return errors.New(i18n.T("gui.err.no_request"))
	case errors.Is(err, gui.ErrBusy):
		return errors.New(i18n.T("gui.err.busy"))
	case errors.Is(err, context.Canceled):
		return errors.New(i18n.T("gui.err.stopped"))
	}
	return err
}

// Task is a preset the home screen offers.
type Task struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

// Language is one language the interface can be shown in.
type Language struct {
	Code string `json:"code"`
	Name string `json:"name"` // in that language
}

// Info is what the frontend needs before any session exists.
type Info struct {
	Version string `json:"version"`
	OS      string `json:"os"`
	Lang    string `json:"lang"`
	Demo    bool   `json:"demo"`
	Tagline string `json:"tagline"`
	// Tasks are the presets; the first is the one the home screen leads with.
	Tasks     []Task     `json:"tasks"`
	Languages []Language `json:"languages"`
	// Provider is the configured provider; Ready says it can be used as it
	// is (it has a key, or needs none).
	Provider string `json:"provider"`
	Ready    bool   `json:"ready"`
	// LastRun says how long ago the last session was; empty when none.
	LastRun string            `json:"lastRun"`
	Text    map[string]string `json:"text"`
}

// SessionInfo describes a session that has just started.
type SessionInfo struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Task     string `json:"task"`
	Mode     string `json:"mode"`
}

// homeTasks are the presets in the order the home screen shows them. The
// ones this system has no prompt for are left out.
var homeTasks = []string{agent.TaskFull, agent.TaskQuick, agent.TaskUpdate, agent.TaskClean, agent.TaskAnalyze, agent.TaskHealth,
	agent.TaskFix, agent.TaskTune, agent.TaskDev, agent.TaskMac, agent.TaskDocker}

// Info returns the texts and the tasks of the home screen. The frontend asks
// again after the language changes.
func (a *App) Info() Info {
	in := Info{Version: cli.Version(), OS: runtime.GOOS, Lang: i18n.Lang(), Demo: a.demo,
		Tagline: i18n.T("ui.welcome_tagline"), LastRun: cli.LastRun(),
		Text: i18n.WithPrefix("gui.", "setup.no_key", "ui.mode_", "ui.password_", "ui.always_note", "ui.denied", "ui.confirm",
			"ui.model_switch", "logs.empty", "notes.empty")}
	in.Provider, in.Ready = cli.CurrentProvider()
	for _, id := range homeTasks {
		if _, ok := agent.TaskPrompts[id]; !ok || (id == agent.TaskMac && runtime.GOOS != "darwin") {
			continue
		}
		in.Tasks = append(in.Tasks, Task{ID: id, Title: i18n.T("gui.task." + id), Detail: i18n.T("gui.task." + id + ".detail")})
	}
	for _, code := range i18n.Supported {
		in.Languages = append(in.Languages, Language{Code: code, Name: i18n.Native[code]})
	}
	return in
}

// Settings returns the values of the settings screen.
func (a *App) Settings() (cli.Settings, error) { return cli.GetSettings() }

// SetSetting changes one setting: language, approval, effort,
// command_timeout or model_catalog.
func (a *App) SetSetting(key, value string) error {
	switch key {
	case "language", "approval", "effort", "command_timeout", "model_catalog":
		return cli.SetConfig(key, value)
	}
	return errors.New("unknown setting: " + key)
}

// Models lists what the configured provider's model can be set to.
func (a *App) Models() ([]cli.ModelChoice, error) {
	if a.demo {
		return []cli.ModelChoice{{ID: "auto", Label: i18n.T("model.auto")}}, nil
	}
	return cli.ModelChoices()
}

// SetModel fixes the model, or frees it with "auto".
func (a *App) SetModel(choice string) error { return cli.SetModel(choice) }

// Steps lists the maintenance steps of this machine.
func (a *App) Steps() ([]cli.StepInfo, error) { return cli.StepList() }

// SkipStep turns a step off or back on.
func (a *App) SkipStep(id string, skip bool) error { return cli.SkipStep(id, skip) }

// Check runs the setup checks of `omega doctor`.
func (a *App) Check() cli.Check { return cli.RunCheck() }

// Runs lists the saved sessions, newest first.
func (a *App) Runs() ([]cli.RunInfo, error) { return cli.RunList() }

// RunText returns the transcript of a saved session.
func (a *App) RunText(name string) (string, error) { return cli.RunText(name) }

// Notes is what omega remembers about this machine.
func (a *App) Notes() string { return cli.Notes() }

// ClearNotes forgets it.
func (a *App) ClearNotes() error { return cli.ClearNotes() }

// Tools lists the checks that need no model: network, security, disk, git
// and the system, the same commands the terminal has.
func (a *App) Tools() []cli.ToolInfo { return cli.ToolList() }

// RunTool runs one of them and returns what it printed. arg is its optional
// argument: a port, a folder, a number of days or a file.
func (a *App) RunTool(id, arg string) (string, error) { return cli.RunTool(id, arg) }

// Skills lists the AI coding agents of this machine and their skills.
func (a *App) Skills() (cli.SkillsView, error) { return cli.SkillList() }

// SkillAction installs ("install", arg is a folder, a git URL or
// owner/repo), shares ("sync"), updates ("update") or removes ("remove",
// arg is the location or empty for every one) a skill, and returns what the
// command printed. An empty name means every skill for sync and update.
func (a *App) SkillAction(action, name, arg string) (string, error) {
	return cli.SkillAction(action, name, arg)
}

// Providers lists the providers for the setup screen.
func (a *App) Providers() ([]cli.ProviderStatus, error) { return cli.SetupProviders() }

// SaveSetup makes provider the one omega uses. key may be empty to keep the
// stored one; it is checked with the provider before it is stored.
func (a *App) SaveSetup(provider, key, baseURL string) (cli.SetupResult, error) {
	return cli.SaveSetup(provider, key, baseURL)
}

var (
	errRunning   = errors.New("a session is already running")
	errNoSession = errors.New("no session is running")
)

// Start begins a session: a preset task, or prompt as free text.
func (a *App) Start(task, prompt string, dryRun bool) (SessionInfo, error) {
	// The place is taken under the lock, and the lock is let go while the
	// provider and the model are looked up: that goes over the network, and
	// Stop and closing the window must not wait for it. Stop cancels ctx.
	a.mu.Lock()
	if a.bridge != nil || a.starting {
		a.mu.Unlock()
		return SessionInfo{}, local(errRunning)
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.starting, a.cancel = true, cancel
	a.seq, a.log, a.prompt = 0, nil, prompt
	a.title = SessionInfo{Task: task}
	a.mu.Unlock()

	s, err := cli.StartSession(ctx, cli.SessionRequest{Task: task, Prompt: prompt, DryRun: dryRun, Demo: a.demo})

	a.mu.Lock()
	defer a.mu.Unlock()
	a.starting = false
	if err != nil {
		cancel()
		a.cancel = nil
		return SessionInfo{}, local(err)
	}
	b := gui.New(ctx, s.Agent, a.send)
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
	a.title = SessionInfo{Provider: s.Provider, Model: s.Model, Task: s.Task, Mode: b.Mode()}
	return a.title, nil
}

func (a *App) session() (*gui.Bridge, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.bridge == nil {
		return nil, local(errNoSession)
	}
	return a.bridge, nil
}

// Send gives the agent a follow-up message.
func (a *App) Send(text string) error {
	b, err := a.session()
	if err != nil {
		return err
	}
	return local(b.Send(text))
}

// Confirm answers an approval card: "approve", "all" or "deny".
func (a *App) Confirm(id int, decision string) error {
	b, err := a.session()
	if err != nil {
		return err
	}
	return local(b.Confirm(id, decision))
}

// Choose answers a selection card with the picked IDs.
func (a *App) Choose(id int, picks []string) error {
	b, err := a.session()
	if err != nil {
		return err
	}
	return local(b.Choose(id, picks))
}

// Password answers a sudo prompt; "" skips what needs administrator access.
func (a *App) Password(id int, secret string) error {
	b, err := a.session()
	if err != nil {
		return err
	}
	return local(b.Password(id, secret))
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
	a.mu.Lock()
	a.title.Mode = b.Mode()
	a.mu.Unlock()
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

// Busy reports whether a session is running or starting.
func (a *App) Busy() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.bridge != nil || a.starting
}

// closing is asked when the window is about to close. It answers whether
// it may: with a session running the window stays and asks the user, since
// closing stops the command in flight.
func (a *App) closing() bool {
	a.mu.Lock()
	busy := (a.bridge != nil || a.starting) && !a.quitting
	a.mu.Unlock()
	if busy {
		a.emit(gui.Message{Kind: "quit_ask"})
	}
	return !busy
}

// Quit ends the session and closes the app: the answer to "quit_ask".
func (a *App) Quit() {
	a.mu.Lock()
	a.quitting = true
	quit := a.quit
	a.mu.Unlock()
	if quit != nil {
		go quit()
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
	if done == nil {
		return // still starting: nothing to wait for
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}
}
