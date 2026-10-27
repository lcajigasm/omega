package ui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/i18n"
)

// RunPlain prints events line by line and reads approvals and follow-ups from
// stdin. Without a terminal on stdin every approval request is denied.
func RunPlain(ag *agent.Agent, in io.Reader, interactive bool) {
	reader := bufio.NewReader(in)
	readLine := func() (string, bool) {
		s, err := reader.ReadString('\n')
		if err != nil && s == "" {
			return "", false
		}
		return strings.TrimSpace(s), true
	}
	for e := range ag.Events() {
		switch e.Kind {
		case agent.EvAssistant:
			fmt.Printf("\n%s\n", e.Text)
		case agent.EvToolStart:
			fmt.Printf("\n%s %s\n", titleStyle.Render("$"), cmdStyle.Render(e.Command))
		case agent.EvOutput:
			fmt.Printf("  %s\n", mutedStyle.Render(e.Text))
		case agent.EvToolEnd:
			if e.Failed {
				fmt.Println(errStyle.Render(fmt.Sprintf("✗ exit %d", e.ExitCode)))
			} else if e.Duration >= time.Second {
				fmt.Println(okStyle.Render("✓ " + e.Duration.Round(time.Second).String()))
			}
		case agent.EvConfirm:
			fmt.Printf("\n%s (%s)\n  %s\n", warnStyle.Render(i18n.T("ui.confirm")), i18n.T("ui.risk", riskLabel(e.Risk)), cmdStyle.Render(e.Command))
			if e.Reason != "" {
				fmt.Println(mutedStyle.Render("  " + strings.ReplaceAll(e.Reason, "\n", "\n  ")))
			}
			if !interactive {
				fmt.Println(errStyle.Render(i18n.T("ui.no_tty")))
				e.Reply <- agent.Deny
				continue
			}
			fmt.Print(i18n.T("ui.confirm_line"))
			ans, _ := readLine()
			switch {
			case strings.EqualFold(ans, "a"):
				e.Reply <- agent.ApproveAll
			case i18n.IsYes(ans):
				e.Reply <- agent.Approve
			default:
				e.Reply <- agent.Deny
			}
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
			u := ag.Usage()
			fmt.Println()
			fmt.Println(mutedStyle.Render(i18n.T("ui.summary", u.InputTokens, u.OutputTokens, ag.LogPath())))
		}
	}
}
