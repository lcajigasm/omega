package ui

import (
	"math"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ---------- the mascot ----------

// Moods of the mascot.
const (
	moodIdle = iota
	moodWork
	moodThink
	moodOops
	moodHappy
	moodSad
	moodStop
)

// The mascot is drawn on a small canvas of cells, each with its own
// colour: the body, the glint that runs along it, the face and what floats
// around it.
const (
	inkNone = iota
	inkBody
	inkGlint    // the head of the glint
	inkGlintMid // its trail
	inkFace     // eyes and mouth
	inkBlush
	inkSpark // sparkles, thought bubbles
	inkJoy   // confetti in the colour of success
	inkGold  // confetti in the warning colour
	inkAlarm // a failure: the "!" and the flash of the body
	inkTear
)

type cell struct {
	r   rune
	ink int
}

type canvas struct {
	w, h  int
	cells [][]cell
}

func newCanvas(w, h int) *canvas {
	c := &canvas{w: w, h: h, cells: make([][]cell, h)}
	for y := range c.cells {
		c.cells[y] = make([]cell, w)
		for x := range c.cells[y] {
			c.cells[y][x] = cell{r: ' '}
		}
	}
	return c
}

// put writes s from (x, y); spaces in s leave the canvas as it was.
func (c *canvas) put(x, y int, s string, ink int) {
	if y < 0 || y >= c.h {
		return
	}
	for _, r := range s {
		if x >= 0 && x < c.w && r != ' ' {
			c.cells[y][x] = cell{r: r, ink: ink}
		}
		x++
	}
}

// erase blanks n cells from (x, y).
func (c *canvas) erase(x, y, n int) {
	if y < 0 || y >= c.h {
		return
	}
	for i := x; i < x+n && i < c.w; i++ {
		if i >= 0 {
			c.cells[y][i] = cell{r: ' '}
		}
	}
}

// lines renders the canvas, one style per run of cells of the same ink.
func (c *canvas) lines(body lipgloss.Style) []string {
	style := func(ink int) lipgloss.Style {
		switch ink {
		case inkBody:
			return body
		case inkGlint:
			return th.shimBright
		case inkGlintMid:
			return th.shimMid
		case inkFace:
			return th.shimBright
		case inkBlush:
			return errStyle
		case inkSpark:
			return accentStyle
		case inkJoy:
			return okStyle
		case inkGold:
			return warnPlainStyle
		case inkAlarm:
			return errStyle.Bold(true)
		case inkTear:
			return mutedStyle
		}
		return lipgloss.NewStyle()
	}
	out := make([]string, c.h)
	for y, row := range c.cells {
		var b strings.Builder
		for x := 0; x < len(row); {
			ink := row[x].ink
			var run strings.Builder
			for x < len(row) && row[x].ink == ink {
				run.WriteRune(row[x].r)
				x++
			}
			if ink == inkNone {
				b.WriteString(run.String())
			} else {
				b.WriteString(style(ink).Render(run.String()))
			}
		}
		out[y] = b.String()
	}
	return out
}

// ---------- the big Ω ----------

// avatarWidth and avatarHeight are the size of the big mascot.
const (
	avatarWidth  = 26
	avatarHeight = 10
)

// omegaBody is the letter Ω drawn in blocks, 22 columns wide: a thick arc
// that ends in two legs with their feet turned out.
var omegaBody = []string{
	"      ▄▄██████▄▄      ",
	"   ▄█▀▀        ▀▀█▄   ",
	"  ██              ██  ",
	" ██                ██ ",
	" ██                ██ ",
	"  ██              ██  ",
	"   ▀█▄          ▄█▀   ",
	"▄▄▄▄▄██        ██▄▄▄▄▄",
	"▀▀▀▀▀▀▀        ▀▀▀▀▀▀▀",
}

// omegaPath is every cell of the body in the order a glint runs along it:
// from the left foot, up and over the arc, down to the right foot.
var omegaPath = func() [][2]int {
	type pt struct {
		x, y int
		k    float64
	}
	var pts []pt
	for y, row := range omegaBody {
		for x, r := range []rune(row) {
			if r == ' ' {
				continue
			}
			a := math.Atan2(-(float64(y)-4), float64(x)-10.5) * 180 / math.Pi
			if a < -90 {
				a += 360
			}
			pts = append(pts, pt{x, y, 225 - a})
		}
	}
	sort.SliceStable(pts, func(i, j int) bool { return pts[i].k < pts[j].k })
	out := make([][2]int, len(pts))
	for i, p := range pts {
		out[i] = [2]int{p.x, p.y}
	}
	return out
}()

// eye is one eye over two rows of the face.
type eye struct{ top, bot string }

var (
	eyeOpen   = eye{"▄▄", "██"} // tall and round
	eyeShut   = eye{"  ", "▄▄"} // the lid down
	eyeHappy  = eye{"╭╮", "  "}
	eyeCalm   = eye{"──", "  "}
	eyeLeftX  = eye{"▀▄", "▄▀"} // >
	eyeRightX = eye{"▄▀", "▀▄"} // <
)

// avatar draws the mascot big: the letter Ω itself, with a face inside it.
// It never sits still while the run works: a glint runs along the arc, the
// feet tap, the eyes look around and blink, sparkles twinkle around the
// head; it ponders with thought bubbles, winces and flashes at a failure,
// jumps among confetti when a stage ends well and droops when one fails.
// frame counts ticks of the screen. A terminal without block characters
// gets the drawing in plain ASCII.
func avatar(mood, frame int) []string { return drawAvatar(mood, frame, true) }

// drawAvatar is avatar, with or without the glint: the portrait left on
// the terminal at the end is a still picture.
func drawAvatar(mood, frame int, glint bool) []string {
	if asciiMode() {
		return asciiAvatar(mood, frame)
	}
	c := newCanvas(avatarWidth, avatarHeight)
	x0, y0 := 2, 1
	if mood == moodHappy && frame/4%2 == 1 {
		y0 = 0 // a jump
	}
	if mood == moodOops && frame%4 < 2 {
		x0 = 3 // a shiver
	}
	for y, row := range omegaBody {
		c.put(x0, y0+y, row, inkBody)
	}

	// The feet tap while it works.
	if mood == moodWork {
		side := frame / 4 % 2
		fx := x0 + side*17
		c.put(fx, y0+7, "▀▀▀▀▀", inkBody)
		c.erase(fx, y0+8, 5)
	}

	// The glint along the arc; on the green of a success, without the
	// accent-coloured trail.
	switch mood {
	case moodWork, moodThink, moodHappy, moodIdle:
		if !glint {
			break
		}
		speed := map[int]int{moodWork: 1, moodThink: 2, moodHappy: 1, moodIdle: 3}[mood]
		n := len(omegaPath)
		at := frame / speed % (n + 12)
		for i, p := range omegaPath {
			ink := -1
			switch d := at - i; {
			case d == 0:
				ink = inkGlint
			case d > 0 && d <= 3 && mood != moodHappy:
				ink = inkGlintMid
			}
			if ink >= 0 {
				cl := &c.cells[y0+p[1]][x0+p[0]]
				if cl.r != ' ' {
					cl.ink = ink
				}
			}
		}
	}

	// The face: eyes on rows 4 and 5 of the body (3 and 4 when it looks
	// up), the mouth on row 6.
	left, right := eyeOpen, eyeOpen
	look, up := 0, 0
	mouth := "╰──╯"
	blink := frame%46 < 2
	switch mood {
	case moodIdle:
		if blink {
			left, right = eyeShut, eyeShut
		}
	case moodWork:
		look = []int{0, -1, 0, 1, 0, 0}[frame/9%6]
		if blink {
			left, right = eyeShut, eyeShut
		}
		if frame/3%5 == 2 {
			mouth = "╰▄▄╯" // it talks to itself
		}
	case moodThink:
		up = 1
		look = []int{-1, -1, 0, 1, 1, 0}[frame/7%6]
		mouth = []string{" ~~ ", "~~  ", " ~~ ", "  ~~"}[frame/6%4]
	case moodOops:
		left, right, mouth = eyeLeftX, eyeRightX, " ╭╮ "
	case moodHappy:
		left, right, mouth = eyeHappy, eyeHappy, "╰▄▄▄▄╯"
	case moodSad:
		mouth = "╭──╮"
	case moodStop:
		left, right, mouth = eyeCalm, eyeCalm, "────"
	}
	// A narrow eye sits at columns 7 and 14 of the body, a wide one at 6-7
	// and 14-15: the face stays symmetric.
	ey := y0 + 3 - up
	lx := x0 + 8 - len([]rune(left.top)) + look
	c.put(lx, ey, left.top, inkFace)
	c.put(lx, ey+1, left.bot, inkFace)
	c.put(x0+14+look, ey, right.top, inkFace)
	c.put(x0+14+look, ey+1, right.bot, inkFace)
	c.put(x0+11-len([]rune(mouth))/2, y0+5, mouth, inkFace)

	// What goes on around it.
	switch mood {
	case moodWork:
		spots := [][2]int{{1, 0}, {24, 1}, {0, 4}, {25, 5}, {3, 1}, {22, 0}}
		for i, s := range spots {
			glyph := []string{" ", "·", "✦", "✧", "·", " ", " ", " "}[(frame/3+i*3)%8]
			c.put(s[0], s[1], glyph, inkSpark)
		}
	case moodThink:
		bubbles := [][2]int{{21, 2}, {23, 1}, {25, 0}}
		marks := []string{"·", "o", "O"}
		for i := 0; i <= frame/5%4 && i < 3; i++ {
			c.put(bubbles[i][0], bubbles[i][1], marks[i], inkSpark)
		}
	case moodHappy:
		spots := [][2]int{{0, 0}, {25, 0}, {2, 2}, {23, 3}, {0, 6}, {25, 7}, {5, 0}, {20, 0}}
		inks := []int{inkJoy, inkGold, inkSpark}
		for i, s := range spots {
			glyph := []string{"✦", "·", "✧", "*", " "}[(frame/2+i)%5]
			c.put(s[0], s[1], glyph, inks[i%3])
		}
		c.put(x0+4, y0+4, "░", inkBlush)
		c.put(x0+17, y0+4, "░", inkBlush)
	case moodOops:
		c.put(24, 1, "!", inkAlarm)
		if frame%6 < 3 {
			c.put(22, 0, "!", inkAlarm)
		}
	case moodSad:
		// Worried brows and a tear that falls.
		c.put(x0+7, y0+2, "╱", inkFace)
		c.put(x0+14, y0+2, "╲", inkFace)
		c.put(x0+7, y0+5+frame/5%2, "╷", inkTear)
	}

	body := accentStyle
	switch mood {
	case moodOops:
		if frame/2%2 == 0 {
			body = errStyle
		}
	case moodSad:
		body = mutedStyle
	case moodStop:
		body = dimStyle
	case moodHappy:
		body = okStyle
	}
	return c.lines(body)
}

// ---------- the small one ----------

// mascotWidth and mascotHeight are the size of the small mascot.
const (
	mascotWidth  = 10
	mascotHeight = 4
)

// mascot draws the mascot small, for a terminal without room for its big
// Ω: the same letter in four rows, with the same face.
func mascot(mood, frame int) []string {
	if asciiMode() {
		return asciiMascot(mood, frame)
	}
	c := newCanvas(mascotWidth, mascotHeight)
	c.put(0, 0, " ▄█▀▀▀▀█▄ ", inkBody)
	c.put(0, 1, "██      ██", inkBody)
	c.put(0, 2, " ▀▄    ▄▀ ", inkBody)
	c.put(0, 3, "▄▄█    █▄▄", inkBody)
	if mood == moodWork && frame/4%2 == 1 {
		c.put(0, 3, "▀▀█    █▄▄", inkBody) // a step
	}
	l, r, mouth, look := "●", "●", "╰╯", 0
	switch mood {
	case moodIdle, moodWork:
		if frame%46 < 2 {
			l, r = "─", "─"
		}
		if mood == moodWork {
			look = []int{0, -1, 0, 1, 0, 0}[frame/9%6]
		}
	case moodThink:
		l, r, mouth = "•", "•", "~~"
		look = []int{-1, 0, 1, 0}[frame/7%4]
	case moodOops:
		l, r, mouth = ">", "<", "╭╮"
	case moodHappy:
		l, r, mouth = "^", "^", "▄▄"
	case moodSad:
		l, r, mouth = "╥", "╥", "╭╮"
	case moodStop:
		l, r, mouth = "─", "─", "──"
	}
	c.put(3+look, 1, l, inkFace)
	c.put(6+look, 1, r, inkFace)
	c.put(4, 2, mouth, inkFace)
	body := accentStyle
	switch mood {
	case moodOops:
		body = errStyle
	case moodSad:
		body = mutedStyle
	case moodStop:
		body = dimStyle
	case moodHappy:
		body = okStyle
	}
	return c.lines(body)
}

// ---------- in plain ASCII ----------

// mid centres s in a field w wide.
func mid(s string, w int) string {
	gap := w - len([]rune(s))
	if gap <= 0 {
		return s
	}
	return strings.Repeat(" ", gap/2) + s + strings.Repeat(" ", gap-gap/2)
}

// asciiAvatar is the big mascot for a terminal without block characters.
func asciiAvatar(mood, frame int) []string {
	spark, left, right, mouth := " ", " o ", " o ", ` \___/ `
	arc, footL, footR := "=========", "'=====:", ":====='"
	look := func(n int) string { return []string{" o ", "o  ", " o ", "  o"}[n%4] }
	glint := func(at int, with string) string {
		at %= len(arc) + 3
		if at >= len(arc) {
			return arc
		}
		return arc[:at] + with + arc[at+1:]
	}
	switch mood {
	case moodIdle:
		if frame%40 < 2 {
			left, right = " - ", " - "
		}
	case moodWork:
		spark = []string{".", "*", `\|/`, "*"}[frame/3%4]
		arc = glint(frame/2, "*")
		if frame/4%2 == 0 {
			footL = "'-----:"
		} else {
			footR = ":-----'"
		}
		left = look(frame / 10)
		right = left
		mouth = []string{` \___/ `, `\_____/`, ` \___/ `, "  ---  "}[frame/2%4]
	case moodThink:
		spark = []string{".", "o", "O", "o O", ". o O"}[frame/4%5]
		arc = glint(frame/5, "~")
		left = []string{"o  ", " o ", "  o", " o "}[frame/6%4]
		right = left
		mouth = "   ~   "
	case moodOops:
		spark, left, right, mouth = "!", " > ", " < ", ` /~~~\ `
	case moodHappy:
		spark = []string{`\|/`, "*"}[frame/3%2]
		arc = []string{"=*=*=*=*=", "*=*=*=*=*"}[frame/3%2]
		left, right, mouth = " ^ ", " ^ ", `\_____/`
	case moodSad:
		left, right, mouth = " ; ", " ; ", " .---. "
	case moodStop:
		left, right, mouth = " - ", " - ", "  ---  "
	}
	rows := []string{
		mid(spark, 23),
		mid(".-"+arc+"-.", 23),
		"   .'" + strings.Repeat(" ", 13) + "'.   ",
		"  /" + strings.Repeat(" ", 17) + `\  `,
		" |" + mid("("+left+")   ("+right+")", 19) + "| ",
		" |" + strings.Repeat(" ", 19) + "| ",
		`  \` + mid(mouth, 17) + "/  ",
		"   '." + strings.Repeat(" ", 13) + ".'   ",
		mid("'.         .'", 23),
		" " + footL + strings.Repeat(" ", 7) + footR + " ",
	}
	for i := range rows {
		rows[i] = accentStyle.Render(mid(rows[i], avatarWidth))
	}
	return rows
}

// asciiMascot is the small mascot for a terminal without block characters.
func asciiMascot(mood, frame int) []string {
	eyes := " o o "
	switch mood {
	case moodIdle, moodWork:
		if frame%40 < 2 {
			eyes = " - - "
		}
	case moodThink:
		eyes = []string{" o o ", "o o  ", " o o ", "  o o"}[frame/6%4]
	case moodOops:
		eyes = " > < "
	case moodHappy:
		eyes = " ^ ^ "
	case moodSad:
		eyes = " ; ; "
	case moodStop:
		eyes = " - - "
	}
	rows := []string{" .---.  ", "(" + eyes + ") ", "/| O |\\ ", " '---'  "}
	for i := range rows {
		rows[i] = accentStyle.Render(mid(rows[i], mascotWidth))
	}
	return rows
}

func (m *pilotModel) mood() int {
	switch {
	case m.finished:
		for _, r := range m.res {
			if r.Status == StageFailed {
				return moodSad
			}
		}
		if m.stopping {
			return moodStop
		}
		return moodHappy
	case m.stopping:
		return moodStop
	case m.oops > 0:
		return moodOops
	case m.cheer > 0:
		return moodHappy // a stage just ended well
	case m.thinking:
		return moodThink
	case m.cur >= 0:
		return moodWork // a stage is running: the mascot never sits still
	}
	return moodIdle
}
