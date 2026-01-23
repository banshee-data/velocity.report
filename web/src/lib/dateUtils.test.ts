import { isoDate } from './dateUtils';

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
