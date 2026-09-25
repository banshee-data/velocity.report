// Presentation helpers for a scene's following distribution
// (GET /api/scenes/<id>/headway). They format what the server served and
// decide nothing: which encounters enter the distribution, the bins, the
// excluded time and every statistic come from the Go aggregation in
// internal/lidar/l8behaviour/distribution.go. Metric ids and reason tokens
// are shown verbatim, as the registries define them.

import type {
	HeadwayDistribution,
	HeadwayEncounter,
	HeadwayMeasurement,
	HeadwayStatus,
	HeadwayVersionSummary,
	InteractionVersion,
	SceneHeadway
} from '$lib/api';

/** Registry ids the page reads (docs/platform/architecture/metrics-registry.md). */
export const HEADWAY_METRICS = {
	netTimeGap: 'interaction.following_net_time_gap_s',
	spatialGap: 'interaction.following_spatial_gap_m',
	validTime: 'interaction.following_valid_time_s',
	netTimeGapMin: 'interaction.following_net_time_gap_min_s',
	netTimeGapP50: 'interaction.following_net_time_gap_p50_s',
	spatialGapMin: 'interaction.following_spatial_gap_min_m'
} as const;

/**
 * The status as the chart draws it. Mirrors SurfaceStatus.Label in
 * internal/lidar/l8behaviour/distribution.go: both labels begin PROVISIONAL.
 */
export function headwayStatusLabel(status: HeadwayStatus): string {
	return status === 'synthetic_oracle' ? 'PROVISIONAL · SYNTHETIC ORACLE' : 'PROVISIONAL';
}

export function headwayStatusNote(status: HeadwayStatus): string {
	if (status === 'synthetic_oracle') {
		return 'Synthetic oracle: analytic trajectories with known gaps, not sensor data. Shown so the contract can be reviewed.';
	}
	return 'Estimator output that has not passed the field promotion gates. It claims no physical accuracy.';
}

/** Why a response carries no distribution, in words a reader can act on. */
export function headwayAvailabilityMessage(h: SceneHeadway, requestedStage = 'final'): string {
	switch (h.availability) {
		case 'available':
			return '';
		case 'no_capture_window':
			return 'This scene has no capture window, so no following analysis can be matched to it. Set its capture start and end.';
		case 'no_encounters':
			return 'No following encounters are stored for this capture window.';
		case 'source_ambiguous':
			return `${h.sources.length} analyses cover this capture window. Choose one to see its distribution.`;
		case 'no_matching_version':
			return h.versions.length > 0
				? `No ${requestedStage} analysis exists for this scene. Choose one of the versions that do.`
				: `No ${requestedStage} analysis exists for this scene.`;
	}
}

/** Seconds to one decimal place, from integer nanoseconds. */
export function formatNanos(nanos: number): string {
	return `${(nanos / 1e9).toFixed(1)} s`;
}

/** A share of a whole as a percentage; one decimal place below 10 %. */
export function formatShare(part: number, whole: number): string {
	if (whole <= 0) return '–';
	const pct = (part / whole) * 100;
	return pct > 0 && pct < 10 ? `${pct.toFixed(1)}%` : `${pct.toFixed(0)}%`;
}

/** A measurement's value with its interval, or its suppression reason. */
export function formatMeasurement(m: HeadwayMeasurement | undefined, digits = 2): string {
	if (!m) return '–';
	if (m.suppressed || m.value == null) return `suppressed: ${m.reason ?? 'unspecified'}`;
	const value = `${m.value.toFixed(digits)} ${m.unit}`;
	const u = m.uncertainty;
	if (u?.kind === 'interval' && u.lower != null && u.upper != null) {
		const coverage = u.coverage != null ? ` ${Math.round(u.coverage * 100)}%` : '';
		return `${value} [${u.lower.toFixed(digits)}, ${u.upper.toFixed(digits)}]${coverage}`;
	}
	return value;
}

export interface HeadwaySummaryRow {
	label: string;
	value: string;
	share?: string;
}

/** Where the accounted time went, and how many encounters fill the bins. */
export function headwaySummaryRows(d: HeadwayDistribution): HeadwaySummaryRow[] {
	const accounted = d.accounted_nanos;
	const excluded = Object.values(d.excluded).reduce((sum, x) => sum + x.nanos, 0);
	const binned = accounted - excluded;
	return [
		{ label: 'Encounters', value: `${d.events} (${d.exposure_events} in the distribution)` },
		{ label: 'Accounted time', value: formatNanos(accounted) },
		{
			label: 'Valid following time',
			value: formatNanos(d.accounting.valid_nanos),
			share: formatShare(d.accounting.valid_nanos, accounted)
		},
		{
			label: 'In the distribution',
			value: formatNanos(binned),
			share: formatShare(binned, accounted)
		},
		{
			label: 'Beside it, by reason',
			value: formatNanos(excluded),
			share: formatShare(excluded, accounted)
		},
		{
			label: 'Predicted-only',
			value: formatNanos(d.accounting.predicted_only_nanos),
			share: formatShare(d.accounting.predicted_only_nanos, accounted)
		},
		{ label: 'Record gaps, not counted', value: formatNanos(d.accounting.record_gap_nanos) }
	];
}

export interface HeadwayBandRow {
	name: string;
	threshold: string;
	below: string;
	rate: string;
	rateName: string;
	events: number;
}

/** Named-band exposure over the encounters in the distribution. */
export function headwayBandRows(d: HeadwayDistribution): HeadwayBandRow[] {
	return d.bands.map((b) => ({
		name: b.name,
		threshold: `${b.threshold.toFixed(1)} s`,
		below: formatNanos(b.nanos),
		rate:
			b.rate.suppressed || b.rate.value == null
				? `suppressed: ${b.rate.reason}`
				: formatShare(b.rate.value, 1),
		rateName: b.rate.name,
		events: b.events
	}));
}

export interface HeadwayExcludedRow {
	reason: string;
	instants: number;
	time: string;
	predictedOnly: string;
	share: string;
}

/** Accounted time beside the distribution, most first, then by token. */
export function headwayExcludedRows(d: HeadwayDistribution): HeadwayExcludedRow[] {
	return Object.entries(d.excluded)
		.sort(([a, x], [b, y]) => y.nanos - x.nanos || a.localeCompare(b))
		.map(([reason, x]) => ({
			reason,
			instants: x.instants,
			time: formatNanos(x.nanos),
			predictedOnly: formatNanos(x.predicted_only_nanos),
			share: formatShare(x.nanos, d.accounted_nanos)
		}));
}

export interface HeadwayEncounterRow {
	eventId: string;
	follower: string;
	leader: string;
	/** Seconds from the scene's capture start. */
	start: string;
	validTime: string;
	minNetTimeGap: string;
	medianNetTimeGap: string;
	minSpatialGap: string;
	/** Which stored block the values come from. */
	block: 'measurements' | 'provisional';
}

/** One row per encounter, values from the block its stage allows. */
export function headwayEncounterRows(h: SceneHeadway): HeadwayEncounterRow[] {
	const final = h.version?.estimate_stage === 'final';
	const origin = h.start_unix_nanos ?? 0;
	return h.encounters.map((e: HeadwayEncounter) => {
		const block = final ? e.measurements : (e.provisional ?? e.measurements);
		return {
			eventId: e.event_id,
			follower: e.primary_track_id,
			leader: e.secondary_track_id,
			start: `+${((e.start_unix_nanos - origin) / 1e9).toFixed(1)} s`,
			validTime: formatNanos(e.accounting.valid_nanos),
			minNetTimeGap: formatMeasurement(block[HEADWAY_METRICS.netTimeGapMin]),
			medianNetTimeGap: formatMeasurement(block[HEADWAY_METRICS.netTimeGapP50]),
			minSpatialGap: formatMeasurement(block[HEADWAY_METRICS.spatialGapMin], 1),
			block: final ? 'measurements' : 'provisional'
		};
	});
}

/** A stable key for one version, for a select's value. */
export function versionKey(v: InteractionVersion): string {
	return [v.estimate_stage, v.estimator_id, v.obs_model_id, v.method_id, v.param_hash].join('|');
}

export function versionLabel(v: HeadwayVersionSummary): string {
	return `${v.version.estimate_stage} · ${v.version.estimator_id} · ${v.version.method_id} · ${v.events} encounters`;
}
