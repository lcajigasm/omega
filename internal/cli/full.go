package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/runner"
	"github.com/lcajigasm/omega/internal/ui"
)

// fullStage is one step of the guided run: an omega command, started when
// the one before it ends.
type fullStage struct {
	id     string
	args   []string // the omega command line
	detail string   // i18n key of what it does
	risk   string
	agent  bool // starts a session with the agent, so it takes the run flags
	// when says whether the step applies to this machine; nil means always.
	when func() bool
}

// fullStages is everything omega can do to leave a machine better, in the
// order that makes sense: check that omega itself works, look before
// touching, update, clean, then the reviews that ask for decisions.
var fullStages = []fullStage{
	{id: "doctor", args: []string{"doctor"}, detail: "cmd.doctor", risk: "read-only"},
	{id: "analyze", args: []string{"analyze"}, detail: "slash.task_analyze", risk: "read-only", agent: true},
	{id: "update", args: []string{"update"}, detail: "slash.task_update", risk: "normal", agent: true},
	{id: "clean", args: []string{"clean"}, detail: "slash.task_clean", risk: "dangerous", agent: true},
	{id: "tune", args: []string{"tune"}, detail: "slash.task_tune", risk: "normal", agent: true},
	{id: "dev", args: []string{"dev"}, detail: "slash.task_dev", risk: "normal", agent: true},
	{id: "docker", args: []string{"docker"}, detail: "cmd.docker", risk: "dangerous", agent: true, when: func() bool { return hasTool("docker") }},
	{id: "mac", args: []string{"mac"}, detail: "slash.task_mac", risk: "normal", agent: true, when: func() bool { return goos == "darwin" }},
}

// hasTool is replaced by tests.
var hasTool = runner.Has

// Outcomes of a stage.
const (
	stDone    = "done"
	stSkipped = "skipped"
	stFailed  = "failed"
	stPending = "pending"
)

// stagesHere are the stages that apply to this machine.
func stagesHere() []fullStage {
	var out []fullStage
	for _, s := range fullStages {
		if s.when == nil || s.when() {
			out = append(out, s)
		}
	}
	return out
}

// stageArgs is the command line of a stage with the run flags the user
// gave the guided run.
func stageArgs(s fullStage, f runFlags) []string {
	args := append([]string(nil), s.args...)
	if !s.agent {
		return args
	}
	if f.provider != "" {
		args = append(args, "--provider", f.provider)
	}
	if f.model != "" {
		args = append(args, "--model", f.model)
	}
	if f.effort != "" {
		args = append(args, "--effort", f.effort)
	}
	for flag, on := range map[string]bool{"--dry-run": f.dryRun, "--auto": f.auto, "--yes": f.yes, "--sudo": f.sudo, "--no-tui": f.noTUI} {
		if on {
			args = append(args, flag)
		}
	}
	return args
}

// runFull walks the chosen stages. run starts one; ask reads the user's
// answer between two stages ("" or Enter to go on, s to skip the next one,
// q to stop). It returns the outcome of every stage, in order.
func runFull(w io.Writer, stages []fullStage, chosen map[string]bool, f runFlags, run func(args []string) error, ask func(next string) string) map[string]string {
	result := map[string]string{}
	var todo []fullStage
	for _, s := range stages {
		if chosen[s.id] {
			todo = append(todo, s)
			result[s.id] = stPending
		} else {
			result[s.id] = stSkipped
		}
	}
	for i := 0; i < len(todo); i++ {
		s := todo[i]
		fmt.Fprintln(w, "\n"+i18n.T("full.step", i+1, len(todo), cmdLabel(s.args...)))
		fmt.Fprintln(w, "  "+firstLine(i18n.T(s.detail)))
		if err := run(stageArgs(s, f)); err != nil {
			result[s.id] = stFailed
		} else {
			result[s.id] = stDone
		}
		// Between two stages the user decides: go on, skip the next, or stop.
		for i+1 < len(todo) {
			answer := strings.ToLower(strings.TrimSpace(ask(cmdLabel(todo[i+1].args...))))
			if answer == "q" {
				return result
			}
			if answer != "s" {
				break
			}
			i++
			result[todo[i].id] = stSkipped
		}
	}
	return result
}

// fullSummary says what happened to each stage.
func fullSummary(w io.Writer, stages []fullStage, result map[string]string) {
	fmt.Fprintln(w, "\n"+i18n.T("full.summary"))
	for _, s := range stages {
		mark, word := "·", i18n.T("full.st_"+result[s.id])
		switch result[s.id] {
		case stDone:
			mark = "✓"
		case stFailed:
			mark = "✗"
		}
		fmt.Fprintf(w, "  %s %-22s %s\n", mark, cmdLabel(s.args...), word)
	}
}

// fullCmd is the guided run of everything: a card to choose the steps, then
// each one in turn, with the chance to skip the next or stop between them.
func fullCmd() *cobra.Command {
	var f runFlags
	var skip []string
	c := &cobra.Command{
		Use: "full", Aliases: []string{"all", "everything", "todo"}, Short: i18n.T("cmd.full"), Long: i18n.T("cmd.full_long"), Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			stages := stagesHere()
			skipped := map[string]bool{}
			for _, id := range skip {
				for _, part := range strings.Split(id, ",") {
					skipped[strings.TrimSpace(part)] = true
				}
			}
			tty := term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
			chosen := map[string]bool{}
			if tty {
				var opts []ui.Option
				for _, s := range stages {
					opts = append(opts, ui.Option{ID: s.id, Label: cmdLabel(s.args...), Detail: firstLine(i18n.T(s.detail)), Risk: s.risk, Selected: !skipped[s.id]})
				}
				ids, err := ui.Pick(i18n.T("full.title"), i18n.T("full.hint"), opts, true, true, os.Stdin, os.Stdout)
				if err != nil {
					if err == ui.ErrCancelled {
						return nil
					}
					return err
				}
				for _, id := range ids {
					chosen[id] = true
				}
			} else {
				// Without a terminal there is nobody to ask: every step that
				// was not skipped runs, one after another, unattended.
				if !f.yes {
					return fmt.Errorf("%s", i18n.T("full.need_tty"))
				}
				f.noTUI = true
				for _, s := range stages {
					chosen[s.id] = !skipped[s.id]
				}
			}
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			in := bufio.NewReader(os.Stdin)
			run := func(args []string) error {
				if !tty && len(args) > 0 && args[0] != "doctor" {
					args = append(args, "--once")
				}
				c := exec.Command(exe, args...)
				c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
				return c.Run()
			}
			ask := func(next string) string {
				if !tty {
					return ""
				}
				fmt.Print("\n" + i18n.T("full.next", next))
				line, err := in.ReadString('\n')
				if err != nil {
					return "q"
				}
				return line
			}
			result := runFull(os.Stdout, stages, chosen, f, run, ask)
			fullSummary(os.Stdout, stages, result)
			return nil
		},
	}
	bindRunFlags(c.Flags(), &f)
	c.Flags().MarkHidden("prompt")
	c.Flags().MarkHidden("once")
	c.Flags().MarkHidden("demo")
	c.Flags().StringSliceVar(&skip, "skip", nil, i18n.T("flag.full_skip"))
	return c
}
