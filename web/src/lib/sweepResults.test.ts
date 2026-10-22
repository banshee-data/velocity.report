import { VIRIDIS } from './components/charts/chartData';
import {
	CSV_METRIC_COLUMNS,
	parseScoreComponents,
	recommendationMetrics,
	resultsCsv,
	roundSummary,
	autoTuneEtaSeconds,
	comboBars,
	comboLabel,
	defaultSweepCharts,
	extractValue,
	formatParamValues,
	formatScore,
	hintGates,
	metricLabel,
	numericParams,
	paramHeatmap,
	resultColumns
} from './sweepResults';

const EPS = 'l4.dbscan_xy_v1.foreground_dbscan_eps';
const POINTS = 'l4.dbscan_xy_v1.foreground_min_cluster_points';

const results = [
	{
		param_values: { foreground_dbscan_eps: 0.3, foreground_min_cluster_points: 4 },
		overall_accept_mean: 0.9,
		overall_accept_stddev: 0.01,
		nonzero_cells_mean: 1000,
		nonzero_cells_stddev: 10,
		active_tracks_mean: 5,
		alignment_deg_mean: 2.5
	},
	{
		param_values: { foreground_dbscan_eps: 0.6, foreground_min_cluster_points: 8 },
		overall_accept_mean: 0.8,
		overall_accept_stddev: 0.02,
		nonzero_cells_mean: 900,
		nonzero_cells_stddev: 9,
		active_tracks_mean: 6,
		alignment_deg_mean: 3
	}
];

describe('extractValue', () => {
	it('reads parameters before metrics, by full key or suffix', () => {
		expect(extractValue(results[0], EPS)).toBe(0.3);
		expect(extractValue(results[0], 'foreground_dbscan_eps')).toBe(0.3);
		expect(extractValue(results[0], 'overall_accept_mean')).toBe(0.9);
		expect(extractValue({ 'odd.key': 1 }, 'odd.key')).toBe(1);
		expect(extractValue({ [EPS]: 7 }, 'foreground_dbscan_eps')).toBe(7);
		expect(extractValue({}, 'missing')).toBeNull();
	});
});

describe('labels', () => {
	it('labels a combination by short keys', () => {
		expect(comboLabel(results[0])).toBe('eps=0.300, points=4');
		expect(comboLabel({ param_values: { speed: true } })).toBe('speed=true');
		expect(comboLabel({})).toBe('(defaults)');
	});

	it('labels metrics and parameters', () => {
		expect(metricLabel('overall_accept_mean')).toBe('Overall Accept Mean');
		expect(metricLabel(EPS)).toBe('FG DBSCAN Eps');
		expect(metricLabel('foreground_min_cluster_points')).toBe('FG Min Cluster Points');
	});
});

describe('charts', () => {
	it('draws one bar per combination, null for a missing metric', () => {
		expect(comboBars(results, 'active_tracks_mean')).toEqual([
			{ label: 'eps=0.300, points=4', value: 5 },
			{ label: 'eps=0.600, points=8', value: 6 }
		]);
		expect(comboBars(results, 'nope').map((b) => b.value)).toEqual([null, null]);
	});

	it('lays a metric over two parameters', () => {
		const hm = paramHeatmap(results, EPS, POINTS, 'overall_accept_mean');
		expect(hm.xValues).toEqual(['0.3', '0.6']);
		expect(hm.yValues).toEqual(['4', '8']);
		expect(hm.cells.map((c) => c.colour)).toEqual([VIRIDIS[9], VIRIDIS[0]]);
		expect(paramHeatmap([{ param_values: {} }], EPS, POINTS, 'x').cells).toEqual([]);
		expect(paramHeatmap(results, EPS, POINTS, 'missing').cells.map((c) => c.value)).toEqual([0, 0]);
	});

	it('finds the numeric parameters', () => {
		expect(numericParams(results)).toEqual([EPS, POINTS]);
		expect(numericParams([])).toEqual([]);
	});

	it('adds the heatmaps only with two numeric parameters', () => {
		expect(defaultSweepCharts(results).map((c) => c.id)).toEqual([
			'accept',
			'nzcells',
			'tracks',
			'align',
			'accept-hm',
			'tracks-hm',
			'align-hm'
		]);
		const one = [{ param_values: { foreground_dbscan_eps: 0.3 }, overall_accept_mean: 1 }];
		expect(defaultSweepCharts(one).map((c) => c.kind)).toEqual(['bars', 'bars', 'bars', 'bars']);
	});
});

describe('resultColumns', () => {
	it('lists parameters then the standard metrics', () => {
		const cols = resultColumns(results);
		expect(cols.slice(0, 2).map((c) => c.header)).toEqual([
			'FG DBSCAN Eps',
			'FG Min Cluster Points'
		]);
		const cells = cols.map((c) => c.cell(results[0]));
		expect(cells.slice(0, 7)).toEqual(['0.3000', '4', '90.00%', '±1.00%', '1000', '±10', '5.0']);
		expect(cells).toContain('2.5°');
		expect(cols.find((c) => c.key === 'misalignment_ratio_mean')?.cell(results[0])).toBe('0.0%');
	});

	it('switches to ground-truth scores when the results have them', () => {
		const gt = [{ param_values: {}, detection_rate: 0.5, composite_score: 0.25 }];
		const cols = resultColumns(gt);
		expect(cols.map((c) => c.cell(gt[0]))).toEqual([
			'25.00',
			'50.0%',
			'0.0%',
			'0.0%',
			'0.0%',
			'0.0%',
			'0.0%',
			'0.0%'
		]);
		expect(cols[0].cell({ ground_truth_score: 0.5 })).toBe('50.00');
		expect(cols[0].cell({})).toBe('0.00');
	});

	it('has no columns without results, and blanks a missing parameter', () => {
		expect(resultColumns([])).toEqual([]);
		expect(resultColumns(results)[0].cell({})).toBe('');
	});
});

describe('formatting', () => {
	it('formats scores, guarding the sentinel', () => {
		expect(formatScore(0.123456)).toBe('0.1235');
		expect(formatScore(-1.7976931348623157e308)).toBe('N/A');
		expect(formatScore(null)).toBe('N/A');
		expect(formatScore('x')).toBe('N/A');
	});

	it('formats parameter values without the metric keys', () => {
		expect(
			formatParamValues({
				foreground_dbscan_eps: 0.31234,
				foreground_min_cluster_points: 4,
				score: 0.9
			})
		).toBe('FG DBSCAN Eps=0.3123, FG Min Cluster Points=4');
		expect(formatParamValues(undefined)).toBe('');
	});
});

describe('hintGates', () => {
	it('checks the share and each class, and never claims the spread', () => {
		expect(
			hintGates({
				min_label_threshold: 0.5,
				label_progress: { progress_pct: 60, by_class: { car: 2 } },
				min_class_coverage: { car: 3, pedestrian: 0 },
				min_temporal_spread_secs: 30
			})
		).toEqual([
			{ label: 'Labelled ≥ 50%', met: true },
			{ label: 'car: 2/3', met: false },
			{ label: 'pedestrian: 0/0', met: true },
			{ label: 'Temporal spread ≥ 30s', met: false }
		]);
		expect(hintGates({ min_label_threshold: 0.9 })).toEqual([
			{ label: 'Labelled ≥ 90%', met: false }
		]);
	});
});

describe('autoTuneEtaSeconds', () => {
	const started = '2026-10-21T10:00:00Z';
	const now = new Date('2026-10-21T10:01:40Z').getTime();
	it('projects the pace so far', () => {
		expect(
			autoTuneEtaSeconds(
				{ status: 'running', started_at: started, cumulative_completed: 10, cumulative_total: 30 },
				now
			)
		).toBe(200);
	});

	it('has no estimate before progress, when finished, or when not running', () => {
		expect(autoTuneEtaSeconds({ status: 'running', started_at: started }, now)).toBeNull();
		expect(
			autoTuneEtaSeconds(
				{ status: 'running', started_at: started, cumulative_completed: 5, cumulative_total: 5 },
				now
			)
		).toBeNull();
		expect(
			autoTuneEtaSeconds(
				{ status: 'complete', started_at: started, cumulative_completed: 1, cumulative_total: 5 },
				now
			)
		).toBeNull();
	});
});

describe('resultsCsv', () => {
	it('writes parameters then metrics, leaving missing values empty', () => {
		const csv = resultsCsv([
			{ param_values: { foreground_dbscan_eps: 0.3 }, overall_accept_mean: 0.9 },
			{ param_values: { foreground_dbscan_eps: 'a,"b"' } }
		]).split('\n');
		expect(csv[0]).toBe([EPS, ...CSV_METRIC_COLUMNS].join(','));
		expect(csv[1].startsWith('0.3,0.9,')).toBe(true);
		expect(csv[2].startsWith('"a,""b""",,')).toBe(true);
		expect(resultsCsv([])).toBe(CSV_METRIC_COLUMNS.join(','));
	});
});

describe('recommendationMetrics', () => {
	it('labels the metrics present and skips the rest', () => {
		expect(
			recommendationMetrics({
				score: 0.5,
				acceptance_rate: 0.9,
				misalignment_ratio: 0.1,
				alignment_deg: 3,
				nonzero_cells: 1200,
				foreground_capture: 0.8,
				unbounded_point_ratio: 0.2,
				empty_box_ratio: 0.3,
				fragmentation_ratio: 0.4,
				heading_jitter_deg: 5
			}).map((m) => `${m.label}=${m.value}`)
		).toEqual([
			'Score=0.5000',
			'Acceptance=90.00%',
			'Misalignment=10.0%',
			'Alignment=3.0°',
			'Nonzero cells=1200',
			'Foreground capture=80.0%',
			'Unbounded=20.0%',
			'Empty boxes=30.0%',
			'Fragmentation=40.0%',
			'Heading jitter=5.0°'
		]);
		expect(recommendationMetrics({ score: 'x' })).toEqual([]);
		expect(recommendationMetrics(null)).toEqual([]);
	});
});

describe('roundSummary', () => {
	it('summarises a round and its bounds', () => {
		expect(
			roundSummary({
				round: 2,
				num_combos: 25,
				best_score: 0.75,
				bounds: { foreground_dbscan_eps: [0.1, 0.5], bad: [1] }
			})
		).toEqual({
			title: 'Round 2: 25 combinations, best score 0.7500',
			bounds: 'FG DBSCAN Eps: [0.1000, 0.5000]'
		});
		expect(roundSummary({}).title).toBe('Round ?: ? combinations, best score N/A');
	});
});

describe('parseScoreComponents', () => {
	it('accepts an object or JSON text', () => {
		expect(parseScoreComponents({ a: 1 })).toEqual({ a: 1 });
		expect(parseScoreComponents('{"a":1}')).toEqual({ a: 1 });
		expect(parseScoreComponents('[1]')).toBeNull();
		expect(parseScoreComponents('{bad')).toBeNull();
		expect(parseScoreComponents([1])).toBeNull();
		expect(parseScoreComponents(null)).toBeNull();
	});
});
