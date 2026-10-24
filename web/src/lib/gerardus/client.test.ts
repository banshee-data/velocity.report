import {
	GERARDUS_ADDRESS_KEY,
	GERARDUS_DEFAULT_ADDRESS,
	GerardusError,
	KNOWN_LAYERS,
	OSM_LAYERS,
	SITE_LAYERS,
	gerardusAddress,
	getSiteFrameGeojson,
	getSiteGeojson,
	getSiteGround,
	getSiteScans,
	groupByLayer,
	isL16Token,
	isOsmLayer,
	listSites,
	frameOf,
	looksGeographic,
	normaliseAddress,
	parseCollection,
	parseFeature,
	parseGround,
	parseScanPoints,
	parseSite,
	parseSiteFrameCollection,
	parseSiteFrameFeature,
	parseSites,
	rememberGerardusAddress,
	voxelUsed
} from './client';

const BASE = 'http://127.0.0.1:50061';
const SITE = '808580f3f';

function jsonResponse(body: unknown, status = 200): Response {
	return {
		ok: status >= 200 && status < 300,
		status,
		json: async () => body,
		text: async () => JSON.stringify(body)
	} as unknown as Response;
}

function textResponse(text: string, status: number): Response {
	return {
		ok: false,
		status,
		json: async () => JSON.parse(text),
		text: async () => text
	} as unknown as Response;
}

const SITE_GEOJSON = {
	type: 'FeatureCollection',
	features: [
		{
			type: 'Feature',
			geometry: {
				type: 'LineString',
				coordinates: [
					[-122.4065, 37.7978, 12.1],
					[-122.406, 37.798, 12.3]
				]
			},
			properties: {
				layer: 'city_streets',
				id: 'cnn:1234',
				kind: 'road',
				street: 'COLUMBUS AVE',
				lanes: 2
			}
		},
		{
			type: 'Feature',
			geometry: { type: 'Point', coordinates: [-122.4062, 37.7979, 12.2] },
			properties: { layer: 'city_intersections', id: 'cnn:55', kind: 'other' }
		},
		{
			type: 'Feature',
			geometry: {
				type: 'Polygon',
				coordinates: [
					[
						[-122.4064, 37.7978],
						[-122.4063, 37.7978],
						[-122.4063, 37.7979],
						[-122.4064, 37.7978]
					]
				]
			},
			properties: {
				layer: 'street_use',
				id: 'gms:01J',
				kind: 'parking_space',
				space_type: 'metered',
				count: 3,
				author: 'dd',
				edited_at: '2026-10-23T10:00:00Z'
			}
		},
		{ type: 'Feature', geometry: { type: 'LineString', coordinates: [[1]] }, properties: {} },
		{ type: 'Nope' }
	]
};

describe('Gerardus settings', () => {
	beforeEach(() => localStorage.clear());

	it('tidies a typed address', () => {
		expect(normaliseAddress('  127.0.0.1:50061/ ')).toBe('http://127.0.0.1:50061');
		expect(normaliseAddress('https://gerardus.tail1234.ts.net/')).toBe(
			'https://gerardus.tail1234.ts.net'
		);
		expect(normaliseAddress('http://host:1/prefix//')).toBe('http://host:1/prefix');
		expect(normaliseAddress('')).toBe('');
		expect(normaliseAddress('ftp://host')).toBe('');
		expect(normaliseAddress('http://')).toBe('');
	});

	it('defaults to the local Gerardus port and remembers another', () => {
		expect(gerardusAddress()).toBe(GERARDUS_DEFAULT_ADDRESS);
		expect(rememberGerardusAddress('mini.tail.ts.net:50061')).toBe('http://mini.tail.ts.net:50061');
		expect(localStorage.getItem(GERARDUS_ADDRESS_KEY)).toBe('http://mini.tail.ts.net:50061');
		expect(gerardusAddress()).toBe('http://mini.tail.ts.net:50061');
	});

	it('forgets the setting for the default or an unusable address', () => {
		rememberGerardusAddress('http://elsewhere:1');
		expect(rememberGerardusAddress(GERARDUS_DEFAULT_ADDRESS)).toBe(GERARDUS_DEFAULT_ADDRESS);
		expect(localStorage.getItem(GERARDUS_ADDRESS_KEY)).toBeNull();
		rememberGerardusAddress('http://elsewhere:1');
		expect(rememberGerardusAddress('ftp://x')).toBe(GERARDUS_DEFAULT_ADDRESS);
		expect(localStorage.getItem(GERARDUS_ADDRESS_KEY)).toBeNull();
		localStorage.setItem(GERARDUS_ADDRESS_KEY, 'ftp://stale');
		expect(gerardusAddress()).toBe(GERARDUS_DEFAULT_ADDRESS);
	});

	it('treats unusable storage as nothing remembered', () => {
		const broken = {
			getItem: () => {
				throw new Error('blocked');
			},
			setItem: () => {
				throw new Error('blocked');
			},
			removeItem: () => {
				throw new Error('blocked');
			}
		} as unknown as Storage;
		expect(gerardusAddress(broken)).toBe(GERARDUS_DEFAULT_ADDRESS);
		expect(rememberGerardusAddress('http://h:1', broken)).toBe('http://h:1');
		expect(gerardusAddress(undefined)).toBe(GERARDUS_DEFAULT_ADDRESS);
		expect(rememberGerardusAddress('http://h:1', undefined)).toBe('http://h:1');
	});
});

describe('parsing', () => {
	it('checks L16 tokens', () => {
		expect(isL16Token('808580f3f')).toBe(true);
		expect(isL16Token('80858-0f3f')).toBe(false);
		expect(isL16Token('')).toBe(false);
		expect(isL16Token('808580F3F')).toBe(false);
	});

	it('lifts the reserved properties and keeps the rest as string tags', () => {
		const { features, skipped } = parseCollection(SITE_GEOJSON);
		expect(skipped).toBe(2);
		expect(features).toHaveLength(3);
		expect(features[0]).toEqual({
			layer: 'city_streets',
			id: 'cnn:1234',
			kind: 'road',
			tags: { street: 'COLUMBUS AVE', lanes: '2' },
			geometry: SITE_GEOJSON.features[0].geometry
		});
		expect(features[2].author).toBe('dd');
		expect(features[2].editedAt).toBe('2026-10-23T10:00:00Z');
		expect(features[2].tags).toEqual({ space_type: 'metered', count: '3' });
	});

	it('takes the feature id when properties have none, and a default layer', () => {
		const f = parseFeature(
			{
				type: 'Feature',
				id: 7,
				geometry: {
					type: 'MultiLineString',
					coordinates: [
						[
							[0, 0],
							[1, 1]
						]
					]
				},
				properties: { kind: 'kerb', flag: true, extra: { a: 1 }, gone: null }
			},
			'street_use'
		);
		expect(f).toMatchObject({ id: '7', layer: 'street_use', kind: 'kerb' });
		expect(f?.tags).toEqual({ flag: 'true', extra: '{"a":1}' });
		expect(f?.author).toBeUndefined();
	});

	it('accepts a feature without properties, and refuses bad geometry', () => {
		const bare = parseFeature({
			type: 'Feature',
			geometry: {
				type: 'MultiPolygon',
				coordinates: [
					[
						[
							[0, 0],
							[1, 0],
							[0, 1],
							[0, 0]
						]
					]
				]
			}
		});
		expect(bare).toMatchObject({ layer: '', id: '', kind: '', tags: {} });
		expect(
			parseFeature({
				type: 'Feature',
				id: {},
				geometry: { type: 'MultiPoint', coordinates: [[0, 0]] }
			})?.id
		).toBe('');
		expect(parseFeature(null)).toBeNull();
		expect(parseFeature({ type: 'Feature', geometry: null })).toBeNull();
		expect(
			parseFeature({ type: 'Feature', geometry: { type: 'Circle', coordinates: [] } })
		).toBeNull();
		expect(
			parseFeature({ type: 'Feature', geometry: { type: 'Point', coordinates: [0, NaN] } })
		).toBeNull();
		expect(
			parseFeature({ type: 'Feature', geometry: { type: 'Point', coordinates: [0, 1, 2, 3] } })
		).toBeNull();
	});

	it('refuses something that is not a FeatureCollection', () => {
		expect(() => parseCollection({ type: 'Feature' })).toThrow(GerardusError);
		expect(() => parseCollection(null)).toThrow('FeatureCollection');
		expect(() => parseCollection({ type: 'FeatureCollection' })).toThrow(GerardusError);
	});

	it('groups features by layer in the order they came', () => {
		const { features } = parseCollection(SITE_GEOJSON);
		const groups = groupByLayer([...features, { ...features[0], id: 'cnn:2' }]);
		expect([...groups.keys()]).toEqual(['city_streets', 'city_intersections', 'street_use']);
		expect(groups.get('city_streets')!.map((f) => f.id)).toEqual(['cnn:1234', 'cnn:2']);
	});
});

describe('GET the site layers', () => {
	it('asks for every layer the viewer draws and parses the collection', async () => {
		const fetch = jest.fn().mockResolvedValue(jsonResponse(SITE_GEOJSON));
		const { features, skipped } = await getSiteGeojson(BASE, SITE, SITE_LAYERS, { fetch });
		expect(fetch).toHaveBeenCalledTimes(1);
		const [url, init] = fetch.mock.calls[0];
		expect(url).toBe(
			`${BASE}/api/v1/sites/${SITE}/geojson?layers=street_use,footprints,boxes,wall_planes,city_streets,city_intersections,kerbs,observed`
		);
		expect(init.method).toBe('GET');
		expect(init.headers.Accept).toContain('application/geo+json');
		expect(features.map((f) => f.layer)).toEqual([
			'city_streets',
			'city_intersections',
			'street_use'
		]);
		expect(skipped).toBe(2);
	});

	it('uses the global fetch by default', async () => {
		const saved = global.fetch;
		global.fetch = jest
			.fn()
			.mockResolvedValue(jsonResponse({ type: 'FeatureCollection', features: [] }));
		try {
			expect((await getSiteGeojson(BASE, SITE)).features).toEqual([]);
			expect(global.fetch).toHaveBeenCalled();
		} finally {
			global.fetch = saved;
		}
	});

	it('says Gerardus is unreachable when nothing answers', async () => {
		const fetch = jest.fn().mockRejectedValue(new TypeError('Failed to fetch'));
		await expect(getSiteGeojson(BASE, SITE, SITE_LAYERS, { fetch })).rejects.toMatchObject({
			kind: 'unreachable',
			message: expect.stringContaining(`not answering at ${BASE}`)
		});
	});

	it('passes an abort through untouched', async () => {
		const abort = new DOMException('aborted', 'AbortError');
		const fetch = jest.fn().mockRejectedValue(abort);
		await expect(getSiteGeojson(BASE, SITE, SITE_LAYERS, { fetch })).rejects.toBe(abort);
	});

	it('says the site is not prepared on 404', async () => {
		const fetch = jest.fn().mockResolvedValue(jsonResponse({ error: 'no bundle' }, 404));
		await expect(getSiteGeojson(BASE, SITE, SITE_LAYERS, { fetch })).rejects.toMatchObject({
			kind: 'not_prepared',
			status: 404,
			message: expect.stringContaining(`no prepared site ${SITE} (no bundle)`)
		});
		fetch.mockResolvedValue(textResponse('', 404));
		await expect(getSiteGeojson(BASE, SITE, SITE_LAYERS, { fetch })).rejects.toMatchObject({
			message: expect.stringContaining(`no prepared site ${SITE}. Prepare`)
		});
	});

	it('reports other failures with their status and message', async () => {
		const fetch = jest.fn().mockResolvedValue(textResponse('bundle unreadable', 500));
		await expect(getSiteGeojson(BASE, SITE, SITE_LAYERS, { fetch })).rejects.toMatchObject({
			kind: 'server',
			status: 500,
			message: 'Gerardus failed (HTTP 500): bundle unreadable.'
		});
		fetch.mockResolvedValue(textResponse('x'.repeat(400), 502));
		const err = await getSiteGeojson(BASE, SITE, SITE_LAYERS, { fetch }).catch((e) => e);
		expect(err.message.length).toBeLessThan(340);
		fetch.mockResolvedValue({
			ok: false,
			status: 503,
			text: async () => {
				throw new Error('gone');
			}
		});
		await expect(getSiteGeojson(BASE, SITE, SITE_LAYERS, { fetch })).rejects.toMatchObject({
			message: 'Gerardus failed (HTTP 503).'
		});
	});

	it('says so when Gerardus has no site API and answers as gRPC', async () => {
		const headers = new Map([
			['content-type', 'application/grpc'],
			['grpc-status', '12']
		]);
		const fetch = jest.fn().mockResolvedValue({ ok: true, status: 200, headers });
		await expect(getSiteGeojson(BASE, SITE, SITE_LAYERS, { fetch })).rejects.toMatchObject({
			kind: 'unsupported',
			message: expect.stringContaining('no site API (gRPC status 12)')
		});
		headers.delete('grpc-status');
		await expect(getSiteGeojson(BASE, SITE, SITE_LAYERS, { fetch })).rejects.toMatchObject({
			message: expect.stringContaining('no site API. Run')
		});
		const json = new Map([['content-type', 'application/geo+json']]);
		fetch.mockResolvedValue({
			ok: true,
			status: 200,
			headers: json,
			json: async () => ({ type: 'FeatureCollection', features: [] })
		});
		expect((await getSiteGeojson(BASE, SITE, SITE_LAYERS, { fetch })).features).toEqual([]);
	});

	it('refuses an answer that is not JSON, or not GeoJSON', async () => {
		const fetch = jest.fn().mockResolvedValue({
			ok: true,
			status: 200,
			json: async () => {
				throw new SyntaxError('bad');
			}
		});
		await expect(getSiteGeojson(BASE, SITE, SITE_LAYERS, { fetch })).rejects.toMatchObject({
			kind: 'malformed'
		});
		fetch.mockResolvedValue(jsonResponse({ hello: 'world' }));
		await expect(getSiteGeojson(BASE, SITE, SITE_LAYERS, { fetch })).rejects.toMatchObject({
			kind: 'malformed'
		});
	});

	it('refuses a token that is not an S2 cell before calling', async () => {
		const fetch = jest.fn();
		await expect(getSiteGeojson(BASE, '../etc', SITE_LAYERS, { fetch })).rejects.toMatchObject({
			kind: 'rejected'
		});
		expect(fetch).not.toHaveBeenCalled();
	});

	it('shows the server’s message for a 400', async () => {
		const fetch = jest.fn().mockResolvedValue(textResponse('unknown layer: nope', 400));
		await expect(getSiteGeojson(BASE, SITE, ['nope'], { fetch })).rejects.toMatchObject({
			kind: 'rejected',
			status: 400,
			message: 'Gerardus refused the request: unknown layer: nope'
		});
		fetch.mockResolvedValue(jsonResponse({ message: 'layers is required' }, 400));
		await expect(getSiteGeojson(BASE, SITE, [], { fetch })).rejects.toThrow(
			'Gerardus refused the request: layers is required'
		);
		fetch.mockResolvedValue(textResponse('', 400));
		await expect(getSiteGeojson(BASE, SITE, [], { fetch })).rejects.toThrow(
			'Gerardus refused the request (HTTP 400).'
		);
	});
});

describe('layers', () => {
	it('keeps the OpenStreetMap layers apart', () => {
		expect(OSM_LAYERS).toEqual(['osm', 'intersections']);
		expect(KNOWN_LAYERS).toEqual([...SITE_LAYERS, ...OSM_LAYERS]);
		expect(SITE_LAYERS.some(isOsmLayer)).toBe(false);
		expect(isOsmLayer('intersections')).toBe(true);
		expect(isOsmLayer('city_intersections')).toBe(false);
	});
});

const SITES = {
	epoch: 2026.8111,
	sites: [
		{
			l16: '808580f3f',
			label: 'Columbus & Green',
			lon: -122.4065,
			lat: 37.7978,
			height_m: 11.5,
			layers: ['city_streets', 'kerbs', 'boxes', 7, '']
		},
		{ l16: '80858a1', lon: -122.41, lat: 37.79 },
		{ l16: 'NOT-A-TOKEN', lon: 0, lat: 0, height_m: 0, layers: [] },
		{ l16: '80858b', lon: 200, lat: 0 },
		{ l16: '80858c', lon: 1, lat: 1, height_m: 'tall' },
		{ l16: '80858d', lat: 1 },
		'nope'
	]
};

describe('GET the site list', () => {
	it('lists each prepared site with its origin and layers', async () => {
		const fetch = jest.fn().mockResolvedValue(jsonResponse(SITES));
		const { sites, skipped } = await listSites(BASE, { fetch });
		expect(fetch.mock.calls[0][0]).toBe(`${BASE}/api/v1/sites`);
		expect(sites).toEqual([
			{
				l16: '808580f3f',
				label: 'Columbus & Green',
				lon: -122.4065,
				lat: 37.7978,
				heightM: 11.5,
				layers: ['city_streets', 'kerbs', 'boxes']
			},
			{ l16: '80858a1', label: '', lon: -122.41, lat: 37.79, heightM: 0, layers: [] }
		]);
		expect(skipped).toBe(5);
		expect((await listSites(BASE, { fetch })).epoch).toBe(2026.8111);
	});

	it('parses one site, and refuses what is not one', () => {
		expect(parseSite(null)).toBeNull();
		expect(parseSite({ l16: '8085', lon: 1, lat: 95 })).toBeNull();
		expect(parseSite({ l16: '8085', lon: 1, lat: 2, height_m: null })?.heightM).toBe(0);
	});

	it('refuses an answer without a list of sites', () => {
		expect(() => parseSites({})).toThrow(GerardusError);
		expect(() => parseSites(null)).toThrow('list of sites');
		expect(parseSites({ sites: [] })).toEqual({ sites: [], skipped: 0 });
		expect(parseSites({ sites: [], epoch: 'now' })).toEqual({ sites: [], skipped: 0 });
	});

	it('calls a Gerardus without the list unsupported, not unprepared', async () => {
		const fetch = jest.fn().mockResolvedValue(textResponse('404 page not found', 404));
		await expect(listSites(BASE, { fetch })).rejects.toMatchObject({
			kind: 'unsupported',
			status: 404,
			message: expect.stringContaining('does not list its sites')
		});
		const headers = new Map([['content-type', 'application/grpc']]);
		fetch.mockResolvedValue({ ok: true, status: 200, headers });
		await expect(listSites(BASE, { fetch })).rejects.toMatchObject({ kind: 'unsupported' });
	});

	it('keeps the other error kinds', async () => {
		const fetch = jest.fn().mockRejectedValue(new TypeError('Failed to fetch'));
		await expect(listSites(BASE, { fetch })).rejects.toMatchObject({ kind: 'unreachable' });
		fetch.mockResolvedValue(textResponse('boom', 500));
		await expect(listSites(BASE, { fetch })).rejects.toMatchObject({ kind: 'server' });
		fetch.mockResolvedValue(jsonResponse([1, 2]));
		await expect(listSites(BASE, { fetch })).rejects.toMatchObject({ kind: 'malformed' });
	});
});

const SITE_FRAME = {
	type: 'FeatureCollection',
	l16_token: '808580f3f',
	layers: ['boxes', 'wall_planes', 'observed', 'kerbs'],
	frame: 'site',
	epoch: 2026.8111,
	features: [
		{
			type: 'Feature',
			geometry: {
				type: 'Polygon',
				coordinates: [
					[
						[0, 0, 0.1],
						[4, 0, 0.1],
						[4, 6, 0.2],
						[0, 0, 0.1]
					]
				]
			},
			properties: { layer: 'boxes', id: 'box:1', kind: 'vehicle', height_m: 1.6 }
		},
		{
			type: 'Feature',
			geometry: {
				type: 'Polygon',
				coordinates: [
					[
						[10, 2, 0],
						[14, 2, 0],
						[14, 2, 6],
						[10, 2, 6],
						[10, 2, 0]
					]
				]
			},
			properties: {
				layer: 'wall_planes',
				id: 'wp:3',
				kind: 'wall_plane',
				role: 'popout',
				offset_m: 0.4
			}
		},
		{
			type: 'Feature',
			geometry: { type: 'Point', coordinates: [3, 4, 0.3] },
			properties: { layer: 'observed', id: 'obs:9', kind: 'pole', height_m: 5.5 }
		},
		// No z: the site frame needs one.
		{
			type: 'Feature',
			geometry: {
				type: 'LineString',
				coordinates: [
					[0, 0],
					[1, 1]
				]
			},
			properties: { layer: 'kerbs', id: 'k:1', kind: 'kerb' }
		}
	]
};

describe('GET the site frame', () => {
	it('asks for frame=site and keeps only positions with z', async () => {
		const fetch = jest.fn().mockResolvedValue(jsonResponse(SITE_FRAME));
		const { features, skipped } = await getSiteFrameGeojson(BASE, SITE, ['boxes', 'wall_planes'], {
			fetch,
			origin: { lon: -122.4065, lat: 37.7978 }
		});
		expect(fetch.mock.calls[0][0]).toBe(
			`${BASE}/api/v1/sites/${SITE}/geojson?layers=boxes,wall_planes&frame=site`
		);
		expect(skipped).toBe(1);
		expect(features.map((f) => f.layer)).toEqual(['boxes', 'wall_planes', 'observed']);
		expect(features[0].tags.height_m).toBe('1.6');
		expect(features[1].tags).toEqual({ role: 'popout', offset_m: '0.4' });
		expect(features[2].geometry).toEqual({ type: 'Point', coordinates: [3, 4, 0.3] });
	});

	it('parses one site-frame feature, and a collection', () => {
		expect(parseSiteFrameFeature(SITE_FRAME.features[3])).toBeNull();
		expect(parseSiteFrameFeature(SITE_FRAME.features[2], 'x')?.layer).toBe('observed');
		expect(parseSiteFrameCollection(SITE_FRAME).features).toHaveLength(3);
		expect(() => parseSiteFrameCollection([])).toThrow(GerardusError);
	});

	it('refuses a collection that says it is in WGS 84', async () => {
		const fetch = jest.fn().mockResolvedValue(jsonResponse({ ...SITE_FRAME, frame: 'wgs84' }));
		await expect(getSiteFrameGeojson(BASE, SITE, SITE_LAYERS, { fetch })).rejects.toMatchObject({
			kind: 'unsupported'
		});
		// A collection that says it is in the site frame is believed, origin or not.
		fetch.mockResolvedValue(jsonResponse(SITE_FRAME));
		const { features } = await getSiteFrameGeojson(BASE, SITE, SITE_LAYERS, {
			fetch,
			origin: { lon: 1, lat: 2 }
		});
		expect(features).toHaveLength(3);
		expect(frameOf(SITE_FRAME)).toBe('site');
		expect(frameOf(SITE_GEOJSON)).toBeNull();
		expect(frameOf(null)).toBeNull();
	});

	it('refuses longitude and latitude from a Gerardus too old to say its frame', async () => {
		const fetch = jest.fn().mockResolvedValue(jsonResponse(SITE_GEOJSON));
		await expect(
			getSiteFrameGeojson(BASE, SITE, SITE_LAYERS, {
				fetch,
				origin: { lon: -122.4065, lat: 37.7978 }
			})
		).rejects.toMatchObject({
			kind: 'unsupported',
			message: expect.stringContaining('frame=site')
		});
		// Without an origin it cannot tell, and draws what has z.
		const { features } = await getSiteFrameGeojson(BASE, SITE, SITE_LAYERS, { fetch });
		expect(features.map((f) => f.id)).toEqual(['cnn:1234', 'cnn:55']);
	});

	it('tells site-frame metres from degrees near the origin', () => {
		const origin = { lon: -122.4065, lat: 37.7978 };
		const geo = parseCollection(SITE_GEOJSON).features;
		expect(looksGeographic(geo, origin)).toBe(true);
		expect(looksGeographic(parseSiteFrameCollection(SITE_FRAME).features, origin)).toBe(false);
		expect(looksGeographic([], origin)).toBe(false);
		expect(looksGeographic(geo, { lon: 0, lat: 0 })).toBe(false);
	});

	it('says the site is not prepared on 404', async () => {
		const fetch = jest.fn().mockResolvedValue(textResponse('', 404));
		await expect(getSiteFrameGeojson(BASE, SITE, SITE_LAYERS, { fetch })).rejects.toMatchObject({
			kind: 'not_prepared'
		});
	});
});

describe('GET the ground', () => {
	const GRID = {
		l16: '808580f3f',
		x0: -10,
		y0: -20,
		cell_m: 0.5,
		nx: 3,
		ny: 2,
		heights: [0.1, null, 0.3, 0.4, 0.5, null]
	};

	it('reads the grid, row-major with y outer, null where empty', async () => {
		const fetch = jest.fn().mockResolvedValue(jsonResponse(GRID));
		const grid = await getSiteGround(BASE, SITE, { fetch });
		expect(fetch.mock.calls[0][0]).toBe(`${BASE}/api/v1/sites/${SITE}/ground`);
		expect(grid).toEqual({
			x0: -10,
			y0: -20,
			cellM: 0.5,
			nx: 3,
			ny: 2,
			heights: [0.1, null, 0.3, 0.4, 0.5, null]
		});
	});

	it('answers null when the site has no grid', async () => {
		const fetch = jest.fn().mockResolvedValue(textResponse('no ground grid', 404));
		expect(await getSiteGround(BASE, SITE, { fetch })).toBeNull();
	});

	it('passes the other failures on', async () => {
		const fetch = jest.fn().mockResolvedValue(textResponse('boom', 500));
		await expect(getSiteGround(BASE, SITE, { fetch })).rejects.toMatchObject({ kind: 'server' });
	});

	it('refuses a grid off the contract', () => {
		expect(() => parseGround(null)).toThrow('not an object');
		expect(() => parseGround({ ...GRID, nx: undefined })).toThrow('no number nx');
		expect(() => parseGround({ ...GRID, cell_m: 0 })).toThrow('not positive');
		expect(() => parseGround({ ...GRID, nx: 1.5 })).toThrow('whole cells');
		expect(() => parseGround({ ...GRID, ny: -1 })).toThrow('whole cells');
		expect(() => parseGround({ ...GRID, heights: [1, 2] })).toThrow('nx × ny = 6');
		expect(() => parseGround({ ...GRID, heights: 'x' })).toThrow(GerardusError);
		expect(() => parseGround({ ...GRID, heights: [0, 0, 0, 0, 0, 'x'] })).toThrow(
			'neither a number nor null'
		);
		expect(parseGround({ ...GRID, nx: 0, ny: 0, heights: [] }).heights).toEqual([]);
	});
});

/** Little-endian float32 triplets, as Gerardus sends them. */
function scanBytes(points: number[]): ArrayBuffer {
	const buf = new ArrayBuffer(points.length * 4);
	const view = new DataView(buf);
	points.forEach((v, i) => view.setFloat32(i * 4, v, true));
	return buf;
}

function binaryResponse(buf: ArrayBuffer, headers: Record<string, string> = {}): Response {
	const map = new Map(Object.entries({ 'content-type': 'application/octet-stream', ...headers }));
	return {
		ok: true,
		status: 200,
		headers: { get: (k: string) => map.get(k.toLowerCase()) ?? null },
		arrayBuffer: async () => buf
	} as unknown as Response;
}

describe('GET the scans', () => {
	const POINTS = [1.5, -2.25, 0.125, 100, 200, 3.75];

	it('reads little-endian x, y, z triplets and the headers', async () => {
		const fetch = jest.fn().mockResolvedValue(
			binaryResponse(scanBytes(POINTS), {
				'x-gerardus-model-id': 'model-7',
				'x-gerardus-point-count': '2',
				'x-gerardus-voxel-m': '0.5'
			})
		);
		const cloud = await getSiteScans(BASE, SITE, 0.5, { fetch });
		const [url, init] = fetch.mock.calls[0];
		expect(url).toBe(`${BASE}/api/v1/sites/${SITE}/scans?voxel_m=0.5`);
		expect(init.headers.Accept).toBe('application/octet-stream');
		expect(Array.from(cloud.points)).toEqual(POINTS);
		expect(cloud).toMatchObject({ count: 2, modelId: 'model-7', voxelM: 0.5 });
	});

	it('reads the bytes as little-endian whatever the machine', () => {
		// 1.0 as float32 is 0x3f800000: little-endian bytes 00 00 80 3f.
		const bytes = new Uint8Array([0, 0, 0x80, 0x3f, 0, 0, 0, 0x40, 0, 0, 0x40, 0x40]);
		expect(Array.from(parseScanPoints(bytes.buffer, null))).toEqual([1, 2, 3]);
		// The same bytes big-endian would be tiny denormals, not 1, 2, 3.
		const big = new Uint8Array([0x3f, 0x80, 0, 0, 0x40, 0, 0, 0, 0x40, 0x40, 0, 0]);
		expect(Array.from(parseScanPoints(big.buffer, '1'))).not.toEqual([1, 2, 3]);
	});

	it('defaults to a 0.2 m voxel, with no model id or count', async () => {
		const fetch = jest.fn().mockResolvedValue(binaryResponse(scanBytes([])));
		const cloud = await getSiteScans(BASE, SITE, undefined, { fetch });
		expect(fetch.mock.calls[0][0]).toBe(`${BASE}/api/v1/sites/${SITE}/scans?voxel_m=0.2`);
		expect(cloud).toMatchObject({ count: 0, modelId: '', voxelM: 0.2 });
	});

	it('takes the voxel Gerardus used over the one asked for', async () => {
		const fetch = jest
			.fn()
			.mockResolvedValue(binaryResponse(scanBytes([]), { 'x-gerardus-voxel-m': '2' }));
		expect((await getSiteScans(BASE, SITE, 5, { fetch })).voxelM).toBe(2);
		expect(voxelUsed(null, 0.2)).toBe(0.2);
		expect(voxelUsed(' ', 0.2)).toBe(0.2);
		expect(voxelUsed('0.05', 0.01)).toBe(0.05);
		expect(() => voxelUsed('fine', 0.2)).toThrow(GerardusError);
		expect(() => voxelUsed('0', 0.2)).toThrow('voxel of “0”');
	});

	it('calls a length that is not whole points malformed', async () => {
		const fetch = jest.fn().mockResolvedValue(binaryResponse(new ArrayBuffer(13)));
		await expect(getSiteScans(BASE, SITE, 0.2, { fetch })).rejects.toMatchObject({
			kind: 'malformed',
			message: expect.stringContaining('13 bytes')
		});
		expect(() => parseScanPoints(new ArrayBuffer(8), null)).toThrow(GerardusError);
	});

	it('calls a point count that disagrees with the bytes malformed', async () => {
		const fetch = jest
			.fn()
			.mockResolvedValue(binaryResponse(scanBytes(POINTS), { 'x-gerardus-point-count': '3' }));
		await expect(getSiteScans(BASE, SITE, 0.2, { fetch })).rejects.toMatchObject({
			kind: 'malformed',
			message: expect.stringContaining('said it sent 3')
		});
		expect(() => parseScanPoints(scanBytes(POINTS), 'two')).toThrow(GerardusError);
		expect(parseScanPoints(scanBytes(POINTS), ' ')).toHaveLength(6);
	});

	it('says scans are not served to this browser on 403', async () => {
		const fetch = jest
			.fn()
			.mockResolvedValue(textResponse('scans are not served to 100.64.0.9', 403));
		await expect(getSiteScans(BASE, SITE, 0.2, { fetch })).rejects.toMatchObject({
			kind: 'forbidden',
			status: 403,
			message: expect.stringContaining('100.64.0.9')
		});
		fetch.mockResolvedValue(textResponse('', 403));
		await expect(getSiteScans(BASE, SITE, 0.2, { fetch })).rejects.toThrow(
			'Gerardus does not serve this to this browser.'
		);
	});

	it('refuses a voxel that is not a size before calling', async () => {
		const fetch = jest.fn();
		for (const v of [0, -1, NaN, Infinity]) {
			await expect(getSiteScans(BASE, SITE, v, { fetch })).rejects.toMatchObject({
				kind: 'rejected'
			});
		}
		expect(fetch).not.toHaveBeenCalled();
	});

	it('calls an unreadable body malformed', async () => {
		const fetch = jest.fn().mockResolvedValue({
			ok: true,
			status: 200,
			headers: new Map(),
			arrayBuffer: async () => {
				throw new Error('reset');
			}
		});
		await expect(getSiteScans(BASE, SITE, 0.2, { fetch })).rejects.toMatchObject({
			kind: 'malformed'
		});
	});
});
