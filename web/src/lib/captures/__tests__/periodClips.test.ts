import {
	clipDurationText,
	clipSpanNs,
	clipWindow,
	clipsOverlapping,
	composerSummary,
	describePlannedClip,
	isClipPeriod,
	planKey,
	planPeriodClips
} from '../periodClips';
import type { CaptureFile, MotionPeriod } from '#lib/types/captures.js';
import type { LidarClip } from '#lib/types/lidar.js';

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
	const files = [file('a', -50, 250), file('b', 250, 650), file('c', 650, 1000)];

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
		expect(window?.durationSecs).toBe(300);
	});

	it('never replays past the period when the captures start late', () => {
		// The review's case: static 320–600 s, motion 600–620 s; the first
		// probed capture starts at 400 s. The clip is 400–600 s, not 400–680 s.
		const [plan] = planPeriodClips(periods, ['p2']);
		const window = clipWindow(plan, [file('b', 400, 700)]);
		expect(window?.startSecs).toBe(0);
		expect(window?.durationSecs).toBe(200);
		expect(window?.usable).toBe(true);
	});

	it('refuses a window an unprobed capture overlaps, rather than dropping it', () => {
		const [plan] = planPeriodClips(periods, ['p2']);
		const window = clipWindow(plan, [
			file('a', 300, 400, { probe_state: 'pending' } as Partial<CaptureFile>),
			file('b', 400, 700)
		]);
		expect(window?.usable).toBe(false);
		expect(window?.reason).toMatch(/s\/a\.pcap.*not probed/);
	});

	it('refuses a window no probed capture covers', () => {
		const [plan] = planPeriodClips(periods, ['p2']);
		const window = clipWindow(plan, [
			file('a', 300, 700, { probe_state: 'error' } as Partial<CaptureFile>)
		]);
		expect(window?.usable).toBe(false);
		expect(window?.files).toEqual([]);
	});

	it('reports a broken seam inside the window', () => {
		const [plan] = planPeriodClips(periods, ['p2', 'p3']);
		const window = clipWindow(plan, [file('b', 250, 600), file('c', 700, 1000)]);
		expect(window?.usable).toBe(false);
		expect(window?.reason).toMatch(/broken/);
	});

	it('is null when no capture covers the window', () => {
		const [plan] = planPeriodClips(periods, ['p5']);
		expect(clipWindow(plan, [file('x', 0, 100)])).toBeNull();
	});
});

describe('planKey', () => {
	it('names a plan by every period it covers, so an edit does not follow a shrunk plan', () => {
		const [both] = planPeriodClips(periods, ['p2', 'p3']);
		const [one] = planPeriodClips(periods, ['p2']);
		expect(planKey(both)).toBe('p2+p3');
		expect(planKey(one)).toBe('p2');
	});
});

describe('clipsOverlapping', () => {
	const files = [file('a', -50, 250), file('b', 250, 650), file('c', 650, 1000)];
	const clip = (id: string, extra: Partial<LidarClip>): LidarClip => ({
		clip_id: id,
		sensor_id: 'hesai-pandar40p',
		created_at_ns: 0,
		file_count: 1,
		...extra
	});

	it('finds a clip whose window overlaps the plan, whatever period it came from', () => {
		const [plan] = planPeriodClips(periods, ['p2']); // 320–600 s
		const existing = [
			// b from 100 s for 50 s: 350–400 s, inside p2, recorded against another period.
			clip('inside', {
				source_period_id: 'renumbered',
				pcap_start_secs: 100,
				pcap_duration_secs: 50,
				files: [{ ordinal: 0, capture_file_id: 'b', pcap_file: 's/b.pcap' }]
			}),
			// a from 0 for 200 s: −50–150 s, before p2.
			clip('before', {
				pcap_start_secs: 0,
				pcap_duration_secs: 200,
				files: [{ ordinal: 0, pcap_file: 's/a.pcap' }]
			}),
			// c to its end: 650–1000 s, after p2.
			clip('after', { files: [{ ordinal: 0, pcap_file: 's/c.pcap' }] }),
			// another session's capture.
			clip('elsewhere', { files: [{ ordinal: 0, pcap_file: 's/zz.pcap' }] })
		];
		expect(clipsOverlapping(plan, existing, files).map((c) => c.clip_id)).toEqual(['inside']);
	});

	it('reads a clip with no duration to its last capture', () => {
		expect(
			clipSpanNs(
				clip('x', {
					pcap_start_secs: 10,
					files: [
						{ ordinal: 1, pcap_file: 's/c.pcap' },
						{ ordinal: 0, pcap_file: 's/b.pcap' }
					]
				}),
				files
			)
		).toEqual({ startNs: T0 + 260 * S, endNs: T0 + 1000 * S });
	});
});

describe('descriptions', () => {
	it('says a duration as a description reads it', () => {
		expect(clipDurationText(45)).toBe('45 s');
		expect(clipDurationText(580)).toBe('10 min');
		expect(clipDurationText(3600)).toBe('1 h');
		expect(clipDurationText(3900)).toBe('1 h 5 min');
	});

	it('prefills from the site, times and duration, naming no period', () => {
		const [plan] = planPeriodClips(periods, ['p2', 'p3']);
		const clock = (ns: number) => `t+${(ns - T0) / S}`;
		expect(describePlannedClip('Kirkham at 9th', plan, clock)).toBe(
			'Kirkham at 9th, t+320–t+900, 10 min static'
		);
		expect(describePlannedClip(' ', plan, clock)).toBe('Unnamed site, t+320–t+900, 10 min static');
		expect(describePlannedClip('kirk', plan)).not.toMatch(/static-\d/);
	});

	it('says how many clips from how many periods', () => {
		expect(composerSummary(planPeriodClips(periods, ['p0', 'p2', 'p3']))).toBe(
			'Make 2 clips from 3 periods'
		);
		expect(composerSummary(planPeriodClips(periods, ['p5']))).toBe('Make 1 clip from 1 period');
		expect(composerSummary([])).toBe('Select static periods to make clips from them.');
	});
});
