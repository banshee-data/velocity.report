#!/usr/bin/env python3
"""Publish a web scene for every site in the archive index.

Resumable. A site whose scene already carries a .rebuilt marker is skipped, so
this can be stopped — or the machine shut down and the drive unplugged — and
started again from where it left off.

Prefer the Makefile, which supplies the settings below and checks the corpus
before it starts a fourteen-hour batch:

    make dev-go-lidar LIDAR_PCAP_DIR=/Volumes/lidar/lidar   # in another shell
    make scene-assets-status                                # what is outstanding
    make scene-assets                                       # rebuild it

Run directly only when you want something the target does not offer:

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
      python3 tools/s2-archive/publish-scenes.py

  LIDAR_API_URL=http://localhost:8080/api/lidar
      Optional: direct an isolated worktree run to its own LiDAR server.

A scene is built beside the published one and swapped in only when whole, so a
half-finished export never reaches the site.

Where the packets come from
---------------------------

Two sources, selected by --source (SCENE_SOURCE):

  corpus (default)
      One trimmed PCAPNG per site from the published dataset, already clipped
      to the site's exact bounds by export-static-pcaps.py. The manifest names
      the file and its duration, so a scene is one file replayed whole with no
      offset arithmetic — and the packets are the ones the dataset publishes,
      which is what makes a scene reproducible by anyone who downloads it.

  archive
      The original rolling captures, joined in order and clipped at replay time
      by a start offset measured from the first packet in the first capture. This
      is how the scenes published before the dataset existed were made; keep it
      to reproduce one of those.

Either way the server resolves a replay path against --lidar-pcap-dir, so both
the corpus and the archive have to sit under it, and paths are sent relative
to it.
"""

import argparse
import json
import os
import shutil
import subprocess
import sqlite3
import struct
import sys
import time
import urllib.error
import urllib.request
from datetime import datetime, timedelta, timezone

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.dirname(os.path.dirname(HERE))
# LiDAR routes are registered on the main HTTP API; 8081 is the legacy monitor.
API = os.environ.get("LIDAR_API_URL", "http://localhost:8080/api/lidar").rstrip("/")
PCAP_SUBDIR = os.environ.get("REPLAY_PCAP_SUBDIR", "s2")
SPEED_MODE = os.environ.get("REPLAY_SPEED_MODE", "analysis")
SPEED_RATIO = float(os.environ.get("REPLAY_SPEED_RATIO", "0") or 0)
SETTLE = os.environ.get("REPLAY_SETTLE", "1") not in ("0", "false", "False")
SENSOR = "hesai-pandar40p"
# Where the run records are. DB_PATH is what the dev target starts the server
# with, so following it keeps the two from naming different files: a server
# writing its records one place while this script reads another looks like a
# clean batch until every export fails on a database that was never there.
DB = (
    os.environ.get("LIDAR_DB_PATH")
    or os.environ.get("DB_PATH")
    or os.path.join(REPO, "sensor_data.db")
)
SITE_INDEX = os.path.join(HERE, "site-index.json")
# The server's safe directory for replay, and the published dataset root that
# holds the trimmed per-site captures. The corpus has to sit under the safe
# directory: resolvePCAPPath joins what it is given to that directory, so an
# absolute path outside it is refused.
PCAP_DIR = os.environ.get("LIDAR_PCAP_DIR", "")
CORPUS_DIR = os.environ.get("S2_CORPUS_DIR", "")
CORPUS_MANIFEST = "manifest.json"
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
# The per-frame foreground cap and chunk span the published point-cloud clip was
# made with (docs/plans/lidar-web-scene-export-plan.md, item 5): 10-second chunks
# keep each request below 1 MiB, where the exporter's default would make one.
CLIP_MAX_POINTS = 1200
CLIP_CHUNK_SECONDS = 10


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


def db_problem():
    """Why the run records cannot be read, or None if they can.

    Checked before the first replay because it is only needed after one: a
    batch that cannot reach the records replays every scene in full, at forty
    minutes each, and fails each one at the export that follows. That cost a
    whole run once.
    """
    try:
        with sqlite3.connect(f"file:{DB}?mode=ro", uri=True) as connection:
            connection.execute("SELECT 1 FROM lidar_run_records LIMIT 1")
    except sqlite3.OperationalError as error:
        return (
            f"cannot read run records from {DB}: {error}. This is the database "
            "the server writes; set LIDAR_DB_PATH (or DB_PATH) to the one it "
            "was started with."
        )
    return None


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
    # Already relative to the server's safe directory: the scene builders
    # resolve that once, so a failure lands before the batch starts rather
    # than as a 403 an hour in.
    files = scene["files"]
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

    # By basename, not by the path we asked with. The server resolves what it
    # is given before recording it — a capture reached through a symlink is
    # recorded at its real location — so matching the request path finds
    # nothing and the replay looks as though it never happened.
    row = newest_run(os.path.basename(files[0]))
    if not row:
        raise RuntimeError("no run record for the sequence")
    run_id, status, vrlog = row
    if status != "completed" or not vrlog:
        raise RuntimeError(f"run {run_id} ended {status!r}, vrlog={vrlog!r}")
    if not os.path.exists(os.path.join(vrlog, "header.json")):
        raise RuntimeError(f"{vrlog} has no header.json")
    return vrlog


def carry_over(vrlog, live, assets, site, title):
    """Keep what a scene has that this script does not make.

    The swap replaces assets/ wholesale, and two things in it are chosen by hand:
    vantages.json, the camera positions somebody picked, and a point-cloud
    clip, whose manifest records which 30 seconds were selected and why.
    Dropping them leaves a page asking for a clip that is no longer there.
    The vantages are copied as they are. The clip is exported again from the
    new recording over the same source frames, so it stays aligned with the
    tracks it plays under and names the recording it came from; its manifest,
    being the record of the selection, is kept as it was.
    """
    vantages = os.path.join(live, "vantages.json")
    if os.path.exists(vantages):
        shutil.copy2(vantages, os.path.join(assets, "vantages.json"))
    clip_manifest = os.path.join(live, "clip", "manifest.json")
    if not os.path.exists(clip_manifest):
        return
    with open(clip_manifest) as fh:
        try:
            manifest = json.load(fh)
            selection = manifest["selection"]
            start_frame = selection["source_start_frame"]
            frame_count = selection["source_frame_count"]
        except (json.JSONDecodeError, KeyError, TypeError) as error:
            raise RuntimeError(f"bad clip manifest {clip_manifest}: {error}") from error
    clip = os.path.join(assets, "clip")
    os.makedirs(clip, exist_ok=True)
    export(
        vrlog,
        os.path.join(clip, "part-000"),
        site,
        title,
        kind="clip",
        extra=[
            "--start-frame",
            str(start_frame),
            "--frame-count",
            str(frame_count),
            "--max-points",
            str(CLIP_MAX_POINTS),
            "--chunk-seconds",
            str(CLIP_CHUNK_SECONDS),
        ],
    )
    shutil.copy2(clip_manifest, os.path.join(clip, "manifest.json"))


def build_scene(scene):
    site, title = scene["site"], scene["title"]
    live = os.path.join(SCENES, site, "assets")
    # Build beside the published assets and swap only when the export is whole,
    # so a scene is never half-replaced while the batch runs — the page keeps
    # serving the old recording until there is a new one to serve.
    assets = live + ".new"
    part = os.path.join(assets, "part-000")
    if published(site):
        log(f"{site}: already rebuilt, skipping")
        return

    shutil.rmtree(assets, ignore_errors=True)
    os.makedirs(assets, exist_ok=True)
    log(
        f"=== {site}: {scene['minutes']:.1f} min from {len(scene['files'])} file(s)"
        f" [{scene['source']}] ==="
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
    carry_over(vrlog, live, assets, site, title)

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


# =============================================================================
# WHERE THE PACKETS COME FROM
# =============================================================================


def replay_relative(path, pcap_dir):
    """Express an absolute capture path the way the replay API wants it.

    resolvePCAPPath joins the candidate to --lidar-pcap-dir, so a path outside
    that directory cannot be named at all and an absolute one inside it is
    silently wrong. Returning None here lets the caller name every unreachable
    capture at once instead of discovering them one 403 at a time.
    """
    if not pcap_dir:
        return None
    relative = os.path.relpath(os.path.realpath(path), os.path.realpath(pcap_dir))
    if relative == os.pardir or relative.startswith(os.pardir + os.sep):
        return None
    return relative


def first_packet_time(path):
    """Read the first packet clock without scanning a rolling capture.

    Rolling files may be PCAPNG despite their .pcap suffix. The filename stamp
    is only whole seconds, while replay offsets start at the first packet.
    """
    classic = {
        b"\xd4\xc3\xb2\xa1": ("<", 1_000_000),
        b"\xa1\xb2\xc3\xd4": (">", 1_000_000),
        b"\x4d\x3c\xb2\xa1": ("<", 1_000_000_000),
        b"\xa1\xb2\x3c\x4d": (">", 1_000_000_000),
    }

    def clock(seconds, fraction, units, offset=0):
        return datetime.fromtimestamp(seconds + offset, timezone.utc) + timedelta(
            microseconds=round(fraction * 1_000_000 / units)
        )

    with open(path, "rb") as capture:
        header = capture.read(12)
        if len(header) < 12:
            raise ValueError(f"{path}: capture header is incomplete")
        if header[:4] in classic:
            endian, units = classic[header[:4]]
            capture.seek(24)
            packet = capture.read(16)
            if len(packet) < 16:
                raise ValueError(f"{path}: capture has no packet")
            seconds, fraction = struct.unpack(endian + "II", packet[:8])
            return clock(seconds, fraction, units)
        if header[:4] != b"\x0a\x0d\x0d\x0a":
            raise ValueError(f"{path}: unsupported capture format")
        byte_order = {b"\x4d\x3c\x2b\x1a": "<", b"\x1a\x2b\x3c\x4d": ">"}
        endian = byte_order.get(header[8:12])
        if endian is None:
            raise ValueError(f"{path}: invalid PCAPNG byte order")
        section_length = struct.unpack(endian + "I", header[4:8])[0]
        if section_length < 28:
            raise ValueError(f"{path}: invalid PCAPNG section length")
        capture.seek(section_length)
        interfaces = []
        while block := capture.read(8):
            if len(block) < 8:
                break
            kind, length = struct.unpack(endian + "II", block)
            if length < 12:
                raise ValueError(f"{path}: invalid PCAPNG block length")
            if kind == 1:  # Interface Description Block
                body = capture.read(length - 12)
                if len(body) != length - 12 or len(body) < 8:
                    break
                units, offset = 1_000_000, 0  # PCAPNG defaults to microseconds
                options = body[8:]
                cursor = 0
                while cursor + 4 <= len(options):
                    code, size = struct.unpack(
                        endian + "HH", options[cursor : cursor + 4]
                    )
                    cursor += 4
                    value = options[cursor : cursor + size]
                    if len(value) < size or code == 0:
                        break
                    if code == 9 and size == 1:  # if_tsresol
                        exponent = value[0]
                        units = (
                            (2 ** (exponent & 0x7F))
                            if exponent & 0x80
                            else (10**exponent)
                        )
                    if code == 14 and size == 8:  # if_tsoffset, in seconds
                        offset = struct.unpack(endian + "q", value)[0]
                    cursor += (size + 3) & ~3
                interfaces.append((units, offset))
                capture.seek(4, 1)  # trailing block length
            elif kind == 6:  # Enhanced Packet Block
                body = capture.read(20)
                if len(body) < 20:
                    break
                interface, high, low = struct.unpack(endian + "III", body[:12])
                if interface >= len(interfaces):
                    raise ValueError(f"{path}: packet references an absent interface")
                units, offset = interfaces[interface]
                ticks = high << 32 | low
                seconds, fraction = divmod(ticks, units)
                return clock(seconds, fraction, units, offset)
            else:
                capture.seek(length - 8, 1)
    raise ValueError(f"{path}: capture has no timestamped packet")


def load_corpus(corpus_dir):
    """Index the published dataset manifest by site slug.

    One entry per capture, and the LiDAR captures are one per site, so the slug
    is a key. A manifest that ever grows a second capture for a site would
    collide here rather than pick one at random.
    """
    manifest_path = os.path.join(corpus_dir, CORPUS_MANIFEST)
    with open(manifest_path) as fh:
        entries = json.load(fh)
    by_slug = {}
    for entry in entries:
        if entry.get("sensor_type") != "lidar":
            continue
        slug = entry.get("site_slug")
        if not slug:
            continue
        if slug in by_slug:
            raise RuntimeError(
                f"{manifest_path}: {slug} appears twice; a scene needs one capture"
            )
        by_slug[slug] = entry
    return by_slug


def scenes_from_corpus(index, corpus, corpus_dir, pcap_dir):
    """One trimmed capture per site, replayed whole.

    The dataset's captures are already clipped to the site bounds, so there is
    no offset to derive and nothing to join: the scene is the file. Identity —
    the id, the title, the position — still comes from the site index, which is
    what the map and the scene pages read. Refuse a stale corpus entry whose
    duration no longer agrees with a corrected site interval.
    """
    scenes, problems = [], []
    for entry in index:
        site = entry["id"]
        published = corpus.get(site)
        if not published:
            problems.append(f"{site}: not in the corpus manifest")
            continue
        absolute = os.path.join(corpus_dir, published["raw_path"])
        if not os.path.exists(absolute):
            problems.append(f"{site}: {published['raw_path']} is not on disk")
            continue
        relative = replay_relative(absolute, pcap_dir)
        if relative is None:
            problems.append(
                f"{site}: {absolute} is outside the replay directory {pcap_dir}"
            )
            continue
        duration = float(published.get("duration_seconds") or 0.0)
        if duration <= 0:
            problems.append(f"{site}: the manifest gives no duration")
            continue
        expected = (
            datetime.fromisoformat(entry["end"])
            - datetime.fromisoformat(entry["start"])
        ).total_seconds()
        if abs(duration - expected) > 1.0:
            problems.append(
                f"{site}: corpus duration {duration:.3f}s differs from the "
                f"site index {expected:.3f}s; rebuild the capture and manifest"
            )
            continue
        scenes.append(
            {
                "site": site,
                "title": entry["where"] or site,
                "minutes": duration / 60.0,
                "files": [relative],
                "duration": duration,
                "start_secs": 0.0,
                "source": "corpus",
            }
        )
    return scenes, problems


def scenes_from_archive(index, pcap_dir):
    """The original rolling captures, joined and clipped at replay time.

    Identity is the site's id, slugged from the field mark, so a scene
    directory is named for where it was recorded rather than for the capture
    prefix its files happen to carry. The prefix named a deployment, not a
    place, and six sites shared one.
    """
    scenes, problems = [], []
    for entry in index:
        if not pcap_dir:
            problems.append(f"{entry['id']}: --pcap-dir is required for archive replay")
            continue
        captures = sorted(entry["captures"])
        if not captures:
            problems.append(f"{entry['id']}: the site index names no captures")
            continue
        # Replay offsets are measured from the first packet, which can be a
        # fraction of a second after the rounded timestamp in a filename.
        offset = 0.0
        first_path = (
            os.path.join(pcap_dir, PCAP_SUBDIR, captures[0])
            if PCAP_SUBDIR
            else os.path.join(pcap_dir, captures[0])
        )
        if os.path.exists(first_path):
            try:
                offset = (
                    datetime.fromisoformat(entry["start"])
                    - first_packet_time(first_path)
                ).total_seconds()
            except ValueError as error:
                problems.append(f"{entry['id']}: {error}")
                continue
        duration = (
            datetime.fromisoformat(entry["end"])
            - datetime.fromisoformat(entry["start"])
        ).total_seconds()
        files = [
            os.path.join(PCAP_SUBDIR, name) if PCAP_SUBDIR else name
            for name in captures
        ]
        if pcap_dir:
            absent = [
                name
                for name in files
                if not os.path.exists(os.path.join(pcap_dir, name))
            ]
            if absent:
                problems.append(f"{entry['id']}: {', '.join(absent)} not on disk")
                continue
        scenes.append(
            {
                "site": entry["id"],
                "title": entry["where"] or entry["id"],
                "minutes": duration / 60.0,
                "files": files,
                "duration": duration,
                "start_secs": max(offset, 0.0),
                "source": "archive",
            }
        )
    return scenes, problems


def plan(source, wanted, corpus_dir, pcap_dir):
    """Resolve every wanted site to packets, before anything is replayed."""
    with open(SITE_INDEX) as fh:
        index = json.load(fh)
    if wanted:
        index = [e for e in index if e["id"] in wanted or e["site"] in wanted]
        known = {e["id"] for e in index} | {e["site"] for e in index}
        unknown = [name for name in wanted if name not in known]
        if unknown:
            raise SystemExit(
                f"not in {os.path.basename(SITE_INDEX)}: {', '.join(sorted(unknown))}"
            )
    if source == "archive":
        return scenes_from_archive(index, pcap_dir)
    if not corpus_dir:
        raise SystemExit(
            "the corpus source needs S2_CORPUS_DIR (or --corpus); "
            "run this through `make scene-assets`, which sets it"
        )
    try:
        corpus = load_corpus(corpus_dir)
    except FileNotFoundError:
        raise SystemExit(
            f"no {CORPUS_MANIFEST} in {corpus_dir}: point --corpus at the "
            "published dataset root, or pass --source archive"
        ) from None
    return scenes_from_corpus(index, corpus, corpus_dir, pcap_dir)


def published(site):
    return os.path.exists(os.path.join(SCENES, site, ".rebuilt"))


def report_status(scenes, problems):
    """What a rebuild would do, without starting one."""
    done = [s for s in scenes if published(s["site"])]
    outstanding = [s for s in scenes if not published(s["site"])]
    minutes = sum(s["minutes"] for s in outstanding)
    for scene in scenes:
        mark = "published" if published(scene["site"]) else "OUTSTANDING"
        print(f"  {mark:<11} {scene['site']:<24} {scene['minutes']:>6.1f} min")
    for problem in problems:
        print(f"  UNRESOLVED  {problem}")
    print(
        f"\n{len(done)} published, {len(outstanding)} outstanding, "
        f"{len(problems)} unresolved"
    )
    if outstanding:
        # At half speed a replay takes twice the recording, and the export adds
        # a little on top. Two and a bit is close enough to plan an evening by.
        print(f"outstanding work is about {minutes * 2.2 / 60:.1f} h of replay")
    return 1 if problems else 0


def main():
    parser = argparse.ArgumentParser(
        description="Publish the web scene assets for the S2 archive sites."
    )
    parser.add_argument("sites", nargs="*", help="site ids; default is all of them")
    parser.add_argument(
        "--source",
        choices=("corpus", "archive"),
        default=os.environ.get("SCENE_SOURCE", "corpus"),
        help="trimmed dataset captures (default) or the original rolling ones",
    )
    parser.add_argument(
        "--corpus",
        default=CORPUS_DIR,
        help="published dataset root holding manifest.json (env S2_CORPUS_DIR)",
    )
    parser.add_argument(
        "--pcap-dir",
        default=PCAP_DIR,
        help="the server's replay directory (env LIDAR_PCAP_DIR)",
    )
    parser.add_argument(
        "--status",
        action="store_true",
        help="report what a rebuild would do and exit without replaying",
    )
    parser.add_argument(
        "--force",
        action="store_true",
        help="rebuild scenes that already carry a .rebuilt marker",
    )
    args = parser.parse_args()

    records = db_problem()
    scenes, problems = plan(args.source, args.sites, args.corpus, args.pcap_dir)
    if args.status:
        status = report_status(scenes, problems)
        if records:
            print(f"  {records}")
        return 1 if records else status
    if records:
        log(f"  {records}")
        log("the run records are unreadable, so no export could finish; nothing ran")
        return 1
    if problems:
        for problem in problems:
            log(f"  UNRESOLVED {problem}")
        log(f"{len(problems)} site(s) could not be resolved to packets; nothing ran")
        return 1
    if args.force:
        for scene in scenes:
            marker = os.path.join(SCENES, scene["site"], ".rebuilt")
            if os.path.exists(marker):
                os.remove(marker)

    log(
        f"{len(scenes)} scene(s) [{args.source}]: {', '.join(s['site'] for s in scenes)}"
    )
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
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
