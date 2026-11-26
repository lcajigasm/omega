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

Your routines (`omega custom`) live beside it in `~/.config/omega/custom.toml`:
`default` names the routine `omega custom` runs, and each `[routines.<name>]`
has `steps` (in running order), `stop_on_fail`, `only_below_free_gb` and
`days` (`mon` … `sun`; a scheduled run skips the other days). See
[Your own routine](tasks.md#your-own-routine).

```toml
provider = "anthropic"          # active provider
model = ""                      # empty or "auto": automatic; or a fixed model id
effort = "medium"               # Claude models only: low|medium|high|xhigh|max
approval = "dangerous"          # dangerous | all | never
language = "auto"               # en | es | fr | de | pt | auto (system locale)
command_timeout = "45m"         # time limit per command
# model_catalog = "off"         # do not download the public model catalogue (automatic mode then uses built-in names)
skip_steps = ["macos"]

# [providers.anthropic]
# ultra_model = "claude-opus-5-5" # the model omega ultra moves to for its deepest analysis

[disk_budgets]                    # omega disk limits set <check> <size>
"claude-desktop.vm" = "8.0 GB"

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
| `~/.local/state/omega/backups/<tool>/` | the copy of a tool's file kept before `omega assist` or `omega disk limits` changed it |
| `~/.local/state/omega/runs/*.log` | log of each run (commands, output, approvals, denials, tokens and cost). omega keeps the newest 30 |
| macOS Keychain, service `omega-cli` | API keys on macOS |
| Secret Service, attributes `service=omega-cli account=<provider>` | API keys on Linux with a keyring |
| `~/.config/omega/keys` (mode `0600`) | API keys on Linux without a keyring |
| Windows Credential Manager, generic credential `omega-cli:<provider>` | API keys on Windows |

## Command reference

```text
omega [task] [options]
```

`update`, `clean`, `analyze`, `health` and `tune` are commands of their own, like `full`, `quick` and `fix`: `omega health --help` shows the help of the health check. `omega update` still works as it always did, and `omega update brew and npm` (three words or more) is still a request in your own words. Every help is in your language, and its options come in groups: the command's own, **Agent options** (the run with a model) and **Options without AI** (with `--no-ai`). A flag of the runs without AI given to a run with the model (`-v`, `--fail-on-*`) is ignored, and omega says so on stderr.

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
| `--only`, `--skip` | `omega full` and `omega ultra`: the stages to run or leave out, by id |
| `-U, --unattended` | `omega full` only: the upkeep stages (doctor, analyze, health, update, clean, docker) with no questions, the sudo password asked once ([more](tasks.md#unattended-runs)) |
| `-v, --version` | version |
| `--no-ai`, `--offline` | run deterministically without calling an AI model. Only `full`, `quick`, `update`, `clean`, `analyze` and `health` exist without it: any other task, and a request in your own words, is an error |
| `--json` | one JSON report on stdout; implies `--no-ai`. `omega full` has none |
| `--format json\|text` | `json` is the same as `--json`, `text` the default; anything else is an error |
| `-q, --quiet` | with `--no-ai`: suppress the progress lines |
| `-v, --verbose` | with `--no-ai`: show the output of every command |
| `--debug` | accepted, and reserved: it prints nothing more yet |
| `--fail-on-warning` | with `--no-ai`: exit with 2 if a check warns |
| `--fail-on-outdated` | with `--no-ai`: exit with 2 if outdated packages are found |
| `--fail-on-critical` | with `--no-ai`: exit with 1 if any step fails, checks included (a step that changes something and fails already does) |
| `--max-cost` | stop at this estimated session cost in USD; `0` (the default) is no limit |
| `--max-turns` | stop after this many turns of the model; `0` (the default) is no limit |

### Exit status

| Status | Meaning |
|---|---|
| `0` | the run worked |
| `1` | something failed: a step, a check of `omega doctor`, the model or the provider |
| `2` | the run worked, but `--fail-on-warning` or `--fail-on-outdated` found what they look for |
| `64` | a command line omega cannot use: an unknown command, option or value, options that cannot go together, the wrong number of arguments. The message ends with the help to read, and a mistyped word gets the closest names (a typo in `omega config set` offers it); asking for the help of a word that is not a command fails the same way |

A run with `--no-ai` follows the same confirmation rules as the agent: a dangerous step asks at the terminal unless `--yes` was given, `approval = "all"` asks before every step that is not read-only, and the Docker cleanups always ask. With nobody at a terminal such a step is skipped and named. The configuration must load and be valid: a `config.toml` that cannot be read stops the run instead of running with the defaults.

The run without AI speaks your language, its summary included. The JSON report keeps its field names in every language. It has `task`, `timestamp`, `dry_run`, `steps_run` (always a list; each step with `id`, `name`, `category`, `command`, `success`, `exit_code`, `duration` in nanoseconds, `duration_ms` and `output`), `would_run` (under `--dry-run`: the steps that would have run), `skipped` (the steps nobody could confirm, with a `reason`), `outdated_count`, `warning_count`, `total_duration` (nanoseconds), `total_duration_ms` and `summary`.

| Subcommand | Description |
|---|---|
| `omega setup` | setup wizard |
| `omega model [model\|auto] [--provider p]` | choose the model on a card, or set it directly |
| `omega model list [provider] [filter] [--json] [--refresh]` | list models with score and prices (`omega models` still works) |
| `omega docker` | measure Docker and clean what you select |
| `omega menu` | home screen: every command on one screen, started with Enter |
| `omega full [--only a,b] [--skip a,b]` | everything, stage by stage in one interface: choose the stages, see what each does and why, skip, repeat or stop |
| `omega unattended [--only a,b] [--skip a,b]` | `omega full --unattended`: upkeep only (checks, updates, caches) with no questions, the sudo password asked once and every approval answered yes; it never changes how you set up the machine |
| `omega mac [status\|login\|background\|menubar\|dock\|shortcuts]` | review what runs at login and in the background, the menu bar, the Dock and shortcut conflicts (AI-guided without a subcommand) |
| `omega mac permissions` | remove privacy entries of binaries and apps that are gone |
| `omega mac apps` | unused apps, Intel-only apps and leftovers of removed apps |
| `omega dev [status\|install [tool…]\|activate]` | review, install and activate languages and developer tools (AI-guided without a subcommand) |
| `omega mac policy [edit]` | your rules (`mac.toml`) and omega's defaults |
| `omega mac undo` | revert changes made by `omega mac` |
| `omega auth set\|remove\|status [--json]` | manage API keys (validated before saving); the JSON has the masked key only |
| `omega config show [--json]\|path\|edit\|check\|set <key> <value>` | configuration (`check` validates it and names the keys omega does not know, such as a mistyped `aproval`; `omega doctor` names them too) |
| `omega steps [--json] [--installed] [--category check\|update\|clean\|backend]` | step catalogue and availability |
| `omega steps show <id> [--json]` | everything about one step: full command, what it does, risk (and whether it always asks), whether it is installed and how to install it, and its last runs by `omega custom` |
| `omega steps run <id>… [-y] [-n]` | run steps without AI; `-n` shows the commands only. `-y` does not answer for a step that always asks, and a step nobody could confirm makes the command fail |
| `omega custom [routine] [--show [--json]\|--dry-run\|-y]` | run a routine (the default one without a name): the steps you chose, in your order, here and without any AI model. A scheduled run on a day the routine does not list does nothing; one started by hand warns and runs |
| `omega custom --set [routine] [id…] [--stop-on-fail id,…] [--only-below-free-gb N] [--days mon,…]` | choose a routine's steps and order in an editor (keys: space add/remove, ⇧↑↓ reorder, `f` stop if it fails, `b` free-space limit, `w` days, `i` ask your model for a recommendation, `d` send machine data with it; each step shows its last duration and the space it freed); with ids, set it directly; with only options, change just those. A first word that is not a step id names the routine. `--days ""` clears the days, `--only-below-free-gb 0` the limit |
| `omega custom --list [--json]`, `omega custom <routine> --default`, `omega custom [routine] --clear` | list the routines with their options, choose the one `omega custom` runs, delete one |
| `omega history [-n N] [--task t] [--since 7d\|2026-11-01] [--json]` | the latest runs, what is still pending and the newest notes on one screen |
| `omega notes [--last N] [--grep text] [--json] [clear]` | the agent's memory, newest first |
| `omega notes compact [--dry-run] [-y]` | remove the notes that repeat a newer one (same words once dates, case and punctuation are left aside), after showing them and a yes; the old file stays as `notes.md.bak`. No AI |
| `omega pending [--all] [--json]` | what earlier runs left for you to do by hand, with how long it has been there; `--all` adds what is postponed, dismissed or done |
| `omega pending dismiss\|snooze <id> <3d\|2w\|1m>\|reopen\|done <id>` | stop runs from repeating an item, postpone it, bring it back or mark it done |
| `omega logs [-n N] [--task t] [--failed] [--json]` | list run transcripts, newest first |
| `omega logs view [n\|file] [--tools]` | print one transcript (the latest if omitted); `--tools` keeps only the tool calls and what they returned |
| `omega doctor [--json]` | check config, key shape, provider, the configured model, your own steps and installed steps; no chat, no tokens |
| `omega completion zsh\|bash\|fish\|powershell\|install` | completion script, with a description for every command, and automatic installer (`install` reads `$SHELL`; on Windows it writes the PowerShell script and prints the line for your profile). Values complete too: `--provider`, `--effort`, `--format`, `--model` (from the stored catalogue, never the network), `schedule install --task/--interval`, the ids of `pending`, `steps run`, `steps show` and `custom`, and the numbers of `logs view` |
| `omega quick` | the short safe round: with `--no-ai` or no API key it empties the Trash and two caches (asking first), shows the battery and the free disk space and counts pending updates |
| `omega net flush\|ping\|ports\|kill\|audit` | network, DNS cache flush, registry ping, port conflict hunter and hosts audit |
| `omega git status\|clean\|gc` | git repository hygiene across dev folders: merged branches, unpushed work, .git optimization |
| `omega skills [list\|audit]` | the skills of your AI coding agents, who sees each one and what is wrong with them; `--json` for the inventory |
| `omega skills install <folder\|git-url\|owner/repo>` | install the skills of a folder or a repository for every agent (`--skill`, `--to`, `--copy`, `--force`) |
| `omega skills sync\|update\|remove` | share skills with every installed agent, update them from their source, remove them |
| `omega agents [list]` | what closed AI agent sessions left running and the open sessions; `--json` for the report (also `omega claude`) |
| `omega agents kill [pid\|session-id]...` | end what closed sessions left: everything, one process or one session's worth (`--yes`); shared daemons only by PID |
| `omega agents end <pid\|session-id>` | end an open agent session and what it runs (`--yes`) |
| `omega agents idle [--after 2h]` | end the Claude Code sessions idle for longer than `--after`, 30 minutes at least (`--yes`) |
| `omega assist [tool]` | configuration assistants for Claude Code, Claude Desktop, ChatGPT/Codex, Grok, Docker, Cursor, VS Code, Gemini CLI, Homebrew, Xcode and Ollama; `--yes` applies every recommendation, `--json` |
| `omega assist apply <check> [value]` | apply one check with its recommended value or yours (`--yes`) |
| `omega disk limits [--json]` | what each tool takes on disk and how it is limited |
| `omega disk limits apply\|set <check> <value>\|enforce` | write the recommended limits, set one (a budget: `8GB` or `off`), free what is over its limit (`--yes`, `--dry-run`) |
| `omega mcp` | MCP server on stdin/stdout so AI agents can use omega's commands |
| `omega docs [page]`, `omega docs search <words>`, `omega docs ask <question>` | the manual inside omega: a page, an offline search, an answer from your model |
| `omega disk duplicates\|large\|artifacts` | duplicate file finder by SHA-256, large files, and dev build artifact cleanup |
| `omega audit ssh\|secrets` | SSH directory & key permissions audit, and shell history secret leak scanner |
| `omega privacy [--clean]` | review OS telemetry and safely clean browser caches |
| `omega uninstall <app>` | finds one app by its exact name and what it left behind (preferences, caches, app support), and moves it all to the Trash after asking |
| `omega schedule install\|status\|uninstall` | automated background maintenance with native daemons (launchd, systemd, Windows Task Scheduler); `--interval daily\|weekly\|monthly`, `--task quick\|update\|clean\|full\|analyze\|health`. The job runs `omega <task> --yes --no-tui --no-ai`: the built-in steps, never the model, and it never empties the Trash |
| `omega rollback` | inspect and restore safety filesystem snapshots (APFS, Btrfs, Windows System Restore) |
| `omega export [file]` | write the machine's blueprint to omega-blueprint.json (mode `0600`): Homebrew formulae and casks, global npm packages, pipx packages, the developer tools found (for reference), omega's settings and your routines (`custom.toml`). What could not be listed is named |
| `omega bootstrap <file> [-n] [-y] [-f]` | show what a blueprint would install and restore, then do it after a yes at the terminal (or with `-y`); `-n` shows the plan only. Routines that differ from this machine's `custom.toml` replace it only after a yes at the terminal or with `--force` (the old file stays as `custom.toml.bak`) |
| `omega shell` | interactive maintenance REPL console |
| `omega man [--output dir]` | the man page (in English in every language): shown with `man` at a terminal, printed as troff anywhere else (`omega man > omega.1`); `--output` writes `dir/omega.1` |
| `omega fix ["error…"]`, `… \| omega fix -`, `omega fix --log <file>` | the model diagnoses an error and fixes it: one you describe or paste, one read from stdin, or a log file; with nothing given, why topgrade fails. Agent options only |

`omega logs --json` and `omega history --json` give each run with `n` (the number `logs view` takes), `file`, `path`, `when`, `task`, `model`, `cost_usd`, `failed_tools`, `ended` and `bytes`. `omega pending --json` gives the items as they are stored (`id`, `text`, `status`, `first_seen`, `last_seen`, `seen`, `until`); `omega notes --json` the notes with `day` and `text`.

`omega logs view` prints the newest transcript, `omega logs view 2` prints the second row of `omega logs`, and `omega logs view <file>` prints that file. A new run deletes older transcripts until 30 remain.

`omega doctor` prints the version and, after asking GitHub for the latest release, whether a newer one exists (an old version is reported, not counted as a failure). It then loads the config, checks that a cloud key is present and shaped like a key (no spaces, not a URL, long enough), and asks the provider whether it is reachable. A cloud provider is asked for its model list. A local one is probed at `<base_url>/models`. Neither call sends a chat message. When `model`, `fast_model` or `smart_model` is set to a name other than `auto`, and the provider returned at least one model id, doctor fails if that name is not in the list. A file in `steps.d` that does not parse, or a step in one that omega leaves out, counts as a failed check. The command then lists catalogue steps that are installed on this machine. It exits with status 1 when the config, the key, the provider or a configured model check fails. A missing optional tool does not.

`omega steps --json` prints the catalogue as a JSON list. Each step has `id`, `name`, `command`, `category_id` (`check`, `update`, `clean` or `backend`), `risk_id` (`read-only`, `normal` or `dangerous`), `available` (what it needs is installed) and `skipped` (turned off in `skip_steps`). `category`, `risk`, `dangerous` and `installed` are there too; `category` and `risk` are in your language, so a program should read the `_id` fields.

`omega doctor --json` prints one JSON document and nothing else: `ok`, `lines` (the report as text), `version`, `config` (`path`, `ok`, and `error` when it is not), `provider` (`name`, `ok`), `custom_step_problems` and `steps` (the same list as `omega steps --json`). It exits with status 1 when `ok` is false.

`omega bootstrap` refuses a blueprint in which a package name is anything but a name (letters, digits and `@ / . _ + -`, not starting with `-`), and never runs anything without showing it first. It restores omega's settings except `approval` and a provider's `base_url`, which keep the value this machine has, and keeps the previous file as `config.toml.bak`. The developer tools listed in a blueprint are not installed; `omega dev install` does that. It exits with an error when an installation fails.

Your own steps live in `~/.config/omega/steps.d/*.toml`. A step needs an `id` and a `command`; `category` is `check`, `update`, `clean` (the default) or `backend`, and `risk` is `read-only`, `normal` (the default) or `dangerous`. A step whose command the safety policy blocks, or whose `id` is one of omega's own steps, is left out, and a file that another user of the machine can write (or that belongs to another user) is not read. That, a file that does not parse, or a step with another category or risk, is reported on stderr when omega starts and by `omega doctor` and `omega config check`. The same files may hold `[hooks]`: `pre_<task>` runs before a task and stops it when it fails, `post_<task>` runs after it, and `on_error` runs when the task failed. Hooks run with and without `--no-ai`; under `--dry-run` they are named, not run. A request in your own words has no hooks.

`omega config edit` and `omega mac policy edit` open `$VISUAL`, then `$EDITOR` (either may hold arguments, as in `code -w`), then `nano`, or `notepad` on Windows.

A 429, a 5xx or a dropped connection on Anthropic or an OpenAI-compatible provider is retried three times. The wait is the `Retry-After` value when the server sends one (a number of seconds or an HTTP date), and never more than 30 seconds. Otherwise the wait is 2s, then 4s, then 8s. Any other 4xx is reported immediately.
