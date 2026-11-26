package ui

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/i18n"
)

// The review of omega ultra's plan: what the audit found, one action per
// row, grouped as the user thinks of them, and beside the list everything
// a person who did not watch the audit needs to decide: what was found,
// what will happen, what verified it and the exact steps that run.
//
//	 Ω omega  Action plan ULTRA  v0.36 · audit 14m              $1.24
//	 CPU ━━━─── 23%   RAM ━━━━── 62%   DISK ━━── 45%
//	 23 actions · 9 ticked · 2 high risk     frees 14.2 GB of 20.1 GB ━━━━━━──
//
//	 ACTIONS                                │ Clear the Homebrew cache
//	 ▾ Cleanup                 3/5 · 9.1 GB │ ● routine · ✦ recommended · ~8.9 GB
//	 ▌◼ ● Clear the Homebrew cache   8.9 GB │
//	   ◻ ▲ Delete Xcode DerivedData  4.1 GB │ WHAT THE AUDIT FOUND
//	 ▸ Docker                  1/2 · 6.2 GB │ ~/Library/Caches/Homebrew holds …
//	────────────────────────────────────────────────────────────────────────
//	  ↑↓ move · space tick · a all · n none · r recommended · enter run · q quit

// PlanReviewInfo is what the review shows about the audit behind it.
type PlanReviewInfo struct {
	Version string
	Flags   []string
	// Audit is how long the audit took and Cost what it cost.
	Audit     time.Duration
	Cost      float64
	CostKnown bool
	// StageTitle names an audit stage (a task) for the details.
	StageTitle func(task string) string
	// DryRun says the plan is only shown: enter ends the review without
	// running anything.
	DryRun bool
}

type reviewGroup struct {
	name      string
	acts      []int
	collapsed bool
}

// reviewRow is one line of the list: a group's header (act < 0) or an
// action.
type reviewRow struct{ group, act int }

type reviewModel struct {
	info   PlanReviewInfo
	acts   []agent.PlannedAction
	groups []reviewGroup
	sel    []bool

	cursor, top int // the row the cursor is on, the first row shown
	detail      int // lines of the details scrolled away

	width, height int
	confirming    bool
	quitting      bool
	warn          string // a short note under the summary: nothing ticked
	done, ok      bool
}

func newReviewModel(info PlanReviewInfo, acts []agent.PlannedAction) *reviewModel {
	m := &reviewModel{info: info, acts: acts, sel: make([]bool, len(acts))}
	at := map[string]int{}
	for i, a := range acts {
		name := strings.TrimSpace(a.Category)
		if name == "" && info.StageTitle != nil {
			name = info.StageTitle(a.Stage)
		}
		if name == "" {
			name = a.Stage
		}
		key := strings.ToLower(name)
		g, ok := at[key]
		if !ok {
			g = len(m.groups)
			at[key] = g
			m.groups = append(m.groups, reviewGroup{name: name})
		}
		m.groups[g].acts = append(m.groups[g].acts, i)
		m.sel[i] = a.Recommended
	}
	// The cursor starts on the first action, not on a header.
	if len(m.groups) > 0 {
		m.cursor = 1
	}
	return m
}

// RunPlanReview shows the plan and returns the ids of the actions the
// person ticked; ok is false when they left without running anything.
func RunPlanReview(info PlanReviewInfo, acts []agent.PlannedAction) (ids []string, ok bool, err error) {
	if len(acts) == 0 {
		return nil, false, nil
	}
	hostMu.Lock()
	defer hostMu.Unlock()
	lipgloss.HasDarkBackground()
	defer watchStats()()
	m := newReviewModel(info, acts)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithOutput(os.Stdout))
	if _, err := p.Run(); err != nil {
		return nil, false, err
	}
	if !m.ok {
		return nil, false, nil
	}
	return m.picked(), true, nil
}

func (m *reviewModel) picked() []string {
	var ids []string
	for i, on := range m.sel {
		if on {
			ids = append(ids, m.acts[i].ID)
		}
	}
	return ids
}

func (m *reviewModel) rows() []reviewRow {
	var out []reviewRow
	for g, grp := range m.groups {
		out = append(out, reviewRow{group: g, act: -1})
		if grp.collapsed {
			continue
		}
		for _, a := range grp.acts {
			out = append(out, reviewRow{group: g, act: a})
		}
	}
	return out
}

// current is the action under the cursor, or the first of the group whose
// header it is on; -1 when there is none.
func (m *reviewModel) current() int {
	rows := m.rows()
	if m.cursor < 0 || m.cursor >= len(rows) {
		return -1
	}
	r := rows[m.cursor]
	if r.act >= 0 {
		return r.act
	}
	if acts := m.groups[r.group].acts; len(acts) > 0 {
		return acts[0]
	}
	return -1
}

func (m *reviewModel) Init() tea.Cmd {
	return tea.Batch(statsTick(), tea.SetWindowTitle(gl.Brand+" omega ultra "+gl.Sep+" "+i18n.T("plan.title")))
}

func (m *reviewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case statsTickMsg:
		return m, statsTick()
	case tea.KeyMsg:
		return m.key(msg.String())
	}
	return m, nil
}

func (m *reviewModel) key(k string) (tea.Model, tea.Cmd) {
	if k == "ctrl+c" {
		m.done = true
		return m, tea.Quit
	}
	if m.confirming {
		switch k {
		case "enter", "y", "s", "o", "j":
			m.ok, m.done = true, true
			return m, tea.Quit
		case "esc", "n", "q", "backspace":
			m.confirming = false
		}
		return m, nil
	}
	if m.quitting {
		switch k {
		case "enter", "y", "s", "o", "j", "q":
			m.done = true
			return m, tea.Quit
		case "esc", "n", "backspace":
			m.quitting = false
		}
		return m, nil
	}
	m.warn = ""
	rows := m.rows()
	move := func(d int) {
		m.cursor = max(0, min(len(rows)-1, m.cursor+d))
		m.detail = 0
	}
	switch k {
	case "up", "k":
		move(-1)
	case "down", "j":
		move(1)
	case "pgup":
		move(-max(1, m.listHeight()-1))
	case "pgdown":
		move(max(1, m.listHeight()-1))
	case "home", "g":
		move(-len(rows))
	case "end", "G":
		move(len(rows))
	case "tab", "shift+tab":
		if len(rows) == 0 {
			break
		}
		g := rows[m.cursor].group
		if k == "tab" {
			g = (g + 1) % len(m.groups)
		} else {
			g = (g - 1 + len(m.groups)) % len(m.groups)
		}
		for i, r := range rows {
			if r.group == g && r.act < 0 {
				m.cursor, m.detail = i, 0
				if !m.groups[g].collapsed && i+1 < len(rows) {
					m.cursor = i + 1
				}
			}
		}
	case "left", "h":
		if len(rows) == 0 {
			break
		}
		g := rows[m.cursor].group
		m.groups[g].collapsed = true
		for i, r := range m.rows() {
			if r.group == g && r.act < 0 {
				m.cursor = i
			}
		}
	case "right", "l":
		if len(rows) > 0 && rows[m.cursor].act < 0 {
			m.groups[rows[m.cursor].group].collapsed = false
		}
	case " ", "x":
		if len(rows) == 0 {
			break
		}
		r := rows[m.cursor]
		if r.act >= 0 {
			m.sel[r.act] = !m.sel[r.act]
			break
		}
		// On a header: the whole group, on unless all of it is on.
		all := true
		for _, a := range m.groups[r.group].acts {
			all = all && m.sel[a]
		}
		for _, a := range m.groups[r.group].acts {
			m.sel[a] = !all
		}
	case "a":
		for i := range m.sel {
			m.sel[i] = true
		}
	case "n":
		for i := range m.sel {
			m.sel[i] = false
		}
	case "r":
		for i := range m.sel {
			m.sel[i] = m.acts[i].Recommended
		}
	case "J", "shift+down", "ctrl+d":
		m.detail++
	case "K", "shift+up", "ctrl+u":
		m.detail = max(0, m.detail-1)
	case "enter":
		if len(m.picked()) == 0 {
			m.warn = i18n.T("plan.none_ticked")
			break
		}
		m.confirming = true
	case "q", "esc":
		if len(m.picked()) == 0 {
			m.done = true
			return m, tea.Quit
		}
		m.quitting = true
	}
	return m, nil
}

// ---------- view ----------

func (m *reviewModel) totals() (ticked, risky int, freed, possible int64) {
	for i, a := range m.acts {
		possible += a.Bytes
		if !m.sel[i] {
			continue
		}
		ticked++
		freed += a.Bytes
		if a.Risk == "dangerous" {
			risky++
		}
	}
	return
}

func (m *reviewModel) headLine(w int) string {
	info := []string{}
	if m.info.Version != "" {
		info = append(info, m.info.Version)
	}
	info = append(info, m.info.Flags...)
	right := []string{}
	if m.info.Audit > 0 {
		right = append(right, i18n.T("plan.audit_took", humanDuration(m.info.Audit)))
	}
	if m.info.CostKnown && m.info.Cost > 0 {
		right = append(right, FormatCost(m.info.Cost))
	}
	return topBar(i18n.T("plan.title")+" "+ultraBadge.Render(" ULTRA "), info, rightInfo(right...), w)
}

// summaryLine is the plan in figures: how many, how many ticked, how many
// of those are high risk, and the space the ticked ones free with a gauge.
func (m *reviewModel) summaryLine(w int) string {
	ticked, risky, freed, possible := m.totals()
	left := " " + cmdStyle.Render(i18n.T("plan.n_actions", len(m.acts))) + faintStyle.Render(" "+gl.Sep+" ") +
		titleStyle.Render(fmt.Sprintf("%s %s", gl.BoxOn, i18n.T("plan.n_ticked", ticked)))
	if risky > 0 {
		left += faintStyle.Render(" "+gl.Sep+" ") + warnPlainStyle.Render("▲ "+i18n.T("plan.n_risky", risky))
	}
	if m.warn != "" {
		left += "   " + warnStyle.Render(m.warn)
	}
	if possible <= 0 {
		return left
	}
	label := i18n.T("plan.frees", Bytes(freed), Bytes(possible))
	bar := 16
	if w < 100 {
		bar = 8
	}
	fill := int(float64(bar) * float64(freed) / float64(possible))
	if freed > 0 && fill == 0 {
		fill = 1
	}
	gauge := okStyle.Render(strings.Repeat(gl.Heavy, fill)) + faintStyle.Render(strings.Repeat(gl.Rule, bar-fill))
	right := mutedStyle.Render(label) + " " + gauge + " "
	if lipgloss.Width(left)+lipgloss.Width(right)+2 > w {
		right = mutedStyle.Render(Bytes(freed)) + " "
	}
	return spread(left, right, w)
}

func (m *reviewModel) splitWide() bool { return m.width >= 96 }

// listHeight is how many rows the list shows.
func (m *reviewModel) listHeight() int {
	body := m.height - 4 - 3 // top lines, footer
	if m.splitWide() {
		return max(3, body-1)
	}
	return max(3, body/2-1)
}

func planRiskMark(risk string) string {
	if risk == "dangerous" {
		return warnPlainStyle.Render("▲")
	}
	return okStyle.Render("●")
}

func (m *reviewModel) impact(a agent.PlannedAction) string {
	if a.Bytes > 0 {
		return Bytes(a.Bytes)
	}
	return a.Impact
}

func (m *reviewModel) listLines(w, h int) []string {
	rows := m.rows()
	if m.cursor >= len(rows) {
		m.cursor = max(0, len(rows)-1)
	}
	// Keep the cursor in view.
	if m.cursor < m.top {
		m.top = m.cursor
	}
	if m.cursor >= m.top+h-1 {
		m.top = m.cursor - h + 2
	}
	m.top = max(0, min(m.top, max(0, len(rows)-(h-1))))
	head := " " + faintStyle.Render(i18n.T("plan.list_head"))
	if len(rows) > h-1 {
		head = spread(head, faintStyle.Render(fmt.Sprintf("%d–%d/%d", m.top+1, min(len(rows), m.top+h-1), len(rows))), w)
	}
	lines := []string{head}
	for i := m.top; i < len(rows) && len(lines) < h; i++ {
		r := rows[i]
		focus := i == m.cursor
		if r.act < 0 {
			lines = append(lines, m.groupLine(r.group, w, focus))
			continue
		}
		lines = append(lines, m.actionLine(r.act, w, focus))
	}
	return lines
}

func (m *reviewModel) groupLine(g, w int, focus bool) string {
	grp := m.groups[g]
	on := 0
	var bytes int64
	for _, a := range grp.acts {
		if m.sel[a] {
			on++
			bytes += m.acts[a].Bytes
		}
	}
	fold := "▾"
	if grp.collapsed {
		fold = "▸"
	}
	if gl.Up != "↑" {
		fold = map[bool]string{true: "+", false: "-"}[grp.collapsed]
	}
	lead := " "
	if focus {
		lead = titleStyle.Render("▌")
	}
	name := titleStyle.Render(fold + " " + strings.ToUpper(grp.name))
	count := fmt.Sprintf("%d/%d", on, len(grp.acts))
	if bytes > 0 {
		count += " " + gl.Sep + " " + Bytes(bytes)
	}
	return spread(lead+name, mutedStyle.Render(count)+" ", w)
}

func (m *reviewModel) actionLine(i, w int, focus bool) string {
	a := m.acts[i]
	box := faintStyle.Render(gl.BoxOff)
	if m.sel[i] {
		box = titleStyle.Render(gl.BoxOn)
	}
	lead := "  "
	title := a.Title
	if focus {
		lead = titleStyle.Render("▌") + " "
		title = cmdStyle.Render(title)
	} else if !m.sel[i] {
		title = mutedStyle.Render(title)
	}
	rec := ""
	if a.Recommended {
		rec = " " + dimStyle.Render(gl.Note)
	}
	left := lead + box + " " + planRiskMark(a.Risk) + " " + title + rec
	return spread(left, mutedStyle.Render(m.impact(a))+" ", w)
}

// detailLines is everything about the action under the cursor.
func (m *reviewModel) detailLines(w int) []string {
	i := m.current()
	if i < 0 {
		return nil
	}
	a := m.acts[i]
	tw := max(10, w-1)
	var out []string
	for _, l := range wrapText(a.Title, tw) {
		out = append(out, cmdStyle.Render(l))
	}
	tags := []string{}
	if a.Risk == "dangerous" {
		tags = append(tags, warnPlainStyle.Render("▲ "+i18n.T("plan.risk_dangerous")))
	} else {
		tags = append(tags, okStyle.Render("● "+i18n.T("plan.risk_normal")))
	}
	if a.Recommended {
		tags = append(tags, titleStyle.Render(gl.Note+" "+i18n.T("plan.recommended")))
	}
	if a.Reversible {
		tags = append(tags, mutedStyle.Render("↺ "+i18n.T("plan.reversible")))
	}
	if imp := m.impact(a); imp != "" {
		tags = append(tags, mutedStyle.Render(imp))
	}
	out = append(out, truncateANSI(strings.Join(tags, faintStyle.Render(" "+gl.Sep+" ")), w))
	riskWhy := i18n.T("plan.risk_normal_why")
	if a.Risk == "dangerous" {
		riskWhy = i18n.T("plan.risk_dangerous_why")
	}
	for _, l := range wrapText(riskWhy, tw) {
		out = append(out, faintStyle.Render(l))
	}
	section := func(head string, body string, style lipgloss.Style, mark string) {
		if strings.TrimSpace(body) == "" {
			return
		}
		out = append(out, "", faintStyle.Render(head))
		for n, l := range wrapText(body, tw-len([]rune(mark))) {
			if n == 0 {
				out = append(out, style.Render(mark+l))
			} else {
				out = append(out, style.Render(strings.Repeat(" ", len([]rune(mark)))+l))
			}
		}
	}
	section(i18n.T("plan.found_head"), a.Why, lipgloss.NewStyle(), "")
	section(i18n.T("plan.what_head"), a.What, lipgloss.NewStyle(), "")
	section(i18n.T("plan.evidence_head"), a.Evidence, okStyle, gl.Check+" ")
	if len(a.Steps) > 0 {
		out = append(out, "", faintStyle.Render(i18n.T("plan.steps_head")))
		for _, s := range a.Steps {
			for n, l := range wrapText(s.Show, tw-2) {
				lead := "$ "
				if n > 0 {
					lead = "  "
				}
				out = append(out, faintStyle.Render(lead)+cmdStyle.Render(l))
			}
		}
	}
	stage := a.Stage
	if m.info.StageTitle != nil {
		if t := m.info.StageTitle(a.Stage); t != "" {
			stage = t
		}
	}
	out = append(out, "", faintStyle.Render(i18n.T("plan.stage_head")+"  ")+mutedStyle.Render(stage+" "+gl.Sep+" "+a.ID))
	return out
}

// scrolled is the details from the line the user scrolled to, h lines.
func (m *reviewModel) scrolled(lines []string, h int) []string {
	if over := len(lines) - h; over > 0 {
		m.detail = min(m.detail, over)
		lines = lines[m.detail:]
		if len(lines) > h {
			lines = lines[:h]
			lines[h-1] = faintStyle.Render(gl.Down + " " + i18n.T("plan.more_details"))
		}
	} else {
		m.detail = 0
	}
	return lines
}

func (m *reviewModel) footer(w int) []string {
	switch {
	case m.confirming:
		return chromeFooter(w, "", hk("enter", "plan.k_run").must(), hk("esc", "uix.back").must())
	case m.quitting:
		return chromeFooter(w, "", hk("enter", "plan.k_leave").must(), hk("esc", "uix.back").must())
	}
	run := "plan.k_run"
	if m.info.DryRun {
		run = "plan.k_show"
	}
	note := ""
	if m.info.DryRun {
		note = warnPlainStyle.Render(i18n.T("plan.dry_note"))
	}
	return chromeFooter(w, note,
		hk("↑↓", "uix.move"), hk("space", "plan.k_toggle").must(), hk("enter", run).must(),
		hk("a", "plan.k_all"), hk("n", "plan.k_none"), hk("r", "plan.k_rec"),
		hk("tab", "plan.k_group"), hk("←→", "plan.k_fold"), hk("J/K", "plan.k_details"), hk("q", "plan.k_quit").must())
}

// modal is a card in the middle of the screen: the confirmation, or the
// question before leaving.
func (m *reviewModel) modal(w int) []string {
	cw := min(64, w-4)
	inner := cw - 4
	var body []string
	var title string
	style := confirmStyle
	if m.confirming {
		ticked, risky, freed, _ := m.totals()
		title = i18n.T("plan.confirm_title")
		if m.info.DryRun {
			title = i18n.T("plan.confirm_dry_title")
		}
		sum := i18n.T("plan.n_ticked", ticked)
		if freed > 0 {
			sum += " " + gl.Sep + " " + Bytes(freed)
		}
		if risky > 0 {
			sum += " " + gl.Sep + " " + i18n.T("plan.n_risky", risky)
			style = confirmWarnStyle
		}
		body = append(body, cmdStyle.Render(sum), "")
		if risky > 0 {
			body = append(body, warnPlainStyle.Render(i18n.T("plan.confirm_risky")))
			shown := 0
			for i, a := range m.acts {
				if m.sel[i] && a.Risk == "dangerous" {
					if shown == 5 {
						body = append(body, faintStyle.Render("  "+gl.Ellipsis))
						break
					}
					body = append(body, "  "+warnPlainStyle.Render("▲ ")+truncate(a.Title, inner-4))
					shown++
				}
			}
			body = append(body, "")
		}
		text := i18n.T("plan.confirm_body")
		if m.info.DryRun {
			text = i18n.T("plan.confirm_dry_body")
		}
		for _, l := range wrapText(text, inner) {
			body = append(body, mutedStyle.Render(l))
		}
	} else {
		title = i18n.T("plan.quit_title")
		for _, l := range wrapText(i18n.T("plan.quit_body"), inner) {
			body = append(body, mutedStyle.Render(l))
		}
	}
	card := style.Width(cw - 2).Render(titleStyle.Render(title) + "\n\n" + strings.Join(body, "\n"))
	var out []string
	pad := strings.Repeat(" ", max(0, (w-lipgloss.Width(strings.Split(card, "\n")[0]))/2))
	for _, l := range strings.Split(card, "\n") {
		out = append(out, pad+l)
	}
	return out
}

func (m *reviewModel) View() string {
	w, h := m.width, m.height
	if w <= 0 || h <= 0 || m.done {
		return ""
	}
	w = max(w, 30)
	top := []string{m.headLine(w), statsLine(w), m.summaryLine(w), chromeRule(w)}
	foot := m.footer(w)
	bodyH := max(4, h-len(top)-len(foot))
	var body []string
	switch {
	case m.confirming || m.quitting:
		card := m.modal(w)
		gap := max(0, (bodyH-len(card))/2)
		for i := 0; i < gap; i++ {
			body = append(body, "")
		}
		body = append(body, card...)
	case m.splitWide():
		listW := min(max(44, w*11/20), w-36)
		detW := w - listW - 3
		list := m.listLines(listW, bodyH)
		det := m.scrolled(append([]string{faintStyle.Render(i18n.T("plan.detail_head"))}, m.detailLines(detW)...), bodyH)
		for i := 0; i < bodyH; i++ {
			l, d := "", ""
			if i < len(list) {
				l = list[i]
			}
			if i < len(det) {
				d = det[i]
			}
			body = append(body, fit(l, listW)+" "+faintStyle.Render(gl.VBar)+" "+d)
		}
	default:
		listH := max(3, bodyH/2)
		list := m.listLines(w-1, listH)
		for len(list) < listH {
			list = append(list, "")
		}
		body = append(body, list...)
		body = append(body, chromeRule(w))
		det := m.scrolled(m.detailLines(w-2), bodyH-listH-1)
		for _, d := range det {
			body = append(body, " "+d)
		}
	}
	for len(body) < bodyH {
		body = append(body, "")
	}
	lines := append(append(top, body[:bodyH]...), foot...)
	if len(lines) > h {
		lines = lines[:h]
	}
	for i := range lines {
		lines[i] = truncateANSI(strings.TrimRight(lines[i], " "), w)
	}
	return strings.Join(lines, "\n")
}
