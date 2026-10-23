import {
	apiPath,
	defaultReplayRequest,
	withSettleRule,
	getAcceptanceMetrics,
	getAutoTuneStatus,
	getBackgroundRegions,
	getClustersChart,
	getDefaultLidarParams,
	getForegroundChart,
	getGridStatus,
	getHeatmapChart,
	getLidarStatus,
	getManualSweepStatus,
	getPlaybackStatus,
	getPolarChart,
	getSuspendedSweep,
	getTrafficChart,
	getTuningConfigs,
	gridChartPath,
	MAINTENANCE_ACTIONS,
	resumeAutoTune,
	replayFieldsForClip,
	resumeLive,
	runMaintenance,
	startAutoTune,
	startManualSweep,
	startReplay,
	stopAutoTune,
	stopManualSweep,
	stopReplay,
	suspendAutoTune,
	tuningConfigLabel,
	validateReplayRequest,
	waitForHINTChange,
	type MaintenanceAction,
	type ReplayStartRequest
} from './lidarControl';

const fetchMock = jest.fn();
global.fetch = fetchMock;

function respond(status: number, body: unknown) {
	const text = typeof body === 'string' ? body : JSON.stringify(body);
	return {
		ok: status >= 200 && status < 300,
		status,
		text: async () => text,
		json: async () => JSON.parse(text)
	} as Response;
}

function lastCall(): [string, RequestInit] {
	return fetchMock.mock.calls[fetchMock.mock.calls.length - 1] as [string, RequestInit];
}

const baseReplay: ReplayStartRequest = {
	pcap_file: 'kirk0.pcapng',
	analysis_mode: true,
	settle_before_recording: true,
	speed_mode: 'analysis',
	speed_ratio: 2,
	start_seconds: 0,
	duration_seconds: -1,
	tuning_config: ''
};

beforeEach(() => fetchMock.mockReset());

describe('apiPath', () => {
	it('drops empty and undefined query values', () => {
		expect(apiPath('/lidar/x', { a: 1, b: undefined, c: '', d: 'y z' })).toBe(
			'/api/lidar/x?a=1&d=y+z'
		);
		expect(apiPath('/lidar/x')).toBe('/api/lidar/x');
	});
});

describe('requests', () => {
	it('reads JSON on success and tolerates an empty body', async () => {
		fetchMock.mockResolvedValueOnce(respond(200, { status: 'ok', sensor_id: 's' }));
		await expect(getLidarStatus()).resolves.toMatchObject({ sensor_id: 's' });
		expect(lastCall()[0]).toBe('/api/lidar/status');
		expect(lastCall()[1].method).toBe('GET');

		fetchMock.mockResolvedValueOnce(respond(200, ''));
		await expect(stopManualSweep()).resolves.toEqual({});
	});

	it('throws the JSON error, the text, or a generic message', async () => {
		fetchMock.mockResolvedValueOnce(respond(409, { error: 'a replay is already running' }));
		await expect(getPlaybackStatus()).rejects.toThrow('a replay is already running');

		fetchMock.mockResolvedValueOnce(respond(400, 'bad sweep request\n'));
		await expect(startManualSweep({})).rejects.toThrow('bad sweep request');

		fetchMock.mockResolvedValueOnce(respond(500, ''));
		await expect(getTuningConfigs()).rejects.toThrow('Could not list tuning configs: 500');

		fetchMock.mockResolvedValueOnce(respond(500, { message: 'no error key' }));
		await expect(getAutoTuneStatus()).rejects.toThrow('{"message":"no error key"}');

		fetchMock.mockResolvedValueOnce({
			ok: false,
			status: 502,
			text: () => Promise.reject(new Error('stream'))
		} as unknown as Response);
		await expect(getManualSweepStatus()).rejects.toThrow('Could not load sweep status: 502');
	});
});

describe('replay', () => {
	it('ticks settle-before-recording and analysis by default, and validates', () => {
		const req = defaultReplayRequest();
		expect(req.settle_before_recording).toBe(true);
		expect(req.analysis_mode).toBe(true);
		expect(validateReplayRequest({ ...req, pcap_file: 'a.pcap' })).toBeNull();
	});

	it('turns analysis on when settle is chosen, and only then', () => {
		const off = { ...defaultReplayRequest(), analysis_mode: false, settle_before_recording: false };
		expect(withSettleRule(off)).toBe(off);
		expect(withSettleRule({ ...off, settle_before_recording: true }).analysis_mode).toBe(true);
	});

	it('labels tuning configs as the dropdown did', () => {
		expect(
			tuningConfigLabel({
				path: 'experiments/a.json',
				kind: 'tuning',
				solid_body: 'box',
				fingerprint: 'abc123',
				applicable: true,
				active: true
			})
		).toBe('experiments/a.json [box] abc123 (active)');
		expect(
			tuningConfigLabel({ path: 'b.json', kind: 'other', applicable: false, active: false })
		).toBe('b.json — cannot be applied');
		expect(
			tuningConfigLabel({
				path: 'c.json',
				kind: 'tuning',
				applicable: false,
				reason: 'changes startup settings',
				active: false
			})
		).toBe('c.json — changes startup settings');
	});

	it('validates a request before sending it', () => {
		expect(validateReplayRequest(baseReplay)).toBeNull();
		expect(validateReplayRequest({ ...baseReplay, pcap_file: ' ' })).toMatch(/capture file/);
		expect(validateReplayRequest({ ...baseReplay, analysis_mode: false })).toMatch(/Settling/);
		expect(validateReplayRequest({ ...baseReplay, speed_mode: 'scaled', speed_ratio: 0 })).toMatch(
			/speed ratio/
		);
		expect(validateReplayRequest({ ...baseReplay, start_seconds: -1 })).toMatch(/Start/);
		expect(validateReplayRequest({ ...baseReplay, duration_seconds: 0 })).toMatch(/Duration/);
		expect(validateReplayRequest({ ...baseReplay, duration_seconds: -2 })).toMatch(/Duration/);
	});

	it('drops the ratio unless scaled and an empty tuning config', async () => {
		fetchMock.mockResolvedValue(respond(200, { status: 'started' }));
		await startReplay('s 1', baseReplay);
		const [url, first] = lastCall();
		expect(url).toBe('/api/lidar/pcap/start?sensor_id=s+1');
		expect(first.method).toBe('POST');
		let body = JSON.parse(first.body as string);
		expect(body).not.toHaveProperty('speed_ratio');
		expect(body).not.toHaveProperty('tuning_config');
		expect(body.analysis_mode).toBe(true);

		await startReplay('s', { ...baseReplay, speed_mode: 'scaled', tuning_config: 'a.json' });
		body = JSON.parse(lastCall()[1].body as string);
		expect(body.speed_ratio).toBe(2);
		expect(body.tuning_config).toBe('a.json');
	});

	it('sends full frames only when asked, and off by default', async () => {
		expect(defaultReplayRequest().full_frames).toBe(false);
		fetchMock.mockResolvedValue(respond(200, { status: 'started', full_frames: true }));
		await startReplay('s', { ...baseReplay, full_frames: true });
		expect(JSON.parse(lastCall()[1].body as string).full_frames).toBe(true);
		await startReplay('s', { ...baseReplay, full_frames: false });
		expect(JSON.parse(lastCall()[1].body as string).full_frames).toBe(false);
	});

	it('fills the form from a clip', () => {
		const base = { clip_id: 'c', sensor_id: 's', created_at_ns: 0, file_count: 1 };
		// A listing carries no captures, only the trim.
		expect(replayFieldsForClip(base)).toEqual({
			pcap_file: '',
			pcap_files: undefined,
			start_seconds: 0,
			duration_seconds: -1
		});
		expect(
			replayFieldsForClip({ ...base, files: [{ ordinal: 0, pcap_file: 'a.pcap' }] }).pcap_file
		).toBe('a.pcap');
		expect(
			replayFieldsForClip({
				...base,
				pcap_start_secs: 12,
				pcap_duration_secs: 30,
				file_count: 2,
				files: [
					{ ordinal: 1, pcap_file: 'b.pcap' },
					{ ordinal: 0, pcap_file: 'a.pcap' }
				]
			})
		).toEqual({
			pcap_file: 'a.pcap',
			pcap_files: ['a.pcap', 'b.pcap'],
			start_seconds: 12,
			duration_seconds: 30
		});
		expect(replayFieldsForClip({ ...base, pcap_duration_secs: 0 }).duration_seconds).toBe(-1);
	});

	it('sends the ordered captures of a multi-file clip', async () => {
		fetchMock.mockResolvedValue(respond(200, { status: 'started' }));
		await startReplay('s', { ...baseReplay, pcap_files: ['a', 'b'] });
		expect(JSON.parse(lastCall()[1].body as string).pcap_files).toEqual(['a', 'b']);
		await startReplay('s', { ...baseReplay, pcap_files: [] });
		expect(JSON.parse(lastCall()[1].body as string)).not.toHaveProperty('pcap_files');
	});

	it('stops and resumes on the canonical routes', async () => {
		fetchMock.mockResolvedValue(respond(200, {}));
		await stopReplay('s');
		expect(lastCall()[0]).toBe('/api/lidar/replay/stop?sensor_id=s');
		await resumeLive('s');
		expect(lastCall()[0]).toBe('/api/lidar/pcap/resume_live?sensor_id=s');
	});
});

describe('maintenance and grid', () => {
	it('posts each maintenance action with the sensor', async () => {
		fetchMock.mockResolvedValue(respond(200, { status: 'ok' }));
		for (const which of Object.keys(MAINTENANCE_ACTIONS) as MaintenanceAction[]) {
			await runMaintenance('s', which);
			expect(lastCall()[0]).toBe(`/api${MAINTENANCE_ACTIONS[which].path}?sensor_id=s`);
			expect(lastCall()[1].method).toBe('POST');
		}
	});

	it('reads the default parameters', async () => {
		fetchMock.mockResolvedValue(respond(200, { version: 2 }));
		await expect(getDefaultLidarParams('s')).resolves.toEqual({ version: 2 });
		expect(lastCall()[0]).toBe('/api/lidar/params?sensor_id=s&source=default');
	});

	it('reads grid status and acceptance metrics', async () => {
		fetchMock.mockResolvedValue(respond(200, { a: 1 }));
		await expect(getGridStatus('s')).resolves.toEqual({ a: 1 });
		expect(lastCall()[0]).toBe('/api/lidar/grid_status?sensor_id=s');
		await getAcceptanceMetrics('s');
		expect(lastCall()[0]).toBe('/api/lidar/acceptance?sensor_id=s');
	});

	it('builds the grid chart path', () => {
		expect(gridChartPath('s', 'fill_rate')).toBe(
			'/api/charts/grid?sensor_id=s&metric=fill_rate&azimuth_bucket_deg=3'
		);
		expect(gridChartPath('s', 'mean_times_seen', 6)).toContain('azimuth_bucket_deg=6');
	});
});

describe('debug charts', () => {
	it('returns null for a 404 and data otherwise', async () => {
		fetchMock.mockResolvedValueOnce(respond(404, { error: 'none yet' }));
		await expect(getPolarChart('s')).resolves.toBeNull();
		expect(lastCall()[0]).toBe('/api/lidar/chart/polar?sensor_id=s&max_points=8000');

		fetchMock.mockResolvedValueOnce(respond(200, { points: [], num_buckets: 0 }));
		await expect(getHeatmapChart('s')).resolves.toMatchObject({ num_buckets: 0 });
		expect(lastCall()[0]).toBe(
			'/api/lidar/chart/heatmap?sensor_id=s&azimuth_bucket_deg=3&settled_threshold=5'
		);

		fetchMock.mockResolvedValueOnce(respond(200, { foreground_points: [] }));
		await getForegroundChart('s');
		expect(lastCall()[0]).toBe('/api/lidar/chart/foreground?sensor_id=s');

		fetchMock.mockResolvedValueOnce(respond(200, { clusters: [] }));
		await getClustersChart('s', 50);
		expect(lastCall()[0]).toBe('/api/lidar/chart/clusters?sensor_id=s&limit=50');

		fetchMock.mockResolvedValueOnce(respond(200, { packets_per_sec: 1 }));
		await getTrafficChart();
		expect(lastCall()[0]).toBe('/api/lidar/chart/traffic');

		fetchMock.mockResolvedValueOnce(respond(200, { region_count: 0 }));
		await getBackgroundRegions('s');
		expect(lastCall()[0]).toBe('/api/lidar/background/regions?sensor_id=s');
	});

	it('throws on other failures', async () => {
		fetchMock.mockResolvedValueOnce(respond(503, { error: 'track DB not configured' }));
		await expect(getClustersChart('s')).rejects.toThrow('track DB not configured');
	});
});

describe('sweep control', () => {
	it('routes each control to its endpoint', async () => {
		fetchMock.mockResolvedValue(respond(200, {}));
		const cases: Array<[() => Promise<unknown>, string, string]> = [
			[() => startManualSweep({ mode: 'params' }), '/api/lidar/sweep/start', 'POST'],
			[() => getManualSweepStatus(), '/api/lidar/sweep/status', 'GET'],
			[() => stopManualSweep(), '/api/lidar/sweep/stop', 'POST'],
			[() => startAutoTune({}), '/api/lidar/sweep/auto', 'POST'],
			[() => getAutoTuneStatus(), '/api/lidar/sweep/auto', 'GET'],
			[() => stopAutoTune(), '/api/lidar/sweep/auto/stop', 'POST'],
			[() => suspendAutoTune(), '/api/lidar/sweep/auto/suspend', 'POST'],
			[() => getSuspendedSweep(), '/api/lidar/sweep/auto/suspended', 'GET']
		];
		for (const [call, url, method] of cases) {
			await call();
			expect(lastCall()[0]).toBe(url);
			expect(lastCall()[1].method).toBe(method);
		}
	});

	it('sends the checkpoint id on resume when there is one', async () => {
		fetchMock.mockResolvedValue(respond(200, {}));
		await resumeAutoTune('sw-1');
		expect(JSON.parse(lastCall()[1].body as string)).toEqual({ sweep_id: 'sw-1' });
		await resumeAutoTune();
		expect(JSON.parse(lastCall()[1].body as string)).toEqual({});
	});

	it('long-polls the HINT state with the caller signal', async () => {
		fetchMock.mockResolvedValue(respond(200, { status: 'awaiting_labels' }));
		const controller = new AbortController();
		await waitForHINTChange('running_reference', controller.signal);
		expect(lastCall()[0]).toBe('/api/lidar/sweep/hint?wait_for_change=running_reference');
		expect(lastCall()[1].signal).toBe(controller.signal);
	});
});
