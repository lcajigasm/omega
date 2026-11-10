# Your Mac

[omega](../../README.md) › [Manual](README.md) › Your Mac

- [Your Mac: login, background, menu bar, Dock and shortcuts](#your-mac-login-background-menu-bar-dock-and-shortcuts)
- [Privacy permissions, apps and leftovers](#privacy-permissions-apps-and-leftovers)

## Your Mac: login, background, menu bar, Dock and shortcuts

Apps add themselves to login, keep helpers running in the background, fill the
menu bar, pin themselves to the Dock and take global keyboard shortcuts that
other apps already use: ChatGPT, Claude, Copilot, Alfred and Raycast all default
to ⌥Space, Things to ⌃Space (macOS's input source switch). `omega mac` puts you
back in charge.

```bash
omega mac               # guided review with AI: cards for each area, applies what you pick
omega mac --auto        # the AI applies the low-risk suggestions itself and offers the rest
omega mac status        # overview of every area, changes nothing
omega mac login         # apps that open at login
omega mac background    # launch agents, daemons, app helpers and background tasks
omega mac menubar       # menu bar apps: quit, stop at login, hide
omega mac dock          # unpin apps that are gone or unused, Dock settings
omega mac shortcuts     # who keeps each contested shortcut
omega mac permissions   # privacy entries of binaries and apps that are gone
omega mac apps          # unused apps, Intel-only apps and leftovers
omega mac policy        # your rules and omega's defaults (policy edit to change them)
omega mac undo          # revert any change
```

The subcommands work without AI. Each one shows a card with omega's suggestions
already ticked; you untick or tick more and press Enter.

**Defaults.** omega sorts every app and job into a category and suggests from it:

| Category | Examples | Suggestion |
|---|---|---|
| updater | Google Updater/Keystone, Adobe, Microsoft AutoUpdate, Zoom | off at login and in the background: the app or Homebrew updates it |
| open when needed | ChatGPT, Claude, Slack, Spotify, Discord, Docker Desktop, browsers, editors | not at login, not in the background |
| essential | VPNs, 1Password, Little Snitch, Dropbox, Logitech, Karabiner, Alfred, Raycast, Bartender, Ice, Thaw | keep |
| Apple | `com.apple.*` | never touched |
| unknown | anything else | the AI works out what it is (developer, path, web search) and recommends |

Launch jobs whose program is no longer installed are always suggested for
removal, as are Dock apps that are gone; Dock apps not opened in 90 days are
offered (not ticked).

**Keyboard shortcuts.** omega reads macOS's own shortcuts
(`com.apple.symbolichotkeys`), the shortcuts apps store with the
KeyboardShortcuts and MASShortcut libraries (Shottr, Rectangle and many more),
and the known defaults of popular apps that only store a shortcut once you change
it. When two want the same combo, it suggests who keeps it: essential macOS
shortcuts first (screenshots, Mission Control), then shortcuts you set on purpose,
then launchers and utilities, then the rest of macOS, and last the apps that
claim a shortcut on install. You choose on a card; the others move to a free
combo (⌃⌥Space, ⌥⇧Space…), and omega writes it where it can or opens the app and
tells you where to change it. Input source shortcuts on a Mac with a single input
source count as unused.

**Your rules** live in `~/.config/omega/mac.toml` and always win over the
defaults. `omega mac shortcuts` and the AI (with the `mac_policy` tool) save your
decisions there, so the next review suggests the same:

```toml
[startup]
keep = ["Claude", "net.mullvad.*"]       # app name, bundle id or launchd label; * is a wildcard
disable = ["com.google.keystone.*"]

[menubar]
hide = ["Docker"]

[dock]
remove = ["App Store"]
add = ["/Applications/Ghostty.app"]
autohide = true
show_recents = false

[shortcuts.owners]
"opt+space" = "ChatGPT"
"ctrl+space" = "macOS"
"opt+shift+cmd+space" = "none"           # nobody: turn it off everywhere
```

**How changes are made.** Every change is a visible command and has an inverse,
recorded in `~/.local/state/omega/mac-journal.jsonl` for `omega mac undo`:

| Change | Command |
|---|---|
| launch agent or helper off | `launchctl bootout` + `launchctl disable gui/<uid>/<label>` (daemons need sudo, asked on the password card) |
| app no longer opens at login | System Events `delete login item`; an app that adds itself to the list is not there, so omega opens the Login Items pane and says so |
| macOS shortcut | `defaults write com.apple.symbolichotkeys … -dict-add <id>` + `activateSettings -u`, no logout needed |
| app shortcut (KeyboardShortcuts, MASShortcut) | quit the app, `defaults write`, open it again |
| Dock | edits `persistent-apps` and restarts the Dock |

Some things macOS only lets you change by hand: an app's switch under "Allow in
the Background", an app that adds itself to the login items, what is left over
in the system-wide privacy lists, menu bar visibility, and shortcuts of apps
that keep them in their own storage. omega knows which ones they are before it
tries: the agent does not offer them as changes it will apply, it lists them
together as what is left for you, opens the right settings pane or app once and
tells you what to change. `disable:<bundle id>` reaches the item that is on:
an app and its background entry share the id, and macOS keeps a switched-off
record for every app that once asked to open at login. Reading the login and background items uses `sfltool dumpbtm`, which
can take up to a minute; omega caches it for an hour and refreshes it after a change.

## Privacy permissions, apps and leftovers

### Permissions that point at nothing

macOS records a permission for a command-line binary by its path. An app that
ships its own `node` in a folder named after its version (Claude Desktop does)
gets a new entry with every update, and the old ones stay: System Settings →
Privacy & Security ends up with a row of items all called `node` and nothing to
tell them apart. Uninstalled apps leave their entries too.

```bash
omega mac permissions
```

omega reads the privacy database (TCC), names every binary with the version
found in its path (`node 2.0.14`), and marks:

| Entry | Suggestion |
|---|---|
| a binary whose path no longer exists | remove (ticked) |
| a bundle id with no app in `/Applications`, inside another app, in Application Support or known to Spotlight | review (not ticked) |
| Apple's own entries, and anything that still exists | keep |

Removing an entry cannot be undone, and it is harmless: whatever needs the
permission asks again. For each entry you pick, omega:

1. runs `tccutil reset <permission> <client>`, Apple's tool. It only accepts a
   bundle id macOS still knows, so it refuses a binary's path and an uninstalled
   app: the very entries worth removing;
2. if the entry is still there and belongs to your own database, saves a copy
   of the database (`~/.local/state/omega/tcc-backup-<date>.db`) and deletes
   that one row;
3. reads the database again. An entry that is still listed is reported as a
   failure, never as done, and omega opens the settings pane where you can
   select it and press –.

Step 2 and reading your own entries (Files & Folders, Automation, Camera, data
from other apps) need **Full Disk Access** for the terminal that runs omega.
Without it omega says so and lists only the system-wide permissions (Full Disk
Access, Accessibility, Screen Recording, Input Monitoring, Developer Tools).
Those live in a database macOS protects from every app, so omega can find the
stale ones but you remove them in System Settings: for those omega does not try
and fail, it opens the pane and names the entry to select. The AI cannot touch either
database: a command that writes to `TCC.db` is blocked. The Local Network list
is stored elsewhere and is not covered.

### Apps and what they leave behind

```bash
omega mac apps
```

- **Apps to review**: not opened for 180 days (Spotlight's last-used date), or
  with no Apple silicon build on an Apple silicon Mac. Each shows its size and
  whether it came from Homebrew or the App Store. Nothing is ticked: you decide
  which apps go. A cask is removed with `brew uninstall --cask`; any other app
  is moved to the Trash.
- **Leftovers**: folders and files in `~/Library` (Application Support, Caches,
  Preferences, Saved Application State, HTTPStorages, WebKit, Logs) named after
  the bundle id of an app that is not installed, from 5 MB up. They are moved to
  one folder in the Trash; `omega mac undo` puts every file back. Leftovers of a
  vendor that still has another app installed are flagged, since the apps may
  share them. `~/Library/Containers` is not read: doing so makes macOS ask for
  the "data from other apps" permission.

Rules in `mac.toml` silence either list: `[permissions] keep = ["node"]`,
`[apps] keep = ["Xcode"]`.
