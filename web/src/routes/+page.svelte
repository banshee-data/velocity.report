<script lang="ts">
	import { browser } from '$app/environment';
	import { PeriodType } from '@layerstack/utils';
	import { scaleOrdinal, scaleTime } from 'd3-scale';
	import { format } from 'date-fns';
	import { Axis, Chart, Highlight, Spline, Svg, Text } from 'layerchart';
	import { onMount } from 'svelte';
	import {
		Card,
		DateRangeField,
		Grid,
		Header,
		SelectField,
		ToggleGroup,
		ToggleOption
	} from 'svelte-ux';
	import { getConfig, getRadarStats, type Config, type RadarStats } from '../lib/api';
	import { displayTimezone, initializeTimezone } from '../lib/stores/timezone';
	import { displayUnits, initializeUnits } from '../lib/stores/units';
	import { getUnitLabel, type Unit } from '../lib/units';

	let stats: RadarStats[] = [];
	let config: Config = { units: 'mph', timezone: 'UTC' }; // default
	let totalCount = 0;
	let p98Speed = 0;
	let loading = true;
	let error = '';
	// default DateRangeField to the last 14 days (inclusive)
	function isoDate(d: Date) {
		return d.toISOString().slice(0, 10);
	}
	const today = new Date();
	const fromDefault = new Date(today);
	fromDefault.setDate(today.getDate() - 13); // last 14 days inclusive
	let dateRange = { from: fromDefault, to: today, periodType: PeriodType.Day };
	let group: string = '4h';
	let chartData: Array<{ date: Date; metric: string; value: number }> = [];
	let graphData: RadarStats[] = [];
	const barSeries = [
		{ key: 'count', label: 'Count', value: (d: RadarStats) => d.count, color: '#16a34a' },
		{ key: 'p50', label: 'p50', value: (d: RadarStats) => d.p50, color: '#2563eb' },
		{ key: 'p85', label: 'p85', value: (d: RadarStats) => d.p85, color: '#16a34a' },
		{ key: 'p98', label: 'p98', value: (d: RadarStats) => d.p98, color: '#f59e0b' }
	];
	let selectedSource: string = 'radar_objects';

	// color map mirrors the cDomain/cRange used by the chart so we don't need
	// to capture cScale via a `let:` slot (which conflicts with internal
	// components that use `let:` themselves and triggers Svelte's
	// invalid_default_snippet error).
	const colorMap: Record<string, string> = {
		p50: '#2563eb',
		p85: '#16a34a',
		p98: '#f59e0b',
		max: '#ef4444'
	};

	const groupOptions = [
		'1h',
		'2h',
		'3h',
		'4h',
		'6h',
		'8h',
		'12h',
		'24h',
		'2d',
		'3d',
		'7d',
		'14d',
		'28d'
	];
	const options = groupOptions.map((o) => ({ value: o, label: o }));

	// Reload behavior: react to dateRange, units, group, or source changes.
	// - If dateRange, units, or source changed -> reload both stats and chart.
	// - If only group changed -> reload only the chart for faster response.
	let lastFrom: number = 0;
	let lastTo: number = 0;
	let lastUnits: Unit | undefined = undefined;
	let lastGroup = '';
	let lastSource = '';
	let initialized = false;
	// Cache the last raw stats response to avoid duplicate network calls
	let lastStatsRaw: any[] | null = null;
	let lastStatsRequestKey = '';

	$: if (initialized && browser && dateRange.from && dateRange.to) {
		const from = dateRange.from.getTime();
		const to = dateRange.to.getTime();

		const dateChanged = from !== lastFrom || to !== lastTo;
		const unitsChanged = $displayUnits !== lastUnits;
		const groupChanged = group !== lastGroup;
		const sourceChanged = selectedSource !== lastSource;

		// Full reload when date range, display units, or source changes
		if (dateChanged || unitsChanged || sourceChanged) {
			lastFrom = from;
			lastTo = to;
			lastUnits = $displayUnits;
			lastGroup = group;
			lastSource = selectedSource;

			loading = true;
			loading = true;
			// run loadStats first so it can populate the cache, then run loadChart which will reuse it
			loadStats($displayUnits)
				.then(() => loadChart())
				.catch((e) => {
					error = e instanceof Error && e.message ? e.message : String(e);
				})
				.finally(() => {
					loading = false;
				});
			// done
		} else if (groupChanged) {
			// Only grouping changed -> refresh chart only
			lastGroup = group;
			loadChart();
		}
	}

	async function loadConfig() {
		try {
			config = await getConfig();
			initializeUnits(config.units);
			// initialize the timezone store as well so the dashboard uses stored timezone
			initializeTimezone(config.timezone);
			if (browser) console.debug('[dashboard] initialized timezone store ->', $displayTimezone);
		} catch (e) {
			error = e instanceof Error && e.message ? e.message : 'Failed to load config';
		}
	}

	async function loadStats(units: Unit) {
		try {
			if (!dateRange.from || !dateRange.to) {
				stats = [];
				totalCount = 0;
				p98Speed = 0;
				return;
			}
			const startUnix = Math.floor(dateRange.from.getTime() / 1000);
			const endUnix = Math.floor(dateRange.to.getTime() / 1000);
			const statsData = await getRadarStats(
				startUnix,
				endUnix,
				group,
				units,
				$displayTimezone,
				selectedSource
			);
			// cache raw response so loadChart can reuse it instead of making a second request
			lastStatsRaw = statsData as any[];
			lastStatsRequestKey = `${startUnix}|${endUnix}|${group}|${units}|${$displayTimezone}|${selectedSource}`;
			if (browser) console.debug('[dashboard] fetch stats timezone ->', $displayTimezone);
			stats = statsData;
			totalCount = stats.reduce((sum, s) => sum + (s.count || 0), 0);
			// Show P98 speed (aggregate percentile) in the summary card
			p98Speed = stats.length > 0 ? Math.max(...stats.map((s) => s.p98 || 0)) : 0;
		} catch (e) {
			error = e instanceof Error && e.message ? e.message : 'Failed to load stats';
		}
	}

	// load chart data for the selected date range and group
	async function loadChart() {
		if (!dateRange.from || !dateRange.to) {
			chartData = [];
			return;
		}
		const startUnix = Math.floor(dateRange.from.getTime() / 1000);
		const endUnix = Math.floor(dateRange.to.getTime() / 1000);
		const units = $displayUnits;
		const requestKey = `${startUnix}|${endUnix}|${group}|${units}|${$displayTimezone}|${selectedSource}`;

		let arr: RadarStats[];
		if (lastStatsRaw && requestKey === lastStatsRequestKey) {
			// reuse cached stats response
			arr = lastStatsRaw;
			if (browser) console.debug('[dashboard] reusing cached stats for chart');
		} else {
			// fetch via the shared API helper so we use the same code path as loadStats
			if (browser)
				console.debug('[dashboard] chart fetching via getRadarStats ->', $displayTimezone);
			arr = await getRadarStats(startUnix, endUnix, group, units, $displayTimezone, selectedSource);
			// cache the response for potential reuse
			lastStatsRaw = arr;
			lastStatsRequestKey = requestKey;
		}

		// filter out rows with missing/invalid date to avoid scale errors
		const validRows = arr.filter((r) => {
			const dt = r.date instanceof Date ? r.date : new Date(r.date);
			return dt && !isNaN(dt.getTime());
		});
		if (validRows.length !== arr.length && browser) {
			console.debug('[dashboard] dropped rows with invalid date', arr.length - validRows.length);
		}

		// prepare graphData for BarChart/LineChart (single-point-per-x series)
		// the API already returns RadarStats shape (date,count,p50,p85,p98,max) so pass through
		graphData = validRows as RadarStats[];

		// transform to multi-series flat data: for each valid row create points for p50, p85, p98, max
		const rows: Array<{ date: Date; metric: string; value: number }> = [];
		for (const r of validRows) {
			const dt = r.date instanceof Date ? (r.date as Date) : new Date(r.date);
			rows.push({ date: dt, metric: 'p50', value: r.p50 || 0 });
			rows.push({ date: dt, metric: 'p85', value: r.p85 || 0 });
			rows.push({ date: dt, metric: 'p98', value: r.p98 || 0 });
			rows.push({ date: dt, metric: 'max', value: r.max || 0 });
		}
		chartData = rows;

		// debug: log a sample to inspect types/values used by BarChart/legend
		// if (browser) console.debug('[dashboard] graphData sample ->', graphData.slice(0, 3));
	}

	async function loadData() {
		loading = true;
		error = '';
		try {
			await loadConfig();
			// establish last-known values so the reactive watcher doesn't think things changed
			lastFrom = dateRange.from.getTime();
			lastTo = dateRange.to.getTime();
			lastUnits = $displayUnits;
			lastGroup = group;
			lastSource = selectedSource;
			await loadStats($displayUnits);
			// populate the chart for the default date range
			await loadChart();
		} finally {
			loading = false;
			// mark initialization complete so the reactive watcher can start firing
			initialized = true;
		}
	}

	onMount(loadData);
</script>

<svelte:head>
	<title>Dashboard 🚴 velocity.report</title>
</svelte:head>

<main class="space-y-6 p-4">
	<Header title="Dashboard" subheading="Vehicle traffic statistics and analytics" />

	{#if loading}
		<p>Loading stats…</p>
	{:else if error}
		<p class="text-red-600">{error}</p>
	{:else}
		<div class="flex items-end gap-2">
			<div class="w-74">
				<DateRangeField bind:value={dateRange} periodTypes={[PeriodType.Day]} stepper />
			</div>
			<div class="w-24">
				<SelectField bind:value={group} label="Group" {options} clearable={false} />
			</div>
			<div class="w-24">
				<ToggleGroup bind:value={selectedSource} vertical inset>
					<ToggleOption value="radar_objects">Objects</ToggleOption>
					<ToggleOption value="radar_data_transits">Transits</ToggleOption>
				</ToggleGroup>
			</div>
		</div>

		<Grid autoColumns="14em" gap={8}>
			<Card title="Vehicle Count">
				<div class="pb-4 pl-4 pr-4 pt-0">
					<p class="text-3xl font-bold text-blue-600">{totalCount}</p>
				</div>
			</Card>

			<Card title="P98 Speed">
				<div class="pb-4 pl-4 pr-4 pt-0">
					<p class="text-3xl font-bold text-green-600">
						{p98Speed.toFixed(1)}
						{getUnitLabel($displayUnits)}
					</p>
				</div>
			</Card>
		</Grid>

		{#if chartData.length > 0}
			<!-- @TODO the chart needs:
				* go to zero when no data
				* hour on the x-axis when zoomed in (Timezone aligned)
				* Tooltip for multiple metrics
				-->
			<div class="mb-4 h-[300px] rounded border p-4">
				<Chart
					data={chartData}
					x="date"
					xScale={scaleTime()}
					y="value"
					yDomain={[0, null]}
					yNice
					c="metric"
					cScale={scaleOrdinal()}
					cDomain={['p50', 'p85', 'p98', 'max']}
					cRange={['#2563eb', '#16a34a', '#f59e0b', '#ef4444']}
					padding={{ left: 16, bottom: 24, right: 8 }}
					tooltip={{ mode: 'voronoi' }}
				>
					<Svg>
						<Axis placement="left" grid rule />
						<Axis
							placement="bottom"
							format={(d) => `${format(d, 'MMM d')}\n${format(d, 'HH:mm')}`}
							rule
							tickSpacing={100}
							tickMultiline
						/>
						{#each ['p50', 'p85', 'p98', 'max'] as metric}
							{@const data = chartData.filter((p) => p.metric === metric)}
							{@const color = colorMap[metric]}
							<Spline {data} class="stroke-2" stroke={color}>
								<circle r={4} fill={color} />
								<Text
									value={metric}
									verticalAnchor="middle"
									dx={6}
									dy={-2}
									class="text-xs"
									fill={color}
								/>
							</Spline>
						{/each}
						<Highlight points lines />
					</Svg>
				</Chart>
			</div>
		{/if}
	{/if}
</main>
