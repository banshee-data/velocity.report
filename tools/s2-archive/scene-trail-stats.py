#!/usr/bin/env python3
"""Read two scene exports of one recording and say how steady each trail is (l5est0002).

l5est0002 (data/experiments/l5est/l5est0002.md) exports one replay of a site
twice: the online posterior (`velocity scene export`) and the whole-track
smoother's estimate (`--smoothed-trail`). The two differ in each track's x, y,
vx and vy and in nothing else. This reads the two part directories, pairs their
tracks by identifier and frame, refuses the pair if anything but the trail
differs, keeps the moving vehicle tracks, and reports the spec's measures per
track and pooled per site:

1. departure from constant velocity: the velocity change over each frame triple
   divided by the mean interval, as an RMS (m/s²);
2. the exported velocity against the trail: against the central-difference
   velocity of the trail's own positions, as an RMS (m/s), and the step residual,
   each position step less the exported velocity times the interval, as an RMS
   (m per step, at the export's own interval: 0.2 s at the published stride);
3. the lateral residual from the track's own fitted straight line, p50 and p90;
4. the smoothed position less the online one, along and across the online
   track's course, median and p90;

and the peak 0.5 s course rate of the smoothed track as a share of the online
track's, read as the criteria document's screen reads it: the course is the
exported velocity's direction, differenced over pairs of samples 0.5 s apart.

    scene-trail-stats.py pair ONLINE/part-000 SMOOTHED/part-000 [--json OUT.json]
    scene-trail-stats.py set OUT/smoothed-trail [--json OUT.json]   # every site of a variant set

It reads the exports only: header.json, index.json and the gzipped NDJSON chunks
under frames/. The Markdown table goes to stdout (or --markdown), the JSON
document to --json. Pooled figures are a median across tracks and an RMS (or
percentile) over every sample of every track; the rule's figure is the pooled
RMS of measure 1, smoothed against online.
"""

import argparse
import collections
import gzip
import json
import math
import os
import statistics
import sys

TRAIL_KEYS = ("x", "y", "vx", "vy")

# The trail of every export made before header.json named one (internal/scene/types.go).
ONLINE_ESTIMATOR = "cv_kf_v1/online"

# The moving vehicle tracks, as the 2026-10-18 read of Columbus at Broadway defined them.
MIN_SAMPLES = 20
MIN_MEAN_SPEED = 3.0  # m/s, mean of the exported spd
MIN_MEDIAN_HEIGHT = 1.2  # m, median of the exported h

# The criteria document's bandwidth for peak rates: differences 0.5 s apart.
COURSE_RATE_WINDOW = 0.5

# The spec's rule and its flags.
DEPARTURE_RATIO_MAX = 0.5
COURSE_RATE_KEPT_MIN = 0.85

# Header fields two exports of one recording share.
HEADER_KEYS = (
    "site",
    "sensor_id",
    "frame_count",
    "start_ns",
    "frame_stride",
    "source_vrlog_sha256",
    "tuning_hash",
)

NAN = float("nan")


class ReaderError(Exception):
    """An export that cannot be read as one."""


# --- reading the exports


def load_json(path):
    with open(path, encoding="utf-8") as fh:
        return json.load(fh)


def read_part(part_dir):
    """One export part: its header and every frame, in index order."""
    header = load_json(os.path.join(part_dir, "header.json"))
    index = load_json(os.path.join(part_dir, "index.json"))
    chunks = sorted(index.get("chunks") or [], key=lambda chunk: chunk["c"])
    if not chunks:
        raise ReaderError(f"{part_dir}: index.json lists no chunks")
    frames = []
    for chunk in chunks:
        path = os.path.join(part_dir, "frames", f"chunk_{chunk['c']:04d}.ndjson.gz")
        count = 0
        with gzip.open(path, "rt", encoding="utf-8") as fh:
            for line in fh:
                if line.strip():
                    frames.append(json.loads(line))
                    count += 1
        if "n" in chunk and count != chunk["n"]:
            raise ReaderError(f"{path}: {count} frames, index.json says {chunk['n']}")
    return {"dir": part_dir, "header": header, "frames": frames}


def describe(part):
    """What the header says the export is."""
    header = part["header"]
    return {
        "dir": part["dir"],
        "trail_estimator": header.get("trail_estimator") or ONLINE_ESTIMATOR,
        "trail_estimator_named": bool(header.get("trail_estimator")),
        "trail_smoothing": header.get("trail_smoothing"),
        "frame_count": header.get("frame_count"),
        "frame_stride": header.get("frame_stride"),
        "build_version": header.get("build_version"),
        "generated_at": header.get("generated_at"),
        "source_vrlog_sha256": header.get("source_vrlog_sha256"),
        "tuning_hash": header.get("tuning_hash"),
    }


# --- the paired check


def pair_parts(online, smoothed):
    """The two exports are one recording and differ only in the trail.

    Frames are paired by position and must agree in f and t; tracks within a
    frame by identifier, and every field but x, y, vx, vy must agree. The
    report counts each field that differed and keeps the first examples; ok is
    False when anything differed.
    """
    differences = collections.Counter()
    examples = []

    def differ(field, example):
        differences[field] += 1
        if len(examples) < 8:
            examples.append(example)

    header_a, header_b = online["header"], smoothed["header"]
    for key in HEADER_KEYS:
        if key in header_a or key in header_b:
            if header_a.get(key) != header_b.get(key):
                differ(
                    f"header.{key}",
                    f"header {key}: {header_a.get(key)!r} against {header_b.get(key)!r}",
                )
    frames_a, frames_b = online["frames"], smoothed["frames"]
    if len(frames_a) != len(frames_b):
        differ("frames", f"{len(frames_a)} frames against {len(frames_b)}")
    track_frames = 0
    ids = set()
    for i, (a, b) in enumerate(zip(frames_a, frames_b)):
        if a.get("f") != b.get("f") or a.get("t") != b.get("t"):
            differ(
                "frame",
                f"frame {i}: f {a.get('f')} t {a.get('t')} against f {b.get('f')} t {b.get('t')}",
            )
            continue
        tracks_a = {tr["id"]: tr for tr in a.get("tr") or []}
        tracks_b = {tr["id"]: tr for tr in b.get("tr") or []}
        if len(tracks_a) != len(a.get("tr") or []) or len(tracks_b) != len(
            b.get("tr") or []
        ):
            raise ReaderError(f"frame {i}: a track identifier repeats within the frame")
        only_one = tracks_a.keys() ^ tracks_b.keys()
        if only_one:
            differ(
                "track_ids",
                f"frame {i} (f {a.get('f')}): tracks {sorted(only_one)} in one export only",
            )
        for tid in tracks_a.keys() & tracks_b.keys():
            track_frames += 1
            ids.add(tid)
            ta, tb = tracks_a[tid], tracks_b[tid]
            for key in sorted(set(ta) | set(tb)):
                if key in TRAIL_KEYS:
                    continue
                if ta.get(key) != tb.get(key):
                    differ(
                        key,
                        f"frame {i} (f {a.get('f')}) track {tid}: {key} {ta.get(key)!r} against {tb.get(key)!r}",
                    )
    return {
        "ok": not differences,
        "frames": min(len(frames_a), len(frames_b)),
        "track_frames": track_frames,
        "tracks": len(ids),
        "differences": dict(differences),
        "examples": examples,
    }


# --- the tracks


def track_series(part):
    """Each track's samples in frame order: (frame index, seconds, track)."""
    series = collections.OrderedDict()
    for i, frame in enumerate(part["frames"]):
        t = frame["t"] / 1e6
        for tr in frame.get("tr") or []:
            series.setdefault(tr["id"], []).append((i, t, tr))
    return series


def moving_vehicle(samples):
    """The spec's selection: 20 samples or more, mean spd 3 m/s or more, median h 1.2 m or more."""
    if len(samples) < MIN_SAMPLES:
        return False
    tracks = [tr for _, _, tr in samples]
    mean_speed = statistics.fmean(tr.get("spd") or 0.0 for tr in tracks)
    median_height = statistics.median(tr.get("h") or 0.0 for tr in tracks)
    return mean_speed >= MIN_MEAN_SPEED and median_height >= MIN_MEDIAN_HEIGHT


def trail_of(samples):
    """A track's trail as parallel lists: frame index, seconds and the four trail fields."""
    return {
        "i": [s[0] for s in samples],
        "t": [s[1] for s in samples],
        "x": [float(s[2]["x"]) for s in samples],
        "y": [float(s[2]["y"]) for s in samples],
        "vx": [float(s[2].get("vx") or 0.0) for s in samples],
        "vy": [float(s[2].get("vy") or 0.0) for s in samples],
    }


def consecutive(trail, k):
    """Whether samples k and k+1 are consecutive exported frames with time advancing."""
    return trail["i"][k + 1] == trail["i"][k] + 1 and trail["t"][k + 1] > trail["t"][k]


def intervals(trail):
    t = trail["t"]
    return [t[k + 1] - t[k] for k in range(len(t) - 1) if consecutive(trail, k)]


def gap_steps(trail):
    return sum(1 for k in range(len(trail["t"]) - 1) if not consecutive(trail, k))


# --- the measures


def departures(trail):
    """Measure 1: the velocity change over each frame triple over the mean interval, m/s²."""
    t, x, y = trail["t"], trail["x"], trail["y"]
    out = []
    for k in range(1, len(t) - 1):
        if not (consecutive(trail, k - 1) and consecutive(trail, k)):
            continue
        dt1, dt2 = t[k] - t[k - 1], t[k + 1] - t[k]
        v1x, v1y = (x[k] - x[k - 1]) / dt1, (y[k] - y[k - 1]) / dt1
        v2x, v2y = (x[k + 1] - x[k]) / dt2, (y[k + 1] - y[k]) / dt2
        out.append(math.hypot(v2x - v1x, v2y - v1y) / (0.5 * (dt1 + dt2)))
    return out


def velocity_residuals(trail):
    """Measure 2a: the exported velocity less the trail's central-difference velocity, m/s."""
    t, x, y, vx, vy = trail["t"], trail["x"], trail["y"], trail["vx"], trail["vy"]
    out = []
    for k in range(1, len(t) - 1):
        if not (consecutive(trail, k - 1) and consecutive(trail, k)):
            continue
        span = t[k + 1] - t[k - 1]
        fdx, fdy = (x[k + 1] - x[k - 1]) / span, (y[k + 1] - y[k - 1]) / span
        out.append(math.hypot(vx[k] - fdx, vy[k] - fdy))
    return out


def step_residuals(trail):
    """Measure 2b: each position step less the exported velocity times the interval, m."""
    t, x, y, vx, vy = trail["t"], trail["x"], trail["y"], trail["vx"], trail["vy"]
    out = []
    for k in range(len(t) - 1):
        if not consecutive(trail, k):
            continue
        dt = t[k + 1] - t[k]
        out.append(
            math.hypot(x[k + 1] - x[k] - vx[k] * dt, y[k + 1] - y[k] - vy[k] * dt)
        )
    return out


def fitted_line(xs, ys):
    """The total-least-squares line through the points, oriented along the net displacement.

    Returns (mx, my, ux, uy): a point on the line and its unit direction, which is
    the track's course for measures 3 and 4.
    """
    mx, my = statistics.fmean(xs), statistics.fmean(ys)
    sxx = sum((x - mx) ** 2 for x in xs)
    syy = sum((y - my) ** 2 for y in ys)
    sxy = sum((x - mx) * (y - my) for x, y in zip(xs, ys))
    theta = 0.5 * math.atan2(2 * sxy, sxx - syy)
    ux, uy = math.cos(theta), math.sin(theta)
    if (xs[-1] - xs[0]) * ux + (ys[-1] - ys[0]) * uy < 0:
        ux, uy = -ux, -uy
    return mx, my, ux, uy


def lateral_residuals(xs, ys, line):
    """Measure 3: each position's distance from the fitted line, m."""
    mx, my, ux, uy = line
    return [abs(-(x - mx) * uy + (y - my) * ux) for x, y in zip(xs, ys)]


def course_offsets(smoothed, online, line):
    """Measure 4: the smoothed position less the online one, along and across the course."""
    _, _, ux, uy = line
    along, across = [], []
    for xs, ys, xo, yo in zip(smoothed["x"], smoothed["y"], online["x"], online["y"]):
        dx, dy = xs - xo, ys - yo
        along.append(dx * ux + dy * uy)
        across.append(-dx * uy + dy * ux)
    return along, across


def wrap(angle):
    return (angle + math.pi) % (2 * math.pi) - math.pi


def peak_course_rate(trail, window=COURSE_RATE_WINDOW):
    """The peak course change over the window, rad/s, as the criteria document's screen reads it.

    The course is the exported velocity's direction. Each sample's partner is
    the first later sample at least the window away; an export is not sampled
    at exactly the window, so a pair spanning more than the window by over one
    typical interval is left out, which keeps a gap in the track out of the peak.
    """
    t = trail["t"]
    courses = [math.atan2(vy, vx) for vx, vy in zip(trail["vx"], trail["vy"])]
    steps = intervals(trail)
    typical = statistics.median(steps) if steps else 0.0
    peak = NAN
    j = 0
    for i in range(len(t)):
        j = max(j, i + 1)
        while j < len(t) and t[j] - t[i] < window - 1e-9:
            j += 1
        if j >= len(t):
            break
        span = t[j] - t[i]
        if span > window + typical + 1e-9:
            continue
        rate = abs(wrap(courses[j] - courses[i])) / span
        if math.isnan(peak) or rate > peak:
            peak = rate
    return peak


# --- statistics


def rms(values):
    return math.sqrt(statistics.fmean(v * v for v in values)) if values else NAN


def median(values):
    values = [v for v in values if not math.isnan(v)]
    return statistics.median(values) if values else NAN


def percentile(values, q):
    """The q-th percentile with linear interpolation between order statistics."""
    if not values:
        return NAN
    ordered = sorted(values)
    k = (len(ordered) - 1) * q / 100.0
    lo, hi = math.floor(k), math.ceil(k)
    if lo == hi:
        return ordered[int(k)]
    return ordered[lo] + (ordered[hi] - ordered[lo]) * (k - lo)


def ratio(numerator, denominator):
    if math.isnan(numerator) or math.isnan(denominator) or denominator <= 0:
        return NAN
    return numerator / denominator


# --- per track and pooled


def arm_measures(trail):
    """Measures 1 to 3 and the course-rate peak of one arm's trail: the figures and the samples."""
    line = fitted_line(trail["x"], trail["y"])
    samples = {
        "departure": departures(trail),
        "velocity": velocity_residuals(trail),
        "step": step_residuals(trail),
        "lateral": lateral_residuals(trail["x"], trail["y"], line),
    }
    figures = {
        "departure_rms": rms(samples["departure"]),
        "velocity_rms": rms(samples["velocity"]),
        "step_rms": rms(samples["step"]),
        "lateral_p50": percentile(samples["lateral"], 50),
        "lateral_p90": percentile(samples["lateral"], 90),
        "course_rate_peak": peak_course_rate(trail),
    }
    return figures, samples, line


def measure_track(tid, samples_online, samples_smoothed):
    """One moving vehicle track's row, and the samples behind it for pooling."""
    online, smoothed = trail_of(samples_online), trail_of(samples_smoothed)
    tracks = [tr for _, _, tr in samples_online]
    figures_o, raw_o, line = arm_measures(online)
    figures_s, raw_s, _ = arm_measures(smoothed)
    along, across = course_offsets(smoothed, online, line)
    raw_s["along"], raw_s["across"] = along, across
    figures_s.update(
        {
            "along_median": median(along),
            "along_p90": percentile([abs(a) for a in along], 90),
            "across_median": median(across),
            "across_p90": percentile([abs(a) for a in across], 90),
            "course_rate_share": ratio(
                figures_s["course_rate_peak"], figures_o["course_rate_peak"]
            ),
        }
    )
    classes = collections.Counter(tr.get("c") or "" for tr in tracks)
    steps = intervals(online)
    row = {
        "id": tid,
        "samples": len(samples_online),
        "class": classes.most_common(1)[0][0],
        "mean_speed": statistics.fmean(tr.get("spd") or 0.0 for tr in tracks),
        "median_height": statistics.median(tr.get("h") or 0.0 for tr in tracks),
        "interval_s": statistics.median(steps) if steps else NAN,
        "gap_steps": gap_steps(online),
        "online": figures_o,
        "smoothed": figures_s,
    }
    return row, {"online": raw_o, "smoothed": raw_s}


def pool_arm(rows, raws, arm):
    """One arm pooled over the site: an RMS or percentile over every sample, and track medians."""
    pooled = {}
    for key in ("departure", "velocity", "step"):
        pooled[f"{key}_rms"] = rms([v for raw in raws for v in raw[arm][key]])
    lateral = [v for raw in raws for v in raw[arm]["lateral"]]
    pooled["lateral_p50"] = percentile(lateral, 50)
    pooled["lateral_p90"] = percentile(lateral, 90)
    if arm == "smoothed":
        along = [v for raw in raws for v in raw[arm]["along"]]
        across = [v for raw in raws for v in raw[arm]["across"]]
        pooled["along_median"] = median(along)
        pooled["along_p90"] = percentile([abs(v) for v in along], 90)
        pooled["across_median"] = median(across)
        pooled["across_p90"] = percentile([abs(v) for v in across], 90)
    keys = rows[0][arm].keys() if rows else []
    track_median = {key: median([row[arm][key] for row in rows]) for key in keys}
    return {"pooled": pooled, "track_median": track_median}


def pool(rows, raws):
    """The site's pooled figures and the spec's rule read from them."""
    online = pool_arm(rows, raws, "online")
    smoothed = pool_arm(rows, raws, "smoothed")
    departure_ratio = ratio(
        smoothed["pooled"]["departure_rms"], online["pooled"]["departure_rms"]
    )
    course_rate_kept = smoothed["track_median"].get("course_rate_share", NAN)
    lateral_fell = smoothed["pooled"]["lateral_p90"] < online["pooled"]["lateral_p90"]
    halved = (
        not math.isnan(departure_ratio)
    ) and departure_ratio <= DEPARTURE_RATIO_MAX
    return {
        "tracks": len(rows),
        "interval_s": median([row["interval_s"] for row in rows]),
        "online": online,
        "smoothed": smoothed,
        "departure_ratio": departure_ratio,
        "course_rate_kept": course_rate_kept,
        "rule": {
            "departure_halved": halved,
            "flattening": bool(
                lateral_fell
                and not math.isnan(course_rate_kept)
                and course_rate_kept < COURSE_RATE_KEPT_MIN
            ),
        },
    }


def analyse(online, smoothed, site=None):
    """The whole document for one pair: the exports, the pairing, the tracks and the pooled figures."""
    doc = {
        "version": 1,
        "site": site
        or online["header"].get("site")
        or smoothed["header"].get("site")
        or "",
        "online": describe(online),
        "smoothed": describe(smoothed),
        "pairing": pair_parts(online, smoothed),
        "selection": {
            "min_samples": MIN_SAMPLES,
            "min_mean_speed": MIN_MEAN_SPEED,
            "min_median_height": MIN_MEDIAN_HEIGHT,
        },
        "warnings": [],
    }
    if doc["online"]["trail_estimator"] == doc["smoothed"]["trail_estimator"]:
        doc["warnings"].append(
            f"both exports carry the trail {doc['online']['trail_estimator']}: "
            "the along-course offset compares a trail with itself"
        )
    fallback = (doc["smoothed"]["trail_smoothing"] or {}).get("online_fallback") or 0
    if fallback:
        doc["warnings"].append(
            f"{fallback} track frames of the smoothed export kept the online posterior"
        )
    if not doc["pairing"]["ok"]:
        return doc
    series_o, series_s = track_series(online), track_series(smoothed)
    rows, raws = [], []
    for tid, samples in series_o.items():
        if not moving_vehicle(samples):
            continue
        row, raw = measure_track(tid, samples, series_s[tid])
        rows.append(row)
        raws.append(raw)
    doc["selection"]["tracks_total"] = len(series_o)
    doc["selection"]["moving_vehicle_tracks"] = len(rows)
    doc["tracks"] = rows
    doc["pooled"] = pool(rows, raws)
    doc["rule"] = {
        "departure_halved": doc["pooled"]["rule"]["departure_halved"],
        "identity_kept": True,
        "passes": doc["pooled"]["rule"]["departure_halved"],
        "flattening": doc["pooled"]["rule"]["flattening"],
    }
    return doc


# --- output


def clean(value):
    """JSON cannot carry NaN or infinity; they become null."""
    if isinstance(value, float):
        return value if math.isfinite(value) else None
    if isinstance(value, dict):
        return {k: clean(v) for k, v in value.items()}
    if isinstance(value, (list, tuple)):
        return [clean(v) for v in value]
    return value


def fmt(value, digits=2, sign=False):
    if value is None or (isinstance(value, float) and not math.isfinite(value)):
        return "—"
    if isinstance(value, bool):
        return "yes" if value else "no"
    if isinstance(value, int):
        return str(value)
    return f"{value:+.{digits}f}" if sign else f"{value:.{digits}f}"


def table(header, rows):
    lines = [
        "| " + " | ".join(header) + " |",
        "| " + " | ".join("---" for _ in header) + " |",
    ]
    for row in rows:
        lines.append("| " + " | ".join(row) + " |")
    return "\n".join(lines)


def flatten(prefix, value, out):
    if isinstance(value, dict):
        for key, inner in value.items():
            flatten(f"{prefix}.{key}" if prefix else key, inner, out)
    else:
        out.append((prefix, value))


def smoothing_lines(doc):
    """header.json's trail_smoothing counts, flattened one per line."""
    summary = doc["smoothed"]["trail_smoothing"]
    if not summary:
        return ["trail_smoothing: none in the header"]
    pairs = []
    flatten("", summary, pairs)
    return [
        "trail_smoothing: "
        + ", ".join(f"{key} {fmt(value, 3)}" for key, value in pairs)
    ]


MEASURE_ROWS = (
    ("departure from constant velocity, pooled RMS (m/s²)", "pooled", "departure_rms"),
    (
        "departure from constant velocity, track median (m/s²)",
        "track_median",
        "departure_rms",
    ),
    ("velocity against the trail, pooled RMS (m/s)", "pooled", "velocity_rms"),
    ("velocity against the trail, track median (m/s)", "track_median", "velocity_rms"),
    ("step residual, pooled RMS (m per step)", "pooled", "step_rms"),
    ("step residual, track median (m per step)", "track_median", "step_rms"),
    ("lateral residual, pooled p50 (m)", "pooled", "lateral_p50"),
    ("lateral residual, pooled p90 (m)", "pooled", "lateral_p90"),
    ("lateral residual p90, track median (m)", "track_median", "lateral_p90"),
    (
        "peak 0.5 s course rate, track median (rad/s)",
        "track_median",
        "course_rate_peak",
    ),
)

OFFSET_ROWS = (
    ("along-course offset, pooled median (m)", "pooled", "along_median", True),
    (
        "along-course offset, pooled p90 of the absolute offset (m)",
        "pooled",
        "along_p90",
        False,
    ),
    ("along-course offset, track median (m)", "track_median", "along_median", True),
    ("across-course offset, pooled median (m)", "pooled", "across_median", True),
    (
        "across-course offset, pooled p90 of the absolute offset (m)",
        "pooled",
        "across_p90",
        False,
    ),
)


def markdown_pair(doc, tracks=False):
    """One site's report: the exports, the pairing, the pooled table and optionally every track."""
    lines = [f"## {doc['site'] or 'scene'}: online against smoothed trail", ""]
    for arm in ("online", "smoothed"):
        info = doc[arm]
        named = "" if info["trail_estimator_named"] else " (not named in the header)"
        lines.append(
            f"- {arm}: `{info['trail_estimator']}`{named}, {info['frame_count']} frames at stride "
            f"{info['frame_stride']}, build {info['build_version']}, generated {info['generated_at']}"
        )
    pairing = doc["pairing"]
    if pairing["ok"]:
        lines.append(
            f"- pairing: identical outside x, y, vx, vy over {pairing['frames']} frames, "
            f"{pairing['tracks']} tracks, {pairing['track_frames']} track frames"
        )
    else:
        fields = ", ".join(
            f"{k} ×{v}" for k, v in sorted(pairing["differences"].items())
        )
        lines.append(f"- pairing: **refused**, the exports differ in {fields}")
        for example in pairing["examples"]:
            lines.append(f"  - {example}")
    lines.append("- " + smoothing_lines(doc)[0])
    for warning in doc["warnings"]:
        lines.append(f"- warning: {warning}")
    if not pairing["ok"]:
        return "\n".join(lines) + "\n"
    pooled = doc["pooled"]
    selection = doc["selection"]
    lines.append(
        f"- moving vehicle tracks: {selection['moving_vehicle_tracks']} of {selection['tracks_total']} "
        f"(≥{selection['min_samples']} samples, mean spd ≥{selection['min_mean_speed']} m/s, "
        f"median h ≥{selection['min_median_height']} m); median step {fmt(pooled['interval_s'], 3)} s"
    )
    rule = doc["rule"]
    lines.append(
        f"- rule: departure ratio {fmt(pooled['departure_ratio'])} "
        f"(halved: {fmt(rule['departure_halved'])}), identity kept: {fmt(rule['identity_kept'])}, "
        f"course rate kept {fmt(pooled['course_rate_kept'])} "
        f"(flattening: {fmt(rule['flattening'])}); passes: {fmt(rule['passes'])}"
    )
    lines.append("")
    rows = []
    for label, group, key in MEASURE_ROWS:
        o, s = pooled["online"][group][key], pooled["smoothed"][group][key]
        rows.append([label, fmt(o, 3), fmt(s, 3), fmt(ratio(s, o))])
    for label, group, key, signed in OFFSET_ROWS:
        rows.append(
            [label, "—", fmt(pooled["smoothed"][group][key], 3, sign=signed), "—"]
        )
    rows.append(
        [
            "peak 0.5 s course rate kept, track median",
            "—",
            fmt(pooled["course_rate_kept"], 3),
            "—",
        ]
    )
    lines.append(table(["measure", "online", "smoothed", "smoothed / online"], rows))
    if tracks:
        lines.append("")
        lines.append(markdown_tracks(doc["tracks"]))
    return "\n".join(lines) + "\n"


def markdown_tracks(rows):
    header = [
        "track",
        "class",
        "n",
        "spd",
        "departure o/s",
        "velocity o/s",
        "step o/s",
        "lateral p90 o/s",
        "along med / p90",
        "course kept",
    ]
    body = []
    for row in rows:
        o, s = row["online"], row["smoothed"]
        body.append(
            [
                row["id"],
                row["class"] or "—",
                str(row["samples"]),
                fmt(row["mean_speed"], 1),
                f"{fmt(o['departure_rms'])} / {fmt(s['departure_rms'])}",
                f"{fmt(o['velocity_rms'])} / {fmt(s['velocity_rms'])}",
                f"{fmt(o['step_rms'])} / {fmt(s['step_rms'])}",
                f"{fmt(o['lateral_p90'])} / {fmt(s['lateral_p90'])}",
                f"{fmt(s['along_median'], sign=True)} / {fmt(s['along_p90'])}",
                fmt(s["course_rate_share"]),
            ]
        )
    return table(header, body)


def markdown_set(docs, notes):
    """Every site on one row, for the report's table."""
    header = [
        "site",
        "tracks",
        "departure o → s (m/s²)",
        "ratio",
        "velocity o → s (m/s)",
        "step o → s (m)",
        "lateral p90 o → s (m)",
        "along med / p90 (m)",
        "course kept",
        "paired",
        "passes",
    ]
    rows = []
    for doc in docs:
        if not doc["pairing"]["ok"]:
            fields = ", ".join(sorted(doc["pairing"]["differences"]))
            rows.append([doc["site"]] + ["—"] * 8 + [f"refused ({fields})", "no"])
            continue
        p = doc["pooled"]
        o, s = p["online"]["pooled"], p["smoothed"]["pooled"]
        rows.append(
            [
                doc["site"],
                str(p["tracks"]),
                f"{fmt(o['departure_rms'])} → {fmt(s['departure_rms'])}",
                fmt(p["departure_ratio"]),
                f"{fmt(o['velocity_rms'])} → {fmt(s['velocity_rms'])}",
                f"{fmt(o['step_rms'])} → {fmt(s['step_rms'])}",
                f"{fmt(o['lateral_p90'])} → {fmt(s['lateral_p90'])}",
                f"{fmt(s['along_median'], sign=True)} / {fmt(s['along_p90'])}",
                fmt(p["course_rate_kept"]),
                "yes",
                fmt(doc["rule"]["passes"])
                + (" (flattening)" if doc["rule"]["flattening"] else ""),
            ]
        )
    lines = ["## Smoothed trail against online, every site", "", table(header, rows)]
    passing = sum(1 for doc in docs if doc.get("rule", {}).get("passes"))
    lines.append("")
    lines.append(
        f"{passing} of {len(docs)} sites pass the rule (departure halved, identity kept)."
    )
    for note in notes:
        lines.append(f"- {note}")
    return "\n".join(lines) + "\n"


def write_text(path, text):
    if path:
        with open(path, "w", encoding="utf-8") as fh:
            fh.write(text)
    else:
        sys.stdout.write(text)


def write_json(path, doc):
    if not path:
        return
    with open(path, "w", encoding="utf-8") as fh:
        json.dump(clean(doc), fh, indent=2, allow_nan=False)
        fh.write("\n")


# --- commands


def run_pair(args):
    doc = analyse(read_part(args.online), read_part(args.smoothed), site=args.site)
    write_json(args.json, doc)
    write_text(args.markdown, markdown_pair(doc, tracks=args.tracks))
    if not doc["pairing"]["ok"]:
        print(
            f"{doc['site'] or args.online}: pairing refused, the exports differ in "
            + ", ".join(sorted(doc["pairing"]["differences"])),
            file=sys.stderr,
        )
        return 1
    return 0


def set_pairs(root, online_variant, smoothed_variant):
    """The sites of a variant set's output: <root>/<site>/<variant>/assets/part-000."""
    pairs, notes = [], []
    for site in sorted(os.listdir(root)):
        site_dir = os.path.join(root, site)
        if not os.path.isdir(site_dir):
            continue
        parts = [
            os.path.join(site_dir, variant, "assets", "part-000")
            for variant in (online_variant, smoothed_variant)
        ]
        missing = [
            p for p in parts if not os.path.exists(os.path.join(p, "header.json"))
        ]
        if missing:
            notes.append(f"{site}: skipped, no export at {missing[0]}")
            continue
        pairs.append((site, parts[0], parts[1]))
    return pairs, notes


def run_set(args):
    pairs, notes = set_pairs(args.root, args.online_variant, args.smoothed_variant)
    docs = []
    for site, online, smoothed in pairs:
        docs.append(analyse(read_part(online), read_part(smoothed), site=site))
    write_json(
        args.json, {"version": 1, "root": args.root, "sites": docs, "notes": notes}
    )
    text = markdown_set(docs, notes)
    if args.tracks:
        text += "\n" + "\n".join(markdown_pair(doc, tracks=True) for doc in docs)
    write_text(args.markdown, text)
    refused = [doc["site"] for doc in docs if not doc["pairing"]["ok"]]
    if refused:
        print("pairing refused for " + ", ".join(refused), file=sys.stderr)
        return 1
    if not docs:
        print(f"{args.root}: no site has both exports", file=sys.stderr)
        return 1
    return 0


def build_parser():
    parser = argparse.ArgumentParser(
        description="The steadiness of two scene exports' trails, online against smoothed (l5est0002).",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    sub = parser.add_subparsers(dest="command", required=True)

    pair = sub.add_parser("pair", help="two part directories of one recording")
    pair.add_argument(
        "online", help="the online export's part directory (header.json beside frames/)"
    )
    pair.add_argument("smoothed", help="the smoothed export's part directory")
    pair.add_argument(
        "--site", help="site name for the report; default from header.json"
    )
    pair.set_defaults(run=run_pair)

    whole = sub.add_parser("set", help="every site of a scene-variants set's output")
    whole.add_argument(
        "root", help="<out>/<set_id>, holding <site>/<variant>/assets/part-000"
    )
    whole.add_argument(
        "--online-variant", default="online", help="variant id of the online export"
    )
    whole.add_argument(
        "--smoothed-variant",
        default="smoothed",
        help="variant id of the smoothed export",
    )
    whole.set_defaults(run=run_set)

    for command in (pair, whole):
        command.add_argument("--json", help="write the JSON document here")
        command.add_argument(
            "--markdown", help="write the Markdown here instead of stdout"
        )
        command.add_argument(
            "--tracks", action="store_true", help="include every track's row"
        )
    return parser


def main(argv=None):
    args = build_parser().parse_args(argv)
    try:
        return args.run(args)
    except ReaderError as err:
        print(f"error: {err}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
