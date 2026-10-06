#!/usr/bin/env python3
"""Reject result directories in the Git index, including force-added output."""

import argparse
import subprocess
import sys
from pathlib import PurePosixPath


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", default=".", help="Git checkout to inspect")
    args = parser.parse_args()
    tracked = subprocess.check_output(["git", "-C", args.repo, "ls-files", "-z"])
    forbidden = [
        path
        for path in tracked.decode("utf-8", errors="surrogateescape").split("\0")
        if path and any(part.casefold() == "results" for part in PurePosixPath(path).parts)
    ]
    if forbidden:
        print("ERROR: result directories must not be committed:", file=sys.stderr)
        for path in forbidden:
            print(f"  {path!r}", file=sys.stderr)
        print("Remove output from the index and preserve it locally.", file=sys.stderr)
        return 1
    print("OK: no result directories in the Git index")
    return 0


if __name__ == "__main__":
    sys.exit(main())
