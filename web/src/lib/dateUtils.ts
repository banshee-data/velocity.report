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
