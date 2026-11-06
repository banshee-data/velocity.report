import type { CaptureJob, LaneProposalRun, LaneRerunResponse } from '#lib/types/captures.js';
import type { LidarClip } from '#lib/types/lidar.js';
import {
	activeLaneJobIds,
	canRunLanes,
	describeRerun,
	describeSkipped,
	laneView,
	pickRecord,
	registrationView,
	skipReason
} from './laneProposals';

const run = (over: Partial<LaneProposalRun> = {}): LaneProposalRun => ({
	proposal_run_id: 'lpr-1',
	job_id: 'job-1',
	record_id: 'gms:cb',
	track_count: 40,
	high: 4,
	medium: 2,
	adopted: 1,
	rejected: 0,
	outstanding: 5,
	derive_secs: 0.4,
	created_at_ns: 1_000,
	completed_at_ns: Date.UTC(2026, 10, 6) * 1e6,
	progress_current: 6,
	progress_total: 6,
	state: 'completed',
	...over
});

const clip = (over: Partial<LidarClip> = {}): LidarClip => ({
	clip_id: 'clip-1',
	sensor_id: 'hesai-pandar40p',
	created_at_ns: 1,
	file_count: 1,
	registration: {
		state: 'registered',
		record_id: 'gms:cb',
		record_state: 'registered',
		checked_at: ''
	},
	...over
});

const job = (over: Partial<CaptureJob>): CaptureJob => ({
	job_id: 'job-2',
	kind: 'lane_proposals',
	state: 'running',
	progress_current: 3,
	progress_total: 6,
	queued_at_ns: 1,
	...over
});

describe('registrationView', () => {
	it('ticks a registered clip and names its record', () => {
		const v = registrationView(clip().registration);
		expect(v.kind).toBe('registered');
		expect(v.title).toContain('gms:cb');
	});
	it('says placed by hand for a manual record', () => {
		expect(
			registrationView({
				state: 'registered',
				record_id: 'r',
				record_state: 'manually_placed',
				checked_at: ''
			}).title
		).toContain('by hand');
	});
	it('keeps unknown apart from not registered', () => {
		expect(registrationView({ state: 'unknown', reason: 'refused', checked_at: '' }).label).toBe(
			'unknown'
		);
		expect(
			registrationView({ state: 'not_registered', reason: 'no accepted pose', checked_at: '' }).kind
		).toBe('unregistered');
		expect(registrationView({ state: 'not_located', checked_at: '' }).label).toBe('not located');
		expect(registrationView(undefined).kind).toBe('unknown');
	});
});

describe('laneView', () => {
	it('ticks a current result with its counts', () => {
		const v = laneView(clip({ lanes: { latest: run(), job: run(), current: true } }));
		expect(v.kind).toBe('current');
		expect(v.label).toBe('✓ 4 high · 2 medium');
		expect(v.title).toContain('5 to review');
		expect(v.title).toContain('2026-11-06');
	});
	it('says stale and why', () => {
		const v = laneView(
			clip({
				lanes: {
					latest: run(),
					job: run(),
					current: false,
					stale_reason: 'built by 0.5.1+a, not 0.5.2+b'
				}
			})
		);
		expect(v.kind).toBe('stale');
		expect(v.title).toContain('built by 0.5.1+a');
	});
	it('shows a running job from the poll, fresher than the list', () => {
		const listed = run({
			job_id: 'job-2',
			state: 'queued',
			completed_at_ns: undefined,
			progress_current: 0,
			created_at_ns: 2_000
		});
		const c = clip({ lanes: { latest: run(), job: listed, current: true } });
		const polled = new Map([['job-2', job({ detail: 'uploading first_half (812 cells)' })]]);
		const v = laneView(c, polled);
		expect(v.kind).toBe('running');
		expect(v.fraction).toBe(0.5);
		expect(v.detail).toBe('uploading first_half (812 cells)');
		expect(canRunLanes(c, polled)).toBe(false);
	});
	it('says failed with the first line of the error, keeping the last result', () => {
		const failed = run({
			job_id: 'job-2',
			state: 'failed',
			completed_at_ns: undefined,
			created_at_ns: 2_000,
			error: 'replay: no packets\nmore'
		});
		const v = laneView(clip({ lanes: { latest: run(), job: failed, current: true } }));
		expect(v.kind).toBe('failed');
		expect(v.title).toBe('replay: no packets. Last result: 4 high · 2 medium');
	});
	it('says none before any run', () => {
		expect(
			laneView(clip({ lanes: { current: false, stale_reason: 'no lane proposals yet' } })).kind
		).toBe('none');
	});
});

describe('canRunLanes and activeLaneJobIds', () => {
	it('needs a registered clip, or an ambiguous one the operator will name', () => {
		expect(canRunLanes(clip())).toBe(true);
		expect(
			canRunLanes(
				clip({ registration: { state: 'ambiguous', candidates: ['a', 'b'], checked_at: '' } })
			)
		).toBe(true);
		expect(canRunLanes(clip({ registration: { state: 'not_registered', checked_at: '' } }))).toBe(
			false
		);
	});
	it('lists active jobs, the poll winning over the list', () => {
		const a = clip({
			clip_id: 'a',
			lanes: { job: run({ job_id: 'j-a', state: 'running' }), current: false }
		});
		const b = clip({
			clip_id: 'b',
			lanes: { job: run({ job_id: 'j-b', state: 'queued' }), current: false }
		});
		const c = clip({ clip_id: 'c', lanes: { job: run({ job_id: 'j-c' }), current: true } });
		expect(activeLaneJobIds([a, b, c])).toEqual(['j-a', 'j-b']);
		expect(
			activeLaneJobIds([a, b, c], new Map([['j-a', job({ job_id: 'j-a', state: 'completed' })]]))
		).toEqual(['j-b']);
	});
});

describe('describeRerun', () => {
	const r: LaneRerunResponse = {
		dry_run: true,
		force: false,
		queued: Array.from({ length: 12 }, (_, i) => ({ clip_id: `q${i}` })),
		current: [{ clip_id: 'c1' }, { clip_id: 'c2' }, { clip_id: 'c3' }],
		active: [],
		skipped: Array.from({ length: 9 }, (_, i) => ({
			clip_id: `s${i}`,
			reason: 'not_located: the clip has no location'
		}))
	};
	it('counts what it will run and skip', () => {
		expect(describeRerun(r)).toBe('12 clips to run, 3 current, 9 skipped');
		expect(describeRerun({ ...r, queued: [{ clip_id: 'x' }], current: [], skipped: [] })).toBe(
			'1 clip to run'
		);
	});
	it('lists the skipped clips with a limit', () => {
		const text = describeSkipped(r, 2);
		expect(text.split('\n')).toHaveLength(3);
		expect(text).toContain('and 7 more');
		expect(text).toContain(': no location');
	});
	it('shortens each reason by its state', () => {
		expect(skipReason('ambiguous: 7 accepted records at this site (a, b)')).toContain(
			'Run lanes asks'
		);
		expect(skipReason('unknown: Gerardus at x did not answer: refused')).toBe(
			'Gerardus did not answer'
		);
		expect(skipReason('other_sensor: accepted only for sensor hesai-01, not s')).toBe(
			'accepted only for sensor hesai-01, not s'
		);
	});
});

describe('pickRecord', () => {
	it('takes an id or a place in the list, nothing else', () => {
		const c = ['gms:a', 'gms:b'];
		expect(pickRecord('2', c)).toBe('gms:b');
		expect(pickRecord(' gms:a ', c)).toBe('gms:a');
		expect(pickRecord('3', c)).toBe('');
		expect(pickRecord('gms:z', c)).toBe('');
		expect(pickRecord(null, c)).toBe('');
	});
	it('reads ambiguous as choose', () => {
		expect(
			registrationView({ state: 'ambiguous', reason: '7 accepted records', checked_at: '' }).label
		).toBe('choose');
	});
});
