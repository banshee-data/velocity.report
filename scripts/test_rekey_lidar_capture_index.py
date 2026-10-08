"""Pin the re-key script's mirrors of the Go capture index to the Go code.

The script rewrites every primary and foreign key in the capture index from its
own copies of the Go identifier hashes, content tag and session sequencing. The
vectors below were printed by the Go code itself (internal/lidar/storage/sqlite
RootID, captureFileID, sessionID, periodID; internal/lidar/capindex ContentTag
and Sessions on main; Sessions on claude/web-ui-coherence-fc5dd5 for the
per-directory derivation), so a change on either side that the other does not
follow fails here rather than mis-keying a database.
"""

import importlib.util
import sqlite3
import tempfile
import unittest
from pathlib import Path

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location(
    "rekey", HERE / "rekey-lidar-capture-index.py"
)
rekey = importlib.util.module_from_spec(spec)
spec.loader.exec_module(rekey)

MIGRATIONS = HERE.parent / "internal" / "db" / "migrations"

ROOT = "/Volumes/lidar/lidar"
ROOT_ID = "root-23c5e1360f5b8eb2"
NEW_ROOT = "/Volumes/lidar/lidar/pcaps"
NEW_ROOT_ID = "root-eecea5df0195e06f"
SESSION = "ses-f3020dcd9afb017997bb"

T0 = 1788387769667000000
S = 1_000_000_000
MS = 1_000_000


def probed(rel, first, last, size):
    return dict(rel=rel, first=first, last=last, size=size)


def summary(sessions):
    return [
        (
            s["files"],
            s["start"],
            s["end"],
            s["covered"],
            s["lost"],
            s["worst"],
            s["size"],
        )
        for s in sessions
    ]


class IdentifierParityTest(unittest.TestCase):
    def test_root_ids_match_go(self):
        self.assertEqual(rekey.root_id(ROOT), ROOT_ID)
        self.assertEqual(rekey.root_id(NEW_ROOT), NEW_ROOT_ID)

    def test_file_ids_match_go(self):
        self.assertEqual(
            rekey.file_id(ROOT_ID, "s2/s2_sf_4_20260902152249_00001.pcap"),
            "cap-19a606469dc4aacba3978708",
        )
        self.assertEqual(
            rekey.file_id(NEW_ROOT_ID, "kirk/kirk0.pcapng"),
            "cap-4ed549dd014a2fa30ed601b7",
        )

    def test_session_ids_match_go(self):
        self.assertEqual(
            rekey.session_id(ROOT_ID, "s2/s2_sf_4_20260902152249_00001.pcap"), SESSION
        )
        self.assertEqual(
            rekey.session_id(NEW_ROOT_ID, "kirk/kirk0.pcapng"),
            "ses-d30b88cff18cd688dc47",
        )

    def test_period_ids_match_go(self):
        for ordinal, want in [
            (0, "per-4e7a6c70215921eb5d0b"),
            (3, "per-115fa2234ae8583f2a6f"),
            (12, "per-f097ccd9ed4c72f6b1cb"),
        ]:
            self.assertEqual(rekey.period_id(SESSION, ordinal), want)


class ContentTagParityTest(unittest.TestCase):
    # Sizes cover empty, head only, exactly one chunk, head with an uncovered
    # middle, exactly two chunks (still no tail), and head plus tail.
    GO_TAGS = {
        0: "9a271f2a916b0b6ee6cecb2426f0b3206ef074578be55d9bc94f6f3fe3ab86aa",
        1000: "d7a82e121b89479e1c449d8b48a87f37a8cb736bec34f79bbf15fd7c61abfeff",
        1 << 20: "46a23ee5b9fbe263d9038df2e2a094d6d7a52524d24a13056e39d00d85208e26",
        1572864: "71b6a984d5e107368b5048f0ef979bdf650b6ec53d0c430924a1eedf7ddcf19c",
        2 << 20: "c567c70df0159e447886b12528c229a144352d0a103903250d9d434ed216b7b4",
        (3 << 20)
        + 5: "1cbe23eec337728a974cee1f37323c6da4b42aa9eaebeecd9adabfd892b675eb",
    }

    def test_content_tags_match_go(self):
        with tempfile.TemporaryDirectory() as tmp:
            for size, want in self.GO_TAGS.items():
                path = Path(tmp) / str(size)
                path.write_bytes(bytes((i * 7 + 3) & 0xFF for i in range(size)))
                self.assertEqual(rekey.content_tag(path, size), want, f"size {size}")


class SequencingParityTest(unittest.TestCase):
    def test_grade_gap_boundaries(self):
        cases = [
            (-10 * MS - 1, "overlap"),
            (-10 * MS, "seamless"),
            (0, "seamless"),
            (10 * MS, "seamless"),
            (10 * MS + 1, "acceptable"),
            (S, "acceptable"),
            (S + 1, "broken"),
        ]
        for gap, want in cases:
            self.assertEqual(rekey.grade_gap(gap), want, f"gap {gap}")

    def test_whole_root_sessions_match_go(self):
        files = [
            probed("a/1.pcap", T0, T0 + 300 * S, 100),
            probed("a/2.pcap", T0 + 300 * S + 5 * MS, T0 + 600 * S, 200),  # seamless
            probed(
                "a/3.pcap", T0 + 600 * S + 500 * MS, T0 + 900 * S, 300
            ),  # acceptable
            probed("a/4.pcap", T0 + 902 * S, T0 + 1000 * S, 400),  # broken: new run
            probed(
                "a/5.pcap", T0 + 1000 * S - 5 * MS, T0 + 1100 * S, 500
            ),  # tolerated overlap
            probed("a/6.pcap", T0 + 1050 * S, T0 + 1200 * S, 600),  # overlap: new run
            probed(
                "b/x.pcap", T0 + 5000 * S, T0 + 4000 * S, 700
            ),  # ends before it starts: unused
        ]
        self.assertEqual(
            summary(rekey.derive_sessions(files, per_dir=False)),
            [
                (
                    ["a/1.pcap", "a/2.pcap", "a/3.pcap"],
                    1788387769667000000,
                    1788388669667000000,
                    899495000000,
                    505000000,
                    "acceptable",
                    600,
                ),
                (
                    ["a/4.pcap", "a/5.pcap"],
                    1788388671667000000,
                    1788388869667000000,
                    198005000000,
                    0,
                    "seamless",
                    900,
                ),
                (
                    ["a/6.pcap"],
                    1788388819667000000,
                    1788388969667000000,
                    150000000000,
                    0,
                    "seamless",
                    600,
                ),
            ],
        )

    def test_per_directory_sessions_match_go(self):
        files = [
            probed("s2/c1.pcap", T0, T0 + 300 * S, 10),
            probed("s2/c2.pcap", T0 + 300 * S + 1 * MS, T0 + 600 * S, 20),
            probed("copy/whole.pcapng", T0, T0 + 600 * S, 30),
            probed("top.pcapng", T0 + 100 * S, T0 + 200 * S, 40),
        ]
        got = sorted(summary(rekey.derive_sessions(files, per_dir=True)))
        self.assertEqual(
            got,
            sorted(
                [
                    (
                        ["copy/whole.pcapng"],
                        1788387769667000000,
                        1788388369667000000,
                        600000000000,
                        0,
                        "seamless",
                        30,
                    ),
                    (
                        ["s2/c1.pcap", "s2/c2.pcap"],
                        1788387769667000000,
                        1788388369667000000,
                        599999000000,
                        1000000,
                        "seamless",
                        30,
                    ),
                    (
                        ["top.pcapng"],
                        1788387869667000000,
                        1788387969667000000,
                        100000000000,
                        0,
                        "seamless",
                        40,
                    ),
                ]
            ),
        )
        # Sequenced together, the copy overlaps the chunks and splits them.
        whole = [s["files"] for s in rekey.derive_sessions(files, per_dir=False)]
        self.assertNotIn(["s2/c1.pcap", "s2/c2.pcap"], whole)


class RelocateTest(unittest.TestCase):
    OPS = [
        ("mv", "s2", "pcaps/s2"),
        ("mv", "pcaps/s2/dup", "work/dup"),
        ("rm-symlink", "pcaps/s2/linked.pcapng", ""),
        ("mv", "linked.pcapng", "pcaps/s2/linked.pcapng"),
        ("mv", "kirk0.pcapng", "pcaps/kirk/kirk0.pcapng"),
    ]

    def test_paths_follow_moves_in_order(self):
        self.assertEqual(rekey.relocate("s2/a.pcap", self.OPS), "pcaps/s2/a.pcap")
        self.assertEqual(
            rekey.relocate("s2/dup/x.pcapng", self.OPS), "work/dup/x.pcapng"
        )
        self.assertEqual(
            rekey.relocate("kirk0.pcapng", self.OPS), "pcaps/kirk/kirk0.pcapng"
        )
        self.assertEqual(
            rekey.relocate("manifests/m.json", self.OPS), "manifests/m.json"
        )
        self.assertEqual(rekey.relocate("s2x/a.pcap", self.OPS), "s2x/a.pcap")

    def test_removed_symlink_drops_its_row_but_keeps_its_path(self):
        self.assertIsNone(rekey.relocate("s2/linked.pcapng", self.OPS))
        self.assertEqual(
            rekey.relocate("s2/linked.pcapng", self.OPS, by_path=True),
            "pcaps/s2/linked.pcapng",
        )
        self.assertEqual(
            rekey.relocate("linked.pcapng", self.OPS), "pcaps/s2/linked.pcapng"
        )


class SelfCheckTest(unittest.TestCase):
    """The checks that guard --apply, against the real capture-index schema."""

    def setUp(self):
        self.con = sqlite3.connect(":memory:")
        for name in (
            "000042_create_lidar_capture_index",
            "000043_create_lidar_capture_jobs",
        ):
            self.con.executescript((MIGRATIONS / f"{name}.up.sql").read_text())
        self.con.execute(
            "INSERT INTO lidar_capture_roots (root_id, path, created_at_ns, updated_at_ns) "
            "VALUES (?, ?, 1, 1)",
            (ROOT_ID, ROOT),
        )
        files = [
            ("s2/s2_sf_4_20260902152249_00001.pcap", T0, T0 + 300 * S, 10),
            (
                "s2/s2_sf_4_20260902152749_00002.pcap",
                T0 + 300 * S + 1 * MS,
                T0 + 600 * S,
                20,
            ),
            ("sf/site.pcapng", T0, T0 + 600 * S, 30),
        ]
        for rel, first, last, size in files:
            self.con.execute(
                """INSERT INTO lidar_capture_files (capture_file_id, root_id, rel_path, size_bytes,
                     modified_at_ns, first_packet_ns, last_packet_ns, probe_state,
                     first_seen_at_ns, last_seen_at_ns) VALUES (?, ?, ?, ?, 1, ?, ?, 'ok', 1, 1)""",
                (rekey.file_id(ROOT_ID, rel), ROOT_ID, rel, size, first, last),
            )
        for s in rekey.derive_sessions([probed(*f) for f in files], per_dir=True):
            sid = rekey.session_id(ROOT_ID, s["files"][0])
            self.con.execute(
                """INSERT INTO lidar_capture_sessions (session_id, root_id, file_count, start_ns,
                     end_ns, covered_ns, lost_ns, worst_seam, size_bytes, derived_at_ns)
                   VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1)""",
                (
                    sid,
                    ROOT_ID,
                    len(s["files"]),
                    s["start"],
                    s["end"],
                    s["covered"],
                    s["lost"],
                    s["worst"],
                    s["size"],
                ),
            )
            for rel in s["files"]:
                self.con.execute(
                    "UPDATE lidar_capture_files SET session_id = ? WHERE rel_path = ?",
                    (sid, rel),
                )
        self.con.execute(
            """INSERT INTO lidar_capture_motion_periods (period_id, session_id, ordinal, period_type,
                 start_ns, end_ns, duration_ns, start_secs, end_secs, created_at_ns)
               VALUES (?, ?, 0, 'static', ?, ?, ?, 0, 1, 1)""",
            (rekey.period_id(SESSION, 0), SESSION, T0, T0 + S, S),
        )
        self.cur = self.con.cursor()

    def tearDown(self):
        self.con.close()

    def test_consistent_index_passes(self):
        rekey.check_identities(self.cur, ROOT, ROOT_ID)
        per_dir, count = rekey.check_derivation(self.cur, ROOT_ID)
        self.assertEqual((per_dir, count), (True, 2))

    def test_whole_root_index_is_detected(self):
        self.con.execute("DELETE FROM lidar_capture_sessions")
        self.con.execute("UPDATE lidar_capture_files SET session_id = NULL")
        rows = self.con.execute(
            "SELECT rel_path, first_packet_ns, last_packet_ns, size_bytes "
            "FROM lidar_capture_files"
        ).fetchall()
        for s in rekey.derive_sessions([probed(*r) for r in rows], per_dir=False):
            sid = rekey.session_id(ROOT_ID, s["files"][0])
            self.con.execute(
                """INSERT INTO lidar_capture_sessions (session_id, root_id, file_count, start_ns,
                     end_ns, covered_ns, lost_ns, worst_seam, size_bytes, derived_at_ns)
                   VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1)""",
                (
                    sid,
                    ROOT_ID,
                    len(s["files"]),
                    s["start"],
                    s["end"],
                    s["covered"],
                    s["lost"],
                    s["worst"],
                    s["size"],
                ),
            )
            for rel in s["files"]:
                self.con.execute(
                    "UPDATE lidar_capture_files SET session_id = ? WHERE rel_path = ?",
                    (sid, rel),
                )
        self.assertEqual(rekey.check_derivation(self.cur, ROOT_ID)[0], False)

    def test_mismatched_file_id_stops(self):
        self.con.execute(
            "UPDATE lidar_capture_files SET capture_file_id = 'cap-0' "
            "WHERE rel_path = 'sf/site.pcapng'"
        )
        with self.assertRaises(rekey.Abort):
            rekey.check_identities(self.cur, ROOT, ROOT_ID)

    def test_mismatched_root_path_stops(self):
        with self.assertRaises(rekey.Abort):
            rekey.check_identities(self.cur, ROOT + "/", ROOT_ID)

    def test_session_that_no_derivation_reproduces_stops(self):
        self.con.execute(
            "UPDATE lidar_capture_sessions SET size_bytes = size_bytes + 1"
        )
        with self.assertRaises(rekey.Abort):
            rekey.check_derivation(self.cur, ROOT_ID)


if __name__ == "__main__":
    unittest.main()
