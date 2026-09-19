#!/usr/bin/env python3
"""Deterministic summary of run_gt_oat_sweep.py --label-free results: does a
direction seen on a labelled capture also appear on longer, unlabelled stretches?

This measures how far a setting moves the pipeline's output. It cannot say
whether the movement is right, because no reference exists for these captures,
so it never calls a setting better or worse and never ranks values. A track-count
increase can be recovered recall or fragmentation; only the labelled captures
(analyze_gt_oat.py) can tell those apart, and this script only checks whether
their *direction* transfers.

Per results.csv (one capture segment) the rules are the ground-truth analyzer's:
  floor_c   range of candidate_count over the warm baselines (start/middle/end)
  reliable  at least 2 usable baselines and floor_c <= max(3, 5% of the median),
            so a leaking-state or nondeterministic segment supports nothing
  T_c       max(5, ceil(15% of the median baseline candidate_count), 2*floor_c)
  class     more_tracks if candidate_count - median >= T_c, fewer_tracks if <=
            -T_c, otherwise no_visible_change
Across reliable segments each (key, value) gets a verdict: consistent_more_tracks,
consistent_fewer_tracks or consistent_no_visible_change when every reliable
segment agrees (at least two of them), single_segment when only one is reliable,
otherwise mixed. With --gt-analysis, that direction is set beside the class the
labelled captures gave the same (key, value):
  agrees         same nonzero direction as every visible labelled capture
  contradicts    opposite to a visible labelled capture
  long_only      visible on the long stretches, invisible on the labelled captures
  gt_only        visible on a labelled capture, invisible on the long stretches
  both_none      invisible on both
  gt_mixed       the labelled captures disagree with each other
  inconclusive   the long-stretch verdict is mixed or single_segment

Usage:
    python3 analyze_label_free_oat.py --out label-free-analysis.json \
        --results oat-lf-columbus-broadway-o0/results.csv \
        --results oat-lf-columbus-broadway-o3/results.csv \
        --gt-analysis gt-oat-analysis.json --gt-analysis gt-oat-analysis-l3keys.json
"""

import argparse
import csv
import json
import math
import statistics
from collections import defaultdict
from pathlib import Path

DIRECTION = {
    "consistent_more_tracks": 1,
    "consistent_fewer_tracks": -1,
    "consistent_no_visible_change": 0,
}


def classify(delta, t_c):
    if delta >= t_c:
        return "more_tracks"
    if delta <= -t_c:
        return "fewer_tracks"
    return "no_visible_change"


def analyze_segment(rows):
    rows = [r for r in rows if not r.get("error")]
    base = [r for r in rows if r["param_path"] == "_baseline"]
    if len(base) < 2:
        return {"reliable": False, "reason": f"only {len(base)} usable baseline run(s)"}
    bc = [int(r["candidate_count"]) for r in base]
    med, floor = statistics.median(bc), max(bc) - min(bc)
    out = {"baseline_candidates": bc}
    if floor > max(3, 0.05 * med):
        out.update(
            reliable=False,
            reason=f"baselines disagree (candidates {bc}): state is leaking between "
            "runs or the replay is nondeterministic",
        )
        return out
    t_c = max(5, math.ceil(0.15 * med), 2 * floor)
    base_conf = statistics.median(int(r["confirmed_count"]) for r in base)
    base_short = statistics.median(int(r["short_track_count"]) for r in base)
    keys = defaultdict(dict)
    for r in rows:
        if r["param_path"].startswith("_"):
            continue
        c = int(r["candidate_count"])
        keys[r["param_path"]][r["param_value"]] = {
            "candidates": c,
            "d_candidates": c - med,
            "d_confirmed": int(r["confirmed_count"]) - base_conf,
            "d_short_tracks": int(r["short_track_count"]) - base_short,
            "class": classify(c - med, t_c),
        }
    out.update(
        reliable=True,
        baseline_median=med,
        threshold_candidates=t_c,
        keys=dict(keys),
        error_rows=sum(1 for r in rows if r.get("error")),
    )
    return out


def verdict(classes):
    if len(classes) < 2:
        return "single_segment"
    if len(set(classes)) == 1:
        return f"consistent_{classes[0]}"
    return "mixed"


def gt_direction(d_candidates, threshold):
    """Track-count direction on a labelled capture, by the same T_c rule. The GT
    class string is not used: it can read pareto_regression while the track
    count also moved visibly."""
    return {"more_tracks": 1, "fewer_tracks": -1}.get(
        classify(d_candidates, threshold), 0
    )


def relation(long_verdict, gt_dirs_by_site):
    if long_verdict not in DIRECTION:
        return "inconclusive"
    long_dir = DIRECTION[long_verdict]
    gt_dirs = {d for d in gt_dirs_by_site.values() if d}
    if len(gt_dirs) > 1:
        return "gt_mixed"
    if not gt_dirs:
        return "both_none" if long_dir == 0 else "long_only"
    if long_dir == 0:
        return "gt_only"
    return "agrees" if gt_dirs == {long_dir} else "contradicts"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--results", action="append", required=True, dest="paths")
    ap.add_argument("--out", required=True)
    ap.add_argument(
        "--gt-analysis",
        action="append",
        default=[],
        help="an analyze_gt_oat.py output; repeatable. Their sites are pooled, "
        "and a (key, value) is compared against every labelled site that ran it",
    )
    args = ap.parse_args()

    segments = {}
    for p in args.paths:
        with open(p, newline="") as f:
            segments[Path(p).parent.name] = analyze_segment(list(csv.DictReader(f)))
    reliable = {k: v for k, v in segments.items() if v.get("reliable")}

    gt = None
    if args.gt_analysis:
        gt = {"sites": {}}
        for path in args.gt_analysis:
            for site, info in json.loads(Path(path).read_text())["sites"].items():
                assert site not in gt["sites"], f"labelled site name reused: {site}"
                gt["sites"][site] = info

    cross = {}
    for path in sorted({k for v in reliable.values() for k in v["keys"]}):
        values = {v for seg in reliable.values() for v in seg["keys"].get(path, {})}
        cross[path] = {}
        for value in sorted(values, key=float):
            per_seg = {
                name: seg["keys"][path][value]["class"]
                for name, seg in reliable.items()
                if value in seg["keys"].get(path, {})
            }
            entry = {
                "classes": per_seg,
                "d_candidates": {
                    n: reliable[n]["keys"][path][value]["d_candidates"] for n in per_seg
                },
                "verdict": verdict(list(per_seg.values())),
            }
            if gt:
                gt_cells = {
                    site: (
                        info["keys"][path][value],
                        info["thresholds"]["candidates"],
                    )
                    for site, info in gt["sites"].items()
                    if info.get("reliable")
                    and value in info.get("keys", {}).get(path, {})
                }
                gt_dirs = {
                    site: gt_direction(cell["d_candidates"], t)
                    for site, (cell, t) in gt_cells.items()
                }
                entry["labelled_classes"] = {
                    site: cell["class"] for site, (cell, _) in gt_cells.items()
                }
                entry["labelled_track_direction"] = gt_dirs
                entry["relation_to_labelled"] = (
                    relation(entry["verdict"], gt_dirs)
                    if gt_dirs
                    else "no_labelled_value"
                )
            cross[path][value] = entry

    result = {
        "measures": "output movement only (candidate track count); not correctness",
        "n_segments": len(segments),
        "n_reliable_segments": len(reliable),
        "segments": segments,
        "cross_segment": cross,
    }
    Path(args.out).write_text(json.dumps(result, indent=2) + "\n")

    print(f"{len(reliable)} of {len(segments)} segments reliable")
    for name, seg in segments.items():
        if not seg.get("reliable"):
            print(f"  {name}: UNRELIABLE -- {seg['reason']}")
    for path, vals in cross.items():
        for value, e in vals.items():
            rel = f" | vs labelled: {e['relation_to_labelled']}" if gt else ""
            print(f"{path}={value}: {e['verdict']}{rel}")


if __name__ == "__main__":
    main()
