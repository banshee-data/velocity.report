#!/usr/bin/env python3
"""Apply the surveyed ground approach to the existing Treasure Island deck."""

import argparse
import gzip
import json
from pathlib import Path
from runpy import run_path

TOKEN = "80858024"
TOOLS = Path(__file__).parent


def bake(parent, contact, output):
    embed = run_path(str(TOOLS / "embed-road-layers.py"))
    release = run_path(str(TOOLS / "apply-tree-classification.py"))
    digest, encode = release["digest"], release["encode"]
    parent_raw = (parent / "tiles/manifest.json").read_bytes()
    manifest = json.loads(parent_raw)
    road_bake = manifest["roadBake"]
    prior_raw = (parent / "structures.json").read_bytes()
    contact_raw = contact.read_bytes()
    product = json.loads(contact_raw)
    review = product.pop("treasure_island_ground_approach")
    if (
        review["parent_game_sha256"] != digest(parent_raw)
        or review["parent_contact_sha256"] != digest(prior_raw)
        or road_bake["playableSha256"] != digest(prior_raw)
    ):
        raise ValueError("The Treasure Island approach needs its reviewed parent")
    patches = product["covered_ground_road_patches"]
    patch = patches[TOKEN].pop()
    if len(patch["parts"]) != 1 or product != json.loads(prior_raw):
        raise ValueError("The approach must replace exactly one ground road row")
    tile_entry = next(tile for tile in manifest["tiles"] if tile["t"] == TOKEN)
    tile_raw = (parent / "tiles" / f"{TOKEN}.json.gz").read_bytes()
    if review["parent_tile_sha256"] != digest(tile_raw):
        raise ValueError("The reviewed Treasure Island tile has changed")
    tile = json.loads(gzip.decompress(tile_raw))
    tile["r"] = embed["patch_source_rows"](tile["r"], [patch])
    packed = gzip.compress(encode(tile), compresslevel=9, mtime=0)
    release["record_tile"](manifest, tile_entry, packed)
    road_bake["playableSha256"] = digest(contact_raw)
    road_bake["coveredGroundRoadPatches"][TOKEN] += 1
    road_bake["treasureIslandGroundApproach"] = {
        **review,
        "sourceRowSha256": patch["sha256"],
        "approachRowSha256": digest(encode(patch["parts"][0])),
        "tiles": {TOKEN: {"parentSha256": digest(tile_raw), "sha256": digest(packed)}},
    }
    return release["write_release"](
        parent, output, manifest, parent_raw, {TOKEN: packed}, contact_raw, carry=True
    )


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("parent", "contact", "output"):
        parser.add_argument(name, type=Path)
    args = parser.parse_args()
    print(bake(args.parent, args.contact, args.output))


if __name__ == "__main__":
    main()
