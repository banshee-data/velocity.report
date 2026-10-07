<script lang="ts">
	/**
	 * Segments — ranked windows of a run's captures, from which clips are made.
	 *
	 * A selector ranks the run's windows; a window worth labelling becomes a
	 * clip, and a clip's pack is cut for review in the macOS annotation tool.
	 * The API still calls the clip a replay case, and the job that cuts its pack
	 * a clip job, until the vocabulary renames land with their aliases.
	 */
	import { resolve } from '$app/paths';
	import { getLidarRuns } from '#lib/api.js';
	import { clipQuery, runQuery, tracksQuery } from '#lib/lidarLinks.js';
	import {
		keepSelector,
		requirementText,
		scoreText,
		segmentDetail,
		segmentQuery,
		selectorGroups,
		statusText,
		type SegmentSelector
	} from '#lib/segments.js';
	import type { AnalysisRun } from '#lib/types/lidar.js';
	import { onMount } from 'svelte';
	import { Button } from 'svelte-ux';

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
	// The clip the last "Make clip" produced, linked beside its message.
	let madeClipID = '';
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
	// offering "Make clip" for a window that already has one.
	function updateSegment(id: string, changes: Partial<Segment>) {
		segments = segments.map((entry) => (entry.id === id ? { ...entry, ...changes } : entry));
		if (selected?.id === id) selected = segments.find((entry) => entry.id === id) ?? null;
	}

	async function makeCase(segment: Segment) {
		busy = true;
		error = '';
		madeClipID = '';
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
			message = `Clip ${result.replay_case_id} is ready.`;
			madeClipID = result.replay_case_id;
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not make the clip.';
		} finally {
			busy = false;
		}
	}

	// Cuts the clip's annotation pack. The route and the job kind still say
	// clip; V18 of the vocabulary plan renames them pack.
	async function queuePack(segment: Segment) {
		if (!segment.replay_case_id) return;
		busy = true;
		error = '';
		madeClipID = '';
		try {
			const result = await api(
				`/api/lidar/scenes/${encodeURIComponent(segment.replay_case_id)}/clip`,
				{ method: 'POST' }
			);
			updateSegment(segment.id, { job_id: result.job.job_id, status: 'clipping' });
			message = `Pack job ${result.job.job_id} queued.`;
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not queue the pack.';
		} finally {
			busy = false;
		}
	}

	function previewQuery(segment: Segment) {
		const run = runs.find((item) => item.run_id === runID);
		return tracksQuery({
			sensorId: run?.sensor_id || 'hesai-pandar40p',
			clipId: segment.replay_case_id,
			runId: runID,
			atNs: segment.peak_timestamp_ns || segment.window_start_unix_nanos
		});
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

<svelte:head>
	<title>Segments — velocity.report</title>
</svelte:head>

<main id="main-content" class="vr-page">
	<div class="vr-toolbar">
		<div class="flex items-center justify-between">
			<div>
				<h1 class="text-surface-content text-2xl font-semibold">Segments</h1>
				<p class="text-surface-content/60 mt-1 text-sm">
					Windows of a run's captures, ranked by a selector. Make a
					<a href={resolve('/lidar/replay-cases')} class="text-primary hover:underline">clip</a>
					from one worth labelling, then queue its pack for review in the macOS annotation tool.
				</p>
			</div>
			<Button variant="outline" on:click={load} disabled={busy}>
				{busy ? 'Loading...' : 'Refresh'}
			</Button>
		</div>
	</div>

	<div class="flex-1 space-y-4 overflow-y-auto p-6">
		<div class="flex flex-wrap items-end gap-4 text-sm">
			<label class="text-surface-content/70 flex flex-col gap-1 font-medium"
				>Run
				<select
					class="border-surface-300 bg-surface-100 text-surface-content rounded border px-2 py-1.5 font-mono text-xs"
					bind:value={runID}
					on:change={load}
				>
					{#each runs as run (run.run_id)}<option value={run.run_id}>{run.run_id}</option>{/each}
				</select>
			</label>
			<label class="text-surface-content/70 flex flex-col gap-1 font-medium"
				>Pack role
				<select
					class="border-surface-300 bg-surface-100 text-surface-content rounded border px-2 py-1.5"
					bind:value={role}
					on:change={changeRole}
				>
					<option value="tuning">Tuning</option><option value="held_out">Held out</option>
				</select>
			</label>
			<label class="text-surface-content/70 flex flex-col gap-1 font-medium"
				>Selector
				<select
					class="border-surface-300 bg-surface-100 text-surface-content rounded border px-2 py-1.5"
					bind:value={selectorID}
					on:change={load}
				>
					{#each groups as group (group.category)}
						<optgroup label={group.category}>
							{#each group.selectors as entry (entry.id)}<option value={entry.id}
									>{entry.label}</option
								>{/each}
						</optgroup>
					{/each}
				</select>
			</label>
			{#if runID}
				<!-- eslint-disable svelte/no-navigation-without-resolve -->
				<a
					href={`${resolve('/lidar/runs')}?${runQuery(runID)}`}
					class="text-primary pb-2 hover:underline">Open run →</a
				>
				<!-- eslint-enable svelte/no-navigation-without-resolve -->
			{/if}
		</div>
		{#if chosen}
			<p class="text-surface-content/70 text-sm">
				{chosen.description} Ranked by {scoreText(chosen)}{#if chosen.require.length}; requires {requirementText(
						chosen
					)}{/if}.
			</p>
		{/if}
		{#if role === 'held_out'}
			<p class="rounded bg-amber-100 p-3 text-sm text-amber-950">
				Choose a random window from each capture first, then add traffic windows. Tracker previews
				are hidden for blind review.
			</p>
		{/if}
		{#if error}<p role="alert" class="rounded bg-red-50 px-4 py-3 text-sm text-red-600">
				{error}
			</p>{/if}
		{#if message}<p role="status" class="text-surface-content text-sm">
				{message}
				{#if madeClipID}
					<!-- eslint-disable svelte/no-navigation-without-resolve -->
					<a
						href={`${resolve('/lidar/replay-cases')}?${clipQuery(madeClipID)}`}
						class="text-primary ml-1 hover:underline">Open clip →</a
					>
					<!-- eslint-enable svelte/no-navigation-without-resolve -->
				{/if}
			</p>{/if}

		{#if stripURL && segments.length}
			<section
				aria-label="Per-capture score strip"
				class="border-surface-content/10 bg-surface-100 overflow-x-auto rounded-lg border p-3"
			>
				<img src={stripURL} alt="Segment scores grouped by capture" />
			</section>
		{/if}

		<div class="border-surface-content/10 bg-surface-100 overflow-x-auto rounded-lg border">
			<table class="w-full text-left text-sm">
				<thead
					><tr class="text-surface-content/70"
						><th class="p-3 font-medium">Window UTC</th><th class="p-3 font-medium"
							>Capture / offset</th
						><th class="p-3 font-medium">Score</th><th class="p-3 font-medium">Finder detail</th><th
							class="p-3 font-medium">Status</th
						><th class="p-3 font-medium">Actions</th></tr
					></thead
				>
				<tbody>
					{#each visible as segment (segment.id)}
						<tr class="border-surface-content/10 border-t">
							<td class="p-3">{new Date(segment.window_start_unix_nanos / 1e6).toISOString()}</td>
							<td class="p-3"
								>{captureName(segment.capture)}{#if segment.capture}
									@ {segment.offset_seconds?.toFixed(1)} s{/if}</td
							>
							<td class="p-3">{segment.score.toFixed(2)}</td>
							<td class="p-3">{segmentDetail(segment, segment.finder)}</td>
							<td class="p-3"
								>{statusText(
									packs.find((pack) => pack.segment_id === segment.id)?.status ?? segment.status
								)}</td
							>
							<td class="text-primary space-x-3 p-3 whitespace-nowrap">
								<button class="hover:underline" on:click={() => (selected = segment)}
									>Details</button
								>
								{#if role !== 'held_out'}
									<!-- The rule accepts only a bare resolve() call; this link adds a query to one. -->
									<!-- eslint-disable svelte/no-navigation-without-resolve -->
									<a
										class="hover:underline"
										href={`${resolve('/lidar/tracks')}?${previewQuery(segment)}`}
										>Preview in Tracks</a
									>
									<!-- eslint-enable svelte/no-navigation-without-resolve -->
								{/if}
								{#if !segment.replay_case_id}<button
										class="hover:underline disabled:opacity-40"
										disabled={busy || !segment.capture}
										on:click={() => makeCase(segment)}>Make clip</button
									>{:else}
									<!-- eslint-disable svelte/no-navigation-without-resolve -->
									<a
										class="hover:underline"
										href={`${resolve('/lidar/replay-cases')}?${clipQuery(segment.replay_case_id)}`}
										>Open clip</a
									>
									<!-- eslint-enable svelte/no-navigation-without-resolve -->
									{#if !segment.job_id}<button
											class="hover:underline disabled:opacity-40"
											disabled={busy}
											on:click={() => queuePack(segment)}>Queue pack</button
										>{/if}
								{/if}
								<button
									class="hover:underline"
									on:click={() => {
										dismissed = new Set([...dismissed, segment.id]);
									}}>Hide</button
								>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
			{#if !busy && !visible.length}<p class="text-surface-content/50 p-4 text-sm">
					No windows for this run and selector.
				</p>{/if}
		</div>

		{#if selected}
			<section
				class="border-surface-content/10 bg-surface-100 space-y-1 rounded-lg border p-4 text-sm"
			>
				<h2 class="text-surface-content text-lg font-semibold">Selected window</h2>
				<p>
					{captureName(selected.capture)} · {new Date(
						selected.window_start_unix_nanos / 1e6
					).toISOString()}
				</p>
				<p>Segment {selected.id} · {statusText(selected.status)}</p>
				{#if selected.replay_case_id}<p>
						Clip
						<!-- eslint-disable svelte/no-navigation-without-resolve -->
						<a
							href={`${resolve('/lidar/replay-cases')}?${clipQuery(selected.replay_case_id)}`}
							class="text-primary font-mono hover:underline">{selected.replay_case_id}</a
						>
						<!-- eslint-enable svelte/no-navigation-without-resolve -->
					</p>{/if}
				{#if selected.job_id}<p>Pack job {selected.job_id}</p>{/if}
				{#if selectedPack}<p>
						Pack: {selectedPack.pack_dir} · {selectedPack.proposal_layers} proposal layers · {selectedPack.reviewed_objects}
						reviewed objects · {selectedPack.reviewed_masks} reviewed masks
					</p>{/if}
				<p class="text-surface-content/70">
					Open the pack directory in the macOS annotation tool to review it.
				</p>
			</section>
		{/if}
	</div>
</main>
