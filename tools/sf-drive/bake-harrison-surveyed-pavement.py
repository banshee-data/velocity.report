#!/usr/bin/env python3
"""Bake the reviewed Harrison loop pavement into its owning SF game tile."""

import argparse
import gzip
import json
from pathlib import Path
from runpy import run_path


def bake(parent, contact, output):
    directory = Path(__file__).parent
    embed = run_path(str(directory / "embed-road-layers.py"))
    release = run_path(str(directory / "apply-tree-classification.py"))
    digest, encode = release["digest"], release["encode"]
    parent_raw = (parent / "tiles/manifest.json").read_bytes()
    manifest = json.loads(parent_raw)
    contact_raw = contact.read_bytes()
    product = json.loads(contact_raw)
    survey = product["i80_harrison_loop_pavement_survey"].copy()
    parent_contact = (parent / "structures.json").read_bytes()
    if not (
        survey["parent_contact_sha256"]
        == manifest["roadBake"]["playableSha256"]
        == digest(parent_contact)
    ):
        raise ValueError("The Harrison update requires the reviewed parent contact")
    road_id = "sf-contact/i80-westbound-harrison-loop"
    previous = json.loads(parent_contact)
    if (
        [road for road in previous["roads"] if road["id"] != road_id]
        != [road for road in product["roads"] if road["id"] != road_id]
        or previous["transitions"] != product["transitions"]
    ):
        raise ValueError("The Harrison update changed another road or join")
    by_tile, _, receipts = embed["playable_product"](
        contact, digest(contact_raw), product["base_sha256"], product["road_revision"], manifest
    )
    for key in ("playableSha256", "playableReplaces", "playableTransitions"):
        manifest["roadBake"][key] = receipts[key]
    token = "8085807c"
    tile = next(tile for tile in manifest["tiles"] if tile["t"] == token)
    raw = (parent / "tiles" / f"{token}.json.gz").read_bytes()
    content = json.loads(gzip.decompress(raw))
    content["playableRoads"] = by_tile[token]
    packed = gzip.compress(encode(content), compresslevel=9, mtime=0)
    release["record_tile"](manifest, tile, packed)
    survey["tiles"] = {token: {"parentSha256": digest(raw), "sha256": digest(packed)}}
    manifest["roadBake"]["i80HarrisonLoopPavementSurvey"] = survey
    return release["write_release"](
        parent, output, manifest, parent_raw, {token: packed}, contact_raw, carry=True
    )


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for argument in ("parent", "contact", "output"):
        parser.add_argument(argument, type=Path)
    args = parser.parse_args()
    print(bake(args.parent, args.contact, args.output))


if __name__ == "__main__":
    main()
