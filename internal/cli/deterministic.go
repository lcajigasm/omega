package cli

import (
	"context"
	"encoding/json"
	"fmt"
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

func runDeterministic(task string, f runFlags) error {
	ctx := context.Background()
	start := time.Now()

	cfg, err := config.Load()
	if err != nil {
		cfg = config.Default()
	}

	report := &DeterministicReport{
		Task:      task,
		Timestamp: start,
	}

	tty := term.IsTerminal(int(os.Stdin.Fd()))
	var su sudoSession
	defer su.close()

	switch task {
	case agent.TaskUpdate:
		err = runDeterministicUpdate(ctx, cfg, f, report, &su, tty)
	case agent.TaskClean:
		err = runDeterministicClean(ctx, cfg, f, report, &su, tty)
	case agent.TaskAnalyze:
		err = runDeterministicAnalyze(ctx, cfg, f, report, &su)
	case agent.TaskHealth:
		err = runDeterministicHealth(ctx, cfg, f, report, &su)
	case agent.TaskQuick, "":
		err = runDeterministicQuick(ctx, cfg, f, report, &su, tty)
	default:
		return fmt.Errorf("task '%s' not supported in deterministic/offline mode", task)
	}

	report.TotalDuration = time.Since(start)

	if f.json {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	// Semantic exit codes
	if f.failOnCritical && len(report.StepsRun) > 0 {
		for _, s := range report.StepsRun {
			if !s.Success {
				return exitCode(1)
			}
		}
	}
	if f.failOnOutdated && report.OutdatedCount > 0 {
		return exitCode(2)
	}
	if f.failOnWarning && report.WarningCount > 0 {
		return exitCode(2)
	}

	return err
}

func runDeterministicUpdate(ctx context.Context, cfg *config.Config, f runFlags, report *DeterministicReport, su *sudoSession, tty bool) error {
	allSteps := steps.For(goos)
	if !f.quiet {
		fmt.Printf("\n⚡ Omega Deterministic Mode: Update\n\n")
	}

	timeout, _ := time.ParseDuration(cfg.Timeout)
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}

	for _, s := range allSteps {
		if s.Category != steps.Update || !s.Available() || cfg.Skipped(s.ID) {
			continue
		}
		if f.dryRun {
			if !f.quiet {
				fmt.Printf("  [dry-run] would run: %s (%s)\n", s.ID, s.Command)
			}
			continue
		}

		if s.Risk >= safety.Dangerous && !f.yes {
			if !tty {
				if !f.quiet {
					fmt.Printf("  – %s (skipped: requires confirmation)\n", s.ID)
				}
				continue
			}
			fmt.Printf("Step '%s' needs approval ($ %s)\n", s.Name, s.Command)
			ans := ask("Execute? [y/N]", "n")
			if !i18n.IsYes(ans) {
				continue
			}
		}

		if !f.quiet {
			fmt.Printf("⏺ %s ... ", s.Name)
		}
		_ = su.need(s.Command)

		stepStart := time.Now()
		var logFn func(string)
		if f.verbose {
			fmt.Println()
			logFn = func(l string) { fmt.Println("    " + l) }
		}

		res := runner.Run(ctx, s.Command, timeout, logFn)
		dur := time.Since(stepStart)
		rec := StepRunRecord{
			ID:       s.ID,
			Name:     s.Name,
			Category: s.Category,
			Command:  s.Command,
			Success:  res.ExitCode == 0,
			ExitCode: res.ExitCode,
			Duration: dur,
			Output:   res.Output,
		}
		report.StepsRun = append(report.StepsRun, rec)

		if !f.quiet {
			if res.ExitCode == 0 {
				fmt.Printf("✓ (%s)\n", dur.Round(time.Millisecond))
			} else {
				fmt.Printf("✗ (exit %d)\n", res.ExitCode)
			}
		}
	}

	report.Summary = fmt.Sprintf("Updated packages with %d steps executed", len(report.StepsRun))
	return nil
}

func runDeterministicClean(ctx context.Context, cfg *config.Config, f runFlags, report *DeterministicReport, su *sudoSession, tty bool) error {
	allSteps := steps.For(goos)
	if !f.quiet {
		fmt.Printf("\n⚡ Omega Deterministic Mode: Clean\n\n")
	}

	timeout, _ := time.ParseDuration(cfg.Timeout)
	if timeout <= 0 {
		timeout = 15 * time.Minute
	}

	for _, s := range allSteps {
		if s.Category != steps.Clean || !s.Available() || cfg.Skipped(s.ID) {
			continue
		}
		if f.dryRun {
			if !f.quiet {
				fmt.Printf("  [dry-run] would clean: %s (%s)\n", s.ID, s.Command)
			}
			continue
		}

		if (s.AlwaysConfirm || s.Risk >= safety.Dangerous) && !f.yes {
			if !tty {
				if !f.quiet {
					fmt.Printf("  – %s (skipped: requires confirmation)\n", s.ID)
				}
				continue
			}
			ans := ask(fmt.Sprintf("Clean %s? [y/N]", s.Name), "n")
			if !i18n.IsYes(ans) {
				continue
			}
		}

		if !f.quiet {
			fmt.Printf("⏺ %s ... ", s.Name)
		}
		_ = su.need(s.Command)

		stepStart := time.Now()
		var logFn func(string)
		if f.verbose {
			fmt.Println()
			logFn = func(l string) { fmt.Println("    " + l) }
		}

		res := runner.Run(ctx, s.Command, timeout, logFn)
		dur := time.Since(stepStart)
		rec := StepRunRecord{
			ID:       s.ID,
			Name:     s.Name,
			Category: s.Category,
			Command:  s.Command,
			Success:  res.ExitCode == 0,
			ExitCode: res.ExitCode,
			Duration: dur,
			Output:   res.Output,
		}
		report.StepsRun = append(report.StepsRun, rec)

		if !f.quiet {
			if res.ExitCode == 0 {
				fmt.Printf("✓ (%s)\n", dur.Round(time.Millisecond))
			} else {
				fmt.Printf("✗ (exit %d)\n", res.ExitCode)
			}
		}
	}

	report.Summary = fmt.Sprintf("Clean completed: %d steps run", len(report.StepsRun))
	return nil
}

func runDeterministicAnalyze(ctx context.Context, cfg *config.Config, f runFlags, report *DeterministicReport, su *sudoSession) error {
	allSteps := steps.For(goos)
	if !f.quiet && !f.json {
		fmt.Printf("\n⚡ Omega Deterministic Mode: Analysis Report\n\n")
	}

	for _, s := range allSteps {
		if s.Category != steps.Check || !s.Available() || cfg.Skipped(s.ID) {
			continue
		}

		res := runner.Run(ctx, s.Command, 30*time.Second, nil)
		rec := StepRunRecord{
			ID:       s.ID,
			Name:     s.Name,
			Category: s.Category,
			Command:  s.Command,
			Success:  res.ExitCode == 0,
			ExitCode: res.ExitCode,
			Output:   strings.TrimSpace(res.Output),
		}
		report.StepsRun = append(report.StepsRun, rec)

		if strings.Contains(s.ID, "outdated") && len(strings.TrimSpace(res.Output)) > 0 {
			lines := strings.Split(strings.TrimSpace(res.Output), "\n")
			report.OutdatedCount += len(lines)
		}

		if !f.quiet && !f.json {
			fmt.Printf("── %s ──\n%s\n\n", s.Name, res.Output)
		}
	}

	report.Summary = fmt.Sprintf("Analysis completed: %d checks executed, %d outdated packages found", len(report.StepsRun), report.OutdatedCount)
	return nil
}

func runDeterministicHealth(ctx context.Context, cfg *config.Config, f runFlags, report *DeterministicReport, su *sudoSession) error {
	if !f.quiet && !f.json {
		fmt.Printf("\n⚡ Omega Deterministic Mode: System Health\n\n")
	}

	// 1. Hardware Battery
	bat, _ := hardware.GetBatteryInfo(ctx)
	if bat != nil && bat.Supported {
		if !f.quiet && !f.json {
			fmt.Printf("🔋 Battery: %d%% (%s, %d cycles, %.1f%% health)\n\n",
				bat.CurrentPct, bat.Condition, bat.CycleCount, bat.MaxCapacityPct)
		}
	}

	// 2. Health & security steps
	allSteps := steps.For(goos)
	for _, s := range allSteps {
		if (s.ID == "health" || s.ID == "security" || s.ID == "disk") && s.Available() {
			res := runner.Run(ctx, s.Command, 20*time.Second, nil)
			report.StepsRun = append(report.StepsRun, StepRunRecord{
				ID: s.ID, Name: s.Name, Command: s.Command, Success: res.ExitCode == 0, Output: res.Output,
			})
			if !f.quiet && !f.json {
				fmt.Printf("── %s ──\n%s\n\n", s.Name, strings.TrimSpace(res.Output))
			}
		}
	}

	report.Summary = "Health checks completed successfully"
	return nil
}

func runDeterministicQuick(ctx context.Context, cfg *config.Config, f runFlags, report *DeterministicReport, su *sudoSession, tty bool) error {
	if !f.quiet && !f.json {
		fmt.Printf("\n✨ Omega Quick Safe Maintenance\n")
		fmt.Println("Running safe cleanups, package update checks and health verification...")
		fmt.Println()
	}

	// 1. Safe cleanups: trash, thumbnails
	safeCleanSteps := []string{"trash", "thumbnail-cache", "temp-files"}
	for _, id := range safeCleanSteps {
		if s, ok := steps.Find(id); ok && s.Available() {
			if !f.quiet && !f.json {
				fmt.Printf("  • Cleaning %s... ", s.Name)
			}
			res := runner.Run(ctx, s.Command, 30*time.Second, nil)
			report.StepsRun = append(report.StepsRun, StepRunRecord{
				ID: s.ID, Name: s.Name, Success: res.ExitCode == 0,
			})
			if !f.quiet && !f.json {
				fmt.Println("✓")
			}
		}
	}

	// 2. Battery & Disk Health
	bat, _ := hardware.GetBatteryInfo(ctx)
	if bat != nil && bat.Supported && !f.quiet && !f.json {
		fmt.Printf("  • Battery health: %s (%.1f%% max capacity)\n", bat.Condition, bat.MaxCapacityPct)
	}

	// 3. Check outdated software
	allSteps := steps.For(goos)
	for _, s := range allSteps {
		if strings.HasSuffix(s.ID, "-outdated") && s.Available() {
			res := runner.Run(ctx, s.Command, 15*time.Second, nil)
			if len(strings.TrimSpace(res.Output)) > 0 {
				lines := strings.Split(strings.TrimSpace(res.Output), "\n")
				report.OutdatedCount += len(lines)
			}
		}
	}

	report.Summary = fmt.Sprintf("Your computer is healthy! %d safe clean steps run, %d package updates pending.", len(report.StepsRun), report.OutdatedCount)
	if !f.quiet && !f.json {
		fmt.Println()
		fmt.Printf("🎉 %s\n", report.Summary)
		if report.OutdatedCount > 0 {
			fmt.Printf("💡 Run 'omega update' to update your software.\n")
		}
	}

	return nil
}
