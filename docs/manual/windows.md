# Windows

[omega](../../README.md) › [Manual](README.md) › Windows

## Windows

Windows is omega's third platform, on Windows 10 and 11. This is exactly what
works there and what does not.

**Works**

| Area | On Windows |
|---|---|
| The agent (`omega`, `full`, `update`, `clean`, `analyze`, `tune`, `fix`, `docker`, `dev`, `-p`) | Its instructions are written for Windows, and `system_info` reports the Windows version, whether omega runs elevated and whether a restart is pending |
| Commands | Run in PowerShell: PowerShell 7 when it is installed, otherwise the Windows PowerShell that ships with Windows. Output is read as UTF-8 and a program's exit code is kept |
| API keys | Environment variables first. `omega setup` and `omega auth set` save to the **Windows Credential Manager**, as a generic credential named `omega-cli:<provider>`, protected by your login |
| Package managers | Check and update steps for **winget**, **Scoop** and **Chocolatey**, each shown only when it is installed |
| Windows itself | `windows-update-check` lists the pending Windows updates; `system-logs` reviews the event log (errors by source, services that should be running and are not, application crashes); `wsl` updates the Windows Subsystem for Linux |
| Disk and cleanup | `disk` (free space per drive, the temporary folders), `temp-files`, `recycle-bin`, `scoop-cleanup`, `component-store` |
| Developer tools | The developer-tool steps that are single commands (npm, pnpm, uv, rustup, cargo, Go, .NET, Docker…), `omega dev status`, and `omega dev install` with Scoop or winget |
| Interface | The TUI, plain output, the menu, the five languages, logs, notes, `omega doctor`, models and local models |

**Does not work**

- **`omega mac`** and `/mac`, `/permissions`, `/apps`, `/undo`: they are macOS
  features and say so.
- **Installing Windows updates.** omega lists what is pending and leaves the
  installation, and any restart, to Windows Update.
- **Elevation.** There is no `sudo`. The steps that need administrator rights
  (`choco`, `component-store`) work when the terminal was opened as
  administrator and fail with "access denied" otherwise; omega does not elevate
  itself. `--sudo` does nothing on Windows.
- **`omega dev activate`.** omega does not edit PowerShell profiles; a tool is
  ready once it is on `PATH`.
- **Tools made for Unix shells** (asdf, nvm, pyenv, rbenv, SDKMAN, tmux,
  direnv, Oh My Zsh, Homebrew) and the steps written for a POSIX shell
  (`dev-artifacts`, `shell-startup`) are not offered.
- **Git Bash and WSL.** omega for Windows always uses PowerShell, also when a
  POSIX shell is installed. Inside WSL, use the Linux build instead.
- **Registry and system tuning.** omega does not "debloat" Windows, remove
  built-in apps or disable services, and never turns off Microsoft Defender,
  the firewall or UAC.

The safety policy reads PowerShell too: read-only cmdlets (`Get-`, `Test-`,
`Measure-`…) run straight away; a delete aimed at a whole drive, your profile
or the Windows directories, a download piped into `iex`, `Format-Volume` and
anything that would reveal a provider key never run; everything else asks.

| id | Category | Command | Risk |
|---|---|---|---|
| `disk` | check | free space per drive and the size of the temporary folders | read-only |
| `winget-outdated` `scoop-outdated` `choco-outdated` | check | `winget upgrade`, `scoop status`, `choco outdated` | read-only |
| `windows-update-check` | check | asks Windows Update what is pending | read-only |
| `health` | check | last start, a pending restart, disk health, memory, battery | read-only |
| `security` | check | Microsoft Defender, the firewall profiles, drive encryption, User Account Control | read-only |
| `defender-signatures` | update | `Update-MpSignature` | normal |
| `system-logs` | check | the event log, failing services and application crashes | read-only |
| `winget` | update | `winget upgrade --all` (installers may ask for administrator rights in their own window) | dangerous |
| `scoop` | update | `scoop update; scoop update *` | normal |
| `choco` | update | `choco upgrade all -y` (elevated terminal) | dangerous |
| `wsl` | update | `wsl --update` | dangerous |
| `scoop-cleanup` | clean | old versions and the download cache | dangerous |
| `temp-files` | clean | empties your temporary folder; files in use are skipped | dangerous |
| `recycle-bin` | clean | `Clear-RecycleBin` | dangerous |
| `component-store` | clean | `Dism.exe /Online /Cleanup-Image /StartComponentCleanup` (elevated terminal) | dangerous |
