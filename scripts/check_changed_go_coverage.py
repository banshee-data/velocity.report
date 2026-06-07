#!/usr/bin/env python3
"""Check per-file Go coverage for Go files changed on a branch."""

from __future__ import annotations

import argparse
import os
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path

MODULE_PREFIX = "github.com/banshee-data/velocity.report/"
DEFAULT_PROFILE = Path(".tmp/changed-go-coverage.out")


@dataclass(frozen=True)
class FileCoverage:
    path: str
    total: int
    covered: int

    @property
    def percent(self) -> float:
        if self.total == 0:
            return 100.0
        return self.covered * 100.0 / self.total


def run_git(args: list[str], cwd: Path) -> list[str]:
	out = subprocess.check_output(["git", *args], cwd=cwd, text=True)
	return [line for line in out.splitlines() if line]


def add_unique(paths: list[str], seen: set[str], new_paths: list[str]) -> None:
    for path in new_paths:
        if path in seen:
            continue
        seen.add(path)
        paths.append(path)


def path_in_scope(path: str, include_prefixes: list[str], exclude_prefixes: list[str]) -> bool:
    if include_prefixes and not any(path.startswith(prefix) for prefix in include_prefixes):
        return False
    return not any(path.startswith(prefix) for prefix in exclude_prefixes)


def changed_go_files(
    base: str,
    cwd: Path,
    diff_filter: str,
    include_prefixes: list[str],
    exclude_prefixes: list[str],
) -> list[str]:
    files: list[str] = []
    seen: set[str] = set()
    diff_args = ["diff", "--name-only", f"--diff-filter={diff_filter}"]
    add_unique(files, seen, run_git([*diff_args, f"{base}...HEAD", "--", "*.go"], cwd))
    add_unique(files, seen, run_git([*diff_args, "HEAD", "--", "*.go"], cwd))
    if "A" in diff_filter:
        add_unique(files, seen, run_git(["ls-files", "--others", "--exclude-standard", "--", "*.go"], cwd))

    out: list[str] = []
    for path in files:
        if path.endswith("_test.go"):
            continue
        if not (cwd / path).is_file():
            continue
        if not path_in_scope(path, include_prefixes, exclude_prefixes):
            continue
        out.append(path)
    return out


def changed_package_patterns(files: list[str]) -> list[str]:
    dirs = sorted({str(Path(path).parent) for path in files})
    return ["." if d == "." else f"./{d}" for d in dirs]


def import_paths_for_patterns(patterns: list[str], cwd: Path) -> list[str]:
    if not patterns:
        return []
    out = subprocess.check_output(["go", "list", *patterns], cwd=cwd, env=go_env(cwd), text=True)
    return [line for line in out.splitlines() if line]


def go_env(cwd: Path) -> dict[str, str]:
    env = os.environ.copy()
    env.setdefault("GOCACHE", str(cwd / ".gocache"))
    return env


def run_go_coverage(
    packages: list[str],
    profile: Path,
    cwd: Path,
    tags: str,
) -> None:
    if not packages:
        profile.parent.mkdir(parents=True, exist_ok=True)
        profile.write_text("mode: atomic\n")
        return

    profile.parent.mkdir(parents=True, exist_ok=True)
    coverpkg = ",".join(packages)
    cmd = [
        "go",
        "test",
        "-covermode=atomic",
        f"-coverpkg={coverpkg}",
        f"-coverprofile={profile}",
    ]
    if tags:
        cmd.insert(2, f"-tags={tags}")
    cmd.extend(packages)
    subprocess.check_call(cmd, cwd=cwd, env=go_env(cwd))


def normalize_profile_path(path: str) -> str:
    if path.startswith(MODULE_PREFIX):
        return path[len(MODULE_PREFIX) :]
    return path


def parse_coverage_profile(profile: Path) -> dict[str, FileCoverage]:
    blocks: dict[tuple[str, str], tuple[int, bool]] = {}
    with profile.open() as f:
        first = f.readline()
        if not first.startswith("mode:"):
            raise ValueError(f"{profile} is not a Go coverprofile")

        for lineno, raw in enumerate(f, start=2):
            line = raw.strip()
            if not line:
                continue
            try:
                location, statements, count = line.split()
                filename, block_range = location.split(":", 1)
            except ValueError as exc:
                raise ValueError(f"invalid coverprofile line {lineno}: {line}") from exc
            rel = normalize_profile_path(filename)
            key = (rel, block_range)
            covered = int(count) > 0
            statement_count = int(statements)
            previous = blocks.get(key)
            if previous is None:
                blocks[key] = (statement_count, covered)
            else:
                blocks[key] = (previous[0], previous[1] or covered)

    totals: dict[str, tuple[int, int]] = {}
    for (path, _), (statements, covered) in blocks.items():
        total, hit = totals.get(path, (0, 0))
        total += statements
        if covered:
            hit += statements
        totals[path] = (total, hit)

    return {
        path: FileCoverage(path=path, total=total, covered=covered)
        for path, (total, covered) in totals.items()
    }


def check_files(
    files: list[str],
    coverage: dict[str, FileCoverage],
    threshold: float,
) -> list[FileCoverage]:
    failures: list[FileCoverage] = []
    for path in files:
        cov = coverage.get(path, FileCoverage(path=path, total=0, covered=0))
        if cov.total > 0 and cov.percent < threshold:
            failures.append(cov)
    return failures


def print_summary(files: list[str], coverage: dict[str, FileCoverage], threshold: float) -> None:
    if not files:
        print("No changed non-test Go files.")
        return

    print(f"Changed Go file coverage threshold: {threshold:.1f}%")
    for path in files:
        cov = coverage.get(path, FileCoverage(path=path, total=0, covered=0))
        if cov.total == 0:
            print(f"  skip  100.0%    0/0    {path}")
            continue
        status = "ok" if cov.percent >= threshold else "fail"
        print(f"  {status:<4} {cov.percent:6.1f}% {cov.covered:4}/{cov.total:<4} {path}")


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", default=os.environ.get("BASE_REF", "main"))
    parser.add_argument("--threshold", type=float, default=98.0)
    parser.add_argument("--profile", type=Path, default=DEFAULT_PROFILE)
    parser.add_argument("--diff-filter", default="ACMRT")
    parser.add_argument("--include-prefix", action="append", default=[])
    parser.add_argument("--exclude-prefix", action="append", default=[])
    parser.add_argument("--run-go-test", action="store_true")
    parser.add_argument("--tags", default="pcap")
    parser.add_argument("--print-packages", action="store_true")
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(sys.argv[1:] if argv is None else argv)
    cwd = Path.cwd()
    files = changed_go_files(
        args.base,
        cwd,
        args.diff_filter,
        args.include_prefix,
        args.exclude_prefix,
    )
    patterns = changed_package_patterns(files)

    if args.print_packages:
        print(" ".join(patterns))
        return 0

    if args.run_go_test:
        packages = import_paths_for_patterns(patterns, cwd)
        run_go_coverage(packages, args.profile, cwd, args.tags)

    coverage = parse_coverage_profile(args.profile)
    failures = check_files(files, coverage, args.threshold)
    print_summary(files, coverage, args.threshold)
    if failures:
        print()
        print(f"{len(failures)} changed Go file(s) are below {args.threshold:.1f}% coverage.")
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
