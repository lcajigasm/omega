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
	"strings"
	"time"

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
			ids, err := ui.Pick(i18n.T("setup.providers"), "", opts, false, tty, os.Stdin, os.Stdout)
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
	c := &cobra.Command{Use: "auth", Short: i18n.T("cmd.auth")}
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
		&cobra.Command{
			Use: "status", Short: i18n.T("cmd.auth_status"), Args: cobra.NoArgs,
			Run: func(cmd *cobra.Command, args []string) {
				for _, p := range llm.Providers {
					key, src, err := secrets.Get(p.Name)
					if err != nil {
						fmt.Printf("  %-12s —\n", p.Name)
						continue
					}
					warn := ""
					if keyLooksWrong(key) {
						warn = "  ⚠ " + i18n.T("auth.looks_wrong", p.Name)
					}
					fmt.Printf("  %-12s %s (%s)%s\n", p.Name, secrets.Mask(key), secrets.Describe(src), warn)
				}
			},
		},
	)
	return c
}

// ---- models ----

func modelsCmd() *cobra.Command {
	var refresh bool
	c := &cobra.Command{
		Use: "models [provider] [filter]", Short: i18n.T("cmd.models"), Long: i18n.T("cmd.models_long"), Args: cobra.MaximumNArgs(2),
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
			modelsReport(os.Stdout, cfg, info, available, filter)
			return nil
		},
	}
	c.Flags().BoolVar(&refresh, "refresh", false, i18n.T("flag.refresh"))
	return c
}

// ---- config ----

func configCmd() *cobra.Command {
	c := &cobra.Command{Use: "config", Short: i18n.T("cmd.config")}
	c.AddCommand(
		&cobra.Command{
			Use: "show", Short: i18n.T("cmd.config_show"), Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				fmt.Println("#", config.Path())
				return tomlEncode(cfg)
			},
		},
		&cobra.Command{
			Use: "path", Short: i18n.T("cmd.config_path"), Args: cobra.NoArgs,
			Run: func(cmd *cobra.Command, args []string) { fmt.Println(config.Path()) },
		},
		&cobra.Command{
			Use:   "set <key> <value>",
			Short: i18n.T("cmd.config_set"),
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
				fmt.Println(i18n.T("config.ok", config.Path()))
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
	return c
}

// ---- steps ----

func stepsCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use: "steps", Short: i18n.T("cmd.steps"), Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if asJSON {
				list, err := StepList()
				if err != nil {
					return err
				}
				return writeJSON(cmd.OutOrStdout(), list)
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			cat := ""
			for _, s := range steps.Grouped(steps.All) {
				if s.Category != cat {
					cat = s.Category
					fmt.Printf("\n%s\n", i18n.T("cat."+cat))
				}
				mark := "✓"
				switch {
				case cfg.Skipped(s.ID):
					mark = "–"
				case !s.Available():
					mark = " "
				}
				fmt.Printf("  %s %-22s %-10s %s\n", mark, s.ID, ui.RiskName(s.Risk), i18n.TOr("step."+s.ID, s.Name))
			}
			fmt.Println(i18n.T("steps.legend"))
			return nil
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, i18n.T("flag.steps_json"))

	var r stepsRun
	run := &cobra.Command{
		Use: "run <id>...", Short: i18n.T("cmd.steps_run"), Args: cobra.MinimumNArgs(1),
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
			r.cfg, r.out, r.find = cfg, os.Stdout, steps.Find
			r.tty = stdinIsTerminal()
			r.confirm = askYesNo
			r.sudo = su.need
			r.exec = runner.Run
			return r.run(args)
		},
	}
	run.Flags().BoolVarP(&r.yes, "yes", "y", false, i18n.T("flag.steps_yes"))
	run.Flags().BoolVarP(&r.dryRun, "dry-run", "n", false, i18n.T("flag.steps_dry"))
	c.AddCommand(run)
	return c
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
	failed, skipped := 0, 0
	for _, id := range ids {
		s, ok := r.find(id)
		if !ok {
			return fmt.Errorf("%s", i18n.T("steps.unknown", id))
		}
		if !s.Available() {
			fmt.Fprintln(r.out, i18n.T("steps.not_installed", id))
			continue
		}
		if r.dryRun {
			fmt.Fprintln(r.out, i18n.T("steps.would_run", id, s.Command))
			continue
		}
		if needsAnswer(r.cfg, s, r.yes) {
			if !r.tty {
				skipped++
				fmt.Fprintln(r.out, i18n.T("steps.skipped", id))
				continue
			}
			if !r.confirm(i18n.T("steps.confirm", i18n.TOr("step."+s.ID, s.Name), s.Command)) {
				continue
			}
		}
		fmt.Fprintf(r.out, "\n$ %s\n", s.Command)
		if err := r.sudo(s.Command); err != nil {
			return err
		}
		res := r.exec(context.Background(), s.Command, timeout, func(l string) { fmt.Fprintln(r.out, "  "+l) })
		if res.ExitCode != 0 {
			failed++
			fmt.Fprintln(r.out, i18n.T("steps.failed", id, res.ExitCode))
		} else {
			fmt.Fprintf(r.out, "✓ %s (%s)\n", id, res.Duration.Round(time.Second))
		}
	}
	if failed > 0 {
		return fmt.Errorf("%s", i18n.T("steps.failed_summary", failed))
	}
	if skipped > 0 {
		return fmt.Errorf("%s", i18n.T("steps.skipped_summary", skipped))
	}
	return nil
}

// ---- notes ----

func notesCmd() *cobra.Command {
	c := &cobra.Command{
		Use: "notes", Short: i18n.T("cmd.notes"), Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			b, err := os.ReadFile(config.NotesPath())
			if err != nil || len(b) == 0 {
				fmt.Println(i18n.T("notes.empty"))
				return
			}
			fmt.Print(string(b))
		},
	}
	c.AddCommand(&cobra.Command{
		Use: "clear", Short: i18n.T("cmd.notes_clear"), Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			err := os.Remove(config.NotesPath())
			if os.IsNotExist(err) {
				return nil
			}
			return err
		},
	})
	return c
}
