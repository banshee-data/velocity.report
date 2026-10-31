/**
 * How the site viewer draws each layer and kind, on the 2D map and in the 3D
 * view alike, so a colour means the same thing in both.
 */
import type { SiteFeature } from './client';

export const LAYER_LABEL: Record<string, string> = {
	street_use: 'street use',
	footprints: 'footprints',
	boxes: 'boxes',
	box_walls: 'box walls',
	wall_planes: 'wall planes',
	city_streets: 'city streets',
	city_intersections: 'city intersections',
	kerbs: 'kerbs and islands',
	observed: 'observed',
	building_parts: 'building parts',
	roof_planes: 'roof planes',
	wall_surfaces: 'building walls',
	lane_markings: 'lane markings',
	street_furniture: 'poles and signs',
	trees: 'trees'
};

export function layerLabel(layer: string): string {
	return LAYER_LABEL[layer] ?? layer.replace(/_/g, ' ');
}

/** The colour that stands for each layer in the layer lists. */
export const LAYER_COLOURS: Record<string, string> = {
	street_use: '#f472b6',
	footprints: '#a78bfa',
	boxes: '#f59e0b',
	box_walls: '#f59e0b',
	wall_planes: '#94a3b8',
	city_streets: '#94a3b8',
	city_intersections: '#cbd5e1',
	kerbs: '#f8fafc',
	observed: '#38bdf8',
	building_parts: '#e7c9a0',
	roof_planes: '#fbbf24',
	wall_surfaces: '#c8a27a',
	lane_markings: '#fef08a',
	street_furniture: '#60a5fa',
	trees: '#4ade80'
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

/** A building part by its `roof:shape`, Simple 3D Buildings' standard values and the cone. */
export const ROOF_SHAPE_COLOURS: Record<string, string> = {
	flat: '#60a5fa',
	gabled: '#f87171',
	hipped: '#fbbf24',
	pyramidal: '#d946ef',
	skillion: '#2dd4bf',
	'half-hipped': '#fca5a5',
	gambrel: '#a16207',
	mansard: '#8b5cf6',
	saltbox: '#fde047',
	dome: '#7dd3fc',
	round: '#4ade80',
	onion: '#f472b6',
	cone: '#bef264'
};
/** A part with no `roof:shape`, `unknown`, or one outside the table. */
export const ROOF_SHAPE_UNKNOWN = '#9ca3af';

export function roofShapeColour(shape: string | undefined): string {
	return (shape && ROOF_SHAPE_COLOURS[shape]) || ROOF_SHAPE_UNKNOWN;
}

/** The slopes of the roof-plane ramp: blue flat, yellow at 22.5°, red at 45° and steeper. */
const SLOPE_STOPS: [number, [number, number, number]][] = [
	[0, [0x60, 0xa5, 0xfa]],
	[22.5, [0xfa, 0xcc, 0x15]],
	[45, [0xef, 0x44, 0x44]]
];

function hex(rgb: number[]): string {
	return `#${rgb.map((c) => Math.round(c).toString(16).padStart(2, '0')).join('')}`;
}

/** A roof plane's colour by its `slope_deg`; the layer's own colour when it has none. */
export function roofSlopeColour(slope: string | number | undefined): string {
	const deg = typeof slope === 'number' ? slope : slope === undefined ? NaN : parseFloat(slope);
	if (!Number.isFinite(deg)) return LAYER_COLOURS.roof_planes;
	const d = Math.min(Math.max(deg, 0), 45);
	for (let i = 1; i < SLOPE_STOPS.length; i++) {
		const [d1, c1] = SLOPE_STOPS[i];
		const [d0, c0] = SLOPE_STOPS[i - 1];
		if (d <= d1) {
			const t = (d - d0) / (d1 - d0);
			return hex(c0.map((c, k) => c + (c1[k] - c) * t));
		}
	}
	return hex(SLOPE_STOPS[SLOPE_STOPS.length - 1][1]);
}

/** A lane marking by its `colour`. */
export const MARKING_COLOURS: Record<string, string> = {
	white: '#f8fafc',
	yellow: '#facc15'
};

/** Street furniture by kind, and a tree's two parts. */
export const FURNITURE_COLOURS: Record<string, string> = {
	pole: '#60a5fa',
	sign: '#f43f5e'
};
export const TREE_COLOURS = { crown: '#4ade80', trunk: '#a16207' } as const;

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
		case 'building_parts':
			return roofShapeColour(f.tags['roof:shape']);
		case 'roof_planes':
			return roofSlopeColour(f.tags.slope_deg);
		case 'lane_markings':
			return MARKING_COLOURS[f.tags.colour] ?? LAYER_COLOURS.lane_markings;
		case 'street_furniture':
			return FURNITURE_COLOURS[f.kind] ?? LAYER_COLOURS.street_furniture;
		case 'trees':
			return TREE_COLOURS.crown;
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
		case 'building_parts':
			return { ...base, weight: 1.2, fillOpacity: 0.12, rank: 2 };
		case 'roof_planes':
			return { ...base, weight: 1, fillOpacity: 0.35, rank: 3 };
		case 'lane_markings':
			return {
				...base,
				weight: f.tags.pattern === 'double' ? 3.5 : 2,
				opacity: 1,
				fillOpacity: 0.5,
				dashArray: f.tags.pattern === 'dashed' ? '6 6' : undefined,
				rank: 8
			};
		case 'street_furniture':
			return { ...base, weight: 2, radius: f.kind === 'sign' ? 3 : 4, rank: 9 };
		case 'trees':
			return { ...base, weight: 1, fillOpacity: 0.25, rank: 5 };
		case 'city_intersections':
			return { ...base, fillOpacity: 0.2, rank: 11 };
		default:
			return base;
	}
}

/** Reads a tag as metres (or degrees): its leading number, so `"12 m"` is 12; NaN when it is none. */
export function tagNumber(tags: Record<string, string>, key: string): number {
	const v = tags[key];
	return v === undefined ? NaN : parseFloat(v);
}

/** A tree's crown radius on the 2D map and in 3D, metres: `crown_radius_m`, else 2 m. */
export const DEFAULT_CROWN_RADIUS_M = 2;

export function crownRadius(tags: Record<string, string>): number {
	const r = tagNumber(tags, 'crown_radius_m');
	return Number.isFinite(r) && r > 0 ? r : DEFAULT_CROWN_RADIUS_M;
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
	if (f.modelId) rows.push(['model_id', f.modelId]);
	if (f.state) rows.push(['state', f.state]);
	for (const key of Object.keys(f.tags).sort()) rows.push([key, f.tags[key]]);
	rows.push(['geometry', f.geometry.type]);
	return rows.filter(([, v]) => v !== '');
}
