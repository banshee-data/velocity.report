// src/lib/speedLimit.ts
// A site configuration period's posted speed limit is stored in km/h, with
// the unit the sign shows, so a 25 mph limit reads back as 25 mph. The server
// checks the same rules (internal/db/site_config_periods.go).

import type { Unit } from './units';

export type SpeedLimitUnit = 'kph' | 'mph';

/** Kilometres per mile. */
export const KPH_PER_MPH = 1.609344;

/** The largest limit the server accepts, in km/h. */
export const MAX_SPEED_LIMIT_KPH = 200;

/** The longest jurisdiction the server accepts, in characters. */
export const MAX_JURISDICTION_LENGTH = 100;

/** The sign unit a new period starts with, from the display units. */
export function defaultSpeedLimitUnit(display: Unit): SpeedLimitUnit {
	return display === 'mph' ? 'mph' : 'kph';
}

/** A posted limit in km/h, for storing. */
export function postedToKph(value: number, unit: SpeedLimitUnit): number {
	return unit === 'mph' ? value * KPH_PER_MPH : value;
}

/**
 * A stored limit in the unit it is signed in, to one decimal place, so the
 * 40.2336 km/h stored for a 25 mph sign reads back as 25.
 */
export function kphToPosted(kph: number, unit: SpeedLimitUnit): number {
	const value = unit === 'mph' ? kph / KPH_PER_MPH : kph;
	return Math.round(value * 10) / 10;
}

/** The period's limit as signed, such as "25 mph", or an em dash for none. */
export function formatSpeedLimit(period: {
	speed_limit_kph?: number | null;
	speed_limit_unit?: string | null;
}): string {
	const unit = period.speed_limit_unit;
	if (period.speed_limit_kph == null || (unit !== 'kph' && unit !== 'mph')) {
		return '—';
	}
	return `${kphToPosted(period.speed_limit_kph, unit)} ${unit === 'mph' ? 'mph' : 'km/h'}`;
}

/**
 * Why a posted limit cannot be saved, or null when it can. An empty value
 * means no limit, which is allowed.
 */
export function speedLimitError(value: string, unit: SpeedLimitUnit): string | null {
	if (value.trim() === '') {
		return null;
	}
	const posted = Number(value);
	if (!Number.isFinite(posted) || posted <= 0) {
		return 'Speed limit must be a positive number';
	}
	if (postedToKph(posted, unit) > MAX_SPEED_LIMIT_KPH) {
		return unit === 'mph'
			? `Speed limit must be at most ${kphToPosted(MAX_SPEED_LIMIT_KPH, 'mph')} mph`
			: `Speed limit must be at most ${MAX_SPEED_LIMIT_KPH} km/h`;
	}
	return null;
}
