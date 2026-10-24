import {
	GERARDUS_ADDRESS_KEY,
	GERARDUS_DEFAULT_ADDRESS,
	GerardusError,
	STREET_EDITOR_AUTHOR_KEY,
	STREET_LAYERS,
	gerardusAddress,
	getSiteGeojson,
	groupByLayer,
	isL16Token,
	mapProblems,
	normaliseAddress,
	parseCollection,
	parseFeature,
	postStreetUse,
	rememberAuthor,
	rememberGerardusAddress,
	rememberedAuthor
} from './gerardus';

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

	it('remembers the author, trimmed', () => {
		expect(rememberedAuthor()).toBe('');
		rememberAuthor('  Dana  ');
		expect(localStorage.getItem(STREET_EDITOR_AUTHOR_KEY)).toBe('Dana');
		expect(rememberedAuthor()).toBe('Dana');
		rememberAuthor('');
		expect(rememberedAuthor()).toBe('');
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
		expect(rememberedAuthor(broken)).toBe('');
		expect(() => rememberAuthor('x', broken)).not.toThrow();
		expect(rememberGerardusAddress('http://h:1', broken)).toBe('http://h:1');
		expect(gerardusAddress(undefined)).toBe(GERARDUS_DEFAULT_ADDRESS);
		expect(rememberedAuthor(undefined)).toBe('');
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

describe('problems', () => {
	it('attaches each problem to the feature it names', () => {
		const { byId, general } = mapProblems(
			[
				'gms:01J: space sits in a lane',
				'gms:01J: no kerb within 0.5 m',
				'gms:02K: driveway has no kerb',
				'gms:unknown: lane crosses a kerb',
				'the batch was trimmed',
				'gms:03L:'
			],
			['gms:01J', 'gms:02K', 'gms:03L']
		);
		expect(byId.get('gms:01J')).toEqual(['space sits in a lane', 'no kerb within 0.5 m']);
		expect(byId.get('gms:02K')).toEqual(['driveway has no kerb']);
		expect(byId.get('gms:03L')).toEqual(['gms:03L:']);
		expect(general).toEqual(['gms:unknown: lane crosses a kerb', 'the batch was trimmed']);
	});

	it('matches the longest id when one id begins another', () => {
		const { byId, general } = mapProblems(
			['gms:12: too narrow', 'gms:1: too wide'],
			['gms:1', 'gms:12', '']
		);
		expect(byId.get('gms:12')).toEqual(['too narrow']);
		expect(byId.get('gms:1')).toEqual(['too wide']);
		expect(general).toEqual([]);
	});
});

describe('GET the site layers', () => {
	it('asks for every editor layer and parses the collection', async () => {
		const fetch = jest.fn().mockResolvedValue(jsonResponse(SITE_GEOJSON));
		const { features, skipped } = await getSiteGeojson(BASE, SITE, STREET_LAYERS, { fetch });
		expect(fetch).toHaveBeenCalledTimes(1);
		const [url, init] = fetch.mock.calls[0];
		expect(url).toBe(
			`${BASE}/api/v1/sites/${SITE}/geojson?layers=footprints,boxes,city_streets,city_intersections,kerbs,street_use`
		);
		expect(init.method).toBe('GET');
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
		await expect(getSiteGeojson(BASE, SITE, STREET_LAYERS, { fetch })).rejects.toMatchObject({
			kind: 'unreachable',
			message: expect.stringContaining(`not answering at ${BASE}`)
		});
	});

	it('passes an abort through untouched', async () => {
		const abort = new DOMException('aborted', 'AbortError');
		const fetch = jest.fn().mockRejectedValue(abort);
		await expect(getSiteGeojson(BASE, SITE, STREET_LAYERS, { fetch })).rejects.toBe(abort);
	});

	it('says the site is not prepared on 404', async () => {
		const fetch = jest.fn().mockResolvedValue(jsonResponse({ error: 'no bundle' }, 404));
		await expect(getSiteGeojson(BASE, SITE, STREET_LAYERS, { fetch })).rejects.toMatchObject({
			kind: 'not_prepared',
			status: 404,
			message: expect.stringContaining(`no prepared site ${SITE} (no bundle)`)
		});
		fetch.mockResolvedValue(textResponse('', 404));
		await expect(getSiteGeojson(BASE, SITE, STREET_LAYERS, { fetch })).rejects.toMatchObject({
			message: expect.stringContaining(`no prepared site ${SITE}. Prepare`)
		});
	});

	it('reports other failures with their status and message', async () => {
		const fetch = jest.fn().mockResolvedValue(textResponse('bundle unreadable', 500));
		await expect(getSiteGeojson(BASE, SITE, STREET_LAYERS, { fetch })).rejects.toMatchObject({
			kind: 'server',
			status: 500,
			message: 'Gerardus failed (HTTP 500): bundle unreadable.'
		});
		fetch.mockResolvedValue(textResponse('x'.repeat(400), 502));
		const err = await getSiteGeojson(BASE, SITE, STREET_LAYERS, { fetch }).catch((e) => e);
		expect(err.message.length).toBeLessThan(340);
		fetch.mockResolvedValue({
			ok: false,
			status: 503,
			text: async () => {
				throw new Error('gone');
			}
		});
		await expect(getSiteGeojson(BASE, SITE, STREET_LAYERS, { fetch })).rejects.toMatchObject({
			message: 'Gerardus failed (HTTP 503).'
		});
	});

	it('says so when Gerardus has no street editor API and answers as gRPC', async () => {
		const headers = new Map([
			['content-type', 'application/grpc'],
			['grpc-status', '12']
		]);
		const fetch = jest.fn().mockResolvedValue({ ok: true, status: 200, headers });
		await expect(getSiteGeojson(BASE, SITE, STREET_LAYERS, { fetch })).rejects.toMatchObject({
			kind: 'unsupported',
			message: expect.stringContaining('no street editor API (gRPC status 12)')
		});
		headers.delete('grpc-status');
		await expect(getSiteGeojson(BASE, SITE, STREET_LAYERS, { fetch })).rejects.toMatchObject({
			message: expect.stringContaining('no street editor API. Run')
		});
		const json = new Map([['content-type', 'application/geo+json']]);
		fetch.mockResolvedValue({
			ok: true,
			status: 200,
			headers: json,
			json: async () => ({ type: 'FeatureCollection', features: [] })
		});
		expect((await getSiteGeojson(BASE, SITE, STREET_LAYERS, { fetch })).features).toEqual([]);
	});

	it('refuses an answer that is not JSON, or not GeoJSON', async () => {
		const fetch = jest.fn().mockResolvedValue({
			ok: true,
			status: 200,
			json: async () => {
				throw new SyntaxError('bad');
			}
		});
		await expect(getSiteGeojson(BASE, SITE, STREET_LAYERS, { fetch })).rejects.toMatchObject({
			kind: 'malformed'
		});
		fetch.mockResolvedValue(jsonResponse({ hello: 'world' }));
		await expect(getSiteGeojson(BASE, SITE, STREET_LAYERS, { fetch })).rejects.toMatchObject({
			kind: 'malformed'
		});
	});

	it('refuses a token that is not an S2 cell before calling', async () => {
		const fetch = jest.fn();
		await expect(getSiteGeojson(BASE, '../etc', STREET_LAYERS, { fetch })).rejects.toMatchObject({
			kind: 'rejected'
		});
		expect(fetch).not.toHaveBeenCalled();
	});
});

describe('POST street use', () => {
	const upsert = {
		type: 'Feature' as const,
		geometry: {
			type: 'LineString' as const,
			coordinates: [
				[-122.4064, 37.7978],
				[-122.4063, 37.7978]
			] as [number, number][]
		},
		properties: { kind: 'driveway', id: '' }
	};

	it('sends the author, note and edits as the contract says', async () => {
		const fetch = jest.fn().mockResolvedValue(
			jsonResponse({
				features: {
					type: 'FeatureCollection',
					features: [
						{
							type: 'Feature',
							geometry: upsert.geometry,
							properties: { id: 'gms:new', kind: 'driveway', author: 'Dana', edited_at: 't' }
						}
					]
				},
				problems: ['gms:new: driveway has no kerb', 42]
			})
		);
		const result = await postStreetUse(
			BASE,
			SITE,
			{ author: ' Dana ', note: ' first pass ', edits: [{ upsert }, { remove: 'gms:old' }] },
			{ fetch }
		);
		const [url, init] = fetch.mock.calls[0];
		expect(url).toBe(`${BASE}/api/v1/sites/${SITE}/street-use`);
		expect(init.method).toBe('POST');
		expect(init.headers['Content-Type']).toBe('application/json');
		expect(JSON.parse(init.body)).toEqual({
			author: 'Dana',
			note: 'first pass',
			edits: [{ upsert }, { remove: 'gms:old' }]
		});
		expect(result.features).toHaveLength(1);
		expect(result.features[0]).toMatchObject({
			id: 'gms:new',
			layer: 'street_use',
			author: 'Dana'
		});
		expect(result.problems).toEqual(['gms:new: driveway has no kerb', '42']);
	});

	it('treats missing problems as none', async () => {
		const fetch = jest
			.fn()
			.mockResolvedValue(jsonResponse({ features: { type: 'FeatureCollection', features: [] } }));
		expect(
			await postStreetUse(BASE, SITE, { author: 'a', note: '', edits: [] }, { fetch })
		).toEqual({
			features: [],
			problems: []
		});
	});

	it('requires an author before sending anything', async () => {
		const fetch = jest.fn();
		await expect(
			postStreetUse(BASE, SITE, { author: '  ', note: '', edits: [] }, { fetch })
		).rejects.toMatchObject({ kind: 'rejected', message: expect.stringContaining('author') });
		expect(fetch).not.toHaveBeenCalled();
	});

	it('shows the server’s message for a 400', async () => {
		const fetch = jest
			.fn()
			.mockResolvedValue(textResponse('upsert 0: kind must be street use', 400));
		await expect(
			postStreetUse(BASE, SITE, { author: 'a', note: '', edits: [] }, { fetch })
		).rejects.toMatchObject({
			kind: 'rejected',
			status: 400,
			message: 'Gerardus refused the request: upsert 0: kind must be street use'
		});
		fetch.mockResolvedValue(jsonResponse({ message: 'author is required' }, 400));
		await expect(
			postStreetUse(BASE, SITE, { author: 'a', note: '', edits: [] }, { fetch })
		).rejects.toThrow('Gerardus refused the request: author is required');
		fetch.mockResolvedValue(textResponse('', 400));
		await expect(
			postStreetUse(BASE, SITE, { author: 'a', note: '', edits: [] }, { fetch })
		).rejects.toThrow('Gerardus refused the request (HTTP 400).');
	});

	it('refuses an answer without the features collection', async () => {
		const fetch = jest.fn().mockResolvedValue(jsonResponse('ok'));
		await expect(
			postStreetUse(BASE, SITE, { author: 'a', note: '', edits: [] }, { fetch })
		).rejects.toMatchObject({ kind: 'malformed' });
		fetch.mockResolvedValue(jsonResponse({ problems: [] }));
		await expect(
			postStreetUse(BASE, SITE, { author: 'a', note: '', edits: [] }, { fetch })
		).rejects.toMatchObject({ kind: 'malformed' });
	});
});
