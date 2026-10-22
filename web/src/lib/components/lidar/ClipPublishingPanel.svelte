<script lang="ts">
	/**
	 * A clip's standalone extract and its place in the published dataset.
	 *
	 * A clip names its source captures and offsets; cutting an extract writes
	 * the window as one PCAPNG of whole rotations, which the clip can then
	 * replay instead of its sources. Staging links that file into the dataset
	 * clone and records it in the manifest. Uploading the dataset stays a
	 * person's step, outside this page.
	 */
	import { getClipExtracts, setClipPublishing, stageClip, startClipExtract } from '#lib/api.js';
	import { formatSize } from '#lib/captures/timeline.js';
	import type { ClipExtract } from '#lib/types/captures.js';
	import type { ClipStorage, LidarClip } from '#lib/types/lidar.js';
	import { onDestroy } from 'svelte';
	import { Button } from 'svelte-ux';

	export let clip: LidarClip;
	/** Called with the clip as the server returns it after a save. */
	export let onChange: (clip: LidarClip) => void = () => {};

	let extracts: ClipExtract[] = [];
	let loadedFor: string | null = null;
	let error: string | null = null;
	let note: string | null = null;
	let busy = false;
	let poll: ReturnType<typeof setTimeout> | null = null;

	let datasetId = '';
	let azimuth = '';
	let storage: ClipStorage = 'sources';

	$: if (clip.clip_id !== loadedFor) reset(clip);

	$: latest = extracts[0];
	$: completed = extracts.find((e) => e.state === 'completed' && e.rel_path);
	$: active = extracts.some((e) => e.state === 'queued' || e.state === 'running');
	$: stageBlocker = !clip.dataset_id
		? 'save a dataset id first'
		: !clip.location
			? 'locate the clip first'
			: !completed
				? 'cut an extract first'
				: null;

	function reset(next: LidarClip) {
		loadedFor = next.clip_id;
		datasetId = next.dataset_id ?? '';
		azimuth = next.north_azimuth_deg != null ? String(next.north_azimuth_deg) : '';
		storage = next.storage ?? 'sources';
		extracts = [];
		error = null;
		note = null;
		void load();
	}

	async function load() {
		const clipId = clip.clip_id;
		try {
			const listed = await getClipExtracts(clipId);
			if (clip.clip_id !== clipId) return;
			extracts = listed;
			schedule();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load the extracts.';
		}
	}

	/** Poll while an export is queued or running, and stop when none is. */
	function schedule() {
		if (poll) clearTimeout(poll);
		poll = null;
		if (extracts.some((e) => e.state === 'queued' || e.state === 'running')) {
			poll = setTimeout(load, 3000);
		}
	}

	onDestroy(() => {
		if (poll) clearTimeout(poll);
	});

	async function cut() {
		busy = true;
		error = null;
		note = null;
		try {
			const { created } = await startClipExtract(clip.clip_id);
			note = created ? 'Extract queued.' : 'An extract is already queued or running.';
			await load();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not queue the extract.';
		} finally {
			busy = false;
		}
	}

	async function save() {
		busy = true;
		error = null;
		note = null;
		const trimmed = azimuth.trim();
		const parsed = trimmed === '' ? null : Number(trimmed);
		if (parsed !== null && !Number.isFinite(parsed)) {
			error = 'The north azimuth must be a number of degrees.';
			busy = false;
			return;
		}
		try {
			const saved = await setClipPublishing(clip.clip_id, {
				dataset_id: datasetId.trim(),
				north_azimuth_deg: parsed,
				storage
			});
			loadedFor = saved.clip_id;
			onChange(saved);
			note = 'Saved.';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not save the publishing settings.';
		} finally {
			busy = false;
		}
	}

	async function stage() {
		busy = true;
		error = null;
		note = null;
		try {
			const result = await stageClip(clip.clip_id);
			note = `Staged as ${result.raw_path}, with its manifest entry. Upload the dataset by hand.`;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not stage the clip.';
		} finally {
			busy = false;
		}
	}

	function percent(e: ClipExtract): string {
		if (!e.progress_total) return '';
		return `${Math.floor((100 * e.progress_current) / e.progress_total)}%`;
	}
</script>

<div class="space-y-3">
	<div class="text-surface-content/70 text-sm font-medium">Extract and dataset</div>

	{#if error}
		<div class="rounded bg-red-50 px-3 py-2 text-xs text-red-600">{error}</div>
	{/if}
	{#if note}
		<div class="rounded bg-emerald-50 px-3 py-2 text-xs text-emerald-700">{note}</div>
	{/if}

	<div class="bg-surface-200 rounded px-3 py-2 text-xs">
		{#if latest}
			<div class="flex justify-between gap-2">
				<span class="font-mono">{latest.state} {percent(latest)}</span>
				<span class="text-surface-content/50">{latest.detail ?? ''}</span>
			</div>
			{#if latest.error}
				<div class="mt-1 text-red-600">{latest.error}</div>
			{/if}
		{:else}
			<span class="text-surface-content/50">No extract yet: the clip replays its sources.</span>
		{/if}
		{#if completed}
			<div class="text-surface-content/70 mt-1 font-mono break-all">{completed.rel_path}</div>
			<div class="text-surface-content/50 mt-0.5 font-mono">
				{formatSize(completed.size_bytes ?? 0)} · {completed.frame_count} rotations · closed by
				{completed.closed_by} · {completed.sha256?.slice(0, 19)}…
			</div>
		{/if}
	</div>
	<Button size="sm" variant="outline" disabled={busy || active} on:click={cut}>
		{active ? 'Extract in progress…' : completed ? 'Cut a new extract' : 'Cut extract'}
	</Button>

	<div class="grid grid-cols-2 gap-2">
		<label class="text-surface-content/60 text-xs">
			Dataset id
			<input
				class="border-surface-content/20 bg-surface-50 mt-1 w-full rounded border px-2 py-1 font-mono text-xs"
				placeholder="kirkham-9th-k3"
				bind:value={datasetId}
			/>
		</label>
		<label class="text-surface-content/60 text-xs">
			North azimuth (°)
			<input
				class="border-surface-content/20 bg-surface-50 mt-1 w-full rounded border px-2 py-1 font-mono text-xs"
				placeholder="optional"
				bind:value={azimuth}
			/>
		</label>
	</div>
	<label class="text-surface-content/60 block text-xs">
		Replay from
		<select
			class="border-surface-content/20 bg-surface-50 mt-1 w-full rounded border px-2 py-1 text-xs"
			bind:value={storage}
		>
			<option value="sources">source captures and offsets</option>
			<option value="extract" disabled={!completed}>the latest extract</option>
		</select>
	</label>
	<div class="flex items-center justify-between gap-2">
		<Button size="sm" variant="outline" disabled={busy} on:click={save}>Save publishing</Button>
		<Button
			size="sm"
			variant="fill"
			color="primary"
			disabled={busy || stageBlocker !== null}
			on:click={stage}>Stage into dataset</Button
		>
	</div>
	{#if stageBlocker}
		<p class="text-surface-content/40 text-xs">To stage: {stageBlocker}.</p>
	{/if}
</div>
