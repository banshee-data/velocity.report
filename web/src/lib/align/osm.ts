/**
 * OpenStreetMap lookups for locating a capture: an address search and the
 * outlines a scan is snapped to.
 *
 * Both are external requests. Callers make them only after the operator has
 * allowed OpenStreetMap requests for the session, as the site map editor does
 * for tiles (TENETS.md §1 and §4): what leaves is the typed address or the map
 * area, never sensor data.
 */
import type { Segment } from './icp';
import { toLocal } from './pose';

export const NOMINATIM_URL = 'https://nominatim.openstreetmap.org/search';
export const OVERPASS_URL = 'https://overpass-api.de/api/interpreter';

export interface Place {
	lat: number;
	lon: number;
	label: string;
}

/** Searches for an address or place name. */
export async function searchPlaces(
	query: string,
	fetchFn: typeof fetch = fetch,
	signal?: AbortSignal
): Promise<Place[]> {
	const q = query.trim();
	if (q === '') return [];
	const url = `${NOMINATIM_URL}?format=jsonv2&limit=6&q=${encodeURIComponent(q)}`;
	const res = await fetchFn(url, { signal, headers: { Accept: 'application/json' } });
	if (!res.ok) throw new Error(`Address search failed (${res.status})`);
	const rows = (await res.json()) as Array<{ lat: string; lon: string; display_name: string }>;
	return rows
		.map((r) => ({ lat: Number(r.lat), lon: Number(r.lon), label: r.display_name }))
		.filter((p) => Number.isFinite(p.lat) && Number.isFinite(p.lon));
}

/** What an outline is: kinds a LiDAR sees as a vertical surface. */
export type OutlineKind = 'building' | 'barrier';

export interface Outline {
	kind: OutlineKind;
	points: Array<{ lat: number; lon: number }>;
}

/** The Overpass query for the outlines within radius metres of a point. */
export function outlineQuery(lat: number, lon: number, radius: number): string {
	const around = `(around:${Math.round(radius)},${lat.toFixed(6)},${lon.toFixed(6)})`;
	return [
		'[out:json][timeout:25];',
		'(',
		`  way["building"]${around};`,
		`  way["building:part"]${around};`,
		`  way["barrier"~"^(wall|fence|retaining_wall|kerb|guard_rail)$"]${around};`,
		');',
		'out geom;'
	].join('\n');
}

interface OverpassWay {
	type: string;
	tags?: Record<string, string | undefined>;
	geometry?: Array<{ lat: number; lon: number }>;
}

/** The outlines in an Overpass response. */
export function parseOutlines(body: { elements?: OverpassWay[] }): Outline[] {
	const out: Outline[] = [];
	for (const el of body.elements ?? []) {
		if (el.type !== 'way' || !el.geometry || el.geometry.length < 2) continue;
		const kind: OutlineKind = el.tags?.barrier ? 'barrier' : 'building';
		out.push({ kind, points: el.geometry.map((g) => ({ lat: g.lat, lon: g.lon })) });
	}
	return out;
}

/** Fetches the outlines around a point. */
export async function fetchOutlines(
	lat: number,
	lon: number,
	radius: number,
	fetchFn: typeof fetch = fetch,
	signal?: AbortSignal
): Promise<Outline[]> {
	const res = await fetchFn(OVERPASS_URL, {
		method: 'POST',
		signal,
		headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
		body: `data=${encodeURIComponent(outlineQuery(lat, lon, radius))}`
	});
	if (!res.ok) throw new Error(`Map outline request failed (${res.status})`);
	return parseOutlines(await res.json());
}

/** The outlines as segments in east/north metres about origin. */
export function outlineSegments(
	outlines: Outline[],
	origin: { lat: number; lon: number }
): Segment[] {
	const segments: Segment[] = [];
	for (const o of outlines) {
		const pts = o.points.map((p) => toLocal(origin, p.lat, p.lon));
		for (let i = 1; i < pts.length; i++) segments.push({ a: pts[i - 1], b: pts[i] });
	}
	return segments;
}
