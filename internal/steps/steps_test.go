package steps

import (
	"strings"
	"testing"

	"github.com/lcajigasm/omega/internal/safety"
)

func ids(list []Step) map[string]Step {
	out := map[string]Step{}
	for _, s := range list {
		out[s.ID] = s
	}
	return out
}

// withBins makes exactly these binaries "installed".
func withBins(t *testing.T, bins ...string) {
	t.Helper()
	old := hasBin
	hasBin = func(bin string) bool {
		for _, b := range bins {
			if b == bin {
				return true
			}
		}
		return false
	}
	t.Cleanup(func() { hasBin = old })
}

func TestEachSystemGetsItsOwnSteps(t *testing.T) {
	macSteps, linuxSteps := ids(For("darwin")), ids(For("linux"))
	for _, id := range []string{"macos", "macos-check", "mas", "brew-greedy", "xcode-derived", "simulators", "ios-device-support", "mole-clean", "mole-update"} {
		if _, ok := macSteps[id]; !ok {
			t.Errorf("%s must be in the macOS catalogue", id)
		}
		if _, ok := linuxSteps[id]; ok {
			t.Errorf("%s is Mac-only and must not show on Linux", id)
		}
	}
	for _, id := range []string{"apt", "apt-outdated", "apt-clean", "apt-autoremove", "dnf", "dnf-outdated", "dnf-clean", "dnf-autoremove",
		"pacman", "pacman-outdated", "pacman-cache", "pacman-orphans", "zypper", "zypper-dup", "zypper-outdated", "zypper-clean",
		"flatpak", "flatpak-outdated", "flatpak-unused", "snap", "snap-outdated", "snap-old", "journal-size", "journal-vacuum", "thumbnail-cache"} {
		if _, ok := linuxSteps[id]; !ok {
			t.Errorf("%s must be in the Linux catalogue", id)
		}
		if _, ok := macSteps[id]; ok {
			t.Errorf("%s is Linux-only and must not show on macOS", id)
		}
		if !Known(id) {
			t.Errorf("%s must be a known id on every system, so a shared config can skip it", id)
		}
	}
	// Steps both systems have under one id, each with its own paths.
	for id, want := range map[string][2]string{
		"disk":             {"~/Library/Caches", "~/.cache"},
		"trash":            {"~/.Trash", "~/.local/share/Trash"},
		"system-logs":      {"launchctl", "journalctl"},
		"docker-df":        {"Docker.raw", "docker system df"},
		"docker-start":     {"open -ga Docker", "systemctl start docker"},
		"playwright-cache": {"~/Library/Caches/ms-playwright", "~/.cache/ms-playwright"},
	} {
		if !strings.Contains(macSteps[id].Command, want[0]) || strings.Contains(macSteps[id].Command, want[1]) && want[1] != "docker system df" {
			t.Errorf("%s on macOS: %q", id, macSteps[id].Command)
		}
		if !strings.Contains(linuxSteps[id].Command, want[1]) || strings.Contains(linuxSteps[id].Command, "Library") {
			t.Errorf("%s on Linux: %q", id, linuxSteps[id].Command)
		}
	}
	// Homebrew and the developer tools work on both.
	for _, id := range []string{"brew", "brew-outdated", "brew-cleanup", "npm", "rustup", "go-cache", "docker", "topgrade", "dev-artifacts"} {
		if _, ok := macSteps[id]; !ok {
			t.Errorf("%s missing on macOS", id)
		}
		if _, ok := linuxSteps[id]; !ok {
			t.Errorf("%s missing on Linux", id)
		}
	}
	// A system omega has no steps for gets the neutral ones only.
	for _, s := range For("freebsd") {
		if s.OS != "" {
			t.Errorf("%s belongs to %s", s.ID, s.OS)
		}
	}
}

func TestWindowsCatalogue(t *testing.T) {
	win := ids(For("windows"))
	// Its own package managers and cleanups, each with the risk it deserves.
	for id, risk := range map[string]safety.Risk{
		"disk": safety.Safe, "winget-outdated": safety.Safe, "scoop-outdated": safety.Safe, "choco-outdated": safety.Safe,
		"windows-update-check": safety.Safe, "system-logs": safety.Safe,
		"winget": safety.Dangerous, "scoop": safety.Normal, "choco": safety.Dangerous,
		"temp-files": safety.Dangerous, "trash": safety.Dangerous, "scoop-cleanup": safety.Dangerous, "component-store": safety.Dangerous,
	} {
		s, ok := win[id]
		if !ok {
			t.Errorf("%s missing on Windows", id)
			continue
		}
		if s.Risk != risk {
			t.Errorf("%s: risk %v, want %v", id, s.Risk, risk)
		}
	}
	// The developer tools that are single commands work there too.
	for _, id := range []string{"npm", "rustup", "cargo", "uv", "go-cache", "docker", "dotnet-tools", "topgrade"} {
		if _, ok := win[id]; !ok {
			t.Errorf("%s missing on Windows", id)
		}
	}
	// Nothing written for a POSIX shell, Homebrew, macOS or a Linux package manager.
	for _, id := range []string{"brew", "brew-cleanup", "dev-artifacts", "shell-startup", "omz", "tmux-plugins", "macos", "mas", "apt", "dnf", "journal-vacuum", "xcode-derived"} {
		if _, ok := win[id]; ok {
			t.Errorf("%s is offered on Windows", id)
		}
	}
	for _, s := range For("windows") {
		for _, posix := range []string{"&&", "2>/dev/null", "~/", "$(", "sudo ", "/usr/", "|| "} {
			if strings.Contains(s.Command, posix) {
				t.Errorf("%s on Windows uses POSIX syntax %q: %s", s.ID, posix, s.Command)
			}
		}
	}
	// A step declared read-only is read-only for the policy too, or a
	// declared dangerous one: the policy never calls a Windows step safer
	// than it is declared.
	for _, s := range For("windows") {
		if s.OS != "windows" {
			continue
		}
		if got := safety.Classify(s.Command); got == safety.Blocked {
			t.Errorf("%s is blocked by the policy: %s", s.ID, s.Command)
		} else if s.Risk >= safety.Dangerous && got < safety.Dangerous {
			t.Errorf("%s is declared %v but the policy reads it as %v", s.ID, s.Risk, got)
		}
	}
}

func TestLinuxStepsAreDetectedByBinary(t *testing.T) {
	linuxSteps := ids(For("linux"))
	oldRel := osRelease
	t.Cleanup(func() { osRelease = oldRel })

	osRelease = func() string { return "ID=ubuntu\nVERSION_ID=\"24.04\"\n" }
	withBins(t, "apt-get", "snap", "journalctl")
	for id, want := range map[string]bool{
		"apt": true, "apt-outdated": true, "apt-clean": true, "apt-autoremove": true, "snap": true, "snap-old": true,
		"journal-size": true, "journal-vacuum": true, "system-logs": true,
		"dnf": false, "pacman": false, "pacman-cache": false, "pacman-orphans": false, "zypper": false, "zypper-dup": false, "flatpak": false, "brew": false,
	} {
		if got := linuxSteps[id].Available(); got != want {
			t.Errorf("Ubuntu: %s available=%v, want %v", id, got, want)
		}
	}

	withBins(t, "zypper")
	osRelease = func() string { return "NAME=\"openSUSE Tumbleweed\"\nID=\"opensuse-tumbleweed\"\n" }
	if linuxSteps["zypper"].Available() || !linuxSteps["zypper-dup"].Available() {
		t.Error("Tumbleweed is upgraded with zypper dup, not zypper update")
	}
	osRelease = func() string { return "NAME=\"openSUSE Leap\"\nID=\"opensuse-leap\"\n" }
	if !linuxSteps["zypper"].Available() || linuxSteps["zypper-dup"].Available() {
		t.Error("Leap is upgraded with zypper update")
	}

	withBins(t, "pacman")
	if !linuxSteps["pacman-cache"].Available() || !linuxSteps["pacman-orphans"].Available() || linuxSteps["apt"].Available() {
		t.Error("Arch: the pacman steps and only those")
	}
}

// Every step that uses sudo or deletes must be dangerous, read-only steps
// must be commands the policy lets run unasked, and no install hint on Linux
// may point at a Mac-only way of installing.
func TestLinuxRisk(t *testing.T) {
	for _, s := range For("linux") {
		if s.OS != "linux" {
			if strings.Contains(s.Install, "--cask") {
				t.Errorf("%s: casks do not exist on Linux: %q", s.ID, s.Install)
			}
			continue
		}
		got := safety.Classify(s.Command)
		if strings.Contains(s.Command, "sudo ") && (s.Risk != safety.Dangerous || got != safety.Dangerous) {
			t.Errorf("%s uses sudo: risk %s, policy %s, want dangerous", s.ID, s.Risk, got)
		}
		if s.Category == Clean && s.Risk != safety.Dangerous {
			t.Errorf("%s deletes things and must be dangerous", s.ID)
		}
		if s.Risk < got && s.ID != "system-logs" {
			t.Errorf("%s is declared %s but the policy says %s for %q", s.ID, s.Risk, got, s.Command)
		}
		if s.Risk == safety.Safe && got != safety.Safe && s.ID != "system-logs" {
			t.Errorf("%s: a check must be read-only for the policy, got %s for %q", s.ID, got, s.Command)
		}
	}
}

// The listing shows each category once, on every system, with every step
// and the catalogue's order kept inside a category.
func TestGroupedListsEachCategoryOnce(t *testing.T) {
	for _, goos := range []string{"darwin", "linux", "windows"} {
		all := For(goos)
		grouped := Grouped(all)
		if len(grouped) != len(all) {
			t.Fatalf("%s: %d steps grouped, %d in the catalogue", goos, len(grouped), len(all))
		}
		var headers []string
		last := ""
		for _, s := range grouped {
			if s.Category != last {
				headers = append(headers, s.Category)
				last = s.Category
			}
		}
		if strings.Join(headers, ",") != "check,update,clean,backend" {
			t.Errorf("%s: categories listed as %v", goos, headers)
		}
		for _, cat := range Categories {
			var want, got []string
			for _, s := range all {
				if s.Category == cat {
					want = append(want, s.ID)
				}
			}
			for _, s := range grouped {
				if s.Category == cat {
					got = append(got, s.ID)
				}
			}
			if strings.Join(want, ",") != strings.Join(got, ",") {
				t.Errorf("%s/%s: order changed: %v, want %v", goos, cat, got, want)
			}
		}
	}
	if s := ids(For("linux"))["apt-outdated"]; !strings.Contains(s.Command, "2>/dev/null") || safety.Classify(s.Command) != safety.Safe {
		t.Errorf("apt-outdated must hide apt's script warning and stay read-only: %q", s.Command)
	}
}
