<script lang="ts">
	/**
	 * LiDAR Runs Page
	 *
	 * Table layout with a detail panel that slides out to the right,
	 * matching the scenes page layout pattern.
	 */
	import {
		deleteRun,
		deleteRunTrack,
		getLabellingProgress,
		getLidarRuns,
		getLidarScenes,
		getRunTracks
	} from '$lib/api';
	import type { AnalysisRun, LabellingProgress, LidarScene, RunTrack } from '$lib/types/lidar';
	import { onMount } from 'svelte';
	import { Button } from 'svelte-ux';

	let runs: AnalysisRun[] = [];
	let scenes: LidarScene[] = [];
	let loading = true;
	let error: string | null = null;

	// Selected run for detail panel
	let selectedRun: AnalysisRun | null = null;
	let runTracks: RunTrack[] = [];
	let labellingProgress: LabellingProgress | null = null;
	let tracksLoading = false;

	async function loadData() {
		loading = true;
		error = null;
		try {
			const [runsResult, scenesResult] = await Promise.all([getLidarRuns(), getLidarScenes()]);
			runs = runsResult;
			scenes = scenesResult;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load data';
		} finally {
			loading = false;
		}
	}

	async function handleDeleteRun(run: AnalysisRun) {
		if (
			!confirm(
				`Are you sure you want to delete run ${run.run_id.substring(0, 8)}? This action cannot be undone and will delete the run and all its tracks.`
			)
		) {
			return;
		}

		try {
			await deleteRun(run.run_id);
			await loadData();
			// If the selected run was deleted, clear the selection
			if (selectedRun && selectedRun.run_id === run.run_id) {
				deselectRun();
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to delete run';
		}
	}

	async function handleDeleteTrack(runId: string, trackId: string) {
		if (
			!confirm(`Are you sure you want to delete track ${trackId}? This action cannot be undone.`)
		) {
			return;
		}

		try {
			await deleteRunTrack(runId, trackId);
			// Reload tracks for the current run
			if (selectedRun && selectedRun.run_id === runId) {
				runTracks = runTracks.filter((t) => t.track_id !== trackId);
				// Reload labelling progress
				try {
					labellingProgress = await getLabellingProgress(runId);
				} catch {
					labellingProgress = null;
				}
			}
		} catch (e) {
			alert(e instanceof Error ? e.message : 'Failed to delete track');
		}
	}

	function handleKeyboardActivation(e: KeyboardEvent, action: () => void) {
		if (e.key === 'Enter' || e.key === ' ') {
			e.preventDefault();
			action();
		}
	}

	function findSceneForRun(run: AnalysisRun): LidarScene | null {
		const byRef = scenes.find((s) => s.reference_run_id === run.run_id);
		if (byRef) return byRef;
		if (run.source_path) {
			return (
				scenes.find((s) => s.pcap_file === run.source_path && s.sensor_id === run.sensor_id) ?? null
			);
		}
		return null;
	}

	async function selectRun(run: AnalysisRun) {
		selectedRun = run;
		tracksLoading = true;
		runTracks = [];
		labellingProgress = null;
		try {
			runTracks = await getRunTracks(run.run_id);
			try {
				labellingProgress = await getLabellingProgress(run.run_id);
			} catch {
				labellingProgress = null;
			}
		} catch {
			runTracks = [];
			labellingProgress = null;
		} finally {
			tracksLoading = false;
		}
	}

	function deselectRun() {
		selectedRun = null;
		runTracks = [];
		labellingProgress = null;
	}

	/** Extract filename from path (strip directory prefix). */
	function basename(path: string): string {
		const i = path.lastIndexOf('/');
		return i >= 0 ? path.substring(i + 1) : path;
	}

	function formatDate(iso: string): string {
		if (!iso) return '-';
		return new Date(iso).toLocaleString();
	}

	function formatDuration(secs: number | undefined): string {
		if (secs == null || secs === 0) return '-';
		if (secs < 60) return `${secs.toFixed(1)}s`;
		if (secs < 3600) return `${Math.floor(secs / 60)}m ${Math.round(secs % 60)}s`;
		return `${Math.floor(secs / 3600)}h ${Math.round((secs % 3600) / 60)}m`;
	}

	function statusColour(status: string): string {
		switch (status) {
			case 'completed':
				return 'bg-green-100 text-green-700';
			case 'running':
				return 'bg-blue-100 text-blue-700';
			case 'failed':
				return 'bg-red-100 text-red-700';
			default:
				return 'bg-gray-100 text-gray-700';
		}
	}

	/** Build href for the tracks page, passing scene and run IDs as query params. */
	function tracksHref(run: AnalysisRun, scene: LidarScene | null): string {
		const parts: string[] = [];
		if (scene) parts.push(`scene_id=${encodeURIComponent(scene.scene_id)}`);
		parts.push(`run_id=${encodeURIComponent(run.run_id)}`);
		return `/app/lidar/tracks?${parts.join('&')}`;
	}

	onMount(loadData);
</script>

<main id="main-content" class="bg-surface-200 flex h-[calc(100vh-64px)] flex-col overflow-hidden">
	<!-- Header -->
	<div class="border-surface-content/10 bg-surface-100 flex-none border-b px-6 py-4">
		<div class="flex items-center justify-between">
			<div>
				<h1 class="text-surface-content text-2xl font-semibold">LiDAR Runs</h1>
				<p class="text-surface-content/60 mt-1 text-sm">
					Analysis runs with parameters, scenes, and track summaries
				</p>
			</div>
			<div class="flex gap-2">
				<Button variant="outline" on:click={loadData} disabled={loading}>
					{loading ? 'Loading...' : 'Refresh'}
				</Button>
			</div>
		</div>
	</div>

	<div class="flex flex-1 overflow-hidden">
		<!-- Left: Run List -->
		<div class="flex-1 overflow-y-auto p-6">
			{#if error}
				<div class="mb-4 rounded bg-red-50 px-4 py-3 text-sm text-red-600">
					{error}
					<button class="ml-2 underline" on:click={loadData}>Retry</button>
				</div>
			{/if}

			{#if loading}
				<div class="text-surface-content/50 py-12 text-center">Loading runs...</div>
			{:else if runs.length === 0}
				<div class="text-surface-content/50 py-12 text-center">
					<p>No analysis runs found.</p>
					<p class="mt-1 text-sm">
						Runs are created when a scene is replayed or live analysis is started.
					</p>
				</div>
			{:else}
				<div class="bg-surface-100 border-surface-content/10 overflow-hidden rounded-lg border">
					<table class="w-full">
						<thead>
							<tr class="border-surface-content/10 border-b">
								<th class="text-surface-content/70 px-4 py-3 text-left text-sm font-medium"
									>Status</th
								>
								<th class="text-surface-content/70 px-4 py-3 text-left text-sm font-medium"
									>Source</th
								>
								<th class="text-surface-content/70 px-4 py-3 text-left text-sm font-medium"
									>Tracks</th
								>
								<th class="text-surface-content/70 px-4 py-3 text-left text-sm font-medium"
									>Scene</th
								>
								<th class="text-surface-content/70 px-4 py-3 text-left text-sm font-medium"
									>Created</th
								>
								<th class="text-surface-content/70 px-4 py-3 text-center text-sm font-medium"
									>Actions</th
								>
							</tr>
						</thead>
						<tbody>
							{#each runs as run (run.run_id)}
								{@const scene = findSceneForRun(run)}
								{@const isSelected = selectedRun?.run_id === run.run_id}
								<tr
									class="border-surface-content/10 hover:bg-surface-200/50 border-b transition-colors last:border-b-0 {isSelected
										? 'bg-primary/5'
										: ''}"
								>
									<td
										class="cursor-pointer px-4 py-3"
										on:click={() => selectRun(run)}
										on:keydown={(e) => handleKeyboardActivation(e, () => selectRun(run))}
										role="button"
										tabindex="0"
									>
										<span
											class="rounded px-2 py-0.5 text-xs font-medium {statusColour(run.status)}"
										>
											{run.status}
										</span>
									</td>
									<td
										class="text-surface-content cursor-pointer px-4 py-3 text-sm"
										on:click={() => selectRun(run)}
										on:keydown={(e) => handleKeyboardActivation(e, () => selectRun(run))}
										role="button"
										tabindex="0"
									>
										{run.source_path ? basename(run.source_path) : run.source_type}
									</td>
									<td
										class="text-surface-content/70 cursor-pointer px-4 py-3 text-sm"
										on:click={() => selectRun(run)}
										on:keydown={(e) => handleKeyboardActivation(e, () => selectRun(run))}
										role="button"
										tabindex="0"
									>
										{run.total_tracks} / {run.confirmed_tracks}
									</td>
									<td
										class="text-surface-content/70 cursor-pointer px-4 py-3 text-sm"
										on:click={() => selectRun(run)}
										on:keydown={(e) => handleKeyboardActivation(e, () => selectRun(run))}
										role="button"
										tabindex="0"
									>
										{scene ? scene.description || scene.scene_id.substring(0, 8) : '-'}
									</td>
									<td
										class="text-surface-content/70 cursor-pointer px-4 py-3 text-sm"
										on:click={() => selectRun(run)}
										on:keydown={(e) => handleKeyboardActivation(e, () => selectRun(run))}
										role="button"
										tabindex="0"
									>
										{formatDate(run.created_at)}
									</td>
									<td class="px-4 py-3 text-center">
										<button
											type="button"
											class="text-surface-content/40 rounded px-2 py-1 text-xs transition-colors hover:bg-red-50 hover:text-red-600"
											on:click|stopPropagation={() => handleDeleteRun(run)}
											title="Delete run"
										>
											Delete
										</button>
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}
		</div>

		<!-- Right: Detail Panel -->
		{#if selectedRun}
			{@const scene = findSceneForRun(selectedRun)}
			<div
				class="border-surface-content/10 bg-surface-100 w-[400px] flex-none overflow-y-auto border-l p-6"
			>
				<div class="mb-4 flex items-center justify-between">
					<h2 class="text-surface-content text-lg font-semibold">Run Details</h2>
					<button
						class="text-surface-content/50 hover:text-surface-content text-sm"
						on:click={deselectRun}
					>
						Close
					</button>
				</div>

				<div class="text-surface-content/50 mb-4 font-mono text-xs">
					{selectedRun.run_id}
				</div>

				<div class="space-y-4">
					<!-- Status -->
					<div>
						<span
							class="rounded px-2 py-0.5 text-xs font-medium {statusColour(selectedRun.status)}"
						>
							{selectedRun.status}
						</span>
					</div>

					<!-- Source -->
					{#if selectedRun.source_path}
						<div>
							<div class="text-surface-content/70 mb-1 block text-sm font-medium">Source</div>
							<div
								class="text-surface-content bg-surface-200 rounded px-3 py-2 font-mono text-xs break-all"
							>
								{selectedRun.source_path}
							</div>
						</div>
					{/if}

					<!-- Core fields -->
					<dl class="text-sm">
						<div class="flex justify-between py-1">
							<dt class="text-surface-content/60">Sensor</dt>
							<dd class="text-surface-content font-mono text-xs">{selectedRun.sensor_id}</dd>
						</div>
						<div class="flex justify-between py-1">
							<dt class="text-surface-content/60">Duration</dt>
							<dd class="text-surface-content">{formatDuration(selectedRun.duration_secs)}</dd>
						</div>
						{#if selectedRun.processing_time_ms}
							<div class="flex justify-between py-1">
								<dt class="text-surface-content/60">Processing</dt>
								<dd class="text-surface-content">
									{(selectedRun.processing_time_ms / 1000).toFixed(1)}s
								</dd>
							</div>
						{/if}
						{#if selectedRun.total_frames}
							<div class="flex justify-between py-1">
								<dt class="text-surface-content/60">Frames</dt>
								<dd class="text-surface-content">
									{selectedRun.total_frames.toLocaleString()}
								</dd>
							</div>
						{/if}
						{#if selectedRun.total_clusters}
							<div class="flex justify-between py-1">
								<dt class="text-surface-content/60">Clusters</dt>
								<dd class="text-surface-content">
									{selectedRun.total_clusters.toLocaleString()}
								</dd>
							</div>
						{/if}
					</dl>

					{#if selectedRun.error_message}
						<div class="rounded bg-red-50 px-3 py-2 text-xs text-red-600">
							{selectedRun.error_message}
						</div>
					{/if}

					<!-- Scene info -->
					<div>
						<div class="text-surface-content/70 mb-1 block text-sm font-medium">Scene</div>
						{#if scene}
							<dl class="text-sm">
								<div class="flex justify-between py-1">
									<dt class="text-surface-content/60">Description</dt>
									<dd class="text-surface-content">{scene.description || '-'}</dd>
								</div>
								<div class="flex justify-between py-1">
									<dt class="text-surface-content/60">PCAP</dt>
									<dd class="text-surface-content font-mono text-xs break-all">
										{scene.pcap_file}
									</dd>
								</div>
								{#if scene.pcap_start_secs != null}
									<div class="flex justify-between py-1">
										<dt class="text-surface-content/60">Start</dt>
										<dd class="text-surface-content">{scene.pcap_start_secs}s</dd>
									</div>
								{/if}
								{#if scene.pcap_duration_secs != null}
									<div class="flex justify-between py-1">
										<dt class="text-surface-content/60">Duration</dt>
										<dd class="text-surface-content">{scene.pcap_duration_secs}s</dd>
									</div>
								{/if}
								{#if scene.reference_run_id}
									<div class="flex justify-between py-1">
										<dt class="text-surface-content/60">Ref. Run</dt>
										<dd class="text-surface-content font-mono text-xs">
											{scene.reference_run_id.substring(0, 12)}
										</dd>
									</div>
								{/if}
							</dl>
						{:else}
							<p class="text-surface-content/50 text-sm">No associated scene found</p>
						{/if}
					</div>

					<!-- Track summary & labelling -->
					<div>
						<div class="text-surface-content/70 mb-1 block text-sm font-medium">Tracks</div>
						{#if tracksLoading}
							<p class="text-surface-content/50 text-sm">Loading tracks...</p>
						{:else}
							<dl class="text-sm">
								<div class="flex justify-between py-1">
									<dt class="text-surface-content/60">Total</dt>
									<dd class="text-surface-content">{runTracks.length}</dd>
								</div>
								<div class="flex justify-between py-1">
									<dt class="text-surface-content/60">Confirmed</dt>
									<dd class="text-surface-content">
										{runTracks.filter((t) => t.track_state === 'confirmed').length}
									</dd>
								</div>
								{#if labellingProgress}
									<div class="flex justify-between py-1">
										<dt class="text-surface-content/60">Labelled</dt>
										<dd class="text-surface-content">
											{labellingProgress.labelled}/{labellingProgress.total}
											({labellingProgress.progress_pct.toFixed(0)}%)
										</dd>
									</div>
									{#if Object.keys(labellingProgress.by_class).length > 0}
										<div class="mt-2">
											<dt class="text-surface-content/60 mb-1 text-xs">Labels</dt>
											<div class="flex flex-wrap gap-1">
												{#each Object.entries(labellingProgress.by_class) as [cls, count] (cls)}
													<span
														class="bg-surface-200 text-surface-content rounded px-2 py-0.5 text-xs"
													>
														{cls}: {count}
													</span>
												{/each}
											</div>
										</div>
									{/if}
								{/if}
							</dl>

							<!-- Track List with Delete buttons -->
							{#if runTracks.length > 0}
								<div class="mt-4">
									<dt class="text-surface-content/70 mb-2 text-sm font-medium">
										Track List ({runTracks.length})
									</dt>
									<div class="bg-surface-200 max-h-[300px] space-y-1 overflow-y-auto rounded p-2">
										{#each runTracks as track (track.track_id)}
											<div
												class="bg-surface-100 flex items-center justify-between rounded px-2 py-1.5 text-xs"
											>
												<div class="flex-1">
													<span class="text-surface-content/70 font-mono"
														>{track.track_id.substring(0, 12)}</span
													>
													<span
														class="text-surface-content/50 ml-2 rounded px-1.5 py-0.5 text-[10px] {track.track_state ===
														'confirmed'
															? 'bg-green-100 text-green-700'
															: 'bg-gray-100 text-gray-700'}"
													>
														{track.track_state}
													</span>
												</div>
												<button
													type="button"
													class="text-surface-content/40 ml-2 rounded px-1.5 py-0.5 text-xs transition-colors hover:bg-red-50 hover:text-red-600"
													on:click={() =>
														selectedRun && handleDeleteTrack(selectedRun.run_id, track.track_id)}
													title="Delete track"
												>
													Delete
												</button>
											</div>
										{/each}
									</div>
								</div>
							{/if}

							<!-- Link to tracks view -->
							<div class="pt-3">
								<!-- eslint-disable svelte/no-navigation-without-resolve -->
								<a
									href={tracksHref(selectedRun, scene)}
									class="bg-primary text-primary-content inline-block rounded px-3 py-1.5 text-sm font-medium transition-opacity hover:opacity-90"
								>
									View Tracks
								</a>
							</div>
						{/if}
					</div>

					<!-- Parameters (collapsible) -->
					{#if selectedRun.params_json}
						<details>
							<summary class="text-surface-content/70 cursor-pointer text-sm font-medium">
								Parameters
							</summary>
							<pre
								class="bg-surface-200 text-surface-content mt-2 max-h-[300px] overflow-auto rounded p-3 font-mono text-xs">{JSON.stringify(
									selectedRun.params_json,
									null,
									2
								)}</pre>
						</details>
					{/if}
				</div>
			</div>
		{/if}
	</div>
</main>
