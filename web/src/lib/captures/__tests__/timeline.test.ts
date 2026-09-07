import type { CaptureFile, CaptureSession, MotionPeriod } from '$lib/types/captures';
import {
	coverageRows,
	fileBoundaries,
	formatClock,
	formatDuration,
	formatGap,
	formatSize,
	gradeGap,
	gradeSelection,
	isReplayable,
	jobProgressPercent,
	nsToDate,
	probedFiles,
	sessionShare,
	toBands
} from '../timeline';

const SEC = 1_000_000_000;
const MS = 1_000_000;
/** 2026-09-02 13:20:37 local, the start of the broadway_columbus session. */
const BASE = new Date(2026, 8, 2, 13, 20, 37).getTime() * MS;

function file(relPath: string, offsetSec: number, durSec: number, probed = true): CaptureFile {
	return {
		capture_file_id: `cap-${relPath}`,
		root_id: 'root-1',
		rel_path: relPath,
		size_bytes: 724 * 1024 * 1024,
		modified_at_ns: BASE,
		first_packet_ns: probed ? BASE + offsetSec * SEC : undefined,
		last_packet_ns: probed ? BASE + (offsetSec + durSec) * SEC : undefined,
		packet_count: probed ? 540000 : undefined,
		probe_state: probed ? 'ok' : 'pending',
		present: true,
		first_seen_at_ns: BASE,
		last_seen_at_ns: BASE
	};
}

function period(type: string, label: string, startSec: number, endSec: number): MotionPeriod {
	return {
		period_id: `per-${label}`,
		session_id: 'ses-1',
		ordinal: 0,
		type,
		label,
		start_ns: BASE + startSec * SEC,
		end_ns: BASE + endSec * SEC,
		duration_ns: (endSec - startSec) * SEC,
		start_secs: startSec,
		end_secs: endSec,
		created_at_ns: BASE
	};
}

function session(id: string, startSec: number, endSec: number): CaptureSession {
	return {
		session_id: id,
		root_id: 'root-1',
		file_count: 8,
		start_ns: BASE + startSec * SEC,
		end_ns: BASE + endSec * SEC,
		covered_ns: (endSec - startSec) * SEC,
		lost_ns: 0,
		worst_seam: 'seamless',
		size_bytes: 5_800_000_000,
		derived_at_ns: BASE
	};
}

describe('gradeGap', () => {
	it.each([
		['exactly abutting', 0, 'seamless'],
		['at the seamless bound', 10, 'seamless'],
		['just past seamless', 10.001, 'acceptable'],
		['a real but tolerable loss', 400, 'acceptable'],
		['at the maximum gap', 1000, 'acceptable'],
		['past the maximum gap', 1000.001, 'broken'],
		['a whole minute missing', 60000, 'broken'],
		['a repeated packet at a roll-over', -5, 'seamless'],
		['an overlap past tolerance', -50, 'overlap']
	])('grades %s', (_name, gapMs, want) => {
		expect(gradeGap(gapMs as number)).toBe(want);
	});

	it('matches the server tolerances of 10ms and 1s', () => {
		// These are the field requirement, and drifting from capseq would show
		// an operator a verdict the server then contradicts.
		expect(gradeGap(10)).toBe('seamless');
		expect(gradeGap(11)).toBe('acceptable');
		expect(gradeGap(1000)).toBe('acceptable');
		expect(gradeGap(1001)).toBe('broken');
	});
});

describe('isReplayable', () => {
	it('accepts seamless and acceptable only', () => {
		expect(isReplayable('seamless')).toBe(true);
		expect(isReplayable('acceptable')).toBe(true);
		expect(isReplayable('broken')).toBe(false);
		expect(isReplayable('overlap')).toBe(false);
	});
});

describe('toBands', () => {
	it('positions periods across the span', () => {
		const bands = toBands(
			[period('static', 'static-0', 0, 300), period('motion', 'motion-0', 300, 600)],
			BASE,
			BASE + 600 * SEC
		);
		expect(bands).toHaveLength(2);
		expect(bands[0].left).toBeCloseTo(0);
		expect(bands[0].width).toBeCloseTo(50);
		expect(bands[1].left).toBeCloseTo(50);
		expect(bands[1].width).toBeCloseTo(50);
		expect(bands[0].durationSecs).toBeCloseTo(300);
	});

	it('clips a period straddling the span edges', () => {
		// A strip drawn for a sub-range must still read correctly.
		const bands = toBands(
			[period('static', 'static-0', 0, 600)],
			BASE + 150 * SEC,
			BASE + 450 * SEC
		);
		expect(bands).toHaveLength(1);
		expect(bands[0].left).toBeCloseTo(0);
		expect(bands[0].width).toBeCloseTo(100);
		expect(bands[0].durationSecs).toBeCloseTo(300);
	});

	it('drops periods entirely outside the span', () => {
		const bands = toBands(
			[period('static', 'static-0', 0, 100)],
			BASE + 200 * SEC,
			BASE + 400 * SEC
		);
		expect(bands).toHaveLength(0);
	});

	it('returns nothing for a zero-width span rather than dividing by zero', () => {
		expect(toBands([period('static', 'static-0', 0, 100)], BASE, BASE)).toEqual([]);
		expect(toBands([period('static', 'static-0', 0, 100)], BASE + SEC, BASE)).toEqual([]);
	});

	it('handles no periods', () => {
		expect(toBands([], BASE, BASE + 600 * SEC)).toEqual([]);
	});
});

describe('fileBoundaries', () => {
	it('ticks every file after the first', () => {
		const files = [file('a.pcap', 0, 300), file('b.pcap', 300, 300), file('c.pcap', 600, 300)];
		const ticks = fileBoundaries(files, BASE, BASE + 900 * SEC);
		expect(ticks).toHaveLength(2);
		expect(ticks[0].relPath).toBe('b.pcap');
		expect(ticks[0].at).toBeCloseTo(33.33, 1);
		expect(ticks[1].at).toBeCloseTo(66.67, 1);
	});

	it('does not tick the first file, which is the strip edge', () => {
		const ticks = fileBoundaries([file('a.pcap', 0, 300)], BASE, BASE + 300 * SEC);
		expect(ticks).toHaveLength(0);
	});

	it('skips unprobed files, which have no position', () => {
		const files = [file('a.pcap', 0, 300), file('b.pcap', 0, 0, false), file('c.pcap', 300, 300)];
		const ticks = fileBoundaries(files, BASE, BASE + 600 * SEC);
		expect(ticks).toHaveLength(1);
		expect(ticks[0].relPath).toBe('c.pcap');
	});
});

describe('probedFiles', () => {
	it('orders by packet time regardless of input order', () => {
		const ordered = probedFiles([
			file('c.pcap', 600, 300),
			file('a.pcap', 0, 300),
			file('b.pcap', 300, 300)
		]);
		expect(ordered.map((f) => f.rel_path)).toEqual(['a.pcap', 'b.pcap', 'c.pcap']);
	});

	it('excludes files with no extent', () => {
		const ordered = probedFiles([file('a.pcap', 0, 300), file('pending.pcap', 0, 0, false)]);
		expect(ordered.map((f) => f.rel_path)).toEqual(['a.pcap']);
	});
});

describe('gradeSelection', () => {
	it('accepts an abutting pair, the shape a real case takes', () => {
		// broadway_columbus: the last static stretch spans files 7 and 8.
		const verdict = gradeSelection([file('file7.pcap', 0, 300), file('file8.pcap', 300.148, 300)]);
		expect(verdict.usable).toBe(true);
		expect(verdict.worst).toBe('acceptable');
		expect(verdict.seams).toHaveLength(1);
		expect(verdict.seams[0].grade).toBe('acceptable');
		expect(verdict.seams[0].gapMs).toBeCloseTo(148, 0);
		expect(verdict.lostMs).toBeCloseTo(148, 0);
		expect(verdict.coveredSecs).toBeCloseTo(600);
	});

	it('orders by packet time, not by the order captures were clicked', () => {
		const verdict = gradeSelection([file('file8.pcap', 300, 300), file('file7.pcap', 0, 300)]);
		expect(verdict.files.map((f) => f.rel_path)).toEqual(['file7.pcap', 'file8.pcap']);
		expect(verdict.usable).toBe(true);
	});

	it('refuses a selection that skips a capture, and says which join broke', () => {
		const verdict = gradeSelection([file('file7.pcap', 0, 300), file('file9.pcap', 900, 300)]);
		expect(verdict.usable).toBe(false);
		expect(verdict.worst).toBe('broken');
		expect(verdict.reason).toContain('file7.pcap');
		expect(verdict.reason).toContain('file9.pcap');
		expect(verdict.reason).toContain('broken');
	});

	it('refuses overlapping captures', () => {
		const verdict = gradeSelection([file('a.pcap', 0, 300), file('b.pcap', 240, 300)]);
		expect(verdict.worst).toBe('overlap');
		expect(verdict.usable).toBe(false);
	});

	it('accepts a single capture, which has no joins', () => {
		const verdict = gradeSelection([file('a.pcap', 0, 300)]);
		expect(verdict.usable).toBe(true);
		expect(verdict.seams).toHaveLength(0);
		expect(verdict.worst).toBe('seamless');
	});

	it('reports an empty selection without claiming it is usable', () => {
		const verdict = gradeSelection([]);
		expect(verdict.usable).toBe(false);
		expect(verdict.reason).toContain('at least one');
	});

	it('refuses unprobed captures rather than guessing their extents', () => {
		const verdict = gradeSelection([file('a.pcap', 0, 300), file('pending.pcap', 0, 0, false)]);
		expect(verdict.usable).toBe(false);
		expect(verdict.unprobed).toHaveLength(1);
		expect(verdict.reason).toContain('probed');
	});

	it('reports a selection of only unprobed captures', () => {
		const verdict = gradeSelection([file('pending.pcap', 0, 0, false)]);
		expect(verdict.usable).toBe(false);
		expect(verdict.reason).toContain('probed');
	});

	it('takes the weakest join across a long run', () => {
		const verdict = gradeSelection([
			file('a.pcap', 0, 300),
			file('b.pcap', 300, 300),
			file('c.pcap', 700, 300),
			file('d.pcap', 1000, 300)
		]);
		expect(verdict.worst).toBe('broken');
		expect(verdict.usable).toBe(false);
	});
});

describe('formatGap', () => {
	it('renders sub-millisecond, millisecond and second gaps', () => {
		expect(formatGap(0.25)).toBe('0.25 ms');
		expect(formatGap(148)).toBe('148 ms');
		expect(formatGap(1500)).toBe('1.50 s');
		expect(formatGap(30000)).toBe('30.0 s');
	});

	it('marks an overlap with a minus sign', () => {
		expect(formatGap(-50)).toBe('−50 ms');
	});
});

describe('formatDuration', () => {
	it('renders minutes and seconds', () => {
		expect(formatDuration(0)).toBe('0:00');
		expect(formatDuration(89)).toBe('1:29');
		expect(formatDuration(449.4)).toBe('7:29');
	});

	it('renders hours past sixty minutes', () => {
		expect(formatDuration(3661)).toBe('1:01:01');
	});

	it('reports nothing for a nonsensical duration', () => {
		expect(formatDuration(-1)).toBe('—');
		expect(formatDuration(NaN)).toBe('—');
	});
});

describe('formatSize', () => {
	it('renders megabytes and gigabytes', () => {
		expect(formatSize(724 * 1024 * 1024)).toBe('724 MB');
		expect(formatSize(5.8 * 1024 * 1024 * 1024)).toBe('5.80 GB');
	});

	it('reports nothing for a nonsensical size', () => {
		expect(formatSize(-1)).toBe('—');
	});
});

describe('formatClock', () => {
	it('renders a wall clock, optionally with seconds', () => {
		expect(formatClock(BASE)).toBe('13:20');
		expect(formatClock(BASE, true)).toBe('13:20:37');
	});

	it('reports nothing for an absent stamp', () => {
		expect(formatClock(0)).toBe('—');
	});
});

describe('nsToDate', () => {
	it('converts epoch nanoseconds', () => {
		expect(nsToDate(BASE).getHours()).toBe(13);
		expect(nsToDate(BASE).getMinutes()).toBe(20);
	});
});

describe('coverageRows', () => {
	it('groups sessions by day, newest day first', () => {
		const oneDay = 24 * 3600;
		const rows = coverageRows([
			session('ses-1', 0, 600),
			session('ses-2', 3600, 4200),
			session('ses-3', oneDay, oneDay + 600)
		]);
		expect(rows).toHaveLength(2);
		expect(rows[0].bars).toHaveLength(1);
		expect(rows[1].bars).toHaveLength(2);
	});

	it('spans the day the sessions occupy, not midnight to midnight', () => {
		// Two forty-minute visits on an empty day would otherwise draw two
		// slivers on a mostly empty bar and answer nothing.
		const rows = coverageRows([session('ses-1', 0, 2400), session('ses-2', 7200, 9600)]);
		const row = rows[0];
		expect(row.bars[0].width).toBeGreaterThan(15);
		expect(row.bars[1].left).toBeGreaterThan(row.bars[0].left);
	});

	it('pads a single session so it does not fill the row', () => {
		// A full-width bar would read as continuous coverage of the whole day.
		const rows = coverageRows([session('ses-1', 0, 600)]);
		expect(rows[0].bars[0].width).toBeLessThan(100);
		expect(rows[0].bars[0].left).toBeGreaterThan(0);
	});

	it('orders bars within a day chronologically', () => {
		const rows = coverageRows([session('late', 7200, 7800), session('early', 0, 600)]);
		expect(rows[0].bars.map((b) => b.session.session_id)).toEqual(['early', 'late']);
	});

	it('handles no sessions', () => {
		expect(coverageRows([])).toEqual([]);
	});
});

describe('sessionShare', () => {
	it('splits static and motion time', () => {
		const share = sessionShare([
			period('static', 'static-0', 0, 300),
			period('motion', 'motion-0', 300, 400)
		]);
		expect(share.staticSecs).toBeCloseTo(300);
		expect(share.motionSecs).toBeCloseTo(100);
		expect(share.staticShare).toBeCloseTo(0.75);
	});

	it('reports a zero share for a session with no timeline yet', () => {
		expect(sessionShare([]).staticShare).toBe(0);
	});
});

describe('jobProgressPercent', () => {
	it('reports a percentage', () => {
		expect(jobProgressPercent(2, 8)).toBe(25);
		expect(jobProgressPercent(8, 8)).toBe(100);
	});

	it('reports null when the total is unknown, rather than a false zero', () => {
		expect(jobProgressPercent(3, 0)).toBeNull();
	});

	it('clamps a progress report past its own total', () => {
		expect(jobProgressPercent(12, 8)).toBe(100);
	});
});
