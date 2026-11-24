package cli

import (
	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/i18n"
)

func quickCmd(f *runFlags) *cobra.Command {
	c := &cobra.Command{
		Use:     "quick",
		Short:   i18n.T("cmd.quick"),
		Long:    i18n.T("cmd.quick_long"),
		Example: "  omega quick\n  omega quick --no-ai --yes",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTask(agent.TaskQuick, *f)
		},
	}
	bindRunFlags(c.Flags(), f)
	return c
}
