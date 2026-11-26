#!/usr/bin/env python3
"""Apply a surveyed terrain detail rectangle to its exact game release."""

import argparse
import gzip
import json
from pathlib import Path
from runpy import run_path

TOOLS = Path(__file__).parent
LAYERS = run_path(str(TOOLS / "embed-road-layers.py"))
RELEASE = run_path(str(TOOLS / "apply-tree-classification.py"))


def bake(source, review, output):
    digest = RELEASE["digest"]
    manifest_raw = (source / "tiles/manifest.json").read_bytes()
    manifest = json.loads(manifest_raw)
    review_raw = review.read_bytes()
    patch = json.loads(review_raw)
    if (
        patch["format"] != "sf-game-terrain-detail/v1"
        or patch["tile_manifest_sha256"] != digest(manifest_raw)
    ):
        raise ValueError("Terrain detail needs its pinned game release")
    token = patch["tile"]
    entry = next((entry for entry in manifest["tiles"] if entry["t"] == token), None)
    if entry is None:
        raise ValueError("Terrain detail must name an existing tile")

    raw = (source / "tiles" / f"{token}.json.gz").read_bytes()
    tile = json.loads(gzip.decompress(raw))
    tile["g"] = LAYERS["patch_terrain_detail"](tile["g"], patch, digest(raw))
    packed = gzip.compress(RELEASE["encode"](tile), compresslevel=9, mtime=0)
    RELEASE["record_tile"](manifest, entry, packed)
    manifest["roadBake"].setdefault("terrainDetailReviews", []).append(
        {
            "tile": token,
            "sha256": digest(packed),
            "parentSha256": digest(raw),
            "parentManifestSha256": patch["tile_manifest_sha256"],
            "surveySha256": patch["survey_sha256"],
            "patchSha256": digest(review_raw),
        }
    )
    return RELEASE["write_release"](
        source, output, manifest, manifest_raw, {token: packed}, carry=True
    )


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("source", "review", "output"):
        parser.add_argument(name, type=Path)
    args = parser.parse_args()
    print(bake(args.source, args.review, args.output))


if __name__ == "__main__":
    main()
