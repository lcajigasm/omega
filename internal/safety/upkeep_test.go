package safety

import "testing"

// What an unattended run must never do on its own, and the upkeep it must
// still do.
func TestNotUpkeep(t *testing.T) {
	refused := []string{
		// What starts at login, and the background.
		`osascript -e 'tell application "System Events" to delete login item "Alfred 5"'`,
		"launchctl bootout gui/501/com.dropbox.DropboxUpdater.wake",
		"launchctl unload ~/Library/LaunchAgents/com.google.keystone.agent.plist",
		"sudo launchctl disable system/com.openvpn.client",
		"rm ~/Library/LaunchAgents/com.spotify.webhelper.plist",
		"sfltool resetbtm",
		"crontab -r",
		// The menu bar and open apps.
		"defaults write com.apple.controlcenter 'NSStatusItem Visible Bluetooth' -bool false",
		"killall SystemUIServer",
		"killall Bartender\\ 5",
		"pkill -f Rectangle",
		"kill -9 4242",
		`osascript -e 'quit app "Slack"'`,
		// Apps.
		"brew uninstall --cask android-studio",
		"brew upgrade --cask alfred",
		"brew upgrade",
		"brew reinstall --cask tailscale-app",
		"brew install --cask rectangle",
		"mas upgrade",
		"mo uninstall",
		"mo optimize",
		"topgrade --yes",
		"rm -rf /Applications/Cursor.app",
		"mv ~/Applications/Foo.app ~/.Trash/",
		"npm uninstall -g typescript",
		"pipx uninstall black",
		// Settings and configuration.
		"sudo pmset -a sleep 0",
		"sudo /usr/libexec/ApplicationFirewall/socketfilterfw --setglobalstate on",
		"sudo softwareupdate --schedule on",
		"defaults write -g AppleShowAllExtensions -bool true",
		"git config --global pull.rebase true",
		"echo 'export PATH=$HOME/bin:$PATH' >> ~/.zshrc",
		"sed -i '' 's/foo/bar/' ~/.gitconfig",
		"npm config set fund false",
		"mise use -g node@22",
		"chsh -s /bin/zsh",
		// The Trash.
		"rm -rf ~/.Trash/*",
	}
	for _, c := range refused {
		if NotUpkeep(c) == "" {
			t.Errorf("an unattended run would run %q", c)
		}
	}
	allowed := []string{
		"brew upgrade --formula",
		"brew cleanup --prune=all",
		"brew autoremove",
		"brew outdated --cask --quiet",
		"npm update -g",
		"pipx upgrade-all",
		"mo clean",
		"topgrade --dry-run --no-retry",
		"rm -rf ~/Library/Caches/ms-playwright/*",
		"rm -rf ~/Library/Developer/Xcode/DerivedData/*",
		"go clean -cache",
		"docker builder prune -f",
		"launchctl list | grep -i git",
		"defaults read com.apple.dock",
		"git config --global --get maintenance.strategy",
		"git -C ~/Dev/omega gc",
		"echo hi > /tmp/omega.txt",
		"softwareupdate --list",
		"tmutil listlocalsnapshots /",
		"killing_floor --help",
	}
	for _, c := range allowed {
		if what := NotUpkeep(c); what != "" {
			t.Errorf("%q is upkeep, refused as %q", c, what)
		}
	}
}
