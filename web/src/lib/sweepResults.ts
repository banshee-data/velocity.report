/**
 * Sweep results for display: a label per parameter combination, the fixed
 * set of charts the sweep pages draw, and the results table's columns.
 */
import { buildHeatmap, type BarDatum, type HeatmapData } from '#lib/components/charts/chartData.js';
import { canonicalParamKey, normaliseParamMap, PARAM_SCHEMA, paramLabel } from '#lib/sweepSetup.js';

export type SweepResult = Record<string, unknown>;

/** A result's value for a parameter or metric key, or null. */
export function extractValue(result: SweepResult, key: string): unknown {
	const canonical = canonicalParamKey(key);
	const params = normaliseParamMap(result.param_values ?? {});
	if (params[canonical] !== undefined) return params[canonical];
	if (result[canonical] !== undefined) return result[canonical];
	if (result[key] !== undefined) return result[key];
	return null;
}

function numberOrNull(v: unknown): number | null {
	return typeof v === 'number' && Number.isFinite(v) ? v : null;
}

/** "short=value" per parameter, shortened to the key's last segment. */
export function comboLabel(result: SweepResult): string {
	const params = normaliseParamMap(result.param_values ?? {});
	const parts = Object.entries(params).map(([key, v]) => {
		const short = key.split('_').length > 1 ? (key.split('_').pop() as string) : key;
		const value = typeof v === 'number' && !Number.isInteger(v) ? v.toFixed(3) : String(v);
		return `${short}=${value}`;
	});
	return parts.length > 0 ? parts.join(', ') : '(defaults)';
}

/** A metric key as a label: "overall_accept_mean" → "Overall Accept Mean". */
export function metricLabel(key: string): string {
	const schema = PARAM_SCHEMA[canonicalParamKey(key)];
	if (schema) return schema.label;
	return key.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
}

/** One chart of the fixed set. */
export type SweepChart =
	| { kind: 'bars'; id: string; title: string; metric: string; data: BarDatum[] }
	| {
			kind: 'heatmap';
			id: string;
			title: string;
			xKey: string;
			yKey: string;
			metric: string;
			data: HeatmapData;
	  };

/** Bars of one metric, one per combination, in run order. */
export function comboBars(results: SweepResult[], metric: string): BarDatum[] {
	return results.map((r) => ({
		label: comboLabel(r),
		value: numberOrNull(extractValue(r, metric))
	}));
}

/** A metric over two swept parameters. */
export function paramHeatmap(
	results: SweepResult[],
	xKey: string,
	yKey: string,
	metric: string
): HeatmapData {
	const cells = [];
	for (const r of results) {
		const x = extractValue(r, xKey);
		const y = extractValue(r, yKey);
		if (x == null || y == null) continue;
		cells.push({ x: String(x), y: String(y), value: numberOrNull(extractValue(r, metric)) ?? 0 });
	}
	return buildHeatmap(cells);
}

/** The swept parameters that are numbers, from the first result. */
export function numericParams(results: SweepResult[]): string[] {
	const first = normaliseParamMap(results[0]?.param_values ?? {});
	return Object.keys(first).filter((k) => typeof first[k] === 'number');
}

/**
 * The charts every sweep shows: acceptance, nonzero background cells,
 * active tracks and alignment per combination, and acceptance, tracks and
 * alignment over the first two numeric parameters when there are two.
 */
export function defaultSweepCharts(results: SweepResult[]): SweepChart[] {
	const bars = (id: string, title: string, metric: string): SweepChart => ({
		kind: 'bars',
		id,
		title,
		metric,
		data: comboBars(results, metric)
	});
	const charts: SweepChart[] = [
		bars('accept', 'Acceptance rate', 'overall_accept_mean'),
		bars('nzcells', 'Nonzero background cells', 'nonzero_cells_mean'),
		bars('tracks', 'Active tracks', 'active_tracks_mean'),
		bars('align', 'Alignment (°)', 'alignment_deg_mean')
	];
	const params = numericParams(results);
	if (params.length >= 2) {
		const [xKey, yKey] = params;
		const heatmap = (id: string, title: string, metric: string): SweepChart => ({
			kind: 'heatmap',
			id,
			title,
			xKey,
			yKey,
			metric,
			data: paramHeatmap(results, xKey, yKey, metric)
		});
		charts.push(
			heatmap('accept-hm', 'Acceptance heatmap', 'overall_accept_mean'),
			heatmap('tracks-hm', 'Tracks heatmap', 'active_tracks_mean'),
			heatmap('align-hm', 'Alignment heatmap', 'alignment_deg_mean')
		);
	}
	return charts;
}

/** A results-table column: a header and a cell formatter. */
export interface ResultColumn {
	key: string;
	header: string;
	cell: (r: SweepResult) => string;
}

const pct =
	(key: string, digits = 1) =>
	(r: SweepResult) =>
		`${((numberOrNull(r[key]) ?? 0) * 100).toFixed(digits)}%`;
const fixed =
	(key: string, digits: number, suffix = '') =>
	(r: SweepResult) =>
		`${(numberOrNull(r[key]) ?? 0).toFixed(digits)}${suffix}`;

const GROUND_TRUTH_COLUMNS: ResultColumn[] = [
	{
		key: 'gt',
		header: 'GT score',
		cell: (r) =>
			(
				((numberOrNull(r.ground_truth_score) ?? numberOrNull(r.composite_score)) || 0) * 100
			).toFixed(2)
	},
	{ key: 'detection_rate', header: 'Detection', cell: pct('detection_rate') },
	{ key: 'fragmentation', header: 'Frag.', cell: pct('fragmentation') },
	{ key: 'false_positive_rate', header: 'FP rate', cell: pct('false_positive_rate') },
	{ key: 'quality_premium', header: 'Qual. prem.', cell: pct('quality_premium') },
	{ key: 'truncation_rate', header: 'Trunc.', cell: pct('truncation_rate') },
	{ key: 'velocity_noise_rate', header: 'Vel. noise', cell: pct('velocity_noise_rate') },
	{ key: 'stopped_recovery_rate', header: 'Stopped OK', cell: pct('stopped_recovery_rate') }
];

const STANDARD_COLUMNS: ResultColumn[] = [
	{ key: 'overall_accept_mean', header: 'Accept rate', cell: pct('overall_accept_mean', 2) },
	{
		key: 'overall_accept_stddev',
		header: '± SD',
		cell: (r) => `±${pct('overall_accept_stddev', 2)(r)}`
	},
	{ key: 'nonzero_cells_mean', header: 'Nonzero cells', cell: fixed('nonzero_cells_mean', 0) },
	{
		key: 'nonzero_cells_stddev',
		header: '± SD',
		cell: (r) => `±${fixed('nonzero_cells_stddev', 0)(r)}`
	},
	{ key: 'active_tracks_mean', header: 'Active tracks', cell: fixed('active_tracks_mean', 1) },
	{ key: 'alignment_deg_mean', header: 'Alignment', cell: fixed('alignment_deg_mean', 1, '°') },
	{ key: 'misalignment_ratio_mean', header: 'Misalignment', cell: pct('misalignment_ratio_mean') },
	{ key: 'foreground_capture_mean', header: 'FG capture', cell: pct('foreground_capture_mean') },
	{ key: 'unbounded_point_mean', header: 'Unbounded', cell: pct('unbounded_point_mean') },
	{ key: 'empty_box_ratio_mean', header: 'Empty box', cell: pct('empty_box_ratio_mean') },
	{
		key: 'fragmentation_ratio_mean',
		header: 'Fragmentation',
		cell: pct('fragmentation_ratio_mean')
	},
	{
		key: 'heading_jitter_deg_mean',
		header: 'Jitter',
		cell: fixed('heading_jitter_deg_mean', 1, '°')
	}
];

/**
 * The table's columns: one per swept parameter, then ground-truth scores
 * when the results carry them, otherwise the standard metrics.
 */
export function resultColumns(results: SweepResult[]): ResultColumn[] {
	const first = results[0];
	if (!first) return [];
	const paramColumns: ResultColumn[] = Object.keys(normaliseParamMap(first.param_values ?? {})).map(
		(key) => ({
			key,
			header: paramLabel(key),
			cell: (r) => {
				const v = normaliseParamMap(r.param_values ?? {})[key];
				return typeof v === 'number' && !Number.isInteger(v) ? v.toFixed(4) : String(v ?? '');
			}
		})
	);
	const groundTruth = first.detection_rate !== undefined || first.ground_truth_score !== undefined;
	return [...paramColumns, ...(groundTruth ? GROUND_TRUTH_COLUMNS : STANDARD_COLUMNS)];
}

/** A score, or "N/A" for null or the -MaxFloat64 sentinel. */
export function formatScore(score: unknown): string {
	const n = numberOrNull(score);
	return n == null || n <= -1e300 ? 'N/A' : n.toFixed(4);
}

/** Metric keys a recommendation carries beside its parameters. */
export const RECOMMENDATION_METRIC_KEYS = new Set([
	'score',
	'acceptance_rate',
	'misalignment_ratio',
	'alignment_deg',
	'nonzero_cells',
	'foreground_capture',
	'unbounded_point_ratio',
	'empty_box_ratio',
	'fragmentation_ratio',
	'heading_jitter_deg',
	'speed_jitter_mps'
]);

/** "Label=value" for each parameter of a recommendation or best result. */
export function formatParamValues(params: unknown): string {
	return Object.entries(normaliseParamMap(params ?? {}))
		.filter(([k]) => !RECOMMENDATION_METRIC_KEYS.has(k))
		.map(
			([k, v]) =>
				`${paramLabel(k)}=${typeof v === 'number' && !Number.isInteger(v) ? v.toFixed(4) : v}`
		)
		.join(', ');
}

/** One HINT labelling gate and whether it is met. */
export interface HINTGate {
	label: string;
	met: boolean;
}

/**
 * The gates a HINT round waits on before it can continue: the labelled share,
 * each class's minimum count, and a temporal spread the server checks.
 */
export function hintGates(st: {
	min_label_threshold: number;
	label_progress?: { progress_pct: number; by_class?: Record<string, number> };
	min_class_coverage?: Record<string, number>;
	min_temporal_spread_secs?: number;
}): HINTGate[] {
	const thresholdPct = st.min_label_threshold * 100;
	const gates: HINTGate[] = [
		{
			label: `Labelled ≥ ${thresholdPct.toFixed(0)}%`,
			met: (st.label_progress?.progress_pct ?? 0) >= thresholdPct
		}
	];
	const byClass = st.label_progress?.by_class ?? {};
	for (const [cls, need] of Object.entries(st.min_class_coverage ?? {})) {
		const have = byClass[cls] ?? 0;
		gates.push({ label: `${cls}: ${have}/${need}`, met: have >= need });
	}
	if ((st.min_temporal_spread_secs ?? 0) > 0) {
		// The server measures spread; the page cannot, so it is never shown met.
		gates.push({ label: `Temporal spread ≥ ${st.min_temporal_spread_secs}s`, met: false });
	}
	return gates;
}

/** Seconds left in a running auto-tune, from its pace so far, or null. */
export function autoTuneEtaSeconds(
	st: {
		status: string;
		started_at?: string;
		cumulative_completed?: number;
		cumulative_total?: number;
	},
	nowMs: number
): number | null {
	const done = st.cumulative_completed ?? 0;
	const total = st.cumulative_total ?? 0;
	if (st.status !== 'running' || !st.started_at || done <= 0 || total <= 0) return null;
	const elapsed = (nowMs - new Date(st.started_at).getTime()) / 1000;
	const remaining = (elapsed / done) * (total - done);
	return remaining > 0 && Number.isFinite(remaining) ? remaining : null;
}

/** Metric columns of the results CSV, after the parameter columns. */
export const CSV_METRIC_COLUMNS = [
	'overall_accept_mean',
	'overall_accept_stddev',
	'nonzero_cells_mean',
	'nonzero_cells_stddev',
	'active_tracks_mean',
	'active_tracks_stddev',
	'alignment_deg_mean',
	'alignment_deg_stddev',
	'misalignment_ratio_mean',
	'misalignment_ratio_stddev',
	'foreground_capture_mean',
	'foreground_capture_stddev',
	'unbounded_point_mean',
	'unbounded_point_stddev',
	'empty_box_ratio_mean',
	'empty_box_ratio_stddev',
	'fragmentation_ratio_mean',
	'heading_jitter_deg_mean',
	'heading_jitter_deg_stddev',
	'speed_jitter_mps_mean',
	'speed_jitter_mps_stddev'
] as const;

function csvCell(v: unknown): string {
	if (v == null) return '';
	const s = String(v);
	return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
}

/**
 * The results as CSV: the first result's parameters, then the metric
 * columns, as the sweep dashboard exported them. A metric a result lacks is
 * left empty rather than written as zero.
 */
export function resultsCsv(results: SweepResult[]): string {
	const paramKeys = Object.keys(normaliseParamMap(results[0]?.param_values ?? {}));
	const header = [...paramKeys, ...CSV_METRIC_COLUMNS];
	const rows = results.map((r) => {
		const params = normaliseParamMap(r.param_values ?? {});
		return [...paramKeys.map((k) => params[k]), ...CSV_METRIC_COLUMNS.map((k) => r[k])]
			.map(csvCell)
			.join(',');
	});
	return [header.join(','), ...rows].join('\n');
}

/** The scores a recommendation carries, labelled for display. */
export function recommendationMetrics(rec: Record<string, unknown> | null | undefined) {
	if (!rec) return [];
	const n = (k: string) => numberOrNull(rec[k]);
	const rows: Array<{ label: string; value: string }> = [];
	const add = (label: string, key: string, fmt: (v: number) => string) => {
		const v = n(key);
		if (v != null) rows.push({ label, value: fmt(v) });
	};
	const pctOf = (v: number) => `${(v * 100).toFixed(1)}%`;
	add('Score', 'score', (v) => formatScore(v));
	add('Acceptance', 'acceptance_rate', (v) => `${(v * 100).toFixed(2)}%`);
	add('Misalignment', 'misalignment_ratio', pctOf);
	add('Alignment', 'alignment_deg', (v) => `${v.toFixed(1)}°`);
	add('Nonzero cells', 'nonzero_cells', (v) => v.toFixed(0));
	add('Foreground capture', 'foreground_capture', pctOf);
	add('Unbounded', 'unbounded_point_ratio', pctOf);
	add('Empty boxes', 'empty_box_ratio', pctOf);
	add('Fragmentation', 'fragmentation_ratio', pctOf);
	add('Heading jitter', 'heading_jitter_deg', (v) => `${v.toFixed(1)}°`);
	return rows;
}

/** One auto-tune round's summary and the bounds it searched. */
export function roundSummary(round: Record<string, unknown>): { title: string; bounds: string } {
	const bounds = (round.bounds ?? {}) as Record<string, unknown>;
	const parts = Object.entries(bounds)
		.filter((entry): entry is [string, [number, number]] => {
			const b = entry[1];
			return Array.isArray(b) && b.length === 2 && b.every((x) => typeof x === 'number');
		})
		.map(([k, [lo, hi]]) => `${paramLabel(k)}: [${lo.toFixed(4)}, ${hi.toFixed(4)}]`);
	return {
		title: `Round ${round.round ?? '?'}: ${round.num_combos ?? '?'} combinations, best score ${formatScore(round.best_score)}`,
		bounds: parts.join(' · ')
	};
}

/** Score components in display order, with their weights and change. */
export const SCORE_COMPONENTS: Array<[string, string]> = [
	['detection_rate', 'Detection rate'],
	['fragmentation', 'Fragmentation'],
	['false_positives', 'False positives'],
	['velocity_coverage', 'Velocity coverage'],
	['quality_premium', 'Quality premium'],
	['truncation_rate', 'Truncation rate'],
	['velocity_noise_rate', 'Velocity noise rate'],
	['stopped_recovery', 'Stopped recovery']
];

/**
 * Parse score components that may arrive as a JSON string or an object;
 * null for anything else.
 */
export function parseScoreComponents(raw: unknown): Record<string, unknown> | null {
	if (typeof raw === 'string') {
		try {
			const parsed = JSON.parse(raw);
			return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed : null;
		} catch {
			return null;
		}
	}
	return raw && typeof raw === 'object' && !Array.isArray(raw)
		? (raw as Record<string, unknown>)
		: null;
}
