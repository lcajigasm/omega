package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/secscan"
	"github.com/lcajigasm/omega/internal/ui"
)

func auditCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "audit",
		Short: i18n.T("cmd.audit"),
		Long:  i18n.T("cmd.audit_long"),
		Example: "  omega audit ssh\n" +
			"  omega audit secrets\n" +
			"  omega audit secrets --json --no-fail",
	}

	var asJSON, noFail bool
	ssh := &cobra.Command{
		Use:   "ssh",
		Short: i18n.T("cmd.audit_ssh"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			findings, err := secscan.AuditSSH()
			if err != nil {
				return err
			}
			if asJSON {
				if findings == nil {
					findings = []secscan.SSHFinding{}
				}
				return writeJSON(cmd.OutOrStdout(), findings)
			}
			writeSSHAudit(cmd.OutOrStdout(), findings)
			return nil
		},
	}
	ssh.Flags().BoolVar(&asJSON, "json", false, i18n.T("flag.tools_json"))

	secrets := &cobra.Command{
		Use:   "secrets",
		Short: i18n.T("cmd.audit_secrets"),
		Long:  i18n.T("cmd.audit_secrets_long"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			scan, err := secscan.ScanHistories()
			if err != nil {
				return err
			}
			if asJSON {
				if err := writeJSON(cmd.OutOrStdout(), scan); err != nil {
					return err
				}
			} else {
				writeSecrets(cmd.OutOrStdout(), scan)
			}
			// Found secrets are a failure scripts can see.
			if len(scan.Findings) > 0 && !noFail {
				return exitCode(1)
			}
			return nil
		},
	}
	secrets.Flags().BoolVar(&asJSON, "json", false, i18n.T("flag.tools_json"))
	secrets.Flags().BoolVar(&noFail, "no-fail", false, i18n.T("flag.audit_no_fail"))

	c.AddCommand(ssh, secrets)
	return c
}

func writeSSHAudit(w io.Writer, findings []secscan.SSHFinding) {
	r := ui.NewReport(w)
	r.Title("omega audit ssh", "~/.ssh")
	r.Blank()
	if len(findings) == 0 {
		r.Check(ui.MarkOK, i18n.T("audit.ssh_ok"), 0, "")
		return
	}
	for _, f := range findings {
		mark := ui.MarkWarn
		if f.Severity == "high" {
			mark = ui.MarkErr
		}
		r.Check(mark, tildePath(f.Path), 0, f.Issue)
		if f.Fix != "" {
			r.Detail(i18n.T("audit.fix", f.Fix))
		}
	}
	r.Blank()
}

func writeSecrets(w io.Writer, scan secscan.SecretScan) {
	r := ui.NewReport(w)
	r.Title("omega audit secrets", i18n.T("audit.secrets_detail", len(scan.Scanned)))
	r.Blank()
	if len(scan.Scanned) == 0 {
		r.Line(i18n.T("audit.no_history"))
		return
	}
	for _, f := range scan.Scanned {
		r.Line(r.Muted(tildePath(f)))
	}
	r.Blank()
	if len(scan.Findings) == 0 {
		r.Check(ui.MarkOK, i18n.T("audit.secrets_ok"), 0, "")
		return
	}
	r.Section(i18n.T("audit.secrets_found"), len(scan.Findings))
	for _, f := range scan.Findings {
		where := tildePath(f.File)
		if f.LineNum > 0 {
			where = fmt.Sprintf("%s:%d", where, f.LineNum)
		}
		r.Check(ui.MarkErr, f.Type, 0, where)
		r.Detail(f.Snippet)
	}
	r.Blank()
	r.Hint(i18n.T("audit.secrets_hint"))
}
