<script lang="ts">
	/**
	 * LiDAR Runs Page
	 *
	 * Lists analysis runs with expandable details showing parameters,
	 * associated scene, tracks summary, and links to the tracks view.
	 */
	import { getLabellingProgress, getLidarRuns, getLidarScenes, getRunTracks } from '$lib/api';
	import type { AnalysisRun, LabellingProgress, LidarScene, RunTrack } from '$lib/types/lidar';
	import { onMount } from 'svelte';
	import { Button } from 'svelte-ux';

	let runs: AnalysisRun[] = [];
	let scenes: LidarScene[] = [];
	let loading = true;
	let error: string | null = null;

	// Expanded run details
	let expandedRunId: string | null = null;
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

	function findSceneForRun(run: AnalysisRun): LidarScene | null {
		// Match by reference_run_id or source_path
		const byRef = scenes.find((s) => s.reference_run_id === run.run_id);
		if (byRef) return byRef;
		// Match by pcap_file and sensor_id
		if (run.source_path) {
			return (
				scenes.find((s) => s.pcap_file === run.source_path && s.sensor_id === run.sensor_id) ?? null
			);
		}
		return null;
	}

	async function toggleExpand(runId: string) {
		if (expandedRunId === runId) {
			expandedRunId = null;
			runTracks = [];
			labellingProgress = null;
			return;
		}
		expandedRunId = runId;
		tracksLoading = true;
		try {
			runTracks = await getRunTracks(runId);
			try {
				labellingProgress = await getLabellingProgress(runId);
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
			<Button variant="outline" on:click={loadData} disabled={loading}>
				{loading ? 'Loading...' : 'Refresh'}
			</Button>
		</div>
	</div>

	<!-- Content -->
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
			<div class="space-y-3">
				{#each runs as run (run.run_id)}
					{@const scene = findSceneForRun(run)}
					{@const isExpanded = expandedRunId === run.run_id}
					<div class="bg-surface-100 border-surface-content/10 overflow-hidden rounded-lg border">
						<!-- Run summary row -->
						<button
							class="hover:bg-surface-200/50 w-full px-5 py-4 text-left transition-colors"
							on:click={() => toggleExpand(run.run_id)}
						>
							<div class="flex items-center gap-4">
								<!-- Status badge -->
								<span class="rounded px-2 py-0.5 text-xs font-medium {statusColour(run.status)}">
									{run.status}
								</span>

								<!-- Run ID -->
								<span class="text-surface-content font-mono text-sm">
									{run.run_id.substring(0, 12)}
								</span>

								<!-- Track counts -->
								<span class="text-surface-content/60 text-sm">
									{run.total_tracks} tracks ({run.confirmed_tracks} confirmed)
								</span>

								<!-- Source type -->
								<span class="text-surface-content/50 rounded bg-gray-100 px-2 py-0.5 text-xs">
									{run.source_type}
								</span>

								<!-- Scene link -->
								{#if scene}
									<span class="text-primary text-xs">
										Scene: {scene.description || scene.scene_id.substring(0, 8)}
									</span>
								{/if}

								<!-- Date -->
								<span class="text-surface-content/50 ml-auto text-xs">
									{formatDate(run.created_at)}
								</span>

								<!-- Expand indicator -->
								<span
									class="text-surface-content/40 text-sm transition-transform {isExpanded
										? 'rotate-90'
										: ''}"
								>
									&#9654;
								</span>
							</div>
						</button>

						<!-- Expanded details -->
						{#if isExpanded}
							<div class="border-surface-content/10 border-t px-5 py-4">
								<div class="grid grid-cols-2 gap-6 lg:grid-cols-3">
									<!-- Run info -->
									<div class="space-y-2">
										<h3 class="text-surface-content text-sm font-semibold">Run Details</h3>
										<dl class="text-sm">
											<div class="flex justify-between py-1">
												<dt class="text-surface-content/60">Run ID</dt>
												<dd class="text-surface-content font-mono text-xs">
													{run.run_id}
												</dd>
											</div>
											<div class="flex justify-between py-1">
												<dt class="text-surface-content/60">Sensor</dt>
												<dd class="text-surface-content">{run.sensor_id}</dd>
											</div>
											{#if run.source_path}
												<div class="flex justify-between py-1">
													<dt class="text-surface-content/60">Source</dt>
													<dd
														class="text-surface-content max-w-[200px] truncate text-xs"
														title={run.source_path}
													>
														{run.source_path}
													</dd>
												</div>
											{/if}
											<div class="flex justify-between py-1">
												<dt class="text-surface-content/60">Duration</dt>
												<dd class="text-surface-content">
													{formatDuration(run.duration_secs)}
												</dd>
											</div>
											{#if run.processing_time_ms}
												<div class="flex justify-between py-1">
													<dt class="text-surface-content/60">Processing</dt>
													<dd class="text-surface-content">
														{(run.processing_time_ms / 1000).toFixed(1)}s
													</dd>
												</div>
											{/if}
											{#if run.total_frames}
												<div class="flex justify-between py-1">
													<dt class="text-surface-content/60">Frames</dt>
													<dd class="text-surface-content">
														{run.total_frames.toLocaleString()}
													</dd>
												</div>
											{/if}
											{#if run.total_clusters}
												<div class="flex justify-between py-1">
													<dt class="text-surface-content/60">Clusters</dt>
													<dd class="text-surface-content">
														{run.total_clusters.toLocaleString()}
													</dd>
												</div>
											{/if}
											{#if run.error_message}
												<div class="mt-2 rounded bg-red-50 px-2 py-1 text-xs text-red-600">
													{run.error_message}
												</div>
											{/if}
										</dl>
									</div>

									<!-- Scene info -->
									<div class="space-y-2">
										<h3 class="text-surface-content text-sm font-semibold">Scene</h3>
										{#if scene}
											<dl class="text-sm">
												<div class="flex justify-between py-1">
													<dt class="text-surface-content/60">Description</dt>
													<dd class="text-surface-content">
														{scene.description || '-'}
													</dd>
												</div>
												<div class="flex justify-between py-1">
													<dt class="text-surface-content/60">PCAP</dt>
													<dd
														class="text-surface-content max-w-[200px] truncate text-xs"
														title={scene.pcap_file}
													>
														{scene.pcap_file}
													</dd>
												</div>
												{#if scene.pcap_start_secs != null}
													<div class="flex justify-between py-1">
														<dt class="text-surface-content/60">Start</dt>
														<dd class="text-surface-content">
															{scene.pcap_start_secs}s
														</dd>
													</div>
												{/if}
												{#if scene.pcap_duration_secs != null}
													<div class="flex justify-between py-1">
														<dt class="text-surface-content/60">Duration</dt>
														<dd class="text-surface-content">
															{scene.pcap_duration_secs}s
														</dd>
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
									<div class="space-y-2">
										<h3 class="text-surface-content text-sm font-semibold">Tracks</h3>
										{#if tracksLoading}
											<p class="text-surface-content/50 text-sm">Loading tracks...</p>
										{:else}
											<dl class="text-sm">
												<div class="flex justify-between py-1">
													<dt class="text-surface-content/60">Total</dt>
													<dd class="text-surface-content">
														{runTracks.length}
													</dd>
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

											<!-- Link to tracks view -->
											<div class="pt-3">
												<!-- eslint-disable svelte/no-navigation-without-resolve -->
												<a
													href="/app/lidar/tracks"
													class="bg-primary text-primary-content inline-block rounded px-3 py-1.5 text-sm font-medium transition-opacity hover:opacity-90"
												>
													View Tracks
												</a>
											</div>
										{/if}
									</div>
								</div>

								<!-- Parameters (collapsible) -->
								{#if run.params_json}
									<details class="mt-4">
										<summary class="text-surface-content/70 cursor-pointer text-sm font-medium">
											Parameters
										</summary>
										<pre
											class="bg-surface-200 text-surface-content mt-2 max-h-[300px] overflow-auto rounded p-3 font-mono text-xs">{JSON.stringify(
												run.params_json,
												null,
												2
											)}</pre>
									</details>
								{/if}
							</div>
						{/if}
					</div>
				{/each}
			</div>
		{/if}
	</div>
</main>
