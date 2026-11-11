package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/cleaner"
	"github.com/lcajigasm/omega/internal/gitops"
)

func diskCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "disk",
		Short: "Disk space analysis, duplicate finder and build artifact pruning",
		Long:  "Tools to inspect large files, identify duplicate files, and prune stale build folders (node_modules, target, .venv).",
	}

	c.AddCommand(
		&cobra.Command{
			Use:   "duplicates [dir]",
			Short: "Find identical duplicate files by size and SHA-256 hash",
			Args:  cobra.MaximumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				dir := "."
				if len(args) == 1 {
					dir = args[0]
				} else {
					home, _ := os.UserHomeDir()
					dir = filepath.Join(home, "Downloads")
				}

				fmt.Printf("\n🔍 Scanning '%s' for duplicate files (>1MB)...\n", dir)
				dups, err := cleaner.FindDuplicates(dir, 1)
				if err != nil {
					return err
				}
				if len(dups) == 0 {
					fmt.Println("  ✓ No duplicate files found.")
					return nil
				}

				var totalWasted float64
				for i, g := range dups {
					wasted := g.SizeMB * float64(len(g.Paths)-1)
					totalWasted += wasted
					fmt.Printf("\n[%d] Duplicate set (%.1f MB per file, wasted: %.1f MB):\n", i+1, g.SizeMB, wasted)
					for _, p := range g.Paths {
						fmt.Printf("    • %s\n", p)
					}
				}
				fmt.Printf("\nTotal space wasted by duplicates: %.1f MB\n", totalWasted)
				return nil
			},
		},
		&cobra.Command{
			Use:   "large [dir] [minMB]",
			Short: "Find large files taking up disk space",
			Args:  cobra.MaximumNArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				dir := "."
				if len(args) >= 1 {
					dir = args[0]
				} else {
					home, _ := os.UserHomeDir()
					dir = home
				}
				minMB := int64(100)
				if len(args) == 2 {
					if v, err := strconv.ParseInt(args[1], 10, 64); err == nil && v > 0 {
						minMB = v
					}
				}

				fmt.Printf("\n🔍 Finding files larger than %d MB in '%s'...\n\n", minMB, dir)
				files, err := cleaner.FindLargeFiles(dir, minMB, 25)
				if err != nil {
					return err
				}
				if len(files) == 0 {
					fmt.Printf("  No files larger than %d MB found.\n", minMB)
					return nil
				}
				for _, f := range files {
					fmt.Printf("  %8.1f MB  %s\n", f.SizeMB, f.Path)
				}
				fmt.Println()
				return nil
			},
		},
		&cobra.Command{
			Use:   "artifacts [days]",
			Short: "Find and prune old build artifacts (node_modules, target, .venv) in dev projects",
			Args:  cobra.MaximumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				days := 30
				if len(args) == 1 {
					if d, err := strconv.Atoi(args[0]); err == nil && d > 0 {
						days = d
					}
				}

				roots := gitops.DefaultRoots()
				fmt.Printf("\n🔍 Scanning dev projects for artifacts untouched for >%d days...\n", days)
				artifacts := cleaner.FindArtifacts(roots, days)
				if len(artifacts) == 0 {
					fmt.Println("  ✓ No stale build artifacts found.")
					return nil
				}

				var totalMB int64
				fmt.Printf("\n%-18s %10s  %s\n", "Type", "Size", "Path")
				fmt.Println(stringsRepeat("─", 75))
				for _, a := range artifacts {
					fmt.Printf("%-18s %8d MB  %s (inactive %d days)\n", a.Type, a.SizeMB, a.Path, a.DaysOld)
					totalMB += a.SizeMB
				}
				fmt.Printf("\nTotal reclaimable space: %d MB (%.1f GB)\n", totalMB, float64(totalMB)/1024)

				ans := ask("\nDelete these stale build artifacts? [y/N]", "n")
				if ans == "y" || ans == "Y" {
					for _, a := range artifacts {
						_ = cleaner.PruneArtifact(a.Path)
						fmt.Printf("  ✓ Removed %s\n", a.Path)
					}
					fmt.Println("\n✓ All selected artifacts deleted.")
				}
				return nil
			},
		},
	)

	return c
}
