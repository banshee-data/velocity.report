import assert from "node:assert/strict";
import { readdirSync, readFileSync, existsSync } from "node:fs";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import { s2 } from "s2js";

import {
  buildCitySites,
  cityEnuFromGeodetic,
  recordingDate,
} from "../city-sites.mjs";

const HERE = path.dirname(fileURLToPath(import.meta.url));
const REPO = path.resolve(HERE, "..", "..", "..");
const FIXTURES = path.join(REPO, "tools", "s2-archive", "testdata", "city-view");
const SCENES = path.join(REPO, "public_html", "src", "scenes");
const CITY_INDEX = path.join(REPO, "public_html", "src", "city", "sites.json");

const readJson = (file) => JSON.parse(readFileSync(file, "utf8"));

function cellCentre(token) {
  const ll = s2.LatLng.fromPoint(s2.cellid.point(s2.cellid.fromToken(token)));
  return [(ll.lat * 180) / Math.PI, (ll.lng * 180) / Math.PI];
}

function* tiles(node) {
  if (node.extras?.s2) yield node;
  for (const child of node.children ?? []) yield* tiles(child);
}

function publishedSidecars() {
  return readdirSync(SCENES)
    .filter((site) => existsSync(path.join(SCENES, site, "assets", "manifest.json")))
    .map((site) => ({
      site,
      sidecar: readJson(path.join(SCENES, site, "assets", "placement.json")),
    }));
}

// The export privacy case for the index (D-34, Gerardus ADR-0011): placement
// and provenance only. Every key the index may hold, at any depth.
const INDEX_KEYS = new Set([
  "comment", "version", "generated_by", "city_frame", "format", "origin",
  "lat_deg", "lon_deg", "navd88_m", "horizontal_datum", "vertical_datum",
  "navd88_zero_ellipsoidal_m", "matrix_layout", "site_count", "placed_count",
  "sites", "id", "title", "published", "cells", "l10", "l13", "l16", "scene",
  "page", "assets", "placement", "captures", "recorded_on", "state",
  "record_id", "registration_state", "heading_deg", "sensor_to_city",
  "marker", "source", "city_enu_m",
]); // prettier-ignore
const FORBIDDEN = ["track", "trail", "speed", "address", "street", "name", "owner"];

function keysOf(value, where = "", out = []) {
  if (Array.isArray(value)) {
    for (const v of value) keysOf(v, where, out);
  } else if (value && typeof value === "object") {
    for (const [k, v] of Object.entries(value)) {
      out.push([`${where}.${k}`, k]);
      keysOf(v, `${where}.${k}`, out);
    }
  }
  return out;
}

test("the city frame puts every Gerardus tile origin on its cell centre", () => {
  const tileset = readJson(path.join(FIXTURES, "tileset.json"));
  let count = 0;
  for (const tile of tiles(tileset.root)) {
    const [lat, lon] = cellCentre(tile.extras.s2);
    const got = cityEnuFromGeodetic(lat, lon, 0);
    tile.extras.origin_city_enu_m.forEach((want, i) => {
      assert.ok(Math.abs(got[i] - want) < 0.0005, `${tile.extras.s2}[${i}]`);
    });
    count += 1;
  }
  assert.ok(count > 10);
});

test("each sidecar's site-to-city origin agrees with an independent computation", () => {
  for (const { site, sidecar } of publishedSidecars()) {
    if (sidecar.state !== "placed") continue;
    const l16 = sidecar.frames.site.l16;
    const [lat, lon] = cellCentre(l16);
    const want = cityEnuFromGeodetic(lat, lon, 0);
    const m = sidecar.site_to_city.matrix;
    want.forEach((w, i) => {
      assert.ok(Math.abs(m[i][3] - w) < 0.0005, `${site} site_to_city[${i}]`);
    });
  }
});

test("the committed index holds placement and provenance only", () => {
  const index = readJson(CITY_INDEX);
  for (const [where, key] of keysOf(index)) {
    assert.ok(INDEX_KEYS.has(key), `index carries ${where}`);
    for (const word of FORBIDDEN) {
      assert.ok(!key.toLowerCase().includes(word), `index carries ${where}`);
    }
  }
});

test("the committed index copies each scene's placement exactly", () => {
  const index = readJson(CITY_INDEX);
  const byId = new Map(index.sites.map((s) => [s.id, s]));
  const sidecars = publishedSidecars();
  assert.equal(sidecars.length, index.sites.filter((s) => s.published).length);
  for (const { site, sidecar } of sidecars) {
    const entry = byId.get(site);
    assert.ok(entry, `${site} missing from the index`);
    assert.equal(entry.placement.state, sidecar.state, site);
    if (sidecar.state === "placed") {
      assert.equal(entry.placement.record_id, sidecar.record.record_id);
      assert.deepEqual(entry.placement.sensor_to_city, sidecar.sensor_to_city.matrix);
    } else {
      assert.equal(entry.placement.sensor_to_city, null);
    }
  }
  assert.equal(
    index.placed_count,
    index.sites.filter((s) => s.placement.state === "placed").length,
  );
});

const PLACED = {
  site: "a",
  state: "placed",
  record: { record_id: "gms:1", registration_state: "registered" },
  heading_deg: 12.5,
  sensor_to_city: {
    matrix: [
      [1, 0, 0, 5],
      [0, 1, 0, 6],
      [0, 0, 1, 7],
      [0, 0, 0, 1],
    ],
  },
  marker: { source: "accepted_pose", city_enu_m: [5, 6, 7] },
};

test("a placed scene is copied, a recorded site gets an eye-read marker", () => {
  const index = buildCitySites({
    sites: [
      {
        id: "a",
        title: "A at B",
        page: "/surveys/a/",
        published: true,
        capture: { raw_path: "raw/lidar/x.pcapng" },
        cells: { area: { token: "808581" }, neighbourhood: { token: "808580f4" }, site: { token: "808580f47" } },
      },
      {
        id: "b",
        title: "C at D",
        page: null,
        published: false,
        position: { lat: 37.76, lon: -122.44 },
      },
    ],
    placements: new Map([["a", PLACED]]),
    startsMs: new Map([["a", Date.parse("2026-09-02T20:20:00Z")]]),
  });
  const [a, b] = index.sites;
  assert.equal(a.placement.state, "placed");
  assert.deepEqual(a.placement.sensor_to_city, PLACED.sensor_to_city.matrix);
  assert.deepEqual(a.cells, { l10: "808581", l13: "808580f4", l16: "808580f47" });
  assert.equal(a.scene.placement, "/surveys/a/assets/placement.json");
  assert.deepEqual(a.captures, ["raw/lidar/x.pcapng"]);
  assert.equal(a.recorded_on, "2026-09-02");
  assert.equal(b.scene, null);
  assert.equal(b.placement.state, "not_placed");
  assert.equal(b.placement.sensor_to_city, null);
  assert.deepEqual(b.placement.marker.city_enu_m, [0, 0, 0]);
  assert.equal(index.placed_count, 1);
});

test("a published scene without a sidecar is refused", () => {
  assert.throws(
    () =>
      buildCitySites({
        sites: [{ id: "a", page: "/surveys/a/", published: true }],
        placements: new Map(),
      }),
    /placement\.json/,
  );
});

test("recording dates are San Francisco dates", () => {
  // 03:30 UTC on the 3rd is the evening of the 2nd in San Francisco.
  assert.equal(recordingDate(Date.parse("2026-09-03T03:30:00Z")), "2026-09-02");
  assert.equal(recordingDate(undefined), null);
});
