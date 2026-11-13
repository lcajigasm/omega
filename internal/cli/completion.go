package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
)

func completionCmd(rootCmd *cobra.Command) *cobra.Command {
	c := &cobra.Command{
		Use:    "completion [bash|zsh|fish|powershell]",
		Short:  "Generate shell completion scripts",
		Long:   "Generate autocompletion script for bash, zsh, fish, or powershell, or install it automatically.",
		Hidden: true,
	}

	c.AddCommand(
		&cobra.Command{
			Use:   "zsh",
			Short: "Generate zsh completion script",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return rootCmd.GenZshCompletion(os.Stdout)
			},
		},
		&cobra.Command{
			Use:   "bash",
			Short: "Generate bash completion script",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return rootCmd.GenBashCompletion(os.Stdout)
			},
		},
		&cobra.Command{
			Use:   "fish",
			Short: "Generate fish completion script",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return rootCmd.GenFishCompletion(os.Stdout, true)
			},
		},
		&cobra.Command{
			Use:   "powershell",
			Short: "Generate powershell completion script",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
			},
		},
		&cobra.Command{
			Use:   "install",
			Short: "Automatically install shell autocompletion for the current user shell",
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
					err = rootCmd.GenBashCompletion(f)
				}
				if err != nil {
					return err
				}
				fmt.Println(i18n.T("completion.installed", kind, target))
				switch kind {
				case "zsh":
					fmt.Println("Tip: ensure 'fpath=(~/.zfunc $fpath)' and 'autoload -Uz compinit && compinit' are in ~/.zshrc")
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
