package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/secscan"
	"github.com/lcajigasm/omega/internal/ui"
)

func privacyCmd() *cobra.Command {
	var clean, yes, asJSON bool
	c := &cobra.Command{
		Use:     "privacy",
		Short:   i18n.T("cmd.privacy"),
		Long:    i18n.T("cmd.privacy_long"),
		Example: "  omega privacy\n  omega privacy --json\n  omega privacy --clean",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			audit, err := secscan.AuditPrivacy()
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			if asJSON {
				if audit.BrowserCaches == nil {
					audit.BrowserCaches = []secscan.BrowserCache{}
				}
				if audit.TelemetryFindings == nil {
					audit.TelemetryFindings = []string{}
				}
				return writeJSON(w, audit)
			}

			r := ui.NewReport(w)
			r.Title("omega privacy", "")
			if len(audit.TelemetryFindings) > 0 {
				r.Section(i18n.T("privacy.telemetry"), -1)
				for _, t := range audit.TelemetryFindings {
					r.Check(ui.MarkInfo, t, 0, "")
				}
			}
			r.Section(i18n.T("privacy.caches"), len(audit.BrowserCaches))
			if len(audit.BrowserCaches) == 0 {
				r.Check(ui.MarkOK, i18n.T("privacy.no_caches"), 0, "")
				return nil
			}
			var total int64
			var rows [][]string
			for _, b := range audit.BrowserCaches {
				size := b.SizeMB << 20
				total += size
				rows = append(rows, []string{b.Browser, ui.Bytes(size), r.Muted(tildePath(b.Path))})
			}
			r.Table([]ui.Column{{Title: i18n.T("privacy.col_browser")}, {Title: i18n.T("disk.col_size"), Right: true},
				{Title: i18n.T("disk.col_path"), Wrap: true}}, rows, nil)
			r.Blank()
			r.Line(i18n.T("privacy.total", ui.Bytes(total)))
			if !clean {
				r.Hint(i18n.T("privacy.hint"))
				return nil
			}
			if !yes {
				if !stdinIsTerminal() {
					return errors.New(i18n.T("privacy.need_yes"))
				}
				if !askYesNo(i18n.T("privacy.confirm")) {
					fmt.Fprintln(w, r.Glyph(ui.MarkSkip)+" "+i18n.T("tools.nothing_done"))
					return nil
				}
			}
			failed := 0
			for _, b := range audit.BrowserCaches {
				// A browser that is open keeps files in use: say what
				// was not cleared instead of a tick.
				err := os.RemoveAll(b.Path)
				if err == nil {
					err = os.MkdirAll(b.Path, 0o755)
				}
				if err != nil {
					failed++
					r.Check(ui.MarkWarn, b.Browser, 0, i18n.T("privacy.not_cleared", err))
					continue
				}
				r.Check(ui.MarkOK, b.Browser, 0, i18n.T("privacy.cleared"))
			}
			if failed > 0 {
				return fmt.Errorf("%s", i18n.T("privacy.failed", failed, len(audit.BrowserCaches)))
			}
			return nil
		},
	}
	c.Flags().BoolVar(&clean, "clean", false, i18n.T("flag.privacy_clean"))
	c.Flags().BoolVarP(&yes, "yes", "y", false, i18n.T("flag.tools_yes"))
	c.Flags().BoolVar(&asJSON, "json", false, i18n.T("flag.tools_json"))
	return c
}
