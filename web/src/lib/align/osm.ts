/**
 * OpenStreetMap lookups for locating a capture: an address search for the browser map.
 *
 * This is an external request. Callers make it only after the operator has
 * allowed OpenStreetMap requests for the session, as the site map editor does
 * for tiles (TENETS.md §1 and §4): what leaves is the typed address or the map
 * area, never sensor data.
 */

export const NOMINATIM_URL = 'https://nominatim.openstreetmap.org/search';

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
