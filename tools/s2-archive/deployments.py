#!/usr/bin/env python3
"""Reconstruct every deployment from capture filenames, analysed or not.

Filenames carry the capture's start time, so the drive's own timeline can be
rebuilt without reading a byte of packet data. A deployment is a run of
captures that follow each other; a gap means the sensor was moved.
"""

import glob
import json
import os
import re
from datetime import datetime, timedelta

PCAP_DIR = "/Volumes/lidar/lidar/s2"
ANALYSIS = os.path.join(PCAP_DIR, "analysis")
NAME = re.compile(r"^(?P<prefix>.+)_(?P<stamp>\d{14})_(?P<seq>\d+)\.pcap$")
# Captures are five minutes apiece; a fresh site is minutes of driving away.
DEPLOYMENT_GAP = timedelta(minutes=11)

captures = []
for path in sorted(glob.glob(os.path.join(PCAP_DIR, "*.pcap"))):
    match = NAME.match(os.path.basename(path))
    if not match:
        continue
    base = os.path.basename(path)[:-5]
    captures.append(
        {
            "base": base,
            "prefix": match["prefix"],
            "start": datetime.strptime(match["stamp"], "%Y%m%d%H%M%S"),
            "analysed": os.path.isdir(os.path.join(ANALYSIS, base)),
        }
    )
captures.sort(key=lambda c: c["start"])

deployments = []
for capture in captures:
    if deployments and capture["start"] - deployments[-1]["last"] <= DEPLOYMENT_GAP:
        deployments[-1]["captures"].append(capture)
        deployments[-1]["last"] = capture["start"]
    else:
        deployments.append(
            {"captures": [capture], "last": capture["start"], "start": capture["start"]}
        )


# Static minutes each deployment's analysis reports, where it has any.
def static_minutes(deployment):
    total = 0.0
    for capture in deployment["captures"]:
        path = os.path.join(ANALYSIS, capture["base"], "segments.json")
        if not os.path.exists(path):
            return None
        with open(path) as fh:
            for segment in json.load(fh).get("segments", []):
                if segment["type"] == "static":
                    total += segment["duration_secs"]
    return total / 60


print(f"{len(captures)} captures in {len(deployments)} deployments\n")
print(f"{'start':17s} {'clock':>7s} {'caps':>4s} {'span':>7s} {'static':>8s}  prefix")
by_day = {}
for deployment in deployments:
    span = (deployment["last"] - deployment["start"]).total_seconds() / 60 + 5
    static = static_minutes(deployment)
    day = deployment["start"].strftime("%Y-%m-%d")
    by_day.setdefault(day, []).append(deployment)
    clock = deployment["start"].strftime("%I:%M").lstrip("0")
    shown = f"{static:6.1f}m" if static is not None else "  none "
    flag = "" if static is not None else "  <- not analysed"
    print(
        f"{deployment['start']:%Y-%m-%d %H:%M}  {clock:>7s} "
        f"{len(deployment['captures']):4d} {span:6.1f}m {shown:>8s}  "
        f"{deployment['captures'][0]['prefix']}{flag}"
    )

print()
for day in sorted(by_day):
    print(f"{day}: {len(by_day[day])} deployments")
