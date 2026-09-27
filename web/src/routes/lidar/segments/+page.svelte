<script lang="ts">
	import { getLidarRuns } from '$lib/api';
	import type { AnalysisRun } from '$lib/types/lidar';
	import { onMount } from 'svelte';

	type Finder = { name: string; version: number; held_out: boolean };
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
	let finder = 'following';
	let finders: Finder[] = [];
	let segments: Segment[] = [];
	let packs: Pack[] = [];
	let dismissed = new Set<string>();
	let selected: Segment | null = null;
	let busy = false;
	let error = '';
	let message = '';
	let stripURL = '';

	$: available = finders.filter((entry) => role !== 'held_out' || entry.held_out);
	$: visible = segments.filter((entry) => !dismissed.has(entry.id));
	$: selectedPack = packs.find((pack) => pack.segment_id === selected?.id);

	function query() {
		return new URLSearchParams({ run_id: runID, finder, role }).toString();
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
		if (!available.some((entry) => entry.name === finder))
			finder = available[0]?.name ?? 'following';
		await load();
	}

	async function makeCase(segment: Segment) {
		busy = true;
		error = '';
		try {
			const result = await api(`/api/lidar/segments/${encodeURIComponent(segment.id)}/case`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ run_id: runID, finder, role })
			});
			segment.replay_case_id = result.replay_case_id;
			segment.status = 'case';
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
			segment.job_id = result.job.job_id;
			segment.status = 'clipping';
			message = `Clip job ${segment.job_id} queued.`;
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not queue clip.';
		} finally {
			busy = false;
		}
	}

	function previewURL(segment: Segment) {
		const run = runs.find((item) => item.run_id === runID);
		const params = new URLSearchParams({
			run_id: runID,
			sensor_id: run?.sensor_id || 'hesai-pandar40p',
			at_ns: String(segment.peak_timestamp_ns || segment.window_start_unix_nanos)
		});
		return `/app/lidar/tracks?${params.toString()}`;
	}

	function captureName(path?: string) {
		return path?.split(/[\\/]/).pop() || 'Unplaced';
	}

	onMount(async () => {
		try {
			const [listed, catalogue] = await Promise.all([
				getLidarRuns(),
				api('/api/lidar/segments/finders')
			]);
			runs = listed;
			finders = catalogue.finders;
			runID = runs[0]?.run_id ?? '';
			await load();
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not load segment finder.';
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
			<select class="rounded border p-2" bind:value={runID} onchange={load}>
				{#each runs as run}<option value={run.run_id}>{run.run_id}</option>{/each}
			</select>
		</label>
		<label class="flex flex-col gap-1"
			>Pack role
			<select class="rounded border p-2" bind:value={role} onchange={changeRole}>
				<option value="tuning">Tuning</option><option value="held_out">Held out</option>
			</select>
		</label>
		<label class="flex flex-col gap-1"
			>Finder
			<select class="rounded border p-2" bind:value={finder} onchange={load}>
				{#each available as entry}<option value={entry.name}>{entry.name}</option>{/each}
			</select>
		</label>
		<button class="rounded border px-4 py-2" onclick={load} disabled={busy}>Refresh</button>
	</div>
	{#if role === 'held_out'}
		<p class="rounded bg-amber-100 p-3 text-amber-950">
			Held-out windows use traffic or seeded random selection. Tracker previews are hidden for blind
			review.
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
						<td class="p-3"
							>{#if finder === 'following' || finder === 'leader_changes'}{segment.pair_seconds ??
									0} pair-s · {segment.pairs ?? 0} pairs · {segment.leaders ?? 0} leaders · {segment.leader_changes ??
									0} changes · {segment.closest_gap_m ?? 0} m{:else}{segment.events ?? 0} observations{/if}</td
						>
						<td class="p-3"
							>{packs.find((pack) => pack.segment_id === segment.id)?.status ?? segment.status}</td
						>
						<td class="space-x-2 p-3">
							<button class="underline" onclick={() => (selected = segment)}>Details</button>
							{#if role !== 'held_out'}<a class="underline" href={previewURL(segment)}
									>Preview in scene player</a
								>{/if}
							{#if !segment.replay_case_id}<button
									class="underline"
									disabled={busy || !segment.capture}
									onclick={() => makeCase(segment)}>Make case</button
								>{:else if !segment.job_id}<button
									class="underline"
									disabled={busy}
									onclick={() => queueClip(segment)}>Queue clip</button
								>{/if}
							<button
								class="underline"
								onclick={() => {
									dismissed = new Set([...dismissed, segment.id]);
								}}>Dismiss</button
							>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
		{#if !busy && !visible.length}<p class="p-4">No windows for this run and finder.</p>{/if}
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
