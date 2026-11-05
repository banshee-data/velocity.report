import {
	bindPlan,
	handoff,
	nextTask,
	planSummary,
	stepStatus,
	type PlanProgress,
	cohortText,
	flagText,
	freezeSplit,
	loadItem,
	previewFreeze,
	shortDigest,
	tbdText,
	whereText,
	workflowFor,
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

	it('names flags, places, TBD reasons and cohorts, passing unknown values through', () => {
		expect(flagText('stale_revision')).toBe('Stale revision');
		expect(whereText('native')).toBe('macOS annotation window');
		expect(tbdText('unassigned_purpose')).toBe('Purpose unassigned');
		expect(cohortText('unassigned')).toBe('Unassigned (TBD)');
		for (const fn of [flagText, whereText, tbdText, cohortText, stageText])
			expect(fn('novel')).toBe('novel');
		expect(shortDigest('sha256:0123456789abcdef')).toBe('01234567');
		expect(shortDigest(undefined)).toBe('');
	});

	it("finds a row's workflow item by layer", () => {
		const item = { layer: 'spans', state: 'tbd' } as WorkspaceRow['workflow'][number];
		expect(workflowFor(row({ workflow: [item] }), 'spans')).toBe(item);
		expect(workflowFor(row(), 'masks')).toBeUndefined();
	});

	it('requests an item, a preview and a freeze at the canonical endpoints', async () => {
		const fetchFn = mockFetch(200, { would_freeze: true });
		await loadItem('label-set:a b');
		await previewFreeze({ d: 1 });
		await freezeSplit({ d: 1 }, 'ana', 'x-r1.json');
		const urls = fetchFn.mock.calls.map((c) => c[0]);
		expect(urls).toEqual([
			'/api/lidar/workspace/item?key=label-set%3Aa+b',
			'/api/lidar/annotations/split/preview',
			'/api/lidar/annotations/split/freeze'
		]);
		expect(JSON.parse(fetchFn.mock.calls[2][1].body)).toEqual({
			draft: { d: 1 },
			author: 'ana',
			output: 'x-r1.json'
		});
	});

	it('refuses an object review for a row with no label set', async () => {
		await expect(
			reviewObject(row({ label_set: undefined }), 'a', 'reviewed', 'ana')
		).rejects.toThrow('no label set');
	});

	it('says what to open in VelocityVisualiser, or why nothing yet', () => {
		const views = row({
			packs: [
				{
					handle: 'k/a/views/run-a',
					pack_digest: 'a',
					dataset_id: 'd',
					run: 'run-a',
					abs_dir: '/p/a',
					vrlog: '/v/a'
				},
				{
					handle: 'k/a/views/run-b',
					pack_digest: 'b',
					dataset_id: 'd',
					run: 'run-b',
					abs_dir: '/p/b'
				}
			]
		});
		const h = handoff(views);
		expect(h).toMatchObject({ kind: 'pack', shared: true });
		if (h.kind === 'pack') expect(h.packs.map((p) => p.path)).toEqual(['/p/a', '/p/b']);
		expect(handoff(row({ protected: true }))).toMatchObject({ kind: 'none' });
		const segment = {
			segment_id: 's',
			clip_id: 'c',
			role: 'held_out',
			finder: 'random',
			held_out_eligible: true,
			capture: 'x',
			window_start_ns: '0',
			window_end_ns: '0'
		};
		expect(handoff(row({ packs: [], segments: [segment] }))).toMatchObject({
			kind: 'none',
			reason: expect.stringContaining('queue the clip')
		});
		expect(
			handoff(row({ packs: [], segments: [{ ...segment, pack_missing: true }] }))
		).toMatchObject({
			reason: expect.stringContaining('not on disk')
		});
	});

	it('summarises a plan from its steps, and names the next unblocked task', () => {
		const plan = {
			steps_done: 1,
			progress: [{ status: 'done' }, { status: 'in_progress' }, { status: 'not_started' }]
		} as PlanProgress;
		expect(planSummary(plan)).toBe('1 of 3 steps done');
		expect(stepStatus('blocked')).toEqual({ mark: '!', text: 'Blocked' });
		expect(stepStatus('odd').text).toBe('odd');
		const blocked = {
			layer: 'purpose',
			action: 'assign',
			blocked: true
		} as WorkspaceRow['tasks'][number];
		const open = {
			layer: 'spans',
			action: 'record',
			blocked: false
		} as WorkspaceRow['tasks'][number];
		expect(nextTask(row({ tasks: [blocked, open] }))?.action).toBe('record');
		expect(nextTask(row({ tasks: [blocked] }))?.action).toBe('assign');
		expect(nextTask(row())).toBeUndefined();
	});

	it('binds a plan against the revision loaded', async () => {
		const fetchFn = mockFetch(200, { revision: 2 });
		await bindPlan('label0003', 'label-set:abc', 1, 'ana');
		const [url, init] = fetchFn.mock.calls[0];
		expect(url).toBe('/api/lidar/workspace/plan-binding');
		expect(JSON.parse(init.body)).toEqual({
			plan_id: 'label0003',
			item_key: 'label-set:abc',
			note: '',
			bound_by: 'ana',
			base_revision: 1
		});
	});
});
