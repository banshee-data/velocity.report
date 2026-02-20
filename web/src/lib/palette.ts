/**
 * Canonical percentile colour palette for velocity.report charts.
 *
 * These hex values are the single authoritative source for the web frontend.
 * They must match DESIGN.md §3.3 and the Python PDF generator palette in
 * tools/pdf-generator/pdf_generator/core/config_manager.py.
 *
 * Legend order: p50, p85, p98, max, then count/auxiliary signals.
 */

/** Percentile metric colours keyed by series name. */
export const PERCENTILE_COLOURS = {
	p50: '#fbd92f',
	p85: '#f7b32b',
	p98: '#f25f5c',
	max: '#2d1e2f',
	count_bar: '#2d1e2f',
	low_sample: '#f7b32b'
} as const;

/** Canonical legend order for percentile chart series. */
export const LEGEND_ORDER = ['p50', 'p85', 'p98', 'max'] as const;

export type PercentileKey = (typeof LEGEND_ORDER)[number];
