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
	"github.com/lcajigasm/omega/internal/customsteps"
	"github.com/lcajigasm/omega/internal/demo"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/runner"
	"github.com/lcajigasm/omega/internal/secrets"
	"github.com/lcajigasm/omega/internal/steps"
	"github.com/lcajigasm/omega/internal/sudo"
	"github.com/lcajigasm/omega/internal/ui"
)

type runFlags struct {
	provider       string
	model          string
	effort         string
	yes            bool
	noTUI          bool
	dryRun         bool
	sudo           bool
	prompt         string
	once           bool
	stage          bool // a stage of the guided run: it ends when the model does
	demo           bool
	auto           bool
	noAI           bool
	offline        bool
	json           bool
	format         string
	quiet          bool
	verbose        bool
	debug          bool
	failOnWarning  bool
	failOnOutdated bool
	failOnCritical bool
	maxCost        float64
	maxTurns       int
}

// version is set by Execute for the TUI header.
var version string

// Init prepares the process for running sessions: the version shown to the
// user, the PATH, the withheld API key variables and the language. Execute
// calls it; the desktop app calls it on its own.
func Init(v string) {
	version = v
	runner.ExtendPath()
	withholdProviderKeys()
	if custom, err := customsteps.LoadCustomSteps(); err == nil {
		for _, s := range custom {
			steps.Register(s)
		}
	}
	// The language must be known before the commands' help text is built.
	if cfg, err := config.Load(); err == nil {
		i18n.Init(cfg.Language)
	} else {
		i18n.Init("")
	}
}

// Execute runs the root command.
func Execute(v string) error {
	Init(v)
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
		ValidArgs:     []string{agent.TaskFull, agent.TaskQuick, agent.TaskUpdate, agent.TaskClean, agent.TaskAnalyze, agent.TaskHealth, agent.TaskFix, agent.TaskTune, agent.TaskDev},
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

	root.AddCommand(
		fullCmd(), quickCmd(f), fixCmd(f), setupCmd(), authCmd(), modelCmd(), modelsCmd(),
		dockerCmd(), macCmd(), devCmd(), menuCmd(), configCmd(), stepsCmd(), notesCmd(),
		logsCmd(), doctorCmd(), netCmd(), auditCmd(), privacyCmd(), gitCmd(), diskCmd(),
		uninstallCmd(), scheduleCmd(), rollbackCmd(), exportCmd(), bootstrapCmd(), shellCmd(),
		completionCmd(root), manCmd(root),
	)
	return root
}

// taskWords are other words for the tasks, in the five interface languages:
// what someone types before reading the help.
var taskWords = map[string]string{
	"maintain": agent.TaskFull, "maintenance": agent.TaskFull, "run": agent.TaskFull,
	"completo": agent.TaskFull, "mantenimiento": agent.TaskFull, "tout": agent.TaskFull, "alles": agent.TaskFull, "tudo": agent.TaskFull,
	"easy": agent.TaskQuick, "rapido": agent.TaskQuick, "facil": agent.TaskQuick,
	"upgrade": agent.TaskUpdate, "updates": agent.TaskUpdate, "actualizar": agent.TaskUpdate, "actualiza": agent.TaskUpdate,
	"aktualisieren": agent.TaskUpdate, "atualizar": agent.TaskUpdate,
	"cleanup": agent.TaskClean, "clear": agent.TaskClean, "limpiar": agent.TaskClean, "limpia": agent.TaskClean, "limpieza": agent.TaskClean,
	"nettoyer": agent.TaskClean, "limpar": agent.TaskClean,
	"analyse": agent.TaskAnalyze, "analysis": agent.TaskAnalyze, "report": agent.TaskAnalyze,
	"analizar": agent.TaskAnalyze, "analiza": agent.TaskAnalyze, "informe": agent.TaskAnalyze,
	"analyser": agent.TaskAnalyze, "analysieren": agent.TaskAnalyze, "analisar": agent.TaskAnalyze,
	"repair": agent.TaskFix, "reparar": agent.TaskFix, "arreglar": agent.TaskFix, "reparieren": agent.TaskFix,
	"security": agent.TaskHealth, "checkup": agent.TaskHealth, "salud": agent.TaskHealth, "seguridad": agent.TaskHealth,
	"santé": agent.TaskHealth, "gesundheit": agent.TaskHealth, "saúde": agent.TaskHealth,
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
	"diagnose": {"doctor"}, "status": {"doctor"}, "check": {"doctor"}, "estado": {"doctor"}, "ranking": {"models"}, "modelos": {"models"},
	"memory": {"notes"}, "notas": {"notes"}, "pasos": {"steps"}, "wizard": {"setup"}, "configure": {"setup"}, "init": {"setup"},
	"cron": {"schedule"}, "daemon": {"schedule"}, "backup": {"export"},
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
var rootTasks = []string{agent.TaskFull, agent.TaskQuick, agent.TaskUpdate, agent.TaskClean, agent.TaskAnalyze, agent.TaskHealth, agent.TaskFix, agent.TaskTune}

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

	pf.BoolVar(&f.noAI, "no-ai", false, "run deterministically without calling an AI model")
	pf.BoolVar(&f.offline, "offline", false, "alias for --no-ai")
	pf.BoolVar(&f.json, "json", false, "output report as structured JSON")
	pf.StringVar(&f.format, "format", "", "output format; json is the only one written for now")
	pf.BoolVarP(&f.quiet, "quiet", "q", false, "quiet mode; suppress non-essential output")
	pf.BoolVarP(&f.verbose, "verbose", "v", false, "verbose output showing commands and streaming output")
	pf.BoolVar(&f.debug, "debug", false, "reserved; prints nothing more yet")
	pf.BoolVar(&f.failOnWarning, "fail-on-warning", false, "exit with non-zero code if any warning is found")
	pf.BoolVar(&f.failOnOutdated, "fail-on-outdated", false, "exit with non-zero code if outdated packages are detected")
	pf.BoolVar(&f.failOnCritical, "fail-on-critical", false, "exit with non-zero code if any step fails")
	pf.Float64Var(&f.maxCost, "max-cost", 0.0, "maximum estimated session cost in USD before stopping")
	pf.IntVar(&f.maxTurns, "max-turns", 0, "maximum number of agent turns")
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

// run is a task made ready to start: the provider, the model plan and the
// opening prompt. The terminal and the desktop app both start from one.
type run struct {
	cfg      *config.Config
	p        llm.Provider
	plan     modelPlan
	task     string
	prompt   string
	readOnly bool
	timeout  time.Duration
	f        runFlags
}

// prepareRun resolves everything a session needs before the agent exists.
func prepareRun(task string, f runFlags) (*run, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	prompt := f.prompt
	if prompt == "" {
		var ok bool
		prompt, ok = agent.TaskPrompts[task]
		if !ok {
			return nil, fmt.Errorf("%s", i18n.T("err.unknown_command", task, version))
		}
	} else {
		task = i18n.T("task.custom")
	}
	if f.auto {
		prompt += agent.AutoSuffix
	}
	if f.stage {
		prompt += agent.StageSuffix
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
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	timeout, parseErr := time.ParseDuration(cfg.Timeout)
	if parseErr != nil || timeout <= 0 {
		timeout = 45 * time.Minute
	}
	return &run{cfg: cfg, p: p, plan: plan, task: task, prompt: prompt, readOnly: readOnly, timeout: timeout, f: f}, nil
}

// agent builds the agent for a prepared run. interactive says whether
// someone can answer follow-ups; demoInteractive is the same for --demo.
func (r *run) agent(interactive, demoInteractive bool, sudoPassword string) *agent.Agent {
	cfg, p := r.cfg, r.p
	opts := agent.Options{
		Model:        r.plan.Model,
		Escalate:     r.plan.Escalate,
		Approval:     cfg.Approval,
		AutoYes:      r.f.yes,
		ReadOnly:     r.readOnly,
		Interactive:  interactive,
		Timeout:      r.timeout,
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
		MaxCost:  r.f.maxCost,
		MaxTurns: r.f.maxTurns,
		OneShot:  r.f.stage,
	}
	if r.f.demo {
		opts.Provider = "anthropic"
		opts.Exec = demo.Exec
		opts.SkipDetect = true
		opts.Interactive = demoInteractive
	}
	return agent.New(p, opts)
}

func runTask(task string, f runFlags) error {
	if f.noAI || f.offline || f.json || f.format == "json" {
		return runDeterministic(task, f)
	}

	_ = customsteps.RunHook(context.Background(), "pre_"+task)
	defer func() {
		_ = customsteps.RunHook(context.Background(), "post_"+task)
	}()

	r, err := prepareRun(task, f)
	if err != nil {
		if f.prompt == "" && (task == agent.TaskQuick || task == agent.TaskUpdate || task == agent.TaskClean || task == agent.TaskAnalyze || task == agent.TaskHealth) {
			return runDeterministic(task, f)
		}
		return err
	}

	stdinTTY := term.IsTerminal(int(os.Stdin.Fd()))
	stdoutTTY := term.IsTerminal(int(os.Stdout.Fd()))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	sudoPassword := ""
	if f.sudo && goos == "windows" {
		fmt.Fprintln(os.Stderr, i18n.T("sudo.not_windows"))
	} else if f.sudo && !r.readOnly && stdinTTY {
		if sudoPassword, err = askSudo(); err != nil {
			return err
		}
	}

	ag := r.agent(stdinTTY && !f.once, stdinTTY, sudoPassword)
	go ag.Run(ctx, r.prompt)

	if f.noTUI || !stdinTTY || !stdoutTTY {
		ui.RunPlain(ag, os.Stdin, stdinTTY)
	} else {
		err = ui.RunTUI(ctx, cancel, ag, ui.Header{Version: version, Provider: r.p.Name(), Model: r.plan.Model,
			Escalate: r.plan.Escalate, Models: r.plan.Choices, Task: r.task})
		cancel()
		// Drain so the agent goroutine can finish writing its log.
		for range ag.Events() {
		}
		if path := ag.LogPath(); path != "" && !ui.Hosted() {
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

// sudoAsked is the password a guided run asked for once, before its
// interface took the terminal; every stage then uses it.
var sudoAsked string

// askSudo asks for the sudo password before the interface starts and checks
// it, so sudo prompts during the run are answered without interrupting.
func askSudo() (string, error) {
	if sudoAsked != "" {
		return sudoAsked, nil
	}
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
