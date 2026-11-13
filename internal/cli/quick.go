package cli

import (
	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/agent"
)

func quickCmd(f *runFlags) *cobra.Command {
	c := &cobra.Command{
		Use:   "quick",
		Short: "One-click safe maintenance for everyday users",
		Long: "A short round for every day. With an AI model it does the routine maintenance; with --no-ai (or with no API key)\n" +
			"it empties the Trash and the thumbnail and temporary-file caches (asking first, or with --yes), shows the battery\n" +
			"and the free disk space, and counts the package updates that are pending. It reports what it looked at;\n" +
			"it does not diagnose the hardware: `omega health` does.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTask(agent.TaskQuick, *f)
		},
	}
	bindRunFlags(c.Flags(), f)
	return c
}
