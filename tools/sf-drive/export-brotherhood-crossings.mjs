import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { gunzipSync } from "node:zlib";
import { steeringLine } from "./steering-line.mjs";

// Both directed street lines are below the Junipero Serra decks.
const assets = new URL("../../public_html/src/sf/", import.meta.url);
const read = (path) => readFileSync(new URL(path, assets));
const manifest = read("tiles/manifest.json");
const manifestSha256 = createHash("sha256").update(manifest).digest("hex");
const { progressBaseSha256 } = JSON.parse(manifest).terrainRebake;
const { placementSha256 } = JSON.parse(read("quest-route.json"));
const tile = JSON.parse(gunzipSync(read("tiles/808f7c4c.json.gz")));

for (const [id, direction, firstX, finish, maxAirborneMetres] of [
  ["brotherhood-westbound", 1, 3644, 75, 12],
  ["brotherhood-eastbound", 2, 3921, 84, 0],
]) {
  const street = tile.r.find(
    ([name, , dir, points]) =>
      tile.n[name] === "Brotherhood Way" &&
      dir === direction &&
      points[0] === firstX,
  );
  assert.ok(street, id);
  const raw = street[3];
  const points = [];
  for (let i = 0; i < raw.length; i += 2)
    points.push([tile.o[0] + raw[i] / 10, tile.o[1] + raw[i + 1] / 10, 0]);
  if (direction === 2) points.reverse();
  const stations = [0];
  for (let i = 1; i < points.length; i++) {
    const [x, y] = points[i];
    const [a, b] = points[i - 1];
    stations.push(stations.at(-1) + Math.hypot(x - a, y - b));
  }
  assert.ok(stations.at(-1) > finish + 1, id);
  let segment = 1;
  const sampled = [];
  for (let station = 1; station <= finish + 1; station++) {
    while (stations[segment] < station) segment++;
    const start = stations[segment - 1];
    const t = (station - start) / (stations[segment] - start);
    const [a, b] = points[segment - 1];
    const [x, y] = points[segment];
    sampled.push([a + (x - a) * t, b + (y - b) * t, 0]);
  }
  const waypoints = steeringLine(sampled, { lane: -1.45 })
    .slice(1, -1)
    .map(([x, y], i) => [x, y, i, 12]);
  console.log(
    JSON.stringify({
      id,
      manifestSha256,
      progressBaseSha256,
      placementSha256,
      finishOnStreet: true,
      expectedSurfaces: [],
      maxAirborneMetres,
      maxDips: 0,
      waypoints,
    }),
  );
}
