/**
 * Turning capture-index records into the shapes the Captures page draws.
 *
 * All of it is pure: given sessions, files and periods, produce positions,
 * labels and verdicts. Keeping it out of the components is what lets the
 * arithmetic — which is where a timeline goes wrong — be tested directly.
 */

import {
	PROBE_FAILED,
	PROBE_OK,
	PROBE_PENDING,
	type CaptureFile,
	type CaptureSession,
	type MotionPeriod
} from '#lib/types/captures.js';

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

/**
 * zoneLabel names the timezone formatClock and formatDay show times in: the
 * browser's own. The times themselves are the capturing host's packet clock
 * (pcap record headers), never the sensor's, whose clock was never set.
 */
export function zoneLabel(date: Date = new Date()): string {
	const zone = Intl.DateTimeFormat().resolvedOptions().timeZone;
	const short = new Intl.DateTimeFormat(undefined, { timeZoneName: 'short' })
		.formatToParts(date)
		.find((part) => part.type === 'timeZoneName')?.value;
	if (zone && short && short !== zone) return `${zone} (${short})`;
	return zone || short || 'local time';
}

/** How many of a volume's captures each probe state holds. */
export interface ProbeCounts {
	total: number;
	probed: number;
	pending: number;
	failed: number;
}

/**
 * probeCounts tallies the captures present on a volume by probe state. Only a
 * probed capture has a packet extent, and only those make sessions, so the
 * pending ones are absent from Coverage until a probe reaches them.
 */
export function probeCounts(files: CaptureFile[]): ProbeCounts {
	const counts: ProbeCounts = { total: 0, probed: 0, pending: 0, failed: 0 };
	for (const f of files) {
		if (!f.present) continue;
		counts.total++;
		if (f.probe_state === PROBE_OK) counts.probed++;
		else if (f.probe_state === PROBE_PENDING) counts.pending++;
		else if (f.probe_state === PROBE_FAILED) counts.failed++;
	}
	return counts;
}

/**
 * formatDay renders an epoch-nanosecond stamp as a local yyyy-mm-dd date: the
 * one date format the Captures page uses, so a day reads the same in the
 * coverage rows, the sessions table and the scan stamp, and sorts as text.
 */
export function formatDay(ns: number): string {
	if (!Number.isFinite(ns) || ns <= 0) return '—';
	const d = nsToDate(ns);
	const pad = (n: number) => String(n).padStart(2, '0');
	return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

/** formatStamp renders an epoch-nanosecond stamp as a local date and time. */
export function formatStamp(ns: number, withSeconds = false): string {
	if (!Number.isFinite(ns) || ns <= 0) return '—';
	return `${formatDay(ns)} ${formatClock(ns, withSeconds)}`;
}

/**
 * formatWindow renders a span as its date and wall-clock bounds,
 * "2026-09-02 13:17–13:56", naming the second date only when the span
 * crosses midnight.
 */
export function formatWindow(startNs: number, endNs: number): string {
	if (formatDay(startNs) === formatDay(endNs)) {
		return `${formatStamp(startNs)}–${formatClock(endNs)}`;
	}
	return `${formatStamp(startNs)} – ${formatStamp(endNs)}`;
}

/** A session laid out on a coverage timeline. */
export interface CoverageBar {
	session: CaptureSession;
	/** Percentage from the left edge of the timeline. */
	left: number;
	width: number;
}

/**
 * One lane of a day: sessions from one folder that do not overlap each other.
 * A folder whose sessions overlap, such as a capture and a trimmed copy of it
 * beside it, gets as many lanes as it needs to draw them all.
 */
export interface CoverageLane {
	key: string;
	/** The folder the lane's captures sit in, within the volume; '' at its top level. */
	folder: string;
	/** True on a folder's first lane, which carries its label. */
	firstOfFolder: boolean;
	bars: CoverageBar[];
}

/** A coverage timeline: the sessions of one day, positioned across it. */
export interface CoverageRow {
	/** Local date key, YYYY-MM-DD. */
	day: string;
	dayLabel: string;
	/** The span the row covers, so bars and axis labels agree. */
	startNs: number;
	endNs: number;
	/**
	 * The same span as nanoseconds after local midnight. Every row covers the
	 * same clock times, so these are equal on all rows.
	 */
	clockStartNs: number;
	clockEndNs: number;
	/** One per folder, or more where a folder's sessions overlap; on one shared axis. */
	lanes: CoverageLane[];
}

/** captureFolder is the folder a capture sits in within its volume; '' at its top level. */
export function captureFolder(relPath: string): string {
	const slash = relPath.lastIndexOf('/');
	return slash < 0 ? '' : relPath.slice(0, slash);
}

/**
 * coverageRows groups sessions by the local day they start on and positions
 * each on a clock-time axis that every day shares.
 *
 * The axis runs from the earliest time of day any session starts to the
 * latest any ends, padded and rounded out to the hour, rather than midnight to
 * midnight: field visits fill a working day, and a 24-hour axis would draw
 * them as slivers. Because every row shares it, a bar's length is the same
 * duration on every row and the same clock time lines up down the page.
 *
 * A day has a lane per folder, because a volume can hold more than one copy of
 * the same hours (the original rolls and an export of them) and drawn in one
 * lane they would sit on top of each other. Sessions in one folder that still
 * overlap take further lanes.
 */
export function coverageRows(
	sessions: CaptureSession[],
	folderOf: (session: CaptureSession) => string = () => ''
): CoverageRow[] {
	const byDay = new Map<string, CaptureSession[]>();
	for (const s of sessions) {
		const key = formatDay(s.start_ns);
		const list = byDay.get(key);
		if (list) list.push(s);
		else byDay.set(key, [s]);
	}

	// One clock-time axis for every day, so a bar's length means the same
	// duration on every row and the same time of day lines up down the page.
	// A per-day axis stretched a lone twenty-minute session across most of its
	// row while a forty-minute one on a busier day took a third of its.
	const midnightOf = (ns: number) => {
		const d = nsToDate(ns);
		return new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime() * NS_PER_MS;
	};
	const offsets = sessions.map((s) => {
		const from = s.start_ns - midnightOf(s.start_ns);
		return { from, to: from + (s.end_ns - s.start_ns) };
	});
	const hourNs = 3600 * NS_PER_SEC;
	// Padded before rounding to the hour, so a session starting on the hour
	// still has room before it and no bar touches an edge.
	const edgeNs = 5 * 60 * NS_PER_SEC;
	const clockStartNs = sessions.length
		? Math.floor((Math.min(...offsets.map((o) => o.from)) - edgeNs) / hourNs) * hourNs
		: 0;
	const clockEndNs = sessions.length
		? Math.max(
				Math.ceil((Math.max(...offsets.map((o) => o.to)) + edgeNs) / hourNs) * hourNs,
				clockStartNs + hourNs
			)
		: hourNs;
	const span = clockEndNs - clockStartNs;

	const rows: CoverageRow[] = [];
	for (const [day, daySessions] of byDay) {
		const midnight = midnightOf(daySessions[0].start_ns);
		const startNs = midnight + clockStartNs;
		const endNs = midnight + clockEndNs;
		const bar = (session: CaptureSession): CoverageBar => ({
			session,
			left: ((session.start_ns - startNs) / span) * 100,
			width: Math.max(((session.end_ns - session.start_ns) / span) * 100, 0.4)
		});

		const byFolder = new Map<string, CaptureSession[]>();
		for (const s of daySessions) {
			const folder = folderOf(s);
			const list = byFolder.get(folder);
			if (list) list.push(s);
			else byFolder.set(folder, [s]);
		}

		const lanes: CoverageLane[] = [];
		for (const folder of [...byFolder.keys()].sort()) {
			// Greedy interval packing: each session takes the first lane it
			// does not overlap, so a folder uses as few lanes as it can.
			const packed: { endNs: number; bars: CoverageBar[] }[] = [];
			const ordered = byFolder
				.get(folder)!
				.slice()
				.sort((a, b) => a.start_ns - b.start_ns || a.end_ns - b.end_ns);
			for (const s of ordered) {
				const lane = packed.find((l) => l.endNs <= s.start_ns);
				if (lane) {
					lane.bars.push(bar(s));
					lane.endNs = s.end_ns;
				} else {
					packed.push({ endNs: s.end_ns, bars: [bar(s)] });
				}
			}
			packed.forEach((lane, i) =>
				lanes.push({ key: `${folder}#${i}`, folder, firstOfFolder: i === 0, bars: lane.bars })
			);
		}

		rows.push({
			day,
			dayLabel: formatDay(daySessions[0].start_ns),
			startNs,
			endNs,
			clockStartNs,
			clockEndNs,
			lanes
		});
	}

	return rows.sort((a, b) => (a.day < b.day ? 1 : -1));
}

/** A clock time marked along the coverage axis. */
export interface CoverageTick {
	label: string;
	/** Percentage from the left edge of the timeline. */
	left: number;
}

/**
 * coverageTicks marks whole hours along the axis every row shares, every
 * second hour once the axis is longer than ten.
 */
export function coverageTicks(rows: CoverageRow[]): CoverageTick[] {
	if (rows.length === 0) return [];
	const { clockStartNs, clockEndNs } = rows[0];
	const hourNs = 3600 * NS_PER_SEC;
	const span = clockEndNs - clockStartNs;
	if (span <= 0) return [];
	const step = span > 10 * hourNs ? 2 * hourNs : hourNs;
	const ticks: CoverageTick[] = [];
	for (let t = Math.ceil(clockStartNs / step) * step; t <= clockEndNs; t += step) {
		const hour = Math.round(t / hourNs) % 24;
		ticks.push({
			label: `${String(hour).padStart(2, '0')}:00`,
			left: ((t - clockStartNs) / span) * 100
		});
	}
	return ticks;
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
