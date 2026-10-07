import { clipQuery, linkedId, runQuery, tracksQuery } from './lidarLinks';

describe('clipQuery and runQuery', () => {
	it('carry the record as id', () => {
		expect(clipQuery('case-1')).toBe('id=case-1');
		expect(runQuery('run-1')).toBe('id=run-1');
	});

	it('encode what a URL cannot carry as is', () => {
		expect(clipQuery('a b&c')).toBe('id=a+b%26c');
	});
});

describe('tracksQuery', () => {
	it('uses the parameter names the Tracks page reads', () => {
		expect(
			tracksQuery({ sensorId: 'hesai-pandar40p', clipId: 'case-1', runId: 'run-1', atNs: 42 })
		).toBe('sensor_id=hesai-pandar40p&replay_case_id=case-1&run_id=run-1&at_ns=42');
	});

	it('leaves out what is not known', () => {
		expect(tracksQuery({ runId: 'run-1', clipId: null, atNs: null })).toBe('run_id=run-1');
		expect(tracksQuery({})).toBe('');
	});

	it('keeps a zero instant', () => {
		expect(tracksQuery({ atNs: 0 })).toBe('at_ns=0');
	});
});

describe('linkedId', () => {
	it('reads back what clipQuery wrote', () => {
		const url = new URL(`http://localhost/app/lidar/replay-cases?${clipQuery('a b&c')}`);
		expect(linkedId(url)).toBe('a b&c');
	});

	it('is null when nothing was linked', () => {
		expect(linkedId(new URL('http://localhost/app/lidar/runs'))).toBeNull();
		expect(linkedId(new URL('http://localhost/app/lidar/runs?id=%20'))).toBeNull();
	});
});
