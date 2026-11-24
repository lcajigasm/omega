package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/pending"
	"github.com/lcajigasm/omega/internal/ui"
)

// pendingCmd lists what earlier runs left for the user to do by hand, and
// lets the user say "I know" (dismiss), "not now" (snooze) or "show it again"
// (reopen). Runs then stop listing the same things every time.
func pendingCmd() *cobra.Command {
	var all, asJSON bool
	c := &cobra.Command{
		Use:         "pending",
		Short:       i18n.T("cmd.pending"),
		Long:        i18n.T("cmd.pending_long"),
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"see_also": "omega history, omega logs, omega notes"},
		Example:     "  omega pending\n  omega pending --all\n  omega pending snooze time-machine 2w     # the start of an id is enough\n  omega pending dismiss ssh-controlmaster\n  omega pending reopen time-machine",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if asJSON {
				items, err := pendingItems(time.Now(), all)
				if err != nil {
					return err
				}
				return writeJSON(cmd.OutOrStdout(), items)
			}
			return writePending(cmd.OutOrStdout(), time.Now(), all)
		},
	}
	c.Flags().BoolVar(&all, "all", false, i18n.T("flag.pending_all"))
	c.Flags().BoolVar(&asJSON, "json", false, i18n.T("flag.json"))
	set := func(use, short, status string) *cobra.Command {
		return &cobra.Command{
			Use:               use + " <id>",
			Short:             short,
			Args:              cobra.ExactArgs(1),
			ValidArgsFunction: completePending,
			RunE: func(cmd *cobra.Command, args []string) error {
				return setPending(cmd.OutOrStdout(), args[0], status, time.Time{})
			},
		}
	}
	c.AddCommand(set("dismiss", i18n.T("cmd.pending_dismiss"), pending.Dismissed),
		set("reopen", i18n.T("cmd.pending_reopen"), pending.Open),
		set("done", i18n.T("cmd.pending_done"), pending.Done))
	c.AddCommand(&cobra.Command{
		Use:               "snooze <id> <period>",
		Short:             i18n.T("cmd.pending_snooze"),
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: completePending,
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

// pendingItems are the items `omega pending` shows: the active ones, or
// all of them. Never nil, so the JSON answer is a list.
func pendingItems(now time.Time, all bool) ([]pending.Item, error) {
	items, err := pending.Load()
	if err != nil {
		return nil, err
	}
	out := []pending.Item{}
	for _, it := range items {
		if all || it.Active(now) {
			out = append(out, it)
		}
	}
	return out, nil
}

func writePending(w io.Writer, now time.Time, all bool) error {
	items, err := pending.Load()
	if err != nil {
		return err
	}
	r := ui.NewReport(w)
	var shown []pending.Item
	hidden := 0
	for _, it := range items {
		if all || it.Active(now) {
			shown = append(shown, it)
		} else {
			hidden++
		}
	}
	if len(shown) == 0 {
		fmt.Fprintln(w, i18n.T("pending.empty"))
		if hidden > 0 {
			r.Hint(i18n.N("pending.hidden", hidden))
		}
		return nil
	}
	r.Title("omega pending", i18n.N("pending.count", len(shown)))
	for _, it := range shown {
		mark, status := ui.MarkWarn, i18n.T("pending.st_open")
		switch {
		case it.Status == pending.Snoozed && !it.Active(now):
			mark, status = ui.MarkSkip, i18n.T("pending.st_snoozed", it.Until.Format("2006-01-02"))
		case it.Status == pending.Stale:
			mark, status = ui.MarkSkip, i18n.T("pending.st_stale")
		case it.Status == pending.Dismissed:
			mark, status = ui.MarkSkip, i18n.T("pending.st_dismissed")
		case it.Status == pending.Done:
			mark, status = ui.MarkOK, i18n.T("pending.st_done")
		}
		r.Blank()
		fmt.Fprintln(w, "  "+r.Glyph(mark)+" "+r.Bold(it.ID)+r.Muted("  · "+status+" · "+i18n.T("pending.seen2", it.FirstSeen.Format("2006-01-02"), it.Seen)))
		r.Detail(it.Text)
	}
	r.Blank()
	if hidden > 0 {
		r.Line(r.Muted(i18n.N("pending.hidden", hidden)))
	}
	r.Hint(i18n.T("pending.hint"))
	return nil
}

func setPending(w io.Writer, id, status string, until time.Time) error {
	items, err := pending.Load()
	if err != nil {
		return err
	}
	i, err := pending.Resolve(items, pending.CleanID(id))
	if err != nil {
		return err
	}
	id = items[i].ID
	if items, err = pending.Set(items, id, status, until); err != nil {
		return err
	}
	if err := pending.Save(items); err != nil {
		return err
	}
	r := ui.NewReport(w)
	fmt.Fprintln(w, r.Glyph(ui.MarkOK)+" "+r.Bold(id)+r.Muted(" · "+statusWord(status, until)))
	return nil
}

func statusWord(status string, until time.Time) string {
	switch status {
	case pending.Snoozed:
		return i18n.T("pending.st_snoozed", until.Format("2006-01-02"))
	case pending.Dismissed:
		return i18n.T("pending.st_dismissed")
	case pending.Done:
		return i18n.T("pending.st_done")
	}
	return i18n.T("pending.st_open")
}
