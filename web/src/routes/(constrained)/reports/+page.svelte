<script lang="ts">
	import { browser } from '$app/environment';
	import {
		buildComparisonChartPath,
		buildHistogramChartPath,
		buildTimeSeriesChartPath,
		generateReport,
		getConfig,
		getReport,
		getSites,
		type Config,
		type Site,
		type SiteReport
	} from '$lib/api';
	import InlineSvgChart from '$lib/components/charts/InlineSvgChart.svelte';
	import DataSourceSelector from '$lib/components/DataSourceSelector.svelte';
	import { isoDate } from '$lib/dateUtils';
	import { isDateRangeStale, REPORT_SETTINGS_KEY } from '$lib/reportSettings';
	import { paperSize, initializePaperSize } from '$lib/stores/paper';
	import { displayTimezone, initializeTimezone } from '$lib/stores/timezone';
	import { displayUnits, initializeUnits } from '$lib/stores/units';
	import { PeriodType } from '@layerstack/utils';
	import { onMount } from 'svelte';
	import { Button, Card, DateRangeField, Header, SelectField } from 'svelte-ux';

	let config: Config = { units: 'mph', timezone: 'UTC' };
	let loading = true;
	let error = '';

	let sites: Site[] = [];
	let selectedSiteId: number | null = null;
	let siteOptions: Array<{ value: number; label: string }> = [];
	let selectedSite: Site | null = null;

	// default DateRangeField to the last 14 days (inclusive)
	const today = new Date();
	const fromDefault = new Date(today); // eslint-disable-line svelte/prefer-svelte-reactivity
	fromDefault.setDate(today.getDate() - 13); // last 14 days inclusive
	let dateRange = { from: fromDefault, to: today, periodType: PeriodType.Day };

	const compareToDefault = new Date(fromDefault); // eslint-disable-line svelte/prefer-svelte-reactivity
	compareToDefault.setDate(fromDefault.getDate() - 1);
	const compareFromDefault = new Date(compareToDefault); // eslint-disable-line svelte/prefer-svelte-reactivity
	compareFromDefault.setDate(compareToDefault.getDate() - 13);
	let compareRange = { from: compareFromDefault, to: compareToDefault, periodType: PeriodType.Day };
	let compareEnabled = false;
	let compareSource: string = 'radar_objects';

	let group: string = '4h';
	let selectedSource: string = 'radar_objects';
	let minSpeed: number = 5;
	let maxSpeedCutoff: number | null = null;
	let boundaryThreshold: number = 5;
	const histogramBucketSize = 5;

	let generatingReport = false;
	let reportMessage = '';
	let lastGeneratedReportId: number | null = null;
	let reportMetadata: SiteReport | null = null;

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

	$: selectedSite =
		selectedSiteId != null ? (sites.find((site) => site.id === selectedSiteId) ?? null) : null;

	$: reportTimeSeriesChartUrl =
		selectedSiteId != null && dateRange.from && dateRange.to
			? buildTimeSeriesChartPath({
					siteId: selectedSiteId,
					startDate: isoDate(dateRange.from),
					endDate: isoDate(dateRange.to),
					group,
					units: $displayUnits,
					timezone: $displayTimezone,
					source: selectedSource,
					minSpeed,
					boundaryThreshold,
					paperSize: $paperSize
				})
			: '';

	$: reportHistogramChartUrl =
		selectedSiteId != null && dateRange.from && dateRange.to
			? buildHistogramChartPath({
					siteId: selectedSiteId,
					startDate: isoDate(dateRange.from),
					endDate: isoDate(dateRange.to),
					units: $displayUnits,
					timezone: $displayTimezone,
					source: selectedSource,
					bucketSize: histogramBucketSize,
					max: maxSpeedCutoff ?? undefined,
					minSpeed,
					boundaryThreshold,
					paperSize: $paperSize
				})
			: '';

	$: reportComparisonChartUrl =
		selectedSiteId != null &&
		compareEnabled &&
		dateRange.from &&
		dateRange.to &&
		compareRange.from &&
		compareRange.to
			? buildComparisonChartPath({
					siteId: selectedSiteId,
					startDate: isoDate(dateRange.from),
					endDate: isoDate(dateRange.to),
					compareStartDate: isoDate(compareRange.from),
					compareEndDate: isoDate(compareRange.to),
					units: $displayUnits,
					timezone: $displayTimezone,
					source: selectedSource,
					compareSource,
					bucketSize: histogramBucketSize,
					max: maxSpeedCutoff ?? undefined,
					minSpeed,
					boundaryThreshold,
					paperSize: $paperSize
				})
			: '';

	async function loadConfig() {
		try {
			config = await getConfig();
			initializeUnits(config.units);
			initializeTimezone(config.timezone);
			initializePaperSize();
		} catch (e) {
			error = e instanceof Error && e.message ? e.message : 'Could not load configuration.';
		}
	}

	async function loadSites() {
		try {
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
		} catch (e) {
			error = e instanceof Error && e.message ? e.message : 'Could not load sites.';
		}
	}

	$: if (browser && selectedSiteId != null) {
		localStorage.setItem('selectedSiteId', selectedSiteId.toString());
	}

	function saveReportSettings() {
		if (!browser) return;
		try {
			const now = new Date().toISOString();
			const settings = {
				dateRange: {
					from: dateRange.from?.toISOString(),
					to: dateRange.to?.toISOString(),
					periodType: dateRange.periodType,
					savedAt: now
				},
				compareRange: {
					from: compareRange.from?.toISOString(),
					to: compareRange.to?.toISOString(),
					periodType: compareRange.periodType,
					savedAt: now
				},
				compareEnabled,
				compareSource,
				group,
				selectedSource,
				minSpeed,
				maxSpeedCutoff,
				boundaryThreshold
			};
			localStorage.setItem(REPORT_SETTINGS_KEY, JSON.stringify(settings));
		} catch (e) {
			console.warn('Could not save report settings:', e);
		}
	}

	function loadReportSettings() {
		if (!browser) return;
		try {
			const saved = localStorage.getItem(REPORT_SETTINGS_KEY);
			if (!saved) return;

			const settings = JSON.parse(saved);
			const stale = isDateRangeStale(settings.dateRange?.savedAt);

			// Restore date ranges only when fresh
			if (!stale && settings.dateRange?.from && settings.dateRange?.to) {
				dateRange = {
					from: new Date(settings.dateRange.from),
					to: new Date(settings.dateRange.to),
					periodType: settings.dateRange.periodType ?? PeriodType.Day
				};
			}

			if (!stale && settings.compareRange?.from && settings.compareRange?.to) {
				compareRange = {
					from: new Date(settings.compareRange.from),
					to: new Date(settings.compareRange.to),
					periodType: settings.compareRange.periodType ?? PeriodType.Day
				};
			}

			// Restore other settings regardless of date staleness
			if (settings.compareEnabled !== undefined) compareEnabled = settings.compareEnabled;
			if (settings.compareSource) compareSource = settings.compareSource;
			if (settings.group) group = settings.group;
			if (settings.selectedSource) selectedSource = settings.selectedSource;
			if (settings.minSpeed !== undefined) minSpeed = settings.minSpeed;
			if (settings.maxSpeedCutoff !== undefined) maxSpeedCutoff = settings.maxSpeedCutoff;
			if (settings.boundaryThreshold !== undefined) boundaryThreshold = settings.boundaryThreshold;
		} catch (e) {
			console.warn('Could not load report settings:', e);
		}
	}

	async function loadData() {
		loading = true;
		error = '';
		try {
			await loadConfig();
			await loadSites();
			loadReportSettings();
		} finally {
			loading = false;
		}
	}

	onMount(loadData);

	async function handleGenerateReport() {
		if (!dateRange.from || !dateRange.to) {
			lastGeneratedReportId = null;
			reportMessage = 'Select a date range first.';
			return;
		}

		if (compareEnabled && (!compareRange.from || !compareRange.to)) {
			lastGeneratedReportId = null;
			reportMessage = 'Select the comparison period dates.';
			return;
		}

		if (selectedSiteId == null) {
			lastGeneratedReportId = null;
			reportMessage = 'Select a site first.';
			return;
		}

		generatingReport = true;
		reportMessage = '';
		lastGeneratedReportId = null;
		reportMetadata = null;

		try {
			const request = {
				start_date: isoDate(dateRange.from),
				end_date: isoDate(dateRange.to),
				timezone: $displayTimezone,
				units: $displayUnits,
				group: group,
				source: selectedSource,
				min_speed: minSpeed,
				hist_max: maxSpeedCutoff ?? undefined,
				boundary_threshold: boundaryThreshold,
				histogram: true,
				hist_bucket_size: 5.0,
				site_id: selectedSiteId,
				paper_size: $paperSize
			};

			if (compareEnabled) {
				Object.assign(request, {
					compare_start_date: isoDate(compareRange.from),
					compare_end_date: isoDate(compareRange.to),
					compare_source: compareSource
				});
			}

			const response = await generateReport(request);
			lastGeneratedReportId = response.report_id;
			reportMetadata = await getReport(response.report_id);
			reportMessage = 'Report ready — use the links below to download.';

			// Save settings for next time
			saveReportSettings();
		} catch (e) {
			reportMessage = e instanceof Error ? e.message : 'Could not generate the report.';
		} finally {
			generatingReport = false;
		}
	}
</script>

<svelte:head>
	<title>Reports 🚴 velocity.report</title>
	<meta name="description" content="Generate traffic reports and compare survey periods" />
</svelte:head>

<div id="main-content" class="space-y-6 p-4">
	<Header title="Report Generator" subheading="Generate PDF reports and compare survey periods" />

	{#if loading}
		<div role="status" aria-live="polite" aria-busy="true">
			<p>Loading report options…</p>
			<span class="sr-only">Please wait while we fetch configuration data</span>
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
		<Card>
			<div class="space-y-4 p-6">
				<div class="flex flex-wrap items-end gap-4">
					<div class="w-70 space-y-2">
						<p class="text-surface-content/80 text-sm font-medium">Primary period</p>
						<DateRangeField bind:value={dateRange} periodTypes={[PeriodType.Day]} stepper />
					</div>
					<div class="w-24">
						<DataSourceSelector bind:value={selectedSource} />
					</div>
					<div class="w-24">
						<SelectField bind:value={group} label="Group" {options} clearable={false} />
					</div>
					<!-- Keep the site selector wide enough for typical site names. -->
					<div class="w-38">
						<SelectField
							bind:value={selectedSiteId}
							label="Site"
							options={siteOptions}
							clearable={false}
						/>
					</div>
				</div>
				<div class="flex flex-wrap items-end gap-4">
					<div class="w-42">
						<label class="text-surface-content/80 block text-sm font-medium">
							Min Speed ({$displayUnits})
							<input
								type="number"
								bind:value={minSpeed}
								min="0"
								step="1"
								class="border-surface-content/20 bg-surface-100 mt-1 block w-full rounded-md border px-3 py-2 text-sm"
							/>
						</label>
					</div>
					<div class="w-42">
						<label class="text-surface-content/80 block text-sm font-medium">
							Max Speed Cutoff ({$displayUnits})
							<input
								type="number"
								bind:value={maxSpeedCutoff}
								min="0"
								step="5"
								placeholder="None"
								class="border-surface-content/20 bg-surface-100 mt-1 block w-full rounded-md border px-3 py-2 text-sm"
							/>
						</label>
					</div>
					<div class="w-42">
						<label class="text-surface-content/80 block text-sm font-medium">
							Min Period Count
							<input
								type="number"
								bind:value={boundaryThreshold}
								min="0"
								step="1"
								class="border-surface-content/20 bg-surface-100 mt-1 block w-full rounded-md border px-3 py-2 text-sm"
							/>
						</label>
					</div>
				</div>

				<label class="text-surface-content/80 flex items-center gap-2 text-sm font-medium">
					<input type="checkbox" bind:checked={compareEnabled} class="h-4 w-4" />
					Compare against another period
				</label>

				{#if compareEnabled}
					<div class="flex flex-wrap items-end gap-4">
						<div class="w-70 space-y-2">
							<p class="text-surface-content/80 text-sm font-medium">Comparison period</p>
							<DateRangeField bind:value={compareRange} periodTypes={[PeriodType.Day]} stepper />
						</div>
						<div class="w-24">
							<DataSourceSelector bind:value={compareSource} />
						</div>
					</div>
				{/if}

				<div class="flex flex-wrap items-center gap-3">
					<Button
						on:click={handleGenerateReport}
						disabled={generatingReport || selectedSiteId == null}
						variant="fill"
						color="primary"
						aria-label={generatingReport ? 'Generating report, please wait' : 'Generate report'}
					>
						{generatingReport ? 'Generating…' : 'Generate Report'}
					</Button>
					<p class="text-surface-content/60 text-xs">
						Reports use {$displayUnits} units and {$displayTimezone} timezone settings.
					</p>
				</div>
			</div>
		</Card>

		<Card>
			<div class="space-y-3 p-6">
				<h3 class="text-base font-semibold">Site Details</h3>
				{#if selectedSite}
					<dl class="text-surface-content/80 grid gap-3 text-sm md:grid-cols-2">
						<div>
							<dt class="text-surface-content font-semibold">Location</dt>
							<dd>{selectedSite.location}</dd>
						</div>
						<div>
							<dt class="text-surface-content font-semibold">Speed Limit</dt>
							<dd>{selectedSite.speed_limit} {$displayUnits}</dd>
						</div>
						<div>
							<dt class="text-surface-content font-semibold">Surveyor</dt>
							<dd>{selectedSite.surveyor}</dd>
						</div>
						<div>
							<dt class="text-surface-content font-semibold">Contact</dt>
							<dd>{selectedSite.contact}</dd>
						</div>
						{#if selectedSite.site_description}
							<div class="md:col-span-2">
								<dt class="text-surface-content font-semibold">Site Description</dt>
								<dd>{selectedSite.site_description}</dd>
							</div>
						{/if}
						{#if selectedSite.speed_limit_note}
							<div class="md:col-span-2">
								<dt class="text-surface-content font-semibold">Speed Limit Notes</dt>
								<dd>{selectedSite.speed_limit_note}</dd>
							</div>
						{/if}
					</dl>
				{:else}
					<p class="text-surface-content/60 text-sm">Select a site to view report details.</p>
				{/if}
			</div>
		</Card>

		{#if reportTimeSeriesChartUrl || reportHistogramChartUrl || reportComparisonChartUrl}
			<Card>
				<div class="space-y-4 p-6">
					<div class="space-y-1">
						<h3 class="text-base font-semibold">Chart Previews</h3>
						<p class="text-surface-content/70 text-sm">
							These previews come from the Go SVG chart endpoints used by the report pipeline.
						</p>
					</div>

					<div class="grid gap-4 lg:grid-cols-2">
						{#if reportTimeSeriesChartUrl}
							<div class="space-y-2 rounded border p-3 lg:col-span-2">
								<h4 class="text-sm font-semibold">Time-series overview</h4>
								<InlineSvgChart
									url={reportTimeSeriesChartUrl}
									label="Preview of the report time-series chart"
									loadingLabel="Loading time-series preview…"
									minHeight={340}
								/>
							</div>
						{/if}

						{#if reportHistogramChartUrl}
							<div class="space-y-2 rounded border p-3">
								<h4 class="text-sm font-semibold">Velocity distribution</h4>
								<InlineSvgChart
									url={reportHistogramChartUrl}
									label="Preview of the report histogram chart"
									loadingLabel="Loading histogram preview…"
									minHeight={250}
								/>
							</div>
						{/if}

						{#if compareEnabled && reportComparisonChartUrl}
							<div class="space-y-2 rounded border p-3">
								<h4 class="text-sm font-semibold">Comparison distribution</h4>
								<InlineSvgChart
									url={reportComparisonChartUrl}
									label="Preview of the report comparison histogram chart"
									loadingLabel="Loading comparison preview…"
									minHeight={250}
								/>
							</div>
						{:else if compareEnabled}
							<div class="space-y-2 rounded border p-3">
								<h4 class="text-sm font-semibold">Comparison distribution</h4>
								<p class="text-surface-content/70 text-sm">
									Select a complete comparison range to preview the comparison chart.
								</p>
							</div>
						{/if}
					</div>
				</div>
			</Card>
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
					The ZIP file contains LaTeX source files and chart SVG assets for custom editing.
				</p>
			</div>
		{/if}
	{/if}
</div>
