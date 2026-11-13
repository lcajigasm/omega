package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/runner"
	"github.com/lcajigasm/omega/internal/sudo"
	"github.com/lcajigasm/omega/internal/ui"
)

// fullStage is one stage of the guided run: a whole task of omega, started
// when the one before it ends.
type fullStage struct {
	id string
	// command is what starts the stage on its own, shown so the user learns
	// it: an omega command line, or a command typed inside a session.
	command string
	// task is the agent task the stage starts; a stage without one writes a
	// report instead.
	task   string
	report func(io.Writer) error
	risk   string
	// when says whether the stage applies to this machine; nil means always.
	when func() bool
}

// fullStages is everything omega can do to leave a machine better, in the
// order that makes sense: check that omega itself works, look before
// touching, update, clean, then the reviews that ask for decisions, and at
// the end what earlier runs left undone.
var fullStages = []fullStage{
	{id: "doctor", command: "omega doctor", report: doctorBrief, risk: "read-only"},
	{id: "analyze", command: "omega analyze", task: agent.TaskAnalyze, risk: "read-only"},
	{id: "health", command: "omega health", task: agent.TaskHealth, risk: "normal"},
	{id: "update", command: "omega update", task: agent.TaskUpdate, risk: "normal"},
	{id: "clean", command: "omega clean", task: agent.TaskClean, risk: "dangerous"},
	{id: "docker", command: "omega docker", task: agent.TaskDocker, risk: "dangerous", when: func() bool { return hasTool("docker") }},
	{id: "tune", command: "omega tune", task: agent.TaskTune, risk: "normal"},
	{id: "dev", command: "omega dev", task: agent.TaskDev, risk: "normal"},
	{id: "mac", command: "omega mac", task: agent.TaskMac, risk: "normal", when: func() bool { return goos == "darwin" }},
	{id: "fix", command: "omega fix", task: agent.TaskFix, risk: "normal",
		when: func() bool { return hasTool("topgrade") || (goos == "darwin" && hasTool("mo")) }},
	{id: "history", command: "/history", task: agent.TaskHistory, risk: "normal", when: func() bool { return hasRuns() }},
}

// doctorBrief is the doctor's report without the line it prints for every
// catalogue step: the guided run wants the verdict, and `omega steps` has
// the list.
func doctorBrief(w io.Writer) error {
	var b strings.Builder
	err := runDoctor(&b)
	skipped := " (" + i18n.T("doctor.skipped") + ")"
	for _, line := range strings.Split(strings.TrimRight(b.String(), "\n"), "\n") {
		// A step's line is indented and holds a mark and an id, with
		// "(skipped)" after it when the user turned the step off.
		t := strings.TrimSuffix(strings.TrimSpace(line), skipped)
		if mark, id, ok := strings.Cut(t, " "); ok && strings.HasPrefix(line, "  ") && (mark == "✓" || mark == "·") && !strings.Contains(id, " ") {
			continue
		}
		fmt.Fprintln(w, line)
	}
	return err
}

// Seams the tests replace.
var (
	hasTool = runner.Has
	hasRuns = func() bool {
		logs, err := agent.ListRunLogs()
		return err == nil && len(logs) > 0
	}
	startTask = runTask
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

// stageIDList names every stage, for the help of --skip and --only.
func stageIDList() string {
	ids := make([]string, len(fullStages))
	for i, s := range fullStages {
		ids[i] = s.id
	}
	return strings.Join(ids, ", ")
}

// guideStage is the stage as the interface shows it.
func guideStage(s fullStage, selected bool) ui.GuideStage {
	return ui.GuideStage{ID: s.id, Command: s.command, Risk: s.risk, Selected: selected,
		Title: i18n.T("full.s." + s.id + ".title"), What: i18n.T("full.s." + s.id + ".what"), Why: i18n.T("full.s." + s.id + ".why")}
}

// pickStages reads --only and --skip: which stages start ticked. A name
// that is not a stage is an error, so a typo does not run everything.
func pickStages(stages []fullStage, only, skip []string) (map[string]bool, error) {
	split := func(list []string) (map[string]bool, error) {
		out := map[string]bool{}
		for _, item := range list {
			for _, id := range strings.Split(item, ",") {
				if id = strings.ToLower(strings.TrimSpace(id)); id == "" {
					continue
				}
				known := false
				for _, s := range fullStages {
					known = known || s.id == id
				}
				if !known {
					return nil, fmt.Errorf("%s", i18n.T("full.unknown_stage", id, stageIDList()))
				}
				out[id] = true
			}
		}
		return out, nil
	}
	onlyIDs, err := split(only)
	if err != nil {
		return nil, err
	}
	skipIDs, err := split(skip)
	if err != nil {
		return nil, err
	}
	chosen := map[string]bool{}
	for _, s := range stages {
		chosen[s.id] = (len(onlyIDs) == 0 || onlyIDs[s.id]) && !skipIDs[s.id]
	}
	return chosen, nil
}

// runStage runs one stage and returns when it has ended.
func runStage(s fullStage, f runFlags, out io.Writer) error {
	if s.task == "" {
		return s.report(out)
	}
	return startTask(s.task, f)
}

// failNote is why a stage failed, when the error says so. A session that
// ended with an error has already shown it.
func failNote(err error) string {
	if err == nil {
		return ""
	}
	return strings.TrimSpace(err.Error())
}

// runFull is the guided run without the full-screen interface: for
// --no-tui, pipes and scheduled runs. run starts a stage; ask reads the
// answer between two stages (Enter goes on, s skips the next one, r repeats
// the last one when it failed, q stops), and is nil when nobody is there to
// answer. stopped reports whether the user interrupted the stage that just
// ran. It returns what happened to every stage, in order.
func runFull(w io.Writer, stages []ui.GuideStage, run func(id string) error, ask func() string, stopped func() bool) []ui.StageResult {
	res := make([]ui.StageResult, len(stages))
	var todo []int
	for i, s := range stages {
		res[i] = ui.StageResult{ID: s.ID, Status: ui.StagePending}
		if s.Selected {
			todo = append(todo, i)
		} else {
			res[i].Status = ui.StageSkipped
		}
	}
	rule := strings.Repeat("━", 4)
	for n := 0; n < len(todo); n++ {
		i := todo[n]
		s := stages[i]
		fmt.Fprintf(w, "\n%s %s · %s · %s\n", rule, i18n.T("full.stage", n+1, len(todo)), s.Title, s.Command)
		fmt.Fprintf(w, "  %s: %s\n  %s: %s\n\n", i18n.T("full.what"), s.What, i18n.T("full.why"), s.Why)
		began := time.Now()
		err := run(s.ID)
		res[i].Duration = time.Since(began)
		interrupted := stopped != nil && stopped()
		switch {
		case interrupted:
			res[i].Status = ui.StageStopped
			fmt.Fprintf(w, "\n■ %s · %s\n", s.Title, ui.StatusWord(ui.StageStopped))
		case err != nil:
			res[i].Status, res[i].Note = ui.StageFailed, failNote(err)
			line := "✗ " + s.Title + " · " + ui.StatusWord(ui.StageFailed)
			if res[i].Note != "" {
				line += " · " + res[i].Note
			}
			fmt.Fprintln(w, "\n"+line)
		default:
			res[i].Status = ui.StageDone
			fmt.Fprintf(w, "\n✓ %s · %s\n", s.Title, i18n.T("full.finished", res[i].Duration.Round(time.Second)))
		}
		if ask == nil {
			if interrupted {
				return res // nobody to ask, and the user said stop
			}
			continue
		}
		// Between two stages the user decides: go on, skip the next one,
		// repeat the one that failed, or stop.
		for n+1 < len(todo) {
			next := stages[todo[n+1]]
			var left []string
			for _, j := range todo[n+1:] {
				left = append(left, stages[j].Title)
			}
			fmt.Fprintf(w, "\n%s\n", i18n.T("full.plain_left", n+1, len(todo), strings.Join(left, ", ")))
			fmt.Fprintf(w, "%s: %s (%s)\n  %s\n", i18n.T("full.next"), next.Title, next.Command, next.What)
			keys := i18n.T("full.between_keys")
			retry := res[i].Status == ui.StageFailed || res[i].Status == ui.StageStopped
			if retry {
				keys = i18n.T("full.between_keys_retry")
			}
			fmt.Fprint(w, keys+": ")
			answer := strings.ToLower(strings.TrimSpace(ask()))
			if answer == "q" {
				return res
			}
			if answer == "r" && retry {
				n-- // the loop's n++ brings the same stage back
				break
			}
			if answer != "s" {
				break
			}
			n++
			res[todo[n]].Status = ui.StageSkipped
		}
	}
	return res
}

// fullSummary prints what happened to each stage.
func fullSummary(w io.Writer, stages []ui.GuideStage, res []ui.StageResult) {
	fmt.Fprintln(w)
	for _, line := range ui.SummaryLines(stages, res) {
		fmt.Fprintln(w, line)
	}
	if dir := agent.RunsDir(); dir != "" {
		for _, r := range res {
			if r.Status == ui.StageDone || r.Status == ui.StageFailed || r.Status == ui.StageStopped {
				fmt.Fprintln(w, "  "+i18n.T("full.logs", dir))
				break
			}
		}
	}
}

// fullFailed reports whether a stage ended with an error.
func fullFailed(res []ui.StageResult) bool {
	for _, r := range res {
		if r.Status == ui.StageFailed {
			return true
		}
	}
	return false
}

// fullCmd is the guided run of everything: a plan to choose the stages,
// then each one in turn inside one interface, with the chance to skip the
// next, repeat one that failed or stop between them.
func fullCmd() *cobra.Command {
	var f runFlags
	var skip, only []string
	c := &cobra.Command{
		Use: "full", Aliases: []string{"all", "everything", "todo"}, Short: i18n.T("cmd.full"), Long: i18n.T("cmd.full_long"), Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := f.validate(); err != nil {
				return err
			}
			// Each stage would print its own document, between the banners
			// of the guided run: that is not something a program can read.
			if f.wantsJSON() {
				return fmt.Errorf("%s", i18n.T("full.no_json"))
			}
			here := stagesHere()
			chosen, err := pickStages(here, only, skip)
			if err != nil {
				return err
			}
			if f.deterministic() {
				// Without the model only some stages exist: the others are
				// left out and named, instead of failing one by one.
				var left []string
				for _, s := range here {
					if s.task != "" && !hasDeterministic(s.task) && chosen[s.id] {
						chosen[s.id] = false
						left = append(left, s.id)
					}
				}
				if len(left) > 0 {
					fmt.Fprintln(os.Stderr, i18n.T("full.no_ai_left", strings.Join(left, ", ")))
				}
			}
			byID := map[string]fullStage{}
			var stages []ui.GuideStage
			for _, s := range here {
				byID[s.id] = s
				stages = append(stages, guideStage(s, chosen[s.id]))
			}
			// A stage ends when its work does: the run goes on to the next
			// one without anybody typing /exit.
			f.stage = true
			if f.unattended {
				return runUnattended(os.Stdout, stages, byID, f)
			}
			tty := term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
			if !tty && !f.yes {
				return fmt.Errorf("%s", i18n.T("full.need_tty"))
			}

			// The password is asked once, before the interface takes the
			// terminal; every stage then uses it, with or without the
			// full-screen interface.
			if f.sudo && goos != "windows" && !f.dryRun && !f.deterministic() && stdinIsTerminal() {
				if sudoAsked, err = askSudoOnce(); err != nil {
					return err
				}
				defer func() { sudoAsked = "" }()
			}
			if tty && !f.noTUI {
				info := ui.GuideInfo{Version: version}
				for _, fl := range []struct {
					on  bool
					key string
				}{{f.dryRun, "full.flag_dry"}, {f.auto, "full.flag_auto"}, {f.yes, "full.flag_yes"}, {f.demo, "full.flag_demo"}} {
					if fl.on {
						info.Flags = append(info.Flags, i18n.T(fl.key))
					}
				}
				res, ok, err := ui.RunGuide(info, stages, func(id string, out io.Writer) error { return runStage(byID[id], f, out) })
				if err != nil || !ok {
					return err
				}
				fullSummary(os.Stdout, stages, res)
				return nil
			}

			// Without the interface: line by line. Without a terminal there
			// is nobody to ask, so every ticked stage runs, one after another.
			f.noTUI = true
			var ask func() string
			if tty {
				var opts []ui.Option
				for _, s := range stages {
					opts = append(opts, ui.Option{ID: s.ID, Label: s.Title + "  " + s.Command, Detail: s.What, Risk: s.Risk, Selected: s.Selected})
				}
				ids, err := ui.Pick(i18n.T("full.title"), i18n.T("full.hint"), opts, true, true, os.Stdin, os.Stdout)
				if err != nil {
					if err == ui.ErrCancelled {
						return nil
					}
					return err
				}
				for i := range stages {
					stages[i].Selected = false
					for _, id := range ids {
						stages[i].Selected = stages[i].Selected || id == stages[i].ID
					}
				}
				in := bufio.NewReader(os.Stdin)
				ask = func() string {
					line, err := in.ReadString('\n')
					if err != nil {
						return "q"
					}
					return line
				}
			} else {
				f.once = true
			}
			// ctrl+c stops the stage that is running; this notices it, so an
			// unattended run stops there instead of starting the next stage.
			sig := make(chan os.Signal, 1)
			signal.Notify(sig, os.Interrupt)
			defer signal.Stop(sig)
			stopped := func() bool {
				select {
				case <-sig:
					return true
				default:
					return false
				}
			}
			res := runFull(os.Stdout, stages, func(id string) error { return runStage(byID[id], f, os.Stdout) }, ask, stopped)
			fullSummary(os.Stdout, stages, res)
			if !tty && fullFailed(res) {
				return exitCode(1)
			}
			return nil
		},
	}
	bindRunFlags(c.Flags(), &f)
	c.Flags().MarkHidden("prompt")
	c.Flags().MarkHidden("once")
	c.Flags().StringSliceVar(&skip, "skip", nil, i18n.T("flag.full_skip", stageIDList()))
	c.Flags().StringSliceVar(&only, "only", nil, i18n.T("flag.full_only"))
	c.Flags().BoolVarP(&f.unattended, "unattended", "U", false, i18n.T("flag.unattended"))
	return c
}

// Seams the tests replace.
var (
	sudoPasswordless = sudo.Passwordless
	askSudoOnce      = askSudo
	stdinIsTerminal  = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }
)

// unattendedFlags turns a guided run into one nobody watches: every approval
// is answered yes, low-risk improvements are applied, every option offered is
// taken, and each stage ends on its own and goes on to the next.
func unattendedFlags(f runFlags) runFlags {
	f.unattended, f.yes, f.auto, f.once, f.noTUI, f.stage = true, true, true, true, true, true
	return f
}

// runUnattended is omega full --unattended: the sudo password is asked once,
// at the start, and from then on nothing waits for anybody. Every stage
// ticked by --only and --skip runs, one after another, and a failure does not
// stop the ones after it.
func runUnattended(w io.Writer, stages []ui.GuideStage, byID map[string]fullStage, f runFlags) error {
	f = unattendedFlags(f)
	fmt.Fprintln(w, i18n.T("unattended.intro"))
	if goos != "windows" && !f.dryRun {
		switch {
		case sudoPasswordless():
			// sudo never asks here: nothing to ask either.
		case stdinIsTerminal():
			pw, err := askSudoOnce()
			if err != nil {
				return err
			}
			sudoAsked, f.sudo = pw, true
			defer func() { sudoAsked = "" }()
		default:
			fmt.Fprintln(w, i18n.T("unattended.no_sudo"))
		}
	}
	// ctrl+c stops the stage that is running and the run with it.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)
	stopped := func() bool {
		select {
		case <-sig:
			return true
		default:
			return false
		}
	}
	res := runFull(w, stages, func(id string) error { return runStage(byID[id], f, w) }, nil, stopped)
	fullSummary(w, stages, res)
	if fullFailed(res) {
		return exitCode(1)
	}
	return nil
}
