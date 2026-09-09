#!/usr/bin/env python3
"""Build the complete site index across all three recording days.

Two sources, because the days were analysed differently. 9/1 was analysed here
as one continuous stream, which keeps the background model settled across file
boundaries and so reports each site as one segment. 9/2 and 9/3 carry the
archive's original per-file analysis, where every file restarts that model and
its settling is reported as motion — so their static periods arrive in pieces
and are stitched back together, bridging motion shorter than BRIDGE_SECONDS.

Positions come from the field map and are matched to a site by day and clock
time. A site whose mark could not be read keeps no position rather than an
invented one.
"""

import glob
import json
import os
import re
from datetime import datetime, timedelta, timezone

# The captures are stamped in local Pacific daylight time.
PACIFIC = timezone(timedelta(hours=-7))

HERE = os.path.dirname(os.path.abspath(__file__))
PER_FILE = "/Volumes/lidar/lidar/s2/analysis"
CONTINUOUS = "/Volumes/lidar/lidar/s2/analysis-continuous"
MARKS = os.path.join(HERE, "map-marks.json")
OUT = os.path.join(HERE, "site-index.json")

BRIDGE_SECONDS = 180
MIN_SITE = timedelta(minutes=10)
MARK_TOLERANCE = timedelta(minutes=25)


def parse(ts):
    return datetime.fromisoformat(
        re.sub(r"\.(\d{6})\d+", lambda m: "." + m.group(1), ts)
    )


CAPTURE_NAME = re.compile(r"_(?P<stamp>\d{14})_\d+\.pcap$")


def capture_spans(names, stream_end):
    """When did each capture in a stream run.

    A continuous analysis reports one stream, so its segments carry offsets into
    that stream and no filename. Consecutive captures abut, so each one covers
    the clock from its own start to the next one's — which is enough to say
    which captures a segment crossed.
    """
    starts = []
    for name in names:
        match = CAPTURE_NAME.search(name)
        if match:
            starts.append(
                (
                    datetime.strptime(match["stamp"], "%Y%m%d%H%M%S"),
                    os.path.basename(name),
                )
            )
    starts.sort()
    spans = []
    for index, (start, name) in enumerate(starts):
        end = starts[index + 1][0] if index + 1 < len(starts) else stream_end
        spans.append((start, end, name))
    return spans


def load(paths):
    out = []
    for path in paths:
        with open(path) as fh:
            doc = json.load(fh)
        segments = doc.get("segments", [])
        if not segments:
            continue

        # A per-file analysis names its one capture; a continuous one lists them
        # all in the config and attributes nothing, so the captures a segment
        # crossed have to be recovered from the clock.
        listed = doc.get("config", {}).get("pcap_files") or []
        spans = []
        if len(listed) > 1:
            last_end = max(parse(s["end_time"]) for s in segments).replace(tzinfo=None)
            spans = capture_spans(listed, last_end)
        source = os.path.basename(doc.get("input_file", ""))

        for segment in segments:
            start, end = parse(segment["start_time"]), parse(segment["end_time"])
            if spans:
                naive_start, naive_end = start.replace(tzinfo=None), end.replace(
                    tzinfo=None
                )
                covered = [
                    name
                    for (span_start, span_end, name) in spans
                    if span_start < naive_end and naive_start < span_end
                ]
            else:
                covered = [source] if source else []
            out.append(
                {
                    "type": segment["type"],
                    "start": start,
                    "end": end,
                    "start_secs": segment["start_secs"],
                    "end_secs": segment["end_secs"],
                    "file": source,
                    "captures": covered,
                }
            )
    return sorted(out, key=lambda s: s["start"])


def stitch(segments, bridge):
    sites, current = [], None
    for segment in segments:
        if segment["type"] != "static":
            continue
        if current and (segment["start"] - current["end"]).total_seconds() <= bridge:
            current["end"] = segment["end"]
            current["parts"].append(segment)
        else:
            current = {
                "start": segment["start"],
                "end": segment["end"],
                "parts": [segment],
            }
            sites.append(current)
    return [s for s in sites if (s["end"] - s["start"]) >= MIN_SITE]


# Which day a continuous analysis covers is read from its directory name, not
# assumed. Both trees grow as blocks are re-analysed, and a day that has been
# run continuously but is still read per file would otherwise be counted twice.
CONTINUOUS_DAYS = {"20260901"}


def day_of(path):
    """The recording day a continuous analysis directory covers."""
    return os.path.basename(os.path.dirname(path))[:8]


# 9/1: analysed here as one stream, so its segments already span the day.
continuous = [
    p
    for p in sorted(glob.glob(os.path.join(CONTINUOUS, "*", "*segments.json")))
    if day_of(p) in CONTINUOUS_DAYS
]
day_one = stitch(load(continuous), BRIDGE_SECONDS) if continuous else []

# Every other day: the archive's per-file analysis, stitched back together.
per_file = [
    p
    for p in sorted(glob.glob(os.path.join(PER_FILE, "*", "segments.json")))
    if not any(f"_{day}" in p for day in CONTINUOUS_DAYS)
]
later = stitch(load(per_file), BRIDGE_SECONDS)

sites = sorted(day_one + later, key=lambda s: s["start"])

with open(MARKS) as fh:
    marks = json.load(fh)["marks"]


def match(site):
    """The field mark closest in time on the same day, if one is close enough."""
    day = site["start"].strftime("%Y-%m-%d")
    best, best_gap = None, MARK_TOLERANCE
    for mark in marks:
        if mark["day"] != day:
            continue
        hour, minute = (int(v) for v in mark["clock"].split(":"))
        # The sheet is a 12-hour clock and the driving day runs 09:00-17:00.
        if hour < 8:
            hour += 12
        stamp = site["start"].replace(hour=hour, minute=minute, second=0, microsecond=0)
        gap = abs(stamp - site["start"])
        if gap < best_gap:
            best, best_gap = mark, gap
    return best, best_gap


# Which sites already have a published scene, matched on the recording start
# their export header carries.
REPO = os.path.normpath(os.path.join(HERE, "..", ".."))
published = {}
for header in glob.glob(
    os.path.join(REPO, "public_html/src/scenes/*/assets/part-000/header.json")
):
    with open(header) as fh:
        start_ns = int(json.load(fh)["start_ns"])
    scene = header.split(os.sep)[-4]
    published[scene] = datetime.fromtimestamp(start_ns / 1e9, tz=PACIFIC)

index, used = [], set()
for number, site in enumerate(sites, 1):
    mark, gap = match(site)
    if mark and mark["clock"] in used:
        mark = None
    if mark:
        used.add(mark["clock"])
    minutes = (site["end"] - site["start"]).total_seconds() / 60
    index.append(
        {
            "site": f"s{number:02d}",
            "day": site["start"].strftime("%Y-%m-%d"),
            "start": site["start"].isoformat(),
            "clock": site["start"].strftime("%I:%M").lstrip("0"),
            "minutes": round(minutes, 1),
            "fragments": len(site["parts"]),
            "captures": sorted({c for p in site["parts"] for c in p["captures"]}),
            "map_mark": mark["clock"] if mark else None,
            "where": mark["where"] if mark else None,
            "lat": mark["lat"] if mark else None,
            "lon": mark["lon"] if mark else None,
            "position_confidence": mark["confidence"] if mark else "no mark matched",
            "published_as": next(
                (
                    scene
                    for scene, start in published.items()
                    if abs((start - site["start"]).total_seconds()) < 300
                ),
                None,
            ),
        }
    )

with open(OUT, "w") as fh:
    json.dump(index, fh, indent=2)
    fh.write("\n")

by_day = {}
for entry in index:
    by_day.setdefault(entry["day"], []).append(entry)

print(f"{len(index)} sites\n")
print(f"{'site':5s} {'day':11s} {'clock':>6s} {'mins':>6s} {'mark':>6s}  position")
for day in sorted(by_day):
    for entry in by_day[day]:
        pos = (
            f"{entry['lat']:.4f}, {entry['lon']:.4f}  ({entry['position_confidence']})"
            if entry["lat"] is not None
            else f"—  ({entry['position_confidence']})"
        )
        print(
            f"{entry['site']:5s} {entry['day']:11s} {entry['clock']:>6s} "
            f"{entry['minutes']:6.1f} {str(entry['map_mark'] or '—'):>6s}  {pos}"
        )
    print()
for day in sorted(by_day):
    located = sum(1 for e in by_day[day] if e["lat"] is not None)
    print(f"{day}: {len(by_day[day])} sites, {located} positioned")
print(f"\nwritten: {OUT}")
