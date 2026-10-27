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
	{ID: "disk", Name: "Espacio en disco", Category: Check, Command: "df -h / && du -sh ~/Library/Caches ~/Library/Logs ~/.Trash 2>/dev/null", Risk: safety.Safe,
		Description: "Espacio libre y tamaño de cachés, logs y papelera"},
	{ID: "brew-outdated", Name: "Paquetes Homebrew desactualizados", Category: Check, Command: "brew outdated --verbose", Risk: safety.Safe, Requires: "brew"},
	{ID: "brew-doctor", Name: "brew doctor", Category: Check, Command: "brew doctor", Risk: safety.Safe, Requires: "brew"},
	{ID: "macos-check", Name: "Actualizaciones de macOS", Category: Check, Command: "softwareupdate --list", Risk: safety.Safe},

	// Updates (topgrade equivalents).
	{ID: "brew", Name: "Homebrew", Category: Update, Command: "brew update && brew upgrade", Risk: safety.Normal, Requires: "brew",
		Description: "Actualiza fórmulas y casks"},
	{ID: "brew-greedy", Name: "Casks con auto-update", Category: Update, Command: "brew upgrade --cask --greedy", Risk: safety.Dangerous, Requires: "brew",
		Description: "También casks que se actualizan solos (puede cerrar apps abiertas)"},
	{ID: "mas", Name: "Mac App Store", Category: Update, Command: "mas upgrade", Risk: safety.Normal, Requires: "mas"},
	{ID: "npm", Name: "npm global", Category: Update, Command: "npm update -g", Risk: safety.Normal, Requires: "npm"},
	{ID: "pnpm", Name: "pnpm global", Category: Update, Command: "pnpm update -g", Risk: safety.Normal, Requires: "pnpm"},
	{ID: "bun", Name: "Bun", Category: Update, Command: "bun upgrade", Risk: safety.Normal, Requires: "bun"},
	{ID: "pipx", Name: "pipx", Category: Update, Command: "pipx upgrade-all", Risk: safety.Normal, Requires: "pipx"},
	{ID: "uv", Name: "uv tools", Category: Update, Command: "uv tool upgrade --all", Risk: safety.Normal, Requires: "uv"},
	{ID: "rustup", Name: "Rust toolchains", Category: Update, Command: "rustup update", Risk: safety.Normal, Requires: "rustup"},
	{ID: "cargo", Name: "Binarios de cargo", Category: Update, Command: "cargo install-update -a", Risk: safety.Normal, Requires: "cargo-install-update"},
	{ID: "gh-ext", Name: "Extensiones de gh", Category: Update, Command: "gh extension upgrade --all", Risk: safety.Normal, Requires: "gh"},
	{ID: "omz", Name: "Oh My Zsh", Category: Update, Command: "sh ~/.oh-my-zsh/tools/upgrade.sh", Risk: safety.Dangerous, Detect: dirExists(".oh-my-zsh")},
	{ID: "macos", Name: "macOS (softwareupdate)", Category: Update, Command: "sudo -n softwareupdate --install --all", Risk: safety.Dangerous,
		Description: "Requiere sudo; puede pedir reinicio"},

	// Cleaning (Mole equivalents).
	{ID: "brew-cleanup", Name: "Limpieza de Homebrew", Category: Clean, Command: "brew cleanup --prune=all", Risk: safety.Dangerous, Requires: "brew",
		Description: "Borra versiones antiguas y la caché de descargas"},
	{ID: "brew-autoremove", Name: "Dependencias huérfanas de Homebrew", Category: Clean, Command: "brew autoremove", Risk: safety.Dangerous, Requires: "brew"},
	{ID: "npm-cache", Name: "Caché de npm", Category: Clean, Command: "npm cache clean --force", Risk: safety.Dangerous, Requires: "npm"},
	{ID: "pnpm-store", Name: "Store de pnpm", Category: Clean, Command: "pnpm store prune", Risk: safety.Dangerous, Requires: "pnpm"},
	{ID: "pip-cache", Name: "Caché de pip", Category: Clean, Command: "pip3 cache purge", Risk: safety.Dangerous, Requires: "pip3"},
	{ID: "uv-cache", Name: "Caché de uv", Category: Clean, Command: "uv cache prune", Risk: safety.Dangerous, Requires: "uv"},
	{ID: "go-cache", Name: "Caché de compilación de Go", Category: Clean, Command: "go clean -cache", Risk: safety.Dangerous, Requires: "go"},
	{ID: "xcode-derived", Name: "Xcode DerivedData", Category: Clean, Command: "rm -rf ~/Library/Developer/Xcode/DerivedData/*", Risk: safety.Dangerous,
		Detect: dirExists("Library/Developer/Xcode/DerivedData")},
	{ID: "simulators", Name: "Simuladores no disponibles", Category: Clean, Command: "xcrun simctl delete unavailable", Risk: safety.Dangerous, Requires: "xcrun",
		Detect: dirExists("Library/Developer/CoreSimulator")},
	{ID: "docker", Name: "Docker (imágenes y contenedores parados)", Category: Clean, Command: "docker system prune -f", Risk: safety.Dangerous, Requires: "docker"},
	{ID: "trash", Name: "Papelera", Category: Clean, Command: "rm -rf ~/.Trash/*", Risk: safety.Dangerous, Detect: dirExists(".Trash")},

	// Backends: the original tools, when installed.
	{ID: "topgrade-preview", Name: "topgrade (simulación)", Category: Backend, Command: "topgrade --dry-run --no-retry --skip-notify", Risk: safety.Safe, Requires: "topgrade"},
	{ID: "topgrade", Name: "topgrade", Category: Backend, Command: "topgrade --yes --no-retry --skip-notify", Risk: safety.Dangerous, Requires: "topgrade",
		Description: "Ejecuta todos los pasos de topgrade según ~/.config/topgrade.toml"},
	{ID: "mole-clean-preview", Name: "Mole clean (simulación)", Category: Backend, Command: "mo clean --dry-run", Risk: safety.Safe, Requires: "mo"},
	{ID: "mole-clean", Name: "Mole clean", Category: Backend, Command: "mo clean", Risk: safety.Dangerous, Requires: "mo"},
	{ID: "mole-optimize-preview", Name: "Mole optimize (simulación)", Category: Backend, Command: "mo optimize --dry-run", Risk: safety.Safe, Requires: "mo"},
	{ID: "mole-optimize", Name: "Mole optimize", Category: Backend, Command: "mo optimize", Risk: safety.Dangerous, Requires: "mo"},
	{ID: "mole-update", Name: "Actualizar Mole", Category: Backend, Command: "mo update", Risk: safety.Normal, Requires: "mo"},
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
