// Export the surveyed northbound right lane for drive-road-audit.cjs.
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { gunzipSync } from "node:zlib";

const assets = new URL("../../public_html/src/sf/", import.meta.url);
const read = (name) => readFileSync(new URL(name, assets));
const manifestBytes = read("tiles/manifest.json");
const manifestSha256 = createHash("sha256").update(manifestBytes).digest("hex");
const manifest = JSON.parse(manifestBytes);
const quest = JSON.parse(read("quest-route.json"));
assert.equal(quest.manifestSha256, manifestSha256);

const tile = JSON.parse(gunzipSync(read("tiles/80858074.json.gz")));
const [east, north] = tile.o;
const road = tile.r.find(
  ([name, , direction, points]) =>
    tile.n[name] === "The Embarcadero" &&
    direction === 1 &&
    north + points[1] / 10 < 3124 &&
    north + points.at(-1) / 10 > 3225,
);
assert.ok(road, "Missing the northbound Embarcadero alignment");
const points = [];
for (let i = 0; i < road[3].length; i += 2) {
  const x = east + road[3][i] / 10;
  const y = north + road[3][i + 1] / 10;
  if (y >= 3124 && y <= 3225) points.push([x, y]);
}

let station = 0;
const waypoints = points.map(([x, y], i) => {
  const [ax, ay] = points[Math.max(0, i - 1)];
  const [bx, by] = points[Math.min(points.length - 1, i + 1)];
  const heading = Math.atan2(by - ay, bx - ax);
  station += Math.hypot(x - ax, y - ay);
  return [x + 3 * Math.sin(heading), y - 3 * Math.cos(heading), station, 8];
});
assert.ok(
  station >= 99,
  "Missing approach or departure for the survey crossing",
);
console.log(
  JSON.stringify({
    id: "embarcadero-northbound-right-lane",
    manifestSha256,
    placementSha256: quest.placementSha256,
    progressBaseSha256: manifest.terrainRebake.progressBaseSha256,
    waypoints,
  }),
);
