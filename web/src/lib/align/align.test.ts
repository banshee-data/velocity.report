import { searchPlaces, NOMINATIM_URL } from './osm';
import {
	fromLocal,
	headingFromNorthAzimuth,
	movePose,
	northAzimuthFromHeading,
	sensorToEN,
	toLocal,
	wrapDegrees
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

	it('moves a pose and wraps its heading', () => {
		const pose = { ...origin, headingDeg: 350 };
		const moved = movePose(pose, 3, 4, 20);
		expect(moved.headingDeg).toBe(10);
		const at = toLocal(origin, moved.lat, moved.lon);
		close(at.east, 3, 1e-6);
		close(at.north, 4, 1e-6);
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
});
