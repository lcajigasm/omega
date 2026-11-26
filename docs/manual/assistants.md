# Assistants, disk limits, MCP and the manual

[omega](../../README.md) › [Manual](README.md) › Assistants, disk limits, MCP and the manual

- [Configuration assistants](#configuration-assistants)
- [Disk limits](#disk-limits)
- [What each tool can be limited by](#what-each-tool-can-be-limited-by)
- [omega for AI agents: the MCP server](#omega-for-ai-agents-the-mcp-server)
- [Ask the manual](#ask-the-manual)

None of this calls a model, except `omega docs ask`. Nothing leaves your
machine.

## Configuration assistants

`omega assist` checks the configuration of the AI apps and developer tools on
this machine and sets what is worth setting in their own files:

| Tool | What the assistant checks |
|---|---|
| topgrade (`topgrade`) | omega's local CI image (`omega:ci*`) in `[containers] ignored_containers`, so topgrade stops trying to pull it; `pre_sudo` and `sudo_loop`, which stop a whole run that has no sudo |
| Mole (`mole`) | Chrome's on-device AI model in `~/.config/mole/whitelist`, so `mo clean` stops deleting what Chrome downloads again. A new whitelist starts with Mole's own defaults, read from the installed Mole, because the file replaces them |
| Claude Code (`claude-code`) | how many days conversations are kept (`cleanupPeriodDays`), deny rules that keep the agent out of `.env` files, SSH and AWS keys and omega's own configuration, omega's MCP server, old versions the native installer kept |
| Claude Desktop (`claude-desktop`) | the Cowork VM image (`vm_bundles`), the app's caches, the old Claude Code versions it keeps, omega's MCP server in `claude_desktop_config.json` |
| ChatGPT / Codex (`codex`) | the size of the prompt history (`[history] max_bytes`), session recordings in `~/.codex/sessions`, omega's MCP server in `config.toml`, the caches of the ChatGPT app's browser |
| Grok (`grok`) | the Grok Bot app's caches; the `grok` command when it is installed |
| Docker (`docker`) | the build cache limit (`builder.gc`), container log rotation, the size of Docker Desktop's disk image |
| Cursor (`cursor`) | old versions of Cursor's background agent and of `cursor-agent`, the app's caches, omega's MCP server in `~/.cursor/mcp.json` |
| VS Code (`vscode`) | the app's caches, omega's MCP server in `mcp.json` |
| Gemini CLI (`gemini-cli`) | chats and checkpoints in `~/.gemini/tmp`, omega's MCP server in `settings.json` |
| Homebrew (`homebrew`) | how many days downloads are kept (`HOMEBREW_CLEANUP_MAX_AGE_DAYS` in `~/.homebrew/brew.env`), the download cache |
| Xcode (`xcode`) | DerivedData, simulator devices |
| Ollama (`ollama`) | the size of the downloaded models (measured only) |

```bash
omega assist                      # every tool: installed, version, what is pending
omega assist claude-code          # walk through one tool and apply what you accept
omega assist docker --yes         # apply every recommendation of a tool
omega assist apply codex.history 50MB
omega assist codex --json         # the checks as JSON
```

Each tool also answers to the names people use: `chatgpt`, `claude`, `grok
bot`, `brew`, `docker desktop`. A tool that is not installed is listed and has
no checks.

Before a file is changed, omega keeps a copy of it in
`~/.local/state/omega/backups/<tool>/`. JSON files keep their order and every
key omega does not set; a JSON file with comments, or a TOML file that does
not parse, is left to you. A TOML file is edited line by line, so its comments
stay, and the result must parse before it is written.

The desktop app has the same in its **Configure** view.

## Disk limits

`omega disk limits` lists what each tool takes and how it is limited:

```bash
omega disk limits                       # size, limit now, recommendation
omega disk limits apply                 # write every recommended setting (asks first)
omega disk limits set docker.build-cache 20GB
omega disk limits set claude-code.transcripts 14
omega disk limits set claude-desktop.vm 8GB     # a budget omega keeps
omega disk limits set claude-desktop.vm off     # no budget
omega disk limits enforce               # free what is over its limit (asks first)
omega disk limits enforce --dry-run
```

A limit is one of three things:

- **A setting** in the tool's own configuration file, when the tool has one:
  the tool keeps itself under it from then on. `apply` writes all the
  recommended ones.
- **A budget** omega keeps, for a folder the tool never trims: Claude Desktop's
  VM image, Xcode's DerivedData, Codex's session recordings, Gemini CLI's
  chats. You choose the number; omega never picks one for you. Once the folder
  holds more, `enforce` empties it (caches and images the app downloads or
  builds again) or removes its oldest files until it fits (conversations and
  sessions). An app that is open is never touched: omega says so and skips it.
- **A cleanup** that loses nothing: the old versions updaters keep next to the
  one in use (Cursor keeps every version of its agent, half a gigabyte each)
  and app caches above 100 MB. `enforce` does them all.

`omega disk limits enforce --yes` can run from cron or launchd to keep the
budgets without asking.

## What each tool can be limited by

| Check | Kind | What omega writes or does |
|---|---|---|
| `claude-code.transcripts` | setting | `cleanupPeriodDays` in `~/.claude/settings.json`; recommended 14 days (Claude Code's default is 30) |
| `claude-desktop.vm` | budget | empties `vm_bundles`; Claude Desktop downloads the image again for the next Cowork task |
| `codex.history` | setting | `[history] max_bytes` in `~/.codex/config.toml`; recommended 20 MB |
| `codex.sessions` | budget | removes the oldest recordings in `~/.codex/sessions` |
| `docker.build-cache` | setting | `builder.gc.enabled` and `defaultKeepStorage` in `~/.docker/daemon.json`; recommended 10 GB. Docker applies it when it restarts |
| `docker.logs` | setting | `log-driver: json-file` with `max-size` and `max-file: 3` in `~/.docker/daemon.json`; for containers created afterwards |
| `docker.disk` | setting | `DiskSizeMiB` in Docker Desktop's `settings-store.json`, with Docker Desktop closed. omega refuses a size below what the disk image holds now plus 2 GB, since Docker Desktop would delete it |
| `homebrew.cleanup-days` | setting | `HOMEBREW_CLEANUP_MAX_AGE_DAYS` in `~/.homebrew/brew.env`; recommended 30 days (Homebrew's default is 120) |
| `homebrew.downloads` | cleanup | `brew cleanup --prune=all` |
| `mole.chrome-model` | action | adds `OptGuideOnDeviceModel/*` to Mole's whitelist (about 4 GB Chrome would download again) |
| `cursor.agent-versions`, `cursor.cli-versions`, `claude-code.versions`, `claude-desktop.versions` | cleanup | removes every version but the one its link points to |
| `*.cache` | cleanup | empties the app's web, code and GPU caches |
| `xcode.derived-data`, `gemini-cli.tmp` | budget | empties DerivedData; removes the oldest chats |
| `xcode.simulators`, `ollama.models` | measured | `xcrun simctl delete unavailable` and `ollama rm` are yours |

What the limits do not cover, because no tool owns it: git worktrees left
behind, build folders (`omega disk artifacts`) and large files
(`omega disk large`). The deep clean of
[`omega ultra --only deep`](tasks.md#everything-deeper-omega-ultra) goes
through all three, and says for each worktree what is already on the remote.

## omega for AI agents: the MCP server

`omega mcp` speaks the [Model Context Protocol](https://modelcontextprotocol.io)
on standard input and output, so an agent uses omega's commands instead of
improvising its own. `omega assist <tool>` registers it in Claude Code, Claude
Desktop, Codex and the ChatGPT app, Cursor, Gemini CLI and VS Code; or by hand:

```bash
claude mcp add --scope user omega -- omega mcp
codex mcp add omega -- omega mcp
```

| Tool | What it does |
|---|---|
| `omega_disk_limits` | the disk limits report (read-only) |
| `omega_assistants` | the configuration checks, of every tool or one (read-only) |
| `omega_set_limit` | `omega disk limits set <check> <value>` |
| `omega_apply_check` | `omega assist apply <check> [value]`; never one that deletes files |
| `omega_free_space` | `omega disk limits enforce`: only lists what it would free unless `dry_run` is false; marked destructive |
| `omega_doctor`, `omega_analyze`, `omega_agents` | `omega doctor`, `omega analyze --json`, `omega agents --json` (read-only) |
| `omega_large_files`, `omega_logs` | `omega disk large`, `omega logs` (read-only) |
| `omega_docs`, `omega_help` | the manual's sections for a question, a command's `--help` (read-only) |

Each tool runs the omega command a person would type, without a terminal, so
nothing can wait for an answer. Arguments are plain words: no flags, no shell.
There is no tool to run an omega task, the model loop or an arbitrary command.
The agent's client shows you each call before it runs.

## Ask the manual

The manual is inside omega:

```bash
omega docs                                  # the pages
omega docs tools                            # one page
omega docs search docker disk               # the sections that match, offline
omega docs ask "how do I stop Docker filling my disk?"
```

`omega docs ask` sends the question, the sections of the manual that match it
and the list of omega's commands to your configured model, and prints its
answer in your language with the commands to type. It answers from the manual
only. Without a key it prints the matching sections instead. `omega docs
search` understands a few words in Spanish, French and German.

`omega man` shows the man page, with every command, its options, examples and
exit statuses: through `man` at a terminal, as troff source anywhere else
(`omega man > omega.1`). `omega man --output DIR` writes `DIR/omega.1`. The
page is in English whatever the language; `omega --help` and every command's
help are in yours, with the options grouped and a "See also" line where
another command covers the rest.
