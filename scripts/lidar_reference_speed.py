#!/usr/bin/env python3
"""Track speed against reviewed physical references (experiment figure0002).

Reads a pack's physical-references.json and one replay's evidence database, and reports, for each
referenced vehicle, a reference speed from its reviewed keyframes and the matched track's speed
over the same span. Definitions are fixed in figure0002 before any track speed was read:

* reference keyframes: review status "reviewed" and origin "independent".
* span: the anchor kind with the most such keyframes; its first and last keyframe of that kind.
* reference speed: the distance between the span's two positions over the time between them; its
  bound, the sum of the two positions' bounds over the same time.
* matched track: at each span keyframe, the track whose row at that frame is nearest the keyframe's
  position within 6.0 m; the track matched at most keyframes (ties to the earlier keyframe). A
  vehicle with no track within 6.0 m at half its keyframes is unmatched.
* track speed: the mean of the matched track's row speeds whose frame lies within the span; and
  its displacement between the rows nearest the span's ends over the time between them.
* error: track speed less reference speed; within the bound when its magnitude is no more than
  the reference bound.

Usage: lidar_reference_speed.py physical-references.json EVIDENCE.db [--json OUT]
"""

from __future__ import annotations

import argparse
import bisect
import json
import math
import sys
from collections import Counter

import lidar_flip_cost as fc

MATCH_M = 6.0


def span(obj):
    """(first, last) keyframes of the anchor kind with most reviewed independent keyframes."""
    kfs = [
        k
        for k in obj["keyframes"]
        if k.get("review", {}).get("status") == "reviewed"
        and k.get("review", {}).get("origin") == "independent"
        and k.get("position", {}).get("status") in ("observed", "inferred")
    ]
    by_kind = {}
    for k in kfs:
        by_kind.setdefault(k["anchor"]["kind"], []).append(k)
    if not by_kind:
        return None
    kind = max(by_kind, key=lambda a: (len(by_kind[a]), a))
    ks = sorted(by_kind[kind], key=lambda k: k["timestamp_ns"])
    if len(ks) < 2:
        return None
    return kind, ks


def reference_speed(first, last):
    dt = (last["timestamp_ns"] - first["timestamp_ns"]) / 1e9
    if dt <= 0:
        raise ValueError(
            f"span from {first['timestamp_ns']} to {last['timestamp_ns']} ns is not positive: "
            "two reviewed keyframes of one anchor share a timestamp, or are out of order"
        )
    p, q = first["position"], last["position"]
    dist = math.hypot(q["x_m"] - p["x_m"], q["y_m"] - p["y_m"])
    return dist / dt, (p["bound_m"] + q["bound_m"]) / dt, dt


def nearest_row(rows, ts):
    return min(rows, key=lambda r: abs(r[0] - ts)) if rows else None


def nearest_frame(frames, ts):
    """The frame time nearest ts in sorted frames, or None."""
    if not frames:
        return None
    i = bisect.bisect_left(frames, ts)
    near = frames[max(0, i - 1) : i + 1]
    return min(near, key=lambda t: abs(t - ts))


def match(tracks, idx, keyframes):
    frames = sorted(idx)
    votes = []
    for k in keyframes:
        frame = nearest_frame(frames, k["timestamp_ns"])
        best = None
        for tid, x, y, _ in idx.get(frame, ()):
            d = math.hypot(x - k["position"]["x_m"], y - k["position"]["y_m"])
            if d <= MATCH_M and (best is None or d < best[1]):
                best = (tid, d)
        if best:
            votes.append(best[0])
    if len(votes) * 2 < len(keyframes):
        return None
    counts = Counter(votes)
    top = max(counts.values())
    return next(v for v in votes if counts[v] == top)


def measure(refs_path, db_path):
    with open(refs_path) as f:
        refs = json.load(f)
    tracks = fc.load(db_path)
    idx = fc.by_time(tracks)
    out = []
    for obj in refs["objects"]:
        s = span(obj)
        if s is None:
            out.append({"object_id": obj["object_id"], "span": None})
            continue
        kind, ks = s
        first, last = ks[0], ks[-1]
        ref, bound, dt = reference_speed(first, last)
        row = {
            "object_id": obj["object_id"],
            "anchor": kind,
            "keyframes": len(ks),
            "span_s": dt,
            "reference_mps": ref,
            "bound_mps": bound,
        }
        tid = match(tracks, idx, ks)
        row["track"] = tid
        if tid is not None:
            rows = tracks[tid]["rows"]
            inside = [
                r[3]
                for r in rows
                if first["timestamp_ns"] <= r[0] <= last["timestamp_ns"]
            ]
            row["track_mean_mps"] = sum(inside) / len(inside) if inside else None
            a, b = nearest_row(rows, first["timestamp_ns"]), nearest_row(
                rows, last["timestamp_ns"]
            )
            tdt = (b[0] - a[0]) / 1e9 if a and b else 0
            row["track_displacement_mps"] = (
                math.hypot(b[1] - a[1], b[2] - a[2]) / tdt if tdt > 0 else None
            )
            for k in ("track_mean_mps", "track_displacement_mps"):
                v = row[k]
                row[k.replace("track_", "error_")] = None if v is None else v - ref
                row[k.replace("track_", "within_bound_")] = (
                    None if v is None else abs(v - ref) <= bound
                )
        out.append(row)
    return {"vehicles": out}


def main():
    ap = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    ap.add_argument("references")
    ap.add_argument("db")
    ap.add_argument("--json")
    args = ap.parse_args()
    out = measure(args.references, args.db)
    text = json.dumps(out, indent=1, sort_keys=True)
    if args.json:
        with open(args.json, "w") as f:
            f.write(text + "\n")
    print(text)
    return 0


if __name__ == "__main__":
    sys.exit(main())
