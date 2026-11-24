package cli

import (
	"context"
	"encoding/json"
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
	"github.com/lcajigasm/omega/internal/ui"
)

func exportCmd() *cobra.Command {
	var force bool
	c := &cobra.Command{
		Use:   "export [file | -]",
		Short: i18n.T("cmd.export"),
		Long:  i18n.T("cmd.export_long"),
		Example: "  omega export\n" +
			"  omega export ~/Desktop/mac.json\n" +
			"  omega export - > blueprint.json\n" +
			"  omega export old.json --force",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			outFile := "omega-blueprint.json"
			if len(args) == 1 {
				outFile = args[0]
			}
			return exportBlueprint(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), outFile, force, stdinIsTerminal(), askYesNo)
		},
	}
	c.Flags().BoolVarP(&force, "force", "f", false, i18n.T("flag.export_force"))
	return c
}

// exportBlueprint writes the blueprint to file, or to w when file is "-".
// A file that is there is replaced only with force or after a yes at a
// terminal; without one it is an error.
func exportBlueprint(ctx context.Context, w, errw io.Writer, file string, force, tty bool, confirm func(string) bool) error {
	if file == "-" {
		bp, err := blueprint.Export(ctx, "")
		if err != nil {
			return err
		}
		data, err := json.MarshalIndent(bp, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, string(data))
		for _, u := range bp.Unlisted {
			fmt.Fprintln(errw, "omega export: ⚠ "+u)
		}
		return err
	}
	absPath, _ := filepath.Abs(file)
	if fi, err := os.Stat(absPath); err == nil {
		if fi.IsDir() {
			return fmt.Errorf("%s", i18n.T("export.is_dir", absPath))
		}
		if !force {
			if !tty {
				return fmt.Errorf("%s", i18n.T("export.exists", absPath))
			}
			if !confirm(i18n.T("export.overwrite", absPath)) {
				fmt.Fprintln(w, i18n.T("tools.nothing_done"))
				return nil
			}
		}
	}
	r := ui.NewReport(w)
	r.Title("omega export", tildePath(absPath))
	bp, err := blueprint.Export(ctx, absPath)
	if err != nil {
		return err
	}
	r.Blank()
	const lw = 20
	r.Check(ui.MarkOK, i18n.T("export.brew"), lw, fmt.Sprint(len(bp.Homebrew)))
	r.Check(ui.MarkOK, i18n.T("export.casks"), lw, fmt.Sprint(len(bp.Casks)))
	r.Check(ui.MarkOK, i18n.T("export.npm"), lw, fmt.Sprint(len(bp.NpmGlobal)))
	r.Check(ui.MarkOK, i18n.T("export.pipx"), lw, fmt.Sprint(len(bp.PipxPackages)))
	r.Check(ui.MarkOK, i18n.T("export.devtools"), lw, fmt.Sprint(len(bp.DevTools)))
	if bp.Routines != "" {
		r.Check(ui.MarkOK, i18n.T("export.routines"), lw, filepath.Base(config.CustomPath()))
	}
	if len(bp.Unlisted) > 0 {
		// An incomplete file that says "exported successfully" would
		// be trusted as the whole machine.
		r.Blank()
		r.Line(i18n.T("bp.unlisted"))
		for _, u := range bp.Unlisted {
			r.Check(ui.MarkWarn, u, 0, "")
		}
	}
	r.Blank()
	r.Line(i18n.T("export.done", tildePath(absPath)))
	r.Hint(i18n.T("export.restore", file))
	return nil
}

// bootstrapRun is one `omega bootstrap`: what was asked for and who answers.
type bootstrapRun struct {
	yes, dryRun bool
	force       bool                       // replace a different custom.toml without asking
	tty         bool                       // somebody can answer
	confirm     func(question string) bool // asks at the terminal
}

func bootstrapCmd() *cobra.Command {
	var b bootstrapRun
	c := &cobra.Command{
		Use:     "bootstrap <blueprint-file>",
		Short:   i18n.T("cmd.bootstrap"),
		Long:    i18n.T("cmd.bootstrap_long"),
		Example: "  omega bootstrap omega-blueprint.json --dry-run\n  omega bootstrap omega-blueprint.json\n  omega bootstrap omega-blueprint.json --yes --force",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b.tty = term.IsTerminal(int(os.Stdin.Fd()))
			b.confirm = askYesNo
			return b.run(cmd.OutOrStdout(), args[0])
		},
	}
	c.Flags().BoolVarP(&b.yes, "yes", "y", false, i18n.T("flag.bp_yes"))
	c.Flags().BoolVarP(&b.dryRun, "dry-run", "n", false, i18n.T("flag.bp_dry"))
	c.Flags().BoolVarP(&b.force, "force", "f", false, i18n.T("flag.bp_force"))
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
	writeRoutines, replacesRoutines := bp.RoutinesChange()

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
	switch {
	case replacesRoutines:
		fmt.Fprintln(w, "  • "+i18n.T("bp.routines_replace", config.CustomPath(), config.CustomPath()+".bak"))
	case writeRoutines:
		fmt.Fprintln(w, "  • "+i18n.T("bp.routines_write", config.CustomPath()))
	}
	if len(actions) == 0 && newCfg == nil && !writeRoutines {
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
	// Routines that differ from the ones here replace them only with
	// --force or after a yes at the terminal: --yes alone keeps them.
	routines := false
	switch {
	case !writeRoutines:
	case b.force:
		routines = true
	case !replacesRoutines:
		routines = b.yes || b.confirm(i18n.T("bp.ask_routines"))
	case b.tty:
		routines = b.confirm(i18n.T("bp.ask_routines_replace", config.CustomPath()))
	default:
		fmt.Fprintln(w, "  – "+i18n.T("bp.routines_kept", config.CustomPath()))
	}
	if !install && !restore && !routines {
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
	if routines {
		backup, err := bp.SaveRoutines()
		switch {
		case err != nil:
			failed++
			fmt.Fprintln(w, "  ✗ "+i18n.T("bp.routines_failed", err))
		case backup != "":
			fmt.Fprintln(w, "  ✓ "+i18n.T("bp.routines_saved", backup))
		default:
			fmt.Fprintln(w, "  ✓ "+i18n.T("bp.routines_new", config.CustomPath()))
		}
	}
	if install {
		failed += len(blueprint.Apply(context.Background(), actions, func(line string) { fmt.Fprintln(w, "  "+line) }))
	}
	if failed > 0 {
		return fmt.Errorf("%s", i18n.N("bp.failed", failed))
	}
	fmt.Fprintln(w, i18n.T("bp.done"))
	return nil
}
