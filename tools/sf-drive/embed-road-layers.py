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


def bake(source, base, quest, output):
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
    tiles_out = output / "tiles"
    tiles_out.mkdir(parents=True)
    for tile in manifest["tiles"]:
        token = tile["t"]
        if token not in by_tile:
            continue
        name = f"{token}.json.gz"
        raw = (base / name).read_bytes()
        if digest(raw) != roads["tiles"][token]["base_tile_gzip_sha256"]:
            raise ValueError(f"Base tile checksum differs for {token}")
        content = json.loads(gzip.decompress(raw))
        content["raisedRoads"] = by_tile[token]
        payload = gzip.compress(encoded(content), compresslevel=9, mtime=0)
        (tiles_out / name).write_bytes(payload)
        tile["bytes"] = len(payload)
    manifest["roadBake"] = {
        "revision": roads["revision"],
        "baseSha256": digest(raw_base),
        "driveSha256": digest(drives),
        "driveSourceRevision": drive_data["source_revision"],
        "runs": run_count,
        **counts,
    }
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
    args = cli.parse_args()
    bake(args.source, args.base, args.quest, args.output)
