import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { gunzipSync } from "node:zlib";
import { streetCourseWaypoints } from "./steering-line.mjs";

const assets = new URL("../../public_html/src/sf/", import.meta.url);
const read = (path) => readFileSync(new URL(path, assets));
const manifest = read("tiles/manifest.json");
const manifestSha256 = createHash("sha256").update(manifest).digest("hex");
const { progressBaseSha256 } = JSON.parse(manifest).terrainRebake;
const { placementSha256 } = JSON.parse(read("quest-route.json"));
const tile = JSON.parse(gunzipSync(read("tiles/808f7e0c.json.gz")));
const [ox, oy] = tile.o;
const street = tile.r.find(
  ([name, , , points]) =>
    tile.n[name] === "Corbett Ave" && points[0] === -2195 && points[1] === 728,
);
assert.ok(street, "Corbett Avenue west of Cuesta Court is missing");
const raw = street[3];
const points = [];
for (let i = 0; i < raw.length; i += 2)
  points.push([ox + raw[i] / 10, oy + raw[i + 1] / 10]);

// The course crosses the Public Works 220M marker's nearest point on Corbett.
for (const [id, start, end] of [
  ["corbett-at-cuesta-forward", 14.18, 104.18],
  ["corbett-at-cuesta-reverse", 104.18, 14.18],
]) {
  console.log(
    JSON.stringify({
      id,
      manifestSha256,
      progressBaseSha256,
      placementSha256,
      finishOnStreet: true,
      expectedSurfaces: [],
      expectedEnclosed: [],
      maxDips: 0,
      maxAirborneMetres: 0,
      waypoints: streetCourseWaypoints(points, start, end, 9),
    }),
  );
}
