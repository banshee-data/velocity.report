/**
 * How the site viewer draws each layer and kind, on the 2D map and in the 3D
 * view alike, so a colour means the same thing in both.
 */
import type { SiteFeature } from './client';

export const LAYER_LABEL: Record<string, string> = {
	street_use: 'street use',
	footprints: 'footprints',
	boxes: 'boxes',
	wall_planes: 'wall planes',
	city_streets: 'city streets',
	city_intersections: 'city intersections',
	kerbs: 'kerbs and islands',
	observed: 'observed',
	osm: 'OpenStreetMap ways',
	intersections: 'OpenStreetMap intersections'
};

export function layerLabel(layer: string): string {
	return LAYER_LABEL[layer] ?? layer.replace(/_/g, ' ');
}

/** The colour that stands for each layer in the layer lists. */
export const LAYER_COLOURS: Record<string, string> = {
	street_use: '#f472b6',
	footprints: '#a78bfa',
	boxes: '#f59e0b',
	wall_planes: '#94a3b8',
	city_streets: '#94a3b8',
	city_intersections: '#cbd5e1',
	kerbs: '#f8fafc',
	observed: '#38bdf8',
	osm: '#7dd3fc',
	intersections: '#7dd3fc'
};
const OTHER = '#64748b';

/** A surface's colour by its `surface` property. */
export const SURFACE_COLOURS: Record<string, string> = {
	carriageway: '#475569',
	sidewalk: '#d6d3d1',
	verge: '#22c55e',
	apron: '#eab308',
	median: '#a855f7'
};

/** Street use by kind; parked-car spaces and lanes refine it by their own type. */
export const STREET_USE_COLOURS: Record<string, string> = {
	driveway: '#f472b6',
	parking_space: '#3b82f6',
	travel_lane: '#e5e7eb',
	surface: '#64748b'
};

export const SPACE_COLOURS: Record<string, string> = {
	metered: '#3b82f6',
	permit: '#a855f7',
	loading: '#eab308',
	disabled: '#06b6d4',
	free: '#10b981'
};

export const LANE_COLOURS: Record<string, string> = {
	general: '#e5e7eb',
	bus: '#ef4444',
	bike: '#22c55e',
	turn: '#f97316'
};

export const OBSERVED_COLOURS: Record<string, string> = {
	facade: '#38bdf8',
	kerb: '#f1f5f9',
	sensor: '#ef4444',
	correction: '#facc15',
	pole: '#fb923c'
};

export const WALL_ROLE_COLOURS: Record<string, string> = {
	wall: '#94a3b8',
	popout: '#f97316',
	recess: '#3b82f6'
};

/** The colour a feature is drawn in. */
export function featureColour(f: Pick<SiteFeature, 'layer' | 'kind' | 'tags'>): string {
	switch (f.layer) {
		case 'street_use':
			if (f.kind === 'surface')
				return SURFACE_COLOURS[f.tags.surface] ?? STREET_USE_COLOURS.surface;
			if (f.kind === 'parking_space')
				return SPACE_COLOURS[f.tags.space_type] ?? STREET_USE_COLOURS.parking_space;
			if (f.kind === 'travel_lane')
				return LANE_COLOURS[f.tags.lane_type] ?? STREET_USE_COLOURS.travel_lane;
			return STREET_USE_COLOURS[f.kind] ?? OTHER;
		case 'observed':
			return OBSERVED_COLOURS[f.kind] ?? LAYER_COLOURS.observed;
		case 'wall_planes':
			return WALL_ROLE_COLOURS[f.tags.role] ?? WALL_ROLE_COLOURS.wall;
		default:
			return LAYER_COLOURS[f.layer] ?? OTHER;
	}
}

/** How a feature is drawn on the 2D map. */
export interface MapStyle {
	colour: string;
	weight: number;
	opacity: number;
	/** A polygon's fill opacity; 0 draws only its outline. */
	fillOpacity: number;
	dashArray?: string;
	/** Draw a point as a ring of this radius in pixels. */
	radius: number;
	/** Draw arrows in the direction of travel. */
	arrows: boolean;
	/** Lower draws first, underneath. */
	rank: number;
}

export function mapStyle(f: Pick<SiteFeature, 'layer' | 'kind' | 'tags'>): MapStyle {
	const colour = featureColour(f);
	const base: MapStyle = {
		colour,
		weight: 1.5,
		opacity: 0.9,
		fillOpacity: 0,
		radius: 4,
		arrows: false,
		rank: 6
	};
	switch (f.layer) {
		case 'street_use':
			if (f.kind === 'surface') return { ...base, weight: 1, fillOpacity: 0.35, rank: 0 };
			if (f.kind === 'parking_space') return { ...base, weight: 2, fillOpacity: 0.25, rank: 3 };
			if (f.kind === 'driveway') return { ...base, weight: 7, opacity: 0.95, rank: 4 };
			if (f.kind === 'travel_lane') return { ...base, weight: 3, arrows: true, rank: 9 };
			return { ...base, weight: 2, rank: 4 };
		case 'footprints':
			return { ...base, weight: 1, dashArray: '4 3', rank: 1 };
		case 'boxes':
			return { ...base, weight: 1.2, fillOpacity: 0.08, rank: 2 };
		case 'wall_planes':
			// A vertical plane seen from above is a line along its foot.
			return { ...base, weight: 3, rank: 5 };
		case 'city_streets':
			return { ...base, opacity: 0.85, rank: 6 };
		case 'kerbs':
			return { ...base, weight: 3, opacity: 0.95, fillOpacity: 0.12, rank: 7 };
		case 'observed':
			return { ...base, weight: 2, rank: 8 };
		case 'osm':
			return { ...base, weight: 1, dashArray: '2 3', opacity: 0.8, rank: 10 };
		case 'city_intersections':
		case 'intersections':
			return { ...base, fillOpacity: 0.2, rank: 11 };
		default:
			return base;
	}
}

/** The reserved properties and tags of a feature as label and value rows, for the property panel. */
export function propertyRows(f: SiteFeature): [string, string][] {
	const rows: [string, string][] = [
		['layer', f.layer],
		['id', f.id],
		['kind', f.kind]
	];
	if (f.author) rows.push(['author', f.author]);
	if (f.editedAt) rows.push(['edited_at', f.editedAt]);
	for (const key of Object.keys(f.tags).sort()) rows.push([key, f.tags[key]]);
	rows.push(['geometry', f.geometry.type]);
	return rows.filter(([, v]) => v !== '');
}
