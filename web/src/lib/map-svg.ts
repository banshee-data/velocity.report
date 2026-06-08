// Pure functions and constants extracted from MapEditorInteractive.svelte.
// Everything here is testable without a DOM or Leaflet.

// ── Types ────────────────────────────────────────────────────────────────────

export interface OverpassMirror {
	id: string;
	name: string;
	flag: string;
	url: string;
}

export interface RoadStyle {
	width: number;
	color: string;
	casing: number;
	dash?: string;
}

export interface PathMidpoint {
	x: number;
	y: number;
	angle: number;
}

export interface LatLon {
	lat: number;
	lon: number;
}

export interface Building {
	nodes: LatLon[];
	part?: boolean;
}

export interface LanduseArea {
	type: string;
	nodes: LatLon[];
}

export interface WaterFeature {
	isLine: boolean;
	nodes: LatLon[];
}

export interface Road {
	highway: string;
	name?: string;
	bridge?: boolean;
	tunnel?: boolean;
	nodes: LatLon[];
}

export interface Railway {
	type: string;
	nodes: LatLon[];
}

export interface Boundary {
	type: string;
	name?: string;
	nodes: LatLon[];
}

export type PoiCategory = 'place' | 'address' | 'landmark';

export interface PoiLabel {
	name: string;
	lat: number;
	lon: number;
	category: PoiCategory;
	icon: string;
}

export interface CategorisedElements {
	buildings: Building[];
	landuse: LanduseArea[];
	water: WaterFeature[];
	roads: Road[];
	railways: Railway[];
	boundaries: Boundary[];
	poiLabels: PoiLabel[];
}

export interface SvgParams {
	buildings: Building[];
	landuse: LanduseArea[];
	water: WaterFeature[];
	roads: Road[];
	railways: Railway[];
	boundaries: Boundary[];
	poiLabels: PoiLabel[];
	bboxNELat: number;
	bboxNELng: number;
	bboxSWLat: number;
	bboxSWLng: number;
	latitude: number | null;
	longitude: number | null;
	radarAngle: number | null;
}

// ── Constants ────────────────────────────────────────────────────────────────

export const OVERPASS_MIRRORS: OverpassMirror[] = [
	{
		id: 'de',
		name: 'overpass-api.de',
		flag: '🇩🇪',
		url: 'https://overpass-api.de/api/interpreter'
	},
	{
		id: 'fr',
		name: 'openstreetmap.fr',
		flag: '🇫🇷',
		url: 'https://overpass.openstreetmap.fr/api/interpreter'
	},
	{
		id: 'ch',
		name: 'kumi.systems',
		flag: '🇨🇭',
		url: 'https://overpass.kumi.systems/api/interpreter'
	},
	{
		id: 'at',
		name: 'private.coffee',
		flag: '🇦🇹',
		url: 'https://overpass.private.coffee/api/interpreter'
	},
	{
		id: 'ru',
		name: 'maps.mail.ru',
		flag: '🇷🇺',
		url: 'https://maps.mail.ru/osm/tools/overpass/api/interpreter'
	}
];

export const POI_ICON_MAP: Record<string, string> = {
	// Education
	school: '🏫',
	kindergarten: '💒',
	university: '🎓',
	college: '🎓',
	// Health
	hospital: '🏥',
	clinic: '🏥',
	doctors: '🏥',
	dentist: '🦷',
	pharmacy: '💊',
	veterinary: '🐾',
	// Religion
	place_of_worship: '⛪',
	// Transport
	bus_stop: '🚏',
	bus_station: '🚏',
	station: '🚉',
	halt: '🚉',
	tram_stop: '🚊',
	crossing: '🚸',
	traffic_signals: '🚦',
	taxi: '🚕',
	bicycle_parking: '🚲',
	bicycle_rental: '🚲',
	parking: '🅿️',
	fuel: '⛽',
	charging_station: '🔌',
	car_wash: '🚿',
	// Parks & nature
	park: '🌳',
	playground: '🛝',
	garden: '🌳',
	nature_reserve: '🌿',
	tree: '🌲',
	water: '💧',
	// Food & drink
	restaurant: '🍽️',
	cafe: '☕',
	fast_food: '🍔',
	pub: '🍺',
	bar: '🍺',
	ice_cream: '🍦',
	// Shops
	supermarket: '🛒',
	convenience: '🏪',
	bakery: '🥖',
	butcher: '🥩',
	greengrocer: '🥬',
	clothes: '👔',
	hairdresser: '✂️',
	beauty: '💅',
	books: '📖',
	hardware: '🔧',
	bicycle: '🚲',
	car: '🚗',
	car_repair: '🔧',
	chemist: '🧪',
	electronics: '📱',
	florist: '💐',
	laundry: '🧺',
	optician: '👓',
	pet: '🐾',
	shoes: '👟',
	stationery: '✏️',
	toys: '🧸',
	// Accommodation
	hotel: '🏨',
	guest_house: '🏨',
	hostel: '🏨',
	camp_site: '⛺',
	// Services
	bank: '🏦',
	atm: '💳',
	post_office: '📮',
	library: '📚',
	police: '🚔',
	fire_station: '🚒',
	townhall: '🏛️',
	courthouse: '🏛️',
	community_centre: '🏘️',
	// Leisure & culture
	theatre: '🎭',
	cinema: '🎬',
	museum: '🏛️',
	arts_centre: '🎨',
	nightclub: '🎶',
	// Sports
	swimming_pool: '🏊',
	sports_centre: '🏟️',
	pitch: '⚽',
	stadium: '🏟️',
	fitness_centre: '🏋️',
	golf_course: '⛳',
	// Tourism
	viewpoint: '🔭',
	information: 'ℹ️',
	attraction: '⭐',
	picnic_site: '🧺',
	// Other
	toilets: '🚻',
	shelter: '🛖',
	cemetery: '⚰️',
	recycling: '♻️',
	waste_basket: '🗑️',
	drinking_water: '🚰',
	fountain: '⛲',
	entrance: '↪',
	clock: '🕐',
	telephone: '📞',
	vending_machine: '🎰',
	marketplace: '🛍️'
};

// Highway values that are roads (labelled by street renderer); everything else is a POI.
const ROAD_HIGHWAY_TYPES = new Set([
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
	'track',
	'bridleway',
	'steps',
	'construction',
	'proposed',
	'road'
]);

const ROAD_STYLES: Record<string, RoadStyle> = {
	motorway: { width: 8, color: '#e892a2', casing: 10 },
	motorway_link: { width: 6, color: '#e892a2', casing: 8 },
	trunk: { width: 7, color: '#f9b29c', casing: 9 },
	trunk_link: { width: 5, color: '#f9b29c', casing: 7 },
	primary: { width: 6, color: '#fcd6a4', casing: 8 },
	primary_link: { width: 4, color: '#fcd6a4', casing: 6 },
	secondary: { width: 5, color: '#f7fabf', casing: 7 },
	secondary_link: { width: 4, color: '#f7fabf', casing: 6 },
	tertiary: { width: 4, color: '#ffffff', casing: 6 },
	tertiary_link: { width: 3, color: '#ffffff', casing: 5 },
	residential: { width: 3, color: '#ffffff', casing: 5 },
	unclassified: { width: 3, color: '#ffffff', casing: 5 },
	living_street: { width: 3, color: '#ededed', casing: 5 },
	service: { width: 2, color: '#ffffff', casing: 3 },
	pedestrian: { width: 3, color: '#dddde8', casing: 5 },
	footway: { width: 1, color: '#fa8072', casing: 0, dash: '4,2' },
	cycleway: { width: 1, color: '#0000ff', casing: 0, dash: '4,2' },
	path: { width: 1, color: '#fa8072', casing: 0, dash: '2,2' },
	track: { width: 2, color: '#996600', casing: 0, dash: '6,3' }
};

const LABEL_FONT_SIZES: Record<string, number> = {
	motorway: 28,
	motorway_link: 22,
	trunk: 28,
	trunk_link: 22,
	primary: 26,
	primary_link: 20,
	secondary: 24,
	secondary_link: 20,
	tertiary: 22,
	residential: 18,
	unclassified: 18
};

const LANDUSE_COLORS: Record<string, string> = {
	grass: '#cdebb0',
	forest: '#add19e',
	wood: '#add19e',
	park: '#c8facc',
	garden: '#cdebb0',
	meadow: '#cdebb0',
	recreation_ground: '#c8facc',
	village_green: '#c8facc',
	playground: '#c8facc',
	pitch: '#aae0cb',
	golf_course: '#b5e3b5',
	nature_reserve: '#c8facc',
	common: '#c8facc',
	scrub: '#c8d7ab',
	heath: '#d6d99f',
	grassland: '#cdebb0',
	farmland: '#eef0d5',
	farmyard: '#f5dcba',
	orchard: '#aedfa3',
	vineyard: '#b3e2a8',
	allotments: '#c9e1bf',
	cemetery: '#aacbaf',
	wetland: '#b5d0d0'
};

/** Drawing order for roads: minor first, major on top. */
export const ROAD_ORDER = [
	'footway',
	'path',
	'cycleway',
	'track',
	'service',
	'pedestrian',
	'living_street',
	'unclassified',
	'residential',
	'tertiary_link',
	'tertiary',
	'secondary_link',
	'secondary',
	'primary_link',
	'primary',
	'trunk_link',
	'trunk',
	'motorway_link',
	'motorway'
];

/** Road types that receive street-name labels. */
export const LABEL_ROADS = [
	'motorway',
	'trunk',
	'primary',
	'secondary',
	'tertiary',
	'residential',
	'unclassified'
];

// ── Pure functions ───────────────────────────────────────────────────────────

export function escapeXml(str: string): string {
	return str
		.replace(/&/g, '&amp;')
		.replace(/</g, '&lt;')
		.replace(/>/g, '&gt;')
		.replace(/"/g, '&quot;')
		.replace(/'/g, '&#39;');
}

export function getRoadStyle(highway: string): RoadStyle {
	return ROAD_STYLES[highway] || { width: 2, color: '#cccccc', casing: 3 };
}

export function getLabelFontSize(highway: string): number {
	return LABEL_FONT_SIZES[highway] || 16;
}

export function getLanduseColor(type: string): string {
	return LANDUSE_COLORS[type] || '#e0e0e0';
}

export function getPoiIcon(tags: Record<string, string>): string {
	const tagVal =
		tags.amenity ||
		tags.shop ||
		tags.leisure ||
		tags.natural ||
		tags.highway ||
		tags.railway ||
		(tags.entrance ? 'entrance' : '') ||
		tags.tourism ||
		tags.sport ||
		'';
	return POI_ICON_MAP[tagVal] || '\u25cf';
}

// Vector POI pictographs. Emoji glyphs in SVG <text> require a colour-emoji
// font, which the Typst/resvg PDF renderer does not support (they render as
// tofu). We therefore draw a small vector pictograph per category, picked from
// the POI's emoji. Paths are authored in a 24\u00d724 grid and scaled at render time.
const POI_MARKER_COLORS: Record<string, string> = {
	water: '#2563eb',
	tree: '#15803d',
	worship: '#7c3aed',
	school: '#b45309',
	medical: '#dc2626',
	food: '#ea580c',
	shop: '#0891b2',
	transport: '#475569',
	dot: '#734a08'
};

const POI_GLYPHS: Record<string, string> = {
	water: '<path d="M12 3.5C12 3.5 5.5 11 5.5 15.2a6.5 6.5 0 0 0 13 0C18.5 11 12 3.5 12 3.5Z"/>',
	tree: '<circle cx="12" cy="9.5" r="6.5"/><rect x="10.8" y="14" width="2.4" height="7"/>',
	worship: '<path d="M6 21V11l6-4 6 4v10Z"/><path d="M11 2h2v2.2h2.2v2H13V21h-2V8.2H8.8v-2H11Z"/>',
	school:
		'<path d="M12 4 23 9 12 14 1 9Z"/><path d="M5 11.2V16c0 1.7 3.1 3 7 3s7-1.3 7-3v-4.8l-7 3.2Z"/>',
	medical: '<path d="M9.6 3h4.8v6.6H21v4.8h-6.6V21H9.6v-6.6H3V9.6h6.6Z"/>',
	food: '<path d="M5 8h11v3.5a5.5 5.5 0 0 1-11 0Z"/><path d="M16 9h2.2a2.3 2.3 0 0 1 0 4.6H16" fill="none" stroke-width="1.6"/><rect x="5" y="19.4" width="11" height="1.8"/>',
	shop: '<path d="M5.5 8h13l-1.1 13H6.6Z"/><path d="M9 8V6.6a3 3 0 0 1 6 0V8h-1.8V6.6a1.2 1.2 0 0 0-2.4 0V8Z"/>',
	transport:
		'<rect x="4.5" y="4.5" width="15" height="12.5" rx="2"/><rect x="6.5" y="7" width="11" height="4.2" fill="#ffffff"/><circle cx="8.5" cy="18.4" r="1.7"/><circle cx="15.5" cy="18.4" r="1.7"/>',
	dot: '<path d="M12 2.5c-3.9 0-7 3-7 6.8 0 4.9 7 12.2 7 12.2s7-7.3 7-12.2c0-3.8-3.1-6.8-7-6.8Z"/><circle cx="12" cy="9.2" r="2.6" fill="#ffffff"/>'
};

export function poiGlyphKind(icon: string): string {
	if (icon === '\ud83d\udca7') return 'water';
	if (['\ud83c\udf33', '\ud83c\udf32', '\ud83c\udf3f', '\ud83d\udedd'].includes(icon))
		return 'tree';
	if (icon === '\u26ea') return 'worship';
	if (['\ud83c\udfeb', '\ud83d\udc92', '\ud83c\udf93'].includes(icon)) return 'school';
	if (['\ud83c\udfe5', '\ud83e\uddb7', '\ud83d\udc8a', '\ud83d\udc3e'].includes(icon))
		return 'medical';
	if (
		['\ud83c\udf7d\ufe0f', '\u2615', '\ud83c\udf54', '\ud83c\udf7a', '\ud83c\udf66'].includes(icon)
	)
		return 'food';
	if (
		[
			'\ud83d\ude8f',
			'\ud83d\udeb8',
			'\ud83d\udea6',
			'\ud83d\ude89',
			'\ud83d\ude8a',
			'\ud83d\ude95',
			'\ud83d\udeb2',
			'\ud83c\udd7f\ufe0f',
			'\u26fd',
			'\ud83d\udd0c',
			'\ud83d\udebf'
		].includes(icon)
	)
		return 'transport';
	if (
		[
			'\ud83d\uded2',
			'\ud83c\udfea',
			'\ud83e\udd56',
			'\ud83e\udd69',
			'\ud83e\udd6c',
			'\ud83d\udc54',
			'\u2702\ufe0f',
			'\ud83d\udc85',
			'\ud83d\udcd6',
			'\ud83d\udd27',
			'\ud83d\ude97',
			'\ud83e\uddea',
			'\ud83d\udcf1',
			'\ud83d\udc90',
			'\ud83e\uddfa',
			'\ud83d\udc53',
			'\ud83d\udc5f',
			'\u270f\ufe0f',
			'\ud83e\uddf8'
		].includes(icon)
	)
		return 'shop';
	return 'dot';
}

// poiMarkerSvg returns a vector pictograph marker centred at (x, y), with a
// white halo (paint-order stroke) so it stays legible over the map tiles.
export function poiMarkerSvg(x: number, y: number, icon: string, size = 18): string {
	const kind = poiGlyphKind(icon);
	const color = POI_MARKER_COLORS[kind] ?? POI_MARKER_COLORS.dot;
	const glyph = POI_GLYPHS[kind] ?? POI_GLYPHS.dot;
	const s = size / 24;
	const tx = (x - size / 2).toFixed(2);
	const ty = (y - size / 2).toFixed(2);
	return `<g class="poi-marker" transform="translate(${tx} ${ty}) scale(${s.toFixed(3)})" fill="${color}" stroke="#ffffff" stroke-width="2.4" stroke-linejoin="round" paint-order="stroke">${glyph}</g>`;
}

export function getPathMidpoint(
	nodes: LatLon[],
	latToY: (lat: number) => number,
	lngToX: (lng: number) => number
): PathMidpoint | null {
	if (nodes.length < 2) return null;

	let totalLength = 0;
	const segments: Array<{ x1: number; y1: number; x2: number; y2: number; length: number }> = [];

	for (let i = 0; i < nodes.length - 1; i++) {
		const x1 = lngToX(nodes[i].lon);
		const y1 = latToY(nodes[i].lat);
		const x2 = lngToX(nodes[i + 1].lon);
		const y2 = latToY(nodes[i + 1].lat);
		const length = Math.sqrt((x2 - x1) ** 2 + (y2 - y1) ** 2);
		segments.push({ x1, y1, x2, y2, length });
		totalLength += length;
	}

	const midLength = totalLength / 2;
	let runningLength = 0;

	for (const seg of segments) {
		if (runningLength + seg.length >= midLength) {
			const t = (midLength - runningLength) / seg.length;
			const x = seg.x1 + t * (seg.x2 - seg.x1);
			const y = seg.y1 + t * (seg.y2 - seg.y1);
			let angle = (Math.atan2(seg.y2 - seg.y1, seg.x2 - seg.x1) * 180) / Math.PI;
			if (angle > 90) angle -= 180;
			if (angle < -90) angle += 180;
			return { x, y, angle };
		}
		runningLength += seg.length;
	}

	return null;
}

// ── SVG geometry helpers ─────────────────────────────────────────────────────

export function toPolygonPath(
	nodes: LatLon[],
	lngToX: (lng: number) => number,
	latToY: (lat: number) => number
): string {
	return (
		nodes
			.map(
				(n, i) => `${i === 0 ? 'M' : 'L'} ${lngToX(n.lon).toFixed(1)} ${latToY(n.lat).toFixed(1)}`
			)
			.join(' ') + ' Z'
	);
}

export function toLinePath(
	nodes: LatLon[],
	lngToX: (lng: number) => number,
	latToY: (lat: number) => number
): string {
	return nodes
		.map((n, i) => `${i === 0 ? 'M' : 'L'} ${lngToX(n.lon).toFixed(1)} ${latToY(n.lat).toFixed(1)}`)
		.join(' ');
}

// ── Coordinate conversion ────────────────────────────────────────────────────

export interface CoordConverters {
	latToY: (lat: number) => number;
	lngToX: (lng: number) => number;
	metersPerPixel: number;
}

export function makeCoordConverters(
	bboxNELat: number,
	bboxNELng: number,
	bboxSWLat: number,
	bboxSWLng: number,
	svgWidth: number,
	svgHeight: number
): CoordConverters {
	const latRange = bboxNELat - bboxSWLat;
	const lngRange = bboxNELng - bboxSWLng;
	const centerLat = (bboxNELat + bboxSWLat) / 2;
	const metersPerDegreeLng = 111320 * Math.cos((centerLat * Math.PI) / 180);
	const mapWidthMeters = lngRange * metersPerDegreeLng;
	const metersPerPixel = mapWidthMeters / svgWidth;

	return {
		latToY: (lat: number) => ((bboxNELat - lat) / latRange) * svgHeight,
		lngToX: (lng: number) => ((lng - bboxSWLng) / lngRange) * svgWidth,
		metersPerPixel
	};
}

// ── Overpass queries ─────────────────────────────────────────────────────────

export function buildOverpassQueries(bbox: string): {
	essentialQuery: string;
	enrichmentQuery: string;
} {
	const essentialQuery = `
				[out:json][timeout:30];
				(
					way["building"](${bbox});
					way["building:part"](${bbox});
					way["highway"~"^(motorway|motorway_link|trunk|trunk_link|primary|primary_link|secondary|secondary_link|tertiary|tertiary_link|residential|unclassified|living_street|service|pedestrian|footway|cycleway|path|track)$"](${bbox});
					way["amenity"="school"](${bbox});
					way["leisure"~"^(park|garden|playground|pitch)$"](${bbox});
					way["natural"="water"](${bbox});
					way["water"](${bbox});
					way["railway"~"^(rail|light_rail|tram)$"](${bbox});
					node["place"](${bbox});
					node["addr:housenumber"](${bbox});
					way["addr:housenumber"](${bbox});
					node["highway"="bus_stop"](${bbox});
				);
				out body;
				>;
				out skel qt;
			`;

	const enrichmentQuery = `
				[out:json][timeout:30];
				(
					way["landuse"](${bbox});
					way["leisure"](${bbox});
					way["natural"~"^(wood|scrub|water|wetland|grassland|heath)$"](${bbox});
					way["water"](${bbox});
					way["waterway"~"^(river|stream|canal)$"](${bbox});
					way["railway"~"^(rail|light_rail|tram)$"](${bbox});
					way["boundary"~"^(administrative|protected_area|national_park)$"](${bbox});
					node["amenity"](${bbox});
					way["amenity"](${bbox});
					node["shop"](${bbox});
					way["shop"](${bbox});
					node["tourism"](${bbox});
					way["tourism"](${bbox});
					node["leisure"](${bbox});
					node["natural"~"^(tree|water|spring|peak)$"](${bbox});
					node["highway"~"^(crossing|traffic_signals)$"](${bbox});
					node["entrance"](${bbox});
					node["barrier"](${bbox});
					nwr["name"](${bbox});
				);
				out body;
				>;
				out skel qt;
			`;

	return { essentialQuery, enrichmentQuery };
}

// ── Mirror endpoint ordering ─────────────────────────────────────────────────

export function orderedEndpoints(
	selectedMirror: string,
	mirrors: OverpassMirror[] = OVERPASS_MIRRORS
): Array<{ id: string; url: string }> {
	const selected = selectedMirror ? mirrors.find((m) => m.id === selectedMirror) : null;
	const rest = mirrors.filter((m) => m.id !== selectedMirror);
	// Keep declaration order as priority — no shuffle.
	const ordered = selected ? [selected, ...rest] : rest;
	return ordered.map((m) => ({ id: m.id, url: m.url }));
}

// ── Element processing ───────────────────────────────────────────────────────

export function buildNodeLookup(elements: Array<Record<string, unknown>>): Record<number, LatLon> {
	const nodes: Record<number, LatLon> = {};
	for (const element of elements) {
		if (element.type === 'node') {
			nodes[element.id as number] = {
				lat: element.lat as number,
				lon: element.lon as number
			};
		}
	}
	return nodes;
}

export function categoriseElements(
	allElements: Array<Record<string, unknown>>,
	nodes: Record<number, LatLon>
): CategorisedElements {
	const buildings: Building[] = [];
	const landuse: LanduseArea[] = [];
	const water: WaterFeature[] = [];
	const roads: Road[] = [];
	const railways: Railway[] = [];
	const boundaries: Boundary[] = [];
	const poiLabels: PoiLabel[] = [];
	const seenPoi = new Set<string>();

	function isRoadFeature(tags: Record<string, string>): boolean {
		return !!tags.highway && ROAD_HIGHWAY_TYPES.has(tags.highway);
	}

	function centroid(coords: LatLon[]): LatLon | null {
		if (coords.length === 0) return null;
		return {
			lat: coords.reduce((s, n) => s + n.lat, 0) / coords.length,
			lon: coords.reduce((s, n) => s + n.lon, 0) / coords.length
		};
	}

	function poiCategory(tags: Record<string, string>): PoiCategory {
		if (tags.place) return 'place';
		if (tags['addr:housenumber'] && !tags.amenity && !tags.shop) return 'address';
		return 'landmark';
	}

	function isPointOfInterest(tags: Record<string, string>): boolean {
		return !!(
			tags.place ||
			tags['addr:housenumber'] ||
			tags.amenity ||
			tags.shop ||
			tags.tourism ||
			tags.leisure ||
			tags.natural ||
			tags.railway ||
			tags.entrance ||
			tags.barrier ||
			tags.highway === 'bus_stop' ||
			tags.highway === 'crossing' ||
			tags.highway === 'traffic_signals'
		);
	}

	function addPoi(
		name: string,
		lat: number,
		lon: number,
		category: PoiCategory,
		icon: string,
		allowUnnamed = false
	) {
		const label = name.trim();
		if (!label && !allowUnnamed) return;
		const key = `${category}|${label}|${lat.toFixed(5)}|${lon.toFixed(5)}|${icon}`;
		if (seenPoi.has(key)) return;
		seenPoi.add(key);
		poiLabels.push({ name: label, lat, lon, category, icon });
	}

	for (const element of allElements) {
		const tags = (element.tags || {}) as Record<string, string>;

		// --- Collect all named features as labels ---
		const name = tags.name || tags['addr:housenumber'];
		if (name) {
			if (isRoadFeature(tags)) {
				// Roads are labelled by the street name renderer — skip
			} else {
				let lat: number | undefined;
				let lon: number | undefined;
				if (element.type === 'node') {
					lat = element.lat as number;
					lon = element.lon as number;
				} else if (element.type === 'way' && element.nodes) {
					const wayCoords = (element.nodes as number[])
						.map((id: number) => nodes[id])
						.filter((n): n is LatLon => !!n);
					if (wayCoords.length > 0) {
						lat = wayCoords.reduce((s, n) => s + n.lat, 0) / wayCoords.length;
						lon = wayCoords.reduce((s, n) => s + n.lon, 0) / wayCoords.length;
					}
				}

				if (lat !== undefined && lon !== undefined) {
					addPoi(name, lat, lon, poiCategory(tags), getPoiIcon(tags));
				}
			}
		}

		if (element.type === 'node' && isPointOfInterest(tags)) {
			const lat = element.lat as number | undefined;
			const lon = element.lon as number | undefined;
			if (lat !== undefined && lon !== undefined) {
				const label = tags.name || tags['addr:housenumber'] || '';
				addPoi(label, lat, lon, poiCategory(tags), getPoiIcon(tags), true);
			}
		}

		// --- Categorise ways for geometry rendering ---
		if (element.type !== 'way') continue;

		const wayNodes = (element.nodes as number[])
			?.map((id: number) => nodes[id])
			.filter((n: LatLon | undefined) => n);

		if (!wayNodes || wayNodes.length < 2) continue;

		if (tags.boundary) {
			boundaries.push({ type: tags.boundary, name: tags.name, nodes: wayNodes });
		}

		if (tags.railway) {
			railways.push({ type: tags.railway, nodes: wayNodes });
		} else if (tags['building:part']) {
			buildings.push({ nodes: wayNodes, part: true });
		} else if (tags.building) {
			buildings.push({ nodes: wayNodes, part: false });
		} else if (tags.natural === 'water' || tags.water) {
			water.push({ isLine: false, nodes: wayNodes });
		} else if (tags.waterway) {
			water.push({ isLine: true, nodes: wayNodes });
		} else if (tags.landuse || tags.leisure || tags.natural) {
			const type = tags.landuse || tags.leisure || tags.natural;
			landuse.push({ type, nodes: wayNodes });
		} else if (tags.highway) {
			roads.push({
				highway: tags.highway,
				name: tags.name,
				bridge: tags.bridge === 'yes',
				tunnel: tags.tunnel === 'yes',
				nodes: wayNodes
			});
		}

		if (!name && isPointOfInterest(tags)) {
			const c = centroid(wayNodes);
			if (c) addPoi('', c.lat, c.lon, poiCategory(tags), getPoiIcon(tags), true);
		}
	}

	return { buildings, landuse, water, roads, railways, boundaries, poiLabels };
}

// ── SVG rendering ────────────────────────────────────────────────────────────

export function generateMapSvg(params: SvgParams): string {
	const svgWidth = 1200;
	const svgHeight = 800;

	const { latToY, lngToX, metersPerPixel } = makeCoordConverters(
		params.bboxNELat,
		params.bboxNELng,
		params.bboxSWLat,
		params.bboxSWLng,
		svgWidth,
		svgHeight
	);

	const compact = metersPerPixel > 1.2;
	const medium = metersPerPixel > 0.65 && metersPerPixel <= 1.2;
	const strokeScale = compact ? 0.78 : medium ? 0.92 : 1.12;
	const labelScale = compact ? 0.72 : medium ? 0.88 : 1;
	const showAddressLabels = !compact;
	const showUnnamedPoiMarkers = !compact;
	const scaleStroke = (width: number) => Math.max(width * strokeScale, width > 0 ? 0.8 : 0);
	const scaleFont = (size: number) => Math.max(size * labelScale, 8);

	// Landuse polygons
	let landusePaths = '';
	for (const area of params.landuse) {
		const color = getLanduseColor(area.type);
		landusePaths += `<path d="${toPolygonPath(area.nodes, lngToX, latToY)}" fill="${color}" stroke="none"/>`;
	}

	// Water
	let waterPaths = '';
	for (const w of params.water) {
		if (w.isLine) {
			waterPaths += `<path d="${toLinePath(w.nodes, lngToX, latToY)}" stroke="#aad3df" stroke-width="4" fill="none"/>`;
		} else {
			waterPaths += `<path d="${toPolygonPath(w.nodes, lngToX, latToY)}" fill="#aad3df" stroke="#6699cc" stroke-width="1"/>`;
		}
	}

	// Buildings
	let buildingPaths = '';
	for (const bldg of params.buildings) {
		const fill = bldg.part ? '#cfc5bd' : '#d9d0c9';
		const stroke = bldg.part ? '#aaa19a' : '#bbb5b0';
		buildingPaths += `<path d="${toPolygonPath(bldg.nodes, lngToX, latToY)}" fill="${fill}" stroke="${stroke}" stroke-width="${scaleStroke(0.6).toFixed(2)}"/>`;
	}

	// Sort roads
	const sortedRoads = [...params.roads].sort(
		(a, b) => ROAD_ORDER.indexOf(a.highway) - ROAD_ORDER.indexOf(b.highway)
	);

	// Road paths
	let roadPaths = '';
	for (const way of sortedRoads) {
		const style = getRoadStyle(way.highway);
		const pathData = toLinePath(way.nodes, lngToX, latToY);

		if (style.casing > 0) {
			const casingColor = way.bridge ? '#000000' : '#666666';
			roadPaths += `<path d="${pathData}" stroke="${casingColor}" stroke-width="${scaleStroke(style.casing).toFixed(2)}" fill="none" stroke-linecap="round" stroke-linejoin="round"/>`;
		}
		let roadAttrs = `stroke="${style.color}" stroke-width="${scaleStroke(style.width).toFixed(2)}" fill="none" stroke-linecap="round" stroke-linejoin="round"`;
		if (style.dash) {
			roadAttrs += ` stroke-dasharray="${style.dash}"`;
		}
		if (way.tunnel) {
			roadAttrs += ` opacity="0.5"`;
		}
		roadPaths += `<path d="${pathData}" ${roadAttrs}/>`;
	}

	let boundaryPaths = '';
	for (const boundary of params.boundaries) {
		const pathData = toLinePath(boundary.nodes, lngToX, latToY);
		boundaryPaths += `<path d="${pathData}" stroke="#9ca3af" stroke-width="${scaleStroke(1.2).toFixed(2)}" fill="none" stroke-dasharray="6,4" stroke-linecap="round"/>`;
	}

	// Street name labels
	const labelledNames = new Set<string>();
	let labels = '';
	for (const way of sortedRoads) {
		if (!way.name || labelledNames.has(way.name)) continue;
		if (!LABEL_ROADS.includes(way.highway)) continue;
		if (compact && ['residential', 'unclassified'].includes(way.highway)) continue;

		const midpoint = getPathMidpoint(way.nodes, latToY, lngToX);
		if (!midpoint) continue;
		if (
			midpoint.x < 10 ||
			midpoint.x > svgWidth - 10 ||
			midpoint.y < 10 ||
			midpoint.y > svgHeight - 10
		)
			continue;

		const fontSize = scaleFont(getLabelFontSize(way.highway));
		labelledNames.add(way.name);
		labels += `<text x="${midpoint.x.toFixed(1)}" y="${midpoint.y.toFixed(1)}" font-family="Arial, sans-serif" font-size="${fontSize.toFixed(1)}" fill="#333333" text-anchor="middle" dominant-baseline="middle" transform="rotate(${midpoint.angle.toFixed(1)}, ${midpoint.x.toFixed(1)}, ${midpoint.y.toFixed(1)})" stroke="white" stroke-width="${Math.max(2.5, 4 * labelScale).toFixed(1)}" paint-order="stroke">${escapeXml(way.name)}</text>`;
	}

	// Railway paths
	let railwayPaths = '';
	for (const rail of params.railways) {
		const pathData = toLinePath(rail.nodes, lngToX, latToY);
		railwayPaths += `<path d="${pathData}" stroke="#999999" stroke-width="${scaleStroke(4).toFixed(2)}" fill="none" stroke-linecap="butt"/>`;
		railwayPaths += `<path d="${pathData}" stroke="#ffffff" stroke-width="${scaleStroke(2).toFixed(2)}" fill="none" stroke-linecap="butt" stroke-dasharray="8,8"/>`;
	}

	// POI/place labels with collision avoidance
	const placedBoxes: Array<{ x1: number; y1: number; x2: number; y2: number }> = [];

	function overlapsExisting(x1: number, y1: number, x2: number, y2: number): boolean {
		for (const box of placedBoxes) {
			if (x1 < box.x2 && x2 > box.x1 && y1 < box.y2 && y2 > box.y1) return true;
		}
		return false;
	}

	function tryPlace(
		cx: number,
		cy: number,
		width: number,
		height: number,
		anchor: 'middle' | 'start' = 'middle'
	): { x: number; y: number } | null {
		const halfH = height / 2;
		const halfW = anchor === 'middle' ? width / 2 : 0;
		// Try original position, then offsets
		const offsets = [
			[0, 0],
			[0, -height],
			[0, height],
			[width * 0.6, 0],
			[-width * 0.6, 0],
			[width * 0.4, -height],
			[width * 0.4, height]
		];
		for (const [dx, dy] of offsets) {
			const px = cx + dx;
			const py = cy + dy;
			const x1 = px - halfW;
			const y1 = py - halfH;
			const x2 = x1 + width;
			const y2 = y1 + height;
			if (x1 < 5 || x2 > svgWidth - 5 || y1 < 5 || y2 > svgHeight - 5) continue;
			if (!overlapsExisting(x1, y1, x2, y2)) {
				placedBoxes.push({ x1, y1, x2, y2 });
				return { x: px, y: py };
			}
		}
		return null; // cannot place without overlap — skip this label
	}

	let poiLabelPaths = '';
	for (const poi of params.poiLabels) {
		const hasName = poi.name.trim().length > 0;
		if (poi.category === 'address' && (!hasName || !showAddressLabels)) continue;
		if (!hasName && !showUnnamedPoiMarkers) continue;

		const x = lngToX(poi.lon);
		const y = latToY(poi.lat);

		if (x < 20 || x > svgWidth - 20 || y < 20 || y > svgHeight - 20) continue;

		if (poi.category === 'place') {
			if (!hasName) continue;
			const fontSize = scaleFont(28);
			const estWidth = poi.name.length * fontSize * 0.55;
			const pos = tryPlace(x, y, estWidth, fontSize * 1.2);
			if (!pos) continue;
			poiLabelPaths += `<text x="${pos.x.toFixed(1)}" y="${pos.y.toFixed(1)}" font-family="Arial, sans-serif" font-size="${fontSize.toFixed(1)}" font-style="italic" fill="#444444" text-anchor="middle" dominant-baseline="middle" stroke="white" stroke-width="${Math.max(2.5, 4 * labelScale).toFixed(1)}" paint-order="stroke">${escapeXml(poi.name)}</text>`;
		} else if (poi.category === 'address') {
			const fontSize = scaleFont(12);
			const estWidth = poi.name.length * fontSize * 0.55;
			const pos = tryPlace(x, y, estWidth, fontSize * 1.2);
			if (!pos) continue;
			poiLabelPaths += `<text x="${pos.x.toFixed(1)}" y="${pos.y.toFixed(1)}" font-family="Arial, sans-serif" font-size="${fontSize.toFixed(1)}" fill="#666666" text-anchor="middle" dominant-baseline="middle" stroke="white" stroke-width="1.5" paint-order="stroke">${escapeXml(poi.name)}</text>`;
		} else {
			// POI marker: a vector pictograph chosen by category, not an emoji
			// glyph. Emoji in SVG <text> rely on a colour-emoji font, which the
			// Typst/resvg PDF renderer does not support (they render as tofu), so
			// we draw shapes that render identically in the browser, rsvg, and
			// Typst.
			const fontSize = scaleFont(20);
			const markerSize = Math.max(12, 18 * labelScale);
			const estWidth = markerSize + (hasName ? 4 + poi.name.length * fontSize * 0.55 : 0);
			const pos = tryPlace(x, y, estWidth, fontSize * 1.2, 'start');
			if (!pos) continue;
			poiLabelPaths += poiMarkerSvg(pos.x, pos.y, poi.icon, markerSize);
			if (hasName) {
				poiLabelPaths += `<text x="${(pos.x + markerSize / 2 + 3).toFixed(1)}" y="${pos.y.toFixed(1)}" font-family="Arial, sans-serif" font-size="${fontSize.toFixed(1)}" fill="#374151" dominant-baseline="middle" stroke="white" stroke-width="${Math.max(2, 3 * labelScale).toFixed(1)}" paint-order="stroke">${escapeXml(poi.name)}</text>`;
			}
		}
	}

	// Radar position and FOV triangle
	const radarX = params.latitude !== null ? lngToX(params.longitude!) : svgWidth / 2;
	const radarY = params.latitude !== null ? latToY(params.latitude!) : svgHeight / 2;

	const triangleLengthMeters = 100;
	const triangleLengthPixels = triangleLengthMeters / metersPerPixel;

	const angle = params.radarAngle || 0;
	const fovWidthDegrees = 20;
	const leftAngleRad = ((angle - fovWidthDegrees / 2) * Math.PI) / 180;
	const rightAngleRad = ((angle + fovWidthDegrees / 2) * Math.PI) / 180;

	const leftX = radarX + Math.sin(leftAngleRad) * triangleLengthPixels;
	const leftY = radarY - Math.cos(leftAngleRad) * triangleLengthPixels;
	const rightX = radarX + Math.sin(rightAngleRad) * triangleLengthPixels;
	const rightY = radarY - Math.cos(rightAngleRad) * triangleLengthPixels;

	const markerRadius = Math.max(8, Math.min(20, 40 / metersPerPixel));

	return `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="${svgWidth}" height="${svgHeight}" viewBox="0 0 ${svgWidth} ${svgHeight}">
	<title>Map Export - velocity.report</title>
	<desc>Data © OpenStreetMap contributors</desc>
	<!-- Background -->
	<rect x="0" y="0" width="${svgWidth}" height="${svgHeight}" fill="#f2efe9"/>
	<!-- Landuse (parks, forests, etc.) -->
	${landusePaths}
	<!-- Water -->
	${waterPaths}
	<!-- Buildings -->
	${buildingPaths}
	<!-- Roads -->
	${roadPaths}
	<!-- Boundaries -->
	${boundaryPaths}
	<!-- Railways -->
	${railwayPaths}
	<!-- Street names -->
	${labels}
	<!-- Place names and POIs -->
	${poiLabelPaths}
	<!-- Radar FOV triangle -->
	<polygon points="${radarX.toFixed(1)},${radarY.toFixed(1)} ${leftX.toFixed(1)},${leftY.toFixed(1)} ${rightX.toFixed(1)},${rightY.toFixed(1)}" fill="#ef4444" fill-opacity="0.4" stroke="#ef4444" stroke-width="1"/>
	<!-- Radar position marker -->
	<circle cx="${radarX.toFixed(1)}" cy="${radarY.toFixed(1)}" r="${markerRadius.toFixed(1)}" fill="#3b82f6" stroke="white" stroke-width="2"/>
	<!-- OSM attribution -->
	<rect x="${svgWidth - 270}" y="${svgHeight - 22}" width="264" height="16" fill="white" fill-opacity="0.75" rx="2"/>
	<text x="${svgWidth - 8}" y="${svgHeight - 8}" font-family="Arial, sans-serif" font-size="11" fill="#333333" text-anchor="end">© OpenStreetMap contributors</text>
</svg>`;
}

// ── Base64 encoding ──────────────────────────────────────────────────────────

export function svgToBase64(svgText: string): string {
	const encoder = new TextEncoder();
	const bytes = encoder.encode(svgText);

	const chunkSize = 8192;
	let binaryString = '';
	for (let i = 0; i < bytes.length; i += chunkSize) {
		const chunk = bytes.slice(i, i + chunkSize);
		binaryString += String.fromCharCode(...chunk);
	}
	return btoa(binaryString);
}
