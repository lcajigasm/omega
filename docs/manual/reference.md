# Reference

[omega](../../README.md) › [Manual](README.md) › Reference

- [Step catalogue](#step-catalogue)
- [Languages](#languages)
- [Configuration](#configuration)
- [Command reference](#command-reference)

## Step catalogue

`omega steps` shows what is available on your machine. The agent uses these with
`run_step`, and you can run them without AI with `omega steps run <id>…`.

| id | Category | Command | Risk |
|---|---|---|---|
| `disk` | check | `df -h /` + size of caches, logs and Trash | read-only |
| `brew-outdated` | check | `brew outdated --verbose` | read-only |
| `brew-doctor` | check | `brew doctor` | read-only |
| `macos-check` | check | `softwareupdate --list` | read-only |
| `health` | check | uptime and load, the disk's SMART status, battery condition, memory pressure, swap, Time Machine | read-only |
| `security` | check | FileVault, the firewall, Gatekeeper, System Integrity Protection, automatic updates | read-only |
| `system-logs` | check | crash reports, failing launch agents, sleep blockers, processes logging faults, log sizes | read-only |
| `dev-artifacts` | check | build folders (`node_modules`, `target`, `.venv`, `Pods`…) in projects untouched for 60 days, largest first | read-only |
| `shell-startup` | check | time three interactive zsh starts | read-only |
| `docker-df` | check | `docker system df` + Docker Desktop disk image | read-only |
| `docker-start` | check | start Docker Desktop and wait for it | dangerous |
| `brew` | update | `brew update && brew upgrade` | normal |
| `brew-greedy` | update | `brew upgrade --cask --greedy` | dangerous |
| `mas` | update | `mas upgrade` | normal |
| `npm` | update | `npm update -g` | normal |
| `pnpm` | update | `pnpm update -g` | normal |
| `bun` | update | `bun upgrade` | normal |
| `pipx` | update | `pipx upgrade-all` | normal |
| `uv` | update | `uv tool upgrade --all` | normal |
| `rustup` | update | `rustup update` | normal |
| `cargo` | update | `cargo install-update -a` | normal |
| `gh-ext` | update | `gh extension upgrade --all` | normal |
| `mise` | update | `mise upgrade` | normal |
| `asdf` | update | `asdf plugin update --all` | normal |
| `gem` | update | `gem update` (not macOS's own Ruby) | normal |
| `composer` | update | `composer global update` | normal |
| `deno`, `poetry`, `flutter`, `gcloud`, `claude-code` | update | each tool's self-update, only when it was not installed by Homebrew | normal |
| `conda` | update | `conda update -n base -y conda` | normal |
| `go-bins` | update | `gup update` (binaries from `go install`) | normal |
| `dotnet-tools` | update | `dotnet tool update --global --all` | normal |
| `vscode-ext` | update | `code --update-extensions` | normal |
| `tldr`, `helm-repos` | update | `tldr --update`, `helm repo update` | normal |
| `tmux-plugins`, `nvim-plugins` | update | tpm `update_plugins all`, lazy.nvim sync | normal |
| `omz` | update | Oh My Zsh upgrader | dangerous |
| `macos` | update | `sudo softwareupdate --install --all` | dangerous |
| `brew-cleanup` | clean | `brew cleanup --prune=all` | dangerous |
| `brew-autoremove` | clean | `brew autoremove` | dangerous |
| `npm-cache` | clean | `npm cache clean --force` | dangerous |
| `pnpm-store` | clean | `pnpm store prune` | dangerous |
| `pip-cache` | clean | `pip3 cache purge` | dangerous |
| `uv-cache` | clean | `uv cache prune` | dangerous |
| `go-cache` | clean | `go clean -cache` | dangerous |
| `go-modcache` | clean | `go clean -modcache` | dangerous |
| `yarn-cache`, `bun-cache`, `deno-cache` | clean | `yarn cache clean`, `bun pm cache rm`, `deno clean` | dangerous |
| `cargo-cache` | clean | `cargo cache --autoclean` | dangerous |
| `gem-cleanup`, `composer-cache`, `cocoapods-cache` | clean | `gem cleanup`, `composer clear-cache`, `pod cache clean --all` | dangerous |
| `gradle-cache` | clean | empty `~/.gradle/caches` and `~/.gradle/daemon` | dangerous |
| `conda-clean`, `nuget-cache`, `flutter-cache` | clean | `conda clean --all -y`, `dotnet nuget locals all --clear`, `flutter pub cache clean -f` | dangerous |
| `mise-prune`, `pre-commit-gc` | clean | `mise prune -y`, `pre-commit gc` | dangerous |
| `playwright-cache` | clean | empty `~/Library/Caches/ms-playwright` | dangerous |
| `ios-device-support` | clean | empty `~/Library/Developer/Xcode/iOS DeviceSupport` | dangerous |
| `xcode-derived` | clean | empty `~/Library/Developer/Xcode/DerivedData` | dangerous |
| `simulators` | clean | `xcrun simctl delete unavailable` | dangerous |
| `docker-containers` | clean | `docker container prune -f` | always ask |
| `docker-images` | clean | `docker image prune -f` | always ask |
| `docker-images-all` | clean | `docker image prune -a -f` | always ask |
| `docker-build-cache` | clean | `docker builder prune -a -f` | always ask |
| `docker-volumes` | clean | `docker volume prune -a -f` (deletes data) | always ask |
| `docker` | clean | `docker system prune -a -f` | always ask |
| `trash` | clean | empty `~/.Trash` | dangerous |
| `topgrade-preview` | backend | `topgrade --dry-run` | read-only |
| `topgrade` | backend | `topgrade --yes` | dangerous |
| `mole-clean-preview` | backend | `mo clean --dry-run` | read-only |
| `mole-clean` | backend | `mo clean` | dangerous |
| `mole-optimize-preview` | backend | `mo optimize --dry-run` | read-only |
| `mole-optimize` | backend | `mo optimize` | dangerous |
| `mole-update` | backend | `mo update` | normal |

On Linux the catalogue has the developer-tool steps above (everything that is
not Homebrew casks, the Mac App Store, macOS, Xcode or Mole) plus these, each
one only when its binary is installed:

| id | Category | Command | Risk |
|---|---|---|---|
| `disk` | check | `df -h /` + size of `~/.cache`, Trash, `/var/cache` and `/var/log` | read-only |
| `apt-outdated` `dnf-outdated` `pacman-outdated` `zypper-outdated` | check | `apt list --upgradable`, `dnf check-update`, `pacman -Qu`, `zypper list-updates` | read-only |
| `flatpak-outdated` `snap-outdated` | check | `flatpak remote-ls --updates`, `snap refresh --list` | read-only |
| `journal-size` | check | `journalctl --disk-usage` | read-only |
| `health` | check | uptime and load, a pending restart, memory and swap, inodes, disks, battery | read-only |
| `security` | check | the firewall, automatic security updates, SSH logins, disk encryption | read-only |
| `firmware-check` | check | `fwupdmgr get-updates` | read-only |
| `firmware` | update | `fwupdmgr refresh` and `fwupdmgr update` | dangerous |
| `system-logs` | check | failed units, errors since boot, core dumps, journal and log sizes | read-only |
| `apt` | update | `sudo apt-get update && sudo apt-get -y upgrade` (non-interactive, keeps edited configuration files) | dangerous |
| `dnf` | update | `sudo dnf upgrade --refresh -y` | dangerous |
| `pacman` | update | `sudo pacman -Syu --noconfirm` | dangerous |
| `zypper` / `zypper-dup` | update | `sudo zypper update`, or `dist-upgrade` on Tumbleweed | dangerous |
| `flatpak` | update | `flatpak update -y` | normal |
| `snap` | update | `sudo snap refresh` | dangerous |
| `apt-clean` `dnf-clean` `pacman-cache` `zypper-clean` | clean | the package cache (`pacman-cache` keeps two versions with `paccache`) | dangerous |
| `apt-autoremove` `dnf-autoremove` `pacman-orphans` | clean | packages nothing depends on any more | dangerous |
| `flatpak-unused` | clean | `flatpak uninstall --unused` | dangerous |
| `snap-old` | clean | removes the disabled revisions snapd keeps | dangerous |
| `journal-vacuum` | clean | `sudo journalctl --vacuum-time=2weeks` | dangerous |
| `thumbnail-cache` | clean | empties `~/.cache/thumbnails` | dangerous |
| `trash` | clean | empties `~/.local/share/Trash` | dangerous |

Every step that uses `sudo` is dangerous: omega asks before running it and asks
for the password in its own interface. `omega steps` lists the catalogue of the
system you are on.

Disable steps you never want to run with
`omega config set skip_steps macos,docker,brew-greedy`. Ids of the other
system are accepted, so one configuration can be shared between a Mac and a
Linux machine.

## Languages

The interface (TUI, plain output, help, cards, prompts and errors) is available in
English, Spanish, French, German and Portuguese. The language is chosen from
`OMEGA_LANG`, then `language` in the configuration (`auto` by default), then the
system locale (`LC_ALL`, `LC_MESSAGES`, `LANG`), then English. The agent replies
in the same language.

```bash
omega config set language es
OMEGA_LANG=fr omega analyze
```

## Configuration

File: `~/.config/omega/config.toml` (`omega config path`). Honours
`XDG_CONFIG_HOME`. `omega config check` reports an approval, language, effort,
timeout or `skip_steps` value omega would misread. Saving writes a temporary
file and replaces the config only after it is valid, so a bad edit does not
wipe the previous one.

```toml
provider = "anthropic"          # active provider
model = ""                      # empty or "auto": automatic; or a fixed model id
effort = "medium"               # Claude models only: low|medium|high|xhigh|max
approval = "dangerous"          # dangerous | all | never
language = "auto"               # en | es | fr | de | pt | auto (system locale)
command_timeout = "45m"         # time limit per command
# model_catalog = "off"         # do not download the public model catalogue (automatic mode then uses built-in names)
skip_steps = ["macos"]

[providers.anthropic]
fast_model = "claude-haiku-5-5"     # optional: override the automatic tiers
smart_model = "claude-sonnet-5-5"

[providers.ollama]
model = "qwen3:14b"
base_url = "http://localhost:11434/v1"   # optional

[providers.custom]
base_url = "http://localhost:8080/v1"    # required for custom
model = "my-model"
```

| Path | Contents |
|---|---|
| `~/.config/omega/config.toml` | configuration |
| `~/.config/omega/notes.md` | the agent's memory |
| `~/.config/omega/mac.toml` | your rules for `omega mac` |
| `~/.local/state/omega/mac-journal.jsonl` | changes made by `omega mac`, for `omega mac undo` |
| `~/.local/state/omega/runs/*.log` | log of each run (commands, output, approvals, denials, tokens and cost). omega keeps the newest 30 |
| macOS Keychain, service `omega-cli` | API keys on macOS |
| Secret Service, attributes `service=omega-cli account=<provider>` | API keys on Linux with a keyring |
| `~/.config/omega/keys` (mode `0600`) | API keys on Linux without a keyring |
| Windows Credential Manager, generic credential `omega-cli:<provider>` | API keys on Windows |

## Command reference

```text
omega [task] [flags]
```

| Flag | Description |
|---|---|
| `--provider` | `anthropic`, `openai`, `openrouter`, `opencode`, `ollama`, `lmstudio`, `custom` |
| `-m, --model` | model for this run (no escalation) |
| `--effort` | effort for Claude models |
| `-p, --prompt` | free-form task in natural language |
| `--auto` | apply low-risk improvements without offering them first |
| `-y, --yes` | approve everything except blocked commands and Docker cleanups |
| `-n, --dry-run` | read-only mode |
| `--sudo` | ask for sudo at the start and keep it alive |
| `--no-tui` | plain text output |
| `--once` | exit when the agent finishes instead of asking for more |
| `-v, --version` | version |
| `--no-ai`, `--offline` | run deterministically without calling an AI model |
| `--json` | output report as structured JSON |
| `--format json` | the same as `--json`; no other format is written yet |
| `-q, --quiet` | quiet mode; suppress non-essential output |
| `-v, --verbose` | verbose output showing commands and streaming output |
| `--debug` | accepted, and reserved: it prints nothing more yet |
| `--fail-on-warning` | exit with non-zero code if any warning is found |
| `--fail-on-outdated` | exit with non-zero code if outdated packages are detected |
| `--fail-on-critical` | exit with non-zero code if any step fails |
| `--max-cost` | maximum estimated session cost in USD before stopping |
| `--max-turns` | maximum number of agent turns |

| Subcommand | Description |
|---|---|
| `omega setup` | setup wizard |
| `omega model [model\|auto] [--provider p]` | choose the model on a card, or set it directly |
| `omega models [provider] [filter]` | list models with prices |
| `omega docker` | measure Docker and clean what you select |
| `omega menu` | home screen: every command on one screen, started with Enter |
| `omega full [--only a,b] [--skip a,b]` | everything, stage by stage in one interface: choose the stages, see what each does and why, skip, repeat or stop |
| `omega mac [status\|login\|background\|menubar\|dock\|shortcuts]` | review what runs at login and in the background, the menu bar, the Dock and shortcut conflicts (AI-guided without a subcommand) |
| `omega mac permissions` | remove privacy entries of binaries and apps that are gone |
| `omega mac apps` | unused apps, Intel-only apps and leftovers of removed apps |
| `omega dev [status\|install [tool…]\|activate]` | review, install and activate languages and developer tools (AI-guided without a subcommand) |
| `omega mac policy [edit]` | your rules (`mac.toml`) and omega's defaults |
| `omega mac undo` | revert changes made by `omega mac` |
| `omega auth set\|remove\|status` | manage API keys (validated before saving) |
| `omega config show\|path\|edit\|check\|set <key> <value>` | configuration (`check` validates it) |
| `omega steps` | step catalogue and availability |
| `omega steps run <id>… [-y]` | run steps without AI |
| `omega notes [clear]` | the agent's memory |
| `omega logs` | list run transcripts, newest first |
| `omega logs view [n\|file]` | print one transcript (the latest if omitted) |
| `omega doctor` | check config, key shape, provider, the configured model and installed steps; no chat, no tokens |
| `omega completion zsh\|bash\|fish\|powershell\|install` | completion script and automatic installer |
| `omega quick` | one-click safe maintenance for everyday users |
| `omega net flush\|ping\|ports\|kill\|audit` | network, DNS cache flush, registry ping, port conflict hunter and hosts audit |
| `omega git status\|clean\|gc` | git repository hygiene across dev folders: merged branches, unpushed work, .git optimization |
| `omega disk duplicates\|large\|artifacts` | duplicate file finder by SHA-256, large files, and dev build artifact cleanup |
| `omega audit ssh\|secrets` | SSH directory & key permissions audit, and shell history secret leak scanner |
| `omega privacy [--clean]` | review OS telemetry and safely clean browser caches |
| `omega uninstall <app>` | deep uninstallation locating leftover preferences, caches and app support files |
| `omega schedule install\|status\|uninstall` | automated background maintenance with native daemons (launchd, systemd, Windows Task Scheduler) and desktop alerts |
| `omega rollback` | inspect and restore safety filesystem snapshots (APFS, Btrfs, Windows System Restore) |
| `omega export [file]` | export machine blueprint (packages, dev tools, config) to omega-blueprint.json |
| `omega bootstrap <file>` | restore and provision a system from an Omega blueprint |
| `omega shell` | interactive maintenance REPL console |
| `omega man [dir]` | generate troff man pages for Omega commands |
| `omega fix [--log <file>] [error...]` | diagnose and fix failing commands, configuration or external build logs |

`omega logs view` prints the newest transcript, `omega logs view 2` prints the second row of `omega logs`, and `omega logs view <file>` prints that file. A new run deletes older transcripts until 30 remain.

`omega doctor` prints the version and, after asking GitHub for the latest release, whether a newer one exists (an old version is reported, not counted as a failure). It then loads the config, checks that a cloud key is present and shaped like a key (no spaces, not a URL, long enough), and asks the provider whether it is reachable. A cloud provider is asked for its model list. A local one is probed at `<base_url>/models`. Neither call sends a chat message. When `model`, `fast_model` or `smart_model` is set to a name other than `auto`, and the provider returned at least one model id, doctor fails if that name is not in the list. The command then lists catalogue steps that are installed on this machine. It exits with status 1 when the config, the key, the provider or a configured model check fails. A missing optional tool does not.

A 429, a 5xx or a dropped connection on Anthropic or an OpenAI-compatible provider is retried three times. The wait is the `Retry-After` value when the server sends one (a number of seconds or an HTTP date), and never more than 30 seconds. Otherwise the wait is 2s, then 4s, then 8s. Any other 4xx is reported immediately.
