package cli

import (
	"errors"
	"fmt"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/cleaner"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/ui"
)

func uninstallCmd() *cobra.Command {
	var yes bool
	c := &cobra.Command{
		Use:     "uninstall <app-name>",
		Short:   i18n.T("cmd.uninstall"),
		Long:    i18n.T("cmd.uninstall_long"),
		Example: "  omega uninstall Slack\n  omega uninstall \"Visual Studio Code\"\n  omega uninstall com.tinyspeck.slackmacgap",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			appName := args[0]
			app, err := cleaner.FindAppArtifacts(appName)
			if err != nil {
				return err
			}
			r := ui.NewReport(w)
			r.Title("omega uninstall", appName)
			r.Blank()
			if app.AppBundle == "" && len(app.DataPaths) == 0 {
				r.Check(ui.MarkOff, i18n.T("uninstall.none", appName), 0, "")
				if len(app.Similar) > 0 {
					r.Hint(i18n.T("uninstall.similar", strings.Join(app.Similar, ", ")))
				}
				return nil
			}
			const lw = 12
			if app.AppBundle != "" {
				r.Check(ui.MarkInfo, i18n.T("uninstall.app"), lw, tildePath(app.AppBundle))
			}
			for _, p := range app.DataPaths {
				r.Check(ui.MarkInfo, i18n.T("uninstall.leftover"), lw, tildePath(p))
			}
			r.Blank()
			r.Line(i18n.T("uninstall.total", ui.Bytes(int64(app.TotalSizeMB*(1<<20)))))

			question := i18n.T("uninstall.ask_trash")
			if runtime.GOOS == "windows" {
				question = i18n.T("uninstall.ask_delete")
			}
			if !yes {
				if !stdinIsTerminal() {
					return errors.New(i18n.T("uninstall.need_yes"))
				}
				fmt.Fprintln(w)
				if !askYesNo(question) {
					fmt.Fprintln(w, r.Glyph(ui.MarkSkip)+" "+i18n.T("uninstall.cancelled"))
					return nil
				}
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
					r.Check(ui.MarkErr, tildePath(p), 0, err.Error())
				case dest != "":
					r.Check(ui.MarkOK, tildePath(p), 0, i18n.T("uninstall.moved", tildePath(dest)))
				default:
					r.Check(ui.MarkOK, tildePath(p), 0, i18n.T("uninstall.removed"))
				}
			}
			if failed > 0 {
				return fmt.Errorf("%s", i18n.T("uninstall.failed", failed, len(paths)))
			}
			r.Blank()
			r.Check(ui.MarkOK, i18n.T("uninstall.done", appName), 0, "")
			return nil
		},
	}
	c.Flags().BoolVarP(&yes, "yes", "y", false, i18n.T("flag.tools_yes"))
	return c
}
