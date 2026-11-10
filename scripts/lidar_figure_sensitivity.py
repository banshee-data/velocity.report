#!/usr/bin/env python3
"""How far a survey's figures move between two arms of one case (experiment figure0001).

Reads two replays of one case, the defaults' (A) and an arm's (B), each an evidence database or a
`lidar-box-eval -rows` CSV as lidar_flip_cost.py reads them, and reports the figures a survey
prints and how far B moves them. Definitions are fixed here, before any arm is read:

* moving track: lidar_flip_cost.py's: lasting 2.0 s or more, 5.0 m or more first to last, median
  speed 1.0 m/s or more.
* a track's maximum speed: the largest rolling median of its row speeds over a 1.0 s window
  holding at least three rows, so one frame's spike is not a maximum. A moving track with no such
  window has no maximum: it is counted but left out of the percentiles.
* count: the number of moving tracks. p85 and p98: nearest-rank percentiles of the moving tracks'
  maximum speeds, in m/s.
* a case with under ten moving tracks in A is reported with small_case true; figure0001 leaves it
  out of its medians.

`--moving peak` (l5id0024, fixed before its replays) replaces the moving test: a track is moving
when it lasts 2.0 s or more and its maximum speed (above) is 1.5 m/s or more. A track with no
maximum is not moving. Neither its life's median speed nor its first-to-last distance enters, so
joining two pieces of one body cannot move the test the way a whole-life median can. The default,
`--moving median`, is lidar_flip_cost.py's test and leaves the output as it was.

`--joins` (l5id0024) adds the join table. Each of A's moving tracks (under the chosen test) is
paired with the B track of best single-track coverage by footprint (lidar_flip_cost.py `--match
footprint`), when that share is 0.5 or more; a tie goes to the B track whose own rows the A track
covers most, so identical arms pair every track with itself. A join is a B track paired so with
two or more of A's moving tracks: B carries them in one track. Each join is listed with its
members' and the B track's row times, in seconds from the first row of either arm, and whether the
members overlap in time (concurrent, a duplicate in A) or follow one another (sequential, pieces).
A member that another moving B track also covers 0.5 or more still counts in B, so it is not
credited. A join's credit is its other members less one (never below 0); join_credit sums them and
credited_count_change adds it to count_change, so a joined body's pieces count as A counted them.
members_less_one is the credit without that exception, reported beside it.

Usage: lidar_figure_sensitivity.py A.db|A.csv B.db|B.csv [--moving median|peak] [--joins]
       [--json OUT]
"""

from __future__ import annotations

import argparse
import json
import sys

import lidar_flip_cost as fc

WINDOW_S = 1.0
MIN_ROWS = 3
SMALL_CASE = 10
PEAK_MPS = 1.5
MOVING_TESTS = ("median", "peak")


def max_speed(rows):
    """The largest rolling median of row speeds over WINDOW_S, or None."""
    best = None
    j = 0
    for i in range(len(rows)):
        while rows[i][0] - rows[j][0] > WINDOW_S * 1e9:
            j += 1
        window = sorted(r[3] for r in rows[j : i + 1])
        if len(window) < MIN_ROWS:
            continue
        mid = len(window) // 2
        med = window[mid] if len(window) % 2 else (window[mid - 1] + window[mid]) / 2
        if best is None or med > best:
            best = med
    return best


def mark_moving(tracks, moving="median"):
    """Set each track's "max_speed" and "fig_moving" under the named test; return tracks."""
    if moving not in MOVING_TESTS:
        raise ValueError(f"unknown moving test {moving!r}")
    for t in tracks.values():
        t["max_speed"] = max_speed(t["rows"])
        if moving == "peak":
            t["fig_moving"] = (
                t["lasting"]
                and t["max_speed"] is not None
                and t["max_speed"] >= PEAK_MPS
            )
        else:
            t["fig_moving"] = t["moving"]
    return tracks


def figures(tracks, moving="median"):
    mark_moving(tracks, moving)
    moving_tracks = [t for t in tracks.values() if t["fig_moving"]]
    maxima = [t["max_speed"] for t in moving_tracks if t["max_speed"] is not None]
    return {
        "count": len(moving_tracks),
        "with_maximum": len(maxima),
        "p85": fc.pct(maxima, 0.85) if maxima else None,
        "p98": fc.pct(maxima, 0.98) if maxima else None,
    }


def shares(track, other_idx):
    """Each other track's single-track footprint coverage of track's rows: {id: share}."""
    rows = track["rows"]
    per = {}
    for row in rows:
        hit = {
            o[0] for o in other_idx.get(row[0], ()) if fc.covers(row, o, "footprint")
        }
        for oid in hit:
            per[oid] = per.get(oid, 0) + 1
    return {oid: n / len(rows) for oid, n in per.items()} if rows else {}


def carrier(aid, a, b, a_shares):
    """The B track that carries A's track aid: best single-track share, 0.5 or more.

    Ties go to the B track whose own rows the A track covers most (the mutual best), so two
    identical arms pair each track with itself and a duplicate in A is not read as a join.
    """
    if not a_shares:
        return None
    best = max(a_shares.values())
    if best < fc.COVER_SHARE:
        return None
    tied = [bid for bid, v in a_shares.items() if v >= best - 1e-12]
    if len(tied) == 1:
        return tied[0]
    a_idx = fc.by_time({aid: a[aid]})
    return max(tied, key=lambda bid: (shares(b[bid], a_idx).get(aid, 0.0), str(bid)))


def joins(a, b):
    """B tracks that carry two or more of A's moving tracks (fig_moving), by footprint."""
    bi = fc.by_time(b)
    rows = [r[0] for t in list(a.values()) + list(b.values()) for r in t["rows"]]
    t0 = min(rows) if rows else 0

    def sec(ns):
        return round((ns - t0) / 1e9, 2)

    carried = {}
    elsewhere = {}
    for aid, t in a.items():
        if not t["fig_moving"] or not t["rows"]:
            continue
        sh = shares(t, bi)
        bid = carrier(aid, a, b, sh)
        if bid is None:
            continue
        carried.setdefault(bid, []).append((aid, sh[bid]))
        elsewhere[aid] = sorted(
            str(o)
            for o, v in sh.items()
            if o != bid and v >= fc.COVER_SHARE and b[o]["fig_moving"]
        )
    out = []
    for bid, members in carried.items():
        if len(members) < 2:
            continue
        members.sort(key=lambda m: a[m[0]]["rows"][0][0])
        spans = [(a[m]["rows"][0][0], a[m]["rows"][-1][0]) for m, _ in members]
        concurrent = any(
            min(spans[i][1], spans[j][1]) > max(spans[i][0], spans[j][0])
            for i in range(len(spans))
            for j in range(i + 1, len(spans))
        )
        only_here = sum(1 for m, _ in members if not elsewhere[m])
        bt = b[bid]
        out.append(
            {
                "b_track": str(bid),
                "b_start_s": sec(bt["rows"][0][0]),
                "b_end_s": sec(bt["rows"][-1][0]),
                "b_moving": bool(bt["fig_moving"]),
                "b_max_speed": bt["max_speed"],
                "b_median_speed": bt["p50"],
                "concurrent": concurrent,
                "credit": max(0, only_here - 1),
                "members": [
                    {
                        "a_track": str(m),
                        "start_s": sec(a[m]["rows"][0][0]),
                        "end_s": sec(a[m]["rows"][-1][0]),
                        "max_speed": a[m]["max_speed"],
                        "median_speed": a[m]["p50"],
                        "coverage": share,
                        "also_carried_by": elsewhere[m],
                    }
                    for m, share in members
                ],
            }
        )
    out.sort(key=lambda j: j["members"][0]["start_s"])
    return out


def compare(a_path, b_path, moving="median", with_joins=False):
    ta, tb = fc.load(a_path), fc.load(b_path)
    a, b = figures(ta, moving), figures(tb, moving)

    def diff(k):
        return None if a[k] is None or b[k] is None else b[k] - a[k]

    out = {
        "a": a,
        "b": b,
        "count_change": b["count"] - a["count"],
        "count_change_share": (
            (b["count"] - a["count"]) / a["count"] if a["count"] else None
        ),
        "p85_change": diff("p85"),
        "p98_change": diff("p98"),
        "small_case": a["count"] < SMALL_CASE,
    }
    if moving != "median":
        out["moving_test"] = moving
    if with_joins:
        js = joins(ta, tb)
        credit = sum(j["credit"] for j in js)
        out["moving_test"] = moving
        out["joins"] = js
        out["join_credit"] = credit
        out["credited_count_change"] = out["count_change"] + credit
        out["credited_count_change_share"] = (
            out["credited_count_change"] / a["count"] if a["count"] else None
        )
        # Every member less one per join, whether or not B counts it elsewhere.
        every = sum(len(j["members"]) - 1 for j in js)
        out["members_less_one"] = every
        out["members_less_one_count_change_share"] = (
            (out["count_change"] + every) / a["count"] if a["count"] else None
        )
    return out


def main():
    ap = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    ap.add_argument("a")
    ap.add_argument("b")
    ap.add_argument("--moving", choices=MOVING_TESTS, default="median")
    ap.add_argument("--joins", action="store_true")
    ap.add_argument("--json")
    args = ap.parse_args()
    out = compare(args.a, args.b, args.moving, args.joins)
    text = json.dumps(out, indent=1, sort_keys=True)
    if args.json:
        with open(args.json, "w") as f:
            f.write(text + "\n")
    print(text)
    return 0


if __name__ == "__main__":
    sys.exit(main())
