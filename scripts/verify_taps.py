#!/usr/bin/env python3
"""Check that lcajigasm/homebrew-tap installs the version of a release tag.

Reads Formula/omega.rb (the command) and Casks/omega-app.rb (the desktop
app) from the public GitHub API. No token is sent. The release workflow
opens a pull request with both, so a mismatch is retried every five minutes until 75 minutes have passed,
then the command fails. --deadline 0 checks once and fails immediately.
"""

import argparse
import base64
import json
import re
import sys
import time
import urllib.error
import urllib.request

TAP = "lcajigasm/homebrew-tap"
INTERVAL = 5 * 60
DEADLINE = 75 * 60
API = "https://api.github.com/repos/{repo}/contents/{path}"
# What brew install reads: the command's formula and the desktop app's cask.
FILES = ("Formula/omega.rb", "Casks/omega-app.rb")

VERSION_LINE = re.compile(r'(?m)^\s*version\s+"(\d+\.\d+\.\d+)"')
RELEASE_URL = re.compile(
    r"releases/download/v(\d+\.\d+\.\d+)/omega_(\d+\.\d+\.\d+)_"
)


class TapError(Exception):
    """The formula could not be read or does not name one version."""


def normalize(tag):
    tag = tag.strip()
    if tag.startswith("v"):
        tag = tag[1:]
    if not re.fullmatch(r"\d+\.\d+\.\d+", tag):
        raise SystemExit(f"tag must look like vX.Y.Z, got {tag!r}")
    return tag


def versions_in(text):
    """Versions a formula would install, ignoring a git head URL.

    A version line and every release archive URL have to agree. The path
    version and the archive name have to agree too.
    """
    found = set(VERSION_LINE.findall(text))
    for path_ver, file_ver in RELEASE_URL.findall(text):
        if path_ver != file_ver:
            raise TapError(f"archive name {file_ver} disagrees with url {path_ver}")
        found.add(path_ver)
    if not found:
        raise TapError("no version line and no release url")
    if len(found) != 1:
        raise TapError("mixed versions: " + ", ".join(sorted(found)))
    return found.pop()


def fetch_formula(repo, path=FILES[0]):
    req = urllib.request.Request(
        API.format(repo=repo, path=path),
        headers={
            "Accept": "application/vnd.github+json",
            "User-Agent": "omega-verify-taps",
            "X-GitHub-Api-Version": "2022-11-28",
        },
    )
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            payload = json.load(resp)
    except urllib.error.HTTPError as err:
        if err.code in (403, 404, 429) or err.code >= 500:
            raise TapError(f"HTTP {err.code}") from err
        raise
    content = payload.get("content") or ""
    return base64.b64decode(content).decode()


def check_once(expected, fetch=fetch_formula):
    problems = []
    for path in FILES:
        name = f"{TAP} {path}"
        try:
            got = versions_in(fetch(TAP, path))
        except TapError as err:
            problems.append(f"{name}: {err}")
            print(f"{name}: {err}", flush=True)
            continue
        if got != expected:
            problems.append(f"{name}: {got}, want {expected}")
            print(f"{name}: {got}, want {expected}", flush=True)
        else:
            print(f"{name}: {got}", flush=True)
    return problems


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("tag", help="release tag, vX.Y.Z or X.Y.Z")
    parser.add_argument("--interval", type=int, default=INTERVAL)
    parser.add_argument("--deadline", type=int, default=DEADLINE)
    args = parser.parse_args(argv)
    expected = normalize(args.tag)
    started = time.monotonic()
    attempt = 0
    while True:
        attempt += 1
        elapsed = int(time.monotonic() - started)
        print(f"attempt {attempt} at +{elapsed}s, want {expected}", flush=True)
        problems = check_once(expected)
        if not problems:
            print("tap matches", flush=True)
            return 0
        remaining = args.deadline - (time.monotonic() - started)
        if remaining <= 0:
            # ::error:: is the Actions annotation. The formula on the tap is
            # what brew install reads, so a mismatch has to fail in the log
            # instead of ending as a quiet retry.
            print(
                "::error::lcajigasm/homebrew-tap must be updated by hand: "
                f"{'; '.join(problems)}. Set the version and the sha256 values "
                "in Formula/omega.rb and Casks/omega-app.rb, or merge the tap "
                "pull request.",
                file=sys.stderr,
            )
            print(
                f"giving up after {attempt} attempts; the tap was not {expected}",
                file=sys.stderr,
            )
            return 1
        pause = min(args.interval, remaining)
        print(f"retrying in {int(pause)}s", flush=True)
        time.sleep(pause)


if __name__ == "__main__":
    sys.exit(main())
