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
		Short: "Write omega's man page: every command, its options and examples",
		Long:  "Writes omega.1 in troff format into the directory given (./man by default). Read it with man -l man/omega.1,\nor copy it into a folder of your MANPATH (for example $(brew --prefix)/share/man/man1).",
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

// roff escapes text for a man page: backslashes, and a dot or quote that
// would start a request at the beginning of a line.
func roff(s string) string {
	s = strings.ReplaceAll(s, `\`, `\e`)
	s = strings.ReplaceAll(s, "-", `\-`)
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(l, ".") || strings.HasPrefix(l, "'") {
			l = `\&` + l
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

func generateManPage(cmd *cobra.Command) string {
	var b strings.Builder
	b.WriteString(".TH OMEGA 1 \"" + version + "\" \"omega\" \"omega manual\"\n")
	b.WriteString(".SH NAME\nomega \\- check, update, clean and tune macOS, Linux and Windows, with or without an AI model\n")
	b.WriteString(".SH SYNOPSIS\n.B omega\n[\\fItask\\fR | \\fIcommand\\fR | \\fB\"what you want, in your words\"\\fR] [\\fIoptions\\fR]\n")
	b.WriteString(".SH DESCRIPTION\n" + roff(cmd.Long) + "\n")
	b.WriteString(".SH GETTING HELP\n")
	b.WriteString(".TP\n\\fBomega <command> \\-\\-help\\fR\nWhat a command does, its subcommands and options.\n")
	b.WriteString(".TP\n\\fBomega docs search <words>\\fR\nSearch the manual, offline.\n")
	b.WriteString(".TP\n\\fBomega docs ask \"<question>\"\\fR\nAsk the manual in your own words and language; your configured model answers from it, with the commands to type.\n")
	b.WriteString(".TP\n\\fBomega mcp\\fR\nLet AI agents (Claude, Codex, Cursor, Gemini CLI) use omega's commands and read this manual.\n")
	b.WriteString(".SH COMMANDS\n")
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			if sub.Hidden || sub.Name() == "help" || !sub.IsAvailableCommand() {
				continue
			}
			b.WriteString(".TP\n\\fB" + roff(sub.CommandPath()) + "\\fR")
			if u := strings.TrimPrefix(sub.Use, sub.Name()); strings.TrimSpace(u) != "" {
				b.WriteString(" \\fI" + roff(strings.TrimSpace(u)) + "\\fR")
			}
			b.WriteString("\n" + roff(sub.Short) + "\n")
			if len(sub.Aliases) > 0 {
				b.WriteString(".br\nAlso: " + roff(strings.Join(sub.Aliases, ", ")) + ".\n")
			}
			if sub.Long != "" {
				b.WriteString(".RS\n.nf\n" + roff(sub.Long) + "\n.fi\n.RE\n")
			}
			if fl := sub.LocalNonPersistentFlags(); fl.HasAvailableFlags() {
				b.WriteString(".RS\n.nf\n" + roff(strings.TrimRight(fl.FlagUsages(), "\n")) + "\n.fi\n.RE\n")
			}
			walk(sub)
		}
	}
	walk(cmd)
	b.WriteString(".SH OPTIONS\n.nf\n" + roff(strings.TrimRight(cmd.Flags().FlagUsages(), "\n")) + "\n.fi\n")
	b.WriteString(".SH EXAMPLES\n.nf\n" + roff(`omega                         the guided run: analyse, then the stages you pick
omega quick --no-ai           the built-in steps, no model
omega "free 20 GB"            a request in your own words
omega disk limits             what Claude, Docker, Cursor... take on disk and their limits
omega disk limits apply       write the recommended limits in each tool's own file
omega assist claude-code      the configuration assistant of one tool
omega agents                  what AI coding agents left running
omega docs ask "how do I schedule a weekly cleanup?"`) + "\n.fi\n")
	b.WriteString(".SH FILES\n.nf\n~/.config/omega/config.toml\n~/.config/omega/notes.md\n~/.local/state/omega/runs/\n~/.local/state/omega/backups/\n.fi\n")
	b.WriteString(".SH SEE ALSO\nThe manual: \\fBomega docs\\fR, or https://github.com/lcajigasm/omega/tree/main/docs/manual\n")
	b.WriteString(".SH AUTHOR\nLuis Cajigas and the omega contributors.\n")
	return b.String()
}
