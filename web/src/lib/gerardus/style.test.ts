import type { SiteFeature } from './client';
import { SURFACE_TYPES } from './client';
import {
	featureColour,
	LAYER_COLOURS,
	layerLabel,
	mapStyle,
	OBSERVED_COLOURS,
	propertyRows,
	SURFACE_COLOURS,
	WALL_ROLE_COLOURS
} from './style';

const f = (layer: string, kind = '', tags: Record<string, string> = {}) => ({ layer, kind, tags });

describe('colours', () => {
	it('colours street use by kind, and surfaces by what they are', () => {
		for (const s of SURFACE_TYPES) {
			expect(featureColour(f('street_use', 'surface', { surface: s }))).toBe(SURFACE_COLOURS[s]);
		}
		expect(new Set(SURFACE_TYPES.map((s) => SURFACE_COLOURS[s])).size).toBe(SURFACE_TYPES.length);
		expect(featureColour(f('street_use', 'surface', { surface: 'moat' }))).toBe('#64748b');
		expect(featureColour(f('street_use', 'driveway'))).toBe('#f472b6');
		expect(featureColour(f('street_use', 'parking_space', { space_type: 'permit' }))).toBe(
			'#a855f7'
		);
		expect(featureColour(f('street_use', 'parking_space'))).toBe('#3b82f6');
		expect(featureColour(f('street_use', 'travel_lane', { lane_type: 'bike' }))).toBe('#22c55e');
		expect(featureColour(f('street_use', 'travel_lane'))).toBe('#e5e7eb');
		expect(featureColour(f('street_use', 'zebra'))).toBe('#64748b');
	});

	it('colours observed features by kind, wall planes by role, the rest by layer', () => {
		expect(featureColour(f('observed', 'pole'))).toBe(OBSERVED_COLOURS.pole);
		expect(featureColour(f('observed', 'ghost'))).toBe(LAYER_COLOURS.observed);
		expect(featureColour(f('wall_planes', 'wall_plane', { role: 'popout' }))).toBe(
			WALL_ROLE_COLOURS.popout
		);
		expect(featureColour(f('wall_planes', 'wall_plane'))).toBe(WALL_ROLE_COLOURS.wall);
		expect(featureColour(f('kerbs', 'kerb'))).toBe(LAYER_COLOURS.kerbs);
		expect(featureColour(f('mystery'))).toBe('#64748b');
	});
});

describe('the 2D map', () => {
	it('fills surfaces and spaces, outlines boxes, and draws lanes with arrows', () => {
		expect(mapStyle(f('street_use', 'surface')).fillOpacity).toBeGreaterThan(0);
		expect(mapStyle(f('street_use', 'parking_space')).fillOpacity).toBeGreaterThan(0);
		expect(mapStyle(f('street_use', 'travel_lane')).arrows).toBe(true);
		expect(mapStyle(f('street_use', 'driveway')).weight).toBe(7);
		expect(mapStyle(f('street_use', 'other')).weight).toBe(2);
		expect(mapStyle(f('footprints')).dashArray).toBe('4 3');
		expect(mapStyle(f('osm')).dashArray).toBe('2 3');
		expect(mapStyle(f('mystery')).rank).toBe(6);
	});

	it('draws surfaces underneath and points on top', () => {
		const order = [
			f('street_use', 'surface'),
			f('footprints'),
			f('boxes'),
			f('street_use', 'parking_space'),
			f('street_use', 'driveway'),
			f('wall_planes'),
			f('city_streets'),
			f('kerbs'),
			f('observed'),
			f('street_use', 'travel_lane'),
			f('osm'),
			f('intersections')
		].map((x) => mapStyle(x).rank);
		expect([...order].sort((a, b) => a - b)).toEqual(order);
		expect(mapStyle(f('city_intersections')).rank).toBe(mapStyle(f('intersections')).rank);
	});

	it('labels layers, the unknown ones from their names', () => {
		expect(layerLabel('kerbs')).toBe('kerbs and islands');
		expect(layerLabel('new_layer_x')).toBe('new layer x');
	});
});

describe('the property panel', () => {
	it('lists the reserved properties first, then the tags in order, skipping blanks', () => {
		const feature: SiteFeature = {
			layer: 'street_use',
			id: 'gms:01J',
			kind: 'surface',
			tags: { surface: 'verge', area_m2: '12.5' },
			author: 'Dana',
			editedAt: '2026-10-23T10:00:00Z',
			geometry: { type: 'Point', coordinates: [0, 0] }
		};
		expect(propertyRows(feature)).toEqual([
			['layer', 'street_use'],
			['id', 'gms:01J'],
			['kind', 'surface'],
			['author', 'Dana'],
			['edited_at', '2026-10-23T10:00:00Z'],
			['area_m2', '12.5'],
			['surface', 'verge'],
			['geometry', 'Point']
		]);
		expect(
			propertyRows({ ...feature, id: '', author: undefined, editedAt: undefined, tags: {} })
		).toEqual([
			['layer', 'street_use'],
			['kind', 'surface'],
			['geometry', 'Point']
		]);
	});
});
