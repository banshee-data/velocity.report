/**
 * Date utility functions for the velocity.report frontend.
 *
 * IMPORTANT: These functions use LOCAL date parts, not UTC.
 * This prevents timezone conversion bugs where dates shift by +1 day
 * when the user's timezone is behind UTC.
 */

/**
 * Convert a Date to an ISO date string (YYYY-MM-DD) using local date parts.
 *
 * This function explicitly uses getFullYear(), getMonth(), and getDate()
 * instead of toISOString() to avoid UTC conversion that can shift dates.
 *
 * Example problem with toISOString():
 * - User selects June 4, 2025 at midnight in Pacific time (UTC-7)
 * - toISOString() converts to UTC: 2025-06-05T07:00:00Z
 * - slice(0, 10) returns "2025-06-05" ❌
 *
 * @param d - The Date object to format
 * @returns The date as YYYY-MM-DD string in local timezone
 */
export function isoDate(d: Date): string {
	const year = d.getFullYear();
	const month = String(d.getMonth() + 1).padStart(2, '0');
	const day = String(d.getDate()).padStart(2, '0');
	return `${year}-${month}-${day}`;
}

/**
 * Returns a Date representing "tomorrow" in browser-local time.
 *
 * Used as the default end-of-range on the dashboard and reports pages so that
 * data captured between local evening and midnight (which is stored against
 * the next UTC day) is always inside the queried window, regardless of the
 * user's selected display timezone.
 */
export function tomorrowLocal(now: Date = new Date()): Date {
	const d = new Date(now);
	d.setDate(now.getDate() + 1);
	return d;
}

/**
 * Milliseconds the wall-clock time shown in `tz` leads UTC at instant `utcMs`.
 *
 * e.g. for America/Los_Angeles in summer this returns -7h (the zone is behind
 * UTC). Computed via Intl so it respects the IANA database, including DST.
 */
function tzOffsetMs(utcMs: number, tz: string): number {
	const dtf = new Intl.DateTimeFormat('en-US', {
		timeZone: tz,
		hourCycle: 'h23',
		year: 'numeric',
		month: '2-digit',
		day: '2-digit',
		hour: '2-digit',
		minute: '2-digit',
		second: '2-digit'
	});
	const p: Record<string, number> = {};
	for (const part of dtf.formatToParts(new Date(utcMs))) {
		if (part.type !== 'literal') p[part.type] = Number(part.value);
	}
	return Date.UTC(p.year, p.month - 1, p.day, p.hour, p.minute, p.second) - utcMs;
}

/**
 * Returns the ISO 8601 instant for a wall-clock time (Y/M/D H:M:S) interpreted
 * in the IANA timezone `tz`.
 *
 * This is the inverse of {@link tzOffsetMs}: it finds the UTC instant whose
 * representation in `tz` is exactly the requested wall-clock time. A second
 * pass corrects for the offset changing across the first guess (DST edges).
 */
function zonedWallTimeToISO(
	year: number,
	month: number,
	day: number,
	hour: number,
	minute: number,
	second: number,
	tz: string
): string {
	const guess = Date.UTC(year, month - 1, day, hour, minute, second);
	const refined = guess - tzOffsetMs(guess - tzOffsetMs(guess, tz), tz);
	return new Date(refined).toISOString();
}

/**
 * ISO 8601 instant for the start of `d`'s calendar day (00:00:00), where the day
 * is the one shown in the picker (browser-local Y/M/D) but interpreted in `tz`.
 *
 * Sending an exact instant — rather than a bare date plus a separate timezone —
 * means the server performs no calendar-day interpretation, so the Vehicle
 * Count card and the chart always query the same window.
 */
export function isoStartOfDay(d: Date, tz: string): string {
	return zonedWallTimeToISO(d.getFullYear(), d.getMonth() + 1, d.getDate(), 0, 0, 0, tz);
}

/**
 * ISO 8601 instant for the inclusive end of `d`'s calendar day (23:59:59) in
 * `tz`. Pairs with {@link isoStartOfDay} to bound an inclusive day range.
 */
export function isoEndOfDay(d: Date, tz: string): string {
	return zonedWallTimeToISO(d.getFullYear(), d.getMonth() + 1, d.getDate(), 23, 59, 59, tz);
}
