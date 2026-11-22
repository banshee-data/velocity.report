import { driveOnRoadSurface, roadSupportsCar } from "./sf-road-contact.js";

const supported = (value) => ["observed", "interpolated"].includes(value);
const seamReachM = 4;
const ownerPreloadM = 12;

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
        name: `${strip.kind.replaceAll("_", " ")} · CNN ${strip.cnn} · ${strip.profile_replay.status.replaceAll("_", " ")} · ${strip.id.slice(-5)}`,
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
    return {
      id: surface.end_ids[end],
      owner: surface.end_tiles[end],
      point,
      dx: dx / length,
      dy: dy / length,
    };
  });
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
    car.v < 0 || (!car.v && input.brake && !input.throttle) ? -1 : 1;
  const hx = Math.cos(car.heading) * travel;
  const hy = Math.sin(car.heading) * travel;
  let waiting = false;
  for (const terminal of terminals(surface)) {
    const { id, owner, point, dx, dy } = terminal;
    if (!near(car, point, ownerPreloadM) || !(hx * dx + hy * dy > 0.25))
      continue;
    waiting = requestOwner(owner) || waiting;
    const candidates = [];
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
          ...candidate,
          z,
          surfaceId: target.id,
          seam: { surfaceId: surface.id, point },
        });
      } else if (
        !pastEnd &&
        surface.heightAt(candidate.x, candidate.y) !== null
      ) {
        candidates.push(candidate);
      }
    }
    if (candidates.length === 1) return candidates[0];
  }
  if (next.blocked) return { ...next, waiting };
  return {
    ...next,
    seam: prior && near(next, car.seam.point, seamReachM) ? car.seam : null,
  };
}
