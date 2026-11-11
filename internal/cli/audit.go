package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/secscan"
)

func auditCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "audit",
		Short: "Security, SSH keys and secret leak audits",
		Long:  "Inspect SSH configuration permissions and scan shell history for leaked credentials.",
	}

	c.AddCommand(
		&cobra.Command{
			Use:   "ssh",
			Short: "Audit ~/.ssh directory and key permissions",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				findings, err := secscan.AuditSSH()
				if err != nil {
					return err
				}
				fmt.Printf("\n🔒 SSH Permissions & Configuration Audit\n\n")
				if len(findings) == 0 {
					fmt.Println("  ✓ All SSH keys and directory permissions are secure (0700 / 0600).")
					return nil
				}
				for _, f := range findings {
					badge := "⚠"
					if f.Severity == "high" {
						badge = "⛔"
					}
					fmt.Printf("  %s %s: %s\n", badge, f.Path, f.Issue)
					if f.Fix != "" {
						fmt.Printf("    Fix: %s\n", f.Fix)
					}
				}
				fmt.Println()
				return nil
			},
		},
		&cobra.Command{
			Use:   "secrets",
			Short: "Scan shell history and dotfiles for leaked API keys and tokens",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Println("\n🔍 Scanning shell history files for leaked credentials...")
				findings, err := secscan.ScanSecrets()
				if err != nil {
					return err
				}
				if len(findings) == 0 {
					fmt.Println("  ✓ No leaked API keys or credentials detected in shell history.")
					return nil
				}
				fmt.Printf("\nFound %d potential credential leaks:\n", len(findings))
				for _, f := range findings {
					fmt.Printf("  ⚠ [%s] %s:%d\n    %s\n", f.Type, f.File, f.LineNum, f.Snippet)
				}
				fmt.Println("\nTip: Edit the history file to redact the leaked secrets or reset the exposed keys.")
				return nil
			},
		},
	)

	return c
}
