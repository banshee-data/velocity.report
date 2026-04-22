<script lang="ts">
	import { browser } from '$app/environment';
	import { isoDate } from '$lib/dateUtils';
	import {
		isDateRangeStale,
		REPORT_SETTINGS_KEY,
		type StoredReportSettings
	} from '$lib/reportSettings';
	import { PeriodType } from '@layerstack/utils';
	import { format } from 'date-fns';
	import { onMount } from 'svelte';
	import { Button, Card, DateRangeField, Header } from 'svelte-ux';
	import {
		buildTimeSeriesChartPath,
		generateReport,
		getConfig,
		getRadarStats,
		getReport,
		getSites,
		type Config,
		type RadarStats,
		type Site,
		type SiteReport
	} from '../lib/api';
	import DataSourceSelector from '../lib/components/DataSourceSelector.svelte';
	import { initializePaperSize, paperSize } from '../lib/stores/paper';
	import { displayTimezone, initializeTimezone } from '../lib/stores/timezone';
	import { displayUnits, initializeUnits } from '../lib/stores/units';
	import { getUnitLabel, type Unit } from '../lib/units';
./lib/stores/units';
	import { getUnitLabel, type Unit } from '../lib/units';

	let stats: RadarStats[] = [];
	let config: Config = { units: 'mph', timezone: 'UTC' }; // default
	let totalCount = 0;
	let p98Speed = 0;
	let loading = true;
	let error = '';
	let refreshError = '';
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
	let selectedSource: string = 'radar_objects';
	let statsRequestSerial = 0;

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

	// Reload behavior: when the dashboard filters change, refresh the summary
	// stats and point the chart image at the matching SVG endpoint.
	let lastFrom: number = 0;
	let lastTo: number = 0;
	let lastUnits: Unit | undefined = undefined;
	let lastGroup = '';
	let lastSource = '';
	let lastSiteId: number | null = null;
	let initialized = false;
	let cosineCorrectionAngles: number[] = [];
	let cosineCorrectionLabel = '';

	$: timeSeriesChartUrl =
		selectedSiteId != null && dateRange.from && dateRange.to
			? buildTimeSeriesChartPath({
					siteId: selectedSiteId,
					startDate: isoDate(dateRange.from),
					endDate: isoDate(dateRange.to),
					group,
					units: $displayUnits,
					timezone: $displayTimezone,
					source: selectedSource,
					paperSize: $paperSize
				})
			: '';

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

		if (dateChanged || unitsChanged || groupChanged || sourceChanged || siteChanged) {
			lastFrom = from;
			lastTo = to;
			lastUnits = $displayUnits;
			lastGroup = group;
			lastSource = selectedSource;
			lastSiteId = selectedSiteId;
			if (dateChanged) {
				saveReportDateRangeSettings();
			}

			const includeAggregate = dateChanged || unitsChanged || sourceChanged || siteChanged;
			void refreshStats($displayUnits, includeAggregate);
		}
	}

	async function loadConfig() {
		config = await getConfig();
		initializeUnits(config.units);
		initializeTimezone(config.timezone);
		initializePaperSize();
		if (browser) console.debug('[dashboard] initialized timezone store ->', $displayTimezone);
	}

	async function loadSites() {
		sites = await getSites();
		siteOptions = sites.map((site) => ({ value: site.id, label: site.name }));

		if (browser) {
			const savedSiteId = localStorage.getItem('selectedSiteId');
			if (savedSiteId) {
				const siteId = parseInt(savedSiteId, 10);
				if (sites.some((s) => s.id === siteId)) {
					selectedSiteId = siteId;
				}
			}
			if (selectedSiteId === null && sites.length > 0) {
				selectedSiteId = sites[0].id;
			}
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
			if (isDateRangeStale(settings?.dateRange?.savedAt)) return;

			const from = settings?.dateRange?.from ? new Date(settings.dateRange.from) : null;
			const to = settings?.dateRange?.to ? new Date(settings.dateRange.to) : null;
			if (!from || !to || Number.isNaN(from.getTime()) || Number.isNaN(to.getTime())) return;

			dateRange = {
				from,
				to,
				periodType: PeriodType.Day
			};
		} catch (e) {
			console.warn('Could not load saved report settings:', e);
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
				periodType: String(dateRange.periodType),
				savedAt: new Date().toISOString()
			};

			localStorage.setItem(REPORT_SETTINGS_KEY, JSON.stringify(settings));
		} catch (e) {
			console.warn('Could not save report settings:', e);
		}
	}

	async function loadStats(units: Unit, includeAggregate = true) {
		const requestSerial = ++statsRequestSerial;

		if (!dateRange.from || !dateRange.to) {
			if (requestSerial !== statsRequestSerial) return;
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
		if (browser) console.debug('[dashboard] fetch stats timezone ->', $displayTimezone);

		const nextStats = statsResp.metrics;
		const nextAngles = statsResp.cosineCorrection?.angles ?? [];
		const nextTotalCount = nextStats.reduce((sum, s) => sum + (s.count || 0), 0);
		let nextP98 = p98Speed;

		if (includeAggregate) {
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
				nextP98 = Number(aggregateBucket.p98);
			} else {
				nextP98 = weightedMedian(
					nextStats.map((s) => ({
						value: Number(s.p98 || 0),
						weight: Math.max(0, Number(s.count || 0))
					}))
				);
			}
		}

		if (requestSerial !== statsRequestSerial) return;

		stats = nextStats;
		cosineCorrectionAngles = nextAngles;
		totalCount = nextTotalCount;
		if (includeAggregate) {
			p98Speed = nextP98;
		}
	}

	async function refreshStats(units: Unit, includeAggregate = true) {
		try {
			await loadStats(units, includeAggregate);
			refreshError = '';
		} catch (e) {
			refreshError = e instanceof Error && e.message ? e.message : 'Could not refresh stats.';
		}
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
			lastSiteId = selectedSiteId;
			await loadStats($displayUnits, true);
			refreshError = '';
		} catch (e) {
			error = e instanceof Error && e.message ? e.message : 'Could not load dashboard data.';
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
			lastGeneratedReportId = null;
			reportMessage = 'Please select a date range first';
			return;
		}

		if (selectedSiteId == null) {
			lastGeneratedReportId = null;
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
				site_id: selectedSiteId,
				paper_size: $paperSize
			});

			lastGeneratedReportId = response.report_id;

			// Fetch report metadata to get filenames
			reportMetadata = await getReport(response.report_id);

			reportMessage = `Report generated. Use the links below to download.`;
		} catch (e) {
			reportMessage = e instanceof Error ? e.message : 'Could not generate the report.';
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
		<div
			role="alert"
			aria-live="assertive"
			class="rounded border border-red-300 bg-red-50 p-3 text-red-800 dark:border-red-700 dark:bg-red-950 dark:text-red-200"
		>
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
				<!-- Use on:change with parseInt to avoid HTML <select> coercing
					 selectedSiteId from number to string. -->
				<select
					id="dashboard-site"
					value={selectedSiteId}
					on:change={(e) => {
						const raw = e.currentTarget.value;
						selectedSiteId = raw === '' ? null : parseInt(raw, 10);
					}}
					class="border-surface-300 bg-surface-100 w-full rounded border px-2 py-2 text-sm"
				>
					{#if siteOptions.length === 0}
						<option value="">No sites</option>
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
					{generatingReport ? 'Generating...' : 'Generate'}
				</Button>
			</div>
		</div>

		{#if refreshError}
			<div
				role="alert"
				aria-live="polite"
				class="rounded border border-red-300 bg-red-50 p-3 text-red-800 dark:border-red-700 dark:bg-red-950 dark:text-red-200"
			>
				{refreshError}
			</div>
		{/if}

		{#if reportMessage}
			<div
				role={lastGeneratedReportId !== null ? 'status' : 'alert'}
				aria-live="polite"
				class="rounded border p-3 {lastGeneratedReportId !== null
					? 'border-green-300 bg-green-50 text-green-800 dark:border-green-700 dark:bg-green-950 dark:text-green-200'
					: 'border-red-300 bg-red-50 text-red-800 dark:border-red-700 dark:bg-red-950 dark:text-red-200'}"
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
							📄 Download Report
						</a>
						{#if reportMetadata.zip_filename}
							<!-- eslint-disable svelte/no-navigation-without-resolve -->
							<a
								href={`/api/reports/${lastGeneratedReportId}/download/${reportMetadata.zip_filename}`}
								class="border-secondary-500 text-secondary-500 hover:bg-secondary-50 inline-flex items-center justify-center rounded-md border px-4 py-2 text-sm font-medium transition-colors hover:text-white"
								download
								aria-label="Download source files as ZIP archive"
							>
								📦 Download ZIP
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

		<div class="vr-stat-grid" role="region" aria-label="Traffic statistics summary">
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

		{#if timeSeriesChartUrl}
			<div class="block w-full rounded border p-4">
				<InlineSvgChart
					url={timeSeriesChartUrl}
					label="Vehicle count bars and percentile speed lines"
					loadingLabel="Refreshing chart…"
					minHeight={340}
				/>
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
							{#each stats as row (row.date.getTime())}
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
