import { clipForRun, labelShare, objectiveText, shortDigest } from './lidarRecords';
import type { AnalysisRun, LidarReplayCase } from './types/lidar';

function clip(id: string, extra: Partial<LidarReplayCase> = {}): LidarReplayCase {
	return {
		replay_case_id: id,
		sensor_id: 'hesai-pandar40p',
		pcap_file: `/c/${id}.pcapng`,
		created_at_ns: 0,
		file_count: 1,
		...extra
	};
}

function run(extra: Partial<AnalysisRun> = {}): AnalysisRun {
	return {
		run_id: 'run-1',
		created_at: '2026-10-07T12:00:00Z',
		source_type: 'pcap',
		sensor_id: 'hesai-pandar40p',
		total_tracks: 0,
		confirmed_tracks: 0,
		status: 'completed',
		...extra
	};
}

describe('clipForRun', () => {
	const kirk0 = clip('kirk0');
	const kirk1 = clip('kirk1', { reference_run_id: 'run-1' });
	const recorded = clip('recorded', { pcap_file: '/c/kirk0.pcapng' });
	const clips = [kirk0, kirk1, recorded];

	it('takes the clip the run recorded first', () => {
		expect(
			clipForRun(run({ replay_case_id: 'recorded', source_path: '/c/kirk0.pcapng' }), clips)
		).toBe(recorded);
	});

	it('falls back to the clip naming it as its reference run', () => {
		expect(clipForRun(run({ source_path: '/c/kirk0.pcapng' }), clips)).toBe(kirk1);
	});

	it('then to a clip over the same capture from the same sensor', () => {
		const other = run({ run_id: 'run-2', source_path: '/c/kirk0.pcapng' });
		expect(clipForRun(other, clips)).toBe(kirk0);
		expect(clipForRun({ ...other, sensor_id: 'other' }, clips)).toBeNull();
	});

	it('matches a clip that names its capture relative to the volume', () => {
		const columbus = clip('columbus', { pcap_file: 's2/s2_sf_3_00002.pcap' });
		const replay = run({
			run_id: 'run-5',
			source_path: '/Volumes/lidar/lidar/s2/s2_sf_3_00002.pcap'
		});
		expect(clipForRun(replay, [columbus])).toBe(columbus);
		// Only on a path boundary: a longer directory name is a different capture.
		expect(
			clipForRun({ ...replay, source_path: '/Volumes/lidar/ss2/s2_sf_3_00002.pcap' }, [columbus])
		).toBeNull();
	});

	it('looks past a recorded clip that is no longer listed', () => {
		const gone = run({
			run_id: 'run-3',
			replay_case_id: 'deleted',
			source_path: '/c/kirk0.pcapng'
		});
		expect(clipForRun(gone, clips)).toBe(kirk0);
	});

	it('is null for live data and for no run', () => {
		expect(clipForRun(run({ run_id: 'run-4', source_type: 'live' }), clips)).toBeNull();
		expect(clipForRun(null, clips)).toBeNull();
	});
});

describe('shortDigest', () => {
	it('drops the algorithm and keeps eight hex digits', () => {
		expect(shortDigest('sha256:2319c8f8aaf95e70887ced03d7e9f42f')).toBe('2319c8f8');
	});

	it('shortens a bare digest the same way', () => {
		expect(shortDigest('d4f801aea55b2f97')).toBe('d4f801ae');
		expect(shortDigest('d4f801aea55b2f97', 4)).toBe('d4f8');
	});

	it('is empty when there is no digest', () => {
		expect(shortDigest(undefined)).toBe('');
		expect(shortDigest(null)).toBe('');
		expect(shortDigest('')).toBe('');
	});
});

describe('labelShare', () => {
	it('counts classified and flag-only tracks as labelled', () => {
		expect(labelShare({ total: 54, classified: 14, tagged_only: 23, unlabelled: 17 })).toEqual({
			labelled: 37,
			total: 54,
			fraction: 37 / 54
		});
	});

	it('is null for a run nobody has labelled', () => {
		expect(labelShare({ total: 190, classified: 0, tagged_only: 0, unlabelled: 190 })).toBeNull();
		expect(labelShare({ total: 0, classified: 0, tagged_only: 0, unlabelled: 0 })).toBeNull();
		expect(labelShare(undefined)).toBeNull();
	});
});

describe('objectiveText', () => {
	it('reads an objective name as words', () => {
		expect(objectiveText('ground_truth')).toBe('ground truth');
		expect(objectiveText('weighted')).toBe('weighted');
		expect(objectiveText(undefined)).toBe('');
	});

	it('says a plain sweep scored nothing', () => {
		expect(objectiveText('manual')).toBe('none, compared by hand');
	});
});
