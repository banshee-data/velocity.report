#!/usr/bin/env python3
"""Rank time windows of an evidence database by vehicle following, to choose clips to annotate.

A review queue for the annotation operator, not a measurement: the pairing
below uses tracker estimates, so a split or merged track shows up as churn
rather than being corrected. That churn is the point. A follower whose
nearest leader keeps changing inside a few seconds is usually one lead
vehicle the tracker broke into several tracks, which is the continuity
failure a headway measurement cannot survive.

Two tracks form a following pair in a frame when both move at least
--min-speed, their headings agree within --max-heading-deg, and the leader is
between --min-gap and --max-gap metres ahead along the follower's course and
within --max-lateral metres of its line. Windows are ranked by pair-seconds.

With --site-index and --case, each window is also placed in its source
capture (file name and offset in seconds), which is what `velocity lidar
pcap-replay --start-seconds` wants when cutting the clip.
"""

import argparse
from collections import defaultdict
from datetime import datetime, timedelta, timezone
import hashlib
import json
import math
from pathlib import Path
import re
import sqlite3
import statistics
import sys

SCHEMA = "velocity.report/following-windows"
SCHEMA_VERSION = 1


def following_pairs(
    estimates, min_speed, max_heading_deg, min_gap, max_gap, max_lateral
):
    """Yield (t, follower, leader, gap) for every qualifying pair in every frame.

    estimates maps a frame time to [(seq, x, y, vx, vy), ...].
    """
    min_cos = math.cos(math.radians(max_heading_deg))
    for t in sorted(estimates):
        moving = []
        for seq, x, y, vx, vy in estimates[t]:
            speed = math.hypot(vx, vy)
            if speed >= min_speed:
                moving.append((seq, x, y, vx / speed, vy / speed))
        for f in moving:
            for lead in moving:
                if lead[0] == f[0]:
                    continue
                if f[3] * lead[3] + f[4] * lead[4] < min_cos:
                    continue
                dx, dy = lead[1] - f[1], lead[2] - f[2]
                along = dx * f[3] + dy * f[4]
                across = abs(dx * f[4] - dy * f[3])
                if min_gap <= along <= max_gap and across <= max_lateral:
                    yield t, f[0], lead[0], along


def frame_period_seconds(frame_times):
    """The median interval between distinct frames, in seconds."""
    times = sorted(set(frame_times))
    if len(times) < 2:
        return 0.1
    return statistics.median(b - a for a, b in zip(times, times[1:])) / 1e9


def rank_windows(estimates, window_seconds=10, **pairing):
    """Summarise following per window, most pair-seconds first."""
    period = frame_period_seconds(estimates.keys())
    width = int(window_seconds * 1e9)
    windows = defaultdict(
        lambda: {
            "frames": 0,
            "pairs": set(),
            "leaders": set(),
            "followers": set(),
            "closest_gap_m": math.inf,
        }
    )
    nearest = defaultdict(dict)  # window -> follower -> [(t, nearest leader)]
    for t, follower, leader, gap in following_pairs(estimates, **pairing):
        w = windows[t // width]
        w["frames"] += 1
        w["pairs"].add((follower, leader))
        w["leaders"].add(leader)
        w["followers"].add(follower)
        w["closest_gap_m"] = min(w["closest_gap_m"], gap)
        best = nearest[t // width].setdefault(follower, {})
        if t not in best or gap < best[t][1]:
            best[t] = (leader, gap)

    out = []
    for key, w in windows.items():
        # A follower's nearest leader changing between frames: one lead
        # vehicle split across tracks, or a genuine cut-in. Either is worth a
        # reviewed reference.
        changes = 0
        for per_frame in nearest[key].values():
            leaders = [per_frame[t][0] for t in sorted(per_frame)]
            changes += sum(1 for a, b in zip(leaders, leaders[1:]) if a != b)
        out.append(
            {
                "window_start_unix_nanos": key * width,
                "pair_frames": w["frames"],
                "pair_seconds": round(w["frames"] * period, 2),
                "pairs": len(w["pairs"]),
                "followers": len(w["followers"]),
                "leaders": len(w["leaders"]),
                "leader_changes": changes,
                "closest_gap_m": round(w["closest_gap_m"], 2),
                "follower_seqs": sorted(w["followers"]),
                "leader_seqs": sorted(w["leaders"]),
            }
        )
    # Rank on the exact frame count: pair_seconds is rounded for reading, and
    # at a high frame rate two counts can round to the same value.
    out.sort(key=lambda w: (-w["pair_frames"], w["window_start_unix_nanos"]))
    return out, period


def load_estimates(conn, stage, source):
    """Read one source's estimates at one stage, keyed by frame time, and hash the rows read."""
    sources = [
        r[0]
        for r in conn.execute(
            "SELECT DISTINCT source_id FROM lidar_track_estimates WHERE stage = ? ORDER BY source_id",
            (stage,),
        )
    ]
    if not sources:
        raise SystemExit(f"no {stage} estimates in the database")
    if source is None:
        if len(sources) > 1:
            raise SystemExit(
                "several sources; pass --source, one of:\n  " + "\n  ".join(sources)
            )
        source = sources[0]
    elif source not in sources:
        raise SystemExit(
            f"source {source} has no {stage} estimates; the database holds:\n  "
            + "\n  ".join(sources)
        )
    digest = hashlib.sha256()
    estimates = defaultdict(list)
    rows = conn.execute(
        "SELECT creation_sequence, frame_unix_nanos, x, y, vx, vy FROM lidar_track_estimates "
        "WHERE stage = ? AND source_id = ? ORDER BY frame_unix_nanos, creation_sequence, estimate_id",
        (stage, source),
    )
    for row in rows:
        digest.update((json.dumps(row, separators=(",", ":")) + "\n").encode())
        seq, t, x, y, vx, vy = row
        if all(v is not None and math.isfinite(v) for v in (x, y, vx, vy)):
            estimates[t].append((seq, x, y, vx, vy))
    return estimates, source, digest.hexdigest()


CAPTURE_TIME = re.compile(r"_(\d{14})_")


def capture_starts(site_index, case_id):
    """The case's captures and their UTC start times, from the local time in each file name."""
    for site in site_index:
        if site.get("id") != case_id:
            continue
        offset = datetime.fromisoformat(site["start"]).utcoffset() or timedelta()
        starts = []
        for name in site["captures"]:
            m = CAPTURE_TIME.search(name)
            if not m:
                raise SystemExit(f"capture {name} has no start time in its name")
            local = datetime.strptime(m.group(1), "%Y%m%d%H%M%S")
            starts.append((name, (local - offset).replace(tzinfo=timezone.utc)))
        return sorted(starts, key=lambda s: s[1])
    raise SystemExit(f"case {case_id} is not in the site index")


def place(window_start_unix_nanos, starts):
    """The capture a window starts in and its offset into that capture, in seconds."""
    when = datetime.fromtimestamp(window_start_unix_nanos / 1e9, tz=timezone.utc)
    found = None
    for name, start in starts:
        if start <= when:
            found = (name, (when - start).total_seconds())
    return found


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    parser.add_argument(
        "database", type=Path, help="evidence database holding lidar_track_estimates"
    )
    parser.add_argument(
        "--stage", default="online", help="estimate stage (default online)"
    )
    parser.add_argument("--source", help="source id, when the database holds several")
    parser.add_argument("--window-seconds", type=float, default=10)
    parser.add_argument(
        "--top", type=int, default=15, help="windows to print (0 for all)"
    )
    parser.add_argument("--min-speed", type=float, default=3, help="m/s, both tracks")
    parser.add_argument("--max-heading-deg", type=float, default=20)
    parser.add_argument(
        "--min-gap",
        type=float,
        default=3,
        help="metres ahead along the follower's course",
    )
    parser.add_argument("--max-gap", type=float, default=40)
    parser.add_argument(
        "--max-lateral", type=float, default=2.5, help="metres from the follower's line"
    )
    parser.add_argument(
        "--site-index",
        type=Path,
        help="tools/s2-archive/site-index.json, to place windows in captures",
    )
    parser.add_argument(
        "--case", help="case id in the site index, e.g. embarcadero-folsom"
    )
    parser.add_argument(
        "--json",
        type=Path,
        help="also write the full ranking here (refuses to overwrite)",
    )
    args = parser.parse_args(argv)
    if (args.site_index is None) != (args.case is None):
        parser.error("--site-index and --case go together")

    # URI mode=ro refuses to create a missing database or modify the source.
    with sqlite3.connect(
        args.database.resolve().as_uri() + "?mode=ro", uri=True
    ) as conn:
        conn.execute("BEGIN")
        estimates, source, rows_sha = load_estimates(conn, args.stage, args.source)
    pairing = dict(
        min_speed=args.min_speed,
        max_heading_deg=args.max_heading_deg,
        min_gap=args.min_gap,
        max_gap=args.max_gap,
        max_lateral=args.max_lateral,
    )
    ranked, period = rank_windows(estimates, args.window_seconds, **pairing)

    starts = None
    if args.site_index:
        starts = capture_starts(json.loads(args.site_index.read_text()), args.case)
        for w in ranked:
            placed = place(w["window_start_unix_nanos"], starts)
            if placed:
                w["capture"], w["offset_seconds"] = placed[0], round(placed[1], 1)

    shown = ranked if args.top == 0 else ranked[: args.top]
    header = f"{'window (UTC)':<20} {'pair-s':>6} {'pairs':>5} {'leaders':>7} {'changes':>7} {'gap m':>5}"
    if starts:
        header += "  capture @ offset"
    print(
        f"{source} ({args.stage}), frame period {period:.3f} s, {len(ranked)} windows with following"
    )
    print(header)
    for w in shown:
        when = datetime.fromtimestamp(
            w["window_start_unix_nanos"] / 1e9, tz=timezone.utc
        )
        line = (
            f"{when:%Y-%m-%d %H:%M:%S} {w['pair_seconds']:>6} {w['pairs']:>5} {w['leaders']:>7} "
            f"{w['leader_changes']:>7} {w['closest_gap_m']:>5}"
        )
        if "capture" in w:
            line += f"  {w['capture']} @ {w['offset_seconds']} s"
        print(line)

    if args.json:
        report = {
            "schema": SCHEMA,
            "schema_version": SCHEMA_VERSION,
            "status": "unreviewed_candidates_from_tracker_estimates",
            "source_basename": args.database.name,
            "source_id": source,
            "stage": args.stage,
            "input_rows_sha256": rows_sha,
            "extractor_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
            "window_seconds": args.window_seconds,
            "frame_period_seconds": period,
            "pairing": pairing,
            "case": args.case,
            "windows": ranked,
        }
        # Refuse to replace an earlier queue, which may have review notes on it.
        with args.json.open("x") as out:
            json.dump(report, out, indent=2, allow_nan=False)
            out.write("\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
