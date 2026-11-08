import {
	clipSiteName,
	compareWithRecommended,
	differenceDetail,
	differenceSummary,
	flattenParams,
	foldRepeats,
	groupRunsByClip,
	paramDifferences
} from './runGroups';
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

const recommended = {
	l3: { engine: 'ema_baseline_v1', ema_baseline_v1: { noise_relative: 0.02, closeness: 2 } },
	l4: { gates: [1, 2] }
};

describe('flattenParams and paramDifferences', () => {
	it('lists leaves by dotted path, an array as one leaf', () => {
		expect([...flattenParams(recommended)]).toEqual([
			['l3.ema_baseline_v1.closeness', '2'],
			['l3.ema_baseline_v1.noise_relative', '0.02'],
			['l3.engine', '"ema_baseline_v1"'],
			['l4.gates', '[1,2]']
		]);
	});

	it('names changed, added and removed keys', () => {
		const mine = {
			l3: { engine: 'ema_baseline_v1', ema_baseline_v1: { noise_relative: 0.03 } },
			l4: { gates: [1, 2] },
			l5: { engine: 'none' }
		};
		expect(paramDifferences(mine, recommended)).toEqual([
			{ key: 'l3.ema_baseline_v1.closeness', run: undefined, recommended: '2' },
			{ key: 'l3.ema_baseline_v1.noise_relative', run: '0.03', recommended: '0.02' },
			{ key: 'l5.engine', run: '"none"', recommended: undefined }
		]);
	});

	it('finds no difference between equal sets in any key order', () => {
		const reordered = {
			l4: { gates: [1, 2] },
			l3: { ema_baseline_v1: { closeness: 2, noise_relative: 0.02 }, engine: 'ema_baseline_v1' }
		};
		expect(paramDifferences(reordered, recommended)).toEqual([]);
	});
});

describe('compareWithRecommended', () => {
	const withRec = clip('kirk0', {
		recommended_params_hash: 'sha256:aaaa',
		recommended_params: recommended
	});

	it('matches on equal digests', () => {
		expect(compareWithRecommended(run({ params_hash: 'sha256:aaaa' }), withRec)).toEqual({
			kind: 'recommended'
		});
	});

	it('lists the keys that differ', () => {
		const r = run({
			params_hash: 'sha256:bbbb',
			execution_config: {
				params: { ...recommended, l4: { gates: [3] } }
			}
		});
		expect(compareWithRecommended(r, withRec)).toEqual({
			kind: 'differs',
			differences: [{ key: 'l4.gates', run: '[3]', recommended: '[1,2]' }]
		});
	});

	it('reads equal values under different digests as recommended', () => {
		const r = run({ params_hash: 'sha256:cccc', execution_config: { params: recommended } });
		expect(compareWithRecommended(r, withRec)).toEqual({ kind: 'recommended' });
	});

	it('says nothing without a clip, a recommendation or the run’s parameters', () => {
		expect(compareWithRecommended(run({ params_hash: 'sha256:aaaa' }), null).kind).toBe('unknown');
		expect(compareWithRecommended(run({ params_hash: 'sha256:aaaa' }), clip('x')).kind).toBe(
			'unknown'
		);
		expect(compareWithRecommended(run({ params_hash: 'sha256:bbbb' }), withRec).kind).toBe(
			'unknown'
		);
		expect(compareWithRecommended(run(), withRec).kind).toBe('unknown');
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
