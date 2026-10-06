#!/usr/bin/env python3
"""Run a tuning sweep over kirk0 and score each config against the annotations.

The grid is a full factorial, but it is run in a shuffled order with a fixed
seed, so that a sweep cut short by the clock is still an unbiased sample of the
whole space rather than a corner of it. Reference configs run first.

Each run writes one JSONL row. The cluster dump behind it is scored and then
deleted, because keeping thousands of them would fill the disk and the row is
what the analysis reads.
"""

import json
import os
import random
import shutil
import subprocess
import sys
import time
import itertools
import hashlib
import concurrent.futures as cf

BENCH = sys.argv[1]
WORK = sys.argv[2]
REPO = sys.argv[3]
HOURS = float(sys.argv[4])
JOBS = int(sys.argv[5])

PCAP = os.path.join(REPO, "internal/lidar/perf/pcap/kirk0.pcapng")
DEFAULTS = os.path.join(REPO, "config/tuning.defaults.json")
GT = os.path.join(WORK, "ground-truth.json")
RESULTS = os.path.join(WORK, "results.jsonl")
SCRATCH = os.path.join(WORK, "scratch")
KEEP = os.path.join(WORK, "keep")
RUN_TIMEOUT = 600  # a config that cannot finish in ten minutes is not a candidate
MIN_FREE_GB = 5.0

L3 = "l3.ema_baseline_v1."
L4 = "l4.dbscan_xy_v1."

# The axes. L5 is absent on purpose: clusters are L4 output and the dump is
# written before tracking, so no L5 parameter can move this score.
AXES = {
    L3 + "closeness_multiplier": [1.5, 2.0, 2.5, 3.0],
    L3 + "safety_margin_metres": [0.05, 0.15],
    L3 + "neighbour_confirmation_count": [2, 3, 4],
    L3 + "noise_relative": [0.005, 0.01, 0.02],
    L3 + "background_update_fraction": [0.001, 0.005, 0.01, 0.02, 0.05],
    L3 + "post_settle_update_fraction": [0.0, 0.005],
    L4 + "foreground_dbscan_eps": [0.3, 0.4, 0.55, 0.7, 0.8],
    L4 + "foreground_min_cluster_points": [3, 5, 8],
}
# The ladders the local search steps along once the grid is done. They are
# finer than the grid and reach past its ends, so a best value sitting on an
# edge can still be walked outwards.
LADDERS = {
    L3
    + "closeness_multiplier": [
        1.0,
        1.25,
        1.5,
        1.75,
        2.0,
        2.25,
        2.5,
        2.75,
        3.0,
        3.25,
        3.5,
    ],
    L3 + "safety_margin_metres": [0.0, 0.025, 0.05, 0.1, 0.15, 0.2, 0.25],
    L3 + "neighbour_confirmation_count": [1, 2, 3, 4, 5, 6],
    L3 + "noise_relative": [0.0025, 0.005, 0.0075, 0.01, 0.015, 0.02, 0.03, 0.04],
    L3
    + "background_update_fraction": [
        0.0005,
        0.001,
        0.002,
        0.005,
        0.0075,
        0.01,
        0.015,
        0.02,
        0.035,
        0.05,
        0.08,
    ],
    L3 + "post_settle_update_fraction": [0.0, 0.001, 0.0025, 0.005, 0.01, 0.02],
    L4
    + "foreground_dbscan_eps": [
        0.25,
        0.3,
        0.35,
        0.4,
        0.475,
        0.55,
        0.625,
        0.7,
        0.75,
        0.8,
        0.9,
    ],
    L4 + "foreground_min_cluster_points": [2, 3, 4, 5, 6, 8, 10, 12],
}
# Held at the same value in every run, including the reference, so that a
# loosened foreground is never silently truncated and the runs stay comparable
# with each other. It is not the shipped default of 8000.
FIXED = {L4 + "foreground_max_input_points": 30000}


def run_id(params):
    body = json.dumps(params, sort_keys=True)
    return hashlib.sha256(body.encode()).hexdigest()[:12]


def combos():
    keys = list(AXES)
    out = [dict(zip(keys, vals)) for vals in itertools.product(*AXES.values())]
    random.Random(20260922).shuffle(out)
    return out


def references():
    """Configs whose results have to exist however far the sweep gets."""
    stock = {k: v for k, v in zip(AXES, [3.0, 0.15, 3, 0.02, 0.02, 0.0, 0.8, 5])}
    refs = [("stock", dict(stock))]
    loosest = {k: vs[0] for k, vs in AXES.items()}
    refs.append(("loosest", loosest))
    # One axis at a time off stock, so a flat axis is visible even if the
    # factorial never completes.
    for k, vals in AXES.items():
        for v in vals:
            if v == stock[k]:
                continue
            probe = dict(stock)
            probe[k] = v
            refs.append((f"probe:{k.split('.')[-1]}={v}", probe))
    return refs


def neighbours(params):
    """Every one-step move off a config, along the fine ladders."""
    out = []
    for key, val in params.items():
        ladder = LADDERS[key]
        try:
            i = ladder.index(val)
        except ValueError:
            # Off-ladder from an earlier round: step from the nearest rung.
            i = min(range(len(ladder)), key=lambda j: abs(ladder[j] - val))
        for j in (i - 1, i + 1):
            if 0 <= j < len(ladder) and ladder[j] != val:
                moved = dict(params)
                moved[key] = ladder[j]
                out.append(moved)
    return out


def best_so_far(path, top):
    """The top configs by F1, read back from what has been written."""
    rows = []
    with open(path) as f:
        for line in f:
            try:
                r = json.loads(line)
            except Exception:  # noqa: BLE001 - a torn last line
                continue
            if r.get("error") or "f1" not in r or "params" not in r:
                continue
            rows.append(r)
    rows.sort(key=lambda r: r["f1"], reverse=True)
    # params are stored by leaf name; put the dotted keys back.
    leaf = {k.split(".")[-1]: k for k in AXES}
    out = []
    for r in rows[:top]:
        try:
            out.append({leaf[k]: v for k, v in r["params"].items()})
        except KeyError:
            continue
    return out


def free_gb(path):
    st = os.statvfs(path)
    return st.f_bavail * st.f_frsize / 1e9


def execute(tag, params, keep_dump):
    rid = run_id(params)
    cfg = os.path.join(SCRATCH, f"{rid}.json")
    dump = os.path.join(SCRATCH, f"{rid}.jsonl")
    bench = os.path.join(SCRATCH, f"{rid}-bench.json")
    args = [
        sys.executable,
        os.path.join(os.path.dirname(__file__), "mkconfig.py"),
        DEFAULTS,
        cfg,
    ]
    args += [f"{k}={v}" for k, v in {**params, **FIXED}.items()]
    subprocess.run(args, check=True)

    row = {
        "id": rid,
        "tag": tag,
        "params": {k.split(".")[-1]: v for k, v in params.items()},
    }
    started = time.time()
    try:
        proc = subprocess.run(
            [
                BENCH,
                "-pcap",
                PCAP,
                "-config",
                cfg,
                "-output",
                SCRATCH,
                "-benchmark-output",
                bench,
                "-clusters-output",
                dump,
                # The frame-budget gate measures contention when several runs
                # share the machine, and it fails hardest on the configs that
                # produce the most foreground: exactly the ones worth scoring.
                # Timing for the shortlist has to be re-measured one at a time.
                "-max-frames-over-budget-pct",
                "100",
                "-quiet",
                "-progress",
                "0",
            ],
            capture_output=True,
            text=True,
            timeout=RUN_TIMEOUT,
            cwd=REPO,
        )
    except subprocess.TimeoutExpired:
        row["error"] = "timeout"
        row["seconds"] = round(time.time() - started, 1)
        cleanup(cfg, dump, bench)
        return row
    row["seconds"] = round(time.time() - started, 1)
    if proc.returncode != 0:
        row["error"] = f"exit {proc.returncode}: {proc.stderr.strip()[:300]}"
        cleanup(cfg, dump, bench)
        return row

    try:
        with open(bench) as f:
            metrics = json.load(f)["metrics"]
        row["work"] = metrics["work"]
        row["frame_p95_ms"] = round(metrics["frame_time_stats"]["p95_ms"], 2)
        row["frame_max_ms"] = round(metrics["frame_time_stats"]["max_ms"], 2)
        row["frames_over_budget"] = metrics["frame_budget"]["frames_over"]
    except Exception as err:  # noqa: BLE001 - reported, not raised
        row["error"] = f"benchmark json: {err}"

    scored = subprocess.run(
        [
            sys.executable,
            os.path.join(os.path.dirname(__file__), "score_run.py"),
            GT,
            dump,
            rid,
        ],
        capture_output=True,
        text=True,
    )
    if scored.returncode != 0:
        row["error"] = f"score: {scored.stderr.strip()[:300]}"
    else:
        row.update(json.loads(scored.stdout))
    if keep_dump:
        os.makedirs(KEEP, exist_ok=True)
        shutil.copy(dump, os.path.join(KEEP, f"{rid}.jsonl"))
        shutil.copy(cfg, os.path.join(KEEP, f"{rid}-config.json"))
    cleanup(cfg, dump, bench)
    return row


def cleanup(*paths):
    for p in paths:
        try:
            os.remove(p)
        except OSError:
            pass


def main():
    os.makedirs(SCRATCH, exist_ok=True)
    done = set()
    if os.path.exists(RESULTS):
        with open(RESULTS) as f:
            for line in f:
                try:
                    done.add(json.loads(line)["id"])
                except Exception:  # noqa: BLE001 - a torn last line
                    pass

    queue = references() + [("grid", c) for c in combos()]
    seen, work = set(), []
    for tag, params in queue:
        rid = run_id(params)
        if rid in seen or rid in done:
            continue
        seen.add(rid)
        work.append((tag, params, tag != "grid"))

    deadline = time.time() + HOURS * 3600
    print(
        f"{len(work)} runs queued ({len(done)} already done), {JOBS} at a time, "
        f"deadline in {HOURS:.1f} h",
        flush=True,
    )

    out = open(RESULTS, "a")
    started = time.time()
    finished = 0
    it = iter(work)
    exhausted = False

    def out_of_time():
        return time.time() > deadline or free_gb(WORK) < MIN_FREE_GB

    with cf.ThreadPoolExecutor(max_workers=JOBS) as pool:
        pending = set()
        while True:
            # Keep the pool full while there is work and time.
            while not exhausted and len(pending) < JOBS and not out_of_time():
                try:
                    tag, params, keep = next(it)
                except StopIteration:
                    exhausted = True
                    break
                pending.add(pool.submit(execute, tag, params, keep))

            if pending:
                done_now, pending = cf.wait(pending, return_when=cf.FIRST_COMPLETED)
                for fut in done_now:
                    try:
                        row = fut.result()
                    except Exception as err:  # noqa: BLE001 - one run, not the sweep
                        row = {"error": f"driver: {err}"}
                    out.write(json.dumps(row) + "\n")
                    out.flush()
                    finished += 1
                    if finished % 100 == 0:
                        rate = finished / max(time.time() - started, 1) * 3600
                        left = (deadline - time.time()) / 3600
                        print(
                            f"{finished} done, {rate:.0f}/h, {left:.2f} h left",
                            flush=True,
                        )
                continue

            # Nothing running and nothing queued.
            if out_of_time():
                print("stopping: out of time or disk", flush=True)
                break
            # The grid is exhausted but the clock is not. Hill-climb from the
            # best configs found so far, one ladder step at a time.
            fresh = []
            for base in best_so_far(RESULTS, 25):
                for cand in neighbours(base):
                    rid = run_id(cand)
                    if rid in seen or rid in done:
                        continue
                    seen.add(rid)
                    fresh.append(("climb", cand, False))
            if not fresh:
                print("nothing left to try", flush=True)
                break
            print(f"climbing from the best 25: {len(fresh)} new configs", flush=True)
            it = iter(fresh)
            exhausted = False
    out.close()
    print(
        f"finished {finished} runs in {(time.time() - started) / 3600:.2f} h",
        flush=True,
    )


main()
