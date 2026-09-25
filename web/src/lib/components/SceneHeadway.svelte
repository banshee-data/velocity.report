<script lang="ts">
	// A scene's following distribution: the Go-served histogram SVG
	// (/api/charts/histogram?kind=headway, D-11/D-17) and the summary tables
	// from GET /api/scenes/<id>/headway. Every figure is the server's; the
	// helpers in $lib/headway only format it.
	import {
		buildHeadwayChartPath,
		getSceneHeadway,
		type HeadwaySelection,
		type SceneHeadway
	} from '$lib/api';
	import InlineSvgChart from '$lib/components/charts/InlineSvgChart.svelte';
	import {
		HEADWAY_METRICS,
		headwayAvailabilityMessage,
		headwayBandRows,
		headwayEncounterRows,
		headwayExcludedRows,
		headwayStatusLabel,
		headwayStatusNote,
		headwaySummaryRows,
		versionKey,
		versionLabel
	} from '$lib/headway';

	export let sceneId: string;

	let headway: SceneHeadway | null = null;
	let loading = true;
	let error = '';
	let selection: HeadwaySelection = {};
	let metric: string = HEADWAY_METRICS.netTimeGap;
	let requestSerial = 0;

	$: void load(sceneId, selection);

	async function load(id: string, sel: HeadwaySelection) {
		const serial = ++requestSerial;
		loading = true;
		error = '';
		try {
			const result = await getSceneHeadway(id, sel);
			if (serial !== requestSerial) return;
			headway = result;
		} catch (e) {
			if (serial !== requestSerial) return;
			headway = null;
			error = e instanceof Error ? e.message : 'Could not load the following distribution.';
		} finally {
			if (serial === requestSerial) loading = false;
		}
	}

	$: requestedStage = selection.version?.estimate_stage ?? selection.stage ?? 'final';
	$: chartUrl =
		headway?.availability === 'available'
			? buildHeadwayChartPath({
					sceneId,
					sourceId: headway.source_id,
					version: headway.version,
					metric
				})
			: '';
	$: summaryRows = headway?.distribution ? headwaySummaryRows(headway.distribution) : [];
	$: bandRows = headway?.distribution ? headwayBandRows(headway.distribution) : [];
	$: excludedRows = headway?.distribution ? headwayExcludedRows(headway.distribution) : [];
	$: encounterRows = headway ? headwayEncounterRows(headway) : [];

	function chooseSource(event: Event) {
		const value = (event.currentTarget as HTMLSelectElement).value;
		selection = { sourceId: value || undefined };
	}

	function chooseVersion(event: Event) {
		const key = (event.currentTarget as HTMLSelectElement).value;
		const chosen = headway?.versions.find((v) => versionKey(v.version) === key);
		if (chosen) selection = { sourceId: headway?.source_id, version: chosen.version };
	}
</script>

<section class="mt-10 max-w-3xl space-y-4" aria-labelledby="scene-headway-heading">
	<h2
		id="scene-headway-heading"
		class="border-surface-content/15 border-b pb-2 text-lg font-semibold"
	>
		Following distribution
	</h2>

	{#if loading && !headway}
		<p class="text-surface-content/70 text-sm" role="status">Loading following distribution…</p>
	{:else if error}
		<div class="border-danger/40 bg-danger/10 rounded border p-3 text-sm" role="alert">{error}</div>
	{:else if headway}
		<div class="flex flex-wrap items-center gap-2 text-sm">
			<span class="rounded border border-amber-600 px-2 py-0.5 text-xs font-semibold tracking-wide">
				{headwayStatusLabel(headway.status)}
			</span>
			<span class="text-surface-content/70">{headwayStatusNote(headway.status)}</span>
		</div>

		<!-- Native selects, as in the scene form above: short lists of opaque
		     analysis ids need no richer control. -->
		{#if headway.sources.length > 1}
			<label class="block">
				<span class="mb-1 block text-sm">Analysis source</span>
				<select
					class="border-surface-content/25 w-full rounded border bg-transparent px-3 py-2 font-mono text-xs"
					value={headway.source_id ?? ''}
					on:change={chooseSource}
				>
					<option value="">Choose one of {headway.sources.length} analyses</option>
					{#each headway.sources as source (source.source_id)}
						<option value={source.source_id}>{source.source_id} · {source.events} encounters</option
						>
					{/each}
				</select>
			</label>
		{/if}

		{#if headway.versions.length > 0}
			<label class="block">
				<span class="mb-1 block text-sm">Analysis version</span>
				<select
					class="border-surface-content/25 w-full rounded border bg-transparent px-3 py-2 font-mono text-xs"
					value={headway.version ? versionKey(headway.version) : ''}
					on:change={chooseVersion}
				>
					{#if !headway.version}
						<option value="">No {requestedStage} version; choose one</option>
					{/if}
					{#each headway.versions as v (versionKey(v.version))}
						<option value={versionKey(v.version)}>{versionLabel(v)}</option>
					{/each}
				</select>
			</label>
		{/if}

		{#if headway.availability !== 'available'}
			<p class="text-surface-content/70 text-sm" role="status">
				{headwayAvailabilityMessage(headway, requestedStage)}
			</p>
		{:else}
			<label class="block">
				<span class="mb-1 block text-sm">Distributed metric</span>
				<select
					class="border-surface-content/25 rounded border bg-transparent px-3 py-2 font-mono text-xs"
					bind:value={metric}
				>
					<option value={HEADWAY_METRICS.netTimeGap}>{HEADWAY_METRICS.netTimeGap}</option>
					<option value={HEADWAY_METRICS.spatialGap}>{HEADWAY_METRICS.spatialGap}</option>
				</select>
			</label>

			<InlineSvgChart
				url={chartUrl}
				label="Time-weighted distribution over valid following time, with suppressed and predicted-only time beside it"
				loadingLabel="Loading chart…"
				themeMode="dashboard"
				minHeight={320}
			/>
			<p class="text-surface-content/60 text-xs">
				Bars are shares of all accounted encounter time, so they add up to the valid share only.
				Time outside the distribution stands beside it by reason. Whiskers span the time within one
				sigma of each bin. Band rules are descriptive bins with no established threshold.
			</p>

			<div class="grid gap-6 md:grid-cols-2">
				<table class="w-full text-sm">
					<caption class="mb-1 text-left font-medium">Accounted time</caption>
					<tbody>
						{#each summaryRows as row (row.label)}
							<tr>
								<th class="py-1 pr-4 text-left font-normal">{row.label}</th>
								<td class="py-1 pr-2 text-right font-mono">{row.value}</td>
								<td class="py-1 text-right font-mono">{row.share ?? ''}</td>
							</tr>
						{/each}
					</tbody>
				</table>

				<table class="w-full text-sm">
					<caption class="mb-1 text-left font-medium">Beside the distribution</caption>
					<thead>
						<tr class="border-surface-content/20 border-b text-left">
							<th class="py-1 pr-2 font-medium">Reason</th>
							<th class="py-1 pr-2 text-right font-medium">Time</th>
							<th class="py-1 text-right font-medium">Predicted-only</th>
						</tr>
					</thead>
					<tbody>
						{#each excludedRows as row (row.reason)}
							<tr>
								<td class="py-1 pr-2 font-mono text-xs">{row.reason}</td>
								<td class="py-1 pr-2 text-right font-mono">{row.time} ({row.share})</td>
								<td class="py-1 text-right font-mono">{row.predictedOnly}</td>
							</tr>
						{:else}
							<tr><td colspan="3" class="py-1">No accounted time outside the distribution.</td></tr>
						{/each}
					</tbody>
				</table>
			</div>

			<table class="w-full text-sm">
				<caption class="mb-1 text-left font-medium">Named-band exposure</caption>
				<thead>
					<tr class="border-surface-content/20 border-b text-left">
						<th class="py-1 pr-2 font-medium">Band</th>
						<th class="py-1 pr-2 font-medium">Metric</th>
						<th class="py-1 pr-2 text-right font-medium">Time below</th>
						<th class="py-1 pr-2 text-right font-medium">Rate</th>
						<th class="py-1 text-right font-medium">Encounters</th>
					</tr>
				</thead>
				<tbody>
					{#each bandRows as row (row.name)}
						<tr>
							<td class="py-1 pr-2 font-mono">{row.threshold}</td>
							<td class="py-1 pr-2 font-mono text-xs">{row.name}</td>
							<td class="py-1 pr-2 text-right font-mono">{row.below}</td>
							<td class="py-1 pr-2 text-right font-mono" title={row.rateName}>{row.rate}</td>
							<td class="py-1 text-right font-mono">{row.events}</td>
						</tr>
					{/each}
				</tbody>
			</table>

			<table class="w-full text-sm">
				<caption class="mb-1 text-left font-medium">
					Encounters ({encounterRows[0]?.block === 'provisional'
						? 'review-only provisional values'
						: 'final-stage values'}; intervals are the stored Monte Carlo bounds)
				</caption>
				<thead>
					<tr class="border-surface-content/20 border-b text-left">
						<th class="py-1 pr-2 font-medium">Start</th>
						<th class="py-1 pr-2 font-medium">Follower, leader</th>
						<th class="py-1 pr-2 text-right font-medium">Valid time</th>
						<th class="py-1 pr-2 text-right font-medium">Minimum time gap</th>
						<th class="py-1 text-right font-medium">Median time gap</th>
					</tr>
				</thead>
				<tbody>
					{#each encounterRows as row (row.eventId)}
						<tr>
							<td class="py-1 pr-2 font-mono">{row.start}</td>
							<td class="py-1 pr-2 font-mono text-xs" title={row.eventId}>
								{row.follower}<br />{row.leader}
							</td>
							<td class="py-1 pr-2 text-right font-mono">{row.validTime}</td>
							<td class="py-1 pr-2 text-right font-mono text-xs">{row.minNetTimeGap}</td>
							<td class="py-1 text-right font-mono text-xs">{row.medianNetTimeGap}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	{/if}
</section>
