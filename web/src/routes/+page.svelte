<script lang="ts">
	import { scaleOrdinal, scaleTime } from 'd3-scale';
	import { format } from 'date-fns';
	import { Axis, Chart, Highlight, Spline, Svg, Text, Tooltip } from 'layerchart';
	import { onMount } from 'svelte';
	import { Card, DateRangeField, Grid, Header, SelectField } from 'svelte-ux';
	import { getConfig, getRadarStats, type Config, type RadarStats } from '../lib/api';
	import { displayUnits, initializeUnits } from '../lib/stores/units';
	import { getUnitLabel, type Unit } from '../lib/units';

	let stats: RadarStats[] = [];
	let config: Config = { units: 'mph', timezone: 'UTC' }; // default
	let totalCount = 0;
	let maxSpeed = 0;
	let loading = true;
	let error = '';
	// default DateRangeField to the last 14 days (inclusive)
	function isoDate(d: Date) {
		return d.toISOString().slice(0, 10);
	}
	const today = new Date();
	const startDefault = new Date(today);
	startDefault.setDate(today.getDate() - 13); // last 14 days inclusive
	let range: { start?: string | null; end?: string | null } = {
		start: isoDate(startDefault),
		end: isoDate(today)
	};
	let group: string = '24h';
	let chartData: Array<{ date: Date; metric: string; value: number }> = [];

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

	// Reactive statement to reload data when units change
	$: if ($displayUnits && !loading) {
		loadStats($displayUnits);
	}

	async function loadConfig() {
		try {
			config = await getConfig();
			initializeUnits(config.units);
		} catch (e) {
			error = e instanceof Error && e.message ? e.message : 'Failed to load config';
		}
	}

	async function loadStats(units: Unit) {
		try {
			if (!range?.start || !range?.end) {
				stats = [];
				totalCount = 0;
				maxSpeed = 0;
				return;
			}
			const startUnix = Math.floor(new Date(range.start).getTime() / 1000);
			const endUnix = Math.floor(new Date(range.end).getTime() / 1000);
			const statsData = await getRadarStats(startUnix, endUnix, group, units);
			stats = statsData;
			totalCount = stats.reduce((sum, s) => sum + (s.Count || 0), 0);
			maxSpeed = stats.length > 0 ? Math.max(...stats.map((s) => s.MaxSpeed || 0)) : 0;
		} catch (e) {
			error = e instanceof Error && e.message ? e.message : 'Failed to load stats';
		}
	}

	// load chart data for the selected date range and group
	async function loadChart() {
		if (!range?.start || !range?.end) {
			chartData = [];
			return;
		}
		const startUnix = Math.floor(new Date(range.start).getTime() / 1000);
		const endUnix = Math.floor(new Date(range.end).getTime() / 1000);
		const units = $displayUnits;
		const raw = await fetch(
			`/api/radar_stats?start=${startUnix}&end=${endUnix}&group=${group}&units=${units}`
		);
		if (!raw.ok) {
			error = `Failed to load chart data: ${raw.statusText}`;
			return;
		}
		const arr = await raw.json();

		// transform to multi-series flat data: for each row create points for p50, p85, p98, max
		const rows: Array<{ date: Date; metric: string; value: number }> = [];
		for (const r of arr) {
			const d = new Date(r.StartTime);
			rows.push({ date: d, metric: 'p50', value: r.P50Speed || 0 });
			rows.push({ date: d, metric: 'p85', value: r.P85Speed || 0 });
			rows.push({ date: d, metric: 'p98', value: r.P98Speed || 0 });
			rows.push({ date: d, metric: 'max', value: r.MaxSpeed || 0 });
		}
		chartData = rows;
	}

	async function loadData() {
		loading = true;
		error = '';
		try {
			await loadConfig();
			await loadStats($displayUnits);
			// populate the chart for the default date range
			await loadChart();
		} finally {
			loading = false;
		}
	}

	onMount(loadData);
</script>

<svelte:head>
	<title>Dashboard 🚴 velocity.report</title>
</svelte:head>

<main class="space-y-6 p-4">
	<Header
		title="Dashboard"
		subheading="Vehicle traffic statistics and analytics over the past 14 days"
	/>

	{#if loading}
		<p>Loading stats…</p>
	{:else if error}
		<p class="text-red-600">{error}</p>
	{:else}
		<div class="flex items-end gap-4">
			<div class="w-[360px]">
				<DateRangeField bind:value={range} on:change={() => loadChart()} />
			</div>
			<div class="w-48">
				<SelectField bind:value={group} on:change={loadChart} label="Group" {options} />
			</div>
		</div>

		<Grid autoColumns="18em" gap={8}>
			<Card title="Vehicle Count">
				<div class="pb-4 pl-4 pr-4 pt-0">
					<p class="text-3xl font-bold text-blue-600">{totalCount}</p>
					<p class="text-surface-content/70 text-sm">vehicles detected</p>
				</div>
			</Card>

			<Card title="Max Speed">
				<div class="pb-4 pl-4 pr-4 pt-0">
					<p class="text-3xl font-bold text-green-600">
						{maxSpeed.toFixed(1)}
						{getUnitLabel($displayUnits)}
					</p>
					<p class="text-surface-content/70 text-sm">last 14 days</p>
				</div>
			</Card>
		</Grid>

		{#if chartData.length > 0}
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
					padding={{ left: 16, bottom: 24, right: 48 }}
					tooltip={{ mode: 'voronoi' }}
				>
					<Svg>
						<Axis placement="left" grid rule />
						<Axis placement="bottom" format={(d) => format(d, 'MMM d')} rule />
						{#each ['p50', 'p85', 'p98', 'max'] as metric}
							{@const data = chartData.filter((p) => p.metric === metric)}
							{@const color = colorMap[metric]}
							<Spline {data} class="stroke-2" stroke={color}>
								<svelte:fragment slot="end">
									<circle r={4} fill={color} />
									<Text
										value={metric}
										verticalAnchor="middle"
										dx={6}
										dy={-2}
										class="text-xs"
										fill={color}
									/>
								</svelte:fragment>
							</Spline>
						{/each}
						<Highlight points lines />
					</Svg>

					<Tooltip.Root>
						<svelte:fragment slot="children" let:data>
							<Tooltip.Header>{format(data.date, 'eee, MMMM do')}</Tooltip.Header>
							<Tooltip.List>
								<Tooltip.Item label={data.metric} value={data.value} />
							</Tooltip.List>
						</svelte:fragment>
					</Tooltip.Root>
				</Chart>
			</div>
		{/if}
	{/if}
</main>
