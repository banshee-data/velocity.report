<script lang="ts">
	/**
	 * LiDAR Debug
	 *
	 * Plan views of the running pipeline: the background grid's settling, its
	 * cells and buckets, the latest foreground frame, recent clusters, active
	 * tracks and packet throughput. These replace the go-echarts debug pages;
	 * point-cloud inspection at full fidelity stays in the macOS visualiser.
	 */
	import { getActiveTracks } from '#lib/api.js';
	import CategoryBars from '#lib/components/charts/CategoryBars.svelte';
	import InlineSvgChart from '#lib/components/charts/InlineSvgChart.svelte';
	import XYScatter from '#lib/components/charts/XYScatter.svelte';
	import {
		DEFAULT_LIDAR_SENSOR_ID,
		getClustersChart,
		getForegroundChart,
		getHeatmapChart,
		getLidarStatus,
		getPolarChart,
		getTrafficChart,
		GRID_METRICS,
		gridChartPath,
		type ForegroundChartData,
		type GridMetric,
		type HeatmapBucketData,
		type PolarChartData,
		type RecentClustersData,
		type TrafficMetrics
	} from '#lib/lidarControl.js';
	import {
		backgroundBucketSeries,
		backgroundCellSeries,
		clusterSeries,
		foregroundSeries,
		trackSeries,
		trafficBars
	} from '#lib/lidarDebugCharts.js';
	import type { Track } from '#lib/types/lidar.js';
	import { onDestroy, onMount } from 'svelte';
	import { Button } from 'svelte-ux';

	const REFRESH_MS = 5000;

	let sensorId = $state(DEFAULT_LIDAR_SENSOR_ID);
	let gridMetric = $state<GridMetric>('settle_rate');
	// Bumped on every refresh so the grid SVG is fetched again.
	let refreshSerial = $state(0);
	let autoRefresh = $state(false);
	let loading = $state(false);
	let errors = $state<string[]>([]);

	let polar = $state<PolarChartData | null>(null);
	let buckets = $state<HeatmapBucketData | null>(null);
	let foreground = $state<ForegroundChartData | null>(null);
	let clusters = $state<RecentClustersData | null>(null);
	let tracks = $state<Track[]>([]);
	let traffic = $state<TrafficMetrics | null>(null);

	const gridUrl = $derived(`${gridChartPath(sensorId, gridMetric)}&_=${refreshSerial}`);

	let timer: ReturnType<typeof setInterval> | null = null;

	async function refresh() {
		loading = true;
		refreshSerial += 1;
		const failures: string[] = [];
		const settle = async <T,>(p: Promise<T>, apply: (v: T) => void) => {
			try {
				apply(await p);
			} catch (e) {
				failures.push(e instanceof Error ? e.message : String(e));
			}
		};
		await Promise.all([
			settle(getPolarChart(sensorId), (v) => (polar = v)),
			settle(getHeatmapChart(sensorId), (v) => (buckets = v)),
			settle(getForegroundChart(sensorId), (v) => (foreground = v)),
			settle(getClustersChart(sensorId), (v) => (clusters = v)),
			settle(getActiveTracks(sensorId), (v) => (tracks = v.tracks ?? [])),
			settle(getTrafficChart(), (v) => (traffic = v))
		]);
		errors = failures;
		loading = false;
	}

	function setAutoRefresh(on: boolean) {
		autoRefresh = on;
		if (timer) clearInterval(timer);
		timer = on ? setInterval(refresh, REFRESH_MS) : null;
	}

	onMount(async () => {
		try {
			sensorId = (await getLidarStatus()).sensor_id || DEFAULT_LIDAR_SENSOR_ID;
		} catch {
			// Keep the default sensor; the charts report their own errors.
		}
		await refresh();
	});

	onDestroy(() => {
		if (timer) clearInterval(timer);
	});
</script>

<svelte:head>
	<title>LiDAR Debug — velocity.report</title>
</svelte:head>

<main id="main-content" class="vr-page">
	<div class="vr-toolbar">
		<div class="flex items-center justify-between">
			<div>
				<h1 class="text-surface-content text-2xl font-semibold">LiDAR Debug</h1>
				<p class="text-surface-content/60 mt-1 text-sm">
					Plan views of the background grid, the latest frame, clusters and tracks for
					<span class="font-mono">{sensorId}</span>.
				</p>
			</div>
			<div class="flex items-center gap-3">
				<label class="flex items-center gap-2 text-sm">
					<input
						type="checkbox"
						checked={autoRefresh}
						onchange={(e) => setAutoRefresh(e.currentTarget.checked)}
					/>
					Refresh every {REFRESH_MS / 1000} s
				</label>
				<Button variant="outline" on:click={refresh} disabled={loading}>
					{loading ? 'Loading…' : 'Refresh'}
				</Button>
			</div>
		</div>
	</div>

	<div class="flex-1 overflow-y-auto p-6">
		<div class="vr-content-wide space-y-6">
			{#each errors as message (message)}
				<div class="rounded bg-red-50 px-4 py-2 text-sm text-red-600">{message}</div>
			{/each}

			<section class="vr-chart-card">
				<div class="mb-2 flex items-center justify-between">
					<h2 class="text-surface-content text-sm font-semibold">Background grid settling</h2>
					<select class="vr-input" bind:value={gridMetric} aria-label="Grid metric">
						{#each GRID_METRICS as m (m.value)}
							<option value={m.value}>{m.label}</option>
						{/each}
					</select>
				</div>
				<InlineSvgChart
					url={gridUrl}
					label="Background grid settling"
					minHeight={360}
					themeMode="dashboard"
				/>
			</section>

			<div class="grid gap-6 lg:grid-cols-2">
				<section class="vr-chart-card">
					<h2 class="text-surface-content text-sm font-semibold">Background cells</h2>
					<p class="text-surface-content/50 mb-2 text-xs">
						{polar
							? `${polar.num_points.toLocaleString()} cells, every ${polar.stride} shown, coloured by times seen`
							: ''}
					</p>
					<XYScatter
						series={backgroundCellSeries(polar)}
						valueLabel="Times seen"
						r={1.5}
						emptyMessage="No background cells yet."
					/>
				</section>

				<section class="vr-chart-card">
					<h2 class="text-surface-content text-sm font-semibold">Background buckets</h2>
					<p class="text-surface-content/50 mb-2 text-xs">
						{buckets
							? `${buckets.num_buckets.toLocaleString()} filled buckets at mean range, coloured by mean times seen`
							: ''}
					</p>
					<XYScatter
						series={backgroundBucketSeries(buckets)}
						valueLabel="Mean times seen"
						r={3}
						emptyMessage="No background buckets yet."
					/>
				</section>

				<section class="vr-chart-card">
					<h2 class="text-surface-content text-sm font-semibold">Foreground and background</h2>
					<p class="text-surface-content/50 mb-2 text-xs">
						{foreground
							? `${new Date(foreground.timestamp).toLocaleTimeString()} · ${foreground.foreground_count.toLocaleString()} foreground of ${foreground.total_points.toLocaleString()} points (${foreground.foreground_percent.toFixed(2)} %)`
							: ''}
					</p>
					<XYScatter
						series={foregroundSeries(foreground)}
						r={1.5}
						emptyMessage="No foreground snapshot yet."
					/>
				</section>

				<section class="vr-chart-card">
					<h2 class="text-surface-content text-sm font-semibold">Recent clusters</h2>
					<p class="text-surface-content/50 mb-2 text-xs">
						{clusters
							? `${clusters.num_clusters} clusters in the last hour, coloured by points`
							: ''}
					</p>
					<XYScatter
						series={clusterSeries(clusters)}
						valueLabel="Points"
						r={4}
						emptyMessage="No clusters in the last hour."
					/>
				</section>

				<section class="vr-chart-card">
					<h2 class="text-surface-content text-sm font-semibold">Active tracks</h2>
					<p class="text-surface-content/50 mb-2 text-xs">
						{tracks.length} tracks at their current position, coloured by observations
					</p>
					<XYScatter
						series={trackSeries(tracks)}
						valueLabel="Observations"
						r={4}
						emptyMessage="No active tracks."
					/>
				</section>

				<section class="vr-chart-card">
					<h2 class="text-surface-content text-sm font-semibold">Traffic</h2>
					<p class="text-surface-content/50 mb-2 text-xs">
						{traffic
							? `Latest packet statistics at ${new Date(traffic.timestamp).toLocaleTimeString()}`
							: ''}
					</p>
					<CategoryBars
						data={trafficBars(traffic)}
						valueLabel="Rate"
						labelWidth={120}
						emptyMessage="No packet statistics yet."
					/>
				</section>
			</div>
		</div>
	</div>
</main>
