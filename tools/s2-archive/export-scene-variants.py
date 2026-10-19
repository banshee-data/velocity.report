#!/usr/bin/env python3
"""Export one window of a site as a web scene once for each tuning config of a set.

The plan is docs/plans/lidar-scene-variant-exports-plan.md. Prefer the Makefile,
which supplies the replay settings a published scene is made with and checks the
binary and the server first:

    make dev-go-lidar LIDAR_PCAP_DIR=/Volumes/lidar/lidar/pcaps     # in another shell
    make scene-variants-status SET=tools/s2-archive/scene-variants/geometry-convergence.json
    make scene-variants SET=tools/s2-archive/scene-variants/geometry-convergence.json

A set file names its variants by tuning config, each a path relative to the
server's config directory exactly as the :8081 replay form's dropdown names it,
and the sites and windows to replay. Every variant is the same packets replayed
through publish-scenes.py's own replay with the variant's tuning_config, and the
same exporter at the same settings, written in the layout of a published scene's
assets/ so the scene player opens it unchanged.

A variant may also name a "trail": "online" (the default, the tracker's
posterior) or "smoothed" (the whole-track RTS smoother over the replay's
filter record, `velocity scene export --smoothed-trail`). Two variants with
the same tuning_config share one recording at each site: the second is
exported from the first's VRLOG rather than replayed again, because a replay
can carry state from the one before it on the same server process (perf0002),
and an export option is a comparison of exports, not of replays.

Nothing here is a second replay path or a second exporter: publish-scenes.py is
loaded as a module, the way its tests load it, and its plan(), replay(),
export(), write_assets_manifest() and swap_assets() are the ones called.

The output is local. It goes under SCENE_VARIANTS_DIR, outside the checkout:

    <out>/<set_id>/set.json                      the set manifest
    <out>/<set_id>/<site>/<variant>/assets/      one scene export per variant
    <out>/<set_id>/<site>/compare/<variant>.json vrlog-compare against the baseline
    <out>/<set_id>/recipes/<site>/<variant>.json scene-capture recipes

Resumable: a variant whose .done marker carries the fingerprint its file has
now is skipped. A marker with another fingerprint means the file was edited
since, and the variant is made again.
"""

import argparse
import hashlib
import importlib.util
import json
import os
import re
import shutil
import subprocess
import sys
import time

HERE = os.path.dirname(os.path.abspath(__file__))

# The script is named with a hyphen, so it is not importable by name.
_spec = importlib.util.spec_from_file_location(
    "publish_scenes", os.path.join(HERE, "publish-scenes.py")
)
publish = importlib.util.module_from_spec(_spec)
sys.modules.setdefault("publish_scenes", publish)
_spec.loader.exec_module(publish)

REPO = publish.REPO
log = publish.log

SET_VERSION = 1
SLUG = re.compile(r"^[a-z0-9][a-z0-9_-]{0,63}$")
# The export drops this many frames as the background model's transient, as a
# published scene does, so a window is replayed from that long before it.
FRAME_RATE_HZ = 10.0
WARMUP_SECONDS = publish.TRANSIENT_FRAMES / FRAME_RATE_HZ
# A replay at the published settings takes about this many times its length,
# export included; publish-scenes.py plans an evening by the same figure.
REPLAY_FACTOR = 2.2
MARKER = ".done"


class SetError(Exception):
    """A set file that cannot be run as written."""


# =============================================================================
# THE SET FILE
# =============================================================================


def config_path_problem(rel):
    """Why a tuning_config path would be refused, or None.

    The same rules the server's resolveTuningConfigPath applies, checked here
    so a typo is reported with every other problem rather than as a 400 an
    hour into a batch.
    """
    if not isinstance(rel, str) or not rel.strip():
        return "is empty"
    rel = rel.strip()
    if os.path.isabs(rel) or rel.startswith("/"):
        return "must be relative to the server's config directory"
    clean = os.path.normpath(rel).replace(os.sep, "/")
    if clean in (".", "..") or clean.startswith("../"):
        return "leaves the config directory"
    if not clean.lower().endswith(".json"):
        return "is not a .json file"
    return None


def _number(value):
    return isinstance(value, (int, float)) and not isinstance(value, bool)


def validate_set(raw):
    """Check a set file and return it normalised, or raise SetError naming every problem."""
    problems = []
    if not isinstance(raw, dict):
        raise SetError("a set file must be a JSON object")
    if raw.get("version") != SET_VERSION:
        problems.append(f"version must be {SET_VERSION}, got {raw.get('version')!r}")
    set_id = raw.get("set_id")
    if not isinstance(set_id, str) or not SLUG.match(set_id):
        problems.append(f"set_id {set_id!r} must be a lower-case slug")

    variants = []
    raw_variants = raw.get("variants")
    if not isinstance(raw_variants, list) or not raw_variants:
        problems.append("variants must be a non-empty list")
        raw_variants = []
    seen = set()
    for index, entry in enumerate(raw_variants):
        where = f"variants[{index}]"
        if not isinstance(entry, dict):
            problems.append(f"{where} must be an object")
            continue
        variant_id = entry.get("id")
        if not isinstance(variant_id, str) or not SLUG.match(variant_id):
            problems.append(f"{where}: id {variant_id!r} must be a lower-case slug")
            continue
        if variant_id in seen:
            problems.append(f"{where}: id {variant_id!r} is used twice")
            continue
        seen.add(variant_id)
        # Every variant names a file. "The server's current parameters" would
        # be the previous variant's after the first replay, so a set with one
        # would mean different things depending on its order.
        tuning_config = entry.get("tuning_config")
        problem = config_path_problem(tuning_config)
        if problem:
            problems.append(f"{where} ({variant_id}): tuning_config {problem}")
            continue
        label = entry.get("label") or variant_id
        trail = entry.get("trail", "online")
        problem = publish.trail_problem(trail)
        if problem:
            problems.append(f"{where} ({variant_id}): trail {problem}")
            continue
        variants.append(
            {
                "id": variant_id,
                "label": str(label),
                "tuning_config": os.path.normpath(tuning_config.strip()).replace(
                    os.sep, "/"
                ),
                "trail": trail,
            }
        )

    baseline = raw.get("baseline")
    if baseline not in seen:
        problems.append(f"baseline {baseline!r} is not one of the variants")

    sites = []
    raw_sites = raw.get("sites")
    if not isinstance(raw_sites, list) or not raw_sites:
        problems.append("sites must be a non-empty list")
        raw_sites = []
    seen_sites = set()
    for index, entry in enumerate(raw_sites):
        where = f"sites[{index}]"
        if isinstance(entry, str):
            entry = {"site": entry}
        if not isinstance(entry, dict) or not isinstance(entry.get("site"), str):
            problems.append(f"{where} must name a site")
            continue
        site = entry["site"]
        if site in seen_sites:
            problems.append(f"{where}: {site} is listed twice")
            continue
        seen_sites.add(site)
        offset, duration = entry.get("offset_seconds"), entry.get("duration_seconds")
        if (offset is None) != (duration is None):
            problems.append(
                f"{where} ({site}): give both offset_seconds and duration_seconds, or neither"
            )
            continue
        if offset is not None:
            if not _number(offset) or offset < 0:
                problems.append(f"{where} ({site}): offset_seconds must be >= 0")
                continue
            if not _number(duration) or duration <= 0:
                problems.append(f"{where} ({site}): duration_seconds must be > 0")
                continue
        sites.append(
            {"site": site, "offset_seconds": offset, "duration_seconds": duration}
        )

    if problems:
        raise SetError("\n".join(problems))
    return {
        "version": SET_VERSION,
        "set_id": set_id,
        "description": str(raw.get("description") or ""),
        "baseline": baseline,
        "variants": variants,
        "sites": sites,
    }


def load_set(path):
    """Read and validate a set file; the digest is of its exact bytes."""
    with open(path, "rb") as fh:
        data = fh.read()
    try:
        raw = json.loads(data)
    except json.JSONDecodeError as error:
        raise SetError(f"{path} is not valid JSON: {error}") from None
    spec = validate_set(raw)
    spec["path"] = os.path.relpath(os.path.abspath(path), REPO)
    spec["digest"] = "sha256:" + hashlib.sha256(data).hexdigest()
    return spec


# =============================================================================
# THE WINDOW
# =============================================================================


def windowed(scene, site_spec):
    """The scene to replay for a site: the whole site, or a window and its warm-up.

    A window is measured from the first packet of the site's capture. The replay
    starts WARMUP_SECONDS earlier and the export drops those frames, exactly as
    a published scene drops its opening transient.
    """
    offset = site_spec.get("offset_seconds")
    if offset is None:
        return dict(scene, window=None)
    duration = site_spec["duration_seconds"]
    site = scene["site"]
    if offset < WARMUP_SECONDS:
        raise SetError(
            f"{site}: a window at {offset:g} s leaves less than the {WARMUP_SECONDS:g} s "
            "warm-up the export drops before it"
        )
    if offset + duration > scene["duration"] + 1e-6:
        raise SetError(
            f"{site}: the window {offset:g} to {offset + duration:g} s runs past the "
            f"capture's {scene['duration']:.1f} s"
        )
    replayed = duration + WARMUP_SECONDS
    return dict(
        scene,
        start_secs=scene["start_secs"] + offset - WARMUP_SECONDS,
        duration=replayed,
        minutes=replayed / 60.0,
        window={
            "offset_seconds": offset,
            "duration_seconds": duration,
            "warmup_seconds": WARMUP_SECONDS,
        },
    )


# =============================================================================
# CONFIGS
# =============================================================================


def check_configs(variants, listing):
    """Every variant's file as GET /api/lidar/configs lists it.

    Returns the problems and each applicable variant's listed fingerprint. A
    file the listing does not have, that is not a tuning config, or that the
    running server cannot apply, is refused before anything is replayed.
    """
    by_path = {entry.get("path"): entry for entry in listing.get("configs") or []}
    config_dir = listing.get("config_dir") or "the server's config directory"
    problems, fingerprints = [], {}
    for variant in variants:
        path = variant["tuning_config"]
        entry = by_path.get(path)
        if entry is None:
            problems.append(f"{variant['id']}: {path} is not in {config_dir}")
        elif entry.get("kind") != "tuning":
            problems.append(
                f"{variant['id']}: {path} is not a tuning config: {entry.get('error', '')}"
            )
        elif not entry.get("applicable"):
            problems.append(
                f"{variant['id']}: {path} cannot be applied to the running server: "
                f"{entry.get('reason', 'no reason given')}"
            )
        else:
            fingerprints[variant["id"]] = entry.get("fingerprint", "")
    return problems, fingerprints


def fetch_configs():
    return publish.api("/configs", timeout=30)


# =============================================================================
# ONE VARIANT
# =============================================================================


def variant_dir(out_root, set_id, site, variant_id):
    return os.path.join(out_root, set_id, site, variant_id)


def read_marker(vdir):
    try:
        with open(os.path.join(vdir, MARKER)) as fh:
            return json.load(fh)
    except (FileNotFoundError, json.JSONDecodeError):
        return None


def is_done(vdir, fingerprint):
    """Made, and made from the file as it is now."""
    marker = read_marker(vdir)
    return (
        marker is not None
        and marker.get("fingerprint") == fingerprint
        and os.path.exists(os.path.join(vdir, "assets", "manifest.json"))
    )


def record_from_disk(variant, vdir):
    """A finished variant's record, read back from its marker and export header."""
    marker = read_marker(vdir) or {}
    with open(os.path.join(vdir, "assets", "part-000", "header.json")) as fh:
        header = json.load(fh)
    return {
        "id": variant["id"],
        "label": variant["label"],
        "tuning_config": variant["tuning_config"],
        "trail": variant.get("trail", "online"),
        "trail_estimator": header.get("trail_estimator", ""),
        "recording_of": marker.get("recording_of", ""),
        "listed_fingerprint": marker.get("fingerprint", ""),
        "applied_fingerprint": marker.get("applied_fingerprint", ""),
        "run_id": marker.get("run_id", ""),
        "vrlog": marker.get("vrlog", ""),
        "source_vrlog_sha256": header.get("source_vrlog_sha256", ""),
        "tuning_hash": header.get("tuning_hash", ""),
        "build_version": header.get("build_version", ""),
        "frame_count": header.get("frame_count"),
        "duration_sec": header.get("duration_sec"),
        "minutes_taken": marker.get("minutes_taken"),
        "assets": os.path.join(vdir, "assets"),
    }


def shared_recording(spec, variant, site, out_root):
    """A finished variant's recording this one can be exported from, or None.

    Variants with one tuning_config are one replay: the first made at a site
    records it, and the others export its VRLOG. Only a variant listed before
    this one, done at this site with the same file and its recording still on
    disk, is reused; the marker names which, and the set checks that the two
    exports came from one recording.
    """
    if spec is None:
        return None
    for other in spec["variants"]:
        if other["id"] == variant["id"]:
            break
        if other["tuning_config"] != variant["tuning_config"]:
            continue
        marker = read_marker(variant_dir(out_root, spec["set_id"], site, other["id"]))
        vrlog = (marker or {}).get("vrlog", "")
        if vrlog and os.path.exists(os.path.join(vrlog, "header.json")):
            return {
                "vrlog": vrlog,
                "run_id": marker.get("run_id", ""),
                "recording_of": other["id"],
                "applied_fingerprint": marker.get("applied_fingerprint", ""),
            }
    return None


def export_variant(scene, variant, vdir, listed_fingerprint, spec=None, out_root=None):
    """Replay the scene under the variant's file and export it as a web scene.

    Built beside the variant's assets and swapped in only when whole, as a
    published scene is, so a failed variant leaves the last good one in place.
    A variant whose tuning_config an earlier variant has already replayed at
    this site is exported from that recording (shared_recording).
    """
    site, title = scene["site"], scene["title"]
    live = os.path.join(vdir, "assets")
    assets = live + ".new"
    part = os.path.join(assets, "part-000")
    shutil.rmtree(assets, ignore_errors=True)
    os.makedirs(assets, exist_ok=True)

    started = time.time()
    shared = shared_recording(spec, variant, site, out_root)
    if shared:
        log(
            f"  exporting the recording of {shared['recording_of']} rather than replaying"
        )
        ran = {"vrlog": shared["vrlog"], "run_id": shared["run_id"], "start": {}}
        echoed = shared["applied_fingerprint"]
    else:
        ran = publish.replay(scene, tuning_config=variant["tuning_config"])
        echoed_path = ran["start"].get("tuning_config", "")
        echoed = ran["start"].get("tuning_fingerprint", "")
        if echoed_path != variant["tuning_config"]:
            raise RuntimeError(
                f"the server says it applied {echoed_path!r}, not {variant['tuning_config']!r}"
            )
        if echoed != listed_fingerprint:
            raise RuntimeError(
                f"the server applied fingerprint {echoed!r} where the listing said "
                f"{listed_fingerprint!r}: the file changed during the batch"
            )

    scene_title = f"{title}: {variant['label']}"
    summary = publish.export(
        ran["vrlog"],
        part,
        site,
        scene_title,
        extra=["--start-frame", str(publish.TRANSIENT_FRAMES)],
        trail=variant.get("trail", "online"),
    )
    for line in summary.splitlines()[1:]:
        log("      " + line.strip())
    publish.export(
        ran["vrlog"],
        os.path.join(assets, "background"),
        site,
        scene_title,
        kind="background",
    )
    publish.write_assets_manifest(assets, site, scene_title)
    publish.swap_assets(assets, live)

    minutes = (time.time() - started) / 60.0
    with open(os.path.join(vdir, MARKER), "w") as fh:
        json.dump(
            {
                "tuning_config": variant["tuning_config"],
                "trail": variant.get("trail", "online"),
                "fingerprint": listed_fingerprint,
                "applied_fingerprint": echoed,
                "run_id": ran["run_id"],
                "vrlog": ran["vrlog"],
                "recording_of": shared["recording_of"] if shared else "",
                "minutes_taken": round(minutes, 2),
            },
            fh,
            indent=2,
        )
        fh.write("\n")
    return record_from_disk(variant, vdir)


# =============================================================================
# THE SET
# =============================================================================


def set_checks(records_by_site):
    """What makes the set's comparison void, whatever each variant looked like."""
    problems = []
    for site, records in records_by_site.items():
        by_hash = {}
        by_id = {record["id"]: record for record in records}
        for record in records:
            tuning_hash = record.get("tuning_hash")
            if not tuning_hash:
                problems.append(
                    f"{site}/{record['id']}: the export records no tuning_hash"
                )
                continue
            # Two variants may share a tuning hash only as two exports of
            # one recording that draw different trails; two of one trail are
            # the same scene twice.
            key = (tuning_hash, record.get("trail_estimator", ""))
            if key in by_hash:
                problems.append(
                    f"{site}: {record['id']} and {by_hash[key]} exported the same "
                    f"tuning_hash {tuning_hash} and trail, so one file's changes did not reach the pipeline"
                )
            else:
                by_hash[key] = record["id"]
            source = by_id.get(record.get("recording_of") or "")
            if source and source.get("source_vrlog_sha256") != record.get(
                "source_vrlog_sha256"
            ):
                problems.append(
                    f"{site}: {record['id']} was exported from {source['id']}'s recording "
                    "but names a different source_vrlog_sha256"
                )
    builds = sorted(
        {
            record.get("build_version", "")
            for records in records_by_site.values()
            for record in records
        }
    )
    if len(builds) > 1:
        problems.append(
            f"the variants were exported by different builds: {', '.join(builds)}"
        )
    return problems


def write_json(path, document):
    """Written whole or not at all, so an interrupted batch leaves the last good set.json."""
    os.makedirs(os.path.dirname(path), exist_ok=True)
    temporary = path + ".tmp"
    with open(temporary, "w") as fh:
        json.dump(document, fh, indent=2)
        fh.write("\n")
    os.replace(temporary, path)


def parse_version(text):
    """The version and git sha from `velocity version`, as scene-assets-preflight reads them.

    The command prints `<binary>  v<VERSION>`, then ` git sha:  <sha>` and
    `   built:  <time>`.
    """
    build = {}
    for line in text.splitlines():
        words = line.split()
        if not words:
            continue
        if line.strip().startswith("git sha:"):
            build["git_sha"] = words[-1]
        elif line.strip().startswith("built:"):
            build["built"] = line.split(":", 1)[1].strip()
        elif "version" not in build and words[-1].startswith("v"):
            build["version"] = words[-1][1:]
    return build


def velocity_build():
    """The build the exports are stamped with: ./velocity version."""
    try:
        done = subprocess.run(
            [publish.VELOCITY, "version"],
            capture_output=True,
            text=True,
            cwd=REPO,
            timeout=30,
        )
    except (OSError, subprocess.TimeoutExpired):
        return {}
    return parse_version(done.stdout)


def compare_to_baseline(spec, records_by_site, out_root):
    """make vrlog-compare for each variant against the baseline, beside the set."""
    for site, records in records_by_site.items():
        base = next((r for r in records if r["id"] == spec["baseline"]), None)
        if not base or not base.get("vrlog"):
            log(f"  {site}: no baseline recording to compare against")
            continue
        for record in records:
            if record["id"] == spec["baseline"] or not record.get("vrlog"):
                continue
            out = os.path.join(
                out_root, spec["set_id"], site, "compare", f"{record['id']}.json"
            )
            os.makedirs(os.path.dirname(out), exist_ok=True)
            done = subprocess.run(
                [
                    "make",
                    "--no-print-directory",
                    "vrlog-compare",
                    f"VRLOG_A={base['vrlog']}",
                    f"VRLOG_B={record['vrlog']}",
                    f"COMPARE_OUT={out}",
                ],
                cwd=REPO,
                capture_output=True,
                text=True,
            )
            if done.returncode != 0:
                tail = (done.stderr or done.stdout).strip().splitlines()[-1:]
                log(f"  {site}/{record['id']}: compare failed: {tail}")
                continue
            record["compare"] = out


def write_recipes(spec, base_recipe_path, out_root):
    """One scene-capture recipe per variant: the base recipe with source and output changed.

    The frame and the views stay the base recipe's, so every variant is seen at
    the same instant from the same cameras. Returns the recipes written.
    """
    with open(base_recipe_path) as fh:
        base = json.load(fh)
    written = []
    for site_spec in spec["sites"]:
        site = site_spec["site"]
        for variant in spec["variants"]:
            vdir = variant_dir(out_root, spec["set_id"], site, variant["id"])
            recipe = json.loads(json.dumps(base))
            recipe["source"] = os.path.abspath(os.path.join(vdir, "assets"))
            output = dict(recipe.get("output") or {})
            output["dir"] = os.path.abspath(
                os.path.join(out_root, spec["set_id"], "stills", site, variant["id"])
            )
            recipe["output"] = output
            path = os.path.join(
                out_root, spec["set_id"], "recipes", site, f"{variant['id']}.json"
            )
            write_json(path, recipe)
            written.append(path)
    return written


def out_root_problem(out_root):
    """Why the output root cannot be used, or None. A set is local output, never in the checkout."""
    if not out_root:
        return "no output directory: set SCENE_VARIANTS_DIR or pass --out; the Makefile sets it"
    absolute = os.path.realpath(os.path.abspath(out_root))
    repo = os.path.realpath(REPO)
    if absolute == repo or absolute.startswith(repo + os.sep):
        return (
            f"{out_root} is inside the checkout. A set is local output and is never "
            "committed: put it outside, as SCENE_VARIANTS_DIR does by default"
        )
    return None


# =============================================================================
# MAIN
# =============================================================================


def resolve(spec, args):
    """Every site in the set as the scene to replay, and every problem found doing so."""
    wanted = [s["site"] for s in spec["sites"]]
    if args.sites:
        unknown = [name for name in args.sites if name not in wanted]
        if unknown:
            raise SystemExit(f"not in the set: {', '.join(unknown)}")
        wanted = [name for name in wanted if name in args.sites]
    scenes, problems = publish.plan(args.source, wanted, args.corpus, args.pcap_dir)
    by_site = {scene["site"]: scene for scene in scenes}
    resolved = []
    for site_spec in spec["sites"]:
        scene = by_site.get(site_spec["site"])
        if scene is None:
            continue
        try:
            resolved.append(windowed(scene, site_spec))
        except SetError as error:
            problems.append(str(error))
    return resolved, problems


def report_status(spec, scenes, problems, fingerprints, out_root):
    outstanding_minutes = 0.0
    print(f"set {spec['set_id']} ({spec['path']}, {spec['digest'][:19]})")
    for scene in scenes:
        window = scene.get("window")
        span = (
            f"{window['offset_seconds']:g} to "
            f"{window['offset_seconds'] + window['duration_seconds']:g} s"
            if window
            else "whole site"
        )
        print(
            f"  {scene['site']}: {span}, {scene['minutes']:.1f} min replayed per variant"
        )
        for variant in spec["variants"]:
            vdir = variant_dir(out_root, spec["set_id"], scene["site"], variant["id"])
            fingerprint = fingerprints.get(variant["id"])
            if fingerprint is not None and is_done(vdir, fingerprint):
                mark = "done"
            elif fingerprint is None and read_marker(vdir):
                mark = "made (unchecked)"
            else:
                mark = "OUTSTANDING"
                outstanding_minutes += scene["minutes"]
            trail = (
                ""
                if variant.get("trail", "online") == "online"
                else "  trail: smoothed"
            )
            print(
                f"    {mark:<16} {variant['id']:<16} {variant['tuning_config']}{trail}"
            )
    for problem in problems:
        print(f"  PROBLEM  {problem}")
    if outstanding_minutes:
        hours = outstanding_minutes * REPLAY_FACTOR / 60
        print(f"outstanding work is about {hours:.1f} h of replay")
    return 1 if problems else 0


def main(argv=None):
    parser = argparse.ArgumentParser(
        description="Export a site window once per tuning config of a set, as web scenes."
    )
    parser.add_argument(
        "set", help="the set file, e.g. tools/s2-archive/scene-variants/<set>.json"
    )
    parser.add_argument("--sites", nargs="*", help="only these sites of the set")
    parser.add_argument(
        "--out",
        default=os.environ.get("SCENE_VARIANTS_DIR", ""),
        help="output root, outside the checkout (env SCENE_VARIANTS_DIR)",
    )
    parser.add_argument(
        "--source",
        choices=("corpus", "archive"),
        default=os.environ.get("SCENE_SOURCE", "corpus"),
        help="where the packets come from, as for publish-scenes.py",
    )
    parser.add_argument(
        "--corpus", default=publish.CORPUS_DIR, help="env S2_CORPUS_DIR"
    )
    parser.add_argument(
        "--pcap-dir", default=publish.PCAP_DIR, help="env LIDAR_PCAP_DIR"
    )
    parser.add_argument(
        "--status", action="store_true", help="report and exit without replaying"
    )
    parser.add_argument("--force", action="store_true", help="remake every variant")
    parser.add_argument(
        "--compare",
        action="store_true",
        help="vrlog-compare each variant against the baseline",
    )
    parser.add_argument(
        "--recipes",
        metavar="BASE_RECIPE",
        help="write one scene-capture recipe per variant from this base recipe, and exit",
    )
    args = parser.parse_args(argv)

    try:
        spec = load_set(args.set)
    except (OSError, SetError) as error:
        log(f"the set cannot be run:\n{error}")
        return 1
    problem = out_root_problem(args.out)
    if problem:
        log(problem)
        return 1
    out_root = os.path.abspath(args.out)

    if args.recipes:
        for path in write_recipes(spec, args.recipes, out_root):
            print(path)
        return 0

    scenes, problems = resolve(spec, args)
    db_issue = publish.db_problem()
    if db_issue:
        problems.append(db_issue)
    try:
        listing = fetch_configs()
    except (
        Exception
    ) as error:  # the server not answering is a problem to report, not a crash
        listing = None
        problems.append(f"the configs could not be listed: {error}")
    fingerprints = {}
    if listing is not None:
        config_problems, fingerprints = check_configs(spec["variants"], listing)
        problems += config_problems

    if args.status:
        return report_status(spec, scenes, problems, fingerprints, out_root)
    if problems:
        for problem in problems:
            log(f"  PROBLEM {problem}")
        log(f"{len(problems)} problem(s) found before any replay; nothing ran")
        return 1

    set_root = os.path.join(out_root, spec["set_id"])
    manifest_path = os.path.join(set_root, "set.json")
    document = {
        "version": SET_VERSION,
        "set": {
            key: spec[key]
            for key in ("set_id", "path", "digest", "description", "baseline")
        },
        "build": velocity_build(),
        "replay": {
            "api": publish.API,
            "speed_mode": publish.SPEED_MODE,
            "speed_ratio": publish.SPEED_RATIO,
            "settle_before_recording": publish.SETTLE,
            "source": args.source,
            "stride": int(publish.STRIDE),
            "transient_frames": publish.TRANSIENT_FRAMES,
        },
        "server_config_before": listing.get("active", {}),
        "sites": [],
        "problems": [],
    }
    records_by_site, failures = {}, []
    for scene in scenes:
        site = scene["site"]
        site_doc = {
            "site": site,
            "title": scene["title"],
            "files": scene["files"],
            "start_seconds": scene["start_secs"],
            "replayed_seconds": scene["duration"],
            "window": scene.get("window"),
            "variants": [],
        }
        document["sites"].append(site_doc)
        records = records_by_site.setdefault(site, [])
        for variant in spec["variants"]:
            vdir = variant_dir(out_root, spec["set_id"], site, variant["id"])
            os.makedirs(vdir, exist_ok=True)
            fingerprint = fingerprints[variant["id"]]
            marker = os.path.join(vdir, MARKER)
            if args.force and os.path.exists(marker):
                os.remove(marker)
            if is_done(vdir, fingerprint):
                log(f"{site}/{variant['id']}: already made from this file, skipping")
                record = record_from_disk(variant, vdir)
            else:
                log(
                    f"=== {site}/{variant['id']}: {variant['tuning_config']} "
                    f"({fingerprint}), {scene['minutes']:.1f} min ==="
                )
                try:
                    record = export_variant(
                        scene, variant, vdir, fingerprint, spec=spec, out_root=out_root
                    )
                except Exception as error:  # one bad variant must not end the set
                    log(f"  FAILED {site}/{variant['id']}: {error}")
                    failures.append(f"{site}/{variant['id']}: {error}")
                    continue
                log(
                    f"  {site}/{variant['id']} done in {record['minutes_taken']:.1f} min"
                )
            records.append(record)
            site_doc["variants"] = records
            write_json(manifest_path, document)

    if args.compare:
        compare_to_baseline(spec, records_by_site, out_root)
    document["problems"] = set_checks(records_by_site) + failures
    try:
        document["server_config_after"] = fetch_configs().get("active", {})
    except Exception as error:
        document["server_config_after"] = {"error": str(error)}
    write_json(manifest_path, document)

    after = document["server_config_after"]
    log(
        "the server is left on "
        f"{after.get('path') or '(its startup config)'} {after.get('fingerprint', '')}"
    )
    for problem in document["problems"]:
        log(f"  PROBLEM {problem}")
    log(f"set written: {manifest_path}")
    return 1 if document["problems"] else 0


if __name__ == "__main__":
    sys.exit(main())
