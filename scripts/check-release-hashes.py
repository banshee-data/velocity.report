#!/usr/bin/env python3
"""Validate SHA256 hashes and file sizes in release JSON files.

Checks public_html/src/_data/release.json and image/os-list-velocity.json
against the actual files at their download URLs. Uses HTTP HEAD for size
and streams the file to compute SHA256 when --verify-sha is passed.

Exit codes:
  0  all checks pass
  1  one or more checks failed
  2  usage / configuration error
"""

from __future__ import annotations

import hashlib
import json
import sys
import urllib.request
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
RELEASE_JSON = REPO_ROOT / "public_html" / "src" / "_data" / "release.json"
OS_LIST_JSON = REPO_ROOT / "image" / "os-list-velocity.json"

# Timeout for HTTP requests (seconds).
HTTP_TIMEOUT = 30


def load_json(path: Path) -> dict:
    with open(path) as f:
        return json.load(f)


def head_size(url: str) -> int | None:
    """Return Content-Length from an HTTP HEAD request, or None."""
    req = urllib.request.Request(url, method="HEAD")
    req.add_header("User-Agent", "velocity-release-checker/1.0")
    try:
        with urllib.request.urlopen(req, timeout=HTTP_TIMEOUT) as resp:
            cl = resp.headers.get("Content-Length")
            return int(cl) if cl else None
    except Exception as exc:
        print(f"  WARN  HEAD {url}: {exc}")
        return None


def stream_sha256(url: str) -> str:
    """Download the file at *url* and return its hex SHA256."""
    req = urllib.request.Request(url)
    req.add_header("User-Agent", "velocity-release-checker/1.0")
    h = hashlib.sha256()
    with urllib.request.urlopen(req, timeout=300) as resp:
        while chunk := resp.read(1 << 20):
            h.update(chunk)
    return h.hexdigest()


def check_entry(
    label: str,
    url: str,
    expected_sha: str | None,
    expected_size: int | None,
    verify_sha: bool,
) -> list[str]:
    """Return a list of failure messages (empty = pass)."""
    failures: list[str] = []
    print(f"  {label}")
    print(f"    url: {url}")

    # --- size via HEAD ---
    if expected_size is not None:
        actual = head_size(url)
        if actual is None:
            print("    size: SKIP (HEAD returned no Content-Length)")
        elif actual != expected_size:
            msg = f"    size: FAIL (expected {expected_size}, got {actual})"
            print(msg)
            failures.append(f"{label}: {msg.strip()}")
        else:
            print(f"    size: OK ({expected_size})")
    else:
        print("    size: SKIP (no expected size)")

    # --- SHA256 ---
    if expected_sha and verify_sha:
        print("    sha256: downloading …", end="", flush=True)
        try:
            actual_sha = stream_sha256(url)
        except Exception as exc:
            print(f" ERROR ({exc})")
            failures.append(f"{label}: download failed: {exc}")
            return failures
        if actual_sha != expected_sha:
            print(" FAIL")
            print(f"      expected: {expected_sha}")
            print(f"      actual:   {actual_sha}")
            failures.append(f"{label}: SHA256 mismatch")
        else:
            print(" OK")
    elif expected_sha:
        print("    sha256: SKIP (pass --verify-sha to download and check)")
    else:
        print("    sha256: SKIP (no expected hash)")

    return failures


def entries_from_release_json(data: dict) -> list[dict]:
    """Extract downloadable entries from release.json."""
    entries = []
    asset_labels = {
        "linux_arm64": "Linux ARM64 server",
        "mac_arm64": "macOS ARM64 server",
        "visualiser": "VelocityVisualiser",
    }
    for channel in ("stable", "prerelease"):
        ch = data.get(channel)
        if not ch:
            continue
        for key, label in asset_labels.items():
            asset = ch.get(key) or {}
            version = asset.get("version")
            url = asset.get("url")
            sha = asset.get("sha256")
            # Skip assets not yet published (empty url, sha, or version).
            if not url or not sha or not version:
                continue
            entries.append(
                {
                    "label": f"[{channel}] {label} v{version}",
                    "url": url,
                    "sha": sha,
                    "size": None,
                }
            )
    # RPi image (nested under rpi_image key).
    rpi = data.get("rpi_image", {})
    if rpi.get("url"):
        entries.append(
            {
                "label": f"RPi image v{rpi.get('version', '?')}",
                "url": rpi["url"],
                "sha": rpi.get("sha256"),
                "size": rpi.get("download_size"),
            }
        )
    return entries


def entries_from_os_list(data: dict) -> list[dict]:
    """Extract downloadable entries from os-list-velocity.json.

    Note: extract_sha256 is the hash of the *extracted* image, not the
    compressed download.  We cannot verify it without decompressing, so we
    only use image_download_size for the download check and leave sha as
    None.  The cross-check below validates extract_sha256 against
    release.json instead.
    """
    entries = []
    for item in data.get("os_list", []):
        entries.append(
            {
                "label": f"Imager: {item['name']}",
                "url": item["url"],
                "sha": None,
                "size": item.get("image_download_size"),
            }
        )
    return entries


def main() -> int:
    verify_sha = "--verify-sha" in sys.argv

    if "--help" in sys.argv or "-h" in sys.argv:
        print(__doc__)
        print("Usage: check-release-hashes.py [--verify-sha] [--check]")
        print()
        print("  --check       Size-only via HEAD (default, fast, CI-safe)")
        print("  --verify-sha  Also download files and verify SHA256 (slow)")
        return 0

    all_failures: list[str] = []

    # --- release.json ---
    print(f"\n── {RELEASE_JSON.relative_to(REPO_ROOT)}")
    if not RELEASE_JSON.exists():
        print("  MISSING — skipping")
    else:
        release = load_json(RELEASE_JSON)
        for entry in entries_from_release_json(release):
            failures = check_entry(
                entry["label"],
                entry["url"],
                entry["sha"],
                entry["size"],
                verify_sha,
            )
            all_failures.extend(failures)

    # --- os-list-velocity.json ---
    print(f"\n── {OS_LIST_JSON.relative_to(REPO_ROOT)}")
    if not OS_LIST_JSON.exists():
        print("  MISSING — skipping")
    else:
        os_list = load_json(OS_LIST_JSON)
        for entry in entries_from_os_list(os_list):
            failures = check_entry(
                entry["label"],
                entry["url"],
                entry["sha"],
                entry["size"],
                verify_sha,
            )
            all_failures.extend(failures)

    # --- cross-check: RPi image URL and size must agree between the two files ---
    print("\n── cross-check: release.json ↔ os-list-velocity.json")
    if RELEASE_JSON.exists() and OS_LIST_JSON.exists():
        release = load_json(RELEASE_JSON)
        os_list = load_json(OS_LIST_JSON)
        os_entries = os_list.get("os_list", [])
        rpi = release.get("rpi_image", {})
        if rpi.get("url") and os_entries:
            os_item = os_entries[0]
            checks = [
                ("url", rpi.get("url"), os_item.get("url")),
                (
                    "download_size",
                    rpi.get("download_size"),
                    os_item.get("image_download_size"),
                ),
                (
                    "extract_sha256",
                    rpi.get("extract_sha256"),
                    os_item.get("extract_sha256"),
                ),
            ]
            for field, a, b in checks:
                if a == b:
                    print(f"  {field}: OK")
                else:
                    msg = f"  {field}: MISMATCH (release.json={a}, os-list={b})"
                    print(msg)
                    all_failures.append(f"cross-check {field}: mismatch")
        else:
            print("  SKIP (RPi image not in both files)")
    else:
        print("  SKIP (one or both files missing)")

    # --- summary ---
    print()
    if all_failures:
        print(f"FAIL: {len(all_failures)} check(s) failed:")
        for f in all_failures:
            print(f"  • {f}")
        return 1
    else:
        print("OK: all release hash/size checks passed")
        return 0


if __name__ == "__main__":
    sys.exit(main())
