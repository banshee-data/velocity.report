#!/usr/bin/env python3
"""Refit city road layers to the reviewed game terrain and retain the evidence chain."""

import argparse
from collections import Counter
import gzip
from hashlib import sha256
import json
import math
from pathlib import Path
import tempfile

import numpy as np
from scipy.spatial import cKDTree

# DataSF has separate TUNL centreline records. Tunnel Avenue is an ordinary street.
TUNNELS = {
    "3186101": "Robert C Levy Tunnel",
    "3186201": "Robert C Levy Tunnel",
    "12225000": "Stockton Tunnel",
    "19939000": "Toll Plaza Tunnel",
}
MISSING = -32768
LAYERS = ("road", "overpass", "bridge")


def digest(raw):
    return sha256(raw).hexdigest()


def encoded(value):
    return (
        json.dumps(value, sort_keys=True, separators=(",", ":"), allow_nan=False) + "\n"
    ).encode()


def describe(raw):
    return {"bytes": len(raw), "sha256": digest(raw)}


def checked(path, expected):
    raw = path.read_bytes()
    if describe(raw) != {k: expected[k] for k in ("bytes", "sha256")}:
        raise ValueError(f"Product hash or size differs at {path}")
    return json.loads(raw)


def prepare_base(bundle, original, layers, output):
    manifest_raw = (bundle / "manifest.json").read_bytes()
    manifest = json.loads(manifest_raw)
    original_raw = (original / "manifest.json").read_bytes()
    old_bake = manifest.pop("roadBake")
    if (
        old_bake["baseSha256"] != layers["base_manifest_sha256"]
        or digest(original_raw) != old_bake["baseSha256"]
    ):
        raise ValueError("The parent city layers and bundled provenance differ")
    if manifest["origin_epsg7131"] != layers["origin_epsg7131"]:
        raise ValueError("The base tiles use a different coordinate frame")
    if {t["t"] for t in manifest["tiles"]} != set(layers["base_tiles"]):
        raise ValueError("The two city footprints differ")

    review = manifest["terrainReview"]
    retained_runs = set(review["addedRoads"])
    renamed = Counter()
    grids = []
    hashes = {}
    parents = {}
    output.mkdir(parents=True, exist_ok=True)
    for entry in manifest["tiles"]:
        token = entry["t"]
        filename = f"{token}.json.gz"
        before = (original / filename).read_bytes()
        if digest(before) != layers["tiles"][token]["base_tile_gzip_sha256"]:
            raise ValueError(f"Parent base tile changed at {token}")
        raw = (bundle / filename).read_bytes()
        if len(raw) != entry["bytes"]:
            raise ValueError(f"Unexpected bundled tile length at {token}")
        tile = json.loads(gzip.decompress(raw))
        before_tile = json.loads(gzip.decompress(before))
        kept = [
            road for road in tile.pop("raisedRoads", []) if road["id"] in retained_runs
        ]
        if token in review["tiles"]:
            if digest(raw) != review["tiles"][token]["sha256"]:
                raise ValueError(f"The bundled review differs at {token}")
            parents[token] = digest(raw)
        elif tile != before_tile:
            raise ValueError(
                f"Unexpected unreviewed differences in bundled tile {token}"
            )
        if kept:
            tile["raisedRoads"] = kept
        names = tile["n"]
        roads = [row for row in tile["r"] if names[row[0]] not in TUNNELS.values()]
        if len(roads) != len(tile["r"]):
            renamed[token] = len(tile["r"]) - len(roads)
            tile["r"] = roads
        new_raw = (
            before
            if tile == before_tile
            else gzip.compress(encoded(tile), compresslevel=9, mtime=0)
        )
        (output / filename).write_bytes(new_raw)
        entry["bytes"] = len(new_raw)
        hashes[token] = digest(new_raw)
        if token in review["tiles"]:
            review["tiles"][token]["sha256"] = hashes[token]
        grid = tile["g"]
        grid["z"] = np.asarray(grid["z"], dtype=np.int16)
        grids.append((tile["o"], grid))
    manifest["terrainRebake"] = {
        "parentGameManifestSha256": digest(manifest_raw),
        "progressBaseSha256": manifest.get("terrainRebake", {}).get(
            "progressBaseSha256", old_bake["baseSha256"]
        ),
        "parentRoadRevision": layers["revision"],
        "retainedReviewTileSha256": parents,
        "removedTunnelRows": dict(renamed),
    }
    data = encoded(manifest)
    (output / "manifest.json").write_bytes(data)
    print(
        f"Built {len(grids)} compatibility-base tiles; omitted {sum(renamed.values())} aerially draped tunnel rows",
        flush=True,
    )
    return data, manifest, hashes, grids


def ground_at(record, x, y):
    (ox, oy), grid = record
    x, y = x - ox, y - oy
    detail = grid.get("detail")
    if detail and (
        detail["x0"] <= x < detail["x0"] + (detail["columns"] - 1) * detail["s"]
        and detail["y0"] <= y < detail["y0"] + (detail["rows"] - 1) * detail["s"]
    ):
        grid = detail
    columns, rows = grid.get("columns", grid.get("n")), grid.get("rows", grid.get("n"))
    fx, fy = (x - grid["x0"]) / grid["s"], (y - grid["y0"]) / grid["s"]
    if not (0 <= fx <= columns - 1 and 0 <= fy <= rows - 1):
        return None
    i, j = min(int(fx), columns - 2), min(int(fy), rows - 2)
    u, v = fx - i, fy - j
    k = j * columns + i
    z = [grid["z"][k + offset] for offset in (0, 1, columns, columns + 1)]
    if MISSING in z:
        return None
    weights = (1 - u - v, u, v, 0) if u + v <= 1 else (0, 1 - v, 1 - u, u + v - 1)
    return round(float(sum(h * w for h, w in zip(z, weights)) / 10), 3)


def station_height(road, station, ground):
    x, y, z = station["position"]
    if z is None or road["cnn"] in TUNNELS:
        return None
    # The reviewed 21st Street centreline over the embankment carries no contact.
    if road["name"] == "21ST ST" and 4701.5 < x < 4729 and -72 < y < -60:
        return None
    if any(
        a <= station["s_m"] <= b for a, b in road.get("unresolved_ground_ranges_m", [])
    ):
        return None
    return ground_at(ground, x, y)


def retain_join(join, roads):
    first, last = (roads[join[key]] for key in ("from_surface_id", "to_surface_id"))
    x, y, z = first["profile"][-1]["position"]
    xx, yy, zz = last["profile"][0]["position"]
    return (
        z is not None
        and zz is not None
        and math.hypot(x - xx, y - yy) <= 0.1002
        and abs(z - zz) <= 0.10
    )


def verify_regional_base(source, token, tiles, before, after):
    for review in tiles["regional"]:
        geometry = source.parent / review["revision"] / "geometry"
        manifest = json.loads((geometry / "manifest.json").read_bytes())
        referenced = manifest.get("base_tiles", manifest["tiles"])
        if isinstance(referenced, list):
            referenced = [t["t"] if isinstance(t, dict) else t for t in referenced]
        if token not in referenced or any(
            before[t]["base_tile_gzip_sha256"] != after[t] for t in referenced
        ):
            raise ValueError(
                f"Regional evidence uses changed base data: {review['revision']}"
            )


def rebake(bundle, original, source, output):
    original_manifest = json.loads((source / "manifest.json").read_bytes())
    layers = original_manifest.copy()
    parent = layers.pop("revision")
    if (
        layers["format"] != "gerardus-sf-city-road-layers"
        or layers["compatibility_version"] != 2
        or parent != "sf-city-roads-" + digest(encoded(layers))
    ):
        raise ValueError("City-layer provenance differs")
    if set(layers["files"]) != {"sources.json", "candidates.json"}:
        raise ValueError("Use the road layer source before adding the game drive")
    base_raw, base, hashes, grids = prepare_base(
        bundle, original, original_manifest, output / "base"
    )
    owners = cKDTree([(t["x"], t["y"]) for t in base["tiles"]])
    metadata = checked(source / "sources.json", layers["files"]["sources.json"])
    candidates = checked(source / "candidates.json", layers["files"]["candidates.json"])
    tunnel_candidates = {}
    tiles = {}
    roads = {}
    revisions = {}
    counts = Counter()
    for token, info in layers["tiles"].items():
        tile = checked(source / f"{token}.json", info)
        verify_regional_base(source, token, tile, layers["tiles"], hashes)
        tiles[token] = tile
        for road in tile["road"]:
            if road["cnn"] in TUNNELS:
                cnn = road["cnn"]
                structure = TUNNELS[cnn].lower().replace(" ", "-")
                candidate = {
                    "candidate_id": f"sf-tunnel-candidate/{cnn}",
                    "structure_id": f"sf-tunnel/{structure}",
                    "datasf_cnn": cnn,
                    "datasf_nodes": road["datasf_nodes"],
                    "name": road["name"],
                    "status": "unresolved",
                    "reason": "Named tunnel centreline; interior, portal contact and clearance are unobserved",
                    "owner": token,
                    "surface_id": road["id"],
                    "evidence": road["evidence"],
                }
                road.update(
                    kind="tunnel",
                    structure_id=candidate["structure_id"],
                    quality="aerial_interior_unobserved",
                    routing_status="requires_review",
                )
                tunnel_candidates[cnn] = candidate
                tile["unresolved"].append(candidate)
                counts["tunnel_alignments"] += 1
            _, nearest = owners.query([s["position"][:2] for s in road["profile"]])
            for station, index in zip(road["profile"], nearest):
                height = station_height(road, station, grids[index])
                station["position"][2] = height
                station["support"] = "missing" if height is None else "interpolated"
                counts[station["support"]] += 1
            roads[road["id"]] = road
            revisions[road["id"]] = digest(encoded(road))
    transitions = set()
    for tile in tiles.values():
        for cover in tile["coverings"]:
            if cover["id"] in revisions:
                cover["revision"] = revisions[cover["id"]]
        tile["transitions"] = [j for j in tile["transitions"] if retain_join(j, roads)]
        transitions.update(j["id"] for j in tile["transitions"])
    layers["counts"] = {**layers["counts"], "ground_continuations": len(transitions)}
    layers["base_manifest_sha256"] = digest(base_raw)
    layers["support_contract"] = (
        "Ground matches game triangles. Unresolved ground, tunnels and class-17 candidates require evidence before routing."
    )
    layers["parent_road_revision"] = parent
    layers["builder_sha256"] = digest(Path(__file__).read_bytes())
    metadata["ground_rebake"] = {
        "parent_city_revision": parent,
        "parent_city_builder_sha256": original_manifest["builder_sha256"],
        "regional_evidence_base_sha256": original_manifest["base_manifest_sha256"],
        "support_counts": dict(counts),
        "tunnel_source_cnns": TUNNELS,
    }
    references = {}
    output.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(dir=output) as stage:
        path = Path(stage) / "revision"
        path.mkdir()
        if any(row["datasf_cnn"] in tunnel_candidates for row in candidates):
            raise ValueError(
                "The parent already classifies a reviewed tunnel candidate"
            )
        files = {
            "sources.json": encoded(metadata),
            "candidates.json": encoded([*candidates, *tunnel_candidates.values()]),
        }
        for name, raw in files.items():
            (path / name).write_bytes(raw)
        layers["files"] = {name: describe(raw) for name, raw in files.items()}
        for token, tile in tiles.items():
            payload = encoded(tile)
            (path / f"{token}.json").write_bytes(payload)
            references[token] = {
                **describe(payload),
                "counts": {k: len(v) for k, v in tile.items() if k != "l13"},
                "base_tile_gzip_sha256": hashes[token],
            }
        layers["tiles"] = references
        revision = "sf-city-roads-" + digest(encoded(layers))
        layers["revision"] = revision
        (path / "manifest.json").write_bytes(encoded(layers))
        destination = output / revision
        if not destination.exists():
            path.rename(destination)
        elif (path / "manifest.json").read_bytes() != (
            destination / "manifest.json"
        ).read_bytes():
            raise ValueError("Immutable rebake differs from existing output")
    print(
        f"{len(roads)} road records; {len(transitions)} ground joins; {dict(counts)}",
        flush=True,
    )
    print(destination, flush=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    for option in ("bundle", "original", "source", "output"):
        parser.add_argument(f"--{option}", type=Path, required=True)
    args = parser.parse_args()
    rebake(args.bundle, args.original, args.source, args.output)
