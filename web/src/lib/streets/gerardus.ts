/**
 * The street editor's client for Gerardus's HTTP API (Gerardus
 * docs/plans/VECTOR-MAP.md §4.1).
 *
 * Gerardus serves it beside `GET /` on its gRPC port, so the browser talks to
 * it directly, not through velocity.report's server:
 *
 * - `GET /api/v1/sites/{l16}/geojson?layers=…` returns an RFC 7946
 *   FeatureCollection in WGS 84, height as a third coordinate. Each feature's
 *   properties carry `layer`, `id`, `kind`, its tags and, for street use,
 *   `author` and `edited_at`. A layer the site's bundle lacks is absent.
 * - `POST /api/v1/sites/{l16}/street-use` takes
 *   `{"author", "note", "edits": [{"upsert": Feature} | {"remove": id}]}` and
 *   answers `{"features": FeatureCollection, "problems": ["<id>: …"]}`, or 400
 *   with a message.
 *
 * Its address is a setting kept in this browser, defaulting to the port
 * Gerardus serves on. The operator's name, which every edit carries, is
 * remembered the same way.
 */

export const GERARDUS_DEFAULT_ADDRESS = 'http://127.0.0.1:50061';
export const GERARDUS_ADDRESS_KEY = 'velocity.report.gerardusAddress';
export const STREET_EDITOR_AUTHOR_KEY = 'velocity.report.streetEditor.author';

/** Every layer the street editor draws, in the order they are drawn. */
export const STREET_LAYERS = [
	'footprints',
	'boxes',
	'city_streets',
	'city_intersections',
	'kerbs',
	'street_use'
] as const;
export type StreetLayer = (typeof STREET_LAYERS)[number];

/** The kinds an operator marks. */
export const STREET_USE_KINDS = ['driveway', 'parking_space', 'travel_lane'] as const;
export type StreetUseKind = (typeof STREET_USE_KINDS)[number];

/** A GeoJSON position: longitude, latitude and, optionally, height. */
export type Position = [number, number] | [number, number, number];

export type Geometry =
	| { type: 'Point'; coordinates: Position }
	| { type: 'MultiPoint'; coordinates: Position[] }
	| { type: 'LineString'; coordinates: Position[] }
	| { type: 'MultiLineString'; coordinates: Position[][] }
	| { type: 'Polygon'; coordinates: Position[][] }
	| { type: 'MultiPolygon'; coordinates: Position[][][] };

export interface GeoJSONFeature {
	type: 'Feature';
	geometry: Geometry;
	properties: Record<string, unknown>;
}

/** A feature as the editor uses it: the reserved properties lifted out, the rest as tags. */
export interface SiteFeature {
	layer: string;
	id: string;
	kind: string;
	tags: Record<string, string>;
	author?: string;
	editedAt?: string;
	geometry: Geometry;
}

export type StreetUseEdit = { upsert: GeoJSONFeature } | { remove: string };

export interface StreetUseRequest {
	author: string;
	note: string;
	edits: StreetUseEdit[];
}

export interface StreetUseResult {
	/** Every current street-use feature after the edit. */
	features: SiteFeature[];
	problems: string[];
}

/**
 * Why a call to Gerardus failed, so the page can say what to do about it:
 * `unreachable` (nothing answered, or the browser refused the answer),
 * `not_prepared` (no bundle for the site), `unsupported` (a Gerardus without
 * this HTTP API, which answers the path as gRPC), `rejected` (400: the request
 * was refused, with Gerardus's message), `server` (any other failure status)
 * and `malformed` (an answer that is not the contract's shape).
 */
export type GerardusErrorKind =
	'unreachable' | 'not_prepared' | 'unsupported' | 'rejected' | 'server' | 'malformed';

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

/** The name this browser last saved street use under. */
export function rememberedAuthor(storage: Storage | undefined = globalThis.localStorage): string {
	return readStorage(STREET_EDITOR_AUTHOR_KEY, storage);
}

export function rememberAuthor(
	author: string,
	storage: Storage | undefined = globalThis.localStorage
): void {
	writeStorage(STREET_EDITOR_AUTHOR_KEY, author.trim(), storage);
}

// ---- Parsing ----------------------------------------------------------------

/** An S2 cell token: up to sixteen hex digits, without the trailing zeros. */
export function isL16Token(token: string): boolean {
	return /^[0-9a-f]{1,16}$/.test(token);
}

const RESERVED = new Set(['layer', 'id', 'kind', 'author', 'edited_at']);

function isPosition(v: unknown): v is Position {
	return (
		Array.isArray(v) &&
		(v.length === 2 || v.length === 3) &&
		v.every((n) => typeof n === 'number' && Number.isFinite(n))
	);
}

function isPositions(v: unknown, depth: number): boolean {
	if (depth === 0) return isPosition(v);
	return Array.isArray(v) && v.every((c) => isPositions(c, depth - 1));
}

const DEPTH: Record<Geometry['type'], number> = {
	Point: 0,
	MultiPoint: 1,
	LineString: 1,
	MultiLineString: 2,
	Polygon: 2,
	MultiPolygon: 3
};

function parseGeometry(v: unknown): Geometry | null {
	if (!v || typeof v !== 'object') return null;
	const g = v as { type?: unknown; coordinates?: unknown };
	if (typeof g.type !== 'string' || !(g.type in DEPTH)) return null;
	const type = g.type as Geometry['type'];
	if (!isPositions(g.coordinates, DEPTH[type])) return null;
	return { type, coordinates: g.coordinates } as Geometry;
}

function tagValue(v: unknown): string {
	if (typeof v === 'string') return v;
	if (typeof v === 'number' || typeof v === 'boolean') return String(v);
	return JSON.stringify(v);
}

/** One GeoJSON feature as a SiteFeature, or null when it is not one. */
export function parseFeature(v: unknown, defaultLayer = ''): SiteFeature | null {
	if (!v || typeof v !== 'object') return null;
	const f = v as { type?: unknown; geometry?: unknown; properties?: unknown; id?: unknown };
	if (f.type !== 'Feature') return null;
	const geometry = parseGeometry(f.geometry);
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
	return feature;
}

/**
 * The features of a FeatureCollection. A feature that does not parse is left
 * out and counted, so one bad geometry does not hide the rest of the site.
 */
export function parseCollection(
	v: unknown,
	defaultLayer = ''
): { features: SiteFeature[]; skipped: number } {
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
	const features: SiteFeature[] = [];
	let skipped = 0;
	for (const item of (v as { features: unknown[] }).features) {
		const f = parseFeature(item, defaultLayer);
		if (f) features.push(f);
		else skipped++;
	}
	return { features, skipped };
}

/** The features of each layer, in the order Gerardus sent them. */
export function groupByLayer(features: SiteFeature[]): Map<string, SiteFeature[]> {
	const out = new Map<string, SiteFeature[]>();
	for (const f of features) {
		const list = out.get(f.layer);
		if (list) list.push(f);
		else out.set(f.layer, [f]);
	}
	return out;
}

/**
 * Sorts the server's problems by the feature they name. Each problem is
 * `"<id>: message"`; one whose id is not among `ids`, or that names none, is
 * kept whole in `general` so nothing the server said is dropped.
 */
export function mapProblems(
	problems: string[],
	ids: Iterable<string>
): { byId: Map<string, string[]>; general: string[] } {
	// Ids hold colons themselves ("gms:<ulid>"), so match known ids as
	// prefixes, longest first, rather than splitting at the first colon.
	const known = [...new Set(ids)].filter(Boolean).sort((a, b) => b.length - a.length);
	const byId = new Map<string, string[]>();
	const general: string[] = [];
	for (const problem of problems) {
		const text = problem.trim();
		const id = known.find((k) => text.startsWith(`${k}:`));
		if (id) {
			const list = byId.get(id) ?? [];
			list.push(text.slice(id.length + 1).trim() || text);
			byId.set(id, list);
		} else {
			general.push(problem);
		}
	}
	return { byId, general };
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

async function call(
	base: string,
	path: string,
	init: RequestInit,
	l16: string,
	opts: CallOptions
): Promise<unknown> {
	const doFetch = opts.fetch ?? ((input, i) => globalThis.fetch(input, i));
	let res: Response;
	try {
		res = await doFetch(`${base}${path}`, { ...init, signal: opts.signal });
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
			`Gerardus at ${base} answered, but has no street editor API${status ? ` (gRPC status ${status})` : ''}. ` +
				`Run a Gerardus that serves /api/v1/sites.`,
			res.status
		);
	}
	if (!res.ok) {
		const detail = await messageOf(res);
		if (res.status === 404) {
			throw new GerardusError(
				'not_prepared',
				`Gerardus has no prepared site ${l16}${detail ? ` (${detail})` : ''}. Prepare it with ` +
					`gerardus prepare-site, then load it again. A Gerardus without the street editor's API ` +
					`answers the same way.`,
				404
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

/** The site's layers as GeoJSON features. Layers the bundle lacks are simply absent. */
export async function getSiteGeojson(
	base: string,
	l16: string,
	layers: readonly string[] = STREET_LAYERS,
	opts: CallOptions = {}
): Promise<{ features: SiteFeature[]; skipped: number }> {
	const query = new URLSearchParams({ layers: layers.join(',') });
	const body = await call(
		base,
		`${sitePath(l16)}/geojson?${query.toString().replace(/%2C/g, ',')}`,
		{ method: 'GET', headers: { Accept: 'application/geo+json, application/json' } },
		l16,
		opts
	);
	return parseCollection(body);
}

/**
 * Sends one batch of street-use edits. The author is required, by the client
 * as by the server; the note says why, and may be empty.
 */
export async function postStreetUse(
	base: string,
	l16: string,
	request: StreetUseRequest,
	opts: CallOptions = {}
): Promise<StreetUseResult> {
	const author = request.author.trim();
	if (!author)
		throw new GerardusError('rejected', 'Say who is making the edits: the author is required.');
	const path = `${sitePath(l16)}/street-use`;
	const body = await call(
		base,
		path,
		{
			method: 'POST',
			headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
			body: JSON.stringify({ author, note: request.note.trim(), edits: request.edits })
		},
		l16,
		opts
	);
	if (!body || typeof body !== 'object') {
		throw new GerardusError('malformed', 'Gerardus answered the edit with something unexpected.');
	}
	const { features } = parseCollection((body as { features?: unknown }).features, 'street_use');
	const raw = (body as { problems?: unknown }).problems;
	const problems = Array.isArray(raw)
		? raw.map((p) => (typeof p === 'string' ? p : tagValue(p)))
		: [];
	return { features, problems };
}
