# Roadmap

omega 0.8 is the current release on main: run logs, Anthropic retries and
`omega doctor`, on top of the 0.7 session controls. This list is what makes the
tool robust, obvious to drive, and downloadable. Items already shipped are
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

## Shipped in 0.8

- [x] **Run logs.** `~/.local/state/omega/runs/` keeps the newest 30 transcripts. `omega logs` lists them and `omega logs view` prints one (the latest, a list index, or a file name).
- [x] **Retries for Anthropic.** A 429, a 5xx or a dropped connection is retried with the same waits as the OpenAI-compatible client (2s, then 4s, then 8s). Other 4xx responses are returned at once. The SDK's own retry loop is off, so the waits do not stack.
- [x] **`omega doctor`.** Preflight with no chat message: config check, key present and shaped like a key, provider reachable (a model list, or a local `/models` probe), and which catalogue steps are installed.
- [x] **CI smoke** also runs `omega logs`.

## Shipped in 0.9

- [x] **TUI card tests.** The approval card, the password card and the choice card render into a fixed cell grid. The snapshots do not depend on the real terminal size or on ANSI color.
- [x] **Mouse on cards.** A click selects a row. A second click on that row, or a click on the key line, confirms it. On an approval card, a click on `[y]`, `[a]` or `[n]` decides.
- [x] **Search the transcript.** `ctrl+f` opens a search line. Matches are drawn in reverse video. Enter or `ctrl+n` jumps to the next one, `ctrl+p` to the previous, Esc closes the search.

## Next

1. **Copy the focused tool output.**
2. **Homebrew formula follows the tag** the release workflow just published. The tap itself stays a separate repository.
3. **A short asciinema** of `?`, Esc and a denial, next to the existing `omega --demo` GIF.
