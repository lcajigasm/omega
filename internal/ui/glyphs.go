package ui

import (
	"os"

	"github.com/charmbracelet/lipgloss"
)

// brand is the name of the app as every screen shows it.
const brand = "Ω omega"

// glyphSet is every symbol the interface draws. The Unicode set uses
// characters most terminal fonts have; the ASCII set is for TERM=dumb,
// OMEGA_ASCII=1 and fonts that show boxes instead.
type glyphSet struct {
	Brand string // the app's name with its mark

	Tool   string // a tool call or a message of the model
	Result string // the line under a tool call
	Mode   string // in front of the approval mode
	Lifted string // a mode change

	Check, Cross, Warn, Error string
	Denied, Stopped, Skipped  string
	Pending, Running          string

	Pointer, Focus string // the highlighted row; the focused card
	BoxOn, BoxOff  string // checkboxes
	Back, Crumb    string // the way back; between levels of a path
	Prompt         string // in front of what the user typed
	Up, Down       string // more above, more below; tokens in and out
	Ellipsis       string
	Sep            string // between hints and parts of a line
	Note           string // a reason the model gave
	Pointing       string // the mascot's speech bubble
	Freed, Used    string // disk space freed or taken

	Rule, Heavy, VBar string // thin and thick rules, a column separator

	Spin []string // spinner frames
}

var unicodeGlyphs = glyphSet{
	Brand: brand,
	Tool:  "●", Result: "└", Mode: "»", Lifted: "»",
	Check: "✓", Cross: "✗", Warn: "⚠", Error: "!",
	Denied: "⊘", Stopped: "■", Skipped: "↷",
	Pending: "·", Running: "●",
	Pointer: "❯", Focus: "▸",
	BoxOn: "◼", BoxOff: "◻",
	Back: "‹", Crumb: "›", Prompt: "›",
	Up: "↑", Down: "↓", Ellipsis: "…", Sep: "·",
	Note: "✦", Pointing: "◂", Freed: "▲", Used: "▼",
	Rule: "─", Heavy: "━", VBar: "│",
	Spin: []string{"·", "✢", "✳", "✶", "✻", "✽", "✻", "✶", "✳", "✢"},
}

var asciiGlyphs = glyphSet{
	Brand: "omega",
	Tool:  "*", Result: "`-", Mode: ">>", Lifted: ">>",
	Check: "+", Cross: "x", Warn: "!", Error: "!",
	Denied: "-", Stopped: "#", Skipped: "~",
	Pending: ".", Running: "*",
	Pointer: ">", Focus: ">",
	BoxOn: "x", BoxOff: "o",
	Back: "<", Crumb: ">", Prompt: ">",
	Up: "^", Down: "v", Ellipsis: "...", Sep: "-",
	Note: "*", Pointing: "<", Freed: "+", Used: "-",
	Rule: "-", Heavy: "=", VBar: "|",
	Spin: []string{"|", "/", "-", `\`},
}

// gl is the set in use.
var gl = pickGlyphs()

// asciiMode reports whether the terminal asked for plain ASCII.
func asciiMode() bool {
	return os.Getenv("TERM") == "dumb" || os.Getenv("OMEGA_ASCII") == "1"
}

func pickGlyphs() glyphSet {
	if asciiMode() {
		return asciiGlyphs
	}
	return unicodeGlyphs
}

// border is the frame of cards and boxes: rounded, or plain ASCII.
func border() lipgloss.Border {
	if asciiMode() {
		return lipgloss.ASCIIBorder()
	}
	return lipgloss.RoundedBorder()
}

// setGlyphs switches the set; tests use it. It rebuilds the styles that
// draw a border.
func setGlyphs(ascii bool) {
	if ascii {
		gl = asciiGlyphs
	} else {
		gl = unicodeGlyphs
	}
	b := lipgloss.RoundedBorder()
	if ascii {
		b = lipgloss.ASCIIBorder()
	}
	applyBorder(b)
}

// spinFrame is the spinner at frame n; a single still frame when motion is
// reduced.
func spinFrame(n int) string {
	if reducedMotion() {
		return gl.Running
	}
	return gl.Spin[n%len(gl.Spin)]
}
