import type {
	CaptureJob,
	CaptureRoot,
	CaptureScanProgress,
	CaptureSession,
	MotionPeriod
} from '#lib/types/captures.js';
import {
	describeQueuedPasses,
	describeScanProgress,
	estimateRemainingSecs,
	formatRemaining,
	motionPassBatch,
	newlyCompletedSessions,
	scanAdvanceKey,
	sessionName,
	sessionsWithoutMotionPass
} from '../progress';

const SEC = 1_000_000_000;
const MS = 1_000_000;
const BASE = new Date(2026, 8, 2, 13, 20, 37).getTime() * MS;

/**
 * A probe that started listing at BASE, began reading 10 s later, and had
 * finished `done` captures at 30 s each when it was asked, `into` seconds into
 * the next.
 */
function probing(done: number, total: number, into = 0): CaptureScanProgress {
	const probeStart = BASE + 10 * SEC;
	const updated = probeStart + done * 30 * SEC;
	return {
		phase: 'probing',
		done,
		total,
		current: done < total ? `s2_sf_3_${String(done + 1).padStart(5, '0')}.pcap` : undefined,
		probed: done,
		probe_failed: 0,
		started_at_ns: BASE,
		probe_started_at_ns: probeStart,
		updated_at_ns: updated,
		elapsed_ns: updated - BASE + into * SEC,
		probe_elapsed_ns: updated - probeStart + into * SEC
	};
}

function phase(name: string): CaptureScanProgress {
	return {
		phase: name,
		done: 0,
		total: 0,
		probed: 0,
		probe_failed: 0,
		started_at_ns: BASE,
		updated_at_ns: BASE,
		elapsed_ns: 4 * SEC
	};
}

function job(id: string, sessionId: string, state: string, kind = 'motion_pass'): CaptureJob {
	return {
		job_id: id,
		kind,
		session_id: sessionId,
		state,
		progress_current: 0,
		progress_total: 0,
		queued_at_ns: BASE
	};
}

function session(id: string, label = ''): CaptureSession {
	return {
		session_id: id,
		root_id: 'root-1',
		label,
		file_count: 1,
		start_ns: BASE,
		end_ns: BASE + 300 * SEC,
		covered_ns: 300 * SEC,
		lost_ns: 0,
		worst_seam: 'seamless',
		size_bytes: 724 * 1024 * 1024,
		derived_at_ns: BASE
	};
}

const PERIOD = { period_id: 'per-1' } as MotionPeriod;

describe('describeScanProgress', () => {
	it('names the capture being read and counts from one', () => {
		const view = describeScanProgress(probing(2, 10));
		expect(view.headline).toBe('Probing 3 of 10 captures');
		expect(view.current).toBe('s2_sf_3_00003.pcap');
		expect(view.percent).toBe(20);
		expect(view.probed).toBe(2);
		expect(view.failed).toBe(0);
	});

	it('keeps counting elapsed time between polls', () => {
		const view = describeScanProgress(probing(2, 10, 5), 1.5);
		expect(view.elapsedSecs).toBeCloseTo(10 + 60 + 5 + 1.5);
	});

	it('says what a scan is doing before it knows how many captures it will read', () => {
		for (const [name, headline] of [
			['queued', 'Waiting for another scan to finish'],
			['listing', 'Listing the captures on the volume']
		]) {
			const view = describeScanProgress(phase(name));
			expect(view.headline).toBe(headline);
			expect(view.percent).toBeNull();
			expect(view.remainingSecs).toBeNull();
			expect(view.elapsedSecs).toBe(4);
		}
	});

	it('reads as all but finished while sessions are rebuilt', () => {
		const view = describeScanProgress(phase('deriving'));
		expect(view.headline).toBe('Building sessions from the probed captures');
		expect(view.percent).toBe(100);
	});

	it('does not invent a total for a probe that has none yet', () => {
		const view = describeScanProgress({ ...phase('probing') });
		expect(view.headline).toBe('Probing captures');
		expect(view.percent).toBeNull();
	});

	it('reports a finished probe without a current capture', () => {
		const view = describeScanProgress(probing(10, 10));
		expect(view.headline).toBe('Probed 10 of 10 captures');
		expect(view.current).toBeNull();
		expect(view.percent).toBe(100);
		expect(view.remainingSecs).toBe(0);
	});

	it('ignores a negative time since the fetch', () => {
		expect(describeScanProgress(probing(2, 10), -5).elapsedSecs).toBeCloseTo(70);
	});
});

describe('estimateRemainingSecs', () => {
	it('extrapolates the pace of the captures already read', () => {
		// 30 s a capture, 8 left, 10 s into the third: 8 × 30 − 10.
		expect(estimateRemainingSecs(probing(2, 10, 10))).toBeCloseTo(230);
	});

	it('counts down between polls', () => {
		expect(estimateRemainingSecs(probing(2, 10, 10), 5)).toBeCloseTo(225);
	});

	it('measures the pace from the first read, not from listing', () => {
		// Listing took 10 s; folding it in would make every capture 35 s.
		expect(estimateRemainingSecs(probing(2, 3))).toBeCloseTo(30);
	});

	it('never promises less than the captures after a slow one', () => {
		// 50 s into a capture that usually takes 30: the rest still need 7 × 30.
		expect(estimateRemainingSecs(probing(2, 10, 50))).toBeCloseTo(210);
	});

	it('has nothing to say before the first capture is read', () => {
		expect(estimateRemainingSecs(probing(0, 10))).toBeNull();
		expect(estimateRemainingSecs({ ...probing(2, 10), probe_started_at_ns: undefined })).toBeNull();
	});

	it('refuses a pace it cannot have measured', () => {
		const p = probing(2, 10);
		expect(
			estimateRemainingSecs({ ...p, updated_at_ns: p.probe_started_at_ns as number })
		).toBeNull();
	});
});

describe('formatRemaining', () => {
	it('rounds to what an estimate can claim', () => {
		expect(formatRemaining(42)).toBe('under a minute left');
		expect(formatRemaining(230)).toBe('about 4 min left');
		expect(formatRemaining(2 * 3600)).toBe('about 2 h left');
		expect(formatRemaining(2 * 3600 + 25 * 60)).toBe('about 2 h 25 min left');
	});

	it('admits when there is no estimate yet', () => {
		expect(formatRemaining(null)).toBe('estimating time left');
		expect(formatRemaining(Number.NaN)).toBe('estimating time left');
		expect(formatRemaining(-1)).toBe('estimating time left');
	});
});

describe('scanAdvanceKey', () => {
	const root = (progress?: CaptureScanProgress, running = true): CaptureRoot => ({
		root_id: 'root-1',
		path: '/Volumes/lidar/lidar',
		enabled: true,
		last_scan_state: 'ok',
		created_at_ns: BASE,
		updated_at_ns: BASE,
		scan_in_progress: running,
		scan_progress: progress
	});

	it('changes when a capture is read, and not when only the clock moves', () => {
		const before = scanAdvanceKey([root(probing(2, 10))]);
		expect(scanAdvanceKey([root(probing(2, 10, 20))])).toBe(before);
		expect(scanAdvanceKey([root(probing(3, 10))])).not.toBe(before);
	});

	it('ignores roots with no scan running', () => {
		expect(scanAdvanceKey([root(undefined), root(probing(2, 10), false)])).toBe('');
	});
});

describe('sessionsWithoutMotionPass', () => {
	const sessions = [session('ses-1'), session('ses-2'), session('ses-3'), session('ses-4')];

	it('leaves out sessions with a timeline or a pass under way', () => {
		const left = sessionsWithoutMotionPass(sessions, { 'ses-1': [PERIOD], 'ses-4': [] }, [
			job('job-2', 'ses-2', 'running'),
			job('job-3', 'ses-3', 'failed')
		]);
		// A failed pass left no timeline, so its session still needs one.
		expect(left.map((s) => s.session_id)).toEqual(['ses-3', 'ses-4']);
	});

	it('counts a queued pass as under way, and a clip job not at all', () => {
		const left = sessionsWithoutMotionPass(sessions, {}, [
			job('job-1', 'ses-1', 'queued'),
			job('job-2', 'ses-2', 'running', 'vrlog_record')
		]);
		expect(left.map((s) => s.session_id)).toEqual(['ses-2', 'ses-3', 'ses-4']);
	});

	it('leaves out sessions whose timeline could not be loaded', () => {
		const left = sessionsWithoutMotionPass(sessions, {}, [], ['ses-1', 'ses-2']);
		expect(left.map((s) => s.session_id)).toEqual(['ses-3', 'ses-4']);
	});
});

describe('motionPassBatch', () => {
	it('tallies what has finished and names what is running', () => {
		const jobs = [
			job('job-1', 'ses-1', 'completed'),
			job('job-2', 'ses-2', 'failed'),
			job('job-3', 'ses-3', 'running'),
			job('job-4', 'ses-4', 'queued'),
			job('job-5', 'ses-5', 'cancelled'),
			job('job-x', 'ses-x', 'running')
		];
		const view = motionPassBatch(['job-1', 'job-2', 'job-3', 'job-4', 'job-5'], jobs);
		expect(view).toMatchObject({
			total: 5,
			finished: 3,
			completed: 1,
			failed: 1,
			cancelled: 1,
			percent: 60,
			done: false
		});
		expect(view.running?.job_id).toBe('job-3');
	});

	it('is done when every pass has finished', () => {
		const view = motionPassBatch(['job-1'], [job('job-1', 'ses-1', 'completed')]);
		expect(view.done).toBe(true);
		expect(view.running).toBeNull();
	});

	it('does not count a job the list no longer holds as finished', () => {
		const view = motionPassBatch(['job-1', 'job-2'], [job('job-1', 'ses-1', 'completed')]);
		expect(view.finished).toBe(1);
		expect(view.done).toBe(false);
	});

	it('handles an empty batch', () => {
		expect(motionPassBatch([], [])).toMatchObject({ total: 0, percent: 0, done: false });
	});
});

describe('describeQueuedPasses', () => {
	const result = (queued: number, active: number, withPeriods: number) => ({
		queued,
		skipped: active + withPeriods,
		skipped_active: active,
		skipped_with_periods: withPeriods,
		jobs: []
	});

	it('says how many were queued and why the rest were not', () => {
		expect(describeQueuedPasses(result(12, 1, 3))).toBe(
			'Queued 12 motion passes: 1 session has one queued or running; 3 sessions have one already.'
		);
		expect(describeQueuedPasses(result(1, 0, 0))).toBe('Queued 1 motion pass.');
	});

	it('reads a second click as already done, not as a failure', () => {
		expect(describeQueuedPasses(result(0, 2, 1))).toBe(
			'No motion passes to queue: 2 sessions have one queued or running; 1 session has one already.'
		);
		expect(describeQueuedPasses(result(0, 0, 0))).toBe('No motion passes to queue.');
	});
});

describe('newlyCompletedSessions', () => {
	it('names sessions whose pass completed since the last look', () => {
		const previous = [
			job('job-1', 'ses-1', 'running'),
			job('job-2', 'ses-2', 'completed'),
			job('job-3', 'ses-3', 'queued')
		];
		const next = [
			job('job-1', 'ses-1', 'completed'),
			job('job-2', 'ses-2', 'completed'),
			job('job-3', 'ses-3', 'failed'),
			job('job-4', 'ses-4', 'completed'),
			job('job-5', 'ses-4', 'completed'),
			job('job-6', '', 'completed'),
			job('job-7', 'ses-7', 'completed', 'vrlog_record')
		];
		expect(newlyCompletedSessions(previous, next)).toEqual(['ses-1', 'ses-4']);
	});
});

describe('sessionName', () => {
	it('prefers the site label', () => {
		expect(sessionName(session('ses-1', 'broadway_columbus'))).toBe('broadway_columbus');
	});

	it('falls back to when the session starts', () => {
		expect(sessionName(session('ses-1'))).toBe('session from 2026-09-02 13:20');
	});

	it('copes with a session no longer listed', () => {
		expect(sessionName(undefined, 'ses-gone')).toBe('ses-gone');
		expect(sessionName(undefined)).toBe('a session no longer listed');
	});
});
