package cli

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/cleaner"
)

func uninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall <app-name>",
		Short: "Deep uninstallation of an application and its leftover data",
		Long:  "Locates an application by its exact name, along with its preferences, caches, application support and logs, and moves them to the Trash after asking.",
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
				if len(app.Similar) > 0 {
					fmt.Printf("Applications with a similar name (give the exact one): %s\n", strings.Join(app.Similar, ", "))
				}
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

			question := "\nMove these files to the Trash? [y/N]"
			if runtime.GOOS == "windows" {
				question = "\nPermanently remove these files? [y/N]"
			}
			ans := ask(question, "n")
			if ans != "y" && ans != "Y" {
				fmt.Println("Uninstall cancelled.")
				return nil
			}

			failed := 0
			paths := app.DataPaths
			if app.AppBundle != "" {
				paths = append([]string{app.AppBundle}, paths...)
			}
			for _, p := range paths {
				dest, err := cleaner.Remove(p)
				switch {
				case err != nil:
					failed++
					fmt.Printf("  ⚠ Failed to remove %s: %v\n", p, err)
				case dest != "":
					fmt.Printf("  ✓ Moved %s to %s\n", p, dest)
				default:
					fmt.Printf("  ✓ Removed %s\n", p)
				}
			}
			if failed > 0 {
				return fmt.Errorf("%d of %d items could not be removed", failed, len(paths))
			}

			fmt.Printf("\n✓ Clean uninstallation of '%s' completed.\n", appName)
			return nil
		},
	}
}
