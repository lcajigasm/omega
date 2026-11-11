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

			fmt.Printf("\n🛡️ System Privacy & Telemetry Review\n\n")
			if len(audit.TelemetryFindings) > 0 {
				fmt.Println("Telemetry & Diagnostics:")
				for _, t := range audit.TelemetryFindings {
					fmt.Printf("  • %s\n", t)
				}
				fmt.Println()
			}

			fmt.Println("Browser Caches:")
			if len(audit.BrowserCaches) == 0 {
				fmt.Println("  ✓ No heavy browser caches detected.")
			} else {
				var totalMB int64
				for _, b := range audit.BrowserCaches {
					fmt.Printf("  • %-24s %6d MB  (%s)\n", b.Browser, b.SizeMB, b.Path)
					totalMB += b.SizeMB
				}
				fmt.Printf("\n  Total browser cache space: %d MB\n", totalMB)

				if clean {
					ans := ask("\nClear all detected browser caches? (cookies and logins are kept) [y/N]", "n")
					if ans == "y" || ans == "Y" {
						for _, b := range audit.BrowserCaches {
							_ = os.RemoveAll(b.Path)
							_ = os.MkdirAll(b.Path, 0755)
							fmt.Printf("  ✓ Cleaned %s cache\n", b.Browser)
						}
					}
				} else {
					fmt.Println("\nTip: Run 'omega privacy --clean' to clear browser caches safely.")
				}
			}
			fmt.Println()
			return nil
		},
	}

	c.Flags().BoolVar(&clean, "clean", false, "Clean detected browser caches")
	return c
}
