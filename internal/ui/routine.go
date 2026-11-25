package ui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
)

// RoutineStep is a step the routine editor can offer.
type RoutineStep struct {
	ID, Label, Category, Command string
	Risk                         string // "read-only", "normal" or "dangerous"
	// LastRun is how long the step took the last time `omega custom` ran
	// it; 0 when it never did.
	LastRun time.Duration
	// Freed is how much free space the disk gained over that run; 0 when
	// it gained none or it was not measured.
	Freed int64
}

// RecommendFunc asks the model for a routine. withData is true when the
// user turned on sharing the machine's aggregated data (key d).
type RecommendFunc func(ctx context.Context, current []string, withData bool) (Recommendation, error)

// RoutineSetup is what the editor starts from.
type RoutineSetup struct {
	Steps           []RoutineStep // installed steps, in category order
	Current         []string      // the saved routine
	StopOnFail      []string      // steps whose failure ends the run
	OnlyBelowFreeGB float64       // 0: always
	Days            []string      // weekdays of a scheduled run (mon … sun); none: every day
	// Recommend may be nil, which hides the AI key.
	Recommend RecommendFunc
	// MachineData says, in words, the aggregated data key d adds to the
	// recommendation request; "" hides the key.
	MachineData string
}

// RoutineChoice is what the user saved.
type RoutineChoice struct {
	Steps           []string
	StopOnFail      []string // only steps of the routine, in its order
	OnlyBelowFreeGB float64
	Days            []string // mon … sun, Monday first; none: every day
}

// Recommendation is what the model suggests for the routine: the steps in
// order, a reason for each and a one-paragraph summary.
type Recommendation struct {
	Steps   []string
	Why     map[string]string
	Summary string
}

// RoutineEditor is the full-screen editor behind `omega custom --set`: the
// routine on top, in running order, and every other installed step below,
// grouped by category. Nothing in it reaches the network unless the user
// presses i, which asks the model for a recommendation.
type RoutineEditor struct {
	steps     []RoutineStep
	byID      map[string]RoutineStep
	order     []string // the routine
	saved     []string // the routine as it was loaded
	cursor    int
	offset    int
	width     int
	height    int
	recommend RecommendFunc

	stops      map[string]bool // stop the run if this step fails
	savedStops map[string]bool
	below      float64 // only below this many GB free; 0: always
	savedBelow float64
	editBelow  bool   // the GB field is open in the status area
	belowIn    string // what has been typed in it
	days       []string
	savedDays  []string
	editDays   bool   // the days field is open in the status area
	daysIn     string // what has been typed in it
	machine    string // the data d would add, in words
	share      bool   // d is on

	asking  bool
	cancel  context.CancelFunc
	rec     *Recommendation
	recErr  string
	why     map[string]string
	confirm bool // q pressed with unsaved changes
	status  string

	done, cancelled bool
}

// NewRoutineEditor builds the editor.
func NewRoutineEditor(set RoutineSetup) *RoutineEditor {
	e := &RoutineEditor{steps: set.Steps, byID: map[string]RoutineStep{}, recommend: set.Recommend, why: map[string]string{},
		stops: map[string]bool{}, savedStops: map[string]bool{}, below: max(0, set.OnlyBelowFreeGB), width: 100, height: 30}
	if set.Recommend != nil {
		e.machine = set.MachineData
	}
	for _, s := range set.Steps {
		e.byID[s.ID] = s
	}
	for _, id := range set.Current {
		if _, ok := e.byID[id]; ok && !slices.Contains(e.order, id) {
			e.order = append(e.order, id)
		}
	}
	for _, id := range set.StopOnFail {
		if slices.Contains(e.order, id) {
			e.stops[id], e.savedStops[id] = true, true
		}
	}
	if days, err := config.ParseDays(strings.Join(set.Days, ",")); err == nil {
		e.days = days
	}
	e.saved = slices.Clone(e.order)
	e.savedBelow = e.below
	e.savedDays = slices.Clone(e.days)
	return e
}

// Routine is the routine as it stands.
func (e *RoutineEditor) Routine() []string { return slices.Clone(e.order) }

// Choice is the routine and its options as they stand.
func (e *RoutineEditor) Choice() RoutineChoice {
	c := RoutineChoice{Steps: e.Routine(), OnlyBelowFreeGB: e.below, Days: slices.Clone(e.days)}
	for _, id := range e.order {
		if e.stops[id] {
			c.StopOnFail = append(c.StopOnFail, id)
		}
	}
	return c
}

// SharesData reports whether the machine's data goes with a recommendation.
func (e *RoutineEditor) SharesData() bool { return e.share }

// row is one line of the list: a routine entry, an entry to add, or a
// heading the cursor passes over.
type routineRow struct {
	id      string
	heading string
	inOrder bool
	pos     int // place in the routine, from 0
}

func (e *RoutineEditor) rows() []routineRow {
	var rows []routineRow
	rows = append(rows, routineRow{heading: "routine"})
	for i, id := range e.order {
		rows = append(rows, routineRow{id: id, inOrder: true, pos: i})
	}
	cat := ""
	for _, s := range e.steps {
		if slices.Contains(e.order, s.ID) {
			continue
		}
		if s.Category != cat {
			cat = s.Category
			rows = append(rows, routineRow{heading: "cat." + cat})
		}
		rows = append(rows, routineRow{id: s.ID})
	}
	return rows
}

// selectable are the indexes of rows the cursor can stand on.
func selectable(rows []routineRow) []int {
	var out []int
	for i, r := range rows {
		if r.id != "" {
			out = append(out, i)
		}
	}
	return out
}

func (e *RoutineEditor) current() (routineRow, bool) {
	sel := selectable(e.rows())
	if len(sel) == 0 {
		return routineRow{}, false
	}
	e.cursor = max(0, min(e.cursor, len(sel)-1))
	return e.rows()[sel[e.cursor]], true
}

// focus puts the cursor on the step id.
func (e *RoutineEditor) focus(id string) {
	rows := e.rows()
	for n, i := range selectable(rows) {
		if rows[i].id == id {
			e.cursor = n
			return
		}
	}
}

func (e *RoutineEditor) dirty() bool {
	if !slices.Equal(e.order, e.saved) || e.below != e.savedBelow || !slices.Equal(e.days, e.savedDays) {
		return true
	}
	for _, id := range e.order {
		if e.stops[id] != e.savedStops[id] {
			return true
		}
	}
	return false
}

// toggleStop turns "stop the run if this fails" on or off for the step
// under the cursor, which must be in the routine.
func (e *RoutineEditor) toggleStop() {
	r, ok := e.current()
	if !ok {
		return
	}
	if !r.inOrder {
		e.status = i18n.T("custom.ui_stop_add_first")
		return
	}
	e.stops[r.id] = !e.stops[r.id]
	if e.stops[r.id] {
		e.status = i18n.T("custom.ui_stop_on", r.id)
	} else {
		e.status = i18n.T("custom.ui_stop_off", r.id)
	}
}

// belowKey handles a key while the GB field is open: digits, one decimal
// point, backspace, enter to keep and esc to leave it as it was.
func (e *RoutineEditor) belowKey(key string) {
	switch key {
	case "esc":
		e.editBelow = false
	case "enter":
		in := strings.TrimSpace(e.belowIn)
		if in == "" {
			e.below, e.editBelow = 0, false
			e.status = i18n.T("custom.ui_below_none")
			return
		}
		v, err := strconv.ParseFloat(in, 64)
		if err != nil || v < 0 {
			e.status = i18n.T("custom.ui_below_bad", in)
			return
		}
		e.below, e.editBelow = v, false
		e.status = i18n.T("custom.opt_below", v)
	case "backspace", "ctrl+h":
		if n := len(e.belowIn); n > 0 {
			e.belowIn = e.belowIn[:n-1]
		}
	default:
		for _, c := range key {
			switch {
			case c >= '0' && c <= '9':
			case (c == '.' || c == ',') && !strings.Contains(e.belowIn, "."):
				c = '.'
			default:
				return
			}
			if len(e.belowIn) < 6 {
				e.belowIn += string(c)
			}
		}
	}
}

// daysKey handles a key while the days field is open: any day names, in
// any of omega's languages, enter to keep and esc to leave them as they were.
func (e *RoutineEditor) daysKey(k tea.KeyMsg) {
	switch k.String() {
	case "esc":
		e.editDays = false
	case "enter":
		days, err := config.ParseDays(e.daysIn)
		if err != nil {
			var bad config.BadDayError
			if errors.As(err, &bad) {
				e.status = i18n.T("custom.ui_days_bad", bad.Word)
			}
			return
		}
		e.days, e.editDays = days, false
		if len(days) == 0 {
			e.status = i18n.T("custom.ui_days_none")
		} else {
			e.status = i18n.T("custom.opt_days", DaysText(days))
		}
	case "backspace", "ctrl+h":
		if r := []rune(e.daysIn); len(r) > 0 {
			e.daysIn = string(r[:len(r)-1])
		}
	case " ":
		if len([]rune(e.daysIn)) < 60 {
			e.daysIn += " "
		}
	default:
		if k.Type != tea.KeyRunes {
			return
		}
		for _, c := range k.Runes {
			if len([]rune(e.daysIn)) < 60 && (unicode.IsLetter(c) || c == ',' || c == '-' || c == '.') {
				e.daysIn += string(c)
			}
		}
	}
}

// DaysText says weekday ids (mon … sun) with the short names of the
// user's language: "lun, mié, vie".
func DaysText(days []string) string {
	names := strings.Split(i18n.T("custom.weekdays"), ",")
	var out []string
	for _, id := range days {
		if n := slices.Index(config.Weekdays, id); n >= 0 && n < len(names) {
			out = append(out, names[n])
		} else {
			out = append(out, id)
		}
	}
	return strings.Join(out, ", ")
}

// gbText writes a number of GB without needless decimals: 30, 12.5.
func gbText(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

func (e *RoutineEditor) toggle() {
	r, ok := e.current()
	if !ok {
		return
	}
	if r.inOrder {
		e.order = slices.Delete(e.order, r.pos, r.pos+1)
		// Stay on the routine where the step was, so several can be taken
		// out in a row.
		if len(e.order) > 0 {
			e.focus(e.order[min(r.pos, len(e.order)-1)])
		} else {
			e.focus(r.id)
		}
		return
	}
	e.order = append(e.order, r.id)
	// Stay in the list below, on the next step to add.
	rows := e.rows()
	sel := selectable(rows)
	if e.cursor >= len(sel) {
		e.cursor = len(sel) - 1
	}
	if e.cursor < len(e.order) && len(sel) > len(e.order) {
		e.cursor = len(e.order)
	}
}

func (e *RoutineEditor) move(delta int) {
	r, ok := e.current()
	if !ok || !r.inOrder {
		return
	}
	to := r.pos + delta
	if to < 0 || to >= len(e.order) {
		return
	}
	e.order[r.pos], e.order[to] = e.order[to], e.order[r.pos]
	e.focus(r.id)
}

type recommendMsg struct {
	rec Recommendation
	err error
}

// Init starts the editor.
func (e *RoutineEditor) Init() tea.Cmd { return statsTick() }

// Update handles keys, the window size and the model's reply.
func (e *RoutineEditor) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		e.width, e.height = msg.Width, msg.Height
	case statsTickMsg:
		return e, statsTick()
	case recommendMsg:
		e.asking = false
		if msg.err != nil {
			e.recErr = msg.err.Error()
			return e, nil
		}
		rec := msg.rec
		e.rec = &rec
	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				e.cursor = max(0, e.cursor-1)
			case tea.MouseButtonWheelDown:
				e.cursor++
				e.current()
			}
		}
	case tea.KeyMsg:
		return e.key(msg)
	}
	return e, nil
}

func (e *RoutineEditor) key(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := k.String()
	e.status = ""
	if key == "ctrl+c" {
		e.stopAsking()
		e.cancelled, e.done = true, true
		return e, tea.Quit
	}
	if e.editBelow {
		e.belowKey(key)
		return e, nil
	}
	if e.editDays {
		e.daysKey(k)
		return e, nil
	}
	if e.confirm {
		e.confirm = false
		switch key {
		case "s", "y", "enter":
			e.done = true
			return e, tea.Quit
		case "n", "q", "d":
			e.cancelled, e.done = true, true
			return e, tea.Quit
		}
		return e, nil
	}
	if e.rec != nil {
		switch key {
		case "a", "enter":
			e.order = slices.Clone(e.rec.Steps)
			for id, why := range e.rec.Why {
				e.why[id] = why
			}
			e.rec = nil
			e.cursor = 0
			e.status = i18n.T("custom.ui_applied")
		case "esc", "q", "n":
			for id, why := range e.rec.Why {
				e.why[id] = why
			}
			e.rec = nil
		}
		return e, nil
	}
	switch key {
	case "up", "k":
		e.cursor = max(0, e.cursor-1)
	case "down", "j":
		e.cursor++
		e.current()
	case "home", "g":
		e.cursor = 0
	case "end", "G":
		e.cursor = len(selectable(e.rows())) - 1
	case "pgup":
		e.cursor = max(0, e.cursor-10)
	case "pgdown":
		e.cursor += 10
		e.current()
	case " ", "x":
		e.toggle()
	case "shift+up", "K", "-", "alt+up":
		e.move(-1)
	case "shift+down", "J", "+", "alt+down":
		e.move(1)
	case "c":
		e.order = nil
		e.cursor = 0
	case "u":
		e.order = slices.Clone(e.saved)
		e.below = e.savedBelow
		e.days = slices.Clone(e.savedDays)
		e.stops = map[string]bool{}
		for id, on := range e.savedStops {
			e.stops[id] = on
		}
	case "i":
		if e.recommend != nil && !e.asking {
			e.asking, e.recErr = true, ""
			ctx, cancel := context.WithCancel(context.Background())
			e.cancel = cancel
			current := e.Routine()
			rec, share := e.recommend, e.share
			return e, func() tea.Msg {
				r, err := rec(ctx, current, share)
				return recommendMsg{r, err}
			}
		}
	case "f":
		e.toggleStop()
	case "b":
		e.editBelow = true
		e.belowIn = ""
		if e.below > 0 {
			e.belowIn = gbText(e.below)
		}
	case "w":
		e.editDays = true
		e.daysIn = DaysText(e.days)
	case "d":
		if e.machine != "" {
			e.share, e.recErr = !e.share, ""
			if e.share {
				e.status = i18n.T("custom.ui_share_on", e.machine)
			} else {
				e.status = i18n.T("custom.ui_share_off")
			}
		}
	case "enter", "s", "ctrl+s":
		if len(e.order) == 0 {
			e.status = i18n.T("custom.ui_empty")
			return e, nil
		}
		e.stopAsking()
		e.done = true
		return e, tea.Quit
	case "esc", "q":
		if e.asking {
			e.stopAsking()
			return e, nil
		}
		if e.dirty() && len(e.order) > 0 {
			e.confirm = true
			return e, nil
		}
		e.cancelled, e.done = true, true
		return e, tea.Quit
	}
	return e, nil
}

func (e *RoutineEditor) stopAsking() {
	if e.cancel != nil {
		e.cancel()
	}
	e.asking = false
}

// riskMark is the coloured risk word used in the list.
func riskMark(risk string) string {
	switch risk {
	case "read-only":
		return okStyle.Render(i18n.T("risk.short_ro"))
	case "dangerous":
		return warnStyle.Render(i18n.T("risk.short_danger"))
	}
	return mutedStyle.Render(i18n.T("risk.short_normal"))
}

// View draws the editor.
func (e *RoutineEditor) View() string {
	if e.done {
		return ""
	}
	w := max(30, min(e.width, 110))
	fw := max(30, e.width) // the chrome takes the whole width; the list stays readable
	inner := w - 4
	rule := chromeRule(fw)

	// Header.
	dangerous := 0
	for _, id := range e.order {
		if e.byID[id].Risk == "dangerous" {
			dangerous++
		}
	}
	right := mutedStyle.Render(i18n.T("custom.ui_count", len(e.order)))
	if dangerous > 0 {
		right += "  " + warnStyle.Render(i18n.T("custom.ui_dangerous", dangerous))
	}
	if e.dirty() {
		right += "  " + warnStyle.Render(gl.Running) + mutedStyle.Render(" "+i18n.T("custom.ui_unsaved"))
	}
	header := topBar("custom", []string{i18n.T("custom.ui_title")}, right, fw) + "\n" + statsLine(fw)

	// Bottom: details of the step under the cursor, then the keys.
	var foot []string
	if r, ok := e.current(); ok {
		s := e.byID[r.id]
		detail := " " + cmdStyle.Render(s.ID) + mutedStyle.Render("  "+s.Label+"  "+gl.Sep+" ") + riskMark(s.Risk)
		if r.inOrder && e.stops[s.ID] {
			detail += mutedStyle.Render("  " + gl.Sep + " " + i18n.T("custom.stops"))
		}
		foot = append(foot, detail)
		foot = append(foot, " "+faintStyle.Render("$ ")+mutedStyle.Render(truncate(s.Command, inner-2)))
		if why := e.why[s.ID]; why != "" {
			for i, l := range wrapText(why, inner-6) {
				p := "   "
				if i == 0 {
					p = " " + titleStyle.Render(gl.Note) + " "
				}
				foot = append(foot, p+mutedStyle.Render(l))
			}
		}
	}
	keyHints := []hint{hk("↑↓", "uix.move"), hk("space", "uix.add_remove"), hk("shift+↑↓/J/K", "uix.reorder"),
		hk("c", "uix.empty"), hk("u", "uix.undo"), hk("enter", "uix.save").must(), hk("esc", "uix.leave").must()}
	keyHints = append(keyHints, hk("f", "custom.ui_key_stop"), hk("b", "custom.ui_key_below"), hk("w", "custom.ui_key_days"))
	if e.recommend != nil {
		keyHints = append(keyHints, hk("i", "uix.ask_ai"))
	}
	if e.machine != "" {
		state := i18n.T("custom.ui_off")
		if e.share {
			state = i18n.T("custom.ui_on")
		}
		keyHints = append(keyHints, hk("d", "custom.ui_key_share", state))
	}
	var status string
	statusLines := 1
	switch {
	case e.editBelow:
		prompt := i18n.T("custom.ui_below_prompt")
		status = titleStyle.Render(prompt+" ") + cmdStyle.Render(e.belowIn+"_") + " GB" +
			faintStyle.Render("  "+truncate(i18n.T("custom.ui_below_hint"), max(0, inner-lipgloss.Width(prompt)-len(e.belowIn)-8)))
	case e.editDays:
		prompt := i18n.T("custom.ui_days_prompt")
		status = titleStyle.Render(prompt+" ") + cmdStyle.Render(e.daysIn+"_") +
			faintStyle.Render("  "+truncate(i18n.T("custom.ui_days_hint"), max(0, inner-lipgloss.Width(prompt)-lipgloss.Width(e.daysIn)-4)))
	case e.confirm:
		status = warnStyle.Render(i18n.T("uix.unsaved")) + " " + strings.TrimLeft(cardHints(inner-lipgloss.Width(i18n.T("uix.unsaved"))-1,
			hk("s", "uix.save").must(), hk("n", "uix.discard").must(), hint{action: i18n.T("uix.keep_editing")}), " ")
	case e.asking:
		status = titleStyle.Render(gl.Note+" ") + mutedStyle.Render(truncate(i18n.T("custom.ui_asking"), inner-2))
	case e.recErr != "":
		status = errStyle.Render(gl.Cross + " " + truncate(i18n.T("custom.ui_ai_failed", e.recErr), inner))
	case e.status != "":
		// What d sends is said whole, over as many lines as it takes.
		ls := wrapText(e.status, inner-2)
		ls = ls[:min(len(ls), 3)]
		statusLines = len(ls)
		status = okStyle.Render(gl.Check+" ") + mutedStyle.Render(ls[0])
		for _, l := range ls[1:] {
			status += "\n   " + mutedStyle.Render(l)
		}
	}

	// The recommendation, when one has come, takes the list's place.
	if e.rec != nil {
		return e.recView(w, header, rule)
	}

	// The list, scrolled so the cursor stays in sight.
	rows := e.rows()
	sel := selectable(rows)
	curRow := -1
	if len(sel) > 0 {
		curRow = sel[e.cursor]
	}
	labelW, idW, riskW, statsW := 0, 0, 0, 0
	stats := map[string]string{}
	for _, r := range rows {
		if r.id != "" {
			s := e.byID[r.id]
			idW = max(idW, lipgloss.Width(r.id))
			labelW = max(labelW, lipgloss.Width(s.Label))
			riskW = max(riskW, lipgloss.Width(riskMark(s.Risk)))
			if st := stepStats(s); st != "" {
				stats[r.id] = st
				statsW = max(statsW, lipgloss.Width(st))
			}
		}
	}
	stopTag := i18n.T("custom.ui_stop_tag")
	stopW := 0
	for _, id := range e.order {
		if e.stops[id] {
			stopW = lipgloss.Width(stopTag)
		}
	}
	idW = min(idW, 20)
	// pointer, mark and AI note take 8 columns; two spaces before each of
	// label, risk and the optional stop tag and figures.
	room := inner - 8 - idW - 2 - 2 - riskW
	if stopW > 0 {
		room -= 2 + stopW
	}
	if statsW > 0 && room-2-statsW < min(labelW, 12) {
		statsW = 0 // no room for the figures: the label comes first
	}
	if statsW > 0 {
		room -= 2 + statsW
	}
	labelW = min(labelW, max(6, room))
	var lines []string
	cursorLine, addShown := 0, false
	for i, r := range rows {
		if r.heading == "routine" {
			head := " " + titleStyle.Render(strings.ToUpper(i18n.T("custom.ui_routine"))) + faintStyle.Render("  "+i18n.T("custom.ui_routine_hint"))
			if e.below > 0 {
				head += mutedStyle.Render("  " + gl.Sep + " " + i18n.T("custom.opt_below", e.below))
			}
			if len(e.days) > 0 {
				head += mutedStyle.Render("  " + gl.Sep + " " + i18n.T("custom.opt_days", DaysText(e.days)))
			}
			lines = append(lines, head)
			if len(e.order) == 0 {
				lines = append(lines, "   "+mutedStyle.Render(i18n.T("custom.ui_routine_empty")))
			}
			continue
		}
		if r.heading != "" {
			if !addShown {
				addShown = true
				lines = append(lines, "", " "+titleStyle.Render(strings.ToUpper(i18n.T("custom.ui_add")))+faintStyle.Render("  "+i18n.T("custom.ui_add_hint")))
			}
			lines = append(lines, "   "+faintStyle.Render(i18n.T(r.heading)))
			continue
		}
		s := e.byID[r.id]
		pointer := "  "
		if i == curRow {
			pointer = titleStyle.Render(gl.Pointer + " ")
			cursorLine = len(lines)
		}
		var mark string
		if r.inOrder {
			mark = titleStyle.Render(fmt.Sprintf("%2d", r.pos+1))
		} else {
			mark = mutedStyle.Render(" " + gl.BoxOff)
		}
		id := pad(truncate(s.ID, idW), idW)
		label := pad(truncate(s.Label, labelW), labelW)
		if i == curRow {
			id = cmdStyle.Render(id)
		}
		ai := " "
		if e.why[s.ID] != "" {
			ai = titleStyle.Render(gl.Note)
		}
		line := " " + pointer + mark + " " + ai + " " + id + "  " + mutedStyle.Render(label) + "  " + riskMark(s.Risk)
		stopped := r.inOrder && e.stops[s.ID]
		if stopped || (statsW > 0 && stats[s.ID] != "") {
			line += strings.Repeat(" ", riskW-lipgloss.Width(riskMark(s.Risk)))
			if stopW > 0 {
				tag := strings.Repeat(" ", stopW)
				if stopped {
					tag = mutedStyle.Render(pad(stopTag, stopW))
				}
				line += "  " + tag
			}
			if statsW > 0 && stats[s.ID] != "" {
				line += "  " + faintStyle.Render(stats[s.ID])
			}
		}
		lines = append(lines, line)
	}
	if len(e.steps) == 0 {
		lines = append(lines, "   "+mutedStyle.Render(i18n.T("custom.ui_no_steps")))
	}

	keyLines := strings.Split(hintBlock(fw, keyHints...), "\n")
	fixed := 2 + 1 + 1 + len(foot) + 1 + len(keyLines) + statusLines // header and stats, rule, rule, foot, rule, keys, status
	if e.height > 0 && e.height-fixed < 3 {
		// A short terminal: the keys that may go give the list its room.
		keyLines = strings.Split(hintLine(fw, keyHints...), "\n")
		fixed = 2 + 1 + 1 + len(foot) + 1 + len(keyLines) + statusLines
	}
	avail := max(3, e.height-fixed)
	// Keep the cursor's line in view.
	if cursorLine < e.offset {
		e.offset = max(0, cursorLine-1)
	}
	if cursorLine >= e.offset+avail {
		e.offset = cursorLine - avail + 1
	}
	e.offset = max(0, min(e.offset, max(0, len(lines)-avail)))
	view := lines[e.offset:min(len(lines), e.offset+avail)]
	for len(view) < avail {
		view = append(view, "")
	}
	midRule := rule
	if n := len(lines) - e.offset - avail; n > 0 {
		more := " " + gl.Down + " " + i18n.T("uix.more", n) + " "
		midRule = faintStyle.Render(strings.Repeat(gl.Rule, max(0, fw-lipgloss.Width(more)-2)) + more + strings.Repeat(gl.Rule, 2))
	}

	var b strings.Builder
	b.WriteString(header + "\n" + rule + "\n")
	b.WriteString(strings.Join(view, "\n") + "\n")
	b.WriteString(midRule + "\n")
	b.WriteString(strings.Join(foot, "\n") + "\n")
	b.WriteString(rule + "\n")
	for _, l := range keyLines {
		b.WriteString(l + "\n")
	}
	b.WriteString(" " + status)
	return b.String()
}

// stepStats are the figures of the step's last run: "1m05s · 2.1 GB".
func stepStats(s RoutineStep) string {
	var parts []string
	if s.LastRun > 0 {
		parts = append(parts, Duration(s.LastRun))
	}
	if s.Freed > 0 {
		parts = append(parts, Bytes(s.Freed))
	}
	return strings.Join(parts, " "+gl.Sep+" ")
}

// recView shows the model's recommendation next to the routine as it is.
func (e *RoutineEditor) recView(w int, header, rule string) string {
	inner := w - 4
	var b strings.Builder
	b.WriteString(header + "\n" + rule + "\n")
	b.WriteString(" " + titleStyle.Render(gl.Note+" "+i18n.T("custom.ui_rec_title")) + "\n\n")
	for _, l := range wrapText(e.rec.Summary, inner-2) {
		b.WriteString("   " + l + "\n")
	}
	b.WriteString("\n")
	idW := 0
	for _, id := range e.rec.Steps {
		idW = max(idW, lipgloss.Width(id))
	}
	for n, id := range e.rec.Steps {
		tag := "  "
		if !slices.Contains(e.order, id) {
			tag = okStyle.Render("+ ")
		}
		line := fmt.Sprintf("   %s%s %s  ", tag, titleStyle.Render(fmt.Sprintf("%2d", n+1)), cmdStyle.Render(pad(id, idW)))
		why := e.rec.Why[id]
		ws := wrapText(why, max(20, inner-lipgloss.Width(line)))
		b.WriteString(line + mutedStyle.Render(ws[0]) + "\n")
		for _, l := range ws[1:] {
			b.WriteString(strings.Repeat(" ", lipgloss.Width(line)) + mutedStyle.Render(l) + "\n")
		}
	}
	var removed []string
	for _, id := range e.order {
		if !slices.Contains(e.rec.Steps, id) {
			removed = append(removed, id)
		}
	}
	if len(removed) > 0 {
		b.WriteString("\n   " + errStyle.Render("− ") + mutedStyle.Render(i18n.T("custom.ui_rec_removed", strings.Join(removed, ", "))) + "\n")
	}
	b.WriteString(rule + "\n")
	b.WriteString(hintBlock(max(30, e.width), hk("a/enter", "uix.use_rec").must(), hk("esc", "uix.keep_mine").must()))
	return b.String()
}

// EditRoutine runs the editor full screen and returns the routine the user
// saved, with its options. It returns ErrCancelled when the user leaves
// without saving.
func EditRoutine(set RoutineSetup) (RoutineChoice, error) {
	lipgloss.HasDarkBackground()
	defer watchStats()()
	e := NewRoutineEditor(set)
	opts := []tea.ProgramOption{tea.WithAltScreen()}
	if mouseWanted() {
		opts = append(opts, tea.WithMouseCellMotion())
	}
	if _, err := tea.NewProgram(e, opts...).Run(); err != nil {
		return RoutineChoice{}, err
	}
	if e.cancelled {
		return RoutineChoice{}, ErrCancelled
	}
	return e.Choice(), nil
}
