import {
	keepSelector,
	requirementText,
	scoreText,
	segmentDetail,
	segmentQuery,
	selectorGroups,
	type SegmentSelector
} from './segments';

function selector(
	id: string,
	category: string,
	heldOut: boolean,
	extra: Partial<SegmentSelector> = {}
): SegmentSelector {
	return {
		id,
		label: id,
		category,
		description: `${id} described`,
		finder: 'following',
		parameters: { window_seconds: 10 },
		score: { measure: 'pair_frames', order: 'descending' },
		require: [],
		digest: `sha256:${id}`,
		held_out: heldOut,
		...extra
	};
}

const catalogue = [
	selector('following', 'Traffic', true),
	selector('close_following', 'Traffic', false),
	selector('random', 'Random', true),
	selector('leader_changes', 'Tracker failure', false),
	selector('exposure', 'Traffic', true)
];

describe('selectorGroups', () => {
	it('groups every selector for tuning, categories in the order the file first uses them', () => {
		const groups = selectorGroups(catalogue, 'tuning');
		expect(groups.map((group) => group.category)).toEqual(['Traffic', 'Random', 'Tracker failure']);
		expect(groups[0].selectors.map((entry) => entry.id)).toEqual([
			'following',
			'close_following',
			'exposure'
		]);
	});

	it('offers only the selectors the server allows for held-out windows', () => {
		const groups = selectorGroups(catalogue, 'held_out');
		expect(groups.map((group) => group.category)).toEqual(['Traffic', 'Random']);
		expect(groups.flatMap((group) => group.selectors.map((entry) => entry.id))).toEqual([
			'following',
			'exposure',
			'random'
		]);
	});

	it('has nothing to offer when there are no selectors', () => {
		expect(selectorGroups([], 'tuning')).toEqual([]);
	});
});

describe('keepSelector', () => {
	it('keeps the current selector when the role still allows it', () => {
		expect(keepSelector('exposure', selectorGroups(catalogue, 'held_out'))).toBe('exposure');
	});

	it('falls back to the first the role allows', () => {
		expect(keepSelector('close_following', selectorGroups(catalogue, 'held_out'))).toBe(
			'following'
		);
	});

	it('names none when the role allows none', () => {
		expect(keepSelector('following', [])).toBe('');
	});
});

describe('segmentQuery', () => {
	it('names the run, the selector and the role', () => {
		expect(segmentQuery('run 1', 'close_following', 'tuning')).toBe(
			'run_id=run+1&selector=close_following&role=tuning'
		);
	});
});

describe('requirementText and scoreText', () => {
	it('says what a selector requires', () => {
		const bounded = selector('x', 'Traffic', false, {
			require: [
				{ measure: 'pair_seconds', min: 2, max: null },
				{ measure: 'pairs', min: null, max: 4 },
				{ measure: 'leaders', min: 1, max: 3 }
			]
		});
		expect(requirementText(bounded)).toBe('pair_seconds ≥ 2, pairs ≤ 4, 1 ≤ leaders ≤ 3');
		expect(requirementText(catalogue[0])).toBe('');
	});

	it('says which way a selector ranks', () => {
		expect(scoreText(catalogue[0])).toBe('pair_frames, largest first');
		expect(
			scoreText(
				selector('x', 'Traffic', false, { score: { measure: 'closest_gap_m', order: 'ascending' } })
			)
		).toBe('closest_gap_m, smallest first');
	});
});

describe('segmentDetail', () => {
	it('shows the following measures for a following pass', () => {
		const measures = {
			pair_seconds: 3,
			pairs: 1,
			leaders: 1,
			leader_changes: 0,
			closest_gap_m: 10
		};
		expect(segmentDetail(measures, 'following')).toBe(
			'3 pair-s · 1 pairs · 1 leaders · 0 changes · 10 m'
		);
		expect(segmentDetail({}, 'leader_changes')).toBe(
			'0 pair-s · 0 pairs · 0 leaders · 0 changes · 0 m'
		);
	});

	it('shows observations for any other finder', () => {
		expect(segmentDetail({ events: 5 }, 'exposure')).toBe('5 observations');
		expect(segmentDetail({}, 'random')).toBe('0 observations');
	});
});
