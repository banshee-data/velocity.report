import {
	clipDurationText,
	clipWindow,
	composerSummary,
	describePlannedClip,
	isClipPeriod,
	planPeriodClips
} from '../periodClips';
import type { CaptureFile, MotionPeriod } from '#lib/types/captures.js';

const S = 1_000_000_000;
const T0 = 1_790_000_000 * S;

function period(
	ordinal: number,
	type: 'static' | 'motion',
	startS: number,
	endS: number
): MotionPeriod {
	return {
		period_id: `p${ordinal}`,
		session_id: 'sess',
		ordinal,
		type,
		label: `${type}-${ordinal}`,
		start_ns: T0 + startS * S,
		end_ns: T0 + endS * S,
		duration_ns: (endS - startS) * S,
		start_secs: startS,
		end_secs: endS,
		created_at_ns: 0
	};
}

function file(
	id: string,
	startS: number,
	endS: number,
	extra: Partial<CaptureFile> = {}
): CaptureFile {
	return {
		capture_file_id: id,
		rel_path: `s/${id}.pcap`,
		probe_state: 'ok',
		first_packet_ns: T0 + startS * S,
		last_packet_ns: T0 + endS * S,
		...extra
	} as CaptureFile;
}

// static-0, motion-1, static-2, static-3 (adjacent), motion-4, static-5
const periods = [
	period(0, 'static', 0, 300),
	period(1, 'motion', 300, 320),
	period(2, 'static', 320, 600),
	period(3, 'static', 600, 900),
	period(4, 'motion', 900, 930),
	period(5, 'static', 930, 1200)
];

describe('planPeriodClips', () => {
	it('makes one clip per static period separated by motion', () => {
		const plans = planPeriodClips(periods, ['p0', 'p2', 'p5']);
		expect(plans.map((p) => p.periods.map((x) => x.period_id))).toEqual([['p0'], ['p2'], ['p5']]);
	});

	it('joins adjacent selected static periods into one clip', () => {
		const plans = planPeriodClips(periods, ['p3', 'p2']);
		expect(plans).toHaveLength(1);
		expect(plans[0].periods.map((p) => p.period_id)).toEqual(['p2', 'p3']);
		expect(plans[0].startNs).toBe(T0 + 320 * S);
		expect(plans[0].endNs).toBe(T0 + 900 * S);
		expect(plans[0].durationSecs).toBe(580);
	});

	it('never puts a motion period inside a clip, even when it is selected', () => {
		const plans = planPeriodClips(periods, ['p0', 'p1', 'p2']);
		expect(plans.map((p) => p.periods.map((x) => x.period_id))).toEqual([['p0'], ['p2']]);
		for (const plan of plans) expect(plan.periods.every(isClipPeriod)).toBe(true);
	});

	it('splits a run at an unselected static period', () => {
		const three = [
			period(0, 'static', 0, 10),
			period(1, 'static', 10, 20),
			period(2, 'static', 20, 30)
		];
		expect(planPeriodClips(three, ['p0', 'p2']).map((p) => p.periods.length)).toEqual([1, 1]);
	});

	it('orders by ordinal whatever order the periods arrive in', () => {
		const plans = planPeriodClips([...periods].reverse(), ['p5', 'p0']);
		expect(plans.map((p) => p.periods[0].period_id)).toEqual(['p0', 'p5']);
	});

	it('makes nothing from nothing', () => {
		expect(planPeriodClips(periods, [])).toEqual([]);
		expect(planPeriodClips([], ['p0'])).toEqual([]);
	});
});

describe('clipWindow', () => {
	const files = [
		file('a', -50, 250),
		file('b', 250, 650),
		file('c', 650, 1000),
		file('bad', 0, 1200, { probe_state: 'error' } as Partial<CaptureFile>)
	];

	it('picks the captures the window overlaps and trims from the first one', () => {
		const [plan] = planPeriodClips(periods, ['p2', 'p3']);
		const window = clipWindow(plan, files);
		expect(window?.files.map((f) => f.capture_file_id)).toEqual(['b', 'c']);
		expect(window?.startSecs).toBe(70);
		expect(window?.durationSecs).toBe(580);
		expect(window?.usable).toBe(true);
	});

	it('skips the first capture’s seconds before the period', () => {
		const [plan] = planPeriodClips(periods, ['p0']);
		const window = clipWindow(plan, files);
		expect(window?.files.map((f) => f.capture_file_id)).toEqual(['a', 'b']);
		expect(window?.startSecs).toBe(50);
	});

	it('reports a broken seam inside the window', () => {
		const [plan] = planPeriodClips(periods, ['p2', 'p3']);
		const window = clipWindow(plan, [file('b', 250, 600), file('c', 700, 1000)]);
		expect(window?.usable).toBe(false);
		expect(window?.reason).toMatch(/broken/);
	});

	it('is null when no probed capture covers the window', () => {
		const [plan] = planPeriodClips(periods, ['p5']);
		expect(clipWindow(plan, [file('x', 0, 100)])).toBeNull();
	});
});

describe('descriptions', () => {
	it('says a duration as a description reads it', () => {
		expect(clipDurationText(45)).toBe('45 s');
		expect(clipDurationText(580)).toBe('10 min');
		expect(clipDurationText(3600)).toBe('1 h');
		expect(clipDurationText(3900)).toBe('1 h 5 min');
	});

	it('prefills from the site, duration and periods', () => {
		const [plan] = planPeriodClips(periods, ['p2', 'p3']);
		expect(describePlannedClip('Kirkham at 9th', plan)).toBe(
			'Kirkham at 9th, 10 min static (static-2, static-3)'
		);
		expect(describePlannedClip(' ', plan)).toBe('Unnamed site, 10 min static (static-2, static-3)');
	});

	it('says how many clips from how many periods', () => {
		expect(composerSummary(planPeriodClips(periods, ['p0', 'p2', 'p3']))).toBe(
			'Make 2 clips from 3 periods'
		);
		expect(composerSummary(planPeriodClips(periods, ['p5']))).toBe('Make 1 clip from 1 period');
		expect(composerSummary([])).toBe('Select static periods to make clips from them.');
	});
});
