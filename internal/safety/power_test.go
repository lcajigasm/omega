package safety

import "testing"

// A3 of the fifth review: restarting, shutting down or logging out is
// never run, whatever the approval mode. Looking at them is still fine.
func TestRestartShutdownAndLogoutAreBlocked(t *testing.T) {
	for _, cmd := range []string{
		`shutdown -r now`, `sudo shutdown -h +5`, `reboot`, `sudo reboot`, `halt`, `poweroff`,
		`systemctl poweroff`, `systemctl reboot`, `sudo systemctl isolate reboot.target`, `loginctl terminate-user luis`,
		`launchctl reboot`, `launchctl reboot userspace`, `launchctl bootout gui/501`,
		`osascript -e 'tell app "System Events" to shut down'`, `osascript -e 'tell application "Finder" to restart'`,
		`osascript -e 'tell app "loginwindow" to «event aevtrlgo»'`,
		`softwareupdate --install --all --restart`, `sudo softwareupdate -i -a -R`, `softwareupdate -iaR`,
		`kill -9 -1`, `kill -KILL -1`, `kill -- -1`, `killall -9 loginwindow`, `sudo pkill WindowServer`,
		`Restart-Computer -Force`, `Stop-Computer`, `shutdown /r /t 0`, `shutdown.exe /s /t 0`,
		`init 6`, `sudo telinit 0`,
	} {
		if got := Classify(cmd); got != Blocked {
			t.Errorf("Classify(%q) = %v, want blocked", cmd, got)
		}
	}
	for cmd, want := range map[string]Risk{
		`softwareupdate --list`:                 Safe,
		`softwareupdate -l`:                     Safe,
		`cat /var/run/reboot-required`:          Safe,
		`man shutdown`:                          Safe,
		`shutdown --help`:                       Dangerous, // help only, so not blocked; noHelp still asks
		`systemctl status`:                      Safe,
		`launchctl bootout gui/501/com.x.agent`: Dangerous,
		`kill 12345`:                            Dangerous,
		`killall Dock`:                          Dangerous,
		`osascript -e 'display notification "restart needed"'`: Dangerous,
	} {
		if got := Classify(cmd); got != want {
			t.Errorf("Classify(%q) = %v, want %v", cmd, got, want)
		}
	}
}
