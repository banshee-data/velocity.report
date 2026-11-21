// Manual CPU comparison for the PR #1096 repaint regression. Raw timings go to
// stdout, not Git. This is not a browser/GPU or physical-device frame-rate test.
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { gunzipSync } from "node:zlib";
import { performance } from "node:perf_hooks";
import { tilesAround } from "../../public_html/src/js/sf-drive-core.js";
import { tileRegion } from "../../public_html/src/js/sf-drive-surface.js";
import {
  createRoadMarkingCache,
  roadDashPolygons,
  roadMarkingTriangles,
} from "../../public_html/src/js/sf-road-markings.js";

const root = new URL("../../public_html/src/sf/tiles/", import.meta.url);
const manifest = JSON.parse(readFileSync(new URL("manifest.json", root)));
const centre = manifest.tiles.find((tile) => tile.t === "808580c4");
const entries = tilesAround(manifest.tiles, centre.x, centre.y).want.map(
  (token) => {
    const row = manifest.tiles.find((tile) => tile.t === token);
    const tile = JSON.parse(
      gunzipSync(readFileSync(new URL(`${token}.json.gz`, root))),
    );
    tile.half = row.half;
    const shapes = tile.r.map((road) => ({
      dashed: !!road[4].length && road[2] === 0,
      centre: Array.from({ length: road[3].length / 2 }, (_, i) => [
        tile.o[0] + road[3][i * 2] / 10,
        tile.o[1] + road[3][i * 2 + 1] / 10,
      ]),
    }));
    return {
      token,
      tile,
      region: tileRegion(row, manifest.tiles),
      markings: roadDashPolygons(shapes),
    };
  },
);
const neighbours = new Map(
  entries.map((target) => [
    target.token,
    entries.filter(
      (source) =>
        Math.hypot(
          source.tile.o[0] - target.tile.o[0],
          source.tile.o[1] - target.tile.o[1],
        ) <=
        source.tile.half + target.tile.half + 100,
    ),
  ]),
);
let clips = 0;
const cache = createRoadMarkingCache((...args) => {
  clips++;
  return roadMarkingTriangles(...args);
});
for (const entry of entries) cache.set(entry.token, entry);
const full = (target) =>
  new Float32Array(
    roadMarkingTriangles(
      target.tile,
      target.region,
      neighbours.get(target.token).flatMap((source) => source.markings),
    ),
  );
const cached = (target) =>
  cache.build(
    target.token,
    neighbours.get(target.token).map((source) => source.token),
  );
let vertices = 0;
for (const target of entries) {
  const expected = full(target),
    actual = cached(target);
  assert.deepEqual(actual, expected);
  vertices += actual.length / 3;
}
const measure = (fn) => {
  for (const target of entries) fn(target); // one warm-up, followed by five measured passes
  const samples = [];
  for (let pass = 0; pass < 5; pass++) {
    const start = performance.now();
    for (const target of entries) fn(target);
    samples.push(performance.now() - start);
  }
  return {
    samples_ms: samples,
    median_ms: samples.toSorted((a, b) => a - b)[2],
  };
};
const fullResult = measure(full),
  coldClips = clips,
  cachedResult = measure(cached);
assert.equal(
  clips,
  coldClips,
  "warm composition must never reclip cached pairs",
);
console.log(
  JSON.stringify(
    {
      node: process.version,
      centre: centre.t,
      tiles: entries.map((e) => e.token),
      vertices,
      full: fullResult,
      cached: cachedResult,
      warm_clip_calls: clips - coldClips,
    },
    null,
    2,
  ),
);
