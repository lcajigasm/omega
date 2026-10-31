// Package cli defines omega's command-line interface.
package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/term"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/demo"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/runner"
	"github.com/lcajigasm/omega/internal/secrets"
	"github.com/lcajigasm/omega/internal/sudo"
	"github.com/lcajigasm/omega/internal/ui"
)

type runFlags struct {
	provider string
	model    string
	effort   string
	yes      bool
	noTUI    bool
	dryRun   bool
	sudo     bool
	prompt   string
	once     bool
	demo     bool
	auto     bool
}

// version is set by Execute for the TUI header.
var version string

// Execute runs the root command.
func Execute(v string) error {
	version = v
	runner.ExtendPath()
	// The language must be known before the commands' help text is built.
	if cfg, err := config.Load(); err == nil {
		i18n.Init(cfg.Language)
	} else {
		i18n.Init("")
	}
	var f runFlags

	root := &cobra.Command{
		Use:           "omega [task]",
		Short:         i18n.T("cli.short"),
		Long:          i18n.T("cli.long"),
		Version:       version,
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		ValidArgs:     []string{agent.TaskFull, agent.TaskUpdate, agent.TaskClean, agent.TaskAnalyze, agent.TaskFix, agent.TaskTune},
		RunE: func(cmd *cobra.Command, args []string) error {
			task := agent.TaskFull
			if len(args) == 1 {
				task = args[0]
			}
			return runTask(task, f)
		},
	}
	bindRunFlags(root.Flags(), &f)

	root.AddCommand(setupCmd(), authCmd(), modelCmd(), modelsCmd(), dockerCmd(), macCmd(), configCmd(), stepsCmd(), notesCmd(), logsCmd(), doctorCmd())
	return root.Execute()
}

// bindRunFlags registers the flags of an agent run.
func bindRunFlags(pf *pflag.FlagSet, f *runFlags) {
	pf.StringVar(&f.provider, "provider", "", i18n.T("flag.provider"))
	pf.StringVarP(&f.model, "model", "m", "", i18n.T("flag.model"))
	pf.StringVar(&f.effort, "effort", "", i18n.T("flag.effort"))
	pf.BoolVarP(&f.yes, "yes", "y", false, i18n.T("flag.yes"))
	pf.BoolVar(&f.noTUI, "no-tui", false, i18n.T("flag.no_tui"))
	pf.BoolVarP(&f.dryRun, "dry-run", "n", false, i18n.T("flag.dry_run"))
	pf.BoolVar(&f.sudo, "sudo", false, i18n.T("flag.sudo"))
	pf.StringVarP(&f.prompt, "prompt", "p", "", i18n.T("flag.prompt"))
	pf.BoolVar(&f.once, "once", false, i18n.T("flag.once"))
	pf.BoolVar(&f.auto, "auto", false, i18n.T("flag.auto"))
	pf.BoolVar(&f.demo, "demo", false, "scripted session with no API calls and no real commands (for screenshots)")
	pf.MarkHidden("demo")
}

var getKey = secrets.Get

// resolveProvider builds the provider and the model plan for a task.
func resolveProvider(cfg *config.Config, name, model, effort, task string) (llm.Provider, modelPlan, error) {
	if name == "" {
		name = cfg.Provider
	}
	info, ok := llm.Lookup(name)
	if !ok {
		return nil, modelPlan{}, fmt.Errorf("%s", i18n.T("err.unknown_provider", name, providerNames()))
	}
	if effort == "" {
		effort = cfg.Effort
	}
	plan, err := planModel(cfg, info, model, task)
	if err != nil {
		return nil, plan, err
	}
	p, err := newProvider(cfg, info, effort)
	return p, plan, err
}

func runTask(task string, f runFlags) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	prompt := f.prompt
	if prompt == "" {
		var ok bool
		prompt, ok = agent.TaskPrompts[task]
		if !ok {
			return fmt.Errorf("%s", i18n.T("err.unknown_task", task))
		}
	} else {
		task = i18n.T("task.custom")
	}
	if f.auto {
		prompt += agent.AutoSuffix
	}
	readOnly := f.dryRun || task == agent.TaskAnalyze

	planTask := task
	if f.prompt != "" {
		planTask = "custom"
	}
	var p llm.Provider
	var plan modelPlan
	if f.demo {
		p, plan = demo.Provider(), modelPlan{Model: "claude-haiku-5-5", Escalate: "claude-sonnet-5-5",
			Choices: []string{"claude-haiku-5-5", "claude-sonnet-5-5"}}
	} else if p, plan, err = resolveProvider(cfg, f.provider, f.model, f.effort, planTask); err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	timeout, parseErr := time.ParseDuration(cfg.Timeout)
	if parseErr != nil || timeout <= 0 {
		timeout = 45 * time.Minute
	}

	stdinTTY := term.IsTerminal(int(os.Stdin.Fd()))
	stdoutTTY := term.IsTerminal(int(os.Stdout.Fd()))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	sudoPassword := ""
	if f.sudo && !readOnly && stdinTTY {
		if sudoPassword, err = askSudo(); err != nil {
			return err
		}
	}

	opts := agent.Options{
		Model:        plan.Model,
		Escalate:     plan.Escalate,
		Approval:     cfg.Approval,
		AutoYes:      f.yes,
		ReadOnly:     readOnly,
		Interactive:  stdinTTY && !f.once,
		Timeout:      timeout,
		Config:       cfg,
		SudoPassword: sudoPassword,
	}
	if f.demo {
		opts.Provider = "anthropic"
		opts.Exec = demo.Exec
		opts.SkipDetect = true
		opts.Interactive = stdinTTY
	}
	ag := agent.New(p, opts)
	go ag.Run(ctx, prompt)

	if f.noTUI || !stdinTTY || !stdoutTTY {
		ui.RunPlain(ag, os.Stdin, stdinTTY)
	} else {
		err = ui.RunTUI(ctx, cancel, ag, ui.Header{Version: version, Provider: p.Name(), Model: plan.Model,
			Escalate: plan.Escalate, Models: plan.Choices, Task: task})
		cancel()
		// Drain so the agent goroutine can finish writing its log.
		for range ag.Events() {
		}
		if path := ag.LogPath(); path != "" {
			fmt.Println(i18n.T("run.log", path))
		}
	}
	if err != nil {
		return err
	}
	if code := ag.ExitCode(); code != 0 {
		return exitCode(code)
	}
	return nil
}

// exitCode is a finished run whose failure was already shown in the UI.
type exitCode int

func (e exitCode) Error() string { return "" }
func (e exitCode) ExitCode() int { return int(e) }

// askSudo asks for the sudo password before the interface starts and checks
// it, so sudo prompts during the run are answered without interrupting.
func askSudo() (string, error) {
	fmt.Println(i18n.T("sudo.intro"))
	for i := 0; i < 3; i++ {
		pw, err := askSecret(i18n.T("sudo.prompt"))
		if err != nil || pw == "" {
			return "", fmt.Errorf("%s", i18n.T("sudo.cancelled"))
		}
		if sudo.Validate(pw) {
			return pw, nil
		}
		fmt.Println(i18n.T("sudo.wrong"))
	}
	return "", fmt.Errorf("%s", i18n.T("sudo.cancelled"))
}

func providerNames() string {
	names := make([]string, 0, len(llm.Providers))
	for _, p := range llm.Providers {
		names = append(names, p.Name)
	}
	return strings.Join(names, ", ")
}
