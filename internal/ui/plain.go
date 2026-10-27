package ui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/lcajigasm/omega/internal/agent"
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
			fmt.Printf("\n%s (riesgo: %s)\n  %s\n", warnStyle.Render("¿Ejecutar?"), riskLabel(e.Risk), cmdStyle.Render(e.Command))
			if e.Reason != "" {
				fmt.Println(mutedStyle.Render("  " + strings.ReplaceAll(e.Reason, "\n", "\n  ")))
			}
			if !interactive {
				fmt.Println(errStyle.Render("  sin terminal: rechazado (usa --yes para aprobar automáticamente)"))
				e.Reply <- agent.Deny
				continue
			}
			fmt.Print("  [y] sí  [a] sí a todo  [n] no: ")
			ans, _ := readLine()
			switch strings.ToLower(ans) {
			case "y", "s", "si", "sí", "yes":
				e.Reply <- agent.Approve
			case "a":
				e.Reply <- agent.ApproveAll
			default:
				e.Reply <- agent.Deny
			}
		case agent.EvIdle:
			fmt.Print("\n› (Enter vacío para salir) ")
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
			fmt.Println(mutedStyle.Render(fmt.Sprintf("Tokens: %d entrada / %d salida. Registro: %s", u.InputTokens, u.OutputTokens, ag.LogPath())))
		}
	}
}
