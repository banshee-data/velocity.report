import * as THREE from "../vendor/three/three.module.js";
import { indexRoadContact } from "./sf-road-contact.js";

export const BAY_BRIDGE_CITY_DRIVE = "bay-bridge-highway-drive";
export const BAY_BRIDGE_APPROACH = "Bay Bridge approach";

const hypot = (a, b) => Math.hypot(a[0] - b[0], a[1] - b[1]);
const bearing = (a, b) => Math.atan2(b[1] - a[1], b[0] - a[0]);
const angle = (value) => Math.atan2(Math.sin(value), Math.cos(value));
const clamp = (value, min, max) => Math.min(max, Math.max(min, value));

export function applyCityDriveScenery(tile, exclusions) {
  if (!exclusions) return;
  for (const layer of ["b", "tr"]) {
    const records = exclusions[layer];
    if (!records) continue;
    tile[layer] = tile[layer].filter((record, index) => {
      const expected = records[index];
      if (!expected) return true;
      const signature =
        layer === "b"
          ? [record[0], record[1].length, record[2] ?? null]
          : record;
      return (
        signature.length !== expected.length ||
        signature.some((value, i) => value !== expected[i])
      );
    });
  }
}

// The published positions have no claimed surveyed width. The separate,
// authored game drive supplies its own road width and ordered connections.
export function buildCityGameDrive(drive, groundHeight) {
  const points = [];
  const spans = new Map();
  const references = new Set();
  for (const span of drive.sections) {
    if (
      spans.has(span.id) ||
      span.positions.length < 3 ||
      !Number.isFinite(span.width_m) ||
      span.width_m <= 2.5 ||
      span.width_m > 20 ||
      span.positions.some(
        (point) => point.length !== 3 || !point.every(Number.isFinite),
      )
    )
      throw new Error("The game drive contains an invalid road section.");
    const previous = points.at(-1);
    const first = span.positions[0];
    if (
      previous &&
      (hypot(previous, first) > 0.002 ||
        Math.abs(previous[2] - first[2]) > 0.002)
    )
      throw new Error("Two game road sections have an open seam.");
    const start = Math.max(0, points.length - 1);
    points.push(...(previous ? span.positions.slice(1) : span.positions));
    spans.set(span.id, { ...span, start, end: points.length - 1 });
    for (const source of span.sources) references.add(source.id);
  }
  if (spans.size < 2)
    throw new Error("The game drive needs connected sections.");
  const stations = [0];
  for (let i = 1; i < points.length; i++) {
    const distance = hypot(points[i - 1], points[i]);
    if (distance <= 0.002)
      throw new Error("The game drive contains overlapping road stations.");
    stations.push(stations[i - 1] + distance);
  }
  const tangents = points.map((_, i) =>
    bearing(
      points[Math.max(0, i - 1)],
      points[Math.min(i + 1, points.length - 1)],
    ),
  );
  const speed = points.map((_, i) => {
    const before = Math.max(0, i - 2);
    const after = Math.min(points.length - 1, i + 2);
    const curvature =
      Math.abs(angle(tangents[after] - tangents[before])) /
      (stations[after] - stations[before]);
    return Math.min(52, Math.sqrt(2.4 / Math.max(0.0001, curvature)));
  });
  const surfaces = [];
  const entries = [...spans.values()];
  const topology = { surfaces, endpoints: [], transitions: [] };
  for (const [number, span] of entries.entries()) {
    const { start, end, id, kind } = span;
    const fromWidth = Math.min(
      span.width_m,
      entries[number - 1]?.width_m ?? span.width_m,
    );
    const toWidth = Math.min(
      span.width_m,
      entries[number + 1]?.width_m ?? span.width_m,
    );
    const profile = [];
    const vertices = [];
    const triangles = [];
    for (let i = start; i <= end; i++) {
      const [x, y, z] = points[i];
      const s = stations[i] - stations[start];
      const remaining = stations[end] - stations[i];
      const width =
        Math.min(
          span.width_m - (span.width_m - fromWidth) * (1 - Math.min(1, s / 12)),
          span.width_m -
            (span.width_m - toWidth) * (1 - Math.min(1, remaining / 12)),
        ) / 2;
      const nx = -Math.sin(tangents[i]);
      const ny = Math.cos(tangents[i]);
      vertices.push(
        [x + nx * width, y + ny * width, z],
        [x - nx * width, y - ny * width, z],
      );
      profile.push({ position: points[i], s_m: s, support: "interpolated" });
      if (i > start) {
        const a = vertices.length - 4;
        triangles.push([a, a + 1, a + 2], [a + 1, a + 3, a + 2]);
      }
      speed[i] = Math.min(
        speed[i],
        span.speed_mps ?? Infinity,
        kind === "ground" ? 20 : kind === "ramp" ? 26 : 52,
      );
    }
    const contact = {
      vertices,
      triangles,
      support: vertices.map(() => "interpolated"),
    };
    const meshHeight = indexRoadContact(contact);
    const end_ids = [`${id}:start`, `${id}:end`];
    surfaces.push({
      id,
      kind,
      label: span.label,
      contact,
      profile,
      end_ids,
      end_tiles: ["game", "game"],
      terminalBearings: [tangents[start] + Math.PI, tangents[end]],
      rail: span.rail,
      railSides: span.rail_sides,
      railBounceOnly: id === "fourth-street-entrance",
      highwayRailHalfWidth: span.rail
        ? Math.min(fromWidth, toWidth) / 2 - 0.35
        : 0,
      heightAt:
        kind === "ground"
          ? (x, y) => (meshHeight(x, y) === null ? null : groundHeight(x, y))
          : meshHeight,
    });
    for (const [i, position] of [points[start], points[end]].entries())
      topology.endpoints.push({
        id: end_ids[i],
        position,
        surface_id: id,
        surface_owner: "game",
        support: "interpolated",
      });
    if (number > 0) {
      const previous = entries[number - 1];
      topology.transitions.push({
        id: `${previous.id}:${id}`,
        from: `${previous.id}:end`,
        to: end_ids[0],
        direction: "forward",
        support: "interpolated",
        status: "joined",
      });
    }
  }
  const end = stations.at(-1);
  for (let i = stations.length - 1; i >= 0; i--) {
    if (stations[i] >= end - 6) speed[i] = 0;
    if (i + 1 < stations.length)
      speed[i] = Math.min(
        speed[i],
        Math.sqrt(speed[i + 1] ** 2 + 9 * (stations[i + 1] - stations[i])),
      );
  }
  const start = Math.min(5, entries[0].end - 1);
  const [x, y] = points[start];
  const exit = spans.get("fremont-exit");
  let exitStart;
  if (exit) {
    const index = stations.findIndex(
      (s, i) => i > exit.start && s >= stations[exit.end] - 26,
    );
    exitStart = {
      id: "fremont-exit",
      name: "Fremont Street exit",
      surfaceId: exit.id,
      x: points[index][0],
      y: points[index][1],
      heading: tangents[index],
    };
  }
  return {
    topology,
    surfaces,
    references,
    sceneExclusions: drive.scene_exclusions,
    points,
    spans,
    stations,
    speed,
    end,
    exitStart,
    start: {
      id: drive.id,
      name:
        drive.id === BAY_BRIDGE_CITY_DRIVE ? BAY_BRIDGE_APPROACH : drive.label,
      surfaceId: entries[0].id,
      x,
      y,
      heading: tangents[start],
      initialSpeed: drive.id === BAY_BRIDGE_CITY_DRIVE ? 50 * 0.44704 : 0,
    },
  };
}

export function cityDriveControls(drive, car) {
  const span = drive.spans.get(car.surfaceId);
  if (!span || car.grounded === false) return null;
  const { points, stations, speed } = drive;
  let best = Infinity;
  let s = stations[span.start];
  let at = span.start;
  for (let i = span.start; i < span.end; i++) {
    const a = points[i];
    const b = points[i + 1];
    const dx = b[0] - a[0];
    const dy = b[1] - a[1];
    const length = stations[i + 1] - stations[i];
    const u = clamp(
      ((car.x - a[0]) * dx + (car.y - a[1]) * dy) / (length * length),
      0,
      1,
    );
    const d = Math.hypot(car.x - a[0] - dx * u, car.y - a[1] - dy * u);
    if (d >= best) continue;
    best = d;
    s = stations[i] + u * length;
    at = i;
  }
  const ahead = s + clamp(4 + Math.abs(car.v) * 0.45, 5, 21);
  let p = at + 1;
  while (p + 1 < points.length && stations[p] < ahead) p++;
  const dx = points[p][0] - car.x;
  const dy = points[p][1] - car.y;
  const rotation = angle(Math.atan2(dy, dx) - car.heading);
  const steer =
    Math.atan((5.4 * Math.sin(rotation)) / Math.max(1, Math.hypot(dx, dy))) /
    (0.58 / (1 + Math.abs(car.v) / 20));
  const goal = Math.min(speed[at], speed[at + 1] + 0.75);
  return {
    throttle: goal > car.v + 0.15 ? clamp((goal - car.v) * 0.35, 0.15, 1) : 0,
    brake:
      car.v > goal + 0.35 ? clamp((car.v - goal) * 0.35, 0.15, 1) : goal === 0,
    steer: clamp(steer, -1, 1),
    finished: s >= drive.end - 8 && goal === 0 && Math.abs(car.v) < 0.1,
  };
}

export function cityRoadPreview(tile, usedProfiles) {
  const group = new THREE.Group();
  for (const [layer, color] of [
    ["overpass", 0x7ccfdf],
    ["bridge", 0xffd072],
  ]) {
    const points = [];
    for (const item of tile[layer]) {
      if (usedProfiles?.has(item.id)) continue;
      for (let i = 1; i < item.profile.length; i++) {
        const a = item.profile[i - 1].position;
        const b = item.profile[i].position;
        if (a[2] === null || b[2] === null) continue;
        for (const [x, y, z] of [a, b]) points.push(x, z + 0.04, -y);
      }
    }
    if (!points.length) continue;
    const geometry = new THREE.BufferGeometry();
    geometry.setAttribute(
      "position",
      new THREE.Float32BufferAttribute(points, 3),
    );
    group.add(
      new THREE.LineSegments(geometry, new THREE.LineBasicMaterial({ color })),
    );
  }
  return group;
}
