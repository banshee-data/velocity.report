#!/usr/bin/env python3
"""Verify GitHub Actions uses: lines are SHA-pinned and that each SHA still exists.

Two checks per uses: line:
  1. Format: must be owner/action@<40-hex-chars> (tags like @v1, @main are rejected).
  2. Liveness: the commit SHA must still be reachable via the GitHub API.

Local composite actions (uses: ./.github/...) are skipped — they carry no
tag-repointing risk.

Usage:
    python3 scripts/check-action-pins.py [--workflows-dir .github/workflows]

Exit codes:
    0  all checks passed
    1  one or more failures (details printed to stdout)

Environment:
    GITHUB_TOKEN  optional but strongly recommended to avoid rate-limiting.
                  Without it the GitHub API allows ~60 unauthenticated requests/hour.
"""

import argparse
import os
import re
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import Optional

SHA_RE = re.compile(r"^[0-9a-f]{40}$")
# Matches:  uses: owner/repo@sha  OR  uses: owner/repo/subdir@sha
USES_RE = re.compile(r"""^\s+uses:\s+(['"]?)([^'"#\s]+)\1""", re.MULTILINE)


def parse_uses_lines(path: Path) -> list[tuple[int, str]]:
    """Return (line_number, ref) for every uses: line in a workflow file."""
    results = []
    for i, line in enumerate(path.read_text().splitlines(), start=1):
        m = re.match(r"""^\s+uses:\s+(['"]?)([^'"#\s]+)\1""", line)
        if m:
            results.append((i, m.group(2)))
    return results


def check_sha_live(owner_repo: str, sha: str, token: Optional[str]) -> tuple:
    """Return (ok, message). Calls GET /repos/{owner}/{repo}/commits/{sha}."""
    url = f"https://api.github.com/repos/{owner_repo}/commits/{sha}"
    req = urllib.request.Request(url, headers={"Accept": "application/vnd.github+json"})
    if token:
        req.add_header("Authorization", f"Bearer {token}")
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            if resp.status == 200:
                return True, "ok"
            return False, f"HTTP {resp.status}"
    except urllib.error.HTTPError as e:
        if e.code == 404:
            return False, "commit not found (SHA deleted or repo gone)"
        if e.code == 422:
            return False, "SHA not a valid commit object"
        if e.code == 403:
            return False, "rate-limited or forbidden (HTTP 403) — set GITHUB_TOKEN"
        return False, f"HTTP {e.code}: {e.reason}"
    except urllib.error.URLError as e:
        return False, f"network error: {e.reason}"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--workflows-dir",
        default=".github/workflows",
        help="directory containing workflow YAML files (default: .github/workflows)",
    )
    parser.add_argument(
        "--no-liveness",
        action="store_true",
        help="skip the GitHub API liveness check (format check only)",
    )
    args = parser.parse_args()

    token = os.environ.get("GITHUB_TOKEN")
    workflows_dir = Path(args.workflows_dir)

    if not workflows_dir.is_dir():
        print(f"error: {workflows_dir} is not a directory", file=sys.stderr)
        return 1

    yaml_files = sorted(workflows_dir.glob("*.yml")) + sorted(
        workflows_dir.glob("*.yaml")
    )
    if not yaml_files:
        print(f"no workflow files found in {workflows_dir}", file=sys.stderr)
        return 1

    failures: list[str] = []
    # Cache liveness results — many workflows pin the same action.
    liveness_cache: dict[str, tuple[bool, str]] = {}
    request_count = 0

    for wf_path in yaml_files:
        for lineno, ref in parse_uses_lines(wf_path):
            # Skip local composite actions.
            if ref.startswith("./"):
                continue

            # Split into action ref and optional version comment.
            action_part = ref  # e.g. "actions/checkout@abc123"

            if "@" not in action_part:
                failures.append(
                    f"{wf_path.name}:{lineno}: missing @ in uses ref: {ref!r}"
                )
                continue

            action_name, pin = action_part.rsplit("@", 1)

            # Check 1: must be a 40-char hex SHA.
            if not SHA_RE.match(pin):
                failures.append(
                    f"{wf_path.name}:{lineno}: not a SHA pin (got {pin!r}): {ref!r}\n"
                    f"  Fix: replace with the commit SHA for this tag, e.g.:\n"
                    f"    uses: {action_name}@<sha> # {pin}"
                )
                continue

            # Check 2: liveness via GitHub API.
            if args.no_liveness:
                continue

            # action_name is owner/repo or owner/repo/subdir — extract owner/repo.
            parts = action_name.split("/")
            if len(parts) < 2:
                failures.append(
                    f"{wf_path.name}:{lineno}: cannot parse owner/repo from {action_name!r}"
                )
                continue
            owner_repo = f"{parts[0]}/{parts[1]}"

            cache_key = f"{owner_repo}@{pin}"
            if cache_key not in liveness_cache:
                # Respect GitHub's secondary rate limit (avoid bursting).
                if request_count > 0 and request_count % 10 == 0:
                    time.sleep(1)
                ok, msg = check_sha_live(owner_repo, pin, token)
                liveness_cache[cache_key] = (ok, msg)
                request_count += 1
            else:
                ok, msg = liveness_cache[cache_key]

            if not ok:
                failures.append(
                    f"{wf_path.name}:{lineno}: SHA {pin[:12]}… for {action_name!r} is unavailable: {msg}\n"
                    f"  Action: re-pin to a current SHA for this action."
                )

    if failures:
        print(f"check-action-pins: {len(failures)} failure(s)\n")
        for f in failures:
            print(f"  ✗ {f}")
        return 1

    pinned = sum(
        1
        for wf in yaml_files
        for _, ref in parse_uses_lines(wf)
        if not ref.startswith("./")
        and "@" in ref
        and SHA_RE.match(ref.rsplit("@", 1)[1])
    )
    liveness_str = "" if args.no_liveness else f", {request_count} SHA(s) verified live"
    print(
        f"check-action-pins: {pinned} pinned ref(s) across {len(yaml_files)} workflow(s) OK{liveness_str}"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
