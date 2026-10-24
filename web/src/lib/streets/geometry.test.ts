import type { EN } from '../align/pose';
import {
	bearing,
	boundsOf,
	closestOnSegment,
	enOf,
	laneArrows,
	laneDirection,
	nearestOn,
	openRing,
	partsOf,
	positionOf,
	rowDividers,
	rowOfSpaces,
	sideOf,
	snapPoint,
	streetSide,
	targetsOf,
	type SnapTarget
} from './geometry';

const en = (east: number, north: number): EN => ({ east, north });
const near = (a: EN, b: EN, tol = 1e-9) => {
	expect(a.east).toBeCloseTo(b.east, -Math.log10(tol));
	expect(a.north).toBeCloseTo(b.north, -Math.log10(tol));
};

const kerb: SnapTarget = {
	id: 'k1',
	layer: 'kerbs',
	points: [en(0, 0), en(20, 0)],
	closed: false
};
const box: SnapTarget = {
	id: 'b1',
	layer: 'boxes',
	points: [en(0, 5), en(10, 5), en(10, 15), en(0, 15)],
	closed: true
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

	it('makes snap targets in metres from a feature', () => {
		const a = positionOf(origin, en(0, 0));
		const b = positionOf(origin, en(10, 0));
		const [t] = targetsOf(origin, {
			id: 'c1',
			layer: 'city_streets',
			geometry: { type: 'LineString', coordinates: [a, b] }
		});
		expect(t.id).toBe('c1');
		expect(t.layer).toBe('city_streets');
		near(t.points[1], en(10, 0), 1e-6);
	});
});

describe('snapping', () => {
	it('finds the nearest point on a segment, clamped to its ends', () => {
		expect(closestOnSegment(en(5, 3), en(0, 0), en(10, 0))).toEqual({
			point: en(5, 0),
			t: 0.5,
			distance: 3
		});
		expect(closestOnSegment(en(-4, 3), en(0, 0), en(10, 0)).point).toEqual(en(0, 0));
		expect(closestOnSegment(en(1, 1), en(2, 2), en(2, 2)).t).toBe(0);
	});

	it('snaps to a kerb within half a metre and not beyond', () => {
		const s = snapPoint(en(7, 0.4), [kerb]);
		expect(s.on).toBe('edge');
		near(s.point, en(7, 0));
		expect(s.distance).toBeCloseTo(0.4);
		expect(s.target?.id).toBe('k1');

		const free = snapPoint(en(7, 0.6), [kerb]);
		expect(free).toEqual({ point: en(7, 0.6), on: 'none', distance: 0 });
	});

	it('prefers a vertex in reach over a nearer edge', () => {
		// 0.3 m from the kerb's end, 0.1 m from the kerb line itself.
		const s = snapPoint(en(19.72, 0.1), [kerb]);
		expect(s.on).toBe('vertex');
		expect(s.point).toEqual(en(20, 0));
	});

	it('picks the nearest of several targets, including a closed ring', () => {
		// 0.3 m from the box's closing edge (x = 0), 5 m from the kerb.
		const s = snapPoint(en(0.3, 10), [kerb, box]);
		expect(s.on).toBe('edge');
		expect(s.target?.id).toBe('b1');
		near(s.point, en(0, 10));
	});

	it('leaves out the mark being edited', () => {
		expect(snapPoint(en(7, 0.2), [kerb], 0.5, 'k1').on).toBe('none');
		expect(snapPoint(en(0.1, 0.1), [kerb, box], 0.5, 'k1').on).toBe('none');
	});

	it('honours a different tolerance', () => {
		expect(snapPoint(en(7, 0.8), [kerb], 1).on).toBe('edge');
	});

	it('finds the nearest point however far, or nothing', () => {
		expect(nearestOn(en(5, 9), [kerb])).toEqual({ point: en(5, 0), distance: 9 });
		const point: SnapTarget = {
			id: 'i',
			layer: 'city_intersections',
			points: [en(3, 4)],
			closed: false
		};
		expect(nearestOn(en(0, 0), [point, kerb])).toEqual({ point: en(0, 0), distance: 0 });
		expect(nearestOn(en(0, 10), [point])?.distance).toBeCloseTo(Math.hypot(3, 6));
		expect(nearestOn(en(0, 0), [])).toBeNull();
	});
});

describe('a row of spaces along the kerb', () => {
	it('tells left from right of a span', () => {
		expect(sideOf(en(0, 0), en(10, 0), en(5, 3))).toBe(1);
		expect(sideOf(en(0, 0), en(10, 0), en(5, -3))).toBe(-1);
		expect(sideOf(en(0, 0), en(10, 0), en(5, 0))).toBe(1);
	});

	it('puts the row on the street side, towards the nearest centreline', () => {
		const north: SnapTarget = {
			id: 'c',
			layer: 'city_streets',
			points: [en(-50, 6), en(50, 6)],
			closed: false
		};
		const south: SnapTarget = {
			id: 'd',
			layer: 'city_streets',
			points: [en(-50, -30), en(50, -30)],
			closed: false
		};
		expect(streetSide(en(0, 0), en(12, 0), [north, south])).toBe(1);
		expect(streetSide(en(12, 0), en(0, 0), [north, south])).toBe(-1);
		expect(streetSide(en(0, 0), en(12, 0), [])).toBe(1);
	});

	it('divides a kerb span into equal spaces of the given depth', () => {
		const row = rowOfSpaces(en(0, 0), en(18, 0), 3, 2.4, 1)!;
		expect(row.spaceLength).toBeCloseTo(6);
		expect(row.ring).toEqual([en(0, 0), en(18, 0), en(18, 2.4), en(0, 2.4)]);
		expect(row.dividers).toHaveLength(2);
		near(row.dividers[0][0], en(6, 0));
		near(row.dividers[0][1], en(6, 2.4));
		near(row.dividers[1][0], en(12, 0));
	});

	it('goes to the right on side −1 and follows a slanted kerb', () => {
		const row = rowOfSpaces(en(0, 0), en(6, 8), 2, 2, -1)!;
		expect(row.spaceLength).toBeCloseTo(5);
		// Right of (0.6, 0.8) is (0.8, −0.6).
		near(row.ring[2], en(6 + 1.6, 8 - 1.2));
		near(row.ring[3], en(1.6, -1.2));
		near(row.dividers[0][0], en(3, 4));
	});

	it('refuses a span with no length, a bad count or depth', () => {
		expect(rowOfSpaces(en(1, 1), en(1, 1), 2, 2, 1)).toBeNull();
		expect(rowOfSpaces(en(0, 0), en(5, 0), 0, 2, 1)).toBeNull();
		expect(rowOfSpaces(en(0, 0), en(5, 0), 1.5, 2, 1)).toBeNull();
		expect(rowOfSpaces(en(0, 0), en(5, 0), 2, 0, 1)).toBeNull();
	});

	it('draws the same dividers from the outline the row was saved as', () => {
		const row = rowOfSpaces(en(0, 0), en(18, 0), 3, 2.4, 1)!;
		const dividers = rowDividers(row.ring, 3);
		expect(dividers).toHaveLength(2);
		dividers.forEach((d, i) => {
			near(d[0], row.dividers[i][0]);
			near(d[1], row.dividers[i][1]);
		});
		expect(rowDividers(row.ring, 1)).toEqual([]);
		expect(rowDividers([en(0, 0), en(1, 0), en(1, 1)], 3)).toEqual([]);
	});
});

describe('a lane direction', () => {
	it('measures bearings clockwise from north', () => {
		expect(bearing(en(0, 0), en(0, 1))).toBeCloseTo(0);
		expect(bearing(en(0, 0), en(1, 0))).toBeCloseTo(90);
		expect(bearing(en(0, 0), en(0, -1))).toBeCloseTo(180);
		expect(bearing(en(0, 0), en(-1, 0))).toBeCloseTo(270);
	});

	it('runs from the first vertex drawn to the last', () => {
		expect(laneDirection([en(0, 0), en(5, 5), en(10, 0)])).toBeCloseTo(90);
		expect(laneDirection([en(10, 0), en(5, 5), en(0, 0)])).toBeCloseTo(270);
		expect(laneDirection([en(0, 0)])).toBeNull();
		expect(laneDirection([en(2, 2), en(3, 3), en(2, 2)])).toBeNull();
	});

	it('spaces arrows along the lane, each pointing along its segment', () => {
		const arrows = laneArrows([en(0, 0), en(0, 20), en(20, 20)], 10);
		expect(arrows.map((a) => a.bearing.toFixed(0))).toEqual(['0', '0', '90', '90']);
		near(arrows[0].at, en(0, 5));
		near(arrows[2].at, en(5, 20));
	});

	it('gives a short lane one arrow at its middle, and none without length', () => {
		const [only, ...rest] = laneArrows([en(0, 0), en(4, 0)], 10);
		expect(rest).toEqual([]);
		near(only.at, en(2, 0));
		expect(only.bearing).toBeCloseTo(90);
		expect(laneArrows([en(1, 1), en(1, 1)])).toEqual([]);
		expect(laneArrows([en(0, 0), en(4, 0)], 0)).toEqual([]);
	});
});
