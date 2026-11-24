package ui

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/term"

	"github.com/lcajigasm/omega/internal/i18n"
)

// Report writes the text output of omega's one-shot commands (doctor, steps,
// logs, pending, models…) with the same look as the full-screen interface:
// section headings in the accent colour, a coloured mark in front of each
// line, columns that line up whatever the width of their text, and long text
// wrapped to the terminal. Colour is only used when w is a terminal that
// takes it: piped or redirected output, NO_COLOR and TERM=dumb get plain
// text, so scripts and tests see stable lines.
type Report struct {
	w     io.Writer
	width int

	accent, muted, faint, ok, err, warn, bold lipgloss.Style
}

// Mark is the state shown in front of a line.
type Mark int

const (
	MarkNone    Mark = iota // two spaces, to keep the column
	MarkOK                  // ✓ done, correct, available
	MarkWarn                // ! works, but needs attention
	MarkErr                 // ✗ failed or missing
	MarkOff                 // · not installed, not applicable
	MarkSkip                // – turned off by the user
	MarkInfo                // › a hint or a next step
	MarkCurrent             // ▸ the one in use
)

// reportWidth is the width used when the output is not a terminal.
const reportWidth = 100

// NewReport writes to w.
func NewReport(w io.Writer) *Report {
	// The standard output shares lipgloss's default renderer, which asks
	// the terminal for its background once and remembers it; a renderer of
	// its own would ask again on every report. Anything else (a file, a
	// buffer in a test) gets one that sees it is not a terminal.
	re := lipgloss.DefaultRenderer()
	if w != io.Writer(os.Stdout) {
		re = lipgloss.NewRenderer(w)
	}
	width := reportWidth
	if f, ok := w.(*os.File); ok {
		if cols, _, err := term.GetSize(int(f.Fd())); err == nil && cols > 0 {
			width = cols
		}
	}
	if width > 120 {
		width = 120
	}
	if width < 40 {
		width = 40
	}
	t := newStyles(re)
	return &Report{
		w: w, width: width,
		accent: t.title, muted: t.muted, faint: t.faint,
		ok: t.ok, err: t.err, warn: t.warn, bold: t.bold,
	}
}

// Width is the number of columns the report wraps to.
func (r *Report) Width() int { return r.width }

// Title opens the output: "Ω  title  · detail".
func (r *Report) Title(title, detail string) {
	s := r.accent.Render("Ω  " + title)
	if detail != "" {
		s += r.muted.Render("  · " + detail)
	}
	fmt.Fprintln(r.w, s)
}

// Section starts a block with a heading. count, when not negative, is shown
// after it in a fainter colour.
func (r *Report) Section(name string, count int) {
	s := r.accent.Render(strings.ToUpper(name))
	if count >= 0 {
		s += " " + r.faint.Render(fmt.Sprint(count))
	}
	fmt.Fprintln(r.w)
	fmt.Fprintln(r.w, s)
}

// Glyph is the coloured mark for m.
func (r *Report) Glyph(m Mark) string {
	switch m {
	case MarkOK:
		return r.ok.Render("✓")
	case MarkWarn:
		return r.warn.Render("!")
	case MarkErr:
		return r.err.Render("✗")
	case MarkOff:
		return r.faint.Render("·")
	case MarkSkip:
		return r.faint.Render("–")
	case MarkInfo:
		return r.accent.Render("›")
	case MarkCurrent:
		return r.accent.Render("▸")
	}
	return " "
}

// Check writes "  ✓ label   value", the label padded to labelWidth so the
// values of consecutive lines line up. value is wrapped under itself.
func (r *Report) Check(m Mark, label string, labelWidth int, value string) {
	head := "  " + r.Glyph(m) + " " + r.bold.Render(pad(label, labelWidth))
	if value == "" {
		fmt.Fprintln(r.w, strings.TrimRight(head, " "))
		return
	}
	indent := 2 + 2 + labelWidth + 2
	lines := wrapText(value, r.width-indent)
	for i, l := range lines {
		if i == 0 {
			fmt.Fprintln(r.w, head+"  "+l)
			continue
		}
		fmt.Fprintln(r.w, strings.Repeat(" ", indent)+l)
	}
}

// Line writes text indented by two spaces, wrapped to the width.
func (r *Report) Line(text string) {
	for _, l := range wrapText(text, r.width-2) {
		fmt.Fprintln(r.w, "  "+l)
	}
}

// Detail writes a fainter, further indented line under the previous one.
func (r *Report) Detail(text string) {
	for _, l := range wrapText(text, r.width-6) {
		fmt.Fprintln(r.w, "    "+r.muted.Render(l))
	}
}

// Hint writes a closing suggestion: "› text".
func (r *Report) Hint(text string) {
	lines := wrapText(text, r.width-4)
	for i, l := range lines {
		if i == 0 {
			fmt.Fprintln(r.w, r.Glyph(MarkInfo)+" "+r.muted.Render(l))
			continue
		}
		fmt.Fprintln(r.w, "  "+r.muted.Render(l))
	}
}

// Blank writes an empty line.
func (r *Report) Blank() { fmt.Fprintln(r.w) }

// Muted, Faint, Bold, OK, Warn and Err colour a span for use inside a line.
func (r *Report) Muted(s string) string { return r.muted.Render(s) }
func (r *Report) Faint(s string) string { return r.faint.Render(s) }
func (r *Report) Bold(s string) string  { return r.bold.Render(s) }
func (r *Report) OK(s string) string    { return r.ok.Render(s) }
func (r *Report) Warn(s string) string  { return r.warn.Render(s) }
func (r *Report) Err(s string) string   { return r.err.Render(s) }
func (r *Report) Accent(s string) string {
	return r.accent.Render(s)
}

// Column describes one column of a Table.
type Column struct {
	Title string
	Right bool // align to the right (numbers, sizes, prices)
	// Wrap lets the column take the width left over and wrap its text. At
	// most one column should wrap; it should be the last.
	Wrap bool
	// Min is the narrowest the column may be, so tables under several
	// headings can line up with each other.
	Min int
}

// Table writes rows under a faint header line, each column as wide as its
// widest cell. Cells may hold colour; widths count what is visible. marks,
// when not nil, puts a mark in front of each row.
func (r *Report) Table(cols []Column, rows [][]string, marks []Mark) {
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = max(c.Min, lipgloss.Width(c.Title))
	}
	for _, row := range rows {
		for i := range cols {
			if i < len(row) && !cols[i].Wrap {
				if w := lipgloss.Width(row[i]); w > widths[i] {
					widths[i] = w
				}
			}
		}
	}
	lead := "  "
	if marks != nil {
		lead = "  " + "  "
	}
	// The wrapping column takes what is left.
	used := len(lead)
	for i, c := range cols {
		if !c.Wrap {
			used += widths[i] + 2
		}
	}
	for i, c := range cols {
		if c.Wrap {
			widths[i] = r.width - used
			if widths[i] < 20 {
				widths[i] = 20
			}
		}
	}
	cell := func(i int, s string) string {
		if cols[i].Wrap {
			return s
		}
		if cols[i].Right {
			return strings.Repeat(" ", widths[i]-lipgloss.Width(s)) + s
		}
		return pad(s, widths[i])
	}
	var head []string
	titled := false
	for i, c := range cols {
		head = append(head, cell(i, c.Title))
		titled = titled || c.Title != ""
	}
	if titled {
		fmt.Fprintln(r.w, lead+r.faint.Render(strings.TrimRight(strings.Join(head, "  "), " ")))
	}
	for n, row := range rows {
		var parts []string
		var wrapped []string
		wrapAt := 0
		for i := range cols {
			v := ""
			if i < len(row) {
				v = row[i]
			}
			if cols[i].Wrap {
				lines := wrapText(v, widths[i])
				v = lines[0]
				wrapped = lines[1:]
				wrapAt = len(lead)
				for j := 0; j < i; j++ {
					wrapAt += widths[j] + 2
				}
			}
			parts = append(parts, cell(i, v))
		}
		prefix := lead
		if marks != nil {
			m := MarkNone
			if n < len(marks) {
				m = marks[n]
			}
			prefix = "  " + r.Glyph(m) + " "
		}
		fmt.Fprintln(r.w, strings.TrimRight(prefix+strings.Join(parts, "  "), " "))
		for _, l := range wrapped {
			fmt.Fprintln(r.w, strings.Repeat(" ", wrapAt)+l)
		}
	}
}

// pad fills s with spaces up to n visible columns.
func pad(s string, n int) string {
	if w := lipgloss.Width(s); w < n {
		return s + strings.Repeat(" ", n-w)
	}
	return s
}

// wrapText breaks plain text into lines of at most width columns, at spaces
// when it can. It always returns at least one line.
func wrapText(s string, width int) []string {
	if width < 10 {
		width = 10
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		if lipgloss.Width(para) <= width {
			out = append(out, para)
			continue
		}
		line := ""
		for _, word := range strings.Fields(para) {
			for lipgloss.Width(word) > width {
				// A word longer than the line (a path, a URL) is cut.
				if line != "" {
					out = append(out, line)
					line = ""
				}
				rs := []rune(word)
				cut := width
				if cut > len(rs) {
					cut = len(rs)
				}
				out = append(out, string(rs[:cut]))
				word = string(rs[cut:])
			}
			switch {
			case word == "":
			case line == "":
				line = word
			case lipgloss.Width(line)+1+lipgloss.Width(word) <= width:
				line += " " + word
			default:
				out = append(out, line)
				line = word
			}
		}
		if line != "" {
			out = append(out, line)
		}
	}
	if len(out) == 0 {
		out = []string{""}
	}
	return out
}

// StepRisk is the risk word of a step for a report, coloured by r.
func (r *Report) StepRisk(risk string) string {
	switch risk {
	case "read-only":
		return r.ok.Render(i18n.T("risk.short_ro"))
	case "dangerous":
		return r.warn.Render(i18n.T("risk.short_danger"))
	}
	return r.muted.Render(i18n.T("risk.short_normal"))
}

// Clip cuts s to what fits on one line after indent columns, ending in "…"
// when something was cut. Newlines become spaces.
func (r *Report) Clip(s string, indent int) string {
	s = strings.Join(strings.Fields(s), " ")
	return ansi.Truncate(s, max(10, r.width-indent), "…")
}
