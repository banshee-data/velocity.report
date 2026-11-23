import {
  driveOnRoadSurface,
  indexRoadContact,
  roadSupportsCar,
} from "./sf-road-contact.js";

const supported = (value) => ["observed", "interpolated"].includes(value);
const seamReachM = 4;
const ownerPreloadM = 12;

export const SIXTH_RAMP_DRIVE = "sixth-brannan-ramp-approach";
export const SIXTH_RAMP_NAME = "6th–Brannan · game ramp approach";
export const SIXTH_RAMP_OWNER = "808f7fd4";
export const SIXTH_RAMP_SURFACE = "gms:55M0AW4H11A0P40ZHJBKQ76ZX7";

export function sixthBrannanRamp(surface) {
  if (
    surface.id !== SIXTH_RAMP_SURFACE ||
    surface.contact.geometry_hash !==
      "86661eb9ac609d680fc67daceb4e143e1765d84d9e658cfa744c424201644f77"
  )
    return null;
  const [east, north] = surface.profile[0].position;
  const [aheadEast, aheadNorth] = surface.profile[3].position;
  const heading = Math.atan2(aheadNorth - north, aheadEast - east);
  const measuredHeight = surface.heightAt;
  const hx = Math.cos(heading);
  const hy = Math.sin(heading);
  const at = (station, offset) => [
    east + hx * station - hy * offset,
    north + hy * station + hx * offset,
  ];
  const halfWidth = 1.7;
  const entrance = -9.5;
  const [x, y] = at(-16, 0);
  const [left, right] = [-halfWidth, halfWidth].map((offset) => {
    const [ex, ey] = at(0, offset);
    const z = measuredHeight(...at(0.01, offset));
    return [ex, ey, z];
  });
  if (left[2] === null || right[2] === null) return null;
  // Authored pavement between the baked street and the low end of this
  // exact Arrow strip. Keep the measured strip's contact over the join.
  const contact = {
    vertices: [
      [...at(-12, -halfWidth), 3.93],
      [...at(-12, halfWidth), 4.02],
      left,
      right,
    ],
    triangles: [
      [0, 2, 1],
      [1, 2, 3],
    ],
    support: Array(4).fill("interpolated"),
  };
  const approachHeight = indexRoadContact(contact);
  return {
    contact,
    entrance,
    along: (car) => (car.x - east) * hx + (car.y - north) * hy,
    heading,
    surfaceId: surface.id,
    heightAt: (x, y) => measuredHeight(x, y) ?? approachHeight(x, y),
    start: {
      id: SIXTH_RAMP_DRIVE,
      name: SIXTH_RAMP_NAME,
      x,
      y,
      heading,
      rampApproach: true,
    },
  };
}

export function crossRampEntrance(car, next, ramp, terrainHeight) {
  if (!ramp || !car.grounded || !next.grounded || next.blocked) return next;
  const leaving = car.surfaceId === ramp.surfaceId;
  if (car.surfaceId && !leaving) return next;
  const direction = leaving ? -1 : 1;
  if (
    (ramp.along(car) - ramp.entrance) * direction >= 0 ||
    (ramp.along(next) - ramp.entrance) * direction < 0 ||
    Math.cos(car.heading - ramp.heading) * Math.sign(next.v) * direction < 0.7
  )
    return next;
  const z = (leaving ? terrainHeight : ramp.heightAt)(next.x, next.y);
  const bothSurfaces = (x, y) => {
    const raised = ramp.heightAt(x, y);
    const base = terrainHeight(x, y);
    return raised !== null && base !== null && Math.abs(raised - base) <= 0.22
      ? raised
      : null;
  };
  if (
    z === null ||
    Math.abs(next.z - z) > 0.22 ||
    !roadSupportsCar(bothSurfaces, next)
  )
    return next;
  return {
    ...next,
    z,
    vz: 0,
    surfaceId: leaving ? undefined : ramp.surfaceId,
    seam: null,
  };
}

export function regionalDrives(review) {
  return [
    ...review.confirmed_connections.map((join) => ({
      id: join.id,
      name: `Connection · ${join.start.owner} → ${join.start.owner === join.from_owner ? join.to_owner : join.from_owner} · ${join.id.slice(-5)}`,
      start: join.start,
    })),
    ...review.geometry_replays
      .map((strip) => ({
        id: strip.id,
        name:
          strip.id === "gms:40FXJKCA78QRCR5YNVX5TV8TVN"
            ? "6th–Brannan · open deck edge"
            : `${strip.kind.replaceAll("_", " ")} · CNN ${strip.cnn} · ${strip.profile_replay.status.replaceAll("_", " ")} · ${strip.id.slice(-5)}`,
        start: strip.reproducible_start,
      }))
      .filter((drive) => drive.start),
  ];
}

export function regionalStart(drive, surface) {
  if (!surface || surface.profile.length < 2) return null;
  const { station_m, forward } = drive.start;
  // Keep starts close to the published station and fit all four wheels.
  const indices = surface.profile
    .map(({ s_m }, i) => ({ i, distance: Math.abs(s_m - station_m) }))
    .filter(({ distance }) => distance <= 6)
    .sort((a, b) => a.distance - b.distance);
  for (const { i } of indices) {
    const {
      position: [x, y],
      support,
    } = surface.profile[i];
    const ahead =
      surface.profile[Math.min(i + 2, surface.profile.length - 1)].position;
    const behind = surface.profile[Math.max(i - 2, 0)].position;
    const heading =
      Math.atan2(ahead[1] - behind[1], ahead[0] - behind[0]) +
      (forward ? 0 : Math.PI);
    const start = {
      id: drive.id,
      name: drive.name,
      surfaceId: surface.id,
      x,
      y,
      heading,
    };
    if (
      supported(support) &&
      surface.heightAt(x, y) !== null &&
      roadSupportsCar(surface.heightAt, start)
    )
      return start;
  }
  return null;
}

export function indexRoadNetwork(confirmedConnections) {
  const confirmed = new Set(
    confirmedConnections
      .filter((join) => join.status === "joined")
      .map((join) => join.id),
  );
  const endpoints = new Map();
  const transitions = new Map();
  return {
    endpoints,
    transitions,
    add(tile) {
      for (const endpoint of tile.endpoints) {
        if (supported(endpoint.support)) endpoints.set(endpoint.id, endpoint);
      }
      for (const join of tile.transitions) {
        if (!confirmed.has(join.id) || !supported(join.support)) continue;
        const directions = [];
        if (["both", "forward"].includes(join.direction))
          directions.push([join.from, join.to]);
        if (["both", "reverse"].includes(join.direction))
          directions.push([join.to, join.from]);
        for (const [from, to] of directions) {
          if (!transitions.has(from)) transitions.set(from, new Set());
          transitions.get(from).add(to);
        }
      }
    },
  };
}

function terminals(surface) {
  const { profile } = surface;
  return [0, 1].map((end) => {
    const point = profile[end ? profile.length - 1 : 0].position;
    const other = profile[end ? profile.length - 2 : 1].position;
    const dx = point[0] - other[0];
    const dy = point[1] - other[1];
    const length = Math.hypot(dx, dy);
    const bearing = surface.terminalBearings?.[end];
    return {
      id: surface.end_ids[end],
      owner: surface.end_tiles[end],
      point,
      dx: bearing === undefined ? dx / length : Math.cos(bearing),
      dy: bearing === undefined ? dy / length : Math.sin(bearing),
    };
  });
}

function branchMiss(car, surface, terminal, travel) {
  const end = terminal === surface.end_ids[1];
  const profile = surface.profile;
  const station = end
    ? (profile.findLast((p) => profile.at(-1).s_m - p.s_m >= 8) ?? profile[0])
    : (profile.find((p) => p.s_m >= 8) ?? profile.at(-1));
  const [x, y] = station.position;
  const reach = Math.hypot(car.x - x, car.y - y);
  return Math.hypot(
    car.x + travel * reach * Math.cos(car.heading) - x,
    car.y + travel * reach * Math.sin(car.heading) - y,
  );
}

export function driveOnRoadNetwork(
  car,
  input,
  dt,
  surface,
  surfaces,
  graph,
  requestOwner,
) {
  const prior = car.seam && surfaces.get(car.seam.surfaceId);
  const near = (car, point, radius) =>
    Math.hypot(car.x - point[0], car.y - point[1]) <= radius;
  const sharedContact = (first, second, point) => (x, y) =>
    first.heightAt(x, y) ??
    (near({ x, y }, point, seamReachM) ? second.heightAt(x, y) : null);
  let next = driveOnRoadSurface(car, input, dt, surface);
  if (next.blocked && prior && near(car, car.seam.point, seamReachM)) {
    const straddling = driveOnRoadSurface(car, input, dt, {
      heightAt: sharedContact(surface, prior, car.seam.point),
    });
    if (surface.heightAt(straddling.x, straddling.y) !== null)
      next = straddling;
  }
  const travel =
    car.v < 0 ||
    (!car.v && (input.throttle < 0 || (input.brake && !input.throttle)))
      ? -1
      : 1;
  const hx = Math.cos(car.heading) * travel;
  const hy = Math.sin(car.heading) * travel;
  let waiting = false;
  for (const terminal of terminals(surface)) {
    const { id, owner, point, dx, dy } = terminal;
    if (!near(car, point, ownerPreloadM) || !(hx * dx + hy * dy > 0.25))
      continue;
    waiting = requestOwner(owner) || waiting;
    const candidates = [];
    let approach;
    for (const to of graph.transitions.get(id) || []) {
      const source = graph.endpoints.get(id);
      const destination = graph.endpoints.get(to);
      if (
        source?.surface_id !== surface.id ||
        !destination ||
        destination.surface_id === surface.id
      )
        continue;
      waiting = requestOwner(destination.surface_owner) || waiting;
      const target = surfaces.get(destination.surface_id);
      if (!target?.end_ids.includes(to) || !near(car, point, seamReachM))
        continue;
      const candidate = driveOnRoadSurface(car, input, dt, {
        heightAt: sharedContact(surface, target, point),
      });
      if (candidate.blocked) continue;
      const z = target.heightAt(candidate.x, candidate.y);
      const pastEnd =
        (candidate.x - point[0]) * dx + (candidate.y - point[1]) * dy >= 0;
      if (z !== null && pastEnd) {
        candidates.push({
          car: {
            ...candidate,
            z,
            surfaceId: target.id,
            seam: { surfaceId: surface.id, point },
          },
          miss: branchMiss(candidate, target, to, travel),
        });
      } else if (
        !pastEnd &&
        surface.heightAt(candidate.x, candidate.y) !== null
      ) {
        approach ??= candidate;
      }
    }
    // A fork can have several legal surfaces at the same transverse seam.
    // Use the vehicle's direction of travel to select among those surfaces.
    if (candidates.length && !waiting) {
      candidates.sort((a, b) => a.miss - b.miss);
      const [first, second] = candidates;
      if (!second || second.miss - first.miss > 0.05) return first.car;
    }
    if (approach) return approach;
  }
  if (next.blocked) return { ...next, waiting };
  return {
    ...next,
    seam: prior && near(next, car.seam.point, seamReachM) ? car.seam : null,
  };
}
