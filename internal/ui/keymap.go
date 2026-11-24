package ui

import (
	"strings"

	kb "github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/lcajigasm/omega/internal/i18n"
)

// keyMap is every key the screens share. A key means the same on every
// screen a user moves between: ctrl+p only switches the model (a session),
// lists move with ↑/↓ and wrap around, q leaves a screen that has nothing
// typed, ctrl+c stops what runs and, pressed again, leaves.
type keyMap struct {
	Up, Down, PgUp, PgDown, Home, End kb.Binding
	Top, Bottom                       kb.Binding // g and G where nothing is typed
	Enter, Back, Quit, Interrupt      kb.Binding
	Help, Search, Mode, Model         kb.Binding
	Expand, Panel, Copy               kb.Binding
	FocusPrev, FocusNext              kb.Binding
	Toggle, All                       kb.Binding
	SearchNext, SearchPrev            kb.Binding
	ApproveAll, Deny                  kb.Binding
}

var km = keyMap{
	Up:         kb.NewBinding(kb.WithKeys("up")),
	Down:       kb.NewBinding(kb.WithKeys("down")),
	PgUp:       kb.NewBinding(kb.WithKeys("pgup")),
	PgDown:     kb.NewBinding(kb.WithKeys("pgdown")),
	Home:       kb.NewBinding(kb.WithKeys("home")),
	End:        kb.NewBinding(kb.WithKeys("end")),
	Top:        kb.NewBinding(kb.WithKeys("home", "g")),
	Bottom:     kb.NewBinding(kb.WithKeys("end", "G")),
	Enter:      kb.NewBinding(kb.WithKeys("enter")),
	Back:       kb.NewBinding(kb.WithKeys("esc")),
	Quit:       kb.NewBinding(kb.WithKeys("q")),
	Interrupt:  kb.NewBinding(kb.WithKeys("ctrl+c")),
	Help:       kb.NewBinding(kb.WithKeys("?")),
	Search:     kb.NewBinding(kb.WithKeys("ctrl+f")),
	Mode:       kb.NewBinding(kb.WithKeys("shift+tab")),
	Model:      kb.NewBinding(kb.WithKeys("ctrl+p", "alt+p")),
	Expand:     kb.NewBinding(kb.WithKeys("ctrl+o")),
	Panel:      kb.NewBinding(kb.WithKeys("ctrl+b")),
	Copy:       kb.NewBinding(kb.WithKeys("ctrl+y")),
	FocusPrev:  kb.NewBinding(kb.WithKeys("alt+up")),
	FocusNext:  kb.NewBinding(kb.WithKeys("alt+down")),
	Toggle:     kb.NewBinding(kb.WithKeys(" ", "x")),
	All:        kb.NewBinding(kb.WithKeys("a")),
	SearchNext: kb.NewBinding(kb.WithKeys("enter", "down")),
	SearchPrev: kb.NewBinding(kb.WithKeys("up")),
	ApproveAll: kb.NewBinding(kb.WithKeys("a", "A")),
	Deny:       kb.NewBinding(kb.WithKeys("n", "N", "esc")),
}

// yesKey is the letter that answers yes in the user's language: y, s, o, j.
func yesKey() string { return i18n.T("uix.yes_key") }

// isYes reports whether a key approves: y always, and the yes letter of the
// user's language.
func isYes(k tea.KeyMsg) bool {
	s := strings.ToLower(k.String())
	return s == "y" || s == yesKey()
}

// wrap moves a cursor by delta through n rows, going round at both ends.
func wrap(cursor, delta, n int) int {
	if n <= 0 {
		return 0
	}
	return ((cursor+delta)%n + n) % n
}
