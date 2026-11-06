/**
 * The clips view's Registered and Lanes columns
 * (docs/plans/lidar-lane-field-batch-plan.md, Item 3): what each clip's
 * registration in Gerardus and its lane proposals read as, from the clips
 * list and the lane jobs the page polls. Pure, so the states are tested
 * without a server.
 */
import type { CaptureJob, ClipRegistration, LaneRerunResponse } from '#lib/types/captures.js';
import type { LidarClip } from '#lib/types/lidar.js';

export type RegistrationKind =
	'registered' | 'ambiguous' | 'not-located' | 'unregistered' | 'unknown';

export interface RegistrationView {
	kind: RegistrationKind;
	label: string;
	title: string;
}

/** What the Registered column shows for a clip. */
export function registrationView(reg: ClipRegistration | undefined): RegistrationView {
	if (!reg) return { kind: 'unknown', label: '…', title: 'Not checked yet' };
	switch (reg.state) {
		case 'registered':
			return {
				kind: 'registered',
				label: '✓',
				title: `Placed by ${reg.record_id}${reg.record_state === 'manually_placed' ? ' (placed by hand)' : ''}`
			};
		case 'ambiguous':
			return {
				kind: 'ambiguous',
				label: 'choose',
				title: `${reg.reason || 'Several accepted records'}. Run lanes asks which one places this clip.`
			};
		case 'not_located':
			return {
				kind: 'not-located',
				label: 'not located',
				title: reg.reason || 'The clip has no location'
			};
		case 'unknown':
			return { kind: 'unknown', label: 'unknown', title: reg.reason || 'Gerardus did not answer' };
		default:
			return { kind: 'unregistered', label: '—', title: reg.reason || reg.state };
	}
}

export type LaneKind = 'current' | 'stale' | 'none' | 'queued' | 'running' | 'failed';

export interface LaneView {
	kind: LaneKind;
	label: string;
	title: string;
	/** 0–1 while a job runs. */
	fraction?: number;
	/** The job's own words while it runs. */
	detail?: string;
}

function isActive(state?: string): boolean {
	return state === 'queued' || state === 'running';
}

function counts(high: number, medium: number): string {
	return `${high} high · ${medium} medium`;
}

function day(ns?: number): string {
	if (!ns) return '';
	return new Date(ns / 1e6).toISOString().slice(0, 10);
}

/**
 * What the Lanes column shows for a clip. A polled job, when there is one for
 * the clip's newest job, is fresher than the list's copy of it.
 */
export function laneView(clip: LidarClip, jobs?: Map<string, CaptureJob>): LaneView {
	const lanes = clip.lanes;
	const latest = lanes?.latest;
	const listed = lanes?.job;
	const polled = listed?.job_id ? jobs?.get(listed.job_id) : undefined;
	const state = polled?.state ?? listed?.state;
	if (listed && isActive(state)) {
		const current = polled?.progress_current ?? listed.progress_current;
		const total = polled?.progress_total ?? listed.progress_total;
		const detail = polled?.detail ?? listed.detail ?? '';
		return {
			kind: state === 'queued' ? 'queued' : 'running',
			label: state === 'queued' ? 'queued' : 'running',
			title: detail,
			fraction: total > 0 ? Math.min(1, current / total) : 0,
			detail
		};
	}
	// A failed job newer than the newest result says so; the older result's
	// counts stay in the title.
	if (listed && state === 'failed' && (!latest || listed.created_at_ns > latest.created_at_ns)) {
		const error = (polled?.error ?? listed.error ?? 'failed').split('\n')[0];
		return {
			kind: 'failed',
			label: 'failed',
			title: latest ? `${error}. Last result: ${counts(latest.high, latest.medium)}` : error
		};
	}
	if (!latest)
		return { kind: 'none', label: '—', title: lanes?.stale_reason || 'No lane proposals yet' };
	const summary = counts(latest.high, latest.medium);
	const when = day(latest.completed_at_ns);
	const review = `${latest.outstanding} to review`;
	if (lanes?.current) {
		return {
			kind: 'current',
			label: `✓ ${summary}`,
			title: `${review}; ${when}; record ${latest.record_id}`
		};
	}
	return {
		kind: 'stale',
		label: `stale · ${summary}`,
		title: `${lanes?.stale_reason || 'stale'}; ${review}; ${when}`
	};
}

/**
 * Whether a clip may be run now: registered (or ambiguous, when the operator
 * names the record), and no job of its own active.
 */
export function canRunLanes(clip: LidarClip, jobs?: Map<string, CaptureJob>): boolean {
	const state = clip.registration?.state;
	if (state !== 'registered' && state !== 'ambiguous') return false;
	const v = laneView(clip, jobs);
	return v.kind !== 'queued' && v.kind !== 'running';
}

/** The ids of every lane job the list or the poll says is still active. */
export function activeLaneJobIds(clips: LidarClip[], jobs?: Map<string, CaptureJob>): string[] {
	const out: string[] = [];
	for (const clip of clips) {
		const id = clip.lanes?.job?.job_id;
		if (!id) continue;
		const state = jobs?.get(id)?.state ?? clip.lanes?.job?.state;
		if (isActive(state)) out.push(id);
	}
	return out;
}

function plural(n: number, one: string, many = one + 's'): string {
	return `${n} ${n === 1 ? one : many}`;
}

/** The confirmation a dry run of the re-run gives. */
export function describeRerun(r: LaneRerunResponse): string {
	const parts = [`${plural(r.queued.length, 'clip')} to run`];
	if (r.current.length) parts.push(`${r.current.length} current`);
	if (r.active.length) parts.push(`${r.active.length} already running`);
	if (r.skipped.length) parts.push(`${r.skipped.length} skipped`);
	return parts.join(', ');
}

/** A skipped clip's reason, short: the server sends "<state>: <why>". */
export function skipReason(reason?: string): string {
	const r = reason ?? '';
	const state = r.split(':', 1)[0];
	switch (state) {
		case 'ambiguous':
			return 'several accepted poses at its site; Run lanes asks which';
		case 'not_located':
			return 'no location';
		case 'not_registered':
			return 'no accepted pose at its site';
		case 'unknown':
			return 'Gerardus did not answer';
		case 'other_sensor':
			return r.slice(state.length + 1).trim();
		default:
			return r;
	}
}

/** The skipped clips and why, one line each, for the confirmation. */
export function describeSkipped(r: LaneRerunResponse, limit = 8): string {
	const lines = r.skipped
		.slice(0, limit)
		.map((c) => `• ${c.description || c.clip_id.slice(0, 8)}: ${skipReason(c.reason)}`);
	if (r.skipped.length > limit) lines.push(`• and ${r.skipped.length - limit} more`);
	return lines.join('\n');
}

/**
 * The record an operator names for an ambiguous clip: one of its candidates,
 * by id or by its 1-based place in the list. Empty when the answer is none.
 */
export function pickRecord(answer: string | null, candidates: string[]): string {
	const a = (answer ?? '').trim();
	if (!a) return '';
	const n = Number(a);
	if (Number.isInteger(n) && n >= 1 && n <= candidates.length) return candidates[n - 1];
	return candidates.includes(a) ? a : '';
}
