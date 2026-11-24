package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/i18n"
)

func fixCmd(f *runFlags) *cobra.Command {
	var logFile string
	c := &cobra.Command{
		Use:     "fix [error...]",
		Short:   i18n.T("cmd.fix"),
		Long:    i18n.T("cmd.fix_long"),
		Example: "  omega fix\n  omega fix \"brew upgrade fails with a permission error\"\n  make 2>&1 | omega fix -\n  omega fix --log build.log",
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			prompt := ""
			if logFile != "" {
				b, err := os.ReadFile(logFile)
				if err != nil {
					return fmt.Errorf("--log %s: %w", logFile, err)
				}
				prompt = "I have an error log from a failed build or command. Please diagnose the failure and fix it:\n\n```\n" + string(b) + "\n```"
			} else if len(args) == 1 && args[0] == "-" {
				b, err := io.ReadAll(os.Stdin)
				if err != nil {
					return fmt.Errorf("stdin: %w", err)
				}
				prompt = "Diagnose and fix this error:\n\n```\n" + string(b) + "\n```"
			} else if len(args) > 0 {
				prompt = "Diagnose and fix this error: " + strings.Join(args, " ")
			}
			run := *f
			if prompt != "" {
				run.prompt = prompt
			}
			return runTask(agent.TaskFix, run)
		},
	}
	c.Flags().StringVar(&logFile, "log", "", i18n.T("flag.fix_log"))
	// Only the agent's flags: fixing is the model's work, and the flags of
	// a run without it (--no-ai, --json, -q, -v, --fail-on-*) would only
	// make the command fail.
	bindAgentFlags(c.Flags(), f)
	return c
}
