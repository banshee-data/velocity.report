// Links between the LiDAR pages. They run in the order the work does —
// Captures, Clips, Segments, Runs, Tracks, Sweeps — and each page names the
// records the next starts from: a clip its reference run, a run its clip, both
// the Tracks view of them. A link carries those identities as a query, and the
// page it opens reads them back here, so a parameter is named in one place.
//
// svelte/no-navigation-without-resolve accepts only a bare resolve() call as
// an href, so a page writes `${resolve('/lidar/runs')}?${runQuery(id)}` inside
// an eslint-disable for that rule, as the Segments page already does.

/** The parameter that opens one record on the Clips and Runs pages. */
const ID_PARAM = 'id';

/** clipQuery opens one clip on the Clips page. */
export function clipQuery(clipId: string): string {
	return new URLSearchParams({ [ID_PARAM]: clipId }).toString();
}

/** runQuery opens one run on the Runs page. */
export function runQuery(runId: string): string {
	return new URLSearchParams({ [ID_PARAM]: runId }).toString();
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
 * `replay_case_id` is the clip's identity until the API is renamed.
 */
export function tracksQuery(link: TracksLink): string {
	const params = new URLSearchParams();
	if (link.sensorId) params.set('sensor_id', link.sensorId);
	if (link.clipId) params.set('replay_case_id', link.clipId);
	if (link.runId) params.set('run_id', link.runId);
	if (link.atNs != null && link.atNs !== '') params.set('at_ns', String(link.atNs));
	return params.toString();
}

/**
 * linkedId reads the record a clipQuery or runQuery link opened, if any. It
 * takes anything with search parameters, since SvelteKit's page.url is a
 * read-only URL rather than a URL.
 */
export function linkedId(url: {
	searchParams: { get(name: string): string | null };
}): string | null {
	const id = url.searchParams.get(ID_PARAM)?.trim();
	return id ? id : null;
}
