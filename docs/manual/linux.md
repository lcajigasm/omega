# Linux

[omega](../../README.md) › [Manual](README.md) › Linux

## Linux

Linux is omega's second platform. This is exactly what works there and what
does not.

**Works**

| Area | On Linux |
|---|---|
| The agent (`omega`, `update`, `clean`, `analyze`, `tune`, `fix`, `docker`, `dev`, `-p`) | The system prompt names the distribution it runs on (from `/etc/os-release`); `system_info` reads `/etc/os-release`, `uname`, `df` and `/proc/meminfo`, and reports when a restart is pending |
| API keys | Environment variables first. `omega setup` and `omega auth set` save to the **Secret Service** (GNOME Keyring, KWallet…) when `secret-tool` is installed and a keyring answers; otherwise to `~/.config/omega/keys`, mode `0600`. omega says which one it used, and `omega auth status` and `omega doctor` show the source. The agent can never read either: commands that would print the key file, anything else in that folder except `config.toml`, `notes.md` and `mac.toml`, or a keyring entry are blocked however they are written, and the provider keys in the environment are not passed to the commands it runs |
| System packages | Check, update and clean steps for **apt**, **dnf**, **pacman** and **zypper** (`zypper dup` on Tumbleweed), each shown only when its binary is installed. They use `sudo`, are classified dangerous and always ask |
| Flatpak and snap | Outdated, update, unused runtimes and old snap revisions |
| Homebrew on Linux | The `brew` steps work as on a Mac, except casks |
| Disk, logs and Trash | `disk` (free space, `~/.cache`, `/var/cache`, `/var/log`, Trash), `journal-size`, `journal-vacuum`, `system-logs` (failed units, errors since boot, core dumps), `thumbnail-cache`, `trash` (`~/.local/share/Trash`) |
| Developer tools | The 46 developer-tool steps (npm, pnpm, uv, rustup, cargo, Go, Docker…), `omega dev status`, `omega dev install` with a command per distribution, `omega dev activate` for zsh and bash |
| Interface | The TUI, plain output, the five languages, logs, notes, `omega doctor`, models and local models |

**Does not work**

- **`omega mac`** and `/mac`, `/permissions`, `/apps`, `/undo`: login items, the
  menu bar, the Dock, shortcuts, privacy permissions and apps are macOS
  features. On Linux they answer that they only work on macOS.
- **Mac-only steps** (macOS updates, Mac App Store, casks, Xcode, simulators,
  Mole) are not in the Linux catalogue.
- **No package manager omega knows**: on a distribution without apt, dnf, pacman
  or zypper (Alpine, NixOS, Void, Gentoo…) the system-package steps do not
  appear; Flatpak, snap, Homebrew and the developer-tool steps still do.
- **`omega dev install`** has no command for a few tools on Linux (Swift,
  CocoaPods, OrbStack, .NET, Google Cloud CLI, Codex) or when the distribution
  does not package one and Homebrew is not installed. Distribution packages can
  be older than the current release.
- **Shells other than zsh and bash** (fish, nushell): `omega dev activate` prints
  the lines instead of editing the startup file.
- **Immutable systems** (Fedora Silverblue, openSUSE MicroOS, SteamOS): the
  package steps assume a normal, writable system and are not adapted to
  `rpm-ostree` or `transactional-update`.
- **Firmware updates** (`fwupdmgr`) and release upgrades (`do-release-upgrade`,
  `dnf system-upgrade`) are left to you.

Without a desktop session (a server over SSH, a container) there is usually no
keyring, so keys go to the `0600` file. Install `libsecret-tools` (Debian,
Ubuntu), `libsecret` (Fedora, Arch) and log in to a desktop session to use the
keyring; the next `omega auth set` moves the key there.
