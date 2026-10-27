# Ω omega

[Website](https://lcajigasm.github.io/omega/) · [Releases](https://github.com/lcajigasm/omega/releases) · [Homebrew tap](https://github.com/lcajigasm/homebrew-omega)

**AI-driven macOS maintenance.** omega combines what
[topgrade](https://github.com/topgrade-rs/topgrade) does (update all installed
software) and what [Mole](https://github.com/tw93/Mole) does (clean and optimise
your Mac) in a single terminal tool, and puts an AI model in charge: it runs each
step, **reads the errors, diagnoses them and fixes the configuration**, instead of
leaving the failure on your screen.

With an API key from **Anthropic**, **OpenAI**, **OpenRouter** or **OpenCode Zen**
you choose the model that looks after your machine. The interface speaks
**English, Spanish, French, German and Portuguese**.

```text
Ω omega  anthropic · claude-opus-5-5 · task: full
✓ system_info
✓ brew update && brew upgrade · 2m14s
✗ exit 1 pipx upgrade-all
  │ Error: python@3.12 not found
Reinstalling the pipx environments against Homebrew's current Python…
? pipx reinstall-all → approved
✓ pipx reinstall-all · 41s
✓ brew cleanup --prune=all · 12s
Summary: 23 packages updated, 6.4 GB freed, pipx repaired (note saved).
```

*(Illustrative session.)*

---

## Contents

- [Why](#why)
- [Features](#features)
- [Requirements](#requirements)
- [Installation](#installation)
- [Getting started](#getting-started)
- [Tasks](#tasks)
- [Interface](#interface)
- [Languages](#languages)
- [Safety and approvals](#safety-and-approvals)
- [Step catalogue](#step-catalogue)
- [Memory between runs](#memory-between-runs)
- [Providers and models](#providers-and-models)
- [Configuration](#configuration)
- [Command reference](#command-reference)
- [Unattended runs](#unattended-runs)
- [How it works](#how-it-works)
- [Troubleshooting](#troubleshooting)
- [FAQ](#faq)
- [Development](#development)
- [Releasing](#releasing)
- [License](#license)

---

## Why

topgrade and Mole are excellent, but when something breaks (a `pipx` with a
broken Python, a cask that asks for a password, a step that does not apply to
your machine, a deprecated option in `topgrade.toml`) you have to read the error,
find the cause, edit the configuration and run it again. omega hands that loop to
an AI model with a narrow set of tools and a strict safety policy.

## Features

- **Updates everything**: Homebrew (formulae and casks), Mac App Store, npm,
  pnpm, bun, pipx, uv, rustup, cargo, `gh` extensions, Oh My Zsh and macOS.
- **Cleans up**: Homebrew, npm/pnpm/pip/uv/Go caches, Xcode DerivedData,
  unavailable simulators, Docker and the Trash, prioritising what frees the most.
- **Uses topgrade and Mole as backends** when they are installed, and can
  diagnose and fix their configuration (`omega fix`).
- **Tool-using agent**: inspects the system, measures disk usage, reads
  configuration, runs steps and commands, and edits files with a backup.
- **Safety policy**: every command is classified (read-only, normal, dangerous,
  blocked); dangerous ones need your approval.
- **Memory**: learns from each run (e.g. "this step fails because of X") and
  takes it into account next time.
- **Four providers**, any model with tool calling.
- **Five interface languages**, detected from your system.
- **Full-screen TUI** (Bubble Tea) or **plain text output** for scripts, cron
  and CI.
- **Complete log** of every run on disk.
- **API keys in the macOS Keychain**, never in plain text.
- Single Go binary, installable with **Homebrew**.

## Requirements

- macOS (Apple Silicon or Intel).
- An API key for at least one supported provider.
- Optional: `topgrade` and `mole` (`brew install topgrade mole`).

## Installation

### Homebrew (recommended)

```bash
brew install lcajigasm/omega/omega
```

> homebrew-core already has a formula called `omega` (a Xapian search engine),
> so always use the full name `lcajigasm/omega/omega`.

### From source

```bash
git clone https://github.com/lcajigasm/omega.git
cd omega
make build          # binary in ./bin/omega
make install        # or install it into $GOPATH/bin
```

or directly:

```bash
go install github.com/lcajigasm/omega/cmd/omega@latest
```

### Shell completion

The Homebrew formula installs zsh, bash and fish completion. If you built it
yourself:

```bash
omega completion zsh > "${fpath[1]}/_omega"
```

## Getting started

```bash
omega setup        # pick a provider, store the API key and choose a model
omega analyze      # first look: a report that changes nothing
omega              # complete maintenance
```

`omega setup` lists the providers and their status, asks for the key with
hidden input, stores it in the macOS Keychain, downloads the provider's model
list and lets you filter and pick one.

## Tasks

| Task | What it does | Changes the system |
|---|---|---|
| `full` (default) | checks the state, updates everything, cleans up and summarises | yes |
| `update` | updates only (like topgrade) | yes |
| `clean` | measures what takes up space and cleans (like `mo clean`) | yes |
| `analyze` | report on disk space, outdated software, `brew doctor` and the topgrade/Mole configuration | **no** |
| `fix` | runs the topgrade and Mole dry runs, diagnoses the errors and fixes their configuration | yes (with approval) |

You can also give it a free-form task:

```bash
omega -p "Why is ~/Library 40 GB? Tell me what I can safely delete"
omega -p "Only update Homebrew and npm, nothing else"
omega -p "topgrade fails on the rustup step, fix it"
```

## Interface

By default omega opens a full-screen TUI:

- **Header**: provider, model, task and whether you are in read-only mode.
- **History**: the agent's messages and one line per command (`✓`/`✗`,
  duration); when a command fails you see its last lines of output.
- **Live panel**: the running command and its latest output.
- **Approvals**: the command, its risk and the reason (or the diff when it edits
  a file). Answer `y` (yes), `a` (yes to everything in this run) or `n` (no).
- **Follow-ups**: when the agent finishes you can ask for more. An empty Enter or
  `Esc` ends the session.

| Key | Action |
|---|---|
| `↑` `↓` `PgUp` `PgDn` / wheel | scroll the history |
| `y` / `Enter` | approve (`s`, `o`, `j` also work) |
| `a` | approve this and every following request |
| `n` / `Esc` | deny |
| `Ctrl+C` | cancel the run |
| `q` | quit when finished |

With `--no-tui`, or when the output is not a terminal, omega prints plain text.
Without a terminal on standard input, any action that needs approval is
**denied** unless you pass `--yes`.

## Languages

The interface (TUI, plain output, help, prompts and errors) is available in:

| Code | Language |
|---|---|
| `en` | English (fallback) |
| `es` | Spanish |
| `fr` | French |
| `de` | German |
| `pt` | Portuguese |

The language is chosen in this order:

1. the `OMEGA_LANG` environment variable;
2. `language` in the configuration (`auto` by default);
3. the system locale (`LC_ALL`, `LC_MESSAGES`, `LANG`);
4. English.

The agent replies in the same language.

```bash
omega config set language de
OMEGA_LANG=fr omega analyze
```

## Safety and approvals

Before running anything, omega classifies the command (including compound
commands with `&&`, `;`, `|`) and applies the highest risk level found:

| Risk | Examples | Default |
|---|---|---|
| **read-only** | `brew outdated`, `df -h`, `du`, `cat`, `topgrade --dry-run`, `mo clean --dry-run` | runs |
| **normal** | `brew upgrade`, `npm update -g`, `pipx upgrade-all`, `rustup update` | runs |
| **dangerous** | `rm`, `sudo`, output redirection to a file, `sed -i`, `find -delete`, editing configs, **any unknown command** | asks |
| **blocked** | `rm -rf ~` or `/`, `diskutil erase…`, `dd of=/dev/…`, `curl … \| sh`, `csrutil disable`, `nvram` | never runs |

Settings:

- `omega config set approval all`: also ask before normal updates.
- `omega config set approval never`, or `--yes`: never ask. Blocked commands
  stay blocked.
- `--dry-run` (or the `analyze` task): only read-only actions are allowed; the
  rest is rejected automatically and the agent describes it in the summary.

Other guarantees:

- Commands run with **stdin closed** and non-interactive variables
  (`NONINTERACTIVE=1`, `GIT_TERMINAL_PROMPT=0`…). Anything that tries to prompt
  fails instead of hanging.
- Each command has a time limit (`command_timeout`, 45 minutes by default).
- `sudo` is always used as `sudo -n`. For steps that need it, run
  `omega --sudo`: it asks for the password once at the start and keeps it alive
  during the run.
- `write_file` **always** requires approval (unless `--yes`), shows the diff and
  saves a `file.omega-bak-YYYYMMDD-HHMMSS` copy of the original.
- The agent is explicitly instructed never to touch personal data (Documents,
  Desktop, Downloads, Photos, iCloud, SSH keys, keychains).

## Step catalogue

`omega steps` shows what is available on your machine. The agent uses these with
`run_step`, and you can run them without AI with `omega steps run <id>…`.

| id | Category | Command | Risk |
|---|---|---|---|
| `disk` | check | `df -h /` + size of caches, logs and Trash | read-only |
| `brew-outdated` | check | `brew outdated --verbose` | read-only |
| `brew-doctor` | check | `brew doctor` | read-only |
| `macos-check` | check | `softwareupdate --list` | read-only |
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
| `omz` | update | Oh My Zsh upgrader | dangerous |
| `macos` | update | `sudo -n softwareupdate --install --all` | dangerous |
| `brew-cleanup` | clean | `brew cleanup --prune=all` | dangerous |
| `brew-autoremove` | clean | `brew autoremove` | dangerous |
| `npm-cache` | clean | `npm cache clean --force` | dangerous |
| `pnpm-store` | clean | `pnpm store prune` | dangerous |
| `pip-cache` | clean | `pip3 cache purge` | dangerous |
| `uv-cache` | clean | `uv cache prune` | dangerous |
| `go-cache` | clean | `go clean -cache` | dangerous |
| `xcode-derived` | clean | empty `~/Library/Developer/Xcode/DerivedData` | dangerous |
| `simulators` | clean | `xcrun simctl delete unavailable` | dangerous |
| `docker` | clean | `docker system prune -f` | dangerous |
| `trash` | clean | empty `~/.Trash` | dangerous |
| `topgrade-preview` | backend | `topgrade --dry-run` | read-only |
| `topgrade` | backend | `topgrade --yes` | dangerous |
| `mole-clean-preview` | backend | `mo clean --dry-run` | read-only |
| `mole-clean` | backend | `mo clean` | dangerous |
| `mole-optimize-preview` | backend | `mo optimize --dry-run` | read-only |
| `mole-optimize` | backend | `mo optimize` | dangerous |
| `mole-update` | backend | `mo update` | normal |

To disable steps you never want to run:

```bash
omega config set skip_steps macos,docker,brew-greedy
```

Besides the catalogue, the agent can run arbitrary commands (`run_command`) to
diagnose problems or when no step fits, always under the same risk policy.

## Memory between runs

When the agent learns something lasting about your machine (a step that always
fails and why, a special path, a preference of yours), it saves it with the
`remember` tool in `~/.config/omega/notes.md`. Those notes are included in the
agent's instructions on the next run.

```bash
omega notes          # show what it has learned
omega notes clear    # forget everything
```

The file is plain Markdown: you can edit it by hand to give permanent
instructions ("never update node, I manage it with fnm").

## Providers and models

| Provider | API | Environment variable | Default model |
|---|---|---|---|
| `anthropic` | Messages API (official Go SDK) | `ANTHROPIC_API_KEY` | `claude-opus-5-5` |
| `openai` | Chat Completions | `OPENAI_API_KEY` | `gpt-5.5` |
| `openrouter` | Chat Completions (OpenAI-compatible) | `OPENROUTER_API_KEY` | `anthropic/claude-opus-5.5` |
| `opencode` | OpenCode Zen: Claude and Qwen Plus/Flash through the Anthropic endpoint; the rest through Chat Completions | `OPENCODE_API_KEY` | `claude-sonnet-4-6` |

- The model must support **tool calling**.
- Environment variables take precedence over the Keychain.
- With Claude you can tune the reasoning effort (`effort`: `low`, `medium`,
  `high`, `xhigh`, `max`). `medium` or `low` is usually enough for routine
  maintenance and costs less.
- On the Anthropic API, omega enables server-side fallback so that a model
  refusal does not end the run.
- OpenCode Zen only serves GPT models through the Responses API, which omega
  does not support yet. Use GPT through `openai` or `openrouter`.

```bash
omega models                       # models of the active provider (* = current)
omega models openrouter claude     # filter
omega --provider openai --model gpt-5.5 analyze
```

## Configuration

File: `~/.config/omega/config.toml` (`omega config path`). Honours
`XDG_CONFIG_HOME`.

```toml
provider = "anthropic"          # active provider
model = "claude-opus-5-5"       # model of the active provider
effort = "medium"               # Claude models only: low|medium|high|xhigh|max
approval = "dangerous"          # dangerous | all | never
language = "auto"               # en | es | fr | de | pt | auto (system locale)
command_timeout = "45m"         # time limit per command
skip_steps = ["macos", "docker"]

# Model and base URL per provider (used when switching with --provider)
[providers.anthropic]
model = "claude-opus-5-5"

[providers.openrouter]
model = "anthropic/claude-opus-5.5"

[providers.opencode]
model = "claude-sonnet-4-6"
base_url = "https://opencode.ai/inference"   # optional

[providers.openai]
base_url = "https://api.openai.com/v1"       # optional: compatible proxies or gateways
```

```bash
omega config show
omega config set provider openrouter
omega config set model anthropic/claude-sonnet-5.5
omega config set effort low
omega config edit                  # opens $EDITOR
```

### Files

| Path | Contents |
|---|---|
| `~/.config/omega/config.toml` | configuration |
| `~/.config/omega/notes.md` | the agent's memory |
| `~/.local/state/omega/runs/*.log` | log of every run (commands, output, decisions, tokens) |
| macOS Keychain, service `omega-cli` | API keys |

## Command reference

```text
omega [task] [flags]
```

| Flag | Description |
|---|---|
| `--provider` | `anthropic`, `openai`, `openrouter`, `opencode` |
| `-m, --model` | model for this run |
| `--effort` | effort for Claude models |
| `-p, --prompt` | free-form task in natural language |
| `-y, --yes` | approve everything except blocked commands |
| `-n, --dry-run` | read-only mode |
| `--sudo` | ask for sudo at the start and keep it alive |
| `--no-tui` | plain text output |
| `--once` | exit when the agent finishes instead of asking for more |
| `-v, --version` | version |

| Subcommand | Description |
|---|---|
| `omega setup` | setup wizard |
| `omega auth set <provider>` | store an API key in the Keychain |
| `omega auth remove <provider>` | delete it |
| `omega auth status` | which providers have a key and where it comes from |
| `omega models [provider] [filter]` | list models |
| `omega config show \| path \| edit \| set <key> <value>` | configuration |
| `omega steps` | step catalogue and availability |
| `omega steps run <id>… [-y]` | run steps without AI |
| `omega notes [clear]` | the agent's memory |
| `omega completion zsh\|bash\|fish` | completion script |

## Unattended runs

To run it periodically without interaction:

```bash
omega update --yes --no-tui --once
```

Example with `launchd` (every Monday at 9:00), in
`~/Library/LaunchAgents/com.lcajigasm.omega.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.lcajigasm.omega</string>
  <key>ProgramArguments</key>
  <array>
    <string>/opt/homebrew/bin/omega</string>
    <string>update</string>
    <string>--yes</string>
    <string>--no-tui</string>
    <string>--once</string>
  </array>
  <key>StartCalendarInterval</key>
  <dict><key>Weekday</key><integer>1</integer><key>Hour</key><integer>9</integer></dict>
  <key>StandardOutPath</key><string>/tmp/omega.out</string>
  <key>StandardErrorPath</key><string>/tmp/omega.err</string>
</dict>
</plist>
```

```bash
launchctl load ~/Library/LaunchAgents/com.lcajigasm.omega.plist
```

With `--yes` the agent may run dangerous actions without asking (never blocked
ones). If you only want unattended updates, combine `--yes` with `skip_steps` or
use `-p` with precise instructions.

## How it works

```text
            ┌──────────────┐   events   ┌────────────────────┐
  task ───▶ │    agent     │ ─────────▶ │ TUI / plain output │ ──▶ you
            │ (loop + AI)  │ ◀───────── │    (approvals)     │
            └──────┬───────┘  decisions └────────────────────┘
                   │ tools
   ┌───────────────┼──────────────────────────────────────┐
   │ system_info  list_steps  run_step  run_command       │
   │ disk_usage   read_file   write_file  remember        │
   └───────────────┬──────────────────────────────────────┘
                   │ risk classifier (safety)
                   ▼
           runner: zsh -lc, stdin closed, timeout, live output
```

1. omega sends the model a system prompt (role, workflow, rules, disabled
   steps and notes from previous runs) and the task.
2. The model answers with text and/or tool calls.
3. Each call goes through the approval policy: it runs, you are asked, or it is
   rejected. The result (exit code and output trimmed to about 10 KB) goes back
   to the model.
4. The loop continues until the model stops asking for tools. In interactive
   mode you can keep the conversation going.

Code layout:

```text
cmd/omega/          entry point
internal/cli/       cobra commands (setup, auth, models, config, steps, notes)
internal/agent/     agent loop, tools, prompt, events, run log
internal/llm/       providers: anthropic (official SDK), openai-compatible, opencode
internal/safety/    command risk classifier
internal/runner/    command execution with live output
internal/steps/     step catalogue
internal/ui/        TUI (Bubble Tea) and plain output
internal/i18n/      translations (en, es, fr, de, pt)
internal/config/    configuration and paths
internal/secrets/   macOS Keychain
Formula/omega.rb    Homebrew formula (copied to the tap)
```

## Troubleshooting

**`no API key configured`**: run `omega setup` or `omega auth set <provider>`,
or export the environment variable.

**`invalid API key (401)`**: check the key with `omega auth status`. An exported
environment variable takes precedence over the Keychain.

**`model or endpoint not found (404)`**: the model does not exist for that
provider. List them with `omega models <provider>`.

**The model does not call tools or behaves oddly**: use a model with tool
calling. On OpenRouter, filter for models that support it.

**A sudo step fails with "a password is required"**: relaunch with
`omega --sudo`.

**A command hangs**: it should not, because stdin is closed, but if a program
ignores that, `command_timeout` stops it. Lower it with
`omega config set command_timeout 15m`.

**I want to see exactly what it did**: open the latest file in
`~/.local/state/omega/runs/`.

**The agent insists on something I do not want**: deny it (`n`) and it will not
retry it in that run. To make it permanent, add it to `skip_steps` or write it
in `~/.config/omega/notes.md`.

**The interface is in the wrong language**: set it explicitly with
`omega config set language en` or `OMEGA_LANG=en`.

## FAQ

**Does it replace topgrade and Mole?** For a typical developer Mac, yes: it has
equivalent native steps. If you have them installed, it uses them as backends
and helps you fix their configuration.

**Does it send my data to the AI?** The provider you choose receives the
instructions, the (trimmed) output of the commands it runs and the files the
agent reads (configuration). Your documents are not sent unless the agent
explicitly reads one, which it is instructed not to do.

**How much does it cost?** It depends on the model and on how much needs doing.
At the end omega shows the tokens used. A routine `update` is usually short; use
`effort = "low"` or a smaller model to make it cheaper.

**Does it work on Linux?** It is built for macOS (Keychain, `softwareupdate`,
Mac App Store, Xcode paths). Parts of the code would work on Linux, but it is not
supported.

**Can I add a language?** Yes: add a column to `internal/i18n/messages.go` and
its code to `i18n.Supported` and `i18n.Names`. A test checks that every key is
translated with the same format arguments.

## Development

```bash
make build     # builds ./bin/omega
make test      # tests: risk classifier, agent loop with a scripted provider, translations
make lint      # gofmt + go vet
```

To add a step, edit `internal/steps/steps.go` (and optionally add a
`step.<id>` translation). To add a provider, implement `llm.Provider` in
`internal/llm/` and register it in `llm.New` and `llm.Providers`.

## Releasing

```bash
git tag v0.2.0 && git push origin v0.2.0
make formula VERSION=0.2.0          # updates url and sha256 in Formula/omega.rb
cp Formula/omega.rb ../homebrew-omega/Formula/omega.rb
cd ../homebrew-omega && git commit -am "omega 0.2.0" && git push
```

## License

[MIT](LICENSE) © Luis Cajigas
