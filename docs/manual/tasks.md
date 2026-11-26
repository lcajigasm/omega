# Tasks and the guided run

[omega](../../README.md) › [Manual](README.md) › Tasks and the guided run

- [Tasks](#tasks)
- [Unattended runs](#unattended-runs)

## Tasks

| Task | What it does | Changes the system | Model (automatic) |
|---|---|---|---|
| none (`omega` alone) | checks the state, updates everything, cleans up and summarises | yes | fast → smart on failure |
| `quick` | one-click safe maintenance: trash, caches, updates check, health | yes | fast → smart on failure |
| `update` | updates only (like topgrade) | yes | fast → smart on failure |
| `clean` | measures what takes up space and cleans (like `mo clean`) | yes | fast → smart on failure |
| `analyze` | report, then a card with the recommendations | only what you pick | fast → smart on failure |
| `health` | disk and battery health, memory, backups, firewall, encryption, automatic updates and the system logs | only what you pick | fast → smart on failure |
| `tune` | reviews your tools' configuration against the current docs | only what you pick | fast → smart on failure |
| `fix` | runs the topgrade and Mole dry runs, diagnoses and fixes their config | yes (with approval) | fast → smart on failure |

`omega mac` and `omega dev` are guided tasks of the same kind, for what runs on
your Mac and for your development setup. `omega menu` lists everything on one
screen and starts what you choose.

Updating an app on a Mac closes it: Homebrew and the App Store quit an app to
replace it and do not open it again, so a menu bar app disappears from the menu
bar until the next login. omega notes which apps of the pending updates are
open, and once the update is done it opens again, in the background, the ones
that were closed. The uninstall step of some casks (Alfred, Tailscale, OpenVPN
Connect) also removes their login item; putting it back would need a macOS
permission only you can give, so the Homebrew step names those and says where
to turn "open at login" on again.

### Everything, stage by stage

```bash
omega full                      # the plan first, then one stage at a time
omega full --skip tune,dev      # the same, with two stages unticked from the start
omega full --only update,clean  # only these two ticked
omega full --dry-run            # every stage in read-only mode
omega unattended                # everything, no questions: the sudo password once, then all approved
omega full --yes --skip mac     # unattended: every other stage, one after another
omega full --no-tui             # the same run, line by line
```

`omega full` is the run that leaves the machine better than it found it, and
it is one interface from start to end. It opens on the plan: every stage that
applies to this machine, with what it does, why it is worth doing at that
point and what it can change. Untick the ones you do not want and press Enter.

| Stage | What it does | When |
|---|---|---|
| `doctor` | checks that omega itself is set up, without spending tokens | always |
| `analyze` | a read-only report: disk space, outdated software, package manager problems | always |
| `health` | disk and battery health, memory, backups, firewall, encryption, automatic updates and the system logs | always |
| `update` | updates everything installed, and fixes what fails | always |
| `clean` | frees disk space, biggest wins first | always |
| `docker` | frees Docker space, only what you pick | Docker is installed |
| `tune` | reviews the configuration of your tools against the current docs | always |
| `dev` | reviews languages, version managers and developer tools | always |
| `mac` | reviews what runs on your Mac outside your apps | macOS |
| `fix` | repairs topgrade and Mole when they fail | topgrade or Mole is installed |
| `history` | picks up what earlier runs left failed or undone | omega has run here before |

The stages run one after another on the same screen. Each one opens with a
heading that repeats what it does and why, then its session, with the same
approvals and cards as the command on its own. The route is always in view:
the panel lists every stage with how it went (`✓` done, `✗` failed, `■` stopped,
`↷` skipped, `·` to come), and on a narrow terminal it becomes one line at the
top.

Between two stages a card says how the last one ended and what comes next:

| Key | What it does |
|---|---|
| `Enter` | starts the next stage |
| `s` | skips it |
| `r` | repeats the last stage, when it failed or you stopped it |
| `p` | opens the plan again, to tick or untick what has not run |
| `q` | ends the run here |

A stage ends when its work does, and the card for the next one appears: you
never type `/exit` to move on. What a stage needs from you it asks on cards
while it runs, never as a question in its last message. `ctrl+c` stops that
stage only. A stage that fails does not stop the run. The run ends
with a summary of what was done, skipped, stopped and failed, with the time
and the cost of each stage and the command that runs a pending one on its own;
the same summary stays in the terminal after omega leaves.

Without a terminal (a scheduled job) `omega full --yes` runs every ticked
stage and exits with an error when one failed; `ctrl+c` stops the stage and
the run.

`omega` alone is still the quick maintenance (check, update, clean, summarise)
in a single session. `omega full` used to be another name for it and is now the
guided run; `omega all` and `omega everything` start it too.

omega takes more than the exact names. All of these work:

| You type | omega runs |
|---|---|
| `omega upgrade`, `omega actualizar`, `omega upd` | `omega update`: other words for a task in the five languages, or the start of its name when only one task starts that way |
| `omega cleanup`, `omega repair`, `omega optimize`, `omega report`, `omega security` | `omega clean`, `fix`, `tune`, `analyze`, `health` |
| `omega perms`, `omega apps`, `omega undo`, `omega dock` | `omega mac permissions`, `mac apps`, `mac undo`, `mac dock` |
| `omega install fzf mise` | `omega dev install fzf mise` |
| `omega historial`, `omega settings`, `omega keys`, `omega status` | `omega history`, `config show`, `auth status`, `doctor` |
| `omega doc`, `omega mac perm` | nothing if the start is ambiguous (`doc` could be `docker` or `doctor`); the command when it is not (`omega mac perm` is `omega mac permissions`) |
| `omega why is my disk full` | the same as `omega -p "why is my disk full"`: three words or more are a request for the agent |
| `omega menuu` | an error that says `Did you mean omega menu?` |

One or two words that omega does not recognise are never guessed at, so a
typo cannot start a paid session.

You can also give it a free-form task:

```bash
omega -p "Why is ~/Library 40 GB? Tell me what I can safely delete"
omega -p "Only update Homebrew and npm, nothing else"
omega -p "topgrade fails on the rustup step, fix it"
```

### Everything, deeper: omega ultra

```bash
omega ultra                     # every stage, deeper, on stronger models
omega ultra --only deep         # only the deep clean
omega ultra --skip update,mac   # everything else
```

`omega ultra` is the deepest pass omega makes, and you are there for all of
it. It runs every stage of `omega full` and two that only ultra has:

| Stage | What it does |
|---|---|
| **Deep clean** (`deep`) | What the cleaning steps never reach. The git worktrees AI agents (Claude Code, Codex, Cursor) and you created, each with a verdict: *safe* when the only uncommitted files are the agent's own (`PLAN.md`, `.claude-run.json`, `.pr-body.md`, `*.codex.txt`…) and nothing else is lost, *review* when it holds work or was used in the last two days, *keep* when a running process works in it, it is locked or omega runs from it. The run files agents leave next to your projects. Docker images, stopped containers and volumes one by one: an image built on this machine is never taken with the others, and a volume is never recommended. Build folders of projects idle two weeks, merged branches, `git gc`, disk limits and old versions, and large and duplicate files (reported, never offered). |
| **Every review** (`care`) | What closed agent sessions left running, the configuration assistants of every tool, SSH keys and secrets in shell histories, privacy, the network, agent skills and what is pending from earlier runs. |

How it differs from `omega full`:

- **Stronger models.** Each stage starts on the provider's smart model with
  effort `high` (or yours, if higher) and moves to its most capable model when
  a step fails; the deep clean starts on the most capable one. With Anthropic
  that is Sonnet, then Opus. Set another with
  `providers.<name>.ultra_model` in `config.toml`, or fix one with `--model`.
  It costs more than `omega full`; `--max-cost` still applies.
- **Evidence for every change.** The model is told to verify before it
  proposes, and omega enforces it: a card whose options change something
  without an `evidence` line (the PR state, the files git reports, who
  recreates a cache) is refused, and the evidence is shown on the card you
  answer.
- **Checked again before acting.** Each deletion of the deep clean is looked
  at again just before it runs and refused if it changed (a worktree that got
  new work, a process that started in it). A worktree's uncommitted changes
  are saved as a patch, its untracked files copied and a commit on no branch
  given the branch `omega/rescued/<name>`, all under
  `~/.local/state/omega/rescued/`, before it goes; its branch always stays.
  Agent files go to the Trash.
- **Every change is asked.** The approval policy is `all` for the run, and
  `--yes` and `--auto` are refused. It needs a terminal: for runs nobody
  watches, use `omega full --unattended`.

## Your own routine

`omega custom` runs a routine you put together: the steps you pick, in the
order you put them. It runs them on this machine with the same approval
rules as `omega steps run`, and it never calls an AI model, so nothing about
the machine leaves it (the steps themselves may still download updates).

```bash
omega custom --set                        # choose steps and order in an editor
omega custom --set disk brew npm-cache    # or name them directly
omega custom --show                       # the routine, without running it
omega custom                              # run it
omega custom --dry-run                    # what it would run
```

### The editor

`omega custom --set` (with a routine name or without one) opens a full-screen
editor: the routine on top, in running order, and every other installed step
below, by category.

| Key | What it does |
|---|---|
| space, `x` | add the step under the cursor to the routine, or take it out |
| ⇧↑ / ⇧↓, `K` / `J` | move a step of the routine up or down |
| `f` | stop the run if this step fails (the step must be in the routine) |
| `b` | run the routine only below N GB free: type the number, enter keeps it, an empty field means always |
| `w` | run a scheduled routine only on some days: type `mon,wed,fri` (or the short names of your language: `lun,mié,vie`, `lun,mer,ven`, `mo,mi,fr`, `seg,qua,sex`); an empty field means every day |
| `i` | ask your AI model for a recommendation |
| `d` | turn on or off sending machine data with that recommendation |
| `c` / `u` | empty the routine / undo every change since it opened |
| enter, `s` | save; esc or `q` leaves, and asks first when something changed |

Each step shows the figures of the last time `omega custom` ran it: how long
it took and how much space the disk gained (`1m05s · 2.1 GB`). The header of
the routine shows its options (`only below 30 GB free · only on mon, fri`).

`i` sends the model the operating system, the installed steps (id, category,
risk and what each does) and your current routine, nothing else, and only
when you press it. With `d` on, it also sends three aggregated figures, which
the status line spells out before you ask: the free space of the system disk
rounded to whole GB, the days since `omega custom` last ran a step, and the
ids of the steps that failed in the runs kept. Nothing names you, a file or a
path. The recommendation shows each step with a reason; `a` takes it (you can
still edit it), esc keeps yours and leaves the reasons as ✦ notes beside the
steps.

The routines live in `~/.config/omega/custom.toml`, so a dotfiles repository
can carry them to other machines; steps a machine lacks are passed over there
and kept in the file.

### Several routines

A routine can have a name. A first word after `--set` that is not a step id
names the routine; the one without a name is called `default`:

```bash
omega custom --set weekly brew mole-clean npm-cache   # a routine called weekly
omega custom --list                                   # every routine, the default marked
omega custom weekly                                   # run it
omega custom weekly --show                            # show it
omega custom weekly --default                         # omega custom now runs weekly
omega custom weekly --clear                           # delete it
```

### Options of a routine

Three options belong to a routine. They are set in the editor or with
`--set`, with or without step ids (without ids only the options change):

- `--stop-on-fail brew,npm` (key `f`): when one of these steps fails, the
  routine stops there and the steps after it do not run;
- `--only-below-free-gb 30` (key `b`): the routine runs only while the system
  disk has less than 30 GB free. With more, it says so and does nothing,
  which is not a failure. `0` turns it off;
- `--days mon,wed,fri` (key `w`): a scheduled run of the routine runs only on
  these weekdays; on the others it says so and does nothing, which is not a
  failure. A run you start by hand runs on any day and only warns.
  `--days ""` clears it. The days are stored as `mon` … `sun` whatever
  language you typed them in.

```bash
omega custom --set weekly --stop-on-fail brew --only-below-free-gb 30 --days mon,thu
```

```toml
# ~/.config/omega/custom.toml
default = "weekly"

[routines.weekly]
steps = ["brew", "mole-clean", "npm-cache"]
stop_on_fail = ["brew"]
only_below_free_gb = 30.0
days = ["mon", "thu"]
```

`--show` and `--list` show the options in words beside each routine;
`--show --json` and `--list --json` give them as stored. `omega steps show
<id>` lists the last runs of a step by `omega custom`. An older
`custom.toml` with a single list of steps is read as the routine `default`.

### Scheduling, other machines and the desktop app

`omega schedule install --task custom` runs the default routine on a
schedule, like any other task without AI: `--yes` answers the steps that
need it, and the steps that always ask are skipped and named. With
`--interval daily`, the routine's `days` choose the days it really runs.

`omega export` puts `custom.toml` in the blueprint, and `omega bootstrap`
writes it on the other machine after a yes. Routines that differ from the
ones already there replace them only after a yes at the terminal or with
`--force` (the old file is kept as `custom.toml.bak`); `--yes` alone keeps
them.

In the desktop app, **System** has a **Your routines** card: pick a routine
(the default is marked) or name a new one, then order its steps with the ↑ ↓
buttons, take one out with ✕, add an installed step from the list, tick
"stop if it fails" per step, set the free-space limit and the days, and
press **Save routine**. The card writes the same `custom.toml`; running a
routine stays in the terminal (`omega custom <name>`).

### Which steps always ask

A step asks every time, even with `--yes`, when the catalogue says so (the
Docker cleanups, `mole-purge`) or when its command is one the safety policy
always asks about: `mo purge --yes` deletes `node_modules`, `.venv` and
`target`, and a script run from your home folder could do anything. This
holds for omega's own steps and for yours. The one exception is an omega step
that asks only because it runs `sudo` (the macOS update, apt, dnf, pacman,
snap): the sudo password is the question, and a machine where sudo needs no
password keeps its scheduled updates working. A step of yours in `steps.d`
that runs `sudo` always asks.

## Unattended runs

### Everything, with one password

```bash
omega unattended                 # the same as omega full --unattended (or -U)
omega unattended --skip docker   # --only and --skip still choose the stages
omega desatendido                # the same, in Spanish
```

`omega unattended` is upkeep with nobody at the keyboard: it checks the
machine, updates what is installed and clears caches. It asks for the sudo
password once, before the first stage, checks it and keeps it in memory until
the run ends; if sudo needs no password on this machine, it asks nothing.
Then the upkeep stages that apply here run, one after another: `doctor`,
`analyze`, `health`, `update`, `clean` and `docker`.

The other stages of `omega full` (`tune`, `dev`, `mac`, `fix` and the loose
ends of `/history`) change how you set up the machine: what starts at login,
the menu bar, your apps, the configuration of your tools and your shell,
decisions an earlier run left for you. A person decides those, so an
unattended run leaves them out and names them; asking for one with `--only`
is an error. Run them yourself when you are at the machine.

- every approval is answered yes, as with `--yes`, and the low-risk upkeep is
  applied, as with `--auto`;
- every option a stage would offer on a card is taken, so the model is told to
  offer only the upkeep it recommends for this machine;
- blocked commands still never run, and the actions that always need a person
  (deleting Docker volumes or images you may want to keep) are skipped and
  listed in the summary as things to do by hand;
- how you set up the machine is never changed. The run has no tool to write a
  configuration file, change what runs on the Mac, install language versions
  or change omega's settings, and it refuses the commands and steps that
  would: installing, removing or reinstalling apps or tools; quitting or
  killing apps; login items, launch agents and daemons; the menu bar, the
  Dock (or the taskbar or panel), keyboard shortcuts and the wallpaper; system
  settings (power, network, firewall, FileVault, Time Machine, the login
  shell, default apps); the preferences of the system and of apps; dotfiles
  and the global configuration of your tools; emptying the Trash; Mole's
  `optimize` (which repairs login items and background services) and
  topgrade (which upgrades apps with its own steps). The summary lists what
  it would suggest instead;
- updating an app can close it or take it out of what starts at login:
  Homebrew quits an app to replace it and does not open it again (a menu bar
  app disappears from the menu bar), and the uninstall step of some casks
  removes their login item or background service (Alfred, Tailscale,
  Dropbox, 1Password…). So the Homebrew step leaves out the casks whose app
  is open and those whose update removes a login item or a background
  service, the App Store step leaves out the apps that are open, and both
  name them: they are updated the next time you run `omega update` at the
  machine, which opens again what the update closed. Formulae, tools and closed apps are updated as usual;
- macOS privacy permissions are not asked for; a stage that needs one says so
  in its summary;
- each stage builds on the ones before it: it is told what they concluded,
  so it does not measure, redo or offer again what an earlier stage already
  did, and the read-only results they measured (the disk scan, the dry runs
  of the catalogue steps) are reused instead of run again. Anything that may
  change the machine (a command, a written file, a hook of yours, the
  built-in steps) throws those results away for every stage after it;
- a stage that fails does not stop the ones after it; Ctrl+C stops the run.

At a terminal the run has a screen of its own, since nobody is going to type
anything into it (here a stage has just failed a step, and the mascot winces):

```text
 Ω omega  unattended run  dev · claude-haiku-5-5 · demo, nothing real runs        45s · $0.0156
 ━━━━━━━━━━━━━━━━━━━━━━━━ ━━━━━━━━━━━━━━━━━━━━━━━━ ──────────────────────────  1/3 · about 40s left

                       !
          ▄▄██████▄▄     !
       ▄█▀▀        ▀▀█▄       ╭─ Stage 2 of 3 · Update ─────────────────────── omega update · 21s ─╮
      ██              ██    ╶─┤ Disk is at 91% and 6 Homebrew packages are outdated. Updating      │
     ██   ▀▄      ▄▀   ██     │ first, then the npm globals and pipx.                              │
     ██   ▄▀      ▀▄   ██     ╰────────────────────────────────────────────────────────────────────╯
      ██      ╭╮      ██
       ▀█▄          ▄█▀       ✶ Step(npm)  2s
    ▄▄▄▄▄██        ██▄▄▄▄▄      └ added 3 packages in 2s
    ▀▀▀▀▀▀▀        ▀▀▀▀▀▀▀

 ROUTE ────────────────────── 1/3   DONE SO FAR ───────────── actions: 1 done · 1 failed · 1 denied
  ✓ Check omega                4s   ─ Check omega ─────────────────────────────────────────────────
 ▌✶ Update                    21s   ─ Update ──────────────────────────────────────────────────────
    omega update  ✓1 ✗1 ⊘1            ✓ Step(brew)                                               4s
  · Clean up                          ✗ Step(pipx) · Error: python@3.12 is not installed     exit 1
                                      ⊘ Bash(docker volume prune -f) · Unattended run: this  denied
 FOR YOU LATER ──────────────── 2     ✶ Step(npm)                                                2s
  ✗ Step(pipx)
    Update · Error: python@3.12
    is not installed
  ⊘ Bash(docker volume prune -f)
    Update · Unattended run: this
    action always needs a person.
────────────────────────────────────────────────────────────────────────────────────────────────────
 ctrl+c stop the run · s skip this stage · ↑↓ scroll
```

- at the top, the progress of the run: one segment per stage, green when it
  went well, red when it failed, with a pulse running along the one that is
  running; beside it, how many have ended and about how long the rest will
  take, from how long the ones that ran took;
- the mascot is the letter Ω itself, with a face, and it never sits still:
  while a stage works a glint runs along its arc, its feet tap, its eyes look
  around and blink and sparkles twinkle around it; it looks up with thought
  bubbles while the model thinks, winces and flashes red at a failure, jumps
  among confetti when a stage ends well, and at the end of the run is pleased
  or sorry, depending on how it went. A terminal under 26 lines or 80 columns
  gets a small version of it, and `OMEGA_ASCII=1` or `TERM=dumb` one drawn in
  plain characters; `OMEGA_REDUCED_MOTION=1` (or `NO_COLOR`) keeps it still;
- its speech bubble says what the run is doing, in the model's own words, with
  the stage it is on, its command and how long it has taken; under it, the
  command that is running and the last line it printed;
- **Route** is every stage and how it went; under the one that is running, its
  command and how many of its actions went through, failed or were refused;
- **Done so far** is every action of the run, stage by stage, with the totals;
- **For you later** is what did not go through (a step that failed, an action
  that needs a person), with the stage it was in and why: what to look at when
  you come back. A terminal under 84 columns leaves it out (the report at the
  end still has it);
- **s** ends the stage that is running and goes on with the next one (it is
  listed as stopped, not failed); ↑↓, PgUp and PgDn scroll back through
  **Done so far**, and End follows the run again;
- the terminal's title says how far the run has got and which stage is
  running, and keeps the verdict (✓ or ✗) when the run ends; the screen stays
  up a moment to show it, and the terminal's bell rings to mark the tab it was
  left in;
- Ctrl+C stops the stage that is running and the run with it; a second Ctrl+C
  leaves at once.

When the run ends the screen closes and leaves on the terminal what each stage
concluded, what failed or was refused in it, and the summary of every stage,
so it is all there when you come back. Without a terminal (a pipe, cron,
launchd), with `TERM=dumb` or with `--no-tui`, the run prints lines instead,
and each stage's line says the time it started, for the log you read later.

It exits with status **1** if any stage failed. With `--dry-run` nothing changes and no password is asked. Without
a terminal (from launchd or cron) there is nobody to type the password, so the
steps that need sudo fail and are listed.

### One task, from a schedule

`omega schedule install` sets this up for you and runs the built-in steps
without the model. A job you write yourself, like the one below, runs the
agent with every approval answered yes and nobody watching; add `--no-ai` to
keep it to the built-in steps.

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

Unattended runs never clean Docker (that always needs you), skip selection cards
(the recommendations go into the summary) and skip anything that needs sudo.
A model error or a turn limit exits with status **1**. Cancelling (Ctrl+C or
SIGTERM) exits with status **130**. A run you end yourself exits with **0**.
