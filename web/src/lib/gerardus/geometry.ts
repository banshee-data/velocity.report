/**
 * Geometry for the site viewer: splitting GeoJSON geometries into lines and
 * rings, their bounds, and the arrows that show a lane's direction of travel.
 *
 * The 2D map works in a local east/north frame in metres about an origin near
 * the site (lib/align/pose's equirectangular frame, good to millimetres over a
 * site cell) when it needs distances; the 3D view's site frame is already in
 * metres, east and north, so the same helpers serve both.
 */
import { fromLocal, toLocal, wrapDegrees, type EN } from '../align/pose';
import type { Geometry, Position } from './client';

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
export function partsOf<P extends Position>(g: Geometry<P>): { points: P[]; closed: boolean }[] {
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

/**
 * The polygons of a geometry, each as its outer ring and its holes, closing
 * vertices dropped. Lines and points have none.
 */
export function polygonsOf<P extends Position>(g: Geometry<P>): { outer: P[]; holes: P[][] }[] {
	const one = (rings: P[][]) => {
		const [outer, ...holes] = rings.map((r) => openRing(r));
		return { outer, holes };
	};
	if (g.type === 'Polygon') return g.coordinates.length > 0 ? [one(g.coordinates)] : [];
	if (g.type === 'MultiPolygon') return g.coordinates.filter((p) => p.length > 0).map(one);
	return [];
}

// ---- Distances and bearings -----------------------------------------------------

export function distance(a: EN, b: EN): number {
	return Math.hypot(a.east - b.east, a.north - b.north);
}

/** The bearing from a to b, degrees clockwise from north. */
export function bearing(a: EN, b: EN): number {
	return wrapDegrees((Math.atan2(b.east - a.east, b.north - a.north) * 180) / Math.PI);
}

export interface LaneArrow {
	at: EN;
	/** Degrees clockwise from north, along the segment the arrow sits on. */
	bearing: number;
	/** The index of the vertex starting that segment, and how far along it the arrow is (0–1). */
	segment: number;
	t: number;
}

/**
 * Arrows along a lane every `spacing` metres, starting half a spacing in,
 * each with the bearing of the segment it sits on: what the map and the 3D
 * view draw to show the direction of travel, which runs first vertex to last.
 * A lane shorter than one spacing gets one arrow at its middle.
 */
export function laneArrows(points: EN[], spacing = 10): LaneArrow[] {
	const segs: { a: EN; b: EN; i: number; len: number; start: number }[] = [];
	let total = 0;
	for (let i = 0; i + 1 < points.length; i++) {
		const len = distance(points[i], points[i + 1]);
		if (len < 1e-9) continue;
		segs.push({ a: points[i], b: points[i + 1], i, len, start: total });
		total += len;
	}
	if (segs.length === 0 || !(spacing > 0)) return [];
	const marks: number[] = [];
	if (total < spacing) marks.push(total / 2);
	else for (let s = spacing / 2; s < total; s += spacing) marks.push(s);
	return marks.map((s) => {
		const seg = segs.find((g) => s <= g.start + g.len) ?? segs[segs.length - 1];
		const t = (s - seg.start) / seg.len;
		return {
			at: {
				east: seg.a.east + (seg.b.east - seg.a.east) * t,
				north: seg.a.north + (seg.b.north - seg.a.north) * t
			},
			bearing: bearing(seg.a, seg.b),
			segment: seg.i,
			t
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
