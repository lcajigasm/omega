package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/runner"
	"github.com/lcajigasm/omega/internal/steps"
	"github.com/lcajigasm/omega/internal/ui"
)

// customOpts are the flags of `omega custom`.
type customOpts struct {
	set, show, list, clear, yes, dryRun, asJSON bool
	makeDefault                                 string
	stopOnFail                                  []string
	belowGB                                     float64
	days                                        string
	noAI, noTUI                                 bool // what a scheduled job passes; custom never uses the AI
}

// customCmd is `omega custom`: the user's own routines. `--set` chooses the
// steps and their order; without it a routine runs here, step by step, with
// the same approval rules as `omega steps run` and without any model.
func customCmd() *cobra.Command {
	var o customOpts
	c := &cobra.Command{
		Use:   "custom [routine] [--set [step...]]",
		Short: i18n.T("cmd.custom"),
		Long:  i18n.T("cmd.custom_long") + "\n\n" + i18n.T("cmd.custom_long2"),
		Example: "  omega custom --set                      # " + i18n.T("custom.ui_title") + "\n" +
			"  omega custom --set brew npm brew-cleanup\n" +
			"  omega custom --set weekly brew mole-clean --stop-on-fail brew --only-below-free-gb 30\n" +
			"  omega custom --set weekly --days mon,thu\n" +
			"  omega custom --list\n" +
			"  omega custom weekly --default\n" +
			"  omega custom --show\n" +
			"  omega custom weekly\n" +
			"  omega custom --dry-run",
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
			var out []string
			if rs, _, err := config.LoadRoutines(); err == nil && len(args) == 0 {
				out = append(out, rs.Names()...)
			}
			for _, s := range steps.All {
				if s.Available() {
					out = append(out, s.ID)
				}
			}
			return out, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return o.run(cmd, args)
		},
	}
	f := c.Flags()
	f.BoolVar(&o.set, "set", false, i18n.T("flag.custom_set"))
	f.BoolVar(&o.show, "show", false, i18n.T("flag.custom_show"))
	f.BoolVar(&o.list, "list", false, i18n.T("flag.custom_list"))
	f.BoolVar(&o.clear, "clear", false, i18n.T("flag.custom_clear"))
	f.StringVar(&o.makeDefault, "default", "", i18n.T("flag.custom_default"))
	f.Lookup("default").NoOptDefVal = " " // `omega custom weekly --default`
	f.StringSliceVar(&o.stopOnFail, "stop-on-fail", nil, i18n.T("flag.custom_stop"))
	f.Float64Var(&o.belowGB, "only-below-free-gb", 0, i18n.T("flag.custom_below"))
	f.StringVar(&o.days, "days", "", i18n.T("flag.custom_days"))
	_ = c.RegisterFlagCompletionFunc("days", func(*cobra.Command, []string, string) ([]cobra.Completion, cobra.ShellCompDirective) {
		return config.Weekdays, cobra.ShellCompDirectiveNoFileComp
	})
	f.BoolVarP(&o.yes, "yes", "y", false, i18n.T("flag.steps_yes"))
	f.BoolVarP(&o.dryRun, "dry-run", "n", false, i18n.T("flag.steps_dry"))
	f.BoolVar(&o.asJSON, "json", false, i18n.T("flag.json"))
	// A scheduled job passes the flags every task gets; a routine never
	// uses the AI or a full-screen interface anyway.
	f.BoolVar(&o.noAI, "no-ai", false, "")
	f.BoolVar(&o.noTUI, "no-tui", false, "")
	_ = f.MarkHidden("no-ai")
	_ = f.MarkHidden("no-tui")
	c.MarkFlagsMutuallyExclusive("set", "show", "clear", "list")
	return c
}

// routineArgs splits the words after `omega custom --set`: a first word that
// is not a step but can name a routine is the routine's name.
func routineArgs(args []string) (name string, ids []string) {
	if len(args) > 0 {
		if _, isStep := steps.Find(args[0]); !isStep && !steps.Known(args[0]) && config.ValidRoutineName(args[0]) {
			return args[0], args[1:]
		}
	}
	return "", args
}

func (o customOpts) run(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	rs, found, err := config.LoadRoutines()
	if err != nil {
		return err
	}
	switch {
	case o.list:
		if len(args) > 0 {
			return usageErr(cmd, i18n.T("custom.no_args", "--list"))
		}
		if o.asJSON {
			return writeJSON(out, rs)
		}
		listRoutines(out, rs)
		return nil
	case o.set:
		name, ids := routineArgs(args)
		return o.setRoutine(out, cmd, rs, name, ids)
	}
	if len(args) > 1 {
		return usageErr(cmd, i18n.T("custom.extra_args", strings.Join(args, " "), strings.Join(args[1:], " ")))
	}
	name := ""
	if len(args) == 1 {
		name = args[0]
		if _, ok := rs.Routines[name]; !ok {
			if _, isStep := steps.Find(name); isStep {
				return usageErr(cmd, i18n.T("custom.extra_args", name, name))
			}
			return usageErr(cmd, i18n.T("custom.no_routine", name, strings.Join(rs.Names(), ", ")))
		}
	}
	if name == "" {
		name = rs.DefaultName()
	}
	switch {
	case o.clear:
		if _, ok := rs.Routines[name]; !ok {
			fmt.Fprintln(out, i18n.T("custom.cleared"))
			return nil
		}
		delete(rs.Routines, name)
		if err := config.SaveRoutines(rs); err != nil {
			return err
		}
		fmt.Fprintln(out, i18n.T("custom.cleared_one", name))
		return nil
	case cmd.Flags().Changed("default"):
		if strings.TrimSpace(o.makeDefault) != "" {
			name = strings.TrimSpace(o.makeDefault)
		}
		if _, ok := rs.Routines[name]; !ok {
			return usageErr(cmd, i18n.T("custom.no_routine", name, strings.Join(rs.Names(), ", ")))
		}
		rs.Default = name
		if err := config.SaveRoutines(rs); err != nil {
			return err
		}
		fmt.Fprintln(out, i18n.T("custom.default_set", name))
		return nil
	}
	rt, ok := rs.Routines[name]
	if !found || !ok || len(rt.Steps) == 0 {
		fmt.Fprintln(out, i18n.T("custom.none"))
		return nil
	}
	if o.show {
		if o.asJSON {
			return writeJSON(out, struct {
				Name string `json:"name"`
				config.Routine
			}{name, rt})
		}
		showRoutine(out, name, rt)
		return nil
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	var su sudoSession
	defer su.close()
	ctx, stop := signalContext()
	defer stop()
	cr := customRun{
		cfg: cfg, out: out, yes: o.yes, dryRun: o.dryRun, name: name, routine: rt,
		now: time.Now, scheduled: config.Scheduled(),
		find: steps.Find, tty: stdinIsTerminal(), confirm: askYesNo,
		sudo: su.need, exec: runner.Run, free: systemFree, history: loadCustomHistory(),
	}
	return cr.run(ctx, rt.Steps)
}

// signalContext is cancelled by ctrl+c, so a running step is stopped rather
// than left behind.
func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

// setRoutine saves a routine from the ids given, changes only its options
// when none are given with --stop-on-fail, --only-below-free-gb or --days,
// and opens the editor otherwise.
func (o customOpts) setRoutine(out io.Writer, cmd *cobra.Command, rs config.Routines, name string, ids []string) error {
	if name == "" {
		name = rs.DefaultName()
	}
	old := rs.Routines[name]
	days, err := config.ParseDays(o.days)
	if err != nil {
		var bad config.BadDayError
		errors.As(err, &bad)
		return usageErr(cmd, i18n.T("custom.bad_days", bad.Word))
	}
	withOptions := func(rt config.Routine) config.Routine {
		if cmd.Flags().Changed("stop-on-fail") {
			rt.StopOnFail = o.stopOnFail
		}
		if cmd.Flags().Changed("only-below-free-gb") {
			rt.OnlyBelowFreeGB = o.belowGB
		}
		if cmd.Flags().Changed("days") {
			rt.Days = days
		}
		rt.Updated = time.Time{}
		return rt
	}
	for _, id := range o.stopOnFail {
		if !steps.Known(id) {
			return usageErr(cmd, i18n.T("custom.unknown", id))
		}
	}
	if o.belowGB < 0 {
		return usageErr(cmd, i18n.T("err.negative", "--only-below-free-gb"))
	}
	optionsOnly := len(ids) == 0 && (cmd.Flags().Changed("stop-on-fail") || cmd.Flags().Changed("only-below-free-gb") || cmd.Flags().Changed("days"))
	switch {
	case len(ids) > 0:
		for _, id := range ids {
			if !steps.Known(id) {
				return usageErr(cmd, i18n.T("custom.unknown", id))
			}
		}
		rt := withOptions(old)
		rt.Steps = ids
		if err := config.SaveNamedRoutine(name, rt); err != nil {
			return err
		}
		fmt.Fprintln(out, i18n.N("custom.saved", len(ids), len(ids), tildePath(config.CustomPath())))
		return nil
	case optionsOnly:
		if len(old.Steps) == 0 {
			return usageErr(cmd, i18n.T("custom.no_routine", name, strings.Join(rs.Names(), ", ")))
		}
		if err := config.SaveNamedRoutine(name, withOptions(old)); err != nil {
			return err
		}
		fmt.Fprintln(out, i18n.T("custom.options_saved", name))
		return nil
	}
	if !stdinIsTerminal() {
		return errors.New(i18n.T("custom.need_tty"))
	}
	hist := loadCustomHistory()
	var offer []ui.RoutineStep
	for _, s := range steps.Grouped(steps.All) {
		if s.Available() {
			offer = append(offer, routineStep(s, hist))
		}
	}
	data := gatherRoutineData(hist, systemFree, time.Now())
	rt := withOptions(old)
	choice, err := ui.EditRoutine(ui.RoutineSetup{
		Steps: offer, Current: old.Steps, StopOnFail: rt.StopOnFail, OnlyBelowFreeGB: rt.OnlyBelowFreeGB, Days: rt.Days,
		Recommend: func(ctx context.Context, current []string, withData bool) (ui.Recommendation, error) {
			if withData {
				return recommendRoutine(ctx, current, &data)
			}
			return recommendRoutine(ctx, current, nil)
		},
		MachineData: data.words(),
	})
	if errors.Is(err, ui.ErrCancelled) {
		fmt.Fprintln(out, i18n.T("custom.unchanged"))
		return nil
	}
	if err != nil {
		return err
	}
	rt.Steps, rt.StopOnFail, rt.OnlyBelowFreeGB = keepMissing(old, choice)
	rt.Days = choice.Days
	chosen := rt.Steps
	if err := config.SaveNamedRoutine(name, rt); err != nil {
		return err
	}
	r := ui.NewReport(out)
	fmt.Fprintln(out, r.OK("✓ ")+i18n.N("custom.saved", len(chosen), len(chosen), tildePath(config.CustomPath())))
	showRoutine(out, name, rt)
	return nil
}

// routineStep is a step as the editor shows it, with how long it took and
// how much space it freed the last time `omega custom` ran it well.
func routineStep(s steps.Step, hist customHistory) ui.RoutineStep {
	rs := ui.RoutineStep{
		ID: s.ID, Label: i18n.TOr("step."+s.ID, s.Name), Category: s.Category,
		Command: s.Command, Risk: s.Risk.String(),
	}
	if last, ok := hist.last(s.ID); ok {
		rs.LastRun = last.Took
		rs.Freed = max(0, last.Freed)
	}
	return rs
}

// routineData is the aggregated, non-personal data about this machine that
// a recommendation may carry when the user turns it on (key d in the
// editor): nothing that names the user, a file or a path.
type routineData struct {
	FreeGB int      // free space of the system disk, rounded; -1 unknown
	Days   int      // days since `omega custom` last ran a step; -1 never
	Failed []string // ids of the steps that failed in the runs kept
}

// gatherRoutineData reads the free space and the history of the runs.
func gatherRoutineData(hist customHistory, free func() (uint64, bool), now time.Time) routineData {
	d := routineData{FreeGB: -1, Days: -1, Failed: hist.failedLately()}
	if free != nil {
		if b, ok := free(); ok {
			d.FreeGB = int(float64(b)/1e9 + 0.5)
		}
	}
	if when, ok := hist.lastRun(); ok {
		d.Days = max(0, int(now.Sub(when).Hours()/24))
	}
	return d
}

// words says the data in the user's language, for the editor's status line.
func (d routineData) words() string {
	var parts []string
	if d.FreeGB >= 0 {
		parts = append(parts, i18n.T("custom.data_free", d.FreeGB))
	}
	if d.Days >= 0 {
		parts = append(parts, i18n.T("custom.data_days", d.Days))
	} else {
		parts = append(parts, i18n.T("custom.data_never"))
	}
	if len(d.Failed) > 0 {
		parts = append(parts, i18n.T("custom.data_failed", strings.Join(d.Failed, ", ")))
	} else {
		parts = append(parts, i18n.T("custom.data_no_fail"))
	}
	return strings.Join(parts, " · ")
}

// prompt is the same data as the model reads it.
func (d routineData) prompt() string {
	var b strings.Builder
	b.WriteString("\nAggregated data about this machine, shared by the user:\n")
	if d.FreeGB >= 0 {
		fmt.Fprintf(&b, "- free space on the system disk: %d GB\n", d.FreeGB)
	}
	if d.Days >= 0 {
		fmt.Fprintf(&b, "- days since omega custom last ran: %d\n", d.Days)
	} else {
		b.WriteString("- omega custom has never run on this machine\n")
	}
	if len(d.Failed) > 0 {
		fmt.Fprintf(&b, "- steps that failed in recent runs: %s\n", strings.Join(d.Failed, ", "))
	} else {
		b.WriteString("- no step failed in recent runs\n")
	}
	return b.String()
}

// keepMissing merges what the editor saved with the steps this machine
// lacks: they stay in the routine (another machine that shares the file may
// have them), at the end, with their stop_on_fail.
func keepMissing(old config.Routine, c ui.RoutineChoice) (ids, stops []string, belowGB float64) {
	ids, stops = slices.Clone(c.Steps), slices.Clone(c.StopOnFail)
	for _, id := range old.Steps {
		if s, ok := steps.Find(id); (!ok || !s.Available()) && !slices.Contains(ids, id) {
			ids = append(ids, id)
			if old.Stops(id) {
				stops = append(stops, id)
			}
		}
	}
	return ids, stops, c.OnlyBelowFreeGB
}

func tildePath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

// listRoutines shows every routine: its name, how many steps, its options,
// and which one runs by default.
func listRoutines(out io.Writer, rs config.Routines) {
	r := ui.NewReport(out)
	r.Title("omega custom --list", tildePath(config.CustomPath()))
	if len(rs.Routines) == 0 {
		r.Blank()
		fmt.Fprintln(out, i18n.T("custom.none"))
		return
	}
	r.Blank()
	def := rs.DefaultName()
	var rows [][]string
	var marks []ui.Mark
	for _, name := range rs.Names() {
		rt := rs.Routines[name]
		mark, label := ui.MarkNone, name
		if name == def {
			mark, label = ui.MarkCurrent, r.Bold(name)
			// A routine called "default" needs no second word saying so;
			// the ▸ already marks it.
			if name != config.DefaultRoutine {
				label += r.Muted(" · " + i18n.T("custom.is_default"))
			}
		}
		rows = append(rows, []string{label, i18n.N("custom.n_steps", len(rt.Steps)), r.Muted(routineOptions(rt))})
		marks = append(marks, mark)
	}
	r.Table([]ui.Column{{Title: i18n.T("custom.col_routine")}, {Title: i18n.T("custom.col_steps")}, {}}, rows, marks)
	r.Blank()
	r.Hint(i18n.T("custom.hint_list"))
}

// routineOptions says a routine's options in words.
func routineOptions(rt config.Routine) string {
	var parts []string
	if rt.OnlyBelowFreeGB > 0 {
		parts = append(parts, i18n.T("custom.opt_below", rt.OnlyBelowFreeGB))
	}
	if len(rt.Days) > 0 {
		parts = append(parts, i18n.T("custom.opt_days", ui.DaysText(rt.Days)))
	}
	if len(rt.StopOnFail) > 0 {
		parts = append(parts, i18n.T("custom.opt_stop", strings.Join(rt.StopOnFail, ", ")))
	}
	return strings.Join(parts, " · ")
}

// showRoutine lists a routine as a table.
func showRoutine(out io.Writer, name string, rt config.Routine) {
	r := ui.NewReport(out)
	r.Title("omega custom", routineTitle(name)+" · "+i18n.T("custom.local"))
	if opts := routineOptions(rt); opts != "" {
		r.Line(r.Muted(opts))
	}
	r.Blank()
	var rows [][]string
	var marks []ui.Mark
	for n, id := range rt.Steps {
		s, ok := steps.Find(id)
		stop := ""
		if rt.Stops(id) {
			stop = r.Muted(" · " + i18n.T("custom.stops"))
		}
		switch {
		case !ok:
			marks = append(marks, ui.MarkOff)
			rows = append(rows, []string{fmt.Sprint(n + 1), id, r.Faint(i18n.T("custom.not_here")), ""})
		case !s.Available():
			marks = append(marks, ui.MarkOff)
			rows = append(rows, []string{fmt.Sprint(n + 1), id, r.Faint(i18n.TOr("step."+s.ID, s.Name) + " · " + i18n.T("custom.not_installed")), ""})
		default:
			marks = append(marks, ui.MarkOK)
			rows = append(rows, []string{fmt.Sprint(n + 1), id, i18n.TOr("step."+s.ID, s.Name) + stop, r.StepRisk(s.Risk.String())})
		}
	}
	r.Table([]ui.Column{{Title: "#", Right: true}, {Title: i18n.T("custom.col_step")}, {Title: i18n.T("custom.col_what")}, {Title: i18n.T("custom.col_risk")}}, rows, marks)
	r.Blank()
	r.Hint(i18n.T("custom.hint_run"))
}

// routineTitle is "your routine" for the default one and its name for the
// others.
func routineTitle(name string) string {
	if name == "" || name == config.DefaultRoutine {
		return i18n.T("custom.title")
	}
	return i18n.T("custom.title_named", name)
}

// customRun is one run of a routine.
type customRun struct {
	cfg     *config.Config
	out     io.Writer
	yes     bool
	dryRun  bool
	name    string
	routine config.Routine   // its options: stop_on_fail, only_below_free_gb, days
	now     func() time.Time // the day the days are checked against; nil: not checked
	// scheduled is a run nobody started by hand: on a day the routine
	// does not list it does nothing. A run started by hand only warns.
	scheduled bool
	find      func(id string) (steps.Step, bool)
	tty       bool
	confirm   func(question string) bool
	sudo      func(command string) error
	exec      func(ctx context.Context, command string, timeout time.Duration, onLine func(string)) runner.Result
	free      func() (uint64, bool) // free bytes on the system disk; nil: not checked
	history   customHistory         // where each step's run is recorded; nil: nowhere
}

// freeNow is the free space of the system disk, when it can be read.
func (c customRun) freeNow() (uint64, bool) {
	if c.free == nil {
		return 0, false
	}
	return c.free()
}

type customResult struct {
	id   string
	mark ui.Mark
	text string
	took time.Duration
}

// run runs the routine in order. Its rules are those of `omega steps run`:
// a step that needs a yes is asked at the terminal and skipped without one,
// and the command fails when a step failed or was skipped, so a script never
// reads a step that did not run as done.
func (c customRun) run(ctx context.Context, ids []string) error {
	timeout, err := time.ParseDuration(c.cfg.Timeout)
	if err != nil {
		return err
	}
	r := ui.NewReport(c.out)
	detail := routineTitle(c.name) + " · " + i18n.T("custom.local")
	if c.dryRun {
		detail += " · dry-run"
	}
	r.Title("omega custom", detail)
	if c.now != nil {
		if today := c.now(); !c.routine.RunsOn(today) {
			day, days := ui.DaysText([]string{config.DayID(today.Weekday())}), ui.DaysText(c.routine.Days)
			r.Blank()
			if c.scheduled {
				r.Line(r.Glyph(ui.MarkSkip) + " " + i18n.T("custom.not_today", day, days))
				return nil
			}
			r.Line(r.Glyph(ui.MarkWarn) + " " + i18n.T("custom.not_today_hand", day, days))
		}
	}
	// The guard of the routine: with enough free space it has nothing to
	// do, which is not a failure.
	if limit := c.routine.OnlyBelowFreeGB; limit > 0 && c.free != nil {
		if free, ok := c.free(); ok && float64(free) >= limit*1e9 {
			r.Blank()
			r.Line(r.Glyph(ui.MarkSkip) + " " + i18n.T("custom.enough_space", ui.Bytes(int64(free)), limit))
			return nil
		}
	}
	start := time.Now()
	var results []customResult
	failed, notRun := 0, 0
	stopped := ""
	for n, id := range ids {
		if ctx.Err() != nil || stopped != "" {
			break
		}
		s, ok := c.find(id)
		if !ok {
			results = append(results, customResult{id, ui.MarkOff, i18n.T("custom.not_here"), 0})
			continue
		}
		if !s.Available() {
			results = append(results, customResult{id, ui.MarkOff, i18n.T("custom.not_installed"), 0})
			continue
		}
		fmt.Fprintln(c.out)
		fmt.Fprintln(c.out, r.Accent("▸ "+i18n.T("custom.step_head", n+1, len(ids)))+"  "+r.Bold(s.ID)+"  "+r.Muted(i18n.TOr("step."+s.ID, s.Name)))
		fmt.Fprintln(c.out, "  "+r.Faint("$ "+r.Clip(s.Command, 4)))
		if c.dryRun {
			results = append(results, customResult{id, ui.MarkInfo, i18n.T("custom.r_dry"), 0})
			continue
		}
		if needsAnswer(c.cfg, s, c.yes) {
			if !c.tty {
				notRun++
				fmt.Fprintln(c.out, "  "+r.Glyph(ui.MarkWarn)+" "+i18n.T("custom.r_skipped"))
				results = append(results, customResult{id, ui.MarkWarn, i18n.T("custom.r_skipped"), 0})
				continue
			}
			if !c.confirm("  " + i18n.T("steps.confirm", i18n.TOr("step."+s.ID, s.Name), s.Command)) {
				notRun++
				results = append(results, customResult{id, ui.MarkSkip, i18n.T("custom.r_declined"), 0})
				continue
			}
		}
		if err := c.sudo(s.Command); err != nil {
			return err
		}
		before, measured := c.freeNow()
		res := c.exec(ctx, s.Command, timeout, func(l string) { fmt.Fprintln(c.out, "  "+r.Faint("│")+" "+l) })
		if c.history != nil && ctx.Err() == nil {
			rec := customRecord{When: time.Now(), Took: res.Duration, OK: res.ExitCode == 0, Routine: c.name}
			if after, ok := c.freeNow(); ok && measured {
				rec.Freed = int64(after) - int64(before)
			}
			c.history.add(id, rec)
		}
		if res.ExitCode != 0 {
			failed++
			text := i18n.T("custom.r_failed", res.ExitCode)
			fmt.Fprintln(c.out, "  "+r.Glyph(ui.MarkErr)+" "+r.Err(text)+r.Faint(" · "+ui.Duration(res.Duration)))
			results = append(results, customResult{id, ui.MarkErr, text, res.Duration})
			if c.routine.Stops(id) {
				stopped = id
				fmt.Fprintln(c.out, "  "+r.Glyph(ui.MarkErr)+" "+i18n.T("custom.stopped", id))
			}
			continue
		}
		fmt.Fprintln(c.out, "  "+r.Glyph(ui.MarkOK)+" "+i18n.T("custom.r_ok")+r.Faint(" · "+ui.Duration(res.Duration)))
		results = append(results, customResult{id, ui.MarkOK, i18n.T("custom.r_ok"), res.Duration})
	}
	if c.history != nil {
		_ = c.history.save() // only a convenience for the editor
	}

	r.Section(i18n.T("custom.summary"), -1)
	var rows [][]string
	var marks []ui.Mark
	done := 0
	for _, res := range results {
		took := ""
		if res.took > 0 {
			took = ui.Duration(res.took)
		}
		if res.mark == ui.MarkOK {
			done++
		}
		rows = append(rows, []string{res.id, res.text, took})
		marks = append(marks, res.mark)
	}
	r.Table([]ui.Column{{Title: i18n.T("custom.col_step")}, {Title: i18n.T("custom.col_result")}, {Title: i18n.T("custom.col_time"), Right: true}}, rows, marks)
	r.Blank()
	r.Line(r.Muted(i18n.T("custom.totals", done, failed, notRun+len(ids)-len(results), ui.Duration(time.Since(start)))))
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if failed > 0 {
		return fmt.Errorf("%s", i18n.N("steps.failed_summary", failed))
	}
	if notRun > 0 && !c.dryRun {
		return fmt.Errorf("%s", i18n.N("steps.skipped_summary", notRun))
	}
	return nil
}

// recommendRoutine asks the configured model for a routine. It sends the
// installed steps (id, category, risk, what it does), the operating system
// and the current routine: nothing about the user or their files. data,
// when the user turned it on, adds the aggregated figures of routineData.
func recommendRoutine(ctx context.Context, current []string, data *routineData) (ui.Recommendation, error) {
	cfg, err := config.Load()
	if err != nil {
		return ui.Recommendation{}, err
	}
	p, plan, err := resolveProvider(cfg, "", "", "", agent.TaskQuick)
	if err != nil {
		return ui.Recommendation{}, err
	}
	if p == nil {
		return ui.Recommendation{}, errors.New("no AI provider is set up (omega setup)")
	}
	prompt, installed := routinePrompt(steps.Grouped(steps.All), current, data)
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	reply, err := p.NewChat(plan.Model, fmt.Sprintf(routineSystem, i18n.Names[i18n.Lang()]), nil).Send(ctx, prompt, nil)
	if err != nil {
		return ui.Recommendation{}, err
	}
	return parseRecommendation(reply.Text, installed)
}

// routinePrompt is what a recommendation sends: the operating system, the
// installed steps, the current routine and, only when data is not nil, the
// machine's aggregated figures.
func routinePrompt(all []steps.Step, current []string, data *routineData) (string, map[string]bool) {
	var b strings.Builder
	fmt.Fprintf(&b, "Operating system: %s/%s\n\nInstalled steps:\n", runtime.GOOS, runtime.GOARCH)
	installed := map[string]bool{}
	for _, s := range all {
		if !s.Available() {
			continue
		}
		installed[s.ID] = true
		desc := s.Description
		if desc == "" {
			desc = s.Name
		}
		fmt.Fprintf(&b, "- %s [%s, %s]: %s\n", s.ID, s.Category, s.Risk, desc)
	}
	if len(current) > 0 {
		fmt.Fprintf(&b, "\nCurrent routine, in order: %s\n", strings.Join(current, ", "))
	} else {
		b.WriteString("\nThe user has no routine yet.\n")
	}
	if data != nil {
		b.WriteString(data.prompt())
	}
	return b.String(), installed
}

const routineSystem = "You help a user choose their own maintenance routine for omega, a tool that maintains their computer. " +
	"You get the steps installed on this machine (id, category, risk, what it does) and the user's current routine. " +
	"Recommend a routine: the steps worth running regularly, in a sensible order: checks whose result informs later steps first, then updates, then cleanups, with each cleanup after the update that leaves its leftovers. " +
	"Leave out steps of little value or high risk for a routine run and mention them in the summary. Use only ids from the list. " +
	"When aggregated machine data is given, use it: little free space favours cleanups, a long time since the last run favours updates, and a step that failed recently may be worth leaving out or placing where its failure blocks nothing. " +
	`Reply with JSON only, no prose around it: {"summary": "...", "steps": [{"id": "...", "why": "..."}]}. ` +
	"Write the summary (two or three sentences) and each why (one short sentence) in %s."

// parseRecommendation reads the model's JSON. Ids that are not installed
// steps and repeats are dropped; an answer with none left is an error.
func parseRecommendation(text string, installed map[string]bool) (ui.Recommendation, error) {
	i, j := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if i < 0 || j <= i {
		return ui.Recommendation{}, errors.New(i18n.T("custom.ai_bad"))
	}
	var raw struct {
		Summary string `json:"summary"`
		Steps   []struct {
			ID  string `json:"id"`
			Why string `json:"why"`
		} `json:"steps"`
	}
	if err := json.Unmarshal([]byte(text[i:j+1]), &raw); err != nil {
		return ui.Recommendation{}, errors.New(i18n.T("custom.ai_bad"))
	}
	rec := ui.Recommendation{Summary: strings.TrimSpace(raw.Summary), Why: map[string]string{}}
	for _, s := range raw.Steps {
		id := strings.TrimSpace(s.ID)
		if !installed[id] || slices.Contains(rec.Steps, id) {
			continue
		}
		rec.Steps = append(rec.Steps, id)
		rec.Why[id] = strings.TrimSpace(s.Why)
	}
	if len(rec.Steps) == 0 {
		return ui.Recommendation{}, errors.New(i18n.T("custom.ai_bad"))
	}
	return rec, nil
}
