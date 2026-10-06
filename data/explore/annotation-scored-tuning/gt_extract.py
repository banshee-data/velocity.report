#!/usr/bin/env python3
"""Turn an annotation pack into per-frame ground truth a sweep can be scored on.

Each labelled object-frame becomes a centroid, an axis-aligned extent and a
point count, taken from the pack's own returns. Objects are split into the road
users a run is supposed to find and the noise it is supposed to not find.
"""

import json
import struct
import sys
import collections
import math

pack = sys.argv[1]
out = sys.argv[2]

manifest = json.load(open(f"{pack}/manifest.json"))
samples = json.load(open(f"{pack}/samples.json"))
ann = json.load(open(f"{pack}/annotations.json"))
raw = open(f"{pack}/points.bin", "rb").read()

assert (
    ann["pack_digest"] == manifest["pack_digest"]
), "annotations belong to another pack"


# Sample block: n X, then n Y, then n Z as little-endian float32, then n
# intensity bytes and n classification bytes.
def block(s):
    n, off = s["point_count"], s["byte_offset"]
    xs = struct.unpack_from(f"<{n}f", raw, off)
    ys = struct.unpack_from(f"<{n}f", raw, off + 4 * n)
    zs = struct.unpack_from(f"<{n}f", raw, off + 8 * n)
    return xs, ys, zs


by_id = {s["sample_id"]: s for s in samples}
objects = {o["object_id"]: o for o in ann["objects"]}

frames = collections.defaultdict(list)
per_object = collections.defaultdict(list)

for m in ann["masks"]:
    idx = m.get("point_indices") or []
    if not idx:
        continue
    sid = m["sample_id"]
    s = by_id[sid]
    xs, ys, zs = block(s)
    n = s["point_count"]
    idx = [i for i in idx if 0 <= i < n]
    if not idx:
        continue
    px = [xs[i] for i in idx]
    py = [ys[i] for i in idx]
    pz = [zs[i] for i in idx]
    obj = objects.get(m["object_id"], {})
    rec = {
        "object_id": m["object_id"],
        "class": obj.get("class", "?"),
        "object_status": obj.get("status", "?"),
        "mask_status": m.get("status", "?"),
        "algorithm": (m.get("provenance") or {}).get("algorithm", ""),
        "n": len(idx),
        "x": sum(px) / len(px),
        "y": sum(py) / len(py),
        "z": sum(pz) / len(pz),
        "lx": max(px) - min(px),
        "ly": max(py) - min(py),
        "lz": max(pz) - min(pz),
    }
    rec["range"] = math.hypot(rec["x"], rec["y"])
    frames[sid].append(rec)
    per_object[m["object_id"]].append((sid, rec))

# An object a run is meant to find is one that moved like a road user: the
# labelling calls the rest noise or ground, and a building is meant to be
# background by the time clustering sees it.
ROAD = {"car", "van", "truck", "bus", "motorcycle", "pedestrian", "cyclist"}
summary = []
for oid, seq in sorted(per_object.items()):
    seq.sort()
    obj = objects.get(oid, {})
    cls = obj.get("class", "?")
    pts = [(r["x"], r["y"]) for _, r in seq]
    travel = max(math.dist(a, b) for a in pts for b in pts) if len(pts) > 1 else 0.0
    summary.append(
        {
            "object_id": oid,
            "class": cls,
            "status": obj.get("status", "?"),
            "frames": len(seq),
            "first": seq[0][0],
            "last": seq[-1][0],
            "travel_m": travel,
            "median_points": sorted(r["n"] for _, r in seq)[len(seq) // 2],
            "median_range": sorted(r["range"] for _, r in seq)[len(seq) // 2],
            "road_user": cls in ROAD,
        }
    )

doc = {
    "pack_digest": manifest["pack_digest"],
    "revision": ann["revision"],
    "sample_count": manifest["sample_count"],
    "height_band": manifest["source"]["height_band"],
    "frame_timestamps": {str(s["sample_id"]): s["timestamp_ns"] for s in samples},
    "objects": summary,
    "frames": {str(k): v for k, v in sorted(frames.items())},
}
json.dump(doc, open(out, "w"))

road = [o for o in summary if o["road_user"]]
other = [o for o in summary if not o["road_user"]]
print(f"objects {len(summary)}  road users {len(road)}  other {len(other)}")
print("classes:", dict(collections.Counter(o["class"] for o in summary)))
print("object status:", dict(collections.Counter(o["status"] for o in summary)))
print(
    "mask status:",
    dict(collections.Counter(r["mask_status"] for fs in frames.values() for r in fs)),
)
print(
    f"labelled object-frames: road {sum(o['frames'] for o in road)}"
    f"  other {sum(o['frames'] for o in other)}"
)
print(f"frames with any label: {len(frames)}")
