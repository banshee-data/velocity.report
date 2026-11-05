#!/usr/bin/env python3
"""What a default flip costs: one arm's evidence against the defaults' on the same segment.

Reads two replays of one case, the defaults' (A) and the candidate's (B), each either an evidence
database (the `lidar_tracks` and `lidar_track_observations` tables that `lidar-box-eval
-evidence-db` and `lidar-state-estimation-baseline -evidence-per-case` write) or a `lidar-box-eval
-rows` CSV (every scored frame of every confirmed track, the published box's centre and the
track's speed; a track's duration is then its span within the scored window), and reports the
measures l5id0012's rule is written on. Definitions are fixed here, before any arm is replayed, and are the same for every
case and arm:

* track: a row of lidar_tracks (the evidence keeps confirmed tracks only); its rows are its
  lidar_track_observations rows.
* brief: duration under 1.0 s. lasting: 2.0 s or more.
* near-static: a lasting track whose first-to-last position is under 2.0 m apart and whose p95
  speed is under 0.5 m/s. clutter: brief and near-static tracks together
  (scripts/lidar_noise_screen.py's definitions).
* moving: a lasting track whose first-to-last position is 5.0 m or more apart and whose median
  speed is 1.0 m/s or more. A count of moving tracks is what a traffic count reads.
* covered: a row of one arm's track is covered by the other arm when some track of the other arm
  has a row at the same frame time within 2.0 m of it. A track's coverage is the share of its
  rows covered; its best single-track coverage is the largest share any one track of the other
  arm covers.
* lost: a moving track of A whose coverage by B is under 0.5. shortened: a moving track of A
  covered 0.5 or more by B but whose best single-track coverage by B is under 0.5 (B carries
  the vehicle only in pieces). added: a moving track of B whose coverage by A is under 0.5.
* matched pair: a moving track of A that is not lost, with the B track of best single-track
  coverage. Its speed difference is the B track's p85 row speed less the A track's.

`--match footprint` (l5id0016, fixed before any row is read again) replaces the 2.0 m point test:
a row is covered when some row of the other arm at the same frame has a drawn footprint (its box
heading, length and width, each extent grown by 0.25 m, as `overlap_end_body` grows a live body)
that contains the row's centre, or whose footprint and the row's overlap by half the smaller's
area or more. The overlap is estimated on a 9 by 9 grid of points over the smaller footprint. A
row without a box falls back to the point test. Everything downstream is unchanged.

The report also carries, whatever the match:

* brief_origin: each brief B track is a re-birth when another B track's last row lies within
  1.0 s before its first row and within 3.0 m of it, and the brief track's rows lie inside a
  lasting B track's grown footprint on half or more of its frames; a split when it is not a
  re-birth but its rows lie inside a lasting B track's grown footprint on half or more of its
  frames; other otherwise. Rows carry no expiry reason, so "ended" is any ending, not only the
  overlap end's.
* speed_tail_by_age: the matched pairs' |p85 difference| binned by the B track's age at the first
  frame both share: under 1 s, 1 to 3 s, 3 s or more; the count, median and p95 of each bin.

Usage: lidar_flip_cost.py A.db|A.csv B.db|B.csv [--match point|footprint] [--json OUT]
"""

from __future__ import annotations

import argparse
import csv
import json
import math
import sqlite3
import sys

BRIEF_S = 1.0
LASTING_S = 2.0
STATIC_DISP_M = 2.0
STATIC_P95_MPS = 0.5
MOVING_DISP_M = 5.0
MOVING_P50_MPS = 1.0
MATCH_M = 2.0
COVER_SHARE = 0.5
FOOTPRINT_MARGIN_M = 0.25
REBIRTH_S = 1.0
REBIRTH_M = 3.0
GRID = 9


def pct(values, q):
    """Nearest-rank percentile, as lidar_noise_screen.py takes it."""
    if not values:
        return 0.0
    v = sorted(values)
    k = max(0, min(len(v) - 1, math.ceil(q * len(v)) - 1))
    return v[k]


def load_rows(path):
    """A lidar-box-eval -rows CSV: the published variant's rows, tracks spanning their rows."""
    tracks = {}
    with open(path, newline="") as f:
        for r in csv.DictReader(f):
            if r["variant"] != "published":
                continue
            t = tracks.setdefault(r["track"], {"rows": []})
            t["rows"].append(
                (
                    int(r["t_ns"]),
                    float(r["x"]),
                    float(r["y"]),
                    float(r["speed"]),
                    csv_box(r),
                )
            )
    for t in tracks.values():
        t["rows"].sort()
        t["t0"], t["t1"] = t["rows"][0][0], t["rows"][-1][0]
        t["dur"] = (t["t1"] - t["t0"]) / 1e9
    return tracks


def load(path):
    if path.endswith(".csv"):
        return derive(load_rows(path))
    con = sqlite3.connect(f"file:{path}?mode=ro", uri=True)
    tracks = {}
    for tid, t0, t1, dur in con.execute(
        "SELECT track_id, start_unix_nanos, end_unix_nanos, track_duration_secs FROM lidar_tracks"
    ):
        tracks[tid] = {
            "t0": t0,
            "t1": t1,
            "dur": dur if dur is not None else (t1 - t0) / 1e9,
            "rows": [],
        }
    cols = {r[1] for r in con.execute("PRAGMA table_info(lidar_track_observations)")}
    box_cols = {"box_x", "box_y", "box_heading_rad", "box_length", "box_width"}
    extra = (
        ", box_x, box_y, box_heading_rad, box_length, box_width"
        if box_cols <= cols
        else ""
    )
    for row in con.execute(
        "SELECT track_id, ts_unix_nanos, x, y, speed_mps"
        + extra
        + " FROM lidar_track_observations ORDER BY track_id, ts_unix_nanos"
    ):
        tid, ts, x, y, spd = row[:5]
        box = None
        if extra and None not in row[5:10] and row[8] > 0 and row[9] > 0:
            box = tuple(float(v) for v in row[5:10])
        if tid in tracks:
            tracks[tid]["rows"].append((ts, x, y, spd or 0.0, box))
    con.close()
    return derive(tracks)


def csv_box(r):
    """A rows CSV row's drawn box: its centre (x, y), heading and extents, or None."""
    try:
        length, width = float(r.get("length") or 0), float(r.get("width") or 0)
        psi = math.radians(float(r.get("psi_deg") or 0))
    except ValueError:
        return None
    if length <= 0 or width <= 0:
        return None
    return (float(r["x"]), float(r["y"]), psi, length, width)


def inside(box, x, y, margin=FOOTPRINT_MARGIN_M):
    """Whether (x, y) lies in the box grown by margin on each side."""
    bx, by, psi, length, width = box
    c, s = math.cos(psi), math.sin(psi)
    dx, dy = x - bx, y - by
    return (
        abs(dx * c + dy * s) <= length / 2 + margin
        and abs(-dx * s + dy * c) <= width / 2 + margin
    )


def grid(box):
    """GRID x GRID points spread over the box."""
    bx, by, psi, length, width = box
    c, s = math.cos(psi), math.sin(psi)
    out = []
    for i in range(GRID):
        a = (i + 0.5) / GRID - 0.5
        for j in range(GRID):
            n = (j + 0.5) / GRID - 0.5
            u, v = a * length, n * width
            out.append((bx + u * c - v * s, by + u * s + v * c))
    return out


def footprints_meet(p, q):
    """Grown footprints overlap by half the smaller's area or more (grid estimate)."""

    def grown(b):
        m = 2 * FOOTPRINT_MARGIN_M
        return (b[0], b[1], b[2], b[3] + m, b[4] + m)

    gp, gq = grown(p), grown(q)
    small, big = (gp, gq) if gp[3] * gp[4] <= gq[3] * gq[4] else (gq, gp)
    pts = grid(small)
    return sum(inside(big, x, y, 0.0) for x, y in pts) * 2 >= len(pts)


def covers(row, other, match):
    """Whether other (a by_time entry) covers row under the match."""
    _, x, y, _, box = row
    _, ox, oy, obox = other
    if match == "footprint" and obox is not None:
        if inside(obox, x, y):
            return True
        return box is not None and footprints_meet(box, obox)
    return math.hypot(ox - x, oy - y) <= MATCH_M


def derive(tracks):
    for t in tracks.values():
        rows = t["rows"]
        t["disp"] = (
            math.hypot(rows[-1][1] - rows[0][1], rows[-1][2] - rows[0][2])
            if rows
            else 0.0
        )
        speeds = [r[3] for r in rows]
        t["p50"], t["p85"], t["p95"] = (
            pct(speeds, 0.5),
            pct(speeds, 0.85),
            pct(speeds, 0.95),
        )
        t["brief"] = t["dur"] < BRIEF_S
        t["lasting"] = t["dur"] >= LASTING_S
        t["static"] = (
            t["lasting"]
            and bool(rows)
            and t["disp"] < STATIC_DISP_M
            and t["p95"] < STATIC_P95_MPS
        )
        t["moving"] = (
            t["lasting"]
            and bool(rows)
            and t["disp"] >= MOVING_DISP_M
            and t["p50"] >= MOVING_P50_MPS
        )
    return tracks


def by_time(tracks):
    """Frame time -> [(track id, x, y, box)]."""
    idx = {}
    for tid, t in tracks.items():
        for ts, x, y, _, box in t["rows"]:
            idx.setdefault(ts, []).append((tid, x, y, box))
    return idx


def coverage(track, other_idx, match="point"):
    """(share of rows covered by any other track, best single-track share, best track id)."""
    rows = track["rows"]
    if not rows:
        return 0.0, 0.0, None
    any_hits = 0
    per = {}
    for row in rows:
        hit = set()
        for other in other_idx.get(row[0], ()):
            if covers(row, other, match):
                hit.add(other[0])
        if hit:
            any_hits += 1
        for oid in hit:
            per[oid] = per.get(oid, 0) + 1
    best = max(per.items(), key=lambda kv: kv[1]) if per else (None, 0)
    return any_hits / len(rows), best[1] / len(rows), best[0]


def summary(tracks):
    return {
        "tracks": len(tracks),
        "brief": sum(t["brief"] for t in tracks.values()),
        "near_static": sum(t["static"] for t in tracks.values()),
        "clutter": sum(t["brief"] or t["static"] for t in tracks.values()),
        "moving": sum(t["moving"] for t in tracks.values()),
        "moving_p85_speed_p50": pct(
            [t["p85"] for t in tracks.values() if t["moving"]], 0.5
        ),
        "moving_p85_speed_p85": pct(
            [t["p85"] for t in tracks.values() if t["moving"]], 0.85
        ),
    }


def brief_origin(tracks):
    """Each brief track of one arm: re-birth, split or other (see the docstring)."""
    idx = by_time(tracks)
    ends = [
        (t["t1"], t["rows"][-1][1], t["rows"][-1][2], tid)
        for tid, t in tracks.items()
        if t["rows"]
    ]
    out = {"re-birth": 0, "split": 0, "other": 0}
    for tid, t in tracks.items():
        if not t["brief"] or not t["rows"]:
            continue
        on = 0
        for ts, x, y, _, _ in t["rows"]:
            if any(
                o[0] != tid
                and o[3] is not None
                and tracks[o[0]]["lasting"]
                and inside(o[3], x, y)
                for o in idx.get(ts, ())
            ):
                on += 1
        on_body = on * 2 >= len(t["rows"])
        t0, x0, y0 = t["rows"][0][0], t["rows"][0][1], t["rows"][0][2]
        after_end = any(
            oid != tid
            and 0 <= t0 - e <= REBIRTH_S * 1e9
            and math.hypot(ex - x0, ey - y0) <= REBIRTH_M
            for e, ex, ey, oid in ends
        )
        out[
            "re-birth" if on_body and after_end else "split" if on_body else "other"
        ] += 1
    return out


def compare(a_path, b_path, match="point"):
    a, b = load(a_path), load(b_path)
    ai, bi = by_time(a), by_time(b)
    lost = shortened = 0
    dv = []
    ages = []
    for t in a.values():
        if not t["moving"]:
            continue
        share, best, bid = coverage(t, bi, match)
        if share < COVER_SHARE:
            lost += 1
            continue
        if best < COVER_SHARE:
            shortened += 1
        dv.append(b[bid]["p85"] - t["p85"])
        common = sorted({r[0] for r in t["rows"]} & {r[0] for r in b[bid]["rows"]})
        ages.append(((common[0] if common else b[bid]["t0"]) - b[bid]["t0"]) / 1e9)
    added = sum(
        1 for t in b.values() if t["moving"] and coverage(t, ai, match)[0] < COVER_SHARE
    )
    absdv = [abs(x) for x in dv]
    tail = {}
    for name, lo, hi in (
        ("under_1s", 0.0, 1.0),
        ("1_to_3s", 1.0, 3.0),
        ("3s_or_more", 3.0, math.inf),
    ):
        vals = [d for d, age in zip(absdv, ages) if lo <= age < hi]
        tail[name] = {
            "pairs": len(vals),
            "median_abs": pct(vals, 0.5),
            "p95_abs": pct(vals, 0.95),
        }
    return {
        "match": match,
        "brief_origin": brief_origin(b),
        "speed_tail_by_age": tail,
        "a": summary(a),
        "b": summary(b),
        "lost": lost,
        "shortened": shortened,
        "added": added,
        "matched": len(dv),
        "speed_p85_diff_median_abs": pct(absdv, 0.5),
        "speed_p85_diff_p95_abs": pct(absdv, 0.95),
        "speed_p85_diff_mean": sum(dv) / len(dv) if dv else 0.0,
        "speed_p85_diffs": dv,
    }


def main():
    ap = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    ap.add_argument("a_db")
    ap.add_argument("b_db")
    ap.add_argument("--match", choices=("point", "footprint"), default="point")
    ap.add_argument("--json")
    args = ap.parse_args()
    out = compare(args.a_db, args.b_db, args.match)
    text = json.dumps(out, indent=1, sort_keys=True)
    if args.json:
        with open(args.json, "w") as f:
            f.write(text + "\n")
    print(text)
    return 0


if __name__ == "__main__":
    sys.exit(main())
