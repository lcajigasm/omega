package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/cleaner"
)

func uninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall <app-name>",
		Short: "Deep uninstallation of an application and its leftover data",
		Long:  "Locates an application bundle/executable along with associated preferences, caches, application support, and logs.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			appName := args[0]
			fmt.Printf("\n🔍 Searching for application artifacts for '%s'...\n", appName)

			app, err := cleaner.FindAppArtifacts(appName)
			if err != nil {
				return err
			}

			if app.AppBundle == "" && len(app.DataPaths) == 0 {
				fmt.Printf("No files found for application '%s'.\n", appName)
				return nil
			}

			fmt.Println("\nIdentified application files:")
			if app.AppBundle != "" {
				fmt.Printf("  • Application: %s\n", app.AppBundle)
			}
			for _, p := range app.DataPaths {
				fmt.Printf("  • Leftover:    %s\n", p)
			}
			fmt.Printf("\nTotal size: %.1f MB\n", app.TotalSizeMB)

			ans := ask("\nDo you want to permanently remove these files? [y/N]", "n")
			if ans != "y" && ans != "Y" {
				fmt.Println("Uninstall cancelled.")
				return nil
			}

			if app.AppBundle != "" {
				if err := os.RemoveAll(app.AppBundle); err != nil {
					fmt.Printf("  ⚠ Failed to remove %s: %v\n", app.AppBundle, err)
				} else {
					fmt.Printf("  ✓ Removed %s\n", app.AppBundle)
				}
			}
			for _, p := range app.DataPaths {
				if err := os.RemoveAll(p); err != nil {
					fmt.Printf("  ⚠ Failed to remove %s: %v\n", p, err)
				} else {
					fmt.Printf("  ✓ Removed %s\n", p)
				}
			}

			fmt.Printf("\n✓ Clean uninstallation of '%s' completed.\n", appName)
			return nil
		},
	}
}
