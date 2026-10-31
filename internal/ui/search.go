package ui

import (
	"strings"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

// highlightQuery wraps case-insensitive matches in reverse video. The marker
// is an explicit SGR sequence so the highlight does not depend on the color
// profile used for the rest of the interface.
func highlightQuery(s, query string) string {
	if query == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = highlightLine(line, query)
	}
	return strings.Join(lines, "\n")
}

func highlightLine(line, query string) string {
	q := []rune(strings.ToLower(query))
	if len(q) == 0 || line == "" {
		return line
	}
	type vis struct {
		r    rune
		at   int
		size int
	}
	var vs []vis
	for i := 0; i < len(line); {
		if line[i] == 0x1b {
			i = skipEsc(line, i)
			continue
		}
		r, n := utf8.DecodeRuneInString(line[i:])
		vs = append(vs, vis{r, i, n})
		i += n
	}
	low := make([]rune, len(vs))
	mark := make([]bool, len(vs))
	for i, v := range vs {
		low[i] = unicode.ToLower(v.r)
	}
	for i := 0; i+len(q) <= len(low); {
		if hasPrefix(low[i:], q) {
			for j := 0; j < len(q); j++ {
				mark[i+j] = true
			}
			i += len(q)
			continue
		}
		i++
	}
	var b strings.Builder
	prev := 0
	in := false
	for i, v := range vs {
		if mark[i] == in {
			continue
		}
		b.WriteString(line[prev:v.at])
		if mark[i] {
			b.WriteString("\x1b[7m")
		} else {
			b.WriteString("\x1b[27m")
		}
		prev = v.at
		in = mark[i]
	}
	b.WriteString(line[prev:])
	if in {
		b.WriteString("\x1b[27m")
	}
	return b.String()
}

func hasPrefix(s, prefix []rune) bool {
	if len(prefix) > len(s) {
		return false
	}
	for i, r := range prefix {
		if s[i] != r {
			return false
		}
	}
	return true
}

func skipEsc(s string, i int) int {
	if i+1 >= len(s) {
		return len(s)
	}
	if s[i+1] == '[' {
		i += 2
		for i < len(s) {
			if s[i] >= 0x40 && s[i] <= 0x7e {
				return i + 1
			}
			i++
		}
		return i
	}
	return i + 1
}

func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			i = skipEsc(s, i)
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// matchLines returns the transcript line index of each line that contains query.
func matchLines(content, query string) []int {
	if query == "" {
		return nil
	}
	q := strings.ToLower(query)
	var out []int
	for i, line := range strings.Split(content, "\n") {
		if strings.Contains(strings.ToLower(stripANSI(line)), q) {
			out = append(out, i)
		}
	}
	return out
}

func (m *model) handleSearchKey(msg tea.KeyMsg) {
	switch msg.String() {
	case "esc", "ctrl+f":
		m.searchOn = false
		m.searchQ = ""
		m.searchAt = -1
		m.refresh()
		return
	case "enter", "ctrl+n", "down":
		m.moveSearch(1)
		return
	case "ctrl+p", "up":
		m.moveSearch(-1)
		return
	case "backspace":
		r := []rune(m.searchQ)
		if len(r) > 0 {
			m.searchQ = string(r[:len(r)-1])
		}
		m.searchAt = 0
		m.refresh()
		return
	}
	if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 {
		m.searchQ += string(msg.Runes)
		m.searchAt = 0
		m.refresh()
	}
}

func (m *model) moveSearch(delta int) {
	lines := matchLines(m.transcript(), m.searchQ)
	if len(lines) == 0 {
		m.searchAt = -1
		m.refresh()
		return
	}
	if m.searchAt < 0 || m.searchAt >= len(lines) {
		m.searchAt = 0
	} else {
		m.searchAt = (m.searchAt + delta) % len(lines)
		if m.searchAt < 0 {
			m.searchAt += len(lines)
		}
	}
	m.refresh()
}
