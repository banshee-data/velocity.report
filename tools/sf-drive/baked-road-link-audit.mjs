import {
  contactStep,
  groundAtWorld,
} from "../../public_html/src/js/sf-drive-core.js";
import { strikeHighwayRail } from "../../public_html/src/js/sf-highway-edge.js";
import { roadSupportsCar } from "../../public_html/src/js/sf-road-contact.js";
import {
  driveOnRoadNetwork,
  indexRoadNetwork,
} from "../../public_html/src/js/sf-road-network.js";
import {
  driveThroughRoadPortal,
  shouldTakePortalRoadJoin,
} from "../../public_html/src/js/sf-road-portal.js";

const dt = 1 / 120;
const spacing = 0.2;
const clamp = (value, min, max) => Math.max(min, Math.min(max, value));
const distance = (a, b) => Math.hypot(a.x - b.x, a.y - b.y);
const smooth = (value) => {
  const t = clamp(value, 0, 1);
  return t * t * (3 - 2 * t);
};

function at({ profile }, station) {
  const s = clamp(station, 0, profile.at(-1).s_m);
  const i = Math.max(
    1,
    profile.findIndex((point) => point.s_m >= s),
  );
  const [a, b] = profile.slice(i - 1, i + 1);
  const fraction = (s - a.s_m) / (b.s_m - a.s_m);
  return a.position.map(
    (value, axis) => value + fraction * (b.position[axis] - value),
  );
}

function seamGuide(source, target, from, to, lane) {
  const fromEnd = from === source.end_ids[1];
  const toStart = to === target.end_ids[0];
  const sourceLength = source.profile.at(-1).s_m;
  const targetLength = target.profile.at(-1).s_m;
  const before = Math.min(23, sourceLength - 2);
  const after = Math.min(15, targetLength - 0.5);
  if (before < 2 || after < 2) return null;
  const [x, y] = at(source, fromEnd ? sourceLength : 0);
  const [px, py] = at(source, fromEnd ? sourceLength - 0.5 : 0.5);
  const heading =
    source.terminalBearings?.[fromEnd ? 1 : 0] ?? Math.atan2(y - py, x - px);
  const segments = Math.ceil((before + after) / spacing);
  const points = Array.from({ length: segments + 1 }, (_, i) => {
    const d = -before + ((before + after) * i) / segments;
    const [east, north] =
      d <= 0
        ? at(source, fromEnd ? sourceLength + d : -d)
        : at(target, toStart ? d : targetLength - d);
    const offset =
      lane *
      (d <= 0 ? smooth((d + before + 4) / (before + 4)) : smooth(1 - d / 11));
    return {
      x: east - Math.sin(heading) * offset,
      y: north + Math.cos(heading) * offset,
      d,
    };
  });
  return { points, finish: Math.min(7, after - 1) };
}

function portalGuide(portal, inward, lane, angle) {
  const { surface, x, y, nx, ny, entryX, entryY } = portal;
  const start = surface.profile[0].position;
  const fromStart = Math.hypot(x - start[0], y - start[1]) < 0.01;
  const length = surface.profile.at(-1).s_m;
  if (length < 3.5) return null;
  const after = Math.min(13, length - 0.75);
  const heading = Math.atan2(entryY, entryX) + angle;
  const points = Array.from(
    { length: 81 + Math.floor(after / spacing) },
    (_, i) => {
      const d = (i - 80) * spacing;
      if (d < 0) {
        return {
          x: x + Math.cos(heading) * d - ny * lane,
          y: y + Math.sin(heading) * d + nx * lane,
          d,
        };
      }
      const [east, north] = at(surface, fromStart ? d : length - d);
      const taper = smooth(1 - d / 10);
      return { x: east - ny * lane * taper, y: north + nx * lane * taper, d };
    },
  );
  if (!inward) points.reverse();
  return { points, finish: inward ? Math.min(5, after - 1) : -6 };
}

function step(car, input, { surfaces, portals, network }, terrain) {
  const road = surfaces.get(car.surfaceId);
  const join = () =>
    driveOnRoadNetwork(car, input, dt, road, surfaces, network, () => false);
  const crossing =
    terrain &&
    driveThroughRoadPortal(
      car,
      input,
      dt,
      road ? [portals.get(road.id)] : portals.values(),
      terrain,
    );
  if (crossing) {
    if (road && !crossing.surfaceId) {
      const connected = join();
      if (
        shouldTakePortalRoadJoin(
          car,
          connected,
          portals.get(connected.surfaceId) ?? [],
        )
      )
        return connected;
    }
    return crossing;
  }
  if (road) {
    const rail = strikeHighwayRail(car, input, dt, road);
    return rail ? { ...car, rail: true } : join();
  }
  return contactStep(
    car,
    input,
    dt,
    (x, y) => ({ height: terrain(x, y), drivable: true }),
    1.35,
  );
}

// The steering guide only supplies input. The actual contact solver decides
// whether to take a join, wait, hit a rail or fall away from the ground.
function drive(guide, route, assets, terrain) {
  if (!guide) return { outcome: "short approach" };
  const { points, finish } = guide;
  const { from, to, speed, look } = route;
  const start = points[0];
  const ahead = points[from && to ? 1 : 3];
  const heading = Math.atan2(ahead.y - start.y, ahead.x - start.x);
  const pavement = from ? assets.surfaces.get(from).heightAt : terrain;
  const z = pavement(start.x, start.y);
  let car = { ...start, z, heading, v: speed, grounded: true, surfaceId: from };
  if (z === null || !roadSupportsCar(pavement, car))
    return { outcome: "unsupported start" };
  if (!from) {
    const dx = Math.cos(heading) * 1.35;
    const dy = Math.sin(heading) * 1.35;
    car.vz =
      (((terrain(car.x + dx, car.y + dy) ?? z) -
        (terrain(car.x - dx, car.y - dy) ?? z)) *
        speed) /
      2.7;
  }

  let index = 0;
  let stalled = 0;
  let air = 0;
  let clearance = 0;
  for (let tick = 0; tick < 1320; tick++) {
    let best = Infinity;
    for (
      let i = Math.max(0, index - 3);
      i < Math.min(index + 13, points.length);
      i++
    ) {
      const gap = distance(car, points[i]);
      if (gap >= best) continue;
      index = i;
      best = gap;
    }
    const d = points[index].d;
    if (
      car.surfaceId === to &&
      car.grounded &&
      (to ? d >= finish : d <= finish)
    ) {
      return {
        outcome: "passed",
        air_m: Math.round(air * 100) / 100,
        clearance_m: Math.round(clearance * 1000) / 1000,
      };
    }
    if (best > 8) return { outcome: "off guide" };
    const aim =
      points[Math.min(index + Math.ceil(look / spacing), points.length - 1)];
    const angle = Math.atan2(aim.y - car.y, aim.x - car.x) - car.heading;
    const demand = Math.atan(
      (5.4 * Math.sin(angle)) / Math.max(1, distance(car, aim)),
    );
    const error = speed - car.v;
    const input = {
      steer: clamp(demand / (0.58 / (1 + Math.abs(car.v) / 20)), -1, 1),
      throttle: error > 0.15 ? clamp(error * 0.5, 0.15, 1) : 0,
      brake: error < -0.3 ? clamp(-error * 0.5, 0.15, 1) : 0,
    };
    const next = step(car, input, assets, terrain);
    if (next.rail) return { outcome: "rail contact" };
    if (next.surfaceId && next.surfaceId !== from && next.surfaceId !== to)
      return { outcome: "other road", road: next.surfaceId };
    if (!next.grounded) {
      if (!terrain) return { outcome: "lost contact" };
      air += distance(next, car);
      clearance = Math.max(
        clearance,
        next.z - (terrain(next.x, next.y) ?? next.z),
      );
    }
    stalled = next.blocked || distance(next, car) < 0.001 ? stalled + 1 : 0;
    if (stalled > 7) return { outcome: "stopped" };
    car = next;
  }
  return { outcome: "no handoff" };
}

function screen(tries, replay) {
  const reports = [];
  for (const attempt of tries) {
    const report = { ...attempt, ...replay(attempt) };
    reports.push(report);
    if (report.outcome === "passed") break;
  }
  return reports;
}

const portalAttempts = [
  [0, 0, 6],
  [0, 0, 4],
  [-0.75, 0, 6],
  [0.75, 0, 6],
  [-1.5, 0, 6],
  [1.5, 0, 6],
  [0, -0.25, 5],
  [0, 0.25, 5],
  [-0.75, -0.24, 5],
  [0.75, 0.24, 5],
  [-0.75, 0.24, 5],
  [0.75, -0.24, 5],
  [0, -0.45, 3],
  [0, 0.45, 3],
  [-1.5, -0.2, 4],
  [1.5, 0.2, 4],
].map(([lane, angle, speed]) => ({
  lane,
  angle,
  speed,
  look: 4 + speed * 0.2,
}));

export function auditBakedRoadLinks(contacts, joins, tiles) {
  const network = indexRoadNetwork(joins);
  const surfaces = new Map(
    contacts.map(({ surface }) => [surface.id, surface]),
  );
  const portals = new Map(
    contacts.map((contact) => [contact.surface.id, contact.portals]),
  );
  const assets = { network, surfaces, portals };
  network.add({
    endpoints: contacts.flatMap((contact) => contact.endpoints),
    transitions: joins,
  });

  const result = {
    directedJoins: { total: 0, passed: 0, adjusted: 0, failures: [] },
    groundHandoffs: {
      total: 0,
      passed: 0,
      adjusted: 0,
      continued: [],
      failures: [],
    },
  };
  const passedJoins = new Set();
  for (const [from, ends] of network.transitions) {
    for (const to of ends) {
      const summary = result.directedJoins;
      summary.total++;
      const source = surfaces.get(network.endpoints.get(from)?.surface_id);
      const target = surfaces.get(network.endpoints.get(to)?.surface_id);
      if (!source || !target) {
        summary.failures.push({ from, to, outcome: "missing road" });
        continue;
      }
      const fast =
        source.kind === "bridge_deck" && target.kind === source.kind ? 14 : 8;
      const width =
        source.terminalHalfWidths?.[source.end_ids.indexOf(from)] ?? 2;
      const lanes = [-0.9, 0.9, -1.8, 1.8, -2.5, 2.5, 0].filter(
        (lane) => Math.abs(lane) <= width - 1,
      );
      const attempts = [
        { lane: 0, speed: fast, look: 5.5 },
        ...[fast, 6, 4].flatMap((speed) =>
          [4, 6.5].flatMap((look) =>
            lanes.map((lane) => ({ lane, speed, look })),
          ),
        ),
      ];
      const traces = screen(attempts, ({ lane, ...input }) =>
        drive(
          seamGuide(source, target, from, to, lane),
          { from: source.id, to: target.id, ...input },
          assets,
        ),
      );
      if (traces.at(-1)?.outcome === "passed") {
        summary.passed++;
        if (traces.length > 1) summary.adjusted++;
        passedJoins.add(`${from}/${target.id}`);
      } else summary.failures.push({ from, to, attempts: traces });
    }
  }

  for (const group of portals.values()) {
    for (const portal of group) {
      const id = portal.surface.id;
      const start = portal.surface.profile[0].position;
      const end =
        Math.hypot(start[0] - portal.x, start[1] - portal.y) < 0.01 ? 0 : 1;
      const nearest = tiles
        .toSorted(
          (a, b) =>
            Math.hypot(a.o[0] - portal.x, a.o[1] - portal.y) -
            Math.hypot(b.o[0] - portal.x, b.o[1] - portal.y),
        )
        .slice(0, 4);
      const terrain = (x, y) => groundAtWorld(nearest, x, y, true);
      for (const inward of portal.exitOnly ? [false] : [true, false]) {
        const summary = result.groundHandoffs;
        summary.total++;
        const from = inward ? undefined : id;
        const to = inward ? id : undefined;
        const traces = screen(
          portalAttempts.filter(
            ({ lane }) => Math.abs(lane) <= portal.halfWidth - 0.9,
          ),
          ({ lane, angle, ...input }) =>
            drive(
              portalGuide(portal, inward, lane, angle),
              { from, to, ...input },
              assets,
              terrain,
            ),
        );
        const description = {
          road: id,
          terminal: portal.surface.end_ids[end],
          direction: inward ? "entry" : "exit",
        };
        const continuation =
          !inward &&
          traces.find(({ road }) =>
            passedJoins.has(`${description.terminal}/${road}`),
          );
        if (traces.at(-1)?.outcome === "passed") {
          summary.passed++;
          if (traces.length > 1) summary.adjusted++;
        } else if (continuation) {
          summary.continued.push({ ...description, onto: continuation.road });
        } else summary.failures.push({ ...description, attempts: traces });
      }
    }
  }
  return result;
}
