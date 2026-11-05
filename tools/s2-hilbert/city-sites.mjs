/**
 * The city site index, public_html/src/city/sites.json: every site with data
 * and where it sits in the city frame of Gerardus's city tiles.
 *
 * The web city view and the macOS visualiser both read this one file. It is
 * derived from what is already published: the page data's sites and each
 * scene's assets/placement.json, which tools/s2-archive/scene-vectors.py
 * writes from the accepted pose. Nothing here is computed from a pose; the
 * matrices are copied from the sidecar, where they were worked out once in
 * double precision. The only geodesy is a marker for a site that has no
 * sidecar (recorded, not yet published), from its eye-read position, using
 * the same GRS80 east-north-up as the sidecar (tools/s2-archive/city_frame.py).
 *
 * Placement and provenance only: no tracks, people, addresses or building
 * names. docs/ui/city-view.md describes the format.
 */

export const CITY_INDEX_VERSION = 1;
export const CITY_FORMAT = "gerardus-city-tiles-1";
export const CITY_ORIGIN = { lat_deg: 37.76, lon_deg: -122.44, navd88_m: 0 };
export const NAVD88_ZERO_ELLIPSOIDAL_M = -32.2;
export const MATRIX_LAYOUT =
  "4x4, row-major, metres: p_out = M[0:3][0:3] * p_in + M[0:3][3]; the last row is [0, 0, 0, 1]";
// Recording dates are local to San Francisco, as the field notes are.
const RECORDING_TIME_ZONE = "America/Los_Angeles";

const GRS80_A = 6378137.0;
const GRS80_F = 1 / 298.257222101;
const E2 = GRS80_F * (2 - GRS80_F);
const rad = (deg) => (deg * Math.PI) / 180;

export function geodeticToEcef(latDeg, lonDeg, h = 0) {
  const sl = Math.sin(rad(latDeg));
  const cl = Math.cos(rad(latDeg));
  const so = Math.sin(rad(lonDeg));
  const co = Math.cos(rad(lonDeg));
  const n = GRS80_A / Math.sqrt(1 - E2 * sl * sl);
  return [(n + h) * cl * co, (n + h) * cl * so, (n * (1 - E2) + h) * sl];
}

/** A geodetic position (NAD83(2011), NAVD88 as ellipsoidal) in the city frame. */
export function cityEnuFromGeodetic(latDeg, lonDeg, h = 0) {
  const o = geodeticToEcef(CITY_ORIGIN.lat_deg, CITY_ORIGIN.lon_deg, 0);
  const p = geodeticToEcef(latDeg, lonDeg, h);
  const d = [p[0] - o[0], p[1] - o[1], p[2] - o[2]];
  const sl = Math.sin(rad(CITY_ORIGIN.lat_deg));
  const cl = Math.cos(rad(CITY_ORIGIN.lat_deg));
  const so = Math.sin(rad(CITY_ORIGIN.lon_deg));
  const co = Math.cos(rad(CITY_ORIGIN.lon_deg));
  return [
    -so * d[0] + co * d[1],
    -sl * co * d[0] - sl * so * d[1] + cl * d[2],
    cl * co * d[0] + cl * so * d[1] + sl * d[2],
  ];
}

const round = (value, places) => {
  const k = 10 ** places;
  return Math.round(value * k) / k + 0;
};

/** YYYY-MM-DD of a start time in milliseconds, in San Francisco. */
export function recordingDate(startMs) {
  if (!Number.isFinite(startMs)) return null;
  return new Intl.DateTimeFormat("en-CA", {
    timeZone: RECORDING_TIME_ZONE,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).format(new Date(startMs));
}

function cellTokens(cells) {
  if (!cells) return null;
  return {
    l10: cells.area?.token ?? null,
    l13: cells.neighbourhood?.token ?? null,
    l16: cells.site?.token ?? null,
  };
}

function eyeReadMarker(position) {
  if (!position || position.lat == null || position.lon == null) return null;
  const enu = cityEnuFromGeodetic(position.lat, position.lon, 0);
  return {
    source: "eye_read",
    lat_deg: position.lat,
    lon_deg: position.lon,
    city_enu_m: [round(enu[0], 3), round(enu[1], 3), 0],
  };
}

/** The index entry's placement, from a scene's sidecar or, without one, a marker. */
function placementOf(site, sidecar) {
  if (sidecar?.state === "placed") {
    return {
      state: "placed",
      record_id: sidecar.record?.record_id ?? null,
      registration_state: sidecar.record?.registration_state ?? null,
      heading_deg: sidecar.heading_deg ?? null,
      sensor_to_city: sidecar.sensor_to_city?.matrix ?? null,
      marker: {
        source: sidecar.marker?.source ?? "accepted_pose",
        city_enu_m: sidecar.marker?.city_enu_m ?? null,
      },
    };
  }
  const marker = sidecar ? sidecar.marker : eyeReadMarker(site.position);
  return {
    state: "not_placed",
    record_id: null,
    registration_state: null,
    heading_deg: null,
    sensor_to_city: null,
    marker: marker
      ? {
          source: marker.source,
          lat_deg: marker.lat_deg,
          lon_deg: marker.lon_deg,
          city_enu_m: marker.city_enu_m,
        }
      : null,
  };
}

/**
 * Build the index.
 *
 * `sites` are the page data's sites (published and recorded); `placements`
 * maps a published site's id to its sidecar; `startsMs` maps it to its
 * recording's start. A published scene without a sidecar is an error: the
 * sidecar is written with the scene, so its absence means the scene was
 * published by hand or half-published.
 */
export function buildCitySites({ sites, placements, startsMs = new Map() }) {
  const entries = sites.map((site) => {
    const sidecar = placements.get(site.id) ?? null;
    if (site.published && !sidecar) {
      throw new Error(
        `${site.id} is published without assets/placement.json; run tools/s2-archive/scene-vectors.py --placement ${site.id}`,
      );
    }
    const scene = site.published
      ? {
          page: site.page,
          assets: `${site.page}assets/`,
          placement: `${site.page}assets/placement.json`,
        }
      : null;
    return {
      id: site.id,
      title: site.title,
      published: site.published === true,
      cells: cellTokens(site.cells),
      scene,
      captures: site.capture?.raw_path ? [site.capture.raw_path] : [],
      recorded_on: recordingDate(startsMs.get(site.id)),
      placement: placementOf(site, sidecar),
    };
  });
  return {
    comment: [
      "Every site with data and its place in the city frame of Gerardus's city tiles.",
      "Generated by tools/s2-hilbert/generate-scene-map.mjs from the page data and each",
      "scene's assets/placement.json. Do not edit: CI regenerates it and fails on any difference.",
      "Placement and provenance only. Each site is its own recording: nothing here joins",
      "one site's road users to another's.",
    ],
    version: CITY_INDEX_VERSION,
    generated_by: "tools/s2-hilbert/generate-scene-map.mjs",
    city_frame: {
      format: CITY_FORMAT,
      origin: CITY_ORIGIN,
      horizontal_datum: "NAD83(2011)",
      vertical_datum: "NAVD88",
      navd88_zero_ellipsoidal_m: NAVD88_ZERO_ELLIPSOIDAL_M,
    },
    matrix_layout: MATRIX_LAYOUT,
    site_count: entries.length,
    placed_count: entries.filter((e) => e.placement.state === "placed").length,
    sites: entries,
  };
}
