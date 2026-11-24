"""Build the coverage map from S2 cells, city streets and DataSF polygons."""

import gzip
import json
from argparse import ArgumentParser
from pathlib import Path

from pyproj import Transformer
from s2sphere import Cell, CellId, LatLng
from shapely import get_parts, line_merge, make_valid, union_all
from shapely.geometry import MultiLineString, box, shape
from shapely.ops import transform


def outline(file, projection, extent, min_area):
    """Keep display-scale polygons and their holes in the game frame."""
    features = json.loads(file.read_text())["features"]
    polygons = [
        make_valid(transform(projection, shape(f["geometry"])))
        for f in features
        if f["geometry"]
    ]
    clipped = union_all(polygons).intersection(extent).simplify(16)
    return [
        [
            [[round(x), round(y)] for x, y in r.coords]
            for r in [p.exterior, *p.interiors]
        ]
        for p in get_parts(clipped)
        if p.geom_type == "Polygon" and p.area >= min_area
    ]


def major_streets(tiles_dir, tiles):
    lines = []
    bridge = {}
    for tile in tiles:
        with gzip.open(tiles_dir / f'{tile["t"]}.json.gz', "rt") as file:
            data = json.load(file)
        ox, oy = data["o"]
        for _, road_class, _, xy, _ in data["r"]:
            # The Gerardus city view also uses DataSF classes 1–3 at L10.
            if road_class in (1, 2, 3) and len(xy) >= 4:
                lines.append(
                    [(ox + x / 10, oy + y / 10) for x, y in zip(xy[::2], xy[1::2])]
                )
        for road in data.get("playableRoads", []):
            if road["id"].startswith("sf-contact/golden-gate-northbound-"):
                # The carriageways are adjacent in the minimap. Draw one line
                # and deduplicate full contact strips shared by neighbouring tiles.
                bridge[road["id"]] = [tuple(point[:2]) for point in road["points"]]
    lines.extend(bridge.values())
    simplified = line_merge(MultiLineString(lines)).simplify(12)
    return [
        [[round(x), round(y)] for x, y in part.coords] for part in get_parts(simplified)
    ]


def main():
    parser = ArgumentParser(description=__doc__)
    parser.add_argument("parks", type=Path, nargs="?", help="DataSF gtr9-ntp6 GeoJSON")
    parser.add_argument("--tiles", type=Path, help="Baked tiles directory")
    parser.add_argument("--coverage", type=Path, help="Coverage map to update")
    args = parser.parse_args()
    src = Path(__file__).resolve().parents[2] / "public_html/src"
    tiles = args.tiles or src / "sf/tiles"
    coverage = args.coverage or src / "sf/coverage.json"
    manifest = json.loads((tiles / "manifest.json").read_text())
    ox, oy = manifest["origin_epsg7131"]
    # Interpret S2 lat/lon in the same NAD83(2011) frame as the baked tile origins.
    project = Transformer.from_crs(6318, 7131, always_xy=True)
    cells = {}
    for tile in manifest["tiles"]:
        token = tile["t"]
        cid = CellId.from_token(token)
        if not cid.is_valid() or cid.level() != 13:
            raise ValueError(f"{token} is not a valid S2 level-13 cell")
        cell = Cell(cid)
        ring = []
        for i in range(4):
            ll = LatLng.from_point(cell.get_vertex(i))
            x, y = project.transform(ll.lng().degrees, ll.lat().degrees)
            ring.append([round(x - ox, 3), round(y - oy, 3)])
        cells[token] = ring
    xs, ys = zip(*(p for ring in cells.values() for p in ring))
    extent = box(min(xs) - 100, min(ys) - 100, max(xs) + 100, max(ys) + 100)
    geographic = Transformer.from_crs(4326, 7131, always_xy=True)

    def local_project(lon, lat):
        x, y = geographic.transform(lon, lat)
        return [v - ox for v in x], [v - oy for v in y]

    if args.parks:
        layers = {
            "coast": outline(
                src / "geo/sf-coastline.geojson", local_project, extent, 3000
            ),
            "parks": outline(args.parks, local_project, extent, 5000),
        }
    else:
        # A road bake can refresh the minimap against the existing land layers.
        existing = json.loads(coverage.read_text())
        layers = {name: existing[name] for name in ("coast", "parks")}
    layers["arterials"] = major_streets(tiles, manifest["tiles"])
    cell_rows = ",\n".join(
        f"    {json.dumps(k)}: {json.dumps(v)}" for k, v in cells.items()
    )
    layer_rows = []
    for name, features in layers.items():
        rows = ",\n".join(
            f"    {json.dumps(f, separators=(',', ':'))}" for f in features
        )
        layer_rows.append(f'  "{name}": [\n{rows}\n  ]')
    coverage.write_text(
        '{\n  "cells": {\n' + cell_rows + "\n  },\n" + ",\n".join(layer_rows) + "\n}\n"
    )


if __name__ == "__main__":
    main()
