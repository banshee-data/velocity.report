<script lang="ts">
	import { browser } from '$app/environment';
	import { isoDate } from '$lib/dateUtils';
	import RadarOverviewChart from '$lib/components/charts/RadarOverviewChart.svelte';
	import { PeriodType } from '@layerstack/utils';
	import { format } from 'date-fns';
	import { onMount } from 'svelte';
	import { Button, Card, DateRangeField, Header } from 'svelte-ux';
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
	let graphData: RadarStats[] = [];
	let selectedSource: string = 'radar_objects';
	const REPORT_SETTINGS_KEY = 'reportSettings';
	type StoredReportSettings = {
		dateRange?: {
			from?: string;
			to?: string;
			periodType?: PeriodType;
		};
		[key: string]: unknown;
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

	function weightedMedian(values: Array<{ value: number; weight: number }>): number {
		const filtered = values
			.filter(
				(item) => Number.isFinite(item.value) && Number.isFinite(item.weight) && item.weight > 0
			)
			.sort((a, b) => a.value - b.value);
		if (filtered.length === 0) return 0;

		const totalWeight = filtered.reduce((sum, item) => sum + item.weight, 0);
		const midpoint = totalWeight / 2;
		let cumulative = 0;
		for (const item of filtered) {
			cumulative += item.weight;
			if (cumulative >= midpoint) return item.value;
		}
		return filtered[filtered.length - 1].value;
	}

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
			if (dateChanged) {
				saveReportDateRangeSettings();
			}

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

	function loadReportDateRangeSettings() {
		if (!browser) return;
		try {
			const saved = localStorage.getItem(REPORT_SETTINGS_KEY);
			if (!saved) return;

			const settings = JSON.parse(saved) as StoredReportSettings;
			const from = settings?.dateRange?.from ? new Date(settings.dateRange.from) : null;
			const to = settings?.dateRange?.to ? new Date(settings.dateRange.to) : null;
			if (!from || !to || Number.isNaN(from.getTime()) || Number.isNaN(to.getTime())) return;

			dateRange = {
				from,
				to,
				periodType: settings?.dateRange?.periodType ?? PeriodType.Day
			};
		} catch (e) {
			console.warn('Failed to load report date range settings:', e);
		}
	}

	function saveReportDateRangeSettings() {
		if (!browser || !dateRange.from || !dateRange.to) return;
		try {
			let settings: StoredReportSettings = {};
			const saved = localStorage.getItem(REPORT_SETTINGS_KEY);
			if (saved) {
				const parsed = JSON.parse(saved) as StoredReportSettings;
				if (parsed && typeof parsed === 'object') {
					settings = parsed;
				}
			}

			settings.dateRange = {
				from: dateRange.from.toISOString(),
				to: dateRange.to.toISOString(),
				periodType: dateRange.periodType
			};

			localStorage.setItem(REPORT_SETTINGS_KEY, JSON.stringify(settings));
		} catch (e) {
			console.warn('Failed to save report date range settings:', e);
		}
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

			// Use a dedicated aggregate query (group=all) for the headline P98 so the
			// summary card and dashed reference line represent the true period aggregate.
			const aggregateResp = await getRadarStats(
				startUnix,
				endUnix,
				'all',
				units,
				$displayTimezone,
				selectedSource,
				selectedSiteId
			);
			const aggregateBucket = aggregateResp.metrics[0];
			if (aggregateBucket && Number.isFinite(Number(aggregateBucket.p98))) {
				p98Speed = Number(aggregateBucket.p98);
			} else {
				// Fallback for unexpected empty aggregate response.
				p98Speed = weightedMedian(
					stats.map((s) => ({
						value: Number(s.p98 || 0),
						weight: Math.max(0, Number(s.count || 0))
					}))
				);
			}
		} catch (e) {
			error = e instanceof Error && e.message ? e.message : 'Failed to load stats'; // eslint-disable-line svelte/infinite-reactive-loop
		}
	}

	// load chart data for the selected date range and group
	async function loadChart() {
		if (!dateRange.from || !dateRange.to) {
			graphData = [];
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

		// normalize data types so chart math never receives strings/invalid values
		graphData = validRows
			.map((row) => {
				const dt = row.date instanceof Date ? row.date : new Date(row.date);
				return {
					...row,
					date: dt,
					count: Number(row.count || 0),
					p50: Number(row.p50 || 0),
					p85: Number(row.p85 || 0),
					p98: Number(row.p98 || 0),
					max: Number(row.max || 0)
				} as RadarStats;
			})
			.filter((row) => row.date instanceof Date && !Number.isNaN(row.date.getTime()))
			.sort((a, b) => a.date.getTime() - b.date.getTime());
	}

	async function loadData() {
		loading = true;
		error = '';
		try {
			await loadConfig();
			await loadSites();
			loadReportDateRangeSettings();
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
		<div class="flex flex-wrap items-end gap-3">
			<div class="w-70">
				<DateRangeField bind:value={dateRange} periodTypes={[PeriodType.Day]} stepper />
			</div>
			<div class="w-24">
				<label for="dashboard-group" class="text-surface-content/70 mb-1 block text-xs font-medium">
					Group
				</label>
				<select
					id="dashboard-group"
					bind:value={group}
					class="border-surface-300 bg-surface-100 w-full rounded border px-2 py-2 text-sm"
				>
					{#each groupOptions as option (option)}
						<option value={option}>{option}</option>
					{/each}
				</select>
			</div>
			<div class="w-24">
				<DataSourceSelector bind:value={selectedSource} />
			</div>
			<div class="w-38">
				<label for="dashboard-site" class="text-surface-content/70 mb-1 block text-xs font-medium">
					Site
				</label>
				<select
					id="dashboard-site"
					bind:value={selectedSiteId}
					class="border-surface-300 bg-surface-100 w-full rounded border px-2 py-2 text-sm"
				>
					{#if siteOptions.length === 0}
						<option value={null}>No sites</option>
					{:else}
						{#each siteOptions as option (option.value)}
							<option value={option.value}>{option.label}</option>
						{/each}
					{/if}
				</select>
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

		<div
			class="grid grid-cols-1 gap-4 md:grid-cols-2"
			role="region"
			aria-label="Traffic statistics summary"
		>
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
		</div>

		{#if cosineCorrectionLabel}
			<p class="text-surface-600-300-token text-xs">
				Corrected for cosine error angle{cosineCorrectionAngles.length > 1 ? 's' : ''}:
				{cosineCorrectionLabel}
			</p>
		{/if}

		{#if graphData.length > 0}
			<RadarOverviewChart
				data={graphData}
				{group}
				speedUnits={getUnitLabel($displayUnits)}
				p98Reference={p98Speed}
			/>

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
