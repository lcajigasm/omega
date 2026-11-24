package customsteps

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/runner"
	"github.com/lcajigasm/omega/internal/safety"
)

// C7: a step of steps.d is at least as risky as its command.
func TestDeclaredRiskIsNotBelowTheCommand(t *testing.T) {
	stepsDir(t, map[string]string{
		"a.toml": "[[steps]]\nid = \"wipe\"\ncommand = \"rm -rf ~/Projects/old\"\nrisk = \"read-only\"\n\n" +
			"[[steps]]\nid = \"look\"\ncommand = \"echo hi\"\nrisk = \"dangerous\"\n",
	})
	loaded, problems := Load()
	if len(loaded) != 2 || len(problems) != 0 {
		t.Fatalf("%+v %q", loaded, problems)
	}
	if loaded[0].Risk != safety.Dangerous {
		t.Errorf("rm -rf declared read-only loaded as %v", loaded[0].Risk)
	}
	if loaded[1].Risk != safety.Dangerous {
		t.Errorf("a declared risk above the command's was lowered: %v", loaded[1].Risk)
	}
}

// fakeSystem answers the snapshot commands.
func fakeSystem(t *testing.T, system string, snapper bool, answer func(string) runner.Result) *[]string {
	t.Helper()
	i18n.Init("en")
	oldOS, oldRun, oldHas := goos, run, has
	t.Cleanup(func() { goos, run, has = oldOS, oldRun, oldHas })
	goos = system
	has = func(string) bool { return snapper }
	var ran []string
	run = func(_ context.Context, cmd string, _ time.Duration, _ func(string)) runner.Result {
		ran = append(ran, cmd)
		return answer(cmd)
	}
	return &ran
}

// C9: no snapshots is said, not an empty list.
func TestRollbackSaysWhenThereAreNone(t *testing.T) {
	fakeSystem(t, "darwin", false, func(string) runner.Result {
		return runner.Result{Output: "Snapshots for disk /:\n"}
	})
	snap, err := ListSnapshots(context.Background())
	if !errors.Is(err, ErrNoSnapshots) || len(snap.List) != 0 {
		t.Errorf("%+v %v", snap, err)
	}
	if msg, err := Rollback(context.Background()); err != nil || !strings.Contains(msg, "no snapshots") {
		t.Errorf("%q %v", msg, err)
	}
	fakeSystem(t, "darwin", false, func(string) runner.Result {
		return runner.Result{Output: "Snapshots for disk /:\ncom.apple.TimeMachine.2026-11-24-080000.local\n"}
	})
	if msg, err := Rollback(context.Background()); err != nil || !strings.Contains(msg, "Snapshots on this system: 1") || !strings.Contains(msg, "TimeMachine.2026-11-24") {
		t.Errorf("%q %v", msg, err)
	}
}

// C9: on Linux the listing never waits for a password.
func TestSnapperNeverWaitsForAPassword(t *testing.T) {
	ran := fakeSystem(t, "linux", true, func(cmd string) runner.Result {
		return runner.Result{ExitCode: 1, Output: "sudo: a password is required"}
	})
	if _, err := ListSnapshots(context.Background()); !errors.Is(err, ErrNeedsSudo) {
		t.Errorf("err %v", err)
	}
	for _, c := range *ran {
		if strings.Contains(c, "sudo ") && !strings.Contains(c, "sudo -n ") {
			t.Errorf("sudo may prompt: %s", c)
		}
	}
	if _, err := Rollback(context.Background()); err == nil || !strings.Contains(err.Error(), "sudo snapper list") {
		t.Errorf("rollback: %v", err)
	}

	table := " # | Type   | Pre # | Date | User | Cleanup | Description | Userdata\n" +
		"---+--------+-------+------+------+---------+-------------+---------\n" +
		"0  | single |       |      | root |         | current     |\n" +
		"1  | pre    |       | Mon  | root | number  | omega       |\n" +
		"2* | post   | 1     | Mon  | root | number  |             |\n"
	fakeSystem(t, "linux", true, func(string) runner.Result { return runner.Result{Output: table} })
	snap, err := ListSnapshots(context.Background())
	if err != nil || len(snap.List) != 2 || !strings.HasPrefix(snap.List[0], "1 ") {
		t.Errorf("%+v %v", snap, err)
	}
	fakeSystem(t, "linux", false, nil)
	if _, err := ListSnapshots(context.Background()); !errors.Is(err, ErrUnsupported) {
		t.Errorf("without snapper: %v", err)
	}
}
