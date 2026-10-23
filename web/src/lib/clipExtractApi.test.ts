import {
	ensurePeriodClip,
	getClipBackground,
	getLocatedClips,
	getClipExtracts,
	getPeriodBackground,
	setClipPublishing,
	stageClip,
	startClipExtract
} from './api';

global.fetch = jest.fn();

function respond(ok: boolean, status: number, body: unknown) {
	(global.fetch as jest.Mock).mockResolvedValueOnce({
		ok,
		status,
		json: async () => body
	});
}

function respondNotJSON(status: number) {
	(global.fetch as jest.Mock).mockResolvedValueOnce({
		ok: false,
		status,
		json: async () => {
			throw new SyntaxError('not JSON');
		}
	});
}

function lastCall(): [string, RequestInit | undefined] {
	const calls = (global.fetch as jest.Mock).mock.calls;
	const [url, init] = calls[calls.length - 1];
	return [url.toString(), init];
}

describe('clip extract and publishing API', () => {
	beforeEach(() => {
		(global.fetch as jest.Mock).mockReset();
	});

	it('lists a clip’s extracts', async () => {
		respond(true, 200, { clip_id: 'c1', extracts: [{ job_id: 'job-1', state: 'completed' }] });
		const extracts = await getClipExtracts('c1');
		expect(extracts).toHaveLength(1);
		expect(lastCall()[0]).toContain('/lidar/clips/c1/extract');
	});

	it('treats a missing list as empty', async () => {
		respond(true, 200, { clip_id: 'c1' });
		expect(await getClipExtracts('c1')).toEqual([]);
	});

	it('reports the server’s reason when listing fails', async () => {
		respond(false, 500, { error: 'disk unplugged' });
		await expect(getClipExtracts('c1')).rejects.toThrow('disk unplugged');
	});

	it('queues an extract with a POST', async () => {
		respond(true, 202, { job: { job_id: 'job-1' }, created: true });
		const result = await startClipExtract('c1');
		expect(result).toEqual({ job: { job_id: 'job-1' }, created: true });
		const [url, init] = lastCall();
		expect(url).toContain('/lidar/clips/c1/extract');
		expect(init?.method).toBe('POST');
	});

	it('falls back to its own message when the error is not JSON', async () => {
		respondNotJSON(502);
		await expect(startClipExtract('c1')).rejects.toThrow('Could not queue the extract');
	});

	it('puts the publishing settings', async () => {
		respond(true, 200, { clip_id: 'c1', dataset_id: 'kirkham-9th-k3', storage: 'extract' });
		const clip = await setClipPublishing('c1', {
			dataset_id: 'kirkham-9th-k3',
			north_azimuth_deg: 131,
			storage: 'extract'
		});
		expect(clip.dataset_id).toBe('kirkham-9th-k3');
		const [url, init] = lastCall();
		expect(url).toContain('/lidar/clips/c1/publishing');
		expect(init?.method).toBe('PUT');
		expect(JSON.parse(init?.body as string)).toEqual({
			dataset_id: 'kirkham-9th-k3',
			north_azimuth_deg: 131,
			storage: 'extract'
		});
	});

	it('surfaces a dataset id clash', async () => {
		respond(false, 409, { error: 'dataset id is already used by another clip' });
		await expect(
			setClipPublishing('c1', { dataset_id: 'x', north_azimuth_deg: null, storage: 'sources' })
		).rejects.toThrow('already used');
	});

	it('stages a clip', async () => {
		respond(true, 200, { clip_id: 'c1', raw_path: 'raw/lidar/a.pcapng', manifest_entry: {} });
		const result = await stageClip('c1');
		expect(result.raw_path).toBe('raw/lidar/a.pcapng');
		const [url, init] = lastCall();
		expect(url).toContain('/lidar/clips/c1/stage');
		expect(init?.method).toBe('POST');
	});

	it('reports why staging was refused', async () => {
		respond(false, 409, { error: 'the clip is not located' });
		await expect(stageClip('c1')).rejects.toThrow('not located');
	});

	it('uses its own message when the refusal has no reason', async () => {
		respond(false, 409, {});
		await expect(stageClip('c1')).rejects.toThrow('Could not stage the clip');
	});

	it('fetches a period background by id', async () => {
		respond(true, 200, { period_id: 'per 1', count: 1, xyz: [1, 2, 3] });
		const bg = await getPeriodBackground('per 1');
		expect(bg.xyz).toEqual([1, 2, 3]);
		expect(lastCall()[0]).toContain('/lidar/capture/periods/background?period_id=per%201');
		respond(false, 500, { error: 'settled background unavailable' });
		await expect(getPeriodBackground('p')).rejects.toThrow('unavailable');
	});

	it('asks for the period clip with a POST', async () => {
		respond(true, 201, { clip_id: 'c9' });
		expect((await ensurePeriodClip('per-1')).clip_id).toBe('c9');
		const [url, init] = lastCall();
		expect(url).toContain('/lidar/capture/periods/clip');
		expect(init?.method).toBe('POST');
		expect(JSON.parse(init?.body as string)).toEqual({ period_id: 'per-1' });
		respondNotJSON(500);
		await expect(ensurePeriodClip('per-1')).rejects.toThrow('Could not make a clip');
	});

	it('fetches a clip window background and the located clips', async () => {
		respond(true, 200, { clip_id: 'c1', xyz: [] });
		expect((await getClipBackground('c1')).clip_id).toBe('c1');
		expect(lastCall()[0]).toContain('/lidar/clips/c1/background');
		respond(false, 404, { error: 'clip not found' });
		await expect(getClipBackground('x')).rejects.toThrow('clip not found');

		respond(true, 200, { clips: [{ clip_id: 'c1' }] });
		expect(await getLocatedClips()).toHaveLength(1);
		expect(lastCall()[0]).toContain('/lidar/clips?located=1');
		respond(true, 200, {});
		expect(await getLocatedClips()).toEqual([]);
		respondNotJSON(500);
		await expect(getLocatedClips()).rejects.toThrow('Could not load the located clips');
	});
});
