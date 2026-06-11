"""Tests for scripts/loc-coverage-chart.py."""

from __future__ import annotations

import importlib.util
import sys
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).resolve().parents[1]
SCRIPT_PATH = REPO_ROOT / "scripts" / "loc-coverage-chart.py"
MODULE_NAME = "loc_coverage_chart"


def load_module():
    if MODULE_NAME in sys.modules:
        return sys.modules[MODULE_NAME]
    spec = importlib.util.spec_from_file_location(MODULE_NAME, SCRIPT_PATH)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    # Register before exec so dataclasses can resolve cls.__module__.
    sys.modules[MODULE_NAME] = module
    spec.loader.exec_module(module)
    return module


def test_parse_go_coverage(tmp_path):
    mod = load_module()
    cov = tmp_path / "coverage.out"
    cov.write_text(
        "mode: atomic\n"
        "github.com/x/y/a.go:1.0,2.0 5 1\n"
        "github.com/x/y/a.go:3.0,4.0 3 0\n"
        "github.com/x/y/b.go:1.0,2.0 2 7\n"
    )
    hit, found = mod.parse_go_coverage(cov)
    assert (hit, found) == (7, 10)


def test_parse_go_coverage_missing(tmp_path):
    mod = load_module()
    assert mod.parse_go_coverage(tmp_path / "nope.out") == (0, 0)


def test_parse_lcov(tmp_path):
    mod = load_module()
    lcov = tmp_path / "lcov.info"
    lcov.write_text(
        "SF:src/a.ts\n"
        "DA:1,1\nDA:2,0\n"
        "LF:10\nLH:7\n"
        "end_of_record\n"
        "SF:src/b.ts\n"
        "LF:5\nLH:2\n"
        "end_of_record\n"
    )
    hit, found = mod.parse_lcov(lcov, "js")
    assert (hit, found) == (9, 15)


def test_build_buckets_groups_languages_and_attaches_coverage():
    mod = load_module()
    cloc_counts = {
        "Go": 1000,
        "TypeScript": 300,
        "Svelte": 100,
        "Swift": 500,
        "Markdown": 800,
        "Python": 50,
        "Bourne Again Shell": 25,
        "Rust": 999,  # unbucketed — should appear in `other`
    }
    buckets, other = mod.build_buckets(
        cloc_counts,
        go_cov=(700, 1000),
        web_cov=(200, 400),
        mac_cov=(100, 500),
    )
    assert buckets["go"].code_loc == 1000
    assert buckets["js"].code_loc == 400
    assert buckets["mac"].code_loc == 500
    assert buckets["markdown"].code_loc == 800
    assert buckets["scripts"].code_loc == 75
    assert buckets["go"].covered_fraction == pytest.approx(0.7)
    assert buckets["js"].covered_fraction == pytest.approx(0.5)
    assert buckets["mac"].covered_fraction == pytest.approx(0.2)
    assert other == ["Rust (999)"]


def test_render_emits_svg_with_pattern_and_categories():
    mod = load_module()
    buckets = {
        "js": mod.BucketStats(code_loc=400, cov_hit=200, cov_found=400),
        "go": mod.BucketStats(code_loc=1000, cov_hit=700, cov_found=1000),
        "mac": mod.BucketStats(code_loc=500, cov_hit=100, cov_found=500),
        "markdown": mod.BucketStats(code_loc=800),
        "scripts": mod.BucketStats(code_loc=75),
    }
    svg = mod.render(buckets)
    assert svg.startswith("<svg")
    assert svg.rstrip().endswith("</svg>")
    assert 'id="hatch"' in svg
    # Each named bucket should produce a label.
    for label in ("js (", "go (", "mac (", "markdown (", "scripts ("):
        assert label in svg
    # Totals footer present and human-readable.
    assert "LOC total" in svg
    assert "line coverage on coded" in svg


def test_render_omits_zero_loc_segments():
    mod = load_module()
    buckets = {
        "js": mod.BucketStats(code_loc=0),
        "go": mod.BucketStats(code_loc=100, cov_hit=50, cov_found=100),
        "mac": mod.BucketStats(code_loc=0),
        "markdown": mod.BucketStats(code_loc=200),
        "scripts": mod.BucketStats(code_loc=10),
    }
    svg = mod.render(buckets)
    assert "js (" not in svg
    assert "mac (" not in svg
    assert "go (" in svg


def test_render_is_deterministic():
    mod = load_module()
    buckets = {
        "js": mod.BucketStats(code_loc=400, cov_hit=200, cov_found=400),
        "go": mod.BucketStats(code_loc=1000, cov_hit=700, cov_found=1000),
        "mac": mod.BucketStats(code_loc=500, cov_hit=100, cov_found=500),
        "markdown": mod.BucketStats(code_loc=800),
        "scripts": mod.BucketStats(code_loc=75),
    }
    a = mod.render(buckets)
    b = mod.render(buckets)
    assert a == b
