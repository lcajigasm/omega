package safety

import (
	"path"
	"regexp"
	"strings"
)

// Restarting, shutting down or logging out ends the session omega runs in,
// and every other program of the user with it, whatever they had open.
// omega never does it; it tells the user when a restart is pending.

// sessionKillers are processes whose end logs the user out or takes the
// machine down.
const sessionKillers = "loginwindow windowserver launchd systemd init gnome-session plasmashell kwin_x11 kwin_wayland xorg explorer.exe winlogon csrss"

// osascriptStops is AppleScript that restarts, shuts down or logs out:
// the System Events and Finder commands, and the raw Apple events
// (aevtrrst, aevtrsdn, aevtrlgo, aevtlogo).
var osascriptStops = regexp.MustCompile(`(?i)aevt(rrst|rsdn|rlgo|logo)|\b(system events|finder|loginwindow)\b.*\b(shut\s*down|restart|log\s*out)\b`)

// stopsMachine reports a command that restarts, shuts down or logs out.
func stopsMachine(cmd string) bool {
	for _, seg := range shellSegments(cmd) {
		run := afterWrappers(seg)
		if len(run) == 0 {
			continue
		}
		verb := strings.TrimSuffix(strings.ToLower(path.Base(run[0])), ".exe")
		args := run[1:]
		lower := make([]string, len(args))
		for i, a := range args {
			lower[i] = strings.ToLower(a)
		}
		if helpArgs(lower) {
			continue
		}
		switch verb {
		case "shutdown", "reboot", "halt", "poweroff", "logoff", "logout", "restart-computer", "stop-computer", "tsdiscon":
			return true
		case "init", "telinit":
			return len(lower) > 0 && has("0 6 s 1", lower[0])
		case "systemctl":
			if anyArg(lower, func(a string) bool {
				return has("poweroff reboot halt kexec soft-reboot emergency rescue", a) || strings.HasSuffix(a, ".target") &&
					has("poweroff.target reboot.target halt.target kexec.target soft-reboot.target rescue.target emergency.target", a)
			}) {
				return true
			}
		case "loginctl":
			if anyArg(lower, func(a string) bool {
				return has("poweroff reboot halt kexec terminate-session terminate-user kill-session kill-user", a)
			}) {
				return true
			}
		case "launchctl":
			sub := subcommand(lower)
			if sub == "reboot" {
				return true
			}
			// bootout of a whole gui or user domain logs the user out; of
			// one service in it (gui/501/com.x) it only stops that service.
			if sub == "bootout" && anyArg(lower, func(a string) bool {
				d := strings.Split(strings.Trim(a, "/"), "/")
				return len(d) == 2 && (d[0] == "gui" || d[0] == "user" || d[0] == "login")
			}) {
				return true
			}
		case "osascript":
			if osascriptStops.MatchString(strings.Join(args, " ")) {
				return true
			}
		case "softwareupdate":
			// --restart and -R restart the Mac once the updates are in.
			if anyArg(args, func(a string) bool {
				return a == "--restart" || (shortFlags.MatchString(a) && strings.Contains(a, "R"))
			}) {
				return true
			}
		case "kill":
			if killsEverything(lower) {
				return true
			}
		case "killall", "pkill", "taskkill", "stop-process":
			if anyArg(lower, func(a string) bool {
				return has(sessionKillers, strings.TrimSuffix(path.Base(a), ".exe")) || has(sessionKillers, path.Base(a))
			}) {
				return true
			}
		case "fdesetup":
			if anyArg(lower, func(a string) bool { return a == "authrestart" }) {
				return true
			}
		}
	}
	return false
}

// helpArgs reports arguments that only ask for help or a version.
func helpArgs(args []string) bool {
	return len(args) == 1 && has("--help -h /? -? --version -v", args[0])
}

// killsEverything reports kill aimed at every process the user may signal
// (-1), at a process group of them all, or at the first process (1): each
// ends the session.
func killsEverything(args []string) bool {
	targets := false // past the signal: the rest are processes
	for i, a := range args {
		switch {
		case a == "--":
			targets = true
			continue
		case a == "-s" || a == "-n" || a == "--signal":
			continue
		case i > 0 && (args[i-1] == "-s" || args[i-1] == "-n" || args[i-1] == "--signal"):
			continue
		case !targets && i == 0 && strings.HasPrefix(a, "-") && a != "-1":
			continue // the signal: -9, -KILL
		case !targets && i == 0 && a == "-1" && len(args) > 1:
			continue // SIGHUP, with targets after it
		}
		if a == "-1" || a == "1" || a == "0" {
			return true
		}
	}
	return false
}
