import { locateQuery, unlocatedStaticPeriods } from './locating';
import type { MotionPeriod } from '#lib/types/captures.js';

function period(id: string, type: string, seconds: number): MotionPeriod {
	return {
		period_id: id,
		session_id: 'ses-1',
		ordinal: 0,
		type,
		label: id,
		start_ns: 0,
		end_ns: seconds * 1e9,
		duration_ns: seconds * 1e9,
		start_secs: 0,
		end_secs: seconds,
		created_at_ns: 0
	};
}

describe('unlocatedStaticPeriods', () => {
	const periods = [
		period('long', 'static', 600),
		period('short', 'static', 30),
		period('moving', 'motion', 900),
		period('located', 'static', 800),
		period('clipped', 'static', 120)
	];
	const clips = {
		located: { clip_id: 'c1', located: true, origin_lat: 1, origin_lon: 2 },
		clipped: { clip_id: 'c2', located: false }
	};

	it('keeps long static periods with no located clip', () => {
		expect(unlocatedStaticPeriods(periods, clips).map((p) => p.period_id)).toEqual([
			'long',
			'clipped'
		]);
	});

	it('takes a different minimum length', () => {
		expect(unlocatedStaticPeriods(periods, clips, 0).map((p) => p.period_id)).toContain('short');
	});
});

describe('locateQuery', () => {
	it('names the session and the period', () => {
		expect(locateQuery('ses 1', 'per-2')).toBe('session=ses+1&period=per-2');
	});
});
