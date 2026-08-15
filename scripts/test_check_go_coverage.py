"""Tests for scripts/check_go_coverage.py."""

from __future__ import annotations

import importlib.util
import json
import sys
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).resolve().parents[1]
SCRIPT_PATH = REPO_ROOT / "scripts" / "check_go_coverage.py"
MODULE_NAME = "check_go_coverage"

PREFIX = "github.com/banshee-data/velocity.report/"


def load_module():
    if MODULE_NAME in sys.modules:
        return sys.modules[MODULE_NAME]
    spec = importlib.util.spec_from_file_location(MODULE_NAME, SCRIPT_PATH)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[MODULE_NAME] = module
    spec.loader.exec_module(module)
    return module


def write_profile(tmp_path: Path, body: str) -> Path:
    profile = tmp_path / "coverage.out"
    profile.write_text("mode: atomic\n" + body)
    return profile


def test_normalise_strips_module_prefix():
    mod = load_module()
    assert mod.normalise(PREFIX + "internal/api/serial.go") == "internal/api/serial.go"
    # Paths that are already repo-relative pass through untouched.
    assert mod.normalise("internal/api/serial.go") == "internal/api/serial.go"


def test_parse_profile_sums_blocks_per_file(tmp_path):
    mod = load_module()
    profile = write_profile(
        tmp_path,
        f"{PREFIX}a/x.go:10.1,12.2 3 1\n"
        f"{PREFIX}a/x.go:20.1,22.2 2 0\n"
        f"{PREFIX}b/y.go:5.1,6.2 4 1\n",
    )
    files = mod.parse_profile(profile)

    assert set(files) == {"a/x.go", "b/y.go"}
    covered, total = files["a/x.go"].totals(set(), False)
    assert (covered, total) == (3, 5)
    covered, total = files["b/y.go"].totals(set(), False)
    assert (covered, total) == (4, 4)


def test_parse_profile_merges_repeated_blocks(tmp_path):
    mod = load_module()
    # The same block can appear once per test binary; a block counts as
    # covered if any run covered it, and must not be double-counted.
    profile = write_profile(
        tmp_path,
        f"{PREFIX}a/x.go:10.1,12.2 3 0\n" f"{PREFIX}a/x.go:10.1,12.2 3 1\n",
    )
    files = mod.parse_profile(profile)
    assert files["a/x.go"].totals(set(), False) == (3, 3)


def test_parse_profile_rejects_non_profile(tmp_path):
    mod = load_module()
    bad = tmp_path / "not-a-profile.txt"
    bad.write_text("hello\n")
    with pytest.raises(SystemExit):
        mod.parse_profile(bad)


def test_parse_profile_rejects_missing_file(tmp_path):
    mod = load_module()
    with pytest.raises(SystemExit):
        mod.parse_profile(tmp_path / "nope.out")


def test_assign_functions_labels_blocks_by_enclosing_function(tmp_path):
    mod = load_module()
    profile = write_profile(
        tmp_path,
        f"{PREFIX}a/x.go:10.1,12.2 1 1\n"
        f"{PREFIX}a/x.go:25.1,26.2 1 0\n"
        f"{PREFIX}a/x.go:40.1,41.2 1 0\n",
    )
    files = mod.parse_profile(profile)
    # Helper starts at 5, Main at 20, Trailer at 35.
    starts = {"a/x.go": [(5, "Helper"), (20, "Main"), (35, "Trailer")]}

    mod.assign_functions(files, starts)

    got = [(b.start_line, b.func) for b in files["a/x.go"].blocks]
    assert got == [(10, "Helper"), (25, "Main"), (40, "Trailer")]


def test_totals_discounts_excluded_functions(tmp_path):
    mod = load_module()
    profile = write_profile(
        tmp_path,
        f"{PREFIX}a/x.go:10.1,12.2 2 1\n"  # Helper, covered
        f"{PREFIX}a/x.go:25.1,26.2 8 0\n",  # Main, uncovered
    )
    files = mod.parse_profile(profile)
    mod.assign_functions(files, {"a/x.go": [(5, "Helper"), (20, "Main")]})
    fc = files["a/x.go"]

    # Without exclusions the uncovered entrypoint dominates.
    assert fc.totals(set(), False) == (2, 10)
    # Excluding it leaves only the helper, which is fully covered.
    assert fc.totals({"Main"}, False) == (2, 2)
    # A wholly excluded file reports nothing at all.
    assert fc.totals(set(), True) == (0, 0)


def write_exclusions(tmp_path: Path, entries) -> Path:
    path = tmp_path / "exclusions.json"
    path.write_text(json.dumps({"exclusions": entries}))
    return path


def test_load_exclusions_requires_a_reason(tmp_path):
    mod = load_module()
    path = write_exclusions(tmp_path, [{"path": "a/x.go"}])
    with pytest.raises(SystemExit):
        mod.load_exclusions(path)


def test_load_exclusions_requires_a_path(tmp_path):
    mod = load_module()
    path = write_exclusions(tmp_path, [{"reason": "because"}])
    with pytest.raises(SystemExit):
        mod.load_exclusions(path)


def test_load_exclusions_rejects_invalid_json(tmp_path):
    mod = load_module()
    path = tmp_path / "exclusions.json"
    path.write_text("{not json")
    with pytest.raises(SystemExit):
        mod.load_exclusions(path)


def test_load_exclusions_missing_file_is_empty(tmp_path):
    mod = load_module()
    assert mod.load_exclusions(tmp_path / "absent.json") == []


def build_files(mod, tmp_path, body, starts):
    profile = write_profile(tmp_path, body)
    files = mod.parse_profile(profile)
    mod.assign_functions(files, starts)
    return files


def test_apply_exclusions_matches_globs(tmp_path):
    mod = load_module()
    files = build_files(
        mod,
        tmp_path,
        f"{PREFIX}pkg/pb/thing.pb.go:1.1,2.2 5 0\n" f"{PREFIX}pkg/real.go:1.1,2.2 5 1\n",
        {},
    )
    exclusions = [mod.Exclusion(pattern="**/*.pb.go", functions=[], reason="generated")]

    _, whole = mod.apply_exclusions(files, exclusions)

    assert whole == {"pkg/pb/thing.pb.go"}


def test_apply_exclusions_function_scoped(tmp_path):
    mod = load_module()
    files = build_files(
        mod,
        tmp_path,
        f"{PREFIX}a/x.go:10.1,12.2 2 1\n" f"{PREFIX}a/x.go:25.1,26.2 8 0\n",
        {"a/x.go": [(5, "Helper"), (20, "Main")]},
    )
    exclusions = [mod.Exclusion(pattern="a/x.go", functions=["Main"], reason="entrypoint")]

    per_file, whole = mod.apply_exclusions(files, exclusions)

    # A function-scoped exclusion must not take the whole file with it.
    assert whole == set()
    assert per_file == {"a/x.go": {"Main"}}


def test_stale_exclusion_detected_when_no_file_matches(tmp_path):
    mod = load_module()
    files = build_files(mod, tmp_path, f"{PREFIX}a/x.go:1.1,2.2 1 1\n", {})
    exclusions = [mod.Exclusion(pattern="gone/removed.go", functions=[], reason="stale")]

    mod.apply_exclusions(files, exclusions)
    problems = mod.stale_exclusion_errors(exclusions)

    assert len(problems) == 1
    assert "matches no file" in problems[0]


def test_stale_exclusion_detected_when_function_is_gone(tmp_path):
    mod = load_module()
    files = build_files(
        mod,
        tmp_path,
        f"{PREFIX}a/x.go:10.1,12.2 1 1\n",
        {"a/x.go": [(5, "Helper")]},
    )
    exclusions = [
        mod.Exclusion(pattern="a/x.go", functions=["Renamed"], reason="entrypoint")
    ]

    mod.apply_exclusions(files, exclusions)
    problems = mod.stale_exclusion_errors(exclusions)

    assert len(problems) == 1
    assert "not found" in problems[0]


def test_line_qualified_selector_disambiguates_same_named_functions(tmp_path):
    """"Name@line" pins one of several same-named functions in a file.

    internal/tailscale/manager.go declares Status twice: a one-line adapter
    method delegating to the tailscale client, and the exported Manager.Status
    that is testable. Excluding a bare "Status" would take both.
    """
    mod = load_module()
    files = build_files(
        mod,
        tmp_path,
        f"{PREFIX}a/x.go:88.1,89.2 1 0\n"  # adapter Status, uncovered
        f"{PREFIX}a/x.go:611.1,620.2 9 9\n",  # Manager.Status, covered
        {"a/x.go": [(87, "Status"), (610, "Status")]},
    )
    exclusions = [
        mod.Exclusion(pattern="a/x.go", functions=["Status@87"], reason="adapter")
    ]

    per_file, _ = mod.apply_exclusions(files, exclusions)

    assert per_file == {"a/x.go": {"Status@87"}}
    # Only the adapter is discounted; the testable Status still counts.
    assert files["a/x.go"].totals(per_file["a/x.go"], False) == (9, 9)


def test_bare_selector_matches_every_same_named_function(tmp_path):
    mod = load_module()
    files = build_files(
        mod,
        tmp_path,
        f"{PREFIX}a/x.go:88.1,89.2 1 0\n" f"{PREFIX}a/x.go:611.1,620.2 9 9\n",
        {"a/x.go": [(87, "Status"), (610, "Status")]},
    )
    exclusions = [mod.Exclusion(pattern="a/x.go", functions=["Status"], reason="both")]

    per_file, _ = mod.apply_exclusions(files, exclusions)

    # Without a line qualifier both declarations are discounted, leaving
    # nothing to measure.
    assert files["a/x.go"].totals(per_file["a/x.go"], False) == (0, 0)


def test_line_qualified_selector_that_matches_nothing_is_stale(tmp_path):
    mod = load_module()
    files = build_files(
        mod,
        tmp_path,
        f"{PREFIX}a/x.go:88.1,89.2 1 0\n",
        {"a/x.go": [(87, "Status")]},
    )
    # The function moved, so the pinned line no longer resolves.
    exclusions = [
        mod.Exclusion(pattern="a/x.go", functions=["Status@999"], reason="adapter")
    ]

    mod.apply_exclusions(files, exclusions)
    problems = mod.stale_exclusion_errors(exclusions)

    assert len(problems) == 1
    assert "not found" in problems[0]


def test_no_stale_problems_when_everything_matches(tmp_path):
    mod = load_module()
    files = build_files(
        mod,
        tmp_path,
        f"{PREFIX}a/x.go:10.1,12.2 1 1\n" f"{PREFIX}a/x.go:25.1,26.2 1 0\n",
        {"a/x.go": [(5, "Helper"), (20, "Main")]},
    )
    exclusions = [mod.Exclusion(pattern="a/x.go", functions=["Main"], reason="entrypoint")]

    mod.apply_exclusions(files, exclusions)

    assert mod.stale_exclusion_errors(exclusions) == []


def test_repo_exclusions_file_is_wellformed():
    """The checked-in list must parse and justify every entry."""
    mod = load_module()
    exclusions = mod.load_exclusions(REPO_ROOT / "scripts" / "coverage_exclusions.json")
    assert exclusions, "expected the repo exclusion list to be non-empty"
    for exc in exclusions:
        assert exc.reason.strip(), f"{exc.pattern} has an empty reason"
