package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/demo"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/ui"
)

// omega ultra is the whole of omega in two stages. The audit runs every
// stage of omega full, plus a deep clean of what the cleaning steps never
// reach and a pass through every review omega has (AI agents, configuration
// assistants, security, privacy, network, skills), on stronger models, with
// more effort and more turns, changing nothing and needing nobody: each
// change it recommends goes into a plan with its evidence and its exact
// steps. Then the person reviews the plan once, ticks what they want, and
// omega carries out exactly that.

// ultraTurns is how many turns an ultra stage may take: it measures,
// inspects and checks again far more than an ordinary one.
const ultraTurns = 200

func init() { agent.OmegaRun = runOmega }

// ultraStages are omega full's stages with the two only ultra has: the
// deep clean after Docker, the reviews after tune.
func ultraStages() []fullStage {
	deep := fullStage{id: "deep", command: "omega ultra --only deep", task: agent.TaskUltraDeep, risk: "dangerous"}
	care := fullStage{id: "care", command: "omega ultra --only care", task: agent.TaskUltraCare, risk: "normal"}
	var out []fullStage
	for _, s := range fullStages {
		out = append(out, s)
		switch s.id {
		case "docker":
			out = append(out, deep)
		case "tune":
			out = append(out, care)
		}
	}
	return out
}

// ultraTier is the provider's most capable model: the user's ultra_model,
// else the provider's, else the smart one.
func ultraTier(cfg *config.Config, info llm.Info) string {
	_, smart := tiers(cfg, info)
	if m := cfg.Providers[info.Name].UltraModel; m != "" {
		return m
	}
	if info.UltraModel != "" {
		return info.UltraModel
	}
	return smart
}

// ultraPlan decides the models of an ultra stage. A model given with
// --model is used as it is. Otherwise the stages start on the smart tier,
// never the fast one, and move to the ultra tier when a step fails; the
// deep clean, where the analysis is hardest and a mistake costs most,
// starts on the ultra tier.
func ultraPlan(cfg *config.Config, info llm.Info, flagModel, task string) modelPlan {
	fast, smart := tiers(cfg, info)
	top := ultraTier(cfg, info)
	var plan modelPlan
	switch {
	case flagModel != "":
		plan = modelPlan{Model: flagModel}
	case task == agent.TaskUltraDeep:
		plan = modelPlan{Model: top}
	case smart != top:
		plan = modelPlan{Model: smart, Escalate: top}
	default:
		plan = modelPlan{Model: smart}
	}
	plan.Choices = uniqueModels(plan.Model, plan.Escalate, top, smart, fast)
	return plan
}

// efforts in order: ultra never lowers what the user configured.
var efforts = []string{"low", "medium", "high", "xhigh", "max"}

// ultraEffort is the effort of an ultra stage: high, or the configured one
// when it is higher.
func ultraEffort(configured string) string {
	rank := func(e string) int {
		for i, v := range efforts {
			if v == e {
				return i
			}
		}
		return -1
	}
	if rank(configured) > rank("high") {
		return configured
	}
	return "high"
}

func ultraCmd() *cobra.Command {
	var f runFlags
	var skip, only []string
	var review bool
	c := &cobra.Command{
		Use:     "ultra",
		Aliases: []string{"deep"},
		Short:   i18n.T("cmd.ultra"),
		Long:    i18n.T("cmd.ultra_long"),
		Example: "  omega ultra\n  omega ultra --review\n  omega ultra --only deep,care\n  omega ultra --skip update,mac\n  omega ultra --dry-run",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := f.validate(); err != nil {
				return err
			}
			// Ultra is the run where a person chooses every change: the
			// flags that answer for them have no place in it.
			for _, fl := range []struct {
				on   bool
				name string
			}{{f.yes, "--yes"}, {f.auto, "--auto"}, {f.noAI, "--no-ai"}, {f.offline, "--offline"}, {f.wantsJSON(), "--json"}} {
				if fl.on {
					return usageErr(cmd, i18n.T("ultra.flag_refused", fl.name))
				}
			}
			if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
				return fmt.Errorf("%s", i18n.T("ultra.need_tty"))
			}
			if review {
				return ultraReview(os.Stdout, f)
			}
			here := ultraStagesHere()
			chosen, err := pickUltraStages(here, only, skip)
			if err != nil {
				return err
			}
			return runUltra(os.Stdout, here, chosen, f)
		},
	}
	bindRunFlags(c.Flags(), &f)
	// What ultra refuses or has no use for is not offered in its help.
	for _, name := range []string{"prompt", "once", "yes", "auto", "no-ai", "offline", "json", "format", "quiet", "verbose",
		"fail-on-warning", "fail-on-outdated", "fail-on-critical", "debug"} {
		c.Flags().MarkHidden(name)
	}
	c.Flags().StringSliceVar(&skip, "skip", nil, i18n.T("flag.full_skip", ultraStageIDList()))
	c.Flags().StringSliceVar(&only, "only", nil, i18n.T("flag.full_only"))
	c.Flags().BoolVar(&review, "review", false, i18n.T("flag.ultra_review"))
	return c
}

// Seams the tests replace.
var (
	reviewPlan  = ui.RunPlanReview
	freeSpace   = systemFree
	planScreen  = func(noTUI bool) bool { return ui.WantTUI(noTUI) }
	ultraBefore uint64 // free space before the audit, for the closing report
)

// runUltra is omega ultra: the audit, then the review, then the plan.
//
// Stage 1, the audit, needs nobody: the sudo password is asked once at the
// start, and every stage looks at the machine read-only, on the ultra
// models, and puts every change it recommends in the plan. Stage 2 shows
// the plan; the person ticks what they want, and omega carries out exactly
// those steps, with no model and nothing more to answer.
func runUltra(w io.Writer, here []fullStage, chosen map[string]bool, f runFlags) error {
	byID := map[string]fullStage{}
	var stages []ui.GuideStage
	for _, s := range here {
		byID[s.id] = s
		stages = append(stages, guideStage(s, chosen[s.id]))
	}
	plan := agent.NewPlan()
	f.stage, f.ultra, f.audit = true, true, plan
	f.share = newStageShare()
	fmt.Fprintln(w, i18n.T("ultra.intro"))
	if err := ultraSudo(w, &f); err != nil {
		return err
	}
	if free, ok := freeSpace(); ok {
		ultraBefore = free
	}
	began := time.Now()
	var res []ui.StageResult
	if planScreen(f.noTUI) {
		info := ui.PilotInfo{Version: version, Title: i18n.T("ultra.audit_title"), Ultra: true, Found: plan.Count,
			Note: i18n.T("ultra.audit_note")}
		if m := ultraModelLabel(f); m != "" {
			info.Flags = append(info.Flags, m)
		}
		info.Flags = append(info.Flags, ultraFlags(f)...)
		var report []string
		var err error
		res, report, err = runPilot(info, stages, func(id string, out io.Writer) error { return runStage(byID[id], f, out) })
		if err != nil {
			return err
		}
		for _, line := range report {
			fmt.Fprintln(w, line)
		}
	} else {
		stopped := interrupted()
		defer stopped.stop()
		res = runFull(w, stages, func(id string) error { return runStage(byID[id], f, w) }, nil, stopped.check)
	}
	fullSummary(w, stages, res)
	if stoppedRun(res) {
		return nil
	}
	// The terminal's bell: the audit has ended and the plan waits.
	fmt.Fprint(w, "\a")
	if err := plan.Save(); err != nil {
		fmt.Fprintln(w, "omega:", err)
	}
	cost, known := 0.0, true
	for _, r := range res {
		cost += r.Cost
		known = known && (r.CostKnown || r.Cost == 0)
	}
	acts := plan.List()
	fmt.Fprintln(w, i18n.T("ultra.audit_done", ui.Duration(time.Since(began)), len(acts)))
	if len(acts) == 0 {
		fmt.Fprintln(w, i18n.T("ultra.nothing"))
		return nil
	}
	info := ui.PlanReviewInfo{Version: version, Audit: time.Since(began), Cost: cost, CostKnown: known, StageTitle: ultraStageTitle,
		DryRun: f.dryRun, Flags: ultraFlags(f)}
	return reviewAndRun(w, plan, acts, info, f)
}

// ultraReview is omega ultra --review: the plan of the last audit, without
// auditing again.
func ultraReview(w io.Writer, f runFlags) error {
	plan, err := agent.LoadPlan()
	if errors.Is(err, agent.ErrNoPlan) {
		return fmt.Errorf("%s", i18n.T("ultra.no_plan"))
	}
	if err != nil {
		return err
	}
	var acts []agent.PlannedAction
	for _, a := range plan.List() {
		if a.Status != agent.ActionDone {
			acts = append(acts, a)
		}
	}
	if len(acts) == 0 {
		fmt.Fprintln(w, i18n.T("ultra.review_done"))
		return nil
	}
	age := time.Since(plan.Created)
	fmt.Fprintln(w, i18n.T("ultra.review_age", ui.Duration(age.Round(time.Minute)), len(acts)))
	f.ultra = true
	if err := ultraSudo(w, &f); err != nil {
		return err
	}
	if free, ok := freeSpace(); ok {
		ultraBefore = free
	}
	info := ui.PlanReviewInfo{Version: version, StageTitle: ultraStageTitle, DryRun: f.dryRun, Flags: ultraFlags(f)}
	return reviewAndRun(w, plan, acts, info, f)
}

// ultraSudo asks for the sudo password once, before any screen takes the
// terminal; nothing else is asked until the plan is shown.
func ultraSudo(w io.Writer, f *runFlags) error {
	if goos == "windows" || f.dryRun || f.demo || sudoPasswordless() {
		return nil
	}
	pw, err := askSudoOnce()
	if err != nil {
		return err
	}
	sudoAsked, f.sudo = pw, true
	return nil
}

func ultraFlags(f runFlags) []string {
	var out []string
	if f.dryRun {
		out = append(out, i18n.T("full.flag_dry"))
	}
	if f.demo {
		out = append(out, i18n.T("full.flag_demo"))
	}
	return out
}

// ultraStageTitle names the stage a task belongs to.
func ultraStageTitle(task string) string {
	for _, s := range ultraStages() {
		if s.task == task || s.id == task {
			return i18n.T("full.s." + s.id + ".title")
		}
	}
	return ""
}

// stoppedRun reports a run the user stopped: nothing is reviewed then.
func stoppedRun(res []ui.StageResult) bool {
	for _, r := range res {
		if r.Status == ui.StageStopped {
			return true
		}
	}
	return false
}

// reviewAndRun shows the plan, then carries out what was ticked.
func reviewAndRun(w io.Writer, plan *agent.Plan, acts []agent.PlannedAction, info ui.PlanReviewInfo, f runFlags) error {
	var ids []string
	if planScreen(f.noTUI) {
		picked, ok, err := reviewPlan(info, acts)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(w, i18n.T("ultra.kept", agent.PlanPath()))
			return nil
		}
		ids = picked
	} else {
		var opts []ui.Option
		for _, a := range acts {
			detail := a.Why + " → " + a.What
			opts = append(opts, ui.Option{ID: a.ID, Label: a.Title, Detail: detail, Badge: planImpact(a), Risk: a.Risk, Selected: a.Recommended})
		}
		picked, err := ui.Pick(i18n.T("plan.title"), i18n.T("ultra.pick_hint"), opts, true, false, os.Stdin, w)
		if err != nil {
			if errors.Is(err, ui.ErrCancelled) {
				fmt.Fprintln(w, i18n.T("ultra.kept", agent.PlanPath()))
				return nil
			}
			return err
		}
		ids = picked
	}
	todo := pickActions(acts, ids)
	if len(todo) == 0 {
		fmt.Fprintln(w, i18n.T("ultra.kept", agent.PlanPath()))
		return nil
	}
	if f.dryRun {
		fmt.Fprintln(w, i18n.T("ultra.dry"))
		for _, a := range todo {
			fmt.Fprintf(w, "\n  %s %s\n", planRiskWord(a.Risk), a.Title)
			for _, s := range a.Steps {
				fmt.Fprintf(w, "    $ %s\n", s.Show)
			}
		}
		return nil
	}
	return executePlan(w, plan, todo, f)
}

func planImpact(a agent.PlannedAction) string {
	if a.Bytes > 0 {
		return ui.Bytes(a.Bytes)
	}
	return a.Impact
}

func planRiskWord(risk string) string {
	if risk == "dangerous" {
		return "▲"
	}
	return "●"
}

// pickActions keeps the ticked actions, in the plan's order.
func pickActions(acts []agent.PlannedAction, ids []string) []agent.PlannedAction {
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var out []agent.PlannedAction
	for _, a := range acts {
		if want[a.ID] {
			out = append(out, a)
		}
	}
	return out
}

// planGroups splits the ticked actions by the stage that found them, in
// the stages' order: each group is one stage of carrying out the plan.
func planGroups(todo []agent.PlannedAction) (order []string, groups map[string][]agent.PlannedAction) {
	groups = map[string][]agent.PlannedAction{}
	for _, a := range todo {
		if _, ok := groups[a.Stage]; !ok {
			order = append(order, a.Stage)
		}
		groups[a.Stage] = append(groups[a.Stage], a)
	}
	rank := map[string]int{}
	for i, s := range ultraStages() {
		rank[s.task] = i + 1
	}
	sort.SliceStable(order, func(i, j int) bool { return rank[order[i]] < rank[order[j]] })
	return order, groups
}

// executePlan carries out the ticked actions on the same screen the audit
// had, stage by stage, and reports how each one ended and the space freed.
func executePlan(w io.Writer, plan *agent.Plan, todo []agent.PlannedAction, f runFlags) error {
	order, groups := planGroups(todo)
	var stages []ui.GuideStage
	for _, st := range order {
		acts := groups[st]
		title := ultraStageTitle(st)
		if title == "" {
			title = st
		}
		risk := "normal"
		var names []string
		for _, a := range acts {
			names = append(names, a.Title)
			if a.Risk == "dangerous" {
				risk = "dangerous"
			}
		}
		stages = append(stages, ui.GuideStage{ID: st, Title: title, Command: i18n.T("ultra.n_actions", len(acts)), Risk: risk, Selected: true,
			What: strings.Join(names, " · "), Why: i18n.T("ultra.run_why")})
	}
	run := func(id string, out io.Writer) error { return runPlanned(groups[id], f, plan) }
	var res []ui.StageResult
	if planScreen(f.noTUI) {
		info := ui.PilotInfo{Version: version, Title: i18n.T("ultra.run_title"), Ultra: true, Note: i18n.T("ultra.run_note"), Flags: ultraFlags(f)}
		var err error
		res, _, err = runPilot(info, stages, run)
		if err != nil {
			return err
		}
	} else {
		stopped := interrupted()
		defer stopped.stop()
		res = runFull(w, stages, func(id string) error { return run(id, w) }, nil, stopped.check)
	}
	if err := plan.Save(); err != nil {
		fmt.Fprintln(w, "omega:", err)
	}
	planReport(w, plan, todo)
	fmt.Fprint(w, "\a")
	if fullFailed(res) {
		return exitCode(1)
	}
	return nil
}

// planReport is what stays on the terminal: every action and how it ended,
// the space measured before and after, and where the plan and logs are.
func planReport(w io.Writer, plan *agent.Plan, todo []agent.PlannedAction) {
	status := map[string]agent.PlannedAction{}
	for _, a := range plan.List() {
		status[a.ID] = a
	}
	fmt.Fprintf(w, "\n%s\n", i18n.T("ultra.results"))
	done, failed := 0, 0
	for _, a := range todo {
		got := status[a.ID]
		mark, note := "·", i18n.T("ultra.not_run")
		switch got.Status {
		case agent.ActionDone:
			mark, note = "✓", planImpact(a)
			done++
		case agent.ActionFailed:
			mark, note = "✗", got.Result
			failed++
		}
		line := fmt.Sprintf("  %s %s", mark, a.Title)
		if note != "" {
			line += " · " + note
		}
		fmt.Fprintln(w, line)
	}
	fmt.Fprintln(w, "  "+i18n.T("ultra.tally", done, failed, len(todo)-done-failed))
	if after, ok := freeSpace(); ok && ultraBefore > 0 {
		diff := int64(after) - int64(ultraBefore)
		sign := "+"
		if diff < 0 {
			sign, diff = "−", -diff
		}
		fmt.Fprintln(w, "  "+i18n.T("ultra.freed", ui.Bytes(int64(ultraBefore)), ui.Bytes(int64(after)), sign+ui.Bytes(diff)))
	}
	fmt.Fprintln(w, "  "+i18n.T("ultra.plan_at", agent.PlanPath()))
	if dir := agent.RunsDir(); dir != "" {
		fmt.Fprintln(w, "  "+i18n.T("full.logs", dir))
	}
}

// runPlanned carries out one stage's ticked actions with the executor: the
// same tools as a session, no model, the tick as the approval.
func runPlanned(acts []agent.PlannedAction, f runFlags, plan *agent.Plan) error {
	cfg, err := config.Load()
	if err != nil {
		cfg = config.Default()
	}
	timeout, perr := time.ParseDuration(cfg.Timeout)
	if perr != nil || timeout <= 0 {
		timeout = 45 * time.Minute
	}
	opts := agent.Options{Config: cfg, SudoPassword: sudoAsked, Version: version, Task: "ultra-plan",
		Command: strings.Join(os.Args, " "), Timeout: timeout, MaxTurns: 1}
	if f.demo {
		opts.Provider, opts.Exec, opts.SkipDetect = "anthropic", demo.Exec, true
	}
	ag := agent.NewExecutor(opts)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	go ag.RunPlanned(ctx, acts, plan.Mark)
	if ui.Piloted() {
		ui.PilotSession(ctx, cancel, ag, ui.Header{Version: version, Provider: "omega", Model: i18n.T("ultra.no_model"), Task: "ultra"})
	} else {
		ui.RunPlain(ctx, ag, os.Stdin, false)
	}
	for range ag.Events() {
	}
	f.share.changed()
	if code := ag.ExitCode(); code != 0 {
		return exitCode(code)
	}
	return nil
}

// interrupt notices ctrl+c in a run without the screen.
type interrupt struct {
	sig chan os.Signal
}

func interrupted() interrupt {
	i := interrupt{sig: make(chan os.Signal, 1)}
	signal.Notify(i.sig, os.Interrupt)
	return i
}

func (i interrupt) stop() { signal.Stop(i.sig) }

func (i interrupt) check() bool {
	select {
	case <-i.sig:
		return true
	default:
		return false
	}
}

// ultraStagesHere are the ultra stages that apply to this machine.
func ultraStagesHere() []fullStage {
	var out []fullStage
	for _, s := range ultraStages() {
		if s.when == nil || s.when() {
			out = append(out, s)
		}
	}
	return out
}

func ultraStageIDList() string {
	var ids []string
	for _, s := range ultraStages() {
		ids = append(ids, s.id)
	}
	return strings.Join(ids, ", ")
}

// pickUltraStages reads --only and --skip against the ultra stages.
func pickUltraStages(stages []fullStage, only, skip []string) (map[string]bool, error) {
	known := map[string]bool{}
	for _, s := range ultraStages() {
		known[s.id] = true
	}
	split := func(list []string) (map[string]bool, error) {
		out := map[string]bool{}
		for _, item := range list {
			for _, id := range strings.Split(item, ",") {
				if id = strings.ToLower(strings.TrimSpace(id)); id == "" {
					continue
				}
				if !known[id] {
					return nil, fmt.Errorf("%s", i18n.T("full.unknown_stage", id, ultraStageIDList()))
				}
				out[id] = true
			}
		}
		return out, nil
	}
	o, err := split(only)
	if err != nil {
		return nil, err
	}
	sk, err := split(skip)
	if err != nil {
		return nil, err
	}
	chosen := map[string]bool{}
	for _, s := range stages {
		chosen[s.id] = (len(o) == 0 || o[s.id]) && !sk[s.id]
	}
	return chosen, nil
}

// ultraModelLabel is what the top bar says about the models before the
// first stage starts: "smart → ultra".
func ultraModelLabel(f runFlags) string {
	if f.demo {
		return ""
	}
	cfg, err := config.Load()
	if err != nil {
		return ""
	}
	name := f.provider
	if name == "" {
		name = cfg.Provider
	}
	info, ok := llm.Lookup(name)
	if !ok {
		return ""
	}
	if f.model != "" {
		return f.model
	}
	_, smart := tiers(cfg, info)
	if top := ultraTier(cfg, info); top != smart {
		return smart + " → " + top
	}
	return smart
}
