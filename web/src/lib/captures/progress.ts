/**
 * What the Captures page says about work under way: a background scan and
 * probe of a volume, and a batch of motion passes.
 *
 * Pure, like timeline.ts, so an estimate's arithmetic and a batch's tally can
 * be tested without a server or a clock.
 */

import {
	JOB_CANCELLED,
	JOB_COMPLETED,
	JOB_FAILED,
	JOB_KIND_MOTION_PASS,
	JOB_QUEUED,
	JOB_RUNNING,
	SCAN_PHASE_DERIVING,
	SCAN_PHASE_LISTING,
	SCAN_PHASE_QUEUED,
	type CaptureJob,
	type CaptureRoot,
	type CaptureScanProgress,
	type CaptureSession,
	type MissingMotionPassesResponse,
	type MotionPeriod
} from '#lib/types/captures.js';
import { formatStamp } from '#lib/captures/timeline.js';

const NS_PER_SEC = 1_000_000_000;

/** How often the page asks how a scan is getting on, in milliseconds. */
export const SCAN_POLL_MS = 2500;

/** A scan's progress as the page shows it. */
export interface ScanProgressView {
	headline: string;
	/** 0-100 once the probe knows how many captures it will read; null before. */
	percent: number | null;
	/** The capture being read now. */
	current: string | null;
	elapsedSecs: number;
	/** A rough estimate; null until at least one capture has been read. */
	remainingSecs: number | null;
	probed: number;
	failed: number;
}

/**
 * describeScanProgress turns a root's scan progress into what the page says.
 *
 * sinceFetchSecs is how long ago the progress was fetched, so elapsed time
 * keeps counting between polls. Everything else is the server's own
 * measurement, which keeps a browser whose clock disagrees with the server's
 * from showing nonsense.
 */
export function describeScanProgress(p: CaptureScanProgress, sinceFetchSecs = 0): ScanProgressView {
	const since = Math.max(0, sinceFetchSecs);
	const view: ScanProgressView = {
		headline: '',
		percent: null,
		current: null,
		elapsedSecs: p.elapsed_ns / NS_PER_SEC + since,
		remainingSecs: null,
		probed: p.probed,
		failed: p.probe_failed
	};
	if (p.phase === SCAN_PHASE_QUEUED) {
		view.headline = 'Waiting for another scan to finish';
		return view;
	}
	if (p.phase === SCAN_PHASE_LISTING) {
		view.headline = 'Listing the captures on the volume';
		return view;
	}
	if (p.phase === SCAN_PHASE_DERIVING) {
		view.headline = 'Building sessions from the probed captures';
		view.percent = 100;
		return view;
	}
	if (p.total <= 0) {
		view.headline = 'Probing captures';
		return view;
	}
	view.percent = Math.min(100, (p.done / p.total) * 100);
	if (p.current) {
		view.current = p.current;
		view.headline = `Probing ${Math.min(p.done + 1, p.total)} of ${p.total} captures`;
	} else {
		view.headline = `Probed ${p.done} of ${p.total} captures`;
	}
	view.remainingSecs = estimateRemainingSecs(p, since);
	return view;
}

/**
 * estimateRemainingSecs is a rough time left for a probe: the captures still
 * to read at the average pace of those already read, less how far into the
 * current one it is.
 *
 * The pace is measured from the first read, not from the start of the scan,
 * because listing a slow volume takes time of its own. Captures vary in size,
 * so this is an estimate and the page says so. A read that is running long
 * never makes the estimate shorter than the captures after it.
 */
export function estimateRemainingSecs(p: CaptureScanProgress, sinceFetchSecs = 0): number | null {
	if (p.total > 0 && p.done >= p.total) return 0;
	if (p.done <= 0 || !p.probe_started_at_ns) return null;
	// At updated_at_ns the probe had just finished its done-th capture.
	const perCapture = (p.updated_at_ns - p.probe_started_at_ns) / NS_PER_SEC / p.done;
	if (!(perCapture > 0)) return null;
	const intoCurrent =
		(p.elapsed_ns - (p.updated_at_ns - p.started_at_ns)) / NS_PER_SEC + Math.max(0, sinceFetchSecs);
	const left = p.total - p.done;
	return Math.max((left - 1) * perCapture, left * perCapture - intoCurrent);
}

/** formatRemaining renders an estimate the way it deserves: roughly. */
export function formatRemaining(secs: number | null): string {
	if (secs == null || !Number.isFinite(secs) || secs < 0) return 'estimating time left';
	if (secs < 60) return 'under a minute left';
	const mins = Math.round(secs / 60);
	if (mins < 60) return `about ${mins} min left`;
	const h = Math.floor(mins / 60);
	const m = mins % 60;
	return m === 0 ? `about ${h} h left` : `about ${h} h ${m} min left`;
}

/**
 * scanAdvanceKey changes whenever any root's scan moves on — a phase, a
 * capture read, a probe that failed — and only then, so the page refreshes
 * the capture list when there is something new in it rather than every poll.
 */
export function scanAdvanceKey(roots: CaptureRoot[]): string {
	return roots
		.filter((r) => r.scan_in_progress && r.scan_progress)
		.map((r) => {
			const p = r.scan_progress as CaptureScanProgress;
			return `${r.root_id}:${p.phase}:${p.done}:${p.probed}:${p.probe_failed}`;
		})
		.join('|');
}

/** isActiveMotionPass reports a motion pass that is queued or running. */
function isActiveMotionPass(job: CaptureJob): boolean {
	return (
		job.kind === JOB_KIND_MOTION_PASS && (job.state === JOB_QUEUED || job.state === JOB_RUNNING)
	);
}

/**
 * sessionsWithoutMotionPass is the sessions a "run every missing pass"
 * request would queue: no timeline, and no pass queued or running.
 *
 * A session whose periods could not be loaded is left out of the count,
 * since whether it has a timeline is unknown; the server decides for it.
 */
export function sessionsWithoutMotionPass(
	sessions: CaptureSession[],
	periodsBySession: Record<string, MotionPeriod[]>,
	jobs: CaptureJob[],
	unknown: string[] = []
): CaptureSession[] {
	const active = new Set(jobs.filter(isActiveMotionPass).map((j) => j.session_id));
	const skip = new Set(unknown);
	return sessions.filter(
		(s) =>
			!skip.has(s.session_id) &&
			(periodsBySession[s.session_id]?.length ?? 0) === 0 &&
			!active.has(s.session_id)
	);
}

/** How a batch of motion passes is getting on. */
export interface MotionPassBatchView {
	total: number;
	/** Completed, failed or cancelled. */
	finished: number;
	completed: number;
	failed: number;
	cancelled: number;
	/** The pass running now, if one of the batch's is. */
	running: CaptureJob | null;
	/** 0-100, of passes finished. */
	percent: number;
	/** Every pass in the batch has finished. */
	done: boolean;
}

/**
 * motionPassBatch tallies a batch of motion passes from the job list. A job
 * the list no longer holds is counted as not yet finished rather than
 * guessed at.
 */
export function motionPassBatch(jobIds: string[], jobs: CaptureJob[]): MotionPassBatchView {
	const byId = new Map(jobs.map((j) => [j.job_id, j]));
	const view: MotionPassBatchView = {
		total: jobIds.length,
		finished: 0,
		completed: 0,
		failed: 0,
		cancelled: 0,
		running: null,
		percent: 0,
		done: false
	};
	for (const id of jobIds) {
		const job = byId.get(id);
		if (!job) continue;
		if (job.state === JOB_COMPLETED) view.completed++;
		else if (job.state === JOB_FAILED) view.failed++;
		else if (job.state === JOB_CANCELLED) view.cancelled++;
		else if (job.state === JOB_RUNNING && !view.running) view.running = job;
	}
	view.finished = view.completed + view.failed + view.cancelled;
	view.percent = view.total > 0 ? (view.finished / view.total) * 100 : 0;
	view.done = view.total > 0 && view.finished === view.total;
	return view;
}

/**
 * describeQueuedPasses says what asking for every missing motion pass did,
 * including why sessions were left alone, so a second click that queues
 * nothing reads as "already done" rather than as a failure.
 */
export function describeQueuedPasses(result: MissingMotionPassesResponse): string {
	const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;
	const reasons: string[] = [];
	if (result.skipped_active > 0) {
		reasons.push(
			`${plural(result.skipped_active, 'session has', 'sessions have')} one queued or running`
		);
	}
	if (result.skipped_with_periods > 0) {
		reasons.push(
			`${plural(result.skipped_with_periods, 'session has', 'sessions have')} one already`
		);
	}
	const head =
		result.queued > 0
			? `Queued ${plural(result.queued, 'motion pass', 'motion passes')}`
			: 'No motion passes to queue';
	return reasons.length > 0 ? `${head}: ${reasons.join('; ')}.` : `${head}.`;
}

/**
 * newlyCompletedSessions is the sessions whose motion pass completed between
 * two looks at the job list, so the page can load just their timelines and
 * let the strips fill in as the queue drains.
 */
export function newlyCompletedSessions(previous: CaptureJob[], next: CaptureJob[]): string[] {
	const before = new Map(previous.map((j) => [j.job_id, j.state]));
	const out: string[] = [];
	for (const j of next) {
		if (j.kind !== JOB_KIND_MOTION_PASS || j.state !== JOB_COMPLETED || !j.session_id) continue;
		if (before.get(j.job_id) === JOB_COMPLETED) continue;
		if (!out.includes(j.session_id)) out.push(j.session_id);
	}
	return out;
}

/**
 * sessionName is how the page names a session in a sentence: by its site
 * label when it has one, and otherwise by when it starts, which an operator
 * can find on the page, unlike its identifier.
 */
export function sessionName(session: CaptureSession | undefined, fallbackId = ''): string {
	if (!session) return fallbackId || 'a session no longer listed';
	return session.label || `session from ${formatStamp(session.start_ns)}`;
}
