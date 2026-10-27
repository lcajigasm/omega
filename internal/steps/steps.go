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
	Requires    string      // binary that must be on PATH
	Detect      func() bool // optional extra detection
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

	// Updates (topgrade equivalents).
	{ID: "brew", Name: "Homebrew", Category: Update, Command: "brew update && brew upgrade", Risk: safety.Normal, Requires: "brew",
		Description: "Upgrades formulae and casks"},
	{ID: "brew-greedy", Name: "Self-updating casks", Category: Update, Command: "brew upgrade --cask --greedy", Risk: safety.Dangerous, Requires: "brew",
		Description: "Also casks that update themselves (may quit running apps)"},
	{ID: "mas", Name: "Mac App Store", Category: Update, Command: "mas upgrade", Risk: safety.Normal, Requires: "mas"},
	{ID: "npm", Name: "npm global", Category: Update, Command: "npm update -g", Risk: safety.Normal, Requires: "npm"},
	{ID: "pnpm", Name: "pnpm global", Category: Update, Command: "pnpm update -g", Risk: safety.Normal, Requires: "pnpm"},
	{ID: "bun", Name: "Bun", Category: Update, Command: "bun upgrade", Risk: safety.Normal, Requires: "bun"},
	{ID: "pipx", Name: "pipx", Category: Update, Command: "pipx upgrade-all", Risk: safety.Normal, Requires: "pipx"},
	{ID: "uv", Name: "uv tools", Category: Update, Command: "uv tool upgrade --all", Risk: safety.Normal, Requires: "uv"},
	{ID: "rustup", Name: "Rust toolchains", Category: Update, Command: "rustup update", Risk: safety.Normal, Requires: "rustup"},
	{ID: "cargo", Name: "cargo binaries", Category: Update, Command: "cargo install-update -a", Risk: safety.Normal, Requires: "cargo-install-update"},
	{ID: "gh-ext", Name: "gh extensions", Category: Update, Command: "gh extension upgrade --all", Risk: safety.Normal, Requires: "gh"},
	{ID: "omz", Name: "Oh My Zsh", Category: Update, Command: "sh ~/.oh-my-zsh/tools/upgrade.sh", Risk: safety.Dangerous, Detect: dirExists(".oh-my-zsh")},
	{ID: "macos", Name: "macOS (softwareupdate)", Category: Update, Command: "sudo -n softwareupdate --install --all", Risk: safety.Dangerous,
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
	{ID: "docker", Name: "Docker (unused images and stopped containers)", Category: Clean, Command: "docker system prune -f", Risk: safety.Dangerous, Requires: "docker"},
	{ID: "trash", Name: "Trash", Category: Clean, Command: "rm -rf ~/.Trash/*", Risk: safety.Dangerous, Detect: dirExists(".Trash")},

	// Backends: the original tools, when installed.
	{ID: "topgrade-preview", Name: "topgrade (dry run)", Category: Backend, Command: "topgrade --dry-run --no-retry --skip-notify", Risk: safety.Safe, Requires: "topgrade"},
	{ID: "topgrade", Name: "topgrade", Category: Backend, Command: "topgrade --yes --no-retry --skip-notify", Risk: safety.Dangerous, Requires: "topgrade",
		Description: "Runs every topgrade step as configured in ~/.config/topgrade.toml"},
	{ID: "mole-clean-preview", Name: "Mole clean (dry run)", Category: Backend, Command: "mo clean --dry-run", Risk: safety.Safe, Requires: "mo"},
	{ID: "mole-clean", Name: "Mole clean", Category: Backend, Command: "mo clean", Risk: safety.Dangerous, Requires: "mo"},
	{ID: "mole-optimize-preview", Name: "Mole optimize (dry run)", Category: Backend, Command: "mo optimize --dry-run", Risk: safety.Safe, Requires: "mo"},
	{ID: "mole-optimize", Name: "Mole optimize", Category: Backend, Command: "mo optimize", Risk: safety.Dangerous, Requires: "mo"},
	{ID: "mole-update", Name: "Update Mole", Category: Backend, Command: "mo update", Risk: safety.Normal, Requires: "mo"},
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
