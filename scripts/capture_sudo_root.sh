#!/usr/bin/env bash
# Run as root from scripts/capture.sh. Requires a password for sudo just long
# enough to record the password card, then deletes the drop-in.
set -euo pipefail

user=${OMEGA_CAPTURE_USER:?}
home=${OMEGA_CAPTURE_HOME:?}
root=${OMEGA_CAPTURE_ROOT:?}
config=${XDG_CONFIG_HOME:?}
state=${XDG_STATE_HOME:?}
data=${XDG_DATA_HOME:?}

f=/etc/sudoers.d/zz-omega-capture
cleanup() { rm -f "$f"; }
trap cleanup EXIT
printf '%s\n' "$user ALL=(ALL:ALL) ALL" >"$f"
chmod 440 "$f"
visudo -cf "$f" >/dev/null
runuser -u "$user" -- env \
  HOME="$home" \
  PATH="$PATH" \
  TERM="${TERM:-xterm-256color}" \
  OMEGA_LANG=en \
  XDG_CONFIG_HOME="$config" \
  XDG_STATE_HOME="$state" \
  XDG_DATA_HOME="$data" \
  bash -lc "cd '$root' && vhs docs/assets/sudo.tape"
