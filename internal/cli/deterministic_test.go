package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/agent"
	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/runner"
	"github.com/lcajigasm/omega/internal/safety"
	"github.com/lcajigasm/omega/internal/steps"
)

// newDet is a run without the model whose commands are recorded instead of
// executed.
func newDet(f runFlags, tty bool, answer bool) (*detRun, *[]string, *int, *bytes.Buffer) {
	var ran []string
	asked := 0
	out := &bytes.Buffer{}
	d := &detRun{
		f:      f,
		cfg:    config.Default(),
		report: &DeterministicReport{},
		su:     &sudoSession{},
		out:    out,
		tty:    tty,
		confirm: func(string) bool {
			asked++
			return answer
		},
		exec: func(_ context.Context, command string, _ time.Duration, _ func(string)) runner.Result {
			ran = append(ran, command)
			if strings.Contains(command, "fails") {
				return runner.Result{ExitCode: 3}
			}
			return runner.Result{}
		},
	}
	return d, &ran, &asked, out
}

var (
	volumes = steps.Step{ID: "docker-volumes", Name: "Docker volumes", Category: steps.Clean, Command: "docker volume prune -a -f", Risk: safety.Dangerous, AlwaysConfirm: true}
	trash   = steps.Step{ID: "trash", Name: "Trash", Category: steps.Clean, Command: "rm -rf ~/.Trash/*", Risk: safety.Dangerous}
)

// --yes does not answer for a step that deletes Docker volumes, and without
// a terminal nobody can: it is skipped.
func TestDeterministicAlwaysConfirm(t *testing.T) {
	d, ran, asked, out := newDet(runFlags{yes: true}, false, true)
	d.change(context.Background(), volumes, time.Second)
	if len(*ran) != 0 || *asked != 0 || !strings.Contains(out.String(), "skipped") {
		t.Errorf("without a terminal: ran %v, asked %d, said %q", *ran, *asked, out.String())
	}

	d, ran, asked, _ = newDet(runFlags{yes: true}, true, false)
	d.change(context.Background(), volumes, time.Second)
	if len(*ran) != 0 || *asked != 1 {
		t.Errorf("answered no: ran %v, asked %d", *ran, *asked)
	}

	d, ran, asked, _ = newDet(runFlags{yes: true}, true, true)
	d.change(context.Background(), volumes, time.Second)
	if len(*ran) != 1 || *asked != 1 {
		t.Errorf("answered yes: ran %v, asked %d", *ran, *asked)
	}
}

func TestDeterministicDangerousStep(t *testing.T) {
	// --dry-run runs nothing, with or without --yes.
	d, ran, _, out := newDet(runFlags{yes: true, dryRun: true}, true, true)
	d.change(context.Background(), trash, time.Second)
	if len(*ran) != 0 || !strings.Contains(out.String(), "would run: trash") {
		t.Errorf("dry run: ran %v, said %q", *ran, out.String())
	}
	// No --yes and nobody to ask: skipped.
	d, ran, _, _ = newDet(runFlags{}, false, true)
	d.change(context.Background(), trash, time.Second)
	if len(*ran) != 0 {
		t.Errorf("ran without approval: %v", *ran)
	}
	// --yes is enough for an ordinary dangerous step.
	d, ran, asked, _ := newDet(runFlags{yes: true}, false, true)
	d.change(context.Background(), trash, time.Second)
	if len(*ran) != 1 || *asked != 0 {
		t.Errorf("--yes: ran %v, asked %d", *ran, *asked)
	}
}

// A step that fails makes the run fail.
func TestDeterministicExit(t *testing.T) {
	d, _, _, out := newDet(runFlags{yes: true}, false, true)
	d.change(context.Background(), steps.Step{ID: "x", Name: "x", Command: "fails", Risk: safety.Normal}, time.Second)
	if err := d.exit(); err == nil {
		t.Error("a failed step left exit status 0")
	}
	if !strings.Contains(out.String(), "✗") {
		t.Errorf("the failure was not shown: %q", out.String())
	}
	d, _, _, _ = newDet(runFlags{}, false, true)
	if err := d.exit(); err != nil {
		t.Errorf("an empty run failed: %v", err)
	}
	d.report.OutdatedCount = 2
	d.f.failOnOutdated = true
	if code, ok := d.exit().(exitCode); !ok || code != 2 {
		t.Errorf("--fail-on-outdated: %v", d.exit())
	}
}

// A check is read-only, whatever the catalogue files under "checks".
func TestDeterministicCheckOnlyReads(t *testing.T) {
	d, ran, _, _ := newDet(runFlags{}, false, true)
	start := steps.Step{ID: "docker-start", Name: "Start Docker", Category: steps.Check, Command: "open -ga Docker", Risk: safety.Dangerous}
	if _, ok := d.check(context.Background(), start, time.Second); ok || len(*ran) != 0 {
		t.Errorf("a dangerous step ran as a check: %v", *ran)
	}
	for _, s := range steps.For(goos) {
		if s.Category == steps.Check && s.Risk != safety.Safe {
			if _, ok := d.check(context.Background(), s, time.Second); ok {
				t.Errorf("%s ran as a check", s.ID)
			}
		}
	}
}

// The default task exists without the model.
func TestDeterministicFullIsSupported(t *testing.T) {
	d, _, _, _ := newDet(runFlags{dryRun: true}, false, true)
	if err := d.run(context.Background(), agent.TaskFull); err != nil {
		t.Fatal(err)
	}
	if err := d.run(context.Background(), "nope"); err == nil {
		t.Error("an unknown task was accepted")
	}
}

// The shell's commands are found by name and get a real command.
func TestShellSubcommands(t *testing.T) {
	if err := runSub(context.Background(), netCmd(), []string{"bogus"}); err == nil {
		t.Error("an unknown subcommand ran something")
	}
	for _, name := range []string{"ssh", "secrets"} {
		if c, _, err := auditCmd().Find([]string{name}); err != nil || c.Name() != name {
			t.Errorf("audit %s: %v", name, err)
		}
	}
}
