// How the LiDAR pages put a run's or a sweep's identity in one table cell: the
// clip a run was replayed from, a parameter digest short enough to compare by
// eye, a run's labelling share, and the objective a sweep scored against.

import type { AnalysisRun, LidarReplayCase } from '#lib/types/lidar.js';

/**
 * clipForRun is the clip a run was replayed from. A run started from a clip
 * records it; most runs predate that or were started from the capture, so
 * the fallbacks are a clip that names the run as its legacy reference run,
 * then a clip over the same capture from the same sensor. That last match is
 * a guess when several clips share one capture; it picks the first listed.
 */
export function clipForRun(
	run: AnalysisRun | null | undefined,
	clips: LidarReplayCase[]
): LidarReplayCase | null {
	if (!run) return null;
	if (run.replay_case_id) {
		const recorded = clips.find((c) => c.replay_case_id === run.replay_case_id);
		if (recorded) return recorded;
	}
	const byReference = clips.find((c) => c.reference_run_id === run.run_id);
	if (byReference) return byReference;
	const source = run.source_path;
	if (!source) return null;
	return (
		clips.find((c) => c.sensor_id === run.sensor_id && sameCapture(source, c.pcap_file)) ?? null
	);
}

/**
 * sameCapture compares a run's source with a clip's capture. A clip records
 * its capture relative to the capture volume (s2/s2_sf_3_….pcap) and a run the
 * absolute path it read, so the clip's path must end the run's, on a path
 * boundary: s2/x.pcap matches /Volumes/lidar/lidar/s2/x.pcap, not …/ss2/x.pcap.
 */
function sameCapture(source: string, capture: string | undefined): boolean {
	if (!capture) return false;
	return source === capture || source.endsWith(`/${capture.replace(/^\/+/, '')}`);
}

/**
 * shortDigest shows a content digest by its leading hex digits. Eight is the
 * floor: a sweep makes hundreds of parameter sets, and at four digits two of
 * them would likely read the same.
 */
export function shortDigest(digest: string | null | undefined, length = 8): string {
	if (!digest) return '';
	const hex = digest.startsWith('sha256:') ? digest.slice('sha256:'.length) : digest;
	return hex.slice(0, length);
}

/** The per-run label counts the runs list returns (label_rollup). */
export type LabelRollup = {
	total: number;
	classified: number;
	tagged_only: number;
	unlabelled: number;
};

export type LabelShare = {
	/** Tracks with a class or a quality flag, as the server counts labelled. */
	labelled: number;
	total: number;
	/** labelled / total, from 0 to 1. */
	fraction: number;
};

/**
 * labelShare is how much of a run someone has labelled in the macOS
 * visualiser, or null when nobody has labelled any of it: most runs are
 * never labelled, and a column of 0/190 bars says less than a dash.
 */
export function labelShare(rollup: LabelRollup | null | undefined): LabelShare | null {
	if (!rollup || rollup.total <= 0) return null;
	const labelled = rollup.classified + rollup.tagged_only;
	if (labelled <= 0) return null;
	return { labelled, total: rollup.total, fraction: Math.min(1, labelled / rollup.total) };
}

/** objectiveText names a sweep's objective as prose: ground_truth → "ground truth". */
export function objectiveText(name: string | null | undefined): string {
	return name ? name.replace(/_/g, ' ') : '';
}
