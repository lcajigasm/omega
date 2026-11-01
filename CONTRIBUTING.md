# Contributing

## Captures

Screenshots, GIFs and asciinema recordings in the README and on the website are
recordings of the `omega` binary built from this repository. A frame drawn by
hand, exported from a design tool, or rendered by calling the UI package
without running that binary does not belong in those pages.

Regenerate them with:

```bash
scripts/capture.sh
```

The script compiles `./bin/omega` and records it with [vhs](https://github.com/charmbracelet/vhs)
(which needs `ttyd` and ffmpeg) and [asciinema](https://github.com/asciinema/asciinema)
on a normal Linux machine, with DejaVu Sans Mono. [agg](https://github.com/asciinema/agg)
is installed by the workflow for turning a cast into a GIF when you want one.
`omega --demo` is the mock provider for the maintenance session, the shortcut
card and the denial. The model picker and the sudo card talk to
`scripts/mock_provider.py` on localhost. Do not point a capture at a paid API.

The same script runs in the `captures` workflow (`workflow_dispatch`, and on a
`v*` tag). That job uploads the files it recorded. It does not commit them and
it does not touch any Homebrew tap.

Logos (`docs/favicon.png`, `docs/og.png`) are not terminal captures.
