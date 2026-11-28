#!/usr/bin/env python3
"""Apply or carry forward a reviewed tree classification for an SF game bake."""

import argparse
from collections import Counter, defaultdict
import gzip
import hashlib
import json
from pathlib import Path
import shutil
from tempfile import TemporaryDirectory


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def encode(value):
    return (json.dumps(value, separators=(",", ":"), allow_nan=False) + "\n").encode()


def record_tile(manifest, tile, payload):
    tile["bytes"] = len(payload)
    reviewed = manifest.get("terrainReview", {}).get("tiles", {}).get(tile["t"])
    if reviewed:
        reviewed["sha256"] = digest(payload)


def write_release(
    release, output, manifest, input_manifest, changed, playable=None, *,
    carry=False, drive_export=None
):
    route_raw = (release / "quest-route.json").read_bytes()
    prior_hash = digest(input_manifest)
    if (
        json.loads(route_raw)["manifestSha256"] != prior_hash
        or route_raw.count(prior_hash.encode()) != 1
    ):
        raise ValueError("Quest route must bind the input tile manifest exactly once")
    drives = (release / "gamedrives.json").read_bytes()
    drive_hash = (
        json.loads(input_manifest)["roadBake"].get("driveSha256")
        if drive_export is not None
        else manifest["roadBake"].get("driveSha256")
    )
    if drive_hash is not None and digest(drives) != drive_hash:
        raise ValueError("Game drive export does not match the input tile manifest")
    if drive_export is not None:
        drives = drive_export
        manifest["roadBake"]["driveSha256"] = digest(drives)
    manifest_raw = encode(manifest)
    manifest_hash = digest(manifest_raw)
    route_raw = route_raw.replace(prior_hash.encode(), manifest_hash.encode(), 1)

    def save(bundle):
        (bundle / "tiles").mkdir(parents=True)
        for tile in manifest["tiles"]:
            token = tile["t"]
            name = f"tiles/{token}.json.gz"
            if token in changed:
                (bundle / name).write_bytes(changed[token])
            else:
                if (release / name).stat().st_size != tile["bytes"]:
                    raise ValueError(f"Baked tile length differs in {token}")
                shutil.copyfile(release / name, bundle / name)
        (bundle / "tiles" / "manifest.json").write_bytes(manifest_raw)
        (bundle / "quest-route.json").write_bytes(route_raw)
        (bundle / "gamedrives.json").write_bytes(drives)
        companions = (
            ("coverage.json", "drive-approaches.json", "structures.json")
            if carry
            else ("coverage.json",)
        )
        for name in companions:
            source = release / name
            if source.is_file():
                shutil.copyfile(source, bundle / name)
        if playable is not None:
            (bundle / "structures.json").write_bytes(playable)

    if carry:
        output.parent.mkdir(parents=True, exist_ok=True)
        with TemporaryDirectory(
            prefix=".sf-tree-review-", dir=output.parent
        ) as temporary:
            bundle = Path(temporary) / "release"
            save(bundle)
            bundle.rename(output)
    else:
        save(output)
    return manifest_hash


def bake(release, patch, output):
    tiles = release / "tiles"
    parent_raw = (tiles / "manifest.json").read_bytes()
    parent = digest(parent_raw)
    manifest = json.loads(parent_raw)
    product_raw = patch.read_bytes()
    product = json.loads(product_raw)
    if (
        product["format"]
        not in ("sf-inferred-tree-classification/v1", "sf-road-tree-clearance/v1")
        or product["tile_manifest_sha256"] != parent
    ):
        raise ValueError("Tree classification needs its pinned game release")
    road_clearance = product["format"] == "sf-road-tree-clearance/v1"
    patches = product["inferred_tree_patches"]
    if not patches.keys() <= {row["t"] for row in manifest["tiles"]}:
        raise ValueError("Tree classification references an unknown tile")
    if not (release / "coverage.json").is_file():
        raise FileNotFoundError(release / "coverage.json")

    changed = {}
    road_bake = manifest["roadBake"]
    previous = road_bake.get("treeClassification")
    if road_clearance and previous and patches.keys() & previous["tiles"].keys():
        raise ValueError("Use an additional tree review for previously classified tiles")
    if road_clearance:
        previous = None
    release_receipts = previous["tiles"] if previous else {}
    if previous and not patches.keys() <= release_receipts.keys():
        raise ValueError("Additional review must use previously classified tiles")
    for tile in manifest["tiles"]:
        token = tile["t"]
        if token not in patches:
            continue
        raw_tile = (tiles / f"{token}.json.gz").read_bytes()
        initial_tile_hash = digest(raw_tile)
        if road_clearance and product["base_tile_sha256"][token] != initial_tile_hash:
            raise ValueError(f"Road clearance tile hash differs in {token}")
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
        removed_rows = [digest(encode(data["tr"][i])) for i in sorted(removed)]
        data["tr"] = [
            row for index, row in enumerate(data["tr"]) if index not in removed
        ]
        payload = gzip.compress(encode(data), compresslevel=9, mtime=0)
        changed[token] = payload
        prior_receipt = release_receipts.get(token)
        if prior_receipt and prior_receipt["sha256"] != initial_tile_hash:
            raise ValueError(f"Previous tree review tile hash differs in {token}")
        release_receipts[token] = {
            **(prior_receipt or {"parentSha256": initial_tile_hash}),
            "sha256": digest(payload),
            "removed": len(removed)
            + (prior_receipt["removed"] if prior_receipt else 0),
        }
        if road_clearance:
            release_receipts[token]["treeRowSha256"] = removed_rows
        record_tile(manifest, tile, payload)

    classification = {
        "parentManifestSha256": parent,
        "classificationSha256": digest(product_raw),
        "surveyIndexSha256": product["ept_sha256"],
        "surveyNodesSha256": product["ept_nodes_sha256"],
        "streetTreeInventorySha256": product["tree_inventory_sha256"],
    }
    if road_clearance:
        road_bake.setdefault("roadTreeClearanceReviews", []).append(
            {
                **classification,
                "surveyReceiptSha256": product["survey_receipt_sha256"],
                "tiles": release_receipts,
            }
        )
    elif previous:
        previous.setdefault("additionalReviews", []).append(classification)
    else:
        road_bake["treeClassification"] = {**classification, "tiles": release_receipts}
    print(
        write_release(
            release, output, manifest, parent_raw, changed,
            carry=road_clearance or bool(previous),
        ),
        {token: release_receipts[token] for token in changed},
    )


def missing_rows(original, remaining, token):
    """Find deleted row identities while preserving order and checking ambiguity."""
    wanted = iter(remaining)
    next_row = next(wanted, None)
    missing = set()
    for row in original:
        if row == next_row:
            next_row = next(wanted, None)
        else:
            missing.add(row)
    if next_row is not None:
        raise ValueError(f"Tree rows were added, changed or reordered in {token}")
    counts = Counter(original)
    if any(counts[row] != 1 for row in missing):
        raise ValueError(f"Tree removal has an ambiguous row identity in {token}")
    return missing


def carry_tile(token, candidate, before, after, receipt, source_removals):
    if digest(before) != receipt["parentSha256"] or digest(after) != receipt["sha256"]:
        raise ValueError(f"Tree review tile hash differs in {token}")
    original, reviewed, rebaked = [
        json.loads(gzip.decompress(raw)) for raw in (before, after, candidate)
    ]
    original_rows, reviewed_rows, candidate_rows = [
        [digest(encode(row)) for row in tile["tr"]]
        for tile in (original, reviewed, rebaked)
    ]
    reviewed_removals = missing_rows(original_rows, reviewed_rows, token)
    earlier_removals = set(receipt.get("rebakeRemovedTreeRows", []))
    inherited = reviewed_removals - earlier_removals
    if (
        not earlier_removals <= reviewed_removals
        or len(inherited) != receipt["removed"]
    ):
        raise ValueError(f"Tree review removal count differs in {token}")
    current_removals = missing_rows(original_rows, candidate_rows, token)
    if not earlier_removals <= current_removals:
        raise ValueError(
            f"The rebake restored a previously removed source tree in {token}"
        )
    if not (current_removals - earlier_removals) <= source_removals:
        raise ValueError(
            f"The rebake removed a tree without a pinned contact patch in {token}"
        )
    rebaked["tr"] = [
        row
        for row, row_hash in zip(rebaked["tr"], candidate_rows, strict=True)
        if row_hash not in inherited
    ]
    payload = gzip.compress(encode(rebaked), compresslevel=9, mtime=0)
    return payload, {
        **receipt,
        "sha256": digest(payload),
        "carriedFromSha256": digest(after),
        "rebakedInputSha256": digest(candidate),
        "rebakeRemovedTreeRows": sorted(current_removals - inherited),
    }


def carry_road_clearance(token, candidate, reviewed, receipts, source_removals):
    """Preserve reviewed row removals when another tile layer has changed."""
    rows = [identity for receipt in receipts for identity in receipt["treeRowSha256"]]
    removed = set(rows)
    if len(removed) != len(rows) or digest(reviewed) != receipts[-1]["sha256"]:
        raise ValueError(f"Road clearance receipt differs in {token}")
    reviewed_rows = [
        digest(encode(row)) for row in json.loads(gzip.decompress(reviewed))["tr"]
    ]
    if removed.intersection(reviewed_rows):
        raise ValueError(f"The reviewed tile restored a road clearance tree in {token}")

    data = json.loads(gzip.decompress(candidate))
    original = data["tr"]
    hashes = [digest(encode(row)) for row in original]
    matches = [identity for identity in hashes if identity in removed]
    if len(matches) != len(set(matches)):
        raise ValueError(f"Road clearance tree identity is ambiguous in {token}")
    remaining = [identity for identity in hashes if identity not in removed]
    if missing_rows(reviewed_rows, remaining, token) - source_removals:
        raise ValueError(
            f"The rebake removed a tree without a pinned contact patch in {token}"
        )
    data["tr"] = [
        row
        for row, identity in zip(original, hashes, strict=True)
        if identity not in removed
    ]
    packed = gzip.compress(encode(data), compresslevel=9, mtime=0)
    for receipt in receipts:
        receipt.setdefault("reviewedSha256", receipt["sha256"])
        receipt["sha256"] = digest(packed)
    return packed


def carry_forward(release, reviewed_release, review_input, output, playable=None):
    raw_manifest = (release / "tiles/manifest.json").read_bytes()
    manifest = json.loads(raw_manifest)
    approved_manifest = json.loads(
        (reviewed_release / "tiles/manifest.json").read_bytes()
    )
    approved = approved_manifest["roadBake"]
    classification = approved.get("treeClassification")
    receipts = classification["tiles"] if classification else {}
    road_reviews = approved.get("roadTreeClearanceReviews", [])
    if not classification and not road_reviews:
        raise ValueError("Carry-forward requires a reviewed release")
    road_receipts = defaultdict(list)
    for review in road_reviews:
        for token, receipt in review["tiles"].items():
            road_receipts[token].append(receipt)
    if any(
        key in manifest["roadBake"]
        for key in ("treeClassification", "roadTreeClearanceReviews")
    ):
        raise ValueError(
            "Carry-forward needs a raw road bake before tree classification"
        )
    tokens = {tile["t"] for tile in manifest["tiles"]}
    if not (receipts.keys() | road_receipts.keys()) <= tokens:
        raise ValueError("The rebake is missing a classified tree tile")

    playable_raw = playable.read_bytes() if playable else None
    patches = {}
    if playable_raw is not None:
        if digest(playable_raw) != manifest["roadBake"].get("playableSha256"):
            raise ValueError("The contact export does not match the rebake")
        patches = json.loads(playable_raw).get("inferred_tree_patches", {})
    changed = {}
    for tile in manifest["tiles"]:
        token = tile["t"]
        if token not in receipts and token not in road_receipts:
            continue
        name = f"{token}.json.gz"
        candidate = (release / "tiles" / name).read_bytes()
        reviewed = (reviewed_release / "tiles" / name).read_bytes()
        source_removals = {
            patch["sha256"]
            for patch in patches.get(token, [])
            if patch["parts"] == []
        }
        if token in receipts:
            payload, receipts[token] = carry_tile(
                token,
                candidate,
                (review_input / name).read_bytes(),
                reviewed,
                receipts[token],
                source_removals,
            )
        else:
            payload = carry_road_clearance(
                token, candidate, reviewed, road_receipts[token], source_removals
            )
        changed[token] = payload
        record_tile(manifest, tile, payload)
    if classification:
        manifest["roadBake"]["treeClassification"] = classification
    if road_reviews:
        manifest["roadBake"]["roadTreeClearanceReviews"] = road_reviews
    print(
        write_release(
            release, output, manifest, raw_manifest, changed, playable_raw, carry=True
        ),
        receipts,
    )


if __name__ == "__main__":
    cli = argparse.ArgumentParser(description=__doc__)
    cli.add_argument("release", type=Path)
    cli.add_argument(
        "patch",
        type=Path,
        help="tree patch, or prior reviewed release with --review-input",
    )
    cli.add_argument("output", type=Path)
    cli.add_argument(
        "--review-input",
        type=Path,
        help="directory of original unclassified tree tiles",
    )
    cli.add_argument(
        "--playable",
        type=Path,
        help="pinned road contact export approving new tree removals",
    )
    args = cli.parse_args()
    if args.review_input:
        carry_forward(
            args.release, args.patch, args.review_input, args.output, args.playable
        )
    else:
        if args.playable:
            cli.error("--playable requires --review-input")
        bake(args.release, args.patch, args.output)
