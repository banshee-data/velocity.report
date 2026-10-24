/**
 * Geometry for the street editor, in a local east/north frame in metres about
 * an origin near the site (lib/align/pose's equirectangular frame, good to
 * millimetres over a site cell).
 *
 * The editor snaps what the operator draws to kerbs, centrelines, box walls
 * and other marks within half a metre, as a preview: Gerardus snaps and drapes
 * every edit again in the site frame when it is saved.
 */
import { fromLocal, toLocal, wrapDegrees, type EN } from '../align/pose';
import type { Geometry, Position } from './gerardus';

/** How close a vertex must be to a target to snap to it, in metres. */
export const SNAP_METRES = 0.5;

/** A line or ring the editor can snap to. */
export interface SnapTarget {
	/** The feature it came from, so a mark being edited can leave itself out. */
	id: string;
	layer: string;
	points: EN[];
	closed: boolean;
}

export interface Snap {
	point: EN;
	/** What it snapped to: a vertex, a point along an edge, or nothing. */
	on: 'vertex' | 'edge' | 'none';
	distance: number;
	target?: SnapTarget;
}

// ---- Frames -------------------------------------------------------------------

export function enOf(origin: { lat: number; lon: number }, p: Position): EN {
	return toLocal(origin, p[1], p[0]);
}

/** A local point as a GeoJSON position, longitude first. */
export function positionOf(origin: { lat: number; lon: number }, p: EN): [number, number] {
	const g = fromLocal(origin, p);
	return [g.lon, g.lat];
}

/** Drops a closing vertex that repeats the first. */
export function openRing<T extends ArrayLike<number>>(ring: T[]): T[] {
	if (ring.length < 2) return ring;
	const a = ring[0];
	const b = ring[ring.length - 1];
	return a[0] === b[0] && a[1] === b[1] ? ring.slice(0, -1) : ring;
}

/** The lines and rings of a geometry, each as positions and whether it closes. */
export function partsOf(g: Geometry): { points: Position[]; closed: boolean }[] {
	switch (g.type) {
		case 'Point':
			return [{ points: [g.coordinates], closed: false }];
		case 'MultiPoint':
			return g.coordinates.map((p) => ({ points: [p], closed: false }));
		case 'LineString':
			return [{ points: g.coordinates, closed: false }];
		case 'MultiLineString':
			return g.coordinates.map((points) => ({ points, closed: false }));
		case 'Polygon':
			return g.coordinates.map((ring) => ({ points: openRing(ring), closed: true }));
		case 'MultiPolygon':
			return g.coordinates.flatMap((poly) =>
				poly.map((ring) => ({ points: openRing(ring), closed: true }))
			);
	}
}

/** Snap targets for a feature's geometry. */
export function targetsOf(
	origin: { lat: number; lon: number },
	feature: { id: string; layer: string; geometry: Geometry }
): SnapTarget[] {
	return partsOf(feature.geometry).map((part) => ({
		id: feature.id,
		layer: feature.layer,
		points: part.points.map((p) => enOf(origin, p)),
		closed: part.closed
	}));
}

// ---- Distances and snapping ---------------------------------------------------

export function distance(a: EN, b: EN): number {
	return Math.hypot(a.east - b.east, a.north - b.north);
}

/** The nearest point to p on the segment a–b, its fraction t along it, and the distance. */
export function closestOnSegment(p: EN, a: EN, b: EN): { point: EN; t: number; distance: number } {
	const de = b.east - a.east;
	const dn = b.north - a.north;
	const len2 = de * de + dn * dn;
	const t =
		len2 === 0
			? 0
			: Math.max(0, Math.min(1, ((p.east - a.east) * de + (p.north - a.north) * dn) / len2));
	const point = { east: a.east + t * de, north: a.north + t * dn };
	return { point, t, distance: distance(p, point) };
}

function* edges(t: SnapTarget): Generator<[EN, EN]> {
	const n = t.points.length;
	for (let i = 0; i + 1 < n; i++) yield [t.points[i], t.points[i + 1]];
	if (t.closed && n > 2) yield [t.points[n - 1], t.points[0]];
}

/**
 * Snaps p to the nearest target within tolerance. A vertex within reach wins
 * over a nearer edge, so marks meet end to end rather than nearly; otherwise
 * the nearest point along any edge. Targets whose id is `excludeId` are left
 * out: a mark being moved does not snap to itself.
 */
export function snapPoint(
	p: EN,
	targets: SnapTarget[],
	tolerance = SNAP_METRES,
	excludeId?: string
): Snap {
	let best: Snap = { point: p, on: 'none', distance: Infinity };
	for (const t of targets) {
		if (excludeId !== undefined && t.id === excludeId) continue;
		for (const v of t.points) {
			const d = distance(p, v);
			if (d <= tolerance && d < best.distance)
				best = { point: v, on: 'vertex', distance: d, target: t };
		}
	}
	if (best.on === 'vertex') return best;
	for (const t of targets) {
		if (excludeId !== undefined && t.id === excludeId) continue;
		for (const [a, b] of edges(t)) {
			const c = closestOnSegment(p, a, b);
			if (c.distance <= tolerance && c.distance < best.distance) {
				best = { point: c.point, on: 'edge', distance: c.distance, target: t };
			}
		}
	}
	return best.on === 'none' ? { point: p, on: 'none', distance: 0 } : best;
}

/** The nearest point on any of the targets, however far, or null when there are none. */
export function nearestOn(p: EN, targets: SnapTarget[]): { point: EN; distance: number } | null {
	let best: { point: EN; distance: number } | null = null;
	for (const t of targets) {
		if (t.points.length === 1) {
			const d = distance(p, t.points[0]);
			if (!best || d < best.distance) best = { point: t.points[0], distance: d };
			continue;
		}
		for (const [a, b] of edges(t)) {
			const c = closestOnSegment(p, a, b);
			if (!best || c.distance < best.distance) best = { point: c.point, distance: c.distance };
		}
	}
	return best;
}

// ---- A row of spaces along the kerb ---------------------------------------------

/** +1 when p is left of the line a→b (seen from above), −1 when right. On the line counts as left. */
export function sideOf(a: EN, b: EN, p: EN): 1 | -1 {
	const cross = (b.east - a.east) * (p.north - a.north) - (b.north - a.north) * (p.east - a.east);
	return cross < 0 ? -1 : 1;
}

/**
 * The side of the kerb span a→b the street is on: towards the nearest
 * centreline from the span's midpoint. Left when there is no centreline.
 */
export function streetSide(a: EN, b: EN, centrelines: SnapTarget[]): 1 | -1 {
	const mid = { east: (a.east + b.east) / 2, north: (a.north + b.north) / 2 };
	const near = nearestOn(mid, centrelines);
	return near ? sideOf(a, b, near.point) : 1;
}

export interface SpaceRow {
	/** The row's outline: a, b, then b and a moved `depth` to the side. */
	ring: EN[];
	/** The lines between neighbouring spaces, from the kerb outwards. */
	dividers: [EN, EN][];
	/** Each space's length along the kerb, metres. */
	spaceLength: number;
}

/**
 * A run of `count` equal spaces along the kerb from a to b, `depth` metres
 * deep on `side` (+1 left of a→b, −1 right). Null when the span has no length
 * or the count is not a positive whole number.
 */
export function rowOfSpaces(
	a: EN,
	b: EN,
	count: number,
	depth: number,
	side: 1 | -1
): SpaceRow | null {
	const de = b.east - a.east;
	const dn = b.north - a.north;
	const length = Math.hypot(de, dn);
	if (length < 1e-6 || !Number.isInteger(count) || count < 1 || !(depth > 0)) return null;
	// The left normal of (e, n) is (−n, e).
	const oe = (side * depth * -dn) / length;
	const on = (side * depth * de) / length;
	const out = (p: EN): EN => ({ east: p.east + oe, north: p.north + on });
	const dividers: [EN, EN][] = [];
	for (let i = 1; i < count; i++) {
		const p = { east: a.east + (de * i) / count, north: a.north + (dn * i) / count };
		dividers.push([p, out(p)]);
	}
	return { ring: [a, b, out(b), out(a)], dividers, spaceLength: length / count };
}

/**
 * The dividers of a row drawn as one four-cornered outline: the first edge is
 * the kerb side and the third the far side, as rowOfSpaces makes it. Any other
 * outline, or a count below two, has none.
 */
export function rowDividers(ring: EN[], count: number): [EN, EN][] {
	if (ring.length !== 4 || !Number.isInteger(count) || count < 2) return [];
	const lerp = (p: EN, q: EN, f: number): EN => ({
		east: p.east + (q.east - p.east) * f,
		north: p.north + (q.north - p.north) * f
	});
	const out: [EN, EN][] = [];
	for (let i = 1; i < count; i++) {
		const f = i / count;
		out.push([lerp(ring[0], ring[1], f), lerp(ring[3], ring[2], f)]);
	}
	return out;
}

// ---- A lane's direction -----------------------------------------------------------

/** The bearing from a to b, degrees clockwise from north. */
export function bearing(a: EN, b: EN): number {
	return wrapDegrees((Math.atan2(b.east - a.east, b.north - a.north) * 180) / Math.PI);
}

/**
 * The direction of travel of a lane drawn first vertex to last: the bearing of
 * the whole run. Null for a lane with fewer than two distinct ends.
 */
export function laneDirection(points: EN[]): number | null {
	if (points.length < 2) return null;
	const a = points[0];
	const b = points[points.length - 1];
	return distance(a, b) < 1e-6 ? null : bearing(a, b);
}

/**
 * Arrows along a lane every `spacing` metres, starting half a spacing in,
 * each with the bearing of the segment it sits on: what the map draws to show
 * the direction of travel. A lane shorter than one spacing gets one arrow at
 * its middle.
 */
export function laneArrows(points: EN[], spacing = 10): { at: EN; bearing: number }[] {
	const segs: { a: EN; b: EN; len: number; start: number }[] = [];
	let total = 0;
	for (let i = 0; i + 1 < points.length; i++) {
		const len = distance(points[i], points[i + 1]);
		if (len < 1e-9) continue;
		segs.push({ a: points[i], b: points[i + 1], len, start: total });
		total += len;
	}
	if (segs.length === 0 || !(spacing > 0)) return [];
	const marks: number[] = [];
	if (total < spacing) marks.push(total / 2);
	else for (let s = spacing / 2; s < total; s += spacing) marks.push(s);
	return marks.map((s) => {
		const seg = segs.find((g) => s <= g.start + g.len) ?? segs[segs.length - 1];
		const f = (s - seg.start) / seg.len;
		return {
			at: {
				east: seg.a.east + (seg.b.east - seg.a.east) * f,
				north: seg.a.north + (seg.b.north - seg.a.north) * f
			},
			bearing: bearing(seg.a, seg.b)
		};
	});
}

/** The longitude and latitude box around every vertex of the features, or null when there are none. */
export function boundsOf(
	features: { geometry: Geometry }[]
): { west: number; south: number; east: number; north: number } | null {
	let west = Infinity;
	let south = Infinity;
	let east = -Infinity;
	let north = -Infinity;
	for (const f of features) {
		for (const part of partsOf(f.geometry)) {
			for (const p of part.points) {
				west = Math.min(west, p[0]);
				east = Math.max(east, p[0]);
				south = Math.min(south, p[1]);
				north = Math.max(north, p[1]);
			}
		}
	}
	return west <= east ? { west, south, east, north } : null;
}
