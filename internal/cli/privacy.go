package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/secscan"
)

func privacyCmd() *cobra.Command {
	var clean bool
	c := &cobra.Command{
		Use:   "privacy",
		Short: "Review OS telemetry settings and browser caches",
		Long:  "Inspect system diagnostic telemetry and report or clear web browser disk caches.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			audit, err := secscan.AuditPrivacy()
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "\n🛡️ System Privacy & Telemetry Review\n\n")
			if len(audit.TelemetryFindings) > 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "Telemetry & Diagnostics:")
				for _, t := range audit.TelemetryFindings {
					fmt.Fprintf(cmd.OutOrStdout(), "  • %s\n", t)
				}
				fmt.Fprintln(cmd.OutOrStdout())
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Browser Caches:")
			if len(audit.BrowserCaches) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "  ✓ No heavy browser caches detected.")
			} else {
				var totalMB int64
				for _, b := range audit.BrowserCaches {
					fmt.Fprintf(cmd.OutOrStdout(), "  • %-24s %6d MB  (%s)\n", b.Browser, b.SizeMB, b.Path)
					totalMB += b.SizeMB
				}
				fmt.Fprintf(cmd.OutOrStdout(), "\n  Total browser cache space: %d MB\n", totalMB)

				if clean {
					if askYesNo("\nClear all detected browser caches? (cookies and logins are kept)") {
						failed := 0
						for _, b := range audit.BrowserCaches {
							// A browser that is open keeps files in use: say what
							// was not cleared instead of a tick.
							err := os.RemoveAll(b.Path)
							if err == nil {
								err = os.MkdirAll(b.Path, 0755)
							}
							if err != nil {
								failed++
								fmt.Fprintf(cmd.OutOrStdout(), "  ⚠ %s cache not fully cleared: %v\n", b.Browser, err)
								continue
							}
							fmt.Fprintf(cmd.OutOrStdout(), "  ✓ Cleaned %s cache\n", b.Browser)
						}
						if failed > 0 {
							return fmt.Errorf("%d of %d browser caches could not be cleared; close the browser and try again", failed, len(audit.BrowserCaches))
						}
					}
				} else {
					fmt.Fprintln(cmd.OutOrStdout(), "\nTip: Run 'omega privacy --clean' to clear browser caches safely.")
				}
			}
			fmt.Fprintln(cmd.OutOrStdout())
			return nil
		},
	}

	c.Flags().BoolVar(&clean, "clean", false, "Clean detected browser caches")
	return c
}
