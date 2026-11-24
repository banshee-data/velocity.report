"""Regression checks for the SF game route bakers."""

import math
import json
from pathlib import Path
import runpy

import pytest

smooth_ramp_join = runpy.run_path(Path(__file__).with_name("bake-city-drive.py"))[
    "smooth_ramp_join"
]
road_layers = runpy.run_path(Path(__file__).with_name("embed-road-layers.py"))


class SlopingPavement:
    def height(self, x, y):
        return 10 + 0.02 * x + 0.01 * y


def heading(a, b):
    return math.atan2(b[1] - a[1], b[0] - a[0])


def grade(a, b):
    return (b[2] - a[2]) / math.hypot(b[0] - a[0], b[1] - a[1])


@pytest.mark.parametrize(
    "left_kind,right_kind", [("ground", "ramp"), ("ramp", "ground")]
)
def test_ramp_join_shares_a_point_and_blends_heading_and_grade(left_kind, right_kind):
    pavement = SlopingPavement()
    bearing = math.radians(12)
    before = [[s, 0, 0] for s in range(-12, 1, 2)]
    after = [[s * math.cos(bearing), s * math.sin(bearing), 0] for s in range(0, 13, 2)]
    for kind, positions in [(left_kind, before), (right_kind, after)]:
        for point in positions:
            x, y, _ = point
            point[2] = pavement.height(x, y) if kind == "ground" else 10 + 0.06 * x

    left = {"id": "left", "kind": left_kind, "positions": before}
    right = {"id": "right", "kind": right_kind, "positions": after}
    smooth_ramp_join(left, right, pavement)

    first, join = left["positions"][-2:]
    next_point = right["positions"][1]
    assert join == right["positions"][0]
    assert left["positions"][0] == before[0]
    assert right["positions"][-1] == after[-1]
    assert abs(heading(first, join) - heading(join, next_point)) < math.radians(3)
    assert abs(grade(first, join) - grade(join, next_point)) < 0.01
    ground = left if left_kind == "ground" else right
    for x, y, z in ground["positions"]:
        assert z == pytest.approx(pavement.height(x, y), abs=0.00006)


def test_overhead_patches_use_original_row_indices_and_check_source_records():
    encoded = road_layers["encoded"]
    digest = road_layers["digest"]
    patch_buildings = road_layers["patch_source_rows"]
    first, untouched, last = [[base, [[base + 20]]] for base in (10, 50, 70)]
    buildings = [first, untouched, last]
    cover, side, other = [[base, [[100]]] for base in (30, 10, 90)]
    patches = [
        {"index": 0, "sha256": digest(encoded(first)), "parts": [cover, side]},
        {"index": 2, "sha256": digest(encoded(last)), "parts": [other]},
    ]
    assert patch_buildings(buildings, patches) == [cover, side, untouched, other]
    assert buildings == [first, untouched, last]
    with pytest.raises(ValueError, match="does not match source row 0"):
        patch_buildings(buildings, [*patches, patches[0]])
    with pytest.raises(ValueError, match="does not match source row 0"):
        patch_buildings([last, untouched, first], patches)


def test_tile_patch_can_remove_a_fully_covered_road_row():
    encoded = road_layers["encoded"]
    digest = road_layers["digest"]
    patch_rows = road_layers["patch_source_rows"]
    rows = [["approach"], ["covered"], ["exit"]]
    patch = {"index": 1, "sha256": digest(encoded(rows[1])), "parts": []}
    assert patch_rows(rows, [patch]) == [["approach"], ["exit"]]
    with pytest.raises(ValueError, match="does not match source row 1"):
        patch_rows(rows, [{**patch, "parts": None}])


@pytest.mark.parametrize(
    ("patch_key", "manifest_key", "output_index"),
    [
        ("covered_ground_road_patches", "coveredGroundRoadPatches", 2),
        ("overhead_building_patches", "overheadBuildingPatches", 1),
    ],
)
def test_scenery_patches_can_belong_to_a_neighbouring_tile(
    tmp_path, patch_key, manifest_key, output_index
):
    encoded = road_layers["encoded"]
    digest = road_layers["digest"]
    playable_product = road_layers["playable_product"]
    patch = {"index": 0, "sha256": "source", "parts": [["approach"], ["exit"]]}
    product = {
        "format": "sf-game-road-contact",
        "version": 1,
        "base_sha256": "base",
        "road_revision": "revision",
        "roads": [],
        "transitions": [],
        patch_key: {"8085871c": [patch]},
    }
    path = tmp_path / "structures.json"
    payload = encoded(product)
    path.write_bytes(payload)
    manifest = {"tiles": [{"t": "8085871c"}]}
    outputs = playable_product(path, digest(payload), "base", "revision", manifest)
    assert outputs[output_index] == {"8085871c": [patch]}
    assert outputs[3][manifest_key] == {"8085871c": 1}
    with pytest.raises(ValueError, match="must name base tiles"):
        playable_product(path, digest(payload), "base", "revision", {"tiles": []})


def test_bake_requires_a_transition_for_a_wider_seam():
    valid = road_layers["valid_width_transitions"]
    road = {
        "width": 8,
        "points": [[0, 0, 0], [20, 0, 1], [40, 0, 2]],
        "seams": {"end": {"bearing": 0, "width": 12}},
    }
    assert not valid(road)
    assert valid({**road, "widthTransitionMetres": {"end": 24}})
    assert not valid({**road, "widthTransitionMetres": {"start": 24}})
    assert not valid({**road, "widthTransitionMetres": {"end": 41}})


def test_bundled_approach_requires_exact_parent_and_preserves_original_sections(
    tmp_path,
):
    encoded = road_layers["encoded"]
    digest = road_layers["digest"]
    with_drive_approaches = road_layers["with_drive_approaches"]
    original = {"id": "bridge", "sections": [{"id": "west-span"}]}
    drives = encoded({"drives": [original]})
    entry = {"id": "island", "label": "Island", "sections": [{"id": "tunnel"}]}
    opposite = {
        "id": "eastbound",
        "label": "Eastbound",
        "replace_sections": True,
        "sections": [{"id": "lower"}],
    }
    extension = {
        "format": "sf-game-drive-approaches",
        "base_drive_sha256": digest(drives),
        "drive_id": "bridge",
        "approaches": [entry, opposite],
    }
    path = tmp_path / "extension.json"
    raw = encoded(extension)
    path.write_bytes(raw)
    with pytest.raises(ValueError, match="checksum differs"):
        with_drive_approaches(drives, path, "incorrect")
    with pytest.raises(ValueError, match="pinned parent"):
        with_drive_approaches(drives + b" ", path, digest(raw))
    bundled, starts = with_drive_approaches(drives, path, digest(raw))
    assert starts == [
        {"id": "island", "label": "Island"},
        {"id": "eastbound", "label": "Eastbound"},
    ]
    assert json.loads(bundled)["drives"] == [
        dict(original, approaches=[entry, opposite])
    ]
    opposite["replace_sections"] = "yes"
    raw = encoded(extension)
    path.write_bytes(raw)
    with pytest.raises(ValueError, match="replace_sections must be a boolean"):
        with_drive_approaches(drives, path, digest(raw))
