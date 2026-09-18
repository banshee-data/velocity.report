#!/usr/bin/env python3
"""One-at-a-time L4/L5 sweep scored by the real GroundTruthEvaluator, for keys
the metric can plausibly see.

Why this exists: the 27-combo L5 noise grid (run_l5_gt_sweep.py) showed the
ground-truth metric doesn't move with process/measurement noise (matched_count
10-11 of 49 across all 27 combos, main effects < 0.2 tracks). EvaluateGroundTruth
matches on *temporal IoU only* (SpatialDistance is unimplemented), and noise
parameters change state estimates, not when a track starts or ends. Keys that
change track existence -- clustering (eps, min points) and track lifecycle
(hits to confirm, max misses, gating) -- do change what the metric measures, so
this sweeps those, one key at a time around the shipped defaults.

Each run POSTs *every* swept key's default plus one override, so no swept key
carries state over from the previous run.

Server state matters and was found the hard way (2026-09-18 test on kirk0): the
first replay after server start is a cold outlier (baseline matched 3/16, then
7/16 on every later baseline, identical to the track), while every earlier L5
sweep here had the same cold first row. POST /api/lidar/grid_reset did not
change that, so it is not relied on. Instead the plan opens with a "_warmup"
run (recorded, excluded from analysis), and the baselines that follow at start,
middle and end double as a drift check: a warm baseline that moves after other
settings have run means state is leaking between runs and analyze_gt_oat.py
refuses to draw conclusions from that site. Every POST response is checked against the
requested values, and the live config is re-read after the replay finishes: a
run whose parameters were not actually applied (or were reset by the replay) is
recorded as an error rather than as a "no effect" result. Baseline (all
defaults) runs are placed at the start, middle and end, so both the metric's own
run-to-run noise and any drift across the sweep are measurable.

Reuses run_l5_gt_sweep.py's server/HTTP helpers. Same isolation guarantees:
its own throwaway server, its own db, exits 3 (blocked, not failed) if the UDP
port is taken.

Usage:
    python3 run_gt_oat_sweep.py --velocity-bin ... --gt-eval-bin ... \
        --reference-db sensor_data.db --reference-run-id <id> \
        --pcap-file kirk0.pcapng --duration-seconds 95 \
        --sweep-json '{"l5.cv_kf_v1.hits_to_confirm": [1, 2, 6, 8]}' --out-dir <dir>
"""

import argparse
import csv
import json
import subprocess
import sys
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from run_l5_gt_sweep import (  # noqa: E402
    git_sha,
    http_get,
    http_post,
    preflight_udp_port_free,
    wait_for_replay_done,
    wait_for_server,
)

REPO_ROOT = Path(__file__).resolve().parents[4]
TUNING_DEFAULTS = REPO_ROOT / "config" / "tuning.defaults.json"

CSV_FIELDS = [
    "timestamp",
    "git_sha",
    "param_path",
    "param_value",
    "repeat",
    "candidate_run_id",
    "reference_run_id",
    "duration_seconds_requested",
    "detection_rate",
    "fragmentation",
    "false_positive_rate",
    "composite_score",
    "matched_count",
    "reference_count",
    "candidate_count",
    "wall_duration_seconds",
    "error",
]


def get_path(obj, dotted):
    for part in dotted.split("."):
        obj = obj[part]
    return obj


def default_for(path):
    return get_path(json.loads(TUNING_DEFAULTS.read_text()), path)


def mismatches(config, expected):
    bad = []
    for path, want in expected.items():
        try:
            got = get_path(config, path)
        except (KeyError, TypeError):
            bad.append(f"{path}: missing from live config")
            continue
        if abs(float(got) - float(want)) > 1e-6:
            bad.append(f"{path}: requested {want}, live config has {got}")
    return bad


def build_plan(sweep):
    runs = [(p, v) for p, values in sweep.items() for v in values]
    mid = len(runs) // 2
    return (
        [("_warmup", "default", 0), ("_baseline", "default", 0)]
        + [(p, v, 0) for p, v in runs[:mid]]
        + [("_baseline", "default", 1)]
        + [(p, v, 0) for p, v in runs[mid:]]
        + [("_baseline", "default", 2)]
    )


def load_done(csv_path):
    done = set()
    if csv_path.exists():
        with csv_path.open() as f:
            for row in csv.DictReader(f):
                if not row.get("error"):
                    done.add((row["param_path"], row["param_value"], row["repeat"]))
    return done


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--velocity-bin", required=True)
    ap.add_argument("--gt-eval-bin", required=True)
    ap.add_argument("--reference-db", required=True)
    ap.add_argument("--reference-run-id", required=True)
    ap.add_argument("--pcap-file", required=True)
    ap.add_argument("--pcap-root", default="/Volumes/lidar/lidar")
    ap.add_argument("--udp-port", type=int, default=2369)
    ap.add_argument("--server-port", type=int, default=18080)
    ap.add_argument("--monitor-port", type=int, default=18081)
    ap.add_argument("--duration-seconds", type=int, required=True)
    ap.add_argument("--replay-timeout", type=int, default=600)
    ap.add_argument("--sweep-json", required=True)
    ap.add_argument("--out-dir", required=True)
    args = ap.parse_args()

    sweep = json.loads(args.sweep_json)
    out_dir = Path(args.out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)
    csv_path = out_dir / "results.csv"
    blocked_path = out_dir / "blocked.json"

    if not preflight_udp_port_free(args.udp_port):
        blocked_path.write_text(
            json.dumps(
                {
                    "blocked_at": time.strftime("%Y-%m-%dT%H:%M:%S%z"),
                    "reason": (
                        f"UDP port {args.udp_port} is already bound by another process; the isolated "
                        "server cannot start its live listener and the PCAP replay filter reuses the "
                        "same port (see run_l5_gt_sweep.py's docstring). Not a failure: retry once the "
                        "port is free. Never stop another process to force this through."
                    ),
                },
                indent=2,
            )
            + "\n"
        )
        print(f"BLOCKED: {blocked_path}", file=sys.stderr)
        return 3

    defaults = {p: default_for(p) for p in sweep}
    server_db = out_dir / "isolated-server.db"
    log_path = out_dir / "isolated-server.log"
    base_url = f"http://127.0.0.1:{args.monitor_port}"
    params_url = f"{base_url}/api/lidar/params?sensor_id=hesai-pandar40p"

    proc = subprocess.Popen(
        [
            args.velocity_bin,
            "serve",
            "--disable-radar",
            f"--listen=:{args.server_port}",
            "--enable-transit-worker=false",
            "--enable-lidar",
            f"--lidar-listen=127.0.0.1:{args.monitor_port}",
            "--log-level=diag",
            f"--lidar-pcap-dir={args.pcap_root}",
            f"--db-path={server_db}",
        ],
        stdout=open(log_path, "w"),
        stderr=subprocess.STDOUT,
        cwd=REPO_ROOT,
    )
    try:
        if not wait_for_server(base_url, timeout=15):
            print(
                f"error: isolated server did not come up; see {log_path}",
                file=sys.stderr,
            )
            return 1

        done = load_done(csv_path)
        plan = build_plan(sweep)
        print(f"{len(plan)} planned runs; {len(done)} already done", file=sys.stderr)

        write_header = not csv_path.exists()
        with csv_path.open("a", newline="") as f:
            writer = csv.DictWriter(f, fieldnames=CSV_FIELDS)
            if write_header:
                writer.writeheader()

            for path, value, repeat in plan:
                if (path, str(value), str(repeat)) in done:
                    continue
                t0 = time.time()
                row = {k: "" for k in CSV_FIELDS}
                row.update(
                    {
                        "timestamp": time.strftime("%Y-%m-%dT%H:%M:%S%z"),
                        "git_sha": git_sha(REPO_ROOT),
                        "param_path": path,
                        "param_value": value,
                        "repeat": repeat,
                        "reference_run_id": args.reference_run_id,
                        "duration_seconds_requested": args.duration_seconds,
                    }
                )
                requested = dict(defaults)
                if path not in ("_baseline", "_warmup"):
                    requested[path] = value
                try:
                    resp = http_post(params_url, requested)
                    bad = mismatches(resp, requested)
                    if bad:
                        raise RuntimeError("params not applied: " + "; ".join(bad))
                    start_resp = http_post(
                        f"{base_url}/api/lidar/pcap/start?sensor_id=hesai-pandar40p",
                        {
                            "pcap_file": args.pcap_file,
                            "analysis_mode": True,
                            "speed_mode": "analysis",
                            "duration_seconds": args.duration_seconds,
                        },
                    )
                    final = wait_for_replay_done(base_url, timeout=args.replay_timeout)
                    candidate_run_id = final.get("last_run_id") or start_resp.get(
                        "last_run_id", ""
                    )
                    if not candidate_run_id:
                        raise RuntimeError("no run_id recorded for this replay")
                    row["candidate_run_id"] = candidate_run_id
                    bad = mismatches(http_get(params_url), requested)
                    if bad:
                        raise RuntimeError(
                            "params changed during replay: " + "; ".join(bad)
                        )

                    score_proc = subprocess.run(
                        [
                            args.gt_eval_bin,
                            "-reference-db",
                            args.reference_db,
                            "-reference-run-id",
                            args.reference_run_id,
                            "-candidate-db",
                            str(server_db),
                            "-candidate-run-id",
                            candidate_run_id,
                        ],
                        capture_output=True,
                        text=True,
                        timeout=60,
                    )
                    if score_proc.returncode != 0:
                        raise RuntimeError(
                            f"lidar-ground-truth-eval exit {score_proc.returncode}: {score_proc.stderr.strip()}"
                        )
                    score = json.loads(score_proc.stdout)
                    for k in (
                        "detection_rate",
                        "fragmentation",
                        "false_positive_rate",
                        "composite_score",
                        "matched_count",
                        "reference_count",
                        "candidate_count",
                    ):
                        row[k] = score[k]
                except Exception as e:
                    row["error"] = str(e)

                row["wall_duration_seconds"] = round(time.time() - t0, 2)
                writer.writerow(row)
                f.flush()
                print(
                    f"{path}={value} r{repeat}: "
                    + (
                        f"error={row['error']}"
                        if row["error"]
                        else f"matched={row['matched_count']}/{row['reference_count']} cand={row['candidate_count']}"
                    ),
                    file=sys.stderr,
                    flush=True,
                )
    finally:
        proc.terminate()
        try:
            proc.wait(timeout=10)
        except subprocess.TimeoutExpired:
            proc.kill()
    return 0


if __name__ == "__main__":
    sys.exit(main())
