package ui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/i18n"
)

// answer waits for one read of the terminal, and stops waiting when the run
// is cancelled. A read of stdin cannot be interrupted, so it happens in a
// goroutine; ctrl+c then ends the run instead of leaving it stuck on a
// question nobody is going to answer. Once the run is cancelled no new read
// starts, so two never race for the same input. ok is false when the run
// was cancelled before an answer came.
func answer[T any](ctx context.Context, read func() T) (v T, ok bool) {
	if ctx.Err() != nil {
		return v, false
	}
	got := make(chan T, 1)
	go func() { got <- read() }()
	select {
	case v = <-got:
		return v, true
	case <-ctx.Done():
		return v, false
	}
}

// reply hands the agent an answer. A cancelled agent has stopped listening,
// and a plain send to it would never return.
func reply[T any](ctx context.Context, to chan<- T, v T) {
	select {
	case to <- v:
	case <-ctx.Done():
	}
}

// RunPlain prints events line by line and reads approvals, choices and
// follow-ups from stdin. Without a terminal on stdin every approval request
// is denied and every choice is left empty.
func RunPlain(ctx context.Context, ag *agent.Agent, in io.Reader, interactive bool) {
	reader := bufio.NewReader(in)
	// readLine reads one line, or gives up when the run is cancelled.
	readLine := func() (string, bool) {
		return answer(ctx, func() string {
			s, _ := reader.ReadString('\n')
			return strings.TrimSpace(s)
		})
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
				reply(ctx, e.Reply, agent.Deny)
				continue
			}
			fmt.Print(i18n.T("ui.confirm_line"))
			ans, ok := readLine()
			switch {
			case !ok:
				fmt.Println()
				reply(ctx, e.Reply, agent.Deny)
			case strings.EqualFold(ans, "a") && !e.Always:
				reply(ctx, e.Reply, agent.ApproveAll)
			case i18n.IsYes(ans):
				reply(ctx, e.Reply, agent.Approve)
			default:
				reply(ctx, e.Reply, agent.Deny)
			}
		case agent.EvPassword:
			if !interactive {
				reply(ctx, e.Secret, "")
				continue
			}
			if e.Failed {
				fmt.Println(errStyle.Render(i18n.T("ui.password_wrong")))
			}
			fmt.Println(mutedStyle.Render(i18n.T("ui.password_hint")))
			fmt.Print(warnStyle.Render(i18n.T("ui.password_title")) + " ")
			// Reading a password turns the echo off; a cancelled read
			// must not leave the terminal that way.
			fd := int(os.Stdin.Fd())
			state, _ := term.GetState(fd)
			pw, ok := answer(ctx, func() string {
				b, err := term.ReadPassword(fd)
				if err != nil {
					return ""
				}
				return string(b)
			})
			if !ok && state != nil {
				term.Restore(fd, state)
			}
			fmt.Println()
			reply(ctx, e.Secret, pw)
		case agent.EvChoice:
			opts := make([]Option, len(e.Choices))
			for i, c := range e.Choices {
				opts[i] = Option{ID: c.ID, Label: c.Label, Detail: c.Detail, Badge: c.Impact, Risk: c.Risk}
			}
			fmt.Println()
			ids, _ := answer(ctx, func() []string {
				ids, _ := pickPlain(NewPicker(e.Text, opts, e.Multi), reader, os.Stdout)
				return ids
			})
			reply(ctx, e.Picks, ids)
		case agent.EvIdle:
			fmt.Print("\n" + i18n.T("ui.followup"))
			// Ctrl+C while the question is open cancels the run: the agent
			// then stops listening, and a send to it would never return.
			if text, ok := readLine(); ok {
				reply(ctx, ag.Input(), text)
			} else {
				fmt.Println()
			}
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
