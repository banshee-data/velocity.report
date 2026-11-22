"""Inventory every stored trail anchor without imposing a radius or support filter."""

import gzip
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

MODULE_PATH = Path(__file__).with_name("scene-trail-inventory.py")
SPEC = importlib.util.spec_from_file_location("scene_trail_inventory", MODULE_PATH)
inventory = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(inventory)


def make_scene(root, parts):
    assets = root / "test-site" / "assets"
    assets.mkdir(parents=True)
    (assets / "manifest.json").write_text(
        json.dumps({"parts": [{"url": f"./part-{i:03d}/"} for i in range(len(parts))]})
    )
    for i, frames in enumerate(parts):
        part = assets / f"part-{i:03d}"
        (part / "frames").mkdir(parents=True)
        (part / "header.json").write_text(json.dumps({"frame_count": len(frames)}))
        (part / "index.json").write_text(
            json.dumps({"chunks": [{"c": 0, "n": len(frames)}]})
        )
        with gzip.open(part / "frames" / "chunk_0000.ndjson.gz", "wt") as stream:
            for frame in frames:
                stream.write(json.dumps(frame) + "\n")
    return assets


class SceneTrailInventoryTests(unittest.TestCase):
    def test_keeps_all_classes_support_states_and_distances(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            make_scene(
                root,
                [
                    [
                        {
                            "tr": [
                                {
                                    "id": "a",
                                    "x": 0,
                                    "y": 0,
                                    "bx": 3,
                                    "by": 4,
                                    "support": "observed",
                                },
                                {
                                    "id": "b",
                                    "x": 190,
                                    "y": 0,
                                    "c": "bird",
                                    "support": "coasted",
                                },
                            ]
                        },
                        {
                            "tr": [
                                {"id": "a", "x": 230, "y": 0},
                                {"id": "b", "x": 100, "y": 0, "bx": 999},
                            ]
                        },
                    ]
                ],
            )
            [row], count, digest = inventory.inventory(root)
            self.assertEqual(
                (row["frames"], row["samples"], row["track_ids"]), (2, 4, 2)
            )
            self.assertEqual(row["max_range_m"], 230)
            self.assertEqual(row["tracks_beyond"][100], 2)
            self.assertEqual(row["samples_beyond"][100], 2)
            self.assertEqual(dict(row["anchor"]), {"box_center": 1, "position": 3})
            self.assertEqual(
                dict(row["support"]), {"observed": 1, "coasted": 1, "unknown": 2}
            )
            self.assertEqual(count, 4)
            self.assertEqual(inventory.inventory(root)[2], digest)

    def test_ids_are_scoped_to_the_part(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            track = {"id": "a", "x": 101, "y": 0}
            make_scene(root, [[{"tr": [track]}], [{"tr": [track]}]])
            [row], _, _ = inventory.inventory(root)
            self.assertEqual(row["track_ids"], 2)
            self.assertEqual(row["tracks_beyond"][100], 2)

    def test_rejects_missing_data_instead_of_silently_trimming(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            assets = make_scene(root, [[{"tr": [{"id": "a", "x": 1, "y": 2}]}]])
            index = assets / "part-000" / "index.json"
            index.write_text(json.dumps({"chunks": [{"c": 0, "n": 2}]}))
            with self.assertRaisesRegex(ValueError, "count differs"):
                inventory.inventory(root)
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            make_scene(root, [[{"tr": [{"id": "a", "x": None, "y": 2}]}]])
            with self.assertRaisesRegex(ValueError, "no finite XY"):
                inventory.inventory(root)
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            make_scene(root, [[{"tr": "invalid"}]])
            with self.assertRaisesRegex(ValueError, "invalid track list"):
                inventory.inventory(root)

    def test_rejects_a_repeated_id_in_the_same_frame(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            track = {"id": "a", "x": 1, "y": 2}
            make_scene(root, [[{"tr": [track, track]}]])
            with self.assertRaisesRegex(ValueError, "repeated track ID"):
                inventory.inventory(root)


if __name__ == "__main__":
    unittest.main()
