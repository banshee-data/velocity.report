#!/usr/bin/env python3
"""Enforce a per-file Go coverage floor, with reviewed exclusions.

Reads a Go coverage profile and fails if any file falls below the threshold
once excluded code is discounted.

Exclusions live in scripts/coverage_exclusions.json and are deliberately
narrow: an entry names a file (or glob) and, optionally, the specific
functions to discount.  A function may be written as "Name" or, when a file
declares the same name more than once (an adapter method and the exported
function it wraps, say), as "Name@declLine" to pin one of them.  Naming functions rather than whole files matters for
mixed files like internal/cmd/server/radar.go, where the process entrypoint is
untestable but the surrounding helpers are not — excluding the whole file
would hide those helpers from the gate.

Stale exclusions are treated as errors: an entry that matches no file, or
names a function that no longer exists, fails the run.  Otherwise the list
silently rots as code moves.

Usage:
    python3 scripts/check_go_coverage.py --profile coverage.out --threshold 82
"""

from __future__ import annotations

import argparse
import fnmatch
import json
import re
import subprocess
import sys
from dataclasses import dataclass, field
from pathlib import Path
from typing import Iterable

MODULE_PREFIX = "github.com/banshee-data/velocity.report/"
DEFAULT_EXCLUSIONS = Path("scripts/coverage_exclusions.json")

# "path/to/file.go:41:\tFuncName\t100.0%"
FUNC_LINE = re.compile(r"^(?P<path>.+):(?P<line>\d+):\s+(?P<name>\S+)\s+(?P<pct>[\d.]+)%$")


@dataclass
class Block:
    """One coverage block: a contiguous run of statements."""

    start_line: int
    statements: int
    covered: bool
    func: str = ""
    # Line the enclosing function is declared on. Used to disambiguate
    # same-named functions in one file (e.g. an adapter's Status method and
    # the exported Status it wraps).
    func_line: int = 0

    def matches(self, selector: str) -> bool:
        """True if this block's function matches a "Name" or "Name@line" selector."""
        name, _, line = selector.partition("@")
        if self.func != name:
            return False
        if not line:
            return True
        return str(self.func_line) == line


@dataclass
class FileCoverage:
    path: str
    blocks: "list[Block]" = field(default_factory=list)

    def totals(self, excluded_funcs: "set[str]", whole_file: bool) -> "tuple[int, int]":
        """Return (covered, total) statements after discounting exclusions."""
        if whole_file:
            return (0, 0)
        covered = total = 0
        for b in self.blocks:
            if any(b.matches(sel) for sel in excluded_funcs):
                continue
            total += b.statements
            if b.covered:
                covered += b.statements
        return (covered, total)


def normalise(path: str) -> str:
    """Strip the module prefix so profile paths match repo-relative paths."""
    if path.startswith(MODULE_PREFIX):
        return path[len(MODULE_PREFIX) :]
    return path


def parse_profile(profile: Path) -> "dict[str, FileCoverage]":
    """Parse a Go coverprofile into per-file blocks.

    Repeated blocks (the same range seen more than once, which happens when a
    package is covered by several test binaries) are merged: a block counts as
    covered if any run covered it.
    """
    if not profile.exists():
        sys.exit(f"coverage profile not found: {profile}")

    merged: "dict[tuple[str, str], tuple[int, int, bool]]" = {}
    with profile.open() as fh:
        first = fh.readline()
        if not first.startswith("mode:"):
            sys.exit(f"{profile} is not a Go coverprofile")
        for lineno, raw in enumerate(fh, start=2):
            line = raw.strip()
            if not line:
                continue
            try:
                location, statements_s, count_s = line.split()
                filename, block_range = location.split(":", 1)
                start_line = int(block_range.split(".", 1)[0])
                statements = int(statements_s)
                covered = int(count_s) > 0
            except ValueError:
                sys.exit(f"invalid coverprofile line {lineno}: {line}")
            key = (normalise(filename), block_range)
            prev = merged.get(key)
            if prev is None:
                merged[key] = (start_line, statements, covered)
            else:
                merged[key] = (prev[0], prev[1], prev[2] or covered)

    files: "dict[str, FileCoverage]" = {}
    for (path, _), (start_line, statements, covered) in merged.items():
        files.setdefault(path, FileCoverage(path=path)).blocks.append(
            Block(start_line=start_line, statements=statements, covered=covered)
        )
    for fc in files.values():
        fc.blocks.sort(key=lambda b: b.start_line)
    return files


def function_starts(profile: Path) -> "dict[str, list[tuple[int, str]]]":
    """Return {file: [(start_line, func_name), ...]} via `go tool cover -func`.

    The profile alone carries no function names, so the boundaries come from
    the Go toolchain. Each file's list is sorted by start line.
    """
    try:
        out = subprocess.run(
            ["go", "tool", "cover", f"-func={profile}"],
            capture_output=True,
            check=True,
            text=True,
        ).stdout
    except (subprocess.CalledProcessError, FileNotFoundError) as exc:
        sys.exit(f"go tool cover -func failed: {exc}")

    starts: "dict[str, list[tuple[int, str]]]" = {}
    for raw in out.splitlines():
        line = raw.rstrip()
        if not line or line.startswith("total:"):
            continue
        m = FUNC_LINE.match(line)
        if not m:
            continue
        path = normalise(m.group("path"))
        starts.setdefault(path, []).append((int(m.group("line")), m.group("name")))
    for entries in starts.values():
        entries.sort()
    return starts


def assign_functions(
    files: "dict[str, FileCoverage]",
    starts: "dict[str, list[tuple[int, str]]]",
) -> None:
    """Label each block with its enclosing function.

    A block belongs to the last function whose declaration starts at or before
    the block's first line.
    """
    for path, fc in files.items():
        entries = starts.get(path, [])
        if not entries:
            continue
        idx = 0
        current = ""
        current_line = 0
        for block in fc.blocks:  # blocks are sorted by start_line
            while idx < len(entries) and entries[idx][0] <= block.start_line:
                current_line, current = entries[idx]
                idx += 1
            block.func = current
            block.func_line = current_line


@dataclass
class Exclusion:
    pattern: str
    functions: "list[str]"
    reason: str
    # optional entries may match nothing without being reported as stale.
    # Reserved for build-tag-conditional files: a !pcap stub simply is not
    # compiled (and so never appears in the profile) when -tags=pcap is set.
    optional: bool = False
    matched_files: "set[str]" = field(default_factory=set)
    matched_funcs: "set[str]" = field(default_factory=set)

    @property
    def whole_file(self) -> bool:
        return not self.functions


def load_exclusions(path: Path) -> "list[Exclusion]":
    if not path.exists():
        return []
    try:
        raw = json.loads(path.read_text())
    except json.JSONDecodeError as exc:
        sys.exit(f"{path} is not valid JSON: {exc}")

    entries = raw.get("exclusions", raw if isinstance(raw, list) else [])
    out: "list[Exclusion]" = []
    for i, entry in enumerate(entries):
        pattern = entry.get("path")
        if not pattern:
            sys.exit(f"{path}: exclusion #{i} has no \"path\"")
        reason = entry.get("reason", "")
        if not reason:
            sys.exit(f"{path}: exclusion for {pattern} has no \"reason\"")
        out.append(
            Exclusion(
                pattern=pattern,
                functions=list(entry.get("functions", [])),
                reason=reason,
                optional=bool(entry.get("optional", False)),
            )
        )
    return out


def apply_exclusions(
    files: "dict[str, FileCoverage]",
    exclusions: "Iterable[Exclusion]",
) -> "tuple[dict[str, set[str]], set[str]]":
    """Return (per-file excluded function names, wholly excluded files).

    Records what each exclusion matched so stale entries can be reported.
    """
    excluded_funcs: "dict[str, set[str]]" = {}
    whole_files: "set[str]" = set()

    for exc in exclusions:
        for path, fc in files.items():
            if not fnmatch.fnmatch(path, exc.pattern):
                continue
            exc.matched_files.add(path)
            if exc.whole_file:
                whole_files.add(path)
                continue
            for fn in exc.functions:
                if any(b.matches(fn) for b in fc.blocks):
                    exc.matched_funcs.add(fn)
                    excluded_funcs.setdefault(path, set()).add(fn)
    return excluded_funcs, whole_files


def stale_exclusion_errors(exclusions: "Iterable[Exclusion]") -> "list[str]":
    """Report exclusions that no longer match anything."""
    problems: "list[str]" = []
    for exc in exclusions:
        if not exc.matched_files:
            if not exc.optional:
                problems.append(f"{exc.pattern}: matches no file in the coverage profile")
            continue
        missing = [fn for fn in exc.functions if fn not in exc.matched_funcs]
        if missing:
            problems.append(
                f"{exc.pattern}: function(s) not found: {', '.join(sorted(missing))}"
            )
    return problems


def main(argv: "list[str] | None" = None) -> int:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--profile", type=Path, default=Path("coverage.out"))
    p.add_argument("--threshold", type=float, default=82.0)
    p.add_argument("--exclusions", type=Path, default=DEFAULT_EXCLUSIONS)
    p.add_argument(
        "--list-excluded",
        action="store_true",
        help="print what the exclusion list discounted, then continue",
    )
    args = p.parse_args(list(argv) if argv is not None else None)

    files = parse_profile(args.profile)
    assign_functions(files, function_starts(args.profile))
    exclusions = load_exclusions(args.exclusions)
    excluded_funcs, whole_files = apply_exclusions(files, exclusions)

    if args.list_excluded:
        print("Coverage exclusions:")
        for exc in exclusions:
            scope = "whole file" if exc.whole_file else ", ".join(exc.functions)
            print(f"  {exc.pattern} [{scope}]")
            print(f"      reason: {exc.reason}")
        print()

    failures: "list[tuple[str, float, int, int]]" = []
    skipped = 0
    for path in sorted(files):
        covered, total = files[path].totals(
            excluded_funcs.get(path, set()), path in whole_files
        )
        if total == 0:
            # Either wholly excluded, or every statement was excluded.
            skipped += 1
            continue
        pct = covered * 100.0 / total
        if pct < args.threshold:
            failures.append((path, pct, covered, total))

    print(f"Per-file Go coverage threshold: {args.threshold:.1f}%")
    print(f"  {len(files) - skipped} file(s) checked, {skipped} excluded")

    problems = stale_exclusion_errors(exclusions)
    if problems:
        print()
        print("Stale exclusions (remove or fix these):")
        for problem in problems:
            print(f"  {problem}")

    if failures:
        print()
        print(f"{len(failures)} file(s) below {args.threshold:.1f}%:")
        for path, pct, covered, total in sorted(failures, key=lambda f: f[1]):
            print(f"  {pct:6.1f}% {covered:5}/{total:<5} {path}")

    if failures or problems:
        return 1
    print("  all files meet the threshold")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
