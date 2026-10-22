/**
 * Data shaping for the LayerChart components in this directory. The
 * components only lay marks out; everything here is plain data in, plain data
 * out, so it is tested without a DOM.
 */

/**
 * Ten-stop viridis ramp for magnitude colouring (points seen, cluster size,
 * a sweep metric). It is neutral and separate from the §3.3 percentile
 * palette in palette.ts, and matches the Go grid chart's ramp.
 */
export const VIRIDIS = [
	'#440154',
	'#482777',
	'#3e4989',
	'#31688e',
	'#26828e',
	'#1f9e89',
	'#35b779',
	'#6ece58',
	'#b5de2b',
	'#fde725'
] as const;

/** Fill for a cell or point with no value. */
export const NO_DATA_COLOUR = '#e5e7eb';

/** Background points on the foreground frame chart. */
export const BACKGROUND_POINT_COLOUR = '#9e9e9e';
/** Foreground points on the foreground frame chart. */
export const FOREGROUND_POINT_COLOUR = '#ff5252';
/** Single-series bars. */
export const BAR_COLOUR = '#3e4989';

/** A point on an XY chart, with the magnitude it is coloured by. */
export interface XYPoint {
	x: number;
	y: number;
	value?: number;
}

/** A point carrying its fill colour. */
export interface ColouredPoint extends XYPoint {
	colour: string;
}

/** One series of an XY scatter. */
export interface ScatterSeries {
	key: string;
	label: string;
	points: Array<XYPoint | ColouredPoint>;
	/** Fill for points that carry no colour of their own. */
	colour?: string;
}

/**
 * Colour for a value on a 0..max scale. A value below the domain takes the
 * first stop and one above it the last; a non-positive max colours
 * everything as the first stop.
 */
export function rampColour(value: number, min: number, max: number): string {
	if (!Number.isFinite(value) || !(max > min)) return VIRIDIS[0];
	const unit = Math.min(1, Math.max(0, (value - min) / (max - min)));
	return VIRIDIS[Math.min(VIRIDIS.length - 1, Math.floor(unit * VIRIDIS.length))];
}

/** Colour each point by its value over the largest value present. */
export function colourByValue(points: XYPoint[], max?: number): ColouredPoint[] {
	const top = max ?? points.reduce((m, p) => Math.max(m, p.value ?? 0), 0);
	return points.map((p) => ({ ...p, colour: rampColour(p.value ?? 0, 0, top) }));
}

/**
 * A square domain centred on the sensor, so an XY chart of metres keeps its
 * aspect: the largest absolute coordinate, padded by 5 %. An empty set gets
 * [-1, 1].
 */
export function symmetricDomain(...sets: XYPoint[][]): [number, number] {
	let maxAbs = 0;
	for (const set of sets) {
		for (const p of set) {
			maxAbs = Math.max(maxAbs, Math.abs(p.x), Math.abs(p.y));
		}
	}
	const pad = maxAbs * 1.05;
	return pad > 0 ? [-pad, pad] : [-1, 1];
}

/** One bar of a category bar chart. */
export interface BarDatum {
	label: string;
	value: number | null;
}

/** One cell of a category heatmap. */
export interface HeatmapCell {
	x: string;
	y: string;
	value: number;
	colour: string;
}

/** A category heatmap ready to draw. */
export interface HeatmapData {
	xValues: string[];
	yValues: string[];
	cells: HeatmapCell[];
	min: number;
	max: number;
}

/**
 * Order category labels: numerically when every label is a number, so 10
 * sorts after 9, otherwise lexically.
 */
export function sortCategories(labels: Iterable<string>): string[] {
	const unique = [...new Set(labels)];
	const numeric = unique.every((l) => l.trim() !== '' && !Number.isNaN(Number(l)));
	return numeric ? unique.sort((a, b) => Number(a) - Number(b)) : unique.sort();
}

/**
 * Lay out a heatmap from cells keyed by category. The colour scale runs from
 * the smallest positive value to the largest, as the sweep dashboard's did,
 * so a column of zeros does not wash the rest out; with no spread it runs
 * from zero.
 */
export function buildHeatmap(cells: Array<{ x: string; y: string; value: number }>): HeatmapData {
	let max = 0;
	let min = Infinity;
	for (const c of cells) {
		if (c.value > max) max = c.value;
		if (c.value > 0 && c.value < min) min = c.value;
	}
	if (min === Infinity || min >= max) min = 0;
	return {
		xValues: sortCategories(cells.map((c) => c.x)),
		yValues: sortCategories(cells.map((c) => c.y)),
		cells: cells.map((c) => ({ ...c, colour: rampColour(c.value, min, max) })),
		min,
		max
	};
}

/** Format a number for an axis or tooltip without trailing noise. */
export function formatValue(value: number | null | undefined, digits = 3): string {
	if (value == null || !Number.isFinite(value)) return '—';
	if (Number.isInteger(value)) return value.toLocaleString();
	return value.toFixed(digits);
}
