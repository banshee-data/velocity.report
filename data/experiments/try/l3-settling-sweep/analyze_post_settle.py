#!/usr/bin/env python3
"""Deterministic post-settle analysis over results.csv + raw settling reports.

Why this exists: analyze_sensitivity.py reads one number per run, the first
frame at which the background grid's four convergence criteria all hold. Every
raw report also carries metrics_history, the four criteria at every frame of
the window, which nothing has used. Two questions need it:

  1. Five L3 keys (reacquisition_boost_multiplier, min_confidence_floor,
     locked_baseline_threshold, locked_baseline_multiplier,
     freeze_threshold_multiplier) give the identical settling frame at 23 of 24
     sites. They act after the grid locks, so "no effect on the settling frame"
     does not show they do nothing: does any of the four per-frame metrics move?
  2. Does a setting that settles the grid on time also keep it settled, or does
     it settle and then fall out again (the sensitivity rule cannot see that)?

Definitions (all fixed here, so the same inputs always give the same output):
  ok(frame)         coverage_rate >= min_coverage AND spread_delta_rate <=
                    max_spread_delta AND region_stability >= min_region_stability
                    AND mean_confidence >= min_confidence, thresholds read from
                    the report itself. The recorded recommended_settling_frame is
                    the frame_number of the first ok frame; that is re-derived for
                    every row and reported as self_check, and the analysis
                    refuses (exit 2) if it does not hold for every converged row.
  post-settle span  frames from the first ok frame to the end of the window.
  ok_fraction       share of the post-settle span that is ok.
  unsettle_events   ok -> not-ok transitions inside the post-settle span.
  ended_unsettled   the last frame of the window is not ok.

A row is flagged against its site's `_baseline=default` row at the same capture
ordinal when the absolute ok_fraction difference is >= OK_FRACTION_DELTA or
ended_unsettled differs. A value is post-settle-sensitive when it is flagged at
>= MIN_SITE_FRACTION of the sites where both rows converged. Rows where either
side never converged are counted, not compared: settling-frame loss is
analyze_sensitivity.py's job.

baseline_summary is the same measurement on the `_baseline=default` rows alone,
per capture ordinal, for a results.csv that holds only baselines (the long-window
runs): how many segments settled, how many stayed settled, and which ones need
attention (never converged, ok_fraction < ATTENTION_OK_FRACTION, or ended
unsettled).

Why there is no exact-equality ("trajectory inert") test: measured on the
2026-09-18 corpus, no two runs have identical metrics_history, not even runs of
the same config at the same site (0 of 24 sites identical across the values of
any key, including keys whose settling frame is identical at 23 of 24 sites).
Per-frame coverage/confidence carry run-to-run noise that the first-ok-frame
threshold crossing is robust to. The threshold-based rule above is therefore the
only comparison made; the per-metric post-settle medians are reported
descriptively and never drive a verdict.

Usage:
    python3 analyze_post_settle.py --results results.csv --out post-settle.json
"""

import argparse
import csv
import itertools
import json
import statistics
import sys
from pathlib import Path

OK_FRACTION_DELTA = 0.05
MIN_SITE_FRACTION = 0.2
ATTENTION_OK_FRACTION = 0.95
METRICS = ("coverage_rate", "spread_delta_rate", "region_stability", "mean_confidence")


def frame_ok(m, t):
    return (
        m["coverage_rate"] >= t["min_coverage"]
        and m["spread_delta_rate"] <= t["max_spread_delta"]
        and m["region_stability"] >= t["min_region_stability"]
        and m["mean_confidence"] >= t["min_confidence"]
    )


def summarise_report(report):
    """Per-run post-settle summary; converged=False when no frame was ever ok."""
    t = report["thresholds"]
    hist = report["metrics_history"]
    oks = [frame_ok(m, t) for m in hist]
    first = next((i for i, x in enumerate(oks) if x), None)
    out = {
        "n_frames": len(hist),
        "recorded_frame": report["recommended_settling_frame"],
        "first_ok_frame_number": (
            hist[first]["frame_number"] if first is not None else -1
        ),
    }
    if first is None:
        out["converged"] = False
        return out
    span = oks[first:]
    out.update(
        converged=True,
        span_frames=len(span),
        ok_fraction=sum(span) / len(span),
        unsettle_events=sum(1 for a, b in zip(span, span[1:]) if a and not b),
        ended_unsettled=not span[-1],
        longest_not_ok_run=max(
            (len(list(g)) for k, g in itertools.groupby(span) if not k),
            default=0,
        ),
        post_settle_median={
            k: statistics.median(m[k] for m in hist[first:]) for k in METRICS
        },
    )
    return out


def load_rows(results_csv):
    with open(results_csv, newline="") as f:
        return [r for r in csv.DictReader(f) if not r.get("error")]


def analyze(rows, load=None):
    load = load or (lambda p: json.loads(Path(p).read_text()))
    baselines, others = [], []
    for r in rows:
        (baselines if r["param_key"] == "_baseline" else others).append(r)

    mismatched = []
    n_converged = [0]

    def summarise(r):
        s = summarise_report(load(r["raw_report_path"]))
        if s["converged"]:
            n_converged[0] += 1
            if s["first_ok_frame_number"] != s["recorded_frame"]:
                mismatched.append(
                    {
                        "run": f"{r['site_id']} {r['param_key']}={r['param_value']}",
                        "recomputed": s["first_ok_frame_number"],
                        "recorded": s["recorded_frame"],
                    }
                )
        return s

    base = {}
    for r in baselines:
        base[(r["site_id"], r.get("capture_ordinal", "0"))] = summarise(r)

    per = {}  # (ordinal, key, value) -> list of per-site dicts
    for r in others:
        b = base.get((r["site_id"], r.get("capture_ordinal", "0")))
        if b is None:
            continue
        s = summarise(r)
        entry = {"site": r["site_id"], "converged": s["converged"]}
        if s["converged"] and b["converged"]:
            entry.update(
                d_ok_fraction=s["ok_fraction"] - b["ok_fraction"],
                d_unsettle_events=s["unsettle_events"] - b["unsettle_events"],
                ok_fraction=s["ok_fraction"],
                baseline_ok_fraction=b["ok_fraction"],
                ended_unsettled=s["ended_unsettled"],
                baseline_ended_unsettled=b["ended_unsettled"],
                d_post_settle_median={
                    k: s["post_settle_median"][k] - b["post_settle_median"][k]
                    for k in METRICS
                },
            )
            entry["flagged"] = (
                abs(entry["d_ok_fraction"]) >= OK_FRACTION_DELTA
                or entry["ended_unsettled"] != entry["baseline_ended_unsettled"]
            )
        per.setdefault(
            (r.get("capture_ordinal", "0"), r["param_key"], r["param_value"]), []
        ).append(entry)

    by_ordinal = {}
    for (ordinal, key, value), entries in sorted(per.items()):
        comparable = [e for e in entries if "flagged" in e]
        flagged = [e for e in comparable if e["flagged"]]
        n = len(comparable)
        by_ordinal.setdefault(ordinal, {}).setdefault(key, {})[value] = {
            "n_sites_run": len(entries),
            "n_sites_comparable": n,
            "n_not_converged": len(entries) - n,
            "n_flagged": len(flagged),
            "flagged_fraction": (len(flagged) / n) if n else None,
            "median_ok_fraction": (
                statistics.median(e["ok_fraction"] for e in comparable) if n else None
            ),
            "median_baseline_ok_fraction": (
                statistics.median(e["baseline_ok_fraction"] for e in comparable)
                if n
                else None
            ),
            "median_d_post_settle_median": (
                {
                    k: statistics.median(
                        e["d_post_settle_median"][k] for e in comparable
                    )
                    for k in METRICS
                }
                if n
                else None
            ),
            "flagged_sites": sorted(e["site"] for e in flagged),
            "verdict": (
                "no_comparable_sites"
                if not n
                else (
                    "post_settle_sensitive"
                    if len(flagged) / n >= MIN_SITE_FRACTION
                    else "post_settle_insensitive"
                )
            ),
        }

    keys = {}
    for ordinal, kd in by_ordinal.items():
        for key, vals in kd.items():
            k = keys.setdefault(key, {"by_ordinal": {}})
            comparable_vals = [v for v in vals.values() if v["n_sites_comparable"]]
            k["by_ordinal"][ordinal] = {
                "verdict": (
                    "post_settle_sensitive"
                    if any(
                        v["verdict"] == "post_settle_sensitive" for v in vals.values()
                    )
                    else (
                        "post_settle_insensitive"
                        if comparable_vals
                        else "no_comparable_sites"
                    )
                ),
                "sensitive_values": sorted(
                    v_
                    for v_, v in vals.items()
                    if v["verdict"] == "post_settle_sensitive"
                ),
            }

    baseline_summary = {}
    for (site, ordinal), b in sorted(base.items(), key=lambda kv: (kv[0][1], kv[0][0])):
        o = baseline_summary.setdefault(
            ordinal,
            {"n_segments": 0, "n_converged": 0, "n_stayed_settled": 0, "attention": []},
        )
        o["n_segments"] += 1
        if not b["converged"]:
            o["attention"].append(
                {"site": site, "why": "never converged", "n_frames": b["n_frames"]}
            )
            continue
        o["n_converged"] += 1
        if b["ok_fraction"] >= ATTENTION_OK_FRACTION and not b["ended_unsettled"]:
            o["n_stayed_settled"] += 1
        else:
            o["attention"].append(
                {
                    "site": site,
                    "why": (
                        "fell out of the settled state"
                        if b["ok_fraction"] < ATTENTION_OK_FRACTION
                        else "ended the window unsettled"
                    ),
                    "settling_frame": b["recorded_frame"],
                    "ok_fraction": round(b["ok_fraction"], 4),
                    "unsettle_events": b["unsettle_events"],
                    "longest_not_ok_run": b["longest_not_ok_run"],
                    "n_frames": b["n_frames"],
                }
            )
        o.setdefault("settling_frames", []).append(b["recorded_frame"])
    for o in baseline_summary.values():
        frames = o.pop("settling_frames", [])
        o["median_settling_frame"] = statistics.median(frames) if frames else None
        o["max_settling_frame"] = max(frames) if frames else None

    return {
        "definition": {
            "ok_fraction_delta": OK_FRACTION_DELTA,
            "min_site_fraction": MIN_SITE_FRACTION,
            "attention_ok_fraction": ATTENTION_OK_FRACTION,
        },
        "baseline_summary": baseline_summary,
        "self_check": {
            "converged_rows": n_converged[0],
            "first_ok_frame_matches_recorded": n_converged[0] - len(mismatched),
            "mismatches": mismatched[:20],
        },
        "n_baselines": len(base),
        "keys": keys,
        "by_ordinal": by_ordinal,
    }


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--results", required=True)
    ap.add_argument("--out", required=True)
    args = ap.parse_args()

    result = analyze(load_rows(args.results))
    Path(args.out).write_text(json.dumps(result, indent=2) + "\n")

    sc = result["self_check"]
    print(
        f"self-check: recomputed first-ok frame equals the recorded settling frame "
        f"for {sc['first_ok_frame_matches_recorded']} of {sc['converged_rows']} converged rows"
    )
    for ordinal, o in sorted(result["baseline_summary"].items()):
        print(
            f"baseline o{ordinal}: {o['n_segments']} segments, {o['n_converged']} "
            f"converged, {o['n_stayed_settled']} stayed settled; median settling "
            f"frame {o['median_settling_frame']}, max {o['max_settling_frame']}; "
            f"needing attention: {[a['site'] for a in o['attention']]}"
        )
    for key, info in sorted(result["keys"].items()):
        parts = []
        for ordinal, o in sorted(info["by_ordinal"].items()):
            parts.append(
                f"o{ordinal}={o['verdict']}"
                + (f" values={o['sensitive_values']}" if o["sensitive_values"] else "")
            )
        print(f"{key}: " + "; ".join(parts))
    return 2 if sc["mismatches"] else 0


if __name__ == "__main__":
    sys.exit(main())
