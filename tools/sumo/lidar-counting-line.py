#!/usr/bin/env python3
"""Counting-line headways from a published scene's tracks, for comparison with SUMO.

Reads public_html/src/scenes/<site>/assets/part-000 (header.json and the frame chunks) and the
site's static parts from tools/s2-archive/site-index.json. Only time inside a static part counts;
headways never span a gap between parts.

Subcommands, in the order they are used on a new site:

  modes     travel-direction modes of vehicle tracks, in the sensor frame
  profile   for one direction: where its vehicles stop (along) and their lanes (lateral)
  headways  per-lane counting-line headways, bootstrap intervals, and a cycle fold

Vehicles are chosen by motion, not class (most published tracks carry no class): travel of at
least 15 m, a peak speed of at least 4 m/s, a median box length of at least 2.5 m, and never
labelled pedestrian, cyclist or bird. Same-lane crossings under 0.8 s apart are one vehicle
split into two tracks and are dropped. Percentiles use the repository's nearest-rank rule (D-18).
"""

import argparse
import gzip
import json
import math
import random
import statistics
from collections import Counter, defaultdict
from datetime import datetime, timezone
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
NOT_VEHICLE = {"pedestrian", "cyclist", "bird"}
MIN_SAME_LANE_S = 0.8


def nearest_rank(values, p):
    """The repository's percentile rule (D-18, internal/stats.NearestRank): index ceil(p*n) - 1."""
    ordered = sorted(values)
    rank = p * len(ordered)
    if abs(rank - round(rank)) <= 1e-9 * max(1, round(rank)):
        rank = round(rank)
    return ordered[max(math.ceil(rank) - 1, 0)]


def load(site):
    part = REPO / "public_html/src/scenes" / site / "assets/part-000"
    header = json.loads((part / "header.json").read_text())
    start = int(header["start_ns"]) / 1e9
    tracks = defaultdict(list)
    for chunk in sorted((part / "frames").glob("chunk_*.ndjson.gz")):
        with gzip.open(chunk, "rt") as fh:
            for line in fh:
                frame = json.loads(line)
                t = frame["t"] / 1e6
                for tr in frame.get("tr", []):
                    tracks[tr["id"]].append(
                        (
                            t,
                            tr["x"],
                            tr["y"],
                            tr.get("spd", 0.0),
                            tr.get("c"),
                            tr.get("l") or 0.0,
                        )
                    )
    index = json.loads((REPO / "tools/s2-archive/site-index.json").read_text())
    entries = (
        index
        if isinstance(index, list)
        else index.get("sites", index.get("recordings", []))
    )
    entry = next(
        e for e in entries if e.get("published_as") == site or e.get("id") == site
    )

    def rel(iso):
        return datetime.fromisoformat(iso).astimezone(timezone.utc).timestamp() - start

    static = [(rel(p["start"]), rel(p["end"])) for p in entry["static_parts"]]
    return header, dict(tracks), static


def part_of(t, static):
    for i, (a, b) in enumerate(static):
        if a <= t <= b:
            return i
    return None


def vehicles(tracks):
    out = {}
    for k, p in tracks.items():
        if len(p) < 5 or {q[4] for q in p} & NOT_VEHICLE:
            continue
        travel = math.hypot(p[-1][1] - p[0][1], p[-1][2] - p[0][2])
        length = sorted(q[5] for q in p)[len(p) // 2]
        if travel >= 15.0 and max(q[3] for q in p) >= 4.0 and length >= 2.5:
            out[k] = p
    return out


def travel_deg(p):
    return math.degrees(math.atan2(p[-1][2] - p[0][2], p[-1][1] - p[0][1])) % 360


def along_lat(x, y, deg):
    a = math.radians(deg)
    return x * math.cos(a) + y * math.sin(a), -x * math.sin(a) + y * math.cos(a)


def heading_to(V, bearing, tol=30.0):
    return {
        k: p
        for k, p in V.items()
        if abs((travel_deg(p) - bearing + 180) % 360 - 180) < tol
    }


def cmd_modes(args):
    header, tracks, static = load(args.site)
    V = vehicles(tracks)
    print(f"{len(V)} vehicle tracks of {len(tracks)}; build {header['build_version']}")
    bins = Counter(int(travel_deg(p) // 15) * 15 for p in V.values())
    print("travel direction, 15° bins:", dict(sorted(bins.items())))
    for centre in args.centres:
        b = [travel_deg(p) for p in heading_to(V, centre).values()]
        s, c = sum(math.sin(math.radians(x)) for x in b), sum(
            math.cos(math.radians(x)) for x in b
        )
        print(
            f"  near {centre:g}°: n={len(b)} circular mean {math.degrees(math.atan2(s, c)) % 360:.1f}°"
        )


def cmd_profile(args):
    _, tracks, _ = load(args.site)
    D = heading_to(vehicles(tracks), args.bearing)
    stopped, lateral = Counter(), Counter()
    for p in D.values():
        for q in p:
            u, v = along_lat(q[1], q[2], args.bearing)
            if q[3] < 0.5:
                stopped[int(u // 5) * 5] += 1
            elif q[3] > 3:
                lateral[round(v * 2) / 2] += 1
    print(f"{len(D)} tracks travelling {args.bearing:g}°")
    print(
        "stopped observations by along position, 5 m bins:",
        dict(sorted(stopped.items())),
    )
    print(
        "moving observations by lateral position, 0.5 m bins:",
        dict(sorted(lateral.items())),
    )


def crossings(D, line, bearing, lane_split, static):
    out = []
    for p in D.values():
        prev = None
        for q in p:
            u, v = along_lat(q[1], q[2], bearing)
            if prev and prev[1] < line <= u:
                f = (line - prev[1]) / (u - prev[1])
                t = prev[0] + f * (q[0] - prev[0])
                part = part_of(t, static)
                if part is not None:
                    out.append(
                        (t, 0 if prev[2] + f * (v - prev[2]) < lane_split else 1, part)
                    )
                break
            prev = (q[0], u, v)
    kept, last, dropped = [], {}, 0
    for c in sorted(out):
        key = (c[1], c[2])
        if key in last and c[0] - last[key] < MIN_SAME_LANE_S:
            dropped += 1
            continue
        kept.append(c)
        last[key] = c[0]
    return kept, dropped


def lane_headways(cr, lane, static):
    h = []
    for part in range(len(static)):
        t = [c[0] for c in cr if c[1] == lane and c[2] == part]
        h += [b - a for a, b in zip(t, t[1:])]
    return h


def cmd_headways(args):
    header, tracks, static = load(args.site)
    D = heading_to(vehicles(tracks), args.bearing)
    static_s = sum(b - a for a, b in static)
    print(
        f"{args.site}: {len(D)} tracks travelling {args.bearing:g}°, static time {static_s / 60:.1f} min,"
        f" build {header['build_version']}, tuning {header['tuning_hash'][:19]}"
    )
    rng = random.Random(args.seed)
    for line in args.lines:
        cr, dropped = crossings(D, line, args.bearing, args.lane_split, static)
        print(
            f"line {line:+g} m: {len(cr)} crossings, {dropped} split-track duplicates dropped"
        )
        for lane in (0, 1):
            h = lane_headways(cr, lane, static)
            n = sum(c[1] == lane for c in cr)
            short = sum(x < 2.5 for x in h) / len(h)
            print(
                f"  lane {lane}: n={n} flow={n / static_s * 3600:.0f}/h headways={len(h)}"
                f" p15={nearest_rank(h, 0.15):.1f}s p50={nearest_rank(h, 0.5):.1f}s"
                f" p85={nearest_rank(h, 0.85):.1f}s"
                f" cv={statistics.pstdev(h) / statistics.mean(h):.2f} share<2.5s={short:.0%}"
            )
            if args.bootstrap:
                stats = {"p15": [], "p85": [], "share": []}
                for _ in range(args.bootstrap):
                    s = [rng.choice(h) for _ in h]
                    stats["p15"].append(nearest_rank(s, 0.15))
                    stats["p85"].append(nearest_rank(s, 0.85))
                    stats["share"].append(sum(x < 2.5 for x in s) / len(s))
                lo = {k: nearest_rank(v, 0.025) for k, v in stats.items()}
                hi = {k: nearest_rank(v, 0.975) for k, v in stats.items()}
                print(
                    f"    95% bootstrap: p15 {lo['p15']:.1f}-{hi['p15']:.1f}s,"
                    f" p85 {lo['p85']:.0f}-{hi['p85']:.0f}s,"
                    f" share<2.5s {lo['share']:.0%}-{hi['share']:.0%}"
                )
    # Queue discharge starts: a vehicle stopped near the queue front that starts moving.
    starts = []
    for p in D.values():
        stopped_at = None
        for q in p:
            u, _ = along_lat(q[1], q[2], args.bearing)
            if q[3] < 0.5 and args.queue_front - 10 <= u <= args.queue_front:
                stopped_at = q[0]
            elif stopped_at is not None and q[3] > 2.0:
                if part_of(q[0], static) is not None:
                    starts.append(q[0])
                break
    merged = []
    for t in sorted(starts):
        if not merged or t - merged[-1] > 15:
            merged.append(t)
    gaps = sorted(
        round(b - a)
        for a, b in zip(merged, merged[1:])
        if part_of(a, static) == part_of(b, static)
    )
    scores = []
    for cycle in range(40, 131):
        ph = [2 * math.pi * (t % cycle) / cycle for t in merged]
        scores.append(
            (
                math.hypot(sum(map(math.cos, ph)), sum(map(math.sin, ph))) / len(ph),
                cycle,
            )
        )
    print(f"queue discharge starts: {len(merged)}; gaps between them (s): {gaps}")
    print(
        "cycle fold, top five (concentration, cycle s):",
        [(round(r, 2), c) for r, c in sorted(scores)[-5:][::-1]],
    )


def main():
    ap = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    ap.add_argument("--site", default="columbus-broadway")
    sub = ap.add_subparsers(dest="cmd", required=True)
    m = sub.add_parser("modes")
    m.add_argument("--centres", type=float, nargs="*", default=[])
    p = sub.add_parser("profile")
    p.add_argument(
        "--bearing",
        type=float,
        required=True,
        help="sensor-frame travel direction, degrees",
    )
    h = sub.add_parser("headways")
    h.add_argument("--bearing", type=float, required=True)
    h.add_argument(
        "--lines",
        type=float,
        nargs="+",
        default=[-2.0, 25.0],
        help="along positions, metres",
    )
    h.add_argument(
        "--lane-split",
        type=float,
        default=-3.6,
        help="lateral metres: below is lane 0 (kerb)",
    )
    h.add_argument(
        "--queue-front",
        type=float,
        default=-3.0,
        help="along position of the queue front, metres",
    )
    h.add_argument("--bootstrap", type=int, default=2000)
    h.add_argument("--seed", type=int, default=1)
    args = ap.parse_args()
    {"modes": cmd_modes, "profile": cmd_profile, "headways": cmd_headways}[args.cmd](
        args
    )


if __name__ == "__main__":
    main()
