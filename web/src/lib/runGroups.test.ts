import {
	clipSiteName,
	compareWithRecommended,
	differenceDetail,
	differenceSummary,
	foldRepeats,
	groupRunsByClip,
	insertRunByCreated,
	paramDifferences
} from './runGroups';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import type { LidarClip, Run } from './types/lidar';

function clip(id: string, extra: Partial<LidarClip> = {}): LidarClip {
	return {
		clip_id: id,
		sensor_id: 'hesai-pandar40p',
		files: [{ ordinal: 0, pcap_file: `/c/${id}.pcapng` }],
		created_at_ns: 0,
		file_count: 1,
		...extra
	};
}

let seq = 0;
function run(extra: Partial<Run> = {}): Run {
	seq += 1;
	return {
		run_id: `run-${seq}`,
		created_at: '2026-11-07T12:00:00Z',
		source_type: 'pcap',
		sensor_id: 'hesai-pandar40p',
		total_tracks: 0,
		confirmed_tracks: 0,
		status: 'completed',
		...extra
	};
}

// A run's composed config as the server stores it (execution_config), over
// the shipped defaults, and a clip's recommendation as a sweep writes it: a
// partial set, keyed by suffix or nested. The two digests hash different
// envelopes (effective and requested) and so never match each other.
const defaults = JSON.parse(
	readFileSync(resolve(process.cwd(), '../config/tuning.defaults.json'), 'utf8')
) as Record<string, unknown>;

function withNoise(params: Record<string, unknown>, noise: number): Record<string, unknown> {
	const copy = JSON.parse(JSON.stringify(params));
	copy.l3.ema_baseline_v1.noise_relative = noise;
	return copy;
}

function effectiveRun(params: Record<string, unknown>, extra: Partial<Run> = {}): Run {
	return run({
		params_hash: 'sha256:c927ea3afe40a1be0fb1812083763d05a37269c7bfe6fe1098f91866fdec570d',
		requested_param_set_id: 'requested-set',
		execution_config: {
			schema_version: 'run_config/v1',
			param_set_type: 'effective',
			build: { build_version: '0.5.1-pre52', build_git_sha: 'ec43762054b9' },
			params
		},
		...extra
	});
}

function recommendingClip(params: Record<string, unknown>): LidarClip {
	return clip('kirk0', {
		recommended_param_set_id: 'recommended-set',
		recommended_params_hash:
			'sha256:d948a67f00000000000000000000000000000000000000000000000000000000',
		recommended_param_set_type: 'requested',
		recommended_params: params
	});
}

describe('paramDifferences', () => {
	it('compares only the keys the recommendation names, however they are written', () => {
		const nested = { l3: { ema_baseline_v1: { noise_relative: 0.02, closeness_multiplier: 2 } } };
		const bySuffix = { noise_relative: 0.02, closeness_multiplier: 2 };
		expect(paramDifferences(defaults, nested)).toEqual([]);
		expect(paramDifferences(defaults, bySuffix)).toEqual([]);
	});

	it('names each recommended key the run disagrees with, or lacks', () => {
		expect(
			paramDifferences(withNoise(defaults, 0.03), { noise_relative: 0.02, not_a_param: true })
		).toEqual([
			{ key: 'l3.ema_baseline_v1.noise_relative', run: '0.03', recommended: '0.02' },
			{ key: 'not_a_param', run: undefined, recommended: 'true' }
		]);
	});
});

describe('compareWithRecommended', () => {
	const recommendation = { noise_relative: 0.02, closeness_multiplier: 2 };

	it('matches a run at exactly its recommended values, whatever the digests', () => {
		expect(
			compareWithRecommended(effectiveRun(defaults), recommendingClip(recommendation))
		).toEqual({ kind: 'recommended' });
	});

	it('matches a run that requested the recommended set', () => {
		const r = effectiveRun(withNoise(defaults, 0.5), { requested_param_set_id: 'recommended-set' });
		expect(compareWithRecommended(r, recommendingClip(recommendation)).kind).toBe('recommended');
	});

	it('lists only the recommended keys that differ', () => {
		expect(
			compareWithRecommended(
				effectiveRun(withNoise(defaults, 0.03)),
				recommendingClip(recommendation)
			)
		).toEqual({
			kind: 'differs',
			differences: [{ key: 'l3.ema_baseline_v1.noise_relative', run: '0.03', recommended: '0.02' }]
		});
	});

	it('says nothing without a clip, a recommendation or the run’s parameters', () => {
		expect(compareWithRecommended(effectiveRun(defaults), null).kind).toBe('unknown');
		expect(compareWithRecommended(effectiveRun(defaults), clip('x')).kind).toBe('unknown');
		expect(compareWithRecommended(effectiveRun(defaults), recommendingClip({})).kind).toBe(
			'unknown'
		);
		expect(compareWithRecommended(run(), recommendingClip(recommendation)).kind).toBe('unknown');
	});
});

describe('foldRepeats', () => {
	it('folds consecutive runs with the same digest and status', () => {
		const a = run({ params_hash: 'h1' });
		const b = run({ params_hash: 'h1' });
		const c = run({ params_hash: 'h1', status: 'failed' });
		const d = run({ params_hash: 'h2' });
		const e = run({ params_hash: 'h1' });
		const rows = foldRepeats([a, b, c, d, e]);
		expect(rows.map((r) => [r.run.run_id, r.repeats.map((x) => x.run_id)])).toEqual([
			[a.run_id, [b.run_id]],
			[c.run_id, []],
			[d.run_id, []],
			[e.run_id, []]
		]);
	});

	it('never folds runs of another capture or frame mode', () => {
		const a = run({ params_hash: 'h1', source_path: '/c/a.pcap' });
		const b = run({ params_hash: 'h1', source_path: '/c/b.pcap' });
		const c = run({ params_hash: 'h1', source_path: '/c/b.pcap', frame_mode: 'full' });
		const d = run({ params_hash: 'h1', source_path: '/c/b.pcap', frame_mode: 'full' });
		expect(foldRepeats([a, b, c, d]).map((r) => r.repeats.length)).toEqual([0, 0, 1]);
	});

	it('never folds runs of another build, day or track count', () => {
		const base = {
			params_hash: 'h1',
			source_path: '/c/a.pcap',
			build_git_sha: 'aaa',
			frame_start_ns: 1,
			frame_end_ns: 2,
			total_tracks: 5
		};
		const rows = foldRepeats([
			run(base),
			run(base),
			run({ ...base, build_git_sha: 'bbb' }),
			run({ ...base, build_git_sha: 'bbb', frame_start_ns: 10, frame_end_ns: 20 }),
			run({ ...base, build_git_sha: 'bbb', frame_start_ns: 10, frame_end_ns: 20, total_tracks: 6 })
		]);
		expect(rows.map((r) => r.repeats.length)).toEqual([1, 0, 0, 0]);
	});

	it('never folds runs without a digest', () => {
		expect(foldRepeats([run(), run()]).map((r) => r.repeats.length)).toEqual([0, 0]);
	});
});

describe('groupRunsByClip', () => {
	const kirk0 = clip('kirk0');
	const ashbury = clip('ashbury');
	const clips = [kirk0, ashbury];

	it('groups by clip in newest-run order, with the clipless runs last', () => {
		const loose = run({ source_path: '/elsewhere.pcap' });
		const k1 = run({ clip_id: 'kirk0' });
		const a1 = run({ clip_id: 'ashbury' });
		const k2 = run({ source_path: '/vol/c/kirk0.pcapng' });
		const groups = groupRunsByClip([loose, k1, a1, k2], clips);
		expect(groups.map((g) => [g.key, g.runCount])).toEqual([
			['kirk0', 2],
			['ashbury', 1],
			['', 1]
		]);
		expect(groups[2].clip).toBeNull();
	});

	it('folds a sweep’s runs under the sweep at its newest run’s place', () => {
		const before = run({ clip_id: 'kirk0', params_hash: 'h0' });
		const s1 = run({ clip_id: 'kirk0', sweep_id: 'sw1', sweep_mode: 'sweep', params_hash: 'h1' });
		const between = run({ clip_id: 'kirk0', params_hash: 'h9' });
		const s2 = run({ clip_id: 'kirk0', sweep_id: 'sw1', sweep_mode: 'sweep', params_hash: 'h2' });
		const s3 = run({ clip_id: 'kirk0', sweep_id: 'sw1', sweep_mode: 'sweep', params_hash: 'h2' });
		const [group] = groupRunsByClip([before, s1, between, s2, s3], clips);
		expect(group.runCount).toBe(5);
		expect(group.rows.map((r) => r.kind)).toEqual(['run', 'sweep', 'run']);
		const sweep = group.rows[1];
		if (sweep.kind !== 'sweep') throw new Error('expected a sweep row');
		expect(sweep.sweepId).toBe('sw1');
		expect(sweep.mode).toBe('sweep');
		expect(sweep.runs).toEqual([s1, s2, s3]);
		expect(sweep.rows.map((r) => r.repeats.length)).toEqual([0, 1]);
	});

	it('folds five identical replays of one clip into one row', () => {
		const same = Array.from({ length: 5 }, () =>
			run({ clip_id: 'ashbury', params_hash: 'c927ea3a' })
		);
		const [group] = groupRunsByClip(same, clips);
		expect(group.rows).toHaveLength(1);
		const row = group.rows[0];
		if (row.kind !== 'run') throw new Error('expected a run row');
		expect(row.repeats).toHaveLength(4);
	});

	it('returns nothing for no runs', () => {
		expect(groupRunsByClip([], clips)).toEqual([]);
	});
});

describe('labels', () => {
	it('names a site by its label, else its cell', () => {
		expect(
			clipSiteName(clip('a', { site_id: '808580c27', site_label: 'Broadway at Franklin' }))
		).toBe('Broadway at Franklin');
		expect(clipSiteName(clip('a', { site_id: '808580aab' }))).toBe('site 808580aab');
		expect(clipSiteName(clip('a'))).toBe('');
		expect(clipSiteName(null)).toBe('');
	});

	it('summarises and details differences', () => {
		const diffs = [
			{ key: 'l3.a.one', run: '1', recommended: '2' },
			{ key: 'l3.a.two', run: undefined, recommended: '2' },
			{ key: 'l4.three', run: '3', recommended: undefined },
			{ key: 'l5.four', run: '4', recommended: '5' }
		];
		expect(differenceSummary(diffs)).toBe('one, two, three +1');
		expect(differenceSummary(diffs.slice(0, 2))).toBe('one, two');
		expect(differenceDetail(diffs.slice(1, 3))).toBe(
			'l3.a.two: (unset) (recommended 2)\nl4.three: 3 (recommended (unset))'
		);
	});
});

describe('insertRunByCreated', () => {
	it('places a fetched run at its own time in a newest-first list', () => {
		const list = [
			run({ run_id: 'c', created_at: '2026-11-07T12:00:00Z' }),
			run({ run_id: 'a', created_at: '2026-11-01T12:00:00Z' })
		];
		const ids = (l: Run[]) => l.map((r) => r.run_id);
		expect(
			ids(insertRunByCreated(list, run({ run_id: 'b', created_at: '2026-11-04T00:00:00Z' })))
		).toEqual(['c', 'b', 'a']);
		expect(
			ids(insertRunByCreated(list, run({ run_id: 'old', created_at: '2026-01-01T00:00:00Z' })))
		).toEqual(['c', 'a', 'old']);
		expect(
			ids(insertRunByCreated(list, run({ run_id: 'new', created_at: '2026-12-01T00:00:00Z' })))
		).toEqual(['new', 'c', 'a']);
	});
});
