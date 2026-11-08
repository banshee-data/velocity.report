// How Captures turns selected motion-pass periods into clips. A clip is one
// window over ordered captures, so it cannot hold a motion period between two
// static ones without replaying the motion: selected static periods become one
// clip each, or one per run of periods with nothing between them, and a
// motion period is never part of one. See the UI workflow plan's "Clips from
// periods".

import { gradeSelection, periodTrim } from '#lib/captures/timeline.js';
import { PROBE_OK, type CaptureFile, type MotionPeriod } from '#lib/types/captures.js';

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
	/** Whether the server would accept the captures as one continuous recording. */
	usable: boolean;
	/** Why it would not, when it would not. */
	reason?: string;
};

/**
 * clipWindow finds the probed captures a planned clip's window overlaps and
 * trims into them: the start is measured from the first overlapping capture's
 * first packet, so that capture's leading seconds, recorded before the period,
 * are skipped. The captures are graded as the hand-picked selection is, so a
 * broken seam inside the window is reported rather than refused by the
 * server. Null when no probed capture overlaps the window.
 */
export function clipWindow(plan: PlannedClip, files: CaptureFile[]): ClipWindow | null {
	const verdict = gradeSelection(
		files.filter(
			(f) =>
				f.probe_state === PROBE_OK &&
				f.first_packet_ns != null &&
				f.last_packet_ns != null &&
				f.last_packet_ns > plan.startNs &&
				f.first_packet_ns < plan.endNs
		)
	);
	const firstNs = verdict.files[0]?.first_packet_ns;
	if (verdict.files.length === 0 || firstNs == null) return null;
	const trim = periodTrim({ start_ns: plan.startNs, end_ns: plan.endNs }, firstNs);
	return {
		files: verdict.files,
		startSecs: trim.startSecs,
		durationSecs: trim.durationSecs,
		usable: verdict.usable,
		reason: verdict.reason
	};
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
 * describePlannedClip prefills a clip's description from its site and
 * duration, with the periods it covers: "Kirkham at 9th, 12 min static
 * (static-0)". The site is the session's name until sessions take their site
 * from a deployment.
 */
export function describePlannedClip(site: string, plan: PlannedClip): string {
	const where = site.trim() || 'Unnamed site';
	const labels = plan.periods.map((p) => p.label).join(', ');
	return `${where}, ${clipDurationText(plan.durationSecs)} static (${labels})`;
}

/** composerSummary says how many clips the composer will make from how many periods. */
export function composerSummary(plans: PlannedClip[]): string {
	const periods = plans.reduce((n, p) => n + p.periods.length, 0);
	if (plans.length === 0) return 'Select static periods to make clips from them.';
	const clips = `${plans.length} ${plans.length === 1 ? 'clip' : 'clips'}`;
	const from = `${periods} ${periods === 1 ? 'period' : 'periods'}`;
	return `Make ${clips} from ${from}`;
}
