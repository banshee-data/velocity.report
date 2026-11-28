// Drive the pinned SF quest courses with the game's public physics functions.
// Run from the repository root: node tools/sf-drive/audit-road-routes.mjs
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { resolve, sep } from "node:path";
import { pathToFileURL } from "node:url";
import { gunzipSync } from "node:zlib";
import {
  advanceTrial,
  buildCollisionIndex,
  contactStep,
  createTrial,
  groundAt,
  nearestRoad,
  tileAt,
  tilesAround,
} from "../../public_html/src/js/sf-drive-core.js";
import {
  drivingHull,
  obstacleContact,
} from "../../public_html/src/js/sf-chassis-contact.js";
import { crashRoadSupport } from "../../public_html/src/js/sf-highway-edge.js";
import { buildBakedRoadContact } from "../../public_html/src/js/sf-baked-road-contact.js";
import { contactGeometry } from "../../public_html/src/js/sf-road-contact.js";
import { driveThroughRoadPortal } from "../../public_html/src/js/sf-road-portal.js";
import {
  driveOnRoadNetwork,
  indexRoadNetwork,
} from "../../public_html/src/js/sf-road-network.js";
import { deckClearanceIndex } from "../../public_html/src/js/sf-tree-clearance.js";
import {
  hardLanding,
  scrapeObstacle,
  seriousImpact,
} from "../../public_html/src/js/sf-crash.js";
import { steeringLine } from "./steering-line.mjs";
import { roadClearanceOffsets } from "./road-clearance-line.mjs";

const exportCourses = process.argv.includes("--export-courses");
const args = process.argv.slice(2).filter((arg) => arg !== "--export-courses");
const assetsPath = args.find((arg) => arg.startsWith("--assets="));
const assets = assetsPath
  ? pathToFileURL(resolve(assetsPath.slice("--assets=".length)) + sep)
  : new URL("../../public_html/src/sf/", import.meta.url);
const alignmentPath = args.find((arg) => arg.startsWith("--alignment-assets="));
const planPath = args.find((arg) => arg.startsWith("--plan="));
const source = planPath
  ? resolve(planPath.slice("--plan=".length))
  : new URL("road-audit-routes.json", import.meta.url);
const bytes = readFileSync(new URL("tiles/manifest.json", assets));
const manifest = JSON.parse(bytes);
const planBytes = readFileSync(source);
const plan = JSON.parse(planBytes);
const quest = JSON.parse(readFileSync(new URL("quest-route.json", assets)));
const digest = (bytes) => createHash("sha256").update(bytes).digest("hex");
assert.equal(digest(bytes), plan.manifestSha256);
assert.equal(quest.manifestSha256, plan.manifestSha256);
assert.ok(plan.routes.length, "the course set is empty");
const ids = new Set(plan.routes.map(({ id }) => id));
assert.equal(ids.size, plan.routes.length);
const chosen = new Set(
  args.filter(
    (arg) => arg !== planPath && arg !== assetsPath && arg !== alignmentPath,
  ),
);
for (const id of chosen) assert.ok(ids.has(id), `Unknown course: ${id}`);
const courseSet = digest(planBytes);
function readTiles(index, directory) {
  return new Map(
    index.tiles.map(({ t, bytes }) => {
      const compressed = readFileSync(new URL(`tiles/${t}.json.gz`, directory));
      assert.equal(compressed.length, bytes, t);
      return [t, JSON.parse(gunzipSync(compressed))];
    }),
  );
}

const tiles = readTiles(manifest, assets);
let alignmentTiles = tiles;
let alignmentManifestSha256;
if (alignmentPath) {
  const directory = pathToFileURL(
    resolve(alignmentPath.slice("--alignment-assets=".length)) + sep,
  );
  const raw = readFileSync(new URL("tiles/manifest.json", directory));
  const alignment = JSON.parse(raw);
  assert.deepEqual(alignment.origin_epsg7131, manifest.origin_epsg7131);
  assert.deepEqual(
    alignment.tiles.map(({ t }) => t),
    manifest.tiles.map(({ t }) => t),
  );
  alignmentTiles = readTiles(alignment, directory);
  alignmentManifestSha256 = digest(raw);
}
const targets = new Map(quest.targets.map((target) => [target.emoji, target]));
const indexes = new Map();
const gradeThreshold = 0.5;
const dipThreshold = 0.5;
const clearanceThreshold = 0.05;
const distance = (a, b) => Math.hypot(a.x - b.x, a.y - b.y);
const angle = (a) => Math.atan2(Math.sin(a), Math.cos(a));
const heading = (a, b) => Math.atan2(b.y - a.y, b.x - a.x);
const clamp = (x, min, max) => Math.min(max, Math.max(min, x));
const round = (n) => Math.round(n * 1000) / 1000;
const blend = (a, b, u) => ({
  x: a.x + (b.x - a.x) * u,
  y: a.y + (b.y - a.y) * u,
});
const ownerOf = (x, y) => tileAt(manifest.tiles, x, y).t;
const heightAt = (x, y) => {
  const tile = tiles.get(ownerOf(x, y));
  return groundAt(tile.g, x - tile.o[0], y - tile.o[1], false);
};
const pieces = new Map();
for (const tile of tiles.values()) {
  for (const road of tile.playableRoads ?? []) {
    if (!pieces.has(road.id)) pieces.set(road.id, road);
  }
}
const contacts = [...pieces.values()].map(buildBakedRoadContact);
const surfaces = new Map(contacts.map(({ surface }) => [surface.id, surface]));
const bakedDecks = deckClearanceIndex();
for (const { surface } of contacts) {
  if (surface.kind === "tunnel") continue;
  const geometry = contactGeometry(surface.contact, false);
  bakedDecks.add(geometry);
  geometry.dispose();
}
const portals = new Map(
  contacts.map(({ surface, portals }) => [surface.id, portals]),
);
const transitions = manifest.roadBake?.playableTransitions ?? [];
const network = indexRoadNetwork(transitions);
network.add({ endpoints: contacts.flatMap((c) => c.endpoints), transitions });
const courseHeight = (road, x, y) =>
  road.surfaceId ? surfaces.get(road.surfaceId).heightAt(x, y) : heightAt(x, y);
const nodes = new Map();
function nodeAt(x, y) {
  const key = `${Math.round(x * 10)},${Math.round(y * 10)}`;
  if (!nodes.has(key)) nodes.set(key, { x, y, edges: [], names: new Set() });
  return nodes.get(key);
}

function roadPoints(tile, road) {
  const xy = road[3];
  const points = [];
  for (let i = 0; i < xy.length; i += 2)
    points.push(nodeAt(tile.o[0] + xy[i] / 10, tile.o[1] + xy[i + 1] / 10));
  return points;
}

function addRoad(tile, road, tunnel = false) {
  const [street, kind, way, , widths] = road;
  if (!widths.length || way === 9) return;
  const name = tile.n[street];
  const points = roadPoints(tile, road);
  for (const point of points) point.names.add(name);
  for (let i = 1; i < points.length; i++) {
    const a = points[i - 1],
      b = points[i];
    const length = distance(a, b);
    if (length < 0.01) continue;
    const data = { length, name, highway: kind === 1 || kind === 6, tunnel };
    if (way !== 2) a.edges.push({ ...data, to: b });
    if (way !== 1) b.edges.push({ ...data, to: a });
  }
}

// An archived alignment can hold the route fixed while the current bake
// supplies all terrain, collision geometry and playable road contact.
for (const tile of alignmentTiles.values())
  for (const road of tile.r) addRoad(tile, road);

// The parent geometry defines the attempted alignment only. Its heights and
// widths never become contact in the rebaked game.
const tunnelEnds = new Map();
const historicalTunnelCourses = plan.routes.some(({ tunnel }) => tunnel);
const archivedTunnelTiles = historicalTunnelCourses
  ? Object.keys(manifest.terrainRebake.removedTunnelRows)
  : [];
for (const token of archivedTunnelTiles) {
  const parent = JSON.parse(
    gunzipSync(
      execFileSync("git", [
        "show",
        `${plan.tunnelSourceCommit}:public_html/src/sf/tiles/${token}.json.gz`,
      ]),
    ),
  );
  const currentTile = tiles.get(token);
  const current = new Set(currentTile.r.map((road) => currentTile.n[road[0]]));
  for (const road of parent.r) {
    const name = parent.n[road[0]];
    if (!name.endsWith("Tunnel") || current.has(name)) continue;
    addRoad(parent, road, true);
    const points = roadPoints(parent, road);
    tunnelEnds.set(name, [points[0], points.at(-1)]);
  }
}
if (historicalTunnelCourses) assert.equal(tunnelEnds.size, 3);

class Queue {
  items = [];
  push(item) {
    const items = this.items;
    let i = items.length;
    items.push(item);
    while (i && item.cost < items[(i - 1) >> 1].cost) {
      items[i] = items[(i - 1) >> 1];
      i = (i - 1) >> 1;
    }
    items[i] = item;
  }
  pop() {
    const items = this.items;
    const first = items[0];
    const last = items.pop();
    if (!items.length) return first;
    let i = 0;
    while (2 * i + 1 < items.length) {
      let child = 2 * i + 1;
      if (items[child + 1]?.cost < items[child].cost) child++;
      if (items[child].cost >= last.cost) break;
      items[i] = items[child];
      i = child;
    }
    items[i] = last;
    return first;
  }
}

function snap(point, roads, limit) {
  let best;
  let gap = limit;
  for (const node of nodes.values()) {
    if (roads && !roads.some((name) => node.names.has(name))) continue;
    const d = distance(node, point);
    if (d >= gap) continue;
    best = node;
    gap = d;
  }
  assert.ok(best, `No road near ${JSON.stringify(point)}: ${roads}`);
  return best;
}

function routeBetween(from, to, tunnel, highway, groundOnly = false) {
  const queue = new Queue();
  const costs = new Map([[from, 0]]);
  const parents = new Map();
  queue.push({ node: from, cost: 0 });
  while (queue.items.length) {
    const { node, cost } = queue.pop();
    if (cost !== costs.get(node)) continue;
    if (node === to) {
      const route = [];
      for (let end = to; end !== from; ) {
        const { previous, edge } = parents.get(end);
        route.push({ ...edge, from: previous });
        end = previous;
      }
      return route.reverse();
    }
    for (const edge of node.edges) {
      if (tunnel ? !edge.tunnel || edge.name !== tunnel : edge.tunnel) continue;
      if (groundOnly && edge.highway) continue;
      const next = cost + edge.length * (edge.highway === highway ? 1 : 2.5);
      if (next >= (costs.get(edge.to) ?? Infinity)) continue;
      costs.set(edge.to, next);
      parents.set(edge.to, { previous: node, edge });
      queue.push({ node: edge.to, cost: next });
    }
  }
  return null;
}

function routeFor(spec, from, to) {
  const start = snap(from, from.names, 27);
  const destination = snap(to, to.names, 27);
  if (spec.contact)
    return layeredRoute(
      spec.contact,
      from,
      to,
      start,
      destination,
      spec.groundOnly,
    );
  const vias = (spec.via || []).map(([x, y, name]) => ({
    node: snap({ x, y }, name ? [name] : null, name ? 600 : 40),
    highway: !!name,
  }));
  const endpoints = spec.tunnel ? tunnelEnds.get(spec.tunnel) : [];
  const orders = spec.tunnel ? [endpoints, [...endpoints].reverse()] : [[]];
  let best;
  for (const [entrance, exit] of orders) {
    const stops = [
      { node: start },
      ...vias,
      ...(entrance
        ? [{ node: entrance }, { node: exit, tunnel: spec.tunnel }]
        : []),
      { node: destination },
    ];
    const legs = stops
      .slice(1)
      .map((stop, i) =>
        routeBetween(
          stops[i].node,
          stop.node,
          stop.tunnel,
          !!stop.highway || !!stops[i].highway,
          spec.groundOnly,
        ),
      );
    if (legs.some((leg) => !leg)) continue;
    const route = legs.flat();
    const length = route.reduce((sum, edge) => sum + edge.length, 0);
    if (!best || length < best.length) best = { route, length };
  }
  if (!best) return null;
  if (distance(from, start) > 0.1)
    best.route.unshift({ from, to: start, name: from.names[0] });
  if (distance(to, destination) > 0.1)
    best.route.push({ from: destination, to, name: to.names[0] });
  return best.route;
}

function groundLegs(from, to, via, groundOnly) {
  const stops = [
    from,
    ...via.map(([x, y, name]) => snap({ x, y }, [name], 10)),
    to,
  ];
  const legs = stops
    .slice(1)
    .map((stop, i) =>
      routeBetween(stops[i], stop, undefined, undefined, groundOnly),
    );
  return legs.every(Boolean) ? legs.flat() : null;
}

function layeredRoute(contact, from, to, start, destination, groundOnly) {
  const {
    roads,
    reverse,
    entry,
    exit,
    beforeGuide = [],
    afterGuide = [],
    beforeVia = [],
    afterVia = [],
    approachOffsets = {},
    laneGuides = {},
    sectionMph = {},
    groundLinkMph = {},
    groundLinks = {},
  } = contact;
  const startsOnContact = from.surfaceId === roads[0];
  const portalNode = ([x, y, name]) => snap({ x, y }, [name], 55);
  const enterNode = startsOnContact ? null : portalNode(entry);
  const exitNode = portalNode(exit);
  const before = startsOnContact
    ? []
    : groundLegs(start, enterNode, beforeVia, groundOnly);
  const after = groundLegs(exitNode, destination, afterVia, groundOnly);
  assert.ok(before && after, "no ground route to a layered course portal");
  const sections = roads.map((id, index) => {
    const surface = pieces.get(id);
    assert.ok(surface, `No baked contact ${id}`);
    const profile = reverse ? surface.points.toReversed() : surface.points;
    const offset = approachOffsets[id];
    const guide = laneGuides[id] ?? (offset ? { approach: offset } : null);
    let points = (guide ? steeringLine(profile, guide) : profile).map(
      ([x, y]) => ({ x, y }),
    );
    if (index === 0 && startsOnContact) {
      const nearest = points.reduce(
        (best, point, i) =>
          distance(point, from) < distance(points[best], from) ? i : best,
        0,
      );
      points = [from, ...points.slice(nearest + 1)];
      assert.ok(
        points.length > 1,
        "The layered pickup needs a running surface ahead",
      );
    }
    return points.slice(1).map((point, i) => ({
      from: points[i],
      to: point,
      name: surface.label,
      surfaceId: surface.id,
      highway: surface.kind !== "tunnel",
      speedMph: sectionMph[id],
    }));
  });
  const connector = (from, to, name) =>
    distance(from, to) > 0.01 ? [{ from, to, name }] : [];
  const guide = (first, waypoints, last, name) => {
    const points = [first, ...waypoints.map(([x, y]) => ({ x, y })), last];
    return points
      .slice(1)
      .flatMap((point, i) => connector(points[i], point, name));
  };
  const ribbon = sections.flatMap((nextSection, i) => {
    const link = groundLinks[roads[i]];
    if (link) {
      assert.ok(i > 0, "A ground link needs two road contacts");
      const departure = sections[i - 1].at(-1).to;
      const arrival = nextSection[0].from;
      const leave = snap(departure, [link.from], 55);
      const enter = link.entry
        ? portalNode(link.entry)
        : snap(arrival, [link.to], 55);
      const street = groundLegs(leave, enter, link.via ?? [], groundOnly);
      assert.ok(street, `No ground connection from ${link.from} to ${link.to}`);
      return [
        ...connector(departure, leave, link.from),
        ...street,
        ...guide(enter, link.arrivalGuide ?? [], arrival, link.to),
        ...nextSection,
      ];
    }
    const linkMph = groundLinkMph[roads[i]];
    if (linkMph === undefined) return nextSection;
    assert.ok(i > 0, "A ground link needs two road contacts");
    const entry = nextSection[0];
    return [
      {
        from: sections[i - 1].at(-1).to,
        to: entry.from,
        name: entry.name,
        speedMph: linkMph,
      },
      ...nextSection,
    ];
  });
  return [
    ...(startsOnContact
      ? []
      : [
          ...connector(from, start, from.names[0]),
          ...before,
          ...guide(enterNode, beforeGuide, ribbon[0].from, entry[2]),
        ]),
    ...ribbon,
    ...guide(ribbon.at(-1).to, afterGuide, exitNode, exit[2]),
    ...after,
    ...connector(destination, to, to.names[0]),
  ];
}

function samplePath(route) {
  const first = route[0];
  const path = [{ x: first.from.x, y: first.from.y, road: first }];
  for (const edge of route) {
    const divisions = Math.ceil(distance(edge.from, edge.to));
    for (let i = 1; i <= divisions; i++)
      path.push({ ...blend(edge.from, edge.to, i / divisions), road: edge });
  }
  let s = 0;
  return path.map((point, i) => {
    if (i) s += distance(path[i - 1], point);
    return { ...point, s, z: courseHeight(point.road, point.x, point.y) };
  });
}

function applyLaneDetours(path, detours) {
  const windows = detours.map(([x, y, lateralMetres]) => {
    const obstacle = { x, y };
    const nearest = path.reduce((best, point) =>
      distance(point, obstacle) < distance(best, obstacle) ? point : best,
    );
    return { station: nearest.s + 2, lateralMetres };
  });
  const shifted = path.map((point, i) => {
    const before = path[Math.max(0, i - 1)];
    const after = path[Math.min(path.length - 1, i + 1)];
    const direction = heading(before, after);
    const offset = windows.reduce((total, { station, lateralMetres }) => {
      const remaining = (point.s - station) / 22;
      return total + lateralMetres * Math.max(0, 1 - remaining ** 2) ** 2;
    }, 0);
    return {
      ...point,
      x: point.x - offset * Math.sin(direction),
      y: point.y + offset * Math.cos(direction),
    };
  });
  let station = 0;
  return shifted.map((point, i) => {
    if (i) station += distance(shifted[i - 1], point);
    return {
      ...point,
      s: station,
      z: courseHeight(point.road, point.x, point.y),
    };
  });
}

function collisionIndexes(nearby) {
  return nearby.map((tile) => {
    if (!indexes.has(tile.t))
      indexes.set(
        tile.t,
        buildCollisionIndex(tile, tile.t, new Map(), bakedDecks.ceiling),
      );
    return indexes.get(tile.t);
  });
}

function clearanceGuide(path, { lateralMetres, speedMph }) {
  assert.ok(path.every(({ road }) => !road.surfaceId && !road.tunnel));
  assert.ok(Number.isFinite(lateralMetres) && lateralMetres >= 0);
  assert.ok(Number.isFinite(speedMph) && speedMph > 0);
  const stations = [];
  let i = 0;
  for (let s = 4; s < path.at(-1).s - 4; s += 2) {
    while (path[i + 1].s < s) i++;
    const a = path[i];
    const b = path[i + 1];
    const headingAtStation = heading(a, b);
    const point = blend(a, b, (s - a.s) / (b.s - a.s));
    stations.push({
      ...point,
      s,
      heading: headingAtStation,
      road: { ...b.road, speedMph },
      obstacles: collisionIndexes(
        tilesAround(manifest.tiles, point.x, point.y, 60).want.map((token) =>
          tiles.get(token),
        ),
      ),
    });
  }
  const lanePose = ({ x, y, heading }, offset) => ({
    x: x - offset * Math.sin(heading),
    y: y + offset * Math.cos(heading),
    heading,
  });
  const offsets = roadClearanceOffsets(
    stations,
    lateralMetres,
    (station, offset) => {
      const pose = lanePose(station, offset);
      const z = heightAt(pose.x, pose.y);
      return (
        z === null ||
        obstacleContact(station.obstacles, drivingHull({ ...pose, z })) !== null
      );
    },
  );
  const guided = [
    path[0],
    ...stations.map((station, index) => ({
      ...lanePose(station, offsets[index]),
      road: station.road,
    })),
    path.at(-1),
  ];
  let s = 0;
  return guided.map((point, index) => {
    if (index) s += distance(guided[index - 1], point);
    return { ...point, s, z: heightAt(point.x, point.y) };
  });
}

function guideAfterContact(path, settings) {
  const lastContact = path.findLastIndex(({ road }) => road.surfaceId);
  assert.ok(
    lastContact >= 0 && lastContact < path.length - 1,
    "Missing ground after the last contact",
  );
  const firstStreet = lastContact + 1;
  const start = path[firstStreet].s;
  const street = path
    .slice(firstStreet)
    .map((point) => ({ ...point, s: point.s - start }));
  return [
    ...path.slice(0, firstStreet),
    ...clearanceGuide(street, settings).map((point) => ({
      ...point,
      s: point.s + start,
    })),
  ];
}

function profile(path) {
  // Inspect elevations at one-metre chainage, independent of digitisation and
  // of the distance the controller ultimately drives.
  let i = 0;
  const samples = [];
  for (let s = 0; s < path.at(-1).s; s++) {
    while (path[i + 1].s < s) i++;
    const a = path[i],
      b = path[i + 1];
    const point = blend(a, b, (s - a.s) / (b.s - a.s));
    samples.push({
      ...point,
      road: b.road.name,
      tunnel: b.road.tunnel,
      surfaceId: b.road.surfaceId,
      z: courseHeight(b.road, point.x, point.y),
    });
  }
  const grades = [];
  const layerGrades = [];
  const events = [];
  for (let j = 1; j < samples.length; j++) {
    const point = samples[j];
    const previous = samples[j - 1];
    if (previous.z === null || point.z === null) continue;
    if (previous.tunnel || point.tunnel) continue;
    const grade = point.z - previous.z;
    grades.push(Math.abs(grade));
    if (point.surfaceId && point.surfaceId === previous.surfaceId)
      layerGrades.push(Math.abs(grade));
    const before = samples[j - 4]?.z;
    const after = samples[j + 4]?.z;
    const depression =
      before != null && after != null ? (before + after) / 2 - point.z : 0;
    if (Math.abs(grade) > gradeThreshold || depression > dipThreshold)
      events.push({
        ...location(point),
        s: j,
        grade: round(grade * 100),
        depression: round(depression),
      });
  }
  grades.sort((a, b) => a - b);
  layerGrades.sort((a, b) => a - b);
  const worstGrade = events.toSorted(
    (a, b) => Math.abs(b.grade) - Math.abs(a.grade),
  )[0];
  const worstDip = events.toSorted((a, b) => b.depression - a.depression)[0];
  return {
    length: round(path.at(-1).s),
    highwayMetres: round(
      path
        .slice(1)
        .reduce((sum, p, i) => sum + (p.road.highway ? p.s - path[i].s : 0), 0),
    ),
    unsupported: samples.filter(({ z }) => z === null).length,
    bakedSamples: samples.filter(({ surfaceId }) => surfaceId).length,
    bakedP95Grade: layerGrades.length
      ? round(layerGrades[Math.floor((layerGrades.length - 1) * 0.95)] * 100)
      : null,
    bakedPeakGrade: layerGrades.length ? round(layerGrades.at(-1) * 100) : null,
    tunnelSamples: samples.filter(
      ({ tunnel, surfaceId }) =>
        tunnel || surfaces.get(surfaceId)?.kind === "tunnel",
    ).length,
    p95Grade: grades.length
      ? round(grades[Math.floor((grades.length - 1) * 0.95)] * 100)
      : null,
    peakGrade: grades.length ? round(grades.at(-1) * 100) : null,
    steep: events.filter(({ grade }) => Math.abs(grade) > 50).length,
    dips: events.filter(({ depression }) => depression > 0.5).length,
    worstGrade,
    worstDip,
    tunnelOverlyingGrade: peakTerrainGrade(samples),
  };
}

function peakTerrainGrade(samples) {
  const grades = samples.slice(1).flatMap((point, i) => {
    const last = samples[i];
    return point.tunnel && last.tunnel && point.z !== null && last.z !== null
      ? [Math.abs(point.z - last.z) * 100]
      : [];
  });
  return grades.length ? round(Math.max(...grades)) : undefined;
}

function location({ x, y, z, road }) {
  return {
    east: round(x),
    north: round(y),
    z: round(z),
    tile: ownerOf(x, y),
    road,
  };
}

function drivenProfile(samples) {
  const grades = [];
  let steepest;
  let deepest;
  let dips = 0;
  for (let i = 1; i < samples.length; i++) {
    const point = samples[i];
    const previous = samples[i - 1];
    if (point.z === null || previous.z === null) continue;
    const grade = (point.z - previous.z) * 100;
    grades.push(Math.abs(grade));
    if (!steepest || Math.abs(grade) > Math.abs(steepest.grade))
      steepest = { ...point, grade };
    const window = samples.slice(i - 4, i + 5);
    if (window.length !== 9 || window.some(({ z }) => z === null)) continue;
    const depth = (window[0].z + window[8].z) / 2 - point.z;
    if (depth <= dipThreshold) continue;
    dips++;
    if (!deepest || depth > deepest.depth) deepest = { ...point, depth };
  }
  grades.sort((a, b) => a - b);
  return {
    p95Grade: grades.length
      ? round(grades[Math.floor((grades.length - 1) * 0.95)])
      : null,
    peakGrade: grades.length ? round(grades.at(-1)) : null,
    worstGrade: steepest && {
      ...location(steepest),
      grade: round(steepest.grade),
    },
    dips,
    worstDip: deepest && { ...location(deepest), depth: round(deepest.depth) },
  };
}

function speedLimits(path) {
  // Limit speed before sharp turns, including when entering a junction on a
  // downhill grade. The policy can see its course but cannot move the car.
  const speed = path.map((p, i) => {
    const a = path[Math.max(0, i - 4)],
      b = path[Math.min(path.length - 1, i + 4)];
    const turn = Math.abs(angle(heading(p, b) - heading(a, p)));
    const limit =
      (p.road.speedMph ?? (p.road.highway ? plan.highwayMph : plan.streetMph)) /
      2.236936;
    return i === 0 || i === path.length - 1
      ? 3
      : Math.min(
          limit,
          Math.sqrt((2.5 * distance(a, b)) / Math.max(0.01, turn)),
        );
  });
  for (let i = speed.length - 2; i >= 0; i--)
    speed[i] = Math.min(
      speed[i],
      Math.sqrt(speed[i + 1] ** 2 + 6 * (path[i + 1].s - path[i].s)),
    );
  return speed;
}

function controlsFor(path) {
  const speed = speedLimits(path);
  let at = 0;
  return (car) => {
    let closest = Infinity;
    let next = at;
    for (let i = Math.max(0, at - 3); i < Math.min(path.length, at + 19); i++) {
      const gap = distance(path[i], car);
      if (gap < closest) {
        closest = gap;
        next = i;
      }
    }
    at = next;
    const lookAhead = clamp(3.3 + car.v * 0.37, 3.3, 11);
    let i = at;
    while (i + 1 < path.length && path[i].s - path[at].s < lookAhead) i++;
    const point = path[i];
    const rotation = angle(heading(car, point) - car.heading);
    const steering = Math.atan(
      (5.4 * Math.sin(rotation)) / Math.max(1, distance(car, point)),
    );
    const goal = speed[at];
    const correction = goal - car.v;
    return {
      input: {
        steer: clamp(steering / (0.58 / (1 + Math.abs(car.v) / 20)), -1, 1),
        throttle: clamp(correction * 0.5 + 0.08, 0, 1),
        brake: clamp(-correction * 0.35 - 0.05, 0, 1),
      },
      distance: closest,
      at,
    };
  };
}

function drive(path, from, to) {
  const steerer = controlsFor(path);
  const startSurface = from.surfaceId && surfaces.get(from.surfaceId);
  if (from.surfaceId && !startSurface) return { status: "start unsupported" };
  const startZ = startSurface
    ? startSurface.heightAt(from.x, from.y)
    : heightAt(from.x, from.y);
  if (startZ === null) return { status: "start unsupported" };
  const startPoint = path.find((point) => point.s >= 4);
  let car = {
    x: from.x,
    y: from.y,
    z: startZ,
    v: 0,
    grounded: true,
    heading: heading(from, startPoint),
    surfaceId: startSurface?.id,
  };
  const step = 1 / 120;
  let nearby = [];
  let collisions = [];
  let road;
  let travelled = 0;
  let progress = 0;
  let air = 0;
  let flightClearance = 0;
  let peakClearance = 0;
  let maxMph = 0;
  let stalls = 0;
  let scrapes = 0;
  let seamCrossings = 0;
  let worstSeamStep = 0;
  let tick = 0;
  let stopCrossTrack = 0;
  let status = "timed out";
  const seenSurfaces = new Set();
  const samples = [{ ...car, road: from.names[0] }];
  const distanceBySurface = { ground: 0, layered: 0, tunnel: 0 };
  let airborneMetres = 0;
  let exitDistance;
  const trial = createTrial([from, to]);
  const limit = (path.at(-1).s / 2 + 90) / step;
  const crashSupport = crashRoadSupport(heightAt, surfaces);
  for (; tick < limit; tick++) {
    if (tick % 120 === 0) {
      nearby = tilesAround(manifest.tiles, car.x, car.y, 60).want.map((token) =>
        tiles.get(token),
      );
      collisions = collisionIndexes(nearby);
    }
    if (tick % 18 === 0) road = nearestRoad(nearby, car.x, car.y);
    const control = steerer(car);
    progress = path[control.at].s;
    stopCrossTrack = control.distance;
    if (path[control.at].road.tunnel) {
      status = "unresolved tunnel";
      break;
    }
    if (control.distance > 16) {
      status = "controller left the course";
      break;
    }
    const surface = surfaces.get(car.surfaceId);
    const portal = driveThroughRoadPortal(
      car,
      control.input,
      step,
      surface ? [portals.get(surface.id)] : portals.values(),
      heightAt,
    );
    let candidate = portal;
    if (!candidate && surface) {
      candidate = driveOnRoadNetwork(
        car,
        control.input,
        step,
        surface,
        surfaces,
        network,
        () => false,
      );
    }
    candidate ??= contactStep(
      car,
      control.input,
      step,
      (x, y) => ({
        height: heightAt(x, y),
        drivable: !!road && road.distance <= road.width,
      }),
      1.35,
    );
    if (candidate.blocked) {
      status = surface ? "end of baked contact" : "missing ground contact";
      break;
    }
    const support = courseHeight(candidate, candidate.x, candidate.y);
    if (candidate.surfaceId) seenSurfaces.add(candidate.surfaceId);
    if (car.grounded === false && hardLanding(car, candidate, crashSupport)) {
      status = "hard landing";
      car = candidate;
      break;
    }
    const hit = obstacleContact(
      collisions,
      drivingHull(candidate),
      car.grounded ? undefined : drivingHull(car),
    );
    if (hit && seriousImpact(candidate, hit)) {
      status = `impact: ${hit.kind}`;
      car = candidate;
      break;
    }
    if (hit) scrapes++;
    if (candidate.grounded === false) {
      flightClearance = Math.max(flightClearance, candidate.z - support);
      peakClearance = Math.max(peakClearance, flightClearance);
    } else if (!car.grounded) {
      if (flightClearance >= clearanceThreshold) air++;
      flightClearance = 0;
    }
    const next = hit ? scrapeObstacle(candidate, hit) : candidate;
    if (car.surfaceId && !next.surfaceId && portal) exitDistance = travelled;
    if (ownerOf(car.x, car.y) !== ownerOf(next.x, next.y)) {
      seamCrossings++;
      worstSeamStep = Math.max(worstSeamStep, Math.abs(next.z - car.z));
    }
    const run = distance(car, next);
    const roadKind = surfaces.get(next.surfaceId)?.kind;
    if (car.grounded && next.grounded) {
      const bucket =
        roadKind === "tunnel" ? "tunnel" : roadKind ? "layered" : "ground";
      distanceBySurface[bucket] += run;
    } else {
      airborneMetres += run;
    }
    while (samples.length <= travelled + run) {
      const fraction = (samples.length - travelled) / run;
      const point = blend(car, next, fraction);
      const z =
        car.grounded && next.grounded
          ? car.z + (next.z - car.z) * fraction
          : null;
      samples.push({ ...point, z, road: road?.name });
    }
    travelled += run;
    maxMph = Math.max(maxMph, next.v * 2.236936);
    stalls = next.v < 0.3 ? stalls + 1 : 0;
    car = next;
    advanceTrial(trial, step, car, control.input.throttle > 0, car.grounded);
    if (trial.status === "finished") {
      status = "both emoji collected";
      break;
    }
    if (stalls > 4 / step) {
      status = "stalled";
      break;
    }
  }
  return {
    status,
    reached: [...trial.collected],
    seconds: round(tick * step),
    travelled: round(travelled),
    progress: round(progress),
    maxMph: round(maxMph),
    air,
    peakClearance: round(peakClearance),
    stopCrossTrack: round(stopCrossTrack),
    scrapes,
    groundedStreetMetres: round(distanceBySurface.ground),
    groundedLayeredMetres: round(distanceBySurface.layered),
    groundedTunnelMetres: round(distanceBySurface.tunnel),
    airborneMetres: round(airborneMetres),
    seamCrossings,
    worstSeamStep: round(worstSeamStep),
    seenSurfaces: [...seenSurfaces],
    afterExitMetres:
      exitDistance === undefined ? null : round(travelled - exitDistance),
    drivenProfile: drivenProfile(samples),
    stop: location({ ...car, road: road?.name }),
  };
}

for (const spec of plan.routes) {
  if (chosen.size && !chosen.has(spec.id)) continue;
  const from = targets.get(spec.from),
    to = targets.get(spec.to);
  assert.ok(from && to, spec.id);
  const route = routeFor(spec, from, to);
  assert.ok(route?.length, `No course found for ${spec.id}`);
  const sampled = samplePath(route);
  let path = sampled;
  if (spec.laneDetours?.length) path = applyLaneDetours(path, spec.laneDetours);
  if (spec.clearanceGuide) path = clearanceGuide(path, spec.clearanceGuide);
  if (spec.afterClearanceGuide)
    path = guideAfterContact(path, spec.afterClearanceGuide);
  const expectedSurfaces = spec.expectContactExit
    ? [
        ...(spec.contact.beforeSurfaces ?? []),
        ...spec.contact.roads,
        ...(spec.contact.afterSurfaces ?? []),
      ]
    : undefined;
  if (exportCourses) {
    const speeds = speedLimits(path);
    console.log(
      JSON.stringify({
        id: spec.id,
        from: from.t,
        to: to.t,
        manifestSha256: plan.manifestSha256,
        alignmentManifestSha256,
        placementSha256: quest.placementSha256,
        progressBaseSha256: manifest.terrainRebake.progressBaseSha256,
        startSurfaceId: from.surfaceId,
        expectedSurfaces,
        expectNoScrapes: spec.expectNoScrapes,
        maxDips: spec.maxDips,
        maxAirborneMetres: spec.maxAirborneMetres,
        expectedEnclosed: spec.expectEnclosed
          ? spec.contact.roads.filter(
              (id) => surfaces.get(id)?.kind === "tunnel",
            )
          : undefined,
        waypoints: path.map(({ x, y, s }, i) => [
          round(x),
          round(y),
          round(s),
          round(speeds[i]),
        ]),
      }),
    );
    continue;
  }
  const metrics = profile(path);
  const result = drive(path, from, to);
  console.log(
    JSON.stringify({
      id: spec.id,
      from: from.names,
      to: to.names,
      manifest: plan.manifestSha256,
      alignmentManifestSha256,
      courseSet,
      ...metrics,
      result,
    }),
  );
  if (spec.expectCompletion ?? plan.expectCompletion)
    assert.equal(result.status, "both emoji collected", spec.id);
  if (spec.expectNoScrapes) assert.equal(result.scrapes, 0, spec.id);
  if (spec.maxDips !== undefined)
    assert.ok(result.drivenProfile.dips <= spec.maxDips, spec.id);
  if (spec.maxAirborneMetres !== undefined)
    assert.ok(
      result.airborneMetres <= spec.maxAirborneMetres,
      `${spec.id} travelled ${result.airborneMetres} m airborne`,
    );
  if (expectedSurfaces) {
    assert.deepEqual(result.seenSurfaces, expectedSurfaces, spec.id);
    assert.ok(
      result.afterExitMetres >= 20,
      `${spec.id} did not clear its final portal`,
    );
  }
}
