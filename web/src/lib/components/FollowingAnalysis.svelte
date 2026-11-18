<script lang="ts">
	import { onDestroy } from 'svelte';
	import { analyseFollowing, type FollowingAnalysisResult } from '#lib/followingAnalysis.js';
	export let runID: string;
	let analysis: FollowingAnalysisResult | null = null;
	let error = '';
	let busy = false;
	let controller: AbortController | null = null;
	let currentRun = runID;
	$: if (runID !== currentRun) {
		controller?.abort();
		controller = null;
		currentRun = runID;
		analysis = null;
		error = '';
		busy = false;
	}
	onDestroy(() => controller?.abort());
	async function analyse() {
		const request = new AbortController();
		controller?.abort();
		controller = request;
		analysis = null;
		error = '';
		busy = true;
		try {
			const body = await analyseFollowing(runID, request.signal);
			if (controller === request) analysis = body;
		} catch (e) {
			if (controller === request && !request.signal.aborted)
				error = e instanceof Error ? e.message : 'Analysis failed.';
		} finally {
			if (controller === request) {
				busy = false;
				controller = null;
			}
		}
	}
</script>

<section aria-label="Following analysis" class="rounded border p-3">
	<h3 class="mb-2 font-semibold">Following time gaps</h3>
	<p class="mb-2 text-sm">
		Analyse the recording’s paired evidence. No file selection or playback is required.
	</p>
	<button class="rounded border px-3 py-2" disabled={busy} onclick={analyse}>
		{busy ? 'Analysing…' : 'Analyse following'}
	</button>
	{#if error}<p role="alert" class="mt-2 text-sm">{error}</p>{/if}
	{#if analysis}
		<h4 class="mt-3 font-medium">{analysis.metadata.binding.siteID} · minimum net time gap</h4>
		<p class="text-sm">
			{analysis.tracks - analysis.without_time_gap} tracks with a usable estimate;
			{analysis.without_time_gap} without one.
		</p>
		<table class="mt-2 w-full text-sm">
			<thead
				><tr
					><th class="text-left">Seconds</th><th class="text-right">Follower tracks</th><th
						>Distribution</th
					></tr
				></thead
			>
			<tbody
				>{#each analysis.histogram as bin (bin.label)}
					<tr
						><th class="text-left">{bin.label}</th><td class="text-right">{bin.count}</td>
						<td class="pl-3"
							><meter
								min="0"
								max={Math.max(1, analysis.tracks - analysis.without_time_gap)}
								value={bin.count}
								aria-label={`${bin.label} seconds: ${bin.count} tracks`}
							></meter></td
						></tr
					>
				{/each}</tbody
			>
		</table>
		<p class="mt-2 text-xs">
			One minimum per software track in this recording. Estimated gaps are provisional; fragmented
			tracks are not independent vehicles. Excluded and missing intervals do not enter the
			histogram.
		</p>
	{/if}
</section>
