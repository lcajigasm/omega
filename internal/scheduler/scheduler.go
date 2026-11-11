// Package scheduler manages automated maintenance scheduling using native OS daemons
// (launchd on macOS, systemd --user on Linux, and Task Scheduler on Windows) and provides
// native desktop notifications.
package scheduler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/lcajigasm/omega/internal/runner"
)

var goos = runtime.GOOS

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
		script := fmt.Sprintf(`osascript -e 'display notification "%s" with title "%s"'`,
			escapeAppleScript(message), escapeAppleScript(title))
		_ = runner.Run(ctx, script, 3*time.Second, nil)
		return nil
	case "windows":
		script := fmt.Sprintf(`[reflection.assembly]::loadwithpartialname('System.Windows.Forms'); `+
			`$notify = new-object system.windows.forms.notifyicon; `+
			`$notify.icon = [system.drawing.systemicons]::Information; `+
			`$notify.visible = $true; `+
			`$notify.showballoontip(10, '%s', '%s', [system.windows.forms.tooltipicon]::Info)`, title, message)
		_ = runner.Run(ctx, script, 5*time.Second, nil)
		return nil
	default: // Linux
		if runner.Has("notify-send") {
			cmd := fmt.Sprintf("notify-send '%s' '%s'", strings.ReplaceAll(title, "'", `'\''`), strings.ReplaceAll(message, "'", `'\''`))
			_ = runner.Run(ctx, cmd, 3*time.Second, nil)
		}
		return nil
	}
}

func escapeAppleScript(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `"`, `\"`)
}

// Install registers background maintenance with the operating system.
func Install(interval, task string, hour, minute int) error {
	if interval == "" {
		interval = "weekly"
	}
	if task == "" {
		task = "quick"
	}
	omegaPath, err := os.Executable()
	if err != nil {
		omegaPath = "omega"
	}

	switch goos {
	case "darwin":
		return installMacLaunchAgent(omegaPath, interval, task, hour, minute)
	case "windows":
		return installWindowsTask(omegaPath, interval, task, hour, minute)
	default:
		return installLinuxSystemd(omegaPath, interval, task, hour, minute)
	}
}

func installMacLaunchAgent(omegaPath, interval, task string, hour, minute int) error {
	home, _ := os.UserHomeDir()
	agentsDir := filepath.Join(home, "Library/LaunchAgents")
	_ = os.MkdirAll(agentsDir, 0755)
	plistPath := filepath.Join(agentsDir, macLaunchAgentLabel+".plist")

	var calendarInterval string
	if interval == "daily" {
		calendarInterval = fmt.Sprintf(`
        <dict>
            <key>Hour</key>
            <integer>%d</integer>
            <key>Minute</key>
            <integer>%d</integer>
        </dict>`, hour, minute)
	} else { // weekly on Saturday
		calendarInterval = fmt.Sprintf(`
        <dict>
            <key>Weekday</key>
            <integer>6</integer>
            <key>Hour</key>
            <integer>%d</integer>
            <key>Minute</key>
            <integer>%d</integer>
        </dict>`, hour, minute)
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
    <key>StartCalendarInterval</key>%s
    <key>StandardOutPath</key>
    <string>/tmp/omega-schedule.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/omega-schedule.err</string>
</dict>
</plist>`, macLaunchAgentLabel, omegaPath, task, calendarInterval)

	if err := os.WriteFile(plistPath, []byte(content), 0644); err != nil {
		return err
	}

	// Load with launchctl
	_ = runner.Run(context.Background(), fmt.Sprintf("launchctl unload %s 2>/dev/null", plistPath), 3*time.Second, nil)
	res := runner.Run(context.Background(), fmt.Sprintf("launchctl load %s", plistPath), 3*time.Second, nil)
	if res.ExitCode != 0 {
		return fmt.Errorf("launchctl load failed: %s", res.Output)
	}
	return nil
}

func installLinuxSystemd(omegaPath, interval, task string, hour, minute int) error {
	home, _ := os.UserHomeDir()
	unitDir := filepath.Join(home, ".config/systemd/user")
	_ = os.MkdirAll(unitDir, 0755)

	serviceFile := filepath.Join(unitDir, systemdTimerName+".service")
	timerFile := filepath.Join(unitDir, systemdTimerName+".timer")

	serviceContent := fmt.Sprintf(`[Unit]
Description=Omega Scheduled Maintenance

[Service]
Type=oneshot
ExecStart=%s %s --yes --no-tui
`, omegaPath, task)

	onCalendar := fmt.Sprintf("Sat *-*-* %02d:%02d:00", hour, minute)
	if interval == "daily" {
		onCalendar = fmt.Sprintf("*-*-* %02d:%02d:00", hour, minute)
	}

	timerContent := fmt.Sprintf(`[Unit]
Description=Omega Scheduled Maintenance Timer

[Timer]
OnCalendar=%s
Persistent=true

[Install]
WantedBy=timers.target
`, onCalendar)

	if err := os.WriteFile(serviceFile, []byte(serviceContent), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(timerFile, []byte(timerContent), 0644); err != nil {
		return err
	}

	res := runner.Run(context.Background(), "systemctl --user daemon-reload && systemctl --user enable --now "+systemdTimerName+".timer", 5*time.Second, nil)
	if res.ExitCode != 0 {
		return fmt.Errorf("systemctl enable failed: %s", res.Output)
	}
	return nil
}

func installWindowsTask(omegaPath, interval, task string, hour, minute int) error {
	scInterval := "WEEKLY /D SAT"
	if interval == "daily" {
		scInterval = "DAILY"
	}
	cmd := fmt.Sprintf(`schtasks /Create /TN "%s" /TR "\"%s\" %s --yes --no-tui" /SC %s /ST %02d:%02d /F`,
		windowsTaskName, omegaPath, task, scInterval, hour, minute)
	res := runner.Run(context.Background(), cmd, 5*time.Second, nil)
	if res.ExitCode != 0 {
		return fmt.Errorf("schtasks failed: %s", res.Output)
	}
	return nil
}

// Status checks whether the scheduled maintenance task is active.
func Status() (*ScheduleStatus, error) {
	st := &ScheduleStatus{Platform: goos}
	home, _ := os.UserHomeDir()

	switch goos {
	case "darwin":
		plistPath := filepath.Join(home, "Library/LaunchAgents", macLaunchAgentLabel+".plist")
		if _, err := os.Stat(plistPath); err == nil {
			st.Installed = true
			st.Target = plistPath
			res := runner.Run(context.Background(), fmt.Sprintf("launchctl list | grep %s", macLaunchAgentLabel), 2*time.Second, nil)
			st.Details = strings.TrimSpace(res.Output)
		}
	case "windows":
		res := runner.Run(context.Background(), fmt.Sprintf(`schtasks /Query /TN "%s" 2>&1`, windowsTaskName), 3*time.Second, nil)
		if res.ExitCode == 0 {
			st.Installed = true
			st.Target = windowsTaskName
			st.Details = strings.TrimSpace(res.Output)
		}
	default:
		timerFile := filepath.Join(home, ".config/systemd/user", systemdTimerName+".timer")
		if _, err := os.Stat(timerFile); err == nil {
			st.Installed = true
			st.Target = timerFile
			res := runner.Run(context.Background(), "systemctl --user is-active "+systemdTimerName+".timer", 2*time.Second, nil)
			st.Details = strings.TrimSpace(res.Output)
		}
	}
	return st, nil
}

// Uninstall removes the background daemon/timer.
func Uninstall() error {
	home, _ := os.UserHomeDir()
	switch goos {
	case "darwin":
		plistPath := filepath.Join(home, "Library/LaunchAgents", macLaunchAgentLabel+".plist")
		_ = runner.Run(context.Background(), fmt.Sprintf("launchctl unload %s 2>/dev/null", plistPath), 3*time.Second, nil)
		_ = os.Remove(plistPath)
		return nil
	case "windows":
		_ = runner.Run(context.Background(), fmt.Sprintf(`schtasks /Delete /TN "%s" /F`, windowsTaskName), 3*time.Second, nil)
		return nil
	default:
		_ = runner.Run(context.Background(), "systemctl --user stop "+systemdTimerName+".timer && systemctl --user disable "+systemdTimerName+".timer", 3*time.Second, nil)
		_ = os.Remove(filepath.Join(home, ".config/systemd/user", systemdTimerName+".timer"))
		_ = os.Remove(filepath.Join(home, ".config/systemd/user", systemdTimerName+".service"))
		return nil
	}
}
