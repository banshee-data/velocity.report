<script lang="ts">
	import { browser } from '$app/environment';
	import { PeriodType } from '@layerstack/utils';
	import { onMount } from 'svelte';
	import { Button, Card, DateRangeField, Header, SelectField, ToggleGroup, ToggleOption } from 'svelte-ux';
	import {
		generateReport,
		getConfig,
		getReport,
		getSites,
		type Config,
		type Site,
		type SiteReport
	} from '$lib/api';
	import { displayTimezone, initializeTimezone } from '$lib/stores/timezone';
	import { displayUnits, initializeUnits } from '$lib/stores/units';

	let config: Config = { units: 'mph', timezone: 'UTC' };
	let loading = true;
	let error = '';

	let sites: Site[] = [];
	let selectedSiteId: number | null = null;
	let siteOptions: Array<{ value: number; label: string }> = [];
	let selectedSite: Site | null = null;

	// default DateRangeField to the last 14 days (inclusive)
	function isoDate(d: Date) {
		return d.toISOString().slice(0, 10);
	}
	const today = new Date();
	const fromDefault = new Date(today); // eslint-disable-line svelte/prefer-svelte-reactivity
	fromDefault.setDate(today.getDate() - 13); // last 14 days inclusive
	let dateRange = { from: fromDefault, to: today, periodType: PeriodType.Day };

	const compareToDefault = new Date(fromDefault);
	compareToDefault.setDate(fromDefault.getDate() - 1);
	const compareFromDefault = new Date(compareToDefault);
	compareFromDefault.setDate(compareToDefault.getDate() - 13);
	let compareRange = { from: compareFromDefault, to: compareToDefault, periodType: PeriodType.Day };
	let compareEnabled = false;

	let group: string = '4h';
	let selectedSource: string = 'radar_objects';

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
		selectedSiteId != null ? sites.find((site) => site.id === selectedSiteId) : null;

	async function loadConfig() {
		try {
			config = await getConfig();
			initializeUnits(config.units);
			initializeTimezone(config.timezone);
		} catch (e) {
			error = e instanceof Error && e.message ? e.message : 'Failed to load config';
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
			error = e instanceof Error && e.message ? e.message : 'Failed to load sites';
		}
	}

	$: if (browser && selectedSiteId != null) {
		localStorage.setItem('selectedSiteId', selectedSiteId.toString());
	}

	async function loadData() {
		loading = true;
		error = '';
		try {
			await loadConfig();
			await loadSites();
		} finally {
			loading = false;
		}
	}

	onMount(loadData);

	async function handleGenerateReport() {
		if (!dateRange.from || !dateRange.to) {
			reportMessage = 'Please select a date range first';
			return;
		}

		if (compareEnabled && (!compareRange.from || !compareRange.to)) {
			reportMessage = 'Please select the comparison period dates';
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
			const request = {
				start_date: isoDate(dateRange.from),
				end_date: isoDate(dateRange.to),
				timezone: $displayTimezone,
				units: $displayUnits,
				group: group,
				source: selectedSource,
				histogram: true,
				hist_bucket_size: 5.0,
				site_id: selectedSiteId
			};

			if (compareEnabled) {
				Object.assign(request, {
					compare_start_date: isoDate(compareRange.from),
					compare_end_date: isoDate(compareRange.to)
				});
			}

			const response = await generateReport(request);
			lastGeneratedReportId = response.report_id;
			reportMetadata = await getReport(response.report_id);
			reportMessage = 'Report generated successfully! Use the links below to download.';
		} catch (e) {
			reportMessage = e instanceof Error ? e.message : 'Failed to generate report';
		} finally {
			generatingReport = false;
		}
	}
</script>

<svelte:head>
	<title>Reports 🚴 velocity.report</title>
	<meta name="description" content="Generate traffic reports and compare survey periods" />
</svelte:head>

<main id="main-content" class="space-y-6 p-4">
	<Header title="Report Generator" subheading="Generate PDF reports and compare survey periods" />

	{#if loading}
		<div role="status" aria-live="polite" aria-busy="true">
			<p>Loading report options…</p>
			<span class="sr-only">Please wait while we fetch configuration data</span>
		</div>
	{:else if error}
		<div role="alert" aria-live="assertive" class="text-red-600">
			<strong>Error:</strong>
			{error}
		</div>
	{:else}
		<Card>
			<div class="space-y-4 p-6">
				<div class="flex flex-wrap items-end gap-4">
					<div class="space-y-2">
						<p class="text-sm font-medium text-surface-content/80">Primary period</p>
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
					<div class="w-42">
						<SelectField
							bind:value={selectedSiteId}
							label="Site"
							options={siteOptions}
							clearable={false}
						/>
					</div>
				</div>

				<label class="flex items-center gap-2 text-sm font-medium text-surface-content/80">
					<input type="checkbox" bind:checked={compareEnabled} class="h-4 w-4" />
					Compare against another period
				</label>

				{#if compareEnabled}
					<div class="space-y-2">
						<p class="text-sm font-medium text-surface-content/80">Comparison period</p>
						<DateRangeField bind:value={compareRange} periodTypes={[PeriodType.Day]} stepper />
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
					<p class="text-xs text-surface-content/60">
						Reports use {$displayUnits} units and {$displayTimezone} timezone settings.
					</p>
				</div>
			</div>
		</Card>

		<Card>
			<div class="space-y-3 p-6">
				<h3 class="text-base font-semibold">Site Details</h3>
				{#if selectedSite}
					<dl class="grid gap-3 text-sm text-surface-content/80 md:grid-cols-2">
						<div>
							<dt class="font-semibold text-surface-content">Location</dt>
							<dd>{selectedSite.location}</dd>
						</div>
						<div>
							<dt class="font-semibold text-surface-content">Speed Limit</dt>
							<dd>{selectedSite.speed_limit} {$displayUnits}</dd>
						</div>
						<div>
							<dt class="font-semibold text-surface-content">Surveyor</dt>
							<dd>{selectedSite.surveyor}</dd>
						</div>
						<div>
							<dt class="font-semibold text-surface-content">Contact</dt>
							<dd>{selectedSite.contact}</dd>
						</div>
						{#if selectedSite.site_description}
							<div class="md:col-span-2">
								<dt class="font-semibold text-surface-content">Site Description</dt>
								<dd>{selectedSite.site_description}</dd>
							</div>
						{/if}
						{#if selectedSite.speed_limit_note}
							<div class="md:col-span-2">
								<dt class="font-semibold text-surface-content">Speed Limit Notes</dt>
								<dd>{selectedSite.speed_limit_note}</dd>
							</div>
						{/if}
					</dl>
				{:else}
					<p class="text-sm text-surface-content/60">Select a site to view report details.</p>
				{/if}
			</div>
		</Card>

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
					The ZIP file contains LaTeX source files and chart PDFs for custom editing.
				</p>
			</div>
		{/if}
	{/if}
</main>
