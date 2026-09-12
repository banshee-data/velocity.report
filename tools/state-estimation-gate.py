#!/usr/bin/env python3
"""Build the fail-closed G-GEO-1 evidence report from identified replay runs.

This program measures each moving track against its own fitted straight line.
It is a geometry metric, unlike an innovation which mixes measurement and model
error.  A final pass still requires reviewed hold-out manoeuvre and ground-truth
quality reports: no heuristic may silently stand in for either annotation set.
"""

import argparse
import json
import math
import sqlite3
from collections import defaultdict
from pathlib import Path

MIN_MOVING_SPEED_MPS = 0.5
MIN_TRACK_SAMPLES = 5
EXCURSION_METRES = 0.5


def percentile(values, q):
    values = sorted(values)
    if not values:
        return None
    index = (len(values) - 1) * q
    lower, upper = math.floor(index), math.ceil(index)
    return values[lower] + (values[upper] - values[lower]) * (index - lower)


def principal_line(points):
    """Return the centroid and unit major axis of a 2-D total-least-squares line."""
    centre_x = sum(point[0] for point in points) / len(points)
    centre_y = sum(point[1] for point in points) / len(points)
    xx = sum((point[0] - centre_x) ** 2 for point in points)
    xy = sum((point[0] - centre_x) * (point[1] - centre_y) for point in points)
    yy = sum((point[1] - centre_y) ** 2 for point in points)
    if xx + yy == 0:
        return None
    angle = 0.5 * math.atan2(2 * xy, xx - yy)
    return centre_x, centre_y, math.cos(angle), math.sin(angle)


def database_identities(connection):
    rows = connection.execute("""
        SELECT source_id, calibration_id, estimator_id, observation_model_id,
               param_hash, COUNT(*)
          FROM lidar_track_estimates
         WHERE stage = 'online'
         GROUP BY source_id, calibration_id, estimator_id, observation_model_id, param_hash
         ORDER BY source_id, calibration_id, estimator_id, observation_model_id, param_hash
        """)
    return [
        {
            "source_id": row[0],
            "calibration_id": row[1],
            "estimator_id": row[2],
            "observation_model_id": row[3],
            "param_hash": row[4],
            "estimate_count": row[5],
        }
        for row in rows
    ]


def metrics(path):
    connection = sqlite3.connect(path)
    rows = connection.execute("""
        SELECT source_id, track_id, measurement_unix_nanos, x, y, vx, vy
          FROM lidar_track_estimates
         WHERE stage = 'online'
         ORDER BY source_id, track_id, measurement_unix_nanos
        """)
    tracks = defaultdict(lambda: defaultdict(list))
    for source, track, _, x, y, vx, vy in rows:
        tracks[source][track].append((x, y, math.hypot(vx, vy)))

    per_source = {}
    for source, source_tracks in tracks.items():
        lateral = []
        eligible_tracks = 0
        excursion_tracks = 0
        excluded = {"short_or_stationary": 0, "degenerate": 0}
        for samples in source_tracks.values():
            if (
                len(samples) < MIN_TRACK_SAMPLES
                or max(sample[2] for sample in samples) <= MIN_MOVING_SPEED_MPS
            ):
                excluded["short_or_stationary"] += 1
                continue
            line = principal_line(samples)
            if line is None:
                excluded["degenerate"] += 1
                continue
            centre_x, centre_y, axis_x, axis_y = line
            residuals = [
                abs(-axis_y * (x - centre_x) + axis_x * (y - centre_y))
                for x, y, _ in samples
            ]
            eligible_tracks += 1
            lateral.extend(residuals)
            excursion_tracks += any(value > EXCURSION_METRES for value in residuals)
        per_source[source] = {
            "tracks": len(source_tracks),
            "eligible_moving_tracks": eligible_tracks,
            "excluded_tracks": excluded,
            "lateral_samples": len(lateral),
            "line_lateral_p99_m": percentile(lateral, 0.99),
            "line_excursion_rate": (
                excursion_tracks / eligible_tracks if eligible_tracks else None
            ),
        }
    return per_source, database_identities(connection)


def valid_manoeuvre(report):
    return (
        report
        and report.get("reviewed") is True
        and report.get("preserved_fraction", 0) >= 0.90
    )


def valid_quality(report):
    return (
        report
        and report.get("reviewed") is True
        and report.get("detection_regression", math.inf) <= 0.05
        and report.get("fragmentation_regression", math.inf) <= 0.05
    )


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--reference-db", required=True, type=Path, help="medoid_v0 replay database"
    )
    parser.add_argument(
        "--candidate-db", required=True, type=Path, help="obb_centre_v1 replay database"
    )
    parser.add_argument(
        "--holdout-source",
        action="append",
        default=[],
        help="source identity reserved from tuning; repeat",
    )
    parser.add_argument(
        "--manoeuvre-report",
        type=Path,
        help="reviewed labelled manoeuvre JSON; requires preserved_fraction >= 0.90",
    )
    parser.add_argument(
        "--quality-report",
        type=Path,
        help="reviewed GroundTruthEvaluator JSON; both regressions must be <= 0.05",
    )
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()

    reference, reference_identities = metrics(args.reference_db)
    candidate, candidate_identities = metrics(args.candidate_db)
    sources = sorted(set(reference) | set(candidate))
    rows, geometry_ok = [], True
    for source in sources:
        base, corrected = reference.get(source), candidate.get(source)
        comparison = {"source_id": source, "reference": base, "candidate": corrected}
        if (
            not base
            or not corrected
            or not base["line_lateral_p99_m"]
            or not corrected["line_lateral_p99_m"]
        ):
            comparison["geometry_gate"] = "missing comparable moving-track population"
            geometry_ok = False
        else:
            reduction = 1 - corrected["line_lateral_p99_m"] / base["line_lateral_p99_m"]
            comparison["p99_lateral_reduction"] = reduction
            comparison["geometry_gate"] = (
                reduction >= 0.50 and corrected["line_excursion_rate"] < 0.04
            )
            geometry_ok = geometry_ok and comparison["geometry_gate"]
        rows.append(comparison)

    holdout = set(args.holdout_source)
    ref_by_source = {
        identity["source_id"]: identity for identity in reference_identities
    }
    candidate_by_source = {
        identity["source_id"]: identity for identity in candidate_identities
    }
    identity_ok = bool(holdout)
    identity_errors = []
    for source in sorted(holdout):
        ref, cand = ref_by_source.get(source), candidate_by_source.get(source)
        if not ref or not cand:
            identity_ok = False
            identity_errors.append(f"{source}: absent from one replay")
            continue
        if (
            ref["observation_model_id"] != "medoid_v0"
            or cand["observation_model_id"] != "obb_centre_v1"
        ):
            identity_ok = False
            identity_errors.append(f"{source}: expected medoid_v0 to obb_centre_v1")
        if any(
            ref[field] != cand[field]
            for field in ("source_id", "calibration_id", "estimator_id", "param_hash")
        ):
            identity_ok = False
            identity_errors.append(
                f"{source}: capture/calibration/estimator identity changed"
            )

    manoeuvre = (
        json.loads(args.manoeuvre_report.read_text()) if args.manoeuvre_report else None
    )
    quality = (
        json.loads(args.quality_report.read_text()) if args.quality_report else None
    )
    report = {
        "schema_version": 2,
        "method": {
            "lateral_residual": "total-least-squares line fitted per moving online track",
            "excursion": f"any fitted-line lateral residual above {EXCURSION_METRES:.1f} m",
            "excluded": "tracks with fewer than five online estimates or no speed above 0.5 m/s",
        },
        "thresholds": {
            "p99_lateral_reduction_min": 0.50,
            "line_excursion_rate_max": 0.04,
            "detection_regression_max": 0.05,
            "fragmentation_regression_max": 0.05,
            "manoeuvre_preservation_min": 0.90,
        },
        "holdout_sources": sorted(holdout),
        "identity_gate": identity_ok,
        "identity_errors": identity_errors,
        "geometry_gate": geometry_ok,
        "manoeuvre_gate": valid_manoeuvre(manoeuvre),
        "quality_gate": valid_quality(quality),
        "manoeuvre_report": manoeuvre,
        "quality_report": quality,
        "reference_identities": reference_identities,
        "candidate_identities": candidate_identities,
        "sources": rows,
    }
    report["status"] = (
        "passed"
        if all(
            (
                identity_ok,
                geometry_ok,
                valid_manoeuvre(manoeuvre),
                valid_quality(quality),
            )
        )
        else "evidence_incomplete_or_failed"
    )
    args.output.write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps({"status": report["status"], "sources": len(rows)}, indent=2))


if __name__ == "__main__":
    main()
