package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/hardware"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/runner"
	"github.com/lcajigasm/omega/internal/safety"
	"github.com/lcajigasm/omega/internal/steps"
)

// StepRunRecord records the outcome of a single maintenance step.
type StepRunRecord struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Command  string `json:"command"`
	Success  bool   `json:"success"`
	ExitCode int    `json:"exit_code"`
	// Duration is in nanoseconds, as it always was; DurationMS is the same
	// time in milliseconds, for whoever reads the report.
	Duration   time.Duration `json:"duration"`
	DurationMS int64         `json:"duration_ms"`
	Output     string        `json:"output,omitempty"`
}

// PlannedStep is a step a run did not carry out: one --dry-run would have
// run, or one that was skipped and why.
type PlannedStep struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Command  string `json:"command"`
	Reason   string `json:"reason,omitempty"`
}

// DeterministicReport contains the full results of an offline or deterministic run.
type DeterministicReport struct {
	Task      string    `json:"task"`
	Timestamp time.Time `json:"timestamp"`
	// TotalDuration is in nanoseconds; TotalDurationMS in milliseconds.
	TotalDuration   time.Duration   `json:"total_duration"`
	TotalDurationMS int64           `json:"total_duration_ms"`
	DryRun          bool            `json:"dry_run"`
	StepsRun        []StepRunRecord `json:"steps_run"`
	WouldRun        []PlannedStep   `json:"would_run,omitempty"` // under --dry-run
	Skipped         []PlannedStep   `json:"skipped,omitempty"`   // nobody was there to confirm them
	FreedMB         int64           `json:"freed_mb,omitempty"`
	OutdatedCount   int             `json:"outdated_count,omitempty"`
	WarningCount    int             `json:"warning_count,omitempty"`
	Summary         string          `json:"summary"`
}

// batteryInfo is a seam the tests replace.
var batteryInfo = hardware.GetBatteryInfo

// detRun is one run without the model: where the readable text goes, how a
// step is confirmed, and what failed.
type detRun struct {
	f       runFlags
	cfg     *config.Config
	report  *DeterministicReport
	su      *sudoSession
	sudo    func(command string) error // prepares sudo for a command that uses it
	out     io.Writer                  // readable progress: nowhere when the answer is JSON
	tty     bool
	confirm func(question string) bool
	exec    func(ctx context.Context, command string, timeout time.Duration, onLine func(string)) runner.Result
	failed  int // steps that change something and did not work
}

// wantsJSON reports whether stdout has to hold one JSON document only.
func (f runFlags) wantsJSON() bool { return f.json || strings.EqualFold(f.format, "json") }

// deterministic reports whether the run was asked to do without the model.
func (f runFlags) deterministic() bool { return f.noAI || f.offline || f.wantsJSON() }

// validate refuses a flag value omega would otherwise ignore or misread.
func (f runFlags) validate() error {
	switch strings.ToLower(f.format) {
	case "", "json", "text":
	default:
		return fmt.Errorf("%s", i18n.T("err.bad_format", f.format))
	}
	switch strings.ToLower(f.effort) {
	case "", "low", "medium", "high", "xhigh", "max":
	default:
		return fmt.Errorf("--effort %s: %s", f.effort, i18n.T("config.bad_effort"))
	}
	if f.maxTurns < 0 {
		return fmt.Errorf("%s", i18n.T("err.negative", "--max-turns"))
	}
	if f.maxCost < 0 {
		return fmt.Errorf("%s", i18n.T("err.negative", "--max-cost"))
	}
	return nil
}

// hasDeterministic reports whether a task can be done without the model.
func hasDeterministic(task string) bool {
	switch task {
	case agent.TaskFull, agent.TaskQuick, agent.TaskUpdate, agent.TaskClean, agent.TaskAnalyze, agent.TaskHealth:
		return true
	}
	return false
}

// writeJSON prints a value as the one JSON document of a --json answer.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// runDeterministic runs a task without the model, between its hooks. A
// request in the user's own words and a task only the model can do are
// refused before anything is printed or run: answering them with the full
// maintenance would do what nobody asked for.
func runDeterministic(task string, f runFlags) error {
	if f.prompt != "" {
		return fmt.Errorf("%s", i18n.T("err.prompt_needs_ai"))
	}
	if !hasDeterministic(task) {
		return fmt.Errorf("%s", i18n.T("err.task_needs_ai", task))
	}
	return withHooks(task, f, func() error { return detTask(task, f) })
}

// detTask is the run itself, without the hooks.
func detTask(task string, f runFlags) error {
	// A configuration that cannot be read is not replaced by the defaults:
	// they would run the steps skip_steps turned off.
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	d := &detRun{
		f:       f,
		cfg:     cfg,
		report:  &DeterministicReport{Task: task, Timestamp: time.Now(), DryRun: f.dryRun, StepsRun: []StepRunRecord{}},
		su:      &sudoSession{},
		out:     os.Stdout,
		tty:     term.IsTerminal(int(os.Stdin.Fd())),
		confirm: askYesNo,
		exec:    runner.Run,
	}
	d.sudo = d.su.need
	defer d.su.close()
	if f.quiet || f.wantsJSON() {
		d.out = io.Discard
	}
	if f.wantsJSON() {
		// A question would be printed in the middle of the document.
		d.tty = false
	}
	err = d.run(context.Background(), task)
	if f.wantsJSON() {
		if encErr := writeJSON(os.Stdout, d.report); encErr != nil {
			return encErr
		}
	}
	if err != nil {
		return err
	}
	return d.exit()
}

// run does the task and fills the report.
func (d *detRun) run(ctx context.Context, task string) error {
	start := time.Now()
	defer func() {
		d.report.TotalDuration = time.Since(start)
		d.report.TotalDurationMS = d.report.TotalDuration.Milliseconds()
	}()
	switch task {
	case agent.TaskUpdate:
		d.update(ctx)
	case agent.TaskClean:
		d.clean(ctx)
	case agent.TaskAnalyze:
		d.analyze(ctx)
	case agent.TaskHealth:
		d.health(ctx)
	case agent.TaskQuick:
		d.quick(ctx)
	case agent.TaskFull:
		// Everything, in the order the model would do it.
		d.update(ctx)
		d.clean(ctx)
		d.analyze(ctx)
		d.report.Summary = fmt.Sprintf("Full maintenance: %d steps run, %d failed, %d outdated packages found",
			len(d.report.StepsRun), d.failed, d.report.OutdatedCount)
	default:
		return fmt.Errorf("%s", i18n.T("err.task_needs_ai", task))
	}
	return nil
}

// exit is the status of the run: a step that failed is an error, and the
// --fail-on flags ask for more.
func (d *detRun) exit() error {
	if d.failed > 0 {
		return exitCode(1)
	}
	if d.f.failOnCritical {
		for _, s := range d.report.StepsRun {
			if !s.Success {
				return exitCode(1)
			}
		}
	}
	if d.f.failOnOutdated && d.report.OutdatedCount > 0 {
		return exitCode(2)
	}
	if d.f.failOnWarning && d.report.WarningCount > 0 {
		return exitCode(2)
	}
	return nil
}

// needsAnswer reports whether a person has to say yes before a step runs,
// by the rule the agent follows.
//
// A step marked AlwaysConfirm (the Docker cleanups, which delete volumes
// and images) needs a person at a terminal, every time: --yes does not
// answer for it. A read-only step never asks. Any other step asks when it is
// dangerous, or when the configuration says approval = "all", unless --yes
// was given.
func needsAnswer(cfg *config.Config, s steps.Step, yes bool) bool {
	if s.AlwaysConfirm || safety.AlwaysConfirm(s.Command) {
		return true
	}
	if s.Risk == safety.Safe || yes {
		return false
	}
	return s.Risk >= safety.Dangerous || cfg.Approval == config.ApproveAll
}

// planned is a step as the report lists the ones that did not run.
func planned(s steps.Step, reason string) PlannedStep {
	return PlannedStep{ID: s.ID, Name: s.Name, Category: s.Category, Command: s.Command, Reason: reason}
}

// allowed decides whether a step that changes something runs: at once when
// nobody has to answer for it, after a yes at the terminal otherwise, and
// not at all when there is no terminal, which is said and reported.
func (d *detRun) allowed(s steps.Step) bool {
	if !needsAnswer(d.cfg, s, d.f.yes) {
		return true
	}
	if !d.tty {
		fmt.Fprintf(d.out, "  – %s (skipped: needs a confirmation at a terminal)\n", s.ID)
		d.report.Skipped = append(d.report.Skipped, planned(s, "needs a confirmation at a terminal"))
		return false
	}
	// The question is asked on the terminal even when stdout is JSON.
	if !d.confirm(fmt.Sprintf("Run '%s'? ($ %s)", s.Name, s.Command)) {
		d.report.Skipped = append(d.report.Skipped, planned(s, "declined"))
		return false
	}
	return true
}

// record adds a step that ran to the report.
func (d *detRun) record(s steps.Step, res runner.Result, dur time.Duration, output string) {
	d.report.StepsRun = append(d.report.StepsRun, StepRunRecord{
		ID: s.ID, Name: s.Name, Category: s.Category, Command: s.Command,
		Success: res.ExitCode == 0, ExitCode: res.ExitCode, Duration: dur, DurationMS: dur.Milliseconds(), Output: output,
	})
}

// change runs a step that changes something, after the dry-run and the
// confirmation rules.
func (d *detRun) change(ctx context.Context, s steps.Step, timeout time.Duration) {
	if d.f.dryRun {
		fmt.Fprintf(d.out, "  [dry-run] would run: %s (%s)\n", s.ID, s.Command)
		d.report.WouldRun = append(d.report.WouldRun, planned(s, ""))
		return
	}
	if !d.allowed(s) {
		return
	}
	fmt.Fprintf(d.out, "⏺ %s ... ", s.Name)
	// Without the password the command would stop at sudo's own prompt,
	// which nobody can answer: the step fails here instead.
	if err := d.sudo(s.Command); err != nil {
		d.failed++
		d.record(s, runner.Result{ExitCode: 1}, 0, "sudo: "+err.Error())
		fmt.Fprintf(d.out, "✗ (sudo: %v)\n", err)
		return
	}

	var logFn func(string)
	if d.f.verbose {
		fmt.Fprintln(d.out)
		logFn = func(l string) { fmt.Fprintln(d.out, "    "+l) }
	}
	stepStart := time.Now()
	res := d.exec(ctx, s.Command, timeout, logFn)
	dur := time.Since(stepStart)
	d.record(s, res, dur, res.Output)
	if res.ExitCode == 0 {
		fmt.Fprintf(d.out, "✓ (%s)\n", dur.Round(time.Millisecond))
		return
	}
	d.failed++
	fmt.Fprintf(d.out, "✗ (exit %d)\n", res.ExitCode)
}

// check runs a read-only step. Anything else is not a check, whatever its
// category says: "docker-start" is listed with the checks and starts Docker.
func (d *detRun) check(ctx context.Context, s steps.Step, timeout time.Duration) (runner.Result, bool) {
	if s.Risk != safety.Safe || !s.Available() || d.cfg.Skipped(s.ID) {
		return runner.Result{}, false
	}
	began := time.Now()
	res := d.exec(ctx, s.Command, timeout, nil)
	d.record(s, res, time.Since(began), strings.TrimSpace(res.Output))
	if res.ExitCode != 0 {
		d.report.WarningCount++
	}
	return res, true
}

func (d *detRun) timeout(def time.Duration) time.Duration {
	if t, _ := time.ParseDuration(d.cfg.Timeout); t > 0 {
		return t
	}
	return def
}

func (d *detRun) update(ctx context.Context) {
	fmt.Fprintf(d.out, "\n⚡ Omega Deterministic Mode: Update\n\n")
	before := len(d.report.StepsRun)
	for _, s := range steps.For(goos) {
		if s.Category != steps.Update || !s.Available() || d.cfg.Skipped(s.ID) {
			continue
		}
		d.change(ctx, s, d.timeout(30*time.Minute))
	}
	d.report.Summary = fmt.Sprintf("Updated packages with %d steps executed", len(d.report.StepsRun)-before)
}

func (d *detRun) clean(ctx context.Context) {
	fmt.Fprintf(d.out, "\n⚡ Omega Deterministic Mode: Clean\n\n")
	before := len(d.report.StepsRun)
	for _, s := range steps.For(goos) {
		if s.Category != steps.Clean || !s.Available() || d.cfg.Skipped(s.ID) {
			continue
		}
		d.change(ctx, s, d.timeout(15*time.Minute))
	}
	d.report.Summary = fmt.Sprintf("Clean completed: %d steps run", len(d.report.StepsRun)-before)
}

// outdatedNoise starts the lines a package manager prints around its list:
// warnings, headers, progress and the closing sentence.
var outdatedNoise = []string{"warning", "error", "==>", "note:", "npm warn", "listing...", "loading ", "reading ",
	"last metadata", "no updates", "no installed package", "all snaps", "everything is ok", "scoop is up to date",
	"chocolatey ", "outdated packages", "output is package", "name ", "obsoleting", "wsl:"}

// outdatedCount counts the packages a "what is outdated" check lists. Only
// the lines that name a package count: a warning is not something to update.
func outdatedCount(id, output string) int {
	n := 0
lines:
	for _, line := range strings.Split(output, "\n") {
		l := strings.TrimSpace(line)
		if strings.Trim(l, "-+=|\\/ ") == "" {
			continue // empty, a rule under a header or a spinner
		}
		lower := strings.ToLower(l)
		for _, p := range outdatedNoise {
			if strings.HasPrefix(lower, p) {
				continue lines
			}
		}
		fields := len(strings.Fields(l))
		ok := true
		switch id {
		case "brew-outdated": // "git (2.44.0) < 2.45.0", or the bare name
			ok = strings.Contains(l, " < ") || strings.Contains(l, " != ") || fields == 1
		case "apt-outdated": // "git/stable 1:2.45 amd64 [upgradable from: 1:2.44]"
			ok = strings.Contains(l, "/") && strings.Contains(l, "[")
		case "dnf-outdated": // "git.x86_64  2.45.0-1.fc40  updates"
			ok = fields == 3
		case "pacman-outdated": // "git 2.44.0-1 -> 2.45.0-1"
			ok = strings.Contains(l, " -> ")
		case "zypper-outdated": // "v | repo | git | 2.44 | 2.45 | x86_64"
			ok = strings.HasPrefix(l, "v ") || strings.HasPrefix(l, "v|")
		case "choco-outdated": // "git|2.44.0|2.45.0|false"
			ok = strings.Count(l, "|") >= 3
		case "winget-outdated": // "Git  Git.Git  2.44.0  2.45.0  winget"; "3 upgrades available."
			ok = fields >= 4 && !strings.HasSuffix(l, ".")
		}
		if ok {
			n++
		}
	}
	return n
}

func (d *detRun) analyze(ctx context.Context) {
	fmt.Fprintf(d.out, "\n⚡ Omega Deterministic Mode: Analysis Report\n\n")
	checks := 0
	for _, s := range steps.For(goos) {
		if s.Category != steps.Check {
			continue
		}
		res, ran := d.check(ctx, s, 30*time.Second)
		if !ran {
			continue
		}
		checks++
		if strings.Contains(s.ID, "outdated") {
			d.report.OutdatedCount += outdatedCount(s.ID, res.Output)
		}
		fmt.Fprintf(d.out, "── %s ──\n%s\n\n", s.Name, res.Output)
	}
	d.report.Summary = fmt.Sprintf("Analysis completed: %d checks executed, %d outdated packages found", checks, d.report.OutdatedCount)
}

func (d *detRun) health(ctx context.Context) {
	fmt.Fprintf(d.out, "\n⚡ Omega Deterministic Mode: System Health\n\n")
	if bat, _ := batteryInfo(ctx); bat != nil && bat.Supported {
		fmt.Fprintf(d.out, "🔋 Battery: %s\n\n", bat.Text())
	}
	for _, s := range steps.For(goos) {
		if s.ID != "health" && s.ID != "security" && s.ID != "disk" {
			continue
		}
		if res, ran := d.check(ctx, s, 20*time.Second); ran {
			fmt.Fprintf(d.out, "── %s ──\n%s\n\n", s.Name, strings.TrimSpace(res.Output))
		}
	}
	d.report.Summary = fmt.Sprintf("Health checks completed: %d run, %d with warnings", len(d.report.StepsRun), d.report.WarningCount)
}

// quick is the short safe round: three cleanups, then a look at the battery,
// the disk and what is outdated. Its last line says what was looked at and
// what went wrong, and nothing more: it does not call a machine healthy.
func (d *detRun) quick(ctx context.Context) {
	fmt.Fprintf(d.out, "\n✨ Omega Quick Safe Maintenance\n")
	fmt.Fprintln(d.out, "Running the safe cleanups, then checking the battery, the disk and pending updates...")
	fmt.Fprintln(d.out)

	// 1. Cleanups. Emptying the Trash deletes for good, so it follows the
	// same rules as every other step: --dry-run shows it, --yes or an
	// answer at the terminal allows it.
	for _, id := range []string{"trash", "thumbnail-cache", "temp-files"} {
		if s, ok := steps.Find(id); ok && s.Available() && !d.cfg.Skipped(s.ID) {
			d.change(ctx, s, 30*time.Second)
		}
	}
	cleaned := len(d.report.StepsRun)

	// 2. Battery
	var looked []string
	if bat, _ := batteryInfo(ctx); bat != nil && bat.Supported {
		fmt.Fprintf(d.out, "  • Battery health: %s\n", bat.HealthText())
		looked = append(looked, "battery")
	}

	// 3. Disk space and outdated software: read-only checks, which leave
	// out what skip_steps turned off like any other run.
	diskSeen, outdatedChecks, checksFailed := false, 0, 0
	for _, s := range steps.For(goos) {
		outdated := strings.HasSuffix(s.ID, "-outdated")
		if s.ID != "disk" && !outdated {
			continue
		}
		res, ran := d.check(ctx, s, 15*time.Second)
		if !ran {
			continue
		}
		out := strings.TrimSpace(res.Output)
		switch {
		case res.ExitCode != 0:
			checksFailed++
			fmt.Fprintf(d.out, "  • %s: could not be checked (exit %d)\n", s.Name, res.ExitCode)
		case outdated:
			outdatedChecks++
			d.report.OutdatedCount += outdatedCount(s.ID, res.Output)
		case d.f.verbose:
			diskSeen = true
			fmt.Fprintf(d.out, "── %s ──\n%s\n", s.Name, out)
		default:
			diskSeen = true
			// df prints its header first; the line after it is the disk.
			if rows := strings.Split(out, "\n"); len(rows) > 1 && goos != "windows" {
				fmt.Fprintf(d.out, "  • Disk: %s\n", strings.Join(strings.Fields(rows[1]), " "))
			}
		}
	}
	if diskSeen {
		looked = append(looked, "disk")
	}
	if outdatedChecks > 0 {
		looked = append(looked, "pending updates")
	}

	var wrong []string
	if d.failed > 0 {
		wrong = append(wrong, fmt.Sprintf("%d cleanup steps failed", d.failed))
	}
	if checksFailed > 0 {
		wrong = append(wrong, fmt.Sprintf("%d checks could not be run", checksFailed))
	}
	state := "No problems found."
	if len(wrong) > 0 {
		state = strings.Join(wrong, ", ") + "."
	}
	checked := "nothing"
	if len(looked) > 0 {
		checked = strings.Join(looked, ", ")
	}
	d.report.Summary = fmt.Sprintf("%s %d cleanup steps run; checked: %s; %d package updates pending.",
		state, cleaned, checked, d.report.OutdatedCount)
	fmt.Fprintf(d.out, "\n%s\n", d.report.Summary)
	if skipped := len(d.report.Skipped); skipped > 0 {
		fmt.Fprintf(d.out, "%d cleanup steps were skipped: they need a yes at a terminal, or --yes.\n", skipped)
	}
	if d.report.OutdatedCount > 0 {
		fmt.Fprintf(d.out, "💡 Run 'omega update' to update your software.\n")
	}
}
