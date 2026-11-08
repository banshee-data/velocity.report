// Links between the LiDAR pages. They run in the order the work does —
// Captures, Clips, Runs, Windows, Tracks, Sweeps — and each page names the
// records the next starts from: a clip its reference run, a run its clip, both
// the Tracks view of them. A link carries those identities as a query, and the
// page it opens reads them back here, so a parameter is named in one place.
//
// svelte/no-navigation-without-resolve accepts only a bare resolve() call as
// an href, so a page writes `${resolve('/lidar/runs')}?${runQuery(id)}` inside
// an eslint-disable for that rule, as the Windows page already does.

/** The parameter that opens one record on the Clips and Runs pages. */
const ID_PARAM = 'id';

/** The parameter that opens the Windows page on a run's ranked windows. */
const RUN_PARAM = 'run';

/** clipQuery opens one clip on the Clips page. */
export function clipQuery(clipId: string): string {
	return new URLSearchParams({ [ID_PARAM]: clipId }).toString();
}

/** runQuery opens one run on the Runs page. */
export function runQuery(runId: string): string {
	return new URLSearchParams({ [ID_PARAM]: runId }).toString();
}

/** sweepQuery opens one sweep on the Sweeps page. */
export function sweepQuery(sweepId: string): string {
	return new URLSearchParams({ [ID_PARAM]: sweepId }).toString();
}

/** windowsQuery opens the Windows page on one run. */
export function windowsQuery(runId: string): string {
	return new URLSearchParams({ [RUN_PARAM]: runId }).toString();
}

/** linkedRun reads the run a windowsQuery link opened, if any. */
export function linkedRun(url: {
	searchParams: { get(name: string): string | null };
}): string | null {
	const id = url.searchParams.get(RUN_PARAM)?.trim();
	return id ? id : null;
}

export type TracksLink = {
	sensorId?: string | null;
	clipId?: string | null;
	runId?: string | null;
	/** An instant to open the player at, in Unix nanoseconds. */
	atNs?: number | string | null;
};

/**
 * tracksQuery opens the Tracks page on a run, its clip and, optionally, an
 * instant. The parameter names are the ones the Tracks page has always read;
 * `clip_id` is the clip's identity until the API is renamed.
 */
export function tracksQuery(link: TracksLink): string {
	const params = new URLSearchParams();
	if (link.sensorId) params.set('sensor_id', link.sensorId);
	if (link.clipId) params.set('clip_id', link.clipId);
	if (link.runId) params.set('run_id', link.runId);
	if (link.atNs != null && link.atNs !== '') params.set('at_ns', String(link.atNs));
	return params.toString();
}

/**
 * linkedId reads the record a clipQuery, runQuery or sweepQuery link opened, if any. It
 * takes anything with search parameters, since SvelteKit's page.url is a
 * read-only URL rather than a URL.
 */
export function linkedId(url: {
	searchParams: { get(name: string): string | null };
}): string | null {
	const id = url.searchParams.get(ID_PARAM)?.trim();
	return id ? id : null;
}

/** The URL scheme the macOS visualiser registers, chosen by the owner on November 7, 2026. */
export const VISUALISER_SCHEME = 'velocityreport';

/**
 * visualiserLink opens a run in the macOS visualiser, paused on a frame when
 * one is given: velocityreport://run/<run_id>?frame=<n>. The frame is the
 * 0-based index of a record in the run's VRLOG, background snapshots
 * included, which is how the visualiser counts; without one the run opens at
 * its start.
 */
export function visualiserLink(runId: string, frame?: number | null): string {
	const base = `${VISUALISER_SCHEME}://run/${encodeURIComponent(runId)}`;
	if (frame == null || !Number.isInteger(frame) || frame < 0) return base;
	return `${base}?frame=${frame}`;
}

/**
 * FRAME_CLOCK_SLACK_MS covers how far the web player's clock can read before
 * the frame it shows: the clock starts from the recording's start truncated
 * to whole milliseconds and adds a microsecond offset, so it sits up to just
 * over a millisecond early. Rotations are about 100 ms apart, so 2 ms of slack
 * cannot reach the next frame.
 */
export const FRAME_CLOCK_SLACK_MS = 2;

/**
 * frameInstantNs is the instant, in Unix nanoseconds as text (they pass
 * Number.MAX_SAFE_INTEGER), at which to ask for the frame on screen: the
 * player's clock floored to the millisecond, plus the clock's slack. The
 * server then names the latest frame at or before it.
 */
export function frameInstantNs(timeMs: number): string {
	return `${Math.floor(timeMs) + FRAME_CLOCK_SLACK_MS}000000`;
}
