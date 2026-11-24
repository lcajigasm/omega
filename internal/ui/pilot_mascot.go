package ui

import "strings"

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

// mascotWidth is the width of every line of the mascot.
const mascotWidth = 9

// mascot draws the mascot small, for a terminal without room for its big
// Ω (avatar): a round robot with an Ω on its chest. Its
// eyes and arms say what the run is doing.
func mascot(mood, frame int) []string {
	eyes, arms := " o o ", `/| Ω |\`
	switch mood {
	case moodIdle, moodWork:
		if frame%40 < 2 {
			eyes = " - - " // a blink
		}
		if mood == moodWork && frame/4%2 == 1 {
			arms = `\| Ω |/`
		}
	case moodThink:
		eyes = []string{" o o ", "o o  ", " o o ", "  o o"}[frame/6%4]
	case moodOops:
		eyes = " > < "
	case moodHappy:
		eyes, arms = " ^ ^ ", `\| Ω |/`
	case moodSad:
		eyes = " ; ; "
	case moodStop:
		eyes = " - - "
	}
	return []string{
		"  .---.  ",
		" (" + eyes + ") ",
		" " + arms + " ",
		"  '---'  ",
		`   " "   `,
	}
}

// avatarWidth is the width of every line of the avatar.
const avatarWidth = 23

// mid centres s in a field w wide.
func mid(s string, w int) string {
	gap := w - len([]rune(s))
	if gap <= 0 {
		return s
	}
	return strings.Repeat(" ", gap/2) + s + strings.Repeat(" ", gap-gap/2)
}

// avatar draws the mascot big, for a terminal with room for it: the letter
// Ω itself, with a face inside it. While the run works a glint runs along
// its arc, its feet step, its eyes look around and blink and its mouth
// moves. frame counts ticks of the screen.
func avatar(mood, frame int) []string {
	spark, left, right, mouth := " ", " o ", " o ", ` \___/ `
	arc, footL, footR := "=========", "'=====:", ":====='"
	look := func(n int) string { return []string{" o ", "o  ", " o ", "  o"}[n%4] }
	glint := func(at int, with string) string {
		at %= len(arc) + 3 // the glint leaves the arc for a moment
		if at >= len(arc) {
			return arc
		}
		return arc[:at] + with + arc[at+1:]
	}
	switch mood {
	case moodIdle:
		if frame%40 < 2 {
			left, right = " - ", " - " // a blink
		}
	case moodWork:
		spark = []string{".", "*", `\|/`, "*"}[frame/3%4]
		arc = glint(frame/2, "*")
		if frame/4%2 == 0 {
			footL = "'-----:" // a step
		} else {
			footR = ":-----'"
		}
		left = look(frame / 10)
		right = left
		if frame%40 < 2 {
			left, right = " - ", " - "
		}
		mouth = []string{` \___/ `, `\_____/`, ` \___/ `, "  ---  "}[frame/2%4]
	case moodThink:
		spark = []string{".", "o", "O", "o O", ". o O"}[frame/4%5]
		arc = glint(frame/5, "~")
		left = []string{"o  ", " o ", "  o", " o "}[frame/6%4]
		right = left
		mouth = []string{"   ~   ", "  ~    ", "   ~   ", "    ~  "}[frame/6%4]
	case moodOops:
		spark, left, right, mouth = "!", " > ", " < ", ` /~~~\ `
	case moodHappy:
		spark = []string{`\|/`, "*"}[frame/3%2]
		arc = []string{"=*=*=*=*=", "*=*=*=*=*"}[frame/3%2]
		left, right, mouth = " ^ ", " ^ ", `\_____/`
	case moodSad:
		left, right, mouth = " ; ", " ; ", " .---. "
		footL, footR = "'-----:", ":-----'"
	case moodStop:
		left, right, mouth = " - ", " - ", "  ---  "
	}
	return []string{
		mid(spark, avatarWidth),
		mid(".-"+arc+"-.", avatarWidth),
		"   .'" + strings.Repeat(" ", 13) + "'.   ",
		"  /" + strings.Repeat(" ", 17) + `\  `,
		" |" + mid("("+left+")   ("+right+")", 19) + "| ",
		" |" + strings.Repeat(" ", 19) + "| ",
		`  \` + mid(mouth, 17) + "/  ",
		"   '." + strings.Repeat(" ", 13) + ".'   ",
		mid("'.         .'", avatarWidth),
		" " + footL + strings.Repeat(" ", 7) + footR + " ",
	}
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
	case m.thinking:
		return moodThink
	case m.cur >= 0:
		return moodWork // a stage is running: the mascot never sits still
	}
	return moodIdle
}
