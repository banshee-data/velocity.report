#!/usr/bin/env python3
"""Put Gerardus's site geometry into a scene, in the scene's own sensor frame.

A scene's points, boxes and background are in the sensor's frame: origin at
the sensor, Z up, X and Y the sensor's own axes. Gerardus serves a site's
geometry in its site frame (x east, y north, z NAVD88 height, origin the
centre of the site's S2 L16 cell) and places a sensor in that frame with an
accepted pose,

    p_site = R * p_sensor + t

so a feature goes into the scene by the inverse, p_sensor = R^T (p_site - t).
The scene keeps its frame, and nothing georeferenced is added to it.

Only a site whose scan has a pose an operator accepted gets vectors. The
eye-read position the site index holds is metres and degrees out (pose0001),
so geometry placed by it would draw a kerb beside the kerb the sensor saw and
present a guess as a measurement. A scene without an accepted pose plays as
it did. Before writing, the pose is checked against the scene's own settled
background: the walls and kerbs must lie closer to its points at the pose
than at the pose moved 2 m or turned 5 degrees. A pose for another sensor
placement fails that check and nothing is written.

Usage, with Gerardus serving on its default port:

    python3 tools/s2-archive/scene-vectors.py columbus-broadway
    python3 tools/s2-archive/scene-vectors.py --status       # which sites can have vectors

publish-scenes.py calls the same export for each scene it builds
(SCENE_VECTORS=auto, the default), so a rebuilt scene carries vectors where
the site has an accepted pose.

What is written: assets/vectors/vectors.geojson.gz, a GeoJSON
FeatureCollection whose coordinates are metres in the scene's sensor frame,
rounded to 0.01 m. It is not RFC 7946 WGS 84: `crs` says so. Its members
carry the context a copied file needs (docs/geometry-priors-data-purpose.md):
each source and its terms, the pose and how it was checked, and per feature
its layer, kind, evidence status, source and Gerardus state. Features keep
geometry and the heights needed to draw them and nothing else: no names,
addresses, building use, materials or roof types. OpenStreetMap layers are
never asked for.

Settings:

  GERARDUS_URL=http://127.0.0.1:50061
      Gerardus's HTTP and gRPC port. The geometry comes over HTTP; the pose
      and the sources over gRPC, through grpcurl and the vendored
      proto/gerardus/align/v1alpha1/align.proto.
  GRPCURL=grpcurl
      The grpcurl binary, when it is not on PATH.
  SCENE_VECTORS_RADIUS_M=90
      Features with no vertex within this distance of the sensor are left
      out. Ground cells are clipped to the same radius.
"""

import argparse
import gzip
import json
import math
import os
import shutil
import subprocess
import sys
import tempfile
import urllib.error
import urllib.parse
import urllib.request
from datetime import datetime, timezone

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.dirname(os.path.dirname(HERE))
SCENES = os.path.join(REPO, "public_html", "src", "scenes")
SCENES_DATA = os.path.join(REPO, "public_html", "src", "_data", "scenes.json")
PROTO_DIR = os.path.join(REPO, "proto", "gerardus", "align", "v1alpha1")
GERARDUS = os.environ.get("GERARDUS_URL", "http://127.0.0.1:50061").rstrip("/")
RADIUS_M = float(os.environ.get("SCENE_VECTORS_RADIUS_M", "90"))
VERSION = 4
BUILDING_LAYERS = {"footprints", "box_walls", "wall_surfaces", "roof_planes"}
ROUNDING_M = 0.01
OUT_DIR = "vectors"
OUT_FILE = "vectors.geojson.gz"

# How each layer is known, and where it comes from when Gerardus's own
# `source` property does not say. Observed: fitted to returns that cover it,
# the scans or 3DEP's. Inferred: carried past the returns, as a wall raised to
# a roofline or a tree's crown sized from a default. Source data: a third
# party's vectors as published. The order is the order the player draws.
LAYERS = {
    "city_streets": ("source_data", ("datasf-streets",)),
    "footprints": ("source_data", ("datasf-footprints",)),
    "observed": ("observed", ("scans",)),
    "wall_planes": ("observed", ("scans",)),
    "box_walls": ("inferred", ("datasf-footprints", "3dep")),
    "wall_surfaces": ("inferred", ("datasf-footprints", "3dep")),
    "roof_planes": ("observed", ("3dep",)),
    "trees": ("inferred", ("scans",)),
}
# The kinds kept from the `observed` layer. Its lane markings repeat the
# lane_markings layer; its sensor and correction marks are about the
# registration, not the street.
OBSERVED_KINDS = ("kerb", "facade")
# Gerardus's source words, as the ids of the sources member.
SOURCE_WORDS = {
    "registered-scans": "scans",
    "3dep": "3dep",
    "datasf": "datasf-footprints",
}
# Gerardus's reference layers, as the ids of the sources member. OSM is not
# among them: nothing published here derives from it (Gerardus ADR-0008).
REFERENCE_SOURCES = {
    "REFERENCE_LAYER_AERIAL_POINTS": "3dep",
    "REFERENCE_LAYER_FOOTPRINTS": "datasf-footprints",
    "REFERENCE_LAYER_CITY_STREETS": "datasf-streets",
}
# Geometry context only; no building names, use, or materials.
KEPT_PROPERTIES = ("height_m", "diameter_m", "dbh_m", "crown_radius_m", "crown_base_m")
# The placement check: the pose must place walls and kerbs nearer the scene's
# background than each of these perturbations does (metres east, north in the
# sensor frame; degrees about the sensor's vertical).
PERTURBATIONS = ((2, 0, 0), (-2, 0, 0), (0, 2, 0), (0, -2, 0), (0, 0, 5), (0, 0, -5))
CHECK_LAYERS = ("wall_surfaces", "box_walls", "wall_planes", "observed")
CHECK_VOXEL_M = 0.25
CHECK_SAMPLES = 8000
# Fewer samples than this cannot tell a fit from luck; nothing is written.
CHECK_MIN_SAMPLES = 50


class NoPose(Exception):
    """The site has no accepted pose, which is a state and not a failure."""


class Unreachable(Exception):
    """Gerardus did not answer."""


def log(message):
    print(f"[{datetime.now():%H:%M:%S}] {message}", flush=True)


# =============================================================================
# GERARDUS
# =============================================================================


def grpcurl_binary():
    named = os.environ.get("GRPCURL")
    if named:
        return named
    found = shutil.which("grpcurl")
    if found:
        return found
    home = os.path.expanduser("~/go/bin/grpcurl")
    return home if os.path.exists(home) else "grpcurl"


def grpc(method, payload, base=None, timeout=900):
    """Call a gerardus.align.v1alpha1 method and return its JSON answer."""
    parsed = urllib.parse.urlparse(base or GERARDUS)
    address = parsed.netloc or parsed.path
    command = [grpcurl_binary()]
    if parsed.scheme != "https":
        command.append("-plaintext")
    command += [
        "-import-path",
        PROTO_DIR,
        "-proto",
        "align.proto",
        "-d",
        json.dumps(payload),
        address,
        f"gerardus.align.v1alpha1.{method}",
    ]
    try:
        done = subprocess.run(command, capture_output=True, text=True, timeout=timeout)
    except FileNotFoundError as error:
        raise Unreachable(f"grpcurl not found ({command[0]}); set GRPCURL") from error
    except subprocess.TimeoutExpired as error:
        raise Unreachable(f"{method} gave no answer in {timeout} s") from error
    if done.returncode != 0:
        message = done.stderr.strip().splitlines()[-1:] or ["no message"]
        if "Unavailable" in done.stderr or "connection refused" in done.stderr:
            raise Unreachable(f"{method}: {message[0]}")
        raise RuntimeError(f"{method}: {message[0]}")
    return json.loads(done.stdout or "{}")


def http_json(path, base=None, timeout=900):
    url = f"{base or GERARDUS}{path}"
    try:
        with urllib.request.urlopen(url, timeout=timeout) as response:
            return json.load(response)
    except urllib.error.HTTPError as error:
        raise RuntimeError(f"GET {path}: HTTP {error.code}") from error
    except (urllib.error.URLError, TimeoutError, ConnectionError) as error:
        raise Unreachable(f"GET {path}: {error}") from error


def served_layers(l16, base=None):
    """The layers Gerardus lists for the site, or None when it does not list it."""
    listing = http_json("/api/v1/sites", base, timeout=60)
    for site in listing.get("sites", []):
        if site.get("l16") == l16:
            return list(site.get("layers", []))
    return None


def wanted_layers(served):
    """The layers to ask for: the known ones the site serves, one set of walls.

    Building walls raised to the roofline replace the boxes' walls.
    Footprints remain as mask data even where their outlines are not drawn.
    """
    layers = [layer for layer in LAYERS if layer in served]
    if "wall_surfaces" in layers and "box_walls" in layers:
        layers.remove("box_walls")
    # Keep footprints for the player's building-interior mask, even with walls.
    return layers


def accepted_scan(l16, scan_id=None, base=None):
    """The site model's accepted scan and its registration record.

    A site with several accepted scans (several sensor positions) needs the
    scene's own named; nothing here can tell which position a scene was
    recorded from.
    """
    model = grpc("SiteService/GetSiteModel", {"l16_token": l16}, base)
    scans = model.get("scans", [])
    if scan_id:
        scans = [s for s in scans if s.get("scanId") == scan_id]
    if not scans:
        raise NoPose(f"no accepted pose for {l16}")
    if len(scans) > 1:
        ids = ", ".join(s.get("scanId", "?") for s in scans)
        raise RuntimeError(f"{l16} has {len(scans)} accepted scans; name one: {ids}")
    scan = scans[0]
    records = grpc(
        "RegistrationService/ListRegistrationRecords", {"l16_token": l16}, base
    ).get("records", [])
    record = next((r for r in records if r.get("recordId") == scan.get("recordId")), {})
    return model, scan, record


def site_sources(l16, base=None):
    """Gerardus's sources for the site, as the file's sources member.

    The URI is left out: Gerardus records where it read a file, which is a
    path on its own machine.
    """
    site = grpc("SiteService/GetSite", {"l16_token": l16}, base)
    site = site.get("site", site)
    sources = {}
    for source in site.get("sources", []):
        source_id = REFERENCE_SOURCES.get(source.get("layer"))
        if not source_id or source_id in sources:
            continue
        entry = {
            "id": source_id,
            "kind": "source_data",
            "name": source.get("name"),
            "version": source.get("version"),
            "terms": source.get("licence"),
            "attribution": source.get("attribution"),
            "retrieved_at": source.get("retrievedAt"),
            "sha256": source.get("sha256"),
        }
        # An omitted member means not recorded, never empty.
        sources[source_id] = {k: v for k, v in entry.items() if v}
    sources["scans"] = {
        "id": "scans",
        "kind": "sensor",
        "name": "The settled background of this site's LiDAR survey, registered"
        " against 3DEP; the features fitted to it by Gerardus",
        # D-34 has not set the licence of the survey data yet.
        "terms": "not yet set",
    }
    return site, sources


def site_geometry(l16, layers, base=None):
    query = urllib.parse.urlencode({"layers": ",".join(layers), "frame": "site"})
    collection = http_json(f"/api/v1/sites/{l16}/geojson?{query}", base)
    if collection.get("frame") != "site":
        raise RuntimeError(f"{l16}: Gerardus sent frame {collection.get('frame')!r}")
    return collection


# =============================================================================
# THE POSE
# =============================================================================


def rotation(q):
    """The rotation matrix of a unit quaternion {w, x, y, z}."""
    w, x, y, z = (float(q.get(k, 0.0)) for k in ("w", "x", "y", "z"))
    norm = math.sqrt(w * w + x * x + y * y + z * z)
    if norm == 0:
        raise ValueError("zero quaternion")
    w, x, y, z = w / norm, x / norm, y / norm, z / norm
    return (
        (1 - 2 * (y * y + z * z), 2 * (x * y - z * w), 2 * (x * z + y * w)),
        (2 * (x * y + z * w), 1 - 2 * (x * x + z * z), 2 * (y * z - x * w)),
        (2 * (x * z - y * w), 2 * (y * z + x * w), 1 - 2 * (x * x + y * y)),
    )


def into_sensor(pose):
    """A function taking a site-frame position to the sensor frame: R^T (p - t)."""
    R = rotation(pose["rotation"])
    t = tuple(float(pose["translationM"].get(k, 0.0)) for k in ("x", "y", "z"))

    def apply(p):
        d = (p[0] - t[0], p[1] - t[1], (p[2] if len(p) > 2 else 0.0) - t[2])
        return [R[0][c] * d[0] + R[1][c] * d[1] + R[2][c] * d[2] for c in range(3)]

    return apply


def heading_deg(pose):
    """The azimuth of sensor +Y, clockwise from site north, in [0, 360)."""
    R = rotation(pose["rotation"])
    east, north = R[0][1], R[1][1]
    return math.degrees(math.atan2(east, north)) % 360


def sigma_horizontal_m(pose):
    covariance = pose.get("covariance") or []
    if len(covariance) < 8:
        return None
    return math.sqrt(max(0.0, covariance[0]) + max(0.0, covariance[7]))


# =============================================================================
# THE FEATURES
# =============================================================================


def map_coordinates(coordinates, apply):
    """Every position of a GeoJSON coordinates array, transformed and rounded."""
    if coordinates and isinstance(coordinates[0], (int, float)):
        return [round(v, 2) + 0.0 for v in apply(coordinates)]
    return [map_coordinates(c, apply) for c in coordinates]


def positions(coordinates):
    if coordinates and isinstance(coordinates[0], (int, float)):
        yield coordinates
        return
    for c in coordinates:
        yield from positions(c)


def sources_of(layer, feature_source):
    if feature_source:
        words = feature_source.replace("+", ";").split(";")
        ids = [SOURCE_WORDS[w] for w in words if w in SOURCE_WORDS]
        if ids:
            return sorted(set(ids))
    return list(LAYERS[layer][1])


def number(value):
    try:
        v = float(value)
    except (TypeError, ValueError):
        return None
    return v if math.isfinite(v) else None


def scene_features(collection, apply, radius_m=RADIUS_M, include_registration=False):
    """The features within radius_m of the sensor, in its frame, properties pared."""
    kept = []
    for feature in collection.get("features", []):
        props = feature.get("properties", {})
        layer = props.get("layer")
        if layer not in LAYERS:
            continue
        # Registered facade patches inform the placement check, not public context.
        if not include_registration and (
            layer == "wall_planes"
            or (layer == "observed" and props.get("kind") == "facade")
        ):
            continue
        if layer == "observed" and props.get("kind") not in OBSERVED_KINDS:
            continue
        geometry = feature.get("geometry") or {}
        if geometry.get("type") not in (
            "LineString",
            "Polygon",
            "MultiLineString",
            "MultiPolygon",
        ):
            continue
        coordinates = map_coordinates(geometry.get("coordinates", []), apply)
        clip = (
            2 * radius_m
            if layer in BUILDING_LAYERS and not include_registration
            else radius_m
        )
        if not any(math.hypot(p[0], p[1]) <= clip for p in positions(coordinates)):
            continue
        out = {
            "layer": layer,
            "kind": props.get("kind", ""),
            "evidence_status": LAYERS[layer][0],
            "source_ids": sources_of(layer, props.get("source")),
        }
        if props.get("state"):
            out["state"] = props["state"]
        for key in KEPT_PROPERTIES:
            v = number(props.get(key))
            if v is not None:
                out[key] = round(v, 2)
        kept.append(
            {
                "type": "Feature",
                "id": feature.get("id") or props.get("id"),
                "geometry": {"type": geometry["type"], "coordinates": coordinates},
                "properties": out,
            }
        )
    order = list(LAYERS)
    kept.sort(key=lambda f: order.index(f["properties"]["layer"]))
    return kept


# =============================================================================
# THE PLACEMENT CHECK
# =============================================================================


def edge_samples(features, step=0.5, limit=CHECK_SAMPLES):
    """Points along the edges of the walls and kerbs, at most `limit` of them."""
    samples = []
    for f in features:
        layer = f["properties"]["layer"]
        if layer not in CHECK_LAYERS:
            continue
        coordinates = f["geometry"]["coordinates"]
        lines = [coordinates] if f["geometry"]["type"] == "LineString" else coordinates
        for line in lines:
            for a, b in zip(line, line[1:]):
                n = max(1, int(math.dist(a, b) / step))
                for i in range(n):
                    k = i / n
                    samples.append(tuple(a[c] + (b[c] - a[c]) * k for c in range(3)))
    if len(samples) > limit:
        stride = len(samples) / limit
        samples = [samples[int(i * stride)] for i in range(limit)]
    return samples


def occupancy(points, voxel=CHECK_VOXEL_M):
    return {
        (math.floor(p[0] / voxel), math.floor(p[1] / voxel), math.floor(p[2] / voxel))
        for p in points
    }


def agreement(samples, occupied, dx=0.0, dy=0.0, dyaw_deg=0.0, voxel=CHECK_VOXEL_M):
    """The share of samples with a background point in their voxel or the next.

    The neighbourhood is one voxel each way: two voxels each way scores a
    site with few walls as well moved as placed (embarcadero-folsom, 90
    samples, October 2026). The perturbation turns the samples about the
    sensor's vertical and moves them, which is the same as moving the pose
    the other way.
    """
    if not samples:
        return 0.0
    c, s = math.cos(math.radians(dyaw_deg)), math.sin(math.radians(dyaw_deg))
    near = 0
    reach = range(-1, 2)
    for x, y, z in samples:
        px, py = c * x - s * y + dx, s * x + c * y + dy
        i, j, k = math.floor(px / voxel), math.floor(py / voxel), math.floor(z / voxel)
        if any(
            (i + a, j + b, k + d) in occupied
            for a in reach
            for b in reach
            for d in reach
        ):
            near += 1
    return near / len(samples)


def placement_check(features, background_points):
    """Whether the pose fits this scene better than the pose moved does."""
    samples = edge_samples(features)
    occupied = occupancy(background_points)
    at_pose = agreement(samples, occupied)
    moved = {
        f"{dx:+g}m_e{dy:+g}m_n{dyaw:+g}deg": agreement(samples, occupied, dx, dy, dyaw)
        for dx, dy, dyaw in PERTURBATIONS
    }
    best_moved = max(moved.values()) if moved else 0.0
    return {
        "method": "share of wall and kerb edge samples with a settled-background"
        f" point in the same or a neighbouring {CHECK_VOXEL_M:g} m voxel, at the"
        " pose and at the pose moved 2 m or turned 5 degrees; the pose passes"
        f" when it scores highest, on {CHECK_MIN_SAMPLES} samples or more",
        "samples": len(samples),
        "at_pose": round(at_pose, 3),
        "moved": {k: round(v, 3) for k, v in moved.items()},
        "passed": len(samples) >= CHECK_MIN_SAMPLES and at_pose > best_moved,
    }


def read_background(path):
    with open(path, "rb") as fh:
        raw = fh.read()
    if raw[:2] == b"\x1f\x8b":
        raw = gzip.decompress(raw)
    return json.loads(raw)


# =============================================================================
# THE FILE
# =============================================================================


def scene_cell(site):
    """The scene's S2 L16 token and title, from the scene data."""
    with open(SCENES_DATA) as fh:
        data = json.load(fh)
    for entry in data.get("sites", []):
        if entry.get("id") == site:
            token = ((entry.get("cells") or {}).get("site") or {}).get("token")
            return token, entry.get("title", site)
    return None, site


def ground_mesh(grid, apply, radius_m=RADIUS_M):
    """Triangulate measured cells, preserving holes, then place by the accepted pose."""
    nx, ny = grid["nx"], grid["ny"]
    heights = grid["heights"]
    if len(heights) != nx * ny or grid["cell_m"] <= 0:
        raise RuntimeError("invalid Gerardus ground grid")
    vertices = {}
    for j in range(ny):
        for i in range(nx):
            h = heights[j * nx + i]
            if h is None or not math.isfinite(h):
                continue
            p = apply(
                [grid["x0"] + i * grid["cell_m"], grid["y0"] + j * grid["cell_m"], h]
            )
            if math.hypot(p[0], p[1]) <= radius_m:
                vertices[j * nx + i] = [round(v, 2) + 0.0 for v in p]
    positions_out, indices, used = [], [], {}
    for j in range(ny - 1):
        for i in range(nx - 1):
            quad = [j * nx + i, j * nx + i + 1, (j + 1) * nx + i + 1, (j + 1) * nx + i]
            present = [k for k in quad if k in vertices]
            triangles = (
                [present[:3], [present[0], present[2], present[3]]]
                if len(present) == 4
                else [present] if len(present) == 3 else []
            )
            for triangle in triangles:
                for k in triangle:
                    if k not in used:
                        used[k] = len(positions_out)
                        positions_out.append(vertices[k])
                    indices.append(used[k])
    return {
        "positions": positions_out,
        "indices": indices,
        "evidence_status": "observed",
        "source_ids": ["3dep", "scans"],
        "cell_m": grid["cell_m"],
    }


def build_collection(site, title, assets, base=None, scan_id=None, radius_m=RADIUS_M):
    """The scene's vectors, or NoPose when the site has no accepted pose."""
    l16, _ = scene_cell(site)
    if not l16:
        raise NoPose(f"{site} is not located; it has no S2 cell")
    served = served_layers(l16, base)
    if served is None:
        raise NoPose(f"Gerardus does not serve {l16}")
    model, scan, record = accepted_scan(l16, scan_id, base)
    pose = scan["pose"]
    gerardus_site, sources = site_sources(l16, base)
    layers = wanted_layers(served)
    collection = site_geometry(l16, layers, base)
    features = scene_features(collection, into_sensor(pose), radius_m)

    background_path = os.path.join(assets, "background", "background.json.gz")
    background = read_background(background_path)
    check_features = scene_features(
        collection, into_sensor(pose), radius_m, include_registration=True
    )
    check = placement_check(check_features, background.get("points", []))
    if not check["passed"]:
        raise RuntimeError(
            f"{site}: the pose does not fit this scene's background"
            f" ({check['at_pose']} at the pose, {max(check['moved'].values())} moved)"
        )

    try:
        ground = ground_mesh(
            http_json(f"/api/v1/sites/{l16}/ground", base),
            into_sensor(pose),
            2 * radius_m,
        )
    except RuntimeError as error:
        # An unprepared site can legitimately have no ground surface.
        if "HTTP 404" not in str(error):
            raise
        ground = None
    used = sorted(
        {s for f in features for s in f["properties"]["source_ids"]}
        | (set(ground["source_ids"]) if ground else set())
    )
    summary = record.get("summary", {})
    placement = {
        "method": "the inverse of Gerardus's accepted pose for the site's scan:"
        " p_sensor = R^T (p_site - t)",
        "record_id": scan.get("recordId", ""),
        "registration_state": scan.get("state", "")
        .replace("REGISTRATION_STATE_", "")
        .lower(),
        "pose_source": pose.get("source", "").replace("POSE_SOURCE_", "").lower(),
        "site_frame_id": pose.get("siteFrameId", ""),
        "sensor_frame_id": pose.get("sensorFrameId", ""),
        "model_id": model.get("modelId", ""),
        "bundle_id": model.get("bundleId", ""),
        "reviewed_at": record.get("reviewedAt", ""),
        "heading_deg": round(heading_deg(pose), 2),
        # Site +Z is true vertical; express it in the scene's sensor axes.
        "up_sensor": [round(v, 10) for v in rotation(pose["rotation"])[2]],
        "check": check,
    }
    placement = {k: v for k, v in placement.items() if v != ""}
    sigma = summary.get("sigmaHorizontalM", sigma_horizontal_m(pose))
    if sigma is not None:
        placement["sigma_horizontal_m"] = round(float(sigma), 3)
    if summary.get("sigmaHeadingDeg") is not None:
        placement["sigma_heading_deg"] = round(float(summary["sigmaHeadingDeg"]), 3)

    counts = {}
    for f in features:
        layer = f["properties"]["layer"]
        counts[layer] = counts.get(layer, 0) + 1
    return {
        "type": "FeatureCollection",
        "version": VERSION,
        "export": "vectors",
        "site": site,
        "title": title,
        "purpose": "Positions and extents of stationary street features (kerbs,"
        " walls, trees and ground) as measured or inferred at the"
        " stated times, placed in this survey's sensor frame for context. Not a"
        " survey, plan or specification; a feature not shown is not asserted"
        " absent.",
        "crs": "the scene's sensor frame (coordinate_frame); not WGS 84",
        "coordinate_frame": background.get(
            "coordinate_frame",
            {"frame_id": pose.get("sensorFrameId", ""), "reference_frame": "ENU"},
        ),
        "units": {"length": "m"},
        "vertical_reference": "sensor_origin",
        "coordinate_rounding_m": ROUNDING_M,
        "clip_radius_m": radius_m,
        "building_clip_radius_m": 2 * radius_m,
        "ground_clip_radius_m": 2 * radius_m,
        "placement": placement,
        "sources": [sources[s] for s in used if s in sources],
        "layers": counts,
        "coverage_notes": [
            f"Building context and ground extend to {2 * radius_m:g} m where Gerardus has them; other features extend to {radius_m:g} m.",
            "Most features are Gerardus's proposals (state proposed), derived and not"
            " yet accepted by an operator.",
            "Walls raised to a roofline and tree crowns are inferred past the returns.",
            "Experimental markings, street furniture, and registered facade patches are omitted.",
            "Ground triangles stop at missing cells; no flat surface is substituted.",
            "Heights are measured from the sensor's origin, as the scene's are.",
        ],
        "generated_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "ground": ground,
        "features": features,
    }


def write_collection(collection, assets):
    """Write vectors/vectors.geojson.gz under assets, whole or not at all."""
    out = os.path.join(assets, OUT_DIR)
    os.makedirs(out, exist_ok=True)
    data = json.dumps(collection, separators=(",", ":")).encode()
    fd, tmp = tempfile.mkstemp(dir=out, prefix=".vectors-")
    try:
        with os.fdopen(fd, "wb") as fh:
            fh.write(gzip.compress(data, mtime=0))
        # mkstemp makes the file private; it is published beside the others.
        os.chmod(tmp, 0o644)
        os.replace(tmp, os.path.join(out, OUT_FILE))
    except BaseException:
        if os.path.exists(tmp):
            os.unlink(tmp)
        raise
    return os.path.join(out, OUT_FILE)


def export_scene_vectors(site, title, assets, mode="auto", base=None, say=log):
    """Write a scene's vectors into its assets, as publish-scenes.py asks.

    mode is SCENE_VECTORS: "off" writes nothing; "auto" writes them where it
    can and says why where it cannot; "required" fails the scene when
    Gerardus cannot be read or the pose fails its check. A site with no
    accepted pose has no vectors in any mode. Returns the record id the
    vectors were placed by, or None.
    """
    if mode == "off":
        return None
    try:
        collection = build_collection(site, title, assets, base)
    except NoPose as why:
        say(f"  vectors: none ({why})")
        return None
    except (Unreachable, RuntimeError) as why:
        if mode == "required":
            raise
        say(f"  vectors: none ({why})")
        return None
    path = write_collection(collection, assets)
    layers = ", ".join(f"{k} {v}" for k, v in collection["layers"].items())
    check = collection["placement"]["check"]
    say(
        f"  vectors: {len(collection['features'])} features ({layers}),"
        f" {os.path.getsize(path) / 1024:.0f} KiB; placement check"
        f" {check['at_pose']} at the pose vs {max(check['moved'].values())} moved"
    )
    return collection["placement"]["record_id"]


# =============================================================================
# COMMAND LINE
# =============================================================================


def published_sites():
    with open(SCENES_DATA) as fh:
        data = json.load(fh)
    return [
        (s["id"], s.get("title", s["id"]))
        for s in data.get("sites", [])
        if os.path.isdir(os.path.join(SCENES, s["id"], "assets"))
    ]


def status(sites, base=None):
    for site, _ in sites:
        l16, _ = scene_cell(site)
        if not l16:
            print(f"  unlocated   {site}")
            continue
        try:
            _, scan, _ = accepted_scan(l16, base=base)
            print(f"  pose        {site:24s} {l16}  {scan.get('recordId', '')}")
        except NoPose:
            print(f"  no pose     {site:24s} {l16}")
        except RuntimeError as why:
            print(f"  ambiguous   {site:24s} {why}")


def main():
    parser = argparse.ArgumentParser(description=__doc__.split("\n\n", 1)[0])
    parser.add_argument(
        "sites", nargs="*", help="scene ids; default is every published scene"
    )
    parser.add_argument(
        "--status", action="store_true", help="say which sites have a pose"
    )
    parser.add_argument("--gerardus", default=GERARDUS, help="Gerardus's address")
    parser.add_argument(
        "--scan", help="the accepted scan to place by, where a site has several"
    )
    parser.add_argument(
        "--radius", type=float, default=RADIUS_M, help="clip radius, metres"
    )
    args = parser.parse_args()

    known = dict(published_sites())
    wanted = args.sites or list(known)
    unknown = [s for s in wanted if s not in known]
    if unknown:
        parser.error(f"no published scene: {', '.join(unknown)}")
    sites = [(s, known[s]) for s in wanted]
    if args.status:
        status(sites, args.gerardus)
        return 0

    failed = 0
    for site, title in sites:
        assets = os.path.join(SCENES, site, "assets")
        log(f"{site}:")
        try:
            collection = build_collection(
                site, title, assets, args.gerardus, args.scan, args.radius
            )
        except NoPose as why:
            log(f"  none: {why}")
            continue
        except (Unreachable, RuntimeError) as why:
            log(f"  FAILED: {why}")
            failed += 1
            continue
        path = write_collection(collection, assets)
        check = collection["placement"]["check"]
        log(
            f"  {len(collection['features'])} features, {os.path.getsize(path) / 1024:.0f} KiB;"
            f" check {check['at_pose']} at the pose vs {max(check['moved'].values())} moved"
        )
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
