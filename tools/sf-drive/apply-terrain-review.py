#!/usr/bin/env python3
"""Apply the reviewed Dogpatch ground corrections to the bundled SF game tiles."""

import argparse
import gzip
from hashlib import sha256
import json
from pathlib import Path

import numpy as np
from scipy.spatial import cKDTree
from shapely import get_parts
from shapely.geometry import Polygon

MISSING = -32768
PARENT = "531f4e9789e9ec4e39713ae02adb50128d93e2ab3d5cd0f094be78bae65f41ac"
LOUISIANA = "808f7fbc"
PENNSYLVANIA = "808f7fb4"
DECK_ID = "bridge/1238000/both/terrain-review"
PARENT_TILES = {
    LOUISIANA: "e89aa96fa01810a218d5e632e53b6ef9bafdeabd1b65ff347b51dfd6800df42a",
    PENNSYLVANIA: "f48e22f825d371b039fcdab99df587ab2f09e82b2bbc9d0c40e2c95a0356c080",
}


def digest(raw):
    return sha256(raw).hexdigest()


def encoded(value):
    return (json.dumps(value, separators=(",", ":"), allow_nan=False) + "\n").encode()


def sample_ground(grid, x, y):
    """Sample the exact original triangle, in decimetres."""
    columns, step = grid["n"], grid["s"]
    fx, fy = (x - grid["x0"]) / step, (y - grid["y0"]) / step
    i, j = int(fx), int(fy)
    u, v = fx - i, fy - j
    k = j * columns + i
    a, b, c, d = (grid["z"][k + di] for di in (0, 1, columns, columns + 1))
    if MISSING in (a, b, c, d):
        return MISSING
    return (
        a * (1 - u - v) + b * u + c * v
        if u + v <= 1
        else (b * (1 - v) + c * (1 - u) + d * (u + v - 1))
    )


def observations(path, extend=False):
    points = np.load(path, allow_pickle=False)
    # Hold out whole 2 m blocks to limit leakage from adjacent returns.
    xy = np.floor(points[:, :2] / 2).astype(np.int64)
    holdout = (xy[:, 0] * 2654435761 ^ xy[:, 1] * 2246822519) % 7 == 0
    fitted = points[~holdout]
    index = cKDTree(fitted[:, :2])

    def height(x, y):
        selection = index.query_ball_point((x, y), 0.85)
        z = fitted[selection, 2]
        if len(z) >= 8 and np.quantile(z, 0.9) - np.quantile(z, 0.1) <= 0.55:
            return float(np.median(z))
        if not extend:
            return None
        # Reconstruct an omitted holdout block only where its neighbours
        # describe a single consistent plane.
        patch = fitted[index.query_ball_point((x, y), 2.0)]
        if len(patch) < 12:
            return None
        matrix = np.column_stack((patch[:, :2] - (x, y), np.ones(len(patch))))
        fit, _, rank, _ = np.linalg.lstsq(matrix, patch[:, 2], rcond=None)
        if (
            rank < 3
            or np.linalg.norm(fit[:2]) > 0.3
            or np.quantile(np.abs(matrix @ fit - patch[:, 2]), 0.9) > 0.13
        ):
            return None
        return float(fit[2])

    return height


def pennsylvania_y(x):
    return -542.9 + 0.077 * (x - 4152.1)


def louisiana_y(x):
    return -65.5 + 0.077 * (x - 4729)


def refine(tile, bounds, height, gap=None):
    grid = tile["g"]
    ox, oy = tile["o"]
    x0, y0, x1, y1 = bounds
    if any(
        (edge - origin) % grid["s"] != 0
        for edge, origin in [
            (x0, ox + grid["x0"]),
            (x1, ox + grid["x0"]),
            (y0, oy + grid["y0"]),
            (y1, oy + grid["y0"]),
        ]
    ):
        raise ValueError("Refinement must start and end on coarse grid lines")
    heights = []
    for y in range(y0, y1 + 1):
        for x in range(x0, x1 + 1):
            prior = sample_ground(grid, x - ox, y - oy)
            weight = min(1.0, min(x - x0, x1 - x, y - y0, y1 - y) / 6)
            observed = height(x, y) if weight > 0 else None
            if gap and gap(x, y):
                # Preserve the aerial shadow and keep the elevated returns out of terrain.
                if observed is None:
                    heights.append(MISSING)
                    continue
            if prior != MISSING and observed is not None:
                prior += weight * (10 * observed - prior)
            heights.append(round(prior, 4))
    return {
        "x0": x0 - ox,
        "y0": y0 - oy,
        "s": 1,
        "columns": x1 - x0 + 1,
        "rows": y1 - y0 + 1,
        "z": heights,
    }


def trim_western_21st_street(tile):
    # The centreline spans a construction embankment. Keep both supported
    # approaches and the original ground through the interrupted section.
    roads = []
    for row in tile["r"]:
        name, _, _, xy, widths = row
        if (
            tile["n"][name] != "21st St"
            or not xy
            or min(xy[1::2]) + 10 * tile["o"][1] > -600
        ):
            roads.append(row)
            continue
        retained = [
            i
            for i in range(0, len(xy), 2)
            if not 4701.5 < tile["o"][0] + xy[i] / 10 < 4728.9
        ]
        if len(retained) < 2 or retained[-1] - retained[0] != 2 * (len(retained) - 1):
            raise ValueError("Expected to trim a single end of a 21st Street row")
        start, stop = retained[0], retained[-1] + 2
        roads.append([*row[:3], xy[start:stop], widths[start:stop]])
    tile["r"] = roads


def clear_reviewed_lane(tile, west_ground, east_ground, west_returns, east_returns):
    ground = np.concatenate(
        [np.load(p, allow_pickle=False) for p in (west_ground, east_ground)]
    )
    returns = np.concatenate(
        [np.load(p, allow_pickle=False) for p in (west_returns, east_returns)]
    )
    ground_index, return_index = (cKDTree(p[:, :2]) for p in (ground, returns))
    start, end, half_width = 4750, 4817, 2.5
    for x in range(start, end + 1):
        for offset in (-2.5, -1.25, 0, 1.25, 2.5):
            xy = (x, louisiana_y(x) + offset)
            base = ground[ground_index.query_ball_point(xy, 0.7), 2]
            raised = returns[return_index.query_ball_point(xy, 0.7), 2]
            if len(base) < 20 or np.any(raised > np.median(base) + 0.4):
                raise ValueError(f"Open pavement is unverified at {xy}")
    ox, oy = tile["o"]
    lane = Polygon(
        [
            (10 * (x - ox), 10 * (louisiana_y(x) + offset - oy))
            for x, offset in [
                (start, -half_width),
                (end, -half_width),
                (end, half_width),
                (start, half_width),
            ]
        ]
    )
    # Only two reviewed prisms extend across this open lane. Retain the
    # remaining roof faces and reconstruct their heights on the cut edge.
    for index in (196, 383):
        building = tile["b"][index]
        kept = []
        for face in building[1]:
            rings = [np.asarray(ring).reshape(-1, 3) for ring in face]
            shape = Polygon(rings[0][:, :2], [r[:, :2] for r in rings[1:]])
            if not shape.intersects(lane):
                kept.append(face)
                continue
            vertices = np.concatenate(rings)
            matrix = np.column_stack((vertices[:, :2], np.ones(len(vertices))))
            plane, _, rank, _ = np.linalg.lstsq(matrix, vertices[:, 2], rcond=None)
            if rank < 3 or np.max(np.abs(matrix @ plane - vertices[:, 2])) > 1:
                raise ValueError(f"Expected a planar roof face at {index}")
            original_z = {(x, y): int(z) for x, y, z in vertices}
            for part in get_parts(shape.difference(lane)):
                if part.geom_type != "Polygon" or part.area < 1:
                    continue
                clipped = []
                for ring in (part.exterior, *part.interiors):
                    points = [
                        (
                            round(x),
                            round(y),
                            original_z.get((x, y), round(plane @ (x, y, 1))),
                        )
                        for x, y in ring.coords
                    ]
                    clipped.append([axis for point in points for axis in point])
                kept.append(clipped)
        if not kept:
            raise ValueError(
                f"The review should retain partial building geometry at {index}"
            )
        building[1] = kept


def bake(source, output, louisiana_points, pennsylvania_points, deck_points, obstacles):
    manifest_bytes = (source / "tiles" / "manifest.json").read_bytes()
    if digest(manifest_bytes) != PARENT:
        raise ValueError("The terrain review was scoped to a specific bundled manifest")
    manifest = json.loads(manifest_bytes)
    corrected_hashes = {}
    for entry in manifest["tiles"]:
        token = entry["t"]
        if token not in (LOUISIANA, PENNSYLVANIA):
            continue
        name = f"{token}.json.gz"
        original = (source / "tiles" / name).read_bytes()
        if digest(original) != PARENT_TILES[token]:
            raise ValueError(f"Reviewed base tile has changed at {token}")
        tile = json.loads(gzip.decompress(original))
        if token == LOUISIANA:
            tile["g"]["detail"] = refine(
                tile,
                (4726, -85, 4766, -45),
                observations(louisiana_points, extend=True),
            )
            trim_western_21st_street(tile)
            clear_reviewed_lane(tile, louisiana_points, *obstacles)
        else:
            terrain_height = observations(pennsylvania_points)

            def approach_height(x, y):
                if 4148 <= x <= 4168 and abs(y - pennsylvania_y(x)) <= 3.5:
                    return terrain_height(x, y)
                return None

            tile["g"]["detail"] = refine(
                tile,
                (4144, -568, 4256, -512),
                approach_height,
                lambda x, y: 4167.5 <= x <= 4238 and abs(y - pennsylvania_y(x)) <= 5.0,
            )
            # A lower class-17 deck crosses the rail cutting. The next deck above it
            # belongs to I-280. Store only the supported lower strip for display.
            points = np.load(deck_points, allow_pickle=False)
            lower = points[(points[:, 2] >= 14.7) & (points[:, 2] <= 15.6)]
            index = cKDTree(lower[:, :2])
            positions = []
            for x in range(4168, 4193):
                y = pennsylvania_y(x)
                for offset in (-3.5, 0, 3.5):
                    if len(index.query_ball_point((x, y + offset), 0.5)) < 8:
                        raise ValueError(f"Unsupported lower deck at {x}, {y + offset}")
                select = index.query_ball_point((x, y), 0.5)
                positions.append(
                    [x, round(y, 4), round(float(np.median(lower[select, 2])), 4)]
                )
            tile["raisedRoads"].append(
                {
                    "id": DECK_ID,
                    "kind": "bridge",
                    "points": positions,
                    "profiles": ["review/23rd-street-class17"],
                }
            )
        raw = gzip.compress(encoded(tile), compresslevel=9, mtime=0)
        (output / "tiles" / name).write_bytes(raw)
        entry["bytes"] = len(raw)
        corrected_hashes[token] = {
            "parentSha256": digest(original),
            "sha256": digest(raw),
        }
    manifest["terrainReview"] = {
        "parentManifestSha256": PARENT,
        "source": "USGS 3DEP CA_SanFrancisco_1_B23",
        "tiles": corrected_hashes,
        "addedRoads": [DECK_ID],
        "groundCropsSha256": [
            digest(p.read_bytes()) for p in (louisiana_points, pennsylvania_points)
        ],
        "lowerDeckCropSha256": digest(deck_points.read_bytes()),
        "laneReviewCropsSha256": [digest(path.read_bytes()) for path in obstacles],
    }
    raw = encoded(manifest)
    (output / "tiles" / "manifest.json").write_bytes(raw)
    route = (source / "quest-route.json").read_bytes()
    if (
        json.loads(route)["manifestSha256"] != PARENT
        or route.count(PARENT.encode()) != 1
    ):
        raise ValueError("The quest route must pin the reviewed parent manifest")
    (output / "quest-route.json").write_bytes(
        route.replace(PARENT.encode(), digest(raw).encode(), 1)
    )
    print(digest(raw))


if __name__ == "__main__":
    cli = argparse.ArgumentParser(description=__doc__)
    cli.add_argument("--source", type=Path, required=True)
    cli.add_argument("--output", type=Path, required=True)
    cli.add_argument("--louisiana-ground", type=Path, required=True)
    cli.add_argument("--pennsylvania-ground", type=Path, required=True)
    cli.add_argument("--pennsylvania-deck", type=Path, required=True)
    cli.add_argument(
        "--lane-review-crops",
        type=Path,
        nargs=3,
        required=True,
        metavar=("EAST_GROUND", "WEST_RETURNS", "EAST_RETURNS"),
    )
    args = cli.parse_args()
    (args.output / "tiles").mkdir(parents=True, exist_ok=True)
    bake(
        args.source,
        args.output,
        args.louisiana_ground,
        args.pennsylvania_ground,
        args.pennsylvania_deck,
        args.lane_review_crops,
    )
