// Package cli defines omega's command-line interface.
package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/runner"
	"github.com/lcajigasm/omega/internal/secrets"
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
}

// Execute runs the root command.
func Execute(version string) error {
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
		ValidArgs:     []string{agent.TaskFull, agent.TaskUpdate, agent.TaskClean, agent.TaskAnalyze, agent.TaskFix},
		RunE: func(cmd *cobra.Command, args []string) error {
			task := agent.TaskFull
			if len(args) == 1 {
				task = args[0]
			}
			return runTask(task, f)
		},
	}
	pf := root.Flags()
	pf.StringVar(&f.provider, "provider", "", i18n.T("flag.provider"))
	pf.StringVarP(&f.model, "model", "m", "", i18n.T("flag.model"))
	pf.StringVar(&f.effort, "effort", "", i18n.T("flag.effort"))
	pf.BoolVarP(&f.yes, "yes", "y", false, i18n.T("flag.yes"))
	pf.BoolVar(&f.noTUI, "no-tui", false, i18n.T("flag.no_tui"))
	pf.BoolVarP(&f.dryRun, "dry-run", "n", false, i18n.T("flag.dry_run"))
	pf.BoolVar(&f.sudo, "sudo", false, i18n.T("flag.sudo"))
	pf.StringVarP(&f.prompt, "prompt", "p", "", i18n.T("flag.prompt"))
	pf.BoolVar(&f.once, "once", false, i18n.T("flag.once"))

	root.AddCommand(setupCmd(), authCmd(), modelsCmd(), configCmd(), stepsCmd(), notesCmd())
	return root.Execute()
}

// resolveProvider builds the provider and model from flags and config.
func resolveProvider(cfg *config.Config, name, model, effort string) (llm.Provider, string, error) {
	if name == "" {
		name = cfg.Provider
	}
	info, ok := llm.Lookup(name)
	if !ok {
		return nil, "", fmt.Errorf("%s", i18n.T("err.unknown_provider", name, providerNames()))
	}
	if model == "" {
		model = cfg.ModelFor(name)
	}
	if model == "" {
		model = info.DefaultModel
	}
	if effort == "" {
		effort = cfg.Effort
	}
	key, _, err := secrets.Get(name)
	if err != nil {
		return nil, "", err
	}
	p, err := llm.New(name, llm.Options{APIKey: key, BaseURL: cfg.BaseURLFor(name), Effort: effort})
	return p, model, err
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
	readOnly := f.dryRun || task == agent.TaskAnalyze

	p, model, err := resolveProvider(cfg, f.provider, f.model, f.effort)
	if err != nil {
		return err
	}
	timeout, err := time.ParseDuration(cfg.Timeout)
	if err != nil || timeout <= 0 {
		timeout = 45 * time.Minute
	}

	stdinTTY := term.IsTerminal(int(os.Stdin.Fd()))
	stdoutTTY := term.IsTerminal(int(os.Stdout.Fd()))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if f.sudo && !readOnly {
		if err := startSudo(ctx); err != nil {
			return err
		}
	}

	ag := agent.New(p, agent.Options{
		Model:       model,
		Approval:    cfg.Approval,
		AutoYes:     f.yes,
		ReadOnly:    readOnly,
		Interactive: stdinTTY && !f.once,
		Timeout:     timeout,
		Config:      cfg,
	})
	go ag.Run(ctx, prompt)

	if f.noTUI || !stdinTTY || !stdoutTTY {
		ui.RunPlain(ag, os.Stdin, stdinTTY)
		return nil
	}
	err = ui.RunTUI(ctx, cancel, ag, ui.Header{Provider: p.Name(), Model: model, Task: task, ReadOnly: readOnly})
	cancel()
	// Drain so the agent goroutine can finish writing its log.
	for range ag.Events() {
	}
	if path := ag.LogPath(); path != "" {
		fmt.Println(i18n.T("run.log", path))
	}
	return err
}

// startSudo validates sudo credentials up front and refreshes them in the
// background so steps can use `sudo -n` without prompting.
func startSudo(ctx context.Context) error {
	fmt.Println(i18n.T("sudo.intro"))
	c := exec.Command("sudo", "-v")
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("sudo -v: %w", err)
	}
	go func() {
		t := time.NewTicker(4 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				exec.Command("sudo", "-n", "-v").Run()
			}
		}
	}()
	return nil
}

func providerNames() string {
	names := make([]string, 0, len(llm.Providers))
	for _, p := range llm.Providers {
		names = append(names, p.Name)
	}
	return strings.Join(names, ", ")
}
