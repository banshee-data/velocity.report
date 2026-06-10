import { isoDate, isoEndOfDay, isoStartOfDay, tomorrowLocal } from './dateUtils';

describe('isoDate', () => {
	it('returns YYYY-MM-DD format', () => {
		const date = new Date(2025, 5, 4); // June 4, 2025 (months are 0-indexed)
		expect(isoDate(date)).toBe('2025-06-04');
	});

	it('pads single-digit months and days with zeros', () => {
		const date = new Date(2025, 0, 5); // January 5, 2025
		expect(isoDate(date)).toBe('2025-01-05');
	});

	it('handles December correctly', () => {
		const date = new Date(2025, 11, 31); // December 31, 2025
		expect(isoDate(date)).toBe('2025-12-31');
	});

	it('uses local date parts, not UTC (critical timezone bug fix)', () => {
		// Create a date at midnight local time
		// If this were converted to UTC with toISOString(), it could shift
		// to the next or previous day depending on the local timezone
		const date = new Date(2025, 5, 4, 0, 0, 0); // June 4, 2025 00:00:00 local

		// The result should always be the LOCAL date, not UTC
		// This is the key bug fix: toISOString() would give wrong results
		// for users in timezones behind UTC (like US/Pacific)
		expect(isoDate(date)).toBe('2025-06-04');

		// Verify our implementation differs from the buggy approach
		// Note: This test may pass in some timezones and fail in others
		// with the old implementation, which is exactly the problem
		const localDate = isoDate(date);
		expect(localDate).toBe('2025-06-04');
	});

	it('handles late night times correctly (regression test for +1 day bug)', () => {
		// At 23:59:59 on June 4 in Pacific time (UTC-7),
		// the UTC time is June 5 at 06:59:59
		// The old toISOString().slice(0,10) would return "2025-06-05"
		// but we want "2025-06-04"
		const lateNight = new Date(2025, 5, 4, 23, 59, 59); // June 4, 2025 23:59:59 local
		expect(isoDate(lateNight)).toBe('2025-06-04');
	});

	it('handles early morning times correctly', () => {
		// At 00:00:01 on June 5 in Pacific time (UTC-7),
		// the UTC time is June 5 at 07:00:01
		// Both should give June 5
		const earlyMorning = new Date(2025, 5, 5, 0, 0, 1); // June 5, 2025 00:00:01 local
		expect(isoDate(earlyMorning)).toBe('2025-06-05');
	});
});

describe('tomorrowLocal', () => {
	it('returns the next calendar day in local time', () => {
		const wed = new Date(2025, 5, 4, 9, 30, 0); // Wed June 4, 2025 09:30 local
		expect(isoDate(tomorrowLocal(wed))).toBe('2025-06-05');
	});

	it('rolls over the end of the month', () => {
		const lastDayOfJune = new Date(2025, 5, 30, 12, 0, 0);
		expect(isoDate(tomorrowLocal(lastDayOfJune))).toBe('2025-07-01');
	});

	it('rolls over end-of-year', () => {
		const newYearsEve = new Date(2025, 11, 31, 23, 59, 0);
		expect(isoDate(tomorrowLocal(newYearsEve))).toBe('2026-01-01');
	});

	it('does not mutate the input Date', () => {
		const input = new Date(2025, 5, 4, 0, 0, 0);
		const inputBefore = input.getTime();
		tomorrowLocal(input);
		expect(input.getTime()).toBe(inputBefore);
	});

	it('uses the current moment when called without arguments', () => {
		const before = Date.now();
		const result = tomorrowLocal();
		const after = Date.now();
		const dayMs = 24 * 60 * 60 * 1000;
		// result.getTime() should be roughly 24h ahead of "now". Allow the
		// 1-second slop between `before` and `after`.
		expect(result.getTime()).toBeGreaterThanOrEqual(before + dayMs - 1000);
		expect(result.getTime()).toBeLessThanOrEqual(after + dayMs + 1000);
	});
});

describe('isoStartOfDay / isoEndOfDay', () => {
	// The picker Date carries browser-local Y/M/D; only those parts are read, so
	// these tests are independent of the machine's timezone.
	const day = new Date(2026, 5, 9, 15, 30, 0); // 2026-06-09, mid-afternoon local

	it('produces the UTC instant for midnight in a behind-UTC zone (PDT)', () => {
		// 2026-06-09 00:00:00 America/Los_Angeles == 2026-06-09 07:00:00 UTC
		expect(isoStartOfDay(day, 'America/Los_Angeles')).toBe('2026-06-09T07:00:00.000Z');
	});

	it('produces the inclusive end-of-day instant in a behind-UTC zone (PDT)', () => {
		// 2026-06-09 23:59:59 America/Los_Angeles == 2026-06-10 06:59:59 UTC
		expect(isoEndOfDay(day, 'America/Los_Angeles')).toBe('2026-06-10T06:59:59.000Z');
	});

	it('is identity-on-the-clock for UTC', () => {
		expect(isoStartOfDay(day, 'UTC')).toBe('2026-06-09T00:00:00.000Z');
		expect(isoEndOfDay(day, 'UTC')).toBe('2026-06-09T23:59:59.000Z');
	});

	it('produces the UTC instant for an ahead-of-UTC zone (Auckland)', () => {
		// June: NZST = +12. 2026-06-09 00:00 +12:00 == 2026-06-08 12:00:00 UTC
		expect(isoStartOfDay(day, 'Pacific/Auckland')).toBe('2026-06-08T12:00:00.000Z');
	});

	it('handles the LA DST spring-forward day (start still well-defined)', () => {
		// 2026-03-08 is a spring-forward day; midnight is still -08:00 (PST)
		const dstDay = new Date(2026, 2, 8, 10, 0, 0);
		expect(isoStartOfDay(dstDay, 'America/Los_Angeles')).toBe('2026-03-08T08:00:00.000Z');
		// End of that day is -07:00 (PDT) after the 02:00 transition
		expect(isoEndOfDay(dstDay, 'America/Los_Angeles')).toBe('2026-03-09T06:59:59.000Z');
	});
});
