package steps

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/safety"
)

// shells are the interpreters a step's command must work in: POSIX sh and
// zsh, the login shell of a Mac, which splits and globs differently.
func shells(t *testing.T) []string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX commands")
	}
	var out []string
	for _, sh := range []string{"sh", "zsh", "bash"} {
		if p, err := exec.LookPath(sh); err == nil {
			out = append(out, p)
		}
	}
	return out
}

// fakeBin writes executable scripts into a new directory and returns it.
func fakeBin(t *testing.T, scripts map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range scripts {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// run runs a step's command in a shell with PATH starting at bin and the
// extra environment, and returns its output and exit code.
func run(t *testing.T, shell, command, bin string, env ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(shell, "-c", command)
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	cmd.Env = append(cmd.Env, env...)
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return string(out), code
}

// A6: below the free-space minimum the update does not start, says why and
// fails; above it, it runs.
func TestFreeSpacePreflight(t *testing.T) {
	cmd := withSpace("Homebrew", "echo RAN")
	for _, sh := range shells(t) {
		out, code := run(t, sh, cmd, t.TempDir(), "OMEGA_MIN_FREE_GB=100000000")
		if code == 0 || strings.Contains(out, "RAN") || !strings.Contains(out, "GB free") || !strings.Contains(out, "Homebrew needs room") ||
			!strings.Contains(out, "mo clean") {
			t.Errorf("%s: with too little space: exit %d, %q", sh, code, out)
		}
		out, code = run(t, sh, cmd, t.TempDir(), "OMEGA_MIN_FREE_GB=0")
		if code != 0 || strings.TrimSpace(out) != "RAN" {
			t.Errorf("%s: with room: exit %d, %q", sh, code, out)
		}
	}
	mac := ids(For("darwin"))
	for _, id := range []string{"brew", "brew-greedy", "topgrade", "mas", "macos"} {
		if !strings.Contains(mac[id].Command, "omega_space") {
			t.Errorf("%s has no free-space check: %s", id, mac[id].Command)
		}
	}
	if !strings.Contains(spaceCheck, "/System/Volumes/Data") || !strings.Contains(spaceCheck, "OMEGA_MIN_FREE_GB:-10") {
		t.Errorf("the check must read the data volume with a 10 GB default: %s", spaceCheck)
	}
	// Windows keeps a plain topgrade.
	if c := ids(For("windows"))["topgrade"].Command; c != "topgrade --yes --no-retry --skip-notify" {
		t.Errorf("Windows topgrade: %q", c)
	}
}

// caskroom builds a Caskroom with a cask whose app is in place, one whose
// app is gone, one installed under another name (target) and one without
// an app, and a brew that answers from it and logs what it was asked.
func caskroom(t *testing.T) (bin, home, log string) {
	t.Helper()
	home = t.TempDir()
	room := filepath.Join(home, "Caskroom")
	receipt := func(token, body string) {
		dir := filepath.Join(room, token, ".metadata")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "INSTALL_RECEIPT.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	receipt("present", `{"uninstall_artifacts": [
    { "uninstall": [ { "delete": ["/Applications/Old.app"] } ] },
    {
      "app": [
        "Present.app"
      ]
    },
    { "zap": [ { "trash": ["~/Library/Gone.app"] } ] }
  ]}`)
	receipt("gone", `{"uninstall_artifacts":[{"app":["Gone 5.app"]},{"zap":[{"trash":["~/x"]}]}]}`)
	receipt("renamed", `{"uninstall_artifacts": [ { "app": [ "Source.app", { "target": "Renamed.app" } ] } ]}`)
	receipt("cli-only", `{"uninstall_artifacts": [ { "binary": [ "tool" ] } ]}`)
	receipt("needs-sudo", `{"uninstall_artifacts": [ { "pkg": [ "NAPS2.pkg" ] } ]}`)
	if err := os.MkdirAll(filepath.Join(home, "Applications", "Present.app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, "Applications", "Renamed.app"), 0o755); err != nil {
		t.Fatal(err)
	}
	log = filepath.Join(home, "brew.log")
	bin = fakeBin(t, map[string]string{"brew": `echo "brew $*" >> "` + log + `"
case "$1" in
  --caskroom) echo "` + room + `" ;;
  outdated) printf 'present\ngone\nrenamed\nneeds-sudo\n' ;;
esac`})
	return bin, home, log
}

// D9: casks whose app is gone are named with their fix and left out of
// the upgrade; the others are upgraded.
func TestGoneCasksAreLeftOut(t *testing.T) {
	for _, sh := range shells(t) {
		bin, home, log := caskroom(t)
		out, code := run(t, sh, goneCasks+"omega_gone", bin, "HOME="+home)
		if code != 0 || strings.TrimSpace(out) != "gone" {
			t.Errorf("%s: omega_gone found %q (exit %d), want only gone", sh, out, code)
		}

		out, code = run(t, sh, brewUpgrade, bin, "HOME="+home, "OMEGA_MIN_FREE_GB=0")
		calls, _ := os.ReadFile(log)
		if code != 0 || !strings.Contains(out, "gone: reinstall it (brew reinstall --cask gone)") {
			t.Errorf("%s: brew step: exit %d, %q", sh, code, out)
		}
		if !strings.Contains(string(calls), "brew update\n") || !strings.Contains(string(calls), "brew upgrade --formula\n") ||
			!strings.Contains(string(calls), "brew upgrade --cask present renamed needs-sudo\n") || strings.Contains(string(calls), "upgrade --cask gone") {
			t.Errorf("%s: brew was asked:\n%s", sh, calls)
		}

		os.Remove(log)
		out, code = run(t, sh, brewGreedy, bin, "HOME="+home, "OMEGA_MIN_FREE_GB=0")
		calls, _ = os.ReadFile(log)
		if code != 0 || !strings.Contains(string(calls), "brew upgrade --cask --greedy present renamed needs-sudo\n") || strings.Contains(string(calls), "gone\n") && strings.Contains(string(calls), "upgrade --cask --greedy gone") {
			t.Errorf("%s: brew-greedy: exit %d, %q, brew was asked:\n%s", sh, code, out, calls)
		}
	}
	// With every app in place, the plain upgrade.
	for _, sh := range shells(t) {
		bin, home, log := caskroom(t)
		os.MkdirAll(filepath.Join(home, "Applications", "Gone 5.app"), 0o755)
		if _, code := run(t, sh, brewUpgrade, bin, "HOME="+home, "OMEGA_MIN_FREE_GB=0"); code != 0 {
			t.Errorf("%s: exit %d", sh, code)
		}
		calls, _ := os.ReadFile(log)
		if !strings.HasSuffix(string(calls), "brew upgrade\n") || strings.Contains(string(calls), "--formula") {
			t.Errorf("%s: with nothing gone brew was asked:\n%s", sh, calls)
		}
	}
	// The fix offered never forces anything itself.
	if strings.Contains(goneWarning, "--force") {
		t.Errorf("the advice must not carry a forced command: %s", goneWarning)
	}
}

// D9: in a run that cannot answer sudo, the casks that install with a
// .pkg are left out and named; with sudo they are upgraded.
func TestSudoCasksAreLeftOutWithoutSudo(t *testing.T) {
	for _, sh := range shells(t) {
		bin, home, log := caskroom(t)
		if out, _ := run(t, sh, sudoCasks+"omega_sudocasks", bin, "HOME="+home); strings.TrimSpace(out) != "needs-sudo" {
			t.Errorf("%s: omega_sudocasks found %q", sh, out)
		}
		out, code := run(t, sh, brewUpgrade, bin, "HOME="+home, "OMEGA_MIN_FREE_GB=0", "OMEGA_SUDO=no")
		calls, _ := os.ReadFile(log)
		if code != 0 || !strings.Contains(out, "ask for the administrator password") || strings.Contains(string(calls), "needs-sudo") ||
			!strings.Contains(string(calls), "brew upgrade --cask present renamed\n") {
			t.Errorf("%s: without sudo: exit %d, %q, brew was asked:\n%s", sh, code, out, calls)
		}
		os.Remove(log)
		if _, code := run(t, sh, brewUpgrade, bin, "HOME="+home, "OMEGA_MIN_FREE_GB=0", "OMEGA_SUDO=yes"); code != 0 {
			t.Errorf("%s: exit %d", sh, code)
		}
		if calls, _ := os.ReadFile(log); !strings.Contains(string(calls), "needs-sudo") {
			t.Errorf("%s: with sudo the cask is upgraded:\n%s", sh, calls)
		}
	}
}

// D8, D12, G4: the topgrade commands.
func TestTopgradeCommand(t *testing.T) {
	mac, linux := ids(For("darwin")), ids(For("linux"))
	if !strings.Contains(mac["topgrade"].Command, "--disable system") || !strings.Contains(mac["topgrade-preview"].Command, "--disable system") {
		t.Error("on a Mac topgrade's system step (softwareupdate, may restart) must be left out")
	}
	if strings.Contains(linux["topgrade"].Command, "--disable system") {
		t.Error("on Linux the system step stays: it asks omega for sudo when it runs")
	}
	// The policy refuses a command that changes a variable and runs sudo;
	// the real run changes variables, so it must not call sudo itself.
	for _, s := range []Step{mac["topgrade"], linux["topgrade"], mac["topgrade-preview"], mac["macos"]} {
		if safety.Classify(s.Command) == safety.Blocked {
			t.Errorf("%s is blocked by the policy", s.ID)
		}
	}
	// Neither runs sudo itself: the preview must not need an approval every time.
	for _, s := range []Step{mac["topgrade"], mac["topgrade-preview"]} {
		if safety.AlwaysConfirm(s.Command) {
			t.Errorf("%s always asks for approval", s.ID)
		}
	}

	for _, sh := range shells(t) {
		home := t.TempDir()
		cfg := filepath.Join(home, ".config")
		os.MkdirAll(filepath.Join(cfg, "topgrade.d"), 0o755)
		os.WriteFile(filepath.Join(cfg, "topgrade.toml"), []byte("[misc]\n# pre_sudo = true\n"), 0o644)
		args := filepath.Join(home, "args")
		// topgrade records its arguments and the configuration it was given,
		// and does what pre_sudo does when the configuration asks for it.
		bin := fakeBin(t, map[string]string{
			"topgrade": `echo "$*" > "` + args + `"
if [ "$1" = --config ]; then cat "$2"; echo; cat "$(sed -n "s/.*'\\(.*omega-nosudo.toml\\)'.*/\\1/p" "$2")"; exit 0; fi
grep -qs 'pre_sudo = true' "$XDG_CONFIG_HOME"/topgrade.d/*.toml && { sudo -v || { echo "Failed to elevate permissions"; exit 1; }; }
echo upgraded`,
			"sudo":   `exit 1`, // no sudo in this run
			"docker": `exit 1`, // the daemon is down
		})
		env := []string{"HOME=" + home, "XDG_CONFIG_HOME=" + cfg, "OMEGA_MIN_FREE_GB=0"}

		// Without pre_sudo nothing changes.
		out, code := run(t, sh, topgradeCommand("linux", false), bin, env...)
		got, _ := os.ReadFile(args)
		if code != 0 || strings.Contains(out, "omega:") || strings.TrimSpace(string(got)) != "--yes --no-retry --skip-notify" {
			t.Errorf("%s: no sudo in the configuration: exit %d, %q, args %q", sh, code, out, got)
		}

		// With pre_sudo in a drop-in, the run gets a configuration that
		// includes the user's files and turns the check off, last.
		os.WriteFile(filepath.Join(cfg, "topgrade.d", "omega.toml"), []byte("[misc]\npre_sudo = true\n"), 0o644)
		os.WriteFile(filepath.Join(cfg, "topgrade.d", "a.toml"), []byte("[misc]\n"), 0o644)
		out, code = run(t, sh, topgradeCommand("darwin", false), bin, env...)
		got, _ = os.ReadFile(args)
		wantPaths := "paths = ['" + filepath.Join(cfg, "topgrade.toml") + "', '" + filepath.Join(cfg, "topgrade.d", "a.toml") + "', '" +
			filepath.Join(cfg, "topgrade.d", "omega.toml") + "', '"
		if code != 0 || !strings.Contains(out, "turns that off") || !strings.Contains(out, wantPaths) ||
			!strings.Contains(out, "omega-nosudo.toml']") || !strings.Contains(out, "pre_sudo = false\nsudo_loop = false") ||
			!strings.HasSuffix(strings.TrimSpace(string(got)), "--yes --no-retry --skip-notify --disable system") {
			t.Errorf("%s: with pre_sudo: exit %d, %q, args %q", sh, code, out, got)
		}
		leftovers, _ := filepath.Glob(filepath.Join(os.TempDir(), "omega-topgrade.*"))
		for _, l := range leftovers {
			if fi, err := os.Stat(l); err == nil && time.Since(fi.ModTime()) < time.Minute {
				t.Errorf("%s: left %s behind", sh, l)
			}
		}

		// The preview warns and leaves out containers with Docker down
		// (the fake topgrade fails at its sudo check, as the real one would).
		out, _ = run(t, sh, topgradeCommand("darwin", true), bin, env...)
		got, _ = os.ReadFile(args)
		if !strings.Contains(out, "needs sudo") || !strings.Contains(out, "will fail at the sudo step") ||
			strings.TrimSpace(string(got)) != "--dry-run --no-retry --skip-notify --disable system --disable containers" {
			t.Errorf("%s: preview: %q, args %q", sh, out, got)
		}

		// sudo_loop counts too; a commented line does not.
		os.WriteFile(filepath.Join(cfg, "topgrade.d", "omega.toml"), []byte("[misc]\n  sudo_loop = true\n"), 0o644)
		if out, _ := run(t, sh, topgradeCommand("darwin", true), bin, env...); !strings.Contains(out, "needs sudo") {
			t.Errorf("%s: sudo_loop not seen: %q", sh, out)
		}
		os.WriteFile(filepath.Join(cfg, "topgrade.d", "omega.toml"), []byte("[misc]\n# sudo_loop = true\n"), 0o644)
		if out, _ := run(t, sh, topgradeCommand("darwin", true), bin, env...); strings.Contains(out, "needs sudo") {
			t.Errorf("%s: a commented line counted: %q", sh, out)
		}
	}
}

// D10: Mole reads nothing from the terminal, its /dev/tty noise is dropped
// and its exit code kept.
func TestMoleRunsWithoutTerminal(t *testing.T) {
	for _, sh := range shells(t) {
		bin := fakeBin(t, map[string]string{"mo": `[ -t 0 ] && echo "stdin is a terminal"
read -r line && echo "read: $line"
echo "cleaning $*"
echo "/x/sudo.sh: line 106: /dev/tty: Device not configured" >&2
exit 3`})
		out, code := run(t, sh, moleQuiet("clean"), bin)
		if code != 3 || !strings.Contains(out, "cleaning clean") || strings.Contains(out, "Device not configured") ||
			strings.Contains(out, "read:") || strings.Contains(out, "terminal") {
			t.Errorf("%s: exit %d, %q", sh, code, out)
		}
	}
	mac := ids(For("darwin"))
	for id, want := range map[string]string{"mole-clean": "mo clean </dev/null", "mole-clean-preview": "mo clean --dry-run </dev/null",
		"mole-optimize": "mo optimize </dev/null", "mole-purge": "mo purge --yes </dev/null", "mole-purge-preview": "mo purge --dry-run </dev/null"} {
		if !strings.Contains(mac[id].Command, want) {
			t.Errorf("%s: %q", id, mac[id].Command)
		}
	}
	if mac["mole-purge"].Category != Clean || mac["mole-purge"].Risk != safety.Dangerous || mac["mole-purge-preview"].Risk != safety.Safe {
		t.Errorf("mole-purge must be a dangerous clean with a safe preview")
	}
}

// D16: one root per folder, staleness from the project's files, and a line
// when there is nothing.
func TestDevArtifacts(t *testing.T) {
	for _, sh := range shells(t) {
		home := t.TempDir()
		old := time.Now().AddDate(0, -6, 0)
		mk := func(rel string, stale bool) {
			p := filepath.Join(home, rel)
			os.MkdirAll(filepath.Dir(p), 0o755)
			os.WriteFile(p, []byte("x"), 0o644)
			if stale {
				os.Chtimes(p, old, old)
			}
		}
		out, code := run(t, sh, devArtifacts, t.TempDir(), "HOME="+home)
		if code != 0 || !strings.Contains(out, "0 stale build folders found") {
			t.Errorf("%s: nothing to find: exit %d, %q", sh, code, out)
		}
		mk("Dev/old/package.json", true)
		mk("Dev/old/node_modules/a/index.js", false) // installs touch the build folder
		mk("Dev/live/package.json", false)
		mk("Dev/live/node_modules/a/index.js", true)
		// The same folder reached twice (~/code → ~/Dev, as ~/dev is on a
		// disk that ignores case).
		os.Symlink(filepath.Join(home, "Dev"), filepath.Join(home, "code"))
		out, code = run(t, sh, devArtifacts, t.TempDir(), "HOME="+home)
		if code != 0 || strings.Count(out, "node_modules") != 1 || !strings.Contains(out, "old/node_modules") {
			t.Errorf("%s: exit %d, %q", sh, code, out)
		}
	}
}

// D4, D5, D6: checks that exit 0 when they did their job.
func TestChecksDoNotFailOnAdvice(t *testing.T) {
	mac, linux := ids(For("darwin")), ids(For("linux"))
	for _, s := range []Step{mac["system-logs"], linux["system-logs"]} {
		if !strings.HasSuffix(s.Command, "; true") {
			t.Errorf("system-logs must not take the exit code of its last pipe: %q", s.Command)
		}
	}
	for _, sh := range shells(t) {
		for code, want := range map[string]int{"0": 0, "1": 0, "2": 2} {
			bin := fakeBin(t, map[string]string{"brew": "echo 'Warning: icu4c@78 has been deprecated'; exit " + code})
			out, got := run(t, sh, mac["brew-doctor"].Command, bin)
			if got != want || (code == "1") != strings.Contains(out, "not a failure") {
				t.Errorf("%s: brew doctor exit %s: step exit %d, %q", sh, code, got, out)
			}
		}
		bin := fakeBin(t, map[string]string{"docker": "echo 'Cannot connect to the Docker daemon' >&2; exit 1"})
		for _, s := range []Step{mac["docker-df"], linux["docker-df"]} {
			out, got := run(t, sh, s.Command, bin, "HOME="+t.TempDir())
			if got != 0 || !strings.Contains(out, "Docker is not running") || strings.Contains(out, "Cannot connect") {
				t.Errorf("%s: %s with Docker down: exit %d, %q", sh, s.OS, got, out)
			}
		}
	}
	if !strings.Contains(mac["docker-df"].Command, "Docker.raw") || strings.Contains(mac["docker-df"].Command, "open ") {
		t.Error("docker-df measures the disk image and never starts Docker")
	}
}

// D7: the updates with nothing to update are not offered.
func TestNothingToUpdateIsNotAvailable(t *testing.T) {
	old := probe
	t.Cleanup(func() { probe = old })
	answers := map[string]struct {
		out string
		ok  bool
	}{}
	probe = func(name string, args ...string) (string, bool) {
		a := answers[name]
		return a.out, a.ok
	}
	set := func(name, out string, ok bool) {
		answers[name] = struct {
			out string
			ok  bool
		}{out, ok}
	}
	for _, c := range []struct {
		name, out string
		ok, want  bool
		f         func() bool
	}{
		{"helm", "[]\n", true, false, helmHasRepos},
		{"helm", "", false, false, helmHasRepos},
		{"helm", `[{"name":"bitnami","url":"https://charts.bitnami.com/bitnami"}]`, true, true, helmHasRepos},
		{"pnpm", `[{"path":"/g","private":true,"dependencies":{}}]`, true, false, pnpmHasGlobals},
		{"pnpm", `[{"path":"/g","dependencies":{"typescript":{"version":"5.9.0"}}}]`, true, true, pnpmHasGlobals},
		{"pnpm", "", false, true, pnpmHasGlobals}, // cannot tell: offered, as before
		{"uv", "", true, false, uvHasTools},
		{"uv", "ruff v0.14.0\n- ruff\n", true, true, uvHasTools},
	} {
		set(c.name, c.out, c.ok)
		if got := c.f(); got != c.want {
			t.Errorf("%s %q ok=%v: %v, want %v", c.name, c.out, c.ok, got, c.want)
		}
	}
	all := ids(For("linux"))
	for _, id := range []string{"helm-repos", "pnpm", "uv"} {
		if all[id].Detect == nil {
			t.Errorf("%s has no detection", id)
		}
	}
	n := 0
	f := once(func() bool { n++; return true })
	f()
	f()
	if n != 1 {
		t.Errorf("detection ran %d times", n)
	}
}
