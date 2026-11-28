"""Replace reviewed road contacts in their existing owning game tile."""

import gzip
import json
from pathlib import Path
from runpy import run_path


def update_drive_walls(parent, roads, previous, encode):
    walls = {
        road["id"]: road.get("tunnel_wall_openings_m")
        for road in roads
        if road.get("tunnel_wall_openings_m")
        != previous[road["id"]].get("tunnel_wall_openings_m")
    }
    if not walls:
        return None

    product = json.loads((parent / "gamedrives.json").read_bytes())
    changed = False
    for drive in product["drives"]:
        for route in [drive, *drive.get("approaches", [])]:
            for span in route["sections"]:
                if span["id"] not in walls:
                    continue
                if walls[span["id"]] is None:
                    span.pop("tunnel_wall_openings_m", None)
                else:
                    span["tunnel_wall_openings_m"] = walls[span["id"]]
                changed = True
    return encode(product) if changed else None


def bake_contact_update(parent, contact, output, *, road_ids, token, source_key, receipt_key):
    directory = Path(__file__).parent
    embed = run_path(str(directory / "embed-road-layers.py"))
    release = run_path(str(directory / "apply-tree-classification.py"))
    digest, encode = release["digest"], release["encode"]
    parent_raw = (parent / "tiles/manifest.json").read_bytes()
    manifest = json.loads(parent_raw)
    contact_raw = contact.read_bytes()
    product = json.loads(contact_raw)
    survey = product[source_key].copy()
    parent_contact = (parent / "structures.json").read_bytes()
    if not (
        survey["parent_contact_sha256"]
        == manifest["roadBake"]["playableSha256"]
        == digest(parent_contact)
    ):
        raise ValueError("The road update requires the reviewed parent contact")
    previous = json.loads(parent_contact)
    if (
        [road for road in previous["roads"] if road["id"] not in road_ids]
        != [road for road in product["roads"] if road["id"] not in road_ids]
        or previous["transitions"] != product["transitions"]
    ):
        raise ValueError("The road update changed another road or join")
    by_tile, _, receipts = embed["playable_product"](
        contact, digest(contact_raw), product["base_sha256"], product["road_revision"], manifest
    )
    owners = {
        key for key, roads in by_tile.items()
        if any(road["id"] in road_ids for road in roads)
    }
    if owners != {token}:
        raise ValueError("The revised road requires a different tile set")
    for key in ("playableSha256", "playableReplaces", "playableTransitions"):
        manifest["roadBake"][key] = receipts[key]
    tile = next(tile for tile in manifest["tiles"] if tile["t"] == token)
    raw = (parent / "tiles" / f"{token}.json.gz").read_bytes()
    content = json.loads(gzip.decompress(raw))
    content["playableRoads"] = by_tile[token]
    packed = gzip.compress(encode(content), compresslevel=9, mtime=0)
    release["record_tile"](manifest, tile, packed)
    survey["tiles"] = {token: {"parentSha256": digest(raw), "sha256": digest(packed)}}
    manifest["roadBake"][receipt_key] = survey
    drive_export = update_drive_walls(
        parent,
        [road for road in product["roads"] if road["id"] in road_ids],
        {road["id"]: road for road in previous["roads"] if road["id"] in road_ids},
        encode,
    )
    return release["write_release"](
        parent, output, manifest, parent_raw, {token: packed}, contact_raw,
        carry=True, drive_export=drive_export
    )
