#!/usr/bin/env bash
# Record the README and Pages captures by running the omega binary built from
# this checkout. The demo provider and a local test server stand in for a
# model, so the script never calls a paid API.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

export OMEGA_LANG=en
export XDG_CONFIG_HOME="${OMEGA_CAPTURE_CONFIG:-$(mktemp -d)}"
export XDG_STATE_HOME="${OMEGA_CAPTURE_STATE:-$(mktemp -d)}"
export XDG_DATA_HOME="${OMEGA_CAPTURE_DATA:-$(mktemp -d)}"
mkdir -p "$XDG_CONFIG_HOME" "$XDG_STATE_HOME" "$XDG_DATA_HOME"

for cmd in go vhs ttyd asciinema ffmpeg python3; do
  command -v "$cmd" >/dev/null || { echo "capture: missing $cmd" >&2; exit 1; }
done
export TERM="${TERM:-xterm-256color}"
if [ "$TERM" = "dumb" ]; then
  export TERM=xterm-256color
fi

echo "capture: building omega"
go build -trimpath -ldflags "-s -w -X main.version=capture" -o bin/omega ./cmd/omega

write_config() {
  local base_url=$1 model=${2:-}
  mkdir -p "$XDG_CONFIG_HOME/omega"
  cat >"$XDG_CONFIG_HOME/omega/config.toml" <<EOF
provider = "ollama"
language = "en"
approval = "dangerous"
command_timeout = "2m"
model = "${model}"

[providers.ollama]
model = "${model}"
base_url = "${base_url}"
EOF
}

echo "capture: demo session (gif and screenshots)"
rm -f "$XDG_CONFIG_HOME/omega/config.toml"
vhs docs/assets/demo.tape

echo "capture: ?, Esc and a denial"
cast=docs/assets/shortcuts.cast
rm -f "$cast"
title="omega: help, Esc and a denial"
if [ -t 1 ]; then
  asciinema rec -q --overwrite --cols 100 --rows 32 -i 2 \
    -t "$title" -c "python3 scripts/drive_shortcuts.py" "$cast"
else
  script -qefc "TERM=xterm-256color asciinema rec -q --overwrite --cols 100 --rows 32 -i 2 -t '$title' -c 'python3 scripts/drive_shortcuts.py' '$cast'" /dev/null
fi
python3 - "$cast" <<'PY'
import json, sys
path = sys.argv[1]
lines = open(path, encoding="utf-8").read().splitlines()
header = json.loads(lines[0])
if header.get("version") != 2 or header.get("timestamp") == 1762000000:
    sys.exit("shortcuts.cast is not a recording of the omega binary")
out = []
for line in lines[1:]:
    ev = json.loads(line)
    if ev[1] == "o":
        out.append(ev[2])
text = "".join(out)
for needle in ("Keyboard shortcuts", "ctrl+y", "Run this?", "denied"):
    if needle not in text and needle.lower() not in text.lower():
        sys.exit(f"shortcuts.cast missing {needle!r}")
if text.find("Keyboard shortcuts") > text.lower().find("denied"):
    sys.exit("help card is not before the denial")
print("capture: shortcuts.cast ok")
PY

echo "capture: a refused key and scrolling during approval"
cast=docs/assets/failure.cast
rm -f "$cast"
title="omega: a refused key and scrolling during approval"
if [ -t 1 ]; then
  asciinema rec -q --overwrite --cols 100 --rows 32 -i 2 \
    -t "$title" -c "python3 scripts/drive_failure.py" "$cast"
else
  script -qefc "TERM=xterm-256color asciinema rec -q --overwrite --cols 100 --rows 32 -i 2 -t '$title' -c 'python3 scripts/drive_failure.py' '$cast'" /dev/null
fi
python3 - "$cast" <<'PY'
import json, sys
path = sys.argv[1]
lines = open(path, encoding="utf-8").read().splitlines()
header = json.loads(lines[0])
if header.get("version") != 2 or header.get("timestamp") == 1762000000:
    sys.exit("failure.cast is not a recording of the omega binary")
import re
text = "".join(json.loads(line)[2] for line in lines[1:] if json.loads(line)[1] == "o")
# Glamour paints a word at a time, so a phrase is split by colour codes.
text = re.sub(r"\x1b\[[0-9;?]*[A-Za-z]", "", text)
for needle in ("refusing to read", "Run this?", "TOP OF THE REPORT"):
    if needle not in text:
        sys.exit(f"failure.cast missing {needle!r}")
print("capture: failure.cast ok")
PY

echo "capture: model picker against the local test server"
python3 scripts/mock_provider.py 18080 models &
models_pid=$!
cleanup_models() { kill "$models_pid" 2>/dev/null || true; wait "$models_pid" 2>/dev/null || true; }
trap cleanup_models EXIT
for _ in 1 2 3 4 5 6 7 8 9 10; do
  python3 -c 'import urllib.request; urllib.request.urlopen("http://127.0.0.1:18080/v1/models", timeout=1)' && break
  sleep 0.2
done
python3 -c 'import urllib.request; urllib.request.urlopen("http://127.0.0.1:18080/v1/models", timeout=1)'
write_config "http://127.0.0.1:18080/v1"
vhs docs/assets/model.tape
cleanup_models
trap - EXIT

if [ "${OMEGA_CAPTURE_SUDO:-1}" = "0" ]; then
  echo "capture: skipping the sudo card (OMEGA_CAPTURE_SUDO=0)"
  exit 0
fi

echo "capture: sudo password card"
command -v sudo >/dev/null || { echo "capture: sudo is required for the password card" >&2; exit 1; }
sudo -n true
user=$(id -un)
python3 scripts/mock_provider.py 18081 sudo &
sudo_pid=$!
cleanup_sudo() { kill "$sudo_pid" 2>/dev/null || true; wait "$sudo_pid" 2>/dev/null || true; }
trap cleanup_sudo EXIT
for _ in 1 2 3 4 5 6 7 8 9 10; do
  python3 -c 'import urllib.request; urllib.request.urlopen("http://127.0.0.1:18081/v1/models", timeout=1)' && break
  sleep 0.2
done
python3 -c 'import urllib.request; urllib.request.urlopen("http://127.0.0.1:18081/v1/models", timeout=1)'
write_config "http://127.0.0.1:18081/v1" "omega-local-test"
# One root shell installs a password requirement, records the card, and
# deletes the sudoers drop-in before it exits.
sudo env \
  OMEGA_CAPTURE_USER="$user" \
  OMEGA_CAPTURE_HOME="$HOME" \
  OMEGA_CAPTURE_ROOT="$root" \
  XDG_CONFIG_HOME="$XDG_CONFIG_HOME" \
  XDG_STATE_HOME="$XDG_STATE_HOME" \
  XDG_DATA_HOME="$XDG_DATA_HOME" \
  PATH="$PATH" \
  TERM=xterm-256color \
  bash "$root/scripts/capture_sudo_root.sh"
cleanup_sudo
trap - EXIT
echo "capture: done"
