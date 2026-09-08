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


def load(paths):
    out = []
    for path in paths:
        with open(path) as fh:
            doc = json.load(fh)
        source = os.path.basename(doc.get("input_file", ""))
        for segment in doc.get("segments", []):
            out.append(
                {
                    "type": segment["type"],
                    "start": parse(segment["start_time"]),
                    "end": parse(segment["end_time"]),
                    "start_secs": segment["start_secs"],
                    "end_secs": segment["end_secs"],
                    "file": source,
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


# 9/1: analysed here as one stream, so its segments already span the day.
continuous = sorted(glob.glob(os.path.join(CONTINUOUS, "*", "*segments.json")))
day_one = stitch(load(continuous), BRIDGE_SECONDS) if continuous else []

# 9/2 and 9/3: the archive's per-file analysis, stitched back together.
per_file = [
    p
    for p in sorted(glob.glob(os.path.join(PER_FILE, "*", "segments.json")))
    if "_202609010" not in p and "_202609011" not in p
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
            "captures": sorted({p["file"] for p in site["parts"] if p["file"]}),
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
