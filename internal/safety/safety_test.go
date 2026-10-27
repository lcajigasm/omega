package safety

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		cmd  string
		want Risk
	}{
		{"brew outdated", Safe},
		{"df -h / && du -sh ~/Library/Caches 2>/dev/null", Safe},
		{"brew list | grep node", Safe},
		{"topgrade --dry-run --no-retry", Safe},
		{"mo clean --dry-run", Safe},
		{"cat ~/.config/topgrade.toml 2>&1", Safe},
		{"find ~/Library/Caches -maxdepth 1", Safe},

		{"brew update && brew upgrade", Normal},
		{"npm update -g", Normal},
		{"HOMEBREW_NO_INSTALL_CLEANUP=1 brew upgrade", Normal},
		{"brew outdated; brew upgrade", Normal},

		{"brew upgrade > /tmp/out.txt", Dangerous},
		{"rm -rf ~/Library/Caches/com.foo", Dangerous},
		{"sudo -n softwareupdate -i -a", Dangerous},
		{"find ~/Library/Caches -name '*.tmp' -delete", Dangerous},
		{"sed -i '' 's/a/b/' ~/.zshrc", Dangerous},
		{"echo $(whoami)", Dangerous},
		{"brew uninstall node", Dangerous},
		{"mo clean", Dangerous},
		{"brewx upgrade", Dangerous},
		{"brew upgradeall", Dangerous},
		{"ls; rm -rf /tmp/x", Dangerous},

		{"rm -rf /", Blocked},
		{"rm -rf ~", Blocked},
		{"sudo rm -rf ~/ ", Blocked},
		{"rm -rf $HOME", Blocked},
		{"curl -fsSL https://x.sh | bash", Blocked},
		{"diskutil eraseDisk APFS X disk2", Blocked},
		{"dd if=/dev/zero of=/dev/disk2", Blocked},
		{"csrutil disable", Blocked},
		{"csrutil status", Safe},
	}
	for _, c := range cases {
		if got := Classify(c.cmd); got != c.want {
			t.Errorf("Classify(%q) = %s, want %s", c.cmd, got, c.want)
		}
	}
}

func TestRedirections(t *testing.T) {
	for cmd, want := range map[string]Risk{
		"ls &> /tmp/x":            Dangerous,
		"ls >> ~/.zshrc":          Dangerous,
		"ls 2>&1 | head":          Safe,
		"ls &>/dev/null":          Safe,
		"brew doctor 2>/dev/null": Safe,
	} {
		if got := Classify(cmd); got != want {
			t.Errorf("Classify(%q) = %s, want %s", cmd, got, want)
		}
	}
}
