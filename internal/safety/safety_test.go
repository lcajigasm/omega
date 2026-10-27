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

func TestAlwaysConfirm(t *testing.T) {
	for cmd, want := range map[string]bool{
		"docker system prune -a -f": true, "docker volume prune -f": true, "docker rmi abc": true,
		"docker builder prune -a -f": true, "docker system df": false, "docker ps -a": false, "brew cleanup": false,
	} {
		if got := AlwaysConfirm(cmd); got != want {
			t.Errorf("AlwaysConfirm(%q) = %v, want %v", cmd, got, want)
		}
	}
	if Classify("git worktree list") != Safe || Classify("docker system df") != Safe {
		t.Error("read-only git/docker commands should be safe")
	}
}

func TestReadOnlyInspection(t *testing.T) {
	for _, cmd := range []string{
		"env | grep ^HOMEBREW_", "git config --global --list", "pnpm store path", "topgrade --help",
		"brew cleanup --prune=all --dry-run", "brew autoremove -n", "mo clean --dry-run", "time zsh -i -c exit",
	} {
		if got := Classify(cmd); got != Safe {
			t.Errorf("Classify(%q) = %s, want read-only", cmd, got)
		}
	}
	for _, cmd := range []string{"git config --global user.name x", "brew cleanup --prune=all", "zsh -c 'rm -rf x'"} {
		if got := Classify(cmd); got == Safe {
			t.Errorf("Classify(%q) should not be read-only", cmd)
		}
	}
}
