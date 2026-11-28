#!/usr/bin/env python3
"""One-session Madrid Zenodo v2 clock/derivative smoke test, not sensor qualification.

Download Participant07.csv from https://zenodo.org/records/24137291 separately.
Run: python scripts/lidar_madrid_kinematics_smoke.py /path/to/Participant07.csv
Requires Python 3 and NumPy >=2.0. Output is aggregate diagnostics only.
"""

import argparse
import csv
import hashlib
import json
import numpy as np


def quant(values):
    return np.round(
        np.quantile(values, [0, 0.01, 0.05, 0.5, 0.95, 0.99, 1]), 6
    ).tolist()


def read(path):
    fields = [
        "Timestamp Eyetracking",
        "Time(s)_GPS",
        "Latitude",
        "Longitude",
        "Speed",
        "Brake",
    ]
    with open(path, newline="") as f:
        reader = csv.DictReader(f, delimiter=";")
        return np.asarray([tuple(float(row[key]) for key in fields) for row in reader])


def main(path):
    raw = open(path, "rb").read()
    md5 = hashlib.md5(raw, usedforsecurity=False).hexdigest()
    sha256 = hashlib.sha256(raw).hexdigest()
    expected_md5 = "6a5e42f16f808ddeb1f91bae76bac4b8"
    expected_sha256 = "01f4a14840a9745f97b0a3bd6401ff1453967d27830ac392a9095438993361d2"
    if md5 != expected_md5 or sha256 != expected_sha256:
        raise SystemExit("Input does not match the pinned Zenodo v2 Participant07.csv")
    obs = read(path)
    eye, t, lat, lon, can, brake = obs.T
    assert np.isfinite(obs).all() and (np.diff(t) > 0).all()
    t = t - t[0]
    d = np.diff(t)
    eye = eye - eye[0]
    xy = np.c_[lat, lon]
    report = dict(
        bytes=len(raw),
        rows=len(t),
        md5=md5,
        sha256=sha256,
        duration=round(t[-1], 3),
        eye_duration=round(eye[-1], 3),
        gps_time_delta_ms=quant(d * 1000),
        eye_delta_ms=quant(np.diff(eye) * 1000),
        clock_duration_difference_s=round(eye[-1] - t[-1], 3),
        eye_clock_gaps_gt_100ms=int((np.diff(eye) > 0.1).sum()),
        eye_clock_gaps_gt_100ms_total_s=round(
            np.diff(eye)[np.diff(eye) > 0.1].sum(), 3
        ),
        exact_position_repeat_fraction=round(
            float(np.mean(np.all(np.diff(xy, axis=0) == 0, axis=1))), 6
        ),
        largest_gps_clock_gap_ms=round(1000 * d.max(), 3),
        gps_clock_gaps_gt_100ms=int((d > 0.1).sum()),
        exact_can_speed_repeat_fraction=round(float(np.mean(np.diff(can) == 0)), 6),
    )
    # The file's coordinate magnitudes cannot be decimal degrees. A total-arcminute
    # encoding is inferred from the Madrid locality and kinematic agreement; the
    # data dictionary did not establish it. Never pass these values through as degrees.
    report["coordinate_encoding"] = (
        "total arcminutes inferred, not independently documented"
    )
    report["can_speed_units"] = "km/h assumed"
    assert (abs(lat) > 90).any() and (abs(lon) > 180).any()
    assert (abs(lat / 60) < 90).all() and (abs(lon / 60) < 180).all()
    # Convert local displacement to metres on a tangent plane; no coordinates in output.
    rad = np.deg2rad((xy - np.median(xy, axis=0)) / 60)
    rad[:, 1] *= np.cos(np.deg2rad(np.median(lat) / 60))
    p = 6371008.8 * rad

    def nmea_degrees(v):
        deg = np.trunc(v / 100)
        return deg + (v - 100 * deg) / 60

    report["coordinate_sanity"] = dict(
        total_arcminutes_within_madrid_region=bool(
            (
                (lat / 60 > 39) & (lat / 60 < 41) & (lon / 60 > -5) & (lon / 60 < -3)
            ).all()
        ),
        signed_nmea_ddmm_within_madrid_region=bool(
            (
                (nmea_degrees(lat) > 39)
                & (nmea_degrees(lat) < 41)
                & (nmea_degrees(lon) > -5)
                & (nmea_degrees(lon) < -3)
            ).all()
        ),
    )

    def gps_speed(grid, window):
        left = np.array([np.interp(grid - window / 2, t, a) for a in p.T])
        right = np.array([np.interp(grid + window / 2, t, a) for a in p.T])
        return np.linalg.norm(right - left, axis=0) / window

    # Common grid no finer than documented GNSS source rate. Protect search and smoothing edges.
    grid = np.arange(3, t[-1] - 3, 0.1)
    if (d > 0.1).any():
        for left, right in zip(t[:-1][d > 0.1], t[1:][d > 0.1]):
            grid = grid[(grid < left - 3) | (grid > right + 3)]

    def ref(grid):
        return np.interp(grid, t, can) / 3.6  # Verify units against paper.

    moving = ref(grid) > 2
    report["speed_comparison"] = {}

    def errors(e):
        return dict(
            n=len(e),
            bias=round(e.mean(), 4),
            mae=round(abs(e).mean(), 4),
            rmse=round(np.sqrt((e**2).mean()), 4),
            p95_abs=round(np.quantile(abs(e), 0.95), 4),
        )

    # Diagnostics only. Differencing a smoothed GPS speed and a CAN speed
    # is not a validation against an independent longitudinal accelerometer.
    report["acceleration_comparison"] = {}
    for halfwidth in (0.25, 0.5, 1.0):
        gps_acc = (gps_speed(grid + halfwidth, 1) - gps_speed(grid - halfwidth, 1)) / (
            2 * halfwidth
        )
        can_acc = (ref(grid + halfwidth) - ref(grid - halfwidth)) / (2 * halfwidth)
        report["acceleration_comparison"][2 * halfwidth] = errors(
            (gps_acc - can_acc)[moving]
        )
    # Descriptive co-occurrence only; pedal application isn't physical deceleration.
    pedal_on = np.interp(grid, t, brake) > 10
    gps_acc = gps_speed(grid + 0.5, 1) - gps_speed(grid - 0.5, 1)
    can_acc = ref(grid + 0.5) - ref(grid - 0.5)
    report["moving_brake_diagnostic"] = dict(
        pedal_threshold_native_units=10,
        n=int((moving & pedal_on).sum()),
        median_gps_accel_mps2=round(np.median(gps_acc[moving & pedal_on]), 4),
        median_can_speed_derivative_mps2=round(
            np.median(can_acc[moving & pedal_on]), 4
        ),
        note="threshold not qualified; no physical brake-onset or acceleration reference",
    )
    assert (np.diff(eye) > 0).all()
    eye_grid = np.interp(grid, t, eye)
    common = moving.copy()
    for left, right in zip(eye[:-1][np.diff(eye) > 0.1], eye[1:][np.diff(eye) > 0.1]):
        common &= (eye_grid + 0.5 < left) | (eye_grid - 0.5 > right)
    eye_displacement = np.array(
        [
            np.interp(eye_grid + 0.5, eye, a) - np.interp(eye_grid - 0.5, eye, a)
            for a in p.T
        ]
    )
    eye_speed = np.linalg.norm(eye_displacement, axis=0)
    ref_eye = np.interp(eye_grid, eye, can) / 3.6
    report["alternate_seconds_clock_common_support"] = dict(
        gps_time_errors=errors((gps_speed(grid, 1) - ref(grid))[common]),
        eye_time_errors=errors((eye_speed - ref_eye)[common]),
        note="same moving row-mapped support; remove eye-axis intervals crossing gaps >100 ms; no verified hardware clock",
    )
    report["distance_km"] = dict(
        can_speed_integral=round(np.trapezoid(can / 3.6, t) / 1000, 4),
        gps_sum_10Hz_chords=round(
            np.linalg.norm(
                np.diff(np.array([np.interp(grid, t, ax) for ax in p.T]), axis=1),
                axis=0,
            ).sum()
            / 1000,
            4,
        ),
        gps_1_second_speed_integral=round(
            np.trapezoid(gps_speed(grid, 1), grid) / 1000, 4
        ),
        note="CAN full session; GPS evaluated grid trims 3 s per edge; sampled chord sum includes noise",
    )
    report["speed_bins_can_mps"] = {}
    for lo, hi in ((2, 5), (5, 8), (8, 12), (12, 16), (16, 21)):
        mask = (ref(grid) >= lo) & (ref(grid) < hi)
        report["speed_bins_can_mps"][f"{lo} to <{hi}"] = errors(
            (gps_speed(grid, 1) - ref(grid))[mask]
        )
    stopped = ref(grid) == 0
    report["can_zero_speed_diagnostic"] = dict(
        n=int(stopped.sum()),
        gps_1_second_speed_mps_quantiles=quant(gps_speed(grid, 1)[stopped]),
    )
    for window in (0.1, 0.5, 1, 2):
        gps = gps_speed(grid, window)

        def score(lag, mask):
            e = (gps - ref(grid + lag))[mask]
            return errors(e)

        early = moving & (grid < t[-1] / 2)
        late = moving & (grid >= t[-1] / 2)
        lags = np.arange(-2, 2.001, 0.1)

        def fit(mask):
            return round(
                float(
                    min(
                        lags,
                        key=lambda lag: np.mean((gps - ref(grid + lag))[mask] ** 2),
                    )
                ),
                2,
            )

        lag = fit(early)
        edges = np.linspace(0, t[-1], 5)
        quarters = []
        for q0, q1 in zip(edges[:-1], edges[1:]):
            support = moving & (grid >= q0) & (grid < q1)
            qlag = fit(support)
            quarters.append(
                dict(
                    best_can_query_shift_seconds=qlag,
                    unshifted=score(0, support),
                    fitted=score(qlag, support),
                )
            )
        report["speed_comparison"][window] = dict(
            no_shift=score(0, moving),
            early_fit_shift_seconds=lag,
            late_fit_shift_seconds=fit(late),
            late_unshifted=score(0, late),
            late_earlyfit_shift=score(lag, late),
            quarters=quarters,
        )
    print(json.dumps(report, indent=2))


if __name__ == "__main__":
    a = argparse.ArgumentParser()
    a.add_argument("file")
    args = a.parse_args()
    main(args.file)
