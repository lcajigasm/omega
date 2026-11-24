package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/lcajigasm/omega/internal/i18n"
)

// hint is one entry of a key line: the key in bold, then what it does. A
// hint without a key is a plain remark ("type to filter").
type hint struct {
	key    string // as typed: "enter", "ctrl+c", "↑↓", "a/enter"
	action string // already translated
	keep   bool   // never dropped for room: help, quit, the answer itself
}

// hk is a hint whose action is the i18n key action.
func hk(key, action string, args ...any) hint {
	return hint{key: key, action: i18n.T(action, args...)}
}

// must marks a hint as one a narrow terminal never drops.
func (h hint) must() hint { h.keep = true; return h }

// hintSep separates hints on a line.
func hintSep() string { return " " + gl.Sep + " " }

// keyLabel writes a key the way the user's language names it: "Strg+C",
// "entrée", "espacio". Alternatives ("a/enter") are named one by one.
func keyLabel(k string) string {
	if k == "" {
		return ""
	}
	parts := strings.Split(k, "/")
	for i, p := range parts {
		parts[i] = keyName(p)
	}
	return strings.Join(parts, "/")
}

func keyName(k string) string {
	if p, rest, ok := strings.Cut(k, "+"); ok && rest != "" {
		switch p {
		case "ctrl":
			p = i18n.T("uix.k_ctrl")
		case "shift":
			p = i18n.T("uix.k_shift")
		}
		return p + "+" + keyName(rest)
	}
	switch k {
	case "enter":
		return i18n.T("uix.k_enter")
	case "esc":
		return i18n.T("uix.k_esc")
	case "space":
		return i18n.T("uix.k_space")
	case "drag":
		return i18n.T("uix.k_drag")
	}
	if gl.Up != "↑" {
		// The ASCII set has no arrows.
		k = strings.NewReplacer("↑↓", i18n.T("uix.k_up")+"/"+i18n.T("uix.k_down"),
			"↑", i18n.T("uix.k_up"), "↓", i18n.T("uix.k_down"), "←", "left", "→", "right").Replace(k)
	}
	return k
}

// plain is the hint as text, for measuring.
func (h hint) plain() string {
	if h.key == "" {
		return h.action
	}
	if h.action == "" {
		return keyLabel(h.key)
	}
	return keyLabel(h.key) + " " + h.action
}

// render is the hint in colour: the key in bold, the action muted.
func (h hint) render() string {
	switch {
	case h.key == "":
		return mutedStyle.Render(h.action)
	case h.action == "":
		return hintKeyStyle.Render(keyLabel(h.key))
	}
	return hintKeyStyle.Render(keyLabel(h.key)) + mutedStyle.Render(" "+h.action)
}

// hintIndent is the margin of a key line under a screen.
const hintIndent = "  "

// hintLine is the key line of a screen, width columns wide. When the hints
// do not fit, the ones that may go are dropped from the end; when even the
// ones that must stay (help, quit) do not fit, the line wraps between hints,
// never inside one.
func hintLine(width int, hints ...hint) string {
	return hintsAt(width, hintIndent, true, hints...)
}

// cardHints is a key line inside a card: no margin of its own.
func cardHints(width int, hints ...hint) string {
	return hintsAt(width, "", true, hints...)
}

// hintBlock is a key line that keeps every hint, wrapped onto as many lines
// as it takes.
func hintBlock(width int, hints ...hint) string {
	return hintsAt(width, hintIndent, false, hints...)
}

func hintsAt(width int, indent string, drop bool, hints ...hint) string {
	room := max(10, width-len(indent))
	hs := append([]hint(nil), hints...)
	for drop && lipgloss.Width(joinPlain(hs)) > room {
		at := -1
		for i := len(hs) - 1; i >= 0; i-- {
			if !hs[i].keep {
				at = i
				break
			}
		}
		if at < 0 {
			break
		}
		hs = append(hs[:at], hs[at+1:]...)
	}
	return renderLines(wrapHintList(hs, room), indent)
}

func joinPlain(hs []hint) string {
	parts := make([]string, len(hs))
	for i, h := range hs {
		parts[i] = h.plain()
	}
	return strings.Join(parts, hintSep())
}

// wrapHintList splits hints into lines no wider than width.
func wrapHintList(hs []hint, width int) [][]hint {
	var out [][]hint
	var line []hint
	w := 0
	for _, h := range hs {
		hw := lipgloss.Width(h.plain())
		if len(line) > 0 && w+lipgloss.Width(hintSep())+hw > width {
			out = append(out, line)
			line, w = nil, 0
		}
		if len(line) > 0 {
			w += lipgloss.Width(hintSep())
		}
		line = append(line, h)
		w += hw
	}
	if len(line) > 0 || len(out) == 0 {
		out = append(out, line)
	}
	return out
}

func renderLines(lines [][]hint, indent string) string {
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(indent)
		for j, h := range line {
			if j > 0 {
				b.WriteString(faintStyle.Render(hintSep()))
			}
			b.WriteString(h.render())
		}
	}
	return b.String()
}

// wrapHints breaks a key line ("a do · b do · …") between hints, never
// inside one, so no hint is cut off on a narrow terminal.
func wrapHints(s string, width int) []string {
	var out []string
	line := ""
	for _, h := range strings.Split(s, " · ") {
		switch {
		case line == "":
			line = h
		case lipgloss.Width(line)+3+lipgloss.Width(h) <= width:
			line += " · " + h
		default:
			out = append(out, line)
			line = h
		}
	}
	return append(out, line)
}

// moreAbove and moreBelow say how many rows of a list are out of sight.
func moreAbove(n int) string { return faintStyle.Render(gl.Up + " " + i18n.T("uix.more", n)) }
func moreBelow(n int) string { return faintStyle.Render(gl.Down + " " + i18n.T("uix.more", n)) }
