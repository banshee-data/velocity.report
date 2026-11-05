"""Pin the city-frame arithmetic against what Gerardus itself wrote.

A placement a metre out still draws; it just draws a scene beside the street
it recorded. These tests hold the S2 cell centres, the ENU frames and the
site-to-city transform to Gerardus's own numbers: its city-tiles fixture
(every tile's origin in the city frame, and the root transform) and one
site's GeoJSON, read in the site frame and in WGS 84.
"""

import json
import math
import os
import sys
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import city_frame  # noqa: E402

FIXTURES = os.path.join(HERE, "testdata", "city-view")


def load(name):
    with open(os.path.join(FIXTURES, name)) as fh:
        return json.load(fh)


def tiles(node):
    if node.get("extras", {}).get("s2"):
        yield node
    for child in node.get("children", []):
        yield from tiles(child)


class TilesetTest(unittest.TestCase):
    def setUp(self):
        self.tileset = load("tileset.json")

    def test_the_root_transform_is_the_city_frame(self):
        city_frame.check_tileset(self.tileset)
        root = self.tileset["root"]["transform"]
        for got, want in zip(root, city_frame.root_transform()):
            self.assertAlmostEqual(got, want, places=6)

    def test_a_moved_origin_is_refused(self):
        moved = json.loads(json.dumps(self.tileset))
        moved["root"]["transform"][12] += 0.5
        with self.assertRaises(ValueError):
            city_frame.check_tileset(moved)

    def test_every_tile_origin_is_its_cell_centre_in_the_city_frame(self):
        city = city_frame.city_frame()
        seen = {10: 0, 13: 0, 16: 0}
        for tile in tiles(self.tileset["root"]):
            token = tile["extras"]["s2"]
            seen[city_frame.cell_level(city_frame.cell_id(token))] += 1
            lat, lon = city_frame.cell_centre(token)
            got = city.from_geodetic(lat, lon, 0.0)
            for g, w in zip(got, tile["extras"]["origin_city_enu_m"]):
                # Gerardus rounds to the millimetre.
                self.assertAlmostEqual(g, w, delta=0.0005, msg=token)
        self.assertEqual(seen[10], 1)
        self.assertEqual(seen[13], 2)
        self.assertGreater(seen[16], 10)

    def test_site_to_city_puts_a_site_origin_on_its_l16_tile(self):
        for tile in tiles(self.tileset["root"]):
            token = tile["extras"]["s2"]
            if len(token) != 9:
                continue
            m = city_frame.site_to_city(token)
            origin = city_frame.apply(m, [0.0, 0.0, 0.0])
            for g, w in zip(origin, tile["extras"]["origin_city_enu_m"]):
                self.assertAlmostEqual(g, w, delta=0.0005, msg=token)


class GerardusGeojsonTest(unittest.TestCase):
    """columbus-broadway, as Gerardus served it on 2026-11-05."""

    def setUp(self):
        self.fixture = load("gerardus-sites-columbus-broadway.json")
        self.epoch = self.fixture["epoch"]
        self.site = self.fixture["sites"][0]

    def test_the_site_origin_is_the_one_gerardus_serves(self):
        lat, lon = city_frame.site_origin(self.site["l16"])
        lat, lon, _ = city_frame.nad83_2011_to_itrf2014(lat, lon, 0.0, self.epoch)
        # Gerardus rounds to 1e-10 degree, about 0.01 mm.
        self.assertAlmostEqual(lat, self.site["lat"], delta=2e-10)
        self.assertAlmostEqual(lon, self.site["lon"], delta=2e-10)

    def test_a_site_point_lands_where_the_wgs84_geojson_puts_it(self):
        vertex = self.fixture["kerb_vertex"]
        m = city_frame.site_to_city(self.site["l16"])
        city = city_frame.city_frame()
        lat, lon, h = city.to_geodetic(city_frame.apply(m, vertex["site_m"]))
        lat, lon, _ = city_frame.nad83_2011_to_itrf2014(lat, lon, h, self.epoch)
        want_lon, want_lat, want_h = vertex["wgs84"]
        metres_north = (lat - want_lat) * 111_000
        metres_east = (lon - want_lon) * 111_000 * math.cos(math.radians(want_lat))
        self.assertLess(math.hypot(metres_north, metres_east), 0.001)
        # Gerardus reports the NAVD88 height before its datum step, to 1 mm.
        self.assertAlmostEqual(h, want_h, delta=0.001)


class TransformTest(unittest.TestCase):
    def test_site_to_city_is_a_rotation(self):
        m = city_frame.site_to_city("808580f47")
        for a in range(3):
            for b in range(3):
                dot = sum(m[a][k] * m[b][k] for k in range(3))
                self.assertAlmostEqual(dot, 1.0 if a == b else 0.0, places=12)
        # The meridians differ by a few hundredths of a degree across the city.
        self.assertGreater(m[0][0], 0.99999)

    def test_compose_applies_the_right_matrix_last(self):
        quarter = city_frame.pose_matrix(((0, -1, 0), (1, 0, 0), (0, 0, 1)), (1, 2, 3))
        shift = city_frame.pose_matrix(((1, 0, 0), (0, 1, 0), (0, 0, 1)), (10, 0, 0))
        p = city_frame.apply(city_frame.compose(shift, quarter), [1, 0, 0])
        self.assertEqual(p, [11, 3, 3])

    def test_bad_tokens_are_refused(self):
        for token in ("", "0", "f" * 17, "e000000000000000"):
            with self.assertRaises(ValueError):
                city_frame.cell_id(token)
        with self.assertRaises(ValueError):
            city_frame.site_origin("8085805c")  # an L13 cell is not a site


if __name__ == "__main__":
    unittest.main()
