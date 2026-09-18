#!/usr/bin/env python3
"""Determinism check: reruns the same L3 config N times per site to measure
settling-eval's own run-to-run noise floor.

Why this exists: the 2026-09-18 interaction-grid rerun disagreed with Batch 1
at 3 of 48 like-for-like comparisons (baseline settling frame 601 vs 693,
550 vs 565, and one site flipping from "did not converge" to 505) on
identical captures and identical code. That means per-site frame deltas at
slow-settling sites have a noise floor of ~100 frames, which the 10-frame
sensitivity threshold in analyze_sensitivity.py can't distinguish from a real
effect. run_sweep.py can't answer this itself: its dedup key is
(site, key, value, ordinal), so an identical config is skipped as "already
done". This script adds a repeat index to the key instead.

Reuses run_sweep.py's load_sites/run_one/git_sha; only the repeat bookkeeping
and result shape are new.

Usage:
    python3 run_repeat_check.py --settling-eval /path/to/settling-eval \
        --configs-json '[{"label": "baseline", "overrides": {}, "repeats": 5},
                         {"label": "nbr1", "overrides": {"neighbour_confirmation_count": 1}, "repeats": 3}]'

Output: repeat-results.csv (one row per site x label x repeat).
"""

import argparse
import copy
import csv
import json
import sys
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from param_types import coerce  # noqa: E402
from run_sweep import DEFAULT_TUNING, git_sha, load_sites, run_one  # noqa: E402

CSV_FIELDS = [
    "timestamp",
    "git_sha",
    "site_id",
    "label",
    "overrides_json",
    "repeat",
    "capture_relative_path",
    "capture_sha256",
    "capture_ordinal",
    "duration_seconds_requested",
    "total_frames",
    "recommended_settling_frame",
    "converged",
    "final_coverage_rate",
    "final_spread_delta_rate",
    "final_region_stability",
    "final_mean_confidence",
    "wall_duration_seconds",
    "error",
]


def load_done(csv_path):
    done = set()
    if csv_path.exists():
        with csv_path.open() as f:
            for row in csv.DictReader(f):
                done.add(
                    (
                        row["site_id"],
                        row["label"],
                        row["repeat"],
                        row.get("capture_ordinal", "0"),
                    )
                )
    return done


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--settling-eval", required=True)
    ap.add_argument("--pcap-root", default="/Volumes/lidar/lidar")
    ap.add_argument("--duration-seconds", type=float, default=120)
    ap.add_argument("--configs-json", required=True)
    ap.add_argument("--capture-ordinal", type=int, default=0)
    ap.add_argument("--limit-sites", type=int, default=0, help="0 = all sites")
    ap.add_argument("--out-dir", default=str(Path(__file__).parent))
    args = ap.parse_args()

    configs = json.loads(args.configs_json)
    out_dir = Path(args.out_dir)
    raw_dir = out_dir / "raw"
    config_scratch = out_dir / "configs"
    raw_dir.mkdir(parents=True, exist_ok=True)
    config_scratch.mkdir(exist_ok=True)
    csv_path = out_dir / "repeat-results.csv"

    base_config = json.loads(DEFAULT_TUNING.read_text())
    sites = load_sites(ordinal=args.capture_ordinal)
    site_ids = sorted(sites)
    if args.limit_sites:
        site_ids = site_ids[: args.limit_sites]

    max_repeats = max(c["repeats"] for c in configs)
    plan = [
        (site_id, c, r)
        for r in range(max_repeats)
        for site_id in site_ids
        for c in configs
        if r < c["repeats"]
    ]
    done = load_done(csv_path)
    write_header = not csv_path.exists()
    print(
        f"{len(site_ids)} sites; {len(plan)} planned runs; {len(done)} already done",
        file=sys.stderr,
    )

    with csv_path.open("a", newline="") as f:
        writer = csv.DictWriter(f, fieldnames=CSV_FIELDS)
        if write_header:
            writer.writeheader()
        n_run = 0
        for site_id, c, repeat in plan:
            label = c["label"]
            if (site_id, label, str(repeat), str(args.capture_ordinal)) in done:
                continue
            cap_info = sites[site_id]
            tag = f"{site_id}__repeat-{label}-r{repeat}__o{args.capture_ordinal}"
            cfg = copy.deepcopy(base_config)
            for k, v in c["overrides"].items():
                cfg["l3"]["ema_baseline_v1"][k] = coerce(k, v)
            cfg_path = config_scratch / f"{tag}.json"
            cfg_path.write_text(json.dumps(cfg, indent=2) + "\n")
            raw_out = raw_dir / f"{tag}.json"
            print(f"[{n_run + 1}] {tag} ...", file=sys.stderr, flush=True)
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
            last = (
                report["metrics_history"][-1]
                if report and report.get("metrics_history")
                else {}
            )
            writer.writerow(
                {
                    "timestamp": time.strftime("%Y-%m-%dT%H:%M:%S%z"),
                    "git_sha": git_sha(),
                    "site_id": site_id,
                    "label": label,
                    "overrides_json": json.dumps(c["overrides"], sort_keys=True),
                    "repeat": repeat,
                    "capture_relative_path": cap_info["relative_path"],
                    "capture_sha256": cap_info["sha256"],
                    "capture_ordinal": cap_info["ordinal"],
                    "duration_seconds_requested": args.duration_seconds,
                    "total_frames": report.get("total_frames", "") if report else "",
                    "recommended_settling_frame": (
                        report.get("recommended_settling_frame", "") if report else ""
                    ),
                    "converged": (
                        (report.get("recommended_settling_frame", -1) >= 0)
                        if report
                        else ""
                    ),
                    "final_coverage_rate": last.get("coverage_rate", ""),
                    "final_spread_delta_rate": last.get("spread_delta_rate", ""),
                    "final_region_stability": last.get("region_stability", ""),
                    "final_mean_confidence": last.get("mean_confidence", ""),
                    "wall_duration_seconds": round(wall, 2),
                    "error": error,
                }
            )
            f.flush()
            n_run += 1
            if error:
                print(f"    ERROR: {error}", file=sys.stderr)

    print(f"done. {n_run} runs this invocation. results: {csv_path}", file=sys.stderr)


if __name__ == "__main__":
    main()
