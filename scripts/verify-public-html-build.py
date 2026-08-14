#!/usr/bin/env python3
"""Verify that the public_html build is safe to mount at /public_html/.

The image build serves ``public_html/_site`` below ``/public_html/`` rather
than at the origin. This script checks the generated output without relying on
ripgrep or optional PCRE2 support.

Usage:
    python3 scripts/verify-public-html-build.py
    python3 scripts/verify-public-html-build.py path/to/_site
"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parent.parent
PATH_PREFIX = "/public_html/"
REQUIRED_FILES = (
    "index.html",
    "guides/setup/index.html",
    "tool/protractor/index.html",
)
# Root-relative asset and document URLs must remain beneath the image mount.
# URLs starting with // are protocol-relative and intentionally excluded.
ROOT_URL = re.compile(r'(?:href|src)="/(?!public_html/|/)')
ROOT_CSS_URL = re.compile(r'url\("/')


def find_matches(path: Path, pattern: re.Pattern[str]) -> list[int]:
    """Return 1-based line numbers containing matches for *pattern*."""
    return [
        line_number
        for line_number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), start=1)
        if pattern.search(line)
    ]


def verify(site_dir: Path) -> list[str]:
    """Return all build errors found beneath *site_dir*."""
    errors: list[str] = []

    for relative_path in REQUIRED_FILES:
        path = site_dir / relative_path
        if not path.is_file():
            errors.append(f"missing required build output: {path}")

    marker = site_dir / ".velocity-public-html-path-prefix"
    if not marker.is_file():
        errors.append(f"missing image path-prefix marker: {marker}")
    elif marker.read_text(encoding="utf-8").strip() != PATH_PREFIX:
        errors.append(f"image path-prefix marker must contain {PATH_PREFIX}: {marker}")

    for path in sorted(site_dir.rglob("*.html")):
        for line_number in find_matches(path, ROOT_URL):
            errors.append(
                f"root-relative href/src escapes {PATH_PREFIX}: {path}:{line_number}"
            )

    css_dir = site_dir / "css"
    if css_dir.is_dir():
        for path in sorted(css_dir.rglob("*.css")):
            for line_number in find_matches(path, ROOT_CSS_URL):
                errors.append(f"root-relative CSS URL escapes {PATH_PREFIX}: {path}:{line_number}")

    return errors


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "site_dir",
        nargs="?",
        type=Path,
        default=REPO_ROOT / "public_html" / "_site",
        help="generated public_html directory (default: public_html/_site)",
    )
    args = parser.parse_args()

    errors = verify(args.site_dir)
    if errors:
        for error in errors:
            print(f"ERROR: {error}", file=sys.stderr)
        return 1

    print(f"Public HTML build is safe to serve at {PATH_PREFIX}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
