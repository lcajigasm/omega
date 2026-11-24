package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/customsteps"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/runner"
	"github.com/lcajigasm/omega/internal/secrets"
	"github.com/lcajigasm/omega/internal/steps"
	"github.com/lcajigasm/omega/internal/ui"
)

var stdin = bufio.NewReader(os.Stdin)

func ask(prompt, def string) string {
	if def != "" {
		fmt.Printf("%s [%s]: ", prompt, def)
	} else {
		fmt.Printf("%s: ", prompt)
	}
	s, _ := stdin.ReadString('\n')
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	return s
}

// showsAnswers matches a question that already ends with its answers, as
// the translated ones do: "Run it? y/N", "¿Ejecutar? s/N".
var showsAnswers = regexp.MustCompile(`\S/N\]?$`)

// yesNoPrompt is the line a yes-or-no question is asked with. The answers
// are shown once: a question that brings its own does not get them again.
func yesNoPrompt(question string) string {
	if !showsAnswers.MatchString(question) {
		question += " " + i18n.T("ask.yes_no")
	}
	return question + ": "
}

// askYesNo asks a question whose answer is no unless the user says yes, in
// any of omega's languages.
func askYesNo(question string) bool {
	fmt.Print(yesNoPrompt(question))
	s, _ := stdin.ReadString('\n')
	return i18n.IsYes(s)
}

// editorArgv is the command line that opens a file in the user's editor:
// $VISUAL, then $EDITOR, then notepad on Windows and nano elsewhere. The
// variable may hold arguments ("code -w") and quoted paths.
func editorArgv(path string) []string {
	editor := os.Getenv("VISUAL")
	if strings.TrimSpace(editor) == "" {
		editor = os.Getenv("EDITOR")
	}
	argv := splitFields(editor)
	if len(argv) == 0 {
		argv = []string{"nano"}
		if goos == "windows" {
			argv = []string{"notepad"}
		}
	}
	return append(argv, path)
}

// splitFields splits a command line at spaces, keeping what is inside
// single or double quotes together.
func splitFields(line string) []string {
	var out []string
	var cur strings.Builder
	quote, open := rune(0), false
	for _, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote, open = r, true
		case r == ' ' || r == '\t':
			if open {
				out = append(out, cur.String())
				cur.Reset()
				open = false
			}
		default:
			cur.WriteRune(r)
			open = true
		}
	}
	if open {
		out = append(out, cur.String())
	}
	return out
}

// editFile opens a file in the user's editor and waits for it to close.
func editFile(path string) error {
	argv := editorArgv(path)
	e := exec.Command(argv[0], argv[1:]...)
	e.Stdin, e.Stdout, e.Stderr = os.Stdin, os.Stdout, os.Stderr
	return e.Run()
}

// askSecret reads a secret without showing it. When stdin is not a terminal
// (a pipe or a file, as in a provisioning script) it reads one line from it.
func askSecret(prompt string) (string, error) {
	fmt.Print(prompt + ": ")
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		line, err := stdin.ReadString('\n')
		fmt.Println()
		if line = strings.TrimSpace(line); line != "" {
			return line, nil
		}
		return "", err
	}
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	return strings.TrimSpace(string(b)), err
}

func validProvider(name string) error {
	if _, ok := llm.Lookup(name); !ok {
		return fmt.Errorf("%s", i18n.T("err.unknown_provider", name, providerNames()))
	}
	return nil
}

// ---- setup ----

func setupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: i18n.T("cmd.setup"),
		Long:  i18n.T("cmd.setup_more"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := cfg.Validate(); err != nil {
				return err
			}
			// The wizard is questions and answers. Without a terminal it
			// used to print the list of providers and exit 0, having
			// configured nothing.
			tty := stdinIsTerminal()
			if !tty {
				return fmt.Errorf("%s", i18n.T("setup.need_tty"))
			}
			var opts []ui.Option
			for _, p := range llm.Providers {
				status := i18n.T("setup.no_key")
				switch {
				case p.Local:
					status = ""
					base := cfg.BaseURLFor(p.Name)
					if base == "" {
						base = p.BaseURL
					}
					if base != "" && probeLocal(base) {
						status = i18n.T("setup.local_found")
					}
				default:
					if _, src, err := secrets.Get(p.Name); err == nil {
						status = i18n.T("setup.key_in", secrets.Describe(src))
					}
				}
				opts = append(opts, ui.Option{ID: p.Name, Label: p.Label, Detail: p.KeyURL, Badge: status})
			}
			// The picker draws a screen only where one can be drawn; elsewhere it
			// asks by number.
			ids, err := ui.Pick(i18n.T("setup.providers"), "", opts, false, ui.WantTUI(false), os.Stdin, os.Stdout)
			if errors.Is(err, ui.ErrCancelled) {
				return nil
			}
			if err != nil {
				return err
			}
			name := ids[0]
			info, _ := llm.Lookup(name)

			if !info.Local {
				key, src, keyErr := secrets.Get(name)
				if keyErr != nil || askYesNo(i18n.T("setup.replace_key", secrets.Mask(key), secrets.Describe(src))) {
					fmt.Println(i18n.T("setup.get_key", info.KeyURL))
					key, err = askSecret(i18n.T("setup.enter_key"))
					key = cleanKey(key)
					if err != nil || key == "" {
						return fmt.Errorf("%s", i18n.T("setup.no_key_entered"))
					}
					if err := checkKey(cfg, name, key); err != nil {
						return err
					}
					where, err := secrets.Set(name, key)
					if err != nil {
						return err
					}
					fmt.Println(i18n.T("setup.key_saved", secrets.Describe(where)))
					if where == secrets.FromFile {
						fmt.Println(i18n.T("secrets.no_keyring"))
					}
				}
			} else if name == "custom" && cfg.BaseURLFor(name) == "" {
				base := ask("base_url (OpenAI-compatible, e.g. http://localhost:8080/v1)", "")
				if base == "" {
					return fmt.Errorf("custom provider needs a base_url")
				}
				pc := cfg.Providers[name]
				pc.BaseURL = base
				cfg.Providers[name] = pc
			}

			cfg.Provider = name
			cfg.Model = cfg.Providers[name].Model
			if err := cfg.Save(); err != nil {
				return err
			}
			p, err := newProvider(cfg, info, "")
			if err != nil {
				return err
			}
			loadModels(cfg, info, p, false)
			if err := chooseModel(cfg, info, p); err != nil && !errors.Is(err, ui.ErrCancelled) {
				return err
			}
			plan, _ := planModel(cfg, info, "", "full")
			fmt.Print(i18n.T("setup.done", name, plan.Model, config.Path()))
			return nil
		},
	}
}

// ---- auth ----

func authCmd() *cobra.Command {
	c := &cobra.Command{Use: "auth", Short: i18n.T("cmd.auth"), Long: i18n.T("cmd.auth_more"),
		Example: "  omega auth status\n  omega auth set anthropic\n  omega auth status --json"}
	c.AddCommand(
		&cobra.Command{
			Use: "set <provider>", Short: i18n.T("cmd.auth_set"), Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := validProvider(args[0]); err != nil {
					return err
				}
				key, err := askSecret(i18n.T("auth.prompt", args[0]))
				key = cleanKey(key)
				if err != nil || key == "" {
					return fmt.Errorf("%s", i18n.T("setup.no_key_entered"))
				}
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				if err := checkKey(cfg, args[0], key); err != nil {
					return err
				}
				where, err := secrets.Set(args[0], key)
				if err != nil {
					return err
				}
				fmt.Println(i18n.T("auth.saved", secrets.Describe(where)))
				if where == secrets.FromFile {
					fmt.Println(i18n.T("secrets.no_keyring"))
				}
				return nil
			},
		},
		&cobra.Command{
			Use: "remove <provider>", Short: i18n.T("cmd.auth_remove"), Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := validProvider(args[0]); err != nil {
					return err
				}
				return secrets.Delete(args[0])
			},
		},
		authStatusCmd(),
	)
	return c
}

// keyStatus is one provider's key as `omega auth status --json` gives it.
// The key itself is never in it, only its masked form.
type keyStatus struct {
	Provider   string `json:"provider"`
	Set        bool   `json:"set"`
	Source     string `json:"source,omitempty"`
	Masked     string `json:"masked,omitempty"`
	LooksWrong bool   `json:"looks_wrong,omitempty"`
}

func authStatusCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use: "status", Short: i18n.T("cmd.auth_status"), Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var list []keyStatus
			for _, p := range llm.Providers {
				key, src, err := secrets.Get(p.Name)
				if err != nil {
					list = append(list, keyStatus{Provider: p.Name})
					continue
				}
				list = append(list, keyStatus{Provider: p.Name, Set: true, Source: secrets.Describe(src),
					Masked: secrets.Mask(key), LooksWrong: keyLooksWrong(key)})
			}
			w := cmd.OutOrStdout()
			if asJSON {
				return writeJSON(w, list)
			}
			for _, k := range list {
				if !k.Set {
					fmt.Fprintf(w, "  %-12s —\n", k.Provider)
					continue
				}
				warn := ""
				if k.LooksWrong {
					warn = "  ⚠ " + i18n.T("auth.looks_wrong", k.Provider)
				}
				fmt.Fprintf(w, "  %-12s %s (%s)%s\n", k.Provider, k.Masked, k.Source, warn)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, i18n.T("flag.json"))
	return c
}

// ---- models ----

// modelsCmd is `omega models`, the old name of `omega model list`: it
// still works and is no longer listed.
func modelsCmd() *cobra.Command {
	c := modelListCmd()
	c.Use = "models [provider] [filter]"
	c.Hidden = true
	return c
}

// modelListCmd is `omega model list`: the models of a provider with their
// score and price.
func modelListCmd() *cobra.Command {
	var refresh, asJSON bool
	c := &cobra.Command{
		Use: "list [provider] [filter]", Short: i18n.T("cmd.models"), Long: i18n.T("cmd.models_long"), Args: cobra.MaximumNArgs(2),
		Example:     "  omega model list\n  omega model list openrouter claude\n  omega model list --json",
		Annotations: map[string]string{"see_also": "omega model, omega config set model, omega setup"},
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			var names []string
			for _, p := range llm.Providers {
				names = append(names, p.Name)
			}
			return names, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			name := cfg.Provider
			if len(args) > 0 {
				name = args[0]
			}
			info, ok := llm.Lookup(name)
			if !ok {
				return fmt.Errorf("%s", i18n.T("err.unknown_provider", name, providerNames()))
			}
			p, err := newProvider(cfg, info, "")
			if err != nil {
				return err
			}
			if err := loadModels(cfg, info, p, refresh); err != nil {
				fmt.Fprintln(os.Stderr, i18n.T("models.fetch_failed", err))
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			available, err := p.Models(ctx)
			if err != nil {
				return err
			}
			filter := ""
			if len(args) == 2 {
				filter = args[1]
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), modelsList(cfg, info, available, filter))
			}
			modelsReport(cmd.OutOrStdout(), cfg, info, available, filter)
			return nil
		},
	}
	c.Flags().BoolVar(&refresh, "refresh", false, i18n.T("flag.refresh"))
	c.Flags().BoolVar(&asJSON, "json", false, i18n.T("flag.json"))
	return c
}

// ---- config ----

func configCmd() *cobra.Command {
	c := &cobra.Command{Use: "config", Short: i18n.T("cmd.config"),
		Example: "  omega config show\n  omega config set approval all\n  omega config check\n  omega config edit"}
	c.AddCommand(
		&cobra.Command{
			Use: "show", Short: i18n.T("cmd.config_show"), Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
					return configJSON(cmd.OutOrStdout(), cfg)
				}
				fmt.Fprintln(cmd.OutOrStdout(), "#", config.Path())
				return tomlEncode(cmd.OutOrStdout(), cfg)
			},
		},
		&cobra.Command{
			Use: "path", Short: i18n.T("cmd.config_path"), Args: cobra.NoArgs,
			Run: func(cmd *cobra.Command, args []string) { fmt.Fprintln(cmd.OutOrStdout(), config.Path()) },
		},
		&cobra.Command{
			Use:   "set <key> <value>",
			Short: i18n.T("cmd.config_set"),
			Long:  i18n.T("cmd.config_set_more"),
			Args:  cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				return SetConfig(args[0], args[1])
			},
		},
		&cobra.Command{
			Use: "check", Short: i18n.T("cmd.config_check"), Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				if err := cfg.Validate(); err != nil {
					return err
				}
				// The user's own steps are configuration too.
				if _, problems := customsteps.Load(); len(problems) > 0 {
					return fmt.Errorf("%s", strings.Join(problems, "\n"))
				}
				fmt.Fprintln(cmd.OutOrStdout(), i18n.T("config.ok", config.Path()))
				// A key omega does not know is a typo more often than not, and
				// its setting silently stays the default.
				for _, k := range unknownConfigKeys() {
					fmt.Fprintln(cmd.OutOrStdout(), "! "+i18n.T("config.unknown_key", k))
				}
				return nil
			},
		},
		&cobra.Command{
			Use: "edit", Short: i18n.T("cmd.config_edit"), Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				if _, err := os.Stat(config.Path()); err != nil {
					if err := cfg.Save(); err != nil {
						return err
					}
				}
				return editFile(config.Path())
			},
		},
	)
	if show, _, err := c.Find([]string{"show"}); err == nil {
		show.Flags().Bool("json", false, i18n.T("flag.json"))
	}
	return c
}

// ---- steps ----

func stepsCmd() *cobra.Command {
	var asJSON bool
	var sf stepFilter
	c := &cobra.Command{
		Use: "steps", Short: i18n.T("cmd.steps"), Args: cobra.NoArgs,
		Example: "  omega steps\n  omega steps --installed --category clean\n  omega steps show brew\n" +
			"  omega steps run brew-cleanup --dry-run",
		RunE: func(cmd *cobra.Command, args []string) error {
			if sf.category != "" && !slices.Contains(steps.Categories, sf.category) {
				return usageErr(cmd, i18n.T("steps.bad_category", sf.category, strings.Join(steps.Categories, ", ")))
			}
			if asJSON {
				list, err := StepList()
				if err != nil {
					return err
				}
				keep := []StepInfo{}
				for _, s := range list {
					if sf.keep(s.CategoryID, s.Installed) {
						keep = append(keep, s)
					}
				}
				return writeJSON(cmd.OutOrStdout(), keep)
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			writeSteps(cmd.OutOrStdout(), cfg, sf)
			return nil
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, i18n.T("flag.steps_json"))
	c.Flags().BoolVar(&sf.installed, "installed", false, i18n.T("flag.steps_installed"))
	c.Flags().StringVar(&sf.category, "category", "", i18n.T("flag.steps_category"))
	_ = c.RegisterFlagCompletionFunc("category", func(*cobra.Command, []string, string) ([]cobra.Completion, cobra.ShellCompDirective) {
		return steps.Categories, cobra.ShellCompDirectiveNoFileComp
	})

	var r stepsRun
	run := &cobra.Command{
		Use: "run <id>...", Short: i18n.T("cmd.steps_run"), Args: cobra.MinimumNArgs(1),
		Example:           "  omega steps run brew-cleanup\n  omega steps run trash npm-cache --dry-run",
		ValidArgsFunction: completeSteps,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := cfg.Validate(); err != nil {
				return err
			}
			var su sudoSession
			defer su.close()
			r.cfg, r.out, r.find = cfg, cmd.OutOrStdout(), steps.Find
			r.tty = stdinIsTerminal()
			r.confirm = askYesNo
			r.sudo = su.need
			r.exec = runner.Run
			return r.run(args)
		},
	}
	run.Flags().BoolVarP(&r.yes, "yes", "y", false, i18n.T("flag.steps_yes"))
	run.Flags().BoolVarP(&r.dryRun, "dry-run", "n", false, i18n.T("flag.steps_dry"))
	c.AddCommand(run, stepsShowCmd())
	return c
}

// stepFilter narrows `omega steps`.
type stepFilter struct {
	installed bool
	category  string
}

func (f stepFilter) keep(category string, installed bool) bool {
	return (!f.installed || installed) && (f.category == "" || f.category == category)
}

// writeSteps lists the catalogue by category: whether each step is here,
// what it does to the machine and what it is.
func writeSteps(w io.Writer, cfg *config.Config, sf stepFilter) {
	r := ui.NewReport(w)
	installed, total := 0, 0
	for _, s := range steps.All {
		total++
		if s.Available() {
			installed++
		}
	}
	r.Title("omega steps", i18n.T("steps.count", installed, total))
	byCat := map[string][]steps.Step{}
	var cats []string
	for _, s := range steps.Grouped(steps.All) {
		if !sf.keep(s.Category, s.Available()) {
			continue
		}
		if _, ok := byCat[s.Category]; !ok {
			cats = append(cats, s.Category)
		}
		byCat[s.Category] = append(byCat[s.Category], s)
	}
	idW, nameW := 0, 0
	for _, s := range steps.All {
		idW = max(idW, len(s.ID))
		nameW = max(nameW, lipgloss.Width(i18n.TOr("step."+s.ID, s.Name)))
	}
	for _, cat := range cats {
		list := byCat[cat]
		r.Section(i18n.T("cat."+cat), len(list))
		var rows [][]string
		var marks []ui.Mark
		for _, s := range list {
			mark := ui.MarkOK
			name := i18n.TOr("step."+s.ID, s.Name)
			risk := r.StepRisk(s.Risk.String())
			id := s.ID
			switch {
			case cfg.Skipped(s.ID):
				mark = ui.MarkSkip
				id, name = r.Faint(id), r.Faint(name+" · "+i18n.T("doctor.skipped"))
			case !s.Available():
				mark = ui.MarkOff
				id, name, risk = r.Faint(id), r.Faint(name), ""
			}
			rows = append(rows, []string{id, name, risk})
			marks = append(marks, mark)
		}
		r.Table([]ui.Column{{Min: idW}, {Min: nameW}, {}}, rows, marks)
	}
	r.Blank()
	r.Line(r.Muted(i18n.T("steps.legend2", r.Glyph(ui.MarkOK), r.Glyph(ui.MarkSkip), r.Glyph(ui.MarkOff))))
	r.Hint(i18n.T("steps.hint"))
}

// stepsRun is one `omega steps run`: the flags, who answers and how a
// command is started.
type stepsRun struct {
	yes, dryRun bool
	cfg         *config.Config
	out         io.Writer
	find        func(id string) (steps.Step, bool)
	tty         bool
	confirm     func(question string) bool
	sudo        func(command string) error
	exec        func(ctx context.Context, command string, timeout time.Duration, onLine func(string)) runner.Result
}

// run runs the named steps in order. A step that needs a yes follows the
// rule of every other run (needsAnswer): --yes answers for a dangerous step
// but not for one that always asks, and with nobody at a terminal the step
// is skipped, said, and the command fails. A script must not read a step
// that did not run as done.
func (r stepsRun) run(ids []string) error {
	timeout, err := time.ParseDuration(r.cfg.Timeout)
	if err != nil {
		return err
	}
	rep := ui.NewReport(r.out)
	failed, skipped := 0, 0
	for _, id := range ids {
		s, ok := r.find(id)
		if !ok {
			return usageErr(nil, i18n.T("steps.unknown", id))
		}
		name := i18n.TOr("step."+s.ID, s.Name)
		if !s.Available() {
			fmt.Fprintln(r.out, rep.Glyph(ui.MarkOff)+" "+i18n.T("steps.not_installed", id))
			continue
		}
		if r.dryRun {
			fmt.Fprintln(r.out, rep.Glyph(ui.MarkInfo)+" "+i18n.T("steps.would_run", id, s.Command))
			continue
		}
		if needsAnswer(r.cfg, s, r.yes) {
			if !r.tty {
				skipped++
				fmt.Fprintln(r.out, rep.Glyph(ui.MarkWarn)+" "+i18n.T("steps.skipped", id))
				continue
			}
			if !r.confirm(i18n.T("steps.confirm", name, s.Command)) {
				continue
			}
		}
		fmt.Fprintln(r.out)
		fmt.Fprintln(r.out, rep.Accent("▸ ")+rep.Bold(s.ID)+"  "+rep.Muted(name))
		fmt.Fprintln(r.out, "  "+rep.Faint("$ "+rep.Clip(s.Command, 4)))
		if err := r.sudo(s.Command); err != nil {
			return err
		}
		res := r.exec(context.Background(), s.Command, timeout, func(l string) { fmt.Fprintln(r.out, "  "+rep.Faint("│")+" "+l) })
		if res.ExitCode != 0 {
			failed++
			fmt.Fprintln(r.out, "  "+rep.Glyph(ui.MarkErr)+" "+rep.Err(i18n.T("steps.failed", id, res.ExitCode)))
		} else {
			fmt.Fprintln(r.out, "  "+rep.Glyph(ui.MarkOK)+" "+id+rep.Faint(" · "+ui.Duration(res.Duration)))
		}
	}
	if failed > 0 {
		return fmt.Errorf("%s", i18n.N("steps.failed_summary", failed))
	}
	if skipped > 0 {
		return fmt.Errorf("%s", i18n.N("steps.skipped_summary", skipped))
	}
	return nil
}

// ---- notes ----

func notesCmd() *cobra.Command {
	var last int
	var grep string
	var asJSON bool
	c := &cobra.Command{
		Use: "notes", Short: i18n.T("cmd.notes"), Args: cobra.NoArgs,
		Example:     "  omega notes\n  omega notes --last 50\n  omega notes --grep docker\n  omega notes compact",
		Annotations: map[string]string{"see_also": "omega history, omega logs, omega pending"},
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := os.ReadFile(config.NotesPath())
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), pickNotes(parseNotes(string(b)), last, grep))
			}
			if len(b) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), i18n.T("notes.empty"))
				return nil
			}
			writeNotes(cmd.OutOrStdout(), string(b), last, grep)
			return nil
		},
	}
	c.Flags().IntVarP(&last, "last", "n", 15, i18n.T("flag.notes_last"))
	c.Flags().StringVar(&grep, "grep", "", i18n.T("flag.notes_grep"))
	c.Flags().BoolVar(&asJSON, "json", false, i18n.T("flag.json"))
	c.AddCommand(notesCompactCmd())
	c.AddCommand(&cobra.Command{
		Use: "clear", Short: i18n.T("cmd.notes_clear"), Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if stdinIsTerminal() && !askYesNo(i18n.T("notes.clear_ask", config.NotesPath())) {
				return nil
			}
			err := os.Remove(config.NotesPath())
			if os.IsNotExist(err) {
				return nil
			}
			if err == nil {
				fmt.Fprintln(cmd.OutOrStdout(), i18n.T("notes.cleared"))
			}
			return err
		},
	})
	return c
}

// noteLine is "- 2026-10-27: text", with the date the model sometimes
// wrote again in front of the text.
var noteLine = regexp.MustCompile(`^-\s*(\d{4}-\d{2}-\d{2}):\s*(?:\d{4}-\d{2}-\d{2}(?:\s*\(\d+\))?\s*[:—–-]?\s*)?(.*)$`)

// note is one line of notes.md: the day it was written and its text.
type note struct {
	Day  string `json:"day,omitempty"`
	Text string `json:"text"`
}

// parseNotes reads the notes, oldest first. Never nil.
func parseNotes(text string) []note {
	notes := []note{}
	for _, l := range strings.Split(strings.TrimSpace(text), "\n") {
		if m := noteLine.FindStringSubmatch(l); m != nil {
			notes = append(notes, note{m[1], m[2]})
		} else if t := strings.TrimSpace(strings.TrimPrefix(l, "-")); t != "" {
			notes = append(notes, note{"", t})
		}
	}
	return notes
}

// pickNotes keeps the notes that contain grep, and of those the last ones.
func pickNotes(notes []note, last int, grep string) []note {
	if grep != "" {
		keep := []note{}
		for _, n := range notes {
			if strings.Contains(strings.ToLower(n.Text), strings.ToLower(grep)) {
				keep = append(keep, n)
			}
		}
		notes = keep
	}
	if last > 0 && len(notes) > last {
		notes = notes[len(notes)-last:]
	}
	return notes
}

// writeNotes shows the newest notes first, grouped by day and wrapped.
func writeNotes(w io.Writer, text string, last int, grep string) {
	total := len(pickNotes(parseNotes(text), 0, grep))
	notes := pickNotes(parseNotes(text), last, grep)
	r := ui.NewReport(w)
	r.Title("omega notes", i18n.T("notes.count", len(notes), total))
	day := "\x00"
	for i := len(notes) - 1; i >= 0; i-- {
		n := notes[i]
		if n.Day != day {
			day = n.Day
			r.Section(n.Day, -1)
		}
		lines := strings.Split(lipgloss.NewStyle().Width(r.Width()-6).Render(n.Text), "\n")
		for j, l := range lines {
			if j == 0 {
				fmt.Fprintln(w, "  "+r.Faint("•")+" "+strings.TrimRight(l, " "))
			} else {
				fmt.Fprintln(w, "    "+strings.TrimRight(l, " "))
			}
		}
	}
	r.Blank()
	if total > len(notes) {
		r.Line(r.Muted(i18n.T("notes.more", total-len(notes))))
	}
	r.Hint(i18n.T("notes.hint", tildePath(config.NotesPath())))
}
