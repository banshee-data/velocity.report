#!/usr/bin/env python3
"""Bake the Bay Bridge and I-80 game drive into an immutable Gerardus city revision."""

import argparse
from bisect import bisect_right
from collections import defaultdict
import gzip
from hashlib import sha256
import json
import math
from pathlib import Path
import shutil
import tempfile

BRIDGE = "bay-bridge-westbound"
EXIT = "fremont-exit"
STREETS = "city-streets"
ENTRANCE = "fourth-street-entrance"
HIGHWAY = "westbound-i80"

# Ordered, directional DataSF street records between the two ramp terminals.
STREET_IDS = (
    "gms:51ESECQNRK79CKT9752RAYJGAZ",
    "gms:7N49A5FG4S8V78C2DDEEZR4YET",
    "gms:0JK5QCNXNJ2TY5CT1DYT3JB1R6",
    "gms:76TDKPTM7K2AGJFEVVWZZKC8HE",
    "gms:1GTT1A2TYB9WYMX3G1GKTTFJD7",
    "gms:48ESN2BPRCA5W23YW6VPW5Y98A",
    "gms:7675V7W48EDGJH9BHETQNY6H4M",
    "gms:1DHB6J94NEMHPXFJXXW8X8TGB0",
    "gms:02HQRV5AXJ02X1T1AXQG95YVJ5",
    "gms:4H4F6R7477Y0CEAFW55204659D",
    "gms:1307PYDB0MGQSWXTVJYTZFEY3W",
    "gms:5PP0BSVK6RANX4E1YJXBZB280P",
    "gms:485PJ3T7D87XA75RWQJ1XH5Z2V",
    "gms:007M9C8QDXQ0V3KCZPAVYYSBD2",
)

SCENE_EXCLUSIONS = {
    # Exact objects in the pinned base that intersect the authored highway.
    # These adjustments apply to the game drive's scene only.
    "8085803c": {"b": {23: [2, 41, 1]}},
    "80858074": {"b": {6: [38, 18, None], 42: [32, 195, 1]}},
    "8085807c": {"tr": {1698: [-216, 2905, 20, 4]}},
    "80858084": {"b": {941: [29, 1, 1]}},
}


def digest(raw):
    return sha256(raw).hexdigest()


def encoded(value):
    return (
        json.dumps(value, sort_keys=True, separators=(",", ":"), allow_nan=False) + "\n"
    ).encode()


def checked(path, info):
    raw = path.read_bytes()
    if len(raw) != info["bytes"] or digest(raw) != info["sha256"]:
        raise ValueError(f"Source changed at {path}")
    return json.loads(raw)


class Pavement:
    def __init__(self, path, manifest):
        raw = path.read_bytes()
        if digest(raw) != manifest["base_manifest_sha256"]:
            raise ValueError(
                "The game drive and city roads need the same base pavement"
            )
        self.directory = path.parent
        self.tiles = json.loads(raw)["tiles"]
        self.hashes = manifest["tiles"]
        self.cache = {}

    def height(self, x, y):
        tile = min(self.tiles, key=lambda t: math.hypot(t["x"] - x, t["y"] - y))
        token = tile["t"]
        if token not in self.cache:
            raw = (self.directory / f"{token}.json.gz").read_bytes()
            if digest(raw) != self.hashes[token]["base_tile_gzip_sha256"]:
                raise ValueError(f"Base pavement tile changed at {token}")
            self.cache[token] = json.loads(gzip.decompress(raw))
        data = self.cache[token]
        g = data["g"]
        ox, oy = data["o"]
        n = g["n"]
        fx = (x - ox - g["x0"]) / g["s"]
        fy = (y - oy - g["y0"]) / g["s"]
        if not (0 <= fx <= n - 1 and 0 <= fy <= n - 1):
            raise ValueError(f"Street route rests outside mapped terrain at {x}, {y}")
        i, j = min(int(fx), n - 2), min(int(fy), n - 2)
        u, v = fx - i, fy - j
        row = j * n + i
        z = [g["z"][k] for k in (row, row + 1, row + n, row + n + 1)]
        if -32768 in z:
            raise ValueError(f"The game route crosses a terrain gap at {x}, {y}")
        weights = (1 - u - v, u, v, 0) if u + v <= 1 else (0, 1 - v, 1 - u, u + v - 1)
        return sum(h * w for h, w in zip(z, weights)) / 10


def distance(a, b):
    return math.hypot(a[0] - b[0], a[1] - b[1])


def concatenate(parts):
    out = []
    for part in parts:
        for row in part:
            if out and distance(out[-1], row) < 0.002:
                if out[-1][2] is None:
                    out[-1] = row
                continue
            out.append(row)
    return out


def chainage(points):
    stations = [0]
    for a, b in zip(points, points[1:]):
        stations.append(stations[-1] + distance(a, b))
    return stations


def join_missing(points, limit_m):
    """Interpolate explicitly authored game contact between known elevations."""
    stations = chainage(points)
    supported = [i for i, p in enumerate(points) if p[2] is not None]
    if supported[0] != 0 or supported[-1] != len(points) - 1:
        raise ValueError("A playable route end requires a fixed height")
    for i, j in zip(supported, supported[1:]):
        length = stations[j] - stations[i]
        if length > limit_m:
            raise ValueError(
                f"Game interpolation exceeds its authored limit: {length:.1f} m"
            )
        for k in range(i + 1, j):
            u = (stations[k] - stations[i]) / length
            points[k] = [*points[k][:2], points[i][2] * (1 - u) + points[j][2] * u]
    return points


def at_station(points, stations, s):
    # Extrapolate the terminal tangents for the symmetric filter at route ends.
    j = min(len(points) - 1, max(1, bisect_right(stations, s)))
    a, b = points[j - 1], points[j]
    u = (s - stations[j - 1]) / (stations[j] - stations[j - 1])
    return [v + (w - v) * u for v, w in zip(a, b)]


def resample(points, step):
    stations = chainage(points)
    count = math.ceil(stations[-1] / step)
    return [
        points[0],
        *(
            at_station(points, stations, n * stations[-1] / count)
            for n in range(1, count)
        ),
        points[-1],
    ]


def rounded_streets(points, pavement):
    # Round the intersections within the game carriageway so the contact
    # strip has a continuous tangent.
    result = [points[0][:2]]
    for a, b, c in zip(points, points[1:], points[2:]):
        ab, bc = distance(a, b), distance(b, c)
        dx, dy = (b[0] - a[0]) / ab, (b[1] - a[1]) / ab
        ex, ey = (c[0] - b[0]) / bc, (c[1] - b[1]) / bc
        if dx * ex + dy * ey > 0.99:
            result.append(b[:2])
            continue
        trim = min(12, ab * 0.4, bc * 0.4)
        for i in range(9):
            u = i / 8
            result.append(
                [
                    b[0] + trim * (ex * u * u - dx * (1 - u) ** 2),
                    b[1] + trim * (ey * u * u - dy * (1 - u) ** 2),
                ]
            )
    result.append(points[-1][:2])
    return [[x, y, pavement.height(x, y)] for x, y in resample(result, 2)]


def smooth_route(sections, pavement):
    # Filter one continuous, metre-parameterised path before splitting it into
    # surfaces again. Filtering individual sections leaves a kink at every join.
    points = []
    bounds = []
    for section in sections:
        part = section["positions"]
        if points and any(abs(a - b) > 0.002 for a, b in zip(points[-1], part[0])):
            raise ValueError(f"Open gameplay seam before {section['id']}")
        first = max(0, len(points) - 1)
        points.extend(part[1:] if points else part)
        bounds.append((first, len(points)))
    stations = chainage(points)
    weights = (1, 8, 28, 56, 70, 56, 28, 8, 1)
    smooth = []
    for s in stations:
        samples = [
            at_station(points, stations, s + 3 * (i - 4)) for i in range(len(weights))
        ]
        smooth.append(
            [
                sum(value * w for value, w in zip(axis, weights)) / 256
                for axis in zip(*samples)
            ]
        )
    smooth[0], smooth[-1] = points[0], points[-1]

    def match_ground(index, direction):
        end = index + direction
        while abs(stations[end] - stations[index]) < 24:
            end += direction
        ground = index - direction
        length = abs(stations[end] - stations[index])
        terrain_grade = (smooth[index][2] - smooth[ground][2]) / distance(
            smooth[index], smooth[ground]
        )
        end_grade = (smooth[end][2] - smooth[end - direction][2]) / distance(
            smooth[end], smooth[end - direction]
        )
        start_z, end_z = smooth[index][2], smooth[end][2]
        for i in range(index + direction, end, direction):
            u = abs(stations[i] - stations[index]) / length
            smooth[i][2] = (
                (2 * u**3 - 3 * u**2 + 1) * start_z
                + (u**3 - 2 * u**2 + u) * length * terrain_grade
                + (-2 * u**3 + 3 * u**2) * end_z
                + (u**3 - u**2) * length * end_grade
            )

    for section, (first, last) in zip(sections, bounds):
        if section["kind"] == "ground":
            for i in range(first, last):
                smooth[i][2] = pavement.height(*smooth[i][:2])
            match_ground(first, -1)
            match_ground(last - 1, 1)
    for section, (first, last) in zip(sections, bounds):
        section["positions"] = [
            [round(v, 4) for v in point] for point in smooth[first:last]
        ]


def interpolate_cubic(a, b, tangent_a, tangent_b, t):
    return (
        (2 * t**3 - 3 * t**2 + 1) * a
        + (t**3 - 2 * t**2 + t) * tangent_a
        + (-2 * t**3 + 3 * t**2) * b
        + (t**3 - t**2) * tangent_b
    )


def smooth_ramp_join(left, right, pavement):
    """Round the authored ramp/street join with matched heading and grade."""
    before, after = left["positions"], right["positions"]
    # Use roughly eight metres on each side of the baseline two-metre stations.
    count = 4
    old = before[-count - 1 :] + after[1 : count + 1]
    arc = chainage(old)
    length = arc[-1]
    a, b = old[0], old[-1]
    outer_a, outer_b = before[-count - 2], after[count + 1]
    scale_a = length / distance(outer_a, a)
    scale_b = length / distance(b, outer_b)

    def xy_at(s):
        return [
            interpolate_cubic(
                a[i],
                b[i],
                (a[i] - outer_a[i]) * scale_a,
                (outer_b[i] - b[i]) * scale_b,
                s / length,
            )
            for i in range(2)
        ]

    if any(distance(point, xy_at(s)) > 0.65 for point, s in zip(old, arc)):
        raise ValueError(f"Ramp join exceeds its authored corridor at {left['id']}")

    def sample_xy(first, last):
        count = math.ceil(last - first)
        return [xy_at(first + i * (last - first) / count) for i in range(count + 1)]

    join = arc[count]
    left_xy = sample_xy(0, join)
    right_xy = sample_xy(join, length)

    def match_ramp(mesh, start_z, end_z, start_grade, end_grade):
        stations = chainage(mesh)
        length = stations[-1]
        return [
            [
                x,
                y,
                interpolate_cubic(
                    start_z,
                    end_z,
                    start_grade * length,
                    end_grade * length,
                    s / length,
                ),
            ]
            for (x, y), s in zip(mesh, stations)
        ]

    if left["kind"] == "ground":
        left_patch = [[x, y, pavement.height(x, y)] for x, y in left_xy]
        left_patch[0] = a
        first, end = left_patch[-2:]
        grade = (end[2] - first[2]) / distance(first, end)
        right_patch = match_ramp(
            right_xy,
            end[2],
            b[2],
            grade,
            (outer_b[2] - b[2]) / distance(b, outer_b),
        )
        right_patch[0], right_patch[-1] = end, b
    else:
        right_patch = [[x, y, pavement.height(x, y)] for x, y in right_xy]
        right_patch[-1] = b
        start, next_point = right_patch[:2]
        grade = (next_point[2] - start[2]) / distance(start, next_point)
        left_patch = match_ramp(
            left_xy,
            a[2],
            start[2],
            (a[2] - outer_a[2]) / distance(outer_a, a),
            grade,
        )
        left_patch[0], left_patch[-1] = a, start
    left["positions"] = before[: -count - 1] + [
        [round(value, 4) for value in point] for point in left_patch
    ]
    right["positions"] = [
        [round(value, 4) for value in point] for point in right_patch
    ] + after[count + 1 :]


def bake(source, base):
    manifest = json.loads((source / "manifest.json").read_bytes())
    if (
        manifest["format"] != "gerardus-sf-city-road-layers"
        or manifest["compatibility_version"] != 2
        or manifest["state"] != "citywide_review"
    ):
        raise ValueError("The game route requires the version-2 citywide road layers")
    pavement = Pavement(base, manifest)
    profiles = defaultdict(list)
    streets = {}
    for token, info in manifest["tiles"].items():
        tile = checked(source / f"{token}.json", info)
        for layer in ("bridge", "overpass"):
            for row in tile[layer]:
                profiles[row["cnn"]].append((token, row))
        for row in tile["road"]:
            if row["id"] in STREET_IDS:
                streets[row["id"]] = (token, row)
    references = {}

    def highway(*cnns, layer):
        span = []
        refs = []
        for cnn in cnns:
            rows = sorted(profiles[cnn], key=lambda v: v[1]["source_chainage_m"])
            if not rows or any(
                row["routing_status"] != "requires_review" for _, row in rows
            ):
                raise ValueError(f"Expected provisional source spans for {cnn}")
            for token, row in rows:
                span.append([p["position"] for p in row["profile"]])
                refs.append({"owner": token, "id": row["id"]})
        references[layer] = refs
        return concatenate(span)

    bridge = highway("14249000", layer=BRIDGE)
    if any(p[2] is None for p in bridge) or chainage(bridge)[-1] < 2799:
        raise ValueError("The west span requires continuous source contact")
    approach = highway("19382000", layer="city-approach")
    bridge.extend(approach[:1])
    approach = join_missing(approach, 22)
    exit_points = highway("19375001", "19375002", layer=EXIT)
    street_points = []
    for i, id in enumerate(STREET_IDS):
        owner, row = streets[id]
        profile = [p["position"] for p in row["profile"]]
        a, b = profile[0], profile[-1]
        length = distance(a, b)
        if any(
            abs((p[0] - a[0]) * (b[1] - a[1]) - (p[1] - a[1]) * (b[0] - a[0])) / length
            > 2
            for p in profile
        ):
            raise ValueError(f"The authored street leg differs from its source at {id}")
        street_points.append([b, a] if i == 0 else [a, b])
        references.setdefault(STREETS, []).append({"owner": owner, "id": id})
    street = rounded_streets(concatenate(street_points), pavement)
    if distance(exit_points[-1], street[0]) > 6:
        raise ValueError(
            "The Fremont ramp terminal no longer meets the authored street route"
        )
    exit_points[-1] = street[0]
    exit_points = join_missing(exit_points, 82)
    on = highway("19380000", layer=ENTRANCE)
    if distance(street[-1], on[0]) > 0.01:
        raise ValueError("The Fourth Street ramp terminal no longer meets the street")
    on[0] = street[-1]
    on = join_missing(on, 90)
    highway_points = join_missing(highway("19386000", layer=HIGHWAY), 16)
    if (
        distance(on[-1], highway_points[0]) > 0.11
        or abs(on[-1][2] - highway_points[0][2]) > 0.1
    ):
        raise ValueError("The re-entry merge does not meet the westbound continuation")
    on[-1] = highway_points[0]

    sections = []
    for id, label, kind, width, rails, points in (
        (BRIDGE, "Bay Bridge west span", "bridge_deck", 10, True, bridge),
        ("city-approach", "I-80 westbound approach", "bridge_deck", 8, True, approach),
        (EXIT, "Fremont Street exit", "ramp", 6, True, exit_points),
        (STREETS, "City streets to Fourth Street", "ground", 7, False, street),
        (ENTRANCE, "Fourth Street entrance", "ramp", 6, True, on),
        (HIGHWAY, "Back on I-80 westbound", "bridge_deck", 8, True, highway_points),
    ):
        part = resample(points, 2)
        sections.append(
            {
                "id": id,
                "label": label,
                "kind": kind,
                "width_m": width,
                "rail": rails,
                **({"rail_sides": ["right"]} if id == EXIT else {}),
                "positions": part,
                "sources": references[id],
            }
        )
    smooth_route(sections, pavement)
    by_id = {section["id"]: section for section in sections}
    smooth_ramp_join(by_id[EXIT], by_id[STREETS], pavement)
    smooth_ramp_join(by_id[STREETS], by_id[ENTRANCE], pavement)
    return manifest, {
        "format": "sf-city-game-drives",
        "source_revision": manifest["revision"],
        "base_manifest_sha256": manifest["base_manifest_sha256"],
        "drives": [
            {
                "id": "bay-bridge-highway-drive",
                "label": "Bay Bridge · Fremont exit · Fourth Street entrance",
                "status": "authored_game_drive",
                "sections": sections,
                "scene_exclusions": SCENE_EXCLUSIONS,
            }
        ],
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", type=Path, required=True)
    parser.add_argument("--base", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args()
    index, drive = bake(args.source, args.base)
    payload = encoded(drive)
    index["files"]["gamedrives.json"] = {
        "bytes": len(payload),
        "sha256": digest(payload),
    }
    index.pop("revision")
    revision = f"sf-city-roads-{digest(encoded(index))}"
    index["revision"] = revision
    args.out.mkdir(parents=True, exist_ok=True)
    target = args.out / revision
    if not target.exists():
        with tempfile.TemporaryDirectory(dir=args.out) as temporary:
            stage = Path(temporary) / revision
            stage.mkdir()
            for name in [*index["tiles"], "sources", "candidates"]:
                shutil.copyfile(args.source / f"{name}.json", stage / f"{name}.json")
            (stage / "gamedrives.json").write_bytes(payload)
            (stage / "manifest.json").write_bytes(encoded(index))
            stage.rename(target)
    if (target / "manifest.json").read_bytes() != encoded(index):
        raise ValueError("Immutable city revision differs")
    for section in drive["drives"][0]["sections"]:
        print(
            section["label"], f"{chainage(section['positions'])[-1]:.1f} m", flush=True
        )
    print(target, flush=True)


if __name__ == "__main__":
    main()
