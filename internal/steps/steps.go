// Package steps is omega's catalogue of maintenance steps: the native
// equivalents of topgrade's updaters and Mole's cleaners, plus both tools as
// optional backends.
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
}

// Available reports whether the step can run on this machine.
func (s Step) Available() bool {
	if s.Requires != "" && !runner.Has(s.Requires) {
		return false
	}
	return s.Detect == nil || s.Detect()
}

func darwinOnly() bool { return runtime.GOOS == "darwin" }

func dirExists(rel string) func() bool {
	return func() bool {
		home, _ := os.UserHomeDir()
		fi, err := os.Stat(filepath.Join(home, rel))
		return err == nil && fi.IsDir()
	}
}

// selfManaged reports whether a tool was installed by its own installer and
// not by Homebrew: only then does its self-update command work.
func selfManaged(bin string) func() bool {
	return func() bool {
		p, err := exec.LookPath(bin)
		if err != nil {
			return false
		}
		if r, err := filepath.EvalSymlinks(p); err == nil {
			p = r
		}
		return !strings.Contains(p, "/Cellar/") && !strings.Contains(p, "/Caskroom/") && !strings.Contains(p, "/homebrew/")
	}
}

// All is the full step catalogue, in the order omega runs them.
var All = []Step{
	// Checks (read-only).
	{ID: "disk", Name: "Disk space", Category: Check, Command: "df -h / && du -sh ~/Library/Caches ~/Library/Logs ~/.Trash 2>/dev/null", Risk: safety.Safe,
		Description: "Free space and size of caches, logs and Trash"},
	{ID: "brew-outdated", Name: "Outdated Homebrew packages", Category: Check, Command: "brew outdated --verbose", Risk: safety.Safe, Requires: "brew"},
	{ID: "brew-doctor", Name: "brew doctor", Category: Check, Command: "brew doctor", Risk: safety.Safe, Requires: "brew"},
	{ID: "macos-check", Name: "macOS updates", Category: Check, Command: "softwareupdate --list", Risk: safety.Safe, Detect: darwinOnly},

	{ID: "system-logs", Name: "System logs review", Category: Check, Risk: safety.Safe, Requires: "launchctl",
		Description: "Crash reports, launch agents that keep failing, sleep blockers, processes logging faults and log sizes",
		Command: `echo '== Crash reports, last 7 days (count, process)'; ` +
			`find ~/Library/Logs/DiagnosticReports /Library/Logs/DiagnosticReports -type f -mtime -7 2>/dev/null | sed -E 's#.*/##; s/[-_][0-9]{4}-.*//' | sort | uniq -c | sort -rn | head -10; ` +
			`echo '== Launch agents that are not running and last exited with an error'; launchctl list | awk 'NR>1 && $1 == "-" && $2 != "0"' | head -15; ` +
			`echo '== Sleep blockers'; pmset -g assertions | grep -E 'Prevent(User)?Idle(System|Display)Sleep|PreventSystemSleep' | grep -v ' 0$' | head -10; ` +
			`echo '== Processes logging faults, last 30 min'; /usr/bin/log show --last 30m --style compact --predicate 'messageType == fault' 2>/dev/null | awk 'NR>1 {print $4}' | sed 's/\[.*//' | sort | uniq -c | sort -rn | head -10; ` +
			`echo '== Log folder sizes'; du -sh ~/Library/Logs /Library/Logs /private/var/log 2>/dev/null`},

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
	{ID: "macos", Name: "macOS (softwareupdate)", Category: Update, Command: "sudo softwareupdate --install --all", Risk: safety.Dangerous,
		Description: "Needs sudo; may require a restart", Detect: darwinOnly},

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
	{ID: "cocoapods-cache", Name: "CocoaPods cache", Category: Clean, Command: "pod cache clean --all", Risk: safety.Dangerous, Requires: "pod"},
	{ID: "gradle-cache", Name: "Gradle caches", Category: Clean, Command: "rm -rf ~/.gradle/caches/* ~/.gradle/daemon/*", Risk: safety.Dangerous,
		Detect: dirExists(".gradle/caches"), Description: "Downloaded dependencies and build caches; the next build downloads them again"},
	{ID: "conda-clean", Name: "conda caches", Category: Clean, Command: "conda clean --all -y", Risk: safety.Dangerous, Requires: "conda"},
	{ID: "mise-prune", Name: "Unused mise versions", Category: Clean, Command: "mise prune -y", Risk: safety.Dangerous, Requires: "mise",
		Description: "Versions no mise config file refers to any more"},
	{ID: "nuget-cache", Name: "NuGet caches", Category: Clean, Command: "dotnet nuget locals all --clear", Risk: safety.Dangerous, Requires: "dotnet"},
	{ID: "flutter-cache", Name: "Dart pub cache", Category: Clean, Command: "flutter pub cache clean -f", Risk: safety.Dangerous, Requires: "flutter"},
	{ID: "pre-commit-gc", Name: "pre-commit environments", Category: Clean, Command: "pre-commit gc", Risk: safety.Dangerous, Requires: "pre-commit",
		Description: "Hook environments no repository uses any more"},
	{ID: "playwright-cache", Name: "Playwright browsers", Category: Clean, Command: "rm -rf ~/Library/Caches/ms-playwright/*", Risk: safety.Dangerous,
		Detect: dirExists("Library/Caches/ms-playwright"), Description: "Downloaded browsers; `npx playwright install` brings them back"},
	{ID: "ios-device-support", Name: "Xcode device support files", Category: Clean, Command: `rm -rf ~/Library/Developer/Xcode/iOS\ DeviceSupport/*`, Risk: safety.Dangerous,
		Detect: dirExists("Library/Developer/Xcode/iOS DeviceSupport"), Description: "Symbols copied from every device once connected; Xcode copies them again when needed"},
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
