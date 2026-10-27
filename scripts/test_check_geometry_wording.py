from __future__ import annotations

import importlib.machinery
import importlib.util
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "check-geometry-wording.py"


def load_check():
    loader = importlib.machinery.SourceFileLoader(
        "check_geometry_wording_test", str(SCRIPT)
    )
    spec = importlib.util.spec_from_loader(loader.name, loader)
    module = importlib.util.module_from_spec(spec)
    sys.modules[loader.name] = module
    loader.exec_module(module)
    return module


check = load_check()


def rules(text: str, code: bool = False) -> list[str]:
    return [name for _, name, _, _ in check.scan_text(text, code=code)]


def test_each_avoided_phrase_is_found_in_prose():
    cases = {
        "The site becomes a digital twin of the street.": "digital-twin",
        "An as-built record of the kerb.": "as-built",
        "A blueprint of the block.": "blueprint",
        "The floor plan of the corner shop.": "floor-plan",
        "Photorealistic facades.": "photorealistic",
        "Survey-grade corners.": "survey-grade",
        "We publish a building model.": "structure-model",
        "The roof is reconstructed from 3DEP.": "reconstructed-structure",
        "Walls reconstructed from 3DEP.": "reconstructed-structure",
        "We reconstruct the building from returns.": "reconstructed-structure",
        "LoD2 buildings from 3DEP.": "lod2-building",
        "A 3D city model of San Francisco.": "3d-building-model",
        "These are construction drawings.": "construction-documents",
        "The building parts are extruded to their height.": "extruded-structure",
    }
    for text, rule in cases.items():
        assert rule in rules(text), (text, rules(text))


def test_a_stated_rule_passes():
    assert rules("It is not a building model or a digital twin.") == []
    assert rules("Raised rather than extruded walls.") == []
    assert rules("A record with no construction drawings.") == []


def test_the_ignore_marker_keeps_a_quoted_title():
    line = '"Digital twins of roads" (2024) <!-- geometry-wording: ignore -->'
    assert rules(line) == []


def test_ordinary_uses_of_model_are_not_structure_wording():
    assert rules("The constant velocity model predicts the next position.") == []
    assert rules("The site model is the site's accepted scans.") == []
    assert rules("Per-city model-level registration data.") == []
    assert rules("Reconstruct world coordinates for the neighbour cell.") == []


def test_prose_link_targets_and_code_spans_are_not_copy():
    assert rules("See the [review](lidar-radiance-field-scene-model-review.md).") == []
    assert rules("The `buildingModel` field is a pointer.") == []
    assert rules("The building model [review](x.md) is wrong.") == ["structure-model"]


def test_code_is_read_only_in_comments_and_strings():
    assert rules("let extrude = walls.isEmpty", code=True) == []
    assert rules("const mesh = extrudeBox(part, roof);", code=True) == []
    assert rules("// the parts are extruded from min_height", code=True) == [
        "extruded-structure"
    ]
    assert rules('label = "Building parts extruded to height"', code=True) == [
        "extruded-structure"
    ]


def test_excluded_paths_carry_a_reason():
    assert check.excluded("docs/geometry-priors-data-purpose.md")
    assert check.excluded("proto/gerardus/align/v1alpha1/align.proto")
    assert check.excluded("docs/lidar/operations/brief/research-briefing-2026-10-26.md")
    assert check.excluded(
        "tools/visualiser-macos/VelocityVisualiser/Generated/align.pb.swift"
    )
    assert not check.excluded("docs/lidar/architecture/vector-scene-map.md")
    assert all(reason for _, reason in check.EXCLUDED_PREFIXES)


def test_the_repository_is_clean():
    result = subprocess.run(
        [sys.executable, str(SCRIPT)], cwd=ROOT, capture_output=True, text=True
    )
    assert result.returncode == 0, result.stdout + result.stderr
