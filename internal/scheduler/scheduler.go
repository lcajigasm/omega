// Package scheduler manages automated maintenance scheduling using native OS daemons
// (launchd on macOS, systemd --user on Linux, and Task Scheduler on Windows). Notify
// can post a desktop notification; nothing calls it yet.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/runner"
)

var goos = runtime.GOOS

// Seams the tests replace, so that no test reaches launchd, systemd, the
// Task Scheduler or the notification centre.
var (
	// run executes a shell command.
	run = func(ctx context.Context, command string, timeout time.Duration) runner.Result {
		return runner.Run(ctx, command, timeout, nil)
	}
	// runArgs executes a program with its arguments as they are.
	runArgs = func(name string, args ...string) ([]byte, error) {
		return exec.Command(name, args...).CombinedOutput()
	}
	executable = os.Executable
	// settle is how long launchd is given to let go of a job it was told
	// to remove before the new one is loaded.
	settle = time.Second
)

// EnvScheduled is set to "1" in the environment of the job the schedule
// runs. A run that sees it is unattended and repeats by itself: it must not
// empty the Trash, which is where omega puts what it removes.
const EnvScheduled = "OMEGA_SCHEDULED"

// taskFlags are the flags of the scheduled command.
const taskFlags = "--yes --no-tui"

// Describe says what the scheduled task does, for the message shown when
// it is installed.
func Describe(task string) string {
	what := map[string]string{
		"quick":   "cleans the thumbnail and temporary-file caches, counts pending software updates and checks the battery",
		"update":  "updates the installed software",
		"clean":   "runs the cleanup steps",
		"full":    "updates the software, runs the cleanup steps and analyzes the system",
		"analyze": "reports on the state of the system and changes nothing",
		"health":  "runs the health checks and changes nothing",
	}[task]
	if what == "" {
		return ""
	}
	return "Each run " + what + ". A scheduled run never empties the Trash."
}

// stablePath is the path the schedule should call omega by. Homebrew
// installs every version in its own folder (…/Cellar/omega/1.2.3/bin/omega)
// and deletes it on upgrade; <prefix>/bin/omega is a link that follows the
// upgrades, so that is used when it is the same program.
func stablePath(exe string) string {
	slashed := filepath.ToSlash(exe)
	i := strings.Index(slashed, "/Cellar/")
	if i < 0 {
		return exe
	}
	link := filepath.Join(filepath.FromSlash(slashed[:i]), "bin", filepath.Base(exe))
	a, errA := os.Stat(link)
	b, errB := os.Stat(exe)
	if errA == nil && errB == nil && os.SameFile(a, b) {
		return link
	}
	return exe
}

// ScheduleStatus reports the current background maintenance automation state.
type ScheduleStatus struct {
	Installed bool   `json:"installed"`
	Platform  string `json:"platform"`
	Interval  string `json:"interval"` // "daily", "weekly", etc.
	Task      string `json:"task"`
	Target    string `json:"target"`
	Details   string `json:"details"`
}

const (
	macLaunchAgentLabel = "com.omega.maintenance"
	systemdTimerName    = "omega-maintenance"
	windowsTaskName     = "OmegaMaintenance"
)

// Notify sends a native desktop notification to the user.
func Notify(ctx context.Context, title, message string) error {
	switch goos {
	case "darwin":
		script := "osascript -e " + sq(fmt.Sprintf(`display notification "%s" with title "%s"`,
			escapeAppleScript(message), escapeAppleScript(title)))
		_ = run(ctx, script, 3*time.Second)
		return nil
	case "windows":
		script := fmt.Sprintf(`[reflection.assembly]::loadwithpartialname('System.Windows.Forms'); `+
			`$notify = new-object system.windows.forms.notifyicon; `+
			`$notify.icon = [system.drawing.systemicons]::Information; `+
			`$notify.visible = $true; `+
			`$notify.showballoontip(10, '%s', '%s', [system.windows.forms.tooltipicon]::Info)`,
			strings.ReplaceAll(title, "'", "''"), strings.ReplaceAll(message, "'", "''"))
		_ = run(ctx, script, 5*time.Second)
		return nil
	default: // Linux
		if runner.Has("notify-send") {
			_ = run(ctx, "notify-send -- "+sq(title)+" "+sq(message), 3*time.Second)
		}
		return nil
	}
}

func escapeAppleScript(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `"`, `\"`)
}

// Validate checks a schedule before anything is written: a task omega does
// not have, or 25:00, would install a job that fails every time it runs.
func Validate(interval, task string, hour, minute int) error {
	switch interval {
	case "daily", "weekly", "monthly":
	default:
		return fmt.Errorf("interval %q: use daily, weekly or monthly", interval)
	}
	switch task {
	case "quick", "update", "clean", "full", "analyze", "health":
	default:
		return fmt.Errorf("task %q: use quick, update, clean, full, analyze or health", task)
	}
	if hour < 0 || hour > 23 {
		return fmt.Errorf("hour %d: use 0 to 23", hour)
	}
	if minute < 0 || minute > 59 {
		return fmt.Errorf("minute %d: use 0 to 59", minute)
	}
	return nil
}

// xmlEscape makes a path safe inside a property list.
func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// sq quotes a path for sh.
func sq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// Install registers background maintenance with the operating system.
func Install(interval, task string, hour, minute int) error {
	if interval == "" {
		interval = "weekly"
	}
	if task == "" {
		task = "quick"
	}
	if err := Validate(interval, task, hour, minute); err != nil {
		return err
	}
	omegaPath, err := executable()
	if err != nil {
		omegaPath = "omega"
	}
	omegaPath = stablePath(omegaPath)

	switch goos {
	case "darwin":
		return installMacLaunchAgent(omegaPath, interval, task, hour, minute)
	case "windows":
		return installWindowsTask(omegaPath, interval, task, hour, minute)
	default:
		return installLinuxSystemd(omegaPath, interval, task, hour, minute)
	}
}

// macCalendar is the StartCalendarInterval of the schedule: every day,
// Saturdays, or the first of the month.
func macCalendar(interval string, hour, minute int) string {
	day := ""
	switch interval {
	case "weekly":
		day = "\n            <key>Weekday</key>\n            <integer>6</integer>"
	case "monthly":
		day = "\n            <key>Day</key>\n            <integer>1</integer>"
	}
	return fmt.Sprintf(`
        <dict>%s
            <key>Hour</key>
            <integer>%d</integer>
            <key>Minute</key>
            <integer>%d</integer>
        </dict>`, day, hour, minute)
}

// systemdCalendar is the same schedule as an OnCalendar expression.
func systemdCalendar(interval string, hour, minute int) string {
	switch interval {
	case "daily":
		return fmt.Sprintf("*-*-* %02d:%02d:00", hour, minute)
	case "monthly":
		return fmt.Sprintf("*-*-01 %02d:%02d:00", hour, minute)
	}
	return fmt.Sprintf("Sat *-*-* %02d:%02d:00", hour, minute)
}

// windowsTaskArgs are the arguments of schtasks. They are passed as they
// are, not through PowerShell: its quoting turned the quoted path of /TR
// into a program name that does not exist, and the task never started.
func windowsTaskArgs(omegaPath, interval, task string, hour, minute int) []string {
	sc := []string{"/SC", "WEEKLY", "/D", "SAT"}
	switch interval {
	case "daily":
		sc = []string{"/SC", "DAILY"}
	case "monthly":
		sc = []string{"/SC", "MONTHLY", "/D", "1"}
	}
	args := []string{"/Create", "/TN", windowsTaskName, "/TR", fmt.Sprintf(`"%s" %s %s`, omegaPath, task, taskFlags)}
	args = append(args, sc...)
	return append(args, "/ST", fmt.Sprintf("%02d:%02d", hour, minute), "/F")
}

// launchdDomain is where a user's agents live, and launchdJob the job in it.
func launchdDomain() string { return "gui/" + strconv.Itoa(os.Getuid()) }
func launchdJob() string    { return launchdDomain() + "/" + macLaunchAgentLabel }

func macPlistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", macLaunchAgentLabel+".plist")
}

// LogDir is where the scheduled job writes what it prints: the user's log
// folder on a Mac, omega's state folder elsewhere. Not /tmp: a fixed name
// there can be read, or put there first, by anyone on the machine.
func LogDir() string {
	if goos == "darwin" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Logs", "omega")
	}
	return filepath.Join(config.StateDir(), "logs")
}

// launchdLoaded asks launchd whether it has the job.
func launchdLoaded() bool {
	return run(context.Background(), "launchctl print "+sq(launchdJob())+" >/dev/null 2>&1", 5*time.Second).ExitCode == 0
}

func installMacLaunchAgent(omegaPath, interval, task string, hour, minute int) error {
	plistPath := macPlistPath()
	if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
		return err
	}
	logs := LogDir()
	if err := os.MkdirAll(logs, 0o700); err != nil {
		return err
	}
	// MkdirAll leaves a folder that was there as it was.
	if err := os.Chmod(logs, 0o700); err != nil {
		return err
	}

	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>%s</string>
        <string>--yes</string>
        <string>--no-tui</string>
    </array>
    <key>EnvironmentVariables</key>
    <dict>
        <key>%s</key>
        <string>1</string>
    </dict>
    <key>StartCalendarInterval</key>%s
    <key>StandardOutPath</key>
    <string>%s</string>
    <key>StandardErrorPath</key>
    <string>%s</string>
</dict>
</plist>`, macLaunchAgentLabel, xmlEscape(omegaPath), task, EnvScheduled, macCalendar(interval, hour, minute),
		xmlEscape(filepath.Join(logs, "schedule.log")), xmlEscape(filepath.Join(logs, "schedule.err")))

	if err := os.WriteFile(plistPath, []byte(content), 0o644); err != nil {
		return err
	}

	// A job of the same name has to go before the new one is loaded.
	// `launchctl load` is not used: it prints "Load failed" and still ends
	// with status 0. bootstrap says when launchd refuses the job, and
	// print confirms it is there.
	ctx := context.Background()
	had := run(ctx, "launchctl bootout "+sq(launchdJob())+" 2>&1", 10*time.Second).ExitCode == 0
	load := "launchctl bootstrap " + sq(launchdDomain()) + " " + sq(plistPath) + " 2>&1"
	res := run(ctx, load, 10*time.Second)
	if res.ExitCode != 0 && had {
		// launchd may still be letting go of the old job.
		time.Sleep(settle)
		res = run(ctx, load, 10*time.Second)
	}
	if res.ExitCode != 0 || !launchdLoaded() {
		// Without this the file would stay and Status would say "installed".
		_ = os.Remove(plistPath)
		detail := strings.TrimSpace(res.Output)
		if detail == "" {
			detail = "launchd does not list the job after loading it"
		}
		return fmt.Errorf("launchd did not accept the job: %s", detail)
	}
	return nil
}

func systemdUnitPaths() (service, timer string) {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".config", "systemd", "user")
	return filepath.Join(dir, systemdTimerName+".service"), filepath.Join(dir, systemdTimerName+".timer")
}

func installLinuxSystemd(omegaPath, interval, task string, hour, minute int) error {
	serviceFile, timerFile := systemdUnitPaths()
	if err := os.MkdirAll(filepath.Dir(serviceFile), 0o755); err != nil {
		return err
	}

	serviceContent := fmt.Sprintf(`[Unit]
Description=Omega Scheduled Maintenance

[Service]
Type=oneshot
Environment=%s=1
ExecStart="%s" %s %s
`, EnvScheduled, strings.ReplaceAll(omegaPath, `"`, `\"`), task, taskFlags)

	timerContent := fmt.Sprintf(`[Unit]
Description=Omega Scheduled Maintenance Timer

[Timer]
OnCalendar=%s
Persistent=true

[Install]
WantedBy=timers.target
`, systemdCalendar(interval, hour, minute))

	if err := os.WriteFile(serviceFile, []byte(serviceContent), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(timerFile, []byte(timerContent), 0o644); err != nil {
		return err
	}

	ctx := context.Background()
	res := run(ctx, "systemctl --user daemon-reload && systemctl --user enable --now "+systemdTimerName+".timer 2>&1", 15*time.Second)
	if res.ExitCode == 0 {
		res = run(ctx, "systemctl --user is-enabled "+systemdTimerName+".timer 2>&1", 5*time.Second)
	}
	if res.ExitCode != 0 {
		// Without this the files would stay and Status would say "installed".
		_ = os.Remove(timerFile)
		_ = os.Remove(serviceFile)
		_ = run(ctx, "systemctl --user daemon-reload", 5*time.Second)
		return fmt.Errorf("systemd did not accept the timer: %s", strings.TrimSpace(res.Output))
	}
	return nil
}

func installWindowsTask(omegaPath, interval, task string, hour, minute int) error {
	out, err := runArgs("schtasks", windowsTaskArgs(omegaPath, interval, task, hour, minute)...)
	if err != nil {
		return fmt.Errorf("schtasks failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// Status checks whether the scheduled maintenance task is active. A job
// file that the system has not loaded does not count as installed: it will
// never run.
func Status() (*ScheduleStatus, error) {
	st := &ScheduleStatus{Platform: goos}
	ctx := context.Background()

	switch goos {
	case "darwin":
		plistPath := macPlistPath()
		if _, err := os.Stat(plistPath); err == nil {
			st.Target = plistPath
			if st.Installed = launchdLoaded(); st.Installed {
				st.Details = "loaded in launchd as " + launchdJob()
			} else {
				st.Details = "the job file exists but launchd has not loaded it; run 'omega schedule install' again"
			}
		}
	case "windows":
		res := run(ctx, fmt.Sprintf(`schtasks /Query /TN "%s" 2>&1`, windowsTaskName), 3*time.Second)
		if res.ExitCode == 0 {
			st.Installed = true
			st.Target = windowsTaskName
			st.Details = strings.TrimSpace(res.Output)
		}
	default:
		_, timerFile := systemdUnitPaths()
		if _, err := os.Stat(timerFile); err == nil {
			st.Target = timerFile
			enabled := run(ctx, "systemctl --user is-enabled "+systemdTimerName+".timer 2>&1", 3*time.Second)
			if st.Installed = enabled.ExitCode == 0; st.Installed {
				active := run(ctx, "systemctl --user is-active "+systemdTimerName+".timer 2>&1", 3*time.Second)
				st.Details = strings.TrimSpace(active.Output)
			} else {
				st.Details = "the timer file exists but systemd has not enabled it; run 'omega schedule install' again"
			}
		}
	}
	return st, nil
}

// Uninstall removes the background daemon/timer.
func Uninstall() error {
	ctx := context.Background()
	remove := func(path string) error {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	switch goos {
	case "darwin":
		_ = run(ctx, "launchctl bootout "+sq(launchdJob())+" 2>&1", 10*time.Second)
		return remove(macPlistPath())
	case "windows":
		_ = run(ctx, fmt.Sprintf(`schtasks /Delete /TN "%s" /F`, windowsTaskName), 3*time.Second)
		return nil
	default:
		serviceFile, timerFile := systemdUnitPaths()
		_ = run(ctx, "systemctl --user disable --now "+systemdTimerName+".timer 2>&1", 5*time.Second)
		err := errors.Join(remove(timerFile), remove(serviceFile))
		_ = run(ctx, "systemctl --user daemon-reload", 5*time.Second)
		return err
	}
}
