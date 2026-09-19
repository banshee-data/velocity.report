#!/usr/bin/env python3
"""Multi-key L3 interaction grid: varies 2-4 keys jointly per run, unlike
run_sweep.py (one key at a time). Only meaningful once per-key sweeps have
identified which keys are actually sensitive -- see
data/experiments/try/multi-key-interaction-grid.md and
docs/plans/lidar-parameter-experiment-campaign-2026-09.md Batch 5.

Reuses run_sweep.py's git_sha/load_sites/run_one rather than duplicating
them; only the "vary several keys per config, several levels per key"
combinatorics and the resulting (differently shaped) result rows are new.

Usage:
    python3 run_interaction_grid.py --settling-eval /tmp/settling-eval \
        --levels-json levels.json   # {"key1": [v_lo, v_default, v_hi], "key2": [...]}

Outputs (separate from results.csv/summary.json -- rows here vary more than
one key at once, which doesn't fit the single param_key/param_value schema):
    interaction-results.csv
    interaction-summary.json
"""

import argparse
import copy
import csv
import itertools
import json
import sys
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from param_types import coerce  # noqa: E402
from row_appender import RowAppender  # noqa: E402
from run_sweep import DEFAULT_TUNING, git_sha, load_sites, run_one  # noqa: E402

CSV_FIELDS = [
    "timestamp",
    "git_sha",
    "site_id",
    "combo",
    "keys_json",
    "capture_relative_path",
    "capture_sha256",
    "capture_ordinal",
    "duration_seconds_requested",
    "total_frames",
    "recommended_settling_frame",
    "recommended_settling_duration",
    "converged",
    "final_coverage_rate",
    "final_spread_delta_rate",
    "final_region_stability",
    "final_mean_confidence",
    "wall_duration_seconds",
    "raw_report_path",
    "error",
]


def make_multi_config(base, overrides, out_path):
    cfg = copy.deepcopy(base)
    for key, value in overrides.items():
        cfg["l3"]["ema_baseline_v1"][key] = coerce(key, value)
    out_path.write_text(json.dumps(cfg, indent=2) + "\n")
    return out_path


def combo_id(overrides):
    return "|".join(f"{k}={v}" for k, v in sorted(overrides.items()))


def load_done(csv_path):
    done = set()
    if csv_path.exists():
        with csv_path.open() as f:
            for row in csv.DictReader(f):
                done.add(
                    (row["site_id"], row["combo"], row.get("capture_ordinal", "0"))
                )
    return done


def row_from_report(
    site_id, overrides, cap_info, duration_seconds, report, wall, raw_path, error
):
    last = (
        report["metrics_history"][-1]
        if report and report.get("metrics_history")
        else {}
    )
    return {
        "timestamp": time.strftime("%Y-%m-%dT%H:%M:%S%z"),
        "git_sha": git_sha(),
        "site_id": site_id,
        "combo": combo_id(overrides),
        "keys_json": json.dumps(overrides, sort_keys=True),
        "capture_relative_path": cap_info["relative_path"],
        "capture_sha256": cap_info["sha256"],
        "capture_ordinal": cap_info["ordinal"],
        "duration_seconds_requested": duration_seconds,
        "total_frames": report.get("total_frames", "") if report else "",
        "recommended_settling_frame": (
            report.get("recommended_settling_frame", "") if report else ""
        ),
        "recommended_settling_duration": (
            report.get("recommended_settling_duration", "") if report else ""
        ),
        "converged": (
            (report.get("recommended_settling_frame", -1) >= 0) if report else ""
        ),
        "final_coverage_rate": last.get("coverage_rate", ""),
        "final_spread_delta_rate": last.get("spread_delta_rate", ""),
        "final_region_stability": last.get("region_stability", ""),
        "final_mean_confidence": last.get("mean_confidence", ""),
        "wall_duration_seconds": round(wall, 2),
        "raw_report_path": str(raw_path) if report else "",
        "error": error,
    }


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--settling-eval", required=True)
    ap.add_argument("--pcap-root", default="/Volumes/lidar/lidar")
    ap.add_argument("--duration-seconds", type=float, default=120)
    ap.add_argument(
        "--levels-json", required=True, help='{"key": [v1,v2,v3], ...}, 2-4 keys'
    )
    ap.add_argument("--capture-ordinal", type=int, default=0)
    ap.add_argument("--limit-sites", type=int, default=0, help="0 = all sites")
    ap.add_argument("--out-dir", default=str(Path(__file__).parent))
    args = ap.parse_args()

    levels = json.loads(Path(args.levels_json).read_text())
    if not (2 <= len(levels) <= 4):
        print(
            f"error: expected 2-4 keys in --levels-json, got {len(levels)}",
            file=sys.stderr,
        )
        sys.exit(2)

    out_dir = Path(args.out_dir)
    raw_dir = out_dir / "raw"
    raw_dir.mkdir(parents=True, exist_ok=True)
    csv_path = out_dir / "interaction-results.csv"
    config_scratch = out_dir / "configs"
    config_scratch.mkdir(exist_ok=True)

    base_config = json.loads(DEFAULT_TUNING.read_text())
    sites = load_sites(ordinal=args.capture_ordinal)
    site_ids = sorted(sites.keys())
    if args.limit_sites:
        site_ids = site_ids[: args.limit_sites]

    keys = sorted(levels.keys())
    combos = [
        dict(zip(keys, values))
        for values in itertools.product(*(levels[k] for k in keys))
    ]

    done = load_done(csv_path)

    plan = [(site_id, overrides) for site_id in site_ids for overrides in combos]
    print(
        f"{len(sites)} sites, {len(combos)} combos over keys {keys}; {len(plan)} planned runs; {len(done)} already done",
        file=sys.stderr,
    )

    with RowAppender(csv_path, CSV_FIELDS) as writer:
        writer.writeheader()

        n_run = 0
        for site_id, overrides in plan:
            cid = combo_id(overrides)
            if (site_id, cid, str(args.capture_ordinal)) in done:
                continue

            cap_info = sites[site_id]
            tag = f"{site_id}__{cid.replace('|', '_').replace('=', '-')}__o{args.capture_ordinal}"
            cfg_path = make_multi_config(
                base_config, overrides, config_scratch / f"{tag}.json"
            )
            raw_out = raw_dir / f"{tag}.json"

            print(f"[{n_run + 1}] {site_id} {cid} ...", file=sys.stderr, flush=True)
            try:
                report, wall, error = run_one(
                    Path(args.settling_eval),
                    args.pcap_root,
                    cap_info["relative_path"],
                    cfg_path,
                    args.duration_seconds,
                    raw_out,
                )
            except Exception as e:
                report, wall, error = None, 0.0, f"driver exception: {e}"

            row = row_from_report(
                site_id,
                overrides,
                cap_info,
                args.duration_seconds,
                report,
                wall,
                raw_out,
                error,
            )
            writer.writerow(row)
            n_run += 1
            if error:
                print(f"    ERROR: {error}", file=sys.stderr)

    print(f"done. {n_run} runs this invocation. results: {csv_path}", file=sys.stderr)


if __name__ == "__main__":
    main()
