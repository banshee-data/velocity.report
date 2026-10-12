"""Tests for podcast-transcribe.py: link resolution, pass loading, alignment and the check.

Network calls are replaced with canned pages and feeds; no ASR engine is run.
"""

import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock

spec = importlib.util.spec_from_file_location(
    "podcast_transcribe", Path(__file__).with_name("podcast-transcribe.py")
)
pt = importlib.util.module_from_spec(spec)
spec.loader.exec_module(pt)

ENCLOSURE = "https://traffic.example.com/show/ep42.mp3"

FEED = f"""<?xml version="1.0"?>
<rss xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd"
     xmlns:podcast="https://podcastindex.org/namespace/1.0">
<channel>
  <title>The Brake: A Streetsblog Podcast</title>
  <itunes:author>StreetsblogUSA</itunes:author>
  <description>Interviews.</description>
  <podcast:person role="host">Kea Wilson</podcast:person>
  <item>
    <title>The Real Reasons Drivers Speed (Rebecca Steinbach)</title>
    <guid isPermaLink="false">guid-42</guid>
    <pubDate>Tue, 06 Oct 2026 05:01:00 +0000</pubDate>
    <enclosure url="{ENCLOSURE}?dest-id=1" length="1" type="audio/mpeg"/>
    <itunes:duration>25:48</itunes:duration>
    <description>&lt;p&gt;A &lt;a href="https://aaa.example/r"&gt;study&lt;/a&gt; with
      Rebecca Steinbach.&lt;/p&gt;</description>
    <podcast:transcript url="https://example.com/t.vtt" type="text/vtt"/>
  </item>
  <item>
    <title>The Real Reasons Drivers Speed (Rebecca Steinbach)</title>
    <guid>guid-rerun</guid>
    <pubDate>Tue, 06 Oct 2020 05:01:00 +0000</pubDate>
    <enclosure url="https://traffic.example.com/show/rerun.mp3" type="audio/mpeg"/>
  </item>
</channel>
</rss>""".encode()

CASTRO = f"""<html><head>
<meta property="og:title" content="The Brake: A Streetsblog Podcast: The Real Reasons Drivers Speed (Rebecca Steinbach) (25m)" />
</head><body>
<a href="https://castro.fm/share/podcast/ad91" aria-label="The Brake: A Streetsblog Podcast page"></a>
<audio id="co-supertop-castro-episode" preload="true"><source src="{ENCLOSURE}?dest-id=9"></audio>
</body></html>"""

SPOTIFY = """<html><head>
<meta property="og:title" content="The Real Reasons Drivers Speed (Rebecca Steinbach)"/>
<meta property="og:description" content="The Brake: A Streetsblog Podcast · Episode"/>
<meta name="music:release_date" content="2026-10-06T05:01:00Z"/>
<meta name="music:duration" content="1547"/>
</head></html>"""


def fake_network(pages):
    def fetch(url, timeout=60):
        for prefix, body in pages.items():
            if url.startswith(prefix):
                return body if isinstance(body, bytes) else body.encode()
        raise AssertionError(f"unexpected fetch {url}")

    return fetch


def words(text, start=0.0, step=0.5, p=0.9):
    return [
        {"t": t, "s": start + i * step, "e": start + i * step + step, "p": p, "seg": 0}
        for i, t in enumerate(text.split())
    ]


class ResolveTests(unittest.TestCase):
    def test_apple_link_without_episode_id_is_refused(self):
        with self.assertRaises(SystemExit):
            pt.resolve_apple(
                "https://podcasts.apple.com/us/podcast/the-brake/id1610029625"
            )

    def test_castro_page_gives_show_title_and_audio(self):
        with mock.patch.object(
            pt, "fetch", fake_network({"https://castro.fm": CASTRO})
        ):
            ep = pt.resolve_castro("https://castro.fm/episode/esLjgi")
        self.assertEqual(ep["show"], "The Brake: A Streetsblog Podcast")
        self.assertEqual(
            ep["title"], "The Real Reasons Drivers Speed (Rebecca Steinbach)"
        )
        self.assertEqual(ep["enclosure_url"], ENCLOSURE + "?dest-id=9")

    def test_spotify_page_gives_show_title_and_date(self):
        with mock.patch.object(
            pt, "fetch", fake_network({"https://open.spotify": SPOTIFY})
        ):
            ep = pt.resolve_spotify("https://open.spotify.com/episode/6Cch")
        self.assertEqual(ep["show"], "The Brake: A Streetsblog Podcast")
        self.assertEqual(ep["released"], "2026-10-06")
        self.assertEqual(ep["duration_s"], 1547.0)

    def test_spotify_resolves_through_the_public_feed(self):
        search = {
            "results": [
                {
                    "collectionName": "The Brake: A Streetsblog Podcast",
                    "feedUrl": "https://feed.example/brake.xml",
                }
            ]
        }
        network = fake_network(
            {"https://open.spotify": SPOTIFY, "https://feed.example": FEED}
        )
        with mock.patch.object(pt, "fetch", network), mock.patch.object(
            pt, "fetch_json", return_value=search
        ):
            ep = pt.resolve("https://open.spotify.com/episode/6Cch")
        self.assertEqual(ep["guid"], "guid-42")  # the 2026 item, not the 2020 rerun
        self.assertEqual(ep["enclosure_url"], ENCLOSURE + "?dest-id=1")
        self.assertEqual(ep["show_persons"], [{"name": "Kea Wilson", "role": "host"}])
        self.assertEqual(ep["publisher_transcripts"][0]["type"], "text/vtt")

    def test_apple_resolves_by_lookup_then_feed(self):
        lookup = {
            "results": [
                {
                    "kind": "podcast",
                    "collectionName": "The Brake: A Streetsblog Podcast",
                    "feedUrl": "https://feed.example/brake.xml",
                },
                {
                    "kind": "podcast-episode",
                    "trackId": 1000793374806,
                    "trackName": "The Real Reasons",
                    "episodeGuid": "guid-42",
                    "releaseDate": "2026-10-06T05:01:00Z",
                    "episodeUrl": ENCLOSURE,
                },
            ]
        }
        with mock.patch.object(
            pt, "fetch", fake_network({"https://feed.example": FEED})
        ), mock.patch.object(pt, "fetch_json", return_value=lookup):
            ep = pt.resolve(
                "https://podcasts.apple.com/us/podcast/x/id1610029625?i=1000793374806"
            )
        self.assertEqual(ep["platform"], "apple")
        self.assertEqual(ep["duration_s"], 25 * 60 + 48)
        self.assertIn("Rebecca Steinbach", pt.html_to_text(ep["notes_html"]))
        self.assertEqual(
            pt.episode_key(ep),
            "2026-10-06-the-real-reasons-drivers-speed-rebecca-steinbach",
        )

    def test_match_item_prefers_guid_then_audio_url_then_nearest_title(self):
        _, items = pt.parse_feed(FEED)
        self.assertEqual(
            pt.match_item({"guid": "guid-rerun"}, items)["guid"], "guid-rerun"
        )
        by_url = pt.match_item({"enclosure_url": ENCLOSURE + "?other=2"}, items)
        self.assertEqual(by_url["guid"], "guid-42")
        by_title = pt.match_item(
            {
                "title": "the real reasons drivers speed (rebecca steinbach)",
                "released": "2020-10-07",
            },
            items,
        )
        self.assertEqual(by_title["guid"], "guid-rerun")
        self.assertIsNone(pt.match_item({"title": "Another episode"}, items))

    def test_html_to_text_keeps_paragraphs_and_link_targets(self):
        text = pt.html_to_text(
            '<p>One <a href="https://a.example/x">link</a>.</p><p>Two</p>'
        )
        self.assertEqual(text, "One link (https://a.example/x).\n\nTwo")


class PassTests(unittest.TestCase):
    def test_pieces_without_a_space_continue_the_word_and_compounds_split(self):
        got = pt.words_from_pieces(
            [
                (" the", 0.0, 0.2, 0.9, 0),
                (" follow", 0.2, 0.5, 0.8, 0),
                ("-up", 0.5, 0.7, 0.6, 0),
                (" report", 0.7, 1.0, 0.9, 0),
                (".", 1.0, 1.0, 0.9, 0),
                (" Next", 1.1, 1.3, 0.9, 1),
            ]
        )
        self.assertEqual(
            [w["t"] for w in got], ["the", "follow-", "up", "report.", "Next"]
        )
        self.assertEqual(
            got[1]["p"], 0.6
        )  # the merged word keeps its least confident piece
        self.assertEqual(
            pt.join_words([w["t"] for w in got]), "the follow-up report. Next"
        )

    def test_engine_loaders_read_native_json(self):
        with tempfile.TemporaryDirectory() as tmp:
            whisper = Path(tmp) / "w.json"
            whisper.write_text(
                json.dumps(
                    {
                        "segments": [
                            {
                                "start": 0,
                                "end": 1,
                                "text": " Hi there.",
                                "words": [
                                    {
                                        "word": " Hi",
                                        "start": 0,
                                        "end": 0.4,
                                        "probability": 0.9,
                                    },
                                    {
                                        "word": " there.",
                                        "start": 0.4,
                                        "end": 1,
                                        "probability": 0.8,
                                    },
                                ],
                            }
                        ]
                    }
                )
            )
            parakeet = Path(tmp) / "p.json"
            parakeet.write_text(
                json.dumps(
                    {
                        "sentences": [
                            {
                                "tokens": [
                                    {
                                        "text": " Hi",
                                        "start": 0,
                                        "end": 0.3,
                                        "confidence": 1.0,
                                    },
                                    {
                                        "text": " the",
                                        "start": 0.4,
                                        "end": 0.6,
                                        "confidence": 0.9,
                                    },
                                    {
                                        "text": "re",
                                        "start": 0.6,
                                        "end": 0.8,
                                        "confidence": 0.7,
                                    },
                                    {
                                        "text": ".",
                                        "start": 0.8,
                                        "end": 0.8,
                                        "confidence": 1.0,
                                    },
                                ]
                            }
                        ]
                    }
                )
            )
            a = pt.ENGINES["openai-whisper"].load(whisper)
            b = pt.ENGINES["parakeet-mlx"].load(parakeet)
        self.assertEqual([w["t"] for w in a], ["Hi", "there."])
        self.assertEqual([w["t"] for w in b], ["Hi", "there."])
        self.assertEqual(b[1]["p"], 0.7)

    def test_pass_spec_must_name_a_known_engine_and_a_model(self):
        self.assertEqual(pt.parse_pass("parakeet-mlx:org/model")[1], "org/model")
        for bad in ("whisper.cpp:large", "openai-whisper", "openai-whisper:"):
            with self.assertRaises(SystemExit):
                pt.parse_pass(bad)


class AlignmentTests(unittest.TestCase):
    def test_alignment_ignores_case_and_punctuation_and_counts_divergence(self):
        a = [pt.norm(w) for w in "So, how do these folks differ?".split()]
        b = [pt.norm(w) for w in "so how did these folks differ".split()]
        ops = pt.align(a, b)
        self.assertEqual([op[0] for op in ops], ["same", "diff", "same"])
        stats = pt.pair_stats(ops, len(a), len(b))
        self.assertEqual(stats["divergent_spans"], 1)
        self.assertAlmostEqual(stats["wer_b_vs_a"], 1 / 6, places=4)

    def test_chunks_close_at_a_sentence_end_and_never_inside_a_divergence(self):
        a = words("One two three. Four five six. Seven eight.")
        b = words("One two three. Four fine six. Seven eight.")
        ops = pt.align([pt.norm(w["t"]) for w in a], [pt.norm(w["t"]) for w in b])
        chunks = pt.chunk_items(pt.build_items(a, b, ops), target=3)
        self.assertEqual(len(chunks), 3)
        self.assertEqual(
            pt.join_words([it["a"][0]["t"] for it in chunks[0]]), "One two three."
        )
        self.assertEqual([it["same"] for it in chunks[1]], [True, False, True])
        self.assertTrue(all(it["same"] for it in chunks[-1]))
        rendered = pt.render_chunk(2, chunks[1], {})
        self.assertIn("⟦A: five (0.90) | B: fine (0.90)⟧", rendered)

    def test_spelling_hint_suggests_the_notes_spelling_only_for_near_misses(self):
        vocab = pt.notes_vocabulary({"title": "Real Reasons (Rebecca Steinbach)"})
        self.assertEqual(pt.spelling_hint("Steinbeck,", vocab), "Steinbach")
        self.assertIsNone(pt.spelling_hint("Really", vocab))  # prefix: an inflection
        self.assertIsNone(
            pt.spelling_hint("steinbeck", vocab)
        )  # not a proper noun as heard
        self.assertIsNone(pt.spelling_hint("Steinbach", vocab))


RECONCILED_HEAD = """speakers:
- Kea Wilson: host
- Rebecca Steinbach: guest
notes: A note.
---
"""


class CheckTests(unittest.TestCase):
    a = words(
        "Hey guys it's Kia so here is the thing about speed limits um that matter a lot"
    )
    b = words(
        "Hey guys it's Kia so here is the thing about speed limits that matter a lot"
    )

    def check(self, body, duration=60.0):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "reconciled.md"
            path.write_text(RECONCILED_HEAD + body)
            rec = pt.parse_reconciled(path)
        return rec, pt.check_reconciled(rec, self.a, self.b, duration)

    def test_faithful_transcript_with_a_spelling_fix_passes(self):
        rec, result = self.check(
            "[00:00:00] Kea Wilson: Hey guys, it's Kea. So here is the thing: about speed limits\n"
            "that matter [crosstalk] a lot.\n"
        )
        self.assertTrue(result["ok"], result["problems"])
        self.assertEqual(rec["paragraphs"][0]["speaker"], "Kea Wilson")
        self.assertEqual(rec["notes"], "A note.")
        self.assertEqual(
            result["editorial_words"], 1
        )  # "Kea"; the bracket is not counted
        self.assertEqual(result["omitted_agreed_words"], 1)  # "Kia"; "um" is a filler

    def test_invented_words_fail(self):
        _, result = self.check(
            "[00:00:00] Kea Wilson: Hey guys, it's Kia. So here is the thing about speed limits\n"
            "that really truly deeply madly matter a lot.\n"
        )
        self.assertFalse(result["ok"])
        self.assertIn("really truly deeply madly", result["problems"][0])

    def test_dropped_words_fail(self):
        _, result = self.check(
            "[00:00:00] Kea Wilson: Hey guys, it's Kia. Speed limits that matter a lot.\n"
        )
        self.assertFalse(result["ok"])
        self.assertIn("so here is the thing about", result["problems"][0])

    def test_structure_problems_are_reported(self):
        _, result = self.check(
            "[00:00:30] Hey guys it's Kia so here is the thing about\n\n"
            "[00:00:10] Rebecca Steinbach: speed limits that matter a lot\n",
            duration=20.0,
        )
        self.assertIn("the first paragraph names no speaker", result["problems"])
        self.assertIn("paragraph timestamps go backwards", result["problems"])
        self.assertTrue(any("past the end" in p for p in result["problems"]))

    def test_a_colon_in_speech_is_not_taken_for_a_speaker(self):
        rec, _ = self.check(
            "[00:00:00] Kea Wilson: Here's the thing: it matters.\n\n"
            "[00:00:05] Subtext: there are rewards.\n"
        )
        self.assertIsNone(rec["paragraphs"][1]["speaker"])
        self.assertEqual(rec["paragraphs"][1]["text"], "Subtext: there are rewards.")

    def test_missing_separator_and_timestamp_are_refused(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "reconciled.md"
            path.write_text("speakers:\n- A: host\n[00:00:00] A: hi\n")
            with self.assertRaises(SystemExit):
                pt.parse_reconciled(path)
            path.write_text(RECONCILED_HEAD + "No timestamp here.\n")
            with self.assertRaises(SystemExit):
                pt.parse_reconciled(path)


class RenderTests(unittest.TestCase):
    def test_wrapped_lines_never_start_a_list_or_quote(self):
        text = "word " * 18 + "1. Then - a dash and > a quote " + "word " * 30
        for line in pt.wrap_md(text, width=40)[1:]:
            self.assertNotRegex(line, r"^(\d+[.)]\s|[-+*>#]\s)")

    def test_links_put_awkward_urls_in_angle_brackets(self):
        self.assertEqual(
            pt.md_link("feed", "https://a.org/x.xml"), "[feed](https://a.org/x.xml)"
        )
        self.assertEqual(
            pt.md_link("ep", "https://a.org/x_(1)"), "[ep](<https://a.org/x_(1)>)"
        )
        self.assertEqual(pt.md_table([("A", "x | y")])[2], "| A | x \\| y |")
        self.assertEqual(pt._site("https://c6996bab-7d65.libsyn.com/ep"), "libsyn.com")

    def test_markdown_is_escaped_outside_urls(self):
        self.assertEqual(
            pt.md_escape("a_b (https://x.org/a_b)."), "a\\_b (<https://x.org/a_b>)."
        )

    def test_rendered_transcript_has_metadata_header_and_provenance(self):
        run = {
            "engine": "openai-whisper",
            "version": "1",
            "model": "turbo",
            "wall_s": 10.0,
            "command": [
                "whisper",
                "a.wav",
                "--model",
                "turbo",
                "--initial_prompt",
                "The Show.",
            ],
        }
        state = {
            "key": "2026-10-06-x",
            "episode": {
                "title": "X_Y",
                "show": "The Show",
                "released": "2026-10-06",
                "source_url": "https://example.com/e",
                "enclosure_url": ENCLOSURE,
                "notes_html": "<p>Notes</p>",
                "audio": {
                    "sha256": "ab" * 32,
                    "duration_s": 61.0,
                    "codec": "mp3",
                    "bit_rate": 128000,
                },
            },
            "runs": [
                run,
                {**run, "engine": "parakeet-mlx", "command": ["parakeet-mlx", "a.wav"]},
            ],
            "stats": {"agreement": 0.98, "wer_b_vs_a": 0.02, "divergent_spans": 3},
        }
        rec = {
            "speakers": [("Kea Wilson", "host")],
            "notes": "",
            "paragraphs": [
                {"t": 1, "speaker": "Kea Wilson", "text": "Hello."},
                {"t": 30, "speaker": None, "text": "Again."},
            ],
        }
        result = {
            "words": 2,
            "paragraphs": 2,
            "supported_by_a": 1.0,
            "supported_by_b": 1.0,
            "editorial_words": 0,
            "omitted_agreed_words": 0,
        }
        md = pt.render_transcript(
            state, rec, result, {"commit": "abc123", "dirty": True}
        )
        self.assertTrue(
            md.startswith("# X\\_Y\n\n- **Source:** _The Show_, released 2026-10-06\n")
        )
        self.assertIn("**Kea Wilson** (00:00:01): Hello.", md)
        self.assertIn("\n(00:00:30) Again.\n", md)
        self.assertIn("`--initial_prompt 'The Show.'`", md)
        self.assertIn("parakeet-mlx 1, model `turbo`, default settings", md)
        self.assertIn("- **Length:** 1 min 01 s", md)
        self.assertIn("| Episode link | [Episode page](https://example.com/e) |", md)
        self.assertIn(
            f"| Audio enclosure | [mp3 on traffic.example.com]({ENCLOSURE}) |", md
        )
        self.assertIn("at `abc123` with uncommitted changes", md)


def make_work_dir(root):
    """A finished work directory: two passes, state, reconciled text and audio."""
    work = Path(root) / "work"
    text = "Hey guys it's Kia welcome back to the show"
    passes = {
        "openai-whisper-turbo": (
            "openai-whisper",
            {
                "segments": [
                    {
                        "start": 0,
                        "end": 5,
                        "words": [
                            {
                                "word": " " + t,
                                "start": i * 0.5,
                                "end": i * 0.5 + 0.5,
                                "probability": 0.9,
                            }
                            for i, t in enumerate(text.split())
                        ],
                    }
                ]
            },
        ),
        "parakeet-mlx-v2": (
            "parakeet-mlx",
            {
                "sentences": [
                    {
                        "tokens": [
                            {
                                "text": " " + t,
                                "start": i * 0.5,
                                "end": i * 0.5 + 0.5,
                                "confidence": 0.9,
                            }
                            for i, t in enumerate(text.split())
                        ]
                    }
                ]
            },
        ),
    }
    runs = []
    for run_id, (engine, native) in passes.items():
        (work / "passes" / run_id).mkdir(parents=True)
        (work / "passes" / run_id / "audio.json").write_text(json.dumps(native))
        runs.append(
            {
                "id": run_id,
                "engine": engine,
                "version": "1",
                "model": "m",
                "command": [engine, "audio.wav"],
                "wall_s": 1.0,
            }
        )
    episode = {
        "title": "Episode",
        "show": "The Show: A Podcast",
        "released": "2026-10-06",
        "source_url": "https://example.com/e",
        "enclosure_url": ENCLOSURE,
        "audio": {
            "sha256": "ab" * 32,
            "duration_s": 10.0,
            "codec": "mp3",
            "bit_rate": 128000,
        },
    }
    state = {
        "key": "2026-10-06-episode",
        "episode": episode,
        "runs": runs,
        "stats": {"agreement": 1.0, "wer_b_vs_a": 0.0, "divergent_spans": 0},
    }
    (work / "state.json").write_text(json.dumps(state))
    (work / "reconciled.md").write_text(
        RECONCILED_HEAD
        + "[00:00:00] Kea Wilson: Hey guys, it's Kea. Welcome back to the show.\n"
    )
    (work / "source.mp3").write_bytes(b"mp3")
    (work / "audio.wav").write_bytes(b"wav")
    return work


class PublishTests(unittest.TestCase):
    def test_publish_archives_the_transcript_and_its_evidence(self):
        with tempfile.TemporaryDirectory() as tmp:
            work = make_work_dir(tmp)
            archive = Path(tmp) / "archive"
            self.assertEqual(
                pt.main(["publish", str(work), "--out-dir", str(archive)]), 0
            )
            dest = archive / "the-show" / "2026-10-06-episode"
            self.assertIn(
                "**Kea Wilson** (00:00:00)", (dest / "transcript.md").read_text()
            )
            for name in ("source.mp3", "state.json", "reconciled.md", "check.json"):
                self.assertTrue((dest / name).exists(), name)
            self.assertTrue(
                (dest / "passes" / "parakeet-mlx-v2" / "audio.json").exists()
            )
            self.assertFalse((dest / "audio.wav").exists())

    def test_cite_repeats_every_transcript_field_and_adds_the_archive_identity(self):
        with tempfile.TemporaryDirectory() as tmp:
            work = make_work_dir(tmp)
            archive = Path(tmp) / "transcripts"
            pt.main(["publish", str(work), "--out-dir", str(archive)])
            dest = archive / "the-show" / "2026-10-06-episode"
            transcript = (dest / "transcript.md").read_text()
            published = json.loads((dest / "published.json").read_text())
            doc = Path(pt.REPO_ROOT) / "docs" / "plans" / "x.md"
            table = pt.cite_table(dest, doc)

        header = [
            line[4:] for line in transcript.splitlines() if line.startswith("- **")
        ]
        provenance = transcript.split("## Provenance\n\n", 1)[1].strip().splitlines()
        cited = {line.split(" | ", 1)[0]: line for line in table}
        for field in header:
            key, value = field.split(":** ", 1)
            self.assertEqual(cited["| " + key], f"| {key} | {value} |")
        for row in provenance[2:]:
            key = row.split(" | ", 1)[0]
            if key not in ("| Tool", "| Evidence"):
                self.assertEqual(cited[key], row)
        self.assertIn(published["tool"]["commit"] or "unrecorded", cited["| Tool"])
        self.assertIn(
            "[podcast-transcribe](../../tools/podcast-transcribe/README.md)",
            cited["| Tool"],
        )
        self.assertIn("Beside the transcript in the archive", cited["| Evidence"])
        self.assertIn(
            "`transcripts/the-show/2026-10-06-episode/transcript.md`",
            cited["| Transcript"],
        )
        self.assertIn(published["transcript_sha256"], cited["| Transcript"])

    def test_publish_refuses_a_git_checkout_and_a_missing_archive(self):
        with tempfile.TemporaryDirectory() as tmp:
            work = make_work_dir(tmp)
            checkout = Path(tmp) / "checkout"
            (checkout / ".git").mkdir(parents=True)
            with self.assertRaises(SystemExit):
                pt.main(["publish", str(work), "--out-dir", str(checkout / "data")])
            self.assertFalse((checkout / "data").exists())
            with mock.patch.dict(pt.os.environ, {}, clear=True), self.assertRaises(
                SystemExit
            ):
                pt.main(["publish", str(work)])


if __name__ == "__main__":
    unittest.main()
