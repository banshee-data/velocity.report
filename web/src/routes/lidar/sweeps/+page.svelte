<script lang="ts">
	/**
	 * LiDAR Sweeps Page
	 *
	 * Lists sweep/auto-tune runs with a detail panel showing recommendation,
	 * request config, and links to the sweep dashboard.
	 */
	import { applyLidarParams, getSweep, listSweeps } from '$lib/api';
	import type { SweepRecord, SweepSummary } from '$lib/types/lidar';
	import { onMount } from 'svelte';
	import { Button } from 'svelte-ux';

	/** Default sensor — matches the single-sensor pattern used elsewhere. */
	const SENSOR_ID = 'hesai-pandar40p';

	/** Score component metric definitions for display */
	const SCORE_METRICS: Array<[string, string]> = [
		['Detection Rate', 'detection_rate'],
		['Fragmentation', 'fragmentation'],
		['False Positives', 'false_positives'],
		['Velocity Coverage', 'velocity_coverage'],
		['Quality Premium', 'quality_premium'],
		['Truncation Rate', 'truncation_rate'],
		['Velocity Noise', 'velocity_noise_rate'],
		['Stopped Recovery', 'stopped_recovery']
	];

	let sweeps: SweepSummary[] = [];
	let loading = true;
	let error: string | null = null;

	// Detail panel state
	let selectedSweep: SweepRecord | null = null;
	let detailLoading = false;
	let applyStatus: string | null = null;

	// Paste-and-apply state
	let pasteJSON = '';
	let pasteError: string | null = null;
	let pasteApplying = false;

	// Metric keys to filter out when applying recommendation params
	const METRIC_KEYS = new Set([
		'score',
		'acceptance_rate',
		'misalignment_ratio',
		'alignment_deg',
		'nonzero_cells',
		'foreground_capture',
		'unbounded_point_ratio',
		'empty_box_ratio',
		'fragmentation_ratio',
		'heading_jitter_deg',
		'speed_jitter_mps'
	]);

	async function loadData() {
		loading = true;
		error = null;
		try {
			sweeps = await listSweeps(SENSOR_ID, 50);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load sweeps';
		} finally {
			loading = false;
		}
	}

	async function selectSweep(summary: SweepSummary) {
		detailLoading = true;
		applyStatus = null;
		try {
			selectedSweep = await getSweep(summary.sweep_id);
		} catch {
			selectedSweep = null;
		} finally {
			detailLoading = false;
		}
	}

	function deselectSweep() {
		selectedSweep = null;
		applyStatus = null;
	}

	function handleKeyboardActivation(e: KeyboardEvent, action: () => void) {
		if (e.key === 'Enter' || e.key === ' ') {
			e.preventDefault();
			action();
		}
	}

	/** Extract only tuning parameters from a recommendation object. */
	function extractTuningParams(rec: Record<string, unknown>): Record<string, unknown> {
		const params: Record<string, unknown> = {};
		for (const [k, v] of Object.entries(rec)) {
			if (!METRIC_KEYS.has(k)) {
				params[k] = v;
			}
		}
		return params;
	}

	async function applyRecommendation() {
		if (!selectedSweep?.recommendation) return;
		const params = extractTuningParams(selectedSweep.recommendation);
		if (Object.keys(params).length === 0) {
			applyStatus = 'No tuning parameters in recommendation';
			return;
		}
		applyStatus = 'Applying...';
		try {
			await applyLidarParams(SENSOR_ID, params);
			applyStatus = `Applied ${Object.keys(params).length} params ✓`;
		} catch (e) {
			applyStatus = `Failed: ${e instanceof Error ? e.message : String(e)}`;
		}
	}

	async function applyPasted() {
		pasteError = null;
		const raw = pasteJSON.trim();
		if (!raw) {
			pasteError = 'Paste a JSON object first.';
			return;
		}
		let parsed: Record<string, unknown>;
		try {
			parsed = JSON.parse(raw);
		} catch (e) {
			pasteError = `Invalid JSON: ${e instanceof Error ? e.message : String(e)}`;
			return;
		}
		if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
			pasteError = 'Expected a JSON object (not array or primitive).';
			return;
		}
		const params = extractTuningParams(parsed);
		if (Object.keys(params).length === 0) {
			pasteError = 'No tuning parameters found after filtering metric keys.';
			return;
		}
		pasteApplying = true;
		try {
			await applyLidarParams(SENSOR_ID, params);
			pasteError = null;
			pasteJSON = '';
			applyStatus = `Pasted & applied ${Object.keys(params).length} params ✓`;
		} catch (e) {
			pasteError = `Apply failed: ${e instanceof Error ? e.message : String(e)}`;
		} finally {
			pasteApplying = false;
		}
	}

	function formatDate(iso: string | undefined): string {
		if (!iso) return '-';
		return new Date(iso).toLocaleString();
	}

	function formatDuration(started: string, completed: string | undefined): string {
		if (!completed) return 'running';
		const ms = new Date(completed).getTime() - new Date(started).getTime();
		const secs = ms / 1000;
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

	function modeLabel(mode: string): string {
		switch (mode) {
			case 'auto':
				return 'Auto-Tune';
			case 'params':
				return 'Manual';
			case 'hint':
				return 'Human-in-the-Loop';
			default:
				return mode;
		}
	}

	/** Build URL to the sweep dashboard with the sweep pre-selected. */
	function dashboardUrl(sweepId: string): string {
		return `/debug/lidar/sweep?sweep_id=${encodeURIComponent(sweepId)}`;
	}

	/** Format a recommendation object as readable key=value lines. */
	function formatRecommendation(rec: Record<string, unknown>): {
		params: [string, string][];
		metrics: [string, string][];
	} {
		const params: [string, string][] = [];
		const metrics: [string, string][] = [];
		for (const [k, v] of Object.entries(rec)) {
			const formatted = typeof v === 'number' ? v.toFixed(4) : String(v);
			if (METRIC_KEYS.has(k)) {
				metrics.push([k, formatted]);
			} else {
				params.push([k, formatted]);
			}
		}
		return { params, metrics };
	}

	onMount(loadData);
</script>

<main id="main-content" class="bg-surface-200 flex h-[calc(100vh-64px)] flex-col overflow-hidden">
	<!-- Header -->
	<div class="border-surface-content/10 bg-surface-100 flex-none border-b px-6 py-4">
		<div class="flex items-center justify-between">
			<div>
				<h1 class="text-surface-content text-2xl font-semibold">LiDAR Sweeps</h1>
				<p class="text-surface-content/60 mt-1 text-sm">
					Parameter sweep and auto-tune history with recommendations
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
		<!-- Left: Sweep List -->
		<div class="flex-1 overflow-y-auto p-6">
			{#if error}
				<div class="mb-4 rounded bg-red-50 px-4 py-3 text-sm text-red-600">
					{error}
					<button class="ml-2 underline" on:click={loadData}>Retry</button>
				</div>
			{/if}

			{#if loading}
				<p class="text-surface-content/50 text-sm">Loading sweeps…</p>
			{:else if sweeps.length === 0}
				<p class="text-surface-content/50 text-sm">
					No sweeps found. Run a sweep from the
					<!-- eslint-disable svelte/no-navigation-without-resolve -->
					<a href="/debug/lidar/sweep" class="text-primary underline">sweep dashboard</a>.
				</p>
			{:else}
				<div class="space-y-2">
					{#each sweeps as sweep (sweep.sweep_id)}
						<div
							class="bg-surface-100 hover:bg-surface-100/80 cursor-pointer rounded-lg border p-4 transition-colors
								{selectedSweep?.sweep_id === sweep.sweep_id
								? 'border-primary ring-primary/20 ring-2'
								: 'border-surface-content/10'}"
							role="button"
							tabindex="0"
							on:click={() => selectSweep(sweep)}
							on:keydown={(e) => handleKeyboardActivation(e, () => selectSweep(sweep))}
						>
							<div class="flex items-center justify-between">
								<div class="flex items-center gap-3">
									<span
										class="rounded px-2 py-0.5 text-xs font-medium {statusColour(sweep.status)}"
									>
										{sweep.status}
									</span>
									<span class="text-surface-content/60 text-xs">
										{modeLabel(sweep.mode)}
									</span>
								</div>
								<span class="text-surface-content/50 text-xs">
									{formatDate(sweep.started_at)}
								</span>
							</div>
							<div class="mt-1 flex items-center justify-between">
								<code class="text-surface-content/70 text-xs"
									>{sweep.sweep_id.substring(0, 12)}</code
								>
								<span class="text-surface-content/50 text-xs">
									{formatDuration(sweep.started_at, sweep.completed_at)}
								</span>
							</div>
							{#if sweep.error}
								<p class="mt-1 truncate text-xs text-red-500">{sweep.error}</p>
							{/if}
						</div>
					{/each}
				</div>
			{/if}

			<!-- Paste & Apply Card -->
			<div class="bg-surface-100 border-surface-content/10 mt-6 rounded-lg border p-4">
				<h3 class="text-surface-content mb-2 text-sm font-semibold">Paste &amp; Apply Params</h3>
				<p class="text-surface-content/60 mb-2 text-xs">
					Paste a JSON object of tuning parameters. Metric keys are filtered automatically.
				</p>
				<textarea
					bind:value={pasteJSON}
					rows="6"
					class="bg-surface-200 border-surface-content/20 w-full rounded border p-2 font-mono text-xs"
					placeholder={'{"foreground_dbscan_eps": 0.3, "measurement_noise": 0.9}'}
				></textarea>
				{#if pasteError}
					<p class="mt-1 text-xs text-red-500">{pasteError}</p>
				{/if}
				<div class="mt-2">
					<Button
						variant="fill"
						color="primary"
						size="sm"
						on:click={applyPasted}
						disabled={pasteApplying}
					>
						{pasteApplying ? 'Applying…' : 'Apply to System'}
					</Button>
				</div>
			</div>
		</div>

		<!-- Right: Detail Panel -->
		{#if selectedSweep || detailLoading}
			<div
				class="bg-surface-100 border-surface-content/10 w-[480px] flex-none overflow-y-auto border-l p-6"
			>
				<div class="mb-4 flex items-center justify-between">
					<h2 class="text-surface-content text-lg font-semibold">Sweep Detail</h2>
					<button
						class="text-surface-content/50 hover:text-surface-content text-sm"
						on:click={deselectSweep}
					>
						✕
					</button>
				</div>

				{#if detailLoading}
					<p class="text-surface-content/50 text-sm">Loading…</p>
				{:else if selectedSweep}
					<!-- Status & Meta -->
					<div class="mb-4 space-y-1 text-sm">
						<div class="flex justify-between">
							<span class="text-surface-content/60">Status</span>
							<span
								class="rounded px-2 py-0.5 text-xs font-medium {statusColour(selectedSweep.status)}"
							>
								{selectedSweep.status}
							</span>
						</div>
						<div class="flex justify-between">
							<span class="text-surface-content/60">Mode</span>
							<span>{modeLabel(selectedSweep.mode)}</span>
						</div>
						<div class="flex justify-between">
							<span class="text-surface-content/60">Started</span>
							<span>{formatDate(selectedSweep.started_at)}</span>
						</div>
						<div class="flex justify-between">
							<span class="text-surface-content/60">Duration</span>
							<span>{formatDuration(selectedSweep.started_at, selectedSweep.completed_at)}</span>
						</div>
						<div class="flex justify-between">
							<span class="text-surface-content/60">Sweep ID</span>
							<code class="text-xs">{selectedSweep.sweep_id}</code>
						</div>
					</div>

					<!-- Dashboard Link -->
					<div class="mb-4">
						<!-- eslint-disable svelte/no-navigation-without-resolve -->
						<a
							href={dashboardUrl(selectedSweep.sweep_id)}
							target="_blank"
							rel="noopener noreferrer"
							class="text-primary text-sm underline"
						>
							Open in Sweep Dashboard →
						</a>
					</div>

					<!-- Error -->
					{#if selectedSweep.error}
						<div class="mb-4 rounded bg-red-50 px-3 py-2 text-sm text-red-600">
							{selectedSweep.error}
						</div>
					{/if}

					<!-- Recommendation -->
					{#if selectedSweep.recommendation}
						{@const { params, metrics } = formatRecommendation(selectedSweep.recommendation)}
						<div class="mb-4">
							<h3 class="text-surface-content mb-2 text-sm font-semibold">
								Recommended Parameters
							</h3>
							<div class="bg-surface-200 rounded p-3">
								<table class="w-full text-xs">
									<tbody>
										{#each params as [key, val] (key)}
											<tr>
												<td class="text-surface-content/70 py-0.5 pr-3 font-mono">{key}</td>
												<td class="text-surface-content py-0.5 text-right font-mono font-medium"
													>{val}</td
												>
											</tr>
										{/each}
									</tbody>
								</table>
							</div>
							{#if metrics.length > 0}
								<details class="mt-2">
									<summary class="text-surface-content/60 cursor-pointer text-xs">
										Metrics ({metrics.length})
									</summary>
									<div class="bg-surface-200 mt-1 rounded p-3">
										<table class="w-full text-xs">
											<tbody>
												{#each metrics as [key, val] (key)}
													<tr>
														<td class="text-surface-content/70 py-0.5 pr-3 font-mono">{key}</td>
														<td class="text-surface-content/50 py-0.5 text-right font-mono"
															>{val}</td
														>
													</tr>
												{/each}
											</tbody>
										</table>
									</div>
								</details>
							{/if}

							<!-- Apply button -->
							<div class="mt-3 flex items-center gap-3">
								<Button variant="fill" color="primary" size="sm" on:click={applyRecommendation}>
									Apply Recommendation
								</Button>
								{#if applyStatus}
									<span
										class="text-xs {applyStatus.includes('✓') ? 'text-green-600' : 'text-red-500'}"
									>
										{applyStatus}
									</span>
								{/if}
							</div>
						</div>
					{/if}

					<!-- HINT Round History -->
					{#if selectedSweep.mode === 'hint' && selectedSweep.round_results && Array.isArray(selectedSweep.round_results)}
						<details class="mb-4" open>
							<summary class="text-surface-content cursor-pointer text-sm font-semibold">
								HINT Rounds ({selectedSweep.round_results.length})
							</summary>
							<div class="mt-2 space-y-2">
								{#each selectedSweep.round_results as round, i (i)}
									<div class="bg-surface-200 rounded p-3 text-xs">
										<div class="flex justify-between font-medium">
											<span>Round {round.round ?? i + 1}</span>
											{#if round.best_score}
												<span>Score: {Number(round.best_score).toFixed(4)}</span>
											{/if}
										</div>
										{#if round.labels_carried_over}
											<div class="text-surface-content/60 mt-1">
												↻ {round.labels_carried_over} labels carried over
											</div>
										{/if}
									</div>
								{/each}
							</div>
						</details>
					{/if}

					<!-- Score Breakdown -->
					{#if selectedSweep.score_components}
						{@const components =
							typeof selectedSweep.score_components === 'string'
								? JSON.parse(selectedSweep.score_components)
								: selectedSweep.score_components}
						{#if components}
							<div class="mb-4">
								<h4 class="text-surface-content mb-2 text-sm font-semibold">Score Breakdown</h4>
								<div class="bg-surface-200 rounded p-3">
									<div class="mb-2 text-sm">
										<span class="text-surface-content/60">Composite Score:</span>
										<strong class="ml-2">{components.composite_score?.toFixed(4) ?? '—'}</strong>
									</div>
									{#if components.top_contributors && components.top_contributors.length > 0}
										<div class="mb-3 text-xs">
											<span class="text-surface-content/60">Top Contributors:</span>
											<span class="ml-2">{components.top_contributors.join(', ')}</span>
										</div>
									{/if}
									{#if components.label_coverage_confidence != null}
										<div class="mb-3 text-xs">
											<span class="text-surface-content/60">Label Coverage:</span>
											<span class="ml-2"
												>{(components.label_coverage_confidence * 100).toFixed(1)}%</span
											>
										</div>
									{/if}
									<table class="w-full text-xs">
										<thead>
											<tr class="border-surface-content/10 border-b">
												<th class="text-surface-content/70 px-2 py-1 text-left">Metric</th>
												<th class="text-surface-content/70 px-2 py-1 text-right">Value</th>
												<th class="text-surface-content/70 px-2 py-1 text-right">Weight</th>
											</tr>
										</thead>
										<tbody>
											{#each SCORE_METRICS as [label, key] (key)}
												<tr class="border-surface-content/5 border-b">
													<td class="px-2 py-1">{label}</td>
													<td class="px-2 py-1 text-right font-mono"
														>{components[key]?.toFixed(4) ?? '—'}</td
													>
													<td class="text-surface-content/60 px-2 py-1 text-right font-mono"
														>{components.weights_used?.[key]?.toFixed(2) ?? '—'}</td
													>
												</tr>
											{/each}
										</tbody>
									</table>
								</div>
							</div>
						{/if}
					{/if}

					<!-- Request Config -->
					{#if selectedSweep.request}
						<details class="mb-4">
							<summary class="text-surface-content cursor-pointer text-sm font-semibold">
								Request Config
							</summary>
							<pre
								class="bg-surface-200 mt-2 max-h-64 overflow-auto rounded p-3 text-xs">{JSON.stringify(
									selectedSweep.request,
									null,
									2
								)}</pre>
						</details>
					{/if}

					<!-- Round Results (auto-tune) -->
					{#if selectedSweep.round_results && Array.isArray(selectedSweep.round_results)}
						<details class="mb-4">
							<summary class="text-surface-content cursor-pointer text-sm font-semibold">
								Rounds ({selectedSweep.round_results.length})
							</summary>
							<div class="mt-2 space-y-2">
								{#each selectedSweep.round_results as round, i (i)}
									<div class="bg-surface-200 rounded p-3 text-xs">
										<span class="font-medium">Round {i + 1}</span>
										<pre class="mt-1 max-h-40 overflow-auto whitespace-pre-wrap">{JSON.stringify(
												round,
												null,
												2
											)}</pre>
									</div>
								{/each}
							</div>
						</details>
					{/if}

					<!-- Results Summary -->
					{#if selectedSweep.results && Array.isArray(selectedSweep.results)}
						<details class="mb-4">
							<summary class="text-surface-content cursor-pointer text-sm font-semibold">
								Results ({selectedSweep.results.length} combos)
							</summary>
							<pre
								class="bg-surface-200 mt-2 max-h-96 overflow-auto rounded p-3 text-xs">{JSON.stringify(
									selectedSweep.results,
									null,
									2
								)}</pre>
						</details>
					{/if}
				{/if}
			</div>
		{/if}
	</div>
</main>
