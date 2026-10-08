<script lang="ts">
	import { getRecentReports, type SiteReport } from '#lib/api.js';
	import { access, hasPermission } from '#lib/stores/access.js';
	import { onMount } from 'svelte';
	import { Button } from 'svelte-ux';

	let reports: SiteReport[] = [];
	let loading = true;
	let error = '';
	let active = true;

	async function loadReports() {
		if (!hasPermission($access, 'reports:read')) return;
		loading = true;
		error = '';
		try {
			const next = await getRecentReports();
			if (active) reports = next ?? [];
		} catch (cause) {
			if (active) error = cause instanceof Error ? cause.message : 'Could not load reports.';
		} finally {
			if (active) loading = false;
		}
	}

	onMount(() => {
		void loadReports();
		return () => {
			active = false;
		};
	});
</script>

<section class="space-y-3" aria-label="Existing reports">
	<div class="flex items-center justify-between gap-3">
		<h2 class="text-lg font-semibold">Existing reports</h2>
		<Button variant="outline" on:click={loadReports} disabled={loading}>Refresh reports</Button>
	</div>
	{#if error}
		<p role="alert">{error}</p>
	{:else if loading}
		<p role="status">Loading existing reports…</p>
	{:else if reports.length === 0}
		<p class="text-surface-content/70 text-sm">No reports have been generated yet.</p>
	{:else}
		<ul class="divide-surface-content/10 divide-y rounded border p-3">
			{#each reports as report (report.id)}
				<li class="flex flex-wrap items-center justify-between gap-3 py-3">
					<div>
						<p class="font-medium">{report.filename}</p>
						<p class="text-surface-content/70 text-sm">{report.start_date} – {report.end_date}</p>
					</div>
					<div class="flex gap-3">
						<!-- eslint-disable svelte/no-navigation-without-resolve -->
						<a
							class="text-primary underline"
							href={`/api/reports/${report.id}/download/report.pdf`}
							download>Download PDF</a
						>
						{#if hasPermission($access, 'data:export') && report.zip_filename}
							<!-- eslint-disable svelte/no-navigation-without-resolve -->
							<a
								class="text-primary underline"
								href={`/api/reports/${report.id}/download/sources.zip`}
								download>Download sources</a
							>
						{/if}
					</div>
				</li>
			{/each}
		</ul>
	{/if}
</section>
