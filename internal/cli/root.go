// Package cli defines omega's command-line interface.
package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"slices"
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
	withholdProviderKeys()
	// The language must be known before the commands' help text is built.
	if cfg, err := config.Load(); err == nil {
		i18n.Init(cfg.Language)
	} else {
		i18n.Init("")
	}
	var f runFlags
	root := newRoot(&f)
	// A unique start of a command is enough: omega doc, omega mac perm.
	cobra.EnablePrefixMatching = true
	root.SetArgs(expandShortcut(root, os.Args[1:]))
	return root.Execute()
}

// newRoot builds the command tree. The documentation test walks it too.
func newRoot(f *runFlags) *cobra.Command {
	root := &cobra.Command{
		Use:           "omega [task]",
		Short:         i18n.T("cli.short"),
		Long:          i18n.T("cli.long"),
		Version:       version,
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		ValidArgs:     []string{agent.TaskFull, agent.TaskUpdate, agent.TaskClean, agent.TaskAnalyze, agent.TaskFix, agent.TaskTune, agent.TaskDev},
		RunE: func(cmd *cobra.Command, args []string) error {
			task, prompt, err := interpret(cmd.Root(), args)
			if err != nil {
				return err
			}
			run := *f
			if prompt != "" && run.prompt == "" {
				run.prompt = prompt
			}
			return runTask(task, run)
		},
	}
	bindRunFlags(root.Flags(), f)

	root.AddCommand(fullCmd(), setupCmd(), authCmd(), modelCmd(), modelsCmd(), dockerCmd(), macCmd(), devCmd(), menuCmd(), configCmd(), stepsCmd(), notesCmd(), logsCmd(), doctorCmd())
	return root
}

// taskWords are other words for the tasks, in the five interface languages:
// what someone types before reading the help.
var taskWords = map[string]string{
	"maintain": agent.TaskFull, "maintenance": agent.TaskFull, "run": agent.TaskFull,
	"completo": agent.TaskFull, "mantenimiento": agent.TaskFull, "tout": agent.TaskFull, "alles": agent.TaskFull, "tudo": agent.TaskFull,
	"upgrade": agent.TaskUpdate, "updates": agent.TaskUpdate, "actualizar": agent.TaskUpdate, "actualiza": agent.TaskUpdate,
	"aktualisieren": agent.TaskUpdate, "atualizar": agent.TaskUpdate,
	"cleanup": agent.TaskClean, "clear": agent.TaskClean, "limpiar": agent.TaskClean, "limpia": agent.TaskClean, "limpieza": agent.TaskClean,
	"nettoyer": agent.TaskClean, "limpar": agent.TaskClean,
	"analyse": agent.TaskAnalyze, "analysis": agent.TaskAnalyze, "report": agent.TaskAnalyze,
	"analizar": agent.TaskAnalyze, "analiza": agent.TaskAnalyze, "informe": agent.TaskAnalyze,
	"analyser": agent.TaskAnalyze, "analysieren": agent.TaskAnalyze, "analisar": agent.TaskAnalyze,
	"repair": agent.TaskFix, "reparar": agent.TaskFix, "arreglar": agent.TaskFix, "reparieren": agent.TaskFix,
	"optimize": agent.TaskTune, "optimise": agent.TaskTune, "optimizar": agent.TaskTune, "ajustar": agent.TaskTune, "optimiser": agent.TaskTune, "otimizar": agent.TaskTune,
}

// shortcuts are words that stand for a longer command line: the commands
// people reach for at the top level although they live under another one.
var shortcuts = map[string][]string{
	"permissions": {"mac", "permissions"}, "perms": {"mac", "permissions"}, "permisos": {"mac", "permissions"},
	"apps": {"mac", "apps"}, "undo": {"mac", "undo"}, "deshacer": {"mac", "undo"},
	"login": {"mac", "login"}, "shortcuts": {"mac", "shortcuts"}, "atajos": {"mac", "shortcuts"},
	"dock": {"mac", "dock"}, "menubar": {"mac", "menubar"}, "background": {"mac", "background"},
	"install": {"dev", "install"}, "instalar": {"dev", "install"}, "activate": {"dev", "activate"},
	"history": {"logs"}, "historial": {"logs"}, "log": {"logs"}, "runs": {"logs"},
	"settings": {"config", "show"}, "ajustes": {"config", "show"}, "configuracion": {"config", "show"},
	"key": {"auth", "status"}, "keys": {"auth", "status"}, "claves": {"auth", "status"},
	"version": {"--version"}, "ayuda": {"--help"},
	"start": {"menu"}, "ui": {"menu"}, "inicio": {"menu"}, "menú": {"menu"},
	// A bare "status" or "check" gets the check that costs nothing.
	"diagnose": {"doctor"}, "health": {"doctor"}, "status": {"doctor"}, "check": {"doctor"}, "estado": {"doctor"}, "ranking": {"models"}, "modelos": {"models"},
	"memory": {"notes"}, "notas": {"notes"}, "pasos": {"steps"}, "wizard": {"setup"}, "configure": {"setup"}, "init": {"setup"},
}

// expandShortcut rewrites a command line whose first word is a shortcut.
// A word that is a real command or a task is left alone.
func expandShortcut(root *cobra.Command, args []string) []string {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return args
	}
	word := strings.ToLower(args[0])
	// Only an exact command name wins over a shortcut: "dock" is the Dock,
	// not the start of "docker".
	for _, c := range root.Commands() {
		if c.Name() == word || c.HasAlias(word) {
			return args
		}
	}
	if slices.Contains(rootTasks, word) {
		return args
	}
	if to, ok := shortcuts[word]; ok {
		return append(append([]string(nil), to...), args[1:]...)
	}
	return args
}

// taskFor resolves a word to a task: its name, another word for it, or the
// start of one when only one task starts that way.
func taskFor(word string) (string, bool) {
	word = strings.ToLower(word)
	for _, t := range rootTasks {
		if t == word {
			return t, true
		}
	}
	if t, ok := taskWords[word]; ok {
		return t, true
	}
	match := ""
	for _, t := range rootTasks {
		if len(word) >= 2 && strings.HasPrefix(t, word) {
			if match != "" {
				return "", false
			}
			match = t
		}
	}
	return match, match != ""
}

// rootTasks are the tasks `omega <task>` starts.
var rootTasks = []string{agent.TaskFull, agent.TaskUpdate, agent.TaskClean, agent.TaskAnalyze, agent.TaskFix, agent.TaskTune}

// editDistance is the Levenshtein distance between two words.
func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}

// suggest names the commands and tasks closest to a word nobody recognised.
func suggest(root *cobra.Command, word string) []string {
	word = strings.ToLower(word)
	var names []string
	for _, c := range root.Commands() {
		if !c.Hidden && c.Name() != "help" && c.Name() != "completion" {
			names = append(names, c.Name())
		}
	}
	names = append(names, rootTasks...)
	var out []string
	for _, n := range names {
		// One slip in a short word, two in a longer one; or the word is part
		// of the name.
		slips := 2
		if len(word) <= 4 {
			slips = 1
		}
		if editDistance(word, n) <= slips || (len(word) >= 3 && strings.Contains(n, word)) {
			out = append(out, "omega "+n)
		}
	}
	return out
}

// interpret reads the words after `omega` that are not a command: a task,
// or a request in the user's own words. A sentence (three words or more, or
// one quoted argument with spaces) goes to the agent as it is; one or two
// words nobody recognises are an error that says what to try.
func interpret(root *cobra.Command, args []string) (task, prompt string, err error) {
	if len(args) == 0 {
		return agent.TaskFull, "", nil
	}
	if len(args) == 1 {
		if t, ok := taskFor(args[0]); ok {
			return t, "", nil
		}
	}
	if text := strings.Join(args, " "); len(strings.Fields(text)) >= 3 {
		return "", text, nil
	}
	msg := i18n.T("err.unknown_command", strings.Join(args, " "), version)
	if s := suggest(root, args[0]); len(s) > 0 {
		msg = i18n.T("err.did_you_mean", strings.Join(s, ", ")) + " " + msg
	}
	return "", "", fmt.Errorf("%s %s", msg, i18n.T("err.ask_words"))
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
	p, perr := newProvider(cfg, info, effort)
	// The catalogue only decides the automatic choice, but it also carries
	// the prices, so it is loaded even when the model is fixed.
	loadModels(cfg, info, p, false)
	plan, err := planModel(cfg, info, model, task)
	if err != nil {
		return nil, plan, err
	}
	return p, plan, perr
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
			return fmt.Errorf("%s", i18n.T("err.unknown_command", task, version))
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
	if f.sudo && goos == "windows" {
		fmt.Fprintln(os.Stderr, i18n.T("sudo.not_windows"))
	} else if f.sudo && !readOnly && stdinTTY {
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
		Models: func() string {
			info, _ := llm.Lookup(p.Name())
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			available, _ := p.Models(ctx)
			var b strings.Builder
			modelsReport(&b, cfg, info, available, "")
			return b.String()
		},
		Doctor: func() string {
			var b strings.Builder
			if err := runDoctor(&b); err != nil {
				b.WriteString(err.Error() + "\n")
			}
			return b.String()
		},
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

// withholdProviderKeys keeps every provider's API key variable away from the
// commands omega runs. omega reads them itself, from its own environment.
func withholdProviderKeys() {
	for _, name := range secrets.EnvVar {
		runner.Withhold(name)
	}
}
