#!/usr/bin/env python3
"""Score a run's cluster dump against the annotated ground truth.

A cluster matches a labelled object in the same frame when its centroid is
inside a gate set by the object's own footprint: a bus tolerates more centroid
drift than a pedestrian because a partial view of it moves the centroid more.

Reported per run:
  recall        labelled road-user object-frames a cluster was found for
  frag          clusters per matched road-user object-frame (1.0 = one each)
  merge         road-user object-frames sharing their cluster with another
  noise_hits    clusters landing on something the operator called noise/ground
  spurious      clusters landing on nothing labelled at all
Precision-like measures are per frame so runs of equal length compare directly.
"""

import json
import sys
import math
import collections

gt = json.load(open(sys.argv[1]))
dump = sys.argv[2]
run_id = sys.argv[3] if len(sys.argv) > 3 else ""


def gate(o):
    # 1 m of slack plus half the object's own horizontal diagonal.
    return 1.0 + 0.5 * math.hypot(o["lx"], o["ly"])


road_by_frame, other_by_frame = {}, {}
for sid, recs in gt["frames"].items():
    f = int(sid)
    road = [
        r
        for r in recs
        if r["class"]
        in ("car", "van", "truck", "bus", "motorcycle", "pedestrian", "cyclist")
    ]
    other = [r for r in recs if r not in road]
    if road:
        road_by_frame[f] = road
    if other:
        other_by_frame[f] = other

clusters = collections.defaultdict(list)
for line in open(dump):
    if line.strip():
        r = json.loads(line)
        clusters[r["f"]].append(r)

total_road = sum(len(v) for v in road_by_frame.values())
matched = 0
matched_clusters = 0
merged = 0
noise_hits = 0
spurious = 0
total_clusters = 0
by_class = collections.Counter()
class_total = collections.Counter()
matched_range = []
reviewed_total = reviewed_matched = 0

for f in range(gt["sample_count"]):
    cs = clusters.get(f, [])
    total_clusters += len(cs)
    road = road_by_frame.get(f, [])
    other = other_by_frame.get(f, [])
    for o in road:
        class_total[o["class"]] += 1
        if o["mask_status"] == "reviewed":
            reviewed_total += 1

    # Which labelled objects each cluster could be. A cluster claimed by more
    # than one road user is a merge for each of them.
    claims = collections.defaultdict(list)
    unclaimed = []
    for ci, c in enumerate(cs):
        hit_road, hit_other = [], False
        for oi, o in enumerate(road):
            if math.dist((c["x"], c["y"]), (o["x"], o["y"])) <= gate(o):
                hit_road.append(oi)
        if hit_road:
            for oi in hit_road:
                claims[oi].append(ci)
            if len(hit_road) > 1:
                merged += len(hit_road)
            continue
        for o in other:
            if math.dist((c["x"], c["y"]), (o["x"], o["y"])) <= gate(o):
                hit_other = True
                break
        if hit_other:
            noise_hits += 1
        else:
            unclaimed.append(ci)
    spurious += len(unclaimed)
    for oi, cis in claims.items():
        matched += 1
        matched_clusters += len(cis)
        by_class[road[oi]["class"]] += 1
        matched_range.append(road[oi]["range"])
        if road[oi]["mask_status"] == "reviewed":
            reviewed_matched += 1

frames = gt["sample_count"]
recall = matched / total_road if total_road else 0.0
out = {
    "run": run_id,
    "recall": round(recall, 4),
    "matched": matched,
    "road_object_frames": total_road,
    "frag": round(matched_clusters / matched, 3) if matched else 0.0,
    "merge_rate": round(merged / matched, 4) if matched else 0.0,
    "noise_hits_per_frame": round(noise_hits / frames, 3),
    "spurious_per_frame": round(spurious / frames, 3),
    "clusters_per_frame": round(total_clusters / frames, 3),
    "total_clusters": total_clusters,
    "recall_by_class": {
        c: round(by_class[c] / class_total[c], 4) for c in sorted(class_total)
    },
    "matched_median_range": (
        round(sorted(matched_range)[len(matched_range) // 2], 1)
        if matched_range
        else 0.0
    ),
}
# Precision counts a cluster as right when it landed on a road user, so the
# extra clusters of a fragmented object count as right and its fragmentation is
# reported separately. F1 then balances finding things against inventing them
# without an arbitrary weight; the weighted scores are kept alongside because
# the true cost of a spurious cluster is a judgement, not a measurement.
precision = matched_clusters / total_clusters if total_clusters else 0.0
out["precision"] = round(precision, 4)
out["f1"] = (
    round(2 * precision * recall / (precision + recall), 4)
    if precision + recall
    else 0.0
)
out["reviewed_recall"] = (
    round(reviewed_matched / reviewed_total, 4) if reviewed_total else 0.0
)
out["reviewed_total"] = reviewed_total
for w in (0.05, 0.1, 0.2, 0.33):
    out[f"score_{w}"] = round(
        recall - w * (noise_hits + spurious) / max(total_road, 1), 4
    )
out["score"] = out["score_0.33"]
print(json.dumps(out))
