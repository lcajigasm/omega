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

func TestEscapeAppleScript(t *testing.T) {
	in := `Hello "World" \ test`
	got := escapeAppleScript(in)
	want := `Hello \"World\" \\ test`
	if got != want {
		t.Errorf("escapeAppleScript(%q) = %q; want %q", in, got, want)
	}
}

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

func TestNotifyDoesNotPostFromTests(t *testing.T) {
	for _, system := range []string{"darwin", "windows"} {
		ran := fake(t, system, nil)
		if err := Notify(context.Background(), `It's "done"`, "Unit test"); err != nil {
			t.Errorf("%s: %v", system, err)
		}
		if len(*ran) != 1 || !strings.Contains((*ran)[0], "Unit test") {
			t.Errorf("%s: commands %q", system, *ran)
		}
	}
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
	if !strings.Contains(string(data), "Environment="+EnvScheduled+"=1") || !strings.Contains(string(data), " quick --yes --no-tui") {
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
	for _, task := range []string{"quick", "update", "clean", "full", "analyze", "health"} {
		if d := Describe(task); !strings.Contains(d, "never empties the Trash") {
			t.Errorf("%s: %q", task, d)
		}
	}
}

func TestValidate(t *testing.T) {
	if err := Validate("monthly", "quick", 3, 0); err != nil {
		t.Errorf("monthly: %v", err)
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
	if !strings.Contains(args, "/SC|MONTHLY|/D|1") || !strings.Contains(args, `/TR|"C:\Program Files\omega\omega.exe" quick --yes --no-tui|`) {
		t.Errorf("schtasks: %s", args)
	}
}
