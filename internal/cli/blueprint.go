package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/lcajigasm/omega/internal/blueprint"
	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
)

func exportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "export [file]",
		Short: "Export system blueprint (packages, dev tools, and Omega configuration)",
		Long: "Writes omega-blueprint.json: the Homebrew formulae and casks, the global npm packages and the pipx packages\n" +
			"installed here, the developer tools omega found (for reference) and omega's settings.\n" +
			"`omega bootstrap` installs the packages on another machine. The file is readable by you only.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			outFile := "omega-blueprint.json"
			if len(args) == 1 {
				outFile = args[0]
			}
			absPath, _ := filepath.Abs(outFile)
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Exporting system blueprint to %s...\n", absPath)

			bp, err := blueprint.Export(context.Background(), absPath)
			if err != nil {
				return err
			}
			fmt.Fprintf(w, "✓ Blueprint exported successfully!\n")
			fmt.Fprintf(w, "  • Homebrew formulae: %d\n", len(bp.Homebrew))
			fmt.Fprintf(w, "  • Homebrew casks:    %d\n", len(bp.Casks))
			fmt.Fprintf(w, "  • Global npm:        %d\n", len(bp.NpmGlobal))
			fmt.Fprintf(w, "  • Pipx packages:     %d\n", len(bp.PipxPackages))
			fmt.Fprintf(w, "  • Dev tools:         %d\n", len(bp.DevTools))
			if len(bp.Unlisted) > 0 {
				// An incomplete file that says "exported successfully" would
				// be trusted as the whole machine.
				fmt.Fprintln(w, "\n"+i18n.T("bp.unlisted"))
				for _, u := range bp.Unlisted {
					fmt.Fprintln(w, "  ⚠ "+u)
				}
			}
			fmt.Fprintf(w, "\nTo restore on another machine: omega bootstrap %s\n", outFile)
			return nil
		},
	}
}

// bootstrapRun is one `omega bootstrap`: what was asked for and who answers.
type bootstrapRun struct {
	yes, dryRun bool
	tty         bool                       // somebody can answer
	confirm     func(question string) bool // asks at the terminal
}

func bootstrapCmd() *cobra.Command {
	var b bootstrapRun
	c := &cobra.Command{
		Use:   "bootstrap <blueprint-file>",
		Short: "Restore and provision a system from an Omega blueprint",
		Long: "Reads an omega-blueprint.json file, shows what it would install (Homebrew formulae and casks, global npm\n" +
			"packages, pipx packages) and installs it after you say yes. omega's settings are restored too, except\n" +
			"`approval` and a provider's `base_url`; the settings file that was there is kept as config.toml.bak.\n" +
			"The developer tools in the file are not installed: use `omega dev install`.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b.tty = term.IsTerminal(int(os.Stdin.Fd()))
			b.confirm = askYesNo
			return b.run(cmd.OutOrStdout(), args[0])
		},
	}
	c.Flags().BoolVarP(&b.yes, "yes", "y", false, i18n.T("flag.bp_yes"))
	c.Flags().BoolVarP(&b.dryRun, "dry-run", "n", false, i18n.T("flag.bp_dry"))
	return c
}

// run shows the plan of a blueprint and carries it out once someone agreed:
// a blueprint is a file from somewhere else that installs software and
// replaces settings, so nothing in it runs unasked.
func (b bootstrapRun) run(w io.Writer, file string) error {
	bp, err := blueprint.Load(file)
	if err != nil {
		return err
	}
	current, err := config.Load()
	if err != nil {
		return err
	}
	actions, skipped := bp.Plan()
	newCfg, kept := bp.RestoredConfig(current)
	if newCfg != nil && reflect.DeepEqual(newCfg, current) {
		newCfg = nil // the settings are already these
	}

	fmt.Fprintln(w, i18n.T("bp.plan", file, bp.ExportDate.Format("2006-01-02"), bp.Platform))
	for _, a := range actions {
		fmt.Fprintf(w, "  • %s\n      $ %s\n", a.What, a.Command)
	}
	for _, s := range skipped {
		fmt.Fprintln(w, "  – "+i18n.T("bp.skipped", s))
	}
	if newCfg != nil {
		fmt.Fprintln(w, "  • "+i18n.T("bp.config_replace", config.Path(), config.Path()+".bak"))
	}
	for _, k := range kept {
		fmt.Fprintln(w, "      "+i18n.T("bp.config_kept", k))
	}
	if len(actions) == 0 && newCfg == nil {
		fmt.Fprintln(w, i18n.T("bp.nothing"))
		return nil
	}
	if b.dryRun {
		fmt.Fprintln(w, "\n"+i18n.T("bp.dry_run"))
		return nil
	}
	if !b.yes && !b.tty {
		return fmt.Errorf("%s", i18n.T("bp.need_yes"))
	}

	fmt.Fprintln(w)
	install := len(actions) > 0 && (b.yes || b.confirm(i18n.T("bp.ask_install")))
	restore := newCfg != nil && (b.yes || b.confirm(i18n.T("bp.ask_config")))
	if !install && !restore {
		fmt.Fprintln(w, i18n.T("bp.cancelled"))
		return nil
	}

	failed := 0
	if restore {
		backup, err := blueprint.SaveConfig(newCfg)
		switch {
		case err != nil:
			failed++
			fmt.Fprintln(w, "  ✗ "+i18n.T("bp.config_failed", err))
		case backup != "":
			fmt.Fprintln(w, "  ✓ "+i18n.T("bp.config_saved", backup))
		default:
			fmt.Fprintln(w, "  ✓ "+i18n.T("bp.config_new", config.Path()))
		}
	}
	if install {
		failed += len(blueprint.Apply(context.Background(), actions, func(line string) { fmt.Fprintln(w, "  "+line) }))
	}
	if failed > 0 {
		return fmt.Errorf("%s", i18n.T("bp.failed", failed))
	}
	fmt.Fprintln(w, i18n.T("bp.done"))
	return nil
}
