#!/usr/bin/env python3
"""Count every track sample stored in scene exports, without a range cutoff.

Uses the same XY anchor as the player: a complete box centre, falling back to
x/y. Counts are per scene part and include every class and support state. The
report contains no track identifiers or per-track positions. Existing scene
exports may omit source frames and do not necessarily record whether a sample
was observed or predicted; this tool does not reconstruct absent information.
"""

import argparse
import collections
import gzip
import hashlib
import json
import math
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
DEFAULT_SCENES = REPO / "public_html" / "src" / "scenes"
# Comparisons only: these values never filter input or set an output bound.
COMPARISON_RADII = (60, 70, 80, 100, 150, 170)


def finite(value):
    return (
        isinstance(value, (int, float))
        and not isinstance(value, bool)
        and math.isfinite(value)
    )


def anchor(track):
    """The position used by scene-player's trailAnchor, or a data error."""
    if finite(track.get("bx")) and finite(track.get("by")):
        return track["bx"], track["by"], "box_center"
    if finite(track.get("x")) and finite(track.get("y")):
        return track["x"], track["y"], "position"
    raise ValueError("track has no finite XY trail anchor")


def inventory_scene(manifest_path, scene_root):
    """Read each indexed frame, returning aggregate statistics and input paths."""
    manifest_path = manifest_path.resolve()
    assets = manifest_path.parent
    site = assets.parent.name
    files = [manifest_path]
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    parts = manifest.get("parts")
    if not isinstance(parts, list) or not parts:
        raise ValueError(f"{site}: manifest has no parts")

    result = {
        "site": site,
        "frames": 0,
        "samples": 0,
        "max_range_m": 0.0,
        "track_ids": 0,
        "samples_beyond": collections.Counter(),
        "tracks_beyond": collections.Counter(),
        "support": collections.Counter(),
        "anchor": collections.Counter(),
    }
    for part_info in parts:
        part = (assets / part_info["url"]).resolve()
        if not part.is_relative_to(assets):
            raise ValueError(f"{site}: part path leaves the scene")
        header_path, index_path = part / "header.json", part / "index.json"
        files.extend((header_path, index_path))
        header = json.loads(header_path.read_text(encoding="utf-8"))
        index = json.loads(index_path.read_text(encoding="utf-8"))
        chunks = index.get("chunks")
        if not isinstance(chunks, list) or not chunks:
            raise ValueError(f"{site}: part has no indexed chunks")
        chunk_ids = [c["c"] for c in chunks]
        if len(set(chunk_ids)) != len(chunk_ids):
            raise ValueError(f"{site}: repeated chunk in index")
        part_frames = 0
        track_ids = set()
        beyond = {r: set() for r in COMPARISON_RADII}
        for chunk in chunks:
            path = part / "frames" / f"chunk_{chunk['c']:04d}.ndjson.gz"
            files.append(path)
            count = 0
            with gzip.open(path, "rt", encoding="utf-8") as stream:
                for line in stream:
                    if not line.strip():
                        continue
                    frame = json.loads(line)
                    count += 1
                    part_frames += 1
                    seen = set()
                    tracks = frame.get("tr")
                    if tracks is None:
                        tracks = []
                    if not isinstance(tracks, list):
                        raise ValueError(f"{site}: frame has an invalid track list")
                    for track in tracks:
                        track_id = track.get("id")
                        if (
                            not isinstance(track_id, str)
                            or not track_id
                            or track_id in seen
                        ):
                            raise ValueError(
                                f"{site}: missing or repeated track ID in a frame"
                            )
                        seen.add(track_id)
                        try:
                            x, y, source = anchor(track)
                        except ValueError as error:
                            raise ValueError(f"{site}: {error}") from error
                        distance = math.hypot(x, y)
                        result["samples"] += 1
                        result["anchor"][source] += 1
                        result["support"][track.get("support") or "unknown"] += 1
                        result["max_range_m"] = max(result["max_range_m"], distance)
                        track_ids.add(track_id)
                        for radius in COMPARISON_RADII:
                            if distance > radius:
                                result["samples_beyond"][radius] += 1
                                beyond[radius].add(track_id)
            if count != chunk.get("n"):
                raise ValueError(
                    f"{site}: chunk {chunk['c']} count differs from its index"
                )
        if part_frames != header.get("frame_count"):
            raise ValueError(f"{site}: part frame count differs from its header")
        result["frames"] += part_frames
        result["track_ids"] += len(track_ids)
        for radius, ids in beyond.items():
            result["tracks_beyond"][radius] += len(ids)

    # Refuse accidental input outside the requested inventory tree.
    for file in files:
        if not file.resolve().is_relative_to(scene_root.resolve()):
            raise ValueError(f"{site}: input path leaves the scene directory")
    return result, files


def inventory(scene_root, requested=()):
    """Read all scenes or named scenes and fingerprint every indexed input."""
    scene_root = Path(scene_root).resolve()
    manifests = sorted(scene_root.glob("*/assets/manifest.json"))
    if requested:
        wanted = set(requested)
        manifests = [p for p in manifests if p.parent.parent.name in wanted]
        missing = wanted - {p.parent.parent.name for p in manifests}
        if missing:
            raise ValueError("no scene manifest for: " + ", ".join(sorted(missing)))
    if not manifests:
        raise ValueError("no scene manifests found")
    rows, files = [], []
    for path in manifests:
        row, inputs = inventory_scene(path, scene_root)
        rows.append(row)
        files.extend(inputs)
    if len(set(files)) != len(files):
        raise ValueError("a scene input was referenced more than once")
    digest = hashlib.sha256()
    for path in sorted(files, key=lambda p: p.relative_to(scene_root).as_posix()):
        digest.update(path.relative_to(scene_root).as_posix().encode("utf-8"))
        digest.update(b"\0")
        digest.update(hashlib.sha256(path.read_bytes()).digest())
    return rows, len(files), digest.hexdigest()


def render(rows, file_count, fingerprint):
    print(
        f"Scenes: {len(rows)}; indexed input files: {file_count}; file-set SHA-256: {fingerprint}"
    )
    print(
        f"Frames: {sum(r['frames'] for r in rows):,}; "
        f"track samples: {sum(r['samples'] for r in rows):,}; "
        f"part-local track IDs: {sum(r['track_ids'] for r in rows):,}"
    )
    supports, anchors = collections.Counter(), collections.Counter()
    for row in rows:
        supports.update(row["support"])
        anchors.update(row["anchor"])
    print(
        f"Support: {dict(sorted(supports.items()))}; anchors: {dict(sorted(anchors.items()))}"
    )
    print("\nRadius comparison (strictly beyond; no data excluded):")
    print("metres | scenes | track IDs | samples")
    for radius in COMPARISON_RADII:
        scenes = sum(r["max_range_m"] > radius for r in rows)
        tracks = sum(r["tracks_beyond"][radius] for r in rows)
        samples = sum(r["samples_beyond"][radius] for r in rows)
        print(f"{radius} | {scenes} | {tracks} | {samples}")
    print(
        "\nscene | frames | samples | track IDs | maximum metres | track IDs beyond 100 m"
    )
    for row in rows:
        print(
            f"{row['site']} | {row['frames']} | {row['samples']} | {row['track_ids']} | "
            f"{row['max_range_m']:.2f} | {row['tracks_beyond'][100]}"
        )


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "sites", nargs="*", help="scene IDs; default is every scene with a manifest"
    )
    parser.add_argument("--scenes-dir", type=Path, default=DEFAULT_SCENES)
    args = parser.parse_args()
    try:
        render(*inventory(args.scenes_dir, args.sites))
    except (OSError, ValueError, KeyError, json.JSONDecodeError) as error:
        parser.exit(1, f"scene trail inventory: {error}\n")


if __name__ == "__main__":
    main()
