"""Tests for lidar_reference_speed.py: figure0002's reference speeds on hand-built inputs."""

from __future__ import annotations

import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import lidar_reference_speed as rs  # noqa: E402
from test_lidar_flip_cost import S, drive, make_db  # noqa: E402


def keyframe(
    t, x, y, kind="rear_face", bound=0.2, status="reviewed", origin="independent"
):
    return {
        "timestamp_ns": int(round(t * S)),
        "anchor": {"kind": kind},
        "position": {"status": "observed", "x_m": x, "y_m": y, "bound_m": bound},
        "review": {"status": status, "origin": origin},
    }


def refs(tmp_path, keyframes):
    p = tmp_path / "refs.json"
    p.write_text(
        json.dumps({"objects": [{"object_id": "car", "keyframes": keyframes}]})
    )
    return str(p)


def test_reference_speed_bound_and_match(tmp_path):
    # A car at 10 m/s along x; its rear face 2 m behind its centre. Keyframes at 1 s and
    # 3 s give 10 m/s with a bound of 0.4 / 2 = 0.2 m/s. A proposed keyframe and one of
    # another anchor kind are ignored.
    kfs = [
        keyframe(1.0, 8.0, 0.0),
        keyframe(2.0, 18.0, 0.0),
        keyframe(3.0, 28.0, 0.0),
        keyframe(2.5, 99.0, 0.0, status="proposed"),
        keyframe(2.0, 20.0, 0.0, kind="front_face"),
    ]
    db = make_db(
        tmp_path / "e.db", {"t1": [(t, x, y, 10.5) for t, x, y, _ in drive(0, 5)]}
    )
    out = rs.measure(refs(tmp_path, kfs), db)["vehicles"][0]
    assert out["anchor"] == "rear_face" and out["keyframes"] == 3
    assert (
        abs(out["reference_mps"] - 10.0) < 1e-9 and abs(out["bound_mps"] - 0.2) < 1e-9
    )
    assert out["track"] == "t1"
    assert (
        abs(out["error_mean_mps"] - 0.5) < 1e-9
        and out["within_bound_mean_mps"] is False
    )
    assert (
        abs(out["error_displacement_mps"]) < 1e-6
        and out["within_bound_displacement_mps"]
    )


def test_unmatched_when_no_track_is_near(tmp_path):
    kfs = [keyframe(1.0, 8.0, 50.0), keyframe(3.0, 28.0, 50.0)]
    db = make_db(tmp_path / "e.db", {"t1": drive(0, 5)})
    assert rs.measure(refs(tmp_path, kfs), db)["vehicles"][0]["track"] is None


def test_no_span_without_two_reviewed_keyframes(tmp_path):
    kfs = [keyframe(1.0, 8.0, 0.0), keyframe(3.0, 28.0, 0.0, origin="tracker_assisted")]
    db = make_db(tmp_path / "e.db", {"t1": drive(0, 5)})
    assert rs.measure(refs(tmp_path, kfs), db)["vehicles"][0]["span"] is None


def test_a_zero_span_is_refused(tmp_path):
    kfs = [keyframe(1.0, 8.0, 0.0), keyframe(1.0, 18.0, 0.0)]
    db = make_db(tmp_path / "e.db", {"t1": drive(0, 5)})
    try:
        rs.measure(refs(tmp_path, kfs), db)
    except ValueError as e:
        assert "not positive" in str(e)
    else:
        raise AssertionError("a zero span was accepted")


def test_nearest_frame():
    assert rs.nearest_frame([10, 20, 30], 24) == 20
    assert rs.nearest_frame([10, 20, 30], 26) == 30
    assert rs.nearest_frame([10, 20, 30], 99) == 30
    assert rs.nearest_frame([], 5) is None
