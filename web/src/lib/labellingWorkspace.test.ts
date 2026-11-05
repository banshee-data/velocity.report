import {
	countFraction,
	countText,
	evaluationText,
	freezeDraft,
	loadWorkspace,
	nsToISO,
	previewProblems,
	reviewObject,
	roleText,
	saveWorkflow,
	stageText,
	workspaceQuery,
	WorkspaceError,
	type ObjectSpan,
	type WorkspaceRow
} from './labellingWorkspace';

function row(extra: Partial<WorkspaceRow> = {}): WorkspaceRow {
	return {
		key: 'label-set:abc',
		kind: 'label_set',
		title: 'kirk0-reference',
		stage: 'in_review',
		flags: [],
		protected: false,
		cohort: 'unassigned',
		roles: [],
		captures: [],
		runs: [],
		segments: [],
		packs: [
			{ handle: 'kirk0/packs/run-a', pack_digest: 'sha256:aa', dataset_id: 'd', run: 'run-a' }
		],
		label_set: {
			id: 'abc',
			name: 'kirk0-reference',
			revision: 7,
			state_digest: 'sha256:ff',
			dir: 'kirk0/x'
		},
		splits: [],
		workflow: [],
		tasks: [],
		...extra
	};
}

function mockFetch(status: number, body: unknown) {
	const fn = jest.fn().mockResolvedValue({ ok: status < 400, status, json: async () => body });
	(globalThis as { fetch: unknown }).fetch = fn;
	return fn;
}

describe('labellingWorkspace', () => {
	it('says counts with their denominator, and an unknown total as unknown', () => {
		expect(countText({ name: 'masks reviewed', done: 42, total: 80, unit: 'masks' })).toBe(
			'42 of 80 masks: masks reviewed'
		);
		expect(countText({ name: 'road users recorded', done: 12, total: null, unit: 'objects' })).toBe(
			'12 objects: road users recorded (total unknown)'
		);
		expect(countFraction({ name: 'x', done: 3, total: null, unit: 'u' })).toBeNull();
		expect(countFraction({ name: 'x', done: 0, total: 0, unit: 'u' })).toBeNull();
		expect(countFraction({ name: 'x', done: 1, total: 4, unit: 'u' })).toBe(0.25);
	});

	it('keeps nanosecond times exact as strings and converts them without a number', () => {
		const ns = '1762000000123456789';
		expect(Number(ns)).not.toBe(1762000000123456789n);
		expect(nsToISO(ns)).toBe('2025-11-01T12:26:40.123Z');
		expect(nsToISO('0')).toBe('');
		expect(nsToISO('not a number')).toBe('');
	});

	it('builds a query from the filters that are set', () => {
		expect(workspaceQuery({ stage: 'in_review', q: '', flag: 'tbd', limit: 50, offset: 0 })).toBe(
			'stage=in_review&flag=tbd&limit=50&offset=0'
		);
	});

	it('names roles, stages and evaluation apart from the cohort', () => {
		expect(roleText('split:held_out')).toBe('split: held out');
		expect(roleText('selection:tuning')).toBe('selected for tuning');
		expect(stageText('protected')).toBe('Protected (counts only)');
		expect(evaluationText(row())).toBe('Unknown');
		expect(
			evaluationText(
				row({
					evaluation: {
						purpose: 'development_evaluation',
						held_out_score: true,
						allowed: false,
						reasons: []
					}
				})
			)
		).toBe('Held-out score: refused');
	});

	it('drafts a freeze over every unrejected object and every sample', () => {
		const objects = [
			{ object_id: 'a', status: 'reviewed' },
			{ object_id: 'b', status: 'rejected' }
		] as ObjectSpan[];
		const draft = freezeDraft(row(), objects, 'held_out', 3);
		expect(draft.packs[0].dir).toBe('kirk0/packs/run-a');
		expect(draft.packs[0].splits[0]).toEqual({
			name: 'held_out',
			role: 'held_out',
			object_ids: ['a']
		});
		expect(draft.packs[0].episodes[0].frame_intervals[0]).toEqual({
			first_sample: 0,
			last_sample: 2
		});
		expect(
			previewProblems({
				membership_problems: ['m'],
				physical_problems: ['p'],
				refusal: 'r',
				would_freeze: false
			})
		).toEqual(['m', 'p', 'r']);
	});

	it('sends a review against the revision on screen, and surfaces a stale refusal', async () => {
		const fetchFn = mockFetch(409, { error: 'the label set is at revision 8', code: 'conflict' });
		await expect(reviewObject(row(), 'car-1', 'reviewed', 'ana')).rejects.toMatchObject({
			status: 409,
			message: 'the label set is at revision 8'
		});
		const [url, init] = fetchFn.mock.calls[0];
		expect(url).toBe('/api/lidar/annotations/objects/review');
		expect(JSON.parse(init.body)).toMatchObject({
			base_revision: 7,
			base_state: 'sha256:ff',
			status: 'reviewed'
		});
		expect(init.headers['X-Velocity-Actor']).toBe('ana');
	});

	it('saves workflow against a base revision and drops a TBD reason from other states', async () => {
		const fetchFn = mockFetch(200, { revision: 3 });
		await saveWorkflow(
			{
				item_key: 'label-set:abc',
				layer: 'spans',
				state: 'blocked',
				blocker: 'capture missing',
				tbd_reason: 'other'
			},
			2,
			'bo'
		);
		const body = JSON.parse(fetchFn.mock.calls[0][1].body);
		expect(body).toMatchObject({
			base_revision: 2,
			state: 'blocked',
			tbd_reason: '',
			updated_by: 'bo'
		});
	});

	it('reports a failed inventory as an error, never as an empty workspace', async () => {
		mockFetch(500, { error: 'disk unreadable' });
		const failure = loadWorkspace({}).catch((e) => e);
		await expect(failure).resolves.toBeInstanceOf(WorkspaceError);
		await expect(failure).resolves.toMatchObject({ status: 500, message: 'disk unreadable' });
	});
});
