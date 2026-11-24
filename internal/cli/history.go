package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/pending"
	"github.com/lcajigasm/omega/internal/ui"
)

// history is `omega history`: the recent runs, what is still pending and
// the latest notes, on one screen. Each has a command of its own for more.
type history struct {
	Runs    []logEntry     `json:"runs"`
	Pending []pending.Item `json:"pending"`
	Notes   []note         `json:"notes"`
}

func historyCmd() *cobra.Command {
	var n int
	var task, since string
	var asJSON bool
	c := &cobra.Command{
		Use:         "history",
		Short:       i18n.T("cmd.history"),
		Long:        i18n.T("cmd.history_long"),
		Args:        cobra.NoArgs,
		Example:     "  omega history\n  omega history --task clean --since 7d\n  omega history --json",
		Annotations: map[string]string{"see_also": "omega logs, omega pending, omega notes"},
		RunE: func(cmd *cobra.Command, args []string) error {
			from, err := parseSince(since, time.Now())
			if err != nil {
				return usageErr(cmd, err.Error())
			}
			h, err := loadHistory(n, task, from, time.Now())
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), h)
			}
			writeHistory(cmd.OutOrStdout(), h, time.Now())
			return nil
		},
	}
	c.Flags().IntVarP(&n, "last", "n", 5, i18n.T("flag.history_last"))
	c.Flags().StringVar(&task, "task", "", i18n.T("flag.logs_task"))
	c.Flags().StringVar(&since, "since", "", i18n.T("flag.history_since"))
	c.Flags().BoolVar(&asJSON, "json", false, i18n.T("flag.json"))
	return c
}

// parseSince reads --since: a date (2026-11-01) or a span back from now
// (7d, 2w, 12h). Empty is no limit.
func parseSince(v string, now time.Time) (time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, nil
	}
	if t, err := time.ParseInLocation("2006-01-02", v, time.Local); err == nil {
		return t, nil
	}
	if len(v) > 1 {
		if n, err := strconv.Atoi(v[:len(v)-1]); err == nil && n >= 0 {
			switch v[len(v)-1] {
			case 'h':
				return now.Add(-time.Duration(n) * time.Hour), nil
			case 'd':
				return now.AddDate(0, 0, -n), nil
			case 'w':
				return now.AddDate(0, 0, -7*n), nil
			}
		}
	}
	return time.Time{}, errors.New(i18n.T("history.bad_since", v))
}

// loadHistory gathers the runs (at most n, of a task, since a time), the
// active pending items and the notes written since then (at most n).
func loadHistory(n int, task string, since, now time.Time) (history, error) {
	h := history{Runs: []logEntry{}, Pending: []pending.Item{}, Notes: []note{}}
	runs, err := logEntries(logFilter{task: task})
	if err != nil {
		return h, err
	}
	for _, r := range runs {
		if !since.IsZero() && r.When.Before(since) {
			continue
		}
		h.Runs = append(h.Runs, r)
		if n > 0 && len(h.Runs) == n {
			break
		}
	}
	if h.Pending, err = pendingItems(now, false); err != nil {
		return h, err
	}
	b, err := os.ReadFile(config.NotesPath())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return h, err
	}
	var notes []note
	for _, nt := range parseNotes(string(b)) {
		if !since.IsZero() && nt.Day != "" && nt.Day < since.Format("2006-01-02") {
			continue
		}
		notes = append(notes, nt)
	}
	h.Notes = pickNotes(notes, n, "")
	return h, nil
}

func writeHistory(w io.Writer, h history, now time.Time) {
	r := ui.NewReport(w)
	r.Title("omega history", i18n.T("history.subtitle"))

	r.Section(i18n.T("history.runs"), len(h.Runs))
	if len(h.Runs) == 0 {
		r.Line(r.Muted(i18n.T("logs.empty")))
	} else {
		var rows [][]string
		var marks []ui.Mark
		for _, e := range h.Runs {
			mark, state := ui.MarkOK, ""
			switch {
			case !e.Ended:
				mark, state = ui.MarkWarn, r.Warn(i18n.T("logs.unfinished"))
			case e.Failed > 0:
				mark, state = ui.MarkErr, r.Err(i18n.T("logs.failed_tools", e.Failed))
			}
			task := e.Task
			if task == "" {
				task = "—"
			}
			rows = append(rows, []string{fmt.Sprint(e.N), e.When.Format("2006-01-02 15:04"), task, state})
			marks = append(marks, mark)
		}
		r.Table([]ui.Column{{Right: true}, {}, {}, {}}, rows, marks)
	}

	r.Section(i18n.T("history.pending"), len(h.Pending))
	if len(h.Pending) == 0 {
		r.Line(r.Muted(i18n.T("pending.empty")))
	}
	for _, it := range h.Pending {
		fmt.Fprintln(w, "  "+r.Glyph(ui.MarkWarn)+" "+r.Bold(it.ID)+r.Muted(" · "+i18n.T("pending.seen2", it.FirstSeen.Format("2006-01-02"), it.Seen)))
		r.Detail(it.Text)
	}

	r.Section(i18n.T("history.notes"), len(h.Notes))
	if len(h.Notes) == 0 {
		r.Line(r.Muted(i18n.T("notes.empty")))
	}
	for i := len(h.Notes) - 1; i >= 0; i-- {
		nt := h.Notes[i]
		day := ""
		if nt.Day != "" {
			day = r.Faint(nt.Day + "  ")
		}
		fmt.Fprintln(w, "  "+r.Faint("•")+" "+day+r.Clip(nt.Text, 16))
	}
	r.Blank()
	r.Hint(i18n.T("history.hint"))
}
