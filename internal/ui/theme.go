package ui

import (
	"os"

	"github.com/charmbracelet/lipgloss"
)

// The palette: every colour the interface uses, for light and dark
// backgrounds. Nothing else in the package names a colour.
var (
	accent    = lipgloss.AdaptiveColor{Light: "#5A4FCF", Dark: "#9D8CFF"}
	accentDim = lipgloss.AdaptiveColor{Light: "#A49DE6", Dark: "#5B5196"}
	// ultraHue marks omega ultra: a warmer colour than the everyday accent.
	ultraHue  = lipgloss.AdaptiveColor{Light: "#B4410F", Dark: "#FF9B54"}
	bright    = lipgloss.AdaptiveColor{Light: "#1E1B4B", Dark: "#FFFFFF"}
	muted     = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#8B93A1"}
	faint     = lipgloss.AdaptiveColor{Light: "#9CA3AF", Dark: "#5C6370"}
	okColor   = lipgloss.AdaptiveColor{Light: "#15803D", Dark: "#4ADE80"}
	errColor  = lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#F87171"}
	warnColor = lipgloss.AdaptiveColor{Light: "#B45309", Dark: "#FBBF24"}
)

// styles is every text style, built on one renderer: the terminal's for the
// interface, the output's own for a report written to a file or a buffer.
type styles struct {
	title     lipgloss.Style // bold accent: headings, the brand, pointers
	accent    lipgloss.Style // accent without bold: bars, the mascot
	dim       lipgloss.Style // the dimmed accent: a blinking dot, rules
	muted     lipgloss.Style // secondary text, and hints
	faint     lipgloss.Style // decoration only: rules, column labels
	ok        lipgloss.Style
	err       lipgloss.Style
	warn      lipgloss.Style // bold: needs attention
	warnPlain lipgloss.Style // the warning colour without bold
	bold      lipgloss.Style // commands, names, the key of a hint
	hintKey   lipgloss.Style // the key of a hint: bold on the muted colour

	shimBright, shimMid, shimDim lipgloss.Style // the shimmer of the thinking line
}

func newStyles(re *lipgloss.Renderer) styles {
	return styles{
		title:      re.NewStyle().Bold(true).Foreground(accent),
		accent:     re.NewStyle().Foreground(accent),
		dim:        re.NewStyle().Foreground(accentDim),
		muted:      re.NewStyle().Foreground(muted),
		faint:      re.NewStyle().Foreground(faint),
		ok:         re.NewStyle().Foreground(okColor),
		err:        re.NewStyle().Foreground(errColor),
		warn:       re.NewStyle().Foreground(warnColor).Bold(true),
		warnPlain:  re.NewStyle().Foreground(warnColor),
		bold:       re.NewStyle().Bold(true),
		hintKey:    re.NewStyle().Bold(true).Foreground(muted),
		shimBright: re.NewStyle().Bold(true).Foreground(bright),
		shimMid:    re.NewStyle().Foreground(accent),
		shimDim:    re.NewStyle().Foreground(accentDim),
	}
}

// th is the interface's set, on the terminal's renderer.
var th = newStyles(lipgloss.DefaultRenderer())

// The names the screens use.
var (
	titleStyle     = th.title
	accentStyle    = th.accent
	dimStyle       = th.dim
	mutedStyle     = th.muted
	faintStyle     = th.faint
	okStyle        = th.ok
	errStyle       = th.err
	warnStyle      = th.warn
	warnPlainStyle = th.warnPlain
	cmdStyle       = th.bold
	hintKeyStyle   = th.hintKey
	// ultraBadge is the ULTRA tag of omega ultra's top bar.
	ultraBadge = lipgloss.DefaultRenderer().NewStyle().Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#1A1A1A"}).Background(ultraHue)
)

// Framed styles: cards, boxes and the input. applyBorder builds them, so
// the ASCII set can swap the frame.
var (
	cardStyle        lipgloss.Style // a card: the picker, the help, the panel
	confirmStyle     lipgloss.Style // an approval of a normal command
	confirmWarnStyle lipgloss.Style // an approval of a dangerous command
	confirmErrStyle  lipgloss.Style // a blocked command, the password
	inputStyle       lipgloss.Style
	welcomeStyle     lipgloss.Style
)

func init() { applyBorder(border()) }

func applyBorder(b lipgloss.Border) {
	cardStyle = lipgloss.NewStyle().Border(b).BorderForeground(accentDim).Padding(0, 1)
	confirmStyle = lipgloss.NewStyle().Border(b).BorderForeground(accent).Padding(0, 1)
	confirmWarnStyle = lipgloss.NewStyle().Border(b).BorderForeground(warnColor).Padding(0, 1)
	confirmErrStyle = lipgloss.NewStyle().Border(b).BorderForeground(errColor).Padding(0, 1)
	inputStyle = lipgloss.NewStyle().Border(b).BorderForeground(faint).Padding(0, 1)
	welcomeStyle = lipgloss.NewStyle().Border(b).BorderForeground(accentDim).Padding(0, 2)
}

// reducedMotion is true when the user asked for less animation
// (OMEGA_REDUCED_MOTION=1) or for no colour (NO_COLOR): the spinner stands
// still, the thinking line does not shimmer and the mascot does not move.
func reducedMotion() bool {
	if v := os.Getenv("OMEGA_REDUCED_MOTION"); v != "" && v != "0" {
		return true
	}
	return os.Getenv("NO_COLOR") != ""
}

// compactWidth is the width below which screens drop borders and label
// columns so nothing runs off the right edge.
const compactWidth = 50
