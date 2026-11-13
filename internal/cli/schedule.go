package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/scheduler"
)

func scheduleCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "schedule",
		Short: "Automated background maintenance scheduling and notifications",
		Long:  "Manage automatic maintenance via native system daemons (launchd, systemd --user, Windows Task Scheduler).",
	}

	var interval, task string
	var hour, minute int

	install := &cobra.Command{
		Use:   "install",
		Short: "Install native background maintenance automation",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := scheduler.Install(interval, task, hour, minute); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Background maintenance installed successfully (%s, task: %s, at %02d:%02d).\n",
				interval, task, hour, minute)
			// Say what will run unattended, and where its output goes.
			fmt.Fprintf(cmd.OutOrStdout(), "  It runs `omega %s --yes --no-tui`. %s\n", task, scheduler.Describe(task))
			if runtime.GOOS == "darwin" {
				fmt.Fprintf(cmd.OutOrStdout(), "  Its output is kept in %s.\n", scheduler.LogDir())
			}
			return nil
		},
	}
	install.Flags().StringVar(&interval, "interval", "weekly", "Schedule interval: 'daily', 'weekly' (Saturdays) or 'monthly' (the 1st)")
	install.Flags().StringVar(&task, "task", "quick", "Omega task to run: 'quick', 'clean', 'update', 'full', 'analyze', 'health'")
	install.Flags().IntVar(&hour, "hour", 3, "Hour of day (0-23)")
	install.Flags().IntVar(&minute, "minute", 0, "Minute of hour (0-59)")

	c.AddCommand(
		install,
		&cobra.Command{
			Use:   "status",
			Short: "Check background maintenance daemon status",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				st, err := scheduler.Status()
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "\nBackground Maintenance Status (%s):\n", st.Platform)
				if !st.Installed {
					fmt.Fprintln(cmd.OutOrStdout(), "  Status: Not installed.")
					if st.Details != "" {
						fmt.Fprintf(cmd.OutOrStdout(), "  Details: %s\n", st.Details)
					}
					fmt.Fprintln(cmd.OutOrStdout(), "  Run 'omega schedule install' to set up automatic maintenance.")
					return nil
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  Status: Installed\n  Target: %s\n", st.Target)
				if st.Details != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "  Details: %s\n", st.Details)
				}
				return nil
			},
		},
		&cobra.Command{
			Use:   "uninstall",
			Short: "Remove background maintenance automation",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := scheduler.Uninstall(); err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), "✓ Background maintenance daemon uninstalled successfully.")
				return nil
			},
		},
	)

	return c
}
