import { isDateRangeStale, STALENESS_THRESHOLD_MS, REPORT_SETTINGS_KEY } from './reportSettings';

describe('isDateRangeStale', () => {
	afterEach(() => {
		jest.useRealTimers();
	});

	it('returns true when savedAt is undefined', () => {
		expect(isDateRangeStale(undefined)).toBe(true);
	});

	it('returns true when savedAt is an empty string', () => {
		expect(isDateRangeStale('')).toBe(true);
	});

	it('returns true when savedAt is invalid/garbage', () => {
		expect(isDateRangeStale('not-a-date')).toBe(true);
		expect(isDateRangeStale('abc123')).toBe(true);
	});

	it('returns false when savedAt is 1 hour ago', () => {
		const oneHourAgo = new Date(Date.now() - 1 * 60 * 60 * 1000).toISOString();
		expect(isDateRangeStale(oneHourAgo)).toBe(false);
	});

	it('returns false when savedAt is just now', () => {
		expect(isDateRangeStale(new Date().toISOString())).toBe(false);
	});

	it('returns true when savedAt is 19 hours ago', () => {
		const nineteenHoursAgo = new Date(Date.now() - 19 * 60 * 60 * 1000).toISOString();
		expect(isDateRangeStale(nineteenHoursAgo)).toBe(true);
	});

	it('returns true when savedAt is 2 days ago', () => {
		const twoDaysAgo = new Date(Date.now() - 2 * 24 * 60 * 60 * 1000).toISOString();
		expect(isDateRangeStale(twoDaysAgo)).toBe(true);
	});

	it('returns false at exactly the threshold boundary', () => {
		jest.useFakeTimers();
		const now = new Date('2026-04-07T12:00:00.000Z');
		jest.setSystemTime(now);

		const exactlyAtThreshold = new Date(now.getTime() - STALENESS_THRESHOLD_MS).toISOString();
		// At exactly the boundary, Date.now() - savedTime === STALENESS_THRESHOLD_MS,
		// and the check is strictly greater-than, so this should NOT be stale.
		expect(isDateRangeStale(exactlyAtThreshold)).toBe(false);
	});

	it('returns true one millisecond past the threshold', () => {
		jest.useFakeTimers();
		const now = new Date('2026-04-07T12:00:00.000Z');
		jest.setSystemTime(now);

		const justPast = new Date(now.getTime() - STALENESS_THRESHOLD_MS - 1).toISOString();
		expect(isDateRangeStale(justPast)).toBe(true);
	});
});

describe('constants', () => {
	it('STALENESS_THRESHOLD_MS is 18 hours', () => {
		expect(STALENESS_THRESHOLD_MS).toBe(18 * 60 * 60 * 1000);
	});

	it('REPORT_SETTINGS_KEY is reportSettings', () => {
		expect(REPORT_SETTINGS_KEY).toBe('reportSettings');
	});
});
