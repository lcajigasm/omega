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
		Long: "Checks the configuration of the AI apps and developer tools on this machine and sets what is\n" +
			"worth setting in their own files: how long they keep conversations, how much cache and log they\n" +
			"may keep, keys out of the agents' reach, and omega's MCP server so their agents can use omega.\n\n" +
			"  omega assist                     every known tool: installed, version, what is pending\n" +
			"  omega assist claude-code         walk through one tool's checks and apply what you accept\n" +
			"  omega assist docker --yes        apply every recommendation of a tool without asking\n" +
			"  omega assist apply <check> [v]   apply one check, with its recommended value or yours\n\n" +
			"Tools: " + strings.Join(assist.IDs(), ", ") + ".\n" +
			"A file is copied to " + assist.BackupDir() + " before it is changed.\n" +
			"The disk side of the same checks is `omega disk limits`. Nothing leaves your machine.",
		Args: cobra.MaximumNArgs(1),
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
				return fmt.Errorf("no assistant for %q: omega knows %s", args[0], strings.Join(assist.IDs(), ", "))
			}
			if asJSON {
				return writeIndented(w, t)
			}
			return walkTool(w, os.Stdin, t, yes, dry)
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "print the report as JSON")
	c.Flags().BoolVarP(&yes, "yes", "y", false, "apply every recommendation without asking")
	c.Flags().BoolVarP(&dry, "dry-run", "n", false, "show the checks and change nothing")

	var applyYes bool
	apply := &cobra.Command{
		Use:   "apply <check> [value]",
		Short: "Apply one check: its recommended value, or the value given",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := assistScan()
			ch, ok := r.Check(args[0])
			if !ok {
				return fmt.Errorf("no check %q on this machine: `omega assist <tool>` lists them", args[0])
			}
			value := ""
			if len(args) == 2 {
				value = args[1]
			}
			return applyOne(cmd.OutOrStdout(), ch, value, applyYes)
		},
	}
	apply.Flags().BoolVarP(&applyYes, "yes", "y", false, "do not ask")
	c.AddCommand(apply)
	return c
}

func writeIndented(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func writeTools(w io.Writer, r assist.Report) {
	fmt.Fprintln(w, "\nCONFIGURATION ASSISTANTS")
	var missing []string
	for _, t := range r.Tools {
		if !t.Installed {
			missing = append(missing, t.Name)
			continue
		}
		state := "✓ nothing pending"
		if t.Pending > 0 {
			state = fmt.Sprintf("● %d to review", t.Pending)
		}
		extra := t.Version
		if t.DataText != "" {
			extra = strings.TrimSpace(extra + "  " + t.DataText)
		}
		if t.Running {
			extra += "  (open)"
		}
		fmt.Fprintf(w, "  %-16s %-18s %-20s %s\n", t.ID, t.Name, state, extra)
	}
	if len(missing) > 0 {
		fmt.Fprintf(w, "\n  Not found here: %s.\n", strings.Join(missing, ", "))
	}
	fmt.Fprintln(w, "\n  omega assist <tool> walks through one; omega disk limits shows what each may take.")
}

func checkLine(w io.Writer, c assist.Check) {
	mark := "✓"
	switch {
	case c.Kind == assist.KindInfo:
		mark = "·"
	case !c.OK:
		mark = "●"
	}
	fmt.Fprintf(w, "\n  %s %s  [%s]\n", mark, c.Title, c.ID)
	fmt.Fprintf(w, "      now: %s", c.Current)
	if c.SizeText != "" && !strings.Contains(c.Current, c.SizeText) {
		fmt.Fprintf(w, "   size: %s", c.SizeText)
	}
	if c.Recommended != "" && !c.OK && c.Kind != assist.KindAction {
		fmt.Fprintf(w, "   recommended: %s %s", c.Recommended, strings.TrimSpace(unitWord(c)))
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "      %s\n", c.Why)
	if c.File != "" {
		fmt.Fprintf(w, "      file: %s\n", c.File)
	}
	if c.Note != "" {
		fmt.Fprintf(w, "      %s\n", c.Note)
	}
	if c.Blocked != "" {
		fmt.Fprintf(w, "      ⚠ %s\n", c.Blocked)
	}
}

func unitWord(c assist.Check) string {
	if c.Unit == "days" {
		return "days"
	}
	return ""
}

// walkTool shows a tool's checks and, on a terminal, asks about each one
// with something to do.
func walkTool(w io.Writer, in io.Reader, t *assist.Tool, yes, dry bool) error {
	fmt.Fprintf(w, "\n%s", strings.ToUpper(t.Name))
	if t.Version != "" {
		fmt.Fprintf(w, "  %s", t.Version)
	}
	fmt.Fprintln(w)
	if !t.Installed {
		fmt.Fprintln(w, "  Not installed on this machine.")
		return nil
	}
	fmt.Fprintf(w, "  %s\n", t.About)
	if t.Where != "" {
		fmt.Fprintf(w, "  %s", t.Where)
		if t.DataText != "" {
			fmt.Fprintf(w, " (%s)", t.DataText)
		}
		fmt.Fprintln(w)
	}
	for _, c := range t.Checks {
		checkLine(w, c)
	}
	pending := 0
	for _, c := range t.Checks {
		if !c.OK && c.Kind != assist.KindInfo {
			pending++
		}
	}
	if pending == 0 {
		fmt.Fprintln(w, "\n✓ Nothing to change.")
		return nil
	}
	if dry {
		return nil
	}
	interactive := !yes && stdinIsTerminal()
	if !yes && !interactive {
		fmt.Fprintln(w, "\nNothing changed: pass --yes to apply the recommendations, or use omega assist apply <check>.")
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
			fmt.Fprintf(w, "%s: budget in GB (empty to skip): ", c.Title)
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
		return "free " + humanSize(c.Free)
	case assist.KindAction:
		return "apply"
	}
	return "set " + c.Recommended + " " + unitWord(c)
}

func humanSize(n int64) string { return assist.Human(n) }

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
		checkLine(w, *c)
		if !stdinIsTerminal() {
			return errors.New("nothing changed: pass --yes to apply without a terminal")
		}
		what := describeChange(*c)
		if value != "" {
			what = "set " + value
		}
		if !askYesNo(fmt.Sprintf("\n%s → %s?", c.Title, what)) {
			return nil
		}
	}
	msg, err := c.Apply(value)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "✓ %s\n", msg)
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
		Long: "Lists what the AI apps and developer tools take on disk and how each is limited. Where the tool\n" +
			"has a setting for it (Claude Code's conversations, Docker's build cache, logs and disk image,\n" +
			"Codex's history, Homebrew's downloads) omega writes it in the tool's own file. Where it has none\n" +
			"(Claude Desktop's VM image, app caches, Xcode's DerivedData, session recordings) you set a budget\n" +
			"and `omega disk limits enforce` empties the folder once it holds more, never while the app is open.\n\n" +
			"  omega disk limits                    what each tool takes and its limit\n" +
			"  omega disk limits apply              write every recommended setting (asks first)\n" +
			"  omega disk limits set <check> <v>    set one: 14 (days), 10GB, 20MB; a budget: 8GB or off\n" +
			"  omega disk limits enforce            clean up old versions and empty what is over budget\n\n" +
			"A file is copied to " + assist.BackupDir() + " before it is changed.",
		Args: cobra.NoArgs,
		RunE: show,
	}
	c.Flags().BoolVar(&asJSON, "json", false, "print the report as JSON")

	var yes, dry bool
	set := &cobra.Command{
		Use:   "set <check> <value>",
		Short: "Set one tool's limit, or the budget omega keeps for a folder",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := assistScan()
			ch, ok := r.Check(args[0])
			if !ok || !ch.Disk {
				return fmt.Errorf("no disk limit %q here: `omega disk limits` lists them", args[0])
			}
			if ch.Kind == assist.KindCleanup || ch.Kind == assist.KindInfo {
				return fmt.Errorf("%s has no limit to set: it is %s", ch.ID, map[string]string{assist.KindCleanup: "a cleanup (omega disk limits enforce)", assist.KindInfo: "measured only"}[ch.Kind])
			}
			msg, err := ch.Apply(args[1])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ %s\n", msg)
			return nil
		},
	}
	apply := &cobra.Command{
		Use:   "apply",
		Short: "Write every recommended limit in the tools' own configuration files",
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
				fmt.Fprintln(w, "✓ Every tool already has a limit omega can set.")
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
					return errors.New("nothing changed: pass --yes to apply without a terminal")
				}
				if !askYesNo(fmt.Sprintf("\nWrite these %d limits?", len(todo))) {
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
		Short: "Remove old versions and empty the folders over their budget (closed apps only)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			w := cmd.OutOrStdout()
			plan, err := assist.Enforce(true)
			if err != nil {
				return err
			}
			if len(plan) == 0 {
				fmt.Fprintln(w, "✓ Nothing over its limit.")
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
					return errors.New("nothing removed: pass --yes to enforce without a terminal")
				}
				if !askYesNo("\nFree this space?") {
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
		sub.Flags().BoolVarP(&yes, "yes", "y", false, "do not ask")
		sub.Flags().BoolVarP(&dry, "dry-run", "n", false, "show what would change and change nothing")
	}
	c.AddCommand(set, apply, enforce)
	return c
}

func writeLimits(w io.Writer, r assist.Report) {
	fmt.Fprintln(w, "\nDISK LIMITS")
	fmt.Fprintf(w, "  %-28s %9s  %-30s %s\n", "CHECK", "SIZE", "LIMIT NOW", "RECOMMENDED")
	for _, c := range r.Disk {
		rec := ""
		switch {
		case c.Kind == assist.KindInfo:
			rec = "measured only"
		case c.Kind == assist.KindCleanup && c.Free > 0 && !c.OK:
			rec = "free " + assist.Human(c.Free)
		case c.Kind == assist.KindBudget && c.Free > 0:
			rec = "over budget: free " + assist.Human(c.Free)
		case c.Kind == assist.KindBudget && c.OK && strings.HasPrefix(c.Current, "no budget"):
			rec = "set a budget"
		case !c.OK:
			rec = c.Recommended + " " + unitWord(c)
		default:
			rec = "✓"
		}
		if c.Blocked != "" && !c.OK {
			rec += " (app open)"
		}
		cur := c.Current
		if len(cur) > 30 {
			cur = cur[:29] + "…"
		}
		fmt.Fprintf(w, "  %-28s %9s  %-30s %s\n", c.ID, c.SizeText, cur, strings.TrimSpace(rec))
	}
	fmt.Fprintf(w, "\n  These tools take %s; %s can be freed now without losing anything you would miss.\n", r.DiskText, r.FreeText)
	fmt.Fprintln(w, "  omega disk limits apply writes the recommended settings; omega disk limits enforce frees the rest.")
}

// ---------- the desktop app ----------

// AssistReport is what the app's Configure view shows.
func AssistReport() assist.Report { return assistScan() }

// AssistApply applies one check from the app: value "" is the recommended
// one, "now" empties a budget's folder at once.
func AssistApply(id, value string) (string, error) { return assist.Apply(id, value) }

// DiskEnforce frees what is over its limit, or with dry only says it.
func DiskEnforce(dry bool) ([]string, error) { return assist.Enforce(dry) }
