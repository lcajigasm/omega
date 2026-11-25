package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/lcajigasm/omega/internal/i18n"
)

// ---------- what is left on the terminal ----------

// reportMarkdown renders what a stage concluded for the terminal; the text
// as it was written when that cannot be done.
func reportMarkdown(s string, width int) string {
	style := "dark"
	if !lipgloss.HasDarkBackground() {
		style = "light"
	}
	r, err := glamour.NewTermRenderer(markdownStyle(style), glamour.WithColorProfile(lipgloss.ColorProfile()), glamour.WithWordWrap(min(max(width, 60), 110)-4), glamour.WithEmoji())
	if err != nil {
		return s
	}
	out, err := r.Render(s)
	if err != nil {
		return s
	}
	return strings.Trim(out, "\n")
}

// report is what an unattended run leaves on the terminal when its screen
// closes: for every stage that ran, what it concluded and what it could not
// do, then the mascot with the verdict.
func (m *pilotModel) report() []string {
	var lines []string
	res := m.results()
	failedStages, ran := 0, 0
	for i, s := range m.stages {
		r := res[i]
		if r.Status != StageDone && r.Status != StageFailed && r.Status != StageStopped {
			continue
		}
		ran++
		head := strings.Repeat(gl.Heavy, 2) + " " + s.Title + " " + gl.Sep + " " + StatusWord(r.Status)
		if r.Duration >= time.Second {
			head += " " + gl.Sep + " " + humanDuration(r.Duration)
		}
		style := titleStyle
		if r.Status == StageFailed {
			style = errStyle
			failedStages++
		}
		lines = append(lines, style.Render(head))
		st := m.st[i]
		switch {
		case st.said != "":
			lines = append(lines, strings.Split(reportMarkdown(st.said, m.width), "\n")...)
		case len(st.out) > 0:
			lines = append(lines, strings.Split(strings.Trim(strings.Join(st.out, "\n"), "\n"), "\n")...)
		}
		if r.Note != "" {
			lines = append(lines, errStyle.Render(gl.Error+" "+r.Note))
		}
		for _, a := range m.acts {
			if a.stage != i || (a.kind != actFailed && a.kind != actDenied && a.kind != actError) {
				continue
			}
			label := a.name
			if a.arg != "" {
				label += "(" + a.arg + ")"
			}
			switch a.kind {
			case actDenied:
				lines = append(lines, warnStyle.Render("  "+gl.Denied+" ")+label+mutedStyle.Render(" "+gl.Sep+" "+i18n.T("ui.denied")+" "+gl.Sep+" "+a.note))
			case actError:
				lines = append(lines, errStyle.Render("  "+gl.Error+" ")+label)
			default:
				if a.exit > 0 {
					label += mutedStyle.Render(fmt.Sprintf(" %s exit %d", gl.Sep, a.exit))
				}
				lines = append(lines, errStyle.Render("  "+gl.Cross+" ")+label)
			}
		}
		if st.log != "" {
			lines = append(lines, faintStyle.Render("  "+i18n.T("run.log", homeTilde(st.log))))
		}
		lines = append(lines, "")
	}

	mood, verdict := moodHappy, okStyle.Render(i18n.T("pilot.bye_ok"))
	switch {
	case m.stopping:
		mood, verdict = moodStop, warnStyle.Render(i18n.T("pilot.bye_stopped"))
	case failedStages > 0:
		mood, verdict = moodSad, errStyle.Render(i18n.T("pilot.bye_failed", failedStages, ran))
	}
	ok, failed, denied := m.tally(-1)
	side := []string{"", "", "", "", verdict, mutedStyle.Render(i18n.T("pilot.tally", ok, failed, denied)), "", "", "", ""}
	for i, line := range drawAvatar(mood, 3, false) {
		lines = append(lines, strings.TrimRight(" "+line+"  "+side[i], " "))
	}
	return lines
}
