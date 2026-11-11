// Package steps is omega's catalogue of maintenance steps: the native
// equivalents of topgrade's updaters and Mole's cleaners, plus both tools as
// optional backends. Steps that only make sense on one operating system say
// so, and the catalogue a machine sees (All) holds only the ones for it.
package steps

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/lcajigasm/omega/internal/runner"
	"github.com/lcajigasm/omega/internal/safety"
)

// Categories.
const (
	Update  = "update"
	Clean   = "clean"
	Check   = "check"
	Backend = "backend"
)

// Step is a single maintenance action.
type Step struct {
	ID          string
	Name        string
	Category    string
	Description string
	Command     string
	Risk        safety.Risk
	Requires    string // binary that must be on PATH
	// AlwaysConfirm makes the user approve this step every time, even with --yes.
	AlwaysConfirm bool
	// Install is how to get the tool when it is missing, offered to the user.
	Install string
	Detect  func() bool // optional extra detection
	// OS is "darwin", "linux" or "windows" for a step that exists only there; empty for
	// a step that works wherever its tool is installed. Two steps may share
	// an id when each belongs to a different system (disk, trash).
	OS string
}

// Available reports whether the step can run on this machine.
func (s Step) Available() bool {
	if s.Requires != "" && !hasBin(s.Requires) {
		return false
	}
	return s.Detect == nil || s.Detect()
}

const (
	mac     = "darwin"
	linux   = "linux"
	windows = "windows"
)

// unixOnly are the steps without an operating system of their own whose
// command is written for a POSIX shell or whose tool does not exist on
// Windows. They are left out of the Windows catalogue.
var unixOnly = map[string]bool{
	"brew-outdated": true, "brew-doctor": true, "brew": true, "brew-cleanup": true, "brew-autoremove": true,
	"dev-artifacts": true, "shell-startup": true, "tmux-plugins": true, "omz": true, "asdf": true,
	"gradle-cache": true,
}

// Commands of the Windows steps, in PowerShell. They run in Windows
// PowerShell 5.1 as well as PowerShell 7, so no && and no ternaries.
const (
	winDisk = "Get-Volume | Where-Object { $_.DriveLetter } | Sort-Object DriveLetter | " +
		"Format-Table DriveLetter, FileSystemLabel, @{n='SizeGB';e={[math]::Round($_.Size/1GB,1)}}, @{n='FreeGB';e={[math]::Round($_.SizeRemaining/1GB,1)}} -AutoSize | Out-String -Width 120; " +
		"foreach ($d in @($env:TEMP, \"$env:LOCALAPPDATA\\Temp\", \"$env:SystemRoot\\Temp\", \"$env:SystemRoot\\SoftwareDistribution\\Download\") | Select-Object -Unique) { " +
		"if (Test-Path $d) { $s = (Get-ChildItem $d -Recurse -Force -ErrorAction SilentlyContinue | Measure-Object Length -Sum).Sum; '{0,8:N2} GB  {1}' -f ($s/1GB), $d } }"
	winLogs = "'== Errors in the last 7 days (count, source)'; " +
		"Get-WinEvent -FilterHashtable @{LogName='System','Application'; Level=1,2; StartTime=(Get-Date).AddDays(-7)} -MaxEvents 2000 -ErrorAction SilentlyContinue | " +
		"Group-Object ProviderName | Sort-Object Count -Descending | Select-Object -First 15 Count, Name | Format-Table -AutoSize | Out-String -Width 120; " +
		"'== Services set to start automatically that are not running'; " +
		"Get-CimInstance Win32_Service -Filter \"StartMode='Auto' AND State<>'Running'\" | Select-Object -First 15 Name, State, ExitCode | Format-Table -AutoSize | Out-String -Width 120; " +
		"'== Application crashes in the last 7 days'; " +
		"Get-WinEvent -FilterHashtable @{LogName='Application'; ProviderName='Application Error'; StartTime=(Get-Date).AddDays(-7)} -MaxEvents 200 -ErrorAction SilentlyContinue | " +
		"ForEach-Object { $_.Properties[0].Value } | Group-Object | Sort-Object Count -Descending | Select-Object -First 10 Count, Name | Format-Table -AutoSize | Out-String -Width 120"
	winUpdates = "$s = (New-Object -ComObject Microsoft.Update.Session).CreateUpdateSearcher(); " +
		"$r = $s.Search('IsInstalled=0 and IsHidden=0'); '{0} update(s) pending' -f $r.Updates.Count; $r.Updates | ForEach-Object { '  ' + $_.Title }"
	winHealth = "'== Last start'; (Get-CimInstance Win32_OperatingSystem).LastBootUpTime; " +
		"'== Restart pending'; " +
		"(Test-Path 'HKLM:\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Component Based Servicing\\RebootPending') -or " +
		"(Test-Path 'HKLM:\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\WindowsUpdate\\Auto Update\\RebootRequired'); " +
		"'== Disks'; Get-PhysicalDisk | Select-Object FriendlyName, MediaType, HealthStatus, OperationalStatus | Format-Table -AutoSize | Out-String -Width 120; " +
		"'== Memory (GB)'; Get-CimInstance Win32_OperatingSystem | " +
		"Select-Object @{n='Total';e={[math]::Round($_.TotalVisibleMemorySize/1MB,1)}}, @{n='Free';e={[math]::Round($_.FreePhysicalMemory/1MB,1)}} | Format-Table -AutoSize | Out-String -Width 120; " +
		"'== Battery'; Get-CimInstance Win32_Battery -ErrorAction SilentlyContinue | Select-Object EstimatedChargeRemaining, BatteryStatus | Format-Table -AutoSize | Out-String -Width 120"
	winSecurity = "'== Microsoft Defender'; Get-MpComputerStatus -ErrorAction SilentlyContinue | " +
		"Select-Object AntivirusEnabled, RealTimeProtectionEnabled, AntivirusSignatureLastUpdated | Format-List | Out-String -Width 120; " +
		"'== Firewall profiles'; Get-NetFirewallProfile -ErrorAction SilentlyContinue | Select-Object Name, Enabled | Format-Table -AutoSize | Out-String -Width 120; " +
		"'== Drive encryption (needs an elevated terminal)'; Get-BitLockerVolume -ErrorAction SilentlyContinue | " +
		"Select-Object MountPoint, VolumeStatus, ProtectionStatus | Format-Table -AutoSize | Out-String -Width 120; " +
		"'== User Account Control'; (Get-ItemProperty 'HKLM:\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Policies\\System' -ErrorAction SilentlyContinue).EnableLUA"
	winTemp = "Get-ChildItem $env:TEMP -Force -ErrorAction SilentlyContinue | Remove-Item -Recurse -Force -ErrorAction SilentlyContinue"
)

// Seams the tests replace.
var (
	hasBin    = runner.Has
	osRelease = func() string {
		data, _ := os.ReadFile("/etc/os-release")
		return string(data)
	}
)

// aptGet is apt-get for a run nobody watches: with stdin closed, a question
// about a changed configuration file would stop dpkg half-way and leave
// packages unconfigured. The options keep the file the machine has (or the
// package's default when it was never edited) instead of asking.
const aptGet = "sudo DEBIAN_FRONTEND=noninteractive apt-get -y -o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold"

// rollingSUSE reports openSUSE Tumbleweed and its relatives, which are
// upgraded with `zypper dup`; the other SUSE systems use `zypper update`.
func rollingSUSE() bool {
	rel := strings.ToLower(osRelease())
	return strings.Contains(rel, "tumbleweed") || strings.Contains(rel, "microos") || strings.Contains(rel, "slowroll")
}

func notRollingSUSE() bool { return !rollingSUSE() }

func dirExists(rel string) func() bool {
	return func() bool {
		home, _ := os.UserHomeDir()
		fi, err := os.Stat(filepath.Join(home, rel))
		return err == nil && fi.IsDir()
	}
}

// selfManaged reports whether a tool was installed by its own installer and
// not by Homebrew or the system's package manager: only then does its
// self-update command work.
func selfManaged(bin string) func() bool {
	return func() bool {
		p, err := exec.LookPath(bin)
		if err != nil {
			return false
		}
		if r, err := filepath.EvalSymlinks(p); err == nil {
			p = r
		}
		for _, managed := range []string{"/Cellar/", "/Caskroom/", "/homebrew/", "/.linuxbrew/", "/snap/"} {
			if strings.Contains(p, managed) {
				return false
			}
		}
		return !strings.HasPrefix(p, "/usr/bin/") && !strings.HasPrefix(p, "/bin/")
	}
}

// All is the step catalogue of this machine's operating system, in the order
// omega runs them.
var All = For(runtime.GOOS)

// For returns the catalogue of an operating system: the steps that work
// anywhere plus the ones of that system.
func For(goos string) []Step {
	var out []Step
	for _, s := range catalog {
		if s.OS != "" && s.OS != goos {
			continue
		}
		if goos == windows && s.OS == "" && unixOnly[s.ID] {
			continue
		}
		if goos == linux {
			if cmd, ok := linuxInstall[s.ID]; ok {
				s.Install = cmd
			}
		}
		out = append(out, s)
	}
	return out
}

// Categories is the order the categories are listed in.
var Categories = []string{Check, Update, Clean, Backend}

// Grouped returns the steps with each category together, in Categories
// order, keeping the catalogue's order inside a category. The catalogue
// itself is in running order, where a check can sit next to the cleanup it
// belongs to (docker-df before the Docker prunes).
func Grouped(list []Step) []Step {
	out := make([]Step, 0, len(list))
	for _, cat := range Categories {
		for _, s := range list {
			if s.Category == cat {
				out = append(out, s)
			}
		}
	}
	return out
}

// Known reports whether an id names a step on any operating system, so a
// configuration shared between machines can skip steps this one lacks.
func Known(id string) bool {
	for _, s := range catalog {
		if s.ID == id {
			return true
		}
	}
	return false
}

// linuxInstall replaces the Homebrew install commands of optional tools.
// Docker has none: its packages differ per distribution.
var linuxInstall = map[string]string{
	"go-bins":            "go install github.com/nao1215/gup@latest",
	"topgrade-preview":   "cargo install topgrade",
	"topgrade":           "cargo install topgrade",
	"docker-df":          "",
	"docker-start":       "",
	"docker-containers":  "",
	"docker-images":      "",
	"docker-images-all":  "",
	"docker-build-cache": "",
	"docker-volumes":     "",
	"docker":             "",
}

// catalog is every step omega knows, for every operating system.
var catalog = []Step{
	// Checks (read-only).
	{OS: mac, ID: "disk", Name: "Disk space", Category: Check, Command: "df -h / && du -sh ~/Library/Caches ~/Library/Logs ~/.Trash 2>/dev/null", Risk: safety.Safe,
		Description: "Free space and size of caches, logs and Trash"},
	{ID: "brew-outdated", Name: "Outdated Homebrew packages", Category: Check, Command: "brew outdated --verbose", Risk: safety.Safe, Requires: "brew"},
	{ID: "brew-doctor", Name: "brew doctor", Category: Check, Command: "brew doctor", Risk: safety.Safe, Requires: "brew"},
	{OS: mac, ID: "macos-check", Name: "macOS updates", Category: Check, Command: "softwareupdate --list", Risk: safety.Safe},

	{OS: linux, ID: "disk", Name: "Disk space", Category: Check, Risk: safety.Safe,
		Command:     "df -h / && du -sh ~/.cache ~/.local/share/Trash /var/cache /var/log 2>/dev/null; true",
		Description: "Free space and size of caches, logs and Trash"},
	{OS: linux, ID: "apt-outdated", Name: "Outdated apt packages", Category: Check, Command: "apt list --upgradable 2>/dev/null", Risk: safety.Safe, Requires: "apt-get",
		Description: "From the package lists as last refreshed; the apt step refreshes them"},
	{OS: linux, ID: "dnf-outdated", Name: "Outdated dnf packages", Category: Check, Command: "dnf check-update; test $? -ne 1", Risk: safety.Safe, Requires: "dnf"},
	{OS: linux, ID: "pacman-outdated", Name: "Outdated pacman packages", Category: Check, Command: "pacman -Qu; test $? -le 1", Risk: safety.Safe, Requires: "pacman",
		Description: "From the package database as last synchronised; the pacman step synchronises it"},
	{OS: linux, ID: "zypper-outdated", Name: "Outdated zypper packages", Category: Check, Command: "zypper --non-interactive list-updates", Risk: safety.Safe, Requires: "zypper"},
	{OS: linux, ID: "flatpak-outdated", Name: "Outdated Flatpak apps", Category: Check, Command: "flatpak remote-ls --updates", Risk: safety.Safe, Requires: "flatpak"},
	{OS: linux, ID: "snap-outdated", Name: "Outdated snaps", Category: Check, Command: "snap refresh --list", Risk: safety.Safe, Requires: "snap"},
	{OS: linux, ID: "journal-size", Name: "Journal size", Category: Check, Command: "journalctl --disk-usage", Risk: safety.Safe, Requires: "journalctl",
		Description: "Space the systemd journal takes"},
	{OS: linux, ID: "system-logs", Name: "System logs review", Category: Check, Risk: safety.Safe, Requires: "journalctl",
		Description: "Units that failed, what logs errors since boot, core dumps and log sizes",
		Command: `echo '== Failed system units'; systemctl --failed --no-pager --no-legend | head -15; ` +
			`echo '== Failed user units'; systemctl --user --failed --no-pager --no-legend 2>/dev/null | head -15; ` +
			`echo '== Errors since boot (count, source)'; journalctl -b -p err -q --no-pager -o short 2>/dev/null | awk '{print $5}' | sed 's/\[.*//; s/:$//' | sort | uniq -c | sort -rn | head -10; ` +
			`echo '== Core dumps, last 7 days'; coredumpctl list --since -7d --no-pager 2>/dev/null | tail -10; ` +
			`echo '== Journal and log sizes'; journalctl --disk-usage 2>/dev/null; du -sh /var/log 2>/dev/null; true`},

	{OS: mac, ID: "system-logs", Name: "System logs review", Category: Check, Risk: safety.Safe, Requires: "launchctl",
		Description: "Crash reports, launch agents that keep failing, sleep blockers, processes logging faults and log sizes",
		Command: `echo '== Crash reports, last 7 days (count, process)'; ` +
			`find ~/Library/Logs/DiagnosticReports /Library/Logs/DiagnosticReports -type f -mtime -7 2>/dev/null | sed -E 's#.*/##; s/[-_][0-9]{4}-.*//' | sort | uniq -c | sort -rn | head -10; ` +
			`echo '== Launch agents that are not running and last exited with an error'; launchctl list | awk 'NR>1 && $1 == "-" && $2 != "0"' | head -15; ` +
			`echo '== Sleep blockers'; pmset -g assertions | grep -E 'Prevent(User)?Idle(System|Display)Sleep|PreventSystemSleep' | grep -v ' 0$' | head -10; ` +
			`echo '== Processes logging faults, last 30 min'; /usr/bin/log show --last 30m --style compact --predicate 'messageType == fault' 2>/dev/null | awk 'NR>1 {print $4}' | sed 's/\[.*//' | sort | uniq -c | sort -rn | head -10; ` +
			`echo '== Log folder sizes'; du -sh ~/Library/Logs /Library/Logs /private/var/log 2>/dev/null`},

	{OS: linux, ID: "health", Name: "Hardware health", Category: Check, Risk: safety.Safe,
		Description: "Uptime and load, whether a restart is pending, memory and swap, inodes, disks and the battery",
		Command: `echo '== Uptime and load'; uptime; ` +
			`echo '== Restart pending (Debian and Ubuntu say so here)'; cat /var/run/reboot-required /var/run/reboot-required.pkgs 2>/dev/null; ` +
			`echo '== Kernels installed, then the one running (not in the list: restart)'; ls /usr/lib/modules 2>/dev/null; uname -r; ` +
			`echo '== Memory and swap'; free -h; ` +
			`echo '== Inodes'; df -i / /home 2>/dev/null; ` +
			`echo '== Disks'; lsblk -d -o NAME,MODEL,SIZE,ROTA,TRAN 2>/dev/null; ` +
			`echo '== Battery'; cat /sys/class/power_supply/BAT*/capacity /sys/class/power_supply/BAT*/status 2>/dev/null; true`},
	{OS: linux, ID: "security", Name: "Security settings", Category: Check, Risk: safety.Safe,
		Description: "The firewall, automatic security updates, SSH logins and disk encryption",
		Command: `echo '== Firewall (ufw, firewalld, nftables)'; systemctl is-active ufw firewalld nftables 2>/dev/null; ` +
			`echo '== Automatic updates (unattended-upgrades, apt-daily-upgrade, dnf-automatic)'; ` +
			`systemctl is-enabled unattended-upgrades.service apt-daily-upgrade.timer dnf-automatic.timer 2>/dev/null; ` +
			`echo '== SSH server (ssh, sshd)'; systemctl is-active ssh sshd 2>/dev/null; ` +
			`grep -hE '^[[:space:]]*(PermitRootLogin|PasswordAuthentication)' /etc/ssh/sshd_config /etc/ssh/sshd_config.d/*.conf 2>/dev/null; ` +
			`echo '== Encrypted volumes'; lsblk -o NAME,TYPE,FSTYPE 2>/dev/null | grep -i crypt; true`},
	{OS: linux, ID: "firmware-check", Name: "Firmware updates", Category: Check, Risk: safety.Safe, Requires: "fwupdmgr",
		Description: "What fwupd would update on this hardware, from the metadata as last refreshed",
		Command:     "fwupdmgr get-updates --no-reboot-check 2>&1; test $? -ne 1"},
	{OS: mac, ID: "health", Name: "Hardware health", Category: Check, Risk: safety.Safe,
		Description: "Uptime and load, the disk's SMART status, battery condition, memory pressure, swap and Time Machine",
		Command: `echo '== Uptime and load'; uptime; ` +
			`echo '== Disk (SMART)'; diskutil info / | grep -E 'SMART Status|Solid State' | sed 's/^ *//'; ` +
			`echo '== Battery'; system_profiler SPPowerDataType 2>/dev/null | grep -E 'Cycle Count|Condition|Maximum Capacity|State of Charge' | sed 's/^ *//'; ` +
			`echo '== Memory'; memory_pressure -Q 2>/dev/null | tail -1; sysctl -n vm.swapusage; ` +
			`echo '== Time Machine'; tmutil destinationinfo 2>&1 | grep -E 'Name|Kind|No destinations' | sed 's/^ *//'; ` +
			`echo "local snapshots: $(tmutil listlocalsnapshots / 2>/dev/null | grep -c com.apple)"`},
	{OS: mac, ID: "security", Name: "Security settings", Category: Check, Risk: safety.Safe,
		Description: "FileVault, the firewall, Gatekeeper, System Integrity Protection and automatic updates",
		Command: `echo '== FileVault'; diskutil apfs list 2>/dev/null | grep -m1 FileVault | sed 's/^ *//'; ` +
			`echo '== Firewall'; /usr/libexec/ApplicationFirewall/socketfilterfw --getglobalstate; ` +
			`echo '== Gatekeeper'; spctl --status; ` +
			`echo '== System Integrity Protection'; csrutil status; ` +
			`echo '== Automatic updates'; softwareupdate --schedule; ` +
			`for k in AutomaticDownload AutomaticallyInstallMacOSUpdates CriticalUpdateInstall; do ` +
			`echo "$k: $(defaults read /Library/Preferences/com.apple.SoftwareUpdate $k 2>/dev/null || echo unset)"; done`},
	{ID: "dev-artifacts", Name: "Stale project build folders", Category: Check, Risk: safety.Safe,
		Description: "node_modules, target, .venv, Pods and similar folders in projects not touched for 60 days, largest first (size in KB)",
		Command: `for d in ~/Dev ~/Developer ~/dev ~/Projects ~/projects ~/Code ~/code ~/src ~/work ~/Work ~/Sites ~/repos ~/git ~/Documents/GitHub; do [ -d "$d" ] && echo "$d"; done | ` +
			`while read -r root; do find "$root" -maxdepth 6 -type d \( -name node_modules -o -name target -o -name .venv -o -name venv -o -name .next -o -name .nuxt ` +
			`-o -name .turbo -o -name Pods -o -name .gradle -o -name DerivedData -o -name .tox -o -name .parcel-cache \) -prune -mtime +60 -print0 2>/dev/null; done | ` +
			`xargs -0 du -sk 2>/dev/null | sort -rn | head -40`},
	{ID: "shell-startup", Name: "Shell startup time", Category: Check, Risk: safety.Safe, Requires: "zsh",
		Description: "How long an interactive zsh takes to start, three times",
		Command:     `for i in 1 2 3; do /usr/bin/time -p zsh -i -c exit 2>&1 | grep real; done`},

	// Updates (topgrade equivalents).
	{ID: "brew", Name: "Homebrew", Category: Update, Command: "brew update && brew upgrade", Risk: safety.Normal, Requires: "brew",
		Description: "Upgrades formulae and casks"},
	{OS: mac, ID: "brew-greedy", Name: "Self-updating casks", Category: Update, Command: "brew upgrade --cask --greedy", Risk: safety.Dangerous, Requires: "brew",
		Description: "Also casks that update themselves (may quit running apps)"},
	{OS: mac, ID: "mas", Name: "Mac App Store", Category: Update, Command: "mas upgrade", Risk: safety.Normal, Requires: "mas", Install: "brew install mas"},
	{ID: "npm", Name: "npm global", Category: Update, Command: "npm update -g", Risk: safety.Normal, Requires: "npm"},
	{ID: "pnpm", Name: "pnpm global", Category: Update, Command: "pnpm update -g", Risk: safety.Normal, Requires: "pnpm"},
	{ID: "bun", Name: "Bun", Category: Update, Command: "bun upgrade", Risk: safety.Normal, Requires: "bun"},
	{ID: "pipx", Name: "pipx", Category: Update, Command: "pipx upgrade-all", Risk: safety.Normal, Requires: "pipx"},
	{ID: "uv", Name: "uv tools", Category: Update, Command: "uv tool upgrade --all", Risk: safety.Normal, Requires: "uv"},
	{ID: "rustup", Name: "Rust toolchains", Category: Update, Command: "rustup update", Risk: safety.Normal, Requires: "rustup"},
	{ID: "cargo", Name: "cargo binaries", Category: Update, Command: "cargo install-update -a", Risk: safety.Normal, Requires: "cargo-install-update", Install: "cargo install cargo-update"},
	{ID: "gh-ext", Name: "gh extensions", Category: Update, Command: "gh extension upgrade --all", Risk: safety.Normal, Requires: "gh"},
	{ID: "mise", Name: "mise tools", Category: Update, Command: "mise upgrade", Risk: safety.Normal, Requires: "mise",
		Description: "Upgrades the language versions and tools mise manages, within what each project pins"},
	{ID: "asdf", Name: "asdf plugins", Category: Update, Command: "asdf plugin update --all", Risk: safety.Normal, Requires: "asdf"},
	{ID: "gem", Name: "RubyGems", Category: Update, Command: "gem update", Risk: safety.Normal, Requires: "gem",
		Description: "Only for a Ruby you installed: the one macOS ships needs sudo and is left alone",
		Detect: func() bool {
			p, err := exec.LookPath("gem")
			return err == nil && !strings.HasPrefix(p, "/usr/bin/")
		}},
	{ID: "composer", Name: "Composer global packages", Category: Update, Command: "composer global update", Risk: safety.Normal, Requires: "composer",
		Detect: dirExists(".composer")},
	{ID: "deno", Name: "Deno", Category: Update, Command: "deno upgrade", Risk: safety.Normal, Requires: "deno", Detect: selfManaged("deno")},
	{ID: "poetry", Name: "Poetry", Category: Update, Command: "poetry self update", Risk: safety.Normal, Requires: "poetry", Detect: selfManaged("poetry")},
	{ID: "conda", Name: "conda", Category: Update, Command: "conda update -n base -y conda", Risk: safety.Normal, Requires: "conda"},
	{ID: "go-bins", Name: "Go binaries", Category: Update, Command: "gup update", Risk: safety.Normal, Requires: "gup", Install: "brew install gup",
		Description: "Binaries installed with go install"},
	{ID: "dotnet-tools", Name: ".NET global tools", Category: Update, Command: "dotnet tool update --global --all", Risk: safety.Normal, Requires: "dotnet"},
	{ID: "flutter", Name: "Flutter", Category: Update, Command: "flutter upgrade", Risk: safety.Normal, Requires: "flutter", Detect: selfManaged("flutter")},
	{ID: "vscode-ext", Name: "VS Code extensions", Category: Update, Command: "code --update-extensions", Risk: safety.Normal, Requires: "code"},
	{ID: "claude-code", Name: "Claude Code", Category: Update, Command: "claude update", Risk: safety.Normal, Requires: "claude", Detect: selfManaged("claude")},
	{ID: "tldr", Name: "tldr pages", Category: Update, Command: "tldr --update", Risk: safety.Normal, Requires: "tldr"},
	{ID: "helm-repos", Name: "Helm repositories", Category: Update, Command: "helm repo update", Risk: safety.Normal, Requires: "helm"},
	{ID: "gcloud", Name: "Google Cloud CLI components", Category: Update, Command: "gcloud components update --quiet", Risk: safety.Normal, Requires: "gcloud",
		Detect: selfManaged("gcloud")},
	{ID: "tmux-plugins", Name: "tmux plugins", Category: Update, Command: "~/.tmux/plugins/tpm/bin/update_plugins all", Risk: safety.Normal,
		Detect: dirExists(".tmux/plugins/tpm")},
	{ID: "nvim-plugins", Name: "Neovim plugins (lazy.nvim)", Category: Update, Command: `nvim --headless "+Lazy! sync" +qa`, Risk: safety.Normal, Requires: "nvim",
		Detect: dirExists(".local/share/nvim/lazy")},
	{ID: "omz", Name: "Oh My Zsh", Category: Update, Command: "sh ~/.oh-my-zsh/tools/upgrade.sh", Risk: safety.Dangerous, Detect: dirExists(".oh-my-zsh")},
	{OS: linux, ID: "apt", Name: "apt packages", Category: Update, Risk: safety.Dangerous, Requires: "apt-get",
		Command:     "sudo apt-get update && " + aptGet + " upgrade",
		Description: "Needs sudo. Refreshes the package lists and upgrades what is installed; never removes a package and keeps your edited configuration files"},
	{OS: linux, ID: "dnf", Name: "dnf packages", Category: Update, Command: "sudo dnf upgrade --refresh -y", Risk: safety.Dangerous, Requires: "dnf",
		Description: "Needs sudo; a new kernel takes effect after a restart"},
	{OS: linux, ID: "pacman", Name: "pacman packages", Category: Update, Command: "sudo pacman -Syu --noconfirm", Risk: safety.Dangerous, Requires: "pacman",
		Description: "Needs sudo. Full system upgrade; read the Arch news first when it has been a while"},
	{OS: linux, ID: "zypper", Name: "zypper packages", Category: Update, Risk: safety.Dangerous, Requires: "zypper", Detect: notRollingSUSE,
		Command:     "sudo zypper --non-interactive refresh && sudo zypper --non-interactive update",
		Description: "Needs sudo"},
	{OS: linux, ID: "zypper-dup", Name: "zypper distribution upgrade", Category: Update, Risk: safety.Dangerous, Requires: "zypper", Detect: rollingSUSE,
		Command:     "sudo zypper --non-interactive refresh && sudo zypper --non-interactive dist-upgrade",
		Description: "Needs sudo. How openSUSE Tumbleweed is kept up to date"},
	{OS: linux, ID: "flatpak", Name: "Flatpak apps", Category: Update, Command: "flatpak update -y --noninteractive", Risk: safety.Normal, Requires: "flatpak"},
	{OS: linux, ID: "snap", Name: "snaps", Category: Update, Command: "sudo snap refresh", Risk: safety.Dangerous, Requires: "snap",
		Description: "Needs sudo; snapd also refreshes on its own several times a day"},
	{OS: linux, ID: "firmware", Name: "Firmware (fwupd)", Category: Update, Risk: safety.Dangerous, Requires: "fwupdmgr",
		Command:     "fwupdmgr refresh --force && fwupdmgr update -y --no-reboot-check",
		Description: "Firmware for the devices fwupd supports; some updates are applied on the next restart"},
	{OS: mac, ID: "macos", Name: "macOS (softwareupdate)", Category: Update, Command: "sudo softwareupdate --install --all", Risk: safety.Dangerous,
		Description: "Needs sudo; may require a restart"},

	// Cleaning (Mole equivalents).
	{ID: "brew-cleanup", Name: "Homebrew cleanup", Category: Clean, Command: "brew cleanup --prune=all", Risk: safety.Dangerous, Requires: "brew",
		Description: "Removes old versions and the download cache"},
	{ID: "brew-autoremove", Name: "Orphaned Homebrew dependencies", Category: Clean, Command: "brew autoremove", Risk: safety.Dangerous, Requires: "brew"},
	{ID: "npm-cache", Name: "npm cache", Category: Clean, Command: "npm cache clean --force", Risk: safety.Dangerous, Requires: "npm"},
	{ID: "pnpm-store", Name: "pnpm store", Category: Clean, Command: "pnpm store prune", Risk: safety.Dangerous, Requires: "pnpm"},
	{ID: "pip-cache", Name: "pip cache", Category: Clean, Command: "pip3 cache purge", Risk: safety.Dangerous, Requires: "pip3"},
	{ID: "uv-cache", Name: "uv cache", Category: Clean, Command: "uv cache prune", Risk: safety.Dangerous, Requires: "uv"},
	{ID: "go-cache", Name: "Go build cache", Category: Clean, Command: "go clean -cache", Risk: safety.Dangerous, Requires: "go"},
	{ID: "go-modcache", Name: "Go module cache", Category: Clean, Command: "go clean -modcache", Risk: safety.Dangerous, Requires: "go",
		Description: "Every downloaded module; they are downloaded again on the next build"},
	{ID: "yarn-cache", Name: "Yarn cache", Category: Clean, Command: "yarn cache clean", Risk: safety.Dangerous, Requires: "yarn"},
	{ID: "bun-cache", Name: "Bun cache", Category: Clean, Command: "bun pm cache rm", Risk: safety.Dangerous, Requires: "bun"},
	{ID: "deno-cache", Name: "Deno cache", Category: Clean, Command: "deno clean", Risk: safety.Dangerous, Requires: "deno"},
	{ID: "cargo-cache", Name: "Cargo registry cache", Category: Clean, Command: "cargo cache --autoclean", Risk: safety.Dangerous, Requires: "cargo-cache",
		Install: "cargo install cargo-cache", Description: "Extracted crate sources and old git checkouts; the compressed crates stay"},
	{ID: "gem-cleanup", Name: "Old gem versions", Category: Clean, Command: "gem cleanup", Risk: safety.Dangerous, Requires: "gem",
		Detect: func() bool {
			p, err := exec.LookPath("gem")
			return err == nil && !strings.HasPrefix(p, "/usr/bin/")
		}},
	{ID: "composer-cache", Name: "Composer cache", Category: Clean, Command: "composer clear-cache", Risk: safety.Dangerous, Requires: "composer"},
	{OS: mac, ID: "cocoapods-cache", Name: "CocoaPods cache", Category: Clean, Command: "pod cache clean --all", Risk: safety.Dangerous, Requires: "pod"},
	{ID: "gradle-cache", Name: "Gradle caches", Category: Clean, Command: "rm -rf ~/.gradle/caches/* ~/.gradle/daemon/*", Risk: safety.Dangerous,
		Detect: dirExists(".gradle/caches"), Description: "Downloaded dependencies and build caches; the next build downloads them again"},
	{ID: "conda-clean", Name: "conda caches", Category: Clean, Command: "conda clean --all -y", Risk: safety.Dangerous, Requires: "conda"},
	{ID: "mise-prune", Name: "Unused mise versions", Category: Clean, Command: "mise prune -y", Risk: safety.Dangerous, Requires: "mise",
		Description: "Versions no mise config file refers to any more"},
	{ID: "nuget-cache", Name: "NuGet caches", Category: Clean, Command: "dotnet nuget locals all --clear", Risk: safety.Dangerous, Requires: "dotnet"},
	{ID: "flutter-cache", Name: "Dart pub cache", Category: Clean, Command: "flutter pub cache clean -f", Risk: safety.Dangerous, Requires: "flutter"},
	{ID: "pre-commit-gc", Name: "pre-commit environments", Category: Clean, Command: "pre-commit gc", Risk: safety.Dangerous, Requires: "pre-commit",
		Description: "Hook environments no repository uses any more"},
	{OS: mac, ID: "playwright-cache", Name: "Playwright browsers", Category: Clean, Command: "rm -rf ~/Library/Caches/ms-playwright/*", Risk: safety.Dangerous,
		Detect: dirExists("Library/Caches/ms-playwright"), Description: "Downloaded browsers; `npx playwright install` brings them back"},
	{OS: mac, ID: "ios-device-support", Name: "Xcode device support files", Category: Clean, Command: `rm -rf ~/Library/Developer/Xcode/iOS\ DeviceSupport/*`, Risk: safety.Dangerous,
		Detect: dirExists("Library/Developer/Xcode/iOS DeviceSupport"), Description: "Symbols copied from every device once connected; Xcode copies them again when needed"},
	{OS: mac, ID: "xcode-derived", Name: "Xcode DerivedData", Category: Clean, Command: "rm -rf ~/Library/Developer/Xcode/DerivedData/*", Risk: safety.Dangerous,
		Detect: dirExists("Library/Developer/Xcode/DerivedData")},
	{OS: mac, ID: "simulators", Name: "Unavailable simulators", Category: Clean, Command: "xcrun simctl delete unavailable", Risk: safety.Dangerous, Requires: "xcrun",
		Detect: dirExists("Library/Developer/CoreSimulator")},
	{OS: mac, ID: "docker-df", Name: "Docker disk usage", Category: Check, Risk: safety.Safe, Requires: "docker", Install: "brew install --cask docker",
		Command:     "docker system df && du -sh ~/Library/Containers/com.docker.docker/Data/vms/0/data/Docker.raw 2>/dev/null",
		Description: "Space used by images, containers, volumes and build cache, plus the Docker Desktop disk image"},
	{OS: mac, ID: "docker-start", Name: "Start Docker Desktop", Category: Check, Risk: safety.Dangerous, Requires: "docker", Install: "brew install --cask docker",
		Command:     "docker info >/dev/null 2>&1 || { open -ga Docker && for i in $(seq 45); do docker info >/dev/null 2>&1 && exit 0; sleep 2; done; exit 1; }",
		Description: "Needed before any Docker cleanup when the daemon is not running"},
	{ID: "docker-containers", Name: "Docker: stopped containers", Category: Clean, Risk: safety.Dangerous, Requires: "docker", Install: "brew install --cask docker", AlwaysConfirm: true,
		Command: "docker container prune -f"},
	{ID: "docker-images", Name: "Docker: dangling images", Category: Clean, Risk: safety.Dangerous, Requires: "docker", Install: "brew install --cask docker", AlwaysConfirm: true,
		Command: "docker image prune -f", Description: "Untagged layers left behind by rebuilds"},
	{ID: "docker-images-all", Name: "Docker: all unused images", Category: Clean, Risk: safety.Dangerous, Requires: "docker", Install: "brew install --cask docker", AlwaysConfirm: true,
		Command: "docker image prune -a -f", Description: "Every image no container uses; they will be pulled again when needed"},
	{ID: "docker-build-cache", Name: "Docker: build cache", Category: Clean, Risk: safety.Dangerous, Requires: "docker", Install: "brew install --cask docker", AlwaysConfirm: true,
		Command: "docker builder prune -a -f", Description: "Cached build layers; the next builds will be slower"},
	{ID: "docker-volumes", Name: "Docker: unused volumes", Category: Clean, Risk: safety.Dangerous, Requires: "docker", Install: "brew install --cask docker", AlwaysConfirm: true,
		Command: "docker volume prune -a -f", Description: "DELETES DATA: databases and files stored in volumes no container uses"},
	{ID: "docker", Name: "Docker: everything unused except volumes", Category: Clean, Risk: safety.Dangerous, Requires: "docker", Install: "brew install --cask docker", AlwaysConfirm: true,
		Command: "docker system prune -a -f", Description: "Stopped containers, unused networks, unused images and build cache"},
	{OS: linux, ID: "playwright-cache", Name: "Playwright browsers", Category: Clean, Command: "find ~/.cache/ms-playwright -mindepth 1 -delete", Risk: safety.Dangerous,
		Detect: dirExists(".cache/ms-playwright"), Description: "Downloaded browsers; `npx playwright install` brings them back"},
	{OS: linux, ID: "docker-df", Name: "Docker disk usage", Category: Check, Risk: safety.Safe, Requires: "docker", Command: "docker system df",
		Description: "Space used by images, containers, volumes and build cache"},
	{OS: linux, ID: "docker-start", Name: "Start Docker", Category: Check, Risk: safety.Dangerous, Requires: "docker",
		Command:     "docker info >/dev/null 2>&1 || sudo systemctl start docker",
		Description: "Needed before any Docker cleanup when the daemon is not running; needs sudo"},
	{OS: linux, ID: "apt-clean", Name: "apt package cache", Category: Clean, Command: "sudo apt-get clean", Risk: safety.Dangerous, Requires: "apt-get",
		Description: "Needs sudo. Downloaded .deb files; apt downloads them again when needed"},
	{OS: linux, ID: "apt-autoremove", Name: "Orphaned apt packages", Category: Clean, Command: aptGet + " autoremove", Risk: safety.Dangerous, Requires: "apt-get",
		Description: "Needs sudo. Packages installed as dependencies that nothing needs any more, old kernels included"},
	{OS: linux, ID: "dnf-clean", Name: "dnf cache", Category: Clean, Command: "sudo dnf clean all", Risk: safety.Dangerous, Requires: "dnf",
		Description: "Needs sudo. Downloaded packages and repository metadata"},
	{OS: linux, ID: "dnf-autoremove", Name: "Orphaned dnf packages", Category: Clean, Command: "sudo dnf autoremove -y", Risk: safety.Dangerous, Requires: "dnf",
		Description: "Needs sudo. Packages installed as dependencies that nothing needs any more"},
	{OS: linux, ID: "pacman-cache", Name: "pacman package cache", Category: Clean, Risk: safety.Dangerous, Requires: "pacman",
		Command: "if command -v paccache >/dev/null; then sudo paccache -rk2; else sudo pacman -Sc --noconfirm; fi",
		Description: "Needs sudo. With paccache (pacman-contrib) keeps the two newest versions of each package; without it, " +
			"removes the cached packages that are no longer installed"},
	{OS: linux, ID: "pacman-orphans", Name: "Orphaned pacman packages", Category: Clean, Risk: safety.Dangerous, Requires: "pacman",
		Command:     "pacman -Qdtq | xargs -r sudo pacman -Rns --noconfirm",
		Description: "Needs sudo. Packages installed as dependencies that nothing needs any more, with their configuration"},
	{OS: linux, ID: "zypper-clean", Name: "zypper cache", Category: Clean, Command: "sudo zypper --non-interactive clean --all", Risk: safety.Dangerous, Requires: "zypper",
		Description: "Needs sudo. Downloaded packages and repository metadata"},
	{OS: linux, ID: "flatpak-unused", Name: "Unused Flatpak runtimes", Category: Clean, Command: "flatpak uninstall --unused -y --noninteractive", Risk: safety.Dangerous, Requires: "flatpak",
		Description: "Runtimes no installed app uses any more"},
	{OS: linux, ID: "snap-old", Name: "Old snap revisions", Category: Clean, Risk: safety.Dangerous, Requires: "snap",
		Command:     `LANG=C snap list --all | awk '/disabled/{print $1, $3}' | while read -r name rev; do sudo snap remove "$name" --revision="$rev"; done`,
		Description: "Needs sudo. The earlier revisions snapd keeps of every snap after an update"},
	{OS: linux, ID: "journal-vacuum", Name: "Old journal entries", Category: Clean, Command: "sudo journalctl --vacuum-time=2weeks", Risk: safety.Dangerous, Requires: "journalctl",
		Description: "Needs sudo. Deletes systemd journal entries older than two weeks"},
	{OS: linux, ID: "thumbnail-cache", Name: "Thumbnail cache", Category: Clean, Command: "find ~/.cache/thumbnails -mindepth 1 -delete", Risk: safety.Dangerous,
		Detect: dirExists(".cache/thumbnails"), Description: "Previews the file manager made; it makes them again when a folder is opened"},
	{OS: linux, ID: "trash", Name: "Trash", Category: Clean, Command: "find ~/.local/share/Trash -mindepth 2 -delete", Risk: safety.Dangerous,
		Detect: dirExists(".local/share/Trash")},
	{OS: mac, ID: "trash", Name: "Trash", Category: Clean, Command: "rm -rf ~/.Trash/*", Risk: safety.Dangerous, Detect: dirExists(".Trash")},

	// Backends: the original tools, when installed.
	{ID: "topgrade-preview", Name: "topgrade (dry run)", Category: Backend, Command: "topgrade --dry-run --no-retry --skip-notify", Risk: safety.Safe, Requires: "topgrade", Install: "brew install topgrade"},
	{ID: "topgrade", Name: "topgrade", Category: Backend, Command: "topgrade --yes --no-retry --skip-notify", Risk: safety.Dangerous, Requires: "topgrade", Install: "brew install topgrade",
		Description: "Runs every topgrade step as configured in ~/.config/topgrade.toml"},
	{OS: mac, ID: "mole-clean-preview", Name: "Mole clean (dry run)", Category: Backend, Command: "mo clean --dry-run", Risk: safety.Safe, Requires: "mo", Install: "brew install mole"},
	{OS: mac, ID: "mole-clean", Name: "Mole clean", Category: Backend, Command: "mo clean", Risk: safety.Dangerous, Requires: "mo", Install: "brew install mole"},
	{OS: mac, ID: "mole-optimize-preview", Name: "Mole optimize (dry run)", Category: Backend, Command: "mo optimize --dry-run", Risk: safety.Safe, Requires: "mo", Install: "brew install mole"},
	{OS: mac, ID: "mole-optimize", Name: "Mole optimize", Category: Backend, Command: "mo optimize", Risk: safety.Dangerous, Requires: "mo", Install: "brew install mole"},
	{OS: mac, ID: "mole-update", Name: "Update Mole", Category: Backend, Command: "mo update", Risk: safety.Normal, Requires: "mo", Install: "brew install mole"},
	// ---- Windows ----
	{OS: windows, ID: "disk", Name: "Disk space", Category: Check, Command: winDisk, Risk: safety.Safe,
		Description: "Free space per drive and the size of the temporary folders"},
	{OS: windows, ID: "winget-outdated", Name: "Outdated winget packages", Category: Check, Risk: safety.Safe, Requires: "winget",
		Command: "winget upgrade --include-unknown --accept-source-agreements --disable-interactivity"},
	{OS: windows, ID: "scoop-outdated", Name: "Outdated Scoop apps", Category: Check, Command: "scoop status", Risk: safety.Safe, Requires: "scoop"},
	{OS: windows, ID: "choco-outdated", Name: "Outdated Chocolatey packages", Category: Check, Command: "choco outdated", Risk: safety.Safe, Requires: "choco"},
	{OS: windows, ID: "windows-update-check", Name: "Windows updates", Category: Check, Command: winUpdates, Risk: safety.Safe,
		Description: "Asks Windows Update what is pending; can take a minute. Installing them is left to Windows"},
	{OS: windows, ID: "system-logs", Name: "System logs review", Category: Check, Command: winLogs, Risk: safety.Safe,
		Description: "Errors by source, services that should be running and are not, and application crashes"},
	{OS: windows, ID: "health", Name: "Hardware health", Category: Check, Command: winHealth, Risk: safety.Safe,
		Description: "When Windows last started, whether a restart is pending, disk health, memory and battery"},
	{OS: windows, ID: "security", Name: "Security settings", Category: Check, Command: winSecurity, Risk: safety.Safe,
		Description: "Microsoft Defender, the firewall profiles, drive encryption and User Account Control"},
	{OS: windows, ID: "docker-df", Name: "Docker disk usage", Category: Check, Command: "docker system df", Risk: safety.Safe, Requires: "docker"},
	{OS: windows, ID: "winget", Name: "winget packages", Category: Update, Risk: safety.Dangerous, Requires: "winget",
		Command:     "winget upgrade --all --include-unknown --accept-source-agreements --accept-package-agreements --disable-interactivity",
		Description: "Installers may ask for administrator rights in their own window"},
	{OS: windows, ID: "scoop", Name: "Scoop apps", Category: Update, Command: "scoop update; scoop update *", Risk: safety.Normal, Requires: "scoop"},
	{OS: windows, ID: "choco", Name: "Chocolatey packages", Category: Update, Command: "choco upgrade all -y", Risk: safety.Dangerous, Requires: "choco",
		Description: "Needs a terminal opened as administrator"},
	{OS: windows, ID: "wsl", Name: "Windows Subsystem for Linux", Category: Update, Command: "wsl --update", Risk: safety.Dangerous, Requires: "wsl",
		Description: "Updates the WSL kernel and tools, not the distributions inside it"},
	{OS: windows, ID: "defender-signatures", Name: "Microsoft Defender definitions", Category: Update, Command: "Update-MpSignature", Risk: safety.Normal,
		Description: "Downloads the newest antivirus definitions"},
	{OS: windows, ID: "scoop-cleanup", Name: "Old Scoop versions and cache", Category: Clean, Command: "scoop cleanup *; scoop cache rm *", Risk: safety.Dangerous, Requires: "scoop"},
	{OS: windows, ID: "temp-files", Name: "Temporary files", Category: Clean, Command: winTemp, Risk: safety.Dangerous,
		Description: "Empties your temporary folder; files in use are skipped"},
	{OS: windows, ID: "gradle-cache", Name: "Gradle caches", Category: Clean, Risk: safety.Dangerous, Detect: dirExists(".gradle/caches"),
		Command: "Remove-Item -Recurse -Force \"$HOME\\.gradle\\caches\\*\", \"$HOME\\.gradle\\daemon\\*\" -ErrorAction SilentlyContinue"},
	{OS: windows, ID: "component-store", Name: "Windows component store", Category: Clean, Risk: safety.Dangerous, Requires: "dism",
		Command:     "Dism.exe /Online /Cleanup-Image /StartComponentCleanup",
		Description: "Removes superseded Windows components. Needs a terminal opened as administrator"},
	{OS: windows, ID: "recycle-bin", Name: "Recycle Bin", Category: Clean, Command: "Clear-RecycleBin -Force -ErrorAction SilentlyContinue", Risk: safety.Dangerous},
}

// Register adds a step to the catalogue.
func Register(s Step) {
	for i, existing := range catalog {
		if existing.ID == s.ID && (existing.OS == s.OS || existing.OS == "") {
			catalog[i] = s
			All = For(runtime.GOOS)
			return
		}
	}
	catalog = append(catalog, s)
	All = For(runtime.GOOS)
}

// Find returns a step of this machine's catalogue by ID.
func Find(id string) (Step, bool) {
	for _, s := range All {
		if s.ID == id {
			return s, true
		}
	}
	return Step{}, false
}
