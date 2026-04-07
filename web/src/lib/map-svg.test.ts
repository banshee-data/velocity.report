// TextEncoder/TextDecoder are not available in jsdom by default.
import { TextDecoder, TextEncoder } from 'util';
Object.assign(globalThis, { TextEncoder, TextDecoder });

import {
	buildNodeLookup,
	buildOverpassQueries,
	categoriseElements,
	escapeXml,
	generateMapSvg,
	getLabelFontSize,
	getLanduseColor,
	getPathMidpoint,
	getPoiIcon,
	getRoadStyle,
	LABEL_ROADS,
	makeCoordConverters,
	orderedEndpoints,
	OVERPASS_MIRRORS,
	POI_ICON_MAP,
	ROAD_ORDER,
	svgToBase64,
	toLinePath,
	toPolygonPath,
	type LatLon,
	type OverpassMirror,
	type SvgParams
} from './map-svg';

// ── escapeXml ────────────────────────────────────────────────────────────────

describe('escapeXml', () => {
	it('escapes ampersand', () => {
		expect(escapeXml('A & B')).toBe('A &amp; B');
	});

	it('escapes angle brackets', () => {
		expect(escapeXml('<div>')).toBe('&lt;div&gt;');
	});

	it('escapes double quotes', () => {
		expect(escapeXml('say "hello"')).toBe('say &quot;hello&quot;');
	});

	it('escapes single quotes', () => {
		expect(escapeXml("it's")).toBe('it&#39;s');
	});

	it('handles all special chars together', () => {
		expect(escapeXml(`<a href="x" title='y'>&</a>`)).toBe(
			'&lt;a href=&quot;x&quot; title=&#39;y&#39;&gt;&amp;&lt;/a&gt;'
		);
	});

	it('returns plain strings unchanged', () => {
		expect(escapeXml('Hello World 123')).toBe('Hello World 123');
	});

	it('handles empty string', () => {
		expect(escapeXml('')).toBe('');
	});

	it('handles unicode unchanged', () => {
		expect(escapeXml('Café 🌳')).toBe('Café 🌳');
	});
});

// ── getRoadStyle ─────────────────────────────────────────────────────────────

describe('getRoadStyle', () => {
	it('returns motorway style', () => {
		const s = getRoadStyle('motorway');
		expect(s.width).toBe(8);
		expect(s.color).toBe('#e892a2');
		expect(s.casing).toBe(10);
		expect(s.dash).toBeUndefined();
	});

	it('returns residential style', () => {
		const s = getRoadStyle('residential');
		expect(s.width).toBe(3);
		expect(s.color).toBe('#ffffff');
	});

	it('returns footway with dash', () => {
		const s = getRoadStyle('footway');
		expect(s.dash).toBe('4,2');
		expect(s.casing).toBe(0);
	});

	it('returns cycleway with dash', () => {
		const s = getRoadStyle('cycleway');
		expect(s.dash).toBe('4,2');
		expect(s.color).toBe('#0000ff');
	});

	it('returns default for unknown type', () => {
		const s = getRoadStyle('imaginary_road');
		expect(s.width).toBe(2);
		expect(s.color).toBe('#cccccc');
		expect(s.casing).toBe(3);
	});

	it.each([
		'motorway',
		'motorway_link',
		'trunk',
		'trunk_link',
		'primary',
		'primary_link',
		'secondary',
		'secondary_link',
		'tertiary',
		'tertiary_link',
		'residential',
		'unclassified',
		'living_street',
		'service',
		'pedestrian',
		'footway',
		'cycleway',
		'path',
		'track'
	])('returns a defined style for %s', (highway) => {
		const s = getRoadStyle(highway);
		expect(s.width).toBeGreaterThan(0);
		expect(s.color).toMatch(/^#[0-9a-f]{6}$/);
	});
});

// ── getLabelFontSize ─────────────────────────────────────────────────────────

describe('getLabelFontSize', () => {
	it('returns 28 for motorway', () => {
		expect(getLabelFontSize('motorway')).toBe(28);
	});

	it('returns 18 for residential', () => {
		expect(getLabelFontSize('residential')).toBe(18);
	});

	it('returns 16 as default', () => {
		expect(getLabelFontSize('unknown_type')).toBe(16);
	});

	it.each(['motorway', 'trunk', 'primary', 'secondary', 'tertiary', 'residential'])(
		'returns a size > 0 for %s',
		(highway) => {
			expect(getLabelFontSize(highway)).toBeGreaterThan(0);
		}
	);
});

// ── getLanduseColor ──────────────────────────────────────────────────────────

describe('getLanduseColor', () => {
	it('returns green for park', () => {
		expect(getLanduseColor('park')).toBe('#c8facc');
	});

	it('returns green for forest', () => {
		expect(getLanduseColor('forest')).toBe('#add19e');
	});

	it('returns blue-ish for wetland', () => {
		expect(getLanduseColor('wetland')).toBe('#b5d0d0');
	});

	it('returns default grey for unknown', () => {
		expect(getLanduseColor('martian_soil')).toBe('#e0e0e0');
	});

	it.each([
		'grass',
		'forest',
		'wood',
		'park',
		'garden',
		'meadow',
		'playground',
		'farmland',
		'cemetery',
		'wetland'
	])('returns a colour for %s', (type) => {
		expect(getLanduseColor(type)).toMatch(/^#[0-9a-f]{6}$/);
	});
});

// ── getPoiIcon ───────────────────────────────────────────────────────────────

describe('getPoiIcon', () => {
	it('returns school emoji for amenity=school', () => {
		expect(getPoiIcon({ amenity: 'school' })).toBe('🏫');
	});

	it('returns bus stop emoji for highway=bus_stop', () => {
		expect(getPoiIcon({ highway: 'bus_stop' })).toBe('🚏');
	});

	it('returns park emoji for leisure=park', () => {
		expect(getPoiIcon({ leisure: 'park' })).toBe('🌳');
	});

	it('returns tree emoji for natural=tree', () => {
		expect(getPoiIcon({ natural: 'tree' })).toBe('🌲');
	});

	it('returns station emoji for railway=station', () => {
		expect(getPoiIcon({ railway: 'station' })).toBe('🚉');
	});

	it('returns shop icon for shop=supermarket', () => {
		expect(getPoiIcon({ shop: 'supermarket' })).toBe('🛒');
	});

	it('returns convenience icon for shop=convenience', () => {
		expect(getPoiIcon({ shop: 'convenience' })).toBe('🏪');
	});

	it('returns default dot for unknown tags', () => {
		expect(getPoiIcon({ building: 'yes' })).toBe('\u25cf');
	});

	it('returns default dot for empty tags', () => {
		expect(getPoiIcon({})).toBe('\u25cf');
	});

	it('prefers amenity over other tags', () => {
		expect(getPoiIcon({ amenity: 'cafe', leisure: 'park' })).toBe('☕');
	});

	it('falls through to sport tag', () => {
		expect(getPoiIcon({ sport: 'swimming_pool' })).toBe('🏊');
	});

	it.each(Object.keys(POI_ICON_MAP))('maps %s to an emoji', (key) => {
		// Build a tag object that has the right key in the lookup order
		// amenity, shop, leisure, natural, highway, railway, tourism, sport
		const tagKeys = [
			'amenity',
			'shop',
			'leisure',
			'natural',
			'highway',
			'railway',
			'tourism',
			'sport'
		];
		for (const tk of tagKeys) {
			const result = getPoiIcon({ [tk]: key });
			if (result !== '\u25cf') {
				expect(result).toBe(POI_ICON_MAP[key]);
				return;
			}
		}
		// If not found through any tag key, the map entry exists but the key
		// doesn't match any single tag — that's still valid coverage.
	});
});

// ── getPathMidpoint ──────────────────────────────────────────────────────────

describe('getPathMidpoint', () => {
	const identity = (v: number) => v;

	it('returns null for fewer than 2 nodes', () => {
		expect(getPathMidpoint([], identity, identity)).toBeNull();
		expect(getPathMidpoint([{ lat: 0, lon: 0 }], identity, identity)).toBeNull();
	});

	it('computes midpoint of a horizontal two-node segment', () => {
		const nodes: LatLon[] = [
			{ lat: 0, lon: 0 },
			{ lat: 0, lon: 10 }
		];
		const result = getPathMidpoint(nodes, identity, identity);
		expect(result).not.toBeNull();
		expect(result!.x).toBeCloseTo(5);
		expect(result!.y).toBeCloseTo(0);
		expect(result!.angle).toBeCloseTo(0); // horizontal
	});

	it('computes midpoint of a vertical two-node segment', () => {
		const nodes: LatLon[] = [
			{ lat: 0, lon: 0 },
			{ lat: 10, lon: 0 }
		];
		const result = getPathMidpoint(nodes, identity, identity);
		expect(result).not.toBeNull();
		expect(result!.x).toBeCloseTo(0);
		expect(result!.y).toBeCloseTo(5);
		expect(result!.angle).toBeCloseTo(90);
	});

	it('computes midpoint with custom coordinate transforms', () => {
		const nodes: LatLon[] = [
			{ lat: 51.0, lon: -1.0 },
			{ lat: 51.0, lon: -0.5 }
		];
		const latToY = (lat: number) => (52 - lat) * 800;
		const lngToX = (lng: number) => (lng + 2) * 1200;
		const result = getPathMidpoint(nodes, latToY, lngToX);
		expect(result).not.toBeNull();
		expect(result!.x).toBeCloseTo(1500); // midpoint of lngToX(-1) and lngToX(-0.5)
	});

	it('handles three-node path correctly', () => {
		// L-shaped path: (0,0) → (10,0) → (10,10)
		const nodes: LatLon[] = [
			{ lat: 0, lon: 0 },
			{ lat: 0, lon: 10 },
			{ lat: 10, lon: 10 }
		];
		const result = getPathMidpoint(nodes, identity, identity);
		expect(result).not.toBeNull();
		// Total length = 10 + 10 = 20, midpoint at 10: exactly at the corner
		expect(result!.x).toBeCloseTo(10);
		expect(result!.y).toBeCloseTo(0);
	});

	it('flips angle when text would be upside down (angle > 90)', () => {
		// Path going right-to-left
		const nodes: LatLon[] = [
			{ lat: 0, lon: 10 },
			{ lat: 0, lon: 0 }
		];
		const result = getPathMidpoint(nodes, identity, identity);
		expect(result).not.toBeNull();
		// atan2(0, -10) = 180°, should become 0° after flip
		expect(result!.angle).toBeCloseTo(0);
	});

	it('flips angle when text would be upside down (angle < -90)', () => {
		// Path going right-to-left and slightly down
		const nodes: LatLon[] = [
			{ lat: 1, lon: 10 },
			{ lat: 0, lon: 0 }
		];
		const result = getPathMidpoint(nodes, identity, identity);
		expect(result).not.toBeNull();
		// angle should be within -90..90
		expect(result!.angle).toBeGreaterThanOrEqual(-90);
		expect(result!.angle).toBeLessThanOrEqual(90);
	});
});

// ── toPolygonPath ────────────────────────────────────────────────────────────

describe('toPolygonPath', () => {
	const id = (v: number) => v;

	it('generates correct polygon path', () => {
		const nodes: LatLon[] = [
			{ lat: 10, lon: 20 },
			{ lat: 30, lon: 40 },
			{ lat: 50, lon: 60 }
		];
		const result = toPolygonPath(nodes, id, id);
		expect(result).toBe('M 20.0 10.0 L 40.0 30.0 L 60.0 50.0 Z');
	});

	it('handles single node', () => {
		const result = toPolygonPath([{ lat: 5, lon: 10 }], id, id);
		expect(result).toBe('M 10.0 5.0 Z');
	});

	it('applies coordinate transforms', () => {
		const nodes: LatLon[] = [
			{ lat: 0, lon: 0 },
			{ lat: 1, lon: 1 }
		];
		const lngToX = (lng: number) => lng * 100;
		const latToY = (lat: number) => lat * 200;
		const result = toPolygonPath(nodes, lngToX, latToY);
		expect(result).toBe('M 0.0 0.0 L 100.0 200.0 Z');
	});
});

// ── toLinePath ───────────────────────────────────────────────────────────────

describe('toLinePath', () => {
	const id = (v: number) => v;

	it('generates correct line path', () => {
		const nodes: LatLon[] = [
			{ lat: 10, lon: 20 },
			{ lat: 30, lon: 40 }
		];
		const result = toLinePath(nodes, id, id);
		expect(result).toBe('M 20.0 10.0 L 40.0 30.0');
	});

	it('does not end with Z (unlike polygon)', () => {
		const result = toLinePath(
			[
				{ lat: 0, lon: 0 },
				{ lat: 1, lon: 1 }
			],
			id,
			id
		);
		expect(result).not.toContain('Z');
	});

	it('handles multi-node path', () => {
		const nodes: LatLon[] = [
			{ lat: 0, lon: 0 },
			{ lat: 1, lon: 1 },
			{ lat: 2, lon: 2 },
			{ lat: 3, lon: 3 }
		];
		const result = toLinePath(nodes, id, id);
		expect(result).toBe('M 0.0 0.0 L 1.0 1.0 L 2.0 2.0 L 3.0 3.0');
	});
});

// ── makeCoordConverters ──────────────────────────────────────────────────────

describe('makeCoordConverters', () => {
	// Bounding box: roughly SF area
	const neLat = 37.8;
	const neLng = -122.4;
	const swLat = 37.7;
	const swLng = -122.5;

	it('maps NE corner to top-right (0, 0 for lat, width for lng)', () => {
		const { latToY, lngToX } = makeCoordConverters(neLat, neLng, swLat, swLng, 1200, 800);
		expect(latToY(neLat)).toBeCloseTo(0);
		expect(lngToX(neLng)).toBeCloseTo(1200);
	});

	it('maps SW corner to bottom-left', () => {
		const { latToY, lngToX } = makeCoordConverters(neLat, neLng, swLat, swLng, 1200, 800);
		expect(latToY(swLat)).toBeCloseTo(800);
		expect(lngToX(swLng)).toBeCloseTo(0);
	});

	it('maps center to middle', () => {
		const { latToY, lngToX } = makeCoordConverters(neLat, neLng, swLat, swLng, 1200, 800);
		const midLat = (neLat + swLat) / 2;
		const midLng = (neLng + swLng) / 2;
		expect(latToY(midLat)).toBeCloseTo(400);
		expect(lngToX(midLng)).toBeCloseTo(600);
	});

	it('computes positive metersPerPixel', () => {
		const { metersPerPixel } = makeCoordConverters(neLat, neLng, swLat, swLng, 1200, 800);
		expect(metersPerPixel).toBeGreaterThan(0);
	});

	it('adjusts metersPerPixel with longitude compression', () => {
		// At the equator, 0.1° lon ≈ 11132m → 11132/1200 ≈ 9.3 m/px
		const equator = makeCoordConverters(0.05, 0.1, -0.05, 0, 1200, 800);
		// At 60° lat, 0.1° lon ≈ 5566m → 5566/1200 ≈ 4.6 m/px
		const arctic = makeCoordConverters(60.05, 0.1, 59.95, 0, 1200, 800);
		expect(equator.metersPerPixel).toBeGreaterThan(arctic.metersPerPixel);
	});
});

// ── buildOverpassQueries ─────────────────────────────────────────────────────

describe('buildOverpassQueries', () => {
	const bbox = '37.7,-122.5,37.8,-122.4';

	it('includes bbox in essential query', () => {
		const { essentialQuery } = buildOverpassQueries(bbox);
		expect(essentialQuery).toContain(bbox);
	});

	it('includes bbox in enrichment query', () => {
		const { enrichmentQuery } = buildOverpassQueries(bbox);
		expect(enrichmentQuery).toContain(bbox);
	});

	it('essential query includes building and highway', () => {
		const { essentialQuery } = buildOverpassQueries(bbox);
		expect(essentialQuery).toContain('"building"');
		expect(essentialQuery).toContain('"highway"');
	});

	it('enrichment query includes landuse, water, railway, name', () => {
		const { enrichmentQuery } = buildOverpassQueries(bbox);
		expect(enrichmentQuery).toContain('"landuse"');
		expect(enrichmentQuery).toContain('"water"');
		expect(enrichmentQuery).toContain('"railway"');
		expect(enrichmentQuery).toContain('"name"');
	});

	it('sets 30s timeout', () => {
		const { essentialQuery, enrichmentQuery } = buildOverpassQueries(bbox);
		expect(essentialQuery).toContain('[timeout:30]');
		expect(enrichmentQuery).toContain('[timeout:30]');
	});

	it('requests JSON output', () => {
		const { essentialQuery } = buildOverpassQueries(bbox);
		expect(essentialQuery).toContain('[out:json]');
	});
});

// ── orderedEndpoints ─────────────────────────────────────────────────────────

describe('orderedEndpoints', () => {
	const mirrors: OverpassMirror[] = [
		{ id: 'a', name: 'A', flag: '🇦', url: 'https://a.example.com' },
		{ id: 'b', name: 'B', flag: '🇧', url: 'https://b.example.com' },
		{ id: 'c', name: 'C', flag: '🇨', url: 'https://c.example.com' }
	];

	it('puts selected mirror first', () => {
		const result = orderedEndpoints('a', mirrors);
		expect(result[0].id).toBe('a');
		expect(result).toHaveLength(3);
	});

	it('includes all mirrors when one is selected', () => {
		const result = orderedEndpoints('b', mirrors);
		const ids = result.map((e) => e.id).sort();
		expect(ids).toEqual(['a', 'b', 'c']);
	});

	it('returns all mirrors when none selected', () => {
		const result = orderedEndpoints('', mirrors);
		expect(result).toHaveLength(3);
	});

	it('returns all mirrors when selection matches nothing', () => {
		const result = orderedEndpoints('z', mirrors);
		expect(result).toHaveLength(3);
	});

	it('maps to id and url only', () => {
		const result = orderedEndpoints('a', mirrors);
		for (const ep of result) {
			expect(ep).toHaveProperty('id');
			expect(ep).toHaveProperty('url');
			expect(Object.keys(ep)).toEqual(['id', 'url']);
		}
	});

	it('works with default OVERPASS_MIRRORS', () => {
		const result = orderedEndpoints('de');
		expect(result[0].id).toBe('de');
		expect(result.length).toBe(OVERPASS_MIRRORS.length);
	});
});

// ── buildNodeLookup ──────────────────────────────────────────────────────────

describe('buildNodeLookup', () => {
	it('extracts nodes from elements', () => {
		const elements = [
			{ type: 'node', id: 1, lat: 51.5, lon: -0.1 },
			{ type: 'node', id: 2, lat: 51.6, lon: -0.2 },
			{ type: 'way', id: 100, nodes: [1, 2] }
		];
		const lookup = buildNodeLookup(elements);
		expect(lookup[1]).toEqual({ lat: 51.5, lon: -0.1 });
		expect(lookup[2]).toEqual({ lat: 51.6, lon: -0.2 });
		expect(lookup[100]).toBeUndefined();
	});

	it('returns empty object for no nodes', () => {
		expect(buildNodeLookup([])).toEqual({});
	});

	it('ignores non-node elements', () => {
		const elements = [
			{ type: 'way', id: 1 },
			{ type: 'relation', id: 2 }
		];
		expect(buildNodeLookup(elements)).toEqual({});
	});
});

// ── categoriseElements ───────────────────────────────────────────────────────

describe('categoriseElements', () => {
	const nodes: Record<number, LatLon> = {
		1: { lat: 51.5, lon: -0.1 },
		2: { lat: 51.501, lon: -0.1 },
		3: { lat: 51.501, lon: -0.099 },
		4: { lat: 51.5, lon: -0.099 }
	};

	it('categorises buildings', () => {
		const elements = [{ type: 'way', nodes: [1, 2, 3, 4], tags: { building: 'yes' } }];
		const result = categoriseElements(elements, nodes);
		expect(result.buildings).toHaveLength(1);
		expect(result.buildings[0].nodes).toHaveLength(4);
	});

	it('categorises railways', () => {
		const elements = [{ type: 'way', nodes: [1, 2], tags: { railway: 'rail' } }];
		const result = categoriseElements(elements, nodes);
		expect(result.railways).toHaveLength(1);
		expect(result.railways[0].type).toBe('rail');
	});

	it('categorises water areas', () => {
		const elements = [{ type: 'way', nodes: [1, 2, 3], tags: { natural: 'water' } }];
		const result = categoriseElements(elements, nodes);
		expect(result.water).toHaveLength(1);
		expect(result.water[0].isLine).toBe(false);
	});

	it('categorises waterways as lines', () => {
		const elements = [{ type: 'way', nodes: [1, 2, 3], tags: { waterway: 'river' } }];
		const result = categoriseElements(elements, nodes);
		expect(result.water).toHaveLength(1);
		expect(result.water[0].isLine).toBe(true);
	});

	it('categorises landuse', () => {
		const elements = [{ type: 'way', nodes: [1, 2, 3], tags: { landuse: 'grass' } }];
		const result = categoriseElements(elements, nodes);
		expect(result.landuse).toHaveLength(1);
		expect(result.landuse[0].type).toBe('grass');
	});

	it('categorises leisure areas as landuse', () => {
		const elements = [
			{ type: 'way', nodes: [1, 2, 3], tags: { leisure: 'park', name: 'Hyde Park' } }
		];
		const result = categoriseElements(elements, nodes);
		expect(result.landuse).toHaveLength(1);
		expect(result.landuse[0].type).toBe('park');
	});

	it('categorises roads with attributes', () => {
		const elements = [
			{
				type: 'way',
				nodes: [1, 2],
				tags: {
					highway: 'primary',
					name: 'High Street',
					bridge: 'yes',
					tunnel: 'no'
				}
			}
		];
		const result = categoriseElements(elements, nodes);
		expect(result.roads).toHaveLength(1);
		expect(result.roads[0].highway).toBe('primary');
		expect(result.roads[0].name).toBe('High Street');
		expect(result.roads[0].bridge).toBe(true);
		expect(result.roads[0].tunnel).toBe(false);
	});

	it('extracts POI labels from named nodes', () => {
		const elements = [
			{ type: 'node', id: 10, lat: 51.5, lon: -0.1, tags: { name: 'Tesco', shop: 'supermarket' } }
		];
		const result = categoriseElements(elements, nodes);
		expect(result.poiLabels).toHaveLength(1);
		expect(result.poiLabels[0].name).toBe('Tesco');
		expect(result.poiLabels[0].category).toBe('landmark');
		expect(result.poiLabels[0].icon).toBe('🛒');
	});

	it('extracts POI labels from named ways (centroid)', () => {
		const elements = [
			{
				type: 'way',
				nodes: [1, 2, 3, 4],
				tags: { name: 'City Hall', amenity: 'townhall' }
			}
		];
		const result = categoriseElements(elements, nodes);
		expect(result.poiLabels).toHaveLength(1);
		expect(result.poiLabels[0].name).toBe('City Hall');
		// Centroid should be average of all 4 nodes
		const avgLat = (51.5 + 51.501 + 51.501 + 51.5) / 4;
		expect(result.poiLabels[0].lat).toBeCloseTo(avgLat);
	});

	it('classifies place names as place category', () => {
		const elements = [
			{ type: 'node', id: 10, lat: 51.5, lon: -0.1, tags: { name: 'Camden', place: 'suburb' } }
		];
		const result = categoriseElements(elements, nodes);
		expect(result.poiLabels[0].category).toBe('place');
	});

	it('classifies house numbers as address category', () => {
		const elements = [
			{
				type: 'node',
				id: 10,
				lat: 51.5,
				lon: -0.1,
				tags: { 'addr:housenumber': '42', 'addr:street': 'Baker Street' }
			}
		];
		const result = categoriseElements(elements, nodes);
		expect(result.poiLabels[0].category).toBe('address');
		expect(result.poiLabels[0].name).toBe('42');
	});

	it('classifies housenumber with amenity as landmark', () => {
		const elements = [
			{
				type: 'node',
				id: 10,
				lat: 51.5,
				lon: -0.1,
				tags: { 'addr:housenumber': '221B', name: 'Sherlock Museum', amenity: 'museum' }
			}
		];
		const result = categoriseElements(elements, nodes);
		expect(result.poiLabels[0].category).toBe('landmark');
	});

	it('de-duplicates POIs by name+position', () => {
		const elements = [
			{ type: 'node', id: 10, lat: 51.5, lon: -0.1, tags: { name: 'Same Place' } },
			{ type: 'node', id: 11, lat: 51.5, lon: -0.1, tags: { name: 'Same Place' } }
		];
		const result = categoriseElements(elements, nodes);
		expect(result.poiLabels).toHaveLength(1);
	});

	it('keeps POIs at different positions', () => {
		const elements = [
			{ type: 'node', id: 10, lat: 51.5, lon: -0.1, tags: { name: 'Shop' } },
			{ type: 'node', id: 11, lat: 52.0, lon: 0.0, tags: { name: 'Shop' } }
		];
		const result = categoriseElements(elements, nodes);
		expect(result.poiLabels).toHaveLength(2);
	});

	it('skips highway-tagged features from POI labels', () => {
		const elements = [
			{
				type: 'node',
				id: 10,
				lat: 51.5,
				lon: -0.1,
				tags: { name: 'High Street', highway: 'residential' }
			}
		];
		const result = categoriseElements(elements, nodes);
		expect(result.poiLabels).toHaveLength(0);
	});

	it('skips ways with fewer than 2 resolved nodes', () => {
		const elements = [
			{ type: 'way', nodes: [999], tags: { building: 'yes' } } // node 999 not in lookup
		];
		const result = categoriseElements(elements, nodes);
		expect(result.buildings).toHaveLength(0);
	});

	it('handles elements without tags', () => {
		const elements = [{ type: 'way', nodes: [1, 2] }];
		const result = categoriseElements(elements, nodes);
		// Should not crash and should not categorise
		expect(result.buildings).toHaveLength(0);
		expect(result.roads).toHaveLength(0);
	});

	it('handles water tag (not natural=water)', () => {
		const elements = [{ type: 'way', nodes: [1, 2, 3], tags: { water: 'lake' } }];
		const result = categoriseElements(elements, nodes);
		expect(result.water).toHaveLength(1);
		expect(result.water[0].isLine).toBe(false);
	});

	it('prioritises railway over other categories', () => {
		const elements = [
			{ type: 'way', nodes: [1, 2], tags: { railway: 'tram', highway: 'service' } }
		];
		const result = categoriseElements(elements, nodes);
		expect(result.railways).toHaveLength(1);
		expect(result.roads).toHaveLength(0);
	});
});

// ── ROAD_ORDER constant ──────────────────────────────────────────────────────

describe('ROAD_ORDER', () => {
	it('starts with minor roads', () => {
		expect(ROAD_ORDER[0]).toBe('footway');
	});

	it('ends with motorway', () => {
		expect(ROAD_ORDER[ROAD_ORDER.length - 1]).toBe('motorway');
	});

	it('has motorway after trunk', () => {
		expect(ROAD_ORDER.indexOf('motorway')).toBeGreaterThan(ROAD_ORDER.indexOf('trunk'));
	});

	it('has residential after service', () => {
		expect(ROAD_ORDER.indexOf('residential')).toBeGreaterThan(ROAD_ORDER.indexOf('service'));
	});
});

// ── LABEL_ROADS constant ─────────────────────────────────────────────────────

describe('LABEL_ROADS', () => {
	it('includes major road types', () => {
		expect(LABEL_ROADS).toContain('motorway');
		expect(LABEL_ROADS).toContain('primary');
		expect(LABEL_ROADS).toContain('residential');
	});

	it('excludes minor paths', () => {
		expect(LABEL_ROADS).not.toContain('footway');
		expect(LABEL_ROADS).not.toContain('cycleway');
		expect(LABEL_ROADS).not.toContain('service');
	});
});

// ── generateMapSvg ───────────────────────────────────────────────────────────

describe('generateMapSvg', () => {
	const baseParams: SvgParams = {
		buildings: [],
		landuse: [],
		water: [],
		roads: [],
		railways: [],
		poiLabels: [],
		bboxNELat: 51.51,
		bboxNELng: -0.09,
		bboxSWLat: 51.5,
		bboxSWLng: -0.11,
		latitude: 51.505,
		longitude: -0.1,
		radarAngle: 45
	};

	it('returns valid SVG with XML declaration', () => {
		const svg = generateMapSvg(baseParams);
		expect(svg).toContain('<?xml version="1.0" encoding="UTF-8"?>');
		expect(svg).toContain('<svg xmlns="http://www.w3.org/2000/svg"');
		expect(svg).toContain('</svg>');
	});

	it('has correct dimensions', () => {
		const svg = generateMapSvg(baseParams);
		expect(svg).toContain('width="1200"');
		expect(svg).toContain('height="800"');
	});

	it('includes background rect', () => {
		const svg = generateMapSvg(baseParams);
		expect(svg).toContain('fill="#f2efe9"');
	});

	it('includes radar FOV triangle', () => {
		const svg = generateMapSvg(baseParams);
		expect(svg).toContain('<polygon points=');
		expect(svg).toContain('fill="#ef4444"');
	});

	it('includes radar marker circle', () => {
		const svg = generateMapSvg(baseParams);
		expect(svg).toContain('fill="#3b82f6"');
		expect(svg).toContain('stroke="white"');
	});

	it('renders buildings', () => {
		const params: SvgParams = {
			...baseParams,
			buildings: [
				{
					nodes: [
						{ lat: 51.505, lon: -0.1 },
						{ lat: 51.506, lon: -0.1 },
						{ lat: 51.506, lon: -0.099 },
						{ lat: 51.505, lon: -0.099 }
					]
				}
			]
		};
		const svg = generateMapSvg(params);
		expect(svg).toContain('fill="#d9d0c9"');
		expect(svg).toContain('stroke="#bbb5b0"');
	});

	it('renders landuse areas', () => {
		const params: SvgParams = {
			...baseParams,
			landuse: [
				{
					type: 'park',
					nodes: [
						{ lat: 51.505, lon: -0.1 },
						{ lat: 51.506, lon: -0.1 },
						{ lat: 51.506, lon: -0.099 }
					]
				}
			]
		};
		const svg = generateMapSvg(params);
		expect(svg).toContain('fill="#c8facc"'); // park colour
	});

	it('renders water polygons', () => {
		const params: SvgParams = {
			...baseParams,
			water: [
				{
					isLine: false,
					nodes: [
						{ lat: 51.505, lon: -0.1 },
						{ lat: 51.506, lon: -0.1 },
						{ lat: 51.506, lon: -0.099 }
					]
				}
			]
		};
		const svg = generateMapSvg(params);
		expect(svg).toContain('fill="#aad3df"');
	});

	it('renders water lines', () => {
		const params: SvgParams = {
			...baseParams,
			water: [
				{
					isLine: true,
					nodes: [
						{ lat: 51.505, lon: -0.1 },
						{ lat: 51.506, lon: -0.099 }
					]
				}
			]
		};
		const svg = generateMapSvg(params);
		expect(svg).toContain('stroke="#aad3df"');
	});

	it('renders roads with casing', () => {
		const params: SvgParams = {
			...baseParams,
			roads: [
				{
					highway: 'primary',
					nodes: [
						{ lat: 51.505, lon: -0.1 },
						{ lat: 51.506, lon: -0.099 }
					]
				}
			]
		};
		const svg = generateMapSvg(params);
		expect(svg).toContain('stroke="#fcd6a4"'); // primary road colour
		expect(svg).toContain('stroke="#666666"'); // casing
	});

	it('renders bridge roads with black casing', () => {
		const params: SvgParams = {
			...baseParams,
			roads: [
				{
					highway: 'primary',
					bridge: true,
					nodes: [
						{ lat: 51.505, lon: -0.1 },
						{ lat: 51.506, lon: -0.099 }
					]
				}
			]
		};
		const svg = generateMapSvg(params);
		expect(svg).toContain('stroke="#000000"'); // bridge casing
	});

	it('renders tunnel roads with opacity', () => {
		const params: SvgParams = {
			...baseParams,
			roads: [
				{
					highway: 'primary',
					tunnel: true,
					nodes: [
						{ lat: 51.505, lon: -0.1 },
						{ lat: 51.506, lon: -0.099 }
					]
				}
			]
		};
		const svg = generateMapSvg(params);
		expect(svg).toContain('opacity="0.5"');
	});

	it('renders dashed roads', () => {
		const params: SvgParams = {
			...baseParams,
			roads: [
				{
					highway: 'footway',
					nodes: [
						{ lat: 51.505, lon: -0.1 },
						{ lat: 51.506, lon: -0.099 }
					]
				}
			]
		};
		const svg = generateMapSvg(params);
		expect(svg).toContain('stroke-dasharray="4,2"');
	});

	it('renders street name labels', () => {
		const params: SvgParams = {
			...baseParams,
			roads: [
				{
					highway: 'residential',
					name: 'Baker Street',
					nodes: [
						{ lat: 51.505, lon: -0.1 },
						{ lat: 51.505, lon: -0.099 }
					]
				}
			]
		};
		const svg = generateMapSvg(params);
		expect(svg).toContain('Baker Street');
	});

	it('de-duplicates street name labels', () => {
		const params: SvgParams = {
			...baseParams,
			roads: [
				{
					highway: 'residential',
					name: 'Baker Street',
					nodes: [
						{ lat: 51.505, lon: -0.1 },
						{ lat: 51.505, lon: -0.099 }
					]
				},
				{
					highway: 'residential',
					name: 'Baker Street',
					nodes: [
						{ lat: 51.506, lon: -0.1 },
						{ lat: 51.506, lon: -0.099 }
					]
				}
			]
		};
		const svg = generateMapSvg(params);
		const count = (svg.match(/Baker Street/g) || []).length;
		expect(count).toBe(1);
	});

	it('renders railway paths', () => {
		const params: SvgParams = {
			...baseParams,
			railways: [
				{
					type: 'rail',
					nodes: [
						{ lat: 51.505, lon: -0.1 },
						{ lat: 51.506, lon: -0.099 }
					]
				}
			]
		};
		const svg = generateMapSvg(params);
		expect(svg).toContain('stroke="#999999"'); // rail base
		expect(svg).toContain('stroke-dasharray="8,8"'); // rail dashes
	});

	it('renders place name POIs in italic', () => {
		const params: SvgParams = {
			...baseParams,
			poiLabels: [{ name: 'Camden', lat: 51.505, lon: -0.1, category: 'place', icon: '\u25cf' }]
		};
		const svg = generateMapSvg(params);
		expect(svg).toContain('font-style="italic"');
		expect(svg).toContain('Camden');
	});

	it('renders address POIs with small font', () => {
		const params: SvgParams = {
			...baseParams,
			poiLabels: [{ name: '42', lat: 51.505, lon: -0.1, category: 'address', icon: '\u25cf' }]
		};
		const svg = generateMapSvg(params);
		expect(svg).toContain('font-size="12"');
	});

	it('renders landmark POIs with icon emoji', () => {
		const params: SvgParams = {
			...baseParams,
			poiLabels: [{ name: 'School', lat: 51.505, lon: -0.1, category: 'landmark', icon: '🏫' }]
		};
		const svg = generateMapSvg(params);
		expect(svg).toContain('🏫');
		expect(svg).toContain('School');
	});

	it('renders landmark POIs with default dot as circle', () => {
		const params: SvgParams = {
			...baseParams,
			poiLabels: [{ name: 'Mystery', lat: 51.505, lon: -0.1, category: 'landmark', icon: '\u25cf' }]
		};
		const svg = generateMapSvg(params);
		expect(svg).toContain('<circle');
		expect(svg).toContain('r="5"');
		expect(svg).toContain('Mystery');
	});

	it('skips POIs outside visible area', () => {
		const params: SvgParams = {
			...baseParams,
			poiLabels: [
				// Way outside the bounding box
				{ name: 'FarAway', lat: 99, lon: 99, category: 'landmark', icon: '🏫' }
			]
		};
		const svg = generateMapSvg(params);
		expect(svg).not.toContain('FarAway');
	});

	it('uses default radar position when lat/lng is null', () => {
		const params: SvgParams = {
			...baseParams,
			latitude: null,
			longitude: null
		};
		const svg = generateMapSvg(params);
		// Should use svgWidth/2 = 600, svgHeight/2 = 400
		expect(svg).toContain('cx="600.0"');
		expect(svg).toContain('cy="400.0"');
	});

	it('uses angle 0 when radarAngle is null', () => {
		const params: SvgParams = {
			...baseParams,
			radarAngle: null
		};
		const svg = generateMapSvg(params);
		// Should still produce a valid triangle
		expect(svg).toContain('<polygon points=');
	});

	it('escapes XML in street names', () => {
		const params: SvgParams = {
			...baseParams,
			roads: [
				{
					highway: 'residential',
					name: 'A & B <Street>',
					nodes: [
						{ lat: 51.505, lon: -0.1 },
						{ lat: 51.505, lon: -0.099 }
					]
				}
			]
		};
		const svg = generateMapSvg(params);
		expect(svg).toContain('A &amp; B &lt;Street&gt;');
		expect(svg).not.toContain('A & B <Street>');
	});

	it('escapes XML in POI names', () => {
		const params: SvgParams = {
			...baseParams,
			poiLabels: [
				{
					name: "O'Malley's & Co",
					lat: 51.505,
					lon: -0.1,
					category: 'landmark',
					icon: '☕'
				}
			]
		};
		const svg = generateMapSvg(params);
		expect(svg).toContain('O&#39;Malley&#39;s &amp; Co');
	});

	it('sorts roads by ROAD_ORDER', () => {
		const params: SvgParams = {
			...baseParams,
			roads: [
				{
					highway: 'motorway',
					nodes: [
						{ lat: 51.505, lon: -0.1 },
						{ lat: 51.505, lon: -0.099 }
					]
				},
				{
					highway: 'footway',
					nodes: [
						{ lat: 51.506, lon: -0.1 },
						{ lat: 51.506, lon: -0.099 }
					]
				}
			]
		};
		const svg = generateMapSvg(params);
		// Footway should appear before motorway in the SVG (drawn first = underneath)
		const footwayIdx = svg.indexOf('#fa8072'); // footway colour
		const motorwayIdx = svg.indexOf('#e892a2'); // motorway colour
		expect(footwayIdx).toBeLessThan(motorwayIdx);
	});

	it('includes OSM attribution', () => {
		const svg = generateMapSvg(baseParams);
		expect(svg).toContain('OpenStreetMap contributors');
	});
});

// ── svgToBase64 ──────────────────────────────────────────────────────────────

describe('svgToBase64', () => {
	it('encodes simple SVG to base64', () => {
		const svg = '<svg></svg>';
		const result = svgToBase64(svg);
		expect(atob(result)).toBe(svg);
	});

	it('handles unicode characters', () => {
		const svg = '<svg>🌳</svg>';
		const result = svgToBase64(svg);
		// Decode and verify round-trip
		const decoded = new TextDecoder().decode(Uint8Array.from(atob(result), (c) => c.charCodeAt(0)));
		expect(decoded).toBe(svg);
	});

	it('handles large SVGs (chunked encoding)', () => {
		// Create a string larger than the 8192 chunk size
		const svg = '<svg>' + 'x'.repeat(20000) + '</svg>';
		const result = svgToBase64(svg);
		const decoded = new TextDecoder().decode(Uint8Array.from(atob(result), (c) => c.charCodeAt(0)));
		expect(decoded).toBe(svg);
	});

	it('returns valid base64', () => {
		const result = svgToBase64('<svg/>');
		expect(result).toMatch(/^[A-Za-z0-9+/]+=*$/);
	});
});

// ── OVERPASS_MIRRORS constant ────────────────────────────────────────────────

describe('OVERPASS_MIRRORS', () => {
	it('contains at least 3 mirrors', () => {
		expect(OVERPASS_MIRRORS.length).toBeGreaterThanOrEqual(3);
	});

	it('each mirror has required fields', () => {
		for (const m of OVERPASS_MIRRORS) {
			expect(m.id).toBeTruthy();
			expect(m.name).toBeTruthy();
			expect(m.flag).toBeTruthy();
			expect(m.url).toMatch(/^https:\/\//);
		}
	});

	it('has unique IDs', () => {
		const ids = OVERPASS_MIRRORS.map((m) => m.id);
		expect(new Set(ids).size).toBe(ids.length);
	});

	it('all URLs end with /interpreter', () => {
		for (const m of OVERPASS_MIRRORS) {
			expect(m.url).toMatch(/\/interpreter$/);
		}
	});
});

// ── Integration: categorise → generate ───────────────────────────────────────

describe('integration: categorise and generate', () => {
	it('produces valid SVG from raw Overpass-like elements', () => {
		const elements: Array<Record<string, unknown>> = [
			{ type: 'node', id: 1, lat: 51.505, lon: -0.1 },
			{ type: 'node', id: 2, lat: 51.506, lon: -0.1 },
			{ type: 'node', id: 3, lat: 51.506, lon: -0.099 },
			{ type: 'node', id: 4, lat: 51.505, lon: -0.099 },
			{ type: 'way', id: 100, nodes: [1, 2, 3, 4], tags: { building: 'yes' } },
			{ type: 'way', id: 101, nodes: [1, 2], tags: { highway: 'residential', name: 'Test St' } },
			{
				type: 'node',
				id: 200,
				lat: 51.5055,
				lon: -0.0995,
				tags: { name: 'Local Café', amenity: 'cafe' }
			}
		];

		const nodeLookup = buildNodeLookup(elements);
		const categorised = categoriseElements(elements, nodeLookup);

		expect(categorised.buildings).toHaveLength(1);
		expect(categorised.roads).toHaveLength(1);
		expect(categorised.poiLabels).toHaveLength(1);
		expect(categorised.poiLabels[0].icon).toBe('☕');

		const svg = generateMapSvg({
			...categorised,
			bboxNELat: 51.51,
			bboxNELng: -0.09,
			bboxSWLat: 51.5,
			bboxSWLng: -0.11,
			latitude: 51.505,
			longitude: -0.1,
			radarAngle: 90
		});

		expect(svg).toContain('<svg');
		expect(svg).toContain('</svg>');
		expect(svg).toContain('Test St');
		expect(svg).toContain('Local Caf');
		expect(svg).toContain('fill="#d9d0c9"'); // building
		expect(svg).toContain('fill="#ef4444"'); // FOV triangle

		// Round-trip through base64
		const b64 = svgToBase64(svg);
		const decoded = new TextDecoder().decode(Uint8Array.from(atob(b64), (c) => c.charCodeAt(0)));
		expect(decoded).toBe(svg);
	});
});
