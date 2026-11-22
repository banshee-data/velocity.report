import * as THREE from "../vendor/three/three.module.js";
import { carStep } from "./sf-drive-core.js";

export function* contactTriangles(contact) {
  for (const triangle of contact.triangles) {
    const vertices = triangle.map((index) => contact.vertices[index]);
    if (vertices.some((point) => !point?.every(Number.isFinite))) continue;
    const support = triangle.map((index) => contact.support[index]);
    if (
      support.some((value) => value !== "observed" && value !== "interpolated")
    )
      continue;
    yield { vertices, interpolated: support.includes("interpolated") };
  }
}

export function contactGeometry(contact, showSupport = true) {
  const observed = [];
  const interpolated = [];
  for (const face of contactTriangles(contact)) {
    const positions =
      showSupport && face.interpolated ? interpolated : observed;
    for (const [east, north, height] of face.vertices)
      positions.push(east, height, -north);
  }
  const geometry = new THREE.BufferGeometry();
  geometry.setAttribute(
    "position",
    new THREE.Float32BufferAttribute(observed.concat(interpolated), 3),
  );
  if (showSupport) {
    geometry.addGroup(0, observed.length / 3, 0);
    geometry.addGroup(observed.length / 3, interpolated.length / 3, 1);
  }
  geometry.computeVertexNormals();
  return geometry;
}

// The overlay carries small triangles. Bucketing them keeps queries local at 120 Hz.
export function indexRoadContact(contact) {
  const cells = new Map();
  const size = 2;
  const key = (x, y) => `${x},${y}`;
  for (const {
    vertices: [a, b, c],
  } of contactTriangles(contact)) {
    const dx = b[0] - a[0],
      dy = b[1] - a[1];
    const ex = c[0] - a[0],
      ey = c[1] - a[1];
    const determinant = dx * ey - dy * ex;
    if (Math.abs(determinant) < 1e-10) continue;
    const face = { a, b, c, dx, dy, ex, ey, determinant };
    for (
      let x = Math.floor(Math.min(a[0], b[0], c[0]) / size);
      x <= Math.floor(Math.max(a[0], b[0], c[0]) / size);
      x++
    )
      for (
        let y = Math.floor(Math.min(a[1], b[1], c[1]) / size);
        y <= Math.floor(Math.max(a[1], b[1], c[1]) / size);
        y++
      ) {
        const k = key(x, y);
        if (!cells.has(k)) cells.set(k, []);
        cells.get(k).push(face);
      }
  }
  return (x, y) => {
    for (const { a, b, c, dx, dy, ex, ey, determinant } of cells.get(
      key(Math.floor(x / size), Math.floor(y / size)),
    ) || []) {
      const px = x - a[0],
        py = y - a[1];
      const u = (px * ey - py * ex) / determinant;
      const v = (dx * py - dy * px) / determinant;
      if (u >= -1e-8 && v >= -1e-8 && u + v <= 1 + 1e-8)
        return a[2] + u * (b[2] - a[2]) + v * (c[2] - a[2]);
    }
    return null;
  };
}

export function roadSupportsCar(heightAt, car) {
  const hx = Math.cos(car.heading),
    hy = Math.sin(car.heading);
  for (const axle of [-1.35, 1.35])
    for (const wheel of [-0.75, 0.75])
      if (
        heightAt(
          car.x + hx * axle - hy * wheel,
          car.y + hy * axle + hx * wheel,
        ) === null
      )
        return false;
  return true;
}

export const BAY_BRIDGE_BASE =
  "48b30e1eeee788e89ed01db81ee4d37c4224b62dcf7902de85bcdc653c634a61";

export function bayBridgeStarts(surfaces) {
  return [
    ["bay-bridge-deck", "Bay Bridge · upper deck", "bridge_deck", -0.018],
    ["bay-bridge-street", "Embarcadero · beneath the bridge", "ground", -0.006],
  ].flatMap(([id, name, kind, alignment]) => {
    const surface = [...surfaces.values()].find((entry) => entry.kind === kind);
    if (!surface || surface.profile.length < 2) return [];
    const ordered =
      surface.direction === "reverse"
        ? [...surface.profile].reverse()
        : surface.profile;
    const begin = ordered[0].position;
    const end = ordered.at(-1).position;
    const bearing = Math.atan2(end[1] - begin[1], end[0] - begin[0]);
    // Inset each start to run through the supported crossing at full car width.
    for (const {
      position: [east, north],
    } of ordered.slice(8)) {
      const x = east + 2 * Math.sin(bearing);
      const y = north - 2 * Math.cos(bearing);
      const start = {
        id,
        name,
        surfaceId: surface.id,
        x,
        y,
        heading: bearing + alignment,
      };
      if (
        roadSupportsCar(surface.heightAt, start) &&
        surface.heightAt(x, y) !== null
      )
        return [start];
    }
    return [];
  });
}

export function roadMotion(car, input, dt, heightAt) {
  const h = Math.cos(car.heading) * 1.35,
    n = Math.sin(car.heading) * 1.35;
  const front = heightAt(car.x + h, car.y + n),
    rear = heightAt(car.x - h, car.y - n);
  return carStep(car, input, dt, {
    slope: front !== null && rear !== null ? (front - rear) / 2.7 : 0,
  });
}

export function driveOnRoadSurface(car, input, dt, surface) {
  const { heightAt } = surface;
  const next = roadMotion(car, input, dt, heightAt);
  const z = heightAt(next.x, next.y);
  if (z === null || !roadSupportsCar(heightAt, next))
    return { ...car, blocked: true };
  return {
    ...next,
    surfaceId: car.surfaceId,
    z,
    grounded: true,
    blocked: false,
  };
}
