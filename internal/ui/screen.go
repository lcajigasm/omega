package ui

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

// screen paints a rendered view into a fixed grid and returns the visible
// text. ANSI sequences are ignored, so the result does not depend on the
// color profile or on the size of a real terminal.
func screen(view string, width, height int) string {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	grid := make([][]rune, height)
	for y := range grid {
		grid[y] = make([]rune, width)
		for x := range grid[y] {
			grid[y][x] = ' '
		}
	}
	x, y := 0, 0
	for i := 0; i < len(view); {
		if view[i] == 0x1b {
			i = skipEsc(view, i)
			continue
		}
		r, n := utf8.DecodeRuneInString(view[i:])
		i += n
		switch r {
		case '\n':
			y++
			x = 0
			continue
		case '\r':
			x = 0
			continue
		}
		if y >= height {
			break
		}
		w := lipgloss.Width(string(r))
		if w < 1 {
			w = 1
		}
		if x >= width {
			continue
		}
		grid[y][x] = r
		for k := 1; k < w; k++ {
			if x+k < width {
				grid[y][x+k] = ' '
			}
		}
		x += w
	}
	var b strings.Builder
	last := height - 1
	for last > 0 && strings.TrimSpace(string(grid[last])) == "" {
		last--
	}
	for row := 0; row <= last; row++ {
		if row > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(strings.TrimRight(string(grid[row]), " "))
	}
	return b.String()
}
