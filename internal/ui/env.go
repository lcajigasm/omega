package ui

import (
	"os"

	"golang.org/x/term"
)

// WantTUI reports whether a command should draw the full-screen interface:
// both stdin and stdout are terminals, the terminal can draw it (TERM is not
// dumb), and neither --no-tui (noTUIFlag) nor NO_TUI asks for plain output.
// Every command that can draw a screen decides with this.
func WantTUI(noTUIFlag bool) bool {
	if noTUIFlag || os.Getenv("NO_TUI") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

// mouseWanted is false when OMEGA_MOUSE=0: the screens then leave the mouse
// to the terminal, so text can be selected with a plain drag.
func mouseWanted() bool { return os.Getenv("OMEGA_MOUSE") != "0" }
