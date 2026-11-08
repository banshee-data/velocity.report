// How the Runs page arranges its list: one block per clip, a sweep's runs
// folded under the sweep, repeated runs folded into one row, and each run's
// parameters compared with the parameters its clip recommends.

import { clipForRun } from '#lib/lidarRecords.js';
import type { LidarClip, Run } from '#lib/types/lidar.js';

/** One parameter that differs between a run and its clip's recommended set. */
export type ParamDifference = {
	/** Dotted path into the parameter set, e.g. l3.ema_baseline_v1.noise_relative. */
	key: string;
	/** The run's value as JSON, or undefined when the run's set lacks the key. */
	run: string | undefined;
	/** The recommended value as JSON, or undefined when the recommended set lacks it. */
	recommended: string | undefined;
};

/** How a run's parameters compare with its clip's recommended set. */
export type ParamComparison =
	| { kind: 'recommended' }
	| { kind: 'differs'; differences: ParamDifference[] }
	/** Nothing to compare: no clip, no recommended set, or no parameters on the run. */
	| { kind: 'unknown' };

function isPlainObject(value: unknown): value is Record<string, unknown> {
	return typeof value === 'object' && value !== null && !Array.isArray(value);
}

/**
 * flattenParams lists a parameter set's leaves by dotted path, each value as
 * JSON. An array is one leaf: a list of gates or regions changes as a whole.
 */
export function flattenParams(value: unknown, prefix = ''): Map<string, string> {
	const out = new Map<string, string>();
	if (!isPlainObject(value)) {
		if (prefix) out.set(prefix, JSON.stringify(value));
		return out;
	}
	for (const key of Object.keys(value).sort()) {
		const path = prefix ? `${prefix}.${key}` : key;
		for (const [leaf, leafValue] of flattenParams(value[key], path)) out.set(leaf, leafValue);
	}
	return out;
}

/** paramDifferences lists the leaves that differ between two parameter sets, by path. */
export function paramDifferences(run: unknown, recommended: unknown): ParamDifference[] {
	const a = flattenParams(run);
	const b = flattenParams(recommended);
	const keys = [...new Set([...a.keys(), ...b.keys()])].sort();
	const out: ParamDifference[] = [];
	for (const key of keys) {
		if (a.get(key) !== b.get(key)) out.push({ key, run: a.get(key), recommended: b.get(key) });
	}
	return out;
}

/** runParams is the parameter set a run executed, from its composed run config. */
export function runParams(run: Run): Record<string, unknown> | null {
	const params = run.execution_config?.params;
	return isPlainObject(params) ? params : null;
}

function recommendedParams(clip: LidarClip): Record<string, unknown> | null {
	const params = clip.recommended_params;
	return isPlainObject(params) ? params : null;
}

/**
 * compareWithRecommended says whether a run used its clip's recommended
 * parameters and, when it did not, which keys differ. Equal digests settle
 * it without reading either set.
 */
export function compareWithRecommended(run: Run, clip: LidarClip | null): ParamComparison {
	if (!clip?.recommended_params_hash || !run.params_hash) return { kind: 'unknown' };
	if (clip.recommended_params_hash === run.params_hash) return { kind: 'recommended' };
	const mine = runParams(run);
	const theirs = recommendedParams(clip);
	if (!mine || !theirs) return { kind: 'unknown' };
	const differences = paramDifferences(mine, theirs);
	// The digests differ but no value does: the same parameters, canonicalised
	// differently when stored. Read it as the recommended set.
	if (differences.length === 0) return { kind: 'recommended' };
	return { kind: 'differs', differences };
}

/** One row of runs: the newest, then the identical runs folded under it. */
export type RunRow = { kind: 'run'; run: Run; repeats: Run[] };

/** One sweep's runs, folded under the sweep. */
export type SweepRow = {
	kind: 'sweep';
	sweepId: string;
	mode: string;
	runs: Run[];
	rows: RunRow[];
};

export type GroupRow = RunRow | SweepRow;

/** One clip's runs, or the runs no clip covers (clip null). */
export type ClipGroup = {
	key: string;
	clip: LidarClip | null;
	runCount: number;
	rows: GroupRow[];
};

/**
 * foldRepeats folds each run into the row above it when it repeats that row
 * exactly: the same source, frame mode, parameter digest and status. Under one
 * clip the same digest means the same difference from the recommended set, so
 * the folded row hides nothing the row above shows. A run with no digest is
 * never folded, since nothing says it repeats another.
 */
export function foldRepeats(runs: Run[]): RunRow[] {
	const rows: RunRow[] = [];
	for (const run of runs) {
		const last = rows[rows.length - 1];
		if (
			last &&
			run.params_hash &&
			last.run.params_hash === run.params_hash &&
			last.run.status === run.status &&
			(last.run.source_path ?? '') === (run.source_path ?? '') &&
			(last.run.frame_mode ?? 'foreground') === (run.frame_mode ?? 'foreground')
		) {
			last.repeats.push(run);
		} else {
			rows.push({ kind: 'run', run, repeats: [] });
		}
	}
	return rows;
}

/**
 * groupRunsByClip arranges a newest-first list of runs into one group per
 * clip, ordered by each clip's newest run, with the runs no clip covers last.
 * Within a group a sweep's runs become one row at its newest run's place, and
 * repeated runs fold into the row above them.
 */
export function groupRunsByClip(runs: Run[], clips: LidarClip[]): ClipGroup[] {
	const groups = new Map<string, { clip: LidarClip | null; runs: Run[] }>();
	for (const run of runs) {
		const clip = clipForRun(run, clips);
		const key = clip?.clip_id ?? '';
		let group = groups.get(key);
		if (!group) {
			group = { clip, runs: [] };
			groups.set(key, group);
		}
		group.runs.push(run);
	}

	const out: ClipGroup[] = [];
	for (const [key, group] of groups) {
		out.push({ key, clip: group.clip, runCount: group.runs.length, rows: groupRows(group.runs) });
	}
	// Map order is first appearance, which is newest run first; move the
	// runs with no clip to the end.
	out.sort((a, b) => Number(a.clip === null) - Number(b.clip === null));
	return out;
}

function groupRows(runs: Run[]): GroupRow[] {
	// Each entry is a sweep's placeholder or a stretch of runs no sweep ran.
	type Slot = { sweep: SweepRow } | { runs: Run[] };
	const slots: Slot[] = [];
	const sweeps = new Map<string, SweepRow>();
	for (const run of runs) {
		if (run.sweep_id) {
			let sweep = sweeps.get(run.sweep_id);
			if (!sweep) {
				sweep = {
					kind: 'sweep',
					sweepId: run.sweep_id,
					mode: run.sweep_mode ?? '',
					runs: [],
					rows: []
				};
				sweeps.set(run.sweep_id, sweep);
				slots.push({ sweep });
			}
			sweep.runs.push(run);
			continue;
		}
		const last = slots[slots.length - 1];
		if (last && 'runs' in last) last.runs.push(run);
		else slots.push({ runs: [run] });
	}
	const rows: GroupRow[] = [];
	for (const slot of slots) {
		if ('sweep' in slot) {
			slot.sweep.rows = foldRepeats(slot.sweep.runs);
			rows.push(slot.sweep);
		} else {
			rows.push(...foldRepeats(slot.runs));
		}
	}
	return rows;
}

/** clipSiteName names a clip's site: its label, else its cell, else nothing. */
export function clipSiteName(clip: LidarClip | null | undefined): string {
	if (!clip) return '';
	if (clip.site_label) return clip.site_label;
	if (clip.site_id) return `site ${clip.site_id}`;
	return '';
}

/** differenceSummary names the differing keys briefly: the last path segment, at most `limit`. */
export function differenceSummary(differences: ParamDifference[], limit = 3): string {
	const names = differences.slice(0, limit).map((d) => d.key.split('.').slice(-1)[0]);
	const more = differences.length - names.length;
	return more > 0 ? `${names.join(', ')} +${more}` : names.join(', ');
}

/** differenceDetail lists every differing key with both values, one per line. */
export function differenceDetail(differences: ParamDifference[]): string {
	return differences
		.map((d) => `${d.key}: ${d.run ?? '(unset)'} (recommended ${d.recommended ?? '(unset)'})`)
		.join('\n');
}
