"""Every file the binary embeds must reach the static build.

The static build runs in Docker, and image/Dockerfile.static-build.dockerignore
excludes everything and then re-includes what the build needs. A file that
assets.go embeds and the ignore file does not re-include is missing inside the
container, and the build fails there, and only there, with "no matching files
found". This test reads both files and fails first.
"""

import re
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
ASSETS = ROOT / "assets.go"
IGNORE = ROOT / "image" / "Dockerfile.static-build.dockerignore"


def embedded_patterns():
    patterns = []
    for line in ASSETS.read_text().splitlines():
        if line.startswith("//go:embed "):
            patterns.extend(line.split()[1:])
    return [p.removeprefix("all:") for p in patterns]


def ignore_rules():
    rules = []
    for line in IGNORE.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        negate = line.startswith("!")
        rules.append((negate, line.lstrip("!").rstrip("/")))
    return rules


def pattern_regex(pattern):
    """Docker's pattern syntax: * and ? stop at a slash, ** does not."""
    out, i = "", 0
    while i < len(pattern):
        if pattern.startswith("**", i):
            out += ".*"
            i += 2
            continue
        c = pattern[i]
        out += "[^/]*" if c == "*" else "[^/]" if c == "?" else re.escape(c)
        i += 1
    return re.compile(out + "$")


def in_context(path, rules):
    """Docker keeps a path unless the last pattern that matches it, or one of
    its parent directories, excludes it."""
    parts = path.split("/")
    prefixes = ["/".join(parts[:n]) for n in range(1, len(parts) + 1)]
    kept = True
    for negate, pattern in rules:
        regex = pattern_regex(pattern)
        if any(regex.match(prefix) for prefix in prefixes):
            kept = negate
    return kept


def sample_path(pattern):
    """A path the pattern embeds: the file it names, or one inside it."""
    if pattern.endswith("/*"):
        return pattern[:-1] + "index.html"
    if "." in Path(pattern).name:
        return pattern
    return pattern + "/index.html"


def test_every_embedded_file_reaches_the_static_build():
    rules = ignore_rules()
    patterns = embedded_patterns()
    assert "config/segment-selectors.defaults.json" in patterns
    missing = [p for p in patterns if not in_context(sample_path(p), rules)]
    assert missing == [], f"add these to {IGNORE.name}: {missing}"


def test_the_context_rules_are_read_as_docker_reads_them():
    rules = ignore_rules()
    # Everything not re-included stays out, and tests are dropped from it.
    assert not in_context("README.md", rules)
    assert not in_context("config/tuning.example.json", rules)
    assert not in_context("internal/lidar/segments/rank_test.go", rules)
    assert in_context("internal/lidar/segments/rank.go", rules)
    assert in_context("config/tuning.defaults.json", rules)
