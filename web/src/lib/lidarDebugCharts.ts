/**
 * Shapes the debug chart JSON into chart series for the LiDAR debug page:
 * the views the go-echarts pages drew, now drawn by LayerChart.
 */
import {
	BACKGROUND_POINT_COLOUR,
	colourByValue,
	FOREGROUND_POINT_COLOUR,
	type BarDatum,
	type ScatterSeries
} from '#lib/components/charts/chartData.js';
import type {
	ForegroundChartData,
	HeatmapBucketData,
	PolarChartData,
	RecentClustersData,
	TrafficMetrics
} from './lidarControl.js';
import type { Track } from './types/lidar.js';

/** Background cells in sensor XY, coloured by how often each was seen. */
export function backgroundCellSeries(data: PolarChartData | null): ScatterSeries[] {
	if (!data) return [];
	return [
		{
			key: 'cells',
			label: 'Background cell',
			points: colourByValue(data.points, data.max_value || undefined)
		}
	];
}

/** Coarse buckets at their mean range, coloured by mean times seen. */
export function backgroundBucketSeries(data: HeatmapBucketData | null): ScatterSeries[] {
	if (!data) return [];
	return [
		{
			key: 'buckets',
			label: 'Grid bucket',
			points: colourByValue(data.points, data.max_value || undefined)
		}
	];
}

/** The latest frame split into background and foreground points. */
export function foregroundSeries(data: ForegroundChartData | null): ScatterSeries[] {
	if (!data) return [];
	return [
		{
			key: 'background',
			label: 'Background',
			points: data.background_points,
			colour: BACKGROUND_POINT_COLOUR
		},
		{
			key: 'foreground',
			label: 'Foreground',
			points: data.foreground_points,
			colour: FOREGROUND_POINT_COLOUR
		}
	];
}

/** Recent cluster centroids, coloured by point count. */
export function clusterSeries(data: RecentClustersData | null): ScatterSeries[] {
	if (!data) return [];
	const points = data.clusters.map((c) => ({ x: c.x, y: c.y, value: c.points_count }));
	return [{ key: 'clusters', label: 'Cluster', points: colourByValue(points) }];
}

/** Active tracks at their current position, coloured by observation count. */
export function trackSeries(tracks: Track[]): ScatterSeries[] {
	const points = tracks.map((t) => ({
		x: t.position.x,
		y: t.position.y,
		value: t.observation_count
	}));
	return [{ key: 'tracks', label: 'Track', points: colourByValue(points) }];
}

/** Throughput as bars, in the order the traffic page drew them. */
export function trafficBars(data: TrafficMetrics | null): BarDatum[] {
	if (!data) return [];
	return [
		{ label: 'Packets/s', value: data.packets_per_sec },
		{ label: 'MB/s', value: data.mb_per_sec },
		{ label: 'Points/s', value: data.points_per_sec },
		{ label: 'Dropped (recent)', value: data.dropped_count }
	];
}
