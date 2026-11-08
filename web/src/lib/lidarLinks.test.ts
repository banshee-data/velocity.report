import {
	clipQuery,
	frameInstantNs,
	linkedId,
	linkedRun,
	runQuery,
	sweepQuery,
	tracksQuery,
	visualiserLink,
	windowsQuery
} from './lidarLinks';

describe('clipQuery and runQuery', () => {
	it('carry the record as id', () => {
		expect(clipQuery('case-1')).toBe('id=case-1');
		expect(runQuery('run-1')).toBe('id=run-1');
		expect(sweepQuery('sweep-1')).toBe('id=sweep-1');
	});

	it('encode what a URL cannot carry as is', () => {
		expect(clipQuery('a b&c')).toBe('id=a+b%26c');
	});
});

describe('tracksQuery', () => {
	it('uses the parameter names the Tracks page reads', () => {
		expect(
			tracksQuery({ sensorId: 'hesai-pandar40p', clipId: 'case-1', runId: 'run-1', atNs: 42 })
		).toBe('sensor_id=hesai-pandar40p&clip_id=case-1&run_id=run-1&at_ns=42');
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
		const url = new URL(`http://localhost/app/lidar/clips?${clipQuery('a b&c')}`);
		expect(linkedId(url)).toBe('a b&c');
	});

	it('is null when nothing was linked', () => {
		expect(linkedId(new URL('http://localhost/app/lidar/runs'))).toBeNull();
		expect(linkedId(new URL('http://localhost/app/lidar/runs?id=%20'))).toBeNull();
	});
});

describe('windowsQuery and linkedRun', () => {
	it('carry the run as run, and read it back', () => {
		expect(windowsQuery('ad8b9438')).toBe('run=ad8b9438');
		expect(linkedRun(new URL(`http://x/app/lidar/windows?${windowsQuery('ad8b9438')}`))).toBe(
			'ad8b9438'
		);
	});

	it('is null when no run was linked', () => {
		expect(linkedRun(new URL('http://x/app/lidar/windows'))).toBeNull();
		expect(linkedRun(new URL('http://x/app/lidar/windows?run=%20'))).toBeNull();
	});
});

describe('visualiserLink', () => {
	it('opens a run at a frame in the decided scheme', () => {
		expect(visualiserLink('b4ca577a-7f05', 412)).toBe(
			'velocityreport://run/b4ca577a-7f05?frame=412'
		);
		expect(visualiserLink('run-1', 0)).toBe('velocityreport://run/run-1?frame=0');
	});

	it('opens at the start without a usable frame', () => {
		expect(visualiserLink('run-1')).toBe('velocityreport://run/run-1');
		expect(visualiserLink('run-1', null)).toBe('velocityreport://run/run-1');
		expect(visualiserLink('run-1', -1)).toBe('velocityreport://run/run-1');
		expect(visualiserLink('run-1', 2.5)).toBe('velocityreport://run/run-1');
	});

	it('encodes a run id a URL cannot carry as is', () => {
		expect(visualiserLink('a b/c', 1)).toBe('velocityreport://run/a%20b%2Fc?frame=1');
	});
});

describe('frameInstantNs', () => {
	it('floors the player clock to the millisecond and adds its slack, as nanosecond text', () => {
		expect(frameInstantNs(1_794_153_505_434.9)).toBe('1794153505436000000');
		expect(frameInstantNs(1_794_153_505_434)).toBe('1794153505436000000');
	});

	it('reaches the frame the clock shows, which can sit just over a millisecond later', () => {
		// The clock starts from the recording's start truncated to whole ms.
		const startNs = 1_788_290_733_019_394_330n;
		const frameNs = startNs + 300_000_123n;
		const clockMs = Number(startNs / 1_000_000n) + Number((frameNs - startNs) / 1_000n) / 1000;
		expect(BigInt(frameInstantNs(clockMs))).toBeGreaterThanOrEqual(frameNs);
		expect(BigInt(frameInstantNs(clockMs))).toBeLessThan(frameNs + 100_000_000n);
	});
});
