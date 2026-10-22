import { icp2d, nearestOnSegment, SegmentIndex, solveRigid, thin, type Segment } from './icp';
import {
	fetchOutlines,
	outlineQuery,
	outlineSegments,
	parseOutlines,
	searchPlaces,
	NOMINATIM_URL,
	OVERPASS_URL
} from './osm';
import {
	fromLocal,
	headingFromNorthAzimuth,
	movePose,
	northAzimuthFromHeading,
	placeScan,
	sensorToEN,
	toLocal,
	wrapDegrees,
	type EN
} from './pose';

const origin = { lat: 37.7604, lon: -122.4664 };

function close(a: number, b: number, tol: number) {
	expect(Math.abs(a - b)).toBeLessThanOrEqual(tol);
}

describe('pose', () => {
	it('wraps angles into [0, 360)', () => {
		expect(wrapDegrees(-10)).toBe(350);
		expect(wrapDegrees(370)).toBe(10);
		expect(wrapDegrees(0)).toBe(0);
	});

	it('round-trips the local frame', () => {
		const p = fromLocal(origin, { east: 120, north: -45 });
		const back = toLocal(origin, p.lat, p.lon);
		close(back.east, 120, 1e-6);
		close(back.north, -45, 1e-6);
		close(toLocal(origin, origin.lat + 1 / 111_320, origin.lon).north, 1, 1e-9);
	});

	it('turns the sensor frame by its heading', () => {
		const ahead = sensorToEN(0, 10, 0);
		close(ahead.north, 10, 1e-9);
		close(ahead.east, 0, 1e-9);
		const facingEast = sensorToEN(0, 10, 90);
		close(facingEast.east, 10, 1e-9);
		close(facingEast.north, 0, 1e-9);
		// +X is 90° clockwise of +Y: with +Y north it points east.
		close(sensorToEN(5, 0, 0).east, 5, 1e-9);
	});

	it('converts north azimuth and heading both ways', () => {
		expect(northAzimuthFromHeading(143.5)).toBeCloseTo(36.5);
		expect(headingFromNorthAzimuth(35)).toBe(145);
		expect(headingFromNorthAzimuth(northAzimuthFromHeading(271))).toBe(271);
	});

	it('moves a pose and places a scan with a height band', () => {
		const pose = { ...origin, headingDeg: 350 };
		const moved = movePose(pose, 3, 4, 20);
		expect(moved.headingDeg).toBe(10);
		const at = toLocal(origin, moved.lat, moved.lon);
		close(at.east, 3, 1e-6);
		close(at.north, 4, 1e-6);

		const scan = placeScan(
			[0, 10, 0.5, 0, 10, -2.5, 1, 1],
			{ ...origin, headingDeg: 90 },
			origin,
			-1,
			3
		);
		expect(scan).toHaveLength(1);
		close(scan[0].east, 10, 1e-6);
	});
});

describe('icp', () => {
	const building: Segment[] = [
		{ a: { east: 10, north: -20 }, b: { east: 10, north: 25 } },
		{ a: { east: 10, north: 25 }, b: { east: -30, north: 25 } },
		{ a: { east: -30, north: 25 }, b: { east: -30, north: 5 } },
		{ a: { east: -12, north: -25 }, b: { east: 5, north: -25 } }
	];

	function sample(segments: Segment[], step: number): EN[] {
		const pts: EN[] = [];
		for (const s of segments) {
			const len = Math.hypot(s.b.east - s.a.east, s.b.north - s.a.north);
			for (let d = 0; d <= len; d += step) {
				const t = d / len;
				pts.push({
					east: s.a.east + t * (s.b.east - s.a.east),
					north: s.a.north + t * (s.b.north - s.a.north)
				});
			}
		}
		return pts;
	}

	it('finds the nearest point on a segment', () => {
		const s = { a: { east: 0, north: 0 }, b: { east: 10, north: 0 } };
		expect(nearestOnSegment({ east: 4, north: 3 }, s)).toEqual({ east: 4, north: 0 });
		expect(nearestOnSegment({ east: -4, north: 3 }, s)).toEqual({ east: 0, north: 0 });
		const point = { a: { east: 1, north: 1 }, b: { east: 1, north: 1 } };
		expect(nearestOnSegment({ east: 5, north: 5 }, point)).toEqual({ east: 1, north: 1 });
	});

	it('indexes segments for a gated nearest search', () => {
		const index = new SegmentIndex(building, 2);
		const hit = index.nearest({ east: 11, north: 0 }, 2);
		expect(hit?.distance).toBeCloseTo(1);
		expect(index.nearest({ east: 0, north: 0 }, 2)).toBeNull();
	});

	it('solves an exact rigid motion', () => {
		const from = [
			{ east: 1, north: 0 },
			{ east: 0, north: 2 },
			{ east: -3, north: 1 }
		];
		const theta = 0.1;
		const to = from.map((p) => ({
			east: Math.cos(theta) * p.east - Math.sin(theta) * p.north + 2,
			north: Math.sin(theta) * p.east + Math.cos(theta) * p.north - 1
		}));
		const r = solveRigid(from, to, { east: 0, north: 0 });
		close(r.theta, theta, 1e-9);
		close(r.tEast, 2, 1e-9);
		close(r.tNorth, -1, 1e-9);
	});

	it('recovers a placement that is off by metres and degrees', () => {
		const truth = sample(building, 0.25);
		// The operator's rough placement: the scan rotated 4° clockwise about the
		// sensor and shifted 1.6 m east and 1.2 m south of where it belongs.
		const theta = (-4 * Math.PI) / 180;
		const placed = truth.map((p, i) => {
			const jitter = ((i * 7919) % 11) / 110 - 0.05;
			return {
				east: Math.cos(theta) * p.east - Math.sin(theta) * p.north + 1.6 + jitter,
				north: Math.sin(theta) * p.east + Math.cos(theta) * p.north - 1.2 - jitter
			};
		});
		const sensor = { east: 1.6, north: -1.2 };
		const r = icp2d(placed, building, sensor, { maxDistance: 5 });
		// Undo: rotate back 4° about the sensor (heading −4°... the scan was
		// turned clockwise, so the heading must turn anticlockwise) and shift back.
		close(r.dHeadingDeg, -4, 0.3);
		expect(r.rms).toBeLessThan(0.1);
		expect(r.inliers / r.total).toBeGreaterThan(0.9);
		const moved = placed.map((p) => {
			const t = (-r.dHeadingDeg * Math.PI) / 180;
			const rx = p.east - sensor.east;
			const ry = p.north - sensor.north;
			return {
				east: sensor.east + Math.cos(t) * rx - Math.sin(t) * ry + r.dEast,
				north: sensor.north + Math.sin(t) * rx + Math.cos(t) * ry + r.dNorth
			};
		});
		const index = new SegmentIndex(building, 2);
		const worst = Math.max(...moved.map((p) => index.nearest(p, 5)?.distance ?? 99));
		expect(worst).toBeLessThan(0.15);
	});

	it('reports no fit when nothing is in reach', () => {
		const r = icp2d([{ east: 500, north: 500 }], building, { east: 0, north: 0 });
		expect(r.inliers).toBe(0);
		expect(r.rms).toBe(Infinity);
	});

	it('thins a scan to one point per cell', () => {
		const pts = [
			{ east: 0.1, north: 0.1 },
			{ east: 0.2, north: 0.3 },
			{ east: 1.5, north: 0.1 }
		];
		expect(thin(pts, 1)).toHaveLength(2);
	});
});

describe('osm', () => {
	function fakeFetch(body: unknown, ok = true, status = 200) {
		return jest.fn(async () => ({ ok, status, json: async () => body })) as unknown as typeof fetch;
	}

	it('searches for places and drops unusable rows', async () => {
		const f = fakeFetch([
			{ lat: '37.76', lon: '-122.46', display_name: 'Kirkham St & 9th Ave' },
			{ lat: 'x', lon: '1', display_name: 'broken' }
		]);
		const places = await searchPlaces(' Kirkham and 9th ', f);
		expect(places).toEqual([{ lat: 37.76, lon: -122.46, label: 'Kirkham St & 9th Ave' }]);
		const url = (f as unknown as jest.Mock).mock.calls[0][0] as string;
		expect(url.startsWith(NOMINATIM_URL)).toBe(true);
		expect(url).toContain('q=Kirkham%20and%209th');
		expect(await searchPlaces('   ', f)).toEqual([]);
		await expect(searchPlaces('x', fakeFetch([], false, 429))).rejects.toThrow('429');
	});

	it('builds the outline query and parses the response', async () => {
		const q = outlineQuery(37.7604, -122.4664, 150.4);
		expect(q).toContain('(around:150,37.760400,-122.466400)');
		expect(q).toContain('way["building"]');
		expect(q).toContain('out geom;');

		const body = {
			elements: [
				{
					type: 'way',
					tags: { building: 'yes' },
					geometry: [
						{ lat: 37.76, lon: -122.466 },
						{ lat: 37.7601, lon: -122.466 },
						{ lat: 37.7601, lon: -122.4659 }
					]
				},
				{
					type: 'way',
					tags: { barrier: 'wall' },
					geometry: [origin, { lat: 37.7605, lon: -122.4664 }]
				},
				{ type: 'way', tags: { building: 'yes' }, geometry: [origin] },
				{ type: 'node' }
			]
		};
		const outlines = parseOutlines(body);
		expect(outlines.map((o) => o.kind)).toEqual(['building', 'barrier']);
		expect(outlineSegments(outlines, origin)).toHaveLength(3);
		expect(parseOutlines({})).toEqual([]);

		const f = fakeFetch(body);
		expect(await fetchOutlines(37.7604, -122.4664, 150, f)).toHaveLength(2);
		const [url, init] = (f as unknown as jest.Mock).mock.calls[0];
		expect(url).toBe(OVERPASS_URL);
		expect(init.method).toBe('POST');
		expect(init.body).toContain('data=');
		await expect(fetchOutlines(0, 0, 10, fakeFetch({}, false, 504))).rejects.toThrow('504');
	});
});
