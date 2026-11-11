package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
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
				shell := strings.ToLower(os.Getenv("SHELL"))

				switch {
				case strings.Contains(shell, "fish"):
					dir := filepath.Join(home, ".config/fish/completions")
					_ = os.MkdirAll(dir, 0755)
					target := filepath.Join(dir, "omega.fish")
					f, err := os.Create(target)
					if err != nil {
						return err
					}
					defer f.Close()
					if err := rootCmd.GenFishCompletion(f, true); err != nil {
						return err
					}
					fmt.Printf("✓ Fish completion installed at %s\n", target)

				case strings.Contains(shell, "zsh"):
					dir := filepath.Join(home, ".zfunc")
					_ = os.MkdirAll(dir, 0755)
					target := filepath.Join(dir, "_omega")
					f, err := os.Create(target)
					if err != nil {
						return err
					}
					defer f.Close()
					if err := rootCmd.GenZshCompletion(f); err != nil {
						return err
					}
					fmt.Printf("✓ Zsh completion installed at %s\n", target)
					fmt.Println("Tip: ensure 'fpath=(~/.zfunc $fpath)' and 'autoload -Uz compinit && compinit' are in ~/.zshrc")

				default: // bash / fallback
					dir := filepath.Join(home, ".local/share/bash-completion/completions")
					_ = os.MkdirAll(dir, 0755)
					target := filepath.Join(dir, "omega")
					f, err := os.Create(target)
					if err != nil {
						return err
					}
					defer f.Close()
					if err := rootCmd.GenBashCompletion(f); err != nil {
						return err
					}
					fmt.Printf("✓ Bash completion installed at %s\n", target)
				}
				return nil
			},
		},
	)

	return c
}
