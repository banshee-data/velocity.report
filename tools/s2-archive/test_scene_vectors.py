"""Cover the parts of scene-vectors.py that would be wrong without anyone seeing.

A feature placed by the wrong side of the pose still draws; it just draws
beside the street it belongs to. These tests pin the transform, what a
published feature may carry, and the check that refuses a pose that does not
fit the scene.
"""

import gzip
import importlib.util
import json
import math
import os
import stat
import sys
import tempfile
import unittest
from unittest import mock

HERE = os.path.dirname(os.path.abspath(__file__))

spec = importlib.util.spec_from_file_location(
    "scene_vectors", os.path.join(HERE, "scene-vectors.py")
)
scene_vectors = importlib.util.module_from_spec(spec)
sys.modules["scene_vectors"] = scene_vectors
spec.loader.exec_module(scene_vectors)


def yaw_pose(deg, t=(10.0, -20.0, 23.0)):
    """A pose turning the sensor deg about the vertical, at t in the site frame."""
    half = math.radians(deg) / 2
    return {
        "translationM": {"x": t[0], "y": t[1], "z": t[2]},
        "rotation": {"w": math.cos(half), "x": 0.0, "y": 0.0, "z": math.sin(half)},
        "siteFrameId": "site/808580f47",
        "sensorFrameId": "sensor/hesai-pandar40p",
        "source": "POSE_SOURCE_ICP",
    }


def to_site(pose, p):
    R = scene_vectors.rotation(pose["rotation"])
    t = [pose["translationM"][k] for k in "xyz"]
    return [sum(R[r][c] * p[c] for c in range(3)) + t[r] for r in range(3)]


def feature(layer, geometry, **props):
    return {
        "type": "Feature",
        "id": f"gms:{layer}",
        "geometry": geometry,
        "properties": {"layer": layer, **props},
    }


class TransformTest(unittest.TestCase):
    def test_into_sensor_inverts_the_pose(self):
        pose = {
            "translationM": {"x": -14.1, "y": -41.9, "z": 23.3},
            "rotation": {"w": 0.3128, "x": -0.0071, "y": 0.0223, "z": -0.9495},
        }
        apply = scene_vectors.into_sensor(pose)
        for p in ([0, 0, 0], [5, -3, 2], [-40, 12, -2.5]):
            back = apply(to_site(pose, p))
            for a, b in zip(back, p):
                self.assertAlmostEqual(a, b, places=9)

    def test_heading_is_the_azimuth_of_sensor_y(self):
        # A quarter turn anticlockwise points sensor +Y west.
        self.assertAlmostEqual(scene_vectors.heading_deg(yaw_pose(90)), 270.0)
        self.assertAlmostEqual(scene_vectors.heading_deg(yaw_pose(0)), 0.0)

    def test_a_zero_quaternion_is_refused(self):
        with self.assertRaises(ValueError):
            scene_vectors.rotation({"w": 0, "x": 0, "y": 0, "z": 0})


class LayersTest(unittest.TestCase):
    def test_building_walls_replace_box_walls_and_footprints(self):
        served = [
            "street_use",
            "boxes",
            "box_walls",
            "footprints",
            "wall_surfaces",
            "osm",
            "trees",
        ]
        self.assertEqual(
            scene_vectors.wanted_layers(served),
            ["footprints", "wall_surfaces", "trees", "street_use"],
        )

    def test_footprints_stand_in_when_there_are_no_walls(self):
        served = ["footprints", "observed", "lane_markings", "intersections"]
        self.assertEqual(
            scene_vectors.wanted_layers(served),
            ["footprints", "observed"],
        )


class FeaturesTest(unittest.TestCase):
    pose = yaw_pose(30)

    def site(self, *p):
        return [round(v, 6) for v in to_site(self.pose, list(p))]

    def features(self, collection, radius=90):
        return scene_vectors.scene_features(
            collection, scene_vectors.into_sensor(self.pose), radius
        )

    def test_coordinates_come_back_in_the_sensor_frame_rounded(self):
        line = {
            "type": "LineString",
            "coordinates": [self.site(1.234, 2.345, -2.5), self.site(4, 5, -2.5)],
        }
        [f] = self.features({"features": [feature("city_streets", line, kind="road")]})
        self.assertEqual(
            f["geometry"]["coordinates"], [[1.23, 2.35, -2.5], [4.0, 5.0, -2.5]]
        )

    def test_only_geometry_and_its_context_are_kept(self):
        ring = [
            self.site(0, 5, -2.5),
            self.site(4, 5, -2.5),
            self.site(4, 5, 6),
            self.site(0, 5, -2.5),
        ]
        props = {
            "kind": "wall_surface",
            "state": "proposed",
            "name": "Some Building",
            "roof:shape": "gabled",
            "building:part": "yes",
            "party": "true",
            "height_m": "8.25",
            "model_id": "sha256:abc",
        }
        [f] = self.features(
            {
                "features": [
                    feature(
                        "wall_surfaces",
                        {"type": "Polygon", "coordinates": [ring]},
                        **props,
                    )
                ]
            }
        )
        self.assertEqual(
            f["properties"],
            {
                "layer": "wall_surfaces",
                "kind": "wall_surface",
                "evidence_status": "inferred",
                "source_ids": ["datasf-footprints", "3dep"],
                "state": "proposed",
                "height_m": 8.25,
            },
        )

    def test_travel_lanes_keep_their_direction_and_limit_only(self):
        line = {
            "type": "LineString",
            "coordinates": [self.site(1, -2, -2), self.site(30, -2, -2)],
        }
        lane = feature(
            "street_use",
            line,
            kind="travel_lane",
            lane_type="general",
            width_m="3.2",
            bound="E",
            direction_name="Kirkham St eastbound",
            speed_limit="25",
            speed_limit_unit="mph",
            speed_limit_source="sign",
            speed_limit_effective_from="2026-01-01",
            author="operator name",
            edited_at="2026-11-04T00:00:00Z",
            proposal="gms:p1",
            row_id="row-1",
        )
        space = feature(
            "street_use",
            {
                "type": "Polygon",
                "coordinates": [
                    [
                        self.site(0, 0, -2),
                        self.site(1, 0, -2),
                        self.site(1, 1, -2),
                        self.site(0, 0, -2),
                    ]
                ],
            },
            kind="parking_space",
        )
        [f] = self.features({"features": [lane, space]})
        self.assertEqual(
            f["properties"],
            {
                "layer": "street_use",
                "kind": "travel_lane",
                "evidence_status": "inferred",
                "source_ids": ["operator"],
                "lane_type": "general",
                "width_m": "3.2",
                "bound": "E",
                "direction_name": "Kirkham St eastbound",
                "speed_limit": "25",
                "speed_limit_unit": "mph",
                "speed_limit_source": "sign",
                "speed_limit_effective_from": "2026-01-01",
            },
        )

    def test_experimental_layers_are_not_exported(self):
        line = {
            "type": "LineString",
            "coordinates": [self.site(1, 1, -2), self.site(2, 2, -2)],
        }
        self.assertEqual(
            self.features(
                {
                    "features": [
                        feature(layer, line)
                        for layer in ("lane_markings", "street_furniture")
                    ]
                }
            ),
            [],
        )

    def test_registered_faces_are_check_only_and_tree_dimensions_are_kept(self):
        line = {
            "type": "LineString",
            "coordinates": [self.site(1, 1, -2), self.site(1, 1, 5)],
        }
        collection = {
            "features": [
                feature("wall_planes", line),
                feature("observed", line, kind="facade"),
                feature(
                    "trees", line, crown_radius_m="3.5", crown_base_m=2, height_m=10
                ),
            ]
        }
        [tree] = self.features(collection)
        self.assertEqual(tree["properties"]["layer"], "trees")
        self.assertEqual(tree["properties"]["crown_radius_m"], 3.5)
        checked = scene_vectors.scene_features(
            collection, scene_vectors.into_sensor(self.pose), include_registration=True
        )
        self.assertEqual(len(checked), 3)

    def test_gerardus_source_words_become_source_ids(self):
        ring = [
            self.site(0, 5, -2),
            self.site(3, 5, -2),
            self.site(3, 5, 2),
            self.site(0, 5, -2),
        ]
        [f] = self.features(
            {
                "features": [
                    feature(
                        "wall_surfaces",
                        {"type": "Polygon", "coordinates": [ring]},
                        source="datasf+registered-scans",
                    )
                ]
            }
        )
        self.assertEqual(f["properties"]["source_ids"], ["datasf-footprints", "scans"])

    def test_far_features_osm_and_registration_marks_are_left_out(self):
        near = {
            "type": "LineString",
            "coordinates": [self.site(1, 1, -2), self.site(2, 2, -2)],
        }
        far = {
            "type": "LineString",
            "coordinates": [self.site(200, 0, -2), self.site(210, 0, -2)],
        }
        collection = {
            "features": [
                feature("observed", near, kind="kerb"),
                feature("observed", near, kind="sensor"),
                feature("observed", near, kind="lane_marking"),
                feature("osm", near, kind="road"),
                feature("city_streets", far, kind="road"),
                feature(
                    "city_intersections",
                    {"type": "Point", "coordinates": self.site(0, 0, 0)},
                ),
            ]
        }
        kept = self.features(collection)
        self.assertEqual(
            [(f["properties"]["layer"], f["properties"]["kind"]) for f in kept],
            [("observed", "kerb")],
        )

    def test_features_come_in_drawing_order(self):
        line = {
            "type": "LineString",
            "coordinates": [self.site(1, 1, -2), self.site(2, 2, -2)],
        }
        collection = {
            "features": [
                feature("trees", line, kind="tree"),
                feature("city_streets", line, kind="road"),
                feature("lane_markings", line, kind="lane_marking"),
            ]
        }
        layers = [f["properties"]["layer"] for f in self.features(collection)]
        self.assertEqual(layers, ["city_streets", "trees"])


class PlacementCheckTest(unittest.TestCase):
    def walls(self):
        """Two walls at right angles, 10 m from the sensor."""
        rings = [
            [
                [-10, 10, -2.5],
                [10, 10, -2.5],
                [10, 10, 5],
                [-10, 10, 5],
                [-10, 10, -2.5],
            ],
            [
                [10, -10, -2.5],
                [10, 10, -2.5],
                [10, 10, 5],
                [10, -10, 5],
                [10, -10, -2.5],
            ],
        ]
        return [
            {
                "geometry": {"type": "Polygon", "coordinates": [r]},
                "properties": {"layer": "wall_planes"},
            }
            for r in rings
        ]

    def background(self, dx=0.0):
        points = []
        for z in (-2.0, 0.0, 2.0, 4.0):
            for u in range(-10, 11):
                points.append([u + dx, 10.0, z])
                points.append([10.0 + dx, float(u), z])
        return points

    def test_a_pose_that_fits_passes(self):
        check = scene_vectors.placement_check(self.walls(), self.background())
        self.assertTrue(check["passed"])
        self.assertGreater(check["at_pose"], max(check["moved"].values()))

    def test_a_pose_from_elsewhere_fails(self):
        check = scene_vectors.placement_check(self.walls(), self.background(dx=4.0))
        self.assertFalse(check["passed"])

    def test_nothing_to_check_fails(self):
        self.assertFalse(scene_vectors.placement_check([], self.background())["passed"])


class ModesTest(unittest.TestCase):
    def test_off_asks_nothing(self):
        with mock.patch.object(scene_vectors, "build_collection") as build:
            self.assertIsNone(
                scene_vectors.export_scene_vectors("s", "S", "/nope", "off")
            )
        build.assert_not_called()

    def test_no_pose_is_no_vectors_in_any_mode(self):
        for mode in ("auto", "required"):
            with mock.patch.object(
                scene_vectors,
                "build_collection",
                side_effect=scene_vectors.NoPose("none"),
            ):
                said = []
                self.assertIsNone(
                    scene_vectors.export_scene_vectors(
                        "s", "S", "/nope", mode, say=said.append
                    )
                )
                self.assertIn("vectors: none", said[0])

    def test_an_unreachable_gerardus_fails_only_when_required(self):
        error = scene_vectors.Unreachable("refused")
        with mock.patch.object(scene_vectors, "build_collection", side_effect=error):
            self.assertIsNone(
                scene_vectors.export_scene_vectors(
                    "s", "S", "/nope", "auto", say=lambda m: None
                )
            )
            with self.assertRaises(scene_vectors.Unreachable):
                scene_vectors.export_scene_vectors(
                    "s", "S", "/nope", "required", say=lambda m: None
                )


class SourcesTest(unittest.TestCase):
    def test_osm_and_local_paths_are_left_out(self):
        site = {
            "site": {
                "sources": [
                    {
                        "layer": "REFERENCE_LAYER_AERIAL_POINTS",
                        "name": "USGS 3DEP",
                        "uri": "/Volumes/banshee-results/sf-gis/ept.json",
                        "licence": "public-domain",
                        "attribution": "U.S. Geological Survey",
                    },
                    {
                        "layer": "REFERENCE_LAYER_OSM",
                        "name": "OpenStreetMap",
                        "licence": "ODbL-1.0",
                    },
                ]
            }
        }
        with mock.patch.object(scene_vectors, "grpc", return_value=site):
            _, sources = scene_vectors.site_sources("808580f47")
        self.assertEqual(sorted(sources), ["3dep", "operator", "scans"])
        self.assertNotIn("uri", sources["3dep"])
        self.assertNotIn(
            "retrieved_at", sources["3dep"], "an absent member stays absent"
        )
        self.assertNotIn("/Volumes", json.dumps(sources))


class WriteTest(unittest.TestCase):
    def test_the_file_is_gzipped_and_readable_by_others(self):
        with tempfile.TemporaryDirectory() as assets:
            path = scene_vectors.write_collection(
                {"type": "FeatureCollection", "features": []}, assets
            )
            self.assertEqual(
                path, os.path.join(assets, "vectors", "vectors.geojson.gz")
            )
            self.assertEqual(stat.S_IMODE(os.stat(path).st_mode), 0o644)
            with gzip.open(path) as fh:
                self.assertEqual(json.load(fh)["type"], "FeatureCollection")
            self.assertEqual(
                os.listdir(os.path.join(assets, "vectors")), ["vectors.geojson.gz"]
            )


class ContextTest(unittest.TestCase):
    def test_building_context_extends_to_twice_the_other_feature_radius(self):
        geometry = {"type": "LineString", "coordinates": [[150, 0, 0], [155, 0, 0]]}
        features = scene_vectors.scene_features(
            {
                "features": [
                    feature("city_streets", geometry),
                    feature("wall_surfaces", geometry),
                    feature("trees", geometry),
                ]
            },
            lambda p: p,
        )
        self.assertEqual(
            [f["properties"]["layer"] for f in features], ["wall_surfaces"]
        )
        self.assertEqual(
            scene_vectors.scene_features(
                {"features": [feature("wall_surfaces", geometry)]},
                lambda p: p,
                include_registration=True,
            ),
            [],
        )
        geometry["coordinates"] = [[190, 0, 0], [195, 0, 0]]
        self.assertEqual(
            scene_vectors.scene_features(
                {"features": [feature("wall_surfaces", geometry)]}, lambda p: p
            ),
            [],
        )

    def test_collection_records_true_vertical_and_separate_building_coverage(self):
        from contextlib import ExitStack

        pose = yaw_pose(0)
        pose["rotation"] = {
            "w": math.cos(math.pi / 12),
            "x": math.sin(math.pi / 12),
            "y": 0,
            "z": 0,
        }
        with ExitStack() as stack:
            for name, value in dict(
                scene_cell=("cell", "Title"),
                served_layers=[],
                accepted_scan=({}, {"pose": pose}, {}),
                site_sources=({}, {}),
                site_geometry={"features": []},
                read_background={"points": []},
                placement_check={"passed": True},
                http_json=dict(nx=2, ny=2, x0=130, y0=-20, cell_m=1, heights=[20] * 4),
            ).items():
                stack.enter_context(
                    mock.patch.object(scene_vectors, name, return_value=value)
                )
            collection = scene_vectors.build_collection("site", "Title", "/unused")
        up = collection["placement"]["up_sensor"]
        R = scene_vectors.rotation(pose["rotation"])
        for r, expected in enumerate([0, 0, 1]):
            self.assertAlmostEqual(
                sum(R[r][c] * up[c] for c in range(3)), expected, places=8
            )
        self.assertEqual(collection["building_clip_radius_m"], 180)
        self.assertEqual(collection["clip_radius_m"], 90)
        self.assertEqual(collection["ground_clip_radius_m"], 180)
        self.assertTrue(collection["ground"]["indices"])
        self.assertGreater(
            max(math.hypot(*p[:2]) for p in collection["ground"]["positions"]), 90
        )


class GroundTest(unittest.TestCase):
    def test_slope_and_pose_are_preserved(self):
        grid = dict(x0=10, y0=-20, cell_m=1, nx=2, ny=2, heights=[20, 21, 22, 23])
        mesh = scene_vectors.ground_mesh(grid, scene_vectors.into_sensor(yaw_pose(0)))
        self.assertEqual(
            mesh["positions"],
            [[0.0, 0.0, -3.0], [1.0, 0.0, -2.0], [1.0, 1.0, 0.0], [0.0, 1.0, -1.0]],
        )
        self.assertEqual(mesh["indices"], [0, 1, 2, 0, 2, 3])

    def test_holes_and_radius_are_not_bridged(self):
        grid = dict(x0=0, y0=0, cell_m=1, nx=2, ny=2, heights=[0, None, 0, 0])
        mesh = scene_vectors.ground_mesh(grid, lambda p: p)
        self.assertEqual(len(mesh["indices"]), 3)
        grid["heights"] = [0, None, None, 0]
        self.assertEqual(scene_vectors.ground_mesh(grid, lambda p: p)["indices"], [])
        grid["heights"] = [0, 0, 0, 0]
        self.assertEqual(
            scene_vectors.ground_mesh(grid, lambda p: p, radius_m=0.1)["indices"], []
        )


# =============================================================================
# THE PLACEMENT SIDECAR
# =============================================================================

SCENES_DIR = os.path.join(
    os.path.dirname(os.path.dirname(HERE)), "public_html", "src", "scenes"
)

# The export privacy case for the sidecar (D-34, Gerardus ADR-0011): placement
# and provenance only. Every key a sidecar may hold, at any depth; a new key
# is a decision to review here, not something a refactor adds quietly.
PLACEMENT_KEYS = {
    # top level
    "version",
    "export",
    "site",
    "state",
    "purpose",
    "reason",
    "record",
    "frames",
    "matrix_layout",
    "sensor_to_site",
    "site_to_city",
    "sensor_to_city",
    "heading_deg",
    "up_sensor",
    "marker",
    "generated_by",
    # record and its check
    "record_id",
    "registration_state",
    "pose_source",
    "model_id",
    "bundle_id",
    "reviewed_at",
    "sigma_horizontal_m",
    "sigma_heading_deg",
    "check",
    "method",
    "samples",
    "at_pose",
    "moved",
    "passed",
    # frames
    "sensor",
    "city",
    "frame_id",
    "axes",
    "l16",
    "origin",
    "lat_deg",
    "lon_deg",
    "navd88_m",
    "horizontal_datum",
    "vertical_datum",
    "format",
    "navd88_zero_ellipsoidal_m",
    # transforms and marker
    "rotation_wxyz",
    "translation_m",
    "matrix",
    "source",
    "note",
    "city_enu_m",
}
# Words that would mean a track, a person or a place's address got in.
FORBIDDEN = ("track", "trail", "speed", "address", "label", "street", "name", "owner")


def placement_keys(value, where="", out=None):
    out = [] if out is None else out
    if isinstance(value, dict):
        for k, v in value.items():
            # The check's perturbation names are data, not schema.
            if where.endswith(".moved"):
                continue
            out.append((f"{where}.{k}", k))
            placement_keys(v, f"{where}.{k}", out)
    elif isinstance(value, list):
        for v in value:
            placement_keys(v, where, out)
    return out


def assert_placement_only(test, placement):
    for path, key in placement_keys(placement):
        test.assertIn(key, PLACEMENT_KEYS, f"placement carries {path}")
        for word in FORBIDDEN:
            test.assertNotIn(word, key.lower(), f"placement carries {path}")


def vectors_block(record_id="gms:01TEST", heading=90.0, passed=True):
    return {
        "record_id": record_id,
        "registration_state": "manually_placed",
        "pose_source": "icp",
        "model_id": "sha256:model",
        "bundle_id": "sha256:bundle",
        "reviewed_at": "2026-10-22T01:33:50Z",
        "heading_deg": heading,
        "sigma_horizontal_m": 0.01,
        "check": {
            "samples": 900,
            "at_pose": 0.05,
            "moved": {"+2m_e+0m_n+0deg": 0.01},
            "passed": passed,
        },
    }


ENTRY = {
    "id": "columbus-broadway",
    "title": "Columbus at Broadway",
    "position": {"lat": 37.7978, "lon": -122.4065, "source": "operator"},
    "cells": {"site": {"token": "808580f47", "level": 16}},
}


class PlacementTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.assets = self.tmp.name
        self.addCleanup(self.tmp.cleanup)

    def write_vectors(self, block):
        scene_vectors.write_collection(
            {"placement": block, "features": []}, self.assets
        )

    def build(self, scan_pose=None, entry=ENTRY, record_id="gms:01TEST"):
        pose = scan_pose or yaw_pose(0.0)
        scan = {"recordId": record_id, "pose": pose}
        with mock.patch.object(
            scene_vectors, "site_entry", return_value=entry
        ), mock.patch.object(
            scene_vectors, "accepted_scan", return_value=({}, scan, {})
        ) as asked:
            placement = scene_vectors.build_placement("columbus-broadway", self.assets)
        return placement, asked

    def test_a_checked_pose_is_placed_through_the_site_into_the_city(self):
        # yaw 0 puts sensor +Y at true north: heading 0.
        self.write_vectors(vectors_block(heading=0.0))
        placement, asked = self.build()
        self.assertEqual(asked.call_args.kwargs["record_id"], "gms:01TEST")
        self.assertEqual(placement["state"], "placed")
        self.assertEqual(
            placement["sensor_to_site"]["translation_m"], [10.0, -20.0, 23.0]
        )
        s2c = scene_vectors.city_frame.site_to_city("808580f47")
        want = scene_vectors.city_frame.apply(s2c, [10.0, -20.0, 23.0])
        got = [row[3] for row in placement["sensor_to_city"]["matrix"][:3]]
        for g, w in zip(got, want):
            self.assertAlmostEqual(g, w, places=4)
        self.assertEqual(placement["marker"]["city_enu_m"], [round(v, 3) for v in got])
        self.assertEqual(placement["record"]["check"]["passed"], True)
        assert_placement_only(self, placement)

    def test_the_matrices_compose(self):
        self.write_vectors(
            vectors_block(heading=round(scene_vectors.heading_deg(yaw_pose(37.0)), 2))
        )
        placement, _ = self.build(yaw_pose(37.0))
        a = placement["site_to_city"]["matrix"]
        b = placement["sensor_to_site"]["matrix"]
        c = placement["sensor_to_city"]["matrix"]
        product = scene_vectors.city_frame.compose(a, b)
        for i in range(4):
            for j in range(4):
                self.assertAlmostEqual(product[i][j], c[i][j], places=3)

    def test_a_heading_that_disagrees_with_the_vectors_is_refused(self):
        self.write_vectors(vectors_block(heading=45.0))
        with self.assertRaises(RuntimeError):
            self.build()

    def test_no_vectors_is_not_placed_with_an_eye_read_marker(self):
        placement, asked = self.build()
        asked.assert_not_called()
        self.assertEqual(placement["state"], "not_placed")
        self.assertEqual(placement["marker"]["source"], "eye_read")
        self.assertNotIn("sensor_to_city", placement)
        assert_placement_only(self, placement)

    def test_a_failed_check_is_not_placed(self):
        self.write_vectors(vectors_block(passed=False))
        placement, asked = self.build()
        asked.assert_not_called()
        self.assertEqual(placement["state"], "not_placed")

    def test_an_unlocated_scene_has_no_marker(self):
        placement, _ = self.build(
            entry={"id": "soma1", "position": None, "cells": None}
        )
        self.assertEqual(placement["state"], "not_placed")
        self.assertIsNone(placement["marker"])

    def test_unreachable_gerardus_fails_only_when_required(self):
        self.write_vectors(vectors_block())
        error = scene_vectors.Unreachable("refused")
        with mock.patch.object(
            scene_vectors, "site_entry", return_value=ENTRY
        ), mock.patch.object(scene_vectors, "accepted_scan", side_effect=error):
            state = scene_vectors.export_scene_placement(
                "columbus-broadway", self.assets, "auto", say=lambda m: None
            )
            self.assertEqual(state, "not_placed")
            with self.assertRaises(scene_vectors.Unreachable):
                scene_vectors.export_scene_placement(
                    "columbus-broadway", self.assets, "required", say=lambda m: None
                )
        with open(os.path.join(self.assets, "placement.json")) as fh:
            self.assertNotIn("refused", fh.read())


class PublishedPlacementTest(unittest.TestCase):
    """Every committed sidecar: placement and provenance, and agreeing with its vectors."""

    def test_every_scene_has_a_sidecar_that_holds_only_placement(self):
        found = 0
        for site in sorted(os.listdir(SCENES_DIR)):
            assets = os.path.join(SCENES_DIR, site, "assets")
            if not os.path.exists(os.path.join(assets, "manifest.json")):
                continue
            found += 1
            path = os.path.join(assets, "placement.json")
            self.assertTrue(os.path.exists(path), f"{site} has no placement.json")
            with open(path) as fh:
                placement = json.load(fh)
            self.assertEqual(placement["site"], site)
            assert_placement_only(self, placement)
            vectors = scene_vectors.vectors_placement(assets)
            if placement["state"] == "placed":
                self.assertEqual(
                    placement["record"]["record_id"], vectors["record_id"], site
                )
                self.assertEqual(placement["record"]["check"], vectors["check"], site)
            else:
                self.assertIsNone(vectors, f"{site} has vectors but is not placed")
        self.assertGreater(found, 0)


if __name__ == "__main__":
    unittest.main()
