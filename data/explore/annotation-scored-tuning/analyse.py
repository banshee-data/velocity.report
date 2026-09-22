#!/usr/bin/env python3
"""Read a sweep's results and say what the annotations imply about the tuning.

Ranking by one number hides the trade this sweep is really about: every config
buys recall with precision. So this prints the best under several readings —
F1, and the best recall available at a floor on precision — and the Pareto
front, which is the set no other config beats on both at once.
"""

import json
import sys
import collections
import statistics

path = sys.argv[1] if len(sys.argv) > 1 else "results.jsonl"
rows, errors = [], collections.Counter()
for line in open(path):
    line = line.strip()
    if not line:
        continue
    try:
        r = json.loads(line)
    except Exception:  # noqa: BLE001 - a torn last line
        errors["unparsable"] += 1
        continue
    if r.get("error"):
        errors[r["error"][:40]] += 1
        continue
    if "f1" in r and "params" in r:
        rows.append(r)

print(f"{len(rows)} scored runs, {sum(errors.values())} failures")
for err, n in errors.most_common(5):
    print(f"  {n:5d}  {err}")
if not rows:
    sys.exit(0)

stock = next((r for r in rows if r.get("tag") == "stock"), None)


def line(r, label=""):
    p = r["params"]
    return (
        f"{label:10s} F1={r['f1']:.3f} recall={r['recall']:.3f} prec={r['precision']:.3f} "
        f"ped={r['recall_by_class'].get('pedestrian', 0):.3f} "
        f"car={r['recall_by_class'].get('car', 0):.3f} "
        f"rev={r.get('reviewed_recall', 0):.3f} "
        f"frag={r['frag']:.2f} spur/f={r['spurious_per_frame']:.2f} | "
        f"close={p['closeness_multiplier']} safe={p['safety_margin_metres']} "
        f"nbr={p['neighbour_confirmation_count']} noise={p['noise_relative']} "
        f"bgupd={p['background_update_fraction']} post={p['post_settle_update_fraction']} "
        f"eps={p['foreground_dbscan_eps']} minpts={p['foreground_min_cluster_points']}"
    )


if stock:
    print("\n--- shipped defaults ---")
    print(line(stock, "stock"))

print("\n--- best by F1 ---")
for r in sorted(rows, key=lambda r: -r["f1"])[:10]:
    print(line(r))

print("\n--- best recall at a precision floor ---")
for floor in (0.8, 0.7, 0.6, 0.5, 0.4):
    ok = [r for r in rows if r["precision"] >= floor]
    if ok:
        print(line(max(ok, key=lambda r: r["recall"]), f"p>={floor}"))

print("\n--- best pedestrian recall at a precision floor ---")
for floor in (0.7, 0.6, 0.5):
    ok = [r for r in rows if r["precision"] >= floor]
    if ok:
        print(
            line(
                max(ok, key=lambda r: r["recall_by_class"].get("pedestrian", 0)),
                f"p>={floor}",
            )
        )

# The Pareto front on recall and precision: nothing beats these on both.
front = []
for r in sorted(rows, key=lambda r: (-r["recall"], -r["precision"])):
    if not front or r["precision"] > front[-1]["precision"]:
        front.append(r)
print(f"\n--- Pareto front (recall vs precision), {len(front)} configs ---")
step = max(1, len(front) // 12)
for r in front[::step]:
    print(line(r))

print("\n--- what each axis does (median F1 and recall by value) ---")
axes = collections.defaultdict(lambda: collections.defaultdict(list))
for r in rows:
    for k, v in r["params"].items():
        axes[k][v].append(r)
for k in sorted(axes):
    parts = []
    for v in sorted(axes[k], key=lambda x: float(x)):
        rs = axes[k][v]
        parts.append(
            f"{v}: F1 {statistics.median(x['f1'] for x in rs):.3f} "
            f"rec {statistics.median(x['recall'] for x in rs):.3f} (n={len(rs)})"
        )
    print(f"  {k}\n    " + "\n    ".join(parts))
