#!/usr/bin/env python3
"""Replay published-corpus captures at several speeds and say what each kept (perf0002).

Each unit is one arm on one case, replayed the way publish-scenes.py replays a
scene: the whole trimmed capture, grid preserved, no settling pass. The unit's
VRLOG is counted (vrlog-check), summarised (vrlog-analyse report) and exported
as a web scene at the published stride and start frame, and the server log
written during the replay is kept beside them. Units are resumable: one with a
result.json is skipped.

    replay-speed-sweep.py run --api http://localhost:18080/api/lidar \\
        --db /path/server.db --log /path/server.log --out RUN \\
        --case ashbury-downey --arm x0.5 [--repeat 2]
    replay-speed-sweep.py table RUN          # the per-arm, per-case table
    replay-speed-sweep.py export-full RUN    # every frame of every unit, for compare
    replay-speed-sweep.py compare A B        # two exported scenes, row for row

Everything written goes under --out, outside the checkout: raw output never
belongs in Git (AGENTS.md). The arms are the spec's,
data/experiments/perf/perf0002.md, and the tuning files are paths relative to
the server's --lidar-config-dir.
"""

import argparse
import gzip
import importlib.util
import json
import os
import sqlite3
import subprocess
import sys
import time

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.dirname(os.path.dirname(HERE))

DEFAULTS = "tuning.defaults.json"

# speed_mode, speed_ratio (scaled only), tuning. Every arm runs its tuning file's L3
# as written: the paced L3 substitution the server made until October 2026 is gone.
ARMS = {
    "analysis": ("analysis", 0, DEFAULTS),
    "x0.25": ("scaled", 0.25, DEFAULTS),
    "x0.5": ("scaled", 0.5, DEFAULTS),
    "x1": ("realtime", 0, DEFAULTS),
    "x2": ("scaled", 2.0, DEFAULTS),
    "x4": ("scaled", 4.0, DEFAULTS),
    # Not one of the spec's arms: analysis with the settling pass, to see whether
    # it removes the dependence on the server's previous replay.
    "analysis-settle": ("analysis", 0, DEFAULTS),
}
SETTLE_ARMS = {"analysis-settle"}

# The published scene's export settings (publish-scenes.py).
STRIDE = 2
START_FRAME = 400


def load_publish(api, db):
    """publish-scenes.py, pointed at this lane's server and database."""
    os.environ["LIDAR_API_URL"] = api
    os.environ["LIDAR_DB_PATH"] = db
    spec = importlib.util.spec_from_file_location(
        "publish_scenes", os.path.join(HERE, "publish-scenes.py")
    )
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def corpus_capture(corpus_dir, case):
    """The case's trimmed capture: its path relative to the corpus root and its length."""
    with open(os.path.join(corpus_dir, "manifest.json")) as f:
        manifest = json.load(f)
    for entry in manifest:
        path = entry.get("raw_path") or ""
        if entry.get("site_slug") == case and path.startswith("raw/lidar/"):
            return path, float(entry["duration_seconds"])
    raise SystemExit(f"no raw LiDAR capture for {case} in {corpus_dir}/manifest.json")


def tool(binary, *args, cwd=REPO):
    done = subprocess.run([binary, *args], capture_output=True, text=True, cwd=cwd)
    if done.returncode != 0:
        raise RuntimeError(
            f"{os.path.basename(binary)} failed: {done.stderr.strip()[-400:]}"
        )
    return done.stdout


def vrlog_counts(binary, vrlog):
    """Frames, background frames and empty frames, from vrlog-check."""
    out = tool(binary, "-limit", "100000000", vrlog).splitlines()
    fields = out[-1].split()
    # RUN ENC FRAMES FIRSTBG BGFRAMES EMPTY VERDICT...
    return {
        "vrlog_frames": int(fields[2]),
        "first_background_frame": int(fields[3]),
        "background_frames": int(fields[4]),
        "empty_frames": int(fields[5]),
        "verdict": " ".join(fields[6:]),
    }


def log_slice(path, start):
    with open(path, "rb") as f:
        f.seek(start)
        return f.read().decode(errors="replace")


def run_unit(args):
    unit = os.path.join(args.out, args.case, args.arm, f"r{args.repeat}")
    if os.path.exists(os.path.join(unit, "result.json")):
        print(f"done already: {unit}")
        return
    os.makedirs(unit, exist_ok=True)
    publish = load_publish(args.api, args.db)
    rel, duration = corpus_capture(args.corpus_dir, args.case)
    pcap_file = os.path.join(args.corpus_subdir, rel) if args.corpus_subdir else rel
    mode, ratio, tuning = ARMS[args.arm]

    payload = {
        "pcap_file": pcap_file,
        "analysis_mode": True,
        "speed_mode": mode,
        "settle_before_recording": args.arm in SETTLE_ARMS,
        "duration_seconds": round(duration, 3),
        "tuning_config": tuning,
    }
    if mode == "scaled":
        payload["speed_ratio"] = ratio

    publish.stop_any_replay()
    if not publish.wait_for_idle(120):
        raise RuntimeError("the replay slot did not clear")
    log_start = os.path.getsize(args.log)
    debug_start = os.path.getsize(args.debug_log)
    requested_ns = time.time_ns() - 1_000_000_000
    began = time.time()
    started = publish.api(f"/pcap/start?sensor_id={publish.SENSOR}", payload)
    time.sleep(5)
    pace = {"scaled": ratio, "realtime": 1.0}.get(mode)
    # A floor for a host under memory pressure, where an analysis replay that
    # takes minutes alone has taken forty (SWEEP_MIN_BUDGET_S, default 15 min).
    floor = float(os.environ.get("SWEEP_MIN_BUDGET_S", "900"))
    budget = max(floor, (duration / pace if pace else duration) * 1.5 + 600)
    if not publish.wait_for_idle(budget):
        raise RuntimeError(f"replay did not finish within {budget / 60:.0f} min")
    wall = time.time() - began

    row = publish.newest_run(os.path.basename(rel), since_ns=requested_ns)
    if not row:
        raise RuntimeError("no run record since the start")
    run_id, status, vrlog = row
    if status != "completed" or not vrlog:
        raise RuntimeError(f"run {run_id} ended {status!r}, vrlog={vrlog!r}")

    with open(os.path.join(unit, "server.log"), "w") as f:
        f.write(log_slice(args.log, log_start))
    counts = vrlog_counts(os.path.join(args.bin, "vrlog-check"), vrlog)
    tool(os.path.join(args.bin, "vrlog-analyse"), "report", vrlog)
    with open(os.path.join(vrlog, "analysis.json")) as f:
        analysis = json.load(f)
    with open(os.path.join(unit, "analysis.json"), "w") as f:
        json.dump(analysis, f)
    scene = os.path.join(unit, "scene")
    tool(
        publish.VELOCITY,
        "scene",
        "export",
        "--vrlog",
        vrlog,
        "--out",
        scene,
        "--site",
        args.case,
        "--title",
        args.case,
        "--stride",
        str(STRIDE),
        "--start-frame",
        str(START_FRAME),
    )

    text = log_slice(args.log, log_start)
    debug = log_slice(args.debug_log, debug_start)
    # The throttle logs its running count every 50 frames at diag level.
    throttled = [
        int(line.split("Throttled ")[1].split()[0])
        for line in debug.splitlines()
        if "[Pipeline] Throttled " in line
    ]
    result = {
        "case": args.case,
        "arm": args.arm,
        "repeat": args.repeat,
        "payload": payload,
        "start_response": started,
        "run_id": run_id,
        "vrlog": vrlog,
        "capture_seconds": duration,
        "wall_seconds": round(wall, 1),
        "pcap_quality_lines": [
            line for line in text.splitlines() if "[PCAP quality]" in line
        ],
        "throttled_frames_logged": max(throttled) if throttled else 0,
        **counts,
    }
    with open(os.path.join(unit, "result.json"), "w") as f:
        json.dump(result, f, indent=2)
    print(
        f"{args.case} {args.arm} r{args.repeat}: {wall / 60:.1f} min, "
        f"{counts['vrlog_frames']} frames, {counts['empty_frames']} empty"
    )


def export_full(run):
    """Every unit's VRLOG again, every frame from the first (scene-full/).

    The published export starts at a VRLOG position and keeps every other
    frame, so two replays whose VRLOGs hold a different number of background
    snapshots export alternate frames and share none. Comparing replays row
    for row needs every frame.
    """
    for case in sorted(os.listdir(run)):
        for arm in ARMS:
            base = os.path.join(run, case, arm)
            if not os.path.isdir(base):
                continue
            for rep in sorted(os.listdir(base)):
                unit = os.path.join(base, rep)
                out = os.path.join(unit, "scene-full")
                if not os.path.exists(
                    os.path.join(unit, "result.json")
                ) or os.path.exists(os.path.join(out, "header.json")):
                    continue
                with open(os.path.join(unit, "result.json")) as f:
                    vrlog = json.load(f)["vrlog"]
                tool(
                    os.path.join(REPO, "velocity"),
                    "scene",
                    "export",
                    "--vrlog",
                    vrlog,
                    "--out",
                    out,
                    "--site",
                    case,
                    "--title",
                    case,
                    "--stride",
                    "1",
                    "--start-frame",
                    "0",
                )
                print(f"exported {unit}")


def scene_frames(scene):
    """{capture time in ms: [track rows]} from an exported scene's NDJSON chunks.

    Frames are keyed by capture time, not by "f": f is the server's running
    frame counter, and t is measured from the scene's first frame. Until
    --start-frame counted rotations it picked that frame by VRLOG position,
    and a VRLOG interleaved background snapshots on the wall clock, so two
    replays of one capture could start their scenes a frame or two apart;
    keying by capture time still reads exports made before then.
    """
    with open(os.path.join(scene, "header.json")) as f:
        start_ns = int(json.load(f)["start_ns"])
    frames = {}
    part = os.path.join(scene, "frames")
    for name in sorted(os.listdir(part)):
        if not name.endswith(".ndjson.gz"):
            continue
        with gzip.open(os.path.join(part, name), "rt") as f:
            for line in f:
                record = json.loads(line)
                key = round((start_ns + record["t"] * 1000) / 1e6)
                frames[key] = record.get("tr", [])
    return frames


ROW_FIELDS = ("x", "y", "z", "vx", "vy", "l", "w", "h", "bh", "bx", "by", "c")


def compare(a, b):
    """Two scenes row for row, under a one-to-one map between their track ids.

    Track ids are drawn at random per run, so they are matched by the frames
    they share: each id in A to the id in B whose rows equal its own most
    often. The scenes agree when every frame holds the same rows under that map.
    """
    fa, fb = scene_frames(a), scene_frames(b)
    common = sorted(set(fa) & set(fb))
    votes = {}
    for f in common:
        rb = {}
        for t in fb[f]:
            rb.setdefault(tuple(t.get(k) for k in ROW_FIELDS), []).append(t["id"])
        for t in fa[f]:
            for idb in rb.get(tuple(t.get(k) for k in ROW_FIELDS), []):
                votes[(t["id"], idb)] = votes.get((t["id"], idb), 0) + 1
    mapping, used = {}, set()
    for (ida, idb), _ in sorted(votes.items(), key=lambda kv: -kv[1]):
        if ida not in mapping and idb not in used:
            mapping[ida] = idb
            used.add(idb)
    rows_a = rows_b = matched = 0
    differing_frames = 0
    for f in common:
        ra = {
            (mapping.get(t["id"]),) + tuple(t.get(k) for k in ROW_FIELDS) for t in fa[f]
        }
        rb = {(t["id"],) + tuple(t.get(k) for k in ROW_FIELDS) for t in fb[f]}
        rows_a += len(fa[f])
        rows_b += len(fb[f])
        matched += len(ra & rb)
        differing_frames += ra != rb
    ids_a = {t["id"] for f in fa for t in fa[f]}
    ids_b = {t["id"] for f in fb for t in fb[f]}
    return {
        "frames_a": len(fa),
        "frames_b": len(fb),
        "frames_common": len(common),
        "frames_differing": differing_frames,
        "rows_a": rows_a,
        "rows_b": rows_b,
        "rows_matched": matched,
        "tracks_a": len(ids_a),
        "tracks_b": len(ids_b),
        "tracks_mapped": len(mapping),
        "frames_only_a": len(fa) - len(common),
        "frames_only_b": len(fb) - len(common),
        # Over the frames both scenes hold: a scene can start a frame or two
        # later than another (see scene_frames), which is not a disagreement.
        "identical": bool(common) and differing_frames == 0,
    }


def run_record(dbs, run_id):
    """The server's own totals for the run: frames processed and clusters found.

    The VRLOG carries only the clusters no track took, so the run record is
    where the total cluster count is.
    """
    for db in dbs:
        with sqlite3.connect(f"file:{db}?mode=ro", uri=True) as connection:
            row = connection.execute(
                "SELECT total_frames, total_clusters FROM lidar_runs WHERE run_id = ?",
                (run_id,),
            ).fetchone()
        if row:
            return {"processed_frames": row[0], "total_clusters": row[1]}
    return {"processed_frames": None, "total_clusters": None}


def summary(unit, dbs):
    with open(os.path.join(unit, "result.json")) as f:
        r = json.load(f)
    rec = run_record(dbs, r["run_id"])
    with open(os.path.join(unit, "analysis.json")) as f:
        a = json.load(f)
    fs, ts = a["frame_summary"], a["track_summary"]
    confirmed = [t for t in a.get("tracks", []) if t.get("ever_confirmed")]
    life = sorted(t["duration_secs"] for t in confirmed)

    def pct(p):
        return round(life[min(len(life) - 1, int(p * len(life)))], 1) if life else None

    return {
        "case": r["case"],
        "arm": r["arm"],
        "repeat": r["repeat"],
        "wall_min": round(r["wall_seconds"] / 60, 1),
        "vrlog_frames": r["vrlog_frames"],
        "data_frames": r["vrlog_frames"] - r["background_frames"],
        "empty_frames": r["empty_frames"],
        "pcap_quality": len(r["pcap_quality_lines"]),
        "throttled": r.get("throttled_frames_logged", 0),
        "fg_points_per_frame": round(fs["avg_foreground_points_per_frame"], 1),
        "processed_frames": rec["processed_frames"],
        "clusters_per_frame": (
            round(rec["total_clusters"] / rec["processed_frames"], 3)
            if rec["processed_frames"]
            else None
        ),
        "unassociated_clusters_per_frame": round(fs["avg_clusters_per_frame"], 3),
        "confirmed_tracks": len(confirmed),
        "track_rows": sum(t.get("observation_count", 0) for t in confirmed),
        "life_p50_s": pct(0.5),
        "life_p90_s": pct(0.9),
        "first_background_frame": r["first_background_frame"],
        "tracks_total": ts["total_tracks"],
    }


def table(run, dbs):
    rows = []
    for case in sorted(os.listdir(run)):
        for arm in ARMS:
            base = os.path.join(run, case, arm)
            if not os.path.isdir(base):
                continue
            for rep in sorted(os.listdir(base)):
                if os.path.exists(os.path.join(base, rep, "result.json")):
                    rows.append(summary(os.path.join(base, rep), dbs))
    json.dump(rows, sys.stdout, indent=1)
    print()


def main():
    parser = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    sub = parser.add_subparsers(dest="command", required=True)
    run = sub.add_parser("run")
    run.add_argument("--api", required=True)
    run.add_argument("--db", required=True)
    run.add_argument(
        "--log", required=True, help="the server's ops log, read for the replay's lines"
    )
    run.add_argument(
        "--debug-log",
        required=True,
        help="the server's VELOCITY_DEBUG_LOG, for the throttle's count",
    )
    run.add_argument("--out", required=True)
    run.add_argument("--case", required=True)
    run.add_argument("--arm", required=True, choices=sorted(ARMS))
    run.add_argument("--repeat", type=int, default=1)
    run.add_argument(
        "--corpus-dir", required=True, help="the corpus root, holding manifest.json"
    )
    run.add_argument(
        "--corpus-subdir",
        default="sf-street-speeds",
        help="the corpus root relative to the server's --lidar-pcap-dir",
    )
    run.add_argument(
        "--bin", required=True, help="directory holding vrlog-check and vrlog-analyse"
    )
    t = sub.add_parser("table")
    t.add_argument("run")
    t.add_argument(
        "--db",
        action="append",
        default=[],
        help="a server database holding the runs' records; repeat",
    )
    e = sub.add_parser("export-full")
    e.add_argument("run")
    c = sub.add_parser("compare")
    c.add_argument("a")
    c.add_argument("b")
    args = parser.parse_args()
    if args.command == "run":
        run_unit(args)
    elif args.command == "export-full":
        export_full(args.run)
    elif args.command == "table":
        table(args.run, args.db)
    else:
        json.dump(compare(args.a, args.b), sys.stdout, indent=1)
        print()


if __name__ == "__main__":
    main()
