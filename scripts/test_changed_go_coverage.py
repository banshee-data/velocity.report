from __future__ import annotations

import importlib.machinery
import importlib.util
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SCRIPT_PATH = ROOT / "scripts" / "check_changed_go_coverage.py"


def load_script_module():
    loader = importlib.machinery.SourceFileLoader(
        "check_changed_go_coverage_test", str(SCRIPT_PATH)
    )
    spec = importlib.util.spec_from_loader("check_changed_go_coverage_test", loader)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def test_changed_package_patterns_are_unique_and_sorted():
    mod = load_script_module()

    assert mod.changed_package_patterns(
        [
            "internal/ctl/manager.go",
            "cmd/velocity/main.go",
            "internal/ctl/swap_other.go",
            "assets.go",
        ]
    ) == [".", "./cmd/velocity", "./internal/ctl"]


def test_path_in_scope_honors_include_and_exclude_prefixes():
    mod = load_script_module()

    assert mod.path_in_scope("internal/cmd/root/root.go", ["internal/"], [])
    assert not mod.path_in_scope("cmd/velocity/main.go", ["internal/"], [])
    assert not mod.path_in_scope(
        "internal/cmd/server/radar.go",
        ["internal/"],
        ["internal/cmd/server/"],
    )


def test_parse_coverage_profile_merges_duplicate_coverpkg_blocks(tmp_path: Path):
    mod = load_script_module()
    profile = tmp_path / "coverage.out"
    profile.write_text(
        "\n".join(
            [
                "mode: atomic",
                "github.com/banshee-data/velocity.report/internal/ctl/manager.go:10.1,12.2 2 0",
                "github.com/banshee-data/velocity.report/internal/ctl/manager.go:10.1,12.2 2 1",
                "github.com/banshee-data/velocity.report/internal/ctl/manager.go:14.1,15.2 1 0",
                "",
            ]
        )
    )

    coverage = mod.parse_coverage_profile(profile)

    got = coverage["internal/ctl/manager.go"]
    assert got.total == 3
    assert got.covered == 2
    assert got.percent == 2 * 100 / 3


def test_check_files_reports_only_files_below_threshold():
    mod = load_script_module()
    coverage = {
        "covered.go": mod.FileCoverage("covered.go", total=50, covered=49),
        "low.go": mod.FileCoverage("low.go", total=50, covered=48),
        "nostmt.go": mod.FileCoverage("nostmt.go", total=0, covered=0),
    }

    failures = mod.check_files(
        ["covered.go", "low.go", "nostmt.go", "missing.go"],
        coverage,
        98.0,
    )

    assert [f.path for f in failures] == ["low.go"]
