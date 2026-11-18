import { analyseFollowing } from './followingAnalysis';

describe('following evidence analysis', () => {
	afterEach(() => jest.restoreAllMocks());
	it('uses a run-bound POST and forwards cancellation', async () => {
		const body = { schema: 'following_analysis_v1', histogram: Array(5).fill({ count: 0 }) };
		global.fetch = jest.fn().mockResolvedValue({ ok: true, json: async () => body });
		const signal = new AbortController().signal;
		expect(await analyseFollowing('run/one', signal)).toBe(body);
		expect(fetch).toHaveBeenCalledWith('/api/lidar/runs/run%2Fone/following-review/analysis', {
			method: 'POST',
			signal
		});
	});
	it('retains an actionable backend refusal', async () => {
		global.fetch = jest.fn().mockResolvedValue({
			ok: false,
			json: async () => ({ error: 'paired evidence mismatched' })
		});
		await expect(analyseFollowing('one', new AbortController().signal)).rejects.toThrow(
			'paired evidence mismatched'
		);
	});
	it('refuses an incompatible histogram and handles errors without details', async () => {
		global.fetch = jest.fn().mockResolvedValue({ ok: true, json: async () => ({ schema: 'old' }) });
		await expect(analyseFollowing('one', new AbortController().signal)).rejects.toThrow(
			'Unsupported'
		);
		global.fetch = jest.fn().mockResolvedValue({ ok: false, json: async () => ({}) });
		await expect(analyseFollowing('one', new AbortController().signal)).rejects.toThrow(
			'unavailable'
		);
	});
});
