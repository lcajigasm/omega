package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/lcajigasm/omega/internal/runner"
)

// hint points the model at a tool's configuration and its authoritative
// documentation. Hints stay short on purpose: the model reads the docs on
// demand with fetch_url and web_search instead of omega shipping advice that
// goes stale.
type hint struct {
	Tool   string
	Bin    string   // binary that must be installed for the hint to apply
	OS     string   // "darwin" for a hint that only applies there
	Config []string // where the configuration lives
	Check  []string // read-only commands that show the current settings
	Ideas  string   // what is usually worth tuning
	Docs   []string // official references
}

var hints = []hint{
	{Tool: "topgrade", Bin: "topgrade",
		Config: []string{"~/.config/topgrade.toml", "~/.config/topgrade.d/"},
		Check:  []string{"topgrade --config-reference", "topgrade --dry-run --no-retry"},
		Ideas:  "disable steps that are not installed or not wanted, [misc] cleanup, [brew] autoremove/greedy_cask, pre_sudo, run steps the user runs by hand in [commands]",
		Docs:   []string{"https://github.com/topgrade-rs/topgrade/blob/main/config.example.toml"}},
	{Tool: "Mole", Bin: "mo",
		Config: []string{"~/.config/mole/"},
		Check:  []string{"mo --help", "mo clean --dry-run", "mo optimize --dry-run"},
		Ideas:  "whitelists for caches that must survive (mo clean --whitelist is interactive: edit the files instead), what optimize touches",
		Docs:   []string{"https://github.com/tw93/Mole#readme"}},
	{Tool: "Homebrew", Bin: "brew",
		Config: []string{"~/.zshrc or ~/.zprofile (HOMEBREW_* variables)", "~/.homebrew/brew.env", "$(brew --prefix)/etc/homebrew/brew.env"},
		Check:  []string{"brew config", "env | grep ^HOMEBREW_"},
		Ideas:  "HOMEBREW_CLEANUP_MAX_AGE_DAYS, HOMEBREW_CLEANUP_PERIODIC_FULL_DAYS, HOMEBREW_NO_INSTALL_CLEANUP (should be unset), HOMEBREW_AUTO_UPDATE_SECS, brew autoremove after upgrades",
		Docs:   []string{"https://docs.brew.sh/Manpage#environment"}},
	{Tool: "npm", Bin: "npm",
		Config: []string{"~/.npmrc"},
		Check:  []string{"npm config list", "npm config get cache"},
		Ideas:  "global prefix outside system dirs, fund/update-notifier noise, cache location",
		Docs:   []string{"https://docs.npmjs.com/cli/configuring-npm/npmrc"}},
	{Tool: "pnpm", Bin: "pnpm",
		Config: []string{"~/Library/Preferences/pnpm/rc", "~/.npmrc"},
		Check:  []string{"pnpm config list", "pnpm store path"},
		Ideas:  "store location on the same volume as projects, store prune schedule",
		Docs:   []string{"https://pnpm.io/settings"}},
	{Tool: "pip and pipx", Bin: "pipx",
		Config: []string{"~/.config/pip/pip.conf", "~/Library/Application Support/pip/pip.conf"},
		Check:  []string{"pip3 config list", "pipx environment"},
		Ideas:  "pipx default Python pinned to a Homebrew python that gets replaced (PIPX_DEFAULT_PYTHON), require-virtualenv",
		Docs:   []string{"https://pipx.pypa.io/stable/docs/", "https://pip.pypa.io/en/stable/topics/configuration/"}},
	{Tool: "uv", Bin: "uv",
		Config: []string{"~/.config/uv/uv.toml"},
		Check:  []string{"uv --version", "uv cache dir", "uv python list --only-installed"},
		Ideas:  "cache-dir, python-preference, old managed Pythons that can be removed",
		Docs:   []string{"https://docs.astral.sh/uv/reference/settings/"}},
	{Tool: "git", Bin: "git",
		Config: []string{"~/.gitconfig", "~/.config/git/config"},
		Check:  []string{"git config --global --list"},
		Ideas:  "fetch.prune, rerere, core.fsmonitor and core.untrackedCache for big repos, maintenance (git maintenance start), credential helper osxkeychain",
		Docs:   []string{"https://git-scm.com/docs/git-config", "https://git-scm.com/docs/git-maintenance"}},
	{Tool: "Docker Desktop", Bin: "docker",
		Config: []string{"~/Library/Group Containers/group.com.docker/settings-store.json", "~/.docker/daemon.json"},
		Check:  []string{"docker info", "docker system df"},
		Ideas:  "disk image size limit, builder garbage collection (builder.gc in daemon.json), resource saver, VirtioFS",
		Docs:   []string{"https://docs.docker.com/desktop/settings-and-maintenance/settings/", "https://docs.docker.com/build/cache/garbage-collection/"}},
	{Tool: "zsh startup", Bin: "zsh",
		Config: []string{"~/.zshrc", "~/.zprofile", "~/.zshenv"},
		Check:  []string{"time zsh -i -c exit"},
		Ideas:  "slow startup (nvm, conda, compinit without cache), duplicated PATH entries, plugins that are not used",
		Docs:   []string{"https://zsh.sourceforge.io/Doc/Release/Options.html"}},
	{Tool: "Oh My Zsh", Bin: "zsh",
		Config: []string{"~/.zshrc (plugins=, zstyle ':omz:update')"},
		Check:  []string{"grep -n 'plugins=\\|omz:update' ~/.zshrc"},
		Ideas:  "update mode (auto/reminder), plugin count",
		Docs:   []string{"https://github.com/ohmyzsh/ohmyzsh/wiki/Settings"}},
	{Tool: "Mac App Store (mas)", Bin: "mas",
		Config: []string{},
		Check:  []string{"mas outdated"},
		Ideas:  "apps that also self update and get updated twice",
		Docs:   []string{"https://github.com/mas-cli/mas#readme"}},
	{Tool: "Rust", Bin: "rustup",
		Config: []string{"~/.rustup/settings.toml", "~/.cargo/config.toml"},
		Check:  []string{"rustup show", "rustup toolchain list"},
		Ideas:  "old toolchains and targets that can be removed, cargo-cache or cargo-sweep for target dirs",
		Docs:   []string{"https://rust-lang.github.io/rustup/", "https://doc.rust-lang.org/cargo/reference/config.html"}},
	{Tool: "Go", Bin: "go",
		Config: []string{"$(go env GOENV)"},
		Check:  []string{"go env GOPATH GOMODCACHE GOCACHE GOFLAGS"},
		Ideas:  "module and build cache size, GOFLAGS=-modcacherw so the module cache can be deleted",
		Docs:   []string{"https://pkg.go.dev/cmd/go#hdr-Environment_variables"}},
	{Tool: "apt", Bin: "apt-get",
		Config: []string{"/etc/apt/apt.conf.d/", "/etc/apt/sources.list.d/", "/etc/apt/apt.conf.d/50unattended-upgrades"},
		Check:  []string{"apt-config dump | grep -i -E 'periodic|unattended'", "apt-mark showhold", "ls /etc/apt/sources.list.d"},
		Ideas:  "unattended security upgrades, held packages that block upgrades, third-party repositories that fail to refresh, how many old kernels autoremove keeps",
		Docs:   []string{"https://wiki.debian.org/UnattendedUpgrades", "https://manpages.debian.org/stable/apt/apt.conf.5.en.html"}},
	{Tool: "dnf", Bin: "dnf",
		Config: []string{"/etc/dnf/dnf.conf", "/etc/yum.repos.d/"},
		Check:  []string{"cat /etc/dnf/dnf.conf", "dnf repolist"},
		Ideas:  "max_parallel_downloads, installonly_limit (kernels kept), repositories that fail to refresh, dnf-automatic for unattended updates",
		Docs:   []string{"https://dnf.readthedocs.io/en/latest/conf_ref.html"}},
	{Tool: "pacman", Bin: "pacman",
		Config: []string{"/etc/pacman.conf", "/etc/pacman.d/mirrorlist"},
		Check:  []string{"grep -v '^#' /etc/pacman.conf", "systemctl status paccache.timer"},
		Ideas:  "ParallelDownloads, the paccache.timer from pacman-contrib to trim the package cache weekly, a fresh mirror list, .pacnew files left to merge",
		Docs:   []string{"https://wiki.archlinux.org/title/Pacman", "https://wiki.archlinux.org/title/System_maintenance"}},
	{Tool: "Flatpak", Bin: "flatpak",
		Config: []string{"~/.local/share/flatpak/", "/var/lib/flatpak/"},
		Check:  []string{"flatpak list --columns=application,size,installation", "flatpak remotes"},
		Ideas:  "unused runtimes, apps installed both per user and system-wide, remotes no longer used",
		Docs:   []string{"https://docs.flatpak.org/en/latest/using-flatpak.html"}},
	{Tool: "snap", Bin: "snap",
		Config: []string{},
		Check:  []string{"snap list --all", "snap refresh --time"},
		Ideas:  "how many old revisions are kept (refresh.retain, minimum 2), when refreshes happen (refresh.timer)",
		Docs:   []string{"https://snapcraft.io/docs/managing-updates"}},
	{Tool: "systemd journal and units", Bin: "journalctl",
		Config: []string{"/etc/systemd/journald.conf", "/etc/systemd/journald.conf.d/", "~/.config/systemd/user/"},
		Check:  []string{"run_step system-logs", "journalctl --disk-usage", "systemctl --failed", "systemctl list-timers --all"},
		Ideas:  "SystemMaxUse to cap the journal, units that keep failing (fix, disable or mask after asking), a service logging in a loop, timers that never ran",
		Docs:   []string{"https://www.freedesktop.org/software/systemd/man/latest/journald.conf.html"}},
	{Tool: "macOS system logs", Bin: "log", OS: "darwin",
		Config: []string{"~/Library/LaunchAgents/", "/Library/LaunchAgents/", "/Library/LaunchDaemons/", "System Settings → General → Login Items"},
		Check:  []string{"run_step system-logs", "launchctl print gui/$(id -u)/<label>", "log show --last 1h --predicate 'process == \"<name>\"' --style compact | tail -50"},
		Ideas:  "apps that crash repeatedly (update or remove them), launch agents of uninstalled apps that keep failing (disable or remove the plist after asking), processes preventing sleep, oversized log folders",
		Docs:   []string{"https://support.apple.com/guide/mac-help/mchlp2304/mac", "https://www.launchd.info"}},
	{Tool: "Time Machine and Spotlight", Bin: "tmutil", OS: "darwin",
		Config: []string{},
		Check:  []string{"tmutil listlocalsnapshots /", "mdutil -s /"},
		Ideas:  "local snapshots holding space, folders worth excluding from backups and indexing (node_modules, build caches, Docker.raw)",
		Docs:   []string{"https://support.apple.com/guide/mac-help/mh15139/mac"}},
}

func toolConfigHints(_ context.Context, a *Agent, in map[string]any) (string, error) {
	want := strings.ToLower(getString(in, "tool"))
	var b strings.Builder
	n := 0
	for _, h := range hints {
		if want != "" && !strings.Contains(strings.ToLower(h.Tool), want) {
			continue
		}
		if h.Bin != "" && !runner.Has(h.Bin) {
			continue
		}
		if h.OS != "" && h.OS != goos {
			continue
		}
		n++
		fmt.Fprintf(&b, "## %s\n", h.Tool)
		if len(h.Config) > 0 {
			fmt.Fprintf(&b, "config: %s\n", strings.Join(h.Config, ", "))
		}
		if len(h.Check) > 0 {
			fmt.Fprintf(&b, "inspect: %s\n", strings.Join(h.Check, " ; "))
		}
		fmt.Fprintf(&b, "usually worth reviewing: %s\n", h.Ideas)
		fmt.Fprintf(&b, "docs: %s\n\n", strings.Join(h.Docs, " "))
	}
	if n == 0 {
		return "No hints for that tool. Use web_search to find its official configuration docs.", nil
	}
	b.WriteString("These are starting points, not answers. Read the current docs with fetch_url or web_search " +
		"before recommending a setting, and cite the source in the option's detail.")
	return b.String(), nil
}
