package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/pending"
)

// pendingCmd lists what earlier runs left for the user to do by hand, and
// lets the user say "I know" (dismiss), "not now" (snooze) or "show it again"
// (reopen). Runs then stop listing the same things every time.
func pendingCmd() *cobra.Command {
	c := &cobra.Command{
		Use:     "pending",
		Short:   i18n.T("cmd.pending"),
		Args:    cobra.NoArgs,
		Example: "  omega pending\n  omega pending snooze time-machine 2w\n  omega pending dismiss docker-build-cache\n  omega pending reopen time-machine",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return writePending(cmd.OutOrStdout(), time.Now())
		},
	}
	set := func(use, short, status string) *cobra.Command {
		return &cobra.Command{
			Use:   use + " <id>",
			Short: short,
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return setPending(cmd.OutOrStdout(), args[0], status, time.Time{})
			},
		}
	}
	c.AddCommand(set("dismiss", i18n.T("cmd.pending_dismiss"), pending.Dismissed),
		set("reopen", i18n.T("cmd.pending_reopen"), pending.Open),
		set("done", i18n.T("cmd.pending_done"), pending.Done))
	c.AddCommand(&cobra.Command{
		Use:   "snooze <id> <period>",
		Short: i18n.T("cmd.pending_snooze"),
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := pending.ParseFor(args[1])
			if err != nil {
				return err
			}
			return setPending(cmd.OutOrStdout(), args[0], pending.Snoozed, time.Now().Add(d))
		},
	})
	return c
}

func writePending(w io.Writer, now time.Time) error {
	items, err := pending.Load()
	if err != nil {
		return err
	}
	if len(items) == 0 {
		fmt.Fprintln(w, i18n.T("pending.empty"))
		return nil
	}
	for _, it := range items {
		status := it.Status
		if it.Status == pending.Snoozed {
			if it.Active(now) {
				status = pending.Open
			} else {
				status += " → " + it.Until.Format("2006-01-02")
			}
		}
		fmt.Fprintf(w, "%-28s %-22s %s\n", it.ID, status, it.Text)
		fmt.Fprintf(w, "%-28s %-22s %s\n", "", "", i18n.T("pending.seen", it.FirstSeen.Format("2006-01-02"), it.Seen))
	}
	return nil
}

func setPending(w io.Writer, id, status string, until time.Time) error {
	items, err := pending.Load()
	if err != nil {
		return err
	}
	if items, err = pending.Set(items, pending.CleanID(id), status, until); err != nil {
		return err
	}
	if err := pending.Save(items); err != nil {
		return err
	}
	fmt.Fprintln(w, i18n.T("pending.set", pending.CleanID(id), status))
	return nil
}
