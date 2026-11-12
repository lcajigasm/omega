#!/usr/bin/env bash
# Install the system webview headers the desktop app needs on Linux
# (libgtk-4-dev and libwebkitgtk-6.0-dev), quickly and without hanging.
#
#   scripts/install_webview_deps.sh [CACHE_DIR]
#
# The CI and release workflows run this on Ubuntu runners. The Azure apt
# mirror has been slow (100 kB/s) or unreachable: a plain apt-get install
# then sat for 45 minutes until the job was cancelled. So:
#
#   * only the packages themselves, without their recommends (gstreamer,
#     pipewire, ...), which the build does not use: a fraction of the download;
#   * short network timeouts and retries, so a dead mirror fails in seconds
#     and apt moves on to the next one;
#   * the .deb files kept in CACHE_DIR, which the workflow saves with
#     actions/cache, so a later run downloads only what changed;
#   * the whole install tried three times before the step gives up.
set -euo pipefail

cache="${1:-$HOME/.cache/webview-debs}"
mkdir -p "$cache/partial"

apt_opts=(
  -o Acquire::Retries=3
  -o Acquire::http::Timeout=20
  -o Acquire::https::Timeout=20
  -o Dpkg::Use-Pty=0
  -o Dir::Cache::Archives="$cache"
)
packages=(libgtk-4-dev libwebkitgtk-6.0-dev)

for attempt in 1 2 3; do
  if sudo apt-get "${apt_opts[@]}" -qq update &&
    sudo DEBIAN_FRONTEND=noninteractive apt-get "${apt_opts[@]}" -y -qq \
      --no-install-recommends install "${packages[@]}"; then
    # apt wrote the cache as root; actions/cache saves it as the runner user.
    sudo chown -R "$(id -u):$(id -g)" "$cache"
    rm -rf "$cache/partial" "$cache/lock"
    exit 0
  fi
  echo "::warning::installing the webview headers failed (attempt ${attempt}), trying again"
  sleep $((attempt * 10))
done

echo "::error::could not install ${packages[*]}"
exit 1
