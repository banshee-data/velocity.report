import {
	areStoredReportSettingsFresh,
	DAY_PERIOD_TYPE,
	isDateRangeStale,
	normaliseStoredPeriodType,
	parseStoredReportSettings,
	REPORT_SETTINGS_KEY,
	STALENESS_THRESHOLD_MS
} from './reportSettings';

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

describe('parseStoredReportSettings', () => {
	it('returns null for missing localStorage data', () => {
		expect(parseStoredReportSettings(null)).toBeNull();
	});

	it('returns null for invalid JSON', () => {
		expect(parseStoredReportSettings('{nope')).toBeNull();
	});

	it('returns the parsed object for valid JSON', () => {
		const settings = parseStoredReportSettings(
			JSON.stringify({ selectedSource: 'radar_data_transits', minSpeed: 5 })
		);

		expect(settings).toEqual({ selectedSource: 'radar_data_transits', minSpeed: 5 });
	});

	it('returns null for arrays (typeof [] === "object")', () => {
		expect(parseStoredReportSettings('[]')).toBeNull();
		expect(parseStoredReportSettings('[{"selectedSource":"radar_objects"}]')).toBeNull();
	});
});

describe('areStoredReportSettingsFresh', () => {
	it('returns false when settings are missing', () => {
		expect(areStoredReportSettingsFresh(null)).toBe(false);
	});

	it('returns false when the saved date-range timestamp is stale', () => {
		const staleSavedAt = new Date(Date.now() - 19 * 60 * 60 * 1000).toISOString();
		expect(
			areStoredReportSettingsFresh({
				dateRange: { savedAt: staleSavedAt },
				selectedSource: 'radar_objects'
			})
		).toBe(false);
	});

	it('returns true when the saved date-range timestamp is fresh', () => {
		const freshSavedAt = new Date(Date.now() - 60 * 60 * 1000).toISOString();
		expect(
			areStoredReportSettingsFresh({
				dateRange: { savedAt: freshSavedAt },
				selectedSource: 'radar_data_transits'
			})
		).toBe(true);
	});
});

describe('normaliseStoredPeriodType', () => {
	it('accepts a numeric PeriodType value', () => {
		expect(normaliseStoredPeriodType(DAY_PERIOD_TYPE)).toBe(DAY_PERIOD_TYPE);
	});

	it('accepts a stringified enum value', () => {
		expect(normaliseStoredPeriodType('10')).toBe(DAY_PERIOD_TYPE);
	});

	it('accepts the symbolic day code', () => {
		expect(normaliseStoredPeriodType('day')).toBe(DAY_PERIOD_TYPE);
	});

	it('falls back to day for unsupported values', () => {
		expect(normaliseStoredPeriodType('quarter')).toBe(DAY_PERIOD_TYPE);
		expect(normaliseStoredPeriodType(999)).toBe(DAY_PERIOD_TYPE);
	});
});
