"""Project the manifest's S2 level-13 corners into the game's planar frame."""

import json
from pathlib import Path

from pyproj import Transformer
from s2sphere import Cell, CellId, LatLng


def main():
    sf = Path(__file__).resolve().parents[2] / "public_html/src/sf"
    manifest = json.loads((sf / "tiles/manifest.json").read_text())
    ox, oy = manifest["origin_epsg7131"]
    # Interpret S2 lat/lon in the same NAD83(2011) frame as the baked tile origins.
    project = Transformer.from_crs(6318, 7131, always_xy=True)
    rows = []
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
        rows.append(f"  {json.dumps(token)}: {json.dumps(ring)}")
    (sf / "coverage.json").write_text("{\n" + ",\n".join(rows) + "\n}\n")


if __name__ == "__main__":
    main()
