#!/usr/bin/env python3
"""Apply a Gerardus classified-tree patch to a pinned baked SF release."""

import argparse
import gzip
import hashlib
import json
from pathlib import Path
import shutil


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def encode(value):
    return (json.dumps(value, separators=(",", ":"), allow_nan=False) + "\n").encode()


def bake(release, patch, output):
    tiles = release / "tiles"
    parent_raw = (tiles / "manifest.json").read_bytes()
    parent = digest(parent_raw)
    manifest = json.loads(parent_raw)
    route_raw = (release / "quest-route.json").read_bytes()
    route = json.loads(route_raw)
    product_raw = patch.read_bytes()
    product = json.loads(product_raw)
    if (
        product["format"] != "sf-inferred-tree-classification/v1"
        or product["tile_manifest_sha256"] != parent
        or route["manifestSha256"] != parent
    ):
        raise ValueError(
            "Tree classification and quest route need their pinned game release"
        )
    patches = product["inferred_tree_patches"]
    if not patches.keys() <= {row["t"] for row in manifest["tiles"]}:
        raise ValueError("Tree classification references an unknown tile")

    changed = {}
    release_receipts = {}
    for tile in manifest["tiles"]:
        token = tile["t"]
        if token not in patches:
            continue
        raw_tile = (tiles / f"{token}.json.gz").read_bytes()
        data = json.loads(gzip.decompress(raw_tile))
        removed = set()
        for row in patches[token]:
            index = row["index"]
            if (
                type(index) is not int
                or index in removed
                or not 0 <= index < len(data["tr"])
                or digest(encode(data["tr"][index])) != row["sha256"]
                or row["parts"]
            ):
                raise ValueError(f"Tree row does not match at {token}/{index}")
            removed.add(index)
        data["tr"] = [
            row for index, row in enumerate(data["tr"]) if index not in removed
        ]
        payload = gzip.compress(encode(data), compresslevel=9, mtime=0)
        changed[token] = payload
        release_receipts[token] = {
            "parentSha256": digest(raw_tile),
            "sha256": digest(payload),
            "removed": len(removed),
        }
        tile["bytes"] = len(payload)
        reviewed = manifest.get("terrainReview", {}).get("tiles", {}).get(token)
        if reviewed:
            reviewed["sha256"] = digest(payload)

    manifest["roadBake"]["treeClassification"] = {
        "parentManifestSha256": parent,
        "classificationSha256": digest(product_raw),
        "surveyIndexSha256": product["ept_sha256"],
        "surveyNodesSha256": product["ept_nodes_sha256"],
        "streetTreeInventorySha256": product["tree_inventory_sha256"],
        "tiles": release_receipts,
    }
    manifest_raw = encode(manifest)
    if route_raw.count(parent.encode()) != 1:
        raise ValueError("Quest manifest binding must occur exactly once")
    route_raw = route_raw.replace(parent.encode(), digest(manifest_raw).encode(), 1)
    (output / "tiles").mkdir(parents=True)
    for tile in manifest["tiles"]:
        token = tile["t"]
        name = f"{token}.json.gz"
        dest = output / "tiles" / name
        if token in changed:
            dest.write_bytes(changed[token])
        else:
            shutil.copyfile(tiles / name, dest)
    (output / "tiles" / "manifest.json").write_bytes(manifest_raw)
    (output / "quest-route.json").write_bytes(route_raw)
    for name in ("gamedrives.json", "coverage.json"):
        shutil.copyfile(release / name, output / name)
    print(digest(manifest_raw), release_receipts)


if __name__ == "__main__":
    cli = argparse.ArgumentParser(description=__doc__)
    cli.add_argument("release", type=Path)
    cli.add_argument("patch", type=Path)
    cli.add_argument("output", type=Path)
    args = cli.parse_args()
    bake(args.release, args.patch, args.output)
