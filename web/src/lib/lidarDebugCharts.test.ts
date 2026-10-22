import {
	BACKGROUND_POINT_COLOUR,
	FOREGROUND_POINT_COLOUR,
	VIRIDIS
} from './components/charts/chartData';
import {
	backgroundBucketSeries,
	backgroundCellSeries,
	clusterSeries,
	foregroundSeries,
	trackSeries,
	trafficBars
} from './lidarDebugCharts';
import type { Track } from './types/lidar';

describe('lidar debug chart series', () => {
	it('returns nothing for a pipeline with nothing to show', () => {
		expect(backgroundCellSeries(null)).toEqual([]);
		expect(backgroundBucketSeries(null)).toEqual([]);
		expect(foregroundSeries(null)).toEqual([]);
		expect(clusterSeries(null)).toEqual([]);
		expect(trafficBars(null)).toEqual([]);
	});

	it('colours background cells and buckets by the reported maximum', () => {
		const cells = backgroundCellSeries({
			points: [
				{ x: 1, y: 2, value: 5 },
				{ x: 3, y: 4, value: 10 }
			],
			max_abs: 4,
			max_value: 10,
			sensor_id: 's',
			stride: 1,
			num_points: 2
		});
		expect(cells[0].points.map((p) => ('colour' in p ? p.colour : ''))).toEqual([
			VIRIDIS[5],
			VIRIDIS[9]
		]);
		const buckets = backgroundBucketSeries({
			points: [{ x: 1, y: 1, value: 2 }],
			max_value: 0,
			max_abs: 1,
			sensor_id: 's',
			num_buckets: 1
		});
		// A zero maximum falls back to the largest value present.
		expect('colour' in buckets[0].points[0] && buckets[0].points[0].colour).toBe(VIRIDIS[9]);
	});

	it('draws background before foreground in fixed colours', () => {
		const series = foregroundSeries({
			foreground_points: [{ x: 1, y: 1, value: 0 }],
			background_points: [{ x: 2, y: 2, value: 0 }],
			max_abs: 2,
			foreground_count: 1,
			background_count: 1,
			total_points: 2,
			foreground_percent: 50,
			timestamp: '',
			sensor_id: 's'
		});
		expect(series.map((s) => [s.key, s.colour])).toEqual([
			['background', BACKGROUND_POINT_COLOUR],
			['foreground', FOREGROUND_POINT_COLOUR]
		]);
	});

	it('maps clusters and tracks to coloured points', () => {
		const clusters = clusterSeries({
			clusters: [{ x: 1, y: 2, points_count: 40 }],
			max_abs: 2,
			max_points: 40,
			num_clusters: 1,
			sensor_id: 's'
		});
		expect(clusters[0].points[0]).toMatchObject({ x: 1, y: 2, value: 40 });
		const tracks = trackSeries([
			{ position: { x: 3, y: -4, z: 0 }, observation_count: 7 } as unknown as Track
		]);
		expect(tracks[0].points[0]).toMatchObject({ x: 3, y: -4, value: 7 });
	});

	it('lists traffic as four bars', () => {
		expect(
			trafficBars({
				packets_per_sec: 1,
				mb_per_sec: 2,
				points_per_sec: 3,
				dropped_count: 4,
				timestamp: ''
			}).map((b) => b.value)
		).toEqual([1, 2, 3, 4]);
	});
});
