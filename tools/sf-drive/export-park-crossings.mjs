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

for (const { id, token, street, starts, station } of [
  {
    id: "jfk-windmill-crossing",
    token: "808587bc",
    street: "John F Kennedy Dr",
    starts: [
      [622, 1004],
      [-468, 1902],
    ],
    station: 168.02155323833796,
  },
  {
    id: "mason-quartermaster-crossing",
    token: "808586d4",
    street: "Mason St",
    starts: [[-1920, -707]],
    station: 73.88431446375097,
  },
]) {
  const tile = JSON.parse(gunzipSync(read(`tiles/${token}.json.gz`)));
  const [ox, oy] = tile.o;
  const points = [];
  for (const [east, north] of starts) {
    const row = tile.r.find(
      ([name, , , xy]) =>
        tile.n[name] === street && xy[0] === east && xy[1] === north,
    );
    assert.ok(row, `Mapped street is missing for ${id}`);
    const xy = row[3];
    if (points.length)
      assert.deepEqual(points.at(-1), [ox + east / 10, oy + north / 10]);
    for (let i = points.length ? 2 : 0; i < xy.length; i += 2)
      points.push([ox + xy[i] / 10, oy + xy[i + 1] / 10]);
  }
  const start = station - 46;
  const end = station + 46;
  for (const [direction, first, finish] of [
    ["forward", start, end],
    ["reverse", end, start],
  ]) {
    console.log(
      JSON.stringify({
        id: `${id}-${direction}`,
        manifestSha256,
        progressBaseSha256,
        placementSha256,
        finishOnStreet: true,
        expectedSurfaces: [],
        expectedEnclosed: [],
        expectNoScrapes: true,
        maxDips: 0,
        maxAirborneMetres: 0,
        waypoints: streetCourseWaypoints(points, first, finish, 9),
      }),
    );
  }
}
