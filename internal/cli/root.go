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
	var f runFlags

	root := &cobra.Command{
		Use:   "omega [tarea]",
		Short: "Mantenimiento de macOS guiado por IA: actualiza (topgrade) y limpia (Mole)",
		Long: `omega usa un modelo de IA (Anthropic, OpenAI, OpenRouter u OpenCode) para mantener tu Mac:
actualiza todo el software, limpia lo que sobra, diagnostica los errores y los corrige.

Tareas:
  full      mantenimiento completo (por defecto)
  update    solo actualizar (equivalente a topgrade)
  clean     solo limpiar (equivalente a mo clean)
  analyze   informe sin cambiar nada
  fix       diagnosticar y arreglar la configuración de topgrade y Mole

Ejemplos:
  omega setup                       configurar proveedor, API key y modelo
  omega                             mantenimiento completo
  omega update --yes                actualizar sin preguntar
  omega -p "¿por qué falla pipx?"   tarea libre
  omega --provider openrouter --model openai/gpt-5.5 analyze`,
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
	pf.StringVar(&f.provider, "provider", "", "proveedor: anthropic, openai, openrouter, opencode")
	pf.StringVarP(&f.model, "model", "m", "", "modelo a usar (ver `omega models`)")
	pf.StringVar(&f.effort, "effort", "", "esfuerzo para modelos Claude: low, medium, high, xhigh, max")
	pf.BoolVarP(&f.yes, "yes", "y", false, "aprobar automáticamente todo excepto comandos bloqueados")
	pf.BoolVar(&f.noTUI, "no-tui", false, "salida de texto simple en vez de interfaz a pantalla completa")
	pf.BoolVarP(&f.dryRun, "dry-run", "n", false, "modo solo lectura: el agente no puede modificar nada")
	pf.BoolVar(&f.sudo, "sudo", false, "pedir la contraseña de sudo al inicio y mantenerla durante la ejecución")
	pf.StringVarP(&f.prompt, "prompt", "p", "", "tarea libre en lenguaje natural en lugar de una tarea predefinida")
	pf.BoolVar(&f.once, "once", false, "terminar cuando el agente acabe, sin pedir más instrucciones")

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
		_, err := llm.New(name, llm.Options{})
		return nil, "", err
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
		return nil, "", fmt.Errorf("%w\nEjecuta `omega setup` para configurarlo", err)
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
			return fmt.Errorf("tarea desconocida %q (full, update, clean, analyze, fix)", task)
		}
	} else {
		task = "personalizada"
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
		fmt.Println("Registro de la ejecución:", path)
	}
	return err
}

// startSudo validates sudo credentials up front and refreshes them in the
// background so steps can use `sudo -n` without prompting.
func startSudo(ctx context.Context) error {
	fmt.Println("omega necesita sudo para algunos pasos (p. ej. actualizaciones de macOS).")
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
