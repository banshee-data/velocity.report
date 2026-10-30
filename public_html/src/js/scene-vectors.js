import { sceneFade } from "./scene-fade.js";
// Gerardus's street geometry in a scene: kerbs, walls, roofs, trees, and registered
// ground, as Gerardus measured or inferred them, drawn in the
// scene's own sensor frame.
//
// tools/s2-archive/scene-vectors.py writes the file, placing each feature by
// the inverse of the site's accepted pose, so it arrives in the same frame as
// the points and boxes and the player only maps it to three.js axes. A scene
// without an accepted pose has no file, and the player draws nothing extra.
//
// Everything here is drawn as thin lines and faint fills under the boxes: the
// geometry is context for the traffic, and should lose every contrast fight
// with it. The geometry is built as plain arrays first (vectorLayers), so it
// can be tested under Node without WebGL.

import * as THREE from "three";
import { horizonBasis } from "./scene-horizon.js";
import { toSceneX, toSceneY, toSceneZ } from "./scene-coords.js";

/**
 * How each layer is drawn. Colours avoid the class colours (vehicle red,
 * cycle blue, walking green, bird purple), so a wall never reads as a road
 * user. `fill` is the opacity of a polygon's face, `line` of its edges.
 */
export const VECTOR_STYLE = {
  trees: { label: "trees", colour: 0x245238, fill: 0.16, line: 0.45 },
  city_streets: { label: "street centrelines", colour: 0x56656e, line: 0.4 },
  footprints: { label: "building footprints", colour: 0x7b8790, line: 0.35 },
  observed: {
    label: "kerbs",
    colour: 0xd3dbe0,
    line: 0.75,
    lift: 0.06,
  },
  box_walls: {
    label: "building walls",
    colour: 0x74838d,
    fill: 0.04,
    line: 0.11,
  },
  wall_surfaces: {
    label: "building walls",
    colour: 0x74838d,
    fill: 0.04,
    line: 0.11,
  },
  roof_planes: {
    label: "roof planes",
    colour: 0x9a8f7c,
    fill: 0.05,
    line: 0.14,
  },
};

const toScene = (p) => [toSceneX(p[0]), toSceneY(p[2] ?? 0), toSceneZ(p[1])];

/** A ring without its closing repeat. */
function openRing(ring) {
  if (ring.length > 1) {
    const a = ring[0];
    const b = ring[ring.length - 1];
    if (a[0] === b[0] && a[1] === b[1] && (a[2] ?? 0) === (b[2] ?? 0)) {
      return ring.slice(0, -1);
    }
  }
  return ring;
}

/** A ring's normal by Newell's method, in the sensor frame. */
export function ringNormal(ring) {
  const n = [0, 0, 0];
  for (let i = 0; i < ring.length; i++) {
    const a = ring[i];
    const b = ring[(i + 1) % ring.length];
    const az = a[2] ?? 0;
    const bz = b[2] ?? 0;
    n[0] += (a[1] - b[1]) * (az + bz);
    n[1] += (az - bz) * (a[0] + b[0]);
    n[2] += (a[0] - b[0]) * (a[1] + b[1]);
  }
  return n;
}

/**
 * Triangulates a planar polygon in 3D, flat or upright: it is projected onto
 * the coordinate plane it faces most squarely and cut into triangles there.
 * Returns indices over the outer ring's vertices, then each hole's.
 */
export function triangulatePolygon(outer, holes = []) {
  if (outer.length < 3) return [];
  const n = ringNormal(outer);
  const a = n.map(Math.abs);
  if (a[0] + a[1] + a[2] < 1e-9) return [];
  const drop = a[2] >= a[0] && a[2] >= a[1] ? 2 : a[0] >= a[1] ? 0 : 1;
  const [u, v] = drop === 2 ? [0, 1] : drop === 0 ? [1, 2] : [2, 0];
  const flat = (ring) => ring.map((p) => new THREE.Vector2(p[u], p[v] ?? 0));
  const faces = THREE.ShapeUtils.triangulateShape(flat(outer), holes.map(flat));
  return faces.flat();
}

function pushSegments(out, points, closed) {
  for (let i = 0; i + 1 < points.length; i++)
    out.push(points[i], points[i + 1]);
  if (closed && points.length > 2)
    out.push(points[points.length - 1], points[0]);
}

function lifted(p, lift) {
  return lift ? [p[0], p[1], (p[2] ?? 0) + lift] : p;
}

/** Infer trunk and crown along registered gravity, retaining the measured base. */
function treeGeometry(destination, points, props, basis) {
  if (!points.length) return;
  // Construct in the level frame; convert every generated vertex back to the
  // sensor frame before the shared renderer maps it to three.js axes.
  points = points.map((p) => {
    const q = basis.toLevel(toScene(p));
    return [q[0], q[2], q[1]];
  });
  const layer = { segments: [], positions: [], indices: [] };
  const a = points[0],
    b = points[points.length - 1];
  const [base, top] = (a[2] ?? 0) <= (b[2] ?? 0) ? [a, b] : [b, a];
  const r = props.crown_radius_m > 0 ? props.crown_radius_m : 2;
  const z0 = base[2] ?? 0;
  const bottom = Number.isFinite(props.crown_base_m)
    ? z0 + props.crown_base_m
    : (top[2] ?? z0);
  const crownTop = Number.isFinite(props.height_m)
    ? z0 + props.height_m
    : bottom + 2 * r;
  const half = Math.max(0.5, (crownTop - bottom) / 2);
  const mid = bottom + half;
  layer.segments.push(base, [base[0], base[1], bottom]);
  const start = layer.positions.length;
  const rows = 10,
    columns = 20;
  for (let j = 0; j <= rows; j++) {
    const latitude = (j / rows) * Math.PI;
    for (let i = 0; i < columns; i++) {
      const longitude = (i / columns) * 2 * Math.PI;
      layer.positions.push([
        base[0] + r * Math.sin(latitude) * Math.cos(longitude),
        base[1] + r * Math.sin(latitude) * Math.sin(longitude),
        mid + half * Math.cos(latitude),
      ]);
    }
  }
  for (let j = 0; j < rows; j++) {
    for (let i = 0; i < columns; i++) {
      const a = start + j * columns + i,
        b = start + j * columns + ((i + 1) % columns);
      layer.indices.push(a, a + columns, b, b, a + columns, b + columns);
    }
  }
  // Three restrained crown rings keep its extent legible without a dense wire mesh.
  for (const plane of [0, 1, 2]) {
    const ring = [];
    for (let i = 0; i < columns; i++) {
      const t = (i / columns) * 2 * Math.PI;
      ring.push(
        plane === 0
          ? [base[0] + r * Math.cos(t), base[1] + r * Math.sin(t), mid]
          : plane === 1
            ? [base[0] + r * Math.cos(t), base[1], mid + half * Math.sin(t)]
            : [base[0], base[1] + r * Math.cos(t), mid + half * Math.sin(t)],
      );
    }
    pushSegments(layer.segments, ring, true);
  }
  const sensor = (p) => {
    const q = basis.fromLevel([p[0], p[2], p[1]]);
    return [q[0], -q[2], q[1]];
  };
  const offset = destination.positions.length;
  destination.segments.push(...layer.segments.map(sensor));
  destination.positions.push(...layer.positions.map(sensor));
  destination.indices.push(...layer.indices.map((i) => i + offset));
}

function polygonsOf(geometry) {
  if (geometry.type === "Polygon") return [geometry.coordinates];
  if (geometry.type === "MultiPolygon") return geometry.coordinates;
  return [];
}

function linesOf(geometry) {
  if (geometry.type === "LineString") return [geometry.coordinates];
  if (geometry.type === "MultiLineString") return geometry.coordinates;
  return [];
}

/**
 * The geometry of each layer the file carries, in three.js scene axes:
 * `lines` as segment pairs for a LineSegments, `fill` as triangles for a
 * Mesh where the layer fills its polygons. Layers come in the file's order.
 */
export function vectorLayers(collection) {
  const layers = new Map();
  const basis = horizonBasis(collection?.placement?.up_sensor);
  for (const f of collection?.features ?? []) {
    const props = f.properties ?? {};
    const style = VECTOR_STYLE[props.layer];
    if (!style || !f.geometry) continue;
    if (props.layer === "observed" && props.kind === "facade") continue;
    if (
      props.layer === "footprints" &&
      collection.features.some((f) =>
        ["wall_surfaces", "box_walls"].includes(f.properties?.layer),
      )
    )
      continue;
    let layer = layers.get(props.layer);
    if (!layer) {
      layer = {
        layer: props.layer,
        style,
        segments: [],
        positions: [],
        indices: [],
        count: 0,
      };
      layers.set(props.layer, layer);
    }
    layer.count++;
    const lift = style.lift ?? 0;
    if (props.layer === "trees") {
      treeGeometry(layer, linesOf(f.geometry)[0] ?? [], props, basis);
      continue;
    }

    for (const line of linesOf(f.geometry)) {
      pushSegments(
        layer.segments,
        line.map((p) => lifted(p, lift)),
        false,
      );
    }
    for (const rings of polygonsOf(f.geometry)) {
      const [outer, ...holes] = rings.map((r) =>
        openRing(r).map((p) => lifted(p, lift)),
      );
      if (!outer || outer.length < 3) continue;
      pushSegments(layer.segments, outer, true);
      for (const h of holes) pushSegments(layer.segments, h, true);
      if (!style.fill) continue;
      const start = layer.positions.length;
      const tris = triangulatePolygon(outer, holes);
      for (const p of [outer, ...holes].flat()) layer.positions.push(p);
      for (const i of tris) layer.indices.push(start + i);
    }
  }
  return [...layers.values()].map((l) => ({
    layer: l.layer,
    style: l.style,
    count: l.count,
    lines: Float32Array.from(l.segments.flatMap(toScene)),
    fill: l.indices.length
      ? {
          positions: Float32Array.from(l.positions.flatMap(toScene)),
          indices: Uint32Array.from(l.indices),
        }
      : null,
  }));
}

/**
 * A group holding every layer's lines and fills. Fills are drawn before the
 * boxes and write no depth, so a wall never hides a car behind it; lines
 * test depth against nothing but each other for the same reason.
 */
export function createVectorGroup(collection) {
  const group = new THREE.Group();
  group.name = "vectors";
  for (const l of vectorLayers(collection)) {
    if (l.fill) {
      const geo = new THREE.BufferGeometry();
      geo.setAttribute(
        "position",
        new THREE.BufferAttribute(l.fill.positions, 3),
      );
      geo.setIndex(new THREE.BufferAttribute(l.fill.indices, 1));
      const mesh = new THREE.Mesh(
        geo,
        sceneFade(
          new THREE.MeshBasicMaterial({
            color: l.style.colour,
            transparent: true,
            opacity: l.style.fill,
            side: THREE.DoubleSide,
            depthWrite: false,
          }),
        ),
      );
      mesh.name = `${l.layer}:fill`;
      mesh.renderOrder = -2;
      group.add(mesh);
    }
    if (l.lines.length) {
      const geo = new THREE.BufferGeometry();
      geo.setAttribute("position", new THREE.BufferAttribute(l.lines, 3));
      const lines = new THREE.LineSegments(
        geo,
        sceneFade(
          new THREE.LineBasicMaterial({
            color: l.style.colour,
            transparent: true,
            opacity: l.style.line,
            depthWrite: false,
          }),
        ),
      );
      lines.name = `${l.layer}:lines`;
      lines.renderOrder = -1;
      group.add(lines);
    }
  }
  return group;
}

/** Ground at its registered heights, independent of the street-geometry switch. */
export function createGroundGroup(collection) {
  const group = new THREE.Group();
  group.name = "ground";
  const ground = collection?.ground;
  if (!ground?.indices?.length) return group;
  const geometry = new THREE.BufferGeometry();
  geometry.setAttribute(
    "position",
    new THREE.BufferAttribute(
      Float32Array.from(ground.positions.flatMap(toScene)),
      3,
    ),
  );
  geometry.setIndex(ground.indices);
  const mesh = new THREE.Mesh(
    geometry,
    sceneFade(
      new THREE.MeshBasicMaterial({
        color: 0x1c2b32,
        transparent: true,
        opacity: 0.12,
        side: THREE.DoubleSide,
        depthWrite: false,
      }),
    ),
  );
  mesh.renderOrder = -4;
  group.add(mesh);
  const wire = new THREE.LineSegments(
    new THREE.WireframeGeometry(geometry),
    sceneFade(
      new THREE.LineBasicMaterial({
        color: 0x2c4049,
        transparent: true,
        opacity: 0.25,
        depthWrite: false,
      }),
    ),
  );
  wire.renderOrder = -3;
  group.add(wire);
  return group;
}

/** Reads a vectors file, gzipped or not; null when the scene has none. */
export async function loadVectors(url) {
  const res = await fetch(url);
  if (!res.ok) return null;
  const raw = new Uint8Array(await res.arrayBuffer());
  const isGzip = raw.length > 1 && raw[0] === 0x1f && raw[1] === 0x8b;
  const text = isGzip
    ? await new Response(
        new Blob([raw]).stream().pipeThrough(new DecompressionStream("gzip")),
      ).text()
    : new TextDecoder().decode(raw);
  const collection = JSON.parse(text);
  if (
    !Array.isArray(collection.features) ||
    (collection.features.length === 0 && !collection.ground?.indices?.length)
  )
    return null;
  return collection;
}
