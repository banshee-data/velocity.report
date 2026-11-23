// Drive the pinned SF quest courses with the game's public physics functions.
// Run from the repository root: node tools/sf-drive/audit-road-routes.mjs
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
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
import {
  hardLanding,
  scrapeObstacle,
  seriousImpact,
} from "../../public_html/src/js/sf-crash.js";

const assets = new URL("../../public_html/src/sf/", import.meta.url);
const source = new URL("road-audit-routes.json", import.meta.url);
const bytes = readFileSync(new URL("tiles/manifest.json", assets));
const manifest = JSON.parse(bytes);
const planBytes = readFileSync(source);
const plan = JSON.parse(planBytes);
const quest = JSON.parse(readFileSync(new URL("quest-route.json", assets)));
const digest = (bytes) => createHash("sha256").update(bytes).digest("hex");
assert.equal(digest(bytes), plan.manifestSha256);
assert.equal(quest.manifestSha256, plan.manifestSha256);
assert.equal(plan.routes.length, 20);
const ids = new Set(plan.routes.map(({ id }) => id));
assert.equal(ids.size, plan.routes.length);
const chosen = new Set(process.argv.slice(2));
for (const id of chosen) assert.ok(ids.has(id), `Unknown course: ${id}`);
const courseSet = digest(planBytes);
const tiles = new Map(
  manifest.tiles.map(({ t, bytes }) => {
    const compressed = readFileSync(new URL(`tiles/${t}.json.gz`, assets));
    assert.equal(compressed.length, bytes, t);
    return [t, JSON.parse(gunzipSync(compressed))];
  }),
);
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

for (const tile of tiles.values())
  for (const road of tile.r) addRoad(tile, road);

// The parent geometry defines the attempted alignment only. Its heights and
// widths never become contact in the rebaked game.
const tunnelEnds = new Map();
for (const token of Object.keys(manifest.terrainRebake.removedTunnelRows)) {
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
assert.equal(tunnelEnds.size, 3);

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

function routeBetween(from, to, tunnel, highway) {
  const queue = new Queue();
  const costs = new Map([[from, 0]]);
  const parents = new Map();
  queue.push({ node: from, cost: 0 });
  while (queue.items.length) {
    const { node, cost } = queue.pop();
    if (cost !== costs.get(node)) continue;
    if (node === to) {
      const route = [];
      for (let end = to; end !== from;) {
        const { previous, edge } = parents.get(end);
        route.push({ ...edge, from: previous });
        end = previous;
      }
      return route.reverse();
    }
    for (const edge of node.edges) {
      if (tunnel ? !edge.tunnel || edge.name !== tunnel : edge.tunnel) continue;
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
    return { ...point, s, z: heightAt(point.x, point.y) };
  });
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
      z: heightAt(point.x, point.y),
    });
  }
  const grades = [];
  const events = [];
  for (let j = 1; j < samples.length; j++) {
    const point = samples[j];
    const previous = samples[j - 1];
    if (previous.z === null || point.z === null) continue;
    if (previous.tunnel || point.tunnel) continue;
    const grade = point.z - previous.z;
    grades.push(Math.abs(grade));
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
    tunnelSamples: samples.filter(({ tunnel }) => tunnel).length,
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

function controlsFor(path) {
  // Limit speed before sharp turns, including when entering a junction on a
  // downhill grade. The policy can see its course but cannot move the car.
  const speed = path.map((p, i) => {
    const a = path[Math.max(0, i - 4)],
      b = path[Math.min(path.length - 1, i + 4)];
    const turn = Math.abs(angle(heading(p, b) - heading(a, p)));
    const limit =
      (p.road.highway ? plan.highwayMph : plan.streetMph) / 2.236936;
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
  const startZ = heightAt(from.x, from.y);
  if (startZ === null) return { status: "start unsupported" };
  const startPoint = path.find((point) => point.s >= 4);
  let car = {
    x: from.x,
    y: from.y,
    z: startZ,
    v: 0,
    grounded: true,
    heading: heading(from, startPoint),
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
  const trial = createTrial([from, to]);
  const limit = (path.at(-1).s / 2 + 90) / step;
  const crashSupport = crashRoadSupport(heightAt, new Map());
  for (; tick < limit; tick++) {
    if (tick % 120 === 0) {
      nearby = tilesAround(manifest.tiles, car.x, car.y, 60).want.map((token) =>
        tiles.get(token),
      );
      collisions = nearby.map((tile) => {
        if (!indexes.has(tile.t))
          indexes.set(tile.t, buildCollisionIndex(tile, tile.t));
        return indexes.get(tile.t);
      });
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
    const candidate = contactStep(
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
      status = "missing ground contact";
      break;
    }
    const support = heightAt(candidate.x, candidate.y);
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
    if (ownerOf(car.x, car.y) !== ownerOf(next.x, next.y)) {
      seamCrossings++;
      worstSeamStep = Math.max(worstSeamStep, Math.abs(next.z - car.z));
    }
    travelled += distance(car, next);
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
    seamCrossings,
    worstSeamStep: round(worstSeamStep),
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
  const path = samplePath(route);
  const metrics = profile(path);
  const result = drive(path, from, to);
  console.log(
    JSON.stringify({
      id: spec.id,
      from: from.names,
      to: to.names,
      manifest: plan.manifestSha256,
      courseSet,
      ...metrics,
      result,
    }),
  );
}
