#!/usr/bin/env python3
"""Deterministic summary of run_nis_sweep.py results: which L5 noise settings
move the filter toward or away from statistical consistency, per speed band.

A consistent filter with a two-dimensional measurement has mean NIS 2 and 5% of
samples over the 95% chi-squared bound. This script never minimises NIS and
never ranks by a composite: it measures each config's distance from those two
targets and compares it with the site's baseline. The deviations are

  dev_mean = |ln(mean_nis / 2)|      symmetric: 1 and 4 are equally wrong
  dev_exc  = |exceedance - 0.05|

Per band (only bands with count >= MIN_COUNT), against the baseline config:
  toward     dev_mean falls by >= MEAN_FLOOR and dev_exc does not rise by more
             than EXC_TOL, or dev_exc falls by >= EXC_FLOOR and dev_mean does
             not rise by more than MEAN_TOL
  away       the mirror image
  mixed      one deviation improves past its floor while the other worsens
  no_change  otherwise
Per site, a config is more_consistent when every moving band (speed floor >= 2)
is toward or no_change with at least one toward, the slow band (floor 0) is not
away, and no moving band's association rate falls by more than ASSOC_TOL:
NIS is accepted-only, so a setting can look better by rejecting the
associations that were hard (censoring_changed). less_consistent mirrors it;
anything else is trade_off. Across sites: consistent_improvement when
more_consistent at at least half the sites and less_consistent at none;
consistent_regression mirrors it; otherwise mixed or no_change.

Two caveats belong next to every reading. NIS also rises with motion-model
mismatch, so a manoeuvring vehicle inflates it without R being at fault. And
K9 predicts that no single scalar measurement_noise can be consistent in both
the slow and the moving bands at once; if every config that helps one band
hurts the other, that is the expected result, and the evidence Phase 3's
anisotropic uncertainty needs.

Usage:
    python3 analyze_nis_sweep.py --out nis-analysis.json \
        --results columbus-broadway/results.csv --results marina-webster-beach/results.csv
"""

import argparse
import csv
import json
import math
from collections import defaultdict
from pathlib import Path

MIN_COUNT = 500
MEAN_FLOOR = 0.10
MEAN_TOL = 0.02
EXC_FLOOR = 0.01
EXC_TOL = 0.005
ASSOC_TOL = 0.05
BASELINE = "baseline"


def deviations(mean_nis, exceedance):
    return abs(math.log(mean_nis / 2.0)) if mean_nis > 0 else float("inf"), abs(
        exceedance - 0.05
    )


def band_class(dm, de, bdm, bde):
    mean_better, mean_worse = dm <= bdm - MEAN_FLOOR, dm >= bdm + MEAN_FLOOR
    exc_better, exc_worse = de <= bde - EXC_FLOOR, de >= bde + EXC_FLOOR
    toward = (mean_better and de <= bde + EXC_TOL) or (
        exc_better and dm <= bdm + MEAN_TOL
    )
    away = (mean_worse and de >= bde - EXC_TOL) or (exc_worse and dm >= bdm - MEAN_TOL)
    if toward and not away:
        return "toward"
    if away and not toward:
        return "away"
    if (mean_better or exc_better) and (mean_worse or exc_worse):
        return "mixed"
    return "no_change"


def load(path):
    rows = [r for r in csv.DictReader(open(path, newline="")) if not r.get("error")]
    errors = sum(1 for r in csv.DictReader(open(path, newline="")) if r.get("error"))
    by = defaultdict(dict)
    for r in rows:
        by[r["config"]][int(float(r["speed_floor_mps"]))] = {
            "count": int(r["count"]),
            "mean_nis": float(r["mean_nis"]),
            "exceedance": float(r["nis_exceedance_ratio"]),
            "assoc_rate": float(r["assoc_rate"]) if r["assoc_rate"] else None,
            "longitudinal_rms": (
                float(r["longitudinal_rms_metres"])
                if r["longitudinal_rms_metres"]
                else None
            ),
            "lateral_rms": (
                float(r["lateral_rms_metres"]) if r["lateral_rms_metres"] else None
            ),
            "overrides": r["overrides"],
        }
    return by, errors


def analyze_site(by):
    if BASELINE not in by:
        return {"reliable": False, "reason": "no baseline config"}
    base = by[BASELINE]
    out = {"reliable": True, "baseline": base, "configs": {}}
    for name, bands in by.items():
        if name == BASELINE:
            continue
        per_band = {}
        for floor, b in sorted(bands.items()):
            if (
                floor not in base
                or b["count"] < MIN_COUNT
                or base[floor]["count"] < MIN_COUNT
            ):
                continue
            dm, de = deviations(b["mean_nis"], b["exceedance"])
            bdm, bde = deviations(base[floor]["mean_nis"], base[floor]["exceedance"])
            assoc_drop = (
                (base[floor]["assoc_rate"] - b["assoc_rate"])
                if b["assoc_rate"] is not None and base[floor]["assoc_rate"] is not None
                else 0.0
            )
            per_band[floor] = {
                "mean_nis": b["mean_nis"],
                "exceedance": b["exceedance"],
                "baseline_mean_nis": base[floor]["mean_nis"],
                "baseline_exceedance": base[floor]["exceedance"],
                "dev_mean": round(dm, 4),
                "dev_exc": round(de, 4),
                "baseline_dev_mean": round(bdm, 4),
                "baseline_dev_exc": round(bde, 4),
                "assoc_rate_drop": round(assoc_drop, 4),
                "class": band_class(dm, de, bdm, bde),
            }
        moving = {f: v for f, v in per_band.items() if f >= 2}
        slow = per_band.get(0)
        classes = {v["class"] for v in moving.values()}
        censoring = any(v["assoc_rate_drop"] > ASSOC_TOL for v in moving.values())
        if not moving:
            verdict = "no_moving_band"
        elif censoring:
            verdict = "censoring_changed"
        elif (
            classes <= {"toward", "no_change"}
            and "toward" in classes
            and (slow is None or slow["class"] != "away")
        ):
            verdict = "more_consistent"
        elif (
            classes <= {"away", "no_change"}
            and "away" in classes
            and (slow is None or slow["class"] != "toward")
        ):
            verdict = "less_consistent"
        elif classes == {"no_change"}:
            verdict = "no_change"
        else:
            verdict = "trade_off"
        out["configs"][name] = {
            "overrides": next(iter(bands.values()))["overrides"],
            "bands": per_band,
            "verdict": verdict,
        }
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--results", action="append", required=True, dest="paths")
    ap.add_argument("--out", required=True)
    args = ap.parse_args()

    sites, errors = {}, {}
    for p in args.paths:
        by, n_err = load(p)
        name = Path(p).parent.name
        sites[name] = analyze_site(by)
        errors[name] = n_err
    reliable = {k: v for k, v in sites.items() if v.get("reliable")}

    cross = {}
    for cfg in sorted({c for v in reliable.values() for c in v["configs"]}):
        verdicts = {
            s: v["configs"][cfg]["verdict"]
            for s, v in reliable.items()
            if cfg in v["configs"]
        }
        n = len(verdicts)
        more = sum(1 for x in verdicts.values() if x == "more_consistent")
        less = sum(1 for x in verdicts.values() if x == "less_consistent")
        if n and more >= math.ceil(n / 2) and less == 0:
            overall = "consistent_improvement"
        elif n and less >= math.ceil(n / 2) and more == 0:
            overall = "consistent_regression"
        elif (
            more == 0
            and less == 0
            and all(x in ("no_change", "no_moving_band") for x in verdicts.values())
        ):
            overall = "no_change"
        else:
            overall = "mixed"
        cross[cfg] = {"per_site": verdicts, "overall": overall}

    result = {
        "rule": {
            "min_count": MIN_COUNT,
            "mean_floor_log_ratio": MEAN_FLOOR,
            "mean_tol": MEAN_TOL,
            "exc_floor": EXC_FLOOR,
            "exc_tol": EXC_TOL,
            "assoc_tol": ASSOC_TOL,
            "targets": {"mean_nis": 2.0, "exceedance": 0.05},
        },
        "caveats": [
            "NIS is accepted-only (censored by association) and rises with motion-model mismatch.",
            "K9: one scalar measurement_noise is not expected to be consistent in both slow and moving bands.",
        ],
        "n_sites": len(sites),
        "n_reliable_sites": len(reliable),
        "error_rows": errors,
        "sites": sites,
        "cross_site": cross,
    }
    Path(args.out).write_text(json.dumps(result, indent=2) + "\n")

    for s, v in sites.items():
        if not v.get("reliable"):
            print(f"{s}: UNRELIABLE -- {v['reason']}")
            continue
        b = v["baseline"]
        print(
            f"{s}: baseline "
            + " ".join(
                f"b{f}:nis={x['mean_nis']:.2f},exc={x['exceedance']:.3f},n={x['count']}"
                for f, x in sorted(b.items())
            )
        )
    for cfg, c in cross.items():
        print(f"{cfg}: {c['overall']} -- {c['per_site']}")


if __name__ == "__main__":
    main()
