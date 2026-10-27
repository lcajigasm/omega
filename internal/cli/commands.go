package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
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
			tty := term.IsTerminal(int(os.Stdin.Fd()))
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
						status = i18n.T("setup.key_in", src)
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
				if keyErr != nil || i18n.IsYes(ask(i18n.T("setup.replace_key", secrets.Mask(key), src), "n")) {
					fmt.Println(i18n.T("setup.get_key", info.KeyURL))
					key, err = askSecret(i18n.T("setup.enter_key"))
					key = cleanKey(key)
					if err != nil || key == "" {
						return fmt.Errorf("%s", i18n.T("setup.no_key_entered"))
					}
					if err := checkKey(cfg, name, key); err != nil {
						return err
					}
					if err := secrets.Set(name, key); err != nil {
						return err
					}
					fmt.Println(i18n.T("setup.key_saved"))
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
					warn := ""
					if keyLooksWrong(key) {
						warn = "  ⚠ " + i18n.T("auth.looks_wrong", p.Name)
					}
					fmt.Printf("  %-12s %s (%s)%s\n", p.Name, secrets.Mask(key), src, warn)
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
			p, plan, err := resolveProvider(cfg, name, "", "", "full")
			if err != nil {
				return err
			}
			current := plan.Model
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
				if badge := priceBadge(p.Name(), m); badge != "" {
					fmt.Printf("%s%-44s %s\n", mark, m, badge)
				} else {
					fmt.Println(mark + m)
				}
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
