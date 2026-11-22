// Replay 50 qualifying airborne events on the practice crest and the pinned SF map.
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { gunzipSync } from "node:zlib";
import { createFlightTimeline } from "../../public_html/src/js/sf-airtime.js";
import {
  applyCityDriveScenery,
  BAY_BRIDGE_CITY_DRIVE,
  buildCityGameDrive,
} from "../../public_html/src/js/sf-city-road-drive.js";
import {
  buildCollisionIndex,
  collisionAt,
  contactStep,
  groundAtWorld,
  nearestRoad,
  tileAt,
  tilesAround,
} from "../../public_html/src/js/sf-drive-core.js";
import {
  fallFromRoad,
  highwayRailSegments,
  leaveRoadSide,
  roadBoundary,
  roadLandingAt,
  strikeHighwayRail,
} from "../../public_html/src/js/sf-highway-edge.js";
import { driveOnRoadSurface } from "../../public_html/src/js/sf-road-contact.js";
import { soloSurface } from "../../public_html/src/js/sf-solo.js";

const directory = new URL("../../public_html/src/sf/tiles/", import.meta.url);
const map = JSON.parse(readFileSync(new URL("manifest.json", directory)));
const driveBytes = readFileSync(new URL("../gamedrives.json", directory));
assert.equal(
  createHash("sha256").update(driveBytes).digest("hex"),
  map.roadBake.driveSha256,
);
const bundle = JSON.parse(driveBytes);
assert.equal(bundle.base_manifest_sha256, map.roadBake.baseSha256);
const drive = bundle.drives.find((entry) => entry.id === BAY_BRIDGE_CITY_DRIVE);
assert.ok(drive);
const tileSizes = new Map(map.tiles.map((tile) => [tile.t, tile.bytes]));
const cache = new Map();
function load(token) {
  if (!cache.has(token)) {
    const bytes = readFileSync(new URL(`${token}.json.gz`, directory));
    assert.equal(bytes.length, tileSizes.get(token), `Invalid tile ${token}`);
    const tile = JSON.parse(gunzipSync(bytes));
    applyCityDriveScenery(tile, drive.scene_exclusions?.[token]);
    cache.set(token, { tile, index: buildCollisionIndex(tile) });
  }
  return cache.get(token);
}
const local = (point, margin) =>
  tilesAround(map.tiles, point.x, point.y, margin).want.map(load);
const ground = (x, y) =>
  groundAtWorld([load(tileAt(map.tiles, x, y).t).tile], x, y, true);
const dt = 1 / 120;
const results = [];

function replay(
  group,
  seed,
  car,
  steps,
  advance,
  prepare,
  accept = () => true,
) {
  const records = [];
  const timeline = createFlightTimeline((record) => records.push(record));
  let env, takeoff;
  for (let tick = 1; tick <= steps; tick++) {
    const next = advance(car, tick);
    if (!next || next.blocked) return;
    const now = tick * dt * 1000;
    if (next.grounded === false && !env) {
      env = prepare(car);
      takeoff = car;
    }
    if (env)
      timeline.step(car, next, dt, now, { ...env, impact: next.roadImpact });
    car = next;
    if (!env || !car.grounded) continue;
    const card = timeline.visible(now)[0];
    if (!card || !accept(car, takeoff)) {
      timeline.reset();
      records.length = 0;
      env = null;
      continue;
    }
    assert.equal(card.label, car.roadImpact ? "Impact" : "Touchdown");
    assert.equal(records.length, car.roadImpact ? 0 : 1);
    assert.equal(timeline.visible(now + 1999)[0], card);
    assert.equal(timeline.visible(now + 2000).length, 0);
    assert.ok(
      card.trace.every(({ s, z }) => Number.isFinite(s) && Number.isFinite(z)),
    );
    assert.ok(card.trace.every(({ s }, i, trace) => !i || s >= trace[i - 1].s));
    assert.equal(card.trace.at(-1).z, car.z);
    const ahead = card.profile.filter(({ s }) => s >= 0 && s <= card.right);
    const drawn = ahead.filter(
      ({ z, ground }) => Number.isFinite(z) || Number.isFinite(ground),
    );
    assert.ok(drawn.length >= 2, `no visible elevation in ${group} ${seed}`);
    return {
      group,
      seed,
      seconds: +card.seconds.toFixed(3),
      drop_m: +(takeoff.z - car.z).toFixed(2),
      profile: drawn.length,
      terrain: drawn.filter(({ ground }) => Number.isFinite(ground)).length,
      road: drawn.filter(({ z }) => Number.isFinite(z)).length,
      impact: car.roadImpact ?? null,
    };
  }
}

for (const sign of [-1, 1]) {
  for (const speed of [44, 47, 50, 53, 58]) {
    const start = {
      x: -42 * sign,
      y: 0,
      z: soloSurface(-42 * sign, 0).height,
      heading: sign > 0 ? 0 : Math.PI,
      v: speed,
      grounded: true,
    };
    const row = replay(
      "practice crest",
      `${sign}:${speed}`,
      start,
      490,
      (car) => contactStep(car, { throttle: 1 }, dt, soloSurface),
      ({ x }) => ({
        road: {
          centreline: [-1700, 0, 1900, 0],
          origin: [0, 0],
          segment: 0,
          fraction: (x + 170) / 360,
          heading: 0,
        },
        heightAt: (x, y) => soloSurface(x, y).height,
      }),
    );
    assert.ok(
      row,
      `practice crest should produce a visible flight at ${sign}:${speed}`,
    );
    results.push(row);
  }
}

for (const place of ["castro", "lombard", "columbus", "downtown", "mission"]) {
  const origin = map.places.find(({ id }) => id === place);
  const cells = local(origin, 350);
  const tiles = cells.map(({ tile }) => tile);
  let count = 0;
  for (const tile of tiles) {
    roadRow: for (const row of tile.r) {
      if (!row[4].length || count >= 4) continue;
      const xy = row[3];
      for (let j = 0; j + 3 < xy.length && count < 4; j += 2) {
        const dx = (xy[j + 2] - xy[j]) / 10,
          dy = (xy[j + 3] - xy[j + 1]) / 10;
        const length = Math.hypot(dx, dy);
        const hx = dx / length,
          hy = dy / length;
        for (
          let s = Math.min(length / 2, 5);
          length >= 0.5 && s < length;
          s += 12
        ) {
          const x = tile.o[0] + xy[j] / 10 + hx * s;
          const y = tile.o[1] + xy[j + 1] / 10 + hy * s;
          const z = ground(x, y);
          const rear = ground(x - 4 * hx, y - 4 * hy);
          const front = ground(x + 7 * hx, y + 7 * hy);
          if (
            [z, rear, front].includes(null) ||
            (front - z) / 7 - (z - rear) / 4 > -0.035
          )
            continue;
          const v = 62 + count * 1.5;
          const sample = (x, y) => ({ height: ground(x, y), drivable: true });
          const start = {
            x,
            y,
            z,
            heading: Math.atan2(dy, dx),
            v,
            grounded: true,
            vz:
              (((ground(x + 1.35 * hx, y + 1.35 * hy) ?? z) -
                (ground(x - 1.35 * hx, y - 1.35 * hy) ?? z)) *
                v) /
              2.7,
          };
          const rowResult = replay(
            "SF street",
            `${place}:${x.toFixed(1)},${y.toFixed(1)}`,
            start,
            840,
            (car) => contactStep(car, { throttle: 1 }, dt, sample, 1.35),
            (car) => {
              const road = nearestRoad(tiles, car.x, car.y);
              return {
                road: road?.distance <= road?.width ? road : null,
                heightAt: ground,
                tiles,
              };
            },
          );
          if (rowResult) {
            results.push(rowResult);
            count++;
            continue roadRow;
          }
        }
      }
    }
  }
  assert.equal(count, 4, `four crests near ${place}`);
}

const { surfaces, exitStart } = buildCityGameDrive(drive, ground);
const exit = surfaces.find(({ id }) => id === "fremont-exit");
exit.boundary = roadBoundary(exit.contact);
exit.railSegments = highwayRailSegments(exit);
const ramp = ({ index, speed, start = null, straightTicks = 0 }) => {
  const { position } = exit.profile[index];
  const a = exit.profile[index - 1].position;
  const b = exit.profile[index + 1].position;
  const car = start || {
    x: position[0],
    y: position[1],
    heading: Math.atan2(b[1] - a[1], b[0] - a[0]),
  };
  const zone = local(car, 100);
  const indexes = zone.map(({ index }) => index);
  const tiles = zone.map(({ tile }) => tile);
  return replay(
    "Fremont left edge",
    `${index}:${speed}`,
    {
      ...car,
      z: exit.heightAt(car.x, car.y),
      v: speed,
      surfaceId: exit.id,
      grounded: true,
    },
    650,
    (car, tick) => {
      const input = { throttle: 1, steer: tick > straightTicks ? 1 : 0 };
      let next;
      if (car.leavingRoad) {
        next = fallFromRoad(car, dt, (x, y, z) =>
          roadLandingAt(x, y, z, ground, surfaces),
        );
      } else {
        next =
          strikeHighwayRail(car, input, dt, exit) ||
          driveOnRoadSurface(car, input, dt, exit);
        if (next.blocked) next = leaveRoadSide(car, input, dt, exit);
      }
      return next && !collisionAt(indexes, next, next.z) ? next : null;
    },
    () => ({ heightAt: ground, deckAt: exit.heightAt }),
    (car) => {
      const road = nearestRoad(tiles, car.x, car.y);
      return road && road.distance <= road.width;
    },
  );
};
const exitIndex = exit.profile.findIndex(
  ({ position }) => position[0] === exitStart.x,
);
const gentle = ramp({
  index: exitIndex,
  start: exitStart,
  speed: 0,
  straightTicks: 95,
});
assert.ok(
  gentle && !gentle.impact,
  "the player can drive off the open side and land on the street",
);
results.push(gentle);
let count = 1;
for (const index of [
  9, 13, 31, 53, 71, 93, 115, 137, 161, 187, 211, 237, 261, 285, 295, 339, 343,
  345, 347, 349,
]) {
  for (const speed of [5, 10, 15]) {
    if (count >= 20) break;
    const row = ramp({ index, speed });
    if (row) {
      results.push(row);
      count++;
    }
  }
}
assert.equal(count, 20);
assert.equal(results.length, 50);
for (const group of new Set(results.map((row) => row.group))) {
  const rows = results.filter((row) => row.group === group);
  console.log(
    group,
    JSON.stringify({
      flights: rows.length,
      terrainFallbacks: rows.filter(({ terrain }) => terrain > 0).length,
      profilesWithRoad: rows.filter(({ road }) => road > 0).length,
      hardLandings: rows.filter(({ impact }) => !!impact).length,
      minSeconds: Math.min(...rows.map(({ seconds }) => seconds)),
      maxSeconds: Math.max(...rows.map(({ seconds }) => seconds)),
    }),
  );
}
if (process.argv.includes("--examples"))
  console.log(JSON.stringify(results, null, 2));
