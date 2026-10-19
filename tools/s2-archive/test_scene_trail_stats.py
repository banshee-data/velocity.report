"""Cover the trail reader without a server, a recording or a binary.

What it must get right on its own: which tracks count as moving vehicles, the
four measures and the course-rate screen on trails whose answers are known in
closed form, the paired check that refuses two exports differing in anything
but the trail, pooling across tracks, and a JSON document with no NaN in it.
"""

import contextlib
import gzip
import importlib.util
import io
import json
import math
import os
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))

# The script is named with a hyphen, so it is not importable by name.
spec = importlib.util.spec_from_file_location(
    "scene_trail_stats", os.path.join(HERE, "scene-trail-stats.py")
)
stats = importlib.util.module_from_spec(spec)
sys.modules["scene_trail_stats"] = stats
spec.loader.exec_module(stats)

DT = 0.2  # the published stride's interval, seconds

ONLINE_HEADER = {"trail_estimator": "cv_kf_v1/online"}
SMOOTHED_HEADER = {
    "trail_estimator": "cv_kf_v1+rts_fixed_assignment_v1/final",
    "trail_smoothing": {
        "final": 10,
        "fixed_lag": 0,
        "substituted": 10,
        "smoother": {
            "barriers": 0,
            "covariance_fallbacks": 0,
            "revisions_without_evidence": 0,
        },
    },
}


def track(x, y, vx, vy, h=1.5, spd=8.0, c="car"):
    """One exported track row: the trail as given, everything else fixed.

    spd, hdg, bx and by are the tracker's own in a real smoothed export, so they
    must not follow the trail here either, or the paired check refuses the pair.
    """
    return {
        "x": x,
        "y": y,
        "z": -1.5,
        "vx": vx,
        "vy": vy,
        "spd": spd,
        "mspd": 9.0,
        "hdg": 0.0,
        "l": 4.2,
        "w": 1.8,
        "h": h,
        "bh": 0.1,
        "bx": 1.0,
        "by": 2.0,
        "c": c,
        "cf": 0.9,
    }


def straight(
    n, speed=8.0, jitter=0.0, v_offset=0.0, shift=0.0, direction=1, h=1.5, spd=None
):
    """n rows along x at the speed: x exact, y alternating ±jitter, velocity exported as given."""
    rows = []
    for i in range(n):
        sign = 1 if i % 2 == 0 else -1
        rows.append(
            track(
                direction * speed * i * DT + shift,
                jitter * sign,
                direction * speed + v_offset,
                0.0,
                h=h,
                spd=speed if spd is None else spd,
            )
        )
    return rows


def turning(n, speed=5.0, rate=0.2):
    """A track whose course turns at the rate: the velocity's direction is rate·t, position integrated."""
    rows, x, y = [], 0.0, 0.0
    for i in range(n):
        theta = rate * i * DT
        vx, vy = speed * math.cos(theta), speed * math.sin(theta)
        rows.append(track(x, y, vx, vy, spd=speed))
        x += vx * DT
        y += vy * DT
    return rows


def frames(tracks, n=None):
    """Frames 0..n-1 holding each track's rows; a None row is a frame the track is absent from."""
    n = n or max(len(rows) for rows in tracks.values())
    out = []
    for i in range(n):
        tr = []
        for tid, rows in tracks.items():
            if i < len(rows) and rows[i] is not None:
                tr.append(dict(rows[i], id=tid))
        out.append({"f": 1000 + 2 * i, "t": round(i * DT * 1e6), "tr": tr})
    return out


def edited(frame_list, edit):
    """A copy of the frames with edit(track id, frame index, row) merged into each track row."""
    out = []
    for i, frame in enumerate(frame_list):
        tr = []
        for row in frame["tr"]:
            row = dict(row)
            row.update(edit(row["id"], i, row))
            tr.append(row)
        out.append(dict(frame, tr=tr))
    return out


def write_part(root, frame_list, header=None, chunks=1):
    """An export part as `velocity scene export` lays it out, split over the chunks."""
    os.makedirs(os.path.join(root, "frames"))
    base = {
        "version": 1,
        "export": "tracks",
        "site": "test-site",
        "sensor_id": "hesai-pandar40p",
        "frame_count": len(frame_list),
        "start_ns": "1700000000000000000",
        "frame_stride": 2,
        "source_vrlog_sha256": "abc",
        "tuning_hash": "sha256:def",
        "build_version": "0.5.1-test",
        "generated_at": "2026-10-19T00:00:00Z",
    }
    base.update(header or {})
    with open(os.path.join(root, "header.json"), "w") as fh:
        json.dump(base, fh)
    per = math.ceil(len(frame_list) / chunks)
    index = []
    for c in range(chunks):
        part = frame_list[c * per : (c + 1) * per]
        if not part:
            break
        path = os.path.join(root, "frames", f"chunk_{c:04d}.ndjson.gz")
        with gzip.open(path, "wt") as fh:
            for frame in part:
                fh.write(json.dumps(frame) + "\n")
        index.append({"c": c, "n": len(part), "t0": part[0]["t"], "t1": part[-1]["t"]})
    with open(os.path.join(root, "index.json"), "w") as fh:
        json.dump({"version": 1, "chunks": index}, fh)
    return root


def analysed(online, smoothed, online_header=None, smoothed_header=None, chunks=1):
    """Both parts written, read back and analysed."""
    with tempfile.TemporaryDirectory() as tmp:
        o = write_part(
            os.path.join(tmp, "online"),
            online,
            {**ONLINE_HEADER, **(online_header or {})},
            chunks,
        )
        s = write_part(
            os.path.join(tmp, "smoothed"),
            smoothed,
            {**SMOOTHED_HEADER, **(smoothed_header or {})},
            chunks,
        )
        return stats.analyse(stats.read_part(o), stats.read_part(s))


def the_track(doc, tid="car"):
    rows = [row for row in doc["tracks"] if row["id"] == tid]
    assert len(rows) == 1, f"track {tid} not among {[r['id'] for r in doc['tracks']]}"
    return rows[0]


class ReadingTest(unittest.TestCase):
    def test_frames_come_back_in_index_order_across_chunks(self):
        with tempfile.TemporaryDirectory() as tmp:
            part = write_part(tmp, frames({"car": straight(45)}), chunks=3)
            read = stats.read_part(part)
        self.assertEqual(len(read["frames"]), 45)
        self.assertEqual(
            [f["f"] for f in read["frames"]], [1000 + 2 * i for i in range(45)]
        )
        self.assertEqual(read["header"]["site"], "test-site")

    def test_chunk_frame_count_must_match_the_index(self):
        with tempfile.TemporaryDirectory() as tmp:
            part = write_part(tmp, frames({"car": straight(30)}), chunks=2)
            with open(os.path.join(part, "index.json")) as fh:
                index = json.load(fh)
            index["chunks"][0]["n"] += 1
            with open(os.path.join(part, "index.json"), "w") as fh:
                json.dump(index, fh)
            with self.assertRaises(stats.ReaderError):
                stats.read_part(part)

    def test_an_index_without_chunks_is_refused(self):
        with tempfile.TemporaryDirectory() as tmp:
            part = write_part(tmp, frames({"car": straight(30)}))
            with open(os.path.join(part, "index.json"), "w") as fh:
                json.dump({"version": 1, "chunks": []}, fh)
            with self.assertRaises(stats.ReaderError):
                stats.read_part(part)

    def test_an_export_before_the_header_named_its_trail_is_online(self):
        online = frames({"car": straight(30)})
        doc = analysed(online, online, online_header={"trail_estimator": None})
        self.assertEqual(doc["online"]["trail_estimator"], "cv_kf_v1/online")
        self.assertFalse(doc["online"]["trail_estimator_named"])
        self.assertTrue(doc["smoothed"]["trail_estimator_named"])


class SelectionTest(unittest.TestCase):
    def samples(self, rows):
        return [(i, i * DT, row) for i, row in enumerate(rows)]

    def test_the_spec_s_three_conditions(self):
        self.assertTrue(stats.moving_vehicle(self.samples(straight(20))))
        self.assertFalse(stats.moving_vehicle(self.samples(straight(19))))
        self.assertFalse(stats.moving_vehicle(self.samples(straight(30, speed=2.9))))
        self.assertTrue(stats.moving_vehicle(self.samples(straight(30, speed=3.0))))
        self.assertFalse(stats.moving_vehicle(self.samples(straight(30, h=1.19))))
        self.assertTrue(stats.moving_vehicle(self.samples(straight(30, h=1.2))))

    def test_only_moving_vehicles_are_measured(self):
        online = frames(
            {
                "car": straight(30),
                "short": straight(19),
                "slow": straight(30, speed=2.0),
                "low": straight(30, h=1.0),
            }
        )
        doc = analysed(online, online)
        self.assertEqual(doc["selection"]["tracks_total"], 4)
        self.assertEqual(doc["selection"]["moving_vehicle_tracks"], 1)
        self.assertEqual([row["id"] for row in doc["tracks"]], ["car"])
        self.assertEqual(doc["pooled"]["tracks"], 1)


class MeasureTest(unittest.TestCase):
    def test_a_constant_velocity_trail_measures_zero(self):
        online = frames({"car": straight(40)})
        row = the_track(analysed(online, online))
        for arm in ("online", "smoothed"):
            for key in (
                "departure_rms",
                "velocity_rms",
                "step_rms",
                "lateral_p50",
                "lateral_p90",
            ):
                self.assertAlmostEqual(row[arm][key], 0.0, places=9, msg=f"{arm} {key}")
        self.assertAlmostEqual(row["smoothed"]["along_median"], 0.0, places=9)
        self.assertAlmostEqual(row["smoothed"]["across_p90"], 0.0, places=9)
        # A course that never turns has no peak rate to share.
        self.assertEqual(row["online"]["course_rate_peak"], 0.0)
        self.assertTrue(math.isnan(row["smoothed"]["course_rate_share"]))
        self.assertAlmostEqual(row["interval_s"], DT, places=9)
        self.assertEqual(row["gap_steps"], 0)
        self.assertEqual(row["class"], "car")
        self.assertAlmostEqual(row["mean_speed"], 8.0)

    def test_alternating_jitter_is_the_second_difference_over_the_interval_squared(
        self,
    ):
        d = 0.05
        online = frames({"car": straight(40, jitter=d)})
        smoothed = frames({"car": straight(40)})
        doc = analysed(online, smoothed)
        row = the_track(doc)
        # y alternates ±d: each triple's velocity change is 4d/dt, over dt again.
        self.assertAlmostEqual(row["online"]["departure_rms"], 4 * d / DT**2, places=6)
        # Each step misses the exported velocity by the 2d zigzag.
        self.assertAlmostEqual(row["online"]["step_rms"], 2 * d, places=9)
        # The central difference straddles the zigzag, so the exported velocity matches it.
        self.assertAlmostEqual(row["online"]["velocity_rms"], 0.0, places=9)
        # Every point sits d from the track's line, give or take the line's tiny tilt.
        self.assertAlmostEqual(row["online"]["lateral_p50"], d, delta=0.005)
        self.assertAlmostEqual(row["online"]["lateral_p90"], d, delta=0.005)
        self.assertAlmostEqual(row["smoothed"]["departure_rms"], 0.0, places=9)
        self.assertAlmostEqual(doc["pooled"]["departure_ratio"], 0.0, places=9)
        self.assertTrue(doc["rule"]["departure_halved"])
        self.assertTrue(doc["rule"]["passes"])
        self.assertFalse(doc["rule"]["flattening"])

    def test_a_velocity_offset_shows_in_both_velocity_measures(self):
        online = frames({"car": straight(40, v_offset=0.5)})
        row = the_track(analysed(online, online))
        self.assertAlmostEqual(row["online"]["velocity_rms"], 0.5, places=9)
        self.assertAlmostEqual(row["online"]["step_rms"], 0.5 * DT, places=9)
        self.assertAlmostEqual(row["online"]["departure_rms"], 0.0, places=9)

    def test_along_course_offset_is_signed_by_the_direction_of_travel(self):
        online = frames({"car": straight(40)})
        smoothed = frames({"car": straight(40, shift=0.3)})
        row = the_track(analysed(online, smoothed))
        self.assertAlmostEqual(row["smoothed"]["along_median"], 0.3, places=9)
        self.assertAlmostEqual(row["smoothed"]["along_p90"], 0.3, places=9)
        self.assertAlmostEqual(row["smoothed"]["across_median"], 0.0, places=9)
        # The same shift in x on a track driving towards -x trails the online trail.
        online = frames({"car": straight(40, direction=-1)})
        smoothed = frames({"car": straight(40, direction=-1, shift=0.3)})
        row = the_track(analysed(online, smoothed))
        self.assertAlmostEqual(row["smoothed"]["along_median"], -0.3, places=9)
        self.assertAlmostEqual(row["smoothed"]["along_p90"], 0.3, places=9)

    def test_course_rate_kept_is_the_share_of_the_online_peak(self):
        online = frames({"car": turning(40, rate=0.2)})
        smoothed = frames({"car": turning(40, rate=0.1)})
        doc = analysed(online, smoothed)
        row = the_track(doc)
        # Pairs 0.6 s apart (the first at or past 0.5 s) read the rate exactly.
        self.assertAlmostEqual(row["online"]["course_rate_peak"], 0.2, places=6)
        self.assertAlmostEqual(row["smoothed"]["course_rate_peak"], 0.1, places=6)
        self.assertAlmostEqual(row["smoothed"]["course_rate_share"], 0.5, places=6)
        self.assertAlmostEqual(doc["pooled"]["course_rate_kept"], 0.5, places=6)
        # The gentler curve sits closer to its own straight line while keeping under
        # 85 % of the course rate: the spec's flattening flag.
        self.assertLess(row["smoothed"]["lateral_p90"], row["online"]["lateral_p90"])
        self.assertTrue(doc["rule"]["flattening"])

    def test_the_course_rate_wraps_across_pi(self):
        # A track driving towards -x with a small wobble: courses near ±π must not
        # read as a 2π turn.
        rows = straight(40, direction=-1)
        for i, row in enumerate(rows):
            row["vy"] = 0.1 if i % 2 == 0 else -0.1
        row = the_track(analysed(frames({"car": rows}), frames({"car": rows})))
        self.assertLess(row["online"]["course_rate_peak"], 0.1)

    def test_a_gap_in_the_track_skips_the_triples_across_it(self):
        d = 0.05
        rows = straight(40, jitter=d)
        rows[20] = None
        online = frames({"car": rows})
        row = the_track(analysed(online, online))
        self.assertEqual(row["samples"], 39)
        self.assertEqual(row["gap_steps"], 1)
        self.assertAlmostEqual(row["online"]["departure_rms"], 4 * d / DT**2, places=6)
        self.assertAlmostEqual(row["online"]["step_rms"], 2 * d, places=9)


class PairingTest(unittest.TestCase):
    def test_a_trail_that_differs_is_a_pair(self):
        online = frames({"car": straight(30, jitter=0.05)})
        smoothed = edited(
            online,
            lambda tid, i, row: {"x": row["x"] + 0.1, "y": 0.0, "vx": 7.9, "vy": 0.01},
        )
        doc = analysed(online, smoothed)
        self.assertTrue(doc["pairing"]["ok"])
        self.assertEqual(doc["pairing"]["frames"], 30)
        self.assertEqual(doc["pairing"]["tracks"], 1)
        self.assertEqual(doc["pairing"]["track_frames"], 30)
        self.assertIn("tracks", doc)

    def refused(self, online, smoothed, **headers):
        doc = analysed(online, smoothed, **headers)
        self.assertFalse(doc["pairing"]["ok"])
        self.assertNotIn("tracks", doc)
        self.assertNotIn("pooled", doc)
        return doc["pairing"]

    def test_a_field_outside_the_trail_refuses_the_pair(self):
        online = frames({"car": straight(30)})
        for field, value in (
            ("z", -1.4),
            ("h", 1.6),
            ("bx", 0.0),
            ("spd", 7.0),
            ("hdg", 0.5),
            ("c", "truck"),
        ):
            smoothed = edited(
                online, lambda tid, i, row, f=field, v=value: {f: v} if i == 7 else {}
            )
            pairing = self.refused(online, smoothed)
            self.assertEqual(pairing["differences"], {field: 1}, field)
            self.assertIn("track car", pairing["examples"][0])

    def test_a_track_in_one_export_only_refuses_the_pair(self):
        online = frames({"car": straight(30), "other": straight(30, speed=4.0)})
        smoothed = frames({"car": straight(30)})
        pairing = self.refused(online, smoothed)
        self.assertEqual(set(pairing["differences"]), {"track_ids"})
        self.assertEqual(pairing["differences"]["track_ids"], 30)

    def test_a_different_lifetime_refuses_the_pair(self):
        online = frames({"car": straight(30)})
        smoothed = frames({"car": straight(28)}, n=30)
        pairing = self.refused(online, smoothed)
        self.assertEqual(pairing["differences"], {"track_ids": 2})

    def test_a_different_frame_count_refuses_the_pair(self):
        online = frames({"car": straight(30)})
        smoothed = frames({"car": straight(31)})
        pairing = self.refused(online, smoothed, smoothed_header={"frame_count": 30})
        self.assertIn("frames", pairing["differences"])

    def test_a_different_recording_refuses_the_pair(self):
        online = frames({"car": straight(30)})
        pairing = self.refused(
            online, online, smoothed_header={"start_ns": "1700000000000000001"}
        )
        self.assertEqual(pairing["differences"], {"header.start_ns": 1})
        pairing = self.refused(
            online, online, smoothed_header={"source_vrlog_sha256": "other"}
        )
        self.assertEqual(pairing["differences"], {"header.source_vrlog_sha256": 1})

    def test_a_frame_whose_time_differs_refuses_the_pair(self):
        online = frames({"car": straight(30)})
        smoothed = [dict(f) for f in online]
        smoothed[3] = dict(smoothed[3], t=smoothed[3]["t"] + 1)
        pairing = self.refused(online, smoothed)
        self.assertEqual(pairing["differences"], {"frame": 1})

    def test_a_repeated_identifier_within_a_frame_cannot_be_paired(self):
        online = frames({"car": straight(30)})
        online[2]["tr"].append(dict(online[2]["tr"][0]))
        with self.assertRaises(stats.ReaderError):
            analysed(online, online)

    def test_the_same_estimator_in_both_headers_is_a_warning(self):
        online = frames({"car": straight(30)})
        doc = analysed(online, online, smoothed_header=ONLINE_HEADER)
        self.assertTrue(
            any("compares a trail with itself" in w for w in doc["warnings"])
        )
        doc = analysed(online, online)
        self.assertEqual(doc["warnings"], [])

    def test_online_fallback_frames_are_a_warning(self):
        online = frames({"car": straight(30)})
        header = {
            "trail_smoothing": dict(
                SMOOTHED_HEADER["trail_smoothing"], online_fallback=3
            )
        }
        doc = analysed(online, online, smoothed_header=header)
        self.assertTrue(any("3 track frames" in w for w in doc["warnings"]))


class PoolingTest(unittest.TestCase):
    def test_pooled_rms_is_over_every_sample_and_the_median_over_tracks(self):
        d = 0.05
        online = frames({"a": straight(40, jitter=d), "b": straight(40)})
        smoothed = frames({"a": straight(40), "b": straight(40)})
        doc = analysed(online, smoothed)
        pooled = doc["pooled"]
        self.assertEqual(pooled["tracks"], 2)
        a = 4 * d / DT**2
        # 38 triples at a and 38 at zero.
        self.assertAlmostEqual(
            pooled["online"]["pooled"]["departure_rms"], math.sqrt(a * a / 2), places=6
        )
        self.assertAlmostEqual(
            pooled["online"]["track_median"]["departure_rms"], a / 2, places=6
        )
        self.assertAlmostEqual(
            pooled["smoothed"]["pooled"]["departure_rms"], 0.0, places=9
        )
        self.assertAlmostEqual(pooled["departure_ratio"], 0.0, places=9)
        self.assertAlmostEqual(pooled["interval_s"], DT, places=9)

    def test_no_moving_vehicle_leaves_the_pooled_figures_empty(self):
        online = frames({"ped": straight(30, speed=1.0, h=0.8)})
        doc = analysed(online, online)
        self.assertEqual(doc["tracks"], [])
        self.assertEqual(doc["pooled"]["tracks"], 0)
        self.assertTrue(math.isnan(doc["pooled"]["departure_ratio"]))
        self.assertFalse(doc["rule"]["departure_halved"])
        json.dumps(stats.clean(doc), allow_nan=False)


class HelperTest(unittest.TestCase):
    def test_percentile_interpolates(self):
        self.assertEqual(stats.percentile([1, 2, 3, 4], 50), 2.5)
        self.assertEqual(stats.percentile([1, 2, 3, 4], 90), 3.7)
        self.assertEqual(stats.percentile([5], 90), 5)
        self.assertTrue(math.isnan(stats.percentile([], 90)))

    def test_rms_and_median_cope_with_nothing(self):
        self.assertTrue(math.isnan(stats.rms([])))
        self.assertTrue(math.isnan(stats.median([float("nan")])))
        self.assertEqual(stats.median([float("nan"), 2.0, 4.0]), 3.0)
        self.assertAlmostEqual(stats.rms([3.0, 4.0]), math.sqrt(12.5))

    def test_the_fitted_line_follows_the_net_displacement(self):
        xs, ys = [0.0, 1.0, 2.0, 3.0], [0.0, 1.0, 2.0, 3.0]
        _, _, ux, uy = stats.fitted_line(xs, ys)
        self.assertAlmostEqual(ux, math.sqrt(0.5))
        self.assertAlmostEqual(uy, math.sqrt(0.5))
        _, _, ux, uy = stats.fitted_line(xs[::-1], ys[::-1])
        self.assertAlmostEqual(ux, -math.sqrt(0.5))
        self.assertAlmostEqual(uy, -math.sqrt(0.5))


class OutputTest(unittest.TestCase):
    def test_the_json_document_carries_no_nan(self):
        online = frames({"car": straight(40)})
        doc = analysed(online, online)
        text = json.dumps(stats.clean(doc), allow_nan=False)
        back = json.loads(text)
        self.assertIsNone(the_track(back)["smoothed"]["course_rate_share"])
        self.assertEqual(back["online"]["trail_estimator"], "cv_kf_v1/online")
        self.assertEqual(back["smoothed"]["trail_smoothing"]["final"], 10)

    def test_markdown_names_the_estimators_the_pairing_and_the_counts(self):
        online = frames({"car": straight(40, jitter=0.05)})
        smoothed = frames({"car": straight(40)})
        text = stats.markdown_pair(analysed(online, smoothed), tracks=True)
        self.assertIn("## test-site", text)
        self.assertIn("`cv_kf_v1/online`", text)
        self.assertIn("`cv_kf_v1+rts_fixed_assignment_v1/final`", text)
        self.assertIn("pairing: identical outside x, y, vx, vy over 40 frames", text)
        self.assertIn("final 10, fixed_lag 0", text)
        self.assertIn("revisions_without_evidence 0", text)
        self.assertIn("passes: yes", text)
        self.assertIn(
            "| departure from constant velocity, pooled RMS (m/s²) | 5.000 | 0.000 | 0.00 |",
            text,
        )
        self.assertIn("| car | car | 40 |", text)
        for line in text.splitlines():
            if line.startswith("|"):
                self.assertEqual(line.count("|"), line.count(" | ") + 2, line)

    def test_markdown_of_a_refused_pair_says_why(self):
        online = frames({"car": straight(30)})
        smoothed = edited(online, lambda tid, i, row: {"z": -1.0} if i == 7 else {})
        text = stats.markdown_pair(analysed(online, smoothed))
        self.assertIn("**refused**", text)
        self.assertIn("z ×1", text)
        self.assertNotIn("| measure |", text)

    def test_the_pair_command_writes_both_outputs_and_reports_the_pairing(self):
        online = frames({"car": straight(40, jitter=0.05)})
        with tempfile.TemporaryDirectory() as tmp:
            o = write_part(os.path.join(tmp, "online"), online, ONLINE_HEADER)
            s = write_part(
                os.path.join(tmp, "smoothed"),
                frames({"car": straight(40)}),
                SMOOTHED_HEADER,
            )
            out_json, out_md = os.path.join(tmp, "out.json"), os.path.join(
                tmp, "out.md"
            )
            code = stats.main(
                [
                    "pair",
                    o,
                    s,
                    "--json",
                    out_json,
                    "--markdown",
                    out_md,
                    "--site",
                    "named",
                ]
            )
            self.assertEqual(code, 0)
            with open(out_json) as fh:
                doc = json.load(fh)
            self.assertEqual(doc["site"], "named")
            self.assertTrue(doc["rule"]["passes"])
            with open(out_md) as fh:
                self.assertIn("## named", fh.read())
            # A refused pair still writes the document, and exits 1.
            bad = write_part(
                os.path.join(tmp, "bad"),
                edited(online, lambda tid, i, row: {"h": 2.0} if i == 1 else {}),
                SMOOTHED_HEADER,
            )
            err = io.StringIO()
            with contextlib.redirect_stderr(err), contextlib.redirect_stdout(
                io.StringIO()
            ):
                code = stats.main(["pair", o, bad, "--json", out_json])
            self.assertEqual(code, 1)
            self.assertIn("differ in h", err.getvalue())
            with open(out_json) as fh:
                self.assertFalse(json.load(fh)["pairing"]["ok"])

    def test_an_unreadable_export_exits_2(self):
        online = frames({"car": straight(30)})
        with tempfile.TemporaryDirectory() as tmp:
            o = write_part(os.path.join(tmp, "online"), online, ONLINE_HEADER)
            with open(os.path.join(o, "index.json"), "w") as fh:
                json.dump({"version": 1, "chunks": []}, fh)
            err = io.StringIO()
            with contextlib.redirect_stderr(err):
                code = stats.main(["pair", o, o])
            self.assertEqual(code, 2)
            self.assertIn("lists no chunks", err.getvalue())


class SetTest(unittest.TestCase):
    def layout(self, tmp):
        """Two sites as export-scene-variants.py lays a set out; the second lacks its smoothed export."""
        root = os.path.join(tmp, "smoothed-trail")
        write_part(
            os.path.join(root, "site-a", "online", "assets", "part-000"),
            frames({"car": straight(40, jitter=0.05)}),
            dict(ONLINE_HEADER, site="site-a"),
        )
        write_part(
            os.path.join(root, "site-a", "smoothed", "assets", "part-000"),
            frames({"car": straight(40)}),
            dict(SMOOTHED_HEADER, site="site-a"),
        )
        write_part(
            os.path.join(root, "site-b", "online", "assets", "part-000"),
            frames({"car": straight(40)}),
            dict(ONLINE_HEADER, site="site-b"),
        )
        os.makedirs(os.path.join(root, "site-b", "compare"))
        # The set's own files beside the sites: never a site.
        os.makedirs(os.path.join(root, "recipes", "site-a"))
        with open(os.path.join(root, "recipes", "site-a", "online.json"), "w") as fh:
            json.dump({}, fh)
        with open(os.path.join(root, "set.json"), "w") as fh:
            json.dump({"set_id": "smoothed-trail"}, fh)
        return root

    def test_every_site_with_both_exports_is_read(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = self.layout(tmp)
            pairs, notes = stats.set_pairs(root, "online", "smoothed")
            self.assertEqual([site for site, _, _ in pairs], ["site-a"])
            self.assertEqual(len(notes), 1)
            self.assertIn("site-b: skipped", notes[0])
            self.assertFalse(any("recipes" in note for note in notes))
            out_json, out_md = os.path.join(tmp, "set.json"), os.path.join(
                tmp, "set.md"
            )
            code = stats.main(["set", root, "--json", out_json, "--markdown", out_md])
            self.assertEqual(code, 0)
            with open(out_json) as fh:
                doc = json.load(fh)
            self.assertEqual([site["site"] for site in doc["sites"]], ["site-a"])
            self.assertTrue(doc["sites"][0]["rule"]["passes"])
            with open(out_md) as fh:
                text = fh.read()
            self.assertIn("| site-a | 1 | 5.00 → 0.00 | 0.00 |", text)
            self.assertIn("1 of 1 sites pass the rule", text)
            self.assertIn("site-b: skipped", text)

    def test_a_refused_site_fails_the_set(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = self.layout(tmp)
            bad = os.path.join(root, "site-b", "smoothed", "assets", "part-000")
            write_part(
                bad,
                edited(
                    frames({"car": straight(40)}),
                    lambda tid, i, row: {"c": "bus"} if i == 0 else {},
                ),
                dict(SMOOTHED_HEADER, site="site-b"),
            )
            err = io.StringIO()
            with contextlib.redirect_stderr(err), contextlib.redirect_stdout(
                io.StringIO()
            ):
                code = stats.main(["set", root])
            self.assertEqual(code, 1)
            self.assertIn("site-b", err.getvalue())

    def test_an_empty_root_fails(self):
        with tempfile.TemporaryDirectory() as tmp:
            err = io.StringIO()
            with contextlib.redirect_stderr(err), contextlib.redirect_stdout(
                io.StringIO()
            ):
                code = stats.main(["set", tmp])
            self.assertEqual(code, 1)


if __name__ == "__main__":
    unittest.main()
