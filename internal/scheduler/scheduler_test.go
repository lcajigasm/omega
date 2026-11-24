package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/runner"
)

// fake points the package at an empty home and at a pretend system: every
// command is recorded and answered by answer (nil answers 0). No test
// reaches launchd, systemd, the Task Scheduler or the notification centre.
func fake(t *testing.T, system string, answer func(command string) runner.Result) *[]string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // the home directory on Windows
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	oldOS, oldRun, oldArgs, oldExe, oldSettle := goos, run, runArgs, executable, settle
	t.Cleanup(func() { goos, run, runArgs, executable, settle = oldOS, oldRun, oldArgs, oldExe, oldSettle })
	goos, settle = system, 0
	var ran []string
	run = func(_ context.Context, command string, _ time.Duration) runner.Result {
		ran = append(ran, command)
		if answer == nil {
			return runner.Result{}
		}
		return answer(command)
	}
	runArgs = func(name string, args ...string) ([]byte, error) {
		ran = append(ran, name+" "+strings.Join(args, " "))
		return nil, nil
	}
	executable = func() (string, error) { return filepath.Join(home, "bin", "omega"), nil }
	return &ran
}

// launchd refusing the job is an error, and nothing is left that would
// make Status say "installed".
func TestInstallFailsWhenLaunchdRefuses(t *testing.T) {
	ran := fake(t, "darwin", func(command string) runner.Result {
		switch {
		case strings.HasPrefix(command, "launchctl bootstrap"):
			return runner.Result{ExitCode: 5, Output: "Bootstrap failed: 5: Input/output error"}
		case strings.HasPrefix(command, "launchctl"):
			return runner.Result{ExitCode: 113} // not loaded
		}
		return runner.Result{}
	})
	err := Install("weekly", "quick", 3, 0)
	if err == nil || !strings.Contains(err.Error(), "Input/output error") {
		t.Fatalf("install: %v", err)
	}
	if _, statErr := os.Stat(macPlistPath()); statErr == nil {
		t.Error("the job file was left behind")
	}
	if st, _ := Status(); st.Installed {
		t.Errorf("status after a refused install: %+v", st)
	}
	for _, c := range *ran {
		if strings.Contains(c, "launchctl load") {
			t.Errorf("launchctl load ends with status 0 on failure and must not be used: %s", c)
		}
	}
}

func TestInstallMac(t *testing.T) {
	loaded := false
	ran := fake(t, "darwin", func(command string) runner.Result {
		switch {
		case strings.HasPrefix(command, "launchctl bootstrap"):
			loaded = true
		case strings.HasPrefix(command, "launchctl bootout"):
			was := loaded
			loaded = false
			if !was {
				return runner.Result{ExitCode: 113}
			}
		case strings.HasPrefix(command, "launchctl print") && !loaded:
			return runner.Result{ExitCode: 113}
		}
		return runner.Result{}
	})
	if err := Install("", "", 3, 0); err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	data, err := os.ReadFile(macPlistPath())
	if err != nil {
		t.Fatal(err)
	}
	plist := string(data)
	logs := filepath.Join(home, "Library", "Logs", "omega")
	for _, want := range []string{"<string>quick</string>", "<key>" + EnvScheduled + "</key>", filepath.Join(logs, "schedule.log"), filepath.Join(logs, "schedule.err")} {
		if !strings.Contains(plist, want) {
			t.Errorf("the job lacks %q:\n%s", want, plist)
		}
	}
	if strings.Contains(plist, "/tmp/omega-schedule") {
		t.Errorf("the job logs to a shared folder:\n%s", plist)
	}
	if fi, err := os.Stat(logs); err != nil || (runtime.GOOS != "windows" && fi.Mode().Perm() != 0o700) {
		t.Errorf("log folder: %v, %v", fi, err)
	}
	if !strings.Contains(strings.Join(*ran, "\n"), "launchctl bootstrap 'gui/") {
		t.Errorf("commands: %q", *ran)
	}
	if st, _ := Status(); !st.Installed || st.Target != macPlistPath() {
		t.Errorf("status: %+v", st)
	}
	// Installing again replaces the job that is loaded.
	if err := Install("daily", "health", 4, 30); err != nil {
		t.Fatalf("second install: %v", err)
	}
	// A file launchd does not know is not an installed schedule.
	loaded = false
	if st, _ := Status(); st.Installed || st.Details == "" {
		t.Errorf("status with the job not loaded: %+v", st)
	}
	if err := Uninstall(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(macPlistPath()); err == nil {
		t.Error("uninstall left the job file")
	}
	if err := Uninstall(); err != nil {
		t.Errorf("uninstalling what is not installed: %v", err)
	}
}

func TestInstallLinux(t *testing.T) {
	fail := true
	fake(t, "linux", func(command string) runner.Result {
		if fail && strings.Contains(command, "enable --now") {
			return runner.Result{ExitCode: 1, Output: "Failed to connect to bus"}
		}
		return runner.Result{}
	})
	service, timer := systemdUnitPaths()
	if err := Install("weekly", "quick", 3, 0); err == nil || !strings.Contains(err.Error(), "Failed to connect to bus") {
		t.Fatalf("install: %v", err)
	}
	for _, f := range []string{service, timer} {
		if _, err := os.Stat(f); err == nil {
			t.Errorf("%s was left behind", f)
		}
	}
	if st, _ := Status(); st.Installed {
		t.Errorf("status after a refused install: %+v", st)
	}
	fail = false
	if err := Install("weekly", "quick", 3, 0); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(service)
	if !strings.Contains(string(data), "Environment="+EnvScheduled+"=1") || !strings.Contains(string(data), " quick --yes --no-tui --no-ai --scheduled") {
		t.Errorf("service:\n%s", data)
	}
	if st, _ := Status(); !st.Installed {
		t.Errorf("status: %+v", st)
	}
}

// Homebrew deletes the versioned folder on upgrade: the schedule calls the
// link that follows the upgrades.
func TestStablePath(t *testing.T) {
	prefix := t.TempDir()
	exe := filepath.Join(prefix, "Cellar", "omega", "1.0.0", "bin", "omega")
	os.MkdirAll(filepath.Dir(exe), 0o755)
	os.WriteFile(exe, []byte("x"), 0o755)
	if got := stablePath(exe); got != exe {
		t.Errorf("without the link: %s", got)
	}
	link := filepath.Join(prefix, "bin", "omega")
	os.MkdirAll(filepath.Dir(link), 0o755)
	os.WriteFile(link, []byte("another program"), 0o755)
	if got := stablePath(exe); got != exe {
		t.Errorf("a different program under the stable name: %s", got)
	}
	os.Remove(link)
	if err := os.Symlink(exe, link); err != nil {
		t.Skip("links are not available here: " + err.Error())
	}
	if got := stablePath(exe); got != link {
		t.Errorf("stablePath = %s, want %s", got, link)
	}
	if got := stablePath(link); got != link {
		t.Errorf("a path outside the Cellar changed: %s", got)
	}
}

// Every task says what it does, and that the Trash is left alone.
func TestDescribe(t *testing.T) {
	for _, task := range []string{"quick", "update", "clean", "full", "analyze", "health", "custom"} {
		if d := Describe(task); !strings.Contains(d, "never empties the Trash") {
			t.Errorf("%s: %q", task, d)
		}
	}
}

func TestValidate(t *testing.T) {
	if err := Validate("monthly", "quick", 3, 0); err != nil {
		t.Errorf("monthly: %v", err)
	}
	// custom runs the default routine, with the built-in steps only.
	if err := Validate("daily", "custom", 3, 0); err != nil {
		t.Errorf("custom: %v", err)
	}
	for _, bad := range []struct {
		interval, task string
		hour, minute   int
	}{
		{"hourly", "quick", 3, 0}, {"weekly", "bogus", 3, 0}, {"weekly", "quick", 99, 0},
		{"weekly", "quick", -1, 0}, {"weekly", "quick", 3, 60}, {"weekly", "quick --sudo", 3, 0},
	} {
		if err := Validate(bad.interval, bad.task, bad.hour, bad.minute); err == nil {
			t.Errorf("%+v was accepted", bad)
		}
	}
}

// monthly is monthly on the three systems, not weekly under another name.
func TestMonthlySchedule(t *testing.T) {
	if c := macCalendar("monthly", 4, 5); !strings.Contains(c, "<key>Day</key>") || strings.Contains(c, "Weekday") {
		t.Errorf("launchd: %s", c)
	}
	if c := macCalendar("daily", 4, 5); strings.Contains(c, "Day") {
		t.Errorf("launchd daily: %s", c)
	}
	if c := systemdCalendar("monthly", 4, 5); c != "*-*-01 04:05:00" {
		t.Errorf("systemd: %s", c)
	}
	args := strings.Join(windowsTaskArgs(`C:\Program Files\omega\omega.exe`, "monthly", "quick", 4, 5), "|")
	if !strings.Contains(args, "/SC|MONTHLY|/D|1") || !strings.Contains(args, `/TR|"C:\Program Files\omega\omega.exe" quick --yes --no-tui --no-ai --scheduled|`) {
		t.Errorf("schtasks: %s", args)
	}
}

// Third review, P1: % and $ in the path of omega are not expanded by systemd.
func TestSystemdQuote(t *testing.T) {
	if got := systemdQuote(`/home/a/50%/$HOME/"x"\omega`); got != `/home/a/50%%/$$HOME/\"x\"\\omega` {
		t.Fatalf("%s", got)
	}
}

// A6: the job says it is scheduled on all three systems, with the flag (the
// Task Scheduler passes no environment) and, where it can, the variable.
// M8: and it runs at low priority.
func TestScheduledFlagAndPriority(t *testing.T) {
	ran := fake(t, "darwin", func(command string) runner.Result { return runner.Result{} })
	_ = ran
	if err := Install("weekly", "clean", 3, 0); err != nil {
		t.Fatal(err)
	}
	plist, _ := os.ReadFile(macPlistPath())
	for _, want := range []string{"<string>--scheduled</string>", "<key>" + EnvScheduled + "</key>", "<key>ProcessType</key>\n    <string>Background</string>"} {
		if !strings.Contains(string(plist), want) {
			t.Errorf("launchd job lacks %q:\n%s", want, plist)
		}
	}

	fake(t, "linux", nil)
	if err := Install("weekly", "clean", 3, 0); err != nil {
		t.Fatal(err)
	}
	service, _ := systemdUnitPaths()
	unit, _ := os.ReadFile(service)
	for _, want := range []string{" clean --yes --no-tui --no-ai --scheduled\n", "Environment=" + EnvScheduled + "=1", "Nice=10", "IOSchedulingClass=idle"} {
		if !strings.Contains(string(unit), want) {
			t.Errorf("systemd unit lacks %q:\n%s", want, unit)
		}
	}

	ran = fake(t, "windows", nil)
	if err := Install("weekly", "clean", 3, 0); err != nil {
		t.Fatal(err)
	}
	if len(*ran) != 1 || !strings.Contains((*ran)[0], " clean --yes --no-tui --no-ai --scheduled ") {
		t.Errorf("schtasks: %q", *ran)
	}
}

// M8: a schedule log over the limit keeps its last lines only, in place.
func TestRotateLogs(t *testing.T) {
	fake(t, "linux", nil)
	oldAbove, oldKeep := rotateAbove, keepBytes
	rotateAbove, keepBytes = 100, 40
	defer func() { rotateAbove, keepBytes = oldAbove, oldKeep }()
	dir := LogDir()
	os.MkdirAll(dir, 0o700)
	big := filepath.Join(dir, "schedule.log")
	small := filepath.Join(dir, "schedule.err")
	var b strings.Builder
	for i := 0; i < 20; i++ {
		b.WriteString("line number " + string(rune('a'+i)) + "\n")
	}
	os.WriteFile(big, []byte(b.String()), 0o600)
	os.WriteFile(small, []byte("short\n"), 0o600)
	RotateLogs()
	got, _ := os.ReadFile(big)
	if len(got) >= b.Len() || !strings.HasSuffix(string(got), "line number t\n") || !strings.HasPrefix(string(got), "[omega: older lines removed") {
		t.Errorf("rotated log:\n%s", got)
	}
	for _, l := range strings.Split(strings.TrimSpace(string(got)), "\n")[1:] {
		if !strings.HasPrefix(l, "line number ") {
			t.Errorf("a line was cut: %q", l)
		}
	}
	if s, _ := os.ReadFile(small); string(s) != "short\n" {
		t.Errorf("a small log changed: %q", s)
	}
}

// A scheduled custom task runs `omega custom` with the flags every task gets.
func TestCustomTaskCommand(t *testing.T) {
	args := strings.Join(windowsTaskArgs(`C:\omega.exe`, "daily", "custom", 3, 0), " ")
	if !strings.Contains(args, `"C:\omega.exe" custom --yes --no-tui --no-ai --scheduled`) {
		t.Errorf("schtasks: %s", args)
	}
}
