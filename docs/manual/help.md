# Troubleshooting and FAQ

[omega](../../README.md) › [Manual](README.md) › Troubleshooting and FAQ

- [Troubleshooting](#troubleshooting)
- [FAQ](#faq)

## Troubleshooting

**`no API key configured`**: run `omega setup` or `omega auth set <provider>`, or
export the environment variable.

**`invalid API key (401)`**: `omega auth status` shows the stored key masked and
flags values that are not keys (for example a URL pasted by mistake). Store it
again with `omega auth set <provider>`; omega checks it with the provider before
saving. An exported environment variable takes precedence over the stored key.

**`model or endpoint not found (404)`**: the model does not exist for that
provider. List them with `omega models <provider>` or pick one with `omega model`.

**A local provider is "not reachable"**: start Ollama (`ollama serve`) or the LM
Studio server, or fix `base_url`.

**The model does not call tools or behaves oddly**: use a model with tool
calling; with local models, try a larger one.

**A step that needs sudo was skipped**: you pressed `Esc` on the password card,
or the password was wrong three times. Run again, or use `omega --sudo` to give the
password before the run starts.

**A read-only command was denied**: the classifier treats unknown commands as
dangerous. Open an issue with the command so it can be added to the read-only
list.

**I want to see exactly what it did**: `omega logs` lists the transcripts and
`omega logs view` prints the latest. omega keeps the newest 30 under
`~/.local/state/omega/runs/`.

**`omega doctor` fails**: it prints which check failed. A key that is a URL or
too short is rejected before any request. "Not reachable" means the provider
did not answer a model list (cloud) or a `/models` probe (local). The command
does not send a chat message.

## FAQ

**Does it replace topgrade and Mole?** For a typical developer Mac, yes. If you
have them installed, it also uses them as backends and helps you fix their
configuration.

**Does it send my data to the AI?** The provider you choose receives the
instructions, your notes, the trimmed output of the commands it runs and the
configuration files it reads. With a local model nothing leaves your Mac except
the documentation pages the agent fetches while tuning.

**How much does it cost?** With the automatic model choice, a routine run usually
costs well under one cent. omega shows tokens and cost live and at the end.

**Does it work on Linux or Windows?** Linux: yes. omega updates and cleans
through apt, dnf, pacman, zypper, Flatpak, snap and Homebrew on Linux, keeps
API keys in the system keyring (or a `0600` file when there is none) and
installs developer tools with your distribution's packages. `omega mac` and the
Mac-only steps do not exist there; [Linux](linux.md#linux) has the full list. Windows:
yes. omega runs its commands in PowerShell, updates through winget, Scoop and
Chocolatey, keeps API keys in the Credential Manager and cleans temporary
files, the Recycle Bin and the component store; [Windows](windows.md#windows) has what
works and what does not.

**Can I add a language?** Add a column to `internal/i18n/messages.go` and the code
to `i18n.Supported` and `i18n.Names`; a test checks every key is translated with
the same format arguments.
