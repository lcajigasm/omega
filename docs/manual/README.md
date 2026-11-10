# omega manual

[omega](../../README.md) › Manual

Everything the [README](../../README.md) leaves out: each task, each system,
every step and every setting.

| Page | What is in it |
|---|---|
| [Install and set up](install.md) | Homebrew, binaries for the three systems, the container image, the desktop app, building from source, `omega setup` |
| [Tasks and the guided run](tasks.md) | What each task does, `omega full` stage by stage, unattended runs (cron, launchd, CI) |
| [The interface](interface.md) | The full-screen interface, the session panel, shortcuts, slash commands, selection cards |
| [Models and cost](models.md) | How the two models are chosen, prices, fixing a model, local models |
| [Your tools](tools.md) | `omega tune`, `omega dev` and `omega docker` |
| [Your Mac](mac.md) | Login and background items, menu bar, Dock, shortcut conflicts, privacy permissions, apps and leftovers |
| [Linux](linux.md) | Exactly what works on Linux and what does not |
| [Windows](windows.md) | Exactly what works on Windows and what does not |
| [Safety, approvals and memory](safety.md) | The risk levels, what always asks, what never runs, notes between runs |
| [Reference](reference.md) | Step catalogue, languages, configuration file, command reference |
| [Troubleshooting and FAQ](help.md) | What to do when something does not work |
| [How it works, development and releasing](development.md) | The agent loop, the code layout, the desktop app, how a release is made |

## Everything omega does

- **Updates everything**: Homebrew (formulae and casks), Mac App Store, npm,
  pnpm, bun, pipx, uv, rustup, cargo, `gh` extensions, Oh My Zsh and macOS.
- **Cleans up**: Homebrew, npm/pnpm/pip/uv/Go caches, Xcode DerivedData,
  unavailable simulators, Docker and the Trash, measuring first and prioritising
  what frees the most.
- **Fixes what breaks**: reads the failure, checks the config behind it, applies
  the smallest fix and retries once. When Docker is not running it offers to
  start it, and when a cleanup needs a tool you do not have it offers to install it.
- **Reads the system logs**: crash reports, launch agents that keep failing,
  processes preventing sleep, apps flooding the log and log folder sizes, with
  fixes offered on a card.
- **sudo inside the interface**: when Mole, `softwareupdate` or any command needs
  administrator access, omega asks for the password on a card instead of the
  terminal.
- **Selection cards**: recommendations arrive as a card where you tick one or
  several actions, and omega carries out exactly those.
- **Model choice from live data**: a fast model for routine work that escalates
  to a stronger one on the first failure. Which two is decided once a day from
  current list prices and benchmark scores (`omega models`), not from a list
  built into the program. Tokens and cost are always visible.
- **Ask in words**: inside a session the AI can do everything the command line
  does: install developer tools, undo a Mac change, read earlier runs, change
  settings. API keys, the provider and the approval policy stay yours alone.
- **Local models**: Ollama, LM Studio or any OpenAI-compatible endpoint, free and
  private.
- **Configuration tuning** (`omega tune`): reviews the config of your installed
  tools, reads the current official docs on the web and proposes improvements,
  guided or automatic.
- **Docker cleanup** (`omega docker`): shows what Docker uses and cleans only what
  you select. Docker cleanups always need your confirmation.
- **Control what runs on your Mac** (`omega mac`): apps that open at login, jobs
  allowed in the background, menu bar apps, the Dock and **global keyboard
  shortcut conflicts** (ChatGPT, Claude, Alfred and macOS all wanting ⌥Space).
  Your rules plus sensible defaults decide what to suggest, the AI explains what
  it does not know, your choices are remembered and every change can be undone.
- **Privacy permissions** (`omega mac permissions`): finds the entries that
  System Settings → Privacy & Security keeps for binaries whose path no longer
  exists (every update of a bundled `node` leaves another one) and for apps that
  are gone, and removes the ones you pick.
- **Apps and leftovers** (`omega mac apps`): apps you have not opened in months,
  apps still built only for Intel, and what removed apps left in `~/Library`.
  Removals go to the Trash and can be undone.
- **Developer tools** (`omega dev`): knows more than 70 languages, version managers and
  command-line tools. It shows what is installed and by which manager, finds
  overlapping version managers, shadowed commands and a broken `PATH`, installs
  what you pick and adds the shell line a tool needs to `~/.zshrc` or `~/.bashrc`.
- **Safety policy**: every command is classified (read-only, normal, dangerous,
  blocked); dangerous ones need your approval.
- **Memory**: learns from each run and takes it into account next time.
- **Full-screen TUI** in the style of Claude Code, with a **session panel**
  (steps done and failed, disk space freed, tokens, activity), a **home screen**
  (`omega menu`) and commands to start another task without leaving, or
  **plain text** for scripts, cron and CI.
- **Keyboard shortcuts and slash commands**: `shift+tab` changes the approval
  mode and `ctrl+p` the model without stopping the run; type `/` for a command
  menu (`/mode`, `/model`, `/help`, `/exit`…) that explains each one as you type.
- **Five interface languages**, a complete log of every run, API keys in the
  macOS Keychain, the Windows Credential Manager or the Linux system keyring,
  and a single Go binary.
- **macOS, Linux and Windows.** The same agent with each system's own steps:
  Homebrew, `softwareupdate` and the Mac App Store on a Mac; apt, dnf, pacman,
  zypper, Flatpak and snap on Linux; winget, Scoop and Chocolatey on Windows.
  See [Linux](linux.md#linux) and [Windows](windows.md#windows) for exactly what works on each.
- **Everything, stage by stage** (`omega full`): a guided run through all of
  it in one interface. A plan says what each stage does and why; you untick
  the ones you do not want, watch the route as they run, and skip, repeat or
  stop between them.
