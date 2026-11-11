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
		Use:   "fix [error...]",
		Short: i18n.T("cmd.fix"),
		Long:  "Diagnose and fix errors from failing commands, topgrade/mole runs, or external build log files.",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			prompt := ""
			if logFile != "" {
				b, err := os.ReadFile(logFile)
				if err != nil {
					return fmt.Errorf("reading log file: %w", err)
				}
				prompt = "I have an error log from a failed build or command. Please diagnose the failure and fix it:\n\n```\n" + string(b) + "\n```"
			} else if len(args) == 1 && args[0] == "-" {
				b, err := io.ReadAll(os.Stdin)
				if err != nil {
					return fmt.Errorf("reading stdin: %w", err)
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
	c.Flags().StringVar(&logFile, "log", "", "path to error log file to diagnose")
	bindRunFlags(c.Flags(), f)
	return c
}
