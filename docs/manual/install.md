# Install and set up

[omega](../../README.md) › [Manual](README.md) › Install and set up

- [Installation](#installation)
  - [Container image](#container-image)
  - [Desktop app](#desktop-app)
- [Getting started](#getting-started)

## Installation

### Homebrew (recommended)

```bash
brew install lcajigasm/tap/omega
```

This works on macOS and on Linux with [Homebrew on Linux](https://docs.brew.sh/Homebrew-on-Linux).

> The formula is published from [`lcajigasm/homebrew-tap`](https://github.com/lcajigasm/homebrew-tap).
> If omega came from the old tap, run `brew update && brew upgrade omega` so Homebrew follows the migration, or reinstall from `lcajigasm/tap`.
> homebrew-core already has a formula called `omega` (a Xapian search engine),
> so always use the full name.

### Updating

```bash
brew update && brew upgrade omega
omega --version
```

This manual describes the latest release. An older omega does not have the
newer commands (`omega dev` and `omega menu` arrived in 0.14.0) and says so,
with the version it is. `omega doctor` tells you when a newer release exists.

### Binaries

Every tagged release publishes archives for **Linux, macOS and Windows** (amd64 and arm64), plus `SHA256SUMS`:

<https://github.com/lcajigasm/omega/releases/latest>

On Linux without Homebrew, download the archive for your architecture, check it
and put the binary on your `PATH`:

```bash
tar -xzf omega_*_linux_amd64.tar.gz        # or linux_arm64
sha256sum -c SHA256SUMS --ignore-missing   # with SHA256SUMS downloaded next to it
sudo install -m 755 omega /usr/local/bin/omega
```

On Windows, download the zip for your architecture, extract it and put the
folder on your `PATH`. In PowerShell:

```powershell
Expand-Archive omega_*_windows_amd64.zip -DestinationPath "$env:LOCALAPPDATA\Programs\omega"   # or windows_arm64
$dir = "$env:LOCALAPPDATA\Programs\omega"
[Environment]::SetEnvironmentVariable("Path", [Environment]::GetEnvironmentVariable("Path", "User") + ";$dir", "User")
```

Open a new terminal and run `omega setup`. Windows Terminal is recommended.

omega supports **macOS, Linux and Windows**: see [what works on Linux](linux.md#linux)
and [what works on Windows](windows.md#windows).

### Container image

Every release also publishes the command as a container image for Linux
(amd64 and arm64) on GitHub Packages:

```bash
docker run --rm -it ghcr.io/lcajigasm/omega --demo        # a scripted session: no API calls, no real commands
docker run --rm -it -e ANTHROPIC_API_KEY ghcr.io/lcajigasm/omega analyze
```

The tags are the version (`0.19.0`), its minor (`0.19`) and `latest`. The
image is Debian with omega as its entry point, so what follows the image name
are omega's own arguments. The key comes from the environment
(`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `OPENROUTER_API_KEY` or
`OPENCODE_API_KEY`); `-e NAME` with no value passes the one your shell has.

omega maintains the system it runs on, and in a container that is the
container: the image is for trying omega, for a pipeline, or as the base of
an image you want it to look after, not for maintaining the machine that runs
Docker. For that, install the command. Without a terminal (no `-it`) use
`--no-tui`, as in [unattended runs](tasks.md#unattended-runs).

Settings, notes and run logs are lost with the container unless you keep them
in volumes:

```bash
docker run --rm -it -e ANTHROPIC_API_KEY \
  -v omega-config:/root/.config/omega -v omega-state:/root/.local/state/omega \
  ghcr.io/lcajigasm/omega
```

### Desktop app

The same agent in a window. It is a separate install from the command, and
the two share their configuration and API keys.

On macOS 13 or later, Apple Silicon and Intel:

```bash
brew install --cask lcajigasm/tap/omega-app
xattr -dr com.apple.quarantine /Applications/Omega.app
```

or download
[`omega-macos-universal.dmg`](https://github.com/lcajigasm/omega/releases/latest/download/omega-macos-universal.dmg),
open it and drag Omega to Applications. The app is ad-hoc signed and not
notarized yet, so macOS quarantines it: the `xattr` line clears that, once.
`brew upgrade --cask omega-app` updates it.

On Windows, take `omega-gui_<version>_windows_amd64.zip` (or `arm64`) from the
[latest release](https://github.com/lcajigasm/omega/releases/latest), extract
it and open `omega-gui.exe`. The build is not signed, so SmartScreen warns on
the first run: choose **More info**, then **Run anyway**.

On Linux, take `omega-gui_<version>_linux_amd64.tar.gz` (or `arm64`):

```bash
tar -xzf omega-gui_*_linux_amd64.tar.gz
./omega-gui
```

It needs GTK 4 and WebKitGTK 6.0, which current desktop distributions ship
(`libgtk-4-1` and `libwebkitgtk-6.0-4` on Debian and Ubuntu).

The first time it opens, the app asks for a provider and its key, as
`omega setup` does. If the command is already set up, the app is too.

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

The Homebrew formula installs zsh, bash and fish completion. If you built it
yourself: `omega completion zsh > "${fpath[1]}/_omega"`.

## Getting started

```bash
omega setup        # pick a provider, store the API key and choose a model
omega analyze      # first look: a report that changes nothing
omega              # complete maintenance
```

`omega setup` shows the providers on a card (local servers that are running are
marked), asks for the key with hidden input, **checks the key with the provider
before saving it**, and then lets you pick a model. The key goes to the macOS
Keychain; on Windows, to the Credential Manager; on Linux, to the system
keyring (Secret Service, through `secret-tool`) or, when there is none, to a
file only you can read. omega says which one it used.
