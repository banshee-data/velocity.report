// How Captures turns selected motion-pass periods into clips. A clip is one
// window over ordered captures, so it cannot hold a motion period between two
// static ones without replaying the motion: selected static periods become one
// clip each, or one per run of periods with nothing between them, and a
// motion period is never part of one. See the UI workflow plan's "Clips from
// periods".

import { formatClock, gradeSelection } from '#lib/captures/timeline.js';
import { PROBE_OK, type CaptureFile, type MotionPeriod } from '#lib/types/captures.js';
import type { ClipCapture, LidarClip } from '#lib/types/lidar.js';

const NS_PER_SEC = 1_000_000_000;

/** One clip the composer will make: a run of adjacent selected static periods. */
export type PlannedClip = {
	/** The periods it covers, in order; the first is recorded as its source period. */
	periods: MotionPeriod[];
	startNs: number;
	endNs: number;
	durationSecs: number;
};

/** Whether a period may be part of a clip: only a static one. */
export function isClipPeriod(period: MotionPeriod): boolean {
	return period.type === 'static';
}

/**
 * planPeriodClips groups the selected static periods into clips. Periods are
 * taken in ordinal order; a selected static period joins the clip before it
 * only when it directly follows that clip's last period, so any period between
 * them (a motion period, or a static one left unselected) starts a new clip.
 * Motion periods in the selection are ignored.
 */
export function planPeriodClips(
	periods: MotionPeriod[],
	selectedIds: Iterable<string>
): PlannedClip[] {
	const selected = new Set(selectedIds);
	const ordered = [...periods].sort((a, b) => a.ordinal - b.ordinal || a.start_ns - b.start_ns);
	const plans: PlannedClip[] = [];
	let current: MotionPeriod[] = [];
	const flush = () => {
		if (current.length === 0) return;
		const startNs = current[0].start_ns;
		const endNs = current[current.length - 1].end_ns;
		plans.push({
			periods: current,
			startNs,
			endNs,
			durationSecs: Math.max(0, (endNs - startNs) / NS_PER_SEC)
		});
		current = [];
	};
	for (const period of ordered) {
		if (selected.has(period.period_id) && isClipPeriod(period)) {
			current.push(period);
		} else {
			flush();
		}
	}
	flush();
	return plans;
}

/** The captures and trim one planned clip replays. */
export type ClipWindow = {
	files: CaptureFile[];
	startSecs: number;
	durationSecs: number;
	/** Whether the clip can be made: every capture probed, one continuous recording. */
	usable: boolean;
	/** Why it cannot, when it cannot. */
	reason?: string;
};

function overlaps(f: CaptureFile, startNs: number, endNs: number): boolean {
	return (
		f.first_packet_ns != null &&
		f.last_packet_ns != null &&
		f.last_packet_ns > startNs &&
		f.first_packet_ns < endNs
	);
}

/**
 * clipWindow finds the captures a planned clip's window overlaps and trims
 * into them. The clip starts at the later of the period's start and the first
 * capture's first packet, and ends at the period's end (or the last capture's
 * last packet, if sooner), so it never runs past the period into the motion
 * after it. A capture that overlaps the window but is not probed refuses the
 * clip rather than being left out: leaving it out would move the start onto a
 * later capture while the duration still reached past the period's end. The
 * captures are graded as the hand-picked selection is, so a broken seam is
 * reported rather than refused by the server. Null when no capture overlaps.
 */
export function clipWindow(plan: PlannedClip, files: CaptureFile[]): ClipWindow | null {
	const overlapping = files.filter((f) => overlaps(f, plan.startNs, plan.endNs));
	if (overlapping.length === 0) return null;
	const unprobed = overlapping.filter((f) => f.probe_state !== PROBE_OK);
	const verdict = gradeSelection(overlapping.filter((f) => f.probe_state === PROBE_OK));
	const first = verdict.files[0];
	const last = verdict.files[verdict.files.length - 1];
	if (!first || first.first_packet_ns == null || last?.last_packet_ns == null) {
		return {
			files: [],
			startSecs: 0,
			durationSecs: 0,
			usable: false,
			reason: `${overlapping.length} capture${overlapping.length === 1 ? ' covers' : 's cover'} this period but none is probed. Run a scan first.`
		};
	}
	const startNs = Math.max(plan.startNs, first.first_packet_ns);
	const endNs = Math.min(plan.endNs, last.last_packet_ns);
	const window = {
		files: verdict.files,
		startSecs: Math.max(0, (startNs - first.first_packet_ns) / NS_PER_SEC),
		durationSecs: Math.max(0, (endNs - startNs) / NS_PER_SEC)
	};
	if (unprobed.length > 0) {
		return {
			...window,
			usable: false,
			reason: `${unprobed.map((f) => f.rel_path).join(', ')} ${unprobed.length === 1 ? 'covers' : 'cover'} part of this period but ${unprobed.length === 1 ? 'is' : 'are'} not probed, so the clip could replay the wrong stretch. Run a scan first.`
		};
	}
	return { ...window, usable: verdict.usable, reason: verdict.reason };
}

/** planKey names a planned clip by every period it covers, for state kept per clip. */
export function planKey(plan: PlannedClip): string {
	return plan.periods.map((p) => p.period_id).join('+');
}

/**
 * clipSpanNs is the capture-clock window an existing clip replays, read from
 * its captures and trim against this session's files: from the first
 * capture's first packet plus its start offset, for its duration or to the
 * last capture's last packet. Null when its first capture is not one of
 * these files.
 */
export function clipSpanNs(
	clip: LidarClip,
	files: CaptureFile[]
): { startNs: number; endNs: number } | null {
	const captures = [...(clip.files ?? [])].sort((a, b) => a.ordinal - b.ordinal);
	if (captures.length === 0) return null;
	const find = (c: ClipCapture) =>
		files.find((f) =>
			c.capture_file_id ? f.capture_file_id === c.capture_file_id : f.rel_path === c.pcap_file
		);
	const first = find(captures[0]);
	const last = find(captures[captures.length - 1]);
	if (first?.first_packet_ns == null) return null;
	const startNs = first.first_packet_ns + (clip.pcap_start_secs ?? 0) * NS_PER_SEC;
	const endNs =
		clip.pcap_duration_secs != null
			? startNs + clip.pcap_duration_secs * NS_PER_SEC
			: (last?.last_packet_ns ?? first.last_packet_ns ?? startNs);
	return { startNs, endNs };
}

/**
 * clipsOverlapping lists the existing clips whose window overlaps a planned
 * clip's, whatever period they were cut from: a fresh motion pass renumbers
 * periods, so a clip's source period is not a reliable match.
 */
export function clipsOverlapping(
	plan: PlannedClip,
	clips: LidarClip[],
	files: CaptureFile[]
): LidarClip[] {
	return clips.filter((clip) => {
		const span = clipSpanNs(clip, files);
		return span != null && span.startNs < plan.endNs && plan.startNs < span.endNs;
	});
}

/** clipDurationText says a clip's length the way a description reads it: 45 s, 12 min, 1 h 5 min. */
export function clipDurationText(secs: number): string {
	if (secs < 90) return `${Math.round(secs)} s`;
	const mins = Math.round(secs / 60);
	if (mins < 60) return `${mins} min`;
	const h = Math.floor(mins / 60);
	const m = mins % 60;
	return m === 0 ? `${h} h` : `${h} h ${m} min`;
}

/**
 * describePlannedClip prefills a clip's description from its site, its
 * capture-clock times and its duration: "Kirkham at 9th, 14:58–15:12, 14 min
 * static". It names no period: period labels are ordinals a fresh motion pass
 * renumbers, so a description that named one would go stale. The site is the
 * session's name until sessions take their site from a deployment.
 */
export function describePlannedClip(
	site: string,
	plan: PlannedClip,
	clock: (ns: number) => string = formatClock
): string {
	const where = site.trim() || 'Unnamed site';
	return `${where}, ${clock(plan.startNs)}–${clock(plan.endNs)}, ${clipDurationText(plan.durationSecs)} static`;
}

/** composerSummary says how many clips the composer will make from how many periods. */
export function composerSummary(plans: PlannedClip[]): string {
	const periods = plans.reduce((n, p) => n + p.periods.length, 0);
	if (plans.length === 0) return 'Select static periods to make clips from them.';
	const clips = `${plans.length} ${plans.length === 1 ? 'clip' : 'clips'}`;
	const from = `${periods} ${periods === 1 ? 'period' : 'periods'}`;
	return `Make ${clips} from ${from}`;
}
