import {
  contactStep,
  groundAtWorld,
} from "../../public_html/src/js/sf-drive-core.js";
import {
  CAR_HALF_LENGTH,
  drivingHull,
  obstacleContact,
} from "../../public_html/src/js/sf-chassis-contact.js";
import {
  scrapeObstacle,
  seriousImpact,
} from "../../public_html/src/js/sf-crash.js";
import {
  highwayRailSegments,
  strikeHighwayRail,
} from "../../public_html/src/js/sf-highway-edge.js";
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

function seamGuide(source, target, from, to, lane, holdLane) {
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
      (holdLane
        ? 1
        : d <= 0
          ? smooth((d + before + 4) / (before + 4))
          : smooth(1 - d / 11));
    return {
      x: east - Math.sin(heading) * offset,
      y: north + Math.cos(heading) * offset,
      d,
    };
  });
  return { points, finish: Math.min(7, after - 1) };
}

function portalGuide(portal, inward, lane, angle) {
  const { surface, x, y, nx, ny, entryX, entryY, sideAccess } = portal;
  const start = surface.profile[0].position;
  const fromStart = Math.hypot(x - start[0], y - start[1]) < 0.01;
  const length = surface.profile.at(-1).s_m;
  if (length < 3.5) return null;
  const maxDepth = sideAccess
    ? (Array.from(
        { length: Math.ceil(13 / spacing) },
        (_, i) => (i + 1) * spacing,
      ).find(
        (depth) => surface.heightAt(x + nx * depth, y + ny * depth) === null,
      ) ?? 13)
    : length;
  const margin = sideAccess ? CAR_HALF_LENGTH + 0.35 : 0.75;
  const after = Math.min(13, maxDepth - margin);
  if (after <= 2) return null;
  const approach = sideAccess ? Math.ceil(14 / spacing) : 80;
  const heading = Math.atan2(entryY, entryX) + angle;
  const points = Array.from(
    { length: approach + 1 + Math.floor(after / spacing) },
    (_, i) => {
      const d = (i - approach) * spacing;
      if (d < 0) {
        return {
          x: x + Math.cos(heading) * d - ny * lane,
          y: y + Math.sin(heading) * d + nx * lane,
          d,
        };
      }
      const [east, north] = sideAccess
        ? [x + nx * d, y + ny * d]
        : at(surface, fromStart ? d : length - d);
      const taper = smooth(1 - d / 10);
      return { x: east - ny * lane * taper, y: north + nx * lane * taper, d };
    },
  );
  if (!inward) points.reverse();
  return { points, finish: inward ? Math.min(5, after - 1) : -6 };
}

function lateralPoint(surface, station, lane) {
  const [x, y] = at(surface, station);
  const [ax, ay] = at(surface, station - 0.25);
  const [bx, by] = at(surface, station + 0.25);
  const heading = Math.atan2(by - ay, bx - ax);
  return {
    x: x - Math.sin(heading) * lane,
    y: y + Math.cos(heading) * lane,
    heading,
  };
}

function interiorEndpoint(surface, lane, end, margin, length) {
  const bearing = surface.terminalBearings?.[end];
  const station = end ? length - margin : margin;
  if (bearing === undefined) return station;
  const [x, y] = at(surface, end ? length : 0);
  const nx = Math.cos(bearing);
  const ny = Math.sin(bearing);
  // The transverse cut may be skewed relative to this lane. Crossings of
  // the cut belong to the join audit, which can support tyres on both roads.
  const inside = (east, north) =>
    (east - x) * nx + (north - y) * ny <= 0 ? 0 : null;
  for (let extra = 0; extra <= Math.min(4, length / 2 - margin); extra += 0.1) {
    const candidate = station + (end ? -extra : extra);
    if (roadSupportsCar(inside, lateralPoint(surface, candidate, lane)))
      return candidate;
  }
  return station;
}

function interiorGuide(surface, reverse, lane) {
  const length = surface.profile.at(-1).s_m;
  const margin = CAR_HALF_LENGTH + 0.35;
  const start = interiorEndpoint(surface, lane, 0, margin, length);
  const end = interiorEndpoint(surface, lane, 1, margin, length);
  const run = end - start;
  if (run <= 1) return null;
  const intervals = Math.ceil(run / 0.5);
  const sampleStep = run / intervals;
  const points = Array.from({ length: intervals + 1 }, (_, i) => {
    const d = i * sampleStep;
    const s = reverse ? end - d : start + d;
    const { x, y } = lateralPoint(surface, s, lane);
    return { x, y, d };
  });
  return { points, finish: run, spacing: sampleStep };
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
  const { from, to, speed, look, tickLimit = 1320 } = route;
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
  let scrapeTicks = 0;
  let lastScrape;
  const stop = (outcome) => ({
    outcome,
    ...(assets.collisions && {
      metres: Math.round(points[index].d * 100) / 100,
      position: [car.x, car.y, car.z].map(
        (value) => Math.round(value * 1000) / 1000,
      ),
      ...(lastScrape && { lastScrape }),
    }),
  });
  for (let tick = 0; tick < tickLimit; tick++) {
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
        ...(assets.collisions && { scrapeTicks }),
        ...(lastScrape && { lastScrape }),
      };
    }
    if (best > 8) return stop("off guide");
    const aim =
      points[
        Math.min(
          index + Math.ceil(look / (guide.spacing ?? spacing)),
          points.length - 1,
        )
      ];
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
    let next = step(car, input, assets, terrain);
    if (next.rail) return stop("rail contact");
    if (assets.collisions && !terrain && !next.surfaceId)
      return stop("lost surface");
    if (next.surfaceId && next.surfaceId !== from && next.surfaceId !== to)
      return { ...stop("other road"), road: next.surfaceId };
    if (!next.grounded) {
      if (!terrain) return stop("lost contact");
      air += distance(next, car);
      clearance = Math.max(
        clearance,
        next.z - (terrain(next.x, next.y) ?? next.z),
      );
    }
    if (assets.collisions) {
      const support = assets.surfaces.get(next.surfaceId);
      const deckSupport =
        support?.kind === "bridge_deck" || support?.kind === "ramp"
          ? support.heightAt
          : undefined;
      const hit = obstacleContact(
        [assets.collisions],
        drivingHull(next),
        undefined,
        deckSupport,
      );
      if (hit) {
        lastScrape = hit.tree
          ? {
              kind: hit.kind,
              tile: hit.tree.tile,
              row: hit.tree.index,
              x: hit.x,
              y: hit.y,
              hullLow: next.z,
              pose: [next.x, next.y, next.z, next.heading],
              support: assets.surfaces
                .get(next.surfaceId)
                ?.heightAt(hit.x, hit.y),
              ceiling: hit.tree.ceiling?.(hit.x, hit.y, hit.tree.base),
            }
          : { kind: hit.kind, x: hit.x, y: hit.y };
        if (seriousImpact(next, hit)) return stop("scenery impact");
        next = scrapeObstacle(next, hit);
        scrapeTicks++;
      }
    }
    stalled = next.blocked || distance(next, car) < 0.001 ? stalled + 1 : 0;
    if (stalled > 7) return stop("stopped");
    car = next;
  }
  return stop(assets.collisions ? "timed out" : "no handoff");
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

function buildAuditSurfaces(contacts) {
  return new Map(
    contacts.map(({ surface }) => [
      surface.id,
      { ...surface, railSegments: highwayRailSegments(surface) },
    ]),
  );
}

export function auditBakedRoadSpans(contacts, joins, collisions, fixedLane) {
  const surfaces = buildAuditSurfaces(contacts);
  const network = indexRoadNetwork(joins);
  network.add({
    endpoints: contacts.flatMap(({ endpoints }) => endpoints),
    transitions: joins,
  });
  const incoming = new Set(
    [...network.transitions.values()].flatMap((ends) => [...ends]),
  );
  const assets = { surfaces, network, collisions };
  const summary = {
    testedSurfaces: 0,
    testedDirections: 0,
    passed: 0,
    adjusted: 0,
    scrapes: [],
    unconnectedTerminals: [],
    failures: [],
    byKind: Object.fromEntries(
      ["bridge_deck", "ramp", "tunnel"].map((kind) => [
        kind,
        { surfaces: 0, testedDirections: 0, passed: 0 },
      ]),
    ),
  };
  for (const { surface, portals } of contacts) {
    const id = surface.id;
    const ends = surface.end_ids.map((end, index) => {
      const [x, y] =
        surface.profile[index === 0 ? 0 : surface.profile.length - 1].position;
      const ground = portals.find(
        (portal) => Math.hypot(portal.x - x, portal.y - y) < 0.01,
      );
      const enter = incoming.has(end) || (ground && !ground.exitOnly);
      const leave = network.transitions.has(end) || !!ground;
      const boundary = !enter && !leave;
      if (boundary) summary.unconnectedTerminals.push(end);
      return { enter: enter || boundary, leave: leave || boundary };
    });
    const directions = [false, true].filter((reverse) => {
      const start = Number(reverse);
      return ends[start].enter && ends[1 - start].leave;
    });
    if (!directions.length) {
      summary.failures.push({ road: id, outcome: "no directed route" });
      continue;
    }
    summary.testedSurfaces++;
    const kind = summary.byKind[surface.kind];
    kind.surfaces++;
    const length = surface.profile.at(-1).s_m;
    const fast = { bridge_deck: 14, tunnel: 12, ramp: 9 }[surface.kind];
    const attempts =
      fixedLane !== undefined
        ? [
            { lane: fixedLane, speed: 8, look: 4 },
            { lane: fixedLane, speed: 5, look: 3.5 },
          ]
        : [
            { lane: 0, speed: fast, look: 5.5 },
            { lane: 0, speed: 6, look: 4 },
            { lane: -0.75, speed: 6, look: 4 },
            { lane: 0.75, speed: 6, look: 4 },
            { lane: 0, speed: 4, look: 3 },
            { lane: -1.5, speed: 4, look: 3 },
            { lane: 1.5, speed: 4, look: 3 },
          ];
    for (const reverse of directions) {
      summary.testedDirections++;
      kind.testedDirections++;
      const traces = screen(attempts, ({ lane, speed, look }) =>
        drive(
          interiorGuide(surface, reverse, lane),
          {
            from: id,
            to: id,
            speed,
            look,
            tickLimit: Math.ceil(((2 * length) / speed + 6) / dt),
          },
          assets,
        ),
      );
      const final = traces.at(-1);
      if (final.outcome === "passed") {
        summary.passed++;
        kind.passed++;
        if (traces.length > 1) summary.adjusted++;
        if (final.scrapeTicks)
          summary.scrapes.push({
            road: id,
            reverse,
            ticks: final.scrapeTicks,
            hit: final.lastScrape,
          });
      } else summary.failures.push({ road: id, reverse, attempts: traces });
    }
  }
  return summary;
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

export function auditBakedRoadLinks(
  contacts,
  joins,
  tiles,
  collisions,
  fixedLane,
) {
  const network = indexRoadNetwork(joins);
  const surfaces = buildAuditSurfaces(contacts);
  const portals = new Map(
    contacts.map((contact) => [contact.surface.id, contact.portals]),
  );
  const assets = { network, surfaces, portals, collisions };
  network.add({
    endpoints: contacts.flatMap((contact) => contact.endpoints),
    transitions: joins,
  });

  const result = {
    directedJoins: {
      total: 0,
      passed: 0,
      adjusted: 0,
      failures: [],
      scrapes: [],
    },
    groundHandoffs: {
      total: 0,
      passed: 0,
      adjusted: 0,
      continued: [],
      failures: [],
      scrapes: [],
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
      const range = network.approachLanes.get(from)?.get(to);
      const centre = range ? (range[0] + range[1]) / 2 : 0;
      const lanes = [-0.9, 0.9, -1.8, 1.8, -2.5, 2.5, 0]
        .map((offset) => centre + offset)
        .filter((lane) =>
          range
            ? lane > range[0] && lane < range[1]
            : Math.abs(lane) <= width - 1,
        );
      const attempts =
        fixedLane === undefined
          ? [
              { lane: centre, speed: fast, look: 5.5 },
              ...[fast, 6, 4].flatMap((speed) =>
                [4, 6.5].flatMap((look) =>
                  lanes.map((lane) => ({ lane, speed, look })),
                ),
              ),
            ]
          : [fast, 6, 4].flatMap((speed) =>
              [4, 6.5].map((look) => ({
                lane: centre + fixedLane,
                speed,
                look,
              })),
            );
      const traces = screen(attempts, ({ lane, ...input }) =>
        drive(
          seamGuide(source, target, from, to, lane, !!range),
          { from: source.id, to: target.id, ...input },
          assets,
        ),
      );
      if (traces.at(-1)?.outcome === "passed") {
        summary.passed++;
        if (traces.length > 1) summary.adjusted++;
        passedJoins.add(`${from}/${target.id}`);
        if (traces.at(-1).scrapeTicks)
          summary.scrapes.push({ from, to, ...traces.at(-1) });
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
          ...(portal.sideAccess
            ? { sideAccess: portal.sideAccess }
            : { terminal: portal.surface.end_ids[end] }),
          direction: inward ? "entry" : "exit",
        };
        const continuation =
          !inward &&
          !portal.sideAccess &&
          traces.find(({ road }) =>
            passedJoins.has(`${description.terminal}/${road}`),
          );
        if (traces.at(-1)?.outcome === "passed") {
          summary.passed++;
          if (traces.length > 1) summary.adjusted++;
          if (traces.at(-1).scrapeTicks)
            summary.scrapes.push({ ...description, ...traces.at(-1) });
        } else if (continuation) {
          summary.continued.push({ ...description, onto: continuation.road });
        } else summary.failures.push({ ...description, attempts: traces });
      }
    }
  }
  return result;
}
