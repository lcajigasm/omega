package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/ui"
)

// notesCompactCmd is `omega notes compact`: without any model, it finds
// the notes that say the same as a newer one and removes them, after
// showing which and asking. The old file stays beside it as notes.md.bak.
func notesCompactCmd() *cobra.Command {
	var yes, dryRun bool
	c := &cobra.Command{
		Use:     "compact",
		Short:   i18n.T("cmd.notes_compact"),
		Long:    i18n.T("cmd.notes_compact_long"),
		Args:    cobra.NoArgs,
		Example: "  omega notes compact\n  omega notes compact --dry-run\n  omega notes compact --yes",
		RunE: func(cmd *cobra.Command, args []string) error {
			return compactNotes(cmd.OutOrStdout(), config.NotesPath(), yes, dryRun, stdinIsTerminal(), askYesNo)
		},
	}
	c.Flags().BoolVarP(&yes, "yes", "y", false, i18n.T("flag.compact_yes"))
	c.Flags().BoolVarP(&dryRun, "dry-run", "n", false, i18n.T("flag.compact_dry"))
	return c
}

// compactNotes removes the repeated notes of the file at path.
func compactNotes(w io.Writer, path string, yes, dryRun, tty bool, confirm func(string) bool) error {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) || (err == nil && len(b) == 0) {
		writeEmpty(w, "omega notes compact", tildePath(path), i18n.T("notes.empty"), i18n.T("notes.empty_hint"))
		return nil
	}
	if err != nil {
		return err
	}
	lines := strings.SplitAfter(string(b), "\n")
	drop := repeatedNotes(lines)
	r := ui.NewReport(w)
	r.Title("omega notes compact", tildePath(path))
	r.Blank()
	if len(drop) == 0 {
		r.Line(r.Glyph(ui.MarkOK) + " " + i18n.T("notes.no_repeats"))
		return nil
	}
	for i, l := range lines {
		if keep, gone := drop[i]; gone {
			fmt.Fprintln(w, "  "+r.Err("−")+" "+r.Clip(strings.TrimSpace(l), 4))
			fmt.Fprintln(w, "    "+r.Faint(i18n.T("notes.same_as", keep+1)))
		}
	}
	r.Blank()
	r.Line(i18n.N("notes.would_remove", len(drop), len(drop), countNotes(lines)))
	if dryRun {
		return nil
	}
	if !yes {
		if !tty {
			return usageErr(nil, i18n.T("notes.compact_need_yes"))
		}
		if !confirm(i18n.T("notes.compact_ask")) {
			return nil
		}
	}
	var kept strings.Builder
	for i, l := range lines {
		if _, gone := drop[i]; !gone {
			kept.WriteString(l)
		}
	}
	if err := os.WriteFile(path+".bak", b, 0o600); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(kept.String()), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	fmt.Fprintln(w, r.Glyph(ui.MarkOK)+" "+i18n.N("notes.compacted", len(drop), len(drop), tildePath(path+".bak")))
	return nil
}

func countNotes(lines []string) int {
	n := 0
	for _, l := range lines {
		if noteKey(l) != "" {
			n++
		}
	}
	return n
}

// repeatedNotes finds the notes that repeat a newer one: the same words,
// once dates, case, punctuation and spacing are left aside, or nearly all
// the same words in a long note. It maps the line of each older repeat to
// the line of the note that stays. The newest wording is the one kept.
func repeatedNotes(lines []string) map[int]int {
	drop := map[int]int{}
	type kept struct {
		line  int
		key   string
		words map[string]bool
	}
	var newer []kept
	for i := len(lines) - 1; i >= 0; i-- {
		key := noteKey(lines[i])
		if key == "" {
			continue
		}
		words := wordSet(key)
		same := -1
		for _, k := range newer {
			if k.key == key || (len(words) >= 8 && jaccard(words, k.words) >= 0.9) {
				same = k.line
				break
			}
		}
		if same >= 0 {
			drop[i] = same
			continue
		}
		newer = append(newer, kept{i, key, words})
	}
	return drop
}

var (
	noteDates = regexp.MustCompile(`\d{4}-\d{2}-\d{2}(\s*\(\d+\))?`)
	noteSpace = regexp.MustCompile(`\s+`)
)

// noteKey is the text of a note line that two repeats share: no list mark,
// no dates, lower case, letters, digits and single spaces. A line that is
// not a note has none.
func noteKey(line string) string {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, "-") {
		return ""
	}
	t = noteDates.ReplaceAllString(strings.TrimPrefix(t, "-"), " ")
	t = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return unicode.ToLower(r)
		}
		return ' '
	}, t)
	return strings.TrimSpace(noteSpace.ReplaceAllString(t, " "))
}

func wordSet(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.Fields(s) {
		out[w] = true
	}
	return out
}

func jaccard(a, b map[string]bool) float64 {
	both := 0
	for w := range a {
		if b[w] {
			both++
		}
	}
	all := len(a) + len(b) - both
	if all == 0 {
		return 0
	}
	return float64(both) / float64(all)
}
