package safety

import "testing"

func TestPersonalSetup(t *testing.T) {
	for cmd, want := range map[string]string{
		"defaults write com.apple.dock persistent-apps -array && killall Dock":                     "the Dock",
		"/opt/homebrew/bin/omega mac op dock-remove 'com.apple.Maps'":                              "the Dock",
		"dockutil --remove Maps --no-restart":                                                      "the Dock",
		"defaults delete com.apple.dock autohide; killall Dock":                                    "the Dock",
		"gsettings set org.gnome.shell favorite-apps \"['firefox.desktop']\"":                      "the Dock",
		"defaults write com.apple.symbolichotkeys AppleSymbolicHotKeys -dict-add 64 '{}'":          "keyboard shortcuts",
		"osascript -e 'tell application \"Finder\" to set desktop picture to POSIX file \"x\"'":    "the wallpaper",
		`Remove-Item 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Explorer\Taskband' -Recurse`: "the taskbar and the Start menu",
		// Reading is not rearranging, and upkeep is upkeep.
		"defaults read com.apple.dock persistent-apps": "",
		"gsettings get org.gnome.shell favorite-apps":  "",
		"brew cleanup":     "",
		"docker system df": "",
		"defaults write com.apple.finder AppleShowAllFiles -bool true": "",
	} {
		if got := PersonalSetup(cmd); got != want {
			t.Errorf("PersonalSetup(%q) = %q, want %q", cmd, got, want)
		}
	}
}
