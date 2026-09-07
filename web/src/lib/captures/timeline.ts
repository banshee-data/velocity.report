/**
 * Turning capture-index records into the shapes the Captures page draws.
 *
 * All of it is pure: given sessions, files and periods, produce positions,
 * labels and verdicts. Keeping it out of the components is what lets the
 * arithmetic — which is where a timeline goes wrong — be tested directly.
 */

import type { CaptureFile, CaptureSession, MotionPeriod } from '$lib/types/captures';

/** Nanoseconds in a millisecond and a second, named so the maths reads. */
const NS_PER_MS = 1_000_000;
const NS_PER_SEC = 1_000_000_000;

/** Seam tolerances, matching internal/lidar/capseq. */
export const SEAMLESS_TOLERANCE_MS = 10;
export const MAX_GAP_MS = 1000;
export const MAX_OVERLAP_MS = 10;

export type SeamGrade = 'seamless' | 'acceptable' | 'broken' | 'overlap';

/**
 * gradeGap classifies the join between two captures from the signed gap in
 * milliseconds. A negative gap is an overlap.
 *
 * This deliberately mirrors capseq.GradeGap so the page can show an operator
 * what the server will decide before they commit to it. The server remains the
 * authority: a case it refuses is refused whatever this said.
 */
export function gradeGap(gapMs: number): SeamGrade {
	if (gapMs < -MAX_OVERLAP_MS) return 'overlap';
	if (gapMs <= SEAMLESS_TOLERANCE_MS) return 'seamless';
	if (gapMs <= MAX_GAP_MS) return 'acceptable';
	return 'broken';
}

/** Whether a join of this grade can be crossed during replay. */
export function isReplayable(grade: SeamGrade): boolean {
	return grade === 'seamless' || grade === 'acceptable';
}

/** nsToDate converts an epoch-nanosecond stamp to a Date. */
export function nsToDate(ns: number): Date {
	return new Date(ns / NS_PER_MS);
}

/** A band on a motion strip, positioned as percentages of the strip. */
export interface Band {
	type: string;
	label: string;
	/** Percentage from the left edge, 0-100. */
	left: number;
	/** Percentage of the total width. */
	width: number;
	durationSecs: number;
	startNs: number;
	endNs: number;
}

/**
 * toBands positions a session's motion periods across a strip spanning
 * [startNs, endNs].
 *
 * Periods outside the span are dropped and ones straddling its edges are
 * clipped, so a strip drawn for a sub-range of a session still reads correctly.
 * A zero-width span yields nothing rather than dividing by zero.
 */
export function toBands(periods: MotionPeriod[], startNs: number, endNs: number): Band[] {
	const span = endNs - startNs;
	if (span <= 0) return [];

	const bands: Band[] = [];
	for (const p of periods) {
		const from = Math.max(p.start_ns, startNs);
		const to = Math.min(p.end_ns, endNs);
		if (to <= from) continue;
		bands.push({
			type: p.type,
			label: p.label,
			left: ((from - startNs) / span) * 100,
			width: ((to - from) / span) * 100,
			durationSecs: (to - from) / NS_PER_SEC,
			startNs: from,
			endNs: to
		});
	}
	return bands;
}

/** "Nice" tick intervals, in milliseconds, from one second to a day. */
const NICE_TICK_INTERVALS_MS = [
	1000,
	5000,
	10_000,
	15_000,
	30_000,
	60_000,
	2 * 60_000,
	3 * 60_000,
	5 * 60_000,
	10 * 60_000,
	15 * 60_000,
	30 * 60_000,
	60 * 60_000,
	2 * 60 * 60_000,
	3 * 60 * 60_000,
	6 * 60 * 60_000,
	12 * 60 * 60_000,
	24 * 60 * 60_000
];

/** One labelled mark on a wall-clock axis. */
export interface ClockTick {
	/** Percentage from the left edge, 0-100. */
	at: number;
	label: string;
}

/**
 * clockTicks lays a wall-clock axis across [startNs, endNs], choosing the
 * coarsest "nice" interval (1s, 5s, 10s, ... up to a day) that still gives at
 * least minCount marks, so a 40-second static clip and an hours-long session
 * both read as real times of day rather than only the two endpoints.
 */
export function clockTicks(startNs: number, endNs: number, minCount = 4): ClockTick[] {
	const span = endNs - startNs;
	if (span <= 0) return [];
	const spanMs = span / NS_PER_MS;

	let intervalMs = NICE_TICK_INTERVALS_MS[0];
	for (const candidate of NICE_TICK_INTERVALS_MS) {
		intervalMs = candidate;
		if (spanMs / candidate <= minCount) break;
	}

	const startMs = startNs / NS_PER_MS;
	const endMs = endNs / NS_PER_MS;
	const first = Math.ceil(startMs / intervalMs) * intervalMs;
	const withSeconds = intervalMs < 60_000;

	const ticks: ClockTick[] = [];
	for (let t = first; t <= endMs; t += intervalMs) {
		const ns = t * NS_PER_MS;
		ticks.push({ at: ((ns - startNs) / span) * 100, label: formatClock(ns, withSeconds) });
	}
	return ticks;
}

/** A file boundary drawn as a tick on a session strip. */
export interface BoundaryTick {
	/** Percentage from the left edge. */
	at: number;
	relPath: string;
}

/**
 * fileBoundaries positions the start of each file after the first as a tick.
 *
 * The first file's start is the strip's own left edge and would draw a tick on
 * the border, which reads as a boundary that is not there.
 */
export function fileBoundaries(
	files: CaptureFile[],
	startNs: number,
	endNs: number
): BoundaryTick[] {
	const span = endNs - startNs;
	if (span <= 0) return [];

	const ordered = probedFiles(files);
	const ticks: BoundaryTick[] = [];
	for (let i = 1; i < ordered.length; i++) {
		const at = ordered[i].first_packet_ns as number;
		if (at <= startNs || at >= endNs) continue;
		ticks.push({ at: ((at - startNs) / span) * 100, relPath: ordered[i].rel_path });
	}
	return ticks;
}

/** probedFiles is the files with a usable extent, in packet-time order. */
export function probedFiles(files: CaptureFile[]): CaptureFile[] {
	return files
		.filter((f) => f.first_packet_ns != null && f.last_packet_ns != null)
		.sort((a, b) => (a.first_packet_ns as number) - (b.first_packet_ns as number));
}

/** One join between two adjacent selected captures. */
export interface Seam {
	beforePath: string;
	afterPath: string;
	gapMs: number;
	grade: SeamGrade;
}

/** The verdict on a proposed selection of captures. */
export interface SelectionVerdict {
	files: CaptureFile[];
	seams: Seam[];
	/** The weakest join; 'seamless' for a single capture, which has none. */
	worst: SeamGrade;
	/** Whether the server would accept this selection as one case. */
	usable: boolean;
	/** Total packet-time the selection covers, in seconds. */
	coveredSecs: number;
	/** Total time unaccounted for at the joins, in milliseconds. */
	lostMs: number;
	/** Files in the selection with no probed extent, which cannot be sequenced. */
	unprobed: CaptureFile[];
	/** Why the selection cannot be used, when it cannot. */
	reason?: string;
}

const GRADE_SEVERITY: Record<SeamGrade, number> = {
	seamless: 0,
	acceptable: 1,
	broken: 2,
	overlap: 3
};

/**
 * gradeSelection reports what would happen if these captures became one replay
 * case, so an operator sees the verdict before committing rather than as a
 * rejection afterwards.
 *
 * Files are ordered by packet time, not by the order they were clicked: the
 * sequence is defined by the capture clock and a selection made bottom-up is
 * as valid as one made top-down.
 */
export function gradeSelection(files: CaptureFile[]): SelectionVerdict {
	const unprobed = files.filter((f) => f.first_packet_ns == null || f.last_packet_ns == null);
	const ordered = probedFiles(files);

	const verdict: SelectionVerdict = {
		files: ordered,
		seams: [],
		worst: 'seamless',
		usable: false,
		coveredSecs: 0,
		lostMs: 0,
		unprobed
	};

	if (ordered.length === 0) {
		verdict.reason =
			unprobed.length > 0
				? 'These captures have not been probed yet, so their extents are unknown. Run a scan first.'
				: 'Select at least one capture.';
		return verdict;
	}
	if (unprobed.length > 0) {
		verdict.reason = `${unprobed.length} selected capture${unprobed.length === 1 ? ' has' : 's have'} not been probed, so ${unprobed.length === 1 ? 'it cannot' : 'they cannot'} be sequenced.`;
		return verdict;
	}

	for (const f of ordered) {
		verdict.coveredSecs +=
			((f.last_packet_ns as number) - (f.first_packet_ns as number)) / NS_PER_SEC;
	}

	for (let i = 1; i < ordered.length; i++) {
		const gapMs =
			((ordered[i].first_packet_ns as number) - (ordered[i - 1].last_packet_ns as number)) /
			NS_PER_MS;
		const grade = gradeGap(gapMs);
		verdict.seams.push({
			beforePath: ordered[i - 1].rel_path,
			afterPath: ordered[i].rel_path,
			gapMs,
			grade
		});
		if (gapMs > 0) verdict.lostMs += gapMs;
		if (GRADE_SEVERITY[grade] > GRADE_SEVERITY[verdict.worst]) verdict.worst = grade;
	}

	verdict.usable = isReplayable(verdict.worst);
	if (!verdict.usable) {
		const broken = verdict.seams.find((s) => !isReplayable(s.grade));
		if (broken) {
			verdict.reason = `${broken.beforePath} → ${broken.afterPath} is ${broken.grade} (${formatGap(broken.gapMs)}). A case must be one continuous recording.`;
		}
	}
	return verdict;
}

/** formatGap renders a signed millisecond gap the way an operator reads it. */
export function formatGap(gapMs: number): string {
	const sign = gapMs < 0 ? '−' : '';
	const abs = Math.abs(gapMs);
	if (abs < 1) return `${sign}${abs.toFixed(2)} ms`;
	if (abs < 1000) return `${sign}${Math.round(abs)} ms`;
	return `${sign}${(abs / 1000).toFixed(abs < 10000 ? 2 : 1)} s`;
}

/** formatDuration renders seconds as m:ss, or h:mm:ss past an hour. */
export function formatDuration(secs: number): string {
	if (!Number.isFinite(secs) || secs < 0) return '—';
	const total = Math.round(secs);
	const h = Math.floor(total / 3600);
	const m = Math.floor((total % 3600) / 60);
	const s = total % 60;
	const pad = (n: number) => String(n).padStart(2, '0');
	return h > 0 ? `${h}:${pad(m)}:${pad(s)}` : `${m}:${pad(s)}`;
}

/** formatSize renders bytes as MB or GB. */
export function formatSize(bytes: number): string {
	if (!Number.isFinite(bytes) || bytes < 0) return '—';
	const mb = bytes / (1024 * 1024);
	if (mb >= 1024) return `${(mb / 1024).toFixed(2)} GB`;
	return `${Math.round(mb)} MB`;
}

/** formatClock renders an epoch-nanosecond stamp as a local wall clock. */
export function formatClock(ns: number, withSeconds = false): string {
	if (!Number.isFinite(ns) || ns <= 0) return '—';
	const d = nsToDate(ns);
	const pad = (n: number) => String(n).padStart(2, '0');
	const base = `${pad(d.getHours())}:${pad(d.getMinutes())}`;
	return withSeconds ? `${base}:${pad(d.getSeconds())}` : base;
}

/** formatDay renders an epoch-nanosecond stamp as a short date. */
export function formatDay(ns: number): string {
	if (!Number.isFinite(ns) || ns <= 0) return '—';
	return nsToDate(ns).toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
}

/** A session laid out on a coverage timeline. */
export interface CoverageBar {
	session: CaptureSession;
	/** Percentage from the left edge of the timeline. */
	left: number;
	width: number;
}

/** A coverage timeline: the sessions of one day, positioned across it. */
export interface CoverageRow {
	/** Local date key, YYYY-MM-DD. */
	day: string;
	dayLabel: string;
	/** The span the row covers, so bars and axis labels agree. */
	startNs: number;
	endNs: number;
	bars: CoverageBar[];
}

/**
 * coverageRows groups sessions by the local day they start on and positions
 * each across that day's occupied span.
 *
 * The span is the day's own sessions rather than midnight to midnight: a
 * volume holding two forty-minute visits would otherwise draw two slivers on a
 * mostly empty bar and answer nothing. Padding keeps a single short session
 * from filling the row edge to edge and implying continuous coverage.
 */
export function coverageRows(sessions: CaptureSession[]): CoverageRow[] {
	const byDay = new Map<string, CaptureSession[]>();
	for (const s of sessions) {
		const d = nsToDate(s.start_ns);
		const key = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
		const list = byDay.get(key);
		if (list) list.push(s);
		else byDay.set(key, [s]);
	}

	const rows: CoverageRow[] = [];
	for (const [day, daySessions] of byDay) {
		const earliest = Math.min(...daySessions.map((s) => s.start_ns));
		const latest = Math.max(...daySessions.map((s) => s.end_ns));
		// A day holding one session would otherwise span exactly that session
		// and fill the row, reading as complete coverage of the day.
		const pad = Math.max((latest - earliest) * 0.05, 5 * 60 * NS_PER_SEC);
		const startNs = earliest - pad;
		const endNs = latest + pad;
		const span = endNs - startNs;

		rows.push({
			day,
			dayLabel: formatDay(earliest),
			startNs,
			endNs,
			bars: daySessions
				.slice()
				.sort((a, b) => a.start_ns - b.start_ns)
				.map((session) => ({
					session,
					left: ((session.start_ns - startNs) / span) * 100,
					width: Math.max(((session.end_ns - session.start_ns) / span) * 100, 0.4)
				}))
		});
	}

	return rows.sort((a, b) => (a.day < b.day ? 1 : -1));
}

/** sessionShare reports what fraction of a session is static, 0-1. */
export function sessionShare(periods: MotionPeriod[]): {
	staticSecs: number;
	motionSecs: number;
	staticShare: number;
} {
	let staticNs = 0;
	let motionNs = 0;
	for (const p of periods) {
		if (p.type === 'static') staticNs += p.duration_ns;
		else motionNs += p.duration_ns;
	}
	const total = staticNs + motionNs;
	return {
		staticSecs: staticNs / NS_PER_SEC,
		motionSecs: motionNs / NS_PER_SEC,
		staticShare: total > 0 ? staticNs / total : 0
	};
}

/** The start offset and duration a case should carry, in seconds. */
export interface PeriodTrim {
	startSecs: number;
	durationSecs: number;
}

/**
 * periodTrim is the pcap_start_secs/pcap_duration_secs a case should be given
 * when it is cut from a motion period, so the case excludes any leading
 * motion in the first selected capture rather than replaying it.
 *
 * selectPeriod (the caller) picks every capture the period overlaps, which for
 * the first static stretch after the sensor stops moving usually means the
 * first file starts with motion before the period itself begins. The offset
 * is measured from the selection's own first packet — not from local file
 * boundaries — because that is what the server's pcap_start_secs already
 * means for a multi-file case.
 */
export function periodTrim(
	period: { start_ns: number; end_ns: number },
	sequenceStartNs: number
): PeriodTrim {
	return {
		startSecs: Math.max(0, (period.start_ns - sequenceStartNs) / NS_PER_SEC),
		durationSecs: Math.max(0, (period.end_ns - period.start_ns) / NS_PER_SEC)
	};
}

/** The wall-clock window a start/duration trim resolves to. */
export interface TrimWindow {
	startNs: number;
	/** null when no duration is set — the trim runs to the end of the selection. */
	endNs: number | null;
}

/**
 * trimWindow resolves the case-builder's start/duration text fields against
 * the selection's own first packet, so an operator sees the real wall-clock
 * window a trim produces before creating the case rather than only the raw
 * seconds they typed.
 *
 * Returns null for text that does not parse as a number, which is a state the
 * inputs can hold transiently while being edited.
 */
export function trimWindow(
	sequenceStartNs: number,
	startSecsText: string,
	durationSecsText: string
): TrimWindow | null {
	const startSecs = startSecsText.trim() === '' ? 0 : Number(startSecsText);
	if (!Number.isFinite(startSecs)) return null;
	const startNs = sequenceStartNs + startSecs * NS_PER_SEC;

	if (durationSecsText.trim() === '') return { startNs, endNs: null };
	const durationSecs = Number(durationSecsText);
	if (!Number.isFinite(durationSecs)) return null;
	return { startNs, endNs: startNs + durationSecs * NS_PER_SEC };
}

/** jobProgressPercent is a job's completion, 0-100, or null when unknowable. */
export function jobProgressPercent(current: number, total: number): number | null {
	if (!Number.isFinite(total) || total <= 0) return null;
	return Math.min(100, Math.max(0, (current / total) * 100));
}
