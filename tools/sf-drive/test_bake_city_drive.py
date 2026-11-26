"""Regression checks for the SF game route bakers."""

import gzip
import math
import json
from pathlib import Path
import runpy

import pytest

smooth_ramp_join = runpy.run_path(Path(__file__).with_name("bake-city-drive.py"))[
    "smooth_ramp_join"
]
road_layers = runpy.run_path(Path(__file__).with_name("embed-road-layers.py"))
tree_bake = runpy.run_path(Path(__file__).with_name("apply-tree-classification.py"))
building_bake = runpy.run_path(Path(__file__).with_name("apply-building-clearance.py"))


class SlopingPavement:
    def height(self, x, y):
        return 10 + 0.02 * x + 0.01 * y


def heading(a, b):
    return math.atan2(b[1] - a[1], b[0] - a[0])


def grade(a, b):
    return (b[2] - a[2]) / math.hypot(b[0] - a[0], b[1] - a[1])


@pytest.mark.parametrize(
    "left_kind,right_kind", [("ground", "ramp"), ("ramp", "ground")]
)
def test_ramp_join_shares_a_point_and_blends_heading_and_grade(left_kind, right_kind):
    pavement = SlopingPavement()
    bearing = math.radians(12)
    before = [[s, 0, 0] for s in range(-12, 1, 2)]
    after = [[s * math.cos(bearing), s * math.sin(bearing), 0] for s in range(0, 13, 2)]
    for kind, positions in [(left_kind, before), (right_kind, after)]:
        for point in positions:
            x, y, _ = point
            point[2] = pavement.height(x, y) if kind == "ground" else 10 + 0.06 * x

    left = {"id": "left", "kind": left_kind, "positions": before}
    right = {"id": "right", "kind": right_kind, "positions": after}
    smooth_ramp_join(left, right, pavement)

    first, join = left["positions"][-2:]
    next_point = right["positions"][1]
    assert join == right["positions"][0]
    assert left["positions"][0] == before[0]
    assert right["positions"][-1] == after[-1]
    assert abs(heading(first, join) - heading(join, next_point)) < math.radians(3)
    assert abs(grade(first, join) - grade(join, next_point)) < 0.01
    ground = left if left_kind == "ground" else right
    for x, y, z in ground["positions"]:
        assert z == pytest.approx(pavement.height(x, y), abs=0.00006)


def test_overhead_patches_use_original_row_indices_and_check_source_records():
    encoded = road_layers["encoded"]
    digest = road_layers["digest"]
    patch_buildings = road_layers["patch_source_rows"]
    first, untouched, last = [[base, [[base + 20]]] for base in (10, 50, 70)]
    buildings = [first, untouched, last]
    cover, side, other = [[base, [[100]]] for base in (30, 10, 90)]
    patches = [
        {"index": 0, "sha256": digest(encoded(first)), "parts": [cover, side]},
        {"index": 2, "sha256": digest(encoded(last)), "parts": [other]},
    ]
    assert patch_buildings(buildings, patches) == [cover, side, untouched, other]
    assert buildings == [first, untouched, last]
    with pytest.raises(ValueError, match="does not match source row 0"):
        patch_buildings(buildings, [*patches, patches[0]])
    with pytest.raises(ValueError, match="does not match source row 0"):
        patch_buildings([last, untouched, first], patches)


def test_tile_patch_can_remove_a_fully_covered_road_row():
    encoded = road_layers["encoded"]
    digest = road_layers["digest"]
    patch_rows = road_layers["patch_source_rows"]
    rows = [["approach"], ["covered"], ["exit"]]
    patch = {"index": 1, "sha256": digest(encoded(rows[1])), "parts": []}
    assert patch_rows(rows, [patch]) == [["approach"], ["exit"]]
    with pytest.raises(ValueError, match="does not match source row 1"):
        patch_rows(rows, [{**patch, "parts": None}])


@pytest.mark.parametrize(
    ("patch_key", "manifest_key", "field"),
    [
        ("covered_ground_road_patches", "coveredGroundRoadPatches", "r"),
        ("overhead_building_patches", "overheadBuildingPatches", "b"),
        ("inferred_tree_patches", "inferredTreePatches", "tr"),
    ],
)
def test_scenery_patches_can_belong_to_a_neighbouring_tile(
    tmp_path, patch_key, manifest_key, field
):
    encoded = road_layers["encoded"]
    digest = road_layers["digest"]
    playable_product = road_layers["playable_product"]
    patch = {"index": 0, "sha256": "source", "parts": [["approach"], ["exit"]]}
    product = {
        "format": "sf-game-road-contact",
        "version": 1,
        "base_sha256": "base",
        "road_revision": "revision",
        "roads": [],
        "transitions": [],
        patch_key: {"8085871c": [patch]},
    }
    path = tmp_path / "structures.json"
    payload = encoded(product)
    path.write_bytes(payload)
    manifest = {"tiles": [{"t": "8085871c"}]}
    _, tile_patches, release = playable_product(
        path, digest(payload), "base", "revision", manifest
    )
    assert tile_patches == {"8085871c": {field: [patch]}}
    assert release[manifest_key] == {"8085871c": 1}
    with pytest.raises(ValueError, match="must name base tiles"):
        playable_product(path, digest(payload), "base", "revision", {"tiles": []})


def test_bake_requires_a_transition_for_a_wider_seam():
    valid = road_layers["valid_width_transitions"]
    road = {
        "width": 8,
        "points": [[0, 0, 0], [20, 0, 1], [40, 0, 2]],
        "seams": {"end": {"bearing": 0, "width": 12}},
    }
    assert not valid(road)
    assert valid({**road, "widthTransitionMetres": {"end": 24}})
    assert not valid({**road, "widthTransitionMetres": {"start": 24}})
    assert not valid({**road, "widthTransitionMetres": {"end": 41}})


def test_bake_carries_a_positive_probe_for_a_drivable_ramp_entrance(tmp_path):
    encoded = road_layers["encoded"]
    digest = road_layers["digest"]
    playable_product = road_layers["playable_product"]
    token = "8085871c"
    manifest = {"tiles": [{"t": token, "x": 0, "y": 0}]}
    path = tmp_path / "structures.json"
    road = {
        "id": "ramp",
        "kind": "ramp",
        "width": 6,
        "points": [[0, 0, 0], [20, 0, 1], [40, 0, 2]],
        "groundEnds": ["start"],
        "owners": [token, token],
        "profiles": [],
        "groundEntryProbeMetres": {"start": 12},
    }

    def bake(candidate):
        product = {
            "format": "sf-game-road-contact",
            "version": 1,
            "base_sha256": "base",
            "road_revision": "revision",
            "roads": [candidate],
            "transitions": [],
        }
        raw = encoded(product)
        path.write_bytes(raw)
        roads, _, _ = playable_product(path, digest(raw), "base", "revision", manifest)
        return roads[token]

    assert bake(road) == [road]
    for invalid in (
        {"groundEntryProbeMetres": {"end": 12}},
        {"groundEntryProbeMetres": {"start": 0}},
        {"groundEntryProbeMetres": {"start": True}},
        {"kind": "bridge_deck"},
        {"groundExitOnly": ["start"]},
    ):
        with pytest.raises(ValueError, match="Invalid playable road ramp"):
            bake({**road, **invalid})


def test_bake_validates_asymmetric_width_and_bounds_its_stations():
    valid = road_layers["valid_width_transitions"]
    road = {
        "width": 8,
        "points": [[0, 0, 0], [20, 0, 1], [40, 0, 2]],
        "seams": {"end": {"bearing": 0, "width": 12, "leftWidth": 8}},
        "widthTransitionMetres": {"end": 12},
    }
    assert valid({**road, "widthSections": [[0, 4, 4], [20, 7, 3], [40, 8, 4]]})
    assert not valid({**road, "widthSections": [[0, 4, 4], [41, 7, 3]]})
    assert not valid({**road, "widthSections": [[15, 4, 4], [15, 7, 3]]})
    assert not valid({**road, "widthSections": [[0, 4, -1]]})
    assert not valid(
        {**road, "seams": {"end": {"bearing": 0, "width": 12, "leftWidth": 12}}}
    )


def test_bake_rejects_rail_ranges_that_cover_portals_or_conflict_with_other_rails():
    valid = road_layers["valid_rail_ranges"]
    rail = {"side": "left", "startMetres": 4.5, "endMetres": 17.75}
    road = {"points": [[0, 0, 0], [10, 0, 1], [20, 0, 2]], "railRanges": [rail]}
    opening = {**rail, "startMetres": 17.75, "endMetres": 20}
    assert valid(road)
    assert valid({**road, "sideGroundAccess": [opening]})
    assert valid({**road, "railRanges": [rail, {**rail, "side": "right"}]})
    assert not valid({**road, "railSides": ["left"]})
    assert not valid({**road, "railRanges": [rail, {**rail, "startMetres": 5}]})
    assert not valid({**road, "railRanges": [{**rail, "endMetres": 20.1}]})
    assert not valid({**road, "sideGroundAccess": [{**opening, "startMetres": 17.5}]})
    assert not valid({**road, "sideGroundAccess": [None]})


def test_bake_bounds_the_directed_approach_lane_by_the_source_seam():
    valid = road_layers["valid_approach_lane"]
    join = {"from": "trunk:end", "direction": "forward"}
    seam = {"width": 19.5, "leftWidth": 2}
    assert valid(join, seam)
    assert valid({**join, "from_lateral_range_m": [-17.5, 0.65]}, seam)
    assert valid({**join, "from_lateral_range_m": [0.65, 2]}, seam)
    for lane in ([-18, 0.65], [0.65, 3], [1, -1], [True, 2], None):
        assert not valid({**join, "from_lateral_range_m": lane}, seam)
    assert not valid(
        {**join, "direction": "both", "from_lateral_range_m": [-1, 1]}, seam
    )


@pytest.mark.parametrize(
    ("covered", "expected"),
    [
        ([0, 40], True),
        ([12, 36], True),
        ([-1, 36], False),
        ([12, 41], False),
        ([26, 12], False),
        ([True, 36], False),
        ([12, math.inf], False),
        ([12], False),
    ],
)
def test_bake_bounds_the_covered_range_by_the_road_length(covered, expected):
    road = {
        "points": [[0, 0, 0], [20, 0, 1], [40, 0, 2]],
        "covered_range_m": covered,
    }
    assert road_layers["valid_covered_range"](road) is expected


def test_bundled_approach_requires_exact_parent_and_preserves_original_sections(
    tmp_path,
):
    encoded = road_layers["encoded"]
    digest = road_layers["digest"]
    with_drive_approaches = road_layers["with_drive_approaches"]
    original = {"id": "bridge", "sections": [{"id": "west-span"}]}
    drives = encoded({"drives": [original]})
    entry = {"id": "island", "label": "Island", "sections": [{"id": "tunnel"}]}
    opposite = {
        "id": "eastbound",
        "label": "Eastbound",
        "replace_sections": True,
        "sections": [{"id": "lower"}],
    }
    extension = {
        "format": "sf-game-drive-approaches",
        "base_drive_sha256": digest(drives),
        "drive_id": "bridge",
        "approaches": [entry, opposite],
    }
    path = tmp_path / "extension.json"
    raw = encoded(extension)
    path.write_bytes(raw)
    with pytest.raises(ValueError, match="checksum differs"):
        with_drive_approaches(drives, path, "incorrect")
    with pytest.raises(ValueError, match="pinned parent"):
        with_drive_approaches(drives + b" ", path, digest(raw))
    bundled, starts = with_drive_approaches(drives, path, digest(raw))
    assert starts == [
        {"id": "island", "label": "Island"},
        {"id": "eastbound", "label": "Eastbound"},
    ]
    assert json.loads(bundled)["drives"] == [
        dict(original, approaches=[entry, opposite])
    ]
    opposite["replace_sections"] = "yes"
    raw = encoded(extension)
    path.write_bytes(raw)
    with pytest.raises(ValueError, match="replace_sections must be a boolean"):
        with_drive_approaches(drives, path, digest(raw))


def test_bake_adds_a_pinned_terrain_detail_without_replacing_the_coarse_grid():
    grid = {"x0": -8, "y0": -8, "s": 8, "n": 4, "z": [60] * 16}
    z = [60] * 25
    z[12] = 53
    detail = {"x0": 0, "y0": 0, "s": 2, "columns": 5, "rows": 5, "z": z}
    patch = {"base_tile_sha256": "tile-sha", "detail": detail}
    add_detail = road_layers["patch_terrain_detail"]

    patched = add_detail(grid, patch, "tile-sha")
    assert patched == {**grid, "detail": detail}
    assert patched["z"] is grid["z"]
    assert "detail" not in grid

    with pytest.raises(ValueError, match="does not match the base tile"):
        add_detail(grid, patch, "other-sha")
    with pytest.raises(ValueError, match="must extend the existing rectangle"):
        add_detail(patched, patch, "tile-sha")
    with pytest.raises(ValueError, match="must align with the coarse grid"):
        add_detail(grid, {**patch, "detail": {**detail, "x0": 1}}, "tile-sha")
    with pytest.raises(ValueError, match="must align with the coarse grid"):
        add_detail(grid, {**patch, "detail": {**detail, "z": []}}, "tile-sha")
    shifted_edge = z.copy()
    shifted_edge[1] = 61
    with pytest.raises(ValueError, match="boundary differs"):
        add_detail(grid, {**patch, "detail": {**detail, "z": shifted_edge}}, "tile-sha")


def test_bake_extends_terrain_detail_and_preserves_the_existing_samples():
    z = [60] * 25
    z[12] = 53
    detail = {"x0": 0.0, "y0": 0.0, "s": 2, "columns": 5, "rows": 5, "z": z}
    grid = {"x0": -8, "y0": -8, "s": 8, "n": 4, "z": [60] * 16, "detail": detail}
    extended = {**detail, "x0": -8, "y0": 0, "columns": 9, "z": [60] * 45}
    extended["z"][24] = 53
    extended["z"][20] = 55
    patch = {"base_tile_sha256": "tile-sha", "detail": extended}
    add_detail = road_layers["patch_terrain_detail"]

    assert add_detail(grid, patch, "tile-sha")["detail"] == extended
    assert grid["detail"] is detail
    extended["z"][24] = 54
    with pytest.raises(ValueError, match="extension changed a base sample"):
        add_detail(grid, patch, "tile-sha")


def test_tunnel_wall_openings_are_ordered_and_confined_to_the_interior():
    valid = road_layers["valid_tunnel_wall_openings"]
    tunnel = {"kind": "tunnel", "interior_range_m": [1, 60]}
    assert valid(tunnel)
    assert valid({**tunnel, "tunnel_wall_openings_m": {"left": [[4, 7], [13, 60]]}})
    for openings in (
        {"up": [[1, 5]]},
        {"left": [[0, 7]]},
        {"left": [[4, 8], [7, 18]]},
        {"right": [[5, math.inf]]},
        {"right": [[6, 61]]},
    ):
        assert not valid({**tunnel, "tunnel_wall_openings_m": openings})


def tree_release(directory, tiles, road_bake):
    encode, digest = tree_bake["encode"], tree_bake["digest"]
    (directory / "tiles").mkdir(parents=True)
    entries = []
    for token, tile in tiles.items():
        raw = gzip.compress(encode(tile), mtime=0)
        (directory / "tiles" / f"{token}.json.gz").write_bytes(raw)
        entries.append({"t": token, "bytes": len(raw)})
    manifest = encode(
        {
            "tiles": entries,
            "roadBake": road_bake,
            "terrainReview": {"tiles": {"80858024": {"sha256": "prior"}}},
        }
    )
    (directory / "tiles/manifest.json").write_bytes(manifest)
    (directory / "quest-route.json").write_bytes(
        encode({"manifestSha256": digest(manifest), "targets": ["macalla"]})
    )
    (directory / "gamedrives.json").write_bytes(encode({"fixture": directory.name}))
    (directory / "coverage.json").write_bytes(encode({"coverage": directory.name}))
    return digest(manifest)


def test_building_clearance_checks_row_and_preserves_road_contact(tmp_path):
    encode, digest = tree_bake["encode"], tree_bake["digest"]
    source = tmp_path / "source"
    output = tmp_path / "packed"
    review = tmp_path / "clearance.json"
    token = "80858024"
    tile_path = f"tiles/{token}.json.gz"
    building = [29, [[[0, 0, 123, 50, 0, 123, 50, 60, 123]]], 1]
    clearance = [29, [[[0, 0, 103, 50, 0, 103, 50, 60, 103]]], 1]
    tile = {"b": [building], "playableRoads": [{"id": "merge"}], "tr": []}
    parent = tree_release(source, {token: tile}, {})
    raw = (source / tile_path).read_bytes()
    product = {
        "format": "sf-baked-building-clearance/v1",
        "tile_manifest_sha256": parent,
        "ept_index_sha256": "index",
        "survey_nodes_sha256": "observations",
        "tiles": {
            token: {
                "sha256": digest(raw),
                "patches": [{"index": 0, "sha256": "wrong", "parts": [clearance]}],
            }
        },
    }
    review.write_bytes(encode(product))
    with pytest.raises(ValueError, match="source row 0"):
        building_bake["bake"](source, review, output)
    assert not output.exists()

    product["tiles"][token]["patches"][0]["sha256"] = digest(encode(building))
    review.write_bytes(encode(product))
    published = building_bake["bake"](source, review, output)
    packed = (output / tile_path).read_bytes()
    assert json.loads(gzip.decompress(packed)) == {**tile, "b": [clearance]}
    manifest = json.loads((output / "tiles/manifest.json").read_bytes())
    receipt = manifest["roadBake"]["buildingClearanceReviews"][0]
    assert receipt["parentManifestSha256"] == parent
    assert receipt["tiles"][token]["sha256"] == digest(packed)
    assert (
        json.loads((output / "quest-route.json").read_bytes())["manifestSha256"]
        == published
    )


@pytest.fixture
def reviewed_trees(tmp_path):
    encode, digest = tree_bake["encode"], tree_bake["digest"]
    original = tmp_path / "original"
    reviewed = tmp_path / "reviewed"
    token = "80858024"
    tile = {
        "tr": [[1, 1, 5, 4], [2, 2, 8, 2], [3, 3, 10, 3], [4, 4, 7, 2]],
        "playableRoads": [{"id": "old-deck"}],
        "g": {"detail": "old-terrain"},
    }
    parent = tree_release(original, {token: tile}, {})
    product = tmp_path / "tree-patch.json"
    product.write_bytes(
        encode(
            {
                "format": "sf-inferred-tree-classification/v1",
                "tile_manifest_sha256": parent,
                "ept_sha256": "survey-index",
                "ept_nodes_sha256": {},
                "tree_inventory_sha256": "tree-inventory",
                "inferred_tree_patches": {
                    token: [
                        {
                            "index": 0,
                            "sha256": digest(encode(tile["tr"][0])),
                            "parts": [],
                        }
                    ],
                },
            }
        )
    )
    reviewed.mkdir()
    log = reviewed / "review.log"
    log.write_text("existing operator log")
    tree_bake["bake"](original, product, reviewed)
    assert log.read_text() == "existing operator log"
    return original, reviewed, tile


def test_additional_tree_review_keeps_prior_removals_on_rebake(
    tmp_path, reviewed_trees
):
    original, reviewed, tile = reviewed_trees
    encode, digest = tree_bake["encode"], tree_bake["digest"]
    token = "80858024"
    name = f"tiles/{token}.json.gz"
    manifest_path = "tiles/manifest.json"
    approved = tmp_path / "supplemental"
    product = tmp_path / "additional-review.json"
    (reviewed / "structures.json").write_bytes(b"preserved contact export\n")
    parent = digest((reviewed / manifest_path).read_bytes())
    product.write_bytes(
        encode(
            {
                "format": "sf-inferred-tree-classification/v1",
                "tile_manifest_sha256": parent,
                "ept_sha256": "survey-index",
                "ept_nodes_sha256": {"exit": "survey-nodes"},
                "tree_inventory_sha256": None,
                "inferred_tree_patches": {
                    token: [
                        {
                            "index": 0,
                            "sha256": digest(encode(tile["tr"][1])),
                            "parts": [],
                        }
                    ],
                },
            }
        )
    )
    tree_bake["bake"](reviewed, product, approved)
    packed = (approved / name).read_bytes()
    assert json.loads(gzip.decompress(packed))["tr"] == tile["tr"][2:]
    manifest = json.loads((approved / manifest_path).read_bytes())
    classification = manifest["roadBake"]["treeClassification"]
    assert classification["tiles"][token]["parentSha256"] == digest(
        (original / name).read_bytes()
    )
    assert classification["tiles"][token]["removed"] == 2
    assert classification["additionalReviews"][0]["parentManifestSha256"] == parent
    assert (approved / "structures.json").read_bytes() == b"preserved contact export\n"

    rebake = tmp_path / "rebake"
    tree_bake["carry_forward"](original, approved, original / "tiles", rebake)
    assert (rebake / name).read_bytes() == packed


def test_carry_forward_preserves_rebaked_roads_and_independent_removals(
    tmp_path, reviewed_trees
):
    encode, digest = tree_bake["encode"], tree_bake["digest"]
    original, reviewed, tile = reviewed_trees
    token = "80858024"
    candidate = tmp_path / "candidate"
    output = tmp_path / "classified"
    contact = tmp_path / "structures.json"
    removal = digest(encode(tile["tr"][1]))
    contact_raw = encode(
        {
            "inferred_tree_patches": {
                token: [{"index": 1, "sha256": removal, "parts": []}]
            }
        }
    )
    contact.write_bytes(contact_raw)
    rebaked_tile = {
        **tile,
        "tr": [tile["tr"][0], *tile["tr"][2:]],
        "g": {"detail": "new-terrain"},
        "playableRoads": [{"id": "southgate-feeder"}],
    }
    other = "8085803c"
    other_tile = {"tr": [[10, 10, 7, 1]], "playableRoads": [{"id": "bridge-join"}]}
    raw_hash = tree_release(
        candidate,
        {token: rebaked_tile, other: other_tile},
        {"playableSha256": digest(contact_raw)},
    )
    tree_bake["carry_forward"](candidate, reviewed, original / "tiles", output, contact)

    path = f"tiles/{token}.json.gz"
    packed = (output / path).read_bytes()
    assert json.loads(gzip.decompress(packed)) == {**rebaked_tile, "tr": tile["tr"][2:]}
    assert (output / f"tiles/{other}.json.gz").read_bytes() == (
        candidate / f"tiles/{other}.json.gz"
    ).read_bytes()
    assert (output / "structures.json").read_bytes() == contact_raw
    assert (output / "gamedrives.json").read_bytes() == (
        candidate / "gamedrives.json"
    ).read_bytes()
    manifest = (output / "tiles/manifest.json").read_bytes()
    manifest_data = json.loads(manifest)
    receipt = manifest_data["roadBake"]["treeClassification"]["tiles"][token]
    assert receipt["carriedFromSha256"] == digest((reviewed / path).read_bytes())
    assert receipt["rebakedInputSha256"] == digest((candidate / path).read_bytes())
    assert receipt["removed"] == 1
    assert receipt["rebakeRemovedTreeRows"] == [removal]
    assert (
        receipt["sha256"]
        == manifest_data["terrainReview"]["tiles"][token]["sha256"]
        == digest(packed)
    )
    assert manifest_data["tiles"][0]["bytes"] == len(packed)
    assert digest(manifest) != raw_hash
    assert json.loads((output / "quest-route.json").read_bytes()) == {
        "manifestSha256": digest(manifest),
        "targets": ["macalla"],
    }

    next_candidate = tmp_path / "next-candidate"
    next_output = tmp_path / "next-classified"
    next_tile = {**rebaked_tile, "playableRoads": [{"id": "latest-deck"}]}
    tree_release(next_candidate, {token: next_tile}, {})
    tree_bake["carry_forward"](next_candidate, output, original / "tiles", next_output)
    assert json.loads(gzip.decompress((next_output / path).read_bytes())) == {
        **next_tile,
        "tr": tile["tr"][2:],
    }


def test_carry_forward_rejects_unauthorised_rows_without_writing_release(
    tmp_path, reviewed_trees
):
    original, reviewed, tile = reviewed_trees
    token = "80858024"
    candidate = tmp_path / "candidate"
    stale = tmp_path / "stale"
    output = tmp_path / "output"
    tree_release(
        candidate, {token: {**tile, "tr": [tile["tr"][0], *tile["tr"][2:]]}}, {}
    )
    tree_release(
        stale,
        {token: {**tile, "tr": [tile["tr"][1], tile["tr"][0], *tile["tr"][2:]]}},
        {},
    )
    with pytest.raises(ValueError, match="without a pinned contact patch"):
        tree_bake["carry_forward"](candidate, reviewed, original / "tiles", output)
    with pytest.raises(ValueError, match="added, changed or reordered"):
        tree_bake["carry_forward"](stale, reviewed, original / "tiles", output)
    with pytest.raises(ValueError, match="review tile hash differs"):
        tree_bake["carry_forward"](candidate, reviewed, stale / "tiles", output)
    unpinned = tmp_path / "other-contact.json"
    unpinned.write_text("{}")
    with pytest.raises(ValueError, match="contact export does not match"):
        tree_bake["carry_forward"](
            candidate, reviewed, original / "tiles", output, unpinned
        )
    assert not output.exists()


def test_tree_bake_does_not_replace_an_existing_release(tmp_path, reviewed_trees):
    original, reviewed, tile = reviewed_trees
    candidate = tmp_path / "candidate"
    output = tmp_path / "already-published"
    tree_release(candidate, {"80858024": tile}, {})
    output.mkdir()
    sentinel = output / "manifest.json"
    sentinel.write_bytes(b"existing release")

    with pytest.raises(OSError):
        tree_bake["carry_forward"](candidate, reviewed, original / "tiles", output)

    assert sentinel.read_bytes() == b"existing release"
    assert list(output.iterdir()) == [sentinel]
    assert list(tmp_path.glob(".sf-tree-review-*")) == []
