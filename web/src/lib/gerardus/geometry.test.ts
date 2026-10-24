import type { EN } from '../align/pose';
import {
	bearing,
	boundsOf,
	distance,
	enOf,
	laneArrows,
	openRing,
	partsOf,
	polygonsOf,
	positionOf
} from './geometry';

const en = (east: number, north: number): EN => ({ east, north });
const near = (a: EN, b: EN, tol = 1e-9) => {
	expect(a.east).toBeCloseTo(b.east, -Math.log10(tol));
	expect(a.north).toBeCloseTo(b.north, -Math.log10(tol));
};

describe('frames', () => {
	const origin = { lat: 37.7978, lon: -122.4065 };

	it('round-trips a position through the local frame', () => {
		const p = positionOf(origin, en(12.5, -7.25));
		near(enOf(origin, p), en(12.5, -7.25), 1e-6);
		expect(enOf(origin, [origin.lon, origin.lat, 12])).toEqual({ east: 0, north: 0 });
	});

	it('opens a closed ring and leaves an open one', () => {
		expect(
			openRing([
				[0, 0],
				[1, 0],
				[0, 0]
			])
		).toEqual([
			[0, 0],
			[1, 0]
		]);
		expect(openRing([[0, 0]])).toEqual([[0, 0]]);
		expect(
			openRing([
				[0, 0],
				[1, 0]
			])
		).toHaveLength(2);
	});

	it('splits every geometry type into lines and rings', () => {
		const p: [number, number] = [1, 2];
		expect(partsOf({ type: 'Point', coordinates: p })).toEqual([{ points: [p], closed: false }]);
		expect(partsOf({ type: 'MultiPoint', coordinates: [p, p] })).toHaveLength(2);
		expect(partsOf({ type: 'LineString', coordinates: [p, [3, 4]] })[0].closed).toBe(false);
		expect(partsOf({ type: 'MultiLineString', coordinates: [[p], [p]] })).toHaveLength(2);
		const ring: [number, number][] = [
			[0, 0],
			[1, 0],
			[1, 1],
			[0, 0]
		];
		expect(partsOf({ type: 'Polygon', coordinates: [ring] })).toEqual([
			{ points: ring.slice(0, 3), closed: true }
		]);
		expect(partsOf({ type: 'MultiPolygon', coordinates: [[ring], [ring, ring]] })).toHaveLength(3);
	});

	it('boxes every vertex of the features', () => {
		expect(
			boundsOf([
				{ geometry: { type: 'Point', coordinates: [-122.41, 37.79, 9] } },
				{
					geometry: {
						type: 'LineString',
						coordinates: [
							[-122.4, 37.8],
							[-122.42, 37.795]
						]
					}
				}
			])
		).toEqual({ west: -122.42, south: 37.79, east: -122.4, north: 37.8 });
		expect(boundsOf([])).toBeNull();
		expect(boundsOf([{ geometry: { type: 'MultiPoint', coordinates: [] } }])).toBeNull();
	});

	it('splits polygons into their outer rings and holes', () => {
		const outer: [number, number][] = [
			[0, 0],
			[4, 0],
			[4, 4],
			[0, 0]
		];
		const hole: [number, number][] = [
			[1, 1],
			[2, 1],
			[2, 2],
			[1, 1]
		];
		expect(polygonsOf({ type: 'Polygon', coordinates: [outer, hole] })).toEqual([
			{ outer: outer.slice(0, 3), holes: [hole.slice(0, 3)] }
		]);
		expect(
			polygonsOf({ type: 'MultiPolygon', coordinates: [[outer], [], [outer, hole]] })
		).toHaveLength(2);
		expect(polygonsOf({ type: 'Polygon', coordinates: [] })).toEqual([]);
		expect(polygonsOf({ type: 'LineString', coordinates: outer })).toEqual([]);
	});
});

describe('a lane direction', () => {
	it('measures bearings clockwise from north', () => {
		expect(bearing(en(0, 0), en(0, 1))).toBeCloseTo(0);
		expect(bearing(en(0, 0), en(1, 0))).toBeCloseTo(90);
		expect(bearing(en(0, 0), en(0, -1))).toBeCloseTo(180);
		expect(bearing(en(0, 0), en(-1, 0))).toBeCloseTo(270);
		expect(distance(en(0, 0), en(3, 4))).toBe(5);
	});

	it('spaces arrows along the lane, each pointing along its segment', () => {
		const arrows = laneArrows([en(0, 0), en(0, 20), en(20, 20)], 10);
		expect(arrows.map((a) => a.bearing.toFixed(0))).toEqual(['0', '0', '90', '90']);
		near(arrows[0].at, en(0, 5));
		near(arrows[2].at, en(5, 20));
		// Where each sits, so a height can be read off the lane's vertices.
		expect(arrows.map((a) => [a.segment, a.t])).toEqual([
			[0, 0.25],
			[0, 0.75],
			[1, 0.25],
			[1, 0.75]
		]);
	});

	it('gives a short lane one arrow at its middle, and none without length', () => {
		const [only, ...rest] = laneArrows([en(0, 0), en(4, 0)], 10);
		expect(rest).toEqual([]);
		near(only.at, en(2, 0));
		expect(only.bearing).toBeCloseTo(90);
		expect(laneArrows([en(1, 1), en(1, 1)])).toEqual([]);
		// A repeated vertex is stepped over, and the segment index still names the original vertex.
		const [past] = laneArrows([en(0, 0), en(0, 0), en(0, 4)], 10);
		expect(past.segment).toBe(1);
		expect(past.t).toBeCloseTo(0.5);
		expect(laneArrows([en(0, 0), en(4, 0)], 0)).toEqual([]);
	});
});
