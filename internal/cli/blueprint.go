package cli

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/blueprint"
)

func exportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "export [file]",
		Short: "Export system blueprint (packages, dev tools, and Omega configuration)",
		Long:  "Generates an omega-blueprint.json file capturing all installed software for portability and backup.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			outFile := "omega-blueprint.json"
			if len(args) == 1 {
				outFile = args[0]
			}
			absPath, _ := filepath.Abs(outFile)
			fmt.Printf("Exporting system blueprint to %s...\n", absPath)

			bp, err := blueprint.Export(context.Background(), absPath)
			if err != nil {
				return err
			}
			fmt.Printf("✓ Blueprint exported successfully!\n")
			fmt.Printf("  • Homebrew formulae: %d\n", len(bp.Homebrew))
			fmt.Printf("  • Homebrew casks:    %d\n", len(bp.Casks))
			fmt.Printf("  • Global npm:        %d\n", len(bp.NpmGlobal))
			fmt.Printf("  • Pipx packages:     %d\n", len(bp.PipxPackages))
			fmt.Printf("  • Dev tools:         %d\n", len(bp.DevTools))
			fmt.Printf("\nTo restore on another machine: omega bootstrap %s\n", outFile)
			return nil
		},
	}
}

func bootstrapCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "bootstrap <blueprint-file>",
		Short: "Restore and provision a system from an Omega blueprint",
		Long:  "Reads an omega-blueprint.json file and installs all specified packages, developer tools, and configuration.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]
			fmt.Printf("Bootstrapping system from %s...\n\n", filePath)
			return blueprint.Bootstrap(context.Background(), filePath, func(line string) {
				fmt.Println("  " + line)
			})
		},
	}
}
