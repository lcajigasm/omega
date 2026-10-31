# Roadmap

omega 0.6 is the last merge on main before this pass: login items, background
jobs, the menu bar, the Dock and shortcut conflicts. This list is what makes
the tool robust, obvious to drive, and downloadable. Items already shipped are
checked. The first three open items are the next pass.

## Shipped in 0.7

- [x] **On-screen help.** `?` (and `/help`) opens a shortcut card over the session. It stays closed while a password or a card filter is being typed.
- [x] **Esc clears the line first.** A typed follow-up is cleared on the first Esc; the next Esc, or an empty Enter, ends the session.
- [x] **Honest exit status.** A model error or the turn limit exits 1. Cancel (Ctrl+C or SIGTERM) exits 130. Ending the session yourself exits 0, including after a recovered retry.
- [x] **Config that fails loudly.** `approval`, `language`, `effort`, `command_timeout` and `skip_steps` are checked. `omega config check` prints the result. `omega config set` can still repair a file because load itself does not refuse it. Saves are atomic.
- [x] **Empty config fields fall back** to the documented defaults instead of an unknown provider or a blank timeout.
- [x] **Shell that exists.** Commands use zsh, bash or sh, whichever is installed. Windows builds use Git Bash, PowerShell or cmd, and they compile (session setup is no longer Unix-only).
- [x] **`macos` and `macos-check` only on macOS.** Off a Mac they no longer show up as steps the agent can run.
- [x] **CI on Linux, macOS and Windows** (test, build, `omega --version`, `omega steps`, `omega config check`) plus a Linux cross-compile of all six release targets. Lint stays `gofmt` and `go vet`.
- [x] **Release archives.** A tag `vX.Y.Z` builds linux/darwin/windows amd64 and arm64, attaches them to the GitHub Release with `SHA256SUMS`.
- [x] **README and the website** describe the binaries, the help key and the exit codes. Pages keeps publishing `docs/` from main.

## Next

1. **Run logs.** `~/.local/state/omega/runs/` grows without a limit. Rotate or cap it, and add `omega logs` to open the latest transcript.
2. **Retries for Anthropic.** OpenAI-compatible providers already retry a 429 or a 5xx. A blip on the Anthropic API still ends a long maintenance run with exit 1. Retry with the same backoff, then surface the error.
3. **`omega doctor`.** A preflight that does not spend tokens: config check, key present and shaped like a key, provider reachable, and which catalogue steps are actually installed.

## Later

- TUI tests that render the approval card, the password card and the choice card, so a key change cannot ship on the unit tests of the menu alone.
- Mouse clicks on a card (tick a row, press the focused button) in addition to the keyboard.
- Search inside the transcript.
- Copy the focused tool output.
- Teach the in-repo Homebrew formula to follow the tag that the release workflow just published. The tap itself stays a separate repository.
- A short asciinema of `?`, Esc and a denial, next to the existing `omega --demo` GIF.
