// Package steps is omega's catalogue of maintenance steps: the native
// equivalents of topgrade's updaters and Mole's cleaners, plus both tools as
// optional backends.
package steps

import (
	"os"
	"path/filepath"

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
}

// Available reports whether the step can run on this machine.
func (s Step) Available() bool {
	if s.Requires != "" && !runner.Has(s.Requires) {
		return false
	}
	return s.Detect == nil || s.Detect()
}

func dirExists(rel string) func() bool {
	return func() bool {
		home, _ := os.UserHomeDir()
		fi, err := os.Stat(filepath.Join(home, rel))
		return err == nil && fi.IsDir()
	}
}

// All is the full step catalogue, in the order omega runs them.
var All = []Step{
	// Checks (read-only).
	{ID: "disk", Name: "Disk space", Category: Check, Command: "df -h / && du -sh ~/Library/Caches ~/Library/Logs ~/.Trash 2>/dev/null", Risk: safety.Safe,
		Description: "Free space and size of caches, logs and Trash"},
	{ID: "brew-outdated", Name: "Outdated Homebrew packages", Category: Check, Command: "brew outdated --verbose", Risk: safety.Safe, Requires: "brew"},
	{ID: "brew-doctor", Name: "brew doctor", Category: Check, Command: "brew doctor", Risk: safety.Safe, Requires: "brew"},
	{ID: "macos-check", Name: "macOS updates", Category: Check, Command: "softwareupdate --list", Risk: safety.Safe},

	{ID: "system-logs", Name: "System logs review", Category: Check, Risk: safety.Safe, Requires: "launchctl",
		Description: "Crash reports, launch agents that keep failing, sleep blockers, processes logging faults and log sizes",
		Command: `echo '== Crash reports, last 7 days (count, process)'; ` +
			`find ~/Library/Logs/DiagnosticReports /Library/Logs/DiagnosticReports -type f -mtime -7 2>/dev/null | sed -E 's#.*/##; s/[-_][0-9]{4}-.*//' | sort | uniq -c | sort -rn | head -10; ` +
			`echo '== Launch agents that are not running and last exited with an error'; launchctl list | awk 'NR>1 && $1 == "-" && $2 != "0"' | head -15; ` +
			`echo '== Sleep blockers'; pmset -g assertions | grep -E 'Prevent(User)?Idle(System|Display)Sleep|PreventSystemSleep' | grep -v ' 0$' | head -10; ` +
			`echo '== Processes logging faults, last 30 min'; /usr/bin/log show --last 30m --style compact --predicate 'messageType == fault' 2>/dev/null | awk 'NR>1 {print $4}' | sed 's/\[.*//' | sort | uniq -c | sort -rn | head -10; ` +
			`echo '== Log folder sizes'; du -sh ~/Library/Logs /Library/Logs /private/var/log 2>/dev/null`},

	// Updates (topgrade equivalents).
	{ID: "brew", Name: "Homebrew", Category: Update, Command: "brew update && brew upgrade", Risk: safety.Normal, Requires: "brew",
		Description: "Upgrades formulae and casks"},
	{ID: "brew-greedy", Name: "Self-updating casks", Category: Update, Command: "brew upgrade --cask --greedy", Risk: safety.Dangerous, Requires: "brew",
		Description: "Also casks that update themselves (may quit running apps)"},
	{ID: "mas", Name: "Mac App Store", Category: Update, Command: "mas upgrade", Risk: safety.Normal, Requires: "mas", Install: "brew install mas"},
	{ID: "npm", Name: "npm global", Category: Update, Command: "npm update -g", Risk: safety.Normal, Requires: "npm"},
	{ID: "pnpm", Name: "pnpm global", Category: Update, Command: "pnpm update -g", Risk: safety.Normal, Requires: "pnpm"},
	{ID: "bun", Name: "Bun", Category: Update, Command: "bun upgrade", Risk: safety.Normal, Requires: "bun"},
	{ID: "pipx", Name: "pipx", Category: Update, Command: "pipx upgrade-all", Risk: safety.Normal, Requires: "pipx"},
	{ID: "uv", Name: "uv tools", Category: Update, Command: "uv tool upgrade --all", Risk: safety.Normal, Requires: "uv"},
	{ID: "rustup", Name: "Rust toolchains", Category: Update, Command: "rustup update", Risk: safety.Normal, Requires: "rustup"},
	{ID: "cargo", Name: "cargo binaries", Category: Update, Command: "cargo install-update -a", Risk: safety.Normal, Requires: "cargo-install-update", Install: "cargo install cargo-update"},
	{ID: "gh-ext", Name: "gh extensions", Category: Update, Command: "gh extension upgrade --all", Risk: safety.Normal, Requires: "gh"},
	{ID: "omz", Name: "Oh My Zsh", Category: Update, Command: "sh ~/.oh-my-zsh/tools/upgrade.sh", Risk: safety.Dangerous, Detect: dirExists(".oh-my-zsh")},
	{ID: "macos", Name: "macOS (softwareupdate)", Category: Update, Command: "sudo softwareupdate --install --all", Risk: safety.Dangerous,
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
	{ID: "xcode-derived", Name: "Xcode DerivedData", Category: Clean, Command: "rm -rf ~/Library/Developer/Xcode/DerivedData/*", Risk: safety.Dangerous,
		Detect: dirExists("Library/Developer/Xcode/DerivedData")},
	{ID: "simulators", Name: "Unavailable simulators", Category: Clean, Command: "xcrun simctl delete unavailable", Risk: safety.Dangerous, Requires: "xcrun",
		Detect: dirExists("Library/Developer/CoreSimulator")},
	{ID: "docker-df", Name: "Docker disk usage", Category: Check, Risk: safety.Safe, Requires: "docker", Install: "brew install --cask docker",
		Command:     "docker system df && du -sh ~/Library/Containers/com.docker.docker/Data/vms/0/data/Docker.raw 2>/dev/null",
		Description: "Space used by images, containers, volumes and build cache, plus the Docker Desktop disk image"},
	{ID: "docker-start", Name: "Start Docker Desktop", Category: Check, Risk: safety.Dangerous, Requires: "docker", Install: "brew install --cask docker",
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
	{ID: "trash", Name: "Trash", Category: Clean, Command: "rm -rf ~/.Trash/*", Risk: safety.Dangerous, Detect: dirExists(".Trash")},

	// Backends: the original tools, when installed.
	{ID: "topgrade-preview", Name: "topgrade (dry run)", Category: Backend, Command: "topgrade --dry-run --no-retry --skip-notify", Risk: safety.Safe, Requires: "topgrade", Install: "brew install topgrade"},
	{ID: "topgrade", Name: "topgrade", Category: Backend, Command: "topgrade --yes --no-retry --skip-notify", Risk: safety.Dangerous, Requires: "topgrade", Install: "brew install topgrade",
		Description: "Runs every topgrade step as configured in ~/.config/topgrade.toml"},
	{ID: "mole-clean-preview", Name: "Mole clean (dry run)", Category: Backend, Command: "mo clean --dry-run", Risk: safety.Safe, Requires: "mo", Install: "brew install mole"},
	{ID: "mole-clean", Name: "Mole clean", Category: Backend, Command: "mo clean", Risk: safety.Dangerous, Requires: "mo", Install: "brew install mole"},
	{ID: "mole-optimize-preview", Name: "Mole optimize (dry run)", Category: Backend, Command: "mo optimize --dry-run", Risk: safety.Safe, Requires: "mo", Install: "brew install mole"},
	{ID: "mole-optimize", Name: "Mole optimize", Category: Backend, Command: "mo optimize", Risk: safety.Dangerous, Requires: "mo", Install: "brew install mole"},
	{ID: "mole-update", Name: "Update Mole", Category: Backend, Command: "mo update", Risk: safety.Normal, Requires: "mo", Install: "brew install mole"},
}

// Find returns a step by ID.
func Find(id string) (Step, bool) {
	for _, s := range All {
		if s.ID == id {
			return s, true
		}
	}
	return Step{}, false
}
