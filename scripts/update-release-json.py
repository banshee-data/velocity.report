#!/usr/bin/env python3
"""Update public_html/src/_data/release.json (and image/os-list-velocity.json)
from the velocity.report GitHub Releases.

For each selected platform, picks the most recent GitHub release that
matches the requested channel and carries the expected asset, streams the
asset to compute its SHA256, and rewrites the corresponding entry. The
rpi_image entry also decompresses the .img.xz stream to capture the
extract_size and extract_sha256, and mirrors those fields into
image/os-list-velocity.json.

Usage:
    scripts/update-release-json.py [options]

Options:
    --channel {stable,prerelease,both}   default: both
    --platform NAME                      repeatable; one of:
                                         linux_arm64, mac_arm64, visualiser, rpi_image
    --all                                shorthand for every known platform
    --ci                                 shorthand for linux_arm64, mac_arm64, rpi_image
                                         (the three CI-built artefacts)
    --tag vX.Y.Z                         pin all updates to a specific release tag
                                         (overrides channel selection)
    --validate                           after writing, re-download and verify
                                         (defers to check-release-hashes.py --verify-sha)
    --dry-run                            compute and print, do not write
    --quiet                              suppress progress noise

Environment:
    GITHUB_TOKEN    required. Needed for rate-limited API calls and to download
                    private assets. A read-only token is enough for public repos.

Exit codes:
    0  success
    1  one or more platforms failed to update (no partial writes on error)
    2  usage / configuration error
"""

from __future__ import annotations

import argparse
import hashlib
import json
import lzma
import os
import re
import subprocess
import sys
import urllib.error
import urllib.request
from pathlib import Path
from typing import Optional

REPO = "banshee-data/velocity.report"
REPO_ROOT = Path(__file__).resolve().parent.parent
RELEASE_JSON = REPO_ROOT / "public_html" / "src" / "_data" / "release.json"
OS_LIST_JSON = REPO_ROOT / "image" / "os-list-velocity.json"
CHECK_SCRIPT = REPO_ROOT / "scripts" / "check-release-hashes.py"

GITHUB_API = f"https://api.github.com/repos/{REPO}"

# Asset-name patterns, anchored to filename (not full URL). Each platform picks
# the first asset in a release whose name matches.
PLATFORM_ASSET_RE: dict[str, re.Pattern[str]] = {
    "linux_arm64": re.compile(r"^velocity-report-.+-linux-arm64$"),
    "mac_arm64": re.compile(r"^velocity-report-.+-darwin-arm64$"),
    "visualiser": re.compile(r"^VelocityVisualiser-.+\.dmg$"),
    "rpi_image": re.compile(r"velocity-report.*\.img\.xz$"),
}

ALL_PLATFORMS = list(PLATFORM_ASSET_RE.keys())
CI_PLATFORMS = ["linux_arm64", "mac_arm64", "rpi_image"]

# Platforms that live under stable/prerelease channels in release.json.
# rpi_image lives at the top level (single slot).
CHANNELED_PLATFORMS = ["linux_arm64", "mac_arm64", "visualiser"]


# ---------------------------------------------------------------------------
# HTTP helpers
# ---------------------------------------------------------------------------


def _auth_headers(token: str, accept: str = "application/vnd.github+json") -> dict:
    return {
        "Authorization": f"Bearer {token}",
        "Accept": accept,
        "User-Agent": "velocity-release-updater/1.0",
        "X-GitHub-Api-Version": "2022-11-28",
    }


def gh_api(path: str, token: str) -> dict | list:
    url = f"{GITHUB_API}{path}"
    req = urllib.request.Request(url, headers=_auth_headers(token))
    with urllib.request.urlopen(req, timeout=30) as resp:
        return json.load(resp)


def list_releases(token: str, per_page: int = 30) -> list[dict]:
    """List releases newest-first. Paginates up to 3 pages (90 releases)."""
    out: list[dict] = []
    for page in range(1, 4):
        batch = gh_api(f"/releases?per_page={per_page}&page={page}", token)
        if not isinstance(batch, list):
            break
        out.extend(batch)
        if len(batch) < per_page:
            break
    return out


def get_release_by_tag(tag: str, token: str) -> dict:
    result = gh_api(f"/releases/tags/{tag}", token)
    if not isinstance(result, dict):
        raise RuntimeError(f"unexpected API response for tag {tag!r}")
    return result


# ---------------------------------------------------------------------------
# Release selection
# ---------------------------------------------------------------------------


def pick_release(
    releases: list[dict],
    channel: str,
    asset_re: re.Pattern[str],
) -> Optional[tuple[dict, dict]]:
    """Return (release, asset) for the newest release matching channel with a
    name-matching asset. `channel` is one of: stable | prerelease | both.
    Draft releases are always skipped.
    """
    for rel in releases:
        if rel.get("draft"):
            continue
        is_pre = bool(rel.get("prerelease"))
        if channel == "stable" and is_pre:
            continue
        if channel == "prerelease" and not is_pre:
            continue
        for asset in rel.get("assets", []) or []:
            if asset_re.match(asset.get("name", "")):
                return rel, asset
    return None


# ---------------------------------------------------------------------------
# Streaming hashers
# ---------------------------------------------------------------------------


def _open_download(url: str, token: str):
    # Assets require Accept: application/octet-stream to return the binary.
    req = urllib.request.Request(
        url, headers=_auth_headers(token, accept="application/octet-stream")
    )
    return urllib.request.urlopen(req, timeout=300)


def stream_sha_and_size(url: str, token: str, log) -> tuple[str, int]:
    """Stream a URL, returning (sha256 hex, bytes read)."""
    h = hashlib.sha256()
    total = 0
    log(f"    download: {url}")
    with _open_download(url, token) as resp:
        while chunk := resp.read(1 << 20):
            h.update(chunk)
            total += len(chunk)
    log(f"    sha256:   {h.hexdigest()}  ({total} bytes)")
    return h.hexdigest(), total


def stream_rpi(url: str, token: str, log) -> tuple[str, int, str, int]:
    """For an .img.xz URL, return (download_sha, download_size,
    extract_sha, extract_size) in one pass."""
    dl_h = hashlib.sha256()
    ex_h = hashlib.sha256()
    dl_bytes = 0
    ex_bytes = 0
    dec = lzma.LZMADecompressor(format=lzma.FORMAT_XZ)
    log(f"    download: {url}")
    with _open_download(url, token) as resp:
        while chunk := resp.read(1 << 20):
            dl_h.update(chunk)
            dl_bytes += len(chunk)
            plain = dec.decompress(chunk)
            if plain:
                ex_h.update(plain)
                ex_bytes += len(plain)
    if not dec.eof:
        raise RuntimeError("xz stream ended before end-of-stream marker")
    log(
        f"    sha256:         {dl_h.hexdigest()}  ({dl_bytes} bytes)\n"
        f"    extract sha256: {ex_h.hexdigest()}  ({ex_bytes} bytes extracted)"
    )
    return dl_h.hexdigest(), dl_bytes, ex_h.hexdigest(), ex_bytes


# ---------------------------------------------------------------------------
# Updaters
# ---------------------------------------------------------------------------


def _strip_tag(tag: str) -> str:
    return tag[1:] if tag.startswith("v") else tag


def update_channeled_platform(
    platform: str,
    channel: str,
    releases: list[dict],
    pinned_release: Optional[dict],
    token: str,
    log,
) -> dict:
    """Return the new {version, url, sha256} entry for stable or prerelease."""
    asset_re = PLATFORM_ASSET_RE[platform]
    if pinned_release is not None:
        asset = next(
            (a for a in pinned_release.get("assets", []) if asset_re.match(a["name"])),
            None,
        )
        if asset is None:
            raise RuntimeError(
                f"pinned tag {pinned_release['tag_name']} has no {platform} asset"
            )
        release = pinned_release
    else:
        hit = pick_release(releases, channel, asset_re)
        if hit is None:
            raise RuntimeError(f"no {channel!r} release found with a {platform} asset")
        release, asset = hit

    version = _strip_tag(release["tag_name"])
    url = asset["browser_download_url"]
    log(f"  → matched {release['tag_name']} asset {asset['name']}")
    sha, _ = stream_sha_and_size(url, token, log)
    return {"version": version, "url": url, "sha256": sha}


def update_rpi_entry(
    channel: str,
    releases: list[dict],
    pinned_release: Optional[dict],
    token: str,
    log,
) -> dict:
    """Return the new rpi_image dict (including extract_sha256 etc.)."""
    asset_re = PLATFORM_ASSET_RE["rpi_image"]
    if pinned_release is not None:
        asset = next(
            (a for a in pinned_release.get("assets", []) if asset_re.match(a["name"])),
            None,
        )
        if asset is None:
            raise RuntimeError(
                f"pinned tag {pinned_release['tag_name']} has no rpi_image asset"
            )
        release = pinned_release
    else:
        hit = pick_release(releases, channel, asset_re)
        if hit is None:
            raise RuntimeError(f"no {channel!r} release found with an rpi_image asset")
        release, asset = hit

    version = _strip_tag(release["tag_name"])
    url = asset["browser_download_url"]
    # published_at is ISO 8601 with trailing Z, e.g. "2026-04-18T03:12:08Z".
    published_at = release.get("published_at") or release.get("created_at") or ""
    release_date = published_at[:10]  # "2026-04-18"
    log(f"  → matched {release['tag_name']} asset {asset['name']}")
    dl_sha, dl_size, ex_sha, ex_size = stream_rpi(url, token, log)
    return {
        "version": version,
        "url": url,
        "sha256": dl_sha,
        "extract_sha256": ex_sha,
        "release_url": release["html_url"],
        "release_date": release_date,
        "download_size": dl_size,
        "extract_size": ex_size,
    }


def sync_os_list(rpi: dict, os_list: dict) -> None:
    """Update the first os_list entry to match the rpi_image block."""
    items = os_list.get("os_list") or []
    if not items:
        raise RuntimeError("os-list-velocity.json has no os_list entries")
    item = items[0]
    item["url"] = rpi["url"]
    item["image_download_size"] = rpi["download_size"]
    item["extract_size"] = rpi["extract_size"]
    item["extract_sha256"] = rpi["extract_sha256"]
    # release_date: from the GitHub release's published_at timestamp.
    release_date = rpi.get("release_date", "")
    if release_date:
        item["release_date"] = release_date


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------


def parse_args(argv: list[str]) -> argparse.Namespace:
    p = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    p.add_argument(
        "--channel", choices=["stable", "prerelease", "both"], default="both"
    )
    p.add_argument(
        "--platform",
        action="append",
        choices=ALL_PLATFORMS,
        default=[],
        help="repeatable",
    )
    p.add_argument("--all", action="store_true", help=f"shorthand for {ALL_PLATFORMS}")
    p.add_argument("--ci", action="store_true", help=f"shorthand for {CI_PLATFORMS}")
    p.add_argument(
        "--tag", help="pin all updates to a specific release tag, e.g. v0.5.2-pre1"
    )
    p.add_argument("--validate", action="store_true")
    p.add_argument("--dry-run", action="store_true")
    p.add_argument("--quiet", action="store_true")
    return p.parse_args(argv)


def resolve_platforms(args: argparse.Namespace) -> list[str]:
    sel: list[str] = []
    if args.all:
        sel = list(ALL_PLATFORMS)
    elif args.ci:
        sel = list(CI_PLATFORMS)
    if args.platform:
        for name in args.platform:
            if name not in sel:
                sel.append(name)
    if not sel:
        sel = list(ALL_PLATFORMS)
    return sel


def run(args: argparse.Namespace) -> int:
    token = os.environ.get("GITHUB_TOKEN")
    if not token:
        print("error: GITHUB_TOKEN is required", file=sys.stderr)
        return 2

    platforms = resolve_platforms(args)
    channels: list[str]
    if args.channel == "both":
        channels = ["stable", "prerelease"]
    else:
        channels = [args.channel]

    def log(msg: str) -> None:
        if not args.quiet:
            print(msg)

    log(f"platforms: {', '.join(platforms)}")
    log(f"channels:  {', '.join(channels)}")
    if args.tag:
        log(f"pinned tag: {args.tag}")
    log("")

    # Load current state. We write back to disk only once at the end, so any
    # mid-flight error leaves the files untouched.
    release_data = json.loads(RELEASE_JSON.read_text())
    os_list_data = json.loads(OS_LIST_JSON.read_text())

    pinned_release: Optional[dict] = None
    releases: list[dict] = []
    if args.tag:
        try:
            pinned_release = get_release_by_tag(args.tag, token)
        except urllib.error.HTTPError as e:
            print(f"error: fetching tag {args.tag}: {e}", file=sys.stderr)
            return 1
    else:
        releases = list_releases(token)
        if not releases:
            print("error: no releases returned from GitHub API", file=sys.stderr)
            return 1

    failures: list[str] = []
    is_both = args.channel == "both"

    # Channeled platforms: iterate over requested channels.
    # When --channel=both, a missing channel is a skip (the platform may only
    # have stable or prerelease releases). It becomes a hard failure only if
    # BOTH channels miss, or if the user explicitly requested a single channel.
    platform_hits: dict[str, int] = {}
    for platform in platforms:
        if platform == "rpi_image":
            continue
        platform_hits[platform] = 0
        for channel in channels:
            label = f"{channel}.{platform}"
            log(f"[{label}]")
            try:
                entry = update_channeled_platform(
                    platform, channel, releases, pinned_release, token, log
                )
            except Exception as exc:
                if is_both:
                    log(f"  skip {label}: {exc}")
                else:
                    msg = f"{label}: {exc}"
                    print(f"  FAIL {msg}", file=sys.stderr)
                    failures.append(msg)
                continue
            platform_hits[platform] += 1
            release_data.setdefault(channel, {})[platform] = entry
            log("")

    # When --channel=both, fail only for platforms with zero hits.
    if is_both:
        for platform, hits in platform_hits.items():
            if hits == 0:
                msg = f"{platform}: no release found in any channel"
                print(f"  FAIL {msg}", file=sys.stderr)
                failures.append(msg)

    # rpi_image: single slot, not channeled in the JSON.
    if "rpi_image" in platforms:
        # When --channel=both and no pin, prefer prerelease for rpi_image
        # because that's the current convention in release.json.
        rpi_channel = args.channel if args.channel != "both" else "prerelease"
        log(f"[rpi_image · {rpi_channel}]")
        try:
            new_rpi = update_rpi_entry(
                rpi_channel, releases, pinned_release, token, log
            )
        except Exception as exc:
            msg = f"rpi_image: {exc}"
            print(f"  FAIL {msg}", file=sys.stderr)
            failures.append(msg)
        else:
            # Preserve existing rpi_imager_repo_url at top level; merge the
            # nested block in place.
            old_rpi = release_data.get("rpi_image") or {}
            merged = {**old_rpi, **new_rpi}
            release_data["rpi_image"] = merged
            sync_os_list(merged, os_list_data)
            log("")

    if failures:
        print(f"\n{len(failures)} failure(s); not writing files:", file=sys.stderr)
        for f in failures:
            print(f"  • {f}", file=sys.stderr)
        return 1

    if args.dry_run:
        log("--dry-run: not writing files. Updated documents below:\n")
        print(json.dumps(release_data, indent=2))
        print()
        print(json.dumps(os_list_data, indent=2))
        return 0

    RELEASE_JSON.write_text(json.dumps(release_data, indent=2) + "\n")
    OS_LIST_JSON.write_text(json.dumps(os_list_data, indent=2) + "\n")
    log(f"wrote {RELEASE_JSON.relative_to(REPO_ROOT)}")
    log(f"wrote {OS_LIST_JSON.relative_to(REPO_ROOT)}")

    if args.validate:
        log("")
        log("validating with check-release-hashes.py --verify-sha ...")
        rc = subprocess.call(
            [sys.executable, str(CHECK_SCRIPT), "--verify-sha"],
            cwd=str(REPO_ROOT),
        )
        if rc != 0:
            return 1

    return 0


def main() -> int:
    args = parse_args(sys.argv[1:])
    try:
        return run(args)
    except KeyboardInterrupt:
        return 130


if __name__ == "__main__":
    sys.exit(main())
