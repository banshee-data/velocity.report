/**
 * Shared report-settings localStorage helpers.
 *
 * Both the dashboard (+page.svelte) and reports page share the same
 * localStorage key. This module centralises the key name, the stored
 * shape, and the date-range staleness check so the two pages stay in
 * sync.
 */

/** Canonical localStorage key used by both dashboard and reports pages. */
export const REPORT_SETTINGS_KEY = 'reportSettings';

/** Numeric enum value used by DateRangeField for day ranges. */
export const DAY_PERIOD_TYPE = 10;

/**
 * Maximum age of a stored date range before it is considered stale.
 *
 * 18 hours is long enough to survive a same-day session break (lunch,
 * meeting) but short enough that returning the next morning resets the
 * picker to "last 14 days".
 */
export const STALENESS_THRESHOLD_MS = 18 * 60 * 60 * 1000;

/** Shape of the date-range portion persisted in localStorage. */
export type StoredDateRange = {
	from?: string;
	to?: string;
	periodType?: number | string;
	savedAt?: string;
};

/** Top-level shape persisted under REPORT_SETTINGS_KEY. */
export type StoredReportSettings = {
	dateRange?: StoredDateRange;
	compareRange?: StoredDateRange;
	compareEnabled?: boolean;
	compareSource?: string;
	group?: string;
	selectedSource?: string;
	minSpeed?: number;
	maxSpeedCutoff?: number | null;
	boundaryThreshold?: number;
};

/**
 * Returns true when the stored date range should be discarded in favour
 * of the computed default (last 14 days).
 *
 * A range is stale when:
 * - `savedAt` is missing (legacy data or first visit)
 * - `savedAt` cannot be parsed as a valid date
 * - `savedAt` is older than {@link STALENESS_THRESHOLD_MS}
 */
export function isDateRangeStale(savedAt: string | undefined): boolean {
	if (!savedAt) return true;
	const savedTime = new Date(savedAt).getTime();
	if (Number.isNaN(savedTime)) return true;
	return Date.now() - savedTime > STALENESS_THRESHOLD_MS;
}

/**
 * Normalise persisted period-type values before handing them back to the
 * date picker. Older localStorage entries may contain stringified enum values
 * like "10" or symbolic codes like "day".
 */
export function normaliseStoredPeriodType(
	periodType: unknown,
	allowed: readonly number[] = [DAY_PERIOD_TYPE],
	fallback = DAY_PERIOD_TYPE
): number {
	if (
		typeof periodType === 'number' &&
		Number.isInteger(periodType) &&
		allowed.includes(periodType)
	) {
		return periodType;
	}

	if (typeof periodType === 'string') {
		const trimmed = periodType.trim();
		const numericValue = Number(trimmed);
		if (Number.isInteger(numericValue) && allowed.includes(numericValue)) {
			return numericValue;
		}

		if (trimmed.toLowerCase() === 'day' && allowed.includes(DAY_PERIOD_TYPE)) {
			return DAY_PERIOD_TYPE;
		}
	}

	return fallback;
}
