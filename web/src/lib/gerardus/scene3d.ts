/**
 * The 3D view's geometry, built as plain typed arrays from Gerardus's
 * site-frame layers, so it can be tested without WebGL. The view turns each
 * array into a three.js buffer geometry and nothing more.
 *
 * The site frame is x east, y north, z up, in metres; the view keeps it as it
 * is and sets the camera's up to +z.
 */
import earcut from 'three/src/extras/lib/earcut.js';
import type { GroundGrid, SiteFrameFeature, SitePosition } from './client';
import { laneArrows, partsOf, polygonsOf } from './geometry';
import { featureColour } from './style';

export type Vec3 = SitePosition;

/** Triangles: x, y, z per vertex, three vertex indices per triangle. */
export interface MeshData {
	positions: Float32Array;
	indices: Uint32Array;
}

const EMPTY_MESH: MeshData = { positions: new Float32Array(0), indices: new Uint32Array(0) };

// ---- The ground -----------------------------------------------------------------

/**
 * The ground grid as a mesh: a vertex at the centre of each cell that has a
 * height, two triangles for each square of four such cells, and one for a
 * square missing one corner, so the mesh stops at empty cells rather than
 * bridging them. Triangles wind counter-clockwise seen from above.
 */
export function groundMesh(g: GroundGrid): MeshData {
	const { nx, ny, cellM } = g;
	const vertex = new Int32Array(nx * ny).fill(-1);
	const positions: number[] = [];
	let n = 0;
	for (let j = 0; j < ny; j++) {
		for (let i = 0; i < nx; i++) {
			const h = g.heights[j * nx + i];
			if (h === null) continue;
			vertex[j * nx + i] = n++;
			positions.push(g.x0 + (i + 0.5) * cellM, g.y0 + (j + 0.5) * cellM, h);
		}
	}
	const indices: number[] = [];
	for (let j = 0; j + 1 < ny; j++) {
		for (let i = 0; i + 1 < nx; i++) {
			// Counter-clockwise from above: (i, j), (i+1, j), (i+1, j+1), (i, j+1).
			const quad = [
				vertex[j * nx + i],
				vertex[j * nx + i + 1],
				vertex[(j + 1) * nx + i + 1],
				vertex[(j + 1) * nx + i]
			];
			const present = quad.filter((v) => v >= 0);
			if (present.length === 4) indices.push(quad[0], quad[1], quad[2], quad[0], quad[2], quad[3]);
			else if (present.length === 3) indices.push(...present);
		}
	}
	return { positions: new Float32Array(positions), indices: new Uint32Array(indices) };
}

// ---- Polygons ----------------------------------------------------------------------

function sub(a: Vec3, b: Vec3): Vec3 {
	return [a[0] - b[0], a[1] - b[1], a[2] - b[2]];
}

function cross(a: Vec3, b: Vec3): Vec3 {
	return [a[1] * b[2] - a[2] * b[1], a[2] * b[0] - a[0] * b[2], a[0] * b[1] - a[1] * b[0]];
}

function dot(a: Vec3, b: Vec3): number {
	return a[0] * b[0] + a[1] * b[1] + a[2] * b[2];
}

/** A ring's normal by Newell's method, its length twice the ring's area. */
export function ringNormal(ring: Vec3[]): Vec3 {
	const n: Vec3 = [0, 0, 0];
	for (let i = 0; i < ring.length; i++) {
		const a = ring[i];
		const b = ring[(i + 1) % ring.length];
		n[0] += (a[1] - b[1]) * (a[2] + b[2]);
		n[1] += (a[2] - b[2]) * (a[0] + b[0]);
		n[2] += (a[0] - b[0]) * (a[1] + b[1]);
	}
	return n;
}

/**
 * Triangulates a planar polygon in 3D, flat or upright: it is projected onto
 * the coordinate plane it faces most squarely, cut into triangles there
 * (earcut), and each triangle is wound counter-clockwise seen from `facing`,
 * the polygon's own normal when not given. Indices run over the outer ring
 * and then each hole. A ring with no area has none.
 */
export function triangulate(outer: Vec3[], holes: Vec3[][] = [], facing?: Vec3): number[] {
	if (outer.length < 3) return [];
	const normal = ringNormal(outer);
	const size = Math.hypot(...normal);
	if (size < 1e-9) return [];
	const abs = normal.map(Math.abs);
	const drop = abs[2] >= abs[0] && abs[2] >= abs[1] ? 2 : abs[0] >= abs[1] ? 0 : 1;
	const [u, v] = drop === 2 ? [0, 1] : drop === 0 ? [1, 2] : [2, 0];
	const all = [outer, ...holes].flat();
	const flat: number[] = [];
	for (const p of all) flat.push(p[u], p[v]);
	const holeStarts: number[] = [];
	let start = outer.length;
	for (const h of holes) {
		holeStarts.push(start);
		start += h.length;
	}
	const tris = earcut(flat, holeStarts.length ? holeStarts : undefined, 2);
	const want = facing ?? normal;
	for (let t = 0; t < tris.length; t += 3) {
		const [a, b, c] = [all[tris[t]], all[tris[t + 1]], all[tris[t + 2]]];
		if (dot(cross(sub(b, a), sub(c, a)), want) < 0) {
			[tris[t + 1], tris[t + 2]] = [tris[t + 2], tris[t + 1]];
		}
	}
	return tris;
}

/** Several meshes as one, so a layer's fills of one colour are one draw. */
export function mergeMeshes(meshes: MeshData[]): MeshData {
	const live = meshes.filter((m) => m.indices.length > 0);
	if (live.length === 0) return EMPTY_MESH;
	if (live.length === 1) return live[0];
	const positions = new Float32Array(live.reduce((n, m) => n + m.positions.length, 0));
	const indices = new Uint32Array(live.reduce((n, m) => n + m.indices.length, 0));
	let p = 0;
	let k = 0;
	for (const m of live) {
		positions.set(m.positions, p);
		const base = p / 3;
		for (let i = 0; i < m.indices.length; i++) indices[k + i] = m.indices[i] + base;
		p += m.positions.length;
		k += m.indices.length;
	}
	return { positions, indices };
}

function lifted(points: Vec3[], lift: number): number[] {
	const out: number[] = [];
	for (const p of points) out.push(p[0], p[1], p[2] + lift);
	return out;
}

/**
 * A surface draped on its own vertices' heights: triangulated facing up and
 * raised by `lift` metres, so a fill on the ground is not lost in it.
 */
export function surfaceMesh(outer: Vec3[], holes: Vec3[][] = [], lift = 0): MeshData {
	const indices = triangulate(outer, holes, [0, 0, 1]);
	if (indices.length === 0) return EMPTY_MESH;
	return {
		positions: new Float32Array(lifted([outer, ...holes].flat(), lift)),
		indices: new Uint32Array(indices)
	};
}

/** An upright polygon, such as a wall plane, triangulated in its own plane. */
export function planeMesh(ring: Vec3[]): MeshData {
	const indices = triangulate(ring);
	if (indices.length === 0) return EMPTY_MESH;
	return { positions: new Float32Array(lifted(ring, 0)), indices: new Uint32Array(indices) };
}

/** Twice a ring's signed area seen from above: positive when it runs counter-clockwise. */
function signedArea2(ring: Vec3[]): number {
	let a = 0;
	for (let i = 0; i < ring.length; i++) {
		const p = ring[i];
		const q = ring[(i + 1) % ring.length];
		a += p[0] * q[1] - q[0] * p[1];
	}
	return a;
}

/**
 * A box: its footprint, at the base z of each vertex, raised `height` metres.
 * Each wall is its own quad, wound to face out of the solid (into a hole for
 * a hole's walls), and the top is the footprint triangulated facing up. There
 * is no floor. A box with no height, or a footprint with no area, has none.
 */
export function extrudeBox(outer: Vec3[], height: number, holes: Vec3[][] = []): MeshData {
	if (!(height > 0) || outer.length < 3 || Math.abs(signedArea2(outer)) < 1e-9) return EMPTY_MESH;
	const positions: number[] = [];
	const indices: number[] = [];
	const rings = [outer, ...holes];
	rings.forEach((ring, r) => {
		// Counter-clockwise outer rings face out to the right of each edge.
		const flip = signedArea2(ring) < 0 !== r > 0;
		for (let i = 0; i < ring.length; i++) {
			const a = ring[i];
			const b = ring[(i + 1) % ring.length];
			const v = positions.length / 3;
			positions.push(a[0], a[1], a[2], b[0], b[1], b[2]);
			positions.push(b[0], b[1], b[2] + height, a[0], a[1], a[2] + height);
			if (flip) indices.push(v, v + 2, v + 1, v, v + 3, v + 2);
			else indices.push(v, v + 1, v + 2, v, v + 2, v + 3);
		}
	});
	const top = triangulate(outer, holes, [0, 0, 1]);
	const v0 = positions.length / 3;
	positions.push(...lifted(rings.flat(), height));
	for (const i of top) indices.push(v0 + i);
	return { positions: new Float32Array(positions), indices: new Uint32Array(indices) };
}

// ---- Lines and points ------------------------------------------------------------------

/** Lines and rings as segment pairs (for a LineSegments), raised by `lift`. Rings close. */
export function lineSegments(parts: { points: Vec3[]; closed: boolean }[], lift = 0): Float32Array {
	const out: number[] = [];
	for (const { points, closed } of parts) {
		const n = points.length;
		const edges = closed && n > 2 ? n : n - 1;
		for (let i = 0; i < edges; i++) {
			const a = points[i];
			const b = points[(i + 1) % n];
			out.push(a[0], a[1], a[2] + lift, b[0], b[1], b[2] + lift);
		}
	}
	return new Float32Array(out);
}

/**
 * Chevrons along a lane, every `spacing` metres in its direction of travel
 * (first vertex to last), each `size` metres from tip to tail, at the lane's
 * height there, raised by `lift`: two segments each.
 */
export function laneArrowSegments(points: Vec3[], spacing = 8, size = 1.2, lift = 0): Float32Array {
	const en = points.map((p) => ({ east: p[0], north: p[1] }));
	const out: number[] = [];
	for (const a of laneArrows(en, spacing)) {
		const p = points[a.segment];
		const q = points[a.segment + 1];
		const z = p[2] + (q[2] - p[2]) * a.t + lift;
		const theta = (a.bearing * Math.PI) / 180;
		const tail = (side: number): [number, number] => [
			a.at.east - size * Math.sin(theta + side),
			a.at.north - size * Math.cos(theta + side)
		];
		const [l, r] = [tail(0.5), tail(-0.5)];
		out.push(l[0], l[1], z, a.at.east, a.at.north, z);
		out.push(a.at.east, a.at.north, z, r[0], r[1], z);
	}
	return new Float32Array(out);
}

/** An upright segment of `height` metres from each point: a pole, or a sensor's mast. */
export function verticalSegments(points: Vec3[], height: number): Float32Array {
	const out: number[] = [];
	for (const p of points) out.push(p[0], p[1], p[2], p[0], p[1], p[2] + height);
	return new Float32Array(out);
}

/**
 * A colour per point, from blue at the lowest height to warm at the highest,
 * so the scan reads as relief. Points with no spread are all one colour.
 */
export function heightColours(points: Float32Array): Float32Array {
	let lo = Infinity;
	let hi = -Infinity;
	for (let i = 2; i < points.length; i += 3) {
		lo = Math.min(lo, points[i]);
		hi = Math.max(hi, points[i]);
	}
	const span = hi - lo > 1e-6 ? hi - lo : 1;
	const out = new Float32Array(points.length);
	for (let i = 0; i < points.length; i += 3) {
		const t = Math.min(1, Math.max(0, (points[i + 2] - lo) / span));
		out[i] = 0.25 + 0.7 * t;
		out[i + 1] = 0.45 + 0.35 * (1 - Math.abs(2 * t - 1));
		out[i + 2] = 0.95 - 0.7 * t;
	}
	return out;
}

// ---- A site's layers -----------------------------------------------------------------------

/** One fill of one colour: meshes merged. */
export interface MeshSpec {
	colour: string;
	opacity: number;
	/** Upright or draped fills are seen from both sides. */
	doubleSided: boolean;
	mesh: MeshData;
}

export interface LineSpec {
	colour: string;
	positions: Float32Array;
}

export interface PointSpec {
	colour: string;
	positions: Float32Array;
}

/** What the 3D view draws for one layer, so the layer can be shown and hidden as one. */
export interface LayerScene {
	layer: string;
	meshes: MeshSpec[];
	lines: LineSpec[];
	points: PointSpec[];
	/** Features the view could not draw: a box with no height, a polygon with no area. */
	undrawn: number;
}

/** Heights above the ground at which fills and lines are drawn, so they are not lost in it. */
const LIFT = { surface: 0.02, space: 0.04, line: 0.06 } as const;

class LayerBuilder {
	private meshes = new Map<string, { spec: Omit<MeshSpec, 'mesh'>; parts: MeshData[] }>();
	private lines = new Map<string, Float32Array[]>();
	private points = new Map<string, number[]>();
	undrawn = 0;

	constructor(readonly layer: string) {}

	mesh(colour: string, opacity: number, doubleSided: boolean, mesh: MeshData) {
		if (mesh.indices.length === 0) return false;
		const key = `${colour}|${opacity}|${doubleSided}`;
		const entry = this.meshes.get(key) ?? { spec: { colour, opacity, doubleSided }, parts: [] };
		entry.parts.push(mesh);
		this.meshes.set(key, entry);
		return true;
	}

	line(colour: string, positions: Float32Array) {
		if (positions.length === 0) return;
		const list = this.lines.get(colour) ?? [];
		list.push(positions);
		this.lines.set(colour, list);
	}

	point(colour: string, p: Vec3) {
		const list = this.points.get(colour) ?? [];
		list.push(p[0], p[1], p[2]);
		this.points.set(colour, list);
	}

	build(): LayerScene {
		const concat = (arrays: Float32Array[]) => {
			const out = new Float32Array(arrays.reduce((n, a) => n + a.length, 0));
			let o = 0;
			for (const a of arrays) {
				out.set(a, o);
				o += a.length;
			}
			return out;
		};
		return {
			layer: this.layer,
			meshes: [...this.meshes.values()].map(({ spec, parts }) => ({
				...spec,
				mesh: mergeMeshes(parts)
			})),
			lines: [...this.lines].map(([colour, arrays]) => ({ colour, positions: concat(arrays) })),
			points: [...this.points].map(([colour, list]) => ({
				colour,
				positions: new Float32Array(list)
			})),
			undrawn: this.undrawn
		};
	}
}

function heightOf(f: SiteFrameFeature): number {
	const h = Number(f.tags.height_m);
	return Number.isFinite(h) && h > 0 ? h : 0;
}

function addFeature(b: LayerBuilder, f: SiteFrameFeature) {
	const colour = featureColour(f);
	const parts = partsOf(f.geometry);
	const polygons = polygonsOf(f.geometry);
	const lines = parts.filter((p) => p.points.length > 1);
	const points = parts.filter((p) => p.points.length === 1).map((p) => p.points[0]);
	const height = heightOf(f);

	if (f.layer === 'boxes' && polygons.length > 0) {
		let drawn = false;
		for (const { outer, holes } of polygons) {
			drawn = b.mesh(colour, 0.55, false, extrudeBox(outer, height, holes)) || drawn;
		}
		// The footprint's outline at its base, whether or not it could be raised.
		b.line(colour, lineSegments(lines, LIFT.line));
		if (!drawn) b.undrawn++;
		return;
	}
	if (f.layer === 'wall_planes' && polygons.length > 0) {
		let drawn = false;
		for (const { outer } of polygons) drawn = b.mesh(colour, 0.6, true, planeMesh(outer)) || drawn;
		b.line(colour, lineSegments(lines));
		if (!drawn) b.undrawn++;
		return;
	}
	if (f.layer === 'street_use' && polygons.length > 0) {
		const surface = f.kind === 'surface';
		const lift = surface ? LIFT.surface : LIFT.space;
		let drawn = false;
		for (const { outer, holes } of polygons) {
			drawn = b.mesh(colour, surface ? 0.4 : 0.5, true, surfaceMesh(outer, holes, lift)) || drawn;
		}
		if (!surface) b.line(colour, lineSegments(lines, lift));
		if (!drawn) b.undrawn++;
		return;
	}
	b.line(colour, lineSegments(lines, LIFT.line));
	if (f.layer === 'street_use' && f.kind === 'travel_lane') {
		for (const l of lines) b.line(colour, laneArrowSegments(l.points, 8, 1.2, LIFT.line));
	}
	for (const p of points) {
		b.point(colour, p);
		if (height > 0) b.line(colour, verticalSegments([p], height));
	}
}

/**
 * The 3D view's drawing of each layer the features belong to, in the order
 * the layers first appear: boxes raised by their `height_m`, wall planes and
 * surfaces as translucent fills, parked-car spaces as fills with outlines,
 * lanes with arrows, every other line at its own height, and points as dots
 * (with an upright of `height_m` where one is set).
 */
export function buildLayerScenes(features: SiteFrameFeature[]): LayerScene[] {
	const builders = new Map<string, LayerBuilder>();
	for (const f of features) {
		let b = builders.get(f.layer);
		if (!b) {
			b = new LayerBuilder(f.layer);
			builders.set(f.layer, b);
		}
		addFeature(b, f);
	}
	return [...builders.values()].map((b) => b.build());
}

// ---- The camera --------------------------------------------------------------------------------

export interface Bounds3 {
	min: Vec3;
	max: Vec3;
}

/** The box around every x, y, z triplet of the arrays, or null when they hold none. */
export function boundsOfArrays(arrays: Float32Array[]): Bounds3 | null {
	const min: Vec3 = [Infinity, Infinity, Infinity];
	const max: Vec3 = [-Infinity, -Infinity, -Infinity];
	for (const a of arrays) {
		for (let i = 0; i + 2 < a.length; i += 3) {
			for (let k = 0; k < 3; k++) {
				const v = a[i + k];
				if (!Number.isFinite(v)) continue;
				if (v < min[k]) min[k] = v;
				if (v > max[k]) max[k] = v;
			}
		}
	}
	return min[0] <= max[0] ? { min, max } : null;
}

/**
 * Where the camera starts and returns to on Reset: south of the middle of
 * everything drawn, looking north and down at about 40°, far enough back to
 * see it all. With nothing drawn it looks at the site's origin from 60 m.
 */
export function homeView(bounds: Bounds3 | null): { target: Vec3; position: Vec3 } {
	if (!bounds) return { target: [0, 0, 0], position: [0, -48, 36] };
	const target: Vec3 = [
		(bounds.min[0] + bounds.max[0]) / 2,
		(bounds.min[1] + bounds.max[1]) / 2,
		(bounds.min[2] + bounds.max[2]) / 2
	];
	const reach = Math.max(
		10,
		Math.hypot(bounds.max[0] - bounds.min[0], bounds.max[1] - bounds.min[1]) / 2
	);
	const distance = reach * 1.6;
	return {
		target,
		position: [target[0], target[1] - distance * 0.8, target[2] + distance * 0.6]
	};
}

/**
 * The bearing on screen of north, degrees clockwise from straight up, given
 * where the view's target and a point north of it land on screen (pixels, y
 * down). The north arrow is turned by it.
 */
export function screenNorth(target: [number, number], north: [number, number]): number {
	const dx = north[0] - target[0];
	const dy = north[1] - target[1];
	if (Math.hypot(dx, dy) < 1e-9) return 0;
	return ((Math.atan2(dx, -dy) * 180) / Math.PI + 360) % 360;
}
