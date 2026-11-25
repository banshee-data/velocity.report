// Screen all packaged bridge, ramp and tunnel contact with the game chassis.
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { gunzipSync } from "node:zlib";
import { buildBakedRoadContact } from "../../public_html/src/js/sf-baked-road-contact.js";
import {
  CAR_HALF_LENGTH,
  CAR_HALF_WIDTH,
  drivingHull,
  obstacleContact,
} from "../../public_html/src/js/sf-chassis-contact.js";
import {
  buildCollisionIndex,
  groundAtWorld,
} from "../../public_html/src/js/sf-drive-core.js";
import {
  contactGeometry,
  roadSupportsCar,
} from "../../public_html/src/js/sf-road-contact.js";
import { deckClearanceIndex } from "../../public_html/src/js/sf-tree-clearance.js";
import {
  auditBakedRoadLinks,
  auditBakedRoadSpans,
} from "./baked-road-link-audit.mjs";

const stationStep = 2;
const lateralStep = 0.75;
const portalTolerance = 0.15;
const layerTolerance = 0.4;
const round = (value) => Math.round(value * 1000) / 1000;
const options = process.argv.slice(2);
const driveLinks = options.includes("--drive-links");
const driveSpans = options.includes("--drive-spans");
const [directory = "public_html/src/sf/tiles"] = options.filter(
  (option) => option !== "--drive-links" && option !== "--drive-spans",
);
const raw = readFileSync(join(directory, "manifest.json"));
const manifest = JSON.parse(raw);
const tiles = manifest.tiles.map(({ t, bytes }) => {
  const rawTile = readFileSync(join(directory, `${t}.json.gz`));
  assert.equal(rawTile.length, bytes, t);
  const tile = JSON.parse(gunzipSync(rawTile));
  assert.equal(tile.t, t);
  return tile;
});

const pieces = new Map();
for (const tile of tiles) {
  for (const road of tile.playableRoads ?? []) {
    if (pieces.has(road.id))
      assert.deepEqual(pieces.get(road.id), road, road.id);
    else pieces.set(road.id, road);
  }
}
const contacts = [...pieces.values()].map((road) => {
  const contact = buildBakedRoadContact(road);
  assert.ok(contact, road.id);
  return contact;
});

const clearance = deckClearanceIndex();
for (const { surface } of contacts) {
  if (surface.kind === "tunnel") continue;
  const geometry = contactGeometry(surface.contact, false);
  clearance.add(geometry, surface.kind);
  geometry.dispose();
}
// Combine the same collision buckets the game consults across its loaded tiles.
// Expanded city assets stay in memory for this offline audit.
const collisions = new Map();
for (const tile of tiles) {
  for (const [key, shapes] of buildCollisionIndex(
    tile,
    tile.t,
    undefined,
    clearance.ceiling,
  )) {
    const bucket = collisions.get(key);
    if (bucket) bucket.push(...shapes);
    else collisions.set(key, shapes);
  }
}

function sample(surface, station, i) {
  const { profile, contact } = surface;
  const a = profile[i - 1],
    b = profile[i];
  const u = (station - a.s_m) / (b.s_m - a.s_m);
  const blend = (p, q) => p.map((value, axis) => value + u * (q[axis] - value));
  const [x, y, z] = blend(a.position, b.position);
  const [left, right] = [0, 1].map((side) =>
    blend(contact.vertices[(i - 1) * 2 + side], contact.vertices[i * 2 + side]),
  );
  const [dx, dy] = [left[0] - right[0], left[1] - right[1]];
  const length = Math.hypot(dx, dy);
  const nx = dx / length,
    ny = dy / length;
  return {
    x,
    y,
    z,
    nx,
    ny,
    heading: Math.atan2(-nx, ny),
    left: (left[0] - x) * nx + (left[1] - y) * ny,
    right: (x - right[0]) * nx + (y - right[1]) * ny,
  };
}

function screen(surface) {
  const { profile, heightAt } = surface;
  const deckSupport =
    surface.kind === "bridge_deck" || surface.kind === "ramp"
      ? heightAt
      : undefined;
  const length = profile.at(-1).s_m;
  const margin = Math.min(CAR_HALF_LENGTH, length / 2);
  const intervals = Math.ceil((length - 2 * margin) / stationStep);
  let profileIndex = 1;
  let reachable = null;
  let closedStation;
  let disconnectedStation;
  let centreHits = 0;
  let centreUnsupported = 0;
  let peakGrade = 0;
  let poses = 0;
  const obstacles = new Map();
  for (let i = 1; i < profile.length; i++) {
    const a = profile[i - 1],
      b = profile[i];
    peakGrade = Math.max(
      peakGrade,
      Math.abs(b.position[2] - a.position[2]) / (b.s_m - a.s_m),
    );
  }
  for (let j = 0; j <= intervals; j++) {
    const station = margin + ((length - 2 * margin) * j) / (intervals || 1);
    while (profile[profileIndex].s_m < station) profileIndex++;
    const p = sample(surface, station, profileIndex);
    const lanes = new Set();
    const first = -Math.floor((p.right - CAR_HALF_WIDTH) / lateralStep);
    const last = Math.floor((p.left - CAR_HALF_WIDTH) / lateralStep);
    for (let lane = first; lane <= last; lane++) {
      poses++;
      const x = p.x + p.nx * lane * lateralStep;
      const y = p.y + p.ny * lane * lateralStep;
      const z = heightAt(x, y);
      const car = { x, y, z, heading: p.heading };
      if (
        z === null ||
        Math.abs(z - p.z) > layerTolerance ||
        !roadSupportsCar(heightAt, car)
      ) {
        if (lane === 0) centreUnsupported++;
        continue;
      }
      const hit = obstacleContact(
        [collisions],
        drivingHull(car),
        undefined,
        deckSupport,
      );
      if (hit) {
        if (lane === 0) {
          centreHits++;
          const tree = hit.tree && [hit.tree.tile, hit.tree.index];
          const key = tree?.join("/") ?? hit.kind;
          if (!obstacles.has(key)) {
            obstacles.set(key, {
              station: round(station),
              pose: [x, y, z].map(round),
              hit: [hit.kind, ...[hit.x, hit.y, hit.z].map(round)],
              ...(tree && { tree }),
            });
          }
        }
      } else lanes.add(lane);
    }
    if (!lanes.size) closedStation ??= round(station);
    if (reachable) {
      reachable = new Set(
        [...lanes].filter(
          (lane) =>
            reachable.has(lane - 1) ||
            reachable.has(lane) ||
            reachable.has(lane + 1),
        ),
      );
    } else reachable = lanes;
    if (!reachable.size) disconnectedStation ??= round(station);
  }
  return {
    length: round(length),
    peakGrade: round(100 * peakGrade),
    stations: intervals + 1,
    poses,
    centreHits,
    centreUnsupported,
    closedStation,
    disconnectedStation,
    ...(obstacles.size && { centreObstacles: [...obstacles.values()] }),
  };
}

const unmatchedPortals = [];
let groundPortals = 0;
const roads = [];
for (const { surface, portals } of contacts) {
  roads.push({ id: surface.id, kind: surface.kind, ...screen(surface) });
  groundPortals += portals.length;
  for (const { x, y, z } of portals) {
    const ground = groundAtWorld(tiles, x, y, true);
    if (ground === null || Math.abs(ground - z) > portalTolerance)
      unmatchedPortals.push({ id: surface.id, position: [x, y, z], ground });
  }
}

const endpoints = new Map(
  contacts.flatMap(({ endpoints }) =>
    endpoints.map(({ id, position }) => [id, position]),
  ),
);
const joins = manifest.roadBake?.playableTransitions ?? [];
const openJoins = joins.filter(({ from, to, status }) => {
  const a = endpoints.get(from),
    b = endpoints.get(to);
  return (
    status !== "joined" ||
    !a ||
    !b ||
    Math.hypot(...a.map((value, i) => value - b[i])) > 0.002
  );
});

const kinds = Object.fromEntries(
  ["bridge_deck", "ramp", "tunnel"].map((kind) => [
    kind,
    roads.filter((road) => road.kind === kind).length,
  ]),
);
const flag = (road) =>
  road.disconnectedStation !== undefined ||
  road.centreHits ||
  road.centreUnsupported ||
  road.peakGrade > 25;
const driving = driveLinks && auditBakedRoadLinks(contacts, joins, tiles);
const traversals =
  driveSpans && auditBakedRoadSpans(contacts, joins, collisions);
console.log(
  JSON.stringify(
    {
      manifestSha256: createHash("sha256").update(raw).digest("hex"),
      tiles: tiles.length,
      settings: { stationStep, lateralStep, portalTolerance, layerTolerance },
      surfaces: roads.length,
      kinds,
      stations: roads.reduce((sum, road) => sum + road.stations, 0),
      poses: roads.reduce((sum, road) => sum + road.poses, 0),
      groundPortals,
      joins: joins.length,
      openJoins,
      surfacesWithCentreHits: roads.filter((road) => road.centreHits).length,
      surfacesWithCentreSupportGaps: roads.filter(
        (road) => road.centreUnsupported,
      ).length,
      surfacesWithClosedCrossSections: roads.filter(
        (road) => road.closedStation !== undefined,
      ).length,
      surfacesWithoutConnectedLane: roads.filter(
        (road) => road.disconnectedStation !== undefined,
      ).length,
      unmatchedPortals,
      flagged: roads.filter(flag),
      ...(driving && { driving }),
      ...(traversals && { traversals }),
    },
    null,
    2,
  ),
);
if (
  (driving &&
    (driving.directedJoins.failures.length ||
      driving.groundHandoffs.failures.length)) ||
  (traversals && traversals.failures.length)
)
  process.exitCode = 1;
