package ui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/i18n"
)

// RunPlain prints events line by line and reads approvals, choices and
// follow-ups from stdin. Without a terminal on stdin every approval request
// is denied and every choice is left empty.
func RunPlain(ag *agent.Agent, in io.Reader, interactive bool) {
	reader := bufio.NewReader(in)
	readLine := func() (string, bool) {
		s, err := reader.ReadString('\n')
		if err != nil && s == "" {
			return "", false
		}
		return strings.TrimSpace(s), true
	}
	var stats agent.Stats
	for e := range ag.Events() {
		if e.Stats.Model != "" {
			stats = e.Stats
		}
		switch e.Kind {
		case agent.EvAssistant:
			fmt.Printf("\n%s\n", e.Text)
		case agent.EvToolStart:
			name, arg := toolLabel(e.Tool, e.Command)
			if arg != "" {
				arg = "(" + arg + ")"
			}
			fmt.Printf("\n%s %s%s\n", titleStyle.Render("⏺"), cmdStyle.Render(name), arg)
			if strings.HasPrefix(e.Tool, "run_step:") {
				fmt.Println(faintStyle.Render("  " + e.Command))
			}
		case agent.EvOutput:
			fmt.Printf("  %s\n", mutedStyle.Render(e.Text))
		case agent.EvToolEnd:
			if e.Failed && e.ExitCode != 0 {
				fmt.Println(errStyle.Render(fmt.Sprintf("  ⎿ ✗ exit %d", e.ExitCode)))
			} else if e.Failed {
				fmt.Println(errStyle.Render("  ⎿ ✗"))
			} else if e.Duration >= time.Second {
				fmt.Println(okStyle.Render("  ⎿ ✓ " + humanDuration(e.Duration)))
			}
		case agent.EvDenied:
			name, arg := toolLabel(e.Tool, e.Command)
			fmt.Printf("\n%s %s(%s)\n  ⎿ %s · %s\n", warnStyle.Render("⏺"), name, arg, i18n.T("ui.denied"), firstSentence(e.Reason))
		case agent.EvModel:
			fmt.Println(mutedStyle.Render("\n↑ " + i18n.T("ui.model_switch", e.Text)))
		case agent.EvMode:
			fmt.Println(mutedStyle.Render("\n⏵ " + i18n.T("ui.mode_lifted")))
		case agent.EvConfirm:
			fmt.Printf("\n%s (%s)\n  %s\n", warnStyle.Render(i18n.T("ui.confirm")), i18n.T("ui.risk", riskLabel(e.Risk)), cmdStyle.Render(e.Command))
			if e.Reason != "" {
				fmt.Println(mutedStyle.Render("  " + strings.ReplaceAll(e.Reason, "\n", "\n  ")))
			}
			if e.Always {
				fmt.Println(warnStyle.Render("  ⚠ ") + i18n.T("ui.always_note"))
			}
			if !interactive {
				fmt.Println(errStyle.Render(i18n.T("ui.no_tty")))
				e.Reply <- agent.Deny
				continue
			}
			fmt.Print(i18n.T("ui.confirm_line"))
			ans, _ := readLine()
			switch {
			case strings.EqualFold(ans, "a") && !e.Always:
				e.Reply <- agent.ApproveAll
			case i18n.IsYes(ans):
				e.Reply <- agent.Approve
			default:
				e.Reply <- agent.Deny
			}
		case agent.EvPassword:
			if !interactive {
				e.Secret <- ""
				continue
			}
			if e.Failed {
				fmt.Println(errStyle.Render(i18n.T("ui.password_wrong")))
			}
			fmt.Println(mutedStyle.Render(i18n.T("ui.password_hint")))
			fmt.Print(warnStyle.Render(i18n.T("ui.password_title")) + " ")
			pw, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Println()
			if err != nil {
				pw = nil
			}
			e.Secret <- string(pw)
		case agent.EvChoice:
			opts := make([]Option, len(e.Choices))
			for i, c := range e.Choices {
				opts[i] = Option{ID: c.ID, Label: c.Label, Detail: c.Detail, Badge: c.Impact, Risk: c.Risk}
			}
			fmt.Println()
			ids, _ := pickPlain(NewPicker(e.Text, opts, e.Multi), reader, os.Stdout)
			e.Picks <- ids
		case agent.EvIdle:
			fmt.Print("\n" + i18n.T("ui.followup"))
			text, ok := readLine()
			if !ok {
				text = ""
			}
			ag.Input() <- text
		case agent.EvError:
			fmt.Fprintln(os.Stderr, errStyle.Render("! "+e.Text))
		case agent.EvDone:
			if e.Text != "" {
				fmt.Println(e.Text)
			}
			cost := ""
			if stats.CostKnown {
				cost = fmt.Sprintf("$%.4f", stats.Cost)
			}
			fmt.Println()
			fmt.Println(mutedStyle.Render(i18n.T("ui.summary_done", humanDuration(stats.Elapsed),
				humanTokens(stats.Usage.Input()+stats.Usage.OutputTokens), cost)))
			fmt.Println(mutedStyle.Render(i18n.T("run.log", ag.LogPath())))
		}
	}
}
