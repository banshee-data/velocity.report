import * as THREE from "../vendor/three/three.core.js";
import { buildingWalls, validBuildingFace } from "./sf-building-walls.js";
import { terrainTriangleRows } from "./sf-drive-surface.js";

const V = (x, y, z) => [x, z, -y];

function withNormals(positions) {
  const geometry = new THREE.BufferGeometry();
  geometry.setAttribute("position", new THREE.BufferAttribute(positions, 3));
  geometry.computeVertexNormals();
  return geometry.attributes.normal.array;
}

function appendBuildingWallCorners(outlines, edges) {
  const corners = new Map();
  for (const { a, b, za, zb } of edges) {
    for (const [point, neighbour, floor] of [
      [a, b, za],
      [b, a, zb],
    ]) {
      const key = point.join(",");
      const corner = corners.get(key) ?? { point, neighbours: [], floor };
      corner.neighbours.push(neighbour);
      corner.floor = Math.min(corner.floor, floor);
      corners.set(key, corner);
    }
  }
  for (const { point, neighbours, floor } of corners.values()) {
    // Only draw wall posts at real turns in the eaves. Straight edge
    // subdivisions and the ends of interior roof ridges need no posts.
    if (neighbours.length !== 2) continue;
    const [a, b] = neighbours;
    const ax = a[0] - point[0],
      ay = a[1] - point[1];
    const bx = b[0] - point[0],
      by = b[1] - point[1];
    if (
      Math.abs(ax * by - ay * bx) >
      0.5 * Math.hypot(ax, ay) * Math.hypot(bx, by)
    )
      outlines.push(...V(...point), ...V(point[0], point[1], floor));
  }
}

function* buildingBatches(tile, C, distant) {
  const [ox, oy] = tile.o;
  // Keep each building whole, but batch nearby buildings so the camera
  // rejects distant blocks before submitting their triangles and outlines.
  const batchSize = 1600; // decimetres; tile coordinates are local
  const batches = new Map();
  for (const b of tile.b) {
    yield;
    const validFaces = b[1].filter(validBuildingFace);
    const firstRing = validFaces[0]?.[0];
    if (!firstRing?.length) continue;
    if (
      distant &&
      !validFaces.some((face) =>
        face.some((ring) =>
          ring.some((z, index) => index % 3 === 2 && z - b[0] >= 420),
        ),
      )
    )
      continue;
    const key = `${Math.floor(firstRing[0] / batchSize)},${Math.floor(firstRing[1] / batchSize)}`;
    let batch = batches.get(key);
    if (!batch) {
      batch = { positions: [], colours: [], outlines: [] };
      batches.set(key, batch);
    }
    const { positions, colours, outlines } = batch;
    const zb = b[0] / 10;
    const roof = b[2] ? C.extra : C.roof;
    const wall = b[2] ? C.extra : C.wall;
    const roofEdges = new Map();
    const faces = validFaces.map((face) =>
      face.map((r) => {
        const a = [];
        for (let i = 0; i < r.length; i += 3)
          a.push([ox + r[i] / 10, oy + r[i + 1] / 10, r[i + 2] / 10]);
        // Drop a closing point that repeats the first: the triangulator drops it
        // too, and its indices must address the same vertices as `all` below.
        const f = a[0];
        const l = a.at(-1);
        if (a.length > 3 && f[0] === l[0] && f[1] === l[1]) a.pop();
        return a;
      }),
    );
    for (const rings of faces) {
      const outer = rings[0].map((v) => new THREE.Vector2(v[0], v[1]));
      const holes = rings
        .slice(1)
        .map((h) => h.map((v) => new THREE.Vector2(v[0], v[1])));
      let tris = [];
      try {
        tris = THREE.ShapeUtils.triangulateShape(outer, holes);
      } catch {
        tris = [];
      }
      const all = [].concat(...rings);
      for (const t of tris) {
        for (const k of t) {
          positions.push(...V(all[k][0], all[k][1], all[k][2]));
          colours.push(roof.r, roof.g, roof.b);
        }
      }
      if (distant) continue;
      for (const r of rings) {
        for (let i = 0; i < r.length; i++) {
          const a = r[i];
          const c = r[(i + 1) % r.length];
          const edge = [a.join(","), c.join(",")].sort().join("|");
          roofEdges.set(edge, [a, c]);
        }
      }
    }
    for (const [a, b] of roofEdges.values())
      outlines.push(...V(...a), ...V(...b));
    const exposedWalls = [];
    buildingWalls(faces, zb, (a, b, za, zb) => {
      const bottomA = [a[0], a[1], za];
      const bottomB = [b[0], b[1], zb];
      for (const point of [bottomA, bottomB, b, bottomA, b, a]) {
        positions.push(...V(...point));
        colours.push(wall.r, wall.g, wall.b);
      }
      if (!distant) exposedWalls.push({ a, b, za, zb });
    });
    if (!distant) appendBuildingWallCorners(outlines, exposedWalls);
  }

  const output = [];
  for (const batch of batches.values()) {
    if (!batch.positions.length) continue;
    const positions = new Float32Array(batch.positions);
    output.push({
      positions,
      normals: distant ? null : withNormals(positions),
      colours: new Float32Array(batch.colours),
      outlines: new Float32Array(batch.outlines),
    });
    yield;
  }
  return output;
}

export function* tileGeometry({ tile, colours, region }) {
  let terrain = null;
  if (region) {
    const surface = yield* terrainTriangleRows(tile, region);
    const positions = new Float32Array(surface.positions);
    terrain = {
      positions,
      uv: new Float32Array(surface.uv),
      normals: withNormals(positions),
    };
    yield;
  }
  const buildings = yield* buildingBatches(tile, colours, !region);
  return { buildings, terrain };
}
