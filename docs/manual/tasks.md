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
| `fix` | runs the topgrade and Mole dry runs, diagnoses and fixes their config | yes (with approval) | smart |

`omega mac` and `omega dev` are guided tasks of the same kind, for what runs on
your Mac and for your development setup. `omega menu` lists everything on one
screen and starts what you choose.

### Everything, stage by stage

```bash
omega full                      # the plan first, then one stage at a time
omega full --skip tune,dev      # the same, with two stages unticked from the start
omega full --only update,clean  # only these two ticked
omega full --dry-run            # every stage in read-only mode
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
| `omega history`, `omega settings`, `omega keys`, `omega status` | `omega logs`, `config show`, `auth status`, `doctor` |
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

## Unattended runs

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
