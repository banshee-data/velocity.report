#!/usr/bin/env python3
"""Write a deterministic SHA-256 manifest for Go's embedded offline docs."""

from __future__ import annotations

import argparse
import hashlib
from pathlib import Path


def digest(path: Path) -> str:
    hasher = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            hasher.update(block)
    return hasher.hexdigest()


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "site_root",
        nargs="?",
        type=Path,
        default=Path("docs_html/_site"),
        help="generated offline docs directory",
    )
    args = parser.parse_args()
    site_root = args.site_root.resolve()
    # The generated site tree is explicitly whitelisted in .dockerignore,
    # unlike sibling dot-files. Docker verifies this manifest and removes it
    # before Go's `all:` embed is compiled.
    manifest = site_root / ".embed-manifest"
    if not site_root.is_dir():
        parser.error(f"offline docs site does not exist: {site_root}")

    files = sorted(
        path for path in site_root.rglob("*") if path.is_file() and path != manifest
    )
    if not files:
        parser.error(f"offline docs site contains no files: {site_root}")

    lines = [f"{digest(path)}  {path.relative_to(site_root).as_posix()}\n" for path in files]
    manifest.write_text("".join(lines), encoding="utf-8")
    print(f"✓ Wrote embedded docs manifest ({len(files)} file(s))")


if __name__ == "__main__":
    main()
