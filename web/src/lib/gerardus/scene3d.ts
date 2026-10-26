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
import { crownRadius, featureColour, tagNumber, TREE_COLOURS } from './style';

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
 * height (x0, y0 being the south-west cell's centre), two triangles for each square of four such cells, and one for a
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
			positions.push(g.x0 + i * cellM, g.y0 + j * cellM, h);
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

// ---- Solids for the W4–W7 layers ----------------------------------------------------------

/**
 * A building part's rise above its outline, metres. Gerardus places the
 * outline at the part's base, `min_height` above the ground, and `height`
 * (Simple 3D Buildings') is measured from the ground, so the part stands
 * `height − min_height` above its outline; without a `height`, its
 * `height_m`. Zero when neither says, or the part has no height above its base.
 */
export function partRise(tags: Record<string, string>): number {
	const height = tagNumber(tags, 'height');
	if (Number.isFinite(height)) {
		const min = tagNumber(tags, 'min_height');
		return Math.max(0, height - (Number.isFinite(min) ? min : 0));
	}
	const h = tagNumber(tags, 'height_m');
	return Number.isFinite(h) && h > 0 ? h : 0;
}

function add(a: Vec3, b: Vec3): Vec3 {
	return [a[0] + b[0], a[1] + b[1], a[2] + b[2]];
}

function scale(a: Vec3, k: number): Vec3 {
	return [a[0] * k, a[1] * k, a[2] * k];
}

function unit(a: Vec3): Vec3 {
	const n = Math.hypot(...a);
	return n > 0 ? scale(a, 1 / n) : [0, 0, 0];
}

/**
 * A cylinder of `radius` from `base` to `top`, with a cap on top and none
 * underneath (it stands on something): `segments` side quads wound to face out.
 */
export function cylinderMesh(base: Vec3, top: Vec3, radius: number, segments = 12): MeshData {
	const axis = sub(top, base);
	if (!(radius > 0) || Math.hypot(...axis) < 1e-9 || segments < 3) return EMPTY_MESH;
	const a = unit(axis);
	// u × v = a, so the angle runs anticlockwise about the axis.
	const u = unit(cross(a, Math.abs(a[2]) < 0.9 ? [0, 0, 1] : [1, 0, 0]));
	const v = cross(a, u);
	const positions: number[] = [];
	for (const centre of [base, top]) {
		for (let i = 0; i < segments; i++) {
			const t = (2 * Math.PI * i) / segments;
			const p = add(centre, add(scale(u, radius * Math.cos(t)), scale(v, radius * Math.sin(t))));
			positions.push(...p);
		}
	}
	positions.push(...top);
	const indices: number[] = [];
	const cap = 2 * segments;
	for (let i = 0; i < segments; i++) {
		const j = (i + 1) % segments;
		indices.push(i, j, segments + j, i, segments + j, segments + i);
		indices.push(cap, segments + i, segments + j);
	}
	return { positions: new Float32Array(positions), indices: new Uint32Array(indices) };
}

/** An axis-aligned box about `centre`, `size` metres along x, y and z, each face wound to face out. */
export function boxMesh(centre: Vec3, size: Vec3): MeshData {
	if (!size.every((s) => s > 0)) return EMPTY_MESH;
	const h = scale(size, 0.5);
	const positions: number[] = [];
	const indices: number[] = [];
	// Each face: its outward axis and sign; corners anticlockwise seen from outside.
	for (let k = 0; k < 3; k++) {
		for (const sign of [1, -1]) {
			const [p, q] = [(k + 1) % 3, (k + 2) % 3];
			const corners = [
				[-1, -1],
				[1, -1],
				[1, 1],
				[-1, 1]
			].map(([a, b]) => {
				const c: Vec3 = [0, 0, 0];
				c[k] = sign * h[k];
				// Swapping the in-face axes on the far side keeps the winding outward.
				c[p] = (sign > 0 ? a : b) * h[p];
				c[q] = (sign > 0 ? b : a) * h[q];
				return add(centre, c);
			});
			const v0 = positions.length / 3;
			for (const c of corners) positions.push(...c);
			indices.push(v0, v0 + 1, v0 + 2, v0, v0 + 2, v0 + 3);
		}
	}
	return { positions: new Float32Array(positions), indices: new Uint32Array(indices) };
}

/** An ellipsoid about `centre` with semi-axes `radii`, in `rings` bands of `segments` quads, facing out. */
export function ellipsoidMesh(centre: Vec3, radii: Vec3, segments = 16, rings = 8): MeshData {
	if (!radii.every((r) => r > 0) || segments < 3 || rings < 2) return EMPTY_MESH;
	const positions: number[] = [];
	for (let r = 0; r <= rings; r++) {
		const phi = -Math.PI / 2 + (Math.PI * r) / rings;
		for (let i = 0; i < segments; i++) {
			const theta = (2 * Math.PI * i) / segments;
			positions.push(
				centre[0] + radii[0] * Math.cos(phi) * Math.cos(theta),
				centre[1] + radii[1] * Math.cos(phi) * Math.sin(theta),
				centre[2] + radii[2] * Math.sin(phi)
			);
		}
	}
	const indices: number[] = [];
	for (let r = 0; r < rings; r++) {
		for (let i = 0; i < segments; i++) {
			const j = (i + 1) % segments;
			const [a, b, c, d] = [
				r * segments + i,
				r * segments + j,
				(r + 1) * segments + j,
				(r + 1) * segments + i
			];
			// The bands at the poles have one collapsed edge: one triangle each.
			if (r > 0) indices.push(a, b, c);
			if (r < rings - 1) indices.push(a, c, d);
		}
	}
	return { positions: new Float32Array(positions), indices: new Uint32Array(indices) };
}

/** A pole's diameter, and a tree's trunk, when they carry none. */
export const DEFAULT_POLE_DIAMETER_M = 0.2;
export const DEFAULT_DBH_M = 0.3;
/** A sign's face when nothing says how big it is. */
export const SIGN_WIDTH_M = 0.6;
export const SIGN_DEPTH_M = 0.05;

/** A pole as a cylinder from base to top, `diameter_m` across (0.2 m when it does not say). */
export function poleMesh(base: Vec3, top: Vec3, tags: Record<string, string>): MeshData {
	const d = tagNumber(tags, 'diameter_m');
	return cylinderMesh(base, top, (Number.isFinite(d) && d > 0 ? d : DEFAULT_POLE_DIAMETER_M) / 2);
}

/** A sign as a small box over its face, bottom to top: 0.6 m wide, 5 cm deep. */
export function signMesh(bottom: Vec3, top: Vec3): MeshData {
	const height = Math.abs(top[2] - bottom[2]);
	const centre = scale(add(bottom, top), 0.5);
	return boxMesh(centre, [SIGN_WIDTH_M, SIGN_DEPTH_M, height]);
}

/** Where a tree's parts are, metres in the site frame. */
export interface TreeShape {
	base: Vec3;
	trunkRadius: number;
	/** The crown's bottom and top heights, and its radius. */
	crownBottom: number;
	crownTop: number;
	crownRadius: number;
}

/**
 * A tree from its tags, heights measured from its trunk's base: the trunk to
 * `crown_base_m` (else the top of the feature's segment), `dbh_m` across (else
 * 0.3 m); the crown from there to `height_m` (else as tall as it is wide), of
 * radius `crown_radius_m` (else 2 m).
 */
export function treeShape(base: Vec3, tags: Record<string, string>, trunkTop?: Vec3): TreeShape {
	const r = crownRadius(tags);
	const crownBase = tagNumber(tags, 'crown_base_m');
	const bottom = Number.isFinite(crownBase) ? base[2] + crownBase : (trunkTop?.[2] ?? base[2]);
	const height = tagNumber(tags, 'height_m');
	const top = Number.isFinite(height) ? base[2] + height : bottom + 2 * r;
	const dbh = tagNumber(tags, 'dbh_m');
	return {
		base,
		trunkRadius: (Number.isFinite(dbh) && dbh > 0 ? dbh : DEFAULT_DBH_M) / 2,
		crownBottom: bottom,
		crownTop: Math.max(top, bottom),
		crownRadius: r
	};
}

/** A tree's trunk, a cylinder to the crown's base, and its crown, an ellipsoid filling its height. */
export function treeMeshes(shape: TreeShape): { trunk: MeshData; crown: MeshData } {
	const { base, crownBottom, crownTop, crownRadius: r } = shape;
	const trunk = cylinderMesh(base, [base[0], base[1], crownBottom], shape.trunkRadius);
	const half = (crownTop - crownBottom) / 2;
	const crown = ellipsoidMesh([base[0], base[1], crownBottom + half], [r, r, half]);
	return { trunk, crown };
}

/**
 * A feature drawn standing up (a pole, a sign's face, a tree's trunk): its
 * first vertex and its last, or its point with no top. Null without one.
 */
export function uprightOf(f: SiteFrameFeature): { base: Vec3; top: Vec3 | null } | null {
	const part = partsOf(f.geometry).find((p) => p.points.length > 0);
	if (!part) return null;
	const pts = part.points;
	if (pts.length === 1) return { base: pts[0], top: null };
	const [a, b] = [pts[0], pts[pts.length - 1]];
	// Base first, whichever way the segment was written.
	return a[2] <= b[2] ? { base: a, top: b } : { base: b, top: a };
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
const LIFT = { surface: 0.02, space: 0.04, marking: 0.05, line: 0.06 } as const;

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

/**
 * Which walls the features carry as vertical polygons. Where they do, nothing
 * else is raised to stand in for them: a part's height is the top of its
 * roof, and a box's is 3DEP's roof p95, so either, extruded, stands its walls
 * over the gutter (Gerardus docs/plans/ROOFLINE-WALLS.md).
 */
interface Walls {
	/** `wall_surfaces`: the building parts are outlines. */
	parts: boolean;
	/** `box_walls`: the boxes are outlines. */
	boxes: boolean;
}

function addFeature(b: LayerBuilder, f: SiteFrameFeature, walls: Walls) {
	const colour = featureColour(f);
	const parts = partsOf(f.geometry);
	const polygons = polygonsOf(f.geometry);
	const lines = parts.filter((p) => p.points.length > 1);
	const points = parts.filter((p) => p.points.length === 1).map((p) => p.points[0]);
	const height = heightOf(f);

	if (f.layer === 'boxes' && polygons.length > 0) {
		// The footprint's outline at its base, whether or not it could be raised.
		b.line(colour, lineSegments(lines, LIFT.line));
		if (walls.boxes) return;
		let drawn = false;
		for (const { outer, holes } of polygons) {
			drawn = b.mesh(colour, 0.55, false, extrudeBox(outer, height, holes)) || drawn;
		}
		if (!drawn) b.undrawn++;
		return;
	}
	// Box walls and building walls stand as they come, each triangulated in
	// its own plane, whatever their tops do: a gable end rises to its ridge,
	// and a wall that meets a step is concave. A box wall's height_m is the
	// box's, and is not added.
	if ((f.layer === 'box_walls' || f.layer === 'wall_surfaces') && polygons.length > 0) {
		const opacity = f.layer === 'box_walls' && f.tags.party === 'true' ? 0.2 : 0.55;
		let drawn = false;
		for (const { outer } of polygons)
			drawn = b.mesh(colour, opacity, true, planeMesh(outer)) || drawn;
		b.line(colour, lineSegments(lines));
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
	if (f.layer === 'building_parts' && polygons.length > 0 && walls.parts) {
		// The outline at the part's base; its wall surfaces are its walls.
		b.line(colour, lineSegments(lines));
		return;
	}
	if (f.layer === 'building_parts' && polygons.length > 0) {
		// An older Gerardus, which sends no wall surfaces.
		const rise = partRise(f.tags);
		let drawn = false;
		for (const { outer, holes } of polygons) {
			drawn = b.mesh(colour, 0.55, false, extrudeBox(outer, rise, holes)) || drawn;
		}
		// The outline at the part's base, and again at its top.
		b.line(colour, lineSegments(lines));
		if (rise > 0) b.line(colour, lineSegments(lines, rise));
		if (!drawn) b.undrawn++;
		return;
	}
	if (f.layer === 'roof_planes' && polygons.length > 0) {
		let drawn = false;
		for (const { outer } of polygons) drawn = b.mesh(colour, 0.8, true, planeMesh(outer)) || drawn;
		b.line(colour, lineSegments(lines));
		if (!drawn) b.undrawn++;
		return;
	}
	if (f.layer === 'lane_markings') {
		// On the road, a little above the surfaces painted on it.
		for (const { outer, holes } of polygons) {
			b.mesh(colour, 0.85, true, surfaceMesh(outer, holes, LIFT.marking));
		}
		b.line(colour, lineSegments(lines, LIFT.marking));
		return;
	}
	const upright = ['pole', 'sign', 'tree'].includes(f.kind) ? uprightOf(f) : null;
	if (upright && (f.layer === 'street_furniture' || f.layer === 'trees')) {
		const { base, top } = upright;
		let drawn = false;
		if (f.kind === 'tree') {
			const { trunk, crown } = treeMeshes(treeShape(base, f.tags, top ?? undefined));
			drawn = b.mesh(TREE_COLOURS.trunk, 1, false, trunk);
			drawn = b.mesh(TREE_COLOURS.crown, 0.7, false, crown) || drawn;
		} else if (top) {
			drawn = b.mesh(
				colour,
				1,
				false,
				f.kind === 'pole' ? poleMesh(base, top, f.tags) : signMesh(base, top)
			);
		}
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
 * the layers first appear: box walls and building walls as translucent fills
 * as they come, boxes and building parts as outlines beside them (or, from
 * an older Gerardus with no walls, boxes raised by their `height_m` and
 * parts from `min_height` to `height`), wall planes and surfaces as
 * translucent fills, parked-car spaces as fills with outlines, lanes with
 * arrows, roof planes as fills at their z, lane markings on the road, poles as
 * cylinders, signs as small boxes, trees as a trunk and a crown, every other
 * line at its own height, and points as dots (with an upright of `height_m`
 * where one is set).
 */
export function buildLayerScenes(features: SiteFrameFeature[]): LayerScene[] {
	const builders = new Map<string, LayerBuilder>();
	const walls: Walls = {
		parts: features.some((f) => f.layer === 'wall_surfaces'),
		boxes: features.some((f) => f.layer === 'box_walls')
	};
	for (const f of features) {
		let b = builders.get(f.layer);
		if (!b) {
			b = new LayerBuilder(f.layer);
			builders.set(f.layer, b);
		}
		addFeature(b, f, walls);
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
