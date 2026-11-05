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

Usage: lidar_figure_sensitivity.py A.db|A.csv B.db|B.csv [--json OUT]
"""

from __future__ import annotations

import argparse
import json
import sys

import lidar_flip_cost as fc

WINDOW_S = 1.0
MIN_ROWS = 3
SMALL_CASE = 10


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


def figures(tracks):
    moving = [t for t in tracks.values() if t["moving"]]
    maxima = [m for m in (max_speed(t["rows"]) for t in moving) if m is not None]
    return {
        "count": len(moving),
        "with_maximum": len(maxima),
        "p85": fc.pct(maxima, 0.85) if maxima else None,
        "p98": fc.pct(maxima, 0.98) if maxima else None,
    }


def compare(a_path, b_path):
    a, b = figures(fc.load(a_path)), figures(fc.load(b_path))

    def diff(k):
        return None if a[k] is None or b[k] is None else b[k] - a[k]

    return {
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
