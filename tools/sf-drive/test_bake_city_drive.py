"""Regression checks for the Bay Bridge game route baker."""

import math
from pathlib import Path
import runpy

import pytest

smooth_ramp_join = runpy.run_path(Path(__file__).with_name("bake-city-drive.py"))[
    "smooth_ramp_join"
]


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
