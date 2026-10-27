/**
 * The site viewer's client for Gerardus's HTTP API (Gerardus
 * docs/plans/VECTOR-MAP.md §4.1). It only reads: editing a site's street use
 * happens in the macOS app.
 *
 * Gerardus serves it beside `GET /` on its gRPC port, so the browser talks to
 * it directly, not through velocity.report's server. Gerardus allows loopback
 * and tailnet origins.
 *
 * - `GET /api/v1/sites` lists the prepared sites:
 *   `{"epoch", "sites": [{"l16", "label", "lon", "lat", "height_m", "layers": […]}]}`,
 *   each with its site-frame origin (WGS 84 at `epoch`) and the GeoJSON
 *   layers it can serve.
 * - `GET /api/v1/sites/{l16}/geojson?layers=…` returns an RFC 7946
 *   FeatureCollection in WGS 84, height as a third coordinate; with
 *   `&frame=site`, positions are site-frame metres `[x east, y north, z up]`.
 *   The collection says which in its `frame` member (`"wgs84"` or `"site"`);
 *   an unknown layer or frame is a 400. Each feature's properties carry `layer`, `id`, `kind`, its tags, `author`
 *   and `edited_at`; a derived row's also carry `model_id` and `state`. A layer the site's bundle
 *   lacks is absent. The W4–W7 layers (MODEL_LAYERS) and the wall layers (WALL_LAYERS) are not
 *   among the defaults: the viewer asks for them only when the site lists them.
 * - `GET /api/v1/sites/{l16}/ground` returns the ground grid,
 *   `{"l16", "x0", "y0", "cell_m", "nx", "ny", "heights": […]}` in the site
 *   frame, (x0, y0) the south-west cell's centre, row-major with y outer and
 *   null where a cell is empty; 404 when the site has no grid.
 * - `GET /api/v1/sites/{l16}/scans?voxel_m=…` returns the scan points as
 *   little-endian float32 x, y, z triplets in the site frame, with the
 *   `X-Gerardus-Model-Id`, `X-Gerardus-Point-Count` and `X-Gerardus-Voxel-M`
 *   (the voxel used: Gerardus clamps it to 0.05–2 m) headers; 403 unless
 *   Gerardus serves scans to this peer.
 *
 * Its address is a setting kept in this browser, defaulting to the port
 * Gerardus serves on.
 */

export const GERARDUS_DEFAULT_ADDRESS = 'http://127.0.0.1:50061';
export const GERARDUS_ADDRESS_KEY = 'velocity.report.gerardusAddress';

/**
 * Gerardus's own layers, from public data and the scans, in the order the
 * viewer lists them.
 */
export const SITE_LAYERS = [
	'street_use',
	'footprints',
	'boxes',
	'wall_planes',
	'city_streets',
	'city_intersections',
	'kerbs',
	'observed'
] as const;

/**
 * Gerardus's W4–W7 layers, built in parallel and served only once built: the
 * site's building parts and their roof planes (W4), painted lane markings
 * (W5), poles and signs (W6) and trees (W7). Gerardus leaves them out of its
 * default layers, and refuses a name it does not know, so the viewer asks for
 * them only when `GET /api/v1/sites` lists them for the site.
 */
export const MODEL_LAYERS = [
	'building_parts',
	'roof_planes',
	'lane_markings',
	'street_furniture',
	'trees'
] as const;

/**
 * Walls as vertical polygons, for the 3D view (Gerardus
 * docs/plans/ROOFLINE-WALLS.md): each box wall (`box_walls`), its top on the
 * building's roofline where it has roof planes (tag `top=roofline`), else at
 * the box's height, and each building part's wall up to its roofline
 * (`wall_surfaces`), each inferred up to that height. Their vertices carry
 * their heights, so neither is raised further. Gerardus leaves them out of its defaults and an older one does
 * not know them, so the viewer asks for them only when the site lists them,
 * and only for the 3D view: seen from above, a wall is a line along its foot.
 */
export const WALL_LAYERS = ['box_walls', 'wall_surfaces'] as const;

/**
 * Layers taken from OpenStreetMap. The viewer asks for them only after this
 * visit's consent, as it does for OpenStreetMap tiles.
 */
export const OSM_LAYERS = ['osm', 'intersections'] as const;

export type SiteLayer =
	| (typeof SITE_LAYERS)[number]
	| (typeof MODEL_LAYERS)[number]
	| (typeof WALL_LAYERS)[number]
	| (typeof OSM_LAYERS)[number];

/**
 * Every layer the viewer knows how to draw, in the order it lists them, each
 * wall layer after the layer it replaces in 3D.
 */
export const KNOWN_LAYERS: readonly SiteLayer[] = [
	...SITE_LAYERS,
	...MODEL_LAYERS,
	...OSM_LAYERS
].flatMap((l): SiteLayer[] =>
	l === 'boxes' ? [l, 'box_walls'] : l === 'building_parts' ? [l, 'wall_surfaces'] : [l]
);

export function isOsmLayer(layer: string): boolean {
	return (OSM_LAYERS as readonly string[]).includes(layer);
}

export function isModelLayer(layer: string): boolean {
	return (MODEL_LAYERS as readonly string[]).includes(layer);
}

export function isWallLayer(layer: string): boolean {
	return (WALL_LAYERS as readonly string[]).includes(layer);
}

/**
 * The layers to ask for a site, given the ones Gerardus lists for it: those
 * the viewer knows in its own order, then any others it lists. With no list
 * (a site velocity.report located, or a Gerardus that lists none), every
 * layer the viewer knows except the W4–W7 and wall ones, which a Gerardus
 * without them would refuse.
 */
export function servedLayers(listed: readonly string[]): string[] {
	if (listed.length === 0) return KNOWN_LAYERS.filter((l) => !isModelLayer(l) && !isWallLayer(l));
	const known = KNOWN_LAYERS.filter((l) => listed.includes(l));
	const other = listed.filter((l) => !(KNOWN_LAYERS as readonly string[]).includes(l));
	return [...known, ...other];
}

/**
 * The served layers one view asks for and lists. The 2D map leaves out the
 * wall layers. The 3D view takes `box_walls` instead of `boxes` when the site
 * serves it, and an older Gerardus's boxes, which serve no walls, are raised
 * by their height instead; it takes `wall_surfaces` beside `building_parts`,
 * which are then drawn as outlines (scene3d).
 */
export function viewLayers(served: readonly string[], view: '2d' | '3d'): string[] {
	if (view === '2d') return served.filter((l) => !isWallLayer(l));
	const walls = served.includes('box_walls');
	return served.filter((l) => !(walls && l === 'boxes'));
}

/** The kinds of street use; operators mark them in the macOS app. */
export const STREET_USE_KINDS = ['driveway', 'parking_space', 'travel_lane', 'surface'] as const;
export type StreetUseKind = (typeof STREET_USE_KINDS)[number];

/** A `surface` feature's `surface` property: what the operator's ground area is. */
export const SURFACE_TYPES = ['carriageway', 'sidewalk', 'verge', 'apron', 'median'] as const;

/** An `observed` feature's kind: what the scans saw. */
export const OBSERVED_KINDS = ['facade', 'kerb', 'sensor', 'correction', 'pole'] as const;

/** A wall plane's `role`. */
export const WALL_ROLES = ['wall', 'popout', 'recess'] as const;

/** The kinds of the W4–W7 layers, as their features name them. */
export const MODEL_KINDS = [
	'building_part',
	'roof_plane',
	'wall_surface',
	'lane_marking',
	'pole',
	'sign',
	'tree'
] as const;

/** A GeoJSON position: longitude, latitude and, optionally, height. */
export type Position = [number, number] | [number, number, number];

/** A site-frame position: metres east, north and up from the site's origin. */
export type SitePosition = [number, number, number];

export type Geometry<P extends Position = Position> =
	| { type: 'Point'; coordinates: P }
	| { type: 'MultiPoint'; coordinates: P[] }
	| { type: 'LineString'; coordinates: P[] }
	| { type: 'MultiLineString'; coordinates: P[][] }
	| { type: 'Polygon'; coordinates: P[][] }
	| { type: 'MultiPolygon'; coordinates: P[][][] };

/** A feature as the viewer uses it: the reserved properties lifted out, the rest as tags. */
export interface SiteFeature<P extends Position = Position> {
	layer: string;
	id: string;
	kind: string;
	tags: Record<string, string>;
	author?: string;
	editedAt?: string;
	/** The derivation a derived row (a building part, a fitted tree) came from (`model_id`), and its state. */
	modelId?: string;
	state?: string;
	geometry: Geometry<P>;
}

/** A feature from `frame=site`: every position in site-frame metres, z included. */
export type SiteFrameFeature = SiteFeature<SitePosition>;

/** One prepared site, as `GET /api/v1/sites` lists it. */
export interface GerardusSite {
	l16: string;
	label: string;
	/** The site frame's origin, WGS 84, and its height in metres. */
	lon: number;
	lat: number;
	heightM: number;
	/** The GeoJSON layers this site can serve. */
	layers: string[];
}

/**
 * The site's ground grid. Cell (i, j) is centred x0 + i·cell_m east and
 * y0 + j·cell_m north, (x0, y0) being the south-west cell's centre; its height
 * is `heights[j·nx + i]`, null when the cell is empty.
 */
export interface GroundGrid {
	x0: number;
	y0: number;
	cellM: number;
	nx: number;
	ny: number;
	heights: (number | null)[];
}

/** The scan points of a site, thinned to one per voxel. */
export interface ScanCloud {
	/** x, y, z triplets in the site frame. */
	points: Float32Array;
	count: number;
	modelId: string;
	/** The voxel Gerardus used, which it may have clamped from the one asked for. */
	voxelM: number;
}

/** Gerardus's site list: the epoch the origins are given at, when it says. */
export interface SiteList {
	sites: GerardusSite[];
	skipped: number;
	epoch?: number;
}

/**
 * Why a call to Gerardus failed, so the page can say what to do about it:
 * `unreachable` (nothing answered, or the browser refused the answer),
 * `not_prepared` (no bundle for the site), `unsupported` (a Gerardus without
 * this part of the HTTP API: it answers the path as gRPC, does not know the
 * path, or ignores `frame=site`), `forbidden` (403: Gerardus does not serve
 * this to this peer), `rejected` (400, or a request the client would not
 * send), `server` (any other failure status) and `malformed` (an answer that
 * is not the contract's shape).
 */
export type GerardusErrorKind =
	| 'unreachable'
	| 'not_prepared'
	| 'unsupported'
	| 'forbidden'
	| 'rejected'
	| 'server'
	| 'malformed';

export class GerardusError extends Error {
	readonly kind: GerardusErrorKind;
	readonly status?: number;

	constructor(kind: GerardusErrorKind, message: string, status?: number) {
		super(message);
		this.name = 'GerardusError';
		this.kind = kind;
		this.status = status;
	}
}

type FetchLike = (input: string, init?: RequestInit) => Promise<Response>;

export interface CallOptions {
	fetch?: FetchLike;
	signal?: AbortSignal;
}

// ---- Settings kept in this browser ------------------------------------------

/**
 * Tidies a typed address: trims it, adds `http://` when no scheme is given and
 * drops a trailing slash. Returns '' for something that is not an http(s) URL.
 */
export function normaliseAddress(input: string): string {
	let text = input.trim();
	if (text === '') return '';
	if (!/^[a-z][a-z0-9+.-]*:\/\//i.test(text)) text = `http://${text}`;
	try {
		const url = new URL(text);
		if (url.protocol !== 'http:' && url.protocol !== 'https:') return '';
		return `${url.protocol}//${url.host}${url.pathname}`.replace(/\/+$/, '');
	} catch {
		return '';
	}
}

function readStorage(key: string, storage: Storage | undefined): string {
	try {
		return storage?.getItem(key) ?? '';
	} catch {
		// Storage can be unavailable (private windows, blocked site data).
		return '';
	}
}

function writeStorage(key: string, value: string, storage: Storage | undefined): void {
	try {
		if (value) storage?.setItem(key, value);
		else storage?.removeItem(key);
	} catch {
		// Nothing to do: the default applies next time.
	}
}

/** Gerardus's address in this browser, or the default. */
export function gerardusAddress(storage: Storage | undefined = globalThis.localStorage): string {
	return normaliseAddress(readStorage(GERARDUS_ADDRESS_KEY, storage)) || GERARDUS_DEFAULT_ADDRESS;
}

/**
 * Records Gerardus's address and returns the one now in force. An empty or
 * unusable address, or the default itself, forgets the setting.
 */
export function rememberGerardusAddress(
	input: string,
	storage: Storage | undefined = globalThis.localStorage
): string {
	const address = normaliseAddress(input);
	writeStorage(GERARDUS_ADDRESS_KEY, address === GERARDUS_DEFAULT_ADDRESS ? '' : address, storage);
	return address || GERARDUS_DEFAULT_ADDRESS;
}

// ---- Parsing ----------------------------------------------------------------

/** An S2 cell token: up to sixteen hex digits, without the trailing zeros. */
export function isL16Token(token: string): boolean {
	return /^[0-9a-f]{1,16}$/.test(token);
}

const RESERVED = new Set(['layer', 'id', 'kind', 'author', 'edited_at', 'model_id', 'state']);

const isFiniteNumber = (n: unknown): n is number => typeof n === 'number' && Number.isFinite(n);

function isPosition(v: unknown, needZ: boolean): v is Position {
	return (
		Array.isArray(v) &&
		(v.length === 3 || (!needZ && v.length === 2)) &&
		v.every((n) => isFiniteNumber(n))
	);
}

function isPositions(v: unknown, depth: number, needZ: boolean): boolean {
	if (depth === 0) return isPosition(v, needZ);
	return Array.isArray(v) && v.every((c) => isPositions(c, depth - 1, needZ));
}

const DEPTH: Record<Geometry['type'], number> = {
	Point: 0,
	MultiPoint: 1,
	LineString: 1,
	MultiLineString: 2,
	Polygon: 2,
	MultiPolygon: 3
};

function parseGeometry(v: unknown, needZ: boolean): Geometry | null {
	if (!v || typeof v !== 'object') return null;
	const g = v as { type?: unknown; coordinates?: unknown };
	if (typeof g.type !== 'string' || !(g.type in DEPTH)) return null;
	const type = g.type as Geometry['type'];
	if (!isPositions(g.coordinates, DEPTH[type], needZ)) return null;
	return { type, coordinates: g.coordinates } as Geometry;
}

function tagValue(v: unknown): string {
	if (typeof v === 'string') return v;
	if (typeof v === 'number' || typeof v === 'boolean') return String(v);
	return JSON.stringify(v);
}

function featureOf(v: unknown, defaultLayer: string, needZ: boolean): SiteFeature | null {
	if (!v || typeof v !== 'object') return null;
	const f = v as { type?: unknown; geometry?: unknown; properties?: unknown; id?: unknown };
	if (f.type !== 'Feature') return null;
	const geometry = parseGeometry(f.geometry, needZ);
	if (!geometry) return null;
	const props =
		f.properties && typeof f.properties === 'object'
			? (f.properties as Record<string, unknown>)
			: {};
	const tags: Record<string, string> = {};
	for (const [key, value] of Object.entries(props)) {
		if (RESERVED.has(key) || value === null || value === undefined) continue;
		tags[key] = tagValue(value);
	}
	const id = props.id ?? f.id ?? '';
	const feature: SiteFeature = {
		layer: typeof props.layer === 'string' && props.layer ? props.layer : defaultLayer,
		id: typeof id === 'string' || typeof id === 'number' ? String(id) : '',
		kind: typeof props.kind === 'string' ? props.kind : '',
		tags,
		geometry
	};
	if (typeof props.author === 'string' && props.author) feature.author = props.author;
	if (typeof props.edited_at === 'string' && props.edited_at) feature.editedAt = props.edited_at;
	if (typeof props.model_id === 'string' && props.model_id) feature.modelId = props.model_id;
	if (typeof props.state === 'string' && props.state) feature.state = props.state;
	return feature;
}

/** One WGS 84 GeoJSON feature as a SiteFeature, or null when it is not one. */
export function parseFeature(v: unknown, defaultLayer = ''): SiteFeature | null {
	return featureOf(v, defaultLayer, false);
}

/**
 * One site-frame feature, or null when it is not one: every position must
 * carry its z, as the contract says, or the feature could not be placed.
 */
export function parseSiteFrameFeature(v: unknown, defaultLayer = ''): SiteFrameFeature | null {
	return featureOf(v, defaultLayer, true) as SiteFrameFeature | null;
}

function collectionFeatures(v: unknown): unknown[] {
	if (
		!v ||
		typeof v !== 'object' ||
		(v as { type?: unknown }).type !== 'FeatureCollection' ||
		!Array.isArray((v as { features?: unknown }).features)
	) {
		throw new GerardusError(
			'malformed',
			'Gerardus did not answer with a GeoJSON FeatureCollection.'
		);
	}
	return (v as { features: unknown[] }).features;
}

function collect<F>(items: unknown[], parse: (v: unknown) => F | null) {
	const features: F[] = [];
	let skipped = 0;
	for (const item of items) {
		const f = parse(item);
		if (f) features.push(f);
		else skipped++;
	}
	return { features, skipped };
}

/**
 * The features of a WGS 84 FeatureCollection. A feature that does not parse
 * is left out and counted, so one bad geometry does not hide the rest of the
 * site.
 */
export function parseCollection(
	v: unknown,
	defaultLayer = ''
): { features: SiteFeature[]; skipped: number } {
	return collect(collectionFeatures(v), (f) => parseFeature(f, defaultLayer));
}

/** The features of a site-frame FeatureCollection, those without z left out and counted. */
export function parseSiteFrameCollection(
	v: unknown,
	defaultLayer = ''
): { features: SiteFrameFeature[]; skipped: number } {
	return collect(collectionFeatures(v), (f) => parseSiteFrameFeature(f, defaultLayer));
}

/**
 * Whether every position lies within a few kilometres' worth of degrees of
 * the site's origin: a collection that should be in site-frame metres but
 * reads as longitude and latitude came from a Gerardus that ignored
 * `frame=site`. Empty collections are not.
 */
export function looksGeographic(
	features: SiteFeature[],
	origin: { lon: number; lat: number }
): boolean {
	let any = false;
	const near = (p: Position) =>
		Math.abs(p[0] - origin.lon) < 0.05 && Math.abs(p[1] - origin.lat) < 0.05;
	const walk = (c: unknown): boolean => {
		if (typeof (c as unknown[])[0] === 'number') {
			any = true;
			return near(c as Position);
		}
		return (c as unknown[]).every(walk);
	};
	const all = features.every((f) => walk(f.geometry.coordinates));
	return any && all;
}

/** The features of each layer, in the order Gerardus sent them. */
export function groupByLayer<F extends { layer: string }>(features: F[]): Map<string, F[]> {
	const out = new Map<string, F[]>();
	for (const f of features) {
		const list = out.get(f.layer);
		if (list) list.push(f);
		else out.set(f.layer, [f]);
	}
	return out;
}

/** One entry of the site list, or null when it is not one. */
export function parseSite(v: unknown): GerardusSite | null {
	if (!v || typeof v !== 'object') return null;
	const s = v as Record<string, unknown>;
	if (typeof s.l16 !== 'string' || !isL16Token(s.l16)) return null;
	if (!isFiniteNumber(s.lon) || !isFiniteNumber(s.lat)) return null;
	if (Math.abs(s.lon) > 180 || Math.abs(s.lat) > 90) return null;
	if (s.height_m !== undefined && s.height_m !== null && !isFiniteNumber(s.height_m)) return null;
	return {
		l16: s.l16,
		label: typeof s.label === 'string' ? s.label : '',
		lon: s.lon,
		lat: s.lat,
		heightM: isFiniteNumber(s.height_m) ? s.height_m : 0,
		layers: Array.isArray(s.layers)
			? s.layers.filter((l): l is string => typeof l === 'string' && l !== '')
			: []
	};
}

/** The body of `GET /api/v1/sites`: its sites, those that do not parse left out and counted. */
export function parseSites(v: unknown): SiteList {
	const body = v && typeof v === 'object' ? (v as { sites?: unknown; epoch?: unknown }) : {};
	if (!Array.isArray(body.sites)) {
		throw new GerardusError('malformed', 'Gerardus did not answer with a list of sites.');
	}
	const { features: sites, skipped } = collect(body.sites, parseSite);
	return isFiniteNumber(body.epoch) ? { sites, skipped, epoch: body.epoch } : { sites, skipped };
}

/** The frame a collection says it is in, `"site"` or `"wgs84"`, or null when it does not say. */
export function frameOf(v: unknown): string | null {
	const frame = v && typeof v === 'object' ? (v as { frame?: unknown }).frame : undefined;
	return typeof frame === 'string' ? frame : null;
}

/** The body of `GET /api/v1/sites/{l16}/ground`. Anything off the contract is malformed. */
export function parseGround(v: unknown): GroundGrid {
	const bad = (why: string) =>
		new GerardusError('malformed', `Gerardus answered with a ground grid that ${why}.`);
	if (!v || typeof v !== 'object') throw bad('is not an object');
	const g = v as Record<string, unknown>;
	for (const key of ['x0', 'y0', 'cell_m', 'nx', 'ny']) {
		if (!isFiniteNumber(g[key])) throw bad(`has no number ${key}`);
	}
	const [x0, y0, cellM, nx, ny] = [g.x0, g.y0, g.cell_m, g.nx, g.ny] as number[];
	if (!(cellM > 0)) throw bad('has a cell size that is not positive');
	if (!Number.isInteger(nx) || !Number.isInteger(ny) || nx < 0 || ny < 0) {
		throw bad('has a size that is not whole cells');
	}
	if (!Array.isArray(g.heights) || g.heights.length !== nx * ny) {
		throw bad(`does not hold nx × ny = ${nx * ny} heights`);
	}
	const heights = g.heights as unknown[];
	if (!heights.every((h) => h === null || isFiniteNumber(h))) {
		throw bad('holds a height that is neither a number nor null');
	}
	return { x0, y0, cellM, nx, ny, heights: heights as (number | null)[] };
}

/**
 * The scan points of a `/scans` answer: little-endian float32 triplets, read
 * as little-endian whatever this machine's own byte order. A length that is
 * not whole points, or a point count header that disagrees with it, is
 * malformed.
 */
export function parseScanPoints(
	buffer: ArrayBuffer,
	pointCountHeader: string | null
): Float32Array {
	if (buffer.byteLength % 12 !== 0) {
		throw new GerardusError(
			'malformed',
			`Gerardus sent ${buffer.byteLength} bytes of scan points, which is not whole x, y, z points.`
		);
	}
	const count = buffer.byteLength / 12;
	if (pointCountHeader !== null && pointCountHeader.trim() !== '') {
		const claimed = Number(pointCountHeader.trim());
		if (!Number.isInteger(claimed) || claimed !== count) {
			throw new GerardusError(
				'malformed',
				`Gerardus said it sent ${pointCountHeader.trim()} scan points but sent ${count}.`
			);
		}
	}
	const view = new DataView(buffer);
	const points = new Float32Array(count * 3);
	for (let i = 0; i < points.length; i++) points[i] = view.getFloat32(i * 4, true);
	return points;
}

// ---- Calls ------------------------------------------------------------------

async function messageOf(res: Response): Promise<string> {
	let text: string;
	try {
		text = (await res.text()).trim();
	} catch {
		return '';
	}
	try {
		const body = JSON.parse(text);
		if (body && typeof body === 'object') {
			const m = (body as { error?: unknown; message?: unknown }).error ?? body.message;
			if (typeof m === 'string') return m;
		}
	} catch {
		// Not JSON: the text is the message.
	}
	return text.length > 300 ? `${text.slice(0, 300)}…` : text;
}

/** What a 404 means for this path, given Gerardus's message. */
type NotFound = (detail: string) => GerardusError;

function siteNotFound(l16: string): NotFound {
	return (detail) =>
		new GerardusError(
			'not_prepared',
			`Gerardus has no prepared site ${l16}${detail ? ` (${detail})` : ''}. Prepare it with ` +
				`gerardus prepare-site, then load it again. A Gerardus without the site API ` +
				`answers the same way.`,
			404
		);
}

async function request(
	base: string,
	path: string,
	accept: string,
	notFound: NotFound,
	opts: CallOptions
): Promise<Response> {
	const doFetch = opts.fetch ?? ((input, i) => globalThis.fetch(input, i));
	let res: Response;
	try {
		res = await doFetch(`${base}${path}`, {
			method: 'GET',
			headers: { Accept: accept },
			signal: opts.signal
		});
	} catch (e) {
		if (e instanceof DOMException && e.name === 'AbortError') throw e;
		throw new GerardusError(
			'unreachable',
			`Gerardus is not answering at ${base}. Start it (gerardus serve), or check its address; ` +
				`a Gerardus that does not allow this page's origin looks the same.`
		);
	}
	// A Gerardus older than this API hands the path to its gRPC router, which
	// answers 200 with grpc-status 12 (UNIMPLEMENTED) and no body.
	const contentType = res.headers?.get('content-type') ?? '';
	if (contentType.startsWith('application/grpc')) {
		const status = res.headers?.get('grpc-status');
		throw new GerardusError(
			'unsupported',
			`Gerardus at ${base} answered, but has no site API${status ? ` (gRPC status ${status})` : ''}. ` +
				`Run a Gerardus that serves /api/v1/sites.`,
			res.status
		);
	}
	if (res.ok) return res;
	const detail = await messageOf(res);
	if (res.status === 404) throw notFound(detail);
	if (res.status === 403) {
		throw new GerardusError(
			'forbidden',
			`Gerardus does not serve this to this browser${detail ? `: ${detail}` : ''}.`,
			403
		);
	}
	if (res.status === 400) {
		throw new GerardusError(
			'rejected',
			detail
				? `Gerardus refused the request: ${detail}`
				: 'Gerardus refused the request (HTTP 400).',
			400
		);
	}
	throw new GerardusError(
		'server',
		`Gerardus failed (HTTP ${res.status})${detail ? `: ${detail}` : ''}.`,
		res.status
	);
}

async function readJson(res: Response): Promise<unknown> {
	try {
		return await res.json();
	} catch {
		throw new GerardusError('malformed', 'Gerardus answered with something that is not JSON.');
	}
}

function sitePath(l16: string): string {
	if (!isL16Token(l16)) {
		throw new GerardusError('rejected', `“${l16}” is not an S2 cell token.`);
	}
	return `/api/v1/sites/${encodeURIComponent(l16)}`;
}

function layersQuery(layers: readonly string[], extra: Record<string, string> = {}): string {
	return new URLSearchParams({ layers: layers.join(','), ...extra })
		.toString()
		.replace(/%2C/g, ',');
}

const GEOJSON = 'application/geo+json, application/json';

/** Every prepared site Gerardus holds, with its origin and the layers it serves. */
export async function listSites(base: string, opts: CallOptions = {}): Promise<SiteList> {
	const res = await request(
		base,
		'/api/v1/sites',
		'application/json',
		() =>
			new GerardusError(
				'unsupported',
				`Gerardus at ${base} does not list its sites (GET /api/v1/sites): it is older than the ` +
					`site viewer.`,
				404
			),
		opts
	);
	return parseSites(await readJson(res));
}

/** The site's layers as WGS 84 GeoJSON features. Layers the bundle lacks are simply absent. */
export async function getSiteGeojson(
	base: string,
	l16: string,
	layers: readonly string[] = SITE_LAYERS,
	opts: CallOptions = {}
): Promise<{ features: SiteFeature[]; skipped: number }> {
	const path = `${sitePath(l16)}/geojson?${layersQuery(layers)}`;
	const res = await request(base, path, GEOJSON, siteNotFound(l16), opts);
	return parseCollection(await readJson(res));
}

/**
 * The site's layers in the site frame, metres about its origin. An answer
 * still in longitude and latitude — from a Gerardus that does not know
 * `frame=site` and ignored it — is refused as unsupported rather than drawn a
 * hundred kilometres off: the collection's `frame` member says so, or, from a
 * Gerardus too old to send one, its positions lie by the origin given.
 */
export async function getSiteFrameGeojson(
	base: string,
	l16: string,
	layers: readonly string[] = SITE_LAYERS,
	opts: CallOptions & { origin?: { lon: number; lat: number } } = {}
): Promise<{ features: SiteFrameFeature[]; skipped: number }> {
	const path = `${sitePath(l16)}/geojson?${layersQuery(layers, { frame: 'site' })}`;
	const res = await request(base, path, GEOJSON, siteNotFound(l16), opts);
	const body = await readJson(res);
	const frame = frameOf(body);
	const geographic =
		frame !== null
			? frame !== 'site'
			: opts.origin !== undefined && looksGeographic(parseCollection(body).features, opts.origin);
	if (geographic) {
		throw new GerardusError(
			'unsupported',
			`Gerardus at ${base} answered in longitude and latitude: it does not serve the site ` +
				`frame (frame=site). Run a newer Gerardus for the 3D view.`
		);
	}
	return parseSiteFrameCollection(body);
}

/** The site's ground grid, or null when the site has none. */
export async function getSiteGround(
	base: string,
	l16: string,
	opts: CallOptions = {}
): Promise<GroundGrid | null> {
	const path = `${sitePath(l16)}/ground`;
	let res: Response;
	try {
		res = await request(base, path, 'application/json', siteNotFound(l16), opts);
	} catch (e) {
		if (e instanceof GerardusError && e.kind === 'not_prepared') return null;
		throw e;
	}
	return parseGround(await readJson(res));
}

/** The voxel sizes the viewer offers for scan points, metres. */
export const SCAN_VOXELS_M = [0.1, 0.2, 0.5, 1] as const;

/**
 * The site's scan points, one per `voxelM` voxel. A 403, which Gerardus
 * answers unless it serves scans to this peer, is a `forbidden` error.
 */
export async function getSiteScans(
	base: string,
	l16: string,
	voxelM = 0.2,
	opts: CallOptions = {}
): Promise<ScanCloud> {
	if (!(Number.isFinite(voxelM) && voxelM > 0)) {
		throw new GerardusError('rejected', `A voxel of ${voxelM} m is not a size.`);
	}
	const path = `${sitePath(l16)}/scans?voxel_m=${encodeURIComponent(String(voxelM))}`;
	const res = await request(base, path, 'application/octet-stream', siteNotFound(l16), opts);
	let buffer: ArrayBuffer;
	try {
		buffer = await res.arrayBuffer();
	} catch {
		throw new GerardusError('malformed', 'Gerardus’s scan points could not be read.');
	}
	const points = parseScanPoints(buffer, res.headers?.get('x-gerardus-point-count') ?? null);
	return {
		points,
		count: points.length / 3,
		modelId: res.headers?.get('x-gerardus-model-id') ?? '',
		voxelM: voxelUsed(res.headers?.get('x-gerardus-voxel-m') ?? null, voxelM)
	};
}

/**
 * The voxel Gerardus says it used, or the one asked for when it does not say.
 * A header that is not a positive size is malformed.
 */
export function voxelUsed(header: string | null, asked: number): number {
	if (header === null || header.trim() === '') return asked;
	const v = Number(header.trim());
	if (!(Number.isFinite(v) && v > 0)) {
		throw new GerardusError('malformed', `Gerardus said it used a voxel of “${header}”.`);
	}
	return v;
}
