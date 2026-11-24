package cli

import (
	"context"
	"io"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/runner"
	"github.com/lcajigasm/omega/internal/ui"
)

// snapshotList is what omega rollback found: the restore points of this
// system and how to go back to one.
type snapshotList struct {
	Platform  string   `json:"platform"`
	Tool      string   `json:"tool,omitempty"` // what listed them
	Snapshots []string `json:"snapshots"`
	Restore   string   `json:"restore"` // how to restore one
	Error     string   `json:"error,omitempty"`
}

// rollbackRun runs a read-only listing command. Tests replace it.
var rollbackRun = func(ctx context.Context, command string) runner.Result {
	return runner.Run(ctx, command, 10*time.Second, nil)
}

var rollbackHas = runner.Has

// listSnapshots reads the restore points: APFS local snapshots on a Mac,
// snapper's on Linux, System Restore points on Windows. It never waits for
// a password: snapper is asked with sudo -n.
func listSnapshots(ctx context.Context, goos string) snapshotList {
	l := snapshotList{Platform: goos, Snapshots: []string{}}
	keep := func(out string, skip func(string) bool) {
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !skip(line) {
				l.Snapshots = append(l.Snapshots, line)
			}
		}
	}
	switch goos {
	case "darwin":
		l.Tool, l.Restore = "tmutil", i18n.T("rollback.restore_mac")
		res := rollbackRun(ctx, "tmutil listlocalsnapshots /")
		if res.ExitCode != 0 {
			l.Error = strings.TrimSpace(res.Output)
			return l
		}
		keep(res.Output, func(s string) bool { return strings.HasPrefix(s, "Snapshots for") })
	case "windows":
		l.Tool, l.Restore = "Get-ComputerRestorePoint", i18n.T("rollback.restore_windows")
		res := rollbackRun(ctx, "Get-ComputerRestorePoint | ForEach-Object { '{0}  {1}  {2}' -f $_.SequenceNumber, $_.ConvertToDateTime($_.CreationTime), $_.Description }")
		if res.ExitCode != 0 {
			l.Error = strings.TrimSpace(res.Output)
			return l
		}
		keep(res.Output, func(string) bool { return false })
	default:
		if !rollbackHas("snapper") {
			l.Restore = i18n.T("rollback.no_tool")
			return l
		}
		l.Tool, l.Restore = "snapper", i18n.T("rollback.restore_snapper")
		res := rollbackRun(ctx, "snapper --no-headers list 2>&1 || sudo -n snapper --no-headers list 2>&1")
		if res.ExitCode != 0 {
			l.Error = i18n.T("rollback.snapper_sudo")
			return l
		}
		keep(res.Output, func(s string) bool { return strings.HasPrefix(s, "0 ") || strings.HasPrefix(s, "─") })
	}
	return l
}

func rollbackCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:     "rollback",
		Short:   i18n.T("cmd.rollback"),
		Long:    i18n.T("cmd.rollback_long"),
		Example: "  omega rollback\n  omega rollback --json",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			l := listSnapshots(cmd.Context(), runtime.GOOS)
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), l)
			}
			writeSnapshots(cmd.OutOrStdout(), l)
			if l.Error != "" {
				return exitCode(1)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, i18n.T("flag.tools_json"))
	return c
}

func writeSnapshots(w io.Writer, l snapshotList) {
	r := ui.NewReport(w)
	r.Title("omega rollback", l.Tool)
	r.Blank()
	switch {
	case l.Error != "":
		r.Check(ui.MarkErr, i18n.T("rollback.unreadable"), 0, l.Error)
	case len(l.Snapshots) == 0:
		r.Check(ui.MarkOff, i18n.T("rollback.none"), 0, "")
	default:
		r.Section(i18n.T("rollback.section"), len(l.Snapshots))
		for _, s := range l.Snapshots {
			r.Line(s)
		}
	}
	r.Blank()
	if l.Restore != "" {
		r.Hint(l.Restore)
	}
}
