#!/usr/bin/env python3
"""Extract a reproducible review queue, not physical ground truth or the lost 33 IDs."""

import argparse
from collections import deque
import hashlib
import json
import math
from pathlib import Path
import sqlite3


def lateral_residual(window):
    """Centre-point residual against a five-point time-domain XY line fit.

    The centre contributes to the fit, so this is an attenuated, non-causal
    anomaly proxy. Curvature, association errors, and geometry can all cause it.
    """
    centre_ns = window[2][0]
    times = [(row[0] - centre_ns) / 1e9 for row in window]
    mean_t = math.fsum(times) / 5
    mean_x = math.fsum(row[1] for row in window) / 5
    mean_y = math.fsum(row[2] for row in window) / 5
    denom = math.fsum((t - mean_t) ** 2 for t in times)
    if denom == 0:
        return None
    vx = math.fsum((t - mean_t) * (row[1] - mean_x) for t, row in zip(times, window)) / denom
    vy = math.fsum((t - mean_t) * (row[2] - mean_y) for t, row in zip(times, window)) / denom
    speed = math.hypot(vx, vy)
    if speed < 2:
        return None
    dx = window[2][1] - (mean_x - vx * mean_t)
    dy = window[2][2] - (mean_y - vy * mean_t)
    return abs((-vy * dx + vx * dy) / speed)


def extract(conn, threshold=0.5, max_gap_seconds=0.3):
    """Read a single SQLite transaction; hash the exact ordered input rows."""
    digest = hashlib.sha256()
    candidates = []
    window = deque(maxlen=5)
    previous_id = None
    current = None
    input_rows = eligible_windows = 0
    # max_speed_mps is a lifetime eligibility proxy, not reference speed.
    rows = conn.execute(
        "SELECT o.track_id,o.ts_unix_nanos,o.x,o.y,t.sensor_id,t.max_speed_mps "
        "FROM lidar_track_observations o JOIN lidar_tracks t ON t.track_id=o.track_id "
        "WHERE t.max_speed_mps>=6 ORDER BY o.track_id,o.ts_unix_nanos"
    )
    for row in rows:
        digest.update((json.dumps(row, separators=(",", ":"), allow_nan=False) + "\n").encode())
        input_rows += 1
        track_id, ts, x, y, sensor, _ = row
        if track_id != previous_id:
            if current is not None:
                candidates.append(current)
            current = None
            window.clear()
            previous_id = track_id
        if x is None or y is None or not all(math.isfinite(v) for v in (x, y)):
            window.clear()
            continue
        if window and not 0 < (ts - window[-1][0]) / 1e9 <= max_gap_seconds:
            window.clear()
        window.append((ts, x, y))
        if len(window) != 5:
            continue
        residual = lateral_residual(window)
        if residual is None:
            continue
        eligible_windows += 1
        if residual <= threshold:
            continue
        if current is None:
            current = {"track_id": track_id, "sensor_id": sensor, "event_count": 0,
                       "max_lateral_residual_m": 0, "peak_timestamp_ns": 0,
                       "review_status": "unreviewed_candidate"}
        current["event_count"] += 1
        if residual > current["max_lateral_residual_m"]:
            current["max_lateral_residual_m"] = residual
            current["peak_timestamp_ns"] = window[2][0]
    if current is not None:
        candidates.append(current)
    return {"schema_version": 1, "method": "five_point_time_xy_fit_v1",
            "reference_status": "replacement_candidates_not_original_33_not_ground_truth",
            "input_rows_sha256": digest.hexdigest(), "input_rows": input_rows,
            "eligible_windows": eligible_windows, "lifetime_min_speed_mps": 6,
            "local_fit_min_speed_mps": 2, "max_gap_seconds": max_gap_seconds,
            "threshold_metres": threshold, "candidate_count": len(candidates),
            "candidates": candidates}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("database", type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    # URI mode=ro refuses to create a missing database or modify the source.
    with sqlite3.connect(args.database.resolve().as_uri() + "?mode=ro", uri=True) as conn:
        conn.execute("BEGIN")
        report = extract(conn)
    report["source_basename"] = args.database.name
    report["extractor_sha256"] = hashlib.sha256(Path(__file__).read_bytes()).hexdigest()
    # Refuse to replace an earlier review queue, which may have human work on it.
    with args.output.open("x") as out:
        json.dump(report, out, indent=2, allow_nan=False)
        out.write("\n")
    print(f"{report['candidate_count']} unreviewed candidates from {report['input_rows']} rows")


if __name__ == "__main__":
    main()
