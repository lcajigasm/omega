package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/customsteps"
)

func rollbackCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rollback",
		Short: "Inspect and restore filesystem safety snapshots (APFS, Btrfs, Windows)",
		Long:  "Inspect available system restore points and snapshots created before maintenance.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			msg, err := customsteps.Rollback(context.Background())
			if err != nil {
				return err
			}
			fmt.Println(msg)
			return nil
		},
	}
}
