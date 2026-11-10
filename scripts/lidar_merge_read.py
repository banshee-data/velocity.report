#!/usr/bin/env python3
"""Inferred merges: candidate footprints that hold two of the defaults' moving tracks (l4clus0011).

Reads two replays of one case, the defaults' (A) and a candidate's (B), each an evidence database
or a `lidar-box-eval -rows` CSV as lidar_flip_cost.py reads them, and lists where B's drawn
footprint holds two vehicles that A keeps apart. Definitions are fixed here, before any arm of
l4clus0011 is replayed:

* moving: lidar_figure_sensitivity.py's peak test (l5id0024): a track lasting 2.0 s or more whose
  maximum speed (the largest 1.0 s rolling median of its row speeds, at least three rows) is
  1.5 m/s or more. Only A's tracks are tested; B's track need not be moving.
* a B row's footprint: its drawn box (heading, length and width) grown by 0.25 m on each side, as
  lidar_flip_cost.py's footprint match grows it. A B row without a box holds nothing.
* a duplicate pair: two of A's moving tracks whose drawn footprints (not grown) overlap, as a mean
  over the frames they share, by half the smaller's area or more. The overlap is estimated on a 9 by
  9 grid over the smaller box, as lidar_flip_cost.py estimates it. Shared frames where either row
  lacks a box are skipped; with no such frame left, the pair is a duplicate when their centres lie
  within 2.0 m of each other on half or more of their shared frames.
* an inferred merge: a B track and an unordered pair of A's moving tracks, not a duplicate pair,
  such that at some frame one of the B track's footprints contains both A tracks' centres at that
  frame. It is counted once per B track and A pair, however many frames hold it.

Read with B the same replay as A, the defaults against themselves, the count is the floor: a
footprint of the defaults' own that already holds two of their moving tracks (a vehicle close
beside or behind another, or a duplicate pair the overlap test misses).

Each merge is listed with its frames (count, first and last, in seconds from the first row of
either arm), the B rows' widths and lengths over those frames, the B rows' speeds, the A tracks'
maximum speeds and the pair's mean overlap.

Usage: lidar_merge_read.py A.db|A.csv B.db|B.csv [--json OUT]
"""

from __future__ import annotations

import argparse
import json
import math
import sys

import lidar_figure_sensitivity as fs
import lidar_flip_cost as fc

DUPLICATE_OVERLAP = 0.5


def overlap_share(p, q):
    """The share of the smaller drawn box (not grown) that lies inside the other (grid estimate)."""
    small, big = (p, q) if p[3] * p[4] <= q[3] * q[4] else (q, p)
    pts = fc.grid(small)
    return sum(fc.inside(big, x, y, 0.0) for x, y in pts) / len(pts)


def pair_overlap(t, u):
    """(mean overlap share over shared boxed frames or None, duplicate?) for two tracks."""
    ur = {r[0]: r for r in u["rows"]}
    shared = [(r, ur[r[0]]) for r in t["rows"] if r[0] in ur]
    boxed = [overlap_share(r[4], s[4]) for r, s in shared if r[4] and s[4]]
    if boxed:
        mean = sum(boxed) / len(boxed)
        return mean, mean >= DUPLICATE_OVERLAP
    near = sum(
        1 for r, s in shared if math.hypot(r[1] - s[1], r[2] - s[2]) <= fc.MATCH_M
    )
    return None, bool(shared) and near * 2 >= len(shared)


def merges(a, b):
    """The inferred merges of B against A (both loaded, A's moving marked under the peak test)."""
    moving_at = {}
    for aid, t in a.items():
        if not t["fig_moving"]:
            continue
        for ts, x, y, _, _ in t["rows"]:
            moving_at.setdefault(ts, []).append((aid, x, y))
    rows = [r[0] for t in list(a.values()) + list(b.values()) for r in t["rows"]]
    t0 = min(rows) if rows else 0

    def sec(ns):
        return round((ns - t0) / 1e9, 2)

    dup = {}
    found = {}
    for bid, bt in b.items():
        for ts, _, _, spd, box in bt["rows"]:
            if box is None:
                continue
            held = sorted(
                (aid for aid, x, y in moving_at.get(ts, ()) if fc.inside(box, x, y)),
                key=str,
            )
            for i in range(len(held)):
                for j in range(i + 1, len(held)):
                    pair = (held[i], held[j])
                    if pair not in dup:
                        dup[pair] = pair_overlap(a[pair[0]], a[pair[1]])
                    if dup[pair][1]:
                        continue
                    found.setdefault((bid, pair), []).append((ts, box, spd))
    out = []
    for (bid, (p, q)), hits in found.items():
        hits.sort(key=lambda h: h[0])
        widths = [h[1][4] for h in hits]
        lengths = [h[1][3] for h in hits]
        speeds = [h[2] for h in hits]
        out.append(
            {
                "b_track": str(bid),
                "a_tracks": [str(p), str(q)],
                "frames": len(hits),
                "first_s": sec(hits[0][0]),
                "last_s": sec(hits[-1][0]),
                "b_width_median": fc.pct(widths, 0.5),
                "b_width_max": max(widths),
                "b_length_median": fc.pct(lengths, 0.5),
                "b_speed_median": fc.pct(speeds, 0.5),
                "a_max_speeds": [a[p]["max_speed"], a[q]["max_speed"]],
                "pair_overlap_mean": dup[(p, q)][0],
            }
        )
    out.sort(key=lambda m: (m["first_s"], m["b_track"], m["a_tracks"]))
    return out


def compare(a_path, b_path):
    a = fc.load(a_path)
    b = a if b_path == a_path else fc.load(b_path)
    fs.mark_moving(a, "peak")
    if b is not a:
        fs.mark_moving(b, "peak")
    ms = merges(a, b)
    return {
        "moving_test": "peak",
        "a_moving": sum(1 for t in a.values() if t["fig_moving"]),
        "merges": len(ms),
        "b_tracks_merging": len({m["b_track"] for m in ms}),
        "merge_list": ms,
    }


def main():
    ap = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    ap.add_argument("a")
    ap.add_argument("b")
    ap.add_argument("--json")
    args = ap.parse_args()
    out = compare(args.a, args.b)
    text = json.dumps(out, indent=1, sort_keys=True)
    if args.json:
        with open(args.json, "w") as f:
            f.write(text + "\n")
    print(text)
    return 0


if __name__ == "__main__":
    sys.exit(main())
