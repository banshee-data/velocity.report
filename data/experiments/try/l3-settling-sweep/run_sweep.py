#!/usr/bin/env python3
"""Offline, ground-truth-free broad sweep of the L3 background-settling keys
named in ../l3-background-settling-sweep.md, across every site in the 24-site
S2 corpus (see docs/lidar/operations/state-estimation-phase01-corpus-baseline.md).

Runs `settling-eval` (internal/cmd/lidar/settling.go, the current replacement
for the removed `pcap-analyse` tool this experiment doc originally named) once
per (site, config) combination. settling-eval replays a PCAP through a
standalone BackgroundManager and reports convergence metrics computed from the
background grid's own measured state (coverage, spread stability, confidence)
-- no live server, no ground-truth track labels, no downstream tracker output.
That makes it a non-circular signal for these specific keys: unlike the L5
alignment-metric pass (see ../l5-tracking-noise-parameter-sweep.md), nothing
here is scored against the thing being tuned.

Resumable: re-running skips any (site, key, value) combination already present
in the output CSV. Safe to kill and restart at any point.

Usage:
    python3 run_sweep.py --settling-eval /tmp/settling-eval [--limit-sites N]

Outputs:
    results.csv   -- one compact row per run (committed to git)
    summary.json  -- rolling aggregate, rewritten every 10 rows (committed)
    raw/*.json    -- full settling-eval report per run (local only, gitignored;
                     cheap and deterministic to regenerate from the row's
                     git_sha + config + pcap_sha256, so not preserved in git)
"""

import argparse
import copy
import csv
import json
import subprocess
import sys
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from param_types import coerce  # noqa: E402
from row_appender import RowAppender  # noqa: E402

REPO_ROOT = Path(__file__).resolve().parents[4]
DEFAULT_TUNING = REPO_ROOT / "config" / "tuning.defaults.json"
MANIFESTS = [
    Path(
        "/Volumes/lidar/lidar/manifests/state-estimation-phase01-24site-20260917.source-pcaps.json"
    ),
    Path(
        "/Volumes/lidar/lidar/manifests/state-estimation-phase01-rebased-20260913.source-pcaps.json"
    ),
]

# The four keys this experiment names (../l3-background-settling-sweep.md),
# with the doc's own sweep ranges. Values are the non-default points; the
# default is run once per site as a shared baseline row (key="_baseline").
SWEEP = {
    "closeness_multiplier": [1.5, 2.25, 4.0, 5.0],
    "safety_margin_metres": [0.05, 0.10, 0.22, 0.30],
    "noise_relative": [0.005, 0.01, 0.035, 0.05],
    "neighbour_confirmation_count": [1, 2, 4, 5],
}

CSV_FIELDS = [
    "timestamp",
    "git_sha",
    "site_id",
    "param_key",
    "param_value",
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


def git_sha():
    return subprocess.run(
        ["git", "rev-parse", "HEAD"],
        cwd=REPO_ROOT,
        capture_output=True,
        text=True,
        check=True,
    ).stdout.strip()


def load_sites(ordinal=0):
    """Merge every manifest into {site_id: capture info} for one ordinal.

    ordinal=0 is the broad-sweep default (Batch 1). A replicate pass can pass
    ordinal=1 to check a finding isn't an artifact of one specific window;
    sites without a second capture segment are silently omitted for that
    ordinal (recorded as fewer rows, not an error).
    """
    sites = {}
    for path in MANIFESTS:
        if not path.exists():
            continue
        data = json.loads(path.read_text())
        for case in data["cases"]:
            cap = next((c for c in case["captures"] if c["ordinal"] == ordinal), None)
            if cap is None:
                continue
            sites.setdefault(
                case["id"],
                {
                    "relative_path": cap["relative_path"],
                    "sha256": cap["sha256"].removeprefix("sha256:"),
                    "ordinal": cap["ordinal"],
                },
            )
    return sites


def make_config(base, key, value, out_path):
    cfg = copy.deepcopy(base)
    cfg["l3"]["ema_baseline_v1"][key] = coerce(key, value)
    out_path.write_text(json.dumps(cfg, indent=2) + "\n")
    return out_path


def load_done(csv_path):
    """(site, key, value, capture_ordinal) already present in results.csv.

    capture_ordinal is part of the key so a replicate pass on a second
    capture segment (ordinal=1) is never mistaken for a duplicate of the
    ordinal=0 broad-sweep row with the same site/key/value.
    """
    done = set()
    if csv_path.exists():
        with csv_path.open() as f:
            for row in csv.DictReader(f):
                done.add(
                    (
                        row["site_id"],
                        row["param_key"],
                        row["param_value"],
                        row.get("capture_ordinal", "0"),
                    )
                )
    return done


def run_one(
    settling_eval_bin,
    pcap_root,
    pcap_relative_path,
    config_path,
    duration_seconds,
    raw_out_path,
):
    pcap_path = Path(pcap_root) / pcap_relative_path
    t0 = time.monotonic()
    proc = subprocess.run(
        [
            str(settling_eval_bin),
            "-duration-seconds",
            str(duration_seconds),
            "-config",
            str(config_path),
            "-sensor",
            "l3-settling-sweep",
            "-output",
            str(raw_out_path),
            str(pcap_path),
        ],
        capture_output=True,
        text=True,
        timeout=600,
    )
    wall = time.monotonic() - t0
    if proc.returncode != 0:
        return (
            None,
            wall,
            (proc.stderr.strip() or proc.stdout.strip() or f"exit {proc.returncode}"),
        )
    report = json.loads(raw_out_path.read_text())
    return report, wall, ""


def row_from_report(
    site_id, key, value, cap_info, duration_seconds, report, wall, raw_path, error
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
        "param_key": key,
        "param_value": value,
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


def write_summary(summary_path, rows):
    by_key = {}
    errors = 0
    for r in rows:
        if r["error"]:
            errors += 1
            continue
        key = r["param_key"]
        by_key.setdefault(key, []).append(r)
    aggregate = {}
    for key, key_rows in by_key.items():
        frames = [
            int(r["recommended_settling_frame"])
            for r in key_rows
            if r["recommended_settling_frame"] != ""
            and int(r["recommended_settling_frame"]) >= 0
        ]
        not_converged = sum(
            1
            for r in key_rows
            if r["recommended_settling_frame"] == ""
            or int(r["recommended_settling_frame"]) < 0
        )
        aggregate[key] = {
            "n_runs": len(key_rows),
            "n_not_converged": not_converged,
            "mean_settling_frame": (
                round(sum(frames) / len(frames), 1) if frames else None
            ),
            "min_settling_frame": min(frames) if frames else None,
            "max_settling_frame": max(frames) if frames else None,
        }
    summary_path.write_text(
        json.dumps(
            {
                "updated_at": time.strftime("%Y-%m-%dT%H:%M:%S%z"),
                "total_rows": len(rows),
                "errors": errors,
                "by_param_key": aggregate,
            },
            indent=2,
        )
        + "\n"
    )


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument(
        "--settling-eval", required=True, help="path to built settling-eval binary"
    )
    ap.add_argument("--pcap-root", default="/Volumes/lidar/lidar")
    ap.add_argument("--duration-seconds", type=float, default=120)
    ap.add_argument("--limit-sites", type=int, default=0, help="0 = all sites")
    ap.add_argument("--out-dir", default=str(Path(__file__).parent))
    ap.add_argument(
        "--sweep-json",
        default="",
        help="JSON file of {key: [values...]} to sweep, overriding the built-in "
        "SWEEP dict. Used by follow-up batches (narrowed range, interaction grid) "
        "so the broad Batch-1 sweep and its follow-ups share this one driver.",
    )
    ap.add_argument(
        "--capture-ordinal",
        type=int,
        default=0,
        help="which capture segment to replay per site (1 = replicate check on a "
        "second, independent window of the same site)",
    )
    ap.add_argument(
        "--sites",
        default="",
        help="comma-separated site IDs to restrict to (default: all sites with "
        "a capture at --capture-ordinal)",
    )
    ap.add_argument(
        "--skip-baseline",
        action="store_true",
        help="don't add a _baseline=default row per site (already covered elsewhere)",
    )
    args = ap.parse_args()

    out_dir = Path(args.out_dir)
    raw_dir = out_dir / "raw"
    raw_dir.mkdir(parents=True, exist_ok=True)
    csv_path = out_dir / "results.csv"
    summary_path = out_dir / "summary.json"
    config_scratch = out_dir / "configs"
    config_scratch.mkdir(exist_ok=True)

    base_config = json.loads(DEFAULT_TUNING.read_text())
    sweep_spec = SWEEP
    if args.sweep_json:
        sweep_spec = json.loads(Path(args.sweep_json).read_text())

    sites = load_sites(ordinal=args.capture_ordinal)
    site_ids = sorted(sites.keys())
    if args.sites:
        wanted = set(args.sites.split(","))
        site_ids = [s for s in site_ids if s in wanted]
    if args.limit_sites:
        site_ids = site_ids[: args.limit_sites]

    done = load_done(csv_path)

    plan = []
    for site_id in site_ids:
        if not args.skip_baseline:
            plan.append((site_id, "_baseline", "default"))
        for key, values in sweep_spec.items():
            for value in values:
                plan.append((site_id, key, value))

    print(
        f"{len(sites)} sites loaded; {len(plan)} planned runs; {len(done)} already done",
        file=sys.stderr,
    )

    with RowAppender(csv_path, CSV_FIELDS) as writer:
        writer.writeheader()

        completed_rows = []
        if csv_path.exists():
            with csv_path.open() as rf:
                completed_rows = list(csv.DictReader(rf))

        n_run = 0
        for site_id, key, value in plan:
            value_str = str(value)
            if (site_id, key, value_str, str(args.capture_ordinal)) in done:
                continue

            cap_info = sites[site_id]
            run_tag = f"{site_id}__{key}__{value}__o{args.capture_ordinal}"
            if key == "_baseline":
                cfg_path = DEFAULT_TUNING
            else:
                cfg_path = make_config(
                    base_config,
                    key,
                    value,
                    config_scratch / f"{run_tag}.json",
                )

            raw_out = raw_dir / f"{run_tag}.json"
            print(
                f"[{n_run + 1}/{len(plan) - len(done)}] {site_id} {key}={value} ...",
                file=sys.stderr,
                flush=True,
            )

            try:
                report, wall, error = run_one(
                    Path(args.settling_eval),
                    args.pcap_root,
                    cap_info["relative_path"],
                    cfg_path,
                    args.duration_seconds,
                    raw_out,
                )
            except subprocess.TimeoutExpired:
                report, wall, error = None, 600.0, "timeout after 600s"
            except Exception as e:  # keep the batch alive; record and move on
                report, wall, error = None, 0.0, f"driver exception: {e}"

            row = row_from_report(
                site_id,
                key,
                value_str,
                cap_info,
                args.duration_seconds,
                report,
                wall,
                raw_out,
                error,
            )
            writer.writerow(row)
            completed_rows.append(row)
            n_run += 1

            if error:
                print(f"    ERROR: {error}", file=sys.stderr)

            if n_run % 10 == 0:
                write_summary(summary_path, completed_rows)

        write_summary(summary_path, completed_rows)

    print(f"done. {n_run} runs this invocation. results: {csv_path}", file=sys.stderr)


if __name__ == "__main__":
    main()
