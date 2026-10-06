<script lang="ts">
	import { resolve } from '$app/paths';
	import { getLidarRuns } from '$lib/api';
	import {
		keepSelector,
		requirementText,
		scoreText,
		segmentDetail,
		segmentQuery,
		selectorGroups,
		type SegmentSelector
	} from '$lib/segments';
	import type { AnalysisRun } from '$lib/types/lidar';
	import { onMount } from 'svelte';

	type Segment = {
		id: string;
		finder: string;
		window_start_unix_nanos: number;
		window_end_unix_nanos: number;
		peak_timestamp_ns: number;
		score: number;
		pair_seconds?: number;
		pairs?: number;
		leaders?: number;
		leader_changes?: number;
		closest_gap_m?: number;
		events?: number;
		capture?: string;
		offset_seconds?: number;
		status: string;
		replay_case_id?: string;
		job_id?: string;
		pack_dir?: string;
	};
	type Pack = {
		segment_id?: string;
		status: string;
		pack_dir: string;
		reviewed_objects: number;
		reviewed_masks: number;
		proposal_layers: number;
		error?: string;
	};

	let runs: AnalysisRun[] = [];
	let runID = '';
	let role = 'tuning';
	let selectorID = 'following';
	let selectors: SegmentSelector[] = [];
	// The digest of the selector the table was ranked with, sent with a case
	// so that the server refuses it if the selector has changed since.
	let rankedDigest = '';
	let segments: Segment[] = [];
	let packs: Pack[] = [];
	let dismissed = new Set<string>();
	let selected: Segment | null = null;
	let busy = false;
	let error = '';
	let message = '';
	let stripURL = '';

	$: groups = selectorGroups(selectors, role);
	$: chosen = selectors.find((entry) => entry.id === selectorID);
	$: visible = segments.filter((entry) => !dismissed.has(entry.id));
	$: selectedPack = packs.find((pack) => pack.segment_id === selected?.id);

	function query() {
		return segmentQuery(runID, selectorID, role);
	}

	async function api(path: string, init?: RequestInit) {
		const response = await fetch(path, init);
		if (!response.ok) {
			const body = await response.json().catch(() => ({}));
			throw new Error(body.error || `Request failed (${response.status})`);
		}
		return response.json();
	}

	async function load() {
		if (!runID) return;
		busy = true;
		error = '';
		try {
			const [ranking, inventory] = await Promise.all([
				api(`/api/lidar/segments?${query()}`),
				api('/api/annotations/packs').catch(() => ({ packs: [] }))
			]);
			segments = ranking.windows || [];
			rankedDigest = ranking.selector?.digest ?? '';
			packs = inventory.packs || [];
			stripURL = `/api/lidar/segments/strip?${query()}`;
			selected = segments.find((item) => item.id === selected?.id) ?? null;
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not load segments.';
		} finally {
			busy = false;
		}
	}

	async function changeRole() {
		selectorID = keepSelector(selectorID, selectorGroups(selectors, role));
		await load();
	}

	// Replace the row rather than assigning to it. A row is a plain object, so
	// an assignment to one of its fields redraws nothing: the table would keep
	// offering "Make case" for a window that already has one.
	function updateSegment(id: string, changes: Partial<Segment>) {
		segments = segments.map((entry) => (entry.id === id ? { ...entry, ...changes } : entry));
		if (selected?.id === id) selected = segments.find((entry) => entry.id === id) ?? null;
	}

	async function makeCase(segment: Segment) {
		busy = true;
		error = '';
		try {
			const result = await api(`/api/lidar/segments/${encodeURIComponent(segment.id)}/case`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					run_id: runID,
					selector: selectorID,
					role,
					selector_digest: rankedDigest
				})
			});
			updateSegment(segment.id, { replay_case_id: result.replay_case_id, status: 'case' });
			message = `Replay case ${result.replay_case_id} is ready.`;
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not make replay case.';
		} finally {
			busy = false;
		}
	}

	async function queueClip(segment: Segment) {
		if (!segment.replay_case_id) return;
		busy = true;
		error = '';
		try {
			const result = await api(
				`/api/lidar/scenes/${encodeURIComponent(segment.replay_case_id)}/clip`,
				{ method: 'POST' }
			);
			updateSegment(segment.id, { job_id: result.job.job_id, status: 'clipping' });
			message = `Clip job ${result.job.job_id} queued.`;
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not queue clip.';
		} finally {
			busy = false;
		}
	}

	function previewQuery(segment: Segment) {
		const run = runs.find((item) => item.run_id === runID);
		return new URLSearchParams({
			run_id: runID,
			sensor_id: run?.sensor_id || 'hesai-pandar40p',
			at_ns: String(segment.peak_timestamp_ns || segment.window_start_unix_nanos)
		}).toString();
	}

	function captureName(path?: string) {
		return path?.split(/[\\/]/).pop() || 'Unplaced';
	}

	onMount(async () => {
		try {
			const [listed, catalogue] = await Promise.all([
				getLidarRuns(),
				api('/api/lidar/segments/selectors')
			]);
			runs = listed;
			selectors = catalogue.selectors || [];
			runID = runs[0]?.run_id ?? '';
			await load();
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not load segment selectors.';
		}
	});
</script>

<svelte:head><title>LiDAR segments</title></svelte:head>

<main class="space-y-6 p-6">
	<header>
		<h1 class="text-2xl font-semibold">Annotation segments</h1>
		<p class="text-surface-content/70">
			Choose capture windows and prepare packs. Review happens in the macOS annotation tool.
		</p>
	</header>

	<div class="flex flex-wrap items-end gap-4">
		<label class="flex flex-col gap-1"
			>Run
			<select class="rounded border p-2" bind:value={runID} on:change={load}>
				{#each runs as run (run.run_id)}<option value={run.run_id}>{run.run_id}</option>{/each}
			</select>
		</label>
		<label class="flex flex-col gap-1"
			>Pack role
			<select class="rounded border p-2" bind:value={role} on:change={changeRole}>
				<option value="tuning">Tuning</option><option value="held_out">Held out</option>
			</select>
		</label>
		<label class="flex flex-col gap-1"
			>Selector
			<select class="rounded border p-2" bind:value={selectorID} on:change={load}>
				{#each groups as group (group.category)}
					<optgroup label={group.category}>
						{#each group.selectors as entry (entry.id)}<option value={entry.id}
								>{entry.label}</option
							>{/each}
					</optgroup>
				{/each}
			</select>
		</label>
		<button class="rounded border px-4 py-2" on:click={load} disabled={busy}>Refresh</button>
	</div>
	{#if chosen}
		<p class="text-surface-content/70">
			{chosen.description} Ranked by {scoreText(chosen)}{#if chosen.require.length}; requires {requirementText(
					chosen
				)}{/if}.
		</p>
	{/if}
	{#if role === 'held_out'}
		<p class="rounded bg-amber-100 p-3 text-amber-950">
			Choose a random window from each capture first, then add traffic windows. Tracker previews are
			hidden for blind review.
		</p>
	{/if}
	{#if error}<p role="alert" class="text-red-700">{error}</p>{/if}
	{#if message}<p role="status">{message}</p>{/if}

	{#if stripURL && segments.length}
		<section aria-label="Per-capture score strip" class="overflow-x-auto rounded border p-3">
			<img src={stripURL} alt="Segment scores grouped by capture" />
		</section>
	{/if}

	<div class="overflow-x-auto rounded border">
		<table class="w-full text-left text-sm">
			<thead class="bg-surface-200"
				><tr
					><th class="p-3">Window UTC</th><th class="p-3">Capture / offset</th><th class="p-3"
						>Score</th
					><th class="p-3">Finder detail</th><th class="p-3">Status</th><th class="p-3">Actions</th
					></tr
				></thead
			>
			<tbody>
				{#each visible as segment (segment.id)}
					<tr class="border-t">
						<td class="p-3">{new Date(segment.window_start_unix_nanos / 1e6).toISOString()}</td>
						<td class="p-3"
							>{captureName(segment.capture)}{#if segment.capture}
								@ {segment.offset_seconds?.toFixed(1)} s{/if}</td
						>
						<td class="p-3">{segment.score.toFixed(2)}</td>
						<td class="p-3">{segmentDetail(segment, segment.finder)}</td>
						<td class="p-3"
							>{packs.find((pack) => pack.segment_id === segment.id)?.status ?? segment.status}</td
						>
						<td class="space-x-2 p-3">
							<button class="underline" on:click={() => (selected = segment)}>Details</button>
							{#if role !== 'held_out'}
								<!-- The rule accepts only a bare resolve() call; this link adds a query to one. -->
								<!-- eslint-disable svelte/no-navigation-without-resolve -->
								<a class="underline" href={`${resolve('/lidar/tracks')}?${previewQuery(segment)}`}
									>Preview in scene player</a
								>
								<!-- eslint-enable svelte/no-navigation-without-resolve -->
							{/if}
							{#if !segment.replay_case_id}<button
									class="underline"
									disabled={busy || !segment.capture}
									on:click={() => makeCase(segment)}>Make case</button
								>{:else if !segment.job_id}<button
									class="underline"
									disabled={busy}
									on:click={() => queueClip(segment)}>Queue clip</button
								>{/if}
							<button
								class="underline"
								on:click={() => {
									dismissed = new Set([...dismissed, segment.id]);
								}}>Hide</button
							>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
		{#if !busy && !visible.length}<p class="p-4">No windows for this run and selector.</p>{/if}
	</div>

	{#if selected}
		<section class="rounded border p-4">
			<h2 class="text-lg font-semibold">Selected window</h2>
			<p>
				{captureName(selected.capture)} · {new Date(
					selected.window_start_unix_nanos / 1e6
				).toISOString()}
			</p>
			<p>Segment {selected.id} · {selected.status}</p>
			{#if selected.replay_case_id}<p>Replay case {selected.replay_case_id}</p>{/if}
			{#if selected.job_id}<p>Clip job {selected.job_id}</p>{/if}
			{#if selectedPack}<p>
					Pack: {selectedPack.pack_dir} · {selectedPack.proposal_layers} proposal layers · {selectedPack.reviewed_objects}
					reviewed objects · {selectedPack.reviewed_masks} reviewed masks
				</p>{/if}
			<p class="text-surface-content/70">
				Open the pack directory in the macOS annotation tool to review it.
			</p>
		</section>
	{/if}
</main>
