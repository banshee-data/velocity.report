"""Tests for lidar_merge_read.py: l4clus0012's inferred merges on hand-built rows."""

from __future__ import annotations

import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import lidar_merge_read as mr  # noqa: E402
from test_lidar_flip_cost import (
    boxed,
    drive,
    make_box_rows,
    make_db,
    park,
)  # noqa: E402


def car(rows, dx=0.0):
    return boxed(rows, 4.5, 1.8, dx)


def stand(t0, t1, x, y):
    """A parked car at (x, y), a row every 0.1 s."""
    return [(t, x, y, 0.0) for t, _, _, _ in park(t0, t1, x)]


class MergeReadTest(unittest.TestCase):
    def setUp(self):
        self._dir = tempfile.TemporaryDirectory()
        self.tmp = Path(self._dir.name)

    def tearDown(self):
        self._dir.cleanup()

    def test_a_pass_beside_a_parked_car_is_not_a_merge(self):
        tmp_path = self.tmp
        # A: a car passes 2.2 m beside a parked car. B draws the passing car 4.5 m wide, so its
        # footprint holds the parked car's centre too; the parked car is not moving.
        passing = drive(0, 5)
        a = make_box_rows(
            tmp_path / "a.csv",
            {"pass": car(passing), "parked": boxed(stand(0, 5, 25.0, 2.2), 4.5, 1.8)},
        )
        wide = [(t, x, y + 1.1, v, 4.5, 4.5) for t, x, y, v in passing]
        b = make_box_rows(
            tmp_path / "b.csv",
            {"pass": wide, "parked": boxed(stand(0, 5, 25.0, 2.2), 4.5, 1.8)},
        )
        out = mr.compare(a, b)
        assert out["a_moving"] == 1
        assert out["merges"] == 0

    def test_two_abreast_in_one_footprint_is_one_merge(self):
        tmp_path = self.tmp
        # A: two cars side by side, 2.5 m apart, for 5 s. B: one track whose box spans both.
        # The pair is held on every frame and counted once.
        left, right = drive(0, 5), drive(0, 5, y=2.5)
        a = make_box_rows(tmp_path / "a.csv", {"left": car(left), "right": car(right)})
        both = [(t, x, 1.25, v, 4.5, 4.4) for t, x, _, v in left]
        b = make_box_rows(tmp_path / "b.csv", {"both": both})
        out = mr.compare(a, b)
        assert out["merges"] == 1 and out["b_tracks_merging"] == 1
        m = out["merge_list"][0]
        assert m["b_track"] == "both" and m["a_tracks"] == ["left", "right"]
        assert m["frames"] == len(left)
        assert m["first_s"] == 0.0 and m["last_s"] == 5.0
        assert m["b_width_median"] == 4.4 and m["b_width_max"] == 4.4
        assert m["b_speed_median"] == 10.0
        assert m["pair_overlap_mean"] == 0.0

    def test_a_duplicate_pair_is_not_a_merge(self):
        tmp_path = self.tmp
        # A carries one car twice, 0.3 m apart: the drawn boxes overlap by far more than half.
        rows = drive(0, 5)
        a = make_box_rows(
            tmp_path / "a.csv",
            {
                "car": car(rows),
                "dup": [(t, x, y + 0.3, v, 4.5, 1.8) for t, x, y, v in rows],
            },
        )
        b = make_box_rows(tmp_path / "b.csv", {"car": car(rows)})
        assert mr.compare(a, b)["merges"] == 0

    def test_neighbours_in_their_own_lanes_are_not_merges(self):
        tmp_path = self.tmp
        # Two cars 3.5 m apart, each drawn 1.8 m wide: neither footprint holds the other.
        left, right = car(drive(0, 5)), car(drive(0, 5, y=3.5))
        tracks = {"left": left, "right": right}
        a = make_box_rows(tmp_path / "a.csv", tracks)
        b = make_box_rows(tmp_path / "b.csv", tracks)
        assert mr.compare(a, b)["merges"] == 0

    def test_the_floor_reads_the_defaults_against_themselves(self):
        tmp_path = self.tmp
        # A's own wide track holds a second moving car: the floor counts it once, the same file
        # read as both arms.
        left, right = drive(0, 5), drive(0, 5, y=2.5)
        wide = [(t, x, y, v, 4.5, 4.6) for t, x, y, v in left]
        a = make_box_rows(tmp_path / "a.csv", {"wide": wide, "right": car(right)})
        out = mr.compare(a, a)
        # "wide", 4.6 m wide at 0 m, grown by 0.25 m, holds "right"'s centre at 2.5 m, and the two
        # drawn boxes overlap by under half of "right"'s.
        assert out["merges"] == 1
        assert out["merge_list"][0]["a_tracks"] == ["right", "wide"]
        assert out["merge_list"][0]["pair_overlap_mean"] < 0.5

    def test_a_neighbour_swallowed_whole_reads_as_a_duplicate(self):
        tmp_path = self.tmp
        # A's wide box holds the whole of the second car's drawn box: by the overlap test the
        # pair is a duplicate, so the floor does not count it (a limit the report names).
        left, right = drive(0, 5), drive(0, 5, y=2.5)
        wide = [(t, x, y + 1.25, v, 4.5, 4.4) for t, x, y, v in left]
        a = make_box_rows(tmp_path / "a.csv", {"wide": wide, "right": car(right)})
        assert mr.compare(a, a)["merges"] == 0

    def test_each_candidate_track_counts_its_own_merge(self):
        tmp_path = self.tmp
        # Two B tracks, one after the other, each hold the same A pair: two merges.
        left, right = drive(0, 5), drive(0, 5, y=2.5)
        a = make_box_rows(tmp_path / "a.csv", {"left": car(left), "right": car(right)})
        first = [(t, x, 1.25, v, 4.5, 4.4) for t, x, _, v in left if t < 2.5]
        second = [(t, x, 1.25, v, 4.5, 4.4) for t, x, _, v in left if t >= 2.5]
        b = make_box_rows(tmp_path / "b.csv", {"first": first, "second": second})
        out = mr.compare(a, b)
        assert out["merges"] == 2 and out["b_tracks_merging"] == 2

    def test_rows_without_a_box_hold_nothing(self):
        tmp_path = self.tmp
        # Evidence without box columns: no footprint, no merge.
        left, right = drive(0, 5), drive(0, 5, y=0.5)
        a = make_db(tmp_path / "a.db", {"left": left, "right": right})
        out = mr.compare(a, a)
        assert out["a_moving"] == 2 and out["merges"] == 0

    def test_pair_overlap_falls_back_to_centres_without_boxes(self):
        t = {"rows": [(i, 0.0, 0.0, 1.0, None) for i in range(10)]}
        near = {"rows": [(i, 0.0, 1.0, 1.0, None) for i in range(10)]}
        far = {"rows": [(i, 0.0, 3.0, 1.0, None) for i in range(10)]}
        assert mr.pair_overlap(t, near) == (None, True)
        assert mr.pair_overlap(t, far) == (None, False)

    def test_cli_writes_json(self):
        tmp_path = self.tmp
        rows = car(drive(0, 5))
        a = make_box_rows(tmp_path / "a.csv", {"car": rows})
        out = tmp_path / "out.json"
        script = Path(__file__).resolve().parent / "lidar_merge_read.py"
        subprocess.run(
            [sys.executable, str(script), a, a, "--json", str(out)],
            check=True,
            capture_output=True,
        )
        assert '"merges": 0' in out.read_text()


if __name__ == "__main__":
    unittest.main()
