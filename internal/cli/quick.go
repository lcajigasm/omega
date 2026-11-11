package cli

import (
	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/agent"
)

func quickCmd(f *runFlags) *cobra.Command {
	c := &cobra.Command{
		Use:   "quick",
		Short: "One-click safe maintenance for everyday users",
		Long:  "Performs completely safe cleanup (trash, caches), checks for pending software updates, and verifies battery and disk health.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTask(agent.TaskQuick, *f)
		},
	}
	bindRunFlags(c.Flags(), f)
	return c
}
