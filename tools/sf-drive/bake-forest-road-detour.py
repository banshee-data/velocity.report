#!/usr/bin/env python3
"""Apply the reviewed Forest Road and Sims Road pavement to their game tile."""

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
    review = product.pop("forest_road_detour_review")
    if (
        review["parent_manifest_sha256"] != digest(parent_raw)
        or review["parent_contact_sha256"] != digest(prior_raw)
        or road_bake["playableSha256"] != digest(prior_raw)
    ):
        raise ValueError("The detour needs its reviewed parent")
    prior = json.loads(prior_raw)
    treasure, forest = review["ground_row_patches"]
    patches = prior["covered_ground_road_patches"][TOKEN]
    prior_treasure = next(
        patch for patch in patches if patch["index"] == treasure["index"]
    )
    prior_treasure["parts"] = treasure["parts"]
    patches.append(forest)
    if product != prior:
        raise ValueError("The detour changed an unreviewed structure")
    tile_entry = next(tile for tile in manifest["tiles"] if tile["t"] == TOKEN)
    tile_raw = (parent / "tiles" / f"{TOKEN}.json.gz").read_bytes()
    if review["parent_tile_sha256"] != digest(tile_raw):
        raise ValueError("The reviewed detour tile has changed")
    tile = json.loads(gzip.decompress(tile_raw))
    tile["n"].extend(review["road_names"])
    tile["r"] = embed["patch_source_rows"](tile["r"], [treasure, forest])
    tree_patch = review["tree_row_patch"]
    tile["tr"] = embed["patch_source_rows"](tile["tr"], [tree_patch])
    packed = gzip.compress(encode(tile), compresslevel=9, mtime=0)
    release["record_tile"](manifest, tile_entry, packed)
    road_bake["playableSha256"] = digest(contact_raw)
    road_bake["coveredGroundRoadPatches"][TOKEN] += 1
    road_bake["forestRoadDetour"] = {
        "parentManifestSha256": review["parent_manifest_sha256"],
        "alignmentSha256": review["alignment_sha256"],
        "orthophotoReceiptSha256": review["orthophoto_receipt_sha256"],
        "sourceOsmWays": review["source_osm_ways"],
        "groundRowSha256": [
            digest(encode(row))
            for patch in (treasure, forest)
            for row in patch["parts"]
        ],
        "treeRowSha256": tree_patch["sha256"],
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
