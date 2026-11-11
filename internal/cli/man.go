package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func manCmd(rootCmd *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:   "man [directory]",
		Short: "Generate troff man pages for Omega commands",
		Long:  "Generates Unix man pages in troff format (omega.1) into the specified directory (default ./man).",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			outDir := "./man"
			if len(args) == 1 {
				outDir = args[0]
			}
			if err := os.MkdirAll(outDir, 0755); err != nil {
				return err
			}
			manFile := filepath.Join(outDir, "omega.1")
			content := generateManPage(rootCmd)
			if err := os.WriteFile(manFile, []byte(content), 0644); err != nil {
				return err
			}
			abs, _ := filepath.Abs(manFile)
			fmt.Printf("✓ Man page generated at %s\n", abs)
			return nil
		},
	}
}

func generateManPage(cmd *cobra.Command) string {
	var b strings.Builder
	b.WriteString(".TH OMEGA 1 \"2026\" \"omega\" \"Omega Manual\"\n")
	b.WriteString(".SH NAME\n")
	b.WriteString("omega \\- Total AI-driven and deterministic system maintenance tool\n")
	b.WriteString(".SH SYNOPSIS\n")
	b.WriteString(".B omega\n[\\fItask\\fR] [\\fIoptions\\fR]\n")
	b.WriteString(".SH DESCRIPTION\n")
	b.WriteString("omega checks, updates, cleans, and optimizes macOS, Linux, and Windows systems.\n")
	b.WriteString(".SH COMMANDS\n")
	for _, c := range cmd.Commands() {
		if c.Hidden {
			continue
		}
		b.WriteString(fmt.Sprintf(".TP\n\\fB%s\\fR\n%s\n", c.Name(), c.Short))
	}
	b.WriteString(".SH OPTIONS\n")
	b.WriteString(".TP\n\\fB--no-ai, --offline\\fR\nRun deterministically without invoking an LLM provider\n")
	b.WriteString(".TP\n\\fB--json, --format\\fR\nProduce machine-readable JSON or YAML output\n")
	b.WriteString(".TP\n\\fB-y, --yes\\fR\nAutomatically approve safe non-destructive actions\n")
	b.WriteString(".TP\n\\fB-n, --dry-run\\fR\nSimulate actions without executing modifications\n")
	b.WriteString(".TP\n\\fB-q, --quiet\\fR\nQuiet mode; only print errors\n")
	b.WriteString(".TP\n\\fB-v, --verbose\\fR\nVerbose execution output\n")
	b.WriteString(".SH AUTHOR\n")
	b.WriteString("Luis Cajigas and the Omega contributors.\n")
	return b.String()
}
