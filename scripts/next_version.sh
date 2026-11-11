#!/usr/bin/env bash
# Print the tag of the next release: the latest vX.Y.Z tag with one part
# raised, or an exact version when one is given.
#
#   scripts/next_version.sh minor          # v0.18.0 -> v0.19.0
#   scripts/next_version.sh patch 1.2.3    # v1.2.3, whatever the tags say
#
# The release workflow calls it; LATEST=vX.Y.Z replaces the tags for a test.
set -euo pipefail

bump="${1:-minor}"
exact="${2:-}"

if [ -n "$exact" ]; then
  exact="${exact#v}"
  if ! [[ "$exact" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "next_version: '${exact}' is not a version like 1.2.3" >&2
    exit 1
  fi
  echo "v${exact}"
  exit 0
fi

latest="${LATEST:-$(git tag --list 'v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname | head -n 1)}"
latest="${latest:-v0.0.0}"
if ! [[ "$latest" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
  echo "next_version: the latest tag '${latest}' is not vX.Y.Z" >&2
  exit 1
fi
major="${BASH_REMATCH[1]}" minor="${BASH_REMATCH[2]}" patch="${BASH_REMATCH[3]}"
case "$bump" in
  major) major=$((major + 1)); minor=0; patch=0 ;;
  minor) minor=$((minor + 1)); patch=0 ;;
  patch) patch=$((patch + 1)) ;;
  *) echo "next_version: bump is patch, minor or major, not '${bump}'" >&2; exit 1 ;;
esac
echo "v${major}.${minor}.${patch}"
