import {
	UNDO_DEPTH,
	addMark,
	canUndo,
	change,
	diffEdits,
	isStreetUseKind,
	markToFeature,
	marksFromFeatures,
	moveVertex,
	newKey,
	pendingEdits,
	removeMark,
	removeVertex,
	reverseMark,
	sameMark,
	setTag,
	startEditing,
	tagsFor,
	undo,
	updateMark,
	type Mark
} from './editor';
import type { SiteFeature } from './gerardus';

const space: SiteFeature = {
	layer: 'street_use',
	id: 'gms:1',
	kind: 'parking_space',
	tags: { space_type: 'metered', count: '3' },
	author: 'Dana',
	editedAt: '2026-10-23T10:00:00Z',
	geometry: {
		type: 'Polygon',
		coordinates: [
			[
				[0, 0, 5],
				[1, 0, 5],
				[1, 1, 5],
				[0, 1, 5],
				[0, 0, 5]
			]
		]
	}
};
const lane: SiteFeature = {
	layer: 'street_use',
	id: 'gms:2',
	kind: 'travel_lane',
	tags: { lane_type: 'bus', width_m: '3.5' },
	geometry: {
		type: 'LineString',
		coordinates: [
			[0, 0],
			[0, 1],
			[0, 2]
		]
	}
};
const driveway: SiteFeature = {
	layer: 'street_use',
	id: 'gms:3',
	kind: 'driveway',
	tags: {},
	geometry: {
		type: 'LineString',
		coordinates: [
			[2, 0],
			[2, 1]
		]
	}
};

function mark(over: Partial<Mark> = {}): Mark {
	return {
		key: newKey(),
		id: '',
		kind: 'driveway',
		points: [
			[5, 5],
			[6, 5]
		],
		tags: {},
		...over
	};
}

describe('marks from Gerardus', () => {
	it('reads spaces, lanes and driveways, opening the space ring', () => {
		const { marks, skipped } = marksFromFeatures([space, lane, driveway]);
		expect(skipped).toBe(0);
		expect(marks.map((m) => m.key)).toEqual(['gms:1', 'gms:2', 'gms:3']);
		expect(marks[0].points).toHaveLength(4);
		expect(marks[0]).toMatchObject({ author: 'Dana', editedAt: '2026-10-23T10:00:00Z' });
		expect(marks[1].tags).toEqual({ lane_type: 'bus', width_m: '3.5' });
		expect(marks[1].author).toBeUndefined();
	});

	it('leaves out other kinds and counts geometry that does not fit', () => {
		const { marks, skipped } = marksFromFeatures([
			{ ...lane, kind: 'kerb' },
			{
				...space,
				geometry: {
					type: 'LineString',
					coordinates: [
						[0, 0],
						[1, 1]
					]
				}
			},
			{ ...lane, geometry: { type: 'Polygon', coordinates: [] } },
			{ ...driveway, geometry: { type: 'LineString', coordinates: [[0, 0]] } },
			{ ...space, geometry: { type: 'Polygon', coordinates: [] } }
		]);
		expect(marks).toEqual([]);
		expect(skipped).toBe(4);
	});

	it('gives a mark without an id a local key', () => {
		const { marks } = marksFromFeatures([{ ...driveway, id: '' }]);
		expect(marks[0].key).toMatch(/^new-/);
		expect(marks[0].id).toBe('');
	});

	it('knows the street-use kinds', () => {
		expect(isStreetUseKind('travel_lane')).toBe(true);
		expect(isStreetUseKind('kerb')).toBe(false);
	});
});

describe('marks as upserts', () => {
	it('closes a space ring and carries kind, id and tags', () => {
		const [m] = marksFromFeatures([space]).marks;
		const f = markToFeature({ ...m, tags: { ...m.tags, time_limit: ' ', kind: 'x', id: 'y' } });
		expect(f.geometry).toEqual(space.geometry);
		expect(f.properties).toEqual({
			kind: 'parking_space',
			id: 'gms:1',
			space_type: 'metered',
			count: '3'
		});
	});

	it('sends a lane as a LineString in its direction of travel, new with an empty id', () => {
		const f = markToFeature(mark({ kind: 'travel_lane', tags: { lane_type: 'bike' } }));
		expect(f.geometry).toEqual({
			type: 'LineString',
			coordinates: [
				[5, 5],
				[6, 5]
			]
		});
		expect(f.properties).toEqual({ kind: 'travel_lane', id: '', lane_type: 'bike' });
	});
});

describe('the batch', () => {
	const base = marksFromFeatures([space, lane, driveway]).marks;

	it('is empty when nothing changed, and ignores height and blank tags', () => {
		expect(diffEdits(base, base)).toEqual([]);
		const copy = base.map((m) => ({
			...m,
			points: m.points.map((p) => [p[0], p[1]] as [number, number])
		}));
		copy[1] = { ...copy[1], tags: { ...copy[1].tags, note: '  ' } };
		expect(diffEdits(base, copy)).toEqual([]);
	});

	it('upserts new and changed marks and removes deleted ones, removes first', () => {
		const moved = moveVertex(base[1], 2, [0.5, 2]);
		const added = mark();
		const edits = diffEdits(base, [base[0], moved, added]);
		expect(edits[0]).toEqual({ remove: 'gms:3' });
		expect(edits).toHaveLength(3);
		expect(edits[1]).toEqual({ upsert: markToFeature(moved) });
		expect(edits[2]).toEqual({ upsert: markToFeature(added) });
	});

	it('compares kind, vertex count, vertices and tags', () => {
		const m = base[2];
		expect(sameMark(m, { ...m, kind: 'travel_lane' })).toBe(false);
		expect(sameMark(m, { ...m, points: [...m.points, [3, 3]] })).toBe(false);
		expect(sameMark(m, moveVertex(m, 0, [2, 0.0001]))).toBe(false);
		expect(sameMark(m, setTag(m, 'surface', 'concrete'))).toBe(false);
		expect(sameMark(m, { ...m, key: 'other' })).toBe(true);
	});
});

describe('edit state and undo', () => {
	const base = marksFromFeatures([lane]).marks;

	it('starts clean, and every change can be undone', () => {
		let s = startEditing(base);
		expect(canUndo(s)).toBe(false);
		expect(pendingEdits(s)).toEqual([]);
		const added = mark();
		s = addMark(s, added);
		s = updateMark(s, added.key, (m) => moveVertex(m, 1, [7, 5]));
		s = removeMark(s, 'gms:2');
		expect(s.marks.map((m) => m.key)).toEqual([added.key]);
		expect(pendingEdits(s)).toHaveLength(2);
		s = undo(s);
		expect(s.marks.map((m) => m.key)).toEqual(['gms:2', added.key]);
		s = undo(s);
		expect(s.marks[1].points[1]).toEqual([6, 5]);
		s = undo(s);
		expect(s.marks).toBe(base);
		expect(undo(s)).toBe(s);
	});

	it('ignores changes to marks that are not there', () => {
		const s = startEditing(base);
		expect(removeMark(s, 'missing')).toBe(s);
		expect(updateMark(s, 'missing', (m) => m)).toBe(s);
	});

	it('keeps a bounded history', () => {
		let s = startEditing(base);
		for (let i = 0; i < UNDO_DEPTH + 20; i++) s = change(s, [...s.marks]);
		expect(s.past).toHaveLength(UNDO_DEPTH);
	});
});

describe('vertex and tag edits', () => {
	const m = mark({
		kind: 'parking_space',
		points: [
			[0, 0, 1],
			[1, 0, 1],
			[1, 1, 1],
			[0, 1, 1]
		]
	});

	it('moves a vertex, dropping its height for Gerardus to drape', () => {
		expect(moveVertex(m, 1, [2, 0]).points[1]).toEqual([2, 0]);
		expect(moveVertex(m, 9, [2, 0])).toBe(m);
	});

	it('removes a vertex only while the kind keeps enough', () => {
		const three = removeVertex(m, 3);
		expect(three.points).toHaveLength(3);
		expect(removeVertex(three, 0)).toBe(three);
		expect(removeVertex(m, -1)).toBe(m);
		const d = mark();
		expect(removeVertex(d, 0)).toBe(d);
	});

	it('reverses a lane without touching the original', () => {
		const l = mark({
			kind: 'travel_lane',
			points: [
				[0, 0],
				[0, 1],
				[0, 2]
			]
		});
		expect(reverseMark(l).points).toEqual([
			[0, 2],
			[0, 1],
			[0, 0]
		]);
		expect(l.points[0]).toEqual([0, 0]);
	});

	it('starts each kind with the tool’s fields', () => {
		const fields = {
			spaceType: 'permit',
			count: 4.4,
			timeLimit: ' 2h ',
			laneType: 'bike',
			widthM: 1.8
		};
		expect(tagsFor('parking_space', fields)).toEqual({
			space_type: 'permit',
			count: '4',
			time_limit: '2h'
		});
		expect(tagsFor('parking_space', { ...fields, count: 0, timeLimit: '' })).toEqual({
			space_type: 'permit',
			count: '1'
		});
		expect(tagsFor('travel_lane', fields)).toEqual({ lane_type: 'bike', width_m: '1.8' });
		expect(tagsFor('travel_lane', { ...fields, widthM: NaN })).toEqual({ lane_type: 'bike' });
		expect(tagsFor('driveway', fields)).toEqual({});
	});
});
