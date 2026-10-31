package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/i18n"
)

func logsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "logs",
		Short: i18n.T("cmd.logs"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return writeLogList(cmd.OutOrStdout())
		},
	}
	c.AddCommand(&cobra.Command{
		Use:   "view [n|file]",
		Short: i18n.T("cmd.logs_view"),
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			return writeLogView(cmd.OutOrStdout(), name)
		},
	})
	return c
}

func writeLogList(w io.Writer) error {
	logs, err := agent.ListRunLogs()
	if err != nil {
		return err
	}
	if len(logs) == 0 {
		fmt.Fprintln(w, i18n.T("logs.empty"))
		return nil
	}
	fmt.Fprintln(w, i18n.T("logs.header", len(logs), agent.RunsDir(), agent.MaxRunLogs))
	for i, l := range logs {
		fmt.Fprintf(w, "  %d\t%s\t%d B\n", i+1, l.Name, l.Size)
	}
	return nil
}

func writeLogView(w io.Writer, name string) error {
	_, b, err := agent.ReadRunLog(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if name == "" {
				return fmt.Errorf("%s", i18n.T("logs.empty"))
			}
			return fmt.Errorf("%s", i18n.T("logs.missing", name))
		}
		return err
	}
	fmt.Fprint(w, string(b))
	return nil
}
