package steps

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// The shell pieces below run in the user's login shell, which is zsh on a
// Mac: they are written for both POSIX sh and zsh. zsh does not split an
// unquoted variable into words and stops on a glob with no match, so lists
// go through `while read` and `xargs`, never `for x in $list` or a glob.

// MinFreeGB is the free space, in GB, below which the steps that download
// and unpack updates (Homebrew, topgrade, the App Store, macOS) do not
// start: a full disk half-way through an upgrade leaves packages broken and
// thousands of "No space left on device" lines. OMEGA_MIN_FREE_GB overrides
// it for one run.
const MinFreeGB = 10

// spaceCheck defines omega_space, which prints why and fails when the data
// volume (on a Mac /System/Volumes/Data, where the user's files and the
// downloads live; / elsewhere) has less than MinFreeGB free. A df that
// cannot be read lets the step run. It sets no shell variable: the policy
// refuses a command that changes its environment and then runs sudo, and
// the macos step does.
var spaceCheck = fmt.Sprintf(`omega_space() { df -Pk "$(if [ -d /System/Volumes/Data ]; then echo /System/Volumes/Data; else echo /; fi)" 2>/dev/null | `+
	`awk -v min="${OMEGA_MIN_FREE_GB:-%d}" -v tool="$1" 'NR == 2 && $4 < min * 1048576 { `+
	`printf "omega: only %%.1f GB free on %%s; %%s needs room (at least %%d GB) to download and unpack updates. Not running it: free space first (mo clean, omega disk) and run again.\n", $4 / 1048576, $6, tool, min; exit 1 }'; }; `, MinFreeGB)

// withSpace runs command only when there is room for what it downloads.
func withSpace(tool, command string) string {
	return spaceCheck + "omega_space '" + tool + "' || exit 1; " + command
}

// goneCasks defines omega_gone, which lists the installed casks whose app is
// no longer where Homebrew put it (deleted by hand, or moved by the app's own
// updater, like Alfred 5 → 6). Upgrading one fails every run until it is
// reinstalled or removed. The apps come from each cask's install receipt; a
// cask counts as present when any of its apps (or their targets) is in
// /Applications or ~/Applications, so a renamed target is not reported.
const goneCasks = `omega_gone() { cr=$(brew --caskroom 2>/dev/null) && [ -d "$cr" ] || return 0; ` +
	`find "$cr" -mindepth 3 -maxdepth 3 -path '*/.metadata/INSTALL_RECEIPT.json' 2>/dev/null | while IFS= read -r r; do ` +
	`t=${r#"$cr"/}; t=${t%%/*}; ` +
	`apps=$(awk '{ s = $0 } !f && match(s, /"app": *\[/) { f = 1; s = substr(s, RSTART + RLENGTH) } ` +
	`f { e = index(s, "]"); if (e) { s = substr(s, 1, e - 1); f = 0 }; while (match(s, /"[^"]*\.app"/)) { print substr(s, RSTART + 1, RLENGTH - 2); s = substr(s, RSTART + RLENGTH) } }' "$r"); ` +
	`[ -n "$apps" ] || continue; ` +
	`printf '%s\n' "$apps" | { ok=; while IFS= read -r a; do case $a in /*) [ -e "$a" ] && ok=1 ;; ` +
	`*) { [ -e "/Applications/$a" ] || [ -e "$HOME/Applications/$a" ]; } && ok=1 ;; esac; done; [ -n "$ok" ] || echo "$t"; }; done; }; `

// sudoCasks defines omega_sudocasks, which lists the installed casks that
// install with a .pkg or an installer of their own: upgrading them asks for
// the administrator password. OMEGA_SUDO, set by omega's agent, says
// whether this run can answer it (yes, ask or no); where it says no they
// are left out and named, instead of failing the whole step with "a
// password is required" (naps2, obs and busycal did, run after run).
const sudoCasks = `omega_sudocasks() { cr=$(brew --caskroom 2>/dev/null) && [ -d "$cr" ] || return 0; ` +
	`find "$cr" -mindepth 3 -maxdepth 3 -path '*/.metadata/INSTALL_RECEIPT.json' 2>/dev/null | while IFS= read -r r; do ` +
	`grep -q -e '"pkg": *\[' -e '"installer": *\[' "$r" && { t=${r#"$cr"/}; echo "${t%%/*}"; }; done; }; `

// sudoWarning names the casks left out because this run has no sudo, and
// adds them to the list of casks the upgrade skips.
const sudoWarning = `if [ "$OMEGA_SUDO" = no ]; then sc=$(omega_sudocasks); if [ -n "$sc" ]; then ` +
	`echo "omega: these casks ask for the administrator password to upgrade, and this run has no sudo; they are left out (run omega with --sudo, or brew upgrade --cask <name> yourself):"; ` +
	`printf '  %s\n' $sc; gone=$(printf '%s\n%s\n' "$gone" "$sc" | grep -v '^$'); fi; fi; `

// keepCasks defines omega_keep, which in an unattended run (OMEGA_UNATTENDED=1,
// set by omega) lists the outdated casks an upgrade would disturb, each
// with why: those whose app is open, because Homebrew quits it to replace
// it and does not open it again (a menu bar app then disappears from the
// menu bar), and those whose uninstall stanza removes a login item or a
// background service (Alfred, Tailscale, Dropbox, 1Password…), which the
// new version does not always put back. Upgrading them is left for when
// the user is there. The stanza is read from the install receipt, before
// its zap section; an app counts as open when a process runs from inside
// its bundle.
const keepCasks = `omega_keep() { [ "$OMEGA_UNATTENDED" = 1 ] || return 0; cr=$(brew --caskroom 2>/dev/null) && [ -d "$cr" ] || return 0; ` +
	`out=$(brew outdated --cask --quiet 2>/dev/null); [ -n "$out" ] || return 0; ps=$(ps -axo command= 2>/dev/null); ` +
	`la=$(lsappinfo list 2>/dev/null | grep -o 'bundleID="[^"]*"'); ` +
	`printf '%s\n' "$out" | while IFS= read -r t; do r="$cr/$t/.metadata/INSTALL_RECEIPT.json"; [ -f "$r" ] || continue; ` +
	`u=$(tr -d '\n' < "$r" | sed 's/"zap".*//'); ` +
	`case $u in *'"login_item"'*|*'"launchctl"'*) echo "$t (upgrading it removes what it starts at login or in the background)"; continue ;; esac; ` +
	// Open, by the bundle ids its stanza quits (a .pkg cask has no app
	// artifact) or by a process running from inside one of its apps.
	`o=$(printf '%s' "$u" | awk '{ if (match($0, /"quit": *(\[[^]]*\]|"[^"]*")/)) { s = substr($0, RSTART + 7, RLENGTH - 7); ` +
	`while (match(s, /"[^"]*"/)) { print substr(s, RSTART + 1, RLENGTH - 2); s = substr(s, RSTART + RLENGTH) } } }' | ` +
	`while IFS= read -r b; do printf '%s\n' "$la" | grep -qxF "bundleID=\"$b\"" && { echo y; break; }; done); ` +
	`[ -n "$o" ] || o=$(awk '{ s = $0 } !f && match(s, /"app": *\[/) { f = 1; s = substr(s, RSTART + RLENGTH) } ` +
	`f { e = index(s, "]"); if (e) { s = substr(s, 1, e - 1); f = 0 }; while (match(s, /"[^"]*\.app"/)) { print substr(s, RSTART + 1, RLENGTH - 2); s = substr(s, RSTART + RLENGTH) } }' "$r" | ` +
	`while IFS= read -r a; do a=${a##*/}; printf '%s\n' "$ps" | grep -qF "/$a/Contents/MacOS/" && { echo y; break; }; done); ` +
	`[ -z "$o" ] || echo "$t (it is open: upgrading it would quit it)"; done; }; `

// keepWarning names the casks omega_keep found and adds them to the casks
// the upgrade leaves out.
const keepWarning = `kl=$(omega_keep); kc=; if [ -n "$kl" ]; then ` +
	`echo "omega: unattended run: these apps are not upgraded now, so nothing you have open is closed and nothing is taken out of what starts at login; they are upgraded the next time you run omega update at the Mac (or brew upgrade --cask <name>):"; ` +
	`printf '%s\n' "$kl" | sed 's/^/  /'; kc=$(printf '%s\n' "$kl" | sed 's/ .*//'); gone=$(printf '%s\n%s\n' "$gone" "$kc" | grep -v '^$'); fi; `

// openCasks defines what brings back what a Homebrew upgrade closes.
// Homebrew quits an app to replace it and does not open it again, so a
// menu bar app disappears from the menu bar until the next login.
//
// omega_open, before the upgrade, lists the apps of the outdated casks
// (--greedy: the self-updating ones too) that are open: the path of each
// running app, or b:<bundle id> for a .pkg cask, known by what its stanza
// quits. omega_reopen, after it, opens again in the background those that
// are no longer running. omega_logins lists the outdated casks whose
// uninstall stanza removes a login item; omega_loginnote, after the
// upgrade, names those that were upgraded: putting a login item back needs
// the Automation permission, which only the user can give, so the user is
// told where to turn it on again.
const openCasks = `omega_open() { cr=$(brew --caskroom 2>/dev/null) && [ -d "$cr" ] || return 0; ` +
	`out=$(brew outdated --cask --quiet $1 2>/dev/null); [ -n "$out" ] || return 0; ps=$(ps -axo command= 2>/dev/null); ` +
	`la=$(lsappinfo list 2>/dev/null | grep -o 'bundleID="[^"]*"'); ` +
	`printf '%s\n' "$out" | while IFS= read -r t; do r="$cr/$t/.metadata/INSTALL_RECEIPT.json"; [ -f "$r" ] || continue; ` +
	`f=$(awk '{ s = $0 } !f && match(s, /"app": *\[/) { f = 1; s = substr(s, RSTART + RLENGTH) } ` +
	`f { e = index(s, "]"); if (e) { s = substr(s, 1, e - 1); f = 0 }; while (match(s, /"[^"]*\.app"/)) { print substr(s, RSTART + 1, RLENGTH - 2); s = substr(s, RSTART + RLENGTH) } }' "$r" | ` +
	`while IFS= read -r a; do a=${a##*/}; printf '%s\n' "$ps" | grep -F "/$a/Contents/MacOS/" | head -n 1 | sed 's#/Contents/MacOS/.*##'; done); ` +
	`if [ -n "$f" ]; then printf '%s\n' "$f"; else tr -d '\n' < "$r" | sed 's/"zap".*//' | ` +
	`awk '{ if (match($0, /"quit": *(\[[^]]*\]|"[^"]*")/)) { s = substr($0, RSTART + 7, RLENGTH - 7); ` +
	`while (match(s, /"[^"]*"/)) { print substr(s, RSTART + 1, RLENGTH - 2); s = substr(s, RSTART + RLENGTH) } } }' | ` +
	`while IFS= read -r b; do printf '%s\n' "$la" | grep -qxF "bundleID=\"$b\"" && echo "b:$b"; done; fi; done | sort -u; }; ` +
	`omega_reopen() { [ -n "$op" ] || return 0; ps=$(ps -axo command= 2>/dev/null); la=$(lsappinfo list 2>/dev/null | grep -o 'bundleID="[^"]*"'); ` +
	`printf '%s\n' "$op" | while IFS= read -r p; do case $p in ` +
	`b:*) b=${p#b:}; printf '%s\n' "$la" | grep -qxF "bundleID=\"$b\"" || { open -g -b "$b" 2>/dev/null && echo "omega: reopened $b, which Homebrew closed to update it"; } ;; ` +
	`*) printf '%s\n' "$ps" | grep -qF "$p/Contents/MacOS/" || { [ -d "$p" ] && open -g "$p" 2>/dev/null && echo "omega: reopened ${p##*/}, which Homebrew closed to update it"; } ;; ` +
	`esac; done; }; ` +
	`omega_logins() { cr=$(brew --caskroom 2>/dev/null) && [ -d "$cr" ] || return 0; brew outdated --cask --quiet $1 2>/dev/null | while IFS= read -r t; do ` +
	`r="$cr/$t/.metadata/INSTALL_RECEIPT.json"; [ -f "$r" ] || continue; case $(tr -d '\n' < "$r" | sed 's/"zap".*//') in *'"login_item"'*) echo "$t" ;; esac; done; }; ` +
	`omega_loginnote() { [ -n "$ll" ] || return 0; now=$(brew outdated --cask --quiet $1 2>/dev/null); ` +
	`printf '%s\n' "$ll" | grep -vxF -e "$kc" -e "$now" | while IFS= read -r t; do [ -n "$t" ] || continue; ` +
	`echo "omega: updating $t removed its login item; if it should open at login, turn that on again in its settings or in System Settings > General > Login Items."; done; }; `

// goneWarning prints the casks omega_gone found, with what fixes each one.
const goneWarning = `echo "omega: these casks are installed but their app is gone, so upgrading them would fail; they are left out of this upgrade:"; ` +
	`printf '%s\n' "$gone" | while IFS= read -r c; do echo "  $c: reinstall it (brew reinstall --cask $c) or remove it (brew uninstall --cask $c, with the force option if Homebrew says the app is missing)"; done; `

// brewUpgrade is the Homebrew step: room first, then everything except the
// casks whose app is gone, which are named with their fix.
var brewUpgrade = withSpace("Homebrew", goneCasks+sudoCasks+keepCasks+openCasks+`brew update || exit 1; gone=$(omega_gone); missing=$gone; `+sudoWarning+keepWarning+
	`op=$(omega_open); ll=$(omega_logins); `+
	`if [ -z "$gone" ]; then brew upgrade; else gone=$missing; [ -z "$gone" ] || { `+goneWarning+`}; gone=$(printf '%s\n%s\n%s\n' "$missing" "$sc" "$kc" | grep -v '^$'); `+
	`brew upgrade --formula; rc=$?; brew outdated --cask --quiet 2>/dev/null | { grep -vxF "$gone" || true; } | xargs -r brew upgrade --cask || rc=1; (exit $rc); fi; brc=$?; omega_reopen; omega_loginnote; (exit $brc)`)

// brewGreedy upgrades the casks that update themselves too, leaving out
// the ones whose app is gone.
var brewGreedy = withSpace("Homebrew", goneCasks+sudoCasks+openCasks+`gone=$(omega_gone); missing=$gone; `+sudoWarning+
	`op=$(omega_open --greedy); ll=$(omega_logins --greedy); `+
	`if [ -z "$gone" ]; then brew upgrade --cask --greedy; else gone=$missing; [ -z "$gone" ] || { `+goneWarning+`}; gone=$(printf '%s\n%s\n' "$missing" "$sc" | grep -v '^$'); `+
	`brew outdated --cask --greedy --quiet 2>/dev/null | { grep -vxF "$gone" || true; } | xargs -r brew upgrade --cask --greedy; fi; brc=$?; omega_reopen; omega_loginnote --greedy; (exit $brc)`)

// masUpgrade is the App Store step. In an unattended run the apps that are
// open are left out and named: the App Store quits an app to update it.
var masUpgrade = withSpace("the App Store", `ps=$(ps -axo command= 2>/dev/null); if [ "$OMEGA_UNATTENDED" = 1 ]; then `+
	`mas outdated 2>/dev/null | while read -r id rest; do n=$(printf '%s' "$rest" | sed 's/ *(.*//; s/ *$//'); [ -n "$id" ] || continue; `+
	`if [ -n "$n" ] && printf '%s\n' "$ps" | grep -qF "/$n.app/Contents/MacOS/"; then `+
	`echo "omega: $n is open, so this unattended run does not update it (the App Store would quit it); it is updated the next time you run omega update at the Mac." >&2; `+
	`else echo "$id"; fi; done | xargs -r mas upgrade; else `+
	// With someone there everything is updated, and what the App Store
	// closed to update it is opened again.
	`mo=$(mas outdated 2>/dev/null | while read -r id rest; do n=$(printf '%s' "$rest" | sed 's/ *(.*//; s/ *$//'); `+
	`[ -n "$n" ] && printf '%s\n' "$ps" | grep -qF "/$n.app/Contents/MacOS/" && echo "$n"; done); mas upgrade; mrc=$?; `+
	`if [ -n "$mo" ]; then ps=$(ps -axo command= 2>/dev/null); printf '%s\n' "$mo" | while IFS= read -r n; do `+
	`printf '%s\n' "$ps" | grep -qF "/$n.app/Contents/MacOS/" || { open -g -a "$n" 2>/dev/null && echo "omega: reopened $n, which the App Store closed to update it"; }; done; fi; (exit $mrc); fi`)

// topgradeSudo defines omega_tg_sudo, true when the user's topgrade
// configuration (main file or drop-ins, in either place topgrade looks)
// asks for sudo at the start with pre_sudo or sudo_loop.
const topgradeSudo = `omega_tg_sudo() { { for c in "${XDG_CONFIG_HOME:-$HOME/.config}" "$HOME/Library/Application Support"; do ` +
	`cat "$c/topgrade.toml" 2>/dev/null; find "$c/topgrade.d" -name '*.toml' -exec cat {} + 2>/dev/null; done; } | ` +
	`grep -q -e '^[[:space:]]*pre_sudo[[:space:]]*=[[:space:]]*true' -e '^[[:space:]]*sudo_loop[[:space:]]*=[[:space:]]*true'; }; `

// topgradeNoPreSudo runs topgrade with the user's configuration plus one
// last file that turns pre_sudo and sudo_loop off. topgrade reads no
// drop-ins with --config, so the generated file includes the main file and
// every drop-in itself, in topgrade's order, and the override last (a later
// include wins). The files go away after the run.
const topgradeNoPreSudo = `c="${XDG_CONFIG_HOME:-$HOME/.config}"; [ -f "$c/topgrade.toml" ] || [ ! -f "$HOME/Library/Application Support/topgrade.toml" ] || c="$HOME/Library/Application Support"; ` +
	`d=$(mktemp -d "${TMPDIR:-/tmp}/omega-topgrade.XXXXXX") || exit 1; ` +
	`printf '[misc]\npre_sudo = false\nsudo_loop = false\n' > "$d/omega-nosudo.toml"; ` +
	`{ printf '[include]\npaths = ['; { [ ! -f "$c/topgrade.toml" ] || echo "$c/topgrade.toml"; find "$c/topgrade.d" -name '*.toml' 2>/dev/null | sort; echo "$d/omega-nosudo.toml"; } | ` +
	`awk '{ printf "%s\047%s\047", (NR > 1 ? ", " : ""), $0 }'; printf ']\n'; } > "$d/topgrade.toml"; ` +
	`echo "omega: this topgrade configuration asks for sudo at the start (pre_sudo or sudo_loop), which stops the whole run when sudo is not available; omega turns that off for this run. The steps that need sudo ask omega for it when they run (or fail on their own without it: run omega with --sudo to include them)."; `

// topgradeCommand is the topgrade step on a POSIX system.
//
// On a Mac topgrade's `system` step (softwareupdate --install --all) is
// always left out: it can restart the Mac in the middle of the run, and
// omega's own macos-check and macos steps cover it.
//
// With pre_sudo or sudo_loop in the configuration, topgrade runs `sudo -v`
// first and stops the whole run when it fails, which is what happens when
// omega has no sudo (no --sudo, nobody to type the password); --disable
// does not skip that check. The real run turns both off (topgradeNoPreSudo):
// under omega a step that needs sudo asks omega's helper when it runs, so
// asking up front gains nothing. The preview warns about it. Neither calls
// sudo: the policy refuses a command that changes a variable and runs sudo
// (how a command would swap what sudo runs), and asks every time for one
// that runs sudo, which a read-only preview must not.
func topgradeCommand(goos string, dry bool) string {
	args := `--no-retry --skip-notify`
	if goos == mac {
		args += ` --disable system`
	}
	if dry {
		// The real run starts Docker in a pre_command when it is configured
		// to; the preview only simulates that, so its containers step would
		// report a failure the real run does not have.
		return topgradeSudo +
			`if omega_tg_sudo; then ` +
			`echo "omega: WARNING: this topgrade configuration needs sudo at the start (pre_sudo or sudo_loop = true); in a run without sudo (no omega --sudo, nobody to type the password) topgrade will fail at the sudo step. ` +
			`omega's topgrade step turns that check off, so only the steps that need sudo fail; run omega with --sudo to include them."; fi; ` +
			`if command -v docker >/dev/null 2>&1 && ! docker info >/dev/null 2>&1; then ` +
			`echo "omega: Docker is not running, so this preview leaves out topgrade's containers step"; ` +
			`topgrade --dry-run ` + args + ` --disable containers; else topgrade --dry-run ` + args + `; fi`
	}
	return topgradeSudo + spaceCheck + `omega_space topgrade || exit 1; ` +
		`if omega_tg_sudo; then ` + topgradeNoPreSudo +
		`topgrade --config "$d/topgrade.toml" --yes ` + args + `; rc=$?; rm -f "$d/topgrade.toml" "$d/omega-nosudo.toml"; rmdir "$d"; (exit $rc); ` +
		`else topgrade --yes ` + args + `; fi`
}

// moleQuiet runs a Mole command with no terminal to read from, so it takes
// its non-interactive path (Mole cleans what needs no sudo and says what it
// skipped) instead of waiting for a key, and drops the lines Mole prints
// when it tries /dev/tty for a password where there is none ("Device not
// configured"). The exit status is Mole's.
func moleQuiet(args string) string {
	return `exec 4>&1; rc=$({ { mo ` + args + ` </dev/null 2>&1; echo $? >&3; } | grep -v '/dev/tty: Device not configured' >&4; } 3>&1); exec 4>&-; (exit $rc)`
}

// devArtifacts lists build folders in projects nobody has touched for 60
// days. The roots are deduplicated by inode: on a disk that ignores case
// ~/Dev and ~/dev are one folder. A project is stale when no file in it
// (outside the build folders and .git, four levels down) changed in 60 days;
// the build folder's own date says nothing, since installs and builds touch
// it. With nothing found it says so.
const devArtifacts = `out=$(for d in ~/Dev ~/Developer ~/dev ~/Projects ~/projects ~/Code ~/code ~/src ~/work ~/Work ~/Sites ~/repos ~/git ~/Documents/GitHub; do ` +
	`[ -d "$d" ] && printf '%s %s\n' "$(command ls -dLi "$d" | awk '{print $1}')" "$d"; done | awk '!seen[$1]++ {sub(/^[^ ]* /, ""); print}' | ` +
	`while IFS= read -r root; do find "$root" -maxdepth 6 -type d \( ` + artifactNames + ` \) -prune -print 2>/dev/null; done | ` +
	`while IFS= read -r a; do p=${a%/*}; ` +
	`[ -n "$(find "$p" -maxdepth 4 \( ` + artifactNames + ` -o -name .git \) -prune -o -type f -mtime -60 -print -quit 2>/dev/null)" ] && continue; ` +
	`du -sk "$a" 2>/dev/null; done | sort -rn | head -40); ` +
	`if [ -n "$out" ]; then echo "$out"; else echo "0 stale build folders found (none in a project untouched for 60 days)"; fi`

const artifactNames = `-name node_modules -o -name target -o -name .venv -o -name venv -o -name .next -o -name .nuxt ` +
	`-o -name .turbo -o -name Pods -o -name .gradle -o -name DerivedData -o -name .tox -o -name .parcel-cache`

// probe runs a read-only command with a short time limit and returns what it
// printed on stdout. ok is false when it failed or ran out of time. Tests
// replace it.
var probe = func(name string, args ...string) (out string, ok bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	return stdout.String(), err == nil
}

// once caches a detection for the rest of the process: the catalogue is
// listed many times in a run and these commands take up to seconds.
func once(f func() bool) func() bool {
	var o sync.Once
	var v bool
	return func() bool {
		o.Do(func() { v = f() })
		return v
	}
}

// helmHasRepos reports whether Helm has any repository to update: without
// one `helm repo update` fails with "no repositories found".
func helmHasRepos() bool {
	out, ok := probe("helm", "repo", "list", "-o", "json")
	if !ok {
		return false // "Error: no repositories to show" in older versions
	}
	var repos []json.RawMessage
	return json.Unmarshal([]byte(out), &repos) == nil && len(repos) > 0
}

// pnpmHasGlobals reports whether pnpm has global packages to update. When
// pnpm cannot be asked the step stays, as it was before.
func pnpmHasGlobals() bool {
	out, ok := probe("pnpm", "ls", "-g", "--depth", "0", "--json")
	if !ok {
		return true
	}
	var dirs []struct {
		Dependencies map[string]json.RawMessage `json:"dependencies"`
	}
	if json.Unmarshal([]byte(out), &dirs) != nil {
		return true
	}
	for _, d := range dirs {
		if len(d.Dependencies) > 0 {
			return true
		}
	}
	return false
}

// uvHasTools reports whether uv has tools installed: `uv tool list` prints
// them on stdout and "No tools installed" on stderr.
func uvHasTools() bool {
	out, ok := probe("uv", "tool", "list")
	return !ok || strings.TrimSpace(out) != ""
}
