package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/assist"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/ui"
)

// The configuration assistants and the disk limits: what omega checks in the
// configuration of Claude, ChatGPT/Codex, Grok, Docker, Cursor and the other
// tools people use every day, and what it can set there. The desktop app
// shows the same through the functions at the end of the file.

var assistScan = assist.Scan

func assistCmd() *cobra.Command {
	var asJSON, yes, dry bool
	c := &cobra.Command{
		Use:     "assist [tool]",
		Aliases: []string{"assistant", "assistants", "asistente", "asistentes"},
		Short:   i18n.T("cmd.assist"),
		Long:    i18n.T("cmd.assist_long", strings.Join(assist.IDs(), ", "), assist.BackupDir()),
		Example: "  omega assist\n  omega assist claude-code\n  omega assist docker --yes\n  omega assist apply <check> 14",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := assistScan()
			w := cmd.OutOrStdout()
			if len(args) == 0 {
				if asJSON {
					return writeIndented(w, r)
				}
				writeTools(w, r)
				return nil
			}
			t, ok := r.Find(args[0])
			if !ok {
				return fmt.Errorf("%s", i18n.T("assist.no_tool", args[0], strings.Join(assist.IDs(), ", ")))
			}
			if asJSON {
				return writeIndented(w, t)
			}
			return walkTool(w, os.Stdin, t, yes, dry)
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, i18n.T("flag.tools_json"))
	c.Flags().BoolVarP(&yes, "yes", "y", false, i18n.T("flag.assist_yes"))
	c.Flags().BoolVarP(&dry, "dry-run", "n", false, i18n.T("flag.assist_dry"))

	var applyYes bool
	apply := &cobra.Command{
		Use:   "apply <check> [value]",
		Short: i18n.T("cmd.assist_apply"),
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := assistScan()
			ch, ok := r.Check(args[0])
			if !ok {
				return fmt.Errorf("%s", i18n.T("assist.no_check", args[0]))
			}
			value := ""
			if len(args) == 2 {
				value = args[1]
			}
			return applyOne(cmd.OutOrStdout(), ch, value, applyYes)
		},
	}
	apply.Flags().BoolVarP(&applyYes, "yes", "y", false, i18n.T("flag.tools_yes"))
	c.AddCommand(apply)
	return c
}

func writeIndented(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func writeTools(w io.Writer, r assist.Report) {
	rep := ui.NewReport(w)
	rep.Title("omega assist", i18n.T("assist.title"))
	rep.Blank()
	var missing []string
	var rows [][]string
	var marks []ui.Mark
	for _, t := range r.Tools {
		if !t.Installed {
			missing = append(missing, t.Name)
			continue
		}
		mark, state := ui.MarkOK, rep.Muted(i18n.T("assist.nothing_pending"))
		if t.Pending > 0 {
			mark, state = ui.MarkWarn, rep.Warn(i18n.T("assist.to_review", t.Pending))
		}
		var extra []string
		for _, e := range []string{t.Version, t.DataText} {
			if e != "" {
				extra = append(extra, e)
			}
		}
		if t.Running {
			extra = append(extra, i18n.T("assist.open"))
		}
		rows = append(rows, []string{t.ID, t.Name, state, rep.Muted(strings.Join(extra, " · "))})
		marks = append(marks, mark)
	}
	rep.Table([]ui.Column{{}, {}, {}, {Wrap: true}}, rows, marks)
	if len(missing) > 0 {
		rep.Blank()
		rep.Line(rep.Glyph(ui.MarkOff) + " " + rep.Muted(i18n.T("assist.not_found", strings.Join(missing, ", "))))
	}
	rep.Blank()
	rep.Hint(i18n.T("assist.hint"))
}

// checkLine is one check of a tool: its title and id, then what it is now,
// why it matters and where it is set, under it.
func checkLine(r *ui.Report, c assist.Check) {
	mark := ui.MarkOK
	switch {
	case c.Kind == assist.KindInfo:
		mark = ui.MarkOff
	case !c.OK:
		mark = ui.MarkWarn
	}
	r.Blank()
	r.Check(mark, c.Title, 0, r.Faint(c.ID))
	facts := []string{i18n.T("assist.now", c.Current)}
	if c.SizeText != "" && !strings.Contains(c.Current, c.SizeText) {
		facts = append(facts, i18n.T("assist.size", c.SizeText))
	}
	if c.Recommended != "" && !c.OK && c.Kind != assist.KindAction {
		facts = append(facts, strings.TrimSpace(i18n.T("assist.recommended", c.Recommended+" "+unitWord(c))))
	}
	r.Detail(strings.Join(facts, " · "))
	r.Detail(c.Why)
	if c.File != "" {
		r.Detail(i18n.T("assist.file", tildePath(c.File)))
	}
	if c.Note != "" {
		r.Detail(c.Note)
	}
	if c.Blocked != "" {
		r.Line("  " + r.Glyph(ui.MarkWarn) + " " + r.Warn(c.Blocked))
	}
}

func unitWord(c assist.Check) string {
	if c.Unit == "days" {
		return i18n.T("assist.days")
	}
	return ""
}

// walkTool shows a tool's checks and, on a terminal, asks about each one
// with something to do.
func walkTool(w io.Writer, in io.Reader, t *assist.Tool, yes, dry bool) error {
	r := ui.NewReport(w)
	r.Title("omega assist "+t.ID, strings.TrimSpace(t.Name+" "+t.Version))
	if !t.Installed {
		r.Blank()
		r.Line(r.Glyph(ui.MarkOff) + " " + i18n.T("assist.not_installed"))
		return nil
	}
	r.Blank()
	r.Line(t.About)
	if t.Where != "" {
		where := tildePath(t.Where)
		if t.DataText != "" {
			where += " · " + t.DataText
		}
		r.Line(r.Muted(where))
	}
	for _, c := range t.Checks {
		checkLine(r, c)
	}
	pending := 0
	for _, c := range t.Checks {
		if !c.OK && c.Kind != assist.KindInfo {
			pending++
		}
	}
	if pending == 0 {
		r.Blank()
		fmt.Fprintln(w, r.Glyph(ui.MarkOK)+" "+i18n.T("assist.nothing_to_change"))
		return nil
	}
	if dry {
		return nil
	}
	interactive := !yes && stdinIsTerminal()
	if !yes && !interactive {
		r.Blank()
		r.Hint(i18n.T("assist.need_yes_walk"))
		return nil
	}
	fmt.Fprintln(w)
	reader := bufio.NewReader(in)
	var errs []error
	for _, c := range t.Checks {
		if c.OK || c.Kind == assist.KindInfo || c.Blocked != "" {
			continue
		}
		// A budget is the user's number: never one omega picks.
		if c.Kind == assist.KindBudget {
			if !interactive {
				continue
			}
			fmt.Fprint(w, i18n.T("assist.ask_budget", c.Title))
			line, _ := reader.ReadString('\n')
			if v := strings.TrimSpace(line); v != "" {
				report(w, &errs, c.ID, func() (string, error) { return c.Apply(v) })
			}
			continue
		}
		if interactive {
			q := fmt.Sprintf("%s → %s?", c.Title, describeChange(c))
			if !askYesNo(q) {
				continue
			}
		}
		report(w, &errs, c.ID, func() (string, error) { return c.Apply("") })
	}
	return errors.Join(errs...)
}

func describeChange(c assist.Check) string {
	switch c.Kind {
	case assist.KindCleanup:
		return i18n.T("assist.free", humanSize(c.Free))
	case assist.KindAction:
		return i18n.T("assist.apply")
	}
	return i18n.T("assist.set", strings.TrimSpace(c.Recommended+" "+unitWord(c)))
}

func humanSize(n int64) string { return ui.Bytes(n) }

func report(w io.Writer, errs *[]error, id string, f func() (string, error)) {
	msg, err := f()
	if err != nil {
		fmt.Fprintf(w, "  ✗ %s: %v\n", id, err)
		*errs = append(*errs, err)
		return
	}
	fmt.Fprintf(w, "  ✓ %s\n", msg)
}

func applyOne(w io.Writer, c *assist.Check, value string, yes bool) error {
	if c.Blocked != "" {
		return fmt.Errorf("%s: %s", c.ID, c.Blocked)
	}
	if !yes {
		checkLine(ui.NewReport(w), *c)
		if !stdinIsTerminal() {
			return errors.New(i18n.T("assist.need_yes"))
		}
		what := describeChange(*c)
		if value != "" {
			what = i18n.T("assist.set", value)
		}
		if !askYesNo(fmt.Sprintf("\n%s → %s?", c.Title, what)) {
			return nil
		}
	}
	msg, err := c.Apply(value)
	if err != nil {
		return err
	}
	say(w, ui.MarkOK, msg)
	return nil
}

// ---------- omega disk limits ----------

func diskLimitsCmd() *cobra.Command {
	var asJSON bool
	show := func(cmd *cobra.Command, _ []string) error {
		r := assistScan()
		if asJSON {
			return writeIndented(cmd.OutOrStdout(), struct {
				Checks   []assist.Check `json:"checks"`
				Size     int64          `json:"size"`
				Freeable int64          `json:"freeable"`
			}{r.Disk, r.DiskSize, r.Freeable})
		}
		writeLimits(cmd.OutOrStdout(), r)
		return nil
	}
	c := &cobra.Command{
		Use:     "limits",
		Aliases: []string{"limit", "budgets", "limites"},
		Short:   i18n.T("cmd.disk_limits"),
		Long:    i18n.T("cmd.disk_limits_long", assist.BackupDir()),
		Example: "  omega disk limits\n  omega disk limits set <check> 10GB\n  omega disk limits apply --dry-run\n  omega disk limits enforce",
		Args:    cobra.NoArgs,
		RunE:    show,
	}
	c.Flags().BoolVar(&asJSON, "json", false, i18n.T("flag.tools_json"))

	var yes, dry bool
	set := &cobra.Command{
		Use:   "set <check> <value>",
		Short: i18n.T("cmd.limits_set"),
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := assistScan()
			ch, ok := r.Check(args[0])
			if !ok || !ch.Disk {
				return fmt.Errorf("%s", i18n.T("limits.no_check", args[0]))
			}
			if ch.Kind == assist.KindCleanup || ch.Kind == assist.KindInfo {
				return fmt.Errorf("%s", i18n.T("limits.no_limit_"+ch.Kind, ch.ID))
			}
			msg, err := ch.Apply(args[1])
			if err != nil {
				return err
			}
			say(cmd.OutOrStdout(), ui.MarkOK, msg)
			return nil
		},
	}
	apply := &cobra.Command{
		Use:   "apply",
		Short: i18n.T("cmd.limits_apply"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			r := assistScan()
			w := cmd.OutOrStdout()
			var todo []assist.Check
			for _, ch := range r.Disk {
				if ch.Kind == assist.KindSetting && !ch.OK && ch.Blocked == "" && ch.Recommended != "" {
					todo = append(todo, ch)
				}
			}
			if len(todo) == 0 {
				say(w, ui.MarkOK, i18n.T("limits.all_set"))
				return nil
			}
			for _, ch := range todo {
				fmt.Fprintf(w, "  %-26s %s → %s\n", ch.ID, ch.Current, ch.Recommended)
			}
			if dry {
				return nil
			}
			if !yes {
				if !stdinIsTerminal() {
					return errors.New(i18n.T("assist.need_yes"))
				}
				if !askYesNo("\n" + i18n.N("limits.ask_write", len(todo))) {
					return nil
				}
			}
			var errs []error
			for _, ch := range todo {
				report(w, &errs, ch.ID, func() (string, error) { return ch.Apply("") })
			}
			return errors.Join(errs...)
		},
	}
	enforce := &cobra.Command{
		Use:   "enforce",
		Short: i18n.T("cmd.limits_enforce"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			plan, err := assist.Enforce(true)
			if err != nil {
				return err
			}
			if len(plan) == 0 {
				say(w, ui.MarkOK, i18n.T("limits.none_over"))
				return nil
			}
			for _, l := range plan {
				fmt.Fprintf(w, "  %s\n", l)
			}
			if dry {
				return nil
			}
			if !yes {
				if !stdinIsTerminal() {
					return errors.New(i18n.T("limits.need_yes"))
				}
				if !askYesNo("\n" + i18n.T("limits.ask_free")) {
					return nil
				}
			}
			done, err := assist.Enforce(false)
			for _, l := range done {
				fmt.Fprintf(w, "  ✓ %s\n", l)
			}
			return err
		},
	}
	for _, sub := range []*cobra.Command{apply, enforce} {
		sub.Flags().BoolVarP(&yes, "yes", "y", false, i18n.T("flag.tools_yes"))
		sub.Flags().BoolVarP(&dry, "dry-run", "n", false, i18n.T("flag.tools_dry"))
	}
	c.AddCommand(set, apply, enforce)
	return c
}

func writeLimits(w io.Writer, r assist.Report) {
	rep := ui.NewReport(w)
	rep.Title("omega disk limits", "")
	rep.Blank()
	var rows [][]string
	var marks []ui.Mark
	for _, c := range r.Disk {
		rec, mark := "", ui.MarkWarn
		switch {
		case c.Kind == assist.KindInfo:
			rec, mark = i18n.T("limits.measured"), ui.MarkOff
		case c.Kind == assist.KindCleanup && c.Free > 0 && !c.OK:
			rec = i18n.T("assist.free", ui.Bytes(c.Free))
		case c.Kind == assist.KindBudget && c.Free > 0:
			rec = i18n.T("limits.over_budget", ui.Bytes(c.Free))
		case c.Kind == assist.KindBudget && c.OK && strings.HasPrefix(c.Current, "no budget"):
			rec, mark = i18n.T("limits.set_budget"), ui.MarkOff
		case !c.OK:
			rec = c.Recommended + " " + unitWord(c)
		default:
			mark = ui.MarkOK
		}
		if c.Blocked != "" && !c.OK {
			rec += " (" + i18n.T("limits.app_open") + ")"
		}
		if mark == ui.MarkWarn {
			rec = rep.Warn(strings.TrimSpace(rec))
		} else {
			rec = rep.Muted(strings.TrimSpace(rec))
		}
		rows = append(rows, []string{c.ID, c.SizeText, cut(c.Current, 30), rec})
		marks = append(marks, mark)
	}
	rep.Table([]ui.Column{{Title: strings.ToUpper(i18n.T("limits.col_check"))}, {Title: strings.ToUpper(i18n.T("disk.col_size")), Right: true},
		{Title: strings.ToUpper(i18n.T("limits.col_now"))}, {Title: strings.ToUpper(i18n.T("limits.col_rec")), Wrap: true}}, rows, marks)
	rep.Blank()
	rep.Line(rep.Muted(i18n.T("limits.summary", r.DiskText, r.FreeText)))
	rep.Blank()
	rep.Hint(i18n.T("limits.hint"))
}

// ---------- the desktop app ----------

// AssistReport is what the app's Configure view shows.
func AssistReport() assist.Report { return assistScan() }

// AssistApply applies one check from the app: value "" is the recommended
// one, "now" empties a budget's folder at once.
func AssistApply(id, value string) (string, error) { return assist.Apply(id, value) }

// DiskEnforce frees what is over its limit, or with dry only says it.
func DiskEnforce(dry bool) ([]string, error) { return assist.Enforce(dry) }
