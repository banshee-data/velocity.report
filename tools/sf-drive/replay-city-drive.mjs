// Exercises the published route against the local game tiles and physics.
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { gunzipSync } from "node:zlib";
import {
  applyCityDriveScenery,
  BAY_BRIDGE_CITY_DRIVE,
  buildCityGameDrive,
  cityDriveApproach,
  cityDriveControls,
} from "../../public_html/src/js/sf-city-road-drive.js";
import {
  buildCollisionIndex,
  CAR_HALF_LENGTH,
  collisionAt,
  groundAtWorld,
  tileAt,
  tilesAround,
} from "../../public_html/src/js/sf-drive-core.js";
import {
  highwayRailSegments,
  strikeHighwayRail,
} from "../../public_html/src/js/sf-highway-edge.js";
import {
  driveOnRoadNetwork,
  indexRoadNetwork,
} from "../../public_html/src/js/sf-road-network.js";

const directory = new URL("../../public_html/src/sf/tiles/", import.meta.url);
const map = JSON.parse(readFileSync(new URL("manifest.json", directory)));
const driveBytes = readFileSync(new URL("../gamedrives.json", directory));
assert.equal(
  createHash("sha256").update(driveBytes).digest("hex"),
  map.roadBake.driveSha256,
);
const bundle = JSON.parse(driveBytes);
assert.equal(bundle.base_manifest_sha256, map.roadBake.baseSha256);
const baseDrive = bundle.drives.find(
  (entry) => entry.id === BAY_BRIDGE_CITY_DRIVE,
);
assert.ok(baseDrive);
const start = process.argv[2];
const published = cityDriveApproach(baseDrive, start);
if (start) assert.equal(published.id, start, "Unknown Bay Bridge start");
const tileSizes = new Map(map.tiles.map((tile) => [tile.t, tile.bytes]));
const loaded = new Map();

function tile(token) {
  if (!loaded.has(token)) {
    const raw = readFileSync(new URL(`${token}.json.gz`, directory));
    assert.equal(raw.length, tileSizes.get(token), `Invalid tile ${token}`);
    const world = JSON.parse(gunzipSync(raw));
    applyCityDriveScenery(world, published.scene_exclusions?.[token]);
    loaded.set(token, { world, index: buildCollisionIndex(world) });
  }
  return loaded.get(token);
}

const drive = buildCityGameDrive(published, (x, y) =>
  groundAtWorld([tile(tileAt(map.tiles, x, y).t).world], x, y, true),
);
const graph = indexRoadNetwork(drive.topology.transitions);
graph.add(drive.topology);
const surfaces = new Map(
  drive.surfaces.map((s) => [
    s.id,
    { ...s, railSegments: highwayRailSegments(s) },
  ]),
);
const step = 1 / 120;
const entrance = surfaces.get("fourth-street-entrance");
for (const fraction of [0.25, 0.75]) {
  const { a, b, nx, ny } =
    entrance.railSegments[Math.floor(fraction * entrance.railSegments.length)];
  const setback = CAR_HALF_LENGTH + 0.06;
  const x = (a[0] + b[0]) / 2 - setback * nx;
  const y = (a[1] + b[1]) / 2 - setback * ny;
  const z = entrance.heightAt(x, y);
  assert.notEqual(z, null);
  const heading = Math.atan2(ny, nx);
  const impact = strikeHighwayRail(
    { x, y, z, heading, v: drive.start.initialSpeed, grounded: true },
    {},
    step,
    entrance,
  );
  assert.equal(impact?.edgeContact, "bounce");
  assert.equal(impact.roadImpact, undefined);
  assert.ok(impact.grounded && !impact.blocked);
  assert.ok(impact.v * Math.cos(impact.heading - heading) < 0);
  assert.equal(
    collisionAt([tile(tileAt(map.tiles, x, y).t).index], impact, impact.z),
    null,
  );
}
console.log("Fourth Street entrance: both 50 mph side impacts rebounded");

let car = { ...drive.start, v: drive.start.initialSpeed, grounded: true };
car.z = surfaces.get(car.surfaceId).heightAt(car.x, car.y);
const seen = [car.surfaceId];
let nearby = [];
let finished = false;
for (let tick = 0; tick < 540 / step; tick++) {
  if (tick % 25 === 0)
    nearby = tilesAround(map.tiles, car.x, car.y, 50).want.map(
      (token) => tile(token).index,
    );
  const input = cityDriveControls(drive, car);
  assert.ok(input, `Lost route contact on ${car.surfaceId}`);
  if (input.finished) {
    finished = true;
    console.log(`Completed in ${(tick * step).toFixed(2)} s of game time`);
    break;
  }
  const surface = surfaces.get(car.surfaceId);
  const next =
    strikeHighwayRail(car, input, step, surface) ||
    driveOnRoadNetwork(car, input, step, surface, surfaces, graph, () => false);
  const at = `${next.surfaceId} (${next.x.toFixed(2)}, ${next.y.toFixed(2)})`;
  assert.ok(!next.blocked && next.grounded, `Lost road contact at ${at}`);
  assert.ok(!next.roadImpact, `Hit a highway rail at ${at}`);
  const hit = collisionAt(nearby, next, next.z);
  assert.ok(!hit, `Hit ${hit?.kind || "scenery"} at ${at}`);
  if (next.surfaceId !== car.surfaceId) {
    seen.push(next.surfaceId);
    console.log(next.surfaceId, `${next.z.toFixed(2)} m NAVD88`);
  }
  car = next;
}
assert.deepEqual(
  seen,
  published.sections.map(({ id }) => id),
);
assert.ok(finished, "The car did not reach the end of the city route");
