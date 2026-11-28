#!/usr/bin/env python3
"""Apply the reviewed Alemany connector and scenery corrections to an SF release."""

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
    survey = product["alemany_northbound_connector_survey"].copy()
    if not (
        survey["parent_contact_sha256"]
        == manifest["roadBake"]["playableSha256"]
        == digest((parent / "structures.json").read_bytes())
    ):
        raise ValueError("The Alemany update requires the reviewed parent contact")
    by_tile, _, receipts = embed["playable_product"](
        contact, digest(contact_raw), product["base_sha256"], product["road_revision"], manifest
    )
    for key in ("playableSha256", "playableReplaces", "playableTransitions"):
        manifest["roadBake"][key] = receipts[key]
    tree_patches = survey.pop("tree_patches")
    manifest["roadBake"]["alemanyNorthboundConnectorSurvey"] = survey
    survey["tiles"] = {}

    changed = {}
    for tile in manifest["tiles"]:
        token = tile["t"]
        raw = (parent / "tiles" / f"{token}.json.gz").read_bytes()
        content = json.loads(gzip.decompress(raw))
        previous = {road["id"]: road for road in content.get("playableRoads", [])}
        revised = {road["id"]: road for road in by_tile.get(token, [])}
        patches = tree_patches.get(token, [])
        if previous == revised and not patches:
            continue
        content["playableRoads"] = list(revised.values())
        if patches:
            content["tr"] = embed["patch_source_rows"](content["tr"], patches)
        packed = gzip.compress(encode(content), compresslevel=9, mtime=0)
        release["record_tile"](manifest, tile, packed)
        changed[token] = packed
        receipt = {"parentSha256": digest(raw), "sha256": digest(packed)}
        if patches:
            receipt["treeRowSha256"] = [patch["sha256"] for patch in patches]
        survey["tiles"][token] = receipt
    return release["write_release"](parent, output, manifest, parent_raw, changed, contact_raw, carry=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for argument in ("parent", "contact", "output"):
        parser.add_argument(argument, type=Path)
    args = parser.parse_args()
    print(bake(args.parent, args.contact, args.output))


if __name__ == "__main__":
    main()
