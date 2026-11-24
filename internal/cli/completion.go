package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/models"
	"github.com/lcajigasm/omega/internal/pending"
	"github.com/lcajigasm/omega/internal/steps"
)

func completionCmd(rootCmd *cobra.Command) *cobra.Command {
	c := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell|install]",
		Short: i18n.T("cmd.completion"),
		Long:  i18n.T("cmd.completion_long"),
		Example: "  omega completion install\n  omega completion zsh > ~/.zfunc/_omega\n" +
			"  source <(omega completion bash)",
	}

	c.AddCommand(
		&cobra.Command{
			Use:   "zsh",
			Short: i18n.T("cmd.completion_shell", "zsh"),
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return rootCmd.GenZshCompletion(cmd.OutOrStdout())
			},
		},
		&cobra.Command{
			Use:   "bash",
			Short: i18n.T("cmd.completion_shell", "bash"),
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return rootCmd.GenBashCompletionV2(cmd.OutOrStdout(), true)
			},
		},
		&cobra.Command{
			Use:   "fish",
			Short: i18n.T("cmd.completion_shell", "fish"),
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return rootCmd.GenFishCompletion(cmd.OutOrStdout(), true)
			},
		},
		&cobra.Command{
			Use:   "powershell",
			Short: i18n.T("cmd.completion_shell", "powershell"),
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return rootCmd.GenPowerShellCompletionWithDesc(cmd.OutOrStdout())
			},
		},
		&cobra.Command{
			Use:   "install",
			Short: i18n.T("cmd.completion_install"),
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				home, _ := os.UserHomeDir()
				kind, target, err := completionTarget(goos, os.Getenv("SHELL"), home)
				if err != nil {
					return err
				}
				if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
					return err
				}
				f, err := os.Create(target)
				if err != nil {
					return err
				}
				defer f.Close()
				switch kind {
				case "fish":
					err = rootCmd.GenFishCompletion(f, true)
				case "zsh":
					err = rootCmd.GenZshCompletion(f)
				case "powershell":
					err = rootCmd.GenPowerShellCompletionWithDesc(f)
				default:
					err = rootCmd.GenBashCompletionV2(f, true)
				}
				if err != nil {
					return err
				}
				fmt.Println(i18n.T("completion.installed", kind, target))
				switch kind {
				case "zsh":
					fmt.Println(i18n.T("completion.zsh_tip"))
				case "powershell":
					// PowerShell loads nothing by itself: the profile has to.
					fmt.Println(i18n.T("completion.powershell", target))
				}
				return nil
			},
		},
	)

	return c
}

// completionTarget says which completion script to install and where, from
// the user's shell. On Windows there is no $SHELL and the shell is
// PowerShell. A shell omega has no script for is an error: a bash script
// written "successfully" for it would complete nothing.
func completionTarget(goos, shell, home string) (kind, path string, err error) {
	if shell = strings.TrimSpace(shell); shell != "" {
		shell = strings.ToLower(filepath.Base(strings.ReplaceAll(shell, `\`, "/")))
	}
	switch {
	case strings.Contains(shell, "fish"):
		return "fish", filepath.Join(home, ".config", "fish", "completions", "omega.fish"), nil
	case strings.Contains(shell, "zsh"):
		return "zsh", filepath.Join(home, ".zfunc", "_omega"), nil
	case strings.Contains(shell, "bash"):
		return "bash", filepath.Join(home, ".local", "share", "bash-completion", "completions", "omega"), nil
	case strings.Contains(shell, "pwsh") || strings.Contains(shell, "powershell") || (shell == "" && goos == "windows"):
		return "powershell", filepath.Join(config.Dir(), "omega-completion.ps1"), nil
	}
	return "", "", fmt.Errorf("%s", i18n.T("completion.unknown_shell", shell))
}

// registerCompletions completes the values of the flags and arguments that
// have a known set: providers, efforts, formats, models, step ids, pending
// ids and run numbers. Nothing here reads the network.
func registerCompletions(root *cobra.Command) {
	fixed := func(values ...string) cobra.CompletionFunc {
		return func(*cobra.Command, []string, string) ([]cobra.Completion, cobra.ShellCompDirective) {
			return values, cobra.ShellCompDirectiveNoFileComp
		}
	}
	var providers []string
	for _, p := range llm.Providers {
		providers = append(providers, p.Name+"\t"+p.Label)
	}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for name, fn := range map[string]cobra.CompletionFunc{
			"provider": fixed(providers...),
			"effort":   fixed("low", "medium", "high", "xhigh", "max"),
			"format":   fixed("json", "text"),
			"model":    completeModels,
		} {
			if f := c.Flags().Lookup(name); f != nil && f.Value.Type() == "string" {
				_ = c.RegisterFlagCompletionFunc(name, fn)
			}
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
	// schedule install is written elsewhere; its flags are found by name.
	if c, _, err := root.Find([]string{"schedule", "install"}); err == nil && c.Name() == "install" {
		if c.Flags().Lookup("task") != nil {
			_ = c.RegisterFlagCompletionFunc("task", fixed("quick", "update", "clean", "analyze", "health", "full", "custom"))
		}
		if c.Flags().Lookup("interval") != nil {
			_ = c.RegisterFlagCompletionFunc("interval", fixed("daily", "weekly", "monthly"))
		}
	}
}

// completeModels offers the models of the provider named on the command
// line (or the configured one) from the stored catalogue, without asking
// the provider.
func completeModels(cmd *cobra.Command, _ []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
	cfg, err := config.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	provider := cfg.Provider
	if f := cmd.Flags().Lookup("provider"); f != nil && f.Value.String() != "" {
		provider = f.Value.String()
	}
	out := []string{"auto"}
	if m := cfg.ModelFor(provider); m != "" && m != "auto" {
		out = append(out, m)
	}
	for _, c := range models.Load(modelStorePath()).For(provider, nil, time.Now()) {
		if !slices.Contains(out, c.Native) {
			out = append(out, c.Native)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

// completeSteps offers the ids of the steps installed here.
func completeSteps(*cobra.Command, []string, string) ([]cobra.Completion, cobra.ShellCompDirective) {
	var ids []string
	for _, s := range steps.All {
		if s.Available() {
			ids = append(ids, s.ID+"\t"+i18n.TOr("step."+s.ID, s.Name))
		}
	}
	return ids, cobra.ShellCompDirectiveNoFileComp
}

// completeAllSteps offers every step id of this system, installed or not.
func completeAllSteps(*cobra.Command, []string, string) ([]cobra.Completion, cobra.ShellCompDirective) {
	var ids []string
	for _, s := range steps.All {
		ids = append(ids, s.ID+"\t"+i18n.TOr("step."+s.ID, s.Name))
	}
	return ids, cobra.ShellCompDirectiveNoFileComp
}

// completePending offers the ids of the pending items, the first argument
// only.
func completePending(_ *cobra.Command, args []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	items, _ := pending.Load()
	var ids []string
	for _, it := range items {
		ids = append(ids, it.ID+"\t"+it.Text)
	}
	return ids, cobra.ShellCompDirectiveNoFileComp
}

// completeLogs offers the numbers `omega logs` shows, newest first.
func completeLogs(_ *cobra.Command, args []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	logs, _ := agent.ListRunLogs()
	var out []string
	for i, l := range logs {
		out = append(out, fmt.Sprintf("%d\t%s", i+1, strings.TrimSuffix(l.Name, ".log")))
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}
