import type { PlanProgress, WorkspaceRow, WorkspaceSegment } from './labellingWorkspace';
import type { SegmentSelector } from './segments';
import {
	boundPlan,
	drawText,
	frozenText,
	heldOutSelectors,
	otherHeldOut,
	progressText,
	rowsBySegment,
	toneClass,
	windowState
} from './windows';

function segment(id: string, extra: Partial<WorkspaceSegment> = {}): WorkspaceSegment {
	return {
		segment_id: id,
		run_id: 'run-a',
		clip_id: `clip-${id}`,
		role: 'held_out',
		finder: 'random',
		held_out_eligible: true,
		capture: 'kirk0.pcapng',
		window_start_ns: '1764972000000000000',
		window_end_ns: '1764972010000000000',
		...extra
	};
}

function row(key: string, extra: Partial<WorkspaceRow> = {}): WorkspaceRow {
	return {
		key,
		kind: 'pack',
		title: key,
		stage: 'unlabelled',
		flags: [],
		protected: false,
		cohort: 'development',
		roles: ['selection:held_out'],
		captures: [],
		runs: ['run-a'],
		segments: [],
		packs: [],
		splits: [],
		workflow: [],
		tasks: [],
		...extra
	};
}

function progress(objects: number, reviewed: number, proposed: number) {
	return {
		objects,
		reviewed_objects: reviewed,
		proposed_objects: proposed,
		masks: 0,
		reviewed_masks: 0,
		proposed_masks: 0,
		freeze_blockers: [],
		layers: []
	};
}

function selector(id: string, finder: string, heldOut: boolean, seed?: number): SegmentSelector {
	return {
		id,
		label: id,
		category: 'c',
		description: '',
		finder,
		parameters:
			seed === undefined ? { window_seconds: 10 } : { window_seconds: 10, random_seed: seed },
		score: { measure: 'draw', order: 'descending' },
		require: [],
		digest: `sha256:${id}`,
		held_out: heldOut
	};
}

describe('windowState', () => {
	it('uses the workspace stage once the label stores know the window', () => {
		expect(windowState('packed', row('p', { stage: 'frozen' }))).toEqual({
			state: 'frozen',
			tone: 'sealed'
		});
		expect(windowState('packed', row('p', { stage: 'ready_to_freeze' }))).toEqual({
			state: 'reviewed',
			tone: 'success'
		});
		expect(windowState('packed', row('p', { stage: 'in_review' })).state).toBe('in review');
	});

	it("keeps the ranking's word while the stage says nothing more", () => {
		expect(windowState('candidate')).toEqual({ state: 'candidate', tone: 'neutral' });
		expect(windowState('case', row('p'))).toEqual({ state: 'clip made', tone: 'progress' });
		expect(windowState('clipping').state).toBe('packing');
	});
});

describe('toneClass', () => {
	it('gives in-progress amber and success green, after DESIGN.md §3.2', () => {
		expect(toneClass('progress')).toContain('amber');
		expect(toneClass('success')).toContain('green');
		expect(toneClass('sealed')).toContain('bg-green-700');
		expect(toneClass('neutral')).toContain('border');
	});
});

describe('progressText', () => {
	it('names the split a frozen window is sealed in', () => {
		const frozen = row('p', {
			stage: 'frozen',
			splits: [
				{
					name: 'embarcadero-heldout-r1',
					split_digest: 'sha256:x',
					revision: 1,
					pack_digest: 'sha256:y',
					pinned_revision: 1,
					head_revision: 1,
					roles: ['held_out'],
					head_moved: false
				}
			]
		});
		expect(progressText('packed', frozen)).toBe('sealed in embarcadero-heldout-r1');
		expect(progressText('packed', row('p', { stage: 'frozen' }))).toBe('sealed');
	});

	it('counts reviewed objects, then proposals', () => {
		expect(progressText('packed', row('p', { progress: progress(7, 4, 3) }))).toBe(
			'4 of 7 objects reviewed'
		);
		expect(progressText('packed', row('p', { progress: progress(0, 0, 5) }))).toBe(
			'5 proposals ready'
		);
	});

	it('says how far a window without review state has got', () => {
		expect(progressText('candidate')).toBe('not selected');
		expect(progressText('case')).toBe('selected, pack not cut');
		expect(progressText('clipping')).toBe('cutting the pack');
		expect(progressText('packed')).toBe('pack cut');
	});
});

describe('heldOutSelectors and drawText', () => {
	const catalogue = [
		selector('following', 'following', true),
		selector('exposure', 'exposure', true),
		selector('random', 'random', true, 1),
		selector('leader_changes', 'leader_changes', false)
	];

	it('offers only the random draw for a held-out pack', () => {
		expect(heldOutSelectors(catalogue).map((s) => s.id)).toEqual(['random']);
	});

	it('reads out the draw with its seed', () => {
		expect(drawText(catalogue[2], 4)).toBe('Random · 4 windows · seed 1');
		expect(drawText(catalogue[2], 1)).toBe('Random · 1 window · seed 1');
		expect(drawText(selector('r', 'random', true), 2)).toBe('Random · 2 windows');
		expect(drawText(undefined, 0)).toBe('No random selector is configured');
	});
});

describe('rowsBySegment', () => {
	it('prefers a pack row to the bare segment row for the same window', () => {
		const bare = row('segment:w1', { kind: 'segment', segments: [segment('w1')] });
		const pack = row('pack:w1', { segments: [segment('w1')] });
		expect(rowsBySegment([pack, bare]).get('w1')?.key).toBe('pack:w1');
		expect(rowsBySegment([bare, pack]).get('w1')?.key).toBe('pack:w1');
	});
});

describe('otherHeldOut', () => {
	it("lists the run's held-out windows the draw does not, once each", () => {
		const items = [
			row('a', { segments: [segment('w1'), segment('w2', { finder: 'exposure' })] }),
			row('b', { segments: [segment('w2', { finder: 'exposure' })] }),
			row('c', { segments: [segment('w3', { role: 'tuning' })] }),
			row('d', { segments: [segment('w4', { run_id: 'run-b' })] })
		];
		expect(otherHeldOut(items, 'run-a', new Set(['w1'])).map((s) => s.segment_id)).toEqual(['w2']);
	});
});

describe('frozenText and boundPlan', () => {
	it('counts frozen windows over every listed window', () => {
		expect(
			frozenText([
				{ state: 'frozen', tone: 'sealed' },
				{ state: 'labelling', tone: 'progress' },
				{ state: 'candidate', tone: 'neutral' }
			])
		).toBe('1 of 3 frozen');
	});

	it('finds the plan bound to a row', () => {
		const plan = {
			id: 'label0001',
			binding: { plan_id: 'label0001', item_key: 'pack:w1' }
		} as unknown as PlanProgress;
		expect(boundPlan([plan], 'pack:w1')?.id).toBe('label0001');
		expect(boundPlan([plan], 'pack:w2')).toBeUndefined();
	});
});
