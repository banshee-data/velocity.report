#!/usr/bin/env python3
"""Build the tools, run the pass-7 scorecard sweep, then analyse it.

One command, so the batch can be started after the macOS app work without
re-deriving any of it:

    cd <repo>
    nohup python3 data/experiments/try/option-scorecard/launch_pass7.py \
        > /Volumes/Dolphin2/velocity-campaign/pass7-launcher.log 2>&1 &

Both Go tools are rebuilt here, from whatever checkout this file is in, and
every results row records the git SHA and the binary digests they were built
at. `-tags=pcap` is required: without it the replay tool exits on the first
capture.

Serial by default, and that is a measured choice rather than caution. On
2026-09-19 the same three runs took 169 s of wall time serially and 218 s at
three workers on this hardware: the two volumes are the bottleneck, not the
CPU. Determinism held either way (identical scorecard digests), so --workers
stays available for a machine with different storage.

The sweep is resumable: rerunning skips any (site, config) already recorded
without an error, so an interrupted batch continues with the same command.
"""

import argparse
import subprocess
import sys
import time
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO_ROOT = HERE.parents[3]
BIN_DIR = REPO_ROOT / "data" / "experiments" / "try" / "campaign" / "bin"

TOOLS = {
    "lidar-state-estimation-baseline": "./cmd/tools/lidar-state-estimation-baseline/",
    "lidar-track-scorecard": "./cmd/tools/lidar-track-scorecard/",
}


def log(msg):
    print(f"{time.strftime('%Y-%m-%dT%H:%M:%S%z')} {msg}", flush=True)


def run(cmd, **kwargs):
    log("+ " + " ".join(str(c) for c in cmd))
    kwargs.setdefault("check", False)
    return subprocess.run([str(c) for c in cmd], cwd=REPO_ROOT, **kwargs)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out-root", default="/Volumes/Dolphin2/velocity-campaign/pass7")
    ap.add_argument("--stage-dir", default="/tmp/velocity-pass7-stage")
    ap.add_argument("--configs-file", default=str(HERE / "pass7-configs.json"))
    ap.add_argument(
        "--sites",
        default="",
        help="comma-separated case ids; empty uses every case in the first-segment corpus",
    )
    ap.add_argument("--duration", type=float, default=200)
    ap.add_argument("--warmup", type=float, default=70)
    ap.add_argument("--workers", type=int, default=1)
    ap.add_argument("--budget-hours", type=float, default=9)
    ap.add_argument("--skip-build", action="store_true")
    args = ap.parse_args()

    if not args.skip_build:
        BIN_DIR.mkdir(parents=True, exist_ok=True)
        for name, pkg in sorted(TOOLS.items()):
            if run(["go", "build", "-tags=pcap", "-o", BIN_DIR / name, pkg]).returncode:
                log(f"FAILED to build {name}")
                return 1
        # The options this batch measures are all default-off, so a broken
        # default would be invisible in the results. Test before replaying.
        if run(
            [
                "go",
                "test",
                "./internal/lidar/l5tracks/",
                "./internal/lidar/l3grid/",
                "./internal/lidar/l8analytics/",
                "./internal/lidar/replayeval/",
                "./cmd/tools/lidar-track-scorecard/",
            ]
        ).returncode:
            log("FAILED tests; not replaying")
            return 1

    sites = args.sites
    if not sites:
        import json

        corpus = json.loads(
            (HERE.parent / "l5-nis-sweep" / "first-segment-corpus.json").read_text()
        )
        sites = ",".join(c["id"] for c in corpus["cases"])

    sweep = run(
        [
            "python3",
            HERE / "run_scorecard_sweep.py",
            "--baseline-bin",
            BIN_DIR / "lidar-state-estimation-baseline",
            "--scorecard-bin",
            BIN_DIR / "lidar-track-scorecard",
            "--sites",
            sites,
            "--configs-file",
            args.configs_file,
            "--out-root",
            args.out_root,
            "--stage-dir",
            args.stage_dir,
            "--duration",
            args.duration,
            "--warmup",
            args.warmup,
            "--workers",
            args.workers,
            "--budget-hours",
            args.budget_hours,
        ]
    )
    log(f"sweep exit {sweep.returncode}")

    # Analyse whatever finished: a batch stopped by the budget or the disk
    # still has every completed run in results.csv.
    analysis = run(
        [
            "python3",
            HERE / "analyze_scorecard_sweep.py",
            "--results",
            Path(args.out_root) / "results.csv",
            "--out",
            Path(args.out_root) / "scorecard-analysis.json",
        ]
    )
    log(f"analysis exit {analysis.returncode} (2 means a determinism check failed)")
    return sweep.returncode or analysis.returncode


if __name__ == "__main__":
    sys.exit(main())
