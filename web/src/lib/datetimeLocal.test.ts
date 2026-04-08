import { fromDatetimeLocalToUnixSeconds, toDatetimeLocalValue } from './datetimeLocal';

describe('datetimeLocal helpers', () => {
	it('formats unix seconds to datetime-local string in local time', () => {
		const local = new Date(2026, 2, 29, 14, 5, 45);
		const unix = Math.floor(local.getTime() / 1000);
		expect(toDatetimeLocalValue(unix)).toBe('2026-03-29T14:05');
	});

	it('parses datetime-local string to unix seconds', () => {
		const value = '2026-03-29T14:05';
		const unix = fromDatetimeLocalToUnixSeconds(value);
		const expected = Math.floor(new Date(2026, 2, 29, 14, 5, 0).getTime() / 1000);
		expect(unix).toBe(expected);
	});

	it('returns null for empty or invalid input', () => {
		expect(fromDatetimeLocalToUnixSeconds('')).toBeNull();
		expect(fromDatetimeLocalToUnixSeconds('not-a-date')).toBeNull();
	});

	it('round-trips local minutes without UTC shift', () => {
		const original = '2026-03-29T00:01';
		const unix = fromDatetimeLocalToUnixSeconds(original);
		expect(unix).not.toBeNull();
		expect(toDatetimeLocalValue(unix as number)).toBe(original);
	});
});
