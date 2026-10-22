"""Per-lane counting-line headways from a SUMO instantInductionLoop output.

Usage: python3 headways.py out/det.<model>.xml

Headway is the time between successive vehicles' fronts crossing the same lane's line. The first
300 s are a warm-up and are left out.
"""

import math
import statistics
import sys
import xml.etree.ElementTree as ET
from collections import defaultdict

WARMUP_S = 300.0


def nearest_rank(values, p):
    """The repository's percentile rule (D-18, internal/stats.NearestRank): index ceil(p*n) - 1."""
    ordered = sorted(values)
    rank = p * len(ordered)
    if abs(rank - round(rank)) <= 1e-9 * max(1, round(rank)):
        rank = round(rank)
    return ordered[max(math.ceil(rank) - 1, 0)]


def main(path):
    lanes = defaultdict(list)
    for e in ET.parse(path).getroot():
        if e.get("state") == "enter" and float(e.get("time")) > WARMUP_S:
            lanes[e.get("id")].append(float(e.get("time")))
    print(path)
    for lane, times in sorted(lanes.items()):
        times.sort()
        h = [b - a for a, b in zip(times, times[1:])]
        if len(h) < 2:
            print(f"  {lane}: n={len(times)} (too few to summarise)")
            continue
        flow = len(times) / (times[-1] - times[0]) * 3600
        cv = statistics.pstdev(h) / statistics.mean(h)
        short = sum(x < 2.5 for x in h) / len(h)
        print(
            f"  {lane}: n={len(times)} flow={flow:.0f}/h"
            f" headway p15={nearest_rank(h, 0.15):.1f}s p50={nearest_rank(h, 0.5):.1f}s"
            f" p85={nearest_rank(h, 0.85):.1f}s cv={cv:.2f} share<2.5s={short:.0%}"
        )


if __name__ == "__main__":
    main(sys.argv[1])
