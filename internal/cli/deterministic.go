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
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Category string        `json:"category"`
	Command  string        `json:"command"`
	Success  bool          `json:"success"`
	ExitCode int           `json:"exit_code"`
	Duration time.Duration `json:"duration"`
	Output   string        `json:"output,omitempty"`
}

// DeterministicReport contains the full results of an offline or deterministic run.
type DeterministicReport struct {
	Task          string          `json:"task"`
	Timestamp     time.Time       `json:"timestamp"`
	TotalDuration time.Duration   `json:"total_duration"`
	StepsRun      []StepRunRecord `json:"steps_run"`
	FreedMB       int64           `json:"freed_mb,omitempty"`
	OutdatedCount int             `json:"outdated_count,omitempty"`
	WarningCount  int             `json:"warning_count,omitempty"`
	Summary       string          `json:"summary"`
}

// detRun is one run without the model: where the readable text goes, how a
// step is confirmed, and what failed.
type detRun struct {
	f       runFlags
	cfg     *config.Config
	report  *DeterministicReport
	su      *sudoSession
	out     io.Writer // readable progress: nowhere when the answer is JSON
	tty     bool
	confirm func(question string) bool
	exec    func(ctx context.Context, command string, timeout time.Duration, onLine func(string)) runner.Result
	failed  int // steps that change something and did not work
}

// wantsJSON reports whether stdout has to hold one JSON document only.
func (f runFlags) wantsJSON() bool { return f.json || strings.EqualFold(f.format, "json") }

func runDeterministic(task string, f runFlags) error {
	cfg, err := config.Load()
	if err != nil {
		cfg = config.Default()
	}
	d := &detRun{
		f:      f,
		cfg:    cfg,
		report: &DeterministicReport{Task: task, Timestamp: time.Now()},
		su:     &sudoSession{},
		out:    os.Stdout,
		tty:    term.IsTerminal(int(os.Stdin.Fd())),
		confirm: func(q string) bool {
			return i18n.IsYes(ask(q+" [y/N]", "n"))
		},
		exec: runner.Run,
	}
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
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if encErr := enc.Encode(d.report); encErr != nil {
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
	defer func() { d.report.TotalDuration = time.Since(start) }()
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
	case agent.TaskFull, "":
		// Everything, in the order the model would do it.
		d.update(ctx)
		d.clean(ctx)
		d.analyze(ctx)
		d.report.Summary = fmt.Sprintf("Full maintenance: %d steps run, %d failed, %d outdated packages found",
			len(d.report.StepsRun), d.failed, d.report.OutdatedCount)
	default:
		return fmt.Errorf("task '%s' not supported in deterministic/offline mode", task)
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

// allowed decides whether a step that changes something runs.
//
// A step marked AlwaysConfirm (the Docker cleanups, which delete volumes
// and images) needs a person to say yes at a terminal, every time: --yes
// does not answer for it, and without a terminal it is skipped. Any other
// dangerous step runs under --yes, is asked at a terminal, and is skipped
// when there is neither.
func (d *detRun) allowed(s steps.Step) bool {
	always := s.AlwaysConfirm || safety.AlwaysConfirm(s.Command)
	if !always && (s.Risk < safety.Dangerous || d.f.yes) {
		return true
	}
	if !d.tty {
		fmt.Fprintf(d.out, "  – %s (skipped: needs a confirmation at a terminal)\n", s.ID)
		return false
	}
	// The question is asked on the terminal even when stdout is JSON.
	return d.confirm(fmt.Sprintf("Run '%s'? ($ %s)", s.Name, s.Command))
}

// change runs a step that changes something, after the dry-run and the
// confirmation rules.
func (d *detRun) change(ctx context.Context, s steps.Step, timeout time.Duration) {
	if d.f.dryRun {
		fmt.Fprintf(d.out, "  [dry-run] would run: %s (%s)\n", s.ID, s.Command)
		return
	}
	if !d.allowed(s) {
		return
	}
	fmt.Fprintf(d.out, "⏺ %s ... ", s.Name)
	_ = d.su.need(s.Command)

	var logFn func(string)
	if d.f.verbose {
		fmt.Fprintln(d.out)
		logFn = func(l string) { fmt.Fprintln(d.out, "    "+l) }
	}
	stepStart := time.Now()
	res := d.exec(ctx, s.Command, timeout, logFn)
	dur := time.Since(stepStart)
	d.report.StepsRun = append(d.report.StepsRun, StepRunRecord{
		ID: s.ID, Name: s.Name, Category: s.Category, Command: s.Command,
		Success: res.ExitCode == 0, ExitCode: res.ExitCode, Duration: dur, Output: res.Output,
	})
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
	res := d.exec(ctx, s.Command, timeout, nil)
	d.report.StepsRun = append(d.report.StepsRun, StepRunRecord{
		ID: s.ID, Name: s.Name, Category: s.Category, Command: s.Command,
		Success: res.ExitCode == 0, ExitCode: res.ExitCode, Output: strings.TrimSpace(res.Output),
	})
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

func outdatedLines(output string) int {
	if out := strings.TrimSpace(output); out != "" {
		return len(strings.Split(out, "\n"))
	}
	return 0
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
			d.report.OutdatedCount += outdatedLines(res.Output)
		}
		fmt.Fprintf(d.out, "── %s ──\n%s\n\n", s.Name, res.Output)
	}
	d.report.Summary = fmt.Sprintf("Analysis completed: %d checks executed, %d outdated packages found", checks, d.report.OutdatedCount)
}

func (d *detRun) health(ctx context.Context) {
	fmt.Fprintf(d.out, "\n⚡ Omega Deterministic Mode: System Health\n\n")
	if bat, _ := hardware.GetBatteryInfo(ctx); bat != nil && bat.Supported {
		fmt.Fprintf(d.out, "🔋 Battery: %d%% (%s, %d cycles, %.1f%% health)\n\n",
			bat.CurrentPct, bat.Condition, bat.CycleCount, bat.MaxCapacityPct)
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

func (d *detRun) quick(ctx context.Context) {
	fmt.Fprintf(d.out, "\n✨ Omega Quick Safe Maintenance\n")
	fmt.Fprintln(d.out, "Running safe cleanups, package update checks and health verification...")
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
	if bat, _ := hardware.GetBatteryInfo(ctx); bat != nil && bat.Supported {
		fmt.Fprintf(d.out, "  • Battery health: %s (%.1f%% max capacity)\n", bat.Condition, bat.MaxCapacityPct)
	}

	// 3. Outdated software
	for _, s := range steps.For(goos) {
		if strings.HasSuffix(s.ID, "-outdated") && s.Risk == safety.Safe && s.Available() {
			res := d.exec(ctx, s.Command, 15*time.Second, nil)
			d.report.OutdatedCount += outdatedLines(res.Output)
		}
	}

	state := "Your computer is healthy!"
	if d.failed > 0 {
		state = fmt.Sprintf("%d cleanup steps failed.", d.failed)
	}
	d.report.Summary = fmt.Sprintf("%s %d clean steps run, %d package updates pending.", state, cleaned, d.report.OutdatedCount)
	fmt.Fprintf(d.out, "\n%s\n", d.report.Summary)
	if d.report.OutdatedCount > 0 {
		fmt.Fprintf(d.out, "💡 Run 'omega update' to update your software.\n")
	}
}
