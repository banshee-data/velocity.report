import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { gunzipSync } from "node:zlib";

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
const segments = [];
let station = 0;
for (let i = 2; i < raw.length; i += 2) {
  const [x, y] = [ox + raw[i - 2] / 10, oy + raw[i - 1] / 10];
  const [dx, dy] = [ox + raw[i] / 10 - x, oy + raw[i + 1] / 10 - y];
  const length = Math.hypot(dx, dy);
  if (length > 0) segments.push({ station, x, y, dx, dy, length });
  station += length;
}

function at(distance) {
  const { station, x, y, dx, dy, length } = segments.find(
    (segment) => distance <= segment.station + segment.length,
  );
  const t = (distance - station) / length;
  return [x + t * dx, y + t * dy];
}

// The course crosses the Public Works 220M marker's nearest point on Corbett.
for (const [id, start, end] of [
  ["corbett-at-cuesta-forward", 14.18, 104.18],
  ["corbett-at-cuesta-reverse", 104.18, 14.18],
]) {
  const intervals = Math.ceil(Math.abs(end - start) / 2.3);
  const waypoints = Array.from({ length: intervals + 1 }, (_, i) => {
    const progress = ((end - start) * i) / intervals;
    const s = start + progress;
    const [x, y] = at(s);
    const [ax, ay] = at(s - 0.75);
    const [bx, by] = at(s + 0.75);
    const right = ((end > start ? 1 : -1) * 1.5) / Math.hypot(bx - ax, by - ay);
    return [
      x + right * (by - ay),
      y - right * (bx - ax),
      Math.abs(progress),
      9,
    ];
  });
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
      waypoints,
    }),
  );
}
