package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/runner"
	"github.com/lcajigasm/omega/internal/safety"
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

func askSecret(prompt string) (string, error) {
	fmt.Print(prompt + ": ")
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
			fmt.Println(i18n.T("setup.providers"))
			for i, p := range llm.Providers {
				status := i18n.T("setup.no_key")
				if _, src, err := secrets.Get(p.Name); err == nil {
					status = i18n.T("setup.key_in", src)
				}
				fmt.Printf("  %d) %-12s %s (%s)\n", i+1, p.Name, p.Label, status)
			}
			choice := ask(i18n.T("setup.choose_provider"), cfg.Provider)
			name := choice
			if n, err := strconv.Atoi(choice); err == nil && n >= 1 && n <= len(llm.Providers) {
				name = llm.Providers[n-1].Name
			}
			if err := validProvider(name); err != nil {
				return err
			}
			info, _ := llm.Lookup(name)

			key, src, keyErr := secrets.Get(name)
			if keyErr != nil || i18n.IsYes(ask(i18n.T("setup.replace_key", secrets.Mask(key), src), "n")) {
				fmt.Println(i18n.T("setup.get_key", info.KeyURL))
				key, err = askSecret(i18n.T("setup.enter_key"))
				if err != nil || key == "" {
					return fmt.Errorf("%s", i18n.T("setup.no_key_entered"))
				}
				if err := secrets.Set(name, key); err != nil {
					return err
				}
				fmt.Println(i18n.T("setup.key_saved"))
			}

			p, err := llm.New(name, llm.Options{APIKey: key, BaseURL: cfg.BaseURLFor(name)})
			if err != nil {
				return err
			}
			def := cfg.ModelFor(name)
			if def == "" {
				def = info.DefaultModel
			}
			model := def
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			models, err := p.Models(ctx)
			cancel()
			if err != nil {
				fmt.Println(i18n.T("setup.models_failed", err))
			} else {
				model = pickModel(models, def)
			}

			cfg.Provider = name
			cfg.Model = model
			pc := cfg.Providers[name]
			pc.Model = model
			cfg.Providers[name] = pc
			if err := cfg.Save(); err != nil {
				return err
			}
			fmt.Print(i18n.T("setup.done", name, model, config.Path()))
			return nil
		},
	}
}

func pickModel(models []string, def string) string {
	fmt.Print(i18n.T("setup.models_count", len(models), def))
	list := models
	for {
		show := list
		if len(show) > 30 {
			show = show[:30]
		}
		for i, m := range show {
			fmt.Printf("  %2d) %s\n", i+1, m)
		}
		if len(list) > len(show) {
			fmt.Print(i18n.T("setup.more", len(list)-len(show)))
		}
		in := ask(i18n.T("setup.model"), def)
		if in == def {
			return def
		}
		if n, err := strconv.Atoi(in); err == nil && n >= 1 && n <= len(show) {
			return show[n-1]
		}
		var filtered []string
		for _, m := range models {
			if m == in {
				return m
			}
			if strings.Contains(strings.ToLower(m), strings.ToLower(in)) {
				filtered = append(filtered, m)
			}
		}
		if len(filtered) == 0 {
			fmt.Println(i18n.T("setup.no_match", in))
			return in
		}
		list = filtered
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
				if err != nil || key == "" {
					return fmt.Errorf("%s", i18n.T("setup.no_key_entered"))
				}
				if err := secrets.Set(args[0], key); err != nil {
					return err
				}
				fmt.Println(i18n.T("auth.saved"))
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
					fmt.Printf("  %-12s %s (%s)\n", p.Name, secrets.Mask(key), src)
				}
			},
		},
	)
	return c
}

// ---- models ----

func modelsCmd() *cobra.Command {
	return &cobra.Command{
		Use: "models [provider] [filter]", Short: i18n.T("cmd.models"), Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			p, current, err := resolveProvider(cfg, name, "", "")
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			models, err := p.Models(ctx)
			if err != nil {
				return err
			}
			for _, m := range models {
				if len(args) == 2 && !strings.Contains(strings.ToLower(m), strings.ToLower(args[1])) {
					continue
				}
				mark := "  "
				if m == current {
					mark = "* "
				}
				fmt.Println(mark + m)
			}
			return nil
		},
	}
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
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				k, v := args[0], args[1]
				switch k {
				case "provider":
					if err := validProvider(v); err != nil {
						return err
					}
					cfg.Provider = v
					cfg.Model = cfg.Providers[v].Model
				case "model":
					cfg.Model = v
					pc := cfg.Providers[cfg.Provider]
					pc.Model = v
					cfg.Providers[cfg.Provider] = pc
				case "effort":
					cfg.Effort = v
				case "approval":
					if v != config.ApproveDangerous && v != config.ApproveAll && v != config.ApproveNever {
						return fmt.Errorf("%s", i18n.T("config.bad_approval"))
					}
					cfg.Approval = v
				case "language":
					if v != "auto" && i18n.Normalize(v) == "" {
						return fmt.Errorf("%s", i18n.T("config.bad_language", v))
					}
					cfg.Language = v
				case "command_timeout":
					if _, err := time.ParseDuration(v); err != nil {
						return fmt.Errorf("%s", i18n.T("config.bad_duration", err))
					}
					cfg.Timeout = v
				case "skip_steps":
					cfg.SkipSteps = nil
					for _, s := range strings.Split(v, ",") {
						if s = strings.TrimSpace(s); s != "" {
							cfg.SkipSteps = append(cfg.SkipSteps, s)
						}
					}
				default:
					return fmt.Errorf("%s", i18n.T("config.unknown_key", k))
				}
				return cfg.Save()
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
				editor := os.Getenv("EDITOR")
				if editor == "" {
					editor = "nano"
				}
				e := exec.Command(editor, config.Path())
				e.Stdin, e.Stdout, e.Stderr = os.Stdin, os.Stdout, os.Stderr
				return e.Run()
			},
		},
	)
	return c
}

// ---- steps ----

func stepsCmd() *cobra.Command {
	c := &cobra.Command{
		Use: "steps", Short: i18n.T("cmd.steps"), Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			cat := ""
			for _, s := range steps.All {
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
	var yes bool
	run := &cobra.Command{
		Use: "run <id>...", Short: i18n.T("cmd.steps_run"), Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			timeout, _ := time.ParseDuration(cfg.Timeout)
			failed := 0
			for _, id := range args {
				s, ok := steps.Find(id)
				if !ok {
					return fmt.Errorf("%s", i18n.T("steps.unknown", id))
				}
				if !s.Available() {
					fmt.Println(i18n.T("steps.not_installed", id))
					continue
				}
				if s.Risk >= safety.Dangerous && !yes &&
					!i18n.IsYes(ask(i18n.T("steps.confirm", i18n.TOr("step."+s.ID, s.Name), s.Command), "n")) {
					continue
				}
				fmt.Printf("\n$ %s\n", s.Command)
				res := runner.Run(context.Background(), s.Command, timeout, func(l string) { fmt.Println("  " + l) })
				if res.ExitCode != 0 {
					failed++
					fmt.Println(i18n.T("steps.failed", id, res.ExitCode))
				} else {
					fmt.Printf("✓ %s (%s)\n", id, res.Duration.Round(time.Second))
				}
			}
			if failed > 0 {
				return fmt.Errorf("%s", i18n.T("steps.failed_summary", failed))
			}
			return nil
		},
	}
	run.Flags().BoolVarP(&yes, "yes", "y", false, i18n.T("flag.steps_yes"))
	c.AddCommand(run)
	return c
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
