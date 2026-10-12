#!/usr/bin/env python3
"""Transcribe a podcast episode with two independent ASR passes and align them.

Takes an Apple Podcasts, Castro or Spotify episode link (or a direct audio URL), finds the
episode in the show's public RSS feed, downloads the audio and transcribes it twice, with engines
from different model families: OpenAI Whisper (an encoder-decoder transformer) and NVIDIA
Parakeet TDT (a transducer). Their mistakes are largely independent, so the places where they
disagree are where the transcript needs judgement. The passes are aligned word by word and every
disagreement is written into a worksheet, which the ``podcast-transcript`` agent skill reads to
write the reconciled transcript. ``check`` then verifies that transcript against both passes, and
``publish`` renders it into an archive outside the repository.

Usage:
    podcast-transcribe.py resolve URL
    podcast-transcribe.py prepare URL [--work-root DIR] [--pass-a ENGINE:MODEL] [--pass-b ...]
    podcast-transcribe.py check WORK_DIR
    podcast-transcribe.py publish WORK_DIR [--out-dir DIR]
    podcast-transcribe.py cite ARCHIVE_EPISODE_DIR [--doc PATH]

Nothing here is committed. Audio and raw engine output stay in the work directory (default
``~/.cache/velocity-report/podcast-transcribe/<episode>/``), and ``publish`` writes the transcript
and the evidence behind it to ``$PODCAST_TRANSCRIPT_DIR/<show>/<episode>/``, which must lie outside
any Git checkout: the full text of someone else's episode does not belong in a public repository.
Documents summarise it, cite it by timestamp, and carry the provenance table ``cite`` prints: the
transcript's header fields and provenance rows, with its archive path and SHA-256.
"""

from __future__ import annotations

import argparse
import hashlib
import html
import json
import os
import re
import shlex
import shutil
import subprocess
import sys
import textwrap
import time
import unicodedata
import xml.etree.ElementTree as ET
from datetime import datetime, timezone
from difflib import SequenceMatcher
from email.utils import parsedate_to_datetime
from html.parser import HTMLParser
from pathlib import Path
from typing import Any
from urllib.parse import parse_qs, quote_plus, urlparse, urlunparse
from urllib.request import Request, urlopen

REPO_ROOT = Path(__file__).resolve().parents[2]
DEFAULT_WORK_ROOT = (
    Path(os.environ.get("XDG_CACHE_HOME", Path.home() / ".cache"))
    / "velocity-report"
    / "podcast-transcribe"
)
DEFAULT_PASS_A = "openai-whisper:turbo"
DEFAULT_PASS_B = "parakeet-mlx:mlx-community/parakeet-tdt-0.6b-v2"
USER_AGENT = (
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
    "velocity.report-podcast-transcribe/1 (+https://github.com/banshee-data/velocity.report)"
)
ITUNES = "https://itunes.apple.com"
NS = {
    "itunes": "http://www.itunes.com/dtds/podcast-1.0.dtd",
    "content": "http://purl.org/rss/1.0/modules/content/",
    "podcast": "https://podcastindex.org/namespace/1.0",
}
AUDIO_EXT_RE = re.compile(r"\.(mp3|m4a|aac|ogg|oga|opus|wav|flac)$", re.I)

# Worksheet and check tuning. A chunk closes at the first sentence end after TARGET words; a pause
# this long between words is printed, because it often marks a change of speaker.
CHUNK_TARGET_WORDS = 220
PAUSE_MARK_S = 1.5
# Words the reconciled transcript may drop without being reported as an omission.
FILLERS = {"um", "uh", "er", "erm", "ah", "eh", "hmm", "mm", "mhm", "uhhuh", "umhum"}
# A run of this many consecutive words fails the check: in the transcript but heard by neither
# pass (an invention), or heard by both passes but missing from the transcript (an omission).
MAX_RUN = 4


# ---------------------------------------------------------------------------
# HTTP and HTML
# ---------------------------------------------------------------------------


def fetch(url: str, timeout: float = 60) -> bytes:
    req = Request(url, headers={"User-Agent": USER_AGENT})
    with urlopen(req, timeout=timeout) as resp:
        return resp.read()


def fetch_json(url: str) -> Any:
    return json.loads(fetch(url).decode("utf-8"))


class PageParser(HTMLParser):
    """Collect what episode pages expose: meta tags, audio sources and labelled links."""

    def __init__(self) -> None:
        super().__init__(convert_charrefs=True)
        self.meta: dict[str, str] = {}
        self.audio: list[str] = []
        self.links: list[dict[str, str]] = []

    def handle_starttag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        a = {k: v or "" for k, v in attrs}
        if tag == "meta":
            key = a.get("property") or a.get("name")
            if key and "content" in a:
                self.meta.setdefault(key, a["content"])
        elif tag in ("audio", "source") and a.get("src"):
            self.audio.append(a["src"])
        elif tag == "a" and a.get("href"):
            self.links.append(a)


def parse_page(raw: bytes | str) -> PageParser:
    parser = PageParser()
    parser.feed(raw.decode("utf-8", "replace") if isinstance(raw, bytes) else raw)
    return parser


class _TextParser(HTMLParser):
    BLOCK = {"p", "br", "div", "li", "ul", "ol", "h1", "h2", "h3", "h4", "tr"}

    def __init__(self) -> None:
        super().__init__(convert_charrefs=True)
        self.parts: list[str] = []
        self.href: str | None = None
        self.link_text: list[str] = []

    def handle_starttag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        if tag in self.BLOCK:
            self.parts.append("\n")
        if tag == "li":
            self.parts.append("- ")
        if tag == "a":
            self.href = dict(attrs).get("href") or None
            self.link_text = []

    def handle_endtag(self, tag: str) -> None:
        if tag == "a" and self.href is not None:
            text = "".join(self.link_text).strip()
            if text and self.href.startswith("http") and self.href not in text:
                self.parts.append(f" ({self.href})")
            self.href = None
        if tag in self.BLOCK:
            self.parts.append("\n")

    def handle_data(self, data: str) -> None:
        self.parts.append(data)
        if self.href is not None:
            self.link_text.append(data)


def html_to_text(fragment: str) -> str:
    """Plain text from show-notes HTML: one paragraph per block, links kept as URLs."""
    if "<" not in fragment:
        return html.unescape(fragment).strip()
    parser = _TextParser()
    parser.feed(fragment)
    text = "".join(parser.parts)
    paras = [re.sub(r"[ \t ]+", " ", p).strip() for p in re.split(r"\n\s*\n|\n", text)]
    return "\n\n".join(p for p in paras if p)


# ---------------------------------------------------------------------------
# Resolving a link to an episode
# ---------------------------------------------------------------------------


def norm_title(text: str) -> str:
    text = unicodedata.normalize("NFKD", text).lower()
    return re.sub(r"[^0-9a-z]+", " ", text).strip()


def slug(text: str, limit: int = 60) -> str:
    s = norm_title(text).replace(" ", "-")
    if len(s) > limit:
        s = s[:limit].rsplit("-", 1)[0]
    return s or "episode"


def strip_query(url: str) -> str:
    p = urlparse(url)
    return urlunparse(("", p.netloc.lower(), p.path, "", "", ""))


def parse_duration(value: str | None) -> float | None:
    if not value:
        return None
    value = value.strip()
    if re.fullmatch(r"\d+(\.\d+)?", value):
        return float(value)
    parts = value.split(":")
    if all(p.isdigit() for p in parts) and 1 < len(parts) <= 3:
        seconds = 0.0
        for p in parts:
            seconds = seconds * 60 + int(p)
        return seconds
    return None


def resolve_apple(url: str) -> dict[str, Any]:
    parsed = urlparse(url)
    show_id = re.search(r"/id(\d+)", parsed.path)
    episode_id = parse_qs(parsed.query).get("i", [None])[0]
    if not show_id or not episode_id:
        raise SystemExit(
            "Apple Podcasts link needs a show id (/id123) and an episode id (?i=456); "
            "share the episode, not the show."
        )
    data = fetch_json(
        f"{ITUNES}/lookup?id={show_id.group(1)}&entity=podcastEpisode&limit=300"
    )
    episode: dict[str, Any] = {"platform": "apple"}
    for r in data.get("results", []):
        if r.get("kind") == "podcast":
            episode.update(show=r.get("collectionName"), feed_url=r.get("feedUrl"))
        elif str(r.get("trackId")) == episode_id:
            episode.update(
                show=r.get("collectionName"),
                feed_url=r.get("feedUrl") or episode.get("feed_url"),
                title=r.get("trackName"),
                guid=r.get("episodeGuid"),
                released=(r.get("releaseDate") or "")[:10] or None,
                enclosure_url=r.get("episodeUrl"),
            )
    if not episode.get("title"):
        # Older than the lookup's window: take the title from the page and match it in the feed.
        page = parse_page(fetch(url))
        episode["title"] = page.meta.get("og:title") or page.meta.get("twitter:title")
    return episode


def resolve_castro(url: str) -> dict[str, Any]:
    page = parse_page(fetch(url))
    show = None
    for link in page.links:
        label = link.get("aria-label", "")
        if "/podcast/" in link["href"] and label.endswith(" page"):
            show = label[: -len(" page")]
            break
    title = re.sub(r"\s*\(\d+\s*[hms][^)]*\)\s*$", "", page.meta.get("og:title", ""))
    if show and title.startswith(show + ": "):
        title = title[len(show) + 2 :]
    if not page.audio:
        raise SystemExit(f"no audio source on the Castro page {url}")
    return {
        "platform": "castro",
        "show": show,
        "title": title or None,
        "enclosure_url": page.audio[0],
    }


def resolve_spotify(url: str) -> dict[str, Any]:
    page = parse_page(fetch(url))
    title = page.meta.get("og:title")
    show = page.meta.get("og:description", "").split(" · ")[0] or None
    if not show:
        m = re.match(
            r"Listen to this episode from (.+?) on Spotify\.",
            page.meta.get("description", ""),
        )
        show = m.group(1) if m else None
    if not title or not show:
        raise SystemExit(f"could not read the episode and show titles from {url}")
    return {
        "platform": "spotify",
        "show": show,
        "title": title,
        "released": (page.meta.get("music:release_date") or "")[:10] or None,
        "duration_s": parse_duration(page.meta.get("music:duration")),
    }


def resolve_generic(url: str) -> dict[str, Any]:
    page = parse_page(fetch(url))
    audio = page.meta.get("og:audio") or page.meta.get("og:audio:url")
    audio = audio or next(iter(page.audio), None)
    if not audio:
        raise SystemExit(
            f"{url} is not an Apple Podcasts, Castro or Spotify link and has no audio on the page"
        )
    return {
        "platform": "web",
        "title": page.meta.get("og:title"),
        "enclosure_url": audio,
    }


def find_feed(show: str) -> str:
    term = quote_plus(show)
    data = fetch_json(
        f"{ITUNES}/search?term={term}&media=podcast&entity=podcast&limit=10"
    )
    results = [r for r in data.get("results", []) if r.get("feedUrl")]
    exact = [
        r
        for r in results
        if norm_title(r.get("collectionName", "")) == norm_title(show)
    ]
    if len(exact) >= 1:
        return exact[0]["feedUrl"]
    names = ", ".join(r.get("collectionName", "?") for r in results) or "none"
    raise SystemExit(
        f"no podcast named {show!r} in the Apple directory (candidates: {names})"
    )


def _text(el: ET.Element | None) -> str | None:
    return el.text.strip() if el is not None and el.text and el.text.strip() else None


def _persons(parent: ET.Element) -> list[dict[str, str]]:
    return [
        {"name": p.text.strip(), "role": p.get("role", "host")}
        for p in parent.findall("podcast:person", NS)
        if p.text and p.text.strip()
    ]


def parse_feed(raw: bytes) -> tuple[dict[str, Any], list[dict[str, Any]]]:
    channel = ET.fromstring(raw).find("channel")
    if channel is None:
        raise SystemExit("feed has no <channel>")
    show = {
        "show": _text(channel.find("title")),
        "show_author": _text(channel.find("itunes:author", NS)),
        "show_link": _text(channel.find("link")),
        "show_description": _text(channel.find("description"))
        or _text(channel.find("itunes:summary", NS)),
        "show_persons": _persons(channel),
    }
    items = []
    for it in channel.findall("item"):
        enc = it.find("enclosure")
        pub = _text(it.find("pubDate"))
        try:
            released = (
                parsedate_to_datetime(pub).astimezone(timezone.utc).date().isoformat()
            )
        except (TypeError, ValueError):
            released = None
        notes = (
            _text(it.find("content:encoded", NS))
            or _text(it.find("description"))
            or _text(it.find("itunes:summary", NS))
        )
        items.append(
            {
                "title": _text(it.find("title")),
                "guid": _text(it.find("guid")),
                "released": released,
                "duration_s": parse_duration(_text(it.find("itunes:duration", NS))),
                "enclosure_url": enc.get("url") if enc is not None else None,
                "enclosure_type": enc.get("type") if enc is not None else None,
                "link": _text(it.find("link")),
                "notes_html": notes,
                "persons": _persons(it),
                "publisher_transcripts": [
                    {"url": t.get("url"), "type": t.get("type")}
                    for t in it.findall("podcast:transcript", NS)
                ],
            }
        )
    return show, items


def match_item(
    episode: dict[str, Any], items: list[dict[str, Any]]
) -> dict[str, Any] | None:
    """Find the episode in the feed: by GUID, then audio URL, then title (nearest date)."""
    if episode.get("guid"):
        for it in items:
            if it["guid"] == episode["guid"]:
                return it
    if episode.get("enclosure_url"):
        want = strip_query(episode["enclosure_url"])
        for it in items:
            if it["enclosure_url"] and strip_query(it["enclosure_url"]) == want:
                return it
    if episode.get("title"):
        want = norm_title(episode["title"])
        same = [it for it in items if it["title"] and norm_title(it["title"]) == want]
        if episode.get("released") and len(same) > 1:
            target = datetime.fromisoformat(episode["released"])

            def distance(it: dict[str, Any]) -> float:
                if not it["released"]:
                    return float("inf")
                return abs((datetime.fromisoformat(it["released"]) - target).days)

            same.sort(key=distance)
        if same:
            return same[0]
    return None


def resolve(url: str) -> dict[str, Any]:
    parsed = urlparse(url)
    host = parsed.netloc.lower()
    if host.endswith("podcasts.apple.com"):
        episode = resolve_apple(url)
    elif host.endswith("castro.fm"):
        episode = resolve_castro(url)
    elif host.endswith("spotify.com"):
        if "/episode/" not in parsed.path:
            raise SystemExit(
                "Spotify link is not an episode (expected open.spotify.com/episode/…)"
            )
        episode = resolve_spotify(url)
    elif AUDIO_EXT_RE.search(parsed.path):
        episode = {
            "platform": "direct",
            "enclosure_url": url,
            "title": Path(parsed.path).stem,
        }
    else:
        episode = resolve_generic(url)
    episode["source_url"] = url

    feed_url = episode.get("feed_url")
    if not feed_url and episode.get("show"):
        feed_url = find_feed(episode["show"])
    if feed_url:
        show, items = parse_feed(fetch(feed_url))
        item = match_item(episode, items)
        if item is None and not episode.get("enclosure_url"):
            raise SystemExit(
                f"episode {episode.get('title')!r} is not in the feed {feed_url}"
            )
        episode["feed_url"] = feed_url
        episode.update({k: v for k, v in show.items() if v})
        if item is not None:
            episode.update({k: v for k, v in item.items() if v})
    if not episode.get("enclosure_url"):
        raise SystemExit(
            "no public audio for this episode: Spotify-exclusive episodes cannot be fetched"
        )
    return episode


def episode_key(episode: dict[str, Any]) -> str:
    title = episode.get("title") or "episode"
    return (
        f"{episode['released']}-{slug(title)}"
        if episode.get("released")
        else slug(title)
    )


# ---------------------------------------------------------------------------
# Audio
# ---------------------------------------------------------------------------


def require_tool(name: str, hint: str) -> str:
    path = shutil.which(name)
    if not path:
        raise SystemExit(f"{name} not found on PATH; {hint}")
    return path


def download(url: str, dest: Path) -> str:
    """Stream ``url`` to ``dest`` and return its SHA-256."""
    digest = hashlib.sha256()
    part = dest.with_suffix(dest.suffix + ".part")
    req = Request(url, headers={"User-Agent": USER_AGENT})
    with urlopen(req, timeout=120) as resp, open(part, "wb") as out:
        while chunk := resp.read(1 << 20):
            digest.update(chunk)
            out.write(chunk)
    part.replace(dest)
    return digest.hexdigest()


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with open(path, "rb") as fh:
        while chunk := fh.read(1 << 20):
            digest.update(chunk)
    return digest.hexdigest()


def probe(path: Path) -> dict[str, Any]:
    ffprobe = require_tool("ffprobe", "install ffmpeg (brew install ffmpeg)")
    out = subprocess.run(
        [
            ffprobe,
            "-v",
            "error",
            "-show_entries",
            "format=duration,bit_rate:stream=codec_name",
            "-of",
            "json",
            str(path),
        ],
        check=True,
        capture_output=True,
        text=True,
    ).stdout
    data = json.loads(out)
    fmt = data.get("format", {})
    streams = data.get("streams") or [{}]
    return {
        "duration_s": round(float(fmt.get("duration", 0)), 2),
        "bit_rate": int(fmt["bit_rate"]) if fmt.get("bit_rate") else None,
        "codec": streams[0].get("codec_name"),
    }


def to_wav(src: Path, wav: Path) -> None:
    ffmpeg = require_tool("ffmpeg", "install ffmpeg (brew install ffmpeg)")
    subprocess.run(
        [
            ffmpeg,
            "-nostdin",
            "-v",
            "error",
            "-y",
            "-i",
            str(src),
            "-ac",
            "1",
            "-ar",
            "16000",
            "-c:a",
            "pcm_s16le",
            str(wav),
        ],
        check=True,
    )


# ---------------------------------------------------------------------------
# ASR engines
# ---------------------------------------------------------------------------


def package_version(executable: str, dist: str) -> str:
    """Version of ``dist`` in the interpreter behind a console script (pipx, venv or --user)."""
    try:
        first = Path(executable).read_text(errors="replace").split("\n", 1)[0]
    except OSError:
        return "unknown"
    if not first.startswith("#!"):
        return "unknown"
    interpreter = first[2:].strip().split()[0]
    code = "import importlib.metadata as m, sys; print(m.version(sys.argv[1]))"
    try:
        out = subprocess.run(
            [interpreter, "-c", code, dist], capture_output=True, text=True, timeout=60
        )
    except OSError:
        return "unknown"
    return (
        out.stdout.strip() if out.returncode == 0 and out.stdout.strip() else "unknown"
    )


class WhisperEngine:
    """OpenAI's reference Whisper (PyTorch). MPS cannot hold its alignment heads, so CPU."""

    name = "openai-whisper"
    dist = "openai-whisper"
    binary = "whisper"
    hint = "pipx install openai-whisper (or make install-transcribe)"

    def command(
        self, exe: str, wav: Path, out: Path, model: str, episode: dict, lang: str
    ):
        prompt = ". ".join(x for x in (episode.get("show"), episode.get("title")) if x)
        cmd = [
            exe,
            str(wav),
            "--model",
            model,
            "--language",
            lang,
            "--task",
            "transcribe",
            "--output_format",
            "json",
            "--output_dir",
            str(out),
            "--word_timestamps",
            "True",
            "--device",
            "cpu",
            "--fp16",
            "False",
            "--verbose",
            "False",
        ]
        if prompt:
            # Spells the show's and guest's names for the opening windows; recorded in provenance.
            cmd += ["--initial_prompt", prompt + "."]
        return cmd

    def load(self, native: Path) -> list[dict[str, Any]]:
        data = json.loads(native.read_text())
        pieces = []
        for si, seg in enumerate(data.get("segments", [])):
            seg_words = seg.get("words")
            if seg_words:
                for w in seg_words:
                    pieces.append(
                        (w["word"], w["start"], w["end"], w.get("probability"), si)
                    )
            else:  # no word timestamps: spread the segment's words over its span
                tokens = seg.get("text", "").split()
                step = (seg["end"] - seg["start"]) / max(len(tokens), 1)
                for k, t in enumerate(tokens):
                    s = seg["start"] + k * step
                    pieces.append((" " + t, s, s + step, None, si))
        return words_from_pieces(pieces)


class ParakeetEngine:
    """NVIDIA Parakeet TDT through MLX on Apple Silicon: a transducer, not a Whisper model."""

    name = "parakeet-mlx"
    dist = "parakeet-mlx"
    binary = "parakeet-mlx"
    hint = "pipx install --python python3.12 parakeet-mlx (or make install-transcribe)"

    def command(
        self, exe: str, wav: Path, out: Path, model: str, episode: dict, lang: str
    ):
        return [
            exe,
            str(wav),
            "--model",
            model,
            "--output-format",
            "json",
            "--output-dir",
            str(out),
        ]

    def load(self, native: Path) -> list[dict[str, Any]]:
        data = json.loads(native.read_text())
        pieces = []
        for si, sentence in enumerate(data.get("sentences", [])):
            for tok in sentence.get("tokens", []):
                pieces.append(
                    (tok["text"], tok["start"], tok["end"], tok.get("confidence"), si)
                )
        return words_from_pieces(pieces)


ENGINES = {e.name: e for e in (WhisperEngine(), ParakeetEngine())}

COMPOUND_RE = re.compile(r"(?<=[^\W_])[-‐‑–—/](?=[^\W_])")


def split_word(text: str) -> list[str]:
    """Split a hyphenated compound after each hyphen, which stays on the left piece.

    Hyphenation is the transcriber's punctuation, not something a pass heard, so "safety-focused"
    and "safety focused" compare equal. Rendering rejoins a piece that ends in a hyphen.
    """
    pieces, last = [], 0
    for m in COMPOUND_RE.finditer(text):
        pieces.append(text[last : m.end()])
        last = m.end()
    pieces.append(text[last:])
    return [p for p in pieces if p]


def join_words(surfaces: list[str]) -> str:
    return re.sub(r"(?<=[^\W_])([-‐‑–—/]) (?=[^\W_])", r"\1", " ".join(surfaces))


def words_from_pieces(
    pieces: list[tuple[str, float, float, float | None, int]],
) -> list[dict]:
    """Words from engine pieces: a piece without a leading space continues the word before it."""
    merged: list[list[Any]] = []
    for text, start, end, p, seg in pieces:
        if merged and not text.startswith(" ") and merged[-1][4] == seg:
            prev = merged[-1]
            prev[0] += text
            prev[2] = end
            if p is not None:
                prev[3] = p if prev[3] is None else min(prev[3], p)
        else:
            merged.append([text, start, end, p, seg])
    words: list[dict[str, Any]] = []
    for text, start, end, p, seg in merged:
        text = text.strip()
        if not text:
            continue
        if not norm(text):  # bare punctuation belongs to the word before it
            if words:
                words[-1]["t"] += text
            continue
        parts = split_word(text)
        total = sum(len(x) for x in parts)
        t = start
        for part in parts:
            span = (end - start) * len(part) / total
            words.append(
                {
                    "t": part,
                    "s": round(t, 2),
                    "e": round(t + span, 2),
                    "p": None if p is None else round(p, 3),
                    "seg": seg,
                }
            )
            t += span
    return words


def parse_pass(spec: str) -> tuple[Any, str]:
    name, _, model = spec.partition(":")
    if name not in ENGINES or not model:
        raise SystemExit(
            f"pass {spec!r}: expected ENGINE:MODEL with ENGINE in {sorted(ENGINES)}"
        )
    return ENGINES[name], model


def pass_id(engine: Any, model: str) -> str:
    return f"{engine.name}-{model.rsplit('/', 1)[-1]}"


def run_pass(
    spec: str, work: Path, episode: dict, audio_sha: str, lang: str, force: bool
) -> dict:
    engine, model = parse_pass(spec)
    out = work / "passes" / pass_id(engine, model)
    native = out / "audio.json"
    run_file = out / "run.json"
    if native.exists() and run_file.exists() and not force:
        run = json.loads(run_file.read_text())
        if run.get("audio_sha256") == audio_sha:
            print(f"  {run['id']}: reusing {native}")
            return run
    exe = require_tool(engine.binary, engine.hint)
    out.mkdir(parents=True, exist_ok=True)
    cmd = engine.command(exe, work / "audio.wav", out, model, episode, lang)
    print(f"  {pass_id(engine, model)}: running {engine.binary} …", flush=True)
    t0 = time.monotonic()
    with open(out / "stderr.log", "w") as log:
        proc = subprocess.run(cmd, stdout=log, stderr=subprocess.STDOUT)
    wall = round(time.monotonic() - t0, 1)
    if proc.returncode != 0 or not native.exists():
        raise SystemExit(
            f"{engine.name} failed (exit {proc.returncode}); see {out / 'stderr.log'}"
        )
    run = {
        "id": pass_id(engine, model),
        "engine": engine.name,
        "version": package_version(exe, engine.dist),
        "model": model,
        "command": [Path(cmd[0]).name, *cmd[1:]],
        "wall_s": wall,
        "audio_sha256": audio_sha,
        "finished": datetime.now(timezone.utc).isoformat(timespec="seconds"),
    }
    run_file.write_text(json.dumps(run, indent=2) + "\n")
    print(f"  {run['id']}: {wall:.0f} s")
    return run


def load_pass(work: Path, run: dict) -> list[dict[str, Any]]:
    return ENGINES[run["engine"]].load(work / "passes" / run["id"] / "audio.json")


# ---------------------------------------------------------------------------
# Alignment
# ---------------------------------------------------------------------------


def norm(word: str) -> str:
    """Comparison form of a word: case, accents, apostrophes and punctuation removed."""
    word = unicodedata.normalize("NFKD", word).lower()
    return re.sub(r"[^0-9a-z]+", "", word)


def align(a: list[str], b: list[str]) -> list[tuple[str, int, int, int, int]]:
    """Opcodes aligning two normalised word lists; ``same`` and ``diff`` spans only."""
    ops = []
    for tag, i1, i2, j1, j2 in SequenceMatcher(
        None, a, b, autojunk=False
    ).get_opcodes():
        ops.append(("same" if tag == "equal" else "diff", i1, i2, j1, j2))
    return ops


def pair_stats(ops: list, na: int, nb: int) -> dict[str, Any]:
    matched = sum(i2 - i1 for op, i1, i2, _, _ in ops if op == "same")
    subs = dels = ins = spans = 0
    for op, i1, i2, j1, j2 in ops:
        if op == "diff":
            spans += 1
            la, lb = i2 - i1, j2 - j1
            subs += min(la, lb)
            dels += max(0, la - lb)
            ins += max(0, lb - la)
    return {
        "words_a": na,
        "words_b": nb,
        "matched": matched,
        "agreement": round(matched / max(na, nb, 1), 4),
        "wer_b_vs_a": round((subs + dels + ins) / max(na, 1), 4),
        "divergent_spans": spans,
    }


# ---------------------------------------------------------------------------
# Worksheet
# ---------------------------------------------------------------------------


def hms(seconds: float, always_hours: bool = True) -> str:
    s = int(seconds)
    h, m, sec = s // 3600, s % 3600 // 60, s % 60
    return f"{h:02d}:{m:02d}:{sec:02d}" if always_hours or h else f"{m:02d}:{sec:02d}"


def notes_vocabulary(episode: dict[str, Any]) -> dict[str, str]:
    """Capitalised words from the title, show, people and notes: how the publisher spells them."""
    persons = episode.get("persons", []) + episode.get("show_persons", [])
    sources = [
        episode.get("title") or "",
        episode.get("show") or "",
        episode.get("show_author") or "",
        html_to_text(episode.get("notes_html") or ""),
        " ".join(p["name"] for p in persons),
    ]
    vocab: dict[str, str] = {}
    for word in re.findall(r"\b[A-Z][A-Za-z'’-]{3,}\b", " ".join(sources)):
        vocab.setdefault(norm(word), word)
    return vocab


def spelling_hint(surface: str, vocab: dict[str, str]) -> str | None:
    """The notes' spelling of a capitalised word the passes heard slightly differently."""
    if not surface[:1].isupper():
        return None
    key = norm(surface)
    if len(key) < 4 or key in vocab:
        return None
    best, best_ratio = None, 0.0
    for k, spelled in vocab.items():
        if abs(len(k) - len(key)) > 2 or k.startswith(key) or key.startswith(k):
            continue  # a prefix is an inflection ("Really", "Real"), not a misspelling
        ratio = SequenceMatcher(None, key, k).ratio()
        if ratio > best_ratio:
            best, best_ratio = spelled, ratio
    return best if best_ratio >= 0.75 else None


def _mean_p(words: list[dict]) -> str:
    ps = [w["p"] for w in words if w.get("p") is not None]
    return f" ({sum(ps) / len(ps):.2f})" if ps else ""


def _side(words: list[dict]) -> str:
    return (join_words([w["t"] for w in words]) + _mean_p(words)) if words else "∅"


def build_items(a: list[dict], b: list[dict], ops: list) -> list[dict[str, Any]]:
    """One item per agreed word, one per divergent span, in time order."""
    items = []
    for op, i1, i2, j1, j2 in ops:
        if op == "same":
            for k in range(i2 - i1):
                items.append({"same": True, "a": [a[i1 + k]], "b": [b[j1 + k]]})
        else:
            items.append({"same": False, "a": a[i1:i2], "b": b[j1:j2]})
    return items


def chunk_items(
    items: list[dict], target: int = CHUNK_TARGET_WORDS
) -> list[list[dict]]:
    """Split items into chunks that end at a sentence end, never inside a divergence."""
    chunks, cur, count = [], [], 0
    for item in items:
        cur.append(item)
        count += max(len(item["a"]), len(item["b"]))
        sentence_end = item["same"] and re.search(r"[.?!][\"'”’)]*$", item["a"][0]["t"])
        if count >= target and (sentence_end or count >= 2 * target) and item["same"]:
            chunks.append(cur)
            cur, count = [], 0
    if cur:
        chunks.append(cur)
    return chunks


def render_chunk(n: int, chunk: list[dict], vocab: dict[str, str]) -> str:
    words_in = [w for it in chunk for w in (it["a"] or it["b"])]
    start, end = words_in[0]["s"], words_in[-1]["e"]
    divergent = sum(1 for it in chunk if not it["same"])
    out: list[str] = []
    last_seg, last_end = None, None
    for it in chunk:
        lead = (it["a"] or it["b"])[0]
        seg = ("a", lead["seg"]) if it["a"] else ("b", lead["seg"])
        if last_end is not None and lead["s"] - last_end >= PAUSE_MARK_S:
            out.append(f"(pause {lead['s'] - last_end:.1f} s)")
        if seg != last_seg and it["a"]:
            out.append(f"[{hms(lead['s'], False)}]")
        last_seg = seg if it["a"] else last_seg
        if it["same"]:
            word = it["a"][0]["t"]
            hint = spelling_hint(word, vocab)
            out.append(f"{word} ⟨notes spell: {hint}⟩" if hint else word)
        else:
            out.append(f"⟦A: {_side(it['a'])} | B: {_side(it['b'])}⟧")
        last_end = max(w["e"] for w in it["a"] + it["b"])
    b_text = join_words([w["t"] for it in chunk for w in it["b"]])
    body = textwrap.fill(
        join_words(out), 100, break_long_words=False, break_on_hyphens=False
    )
    b_fill = textwrap.fill(b_text, 98, break_long_words=False, break_on_hyphens=False)
    return (
        f"## Chunk {n:02d} · {hms(start)}–{hms(end)} · {divergent} divergent\n\n{body}\n\n"
        + "\n".join("> " + line for line in ["B as punctuated:", *b_fill.splitlines()])
        + "\n"
    )


def write_worksheet(
    work: Path,
    episode: dict,
    runs: list[dict],
    a: list,
    b: list,
    ops: list,
    stats: dict,
) -> Path:
    vocab = notes_vocabulary(episode)
    chunks = chunk_items(build_items(a, b, ops))
    people = episode.get("persons", []) + episode.get("show_persons", [])
    people_text = ", ".join(f"{p['name']} ({p['role']})" for p in people) or "none"
    lines = [
        f"# Worksheet: {episode.get('title')}",
        "",
        "Reconcile with the `podcast-transcript` skill and write `reconciled.md` beside this file.",
        "",
        f"- Show: {episode.get('show')} ({episode.get('show_author')}): "
        f"{episode.get('show_description') or ''}",
        f"- Released: {episode.get('released')}; audio {hms(episode['audio']['duration_s'])}",
        f"- People in the feed: {people_text}",
        f"- Publisher transcripts: {episode.get('publisher_transcripts') or 'none'}",
    ]
    for label, run in zip("AB", runs):
        lines.append(
            f"- Pass {label}: {run['engine']} {run['version']}, model `{run['model']}`"
        )
    lines += [
        f"- Agreement: {stats['agreement']:.1%} of words; B differs from A at a word error rate "
        f"of {stats['wer_b_vs_a']:.1%}; {stats['divergent_spans']} divergent spans",
        f"- Spellings from the notes: {', '.join(sorted(set(vocab.values())))}",
        "",
        "Key: `[mm:ss]` starts a pass-A segment; `⟦A: … | B: …⟧` is a divergence with mean word "
        "confidence; `∅` is a pass that heard nothing there; `⟨notes spell: X⟩` follows an agreed "
        "word close to a spelling in the notes; `(pause n s)` is silence between words.",
        "",
        "## Episode notes",
        "",
        html_to_text(episode.get("notes_html") or "") or "(none)",
        "",
    ]
    for n, chunk in enumerate(chunks, 1):
        lines.append(render_chunk(n, chunk, vocab))
    path = work / "worksheet.md"
    path.write_text("\n".join(lines))
    return path


# ---------------------------------------------------------------------------
# Reconciled transcript: parse, check, publish
# ---------------------------------------------------------------------------

PARA_RE = re.compile(r"^\[(\d{1,2}:\d{2}:\d{2})\]\s*(.*)$")
EDITORIAL_RE = re.compile(r"\[[^\]]*\]")


def parse_reconciled(path: Path) -> dict[str, Any]:
    """Read ``reconciled.md``: a speakers header, ``---``, then timestamped paragraphs.

    speakers:
    - Kea Wilson: host, Streetsblog USA
    - Rebecca Steinbach: guest, AAA Foundation for Traffic Safety
    notes: optional sentence for the published introduction
    ---
    [00:00:01] Kea Wilson: Opening words …

    [00:01:12] Continuation by the same speaker …
    """
    text = path.read_text()
    head, sep, body = text.partition("\n---\n")
    if not sep:
        raise SystemExit(f"{path}: missing the '---' line after the speakers header")
    speakers: list[tuple[str, str]] = []
    notes = ""
    for line in head.splitlines():
        m = re.match(r"^\s*-\s*([^:]+):\s*(.*)$", line)
        if m:
            speakers.append((m.group(1).strip(), m.group(2).strip()))
        elif line.startswith("notes:"):
            notes = line[len("notes:") :].strip()
    names = sorted((n for n, _ in speakers), key=len, reverse=True)
    paragraphs: list[dict[str, Any]] = []
    for block in re.split(r"\n\s*\n", body.strip()):
        block = " ".join(block.split())
        m = PARA_RE.match(block)
        if not m:
            raise SystemExit(
                f"{path}: paragraph without a [hh:mm:ss] timestamp: {block[:80]!r}"
            )
        h, mi, s = (int(x) for x in m.group(1).split(":"))
        rest, speaker = m.group(2), None
        for name in names:
            if rest.startswith(name + ":"):
                speaker, rest = name, rest[len(name) + 1 :].strip()
                break
        paragraphs.append(
            {"t": h * 3600 + mi * 60 + s, "speaker": speaker, "text": rest}
        )
    if not paragraphs:
        raise SystemExit(f"{path}: no paragraphs")
    return {"speakers": speakers, "notes": notes, "paragraphs": paragraphs}


def reconciled_tokens(paragraphs: list[dict]) -> list[tuple[str, int]]:
    tokens = []
    for pi, para in enumerate(paragraphs):
        for word in EDITORIAL_RE.sub(" ", para["text"]).split():
            for piece in split_word(word):
                if norm(piece):
                    tokens.append((norm(piece), pi))
    return tokens


def _runs(flags: list[bool]) -> list[tuple[int, int]]:
    runs, start = [], None
    for i, flag in enumerate(flags + [False]):
        if flag and start is None:
            start = i
        elif not flag and start is not None:
            runs.append((start, i))
            start = None
    return runs


def check_reconciled(
    rec: dict, a: list[dict], b: list[dict], duration: float
) -> dict[str, Any]:
    """Every transcript word must be heard by a pass; every word both passes agree on must stay."""
    tokens = reconciled_tokens(rec["paragraphs"])
    r = [t for t, _ in tokens]
    an, bn = [norm(w["t"]) for w in a], [norm(w["t"]) for w in b]
    in_a, in_b = [False] * len(r), [False] * len(r)
    a_kept = [False] * len(an)
    for op, i1, i2, j1, j2 in align(r, an):
        if op == "same":
            for k in range(i2 - i1):
                in_a[i1 + k] = True
                a_kept[j1 + k] = True
    for op, i1, i2, j1, j2 in align(r, bn):
        if op == "same":
            for k in range(i2 - i1):
                in_b[i1 + k] = True
    agreed = [False] * len(an)
    for op, i1, i2, _, _ in align(an, bn):
        if op == "same":
            for k in range(i1, i2):
                agreed[k] = True

    unsupported = []
    for s, e in _runs(
        [
            not (x or y) and r[i] not in FILLERS
            for i, (x, y) in enumerate(zip(in_a, in_b))
        ]
    ):
        para = rec["paragraphs"][tokens[s][1]]
        unsupported.append(
            {"words": " ".join(r[s:e]), "n": e - s, "paragraph_at": hms(para["t"])}
        )
    omitted = []
    for s, e in _runs(
        [agreed[i] and not a_kept[i] and an[i] not in FILLERS for i in range(len(an))]
    ):
        omitted.append(
            {
                "words": join_words([w["t"] for w in a[s:e]]),
                "n": e - s,
                "at": hms(a[s]["s"]),
            }
        )

    problems = []
    declared = {n for n, _ in rec["speakers"]}
    if not declared:
        problems.append("no speakers declared in the header")
    if rec["paragraphs"][0]["speaker"] is None:
        problems.append("the first paragraph names no speaker")
    times = [p["t"] for p in rec["paragraphs"]]
    if any(t2 < t1 for t1, t2 in zip(times, times[1:])):
        problems.append("paragraph timestamps go backwards")
    if max(times) > duration + 1:
        problems.append(f"a timestamp ({hms(max(times))}) is past the end of the audio")
    long_unsupported = [u for u in unsupported if u["n"] >= MAX_RUN]
    long_omitted = [o for o in omitted if o["n"] >= MAX_RUN]
    for u in long_unsupported:
        problems.append(
            f"heard by neither pass ({u['n']} words near {u['paragraph_at']}): {u['words']}"
        )
    for o in long_omitted:
        problems.append(
            f"agreed by both passes but missing ({o['n']} words at {o['at']}): {o['words']}"
        )
    return {
        "ok": not problems,
        "problems": problems,
        "words": len(r),
        "supported_by_a": round(sum(in_a) / max(len(r), 1), 4),
        "supported_by_b": round(sum(in_b) / max(len(r), 1), 4),
        "editorial_words": sum(u["n"] for u in unsupported),
        "unsupported": unsupported,
        "omitted_agreed_words": sum(o["n"] for o in omitted),
        "omitted": omitted,
        "paragraphs": len(rec["paragraphs"]),
    }


def load_work(work: Path) -> tuple[dict, list[dict], list[list[dict]]]:
    state = json.loads((work / "state.json").read_text())
    passes = [load_pass(work, run) for run in state["runs"]]
    return state, state["runs"], passes


def run_check(work: Path) -> tuple[dict, dict, dict]:
    state, _, (a, b) = load_work(work)
    rec = parse_reconciled(work / "reconciled.md")
    result = check_reconciled(rec, a, b, state["episode"]["audio"]["duration_s"])
    (work / "check.json").write_text(json.dumps(result, indent=2) + "\n")
    return state, rec, result


URL_RE = re.compile(r"https?://[^\s)<>]+[^\s)<>.,;:!?]")


def md_escape(text: str) -> str:
    """Escape Markdown in prose; bare URLs become autolinks and are left as written."""
    out, last = [], 0
    for m in URL_RE.finditer(text):
        out.append(re.sub(r"([*_`<])", r"\\\1", text[last : m.start()]))
        out.append(f"<{m.group(0)}>")
        last = m.end()
    out.append(re.sub(r"([*_`<])", r"\\\1", text[last:]))
    return "".join(out)


BLOCK_START_RE = re.compile(r"^(\d+)([.)])(\s|$)|^([-+*>#=|])(\s|$)")


def wrap_md(text: str, width: int = 99, prefix: str = "") -> list[str]:
    """Wrap prose at ``width`` without letting a wrapped line start a list, quote or heading."""
    lines = textwrap.wrap(
        text, width - len(prefix), break_long_words=False, break_on_hyphens=False
    )
    out = []
    for i, line in enumerate(lines):
        if i:
            line = BLOCK_START_RE.sub(
                lambda m: (
                    f"{m.group(1)}\\{m.group(2)}{m.group(3)}"
                    if m.group(1)
                    else f"\\{m.group(4)}{m.group(5)}"
                ),
                line,
            )
        out.append(prefix + line)
    return out


def tool_revision() -> dict[str, Any]:
    """The commit the tool runs from, and whether the tool's own files differ from it."""
    git = ["git", "-C", str(REPO_ROOT)]
    try:
        sha = subprocess.run(
            [*git, "rev-parse", "--short=12", "HEAD"],
            capture_output=True,
            text=True,
            check=True,
        ).stdout.strip()
        dirty = subprocess.run(
            [*git, "status", "--porcelain", "--", str(Path(__file__).parent)],
            capture_output=True,
            text=True,
            check=True,
        ).stdout.strip()
    except (OSError, subprocess.CalledProcessError):
        return {"commit": None, "dirty": False}
    return {"commit": sha, "dirty": bool(dirty)}


def revision_text(rev: dict[str, Any] | None) -> str:
    if not rev or not rev.get("commit"):
        return "an unrecorded revision"
    return f"`{rev['commit']}`" + (
        " with uncommitted changes" if rev.get("dirty") else ""
    )


ARCHIVE_ENV = "PODCAST_TRANSCRIPT_DIR"
# Work-directory files kept beside the transcript; audio.wav is derived from the source audio.
ARCHIVED = ("state.json", "reconciled.md", "check.json", "worksheet.md")


def archive_dir(root: Path, episode: dict) -> Path:
    show = (episode.get("show") or "podcast").split(":")[0]
    return root / slug(show, 40) / episode_key(episode)


def in_git_checkout(path: Path) -> bool:
    for parent in [path.resolve(), *path.resolve().parents]:
        if (parent / ".git").exists():
            return True
    return False


def archive_work(work: Path, dest: Path) -> None:
    """Copy the evidence behind a transcript: source audio, both passes and the reconciliation."""
    for name in ARCHIVED:
        if (work / name).exists():
            shutil.copy2(work / name, dest / name)
    for source in work.glob("source.*"):
        shutil.copy2(source, dest / source.name)
    for run in sorted((work / "passes").iterdir()):
        if run.is_dir():
            shutil.copytree(run, dest / "passes" / run.name, dirs_exist_ok=True)


STATUS = (
    "Machine transcript from two independent speech-recognition passes, reconciled by an agent; "
    "not proofread against the audio by a person"
)
PLATFORM_LABELS = {
    "apple": "Apple Podcasts",
    "castro": "Castro",
    "spotify": "Spotify",
    "direct": "Audio file",
    "web": "Episode page",
}
EVIDENCE = (
    "the source audio, both passes with their logs, the worksheet, `reconciled.md` and "
    "`check.json`"
)


def md_link(text: str, url: str) -> str:
    """A Markdown link; a URL with spaces, parentheses or angle brackets goes in <>."""
    target = f"<{url}>" if re.search(r"[\s()<>]", url) else url
    return f"[{text}]({target})"


def _host(url: str) -> str:
    return urlparse(url).netloc or url


def _site(url: str) -> str:
    """The site's name without hosting subdomains ("c6996bab-….libsyn.com" reads "libsyn.com")."""
    return ".".join(_host(url).split(".")[-2:])


def pass_text(run: dict) -> str:
    cmd = run["command"]
    flags = []
    for i, arg in enumerate(cmd):
        if arg.startswith("--") and arg not in (
            "--output_dir",
            "--output-dir",
            "--output_format",
            "--output-format",
            "--verbose",
            "--model",
        ):
            value = (
                cmd[i + 1]
                if i + 1 < len(cmd) and not cmd[i + 1].startswith("--")
                else ""
            )
            flags += [arg, value] if value else [arg]
    settings = f"`{shlex.join(flags)}`" if flags else "default settings"
    wall = f"; {run['wall_s']:.0f} s wall" if run.get("wall_s") else ""
    return f"{run['engine']} {run['version']}, model `{run['model']}`, {settings}{wall}"


def header_fields(state: dict, rec: dict) -> list[tuple[str, str]]:
    """The transcript's header metadata: source, speakers, length and status."""
    ep = state["episode"]
    source = f"_{md_escape(ep.get('show') or '?')}_"
    if ep.get("show_author"):
        source += f" ({md_escape(ep['show_author'])})"
    if ep.get("released"):
        source += f", released {ep['released']}"
    roles = "; ".join(f"{n} ({r})" if r else n for n, r in rec["speakers"])
    minutes, seconds = divmod(int(round(ep["audio"]["duration_s"])), 60)
    return [
        ("Source", source),
        ("Speakers", md_escape(roles)),
        ("Length", f"{minutes} min {seconds:02d} s"),
        ("Status", STATUS),
    ]


def provenance_rows(
    state: dict,
    result: dict,
    revision: dict | None,
    evidence_where: str = "Beside this file",
    tool_link: str | None = None,
) -> list[tuple[str, str]]:
    """The provenance table, shared by the archived transcript and the documents that cite it."""
    ep, runs, stats = state["episode"], state["runs"], state["stats"]
    audio = ep["audio"]
    label = PLATFORM_LABELS.get(ep.get("platform", ""), "Episode page")
    rows = [("Episode link", md_link(label, ep["source_url"]))]
    if ep.get("link") and ep["link"] != ep["source_url"]:
        rows.append(("Publisher page", md_link(_site(ep["link"]), ep["link"])))
    if ep.get("feed_url"):
        rows.append(("RSS feed", md_link(_host(ep["feed_url"]), ep["feed_url"])))
    if ep.get("guid"):
        rows.append(("Episode GUID", f"`{ep['guid']}`"))
    enclosure = ep["enclosure_url"]
    rows += [
        (
            "Audio enclosure",
            md_link(
                f"{audio.get('codec') or 'audio'} on {_host(enclosure)}", enclosure
            ),
        ),
        (
            "Audio SHA-256",
            f"`{audio['sha256']}` ({audio.get('codec')}, "
            f"{(audio.get('bit_rate') or 0) // 1000} kbit/s, {hms(audio['duration_s'])})",
        ),
        ("Pass A", pass_text(runs[0])),
        ("Pass B", pass_text(runs[1])),
        (
            "Agreement between passes",
            f"{stats['agreement']:.1%} of words; word error rate of B against A "
            f"{stats['wer_b_vs_a']:.1%}; {stats['divergent_spans']} divergent spans",
        ),
        (
            "Reconciliation check",
            f"{result['words']} words in {result['paragraphs']} paragraphs; "
            f"{result['supported_by_a']:.1%} heard by A, {result['supported_by_b']:.1%} by B; "
            f"{result['editorial_words']} words heard by neither (spellings and editorial "
            f"marks); {result['omitted_agreed_words']} words both passes heard left out",
        ),
        (
            "Tool",
            (md_link("podcast-transcribe", tool_link) if tool_link else TOOL_PATH)
            + f" at {revision_text(revision)}, `podcast-transcript` skill",
        ),
        ("Evidence", f"{evidence_where}: {EVIDENCE}; work key `{state['key']}`"),
    ]
    return rows


TOOL_PATH = "`tools/podcast-transcribe/podcast-transcribe.py`"


def md_table(rows: list[tuple[str, str]]) -> list[str]:
    out = ["| Item | Value |", "| ---- | ----- |"]
    for item, value in rows:
        out.append("| " + item + " | " + value.replace("|", "\\|") + " |")
    return out


def render_transcript(
    state: dict, rec: dict, result: dict, revision: dict | None
) -> str:
    ep = state["episode"]
    lines = [f"# {md_escape(ep.get('title') or 'Podcast transcript')}", ""]
    lines += [f"- **{k}:** {v}" for k, v in header_fields(state, rec)] + [""]
    intro = (
        "A transcript of a published podcast episode, kept outside the public repository as a "
        "source for documents that summarise the episode and cite it by timestamp. The words "
        "are the speakers' own; punctuation, paragraphs and the attribution of each paragraph "
        "to a speaker are the transcriber's, inferred from the conversation rather than from "
        "the voices. Timestamps are positions in the episode audio. Fillers such as \u201cum\u201d "
        "and \u201cuh\u201d are left out, and text in square brackets is the transcriber's."
    )
    if rec["notes"]:
        intro += " " + rec["notes"]
    lines += wrap_md(intro) + ["", "## Episode notes", ""]
    notes = html_to_text(ep.get("notes_html") or "") or "None published."
    for i, para in enumerate(notes.split("\n\n")):
        if i:
            lines.append(">")
        lines += wrap_md(md_escape(para), prefix="> ")
    lines += ["", "## Transcript", ""]
    for para in rec["paragraphs"]:
        text = md_escape(para["text"])
        lead = (
            f"**{para['speaker']}** ({hms(para['t'])}):"
            if para["speaker"]
            else f"({hms(para['t'])})"
        )
        lines += wrap_md(f"{lead} {text}") + [""]
    lines += ["## Provenance", ""]
    lines += md_table(provenance_rows(state, result, revision)) + [""]
    return "\n".join(lines)


def cite_table(dest: Path, doc: Path | None = None) -> list[str]:
    """The table a document pastes when it cites an archived transcript.

    Every field of the transcript's header and every row of its provenance table, in the same
    order and wording, then the transcript's own path in the archive and its SHA-256.
    """
    state = json.loads((dest / "state.json").read_text())
    result = json.loads((dest / "check.json").read_text())
    rec = parse_reconciled(dest / "reconciled.md")
    published = {}
    if (dest / "published.json").exists():
        published = json.loads((dest / "published.json").read_text())
    tool_link = None
    if doc is not None:
        readme = REPO_ROOT / "tools" / "podcast-transcribe" / "README.md"
        tool_link = os.path.relpath(readme, Path(doc).resolve().parent)
    transcript = dest / "transcript.md"
    archive_name = dest.parent.parent.name
    rel = f"{archive_name}/{dest.parent.name}/{dest.name}/transcript.md"
    rows = header_fields(state, rec)
    rows += provenance_rows(
        state,
        result,
        published.get("tool"),
        evidence_where="Beside the transcript in the archive",
        tool_link=tool_link,
    )
    rows.append(
        (
            "Transcript",
            f"`{rel}` in the transcript archive, outside the repository; SHA-256 "
            f"`{sha256_file(transcript)}`",
        )
    )
    return md_table(rows)


# ---------------------------------------------------------------------------
# Commands
# ---------------------------------------------------------------------------


def cmd_resolve(args: argparse.Namespace) -> int:
    episode = resolve(args.url)
    episode["notes"] = html_to_text(episode.pop("notes_html", "") or "")
    print(json.dumps(episode, indent=2))
    return 0


def cmd_prepare(args: argparse.Namespace) -> int:
    print(f"Resolving {args.url}")
    episode = resolve(args.url)
    key = episode_key(episode)
    work = Path(args.work_root).expanduser() / key
    work.mkdir(parents=True, exist_ok=True)
    print(
        f"  {episode.get('show')}: {episode.get('title')} ({episode.get('released')})"
    )
    print(f"  work directory {work}")

    ext = AUDIO_EXT_RE.search(urlparse(episode["enclosure_url"]).path)
    source = work / f"source.{ext.group(1).lower() if ext else 'audio'}"
    if source.exists() and not args.force:
        audio_sha = sha256_file(source)
    else:
        print("Downloading audio …", flush=True)
        audio_sha = download(episode["enclosure_url"], source)
    wav = work / "audio.wav"
    if not wav.exists() or args.force:
        to_wav(source, wav)
    episode["audio"] = {"sha256": audio_sha, "file": source.name, **probe(source)}

    print("Transcribing")
    runs = [
        run_pass(spec, work, episode, audio_sha, args.language, args.force)
        for spec in (args.pass_a, args.pass_b)
    ]
    if runs[0]["engine"] == runs[1]["engine"]:
        print("warning: both passes use the same engine; their errors will correlate")
    a, b = load_pass(work, runs[0]), load_pass(work, runs[1])
    ops = align([norm(w["t"]) for w in a], [norm(w["t"]) for w in b])
    stats = pair_stats(ops, len(a), len(b))
    state = {"key": key, "episode": episode, "runs": runs, "stats": stats}
    (work / "state.json").write_text(json.dumps(state, indent=2) + "\n")
    sheet = write_worksheet(work, episode, runs, a, b, ops, stats)
    print(
        f"Agreement {stats['agreement']:.1%}, {stats['divergent_spans']} divergent spans"
    )
    print(f"Worksheet: {sheet}")
    print(f"Next: write {work / 'reconciled.md'}, then `check` and `publish`.")
    return 0


def cmd_check(args: argparse.Namespace) -> int:
    _, _, result = run_check(Path(args.work_dir).expanduser())
    print(
        f"{result['words']} words in {result['paragraphs']} paragraphs; "
        f"{result['supported_by_a']:.1%} heard by A, {result['supported_by_b']:.1%} by B"
    )
    for u in result["unsupported"]:
        print(f"  heard by neither  {u['paragraph_at']}  {u['words']}")
    for o in result["omitted"]:
        print(f"  left out          {o['at']}  {o['words']}")
    for problem in result["problems"]:
        print(f"FAIL: {problem}")
    print("ok" if result["ok"] else "check failed")
    return 0 if result["ok"] else 1


def cmd_publish(args: argparse.Namespace) -> int:
    work = Path(args.work_dir).expanduser()
    state, rec, result = run_check(work)
    if not result["ok"] and not args.force:
        for problem in result["problems"]:
            print(f"FAIL: {problem}")
        print("check failed; fix reconciled.md or pass --force")
        return 1
    root = args.out_dir or os.environ.get(ARCHIVE_ENV)
    if not root:
        raise SystemExit(f"no archive directory: pass --out-dir or set {ARCHIVE_ENV}")
    dest = archive_dir(Path(root).expanduser(), state["episode"])
    if in_git_checkout(dest):
        raise SystemExit(
            f"{dest} is inside a Git checkout; transcripts are kept out of the repository"
        )
    dest.mkdir(parents=True, exist_ok=True)
    archive_work(work, dest)
    revision = tool_revision()
    transcript = dest / "transcript.md"
    transcript.write_text(render_transcript(state, rec, result, revision))
    digest = sha256_file(transcript)
    published = {
        "tool": revision,
        "transcript_sha256": digest,
        "published": datetime.now(timezone.utc).isoformat(timespec="seconds"),
    }
    (dest / "published.json").write_text(json.dumps(published, indent=2) + "\n")
    print(f"Wrote {transcript}")
    print(f"  sha256 {digest}")
    print(f"  cite with: podcast-transcribe.py cite {dest} --doc <document>")
    return 0


def cmd_cite(args: argparse.Namespace) -> int:
    dest = Path(args.archive_dir).expanduser()
    if not (dest / "transcript.md").exists():
        raise SystemExit(
            f"{dest} holds no transcript.md; pass an episode's archive directory"
        )
    print("\n".join(cite_table(dest, Path(args.doc) if args.doc else None)))
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    sub = parser.add_subparsers(dest="command", required=True)
    p = sub.add_parser("resolve", help="print the episode a link resolves to")
    p.add_argument("url")
    p.set_defaults(func=cmd_resolve)
    p = sub.add_parser(
        "prepare", help="download, transcribe twice, align, write the worksheet"
    )
    p.add_argument("url")
    p.add_argument("--work-root", default=str(DEFAULT_WORK_ROOT))
    p.add_argument("--pass-a", default=DEFAULT_PASS_A, help=f"default {DEFAULT_PASS_A}")
    p.add_argument("--pass-b", default=DEFAULT_PASS_B, help=f"default {DEFAULT_PASS_B}")
    p.add_argument("--language", default="en")
    p.add_argument("--force", action="store_true", help="download and transcribe again")
    p.set_defaults(func=cmd_prepare)
    p = sub.add_parser("check", help="verify reconciled.md against both passes")
    p.add_argument("work_dir")
    p.set_defaults(func=cmd_check)
    p = sub.add_parser(
        "publish", help="check, then archive the transcript outside the repository"
    )
    p.add_argument("work_dir")
    p.add_argument(
        "--out-dir",
        help=f"archive root, outside any checkout (default ${ARCHIVE_ENV}); "
        "writes <show>/<released>-<title>/transcript.md",
    )
    p.add_argument(
        "--force", action="store_true", help="publish even if the check fails"
    )
    p.set_defaults(func=cmd_publish)
    p = sub.add_parser(
        "cite", help="print the provenance table a document pastes beside its summary"
    )
    p.add_argument("archive_dir", help="the episode's directory in the archive")
    p.add_argument("--doc", help="the citing document, for a relative link to the tool")
    p.set_defaults(func=cmd_cite)
    args = parser.parse_args(argv)
    return args.func(args)


if __name__ == "__main__":
    sys.exit(main())
