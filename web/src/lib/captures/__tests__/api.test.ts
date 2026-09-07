/**
 * The capture API client: what it asks for, and what it does with an error.
 */
import {
	cancelCaptureJob,
	clearReplayCaseLocation,
	createReplayCaseFromCaptures,
	getCaptureFiles,
	getCaptureJobs,
	getCapturePeriods,
	getCaptureRoots,
	getCaptureSessions,
	getSceneMap,
	scanCaptureRoots,
	setReplayCaseLocation,
	setCaptureSessionLabel,
	startCaptureMotionPass
} from '$lib/api';

const fetchMock = jest.fn();
global.fetch = fetchMock as unknown as typeof fetch;

function ok(body: unknown) {
	return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(body) });
}

function fail(status: number, body?: unknown) {
	return Promise.resolve({
		ok: false,
		status,
		json: () => (body === undefined ? Promise.reject(new Error('not json')) : Promise.resolve(body))
	});
}

/** The URL the client asked for on its most recent call. */
function lastUrl(): string {
	return String(fetchMock.mock.calls[fetchMock.mock.calls.length - 1][0]);
}

beforeEach(() => fetchMock.mockReset());

describe('getCaptureRoots', () => {
	it('returns the roots', async () => {
		fetchMock.mockReturnValue(
			ok({ roots: [{ root_id: 'root-1', path: '/Volumes/lidar' }], count: 1 })
		);
		const roots = await getCaptureRoots();
		expect(roots).toHaveLength(1);
		expect(lastUrl()).toContain('/lidar/capture/roots');
	});

	it('returns an empty list rather than undefined when there are none', async () => {
		fetchMock.mockReturnValue(ok({ count: 0 }));
		await expect(getCaptureRoots()).resolves.toEqual([]);
	});

	it('throws on a failure', async () => {
		fetchMock.mockReturnValue(fail(503));
		await expect(getCaptureRoots()).rejects.toThrow();
	});
});

describe('scanCaptureRoots', () => {
	it('scans every root and probes by default', async () => {
		fetchMock.mockReturnValue(ok({ roots: [], count: 0 }));
		await scanCaptureRoots();
		expect(lastUrl()).not.toContain('probe=');
		expect(fetchMock.mock.calls[0][1]).toMatchObject({ method: 'POST' });
	});

	it('asks for a metadata-only scan when probing is declined', async () => {
		// Probing reads every new capture in full; a quick listing must not.
		fetchMock.mockReturnValue(ok({ roots: [], count: 0 }));
		await scanCaptureRoots({ probe: false });
		expect(lastUrl()).toContain('probe=false');
	});

	it('scopes to one root when asked', async () => {
		fetchMock.mockReturnValue(ok({ roots: [], count: 0 }));
		await scanCaptureRoots({ rootId: 'root-1' });
		expect(lastUrl()).toContain('root_id=root-1');
	});
});

describe('getCaptureFiles', () => {
	it('scopes to a session when one is given', async () => {
		fetchMock.mockReturnValue(ok({ files: [], count: 0 }));
		await getCaptureFiles({ sessionId: 'ses-1' });
		expect(lastUrl()).toContain('session_id=ses-1');
	});

	it('prefers the session over the root, which would widen the answer', async () => {
		fetchMock.mockReturnValue(ok({ files: [], count: 0 }));
		await getCaptureFiles({ sessionId: 'ses-1', rootId: 'root-1' });
		expect(lastUrl()).toContain('session_id=ses-1');
		expect(lastUrl()).not.toContain('root_id');
	});

	it('scopes to a root when there is no session', async () => {
		fetchMock.mockReturnValue(ok({ files: [], count: 0 }));
		await getCaptureFiles({ rootId: 'root-1' });
		expect(lastUrl()).toContain('root_id=root-1');
	});
});

describe('getCaptureSessions', () => {
	it('returns the sessions', async () => {
		fetchMock.mockReturnValue(ok({ sessions: [{ session_id: 'ses-1' }], count: 1 }));
		await expect(getCaptureSessions()).resolves.toHaveLength(1);
	});
});

describe('getCapturePeriods', () => {
	it('encodes the session id', async () => {
		fetchMock.mockReturnValue(ok({ session_id: 'ses/1', periods: [], count: 0 }));
		await getCapturePeriods('ses/1');
		expect(lastUrl()).toContain('session_id=ses%2F1');
	});
});

describe('startCaptureMotionPass', () => {
	it('returns the queued job', async () => {
		fetchMock.mockReturnValue(ok({ job: { job_id: 'job-1', state: 'queued' } }));
		const job = await startCaptureMotionPass('ses-1');
		expect(job.job_id).toBe('job-1');
		expect(fetchMock.mock.calls[0][1]).toMatchObject({ method: 'POST' });
	});
});

describe('getCaptureJobs', () => {
	it('passes a limit through', async () => {
		fetchMock.mockReturnValue(ok({ jobs: [], count: 0 }));
		await getCaptureJobs({ limit: 25 });
		expect(lastUrl()).toContain('limit=25');
	});
});

describe('cancelCaptureJob', () => {
	it('throws when the job is already finished', async () => {
		fetchMock.mockReturnValue(fail(404));
		await expect(cancelCaptureJob('job-1')).rejects.toThrow();
	});
});

describe('setCaptureSessionLabel', () => {
	it('sends the name and sensor', async () => {
		fetchMock.mockReturnValue(ok({}));
		await setCaptureSessionLabel('ses-1', 'broadway_columbus', 'hesai-pandar40p');
		const body = JSON.parse(String(fetchMock.mock.calls[0][1].body));
		expect(body).toEqual({
			session_id: 'ses-1',
			label: 'broadway_columbus',
			sensor_id: 'hesai-pandar40p'
		});
	});

	it('sends an empty sensor rather than undefined, which would drop the key', async () => {
		fetchMock.mockReturnValue(ok({}));
		await setCaptureSessionLabel('ses-1', 'broadway_columbus');
		expect(JSON.parse(String(fetchMock.mock.calls[0][1].body)).sensor_id).toBe('');
	});
});

describe('createReplayCaseFromCaptures', () => {
	it('posts the ordered capture list', async () => {
		fetchMock.mockReturnValue(ok({ replay_case_id: 'case-1' }));
		await createReplayCaseFromCaptures({
			sensor_id: 'hesai-pandar40p',
			pcap_files: ['file7.pcap', 'file8.pcap'],
			pcap_start_secs: 82,
			pcap_duration_secs: 449.4
		});
		const body = JSON.parse(String(fetchMock.mock.calls[0][1].body));
		expect(body.pcap_files).toEqual(['file7.pcap', 'file8.pcap']);
		expect(body.pcap_start_secs).toBe(82);
	});

	it('surfaces the server reason for a refusal', async () => {
		// The server refuses captures that do not abut, and its message names
		// the join. Swallowing it for a generic failure would leave the
		// operator with nothing to act on.
		fetchMock.mockReturnValue(
			fail(400, {
				error:
					'the case captures do not form one continuous stream: the join file7.pcap → file9.pcap is broken (gap 10m0s)'
			})
		);
		await expect(
			createReplayCaseFromCaptures({ sensor_id: 's', pcap_files: ['file7.pcap', 'file9.pcap'] })
		).rejects.toThrow(/file7\.pcap/);
	});

	it('falls back to a generic message when the body is not JSON', async () => {
		fetchMock.mockReturnValue(fail(500));
		await expect(
			createReplayCaseFromCaptures({ sensor_id: 's', pcap_files: ['a.pcap'] })
		).rejects.toThrow(/replay case/i);
	});
});

describe('setReplayCaseLocation', () => {
	it('sends only the position, never tokens', async () => {
		// The S2 family is derived server-side with Parent. A family assembled
		// by a client could disagree with itself.
		fetchMock.mockReturnValue(
			ok({ location: { s2_l10_token: '808581', s2_l10_display: '80858-1' } })
		);
		await setReplayCaseLocation('case-1', {
			origin_lat: 37.7987,
			origin_lon: -122.4073,
			geographic_source: 'surveyed'
		});
		const body = JSON.parse(String(fetchMock.mock.calls[0][1].body));
		expect(body).toEqual({
			origin_lat: 37.7987,
			origin_lon: -122.4073,
			geographic_source: 'surveyed'
		});
		expect(Object.keys(body).some((k) => k.startsWith('s2_'))).toBe(false);
	});

	it('returns the derived location', async () => {
		fetchMock.mockReturnValue(
			ok({
				location: {
					s2_l10_token: '808581',
					s2_l10_display: '80858-1',
					s2_l13_display: '80858-004',
					s2_l16_display: '80858-0f3f'
				}
			})
		);
		const loc = await setReplayCaseLocation('case-1', { origin_lat: 37.8, origin_lon: -122.4 });
		expect(loc.s2_l10_display).toBe('80858-1');
		expect(loc.s2_l16_display).toBe('80858-0f3f');
	});

	it('surfaces the server reason for a rejected position', async () => {
		fetchMock.mockReturnValue(
			fail(400, { error: 'geoindex: no usable WGS84 position: 0,0 is an absent fix' })
		);
		await expect(setReplayCaseLocation('case-1', { origin_lat: 0, origin_lon: 0 })).rejects.toThrow(
			/absent fix/
		);
	});
});

describe('clearReplayCaseLocation', () => {
	it('deletes the location', async () => {
		fetchMock.mockReturnValue(ok({}));
		await clearReplayCaseLocation('case-1');
		expect(fetchMock.mock.calls[0][1]).toMatchObject({ method: 'DELETE' });
		expect(lastUrl()).toContain('/location');
	});
});

describe('getSceneMap', () => {
	it('returns the sites', async () => {
		fetchMock.mockReturnValue(
			ok({
				sites: [{ s2_l10_token: '808581', s2_l10_display: '80858-1', case_count: 2, cases: [] }],
				site_count: 1,
				case_count: 2,
				coarse_level: 10,
				fine_level: 13,
				precise_level: 16
			})
		);
		const map = await getSceneMap();
		expect(map.site_count).toBe(1);
		// The levels are reported so the page need not hard-code them.
		expect(map.coarse_level).toBe(10);
		expect(map.precise_level).toBe(16);
	});

	it('throws on a failure', async () => {
		fetchMock.mockReturnValue(fail(503));
		await expect(getSceneMap()).rejects.toThrow();
	});
});
