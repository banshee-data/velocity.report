#!/usr/bin/env python3
"""Apply a reviewed Gerardus building clearance to an exact SF game bake."""

import argparse
import gzip
import json
from pathlib import Path
from runpy import run_path

TOOLS = Path(__file__).parent
ROWS = run_path(str(TOOLS / "embed-road-layers.py"))
RELEASE = run_path(str(TOOLS / "apply-tree-classification.py"))


def bake(source, review, output):
    digest = RELEASE["digest"]
    manifest_raw = (source / "tiles/manifest.json").read_bytes()
    product_raw = review.read_bytes()
    parent = digest(manifest_raw)
    manifest = json.loads(manifest_raw)
    product = json.loads(product_raw)
    if (
        product["format"] != "sf-baked-building-clearance/v1"
        or product["tile_manifest_sha256"] != parent
    ):
        raise ValueError("Building clearance needs its pinned game release")

    entries = {entry["t"]: entry for entry in manifest["tiles"]}
    reviews = product["tiles"]
    if not reviews or not reviews.keys() <= entries.keys():
        raise ValueError("Building clearance must name existing game tiles")
    changed = {}
    tile_receipts = {}
    for token, record in reviews.items():
        raw = (source / "tiles" / f"{token}.json.gz").read_bytes()
        if digest(raw) != record["sha256"] or not record["patches"]:
            raise ValueError(f"Building clearance does not match tile {token}")
        tile = json.loads(gzip.decompress(raw))
        tile["b"] = ROWS["patch_source_rows"](tile["b"], record["patches"])
        packed = gzip.compress(RELEASE["encode"](tile), compresslevel=9, mtime=0)
        RELEASE["record_tile"](manifest, entries[token], packed)
        changed[token] = packed
        tile_receipts[token] = {
            "parentSha256": record["sha256"],
            "sha256": digest(packed),
        }

    manifest["roadBake"].setdefault("buildingClearanceReviews", []).append(
        {
            "parentManifestSha256": parent,
            "reviewSha256": digest(product_raw),
            "surveyIndexSha256": product["ept_index_sha256"],
            "surveyNodesSha256": product["survey_nodes_sha256"],
            "tiles": tile_receipts,
        }
    )
    return RELEASE["write_release"](
        source, output, manifest, manifest_raw, changed, carry=True
    )


def main():
    cli = argparse.ArgumentParser(description=__doc__)
    for name in ("source", "review", "output"):
        cli.add_argument(name, type=Path)
    args = cli.parse_args()
    print(bake(args.source, args.review, args.output))


if __name__ == "__main__":
    main()
