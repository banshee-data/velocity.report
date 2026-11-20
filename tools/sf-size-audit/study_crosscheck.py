"""Tie the S6 "9.5 MB" budget to its sources, and calibrate the audit's codec against the study's.

Runs in the asset study's work directory (on arrow-worker, /home/banshee/city-assets-20261118),
which holds the study's outputs and encoders and is not in this repository:

    cd /home/banshee/city-assets-20261118
    venv/bin/python -I <repo>/tools/sf-size-audit/study_crosscheck.py <repo>/public_html/src/sf/tiles

It prints JSON with three parts:
  budget       the S6 total recomputed from the study's own outputs, term by term, the way
               scripts/relabel.py computed it;
  original_encoders_on_shipped_tiles
               the study's tree and kerb encoders applied to the shipped tiles;
  window_calibration
               the shipped tile buildings inside the study's three 520 m windows under the audit's
               unwelded codec, against the study's S6 roof bytes (enc_graph2) for the same windows.
"""

import glob
import gzip
import json
import math
import os
import sys

import brotli
import numpy as np

sys.path.insert(0, os.path.join(os.getcwd(), "scripts"))
from common import delta_stream, varint  # noqa: E402  (the study's encoders)

SITES = ("presidio", "sunset", "downtown")
tiles_dir = sys.argv[1]


def load(p):
    return json.load(open(p))


# ---- budget ------------------------------------------------------------------
S = {}
for s in SITES:
    S.update(load(f"viewer/data2/summary-{s}.json"))
roofs_city = load("out/roofs-city.json")
roads_city = load("out/roads-city.json")
l2_city = next(lv for lv in roofs_city["levels"] if lv["tol_m"] == 0.25)["enc_q0.05"][
    "brotli"
]
l2_win = sum(S[s]["levels"]["L2"]["building_bytes"] for s in SITES)
s6_win = sum(S[s]["levels"]["S6"]["roof_bytes_alone"] for s in SITES)
area_k = 121.0 / (3 * 0.52**2)  # city land area over the three windows' area
trees_win = sum(S[s]["tree_bytes_compact"] for s in SITES)
extras_win = sum(S[s]["extras"]["bytes"] for s in SITES)
centre = roads_city["centrelines"][0]["enc_q0.1"]["brotli"]
kerbs = next(lv for lv in roads_city["kerbs"]["levels"] if lv["tol_m"] == 0.25)[
    "enc_q0.05"
]["brotli"]
budget = {
    "l2_roofs_citywide_measured_bytes": l2_city,
    "l2_used_in_formula_mb": 18.0,
    "windows_l2_bytes": l2_win,
    "windows_s6_roof_bytes": s6_win,
    "s6_roofs_mb_estimated": 18.0 * s6_win / l2_win,
    "boxes_unroofed_mb_recorded": 0.35,
    "boxes_unroofed_source": "recorded in the round-1 report; no saved output found",
    "centrelines_citywide_measured_bytes": centre,
    "kerbs_citywide_measured_bytes": kerbs,
    "roads_mb_used": 0.46,
    "area_scale_factor": area_k,
    "windows_tree_bytes_compact": trees_win,
    "trees_mb_estimated": trees_win * area_k / 1e6,
    "windows_trees": sum(S[s]["trees"] for s in SITES),
    "trees_city_count_estimated": sum(S[s]["trees"] for s in SITES) * area_k,
    "windows_3dep_bytes": extras_win,
    "lidar_only_mb_estimated": extras_win * area_k / 1e6,
}
budget["total_mb"] = (
    budget["s6_roofs_mb_estimated"]
    + 0.81
    + budget["trees_mb_estimated"]
    + budget["lidar_only_mb_estimated"]
)

# ---- the study's encoders on the shipped tiles -----------------------------
T, K = [], []
tiles = [json.load(gzip.open(p)) for p in sorted(glob.glob(f"{tiles_dir}/*.json.gz"))]
for d in tiles:
    ox, oy = d["o"]
    T += [(ox + x / 10, oy + y / 10, 0, h, r) for x, y, h, r in d["tr"]]
    K += [np.asarray(k, float).reshape(-1, 2) / 10 + [ox, oy] for k in d["k"]]
T = np.asarray(T)
o = np.lexsort((T[:, 1], np.round(T[:, 0] / 50)))
Ts = T[o]  # viewer_prep2.tree_bytes_compact: 0.5 m positions, 1 m height and crown
raw = (
    delta_stream([Ts[:, :2]], 0.5)
    + varint(np.round(Ts[:, 3]).astype(np.int64))
    + varint(np.round(Ts[:, 4]).astype(np.int64))
)
original = {
    "trees": len(T),
    "trees_tree_bytes_compact_brotli11": len(brotli.compress(raw, quality=11)),
    "kerb_lines": len(K),
    "kerbs_delta_stream_q0.1_brotli11_lgwin24": len(
        brotli.compress(delta_stream(K, 0.1), quality=11, lgwin=24)
    ),
}


# ---- codec calibration on the study's windows --------------------------------
def zz(v):
    return v * 2 if v >= 0 else -v * 2 - 1


def uv(out, v):
    while v >= 0x80:
        out.append((v & 0x7F) | 0x80)
        v >>= 7
    out.append(v)


man = load(f"{tiles_dir}/manifest.json")
centres = {
    p["id"]: (
        p["x"] + 40 * math.cos(p["heading"]),
        p["y"] + 40 * math.sin(p["heading"]),
    )
    for p in man["places"]
    if p["id"] in SITES
}
cal = {}
for k, (cx, cy) in centres.items():
    cols = {n: bytearray() for n in ("nf", "nv", "x", "y", "z", "zb", "flag")}
    px = py = n = 0
    for d in tiles:
        ox, oy = d["o"]
        for b in d["b"]:
            r0 = b[1][0][0]
            if abs(ox + r0[0] / 10 - cx) > 260 or abs(oy + r0[1] / 10 - cy) > 260:
                continue
            n += 1
            uv(cols["zb"], zz(b[0]))
            uv(cols["flag"], 1 if len(b) > 2 and b[2] == 1 else 0)
            rings = [r for f in b[1] for r in f]
            uv(cols["nf"], len(rings))
            for ring in rings:
                uv(cols["nv"], len(ring) // 3)
                pz = b[0]
                for i in range(0, len(ring), 3):
                    x, y = round(ox * 10) + ring[i], round(oy * 10) + ring[i + 1]
                    uv(cols["x"], zz(x - px))
                    uv(cols["y"], zz(y - py))
                    uv(cols["z"], zz(ring[i + 2] - pz))
                    px, py, pz = x, y, ring[i + 2]
    cal[k] = {
        "tile_buildings_in_window": n,
        "audit_codec_brotli11": len(
            brotli.compress(b"".join(bytes(c) for c in cols.values()), quality=11)
        ),
        "study_s6_roofs_plus_3dep_bytes": S[k]["levels"]["S6"]["roof_bytes_alone"]
        + S[k]["extras"]["bytes"],
    }
cal["ratio_audit_codec_over_study"] = sum(
    v["audit_codec_brotli11"] for v in cal.values()
) / sum(v["study_s6_roofs_plus_3dep_bytes"] for v in cal.values())

print(
    json.dumps(
        {
            "budget": budget,
            "original_encoders_on_shipped_tiles": original,
            "window_calibration": cal,
        },
        indent=1,
    )
)
