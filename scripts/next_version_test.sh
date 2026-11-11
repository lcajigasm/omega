#!/usr/bin/env bash
# Tests of scripts/next_version.sh: the version the release workflow publishes.
set -euo pipefail
cd "$(dirname "$0")"

fail=0
check() { # check <expected> <LATEST> <args...>
  local want="$1" latest="$2" got
  shift 2
  got="$(LATEST="$latest" ./next_version.sh "$@" 2>/dev/null)" || got="error"
  if [ "$got" != "$want" ]; then
    echo "next_version.sh $* after ${latest}: got ${got}, want ${want}"
    fail=1
  fi
}

check v0.18.1 v0.18.0 patch
check v0.19.0 v0.18.7 minor
check v1.0.0 v0.18.7 major
check v0.19.0 v0.18.0            # minor is the default
check v2.0.1 v0.18.0 minor 2.0.1 # an exact version wins over the bump
check v2.0.1 v0.18.0 minor v2.0.1
check v0.10.0 v0.9.9 minor       # numbers, not text: 9 -> 10
check error v0.18.0 minor 1.2    # not a full version
check error v0.18.0 huge         # not a bump
check error latest minor         # the latest tag is not a version

[ "$fail" = 0 ] && echo OK
exit "$fail"
