<script lang="ts">
	import { browser } from '$app/environment';
	import { isoDate } from '$lib/dateUtils';
	import { PeriodType } from '@layerstack/utils';
	import { scaleOrdinal, scaleTime } from 'd3-scale';
	import { format } from 'date-fns';
	import { Axis, Chart, Highlight, Spline, Svg, Text } from 'layerchart';
	import { onMount } from 'svelte';
	import { Button, Card, DateRangeField, Grid, Header, SelectField } from 'svelte-ux';
	import {
		generateReport,
		getConfig,
		getRadarStats,
		getReport,
		getSites,
		type Config,
		type RadarStats,
		type RadarStatsResponse,
		type Site,
		type SiteReport
	} from '../lib/api';
	import DataSourceSelector from '../lib/components/DataSourceSelector.svelte';
	import { displayTimezone, initializeTimezone } from '../lib/stores/timezone';
	import { displayUnits, initializeUnits } from '../lib/stores/units';
	import { getUnitLabel, type Unit } from '../lib/units';

	let stats: RadarStats[] = [];
	let config: Config = { units: 'mph', timezone: 'UTC' }; // default
	let totalCount = 0;
	let p98Speed = 0;
	let loading = true;
	let error = '';

	// Site management
	let sites: Site[] = [];
	let selectedSiteId: number | null = null;
	let siteOptions: Array<{ value: number; label: string }> = [];

	// default DateRangeField to the last 14 days (inclusive)
	const today = new Date();
	const fromDefault = new Date(today); // eslint-disable-line svelte/prefer-svelte-reactivity
	fromDefault.setDate(today.getDate() - 13); // last 14 days inclusive
	let dateRange = { from: fromDefault, to: today, periodType: PeriodType.Day };
	let group: string = '4h';
	let chartData: Array<{ date: Date; metric: string; value: number }> = [];
	let graphData: RadarStats[] = [];
	let selectedSource: string = 'radar_objects';

	// color map mirrors the cDomain/cRange used by the chart so we don't need
	// to capture cScale via a `let:` slot (which conflicts with internal
	// components that use `let:` themselves and triggers Svelte's
	// invalid_default_snippet error).
	const colorMap: Record<string, string> = {
		p50: '#ece111',
		p85: '#ed7648',
		p98: '#d50734',
		max: '#000000'
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
	let lastSiteId: number | null = null;
	let initialized = false;
	// Cache the last raw stats response
	let lastStatsRaw: RadarStatsResponse | null = null;
	let cosineCorrectionAngles: number[] = [];
	let cosineCorrectionLabel = '';
	let lastStatsRequestKey = '';

	$: cosineCorrectionLabel =
		cosineCorrectionAngles.length > 0
			? cosineCorrectionAngles.map((angle) => `${angle}°`).join(', ')
			: '';

	$: if (initialized && browser && dateRange.from && dateRange.to) {
		const from = dateRange.from.getTime();
		const to = dateRange.to.getTime();

		const dateChanged = from !== lastFrom || to !== lastTo;
		const unitsChanged = $displayUnits !== lastUnits;
		const groupChanged = group !== lastGroup;
		const sourceChanged = selectedSource !== lastSource;
		const siteChanged = selectedSiteId !== lastSiteId;

		// Full reload when date range, display units, or source changes
		if (dateChanged || unitsChanged || sourceChanged || siteChanged) {
			lastFrom = from;
			lastTo = to;
			lastUnits = $displayUnits;
			lastGroup = group;
			lastSource = selectedSource;
			lastSiteId = selectedSiteId;

			loading = true;
			// run loadStats first so it can populate the cache, then run loadChart which will reuse it
			loadStats($displayUnits) // eslint-disable-line svelte/infinite-reactive-loop
				.then(() => loadChart())
				.catch((e) => {
					error = e instanceof Error && e.message ? e.message : String(e); // eslint-disable-line svelte/infinite-reactive-loop
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

	async function loadSites() {
		try {
			sites = await getSites();
			siteOptions = sites.map((site) => ({ value: site.id, label: site.name }));

			// Load selected site from localStorage or default to first site
			if (browser) {
				const savedSiteId = localStorage.getItem('selectedSiteId');
				if (savedSiteId) {
					const siteId = parseInt(savedSiteId, 10);
					if (sites.some((s) => s.id === siteId)) {
						selectedSiteId = siteId;
					}
				}
				// If no saved site or invalid, default to first site
				if (selectedSiteId === null && sites.length > 0) {
					selectedSiteId = sites[0].id;
				}
			}
		} catch (e) {
			console.error('Failed to load sites:', e);
			// Don't set error here, sites are optional for viewing stats
		}
	}

	// Save selected site to localStorage when it changes
	$: if (browser && selectedSiteId != null) {
		localStorage.setItem('selectedSiteId', selectedSiteId.toString());
	}

	async function loadStats(units: Unit) {
		try {
			if (!dateRange.from || !dateRange.to) {
				stats = [];
				totalCount = 0;
				p98Speed = 0;
				cosineCorrectionAngles = [];
				return;
			}
			const startUnix = Math.floor(dateRange.from.getTime() / 1000);
			const endUnix = Math.floor(dateRange.to.getTime() / 1000);
			const statsResp = await getRadarStats(
				startUnix,
				endUnix,
				group,
				units,
				$displayTimezone,
				selectedSource,
				selectedSiteId
			);
			// cache raw response so loadChart can reuse it instead of making a second request
			lastStatsRaw = statsResp;
			lastStatsRequestKey = `${startUnix}|${endUnix}|${group}|${units}|${$displayTimezone}|${selectedSource}|${selectedSiteId ?? 'all'}`;
			if (browser) console.debug('[dashboard] fetch stats timezone ->', $displayTimezone);
			stats = statsResp.metrics;
			cosineCorrectionAngles = statsResp.cosineCorrection?.angles ?? [];
			totalCount = stats.reduce((sum, s) => sum + (s.count || 0), 0);
			// Show P98 speed (aggregate percentile) in the summary card
			p98Speed = stats.length > 0 ? Math.max(...stats.map((s) => s.p98 || 0)) : 0;
		} catch (e) {
			error = e instanceof Error && e.message ? e.message : 'Failed to load stats'; // eslint-disable-line svelte/infinite-reactive-loop
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
		const requestKey = `${startUnix}|${endUnix}|${group}|${units}|${$displayTimezone}|${selectedSource}|${selectedSiteId ?? 'all'}`;

		let arr: RadarStats[];
		if (lastStatsRaw && requestKey === lastStatsRequestKey) {
			// reuse cached stats response (it may be the root object)
			const cached = lastStatsRaw as RadarStatsResponse;
			arr = Array.isArray(cached) ? cached : cached.metrics || [];
			if (browser) console.debug('[dashboard] reusing cached stats for chart');
		} else {
			// fetch via the shared API helper so we use the same code path as loadStats
			if (browser)
				console.debug('[dashboard] chart fetching via getRadarStats ->', $displayTimezone);
			const resp = await getRadarStats(
				startUnix,
				endUnix,
				group,
				units,
				$displayTimezone,
				selectedSource,
				selectedSiteId
			);
			arr = resp.metrics;
			// cache the response for potential reuse
			lastStatsRaw = resp;
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
			await loadSites();
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

	// Report generation
	let generatingReport = false;
	let reportMessage = '';
	let lastGeneratedReportId: number | null = null;
	let reportMetadata: SiteReport | null = null;

	async function handleGenerateReport() {
		if (!dateRange.from || !dateRange.to) {
			reportMessage = 'Please select a date range first';
			return;
		}

		if (selectedSiteId == null) {
			reportMessage = 'Please select a site first';
			return;
		}

		generatingReport = true;
		reportMessage = '';
		lastGeneratedReportId = null;
		reportMetadata = null;

		try {
			// Generate report and get report ID
			const response = await generateReport({
				start_date: isoDate(dateRange.from),
				end_date: isoDate(dateRange.to),
				timezone: $displayTimezone,
				units: $displayUnits,
				group: group,
				source: selectedSource,
				histogram: true,
				hist_bucket_size: 5.0,
				site_id: selectedSiteId
			});

			lastGeneratedReportId = response.report_id;

			// Fetch report metadata to get filenames
			reportMetadata = await getReport(response.report_id);

			reportMessage = `Report generated successfully! Use the links below to download.`;
		} catch (e) {
			reportMessage = e instanceof Error ? e.message : 'Failed to generate report';
		} finally {
			generatingReport = false;
		}
	}
</script>

<svelte:head>
	<title>Dashboard 🚴 velocity.report</title>
	<meta name="description" content="Real-time vehicle traffic statistics and speed analytics" />
</svelte:head>

<main id="main-content" class="space-y-6 p-4">
	<Header title="Dashboard" subheading="Vehicle traffic statistics and analytics" />

	{#if loading}
		<div role="status" aria-live="polite" aria-busy="true">
			<p>Loading stats…</p>
			<span class="sr-only">Please wait while we fetch your traffic data</span>
		</div>
	{:else if error}
		<div role="alert" aria-live="assertive" class="text-red-600">
			<strong>Error:</strong>
			{error}
		</div>
	{:else}
		<div class="flex flex-wrap items-end gap-2">
			<div class="w-70">
				<DateRangeField bind:value={dateRange} periodTypes={[PeriodType.Day]} stepper />
			</div>
			<div class="w-24">
				<SelectField bind:value={group} label="Group" {options} clearable={false} />
			</div>
			<div class="w-24">
				<DataSourceSelector bind:value={selectedSource} />
			</div>
			<div class="w-38">
				<SelectField
					bind:value={selectedSiteId}
					label="Site"
					options={siteOptions}
					clearable={false}
				/>
			</div>
			<div class="w-18">
				<Button
					on:click={handleGenerateReport}
					disabled={generatingReport || selectedSiteId == null}
					variant="fill"
					color="primary"
					class="whitespace-normal"
					aria-label={generatingReport ? 'Generating report, please wait' : 'Generate report'}
				>
					{generatingReport ? 'Generating...' : 'Generate Report'}
				</Button>
			</div>
		</div>

		{#if reportMessage}
			<div
				role={reportMessage.includes('success') ? 'status' : 'alert'}
				aria-live="polite"
				class="rounded border p-3 {reportMessage.includes('success')
					? 'border-green-300 bg-green-50 text-green-800'
					: 'border-red-300 bg-red-50 text-red-800'}"
			>
				{reportMessage}
			</div>
		{/if}

		{#if lastGeneratedReportId !== null}
			<div class="card space-y-3 p-4" role="region" aria-label="Report download options">
				<h3 class="text-base font-semibold">Report Ready</h3>
				{#if reportMetadata}
					<div class="flex gap-2">
						<!-- eslint-disable svelte/no-navigation-without-resolve -->
						<a
							href={`/api/reports/${lastGeneratedReportId}/download/${reportMetadata.filename}`}
							class="bg-secondary-500 hover:bg-secondary-600 inline-flex items-center justify-center rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
							download
							aria-label="Download PDF report"
						>
							📄 Download PDF
						</a>
						{#if reportMetadata.zip_filename}
							<!-- eslint-disable svelte/no-navigation-without-resolve -->
							<a
								href={`/api/reports/${lastGeneratedReportId}/download/${reportMetadata.zip_filename}`}
								class="border-secondary-500 text-secondary-500 hover:bg-secondary-50 inline-flex items-center justify-center rounded-md border px-4 py-2 text-sm font-medium transition-colors hover:text-white"
								download
								aria-label="Download source files as ZIP archive"
							>
								📦 Download Sources (ZIP)
							</a>
						{/if}
					</div>
				{:else}
					<p class="text-surface-600-300-token text-sm" role="status" aria-live="polite">
						Loading download links...
					</p>
				{/if}
				<p class="text-surface-600-300-token text-xs">
					The ZIP file contains LaTeX source files and chart PDFs for custom editing
				</p>
			</div>
		{/if}

		<Grid autoColumns="14em" gap={8} role="region" aria-label="Traffic statistics summary">
			<Card title="Vehicle Count" role="article">
				<div class="pt-0 pr-4 pb-4 pl-4">
					<p class="text-3xl font-bold text-blue-600" aria-label="Total vehicle count">
						{totalCount}
					</p>
				</div>
			</Card>

			<Card title="P98 Speed" role="article">
				<div class="pt-0 pr-4 pb-4 pl-4">
					<p class="text-3xl font-bold text-green-600" aria-label="98th percentile speed">
						{p98Speed.toFixed(1)}
						{getUnitLabel($displayUnits)}
					</p>
				</div>
			</Card>
		</Grid>

		{#if cosineCorrectionLabel}
			<p class="text-surface-600-300-token text-xs">
				Corrected for cosine error angle{cosineCorrectionAngles.length > 1 ? 's' : ''}:
				{cosineCorrectionLabel}
			</p>
		{/if}

		{#if chartData.length > 0}
			<!-- @TODO the chart needs:
				* go to zero when no data
				* hour on the x-axis when zoomed in (Timezone aligned)
				* Tooltip for multiple metrics
				-->
			<div
				class="mb-4 h-[300px] rounded border p-4"
				role="img"
				aria-label="Speed distribution over time showing P50, P85, P98, and maximum speeds for the selected date range"
			>
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
						{#each ['p50', 'p85', 'p98', 'max'] as metric (metric)}
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

			<!-- Accessible data table fallback -->
			<details class="rounded border p-4">
				<summary class="cursor-pointer text-sm font-medium">View data table</summary>
				<div class="mt-4 overflow-x-auto">
					<table class="w-full text-sm">
						<caption class="sr-only">
							Speed statistics over time showing P50, P85, P98, and maximum values
						</caption>
						<thead>
							<tr class="border-b">
								<th scope="col" class="px-2 py-2 text-left">Time</th>
								<th scope="col" class="px-2 py-2 text-right">Count</th>
								<th scope="col" class="px-2 py-2 text-right">P50</th>
								<th scope="col" class="px-2 py-2 text-right">P85</th>
								<th scope="col" class="px-2 py-2 text-right">P98</th>
								<th scope="col" class="px-2 py-2 text-right">Max</th>
							</tr>
						</thead>
						<tbody>
							{#each graphData as row (row.date.getTime())}
								<tr class="border-b">
									<td class="px-2 py-2">{format(row.date, 'MMM d HH:mm')}</td>
									<td class="px-2 py-2 text-right">{row.count}</td>
									<td class="px-2 py-2 text-right">{row.p50.toFixed(1)}</td>
									<td class="px-2 py-2 text-right">{row.p85.toFixed(1)}</td>
									<td class="px-2 py-2 text-right">{row.p98.toFixed(1)}</td>
									<td class="px-2 py-2 text-right">{row.max.toFixed(1)}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			</details>
		{/if}
	{/if}
</main>
