#!/usr/bin/env python3
"""Publish a verified Gerardus road revision inside the bundled SF tiles."""

import argparse
from collections import defaultdict
from concurrent.futures import ThreadPoolExecutor
import gzip
from hashlib import sha256
import json
import math
from pathlib import Path
from urllib.parse import urljoin, urlsplit
from urllib.request import urlopen


def digest(raw):
    return sha256(raw).hexdigest()


def encoded(value):
    return (json.dumps(value, separators=(",", ":"), allow_nan=False) + "\n").encode()


def read_source(source, name):
    if urlsplit(source).scheme in ("http", "https"):
        with urlopen(urljoin(source.rstrip("/") + "/", name), timeout=30) as response:
            return response.read()
    return (Path(source) / name).read_bytes()


def checked(source, name, entry):
    raw = read_source(source, name)
    if len(raw) != entry["bytes"] or digest(raw) != entry["sha256"]:
        raise ValueError(f"Road source checksum differs for {name}")
    return raw


def patch_source_rows(rows, patches):
    """Apply all replacements against the original, ordered tile records."""
    replacements = {}
    for patch in patches:
        index = patch["index"]
        if (
            type(index) is not int
            or not 0 <= index < len(rows)
            or index in replacements
            or digest(encoded(rows[index])) != patch["sha256"]
            or not isinstance(patch["parts"], list)
        ):
            raise ValueError(f"Tile patch does not match source row {index}")
        replacements[index] = patch["parts"]
    return [
        part
        for index, row in enumerate(rows)
        for part in replacements.get(index, [row])
    ]


def supported_runs(rows):
    """Join adjacent sections only at a shared, supported source station."""
    points = []
    owners, profiles = set(), set()
    end = None
    for owner, row in sorted(rows, key=lambda entry: entry[1]["source_chainage_m"]):
        start = row["source_chainage_m"]
        profile = row["profile"]
        for station in profile:
            point = station["position"]
            chainage = start + station["s_m"]
            broken = point[2] is None or (
                points
                and (
                    chainage < end - 0.05
                    or (station is profile[0] and abs(chainage - end) > 0.05)
                    or (station is profile[0] and math.dist(point, points[-1]) > 0.15)
                )
            )
            if broken:
                if len(points) > 1:
                    yield points, sorted(owners), sorted(profiles)
                points, owners, profiles = [], set(), set()
            if point[2] is None:
                continue
            # The same endpoint can occur in both source tiles.
            if not points or math.dist(point, points[-1]) > 0.001:
                points.append(point)
            owners.add(owner)
            profiles.add(row["id"])
            end = chainage
    if len(points) > 1:
        yield points, sorted(owners), sorted(profiles)


def valid_width_transitions(road):
    seams = road.get("seams", {})
    transitions = road.get("widthTransitionMetres", {})
    if not isinstance(seams, dict) or not isinstance(transitions, dict):
        return False
    if not transitions.keys() <= seams.keys() <= {"start", "end"}:
        return False
    for label, seam in seams.items():
        if (
            not math.isfinite(seam["bearing"])
            or not 2.5 <= seam["width"] <= 30
            or (seam["width"] > road["width"] and label not in transitions)
        ):
            return False
    if not all(math.isfinite(length) and length > 0 for length in transitions.values()):
        return False
    if not transitions:
        return True
    points = road["points"]
    length = sum(math.dist(a[:2], b[:2]) for a, b in zip(points, points[1:]))
    return sum(transitions.values()) <= length


def playable_product(path, sha256, base_hash, revision, manifest):
    raw = path.read_bytes()
    if digest(raw) != sha256:
        raise ValueError("Playable road product checksum differs")
    product = json.loads(raw)
    if (
        product["format"] != "sf-game-road-contact"
        or product["version"] != 1
        or product["base_sha256"] != base_hash
        or product["road_revision"] != revision
    ):
        raise ValueError("Playable roads belong to a different bake")
    tiles = manifest["tiles"]

    def owner(point):
        x, y, _ = point
        return min(tiles, key=lambda t: (x - t["x"]) ** 2 + (y - t["y"]) ** 2)["t"]

    endpoints = {}
    by_tile = defaultdict(list)
    replaced = set()
    for road in product["roads"]:
        points = road["points"]
        rail_sides = road.get("railSides", [])
        if (
            not isinstance(rail_sides, list)
            or any(side not in ("left", "right") for side in rail_sides)
            or len(set(rail_sides)) != len(rail_sides)
        ):
            raise ValueError(f"Invalid playable road guard rails for {road['id']}")
        if (
            len(points) < 3
            or any(
                len(point) != 3 or not all(map(math.isfinite, point))
                for point in points
            )
            or not 2.5 <= road["width"] <= 30
            or road["kind"] not in {"ramp", "bridge_deck", "tunnel"}
            or not set(road["groundEnds"]) <= {"start", "end"}
            or road["owners"] != [owner(points[0]), owner(points[-1])]
            or not valid_width_transitions(road)
        ):
            raise ValueError(f"Invalid playable road {road['id']}")
        for label, point in (("start", points[0]), ("end", points[-1])):
            terminal = f"{road['id']}:{label}"
            if terminal in endpoints:
                raise ValueError(f"Duplicate playable terminal {terminal}")
            endpoints[terminal] = (point, road.get("seams", {}).get(label))
        for token in sorted({owner(point) for point in points}):
            by_tile[token].append(road)
        replaced.update(road["profiles"])
    for transition in product["transitions"]:
        (start, seam_a), (end, seam_b) = (
            endpoints[transition["from"]],
            endpoints[transition["to"]],
        )
        if (
            transition["status"] != "joined"
            or math.dist(start, end) > 0.002
            or seam_a is None
            or seam_a != seam_b
        ):
            raise ValueError(f"Open playable join {transition['id']}")
    base_tiles = {tile["t"] for tile in tiles}
    release = {
        "playableSha256": sha256,
        "playableTransitions": product["transitions"],
        "playableReplaces": sorted(replaced),
    }
    by_tile_patches = defaultdict(dict)
    for field, product_key, manifest_key in (
        ("b", "overhead_building_patches", "overheadBuildingPatches"),
        ("r", "covered_ground_road_patches", "coveredGroundRoadPatches"),
        ("tr", "inferred_tree_patches", "inferredTreePatches"),
    ):
        patches = product.get(product_key, {})
        # Scenery from an adjacent tile can reach the road.
        if not patches.keys() <= base_tiles:
            raise ValueError(f"{product_key} must name base tiles")
        if patches:
            release[manifest_key] = {
                token: len(records) for token, records in patches.items()
            }
            for token, records in patches.items():
                by_tile_patches[token][field] = records
    return by_tile, by_tile_patches, release


def with_drive_approaches(drives, path, expected_sha256):
    raw = path.read_bytes()
    if digest(raw) != expected_sha256:
        raise ValueError("Game drive approaches checksum differs")
    extension = json.loads(raw)
    if extension["format"] != "sf-game-drive-approaches" or extension[
        "base_drive_sha256"
    ] != digest(drives):
        raise ValueError("Game drive approaches need their pinned parent")
    payload = json.loads(drives)
    drive = next(
        (row for row in payload["drives"] if row["id"] == extension["drive_id"]), None
    )
    if drive is None:
        raise ValueError("Game drive approaches reference an unknown drive")
    approaches = extension["approaches"]
    if any(type(row.get("replace_sections", False)) is not bool for row in approaches):
        raise ValueError("Game drive replace_sections must be a boolean")
    ids = {row["id"] for row in approaches}
    if len(ids) != len(approaches) or drive["id"] in ids:
        raise ValueError("Game drive approaches need distinct start IDs")
    drive["approaches"] = approaches
    return encoded(payload), [
        {"id": row["id"], "label": row["label"]} for row in approaches
    ]


def bake(
    source,
    base,
    quest,
    output,
    playable=None,
    playable_sha256=None,
    approaches=None,
    approaches_sha256=None,
):
    raw_base = (base / "manifest.json").read_bytes()
    manifest = json.loads(raw_base)
    roads = json.loads(read_source(source, "manifest.json"))
    if (
        roads["format"] != "gerardus-sf-city-road-layers"
        or roads["compatibility_version"] != 2
        or roads["base_manifest_sha256"] != digest(raw_base)
        or roads["origin_epsg7131"] != manifest["origin_epsg7131"]
    ):
        raise ValueError(
            "Road bake and pristine base map must share a revision and frame"
        )
    drives = checked(source, "gamedrives.json", roads["files"]["gamedrives.json"])
    starts = []
    if approaches is not None:
        drives, starts = with_drive_approaches(drives, approaches, approaches_sha256)
    drive_data = json.loads(drives)
    if drive_data["base_manifest_sha256"] != digest(raw_base):
        raise ValueError("The game route belongs to a different base map")
    groups = defaultdict(list)

    def load(token):
        return json.loads(checked(source, f"{token}.json", roads["tiles"][token]))

    counts = defaultdict(int)
    with ThreadPoolExecutor(max_workers=8) as pool:
        for tile in pool.map(load, sorted(roads["tiles"])):
            for layer in ("overpass", "bridge"):
                counts[layer] += len(tile[layer])
                for row in tile[layer]:
                    key = (layer, row["cnn"], row["source_direction"])
                    groups[key].append((tile["l13"], row))
    by_tile = defaultdict(list)
    run_count = 0
    for (layer, cnn, direction), rows in sorted(groups.items()):
        for index, (points, owners, profiles) in enumerate(supported_runs(rows)):
            run = {
                "id": f"{layer}/{cnn}/{direction}/{index}",
                "kind": layer,
                "points": points,
                "profiles": profiles,
            }
            # Store a complete run with every owner it intersects. A neighbour
            # can be absent or delayed without truncating the rendered road.
            tokens = set(owners)
            for x, y, _ in points:
                tile = min(
                    manifest["tiles"],
                    key=lambda t: (x - t["x"]) ** 2 + (y - t["y"]) ** 2,
                )
                tokens.add(tile["t"])
            for token in sorted(tokens):
                by_tile[token].append(run)
            run_count += 1
    playable_tiles, tile_patches, playable_index = {}, {}, {}
    if playable is not None:
        playable_tiles, tile_patches, playable_index = playable_product(
            playable, playable_sha256, digest(raw_base), roads["revision"], manifest
        )
    tiles_out = output / "tiles"
    tiles_out.mkdir(parents=True)
    for tile in manifest["tiles"]:
        token = tile["t"]
        name = f"{token}.json.gz"
        raw = (base / name).read_bytes()
        if digest(raw) != roads["tiles"][token]["base_tile_gzip_sha256"]:
            raise ValueError(f"Base tile checksum differs for {token}")
        payload = raw
        if any(token in group for group in (by_tile, playable_tiles, tile_patches)):
            content = json.loads(gzip.decompress(raw))
            if token in by_tile:
                content["raisedRoads"] = [
                    *content.get("raisedRoads", []),
                    *by_tile[token],
                ]
            if token in playable_tiles:
                content["playableRoads"] = playable_tiles[token]
            for field, patches in tile_patches.get(token, {}).items():
                content[field] = patch_source_rows(content[field], patches)
            payload = gzip.compress(encoded(content), compresslevel=9, mtime=0)
        (tiles_out / name).write_bytes(payload)
        tile["bytes"] = len(payload)
        reviewed = manifest.get("terrainReview", {}).get("tiles", {}).get(token)
        if reviewed:
            reviewed["sha256"] = digest(payload)
    manifest["roadBake"] = {
        "revision": roads["revision"],
        "baseSha256": digest(raw_base),
        "driveSha256": digest(drives),
        "driveSourceRevision": drive_data["source_revision"],
        "runs": run_count,
        **counts,
        **playable_index,
    }
    if starts:
        manifest["roadBake"]["driveApproaches"] = starts
    bundled_manifest = encoded(manifest)
    (tiles_out / "manifest.json").write_bytes(bundled_manifest)
    route = json.loads(quest.read_bytes())
    route["manifestSha256"] = digest(bundled_manifest)
    (output / "quest-route.json").write_text(
        json.dumps(route, indent=2, ensure_ascii=False, allow_nan=False) + "\n",
        encoding="utf-8",
    )
    (output / "gamedrives.json").write_bytes(drives)
    print(f"{dict(counts)} => {run_count} supported runs in {len(by_tile)} game tiles")


if __name__ == "__main__":
    cli = argparse.ArgumentParser(description=__doc__)
    cli.add_argument("--source", required=True)
    cli.add_argument("--base", type=Path, required=True)
    cli.add_argument("--quest", type=Path, required=True)
    cli.add_argument("--output", type=Path, required=True)
    cli.add_argument("--playable", type=Path)
    cli.add_argument("--playable-sha256")
    cli.add_argument("--approaches", type=Path)
    cli.add_argument("--approaches-sha256")
    args = cli.parse_args()
    if bool(args.playable) != bool(args.playable_sha256):
        cli.error("--playable and --playable-sha256 must be provided together")
    if bool(args.approaches) != bool(args.approaches_sha256):
        cli.error("--approaches and --approaches-sha256 must be provided together")
    bake(
        args.source,
        args.base,
        args.quest,
        args.output,
        args.playable,
        args.playable_sha256,
        args.approaches,
        args.approaches_sha256,
    )
