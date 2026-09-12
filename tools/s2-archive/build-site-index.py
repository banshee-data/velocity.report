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
JOINS = os.path.join(HERE, "site-joins.json")
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


# A motion segment starting within this many seconds of its capture's first
# packet is the background model settling, not the platform moving.
SETTLING_TOLERANCE = 1.0

# The largest raw gap a settling discount may be applied to. Beyond this the
# gap is a drive, whatever the analysis restarts inside it.
DISCOUNT_CEILING = 300.0


def settling_artefact(segment):
    """Is this 'motion' just a per-file analysis restarting its background model?

    A per-file run rebuilds the model at every capture boundary and reports the
    settling as motion. Such a segment always begins at its file's first packet;
    real motion almost never does, because the platform does not start moving at
    the exact instant a capture rolls over.
    """
    return segment["type"] == "motion" and segment["start_secs"] < SETTLING_TOLERANCE


def real_gap_seconds(segments, first, second):
    """The motion between two static stretches, discounting settling artefacts.

    Bridging on the raw gap punishes a site for the analysis's own restarts. On
    9/2 the stretch at 13:20 was cut from the one at 13:37 by 241 s of "motion",
    99 s of which was the model settling at the start of capture 5. The real
    gap is 141 s, inside the bridge, and the two are one site — which is what
    the 35-minute recording of that junction shows.
    """
    total = (second["start"] - first["end"]).total_seconds()
    # A drive between junctions crosses several capture boundaries and would
    # collect a discount at each, so discounting is only allowed to rescue a
    # gap that was nearly short enough already. Without this ceiling 9/2 fell
    # from nine sites to five: the discounts added up and merged real drives.
    if total > DISCOUNT_CEILING:
        return total
    for segment in segments:
        if segment["start"] >= first["end"] and segment["end"] <= second["start"]:
            if settling_artefact(segment):
                total -= (segment["end"] - segment["start"]).total_seconds()
    return total


def load_joins():
    """Spans the operator says are one site, whatever the classifier read.

    Only the person at the junction knows the tripod was nudged rather than
    driven away. Until the background model can tell the difference, that
    knowledge has to arrive from outside — see the nudge-tolerance plan.
    """
    if not os.path.exists(JOINS):
        return []
    with open(JOINS) as fh:
        spans = json.load(fh).get("joins", [])
    return [
        (
            datetime.fromisoformat(f"{span['day']}T{span['from']}"),
            datetime.fromisoformat(f"{span['day']}T{span['to']}"),
        )
        for span in spans
    ]


JOIN_SPANS = load_joins()


def asserted_together(first, second):
    """Do both stretches fall inside one span the operator joined?"""
    for start, end in JOIN_SPANS:
        a = first["end"].replace(tzinfo=None)
        b = second["start"].replace(tzinfo=None)
        if start <= a <= end and start <= b <= end:
            return True
    return False


def stitch(segments, bridge):
    sites, current = [], None
    for segment in segments:
        if segment["type"] != "static":
            continue
        if current and (
            asserted_together(current, segment)
            or real_gap_seconds(segments, current, segment) <= bridge
        ):
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

# Every segment from both analyses, for attributing a site's captures below.
all_segments = load(continuous) + load(per_file)

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

# A mark is claimed by one site. Keyed on the mark's id, which is unique;
# keying it on the clock made two marks on different days collide, because the
# sheet is a 12-hour clock and a run happens at the same hour most afternoons.
# 9/2's 1:15 consumed the key and 9/3's 1:15 was refused, so a site recorded
# two minutes from its mark was left unnamed and off the map. Two sites and two
# marks were lost that way.
index, used = [], set()
for number, site in enumerate(sites, 1):
    mark, gap = match(site)
    if mark and mark["id"] in used:
        mark = None
    if mark:
        used.add(mark["id"])
    minutes = (site["end"] - site["start"]).total_seconds() / 60
    # Export follows the operator-approved *site* interval, rather than only
    # classifier-static fragments. A tripod nudge can briefly look like motion
    # while the sensor remains at the same site; dropping it would make the
    # released PCAP silently disagree with this index and site-joins.json.
    export_overrides = [
        {"from": start.isoformat(), "to": end.isoformat(), "asserted_by": "operator"}
        for start, end in JOIN_SPANS
        if site["start"].replace(tzinfo=None) <= end
        and start <= site["end"].replace(tzinfo=None)
    ]
    index.append(
        {
            # Identity comes from the field mark, which is named for the place
            # and so survives a site being added, dropped or reclassified. The
            # ordinal below is display order, not identity, and must not be
            # used as a key: it shifts whenever the set changes.
            "id": (
                mark["id"]
                if mark
                else f"unrecognised-{site['start'].strftime('%m%d-%H%M')}"
            ),
            "site": f"s{number:02d}",
            "day": site["start"].strftime("%Y-%m-%d"),
            "start": site["start"].isoformat(),
            "end": site["end"].isoformat(),
            "clock": site["start"].strftime("%I:%M").lstrip("0"),
            "minutes": round(minutes, 1),
            "fragments": len(site["parts"]),
            "static_parts": [
                {
                    "start": part["start"].isoformat(),
                    "end": part["end"].isoformat(),
                    "captures": part["captures"],
                }
                for part in site["parts"]
            ],
            "static_export_policy": "all_packets_between_site_bounds",
            "static_export_overrides": export_overrides,
            # Every capture the site's span touches, not only the ones its
            # static stretches fall in. A site joined across a nudge contains
            # motion segments too, and their captures sit between the static
            # ones — omit them and the replay is handed files 1, 4 and 5 and
            # refuses them, correctly, as not one continuous stream.
            "captures": sorted(
                {
                    capture
                    for segment in all_segments
                    if segment["start"] < site["end"] and site["start"] < segment["end"]
                    for capture in segment["captures"]
                }
            ),
            "map_mark": mark["clock"] if mark else None,
            "where": mark["where"] if mark else None,
            "lat": mark["lat"] if mark else None,
            "lon": mark["lon"] if mark else None,
            "position_confidence": mark["confidence"] if mark else "no mark matched",
            # Optional operator-measured angles; see map-marks.json.
            "grid_azimuth_deg": (mark or {}).get("grid_azimuth_deg"),
            "north_azimuth_deg": (mark or {}).get("north_azimuth_deg"),
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
