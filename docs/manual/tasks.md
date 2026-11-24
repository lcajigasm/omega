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

`omega unattended` is the guided run with nobody at the keyboard. It asks for
the sudo password once, before the first stage, checks it and keeps it in
memory until the run ends; if sudo needs no password on this machine, it asks
nothing. Then every stage that applies here runs, one after another:

- every approval is answered yes, as with `--yes`, and the low-risk
  improvements are applied, as with `--auto`;
- every option a stage would offer on a card is taken, so the model is told to
  offer only what it recommends for this machine;
- blocked commands still never run, and the actions that always need a person
  (deleting Docker volumes or images you may want to keep) are skipped and
  listed in the summary as things to do by hand;
- what you arranged yourself is never changed: the Dock (or the taskbar or
  panel) and what is pinned there, the menu bar, keyboard shortcuts, the
  wallpaper and the apps you installed. The run is upkeep, so omega rejects
  those changes in this mode and the summary lists them as suggestions; apply
  the ones you like with `omega mac`;
- macOS privacy permissions are not asked for; a stage that needs one says so
  in its summary;
- a stage that fails does not stop the ones after it; Ctrl+C stops the run.

At a terminal the run has a screen of its own, since nobody is going to type
anything into it (this one is `omega unattended --demo --only update,clean`):

```text
 ✻ omega unattended run  dev · claude-haiku-5-5 · demo, nothing real runs         45s · $0.2207
 ✓✻  1/2  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

            .             ╭────────────────────────────────────────────────────────────────────╮
      .-======*==-.       │ Stage 2 of 2 · Clean up  omega clean · 13s                         │
    .'             '.   ◂ │ Disk is at 91% and 6 Homebrew packages are outdated. Updating      │
   /                 \    │ first.                                                             │
  |   ( o )   ( o )   |   ╰────────────────────────────────────────────────────────────────────╯
  |                   |
   \     \_____/     /      ✶ Step(pipx) 1s
    '.             .'         ⎿ Error: python@3.12 is not installed
      '.         .'
  '-----:       :====='

 ROUTE                      1/2 │ DONE SO FAR            actions: 14 done · 1 failed · 1 denied
 ✓ Update                   32s │  ✓ Disk(~/Library)                                         4s
 ✶ Clean up                 13s │  ✓ Step(brew-cleanup)                                      3s
                                │  ⊘ Step(docker-build-cache) · Unattended run: this ac… denied
                                │  ✓ Step(xcode-derived)                                     2s
                                │  ✓ Step(npm-cache)                                         3s
                                │ ── Clean up
                                │  ✓ System
                                │  ✓ Step(brew-outdated)                                     4s
                                │  ✓ Step(disk)                                              4s
                                │  ✓ Step(brew)                                              5s
                                │  ✶ Step(pipx)                                              1s

 Nobody needs to answer anything: every stage runs on its own · ctrl+c stops the run
```

- the mascot, the letter Ω itself with a face inside it, never sits still
  while a stage runs (a glint runs along its arc, its feet step, its eyes
  look around and blink, its mouth moves) and says what the run is doing, in
  the model's own words, with the stage it is on; its face follows the run
  (working, thinking, wincing at a failure, pleased or sorry at the end). A
  terminal under 26 lines or 72 columns gets a small version of it;
- under it, the command that is running and the last line it printed;
- **Route** is every stage and how it went; **Done so far** is every command
  that has run, failed or been refused, with the totals;
- Ctrl+C stops the stage that is running and the run with it; a second Ctrl+C
  leaves at once.

When the run ends the screen closes and leaves on the terminal what each stage
concluded, what failed or was refused in it, and the summary of every stage,
so it is all there when you come back. Without a terminal (a pipe, cron,
launchd), with `TERM=dumb` or with `--no-tui`, the run prints lines instead.

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
