/**
 * Which static periods still need a place on the map.
 *
 * A static period is located when the clip cut from it has an accepted
 * position. Short static stretches are setup or a pause at a light, not a
 * survey, so a minimum length keeps them out of the operator's to-do list.
 */
import type { MotionPeriod, PeriodClip } from '#lib/types/captures.js';

/** Static periods shorter than this are not offered for locating. */
export const MIN_LOCATABLE_SECONDS = 60;

/** The static periods, at least minSeconds long, with no located clip. */
export function unlocatedStaticPeriods(
	periods: MotionPeriod[],
	clips: Record<string, PeriodClip>,
	minSeconds = MIN_LOCATABLE_SECONDS
): MotionPeriod[] {
	return periods.filter(
		(p) => p.type === 'static' && p.duration_ns / 1e9 >= minSeconds && !clips[p.period_id]?.located
	);
}

/** The query string that opens the Locate page on a period. */
export function locateQuery(sessionId: string, periodId: string): string {
	return new URLSearchParams({ session: sessionId, period: periodId }).toString();
}
