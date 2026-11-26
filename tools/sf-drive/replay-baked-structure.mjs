// Drive the bundled 5th Street ramp with the same switches as free driving.
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { gunzipSync } from "node:zlib";
import { buildBakedRoadContact } from "../../public_html/src/js/sf-baked-road-contact.js";
import {
  buildCityGameDrive,
  cityDriveControls,
} from "../../public_html/src/js/sf-city-road-drive.js";
import {
  buildCollisionIndex,
  contactStep,
  groundAtWorld,
  tileAt,
  tilesAround,
} from "../../public_html/src/js/sf-drive-core.js";
import {
  drivingHull,
  obstacleContact,
} from "../../public_html/src/js/sf-chassis-contact.js";
import {
  driveOnRoadNetwork,
  indexRoadNetwork,
} from "../../public_html/src/js/sf-road-network.js";
import { driveThroughRoadPortal } from "../../public_html/src/js/sf-road-portal.js";

const directory = process.argv[2] || "public_html/src/sf/tiles";
const manifest = JSON.parse(readFileSync(join(directory, "manifest.json")));
const ramps = manifest.roadBake.playableTransitions;
assert.ok(ramps?.length, "no baked contact transitions");
const networks = indexRoadNetwork(ramps);
networks.add({ endpoints: [], transitions: ramps });
const loaded = new Map();
const surfaces = new Map();
const pieces = new Map();
const dt = 1 / 120;

function load(token) {
  if (loaded.has(token)) return loaded.get(token);
  const raw = readFileSync(join(directory, `${token}.json.gz`));
  const tile = JSON.parse(gunzipSync(raw));
  const entry = { tile, obstacles: buildCollisionIndex(tile, token) };
  loaded.set(token, entry);
  for (const road of tile.playableRoads ?? []) {
    const existing = pieces.get(road.id);
    if (existing) {
      assert.deepEqual(road, existing);
      continue;
    }
    const { surface, endpoints, portals } = buildBakedRoadContact(road);
    pieces.set(road.id, road);
    surfaces.set(road.id, { ...surface, portals });
    networks.add({ endpoints, transitions: [] });
  }
  return entry;
}

const owner = (x, y) => tileAt(manifest.tiles, x, y).t;
const ground = (x, y) => groundAtWorld([load(owner(x, y)).tile], x, y, true);
const [entryId, approachId, deckId] = [
  "sf-contact/i280-fifth-south-entry",
  "sf-contact/i280-fifth-south-to-sixth-merge",
  "sf-contact/i280-king-to-mariposa",
];
load("808f7fd4");
load("808f7fcc");
const ramp = pieces.get(entryId);
const deck = pieces.get(deckId);
assert.ok(ramp && deck);
const approach = pieces.get(approachId);
const sections = approach ? [ramp, approach, deck] : [ramp, deck];
const [start, second] = ramp.points;
const heading = Math.atan2(second[1] - start[1], second[0] - start[0]);
const x = start[0] - 12 * Math.cos(heading);
const y = start[1] - 12 * Math.sin(heading);
const pavement = (east, north) => ({
  height: ground(east, north),
  drivable: true,
});
const course = buildCityGameDrive(
  {
    sections: sections.map((road) => ({
      id: road.id,
      kind: road.kind,
      width_m: road.width,
      positions: road.points,
      speed_mps: 13,
      sources: [],
    })),
  },
  ground,
);

let car = { x, y, heading, z: ground(x, y), grounded: true, v: 5 };
let nearby = [];
const seen = new Set([undefined]);
let atEnd = false;
for (let tick = 0; tick < 17000; tick++) {
  if (tick % 30 === 0)
    nearby = tilesAround(manifest.tiles, car.x, car.y, 60).want.map(
      (token) => load(token).obstacles,
    );
  const input = cityDriveControls(course, {
    ...car,
    grounded: true,
    surfaceId: car.surfaceId || entryId,
  });
  assert.ok(input, JSON.stringify(car));
  if (input.finished) {
    atEnd = true;
    break;
  }
  const surface = surfaces.get(car.surfaceId);
  const next =
    driveThroughRoadPortal(
      car,
      input,
      dt,
      surface
        ? [surface.portals]
        : [...surfaces.values()].map((piece) => piece.portals),
      ground,
    ) ||
    (surface
      ? driveOnRoadNetwork(
          car,
          input,
          dt,
          surface,
          surfaces,
          networks,
          () => false,
        )
      : contactStep(car, input, dt, pavement, 1.35));
  const position = `${next.surfaceId || "street"} at ${next.x.toFixed(2)}, ${next.y.toFixed(2)} and ${next.z.toFixed(2)} m`;
  assert.ok(!next.blocked, `Blocked on ${position}`);
  assert.ok(Math.abs(next.z - car.z) < 0.2, `Altitude jump on ${position}`);
  const hit = obstacleContact(nearby, drivingHull(next));
  assert.ok(!hit, `Hit ${hit?.kind} on ${position}`);
  if (!seen.has(next.surfaceId)) console.log(position);
  seen.add(next.surfaceId);
  car = next;
}
assert.deepEqual([...seen], [undefined, ...sections.map(({ id }) => id)]);
assert.ok(atEnd, "the car did not reach the next mainline junction");
const sample = ramp.points.find(([x, y]) => x > 3692 && y < 1355);
assert.ok(sample[2] > ground(...sample.slice(0, 2)) + 10);
console.log("Reached the next I-280 junction above the original terrain");
