"""Regression checks for the SF game route bakers."""

import math
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
    patch_buildings = road_layers["patch_overhead_buildings"]
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
