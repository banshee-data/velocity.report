#!/usr/bin/env python3
"""L5 tracking-noise sweep scored by the real GroundTruthEvaluator (via the
new cmd/tools/lidar-ground-truth-eval CLI) instead of the circular
alignment-vs-own-smoothing metric the preliminary pass used -- see
data/experiments/try/l5-tracking-noise-parameter-sweep.md and
docs/plans/lidar-parameter-experiment-campaign-2026-09.md Batch 3.

Never touches the user's live server (default port 2369 / whatever process
already owns it). It launches its own throwaway server instance against a
fresh, isolated database, drives it exactly the way
data/explore/kirk0-lifecycle/sweep.sh drives the live one (apply L5 params ->
reset grid -> analysis-mode PCAP replay -> wait -> read last_run_id), and
scores each resulting candidate run against a pre-existing, already-labelled
reference run using lidar-ground-truth-eval, then shuts its own server down.

Known, confirmed precondition: this needs a *working* UDP bind for the
isolated server's live listener (internal/lidar/server/server.go Start()
returns before ever starting the monitor HTTP API if that bind fails -- this
was hit and diagnosed by hand before writing this script). The BPF filter
PCAP replay uses to find packets is the *same* configured UDP port
(internal/lidar/server/datasource_handlers.go passes ws.udpPort straight
through), with no per-request override, so the port can't simply be changed
to something free -- it must match what the reference PCAP was captured on.
If something else already owns that port (as of 2026-09-17/18, the operator's
own live dev server does, on 2369), this whole approach is blocked until
either that live server is stopped (not this script's call to make) or the
server gains a way to decouple the replay filter port from the live-listen
bind port. preflight_udp_port_free() checks this before doing anything else
and exits 3 (a distinct "blocked" code, not a failure) if it's not free.

Usage:
    python3 run_l5_gt_sweep.py \
        --velocity-bin /tmp/velocity-l5-isolated \
        --gt-eval-bin /tmp/lidar-ground-truth-eval \
        --reference-db /path/to/sensor_data.db \
        --reference-run-id 60a4774c-db3e-4008-9b7e-d1059ec27319 \
        --pcap-file kirk1.pcapng --pcap-root /Volumes/lidar/lidar
"""

import argparse
import json
import socket
import subprocess
import sys
import time
import urllib.request
from pathlib import Path

# One shared row writer for every sweep driver (see its docstring for why).
sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "l3-settling-sweep"))
from row_appender import RowAppender  # noqa: E402

CSV_FIELDS = [
    "timestamp",
    "git_sha",
    "process_noise_pos",
    "process_noise_vel",
    "measurement_noise",
    "candidate_run_id",
    "reference_run_id",
    "duration_seconds_requested",
    "detection_rate",
    "false_positive_rate",
    "composite_score",
    "matched_count",
    "reference_count",
    "candidate_count",
    "wall_duration_seconds",
    "error",
]

# From l5-tracking-noise-parameter-sweep.md's own stated sweep ranges.
LEVELS = {
    "process_noise_pos": [0.01, 0.05, 0.5],
    "process_noise_vel": [0.05, 0.2, 2.0],
    "measurement_noise": [0.01, 0.05, 0.25],
}


def preflight_udp_port_free(port):
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    try:
        s.bind(("0.0.0.0", port))
        s.close()
        return True
    except OSError:
        return False


def http_post(url, payload, timeout=30):
    data = json.dumps(payload).encode()
    req = urllib.request.Request(
        url, data=data, headers={"Content-Type": "application/json"}, method="POST"
    )
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        return json.loads(resp.read())


def http_get(url, timeout=30):
    with urllib.request.urlopen(url, timeout=timeout) as resp:
        return json.loads(resp.read())


def git_sha(repo_root):
    return subprocess.run(
        ["git", "rev-parse", "HEAD"],
        cwd=repo_root,
        capture_output=True,
        text=True,
        check=True,
    ).stdout.strip()


def wait_for_server(base_url, timeout=15):
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            http_get(
                f"{base_url}/api/lidar/data_source?sensor_id=hesai-pandar40p", timeout=3
            )
            return True
        except Exception:
            time.sleep(0.5)
    return False


def wait_for_replay_done(base_url, sensor="hesai-pandar40p", timeout=300):
    deadline = time.time() + timeout
    while time.time() < deadline:
        resp = http_get(
            f"{base_url}/api/lidar/data_source?sensor_id={sensor}", timeout=10
        )
        if not resp.get("pcap_in_progress", True):
            return resp
        time.sleep(2)
    raise TimeoutError(f"replay did not finish within {timeout}s")


def load_done(csv_path):
    import csv

    done = set()
    if csv_path.exists():
        with csv_path.open() as f:
            for row in csv.DictReader(f):
                if not row.get("error"):
                    done.add(
                        (
                            row["process_noise_pos"],
                            row["process_noise_vel"],
                            row["measurement_noise"],
                        )
                    )
    return done


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--velocity-bin", required=True)
    ap.add_argument("--gt-eval-bin", required=True)
    ap.add_argument("--reference-db", required=True)
    ap.add_argument("--reference-run-id", required=True)
    ap.add_argument("--pcap-file", default="kirk1.pcapng")
    ap.add_argument("--pcap-root", default="/Volumes/lidar/lidar")
    ap.add_argument("--udp-port", type=int, default=2369)
    ap.add_argument("--server-port", type=int, default=18080)
    ap.add_argument("--monitor-port", type=int, default=18081)
    ap.add_argument("--duration-seconds", type=int, default=60)
    ap.add_argument("--replay-timeout", type=int, default=300)
    ap.add_argument("--out-dir", default=str(Path(__file__).parent))
    args = ap.parse_args()

    out_dir = Path(args.out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)
    csv_path = out_dir / "results.csv"
    blocked_path = out_dir / "blocked.json"
    repo_root = Path(__file__).resolve().parents[4]

    if not preflight_udp_port_free(args.udp_port):
        blocked_path.write_text(
            json.dumps(
                {
                    "blocked_at": time.strftime("%Y-%m-%dT%H:%M:%S%z"),
                    "reason": (
                        f"UDP port {args.udp_port} is already bound by another process (the live dev server, "
                        "as of 2026-09-17/18). internal/lidar/server/server.go's Start() returns before starting "
                        "the monitor HTTP API if the live listener fails to bind, and the PCAP-replay BPF filter "
                        "uses the same configured port with no per-request override "
                        "(internal/lidar/server/datasource_handlers.go: ws.udpPort), so this isolated instance "
                        "cannot use a different port and still find kirk1.pcapng's packets. Not fixable from this "
                        "script without either stopping the live server (out of scope -- never do this "
                        "unattended) or a small code change to decouple the replay filter port from the live "
                        "listen port (e.g. auto-detect per capture the way settling-eval already does via "
                        "network.DetectUDPPort). Retry once one of those is true."
                    ),
                },
                indent=2,
            )
        )
        print(f"BLOCKED: {blocked_path}", file=sys.stderr)
        return 3

    server_db = out_dir / "isolated-server.db"
    log_path = out_dir / "isolated-server.log"
    base_url = f"http://127.0.0.1:{args.monitor_port}"

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
        cwd=repo_root,
    )
    try:
        if not wait_for_server(base_url, timeout=15):
            print(
                f"error: isolated server did not come up; see {log_path}",
                file=sys.stderr,
            )
            return 1

        done = load_done(csv_path)
        combos = [
            (pnp, pnv, mn)
            for pnp in LEVELS["process_noise_pos"]
            for pnv in LEVELS["process_noise_vel"]
            for mn in LEVELS["measurement_noise"]
        ]

        with RowAppender(csv_path, CSV_FIELDS) as writer:
            writer.writeheader()

            for pnp, pnv, mn in combos:
                key = (str(pnp), str(pnv), str(mn))
                if key in done:
                    continue

                t0 = time.time()
                row = {
                    "timestamp": time.strftime("%Y-%m-%dT%H:%M:%S%z"),
                    "git_sha": git_sha(repo_root),
                    "process_noise_pos": pnp,
                    "process_noise_vel": pnv,
                    "measurement_noise": mn,
                    "reference_run_id": args.reference_run_id,
                    "duration_seconds_requested": args.duration_seconds,
                    "candidate_run_id": "",
                    "detection_rate": "",
                    "false_positive_rate": "",
                    "composite_score": "",
                    "matched_count": "",
                    "reference_count": "",
                    "candidate_count": "",
                    "wall_duration_seconds": "",
                    "error": "",
                }
                try:
                    http_post(
                        f"{base_url}/api/lidar/params?sensor_id=hesai-pandar40p",
                        {
                            "process_noise_pos": pnp,
                            "process_noise_vel": pnv,
                            "measurement_noise": mn,
                        },
                    )
                    start_resp = http_post(
                        f"{base_url}/api/lidar/pcap/start?sensor_id=hesai-pandar40p",
                        {
                            "pcap_file": args.pcap_file,
                            "analysis_mode": True,
                            "speed_mode": "analysis",
                            "duration_seconds": args.duration_seconds,
                        },
                    )
                    candidate_run_id = start_resp.get("last_run_id", "")
                    final = wait_for_replay_done(base_url, timeout=args.replay_timeout)
                    candidate_run_id = final.get("last_run_id") or candidate_run_id
                    if not candidate_run_id:
                        raise RuntimeError("no run_id recorded for this replay")
                    row["candidate_run_id"] = candidate_run_id

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
                    row.update(
                        {
                            "detection_rate": score["detection_rate"],
                            "false_positive_rate": score["false_positive_rate"],
                            "composite_score": score["composite_score"],
                            "matched_count": score["matched_count"],
                            "reference_count": score["reference_count"],
                            "candidate_count": score["candidate_count"],
                        }
                    )
                except Exception as e:
                    row["error"] = str(e)

                row["wall_duration_seconds"] = round(time.time() - t0, 2)
                writer.writerow(row)
                print(
                    f"{key}: {'error=' + row['error'] if row['error'] else 'composite_score=' + str(row['composite_score'])}",
                    file=sys.stderr,
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
