#!/usr/bin/env python3
"""Publish a web scene for every site in the archive index.

Resumable. A site whose scene already carries a .rebuilt marker is skipped, so
this can be stopped — or the machine shut down and the drive unplugged — and
started again from where it left off.

    make dev-go-lidar                       # the server must be running
    python3 tools/s2-archive/publish-scenes.py            # everything outstanding
    python3 tools/s2-archive/publish-scenes.py laguna-eddy  # or named sites

Two settings matter and neither is the default:

  REPLAY_SPEED_MODE=scaled REPLAY_SPEED_RATIO=0.5
      Analysis mode replays as fast as the pipeline will take packets, and the
      background model's warm-up is gated on wall clock, so a fast replay puts
      most of the recording inside it. Measured on one capture: 2.0 clusters
      per frame in analysis mode against 16.8 at half speed, and a VRLOG
      holding 23% of the frames it was given. Every rate from 0.5x down scores
      the same, so 0.5x is the fastest one that loses nothing.

  REPLAY_SETTLE=0
      settle_before_recording replays the same window twice, settling on the
      first pass. That trains the background model on the very traffic the
      recording exists to show, and it is then suppressed. The opening
      transient it was added for is dropped at export instead, by --start-frame.

The environment the batch expects, in full:

    REPLAY_SETTLE=0 REPLAY_SPEED_MODE=scaled REPLAY_SPEED_RATIO=0.5 \
      REPLAY_PCAP_SUBDIR=s2 python3 tools/s2-archive/publish-scenes.py

A scene is built beside the published one and swapped in only when whole, so a
half-finished export never reaches the site.
"""

import json
import os
import shutil
import subprocess
import sqlite3
import sys
import time
import urllib.error
import urllib.request

REPO = "/Users/david/code/velocity.report"
HERE = os.path.dirname(os.path.abspath(__file__))
API = "http://localhost:8080/api/lidar"
PCAP_SUBDIR = os.environ.get("REPLAY_PCAP_SUBDIR", "s2")
SPEED_MODE = os.environ.get("REPLAY_SPEED_MODE", "analysis")
SPEED_RATIO = float(os.environ.get("REPLAY_SPEED_RATIO", "0") or 0)
SETTLE = os.environ.get("REPLAY_SETTLE", "1") not in ("0", "false", "False")
SENSOR = "hesai-pandar40p"
DB = os.path.join(REPO, "sensor_data.db")
# The multi-call binary dispatches on argv[0], so the scene surface is only
# reachable through a name it recognises. dev-go-lidar builds
# velocity-report-local, which is the server surface; a "velocity" symlink
# beside it is what exposes `scene export`.
VELOCITY = os.path.join(REPO, "velocity")
SCENES = os.path.join(REPO, "public_html", "src", "scenes")
# Scenes come from the archive index; static-stretches.json is no longer read.

STRIDE = "2"
TRANSIENT_FRAMES = 400  # 40 s at 10 Hz, dropped at export not at record
POLL_SECONDS = 20


def log(message):
    print(f"[{time.strftime('%H:%M:%S')}] {message}", flush=True)


def api(path, payload=None, timeout=900):
    data = json.dumps(payload).encode() if payload is not None else None
    request = urllib.request.Request(
        f"{API}{path}",
        data=data,
        headers={"Content-Type": "application/json"} if data else {},
    )
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            return json.load(response)
    except urllib.error.HTTPError as error:
        body = error.read()[:300].decode(errors="replace")
        raise RuntimeError(f"{error.code} on {API}{path}: {body}") from None


def stop_any_replay():
    """Clear the deck before starting.

    Killing a client does not stop the server: an abandoned replay holds the
    slot until it finishes on its own, and the next scene then waits out its
    whole timeout for a run nobody is watching. Ask for a stop first.
    """
    try:
        if not api("/playback/status", timeout=20)["replay_active"]:
            return
    except Exception:
        return
    log("  a replay is already running; stopping it")
    try:
        api(f"/pcap/stop?sensor_id={SENSOR}", payload={}, timeout=60)
    except Exception as error:
        log(f"  stop request failed: {error}")


def wait_for_idle(timeout_seconds=10800, heartbeat_seconds=300):
    """Wait for the replay slot, reporting progress while we wait.

    A scene takes twice its own length to replay, so without a heartbeat the
    log is silent for the better part of an hour and an onlooker cannot tell a
    healthy run from a hung one. Twice that silence has been read as a stall.
    """
    deadline = time.time() + timeout_seconds
    last_beat = time.time()
    while time.time() < deadline:
        try:
            status = api("/playback/status")
            if not status["replay_active"]:
                return True
            if time.time() - last_beat >= heartbeat_seconds:
                last_beat = time.time()
                done, total = status.get("current_frame", 0), status.get(
                    "total_frames", 0
                )
                share = f"{done / total * 100:.0f}%" if total else "?"
                log(f"    ... replaying, {share} of packets read")
        except Exception:
            pass  # a blip in the status endpoint is not the end of the replay
        time.sleep(POLL_SECONDS)
    return False


def newest_run(basename):
    with sqlite3.connect(f"file:{DB}?mode=ro", uri=True) as connection:
        return connection.execute(
            "SELECT run_id, status, vrlog_path FROM lidar_run_records"
            " WHERE source_path LIKE ? ORDER BY created_at DESC LIMIT 1",
            (f"%{basename}",),
        ).fetchone()


def export(vrlog, out_dir, site, title, kind=None, extra=None):
    command = [
        VELOCITY,
        "scene",
        "export",
        "--vrlog",
        vrlog,
        "--out",
        out_dir,
        "--site",
        site,
        "--title",
        title,
    ]
    command += ["--export", kind] if kind else ["--stride", STRIDE]
    command += extra or []
    done = subprocess.run(command, capture_output=True, text=True, cwd=REPO)
    if done.returncode != 0:
        raise RuntimeError(f"export failed: {done.stderr.strip()}")
    return done.stdout.strip()


def replay_stretch(scene):
    """Replay the whole stretch as one sequence and return its VRLOG."""
    # The server resolves a replay path against --lidar-pcap-dir, which is the
    # volume root, so a bare capture name has to carry its subdirectory.
    files = [
        os.path.join(PCAP_SUBDIR, part["file"]) if PCAP_SUBDIR else part["file"]
        for part in scene["parts"]
    ]
    start = scene["start_secs"]
    duration = scene["duration"]

    payload = {
        "pcap_files": files,
        "analysis_mode": True,
        "speed_mode": SPEED_MODE,
        "settle_before_recording": SETTLE,
        "duration_seconds": round(duration, 3),
    }
    if SPEED_MODE == "scaled" and SPEED_RATIO > 0:
        payload["speed_ratio"] = SPEED_RATIO
    if start > 0.5:
        payload["start_seconds"] = round(start, 3)

    # A replay left running by an earlier attempt answers with a 409, so wait
    # for the deck to clear rather than reporting a failure that is really a
    # queue. Starting a sequence also counts every file's packets before it
    # returns, which for seven captures is well over a minute.
    stop_any_replay()
    if not wait_for_idle(120):
        raise RuntimeError("the replay slot did not clear")
    api(f"/pcap/start?sensor_id={SENSOR}", payload)
    time.sleep(5)
    # At half speed a scene takes twice its own length; three times that is
    # generous for a healthy run and still catches a stall the same morning.
    budget = max(600, duration * 6)
    if not wait_for_idle(budget):
        raise RuntimeError(f"replay did not finish within {budget / 60:.0f} min")

    row = newest_run(files[0])
    if not row:
        raise RuntimeError("no run record for the sequence")
    run_id, status, vrlog = row
    if status != "completed" or not vrlog:
        raise RuntimeError(f"run {run_id} ended {status!r}, vrlog={vrlog!r}")
    if not os.path.exists(os.path.join(vrlog, "header.json")):
        raise RuntimeError(f"{vrlog} has no header.json")
    return vrlog


def build_scene(scene):
    site, title = scene["site"], scene["title"]
    live = os.path.join(SCENES, site, "assets")
    # Build beside the published assets and swap only when the export is whole,
    # so a scene is never half-replaced while the batch runs — the page keeps
    # serving the old recording until there is a new one to serve.
    assets = live + ".new"
    part = os.path.join(assets, "part-000")
    if os.path.exists(os.path.join(SCENES, site, ".rebuilt")):
        log(f"{site}: already rebuilt, skipping")
        return

    shutil.rmtree(assets, ignore_errors=True)
    os.makedirs(assets, exist_ok=True)
    log(
        f"=== {site}: {scene['minutes']:.1f} min across {len(scene['parts'])} capture(s) ==="
    )
    started = time.time()
    vrlog = replay_stretch(scene)
    log(f"  vrlog in {(time.time() - started) / 60:.1f} min: {os.path.basename(vrlog)}")

    summary = export(
        vrlog, part, site, title, extra=["--start-frame", str(TRANSIENT_FRAMES)]
    )
    for line in summary.splitlines()[1:]:
        log("    " + line.strip())
    export(vrlog, os.path.join(assets, "background"), site, title, kind="background")

    with open(os.path.join(part, "header.json")) as fh:
        duration = float(json.load(fh)["duration_sec"])
    with open(os.path.join(assets, "manifest.json"), "w") as fh:
        json.dump(
            {
                "version": 1,
                "site": {"id": site, "title": title},
                "parts": [{"url": "./part-000/", "start_seconds": 0}],
            },
            fh,
            indent=2,
        )
        fh.write("\n")
    # Swap: retire the old assets, put the new ones in their place.
    retired = live + ".old"
    shutil.rmtree(retired, ignore_errors=True)
    if os.path.exists(live):
        os.rename(live, retired)
    os.rename(assets, live)
    shutil.rmtree(retired, ignore_errors=True)
    with open(os.path.join(SCENES, site, ".rebuilt"), "w") as fh:
        fh.write(f"{SPEED_MODE} {SPEED_RATIO} settle={SETTLE}\n")
    # Regenerate the map and page data now rather than at the end of the batch.
    # A scene that exists but is still listed as unpublished is the same bug to
    # a reader as one that does not exist, and a fourteen-hour batch is a long
    # time to look wrong.
    refresh = subprocess.run(
        ["make", "render-scene-map"], cwd=REPO, capture_output=True, text=True
    )
    if refresh.returncode != 0:
        log(
            f"  warning: scene map not refreshed: {refresh.stderr.strip().splitlines()[-1:]}"
        )
    log(f"  {site} done: {duration / 60:.1f} min published")


def main():
    """Scenes come from the site index, which is the canonical list of places.

    Identity is the site's id, slugged from the field mark, so a scene
    directory is named for where it was recorded rather than for the capture
    prefix its files happen to carry. The prefix named a deployment, not a
    place, and six sites shared one.
    """
    from datetime import datetime

    with open(os.path.join(REPO, "tools", "s2-archive", "site-index.json")) as fh:
        index = json.load(fh)

    wanted = sys.argv[1:]
    if wanted:
        index = [e for e in index if e["id"] in wanted or e["site"] in wanted]

    scenes = []
    for entry in index:
        captures = sorted(entry["captures"])
        stamp = captures[0].rsplit("_", 2)[-2]
        offset = (
            datetime.fromisoformat(entry["start"]).replace(tzinfo=None)
            - datetime.strptime(stamp, "%Y%m%d%H%M%S")
        ).total_seconds()
        scenes.append(
            {
                "site": entry["id"],
                "title": entry["where"] or entry["id"],
                "minutes": entry["minutes"],
                "parts": [
                    {
                        "file": name,
                        "start_secs": offset if name == captures[0] else 0,
                        "end_secs": (
                            offset + entry["minutes"] * 60 if name == captures[0] else 0
                        ),
                    }
                    for name in captures
                ],
                "duration": entry["minutes"] * 60,
                "start_secs": max(offset, 0.0),
            }
        )

    log(f"{len(scenes)} scene(s): {', '.join(s['site'] for s in scenes)}")
    failures = []
    for scene in scenes:
        try:
            build_scene(scene)
        except Exception as error:  # one bad stretch must not end the batch
            log(f"  FAILED {scene['site']}: {error}")
            failures.append(scene["site"])
    log(f"batch finished; {len(scenes) - len(failures)} ok, {len(failures)} failed")
    if failures:
        log(f"  failed: {', '.join(failures)}")


if __name__ == "__main__":
    main()
