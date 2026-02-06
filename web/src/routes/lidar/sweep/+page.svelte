<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { Button, Card, Header, ProgressCircle, SelectField } from 'svelte-ux';
	import {
		startSweep,
		getSweepStatus,
		stopSweep,
		type SweepRequest,
		type SweepState,
		type ComboResult
	} from '$lib/api';

	// Sweep state
	let sweepState: SweepState | null = null;
	let loading = false;
	let error = '';
	let pollTimer: ReturnType<typeof setInterval> | null = null;

	// Form state
	let mode = 'noise';
	let iterations = 10;
	let interval = '2s';
	let settleTime = '5s';
	let seed = 'true';

	// Noise sweep range
	let noiseStart = 0.005;
	let noiseEnd = 0.05;
	let noiseStep = 0.005;

	// Closeness sweep range
	let closenessStart = 1.5;
	let closenessEnd = 3.0;
	let closenessStep = 0.5;

	// Neighbour sweep range
	let neighbourStart = 0;
	let neighbourEnd = 3;
	let neighbourStep = 1;

	// Fixed values for single-variable sweeps
	let fixedNoise = 0.01;
	let fixedCloseness = 2.0;
	let fixedNeighbour = 1;

	// ECharts instances
	let acceptanceChartEl: HTMLDivElement;
	let nonzeroChartEl: HTMLDivElement;
	let bucketChartEl: HTMLDivElement;
	let acceptanceChart: unknown;
	let nonzeroChart: unknown;
	let bucketChart: unknown;
	let echartsModule: typeof import('echarts') | null = null;

	const modeOptions = [
		{ label: 'Noise', value: 'noise' },
		{ label: 'Closeness', value: 'closeness' },
		{ label: 'Neighbour', value: 'neighbour' },
		{ label: 'Multi (all combinations)', value: 'multi' }
	];

	const seedOptions = [
		{ label: 'True', value: 'true' },
		{ label: 'False', value: 'false' },
		{ label: 'Toggle', value: 'toggle' }
	];

	function formatComboLabel(r: ComboResult): string {
		return `n=${r.noise.toFixed(3)} c=${r.closeness.toFixed(1)} nb=${r.neighbour}`;
	}

	async function initCharts() {
		if (echartsModule) return;
		try {
			echartsModule = await import('echarts');
		} catch {
			console.error('Failed to load ECharts');
		}
	}

	function renderCharts(results: ComboResult[]) {
		if (!echartsModule || !results || results.length === 0) return;

		const echarts = echartsModule;
		const labels = results.map(formatComboLabel);

		// Overall Acceptance Rate chart
		if (acceptanceChartEl) {
			if (!acceptanceChart) {
				acceptanceChart = echarts.init(acceptanceChartEl);
			}
			const chart = acceptanceChart as ReturnType<typeof echarts.init>;
			chart.setOption({
				title: { text: 'Overall Acceptance Rate', left: 'center' },
				tooltip: {
					trigger: 'axis',
					// eslint-disable-next-line @typescript-eslint/no-explicit-any
					formatter: (params: any) => {
						const p = Array.isArray(params) ? params[0] : params;
						const idx = labels.indexOf(p.name);
						const r = results[idx];
						if (!r) return p.name;
						return `${p.name}<br/>Mean: ${(r.overall_accept_mean * 100).toFixed(2)}%<br/>StdDev: ${(r.overall_accept_stddev * 100).toFixed(2)}%`;
					}
				},
				xAxis: {
					type: 'category',
					data: labels,
					axisLabel: { rotate: 45, fontSize: 10 }
				},
				yAxis: {
					type: 'value',
					name: 'Acceptance Rate',
					axisLabel: {
						formatter: (v: number) => `${(v * 100).toFixed(0)}%`
					}
				},
				series: [
					{
						name: 'Mean',
						type: 'bar',
						data: results.map((r) => r.overall_accept_mean),
						itemStyle: { color: '#5470c6' }
					}
				],
				grid: { bottom: 100 }
			});
		}

		// Nonzero Cells chart
		if (nonzeroChartEl) {
			if (!nonzeroChart) {
				nonzeroChart = echarts.init(nonzeroChartEl);
			}
			const chart = nonzeroChart as ReturnType<typeof echarts.init>;
			chart.setOption({
				title: { text: 'Nonzero Background Cells', left: 'center' },
				tooltip: {
					trigger: 'axis',
					// eslint-disable-next-line @typescript-eslint/no-explicit-any
					formatter: (params: any) => {
						const p = Array.isArray(params) ? params[0] : params;
						const idx = labels.indexOf(p.name);
						const r = results[idx];
						if (!r) return p.name;
						return `${p.name}<br/>Mean: ${r.nonzero_cells_mean.toFixed(0)}<br/>StdDev: ${r.nonzero_cells_stddev.toFixed(0)}`;
					}
				},
				xAxis: {
					type: 'category',
					data: labels,
					axisLabel: { rotate: 45, fontSize: 10 }
				},
				yAxis: { type: 'value', name: 'Cell Count' },
				series: [
					{
						name: 'Mean',
						type: 'bar',
						data: results.map((r) => r.nonzero_cells_mean),
						itemStyle: { color: '#91cc75' }
					}
				],
				grid: { bottom: 100 }
			});
		}

		// Per-Bucket Acceptance Heatmap
		if (bucketChartEl && results[0]?.buckets?.length > 0) {
			if (!bucketChart) {
				bucketChart = echarts.init(bucketChartEl);
			}
			const chart = bucketChart as ReturnType<typeof echarts.init>;
			const buckets = results[0].buckets;
			const heatmapData: Array<[number, number, number]> = [];
			let maxVal = 0;
			results.forEach((r, ri) => {
				if (r.bucket_means) {
					r.bucket_means.forEach((v, bi) => {
						heatmapData.push([ri, bi, v]);
						if (v > maxVal) maxVal = v;
					});
				}
			});

			chart.setOption({
				title: { text: 'Per-Bucket Acceptance Rates', left: 'center' },
				tooltip: {
					// eslint-disable-next-line @typescript-eslint/no-explicit-any
					formatter: (params: any) => {
						const [ri, bi, val] = params.value;
						const r = results[ri];
						const bucket = buckets[bi];
						return `${formatComboLabel(r)}<br/>Bucket ${bucket}m: ${(val * 100).toFixed(2)}%`;
					}
				},
				xAxis: {
					type: 'category',
					data: labels,
					axisLabel: { rotate: 45, fontSize: 10 }
				},
				yAxis: {
					type: 'category',
					data: buckets.map((b: string) => `${b}m`),
					name: 'Range Bucket'
				},
				visualMap: {
					min: 0,
					max: maxVal || 1,
					calculable: true,
					orient: 'horizontal',
					left: 'center',
					bottom: 0,
					inRange: {
						color: [
							'#313695',
							'#4575b4',
							'#74add1',
							'#abd9e9',
							'#fee090',
							'#fdae61',
							'#f46d43',
							'#d73027'
						]
					},
					formatter: (v: number) => `${(v * 100).toFixed(0)}%`
				},
				series: [
					{
						type: 'heatmap',
						data: heatmapData,
						emphasis: {
							itemStyle: { shadowBlur: 10, shadowColor: 'rgba(0,0,0,0.5)' }
						}
					}
				],
				grid: { bottom: 80, top: 60 }
			});
		}
	}

	async function handleStart() {
		loading = true;
		error = '';
		try {
			const req: SweepRequest = {
				mode,
				iterations,
				interval,
				settle_time: settleTime,
				seed,
				noise_start: noiseStart,
				noise_end: noiseEnd,
				noise_step: noiseStep,
				closeness_start: closenessStart,
				closeness_end: closenessEnd,
				closeness_step: closenessStep,
				neighbour_start: neighbourStart,
				neighbour_end: neighbourEnd,
				neighbour_step: neighbourStep,
				fixed_noise: fixedNoise,
				fixed_closeness: fixedCloseness,
				fixed_neighbour: fixedNeighbour
			};
			await startSweep(req);
			startPolling();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			loading = false;
		}
	}

	async function handleStop() {
		try {
			await stopSweep();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		}
	}

	async function pollStatus() {
		try {
			sweepState = await getSweepStatus();
			if (sweepState?.results?.length > 0) {
				await initCharts();
				renderCharts(sweepState.results);
			}
			if (sweepState?.status === 'complete' || sweepState?.status === 'error') {
				stopPolling();
			}
		} catch (e) {
			console.error('Failed to poll sweep status:', e);
		}
	}

	function startPolling() {
		stopPolling();
		pollTimer = setInterval(pollStatus, 3000);
		pollStatus();
	}

	function stopPolling() {
		if (pollTimer) {
			clearInterval(pollTimer);
			pollTimer = null;
		}
	}

	function handleResize() {
		if (acceptanceChart) (acceptanceChart as ReturnType<typeof import('echarts').init>).resize();
		if (nonzeroChart) (nonzeroChart as ReturnType<typeof import('echarts').init>).resize();
		if (bucketChart) (bucketChart as ReturnType<typeof import('echarts').init>).resize();
	}

	onMount(async () => {
		// Check if there's an existing sweep in progress
		try {
			sweepState = await getSweepStatus();
			if (sweepState?.status === 'running') {
				startPolling();
			} else if (sweepState?.results?.length > 0) {
				await initCharts();
				renderCharts(sweepState.results);
			}
		} catch {
			// Server may not have sweep runner configured
		}
		window.addEventListener('resize', handleResize);
	});

	onDestroy(() => {
		stopPolling();
		if (typeof window !== 'undefined') {
			window.removeEventListener('resize', handleResize);
		}
		if (acceptanceChart) (acceptanceChart as ReturnType<typeof import('echarts').init>).dispose();
		if (nonzeroChart) (nonzeroChart as ReturnType<typeof import('echarts').init>).dispose();
		if (bucketChart) (bucketChart as ReturnType<typeof import('echarts').init>).dispose();
	});

	$: isRunning = sweepState?.status === 'running';
	$: progressPct =
		sweepState && sweepState.total_combos > 0
			? Math.round((sweepState.completed_combos / sweepState.total_combos) * 100)
			: 0;
</script>

<svelte:head>
	<title>Parameter Sweep 🚴 velocity.report</title>
	<meta name="description" content="LiDAR background model parameter sweep and tuning dashboard" />
</svelte:head>

<div id="main-content" class="space-y-6 p-4">
	<Header
		title="LiDAR Parameter Sweep"
		subheading="Sweep background model parameters and visualise results to identify optimal tuning configuration."
	/>

	{#if error}
		<div class="rounded-lg border border-red-300 bg-red-50 p-4 text-red-700">
			{error}
		</div>
	{/if}

	<!-- Configuration Form -->
	<Card title="Sweep Configuration">
		<div class="flex flex-col gap-4 p-4">
			<div class="grid grid-cols-1 gap-4 md:grid-cols-3">
				<SelectField
					label="Sweep Mode"
					options={modeOptions}
					bind:value={mode}
					disabled={isRunning}
					clearable={false}
				/>
				<SelectField
					label="Seed Behaviour"
					options={seedOptions}
					bind:value={seed}
					disabled={isRunning}
					clearable={false}
				/>
			</div>

			<!-- Noise parameters -->
			{#if mode === 'noise' || mode === 'multi'}
				<div class="border-surface-content/20 rounded border p-3">
					<p class="text-surface-content/70 mb-2 text-sm font-medium">Noise Range</p>
					<div class="grid grid-cols-3 gap-3">
						<label class="flex flex-col gap-1">
							<span class="text-xs">Start</span>
							<input
								type="number"
								step="0.001"
								bind:value={noiseStart}
								disabled={isRunning}
								class="border-surface-300 rounded border p-2 text-sm"
							/>
						</label>
						<label class="flex flex-col gap-1">
							<span class="text-xs">End</span>
							<input
								type="number"
								step="0.001"
								bind:value={noiseEnd}
								disabled={isRunning}
								class="border-surface-300 rounded border p-2 text-sm"
							/>
						</label>
						<label class="flex flex-col gap-1">
							<span class="text-xs">Step</span>
							<input
								type="number"
								step="0.001"
								bind:value={noiseStep}
								disabled={isRunning}
								class="border-surface-300 rounded border p-2 text-sm"
							/>
						</label>
					</div>
				</div>
			{/if}

			<!-- Closeness parameters -->
			{#if mode === 'closeness' || mode === 'multi'}
				<div class="border-surface-content/20 rounded border p-3">
					<p class="text-surface-content/70 mb-2 text-sm font-medium">Closeness Range</p>
					<div class="grid grid-cols-3 gap-3">
						<label class="flex flex-col gap-1">
							<span class="text-xs">Start</span>
							<input
								type="number"
								step="0.5"
								bind:value={closenessStart}
								disabled={isRunning}
								class="border-surface-300 rounded border p-2 text-sm"
							/>
						</label>
						<label class="flex flex-col gap-1">
							<span class="text-xs">End</span>
							<input
								type="number"
								step="0.5"
								bind:value={closenessEnd}
								disabled={isRunning}
								class="border-surface-300 rounded border p-2 text-sm"
							/>
						</label>
						<label class="flex flex-col gap-1">
							<span class="text-xs">Step</span>
							<input
								type="number"
								step="0.5"
								bind:value={closenessStep}
								disabled={isRunning}
								class="border-surface-300 rounded border p-2 text-sm"
							/>
						</label>
					</div>
				</div>
			{/if}

			<!-- Neighbour parameters -->
			{#if mode === 'neighbour' || mode === 'multi'}
				<div class="border-surface-content/20 rounded border p-3">
					<p class="text-surface-content/70 mb-2 text-sm font-medium">Neighbour Range</p>
					<div class="grid grid-cols-3 gap-3">
						<label class="flex flex-col gap-1">
							<span class="text-xs">Start</span>
							<input
								type="number"
								step="1"
								bind:value={neighbourStart}
								disabled={isRunning}
								class="border-surface-300 rounded border p-2 text-sm"
							/>
						</label>
						<label class="flex flex-col gap-1">
							<span class="text-xs">End</span>
							<input
								type="number"
								step="1"
								bind:value={neighbourEnd}
								disabled={isRunning}
								class="border-surface-300 rounded border p-2 text-sm"
							/>
						</label>
						<label class="flex flex-col gap-1">
							<span class="text-xs">Step</span>
							<input
								type="number"
								step="1"
								bind:value={neighbourStep}
								disabled={isRunning}
								class="border-surface-300 rounded border p-2 text-sm"
							/>
						</label>
					</div>
				</div>
			{/if}

			<!-- Fixed values (shown when not sweeping that parameter) -->
			{#if mode !== 'multi'}
				<div class="border-surface-content/20 rounded border p-3">
					<p class="text-surface-content/70 mb-2 text-sm font-medium">
						Fixed Parameters (not being swept)
					</p>
					<div class="grid grid-cols-3 gap-3">
						{#if mode !== 'noise'}
							<label class="flex flex-col gap-1">
								<span class="text-xs">Fixed Noise</span>
								<input
									type="number"
									step="0.001"
									bind:value={fixedNoise}
									disabled={isRunning}
									class="border-surface-300 rounded border p-2 text-sm"
								/>
							</label>
						{/if}
						{#if mode !== 'closeness'}
							<label class="flex flex-col gap-1">
								<span class="text-xs">Fixed Closeness</span>
								<input
									type="number"
									step="0.5"
									bind:value={fixedCloseness}
									disabled={isRunning}
									class="border-surface-300 rounded border p-2 text-sm"
								/>
							</label>
						{/if}
						{#if mode !== 'neighbour'}
							<label class="flex flex-col gap-1">
								<span class="text-xs">Fixed Neighbour</span>
								<input
									type="number"
									step="1"
									bind:value={fixedNeighbour}
									disabled={isRunning}
									class="border-surface-300 rounded border p-2 text-sm"
								/>
							</label>
						{/if}
					</div>
				</div>
			{/if}

			<!-- Sampling parameters -->
			<div class="border-surface-content/20 rounded border p-3">
				<p class="text-surface-content/70 mb-2 text-sm font-medium">Sampling</p>
				<div class="grid grid-cols-3 gap-3">
					<label class="flex flex-col gap-1">
						<span class="text-xs">Iterations</span>
						<input
							type="number"
							step="1"
							min="1"
							max="100"
							bind:value={iterations}
							disabled={isRunning}
							class="border-surface-300 rounded border p-2 text-sm"
						/>
					</label>
					<label class="flex flex-col gap-1">
						<span class="text-xs">Interval</span>
						<input
							type="text"
							bind:value={interval}
							disabled={isRunning}
							placeholder="2s"
							class="border-surface-300 rounded border p-2 text-sm"
						/>
					</label>
					<label class="flex flex-col gap-1">
						<span class="text-xs">Settle Time</span>
						<input
							type="text"
							bind:value={settleTime}
							disabled={isRunning}
							placeholder="5s"
							class="border-surface-300 rounded border p-2 text-sm"
						/>
					</label>
				</div>
			</div>

			<!-- Action buttons -->
			<div class="flex gap-3">
				{#if isRunning}
					<Button on:click={handleStop} color="danger" variant="fill">Stop Sweep</Button>
				{:else}
					<Button on:click={handleStart} color="primary" variant="fill" disabled={loading}>
						{loading ? 'Starting...' : 'Start Sweep'}
					</Button>
				{/if}
			</div>
		</div>
	</Card>

	<!-- Progress -->
	{#if sweepState && sweepState.status !== 'idle'}
		<Card title="Sweep Progress">
			<div class="p-4">
				<div class="mb-4 flex items-center gap-4">
					<div class="flex items-center gap-2">
						<span class="text-sm font-medium">Status:</span>
						<span
							class="rounded-full px-2 py-0.5 text-xs font-medium"
							class:bg-green-100={sweepState.status === 'running'}
							class:text-green-700={sweepState.status === 'running'}
							class:bg-blue-100={sweepState.status === 'complete'}
							class:text-blue-700={sweepState.status === 'complete'}
							class:bg-red-100={sweepState.status === 'error'}
							class:text-red-700={sweepState.status === 'error'}
						>
							{sweepState.status}
						</span>
					</div>
					<span class="text-sm">
						{sweepState.completed_combos} / {sweepState.total_combos} combinations
					</span>
					{#if isRunning}
						<ProgressCircle value={progressPct} />
					{/if}
				</div>

				{#if sweepState.error}
					<div class="rounded border border-red-200 bg-red-50 p-2 text-sm text-red-600">
						{sweepState.error}
					</div>
				{/if}

				{#if sweepState.current_combo && isRunning}
					<p class="text-surface-content/60 text-sm">
						Current: noise={sweepState.current_combo.noise.toFixed(4)}, closeness={sweepState.current_combo.closeness.toFixed(
							2
						)}, neighbour={sweepState.current_combo.neighbour}
						→ acceptance={((sweepState.current_combo.overall_accept_mean || 0) * 100).toFixed(1)}%
					</p>
				{/if}
			</div>
		</Card>
	{/if}

	<!-- Charts -->
	{#if sweepState?.results && sweepState.results.length > 0}
		<Card title="Results">
			<div class="flex flex-col gap-4 p-4">
				<div bind:this={acceptanceChartEl} class="h-[400px] w-full"></div>
				<div bind:this={nonzeroChartEl} class="h-[400px] w-full"></div>
				<div bind:this={bucketChartEl} class="h-[400px] w-full"></div>
			</div>
		</Card>

		<!-- Results Table -->
		<Card title="Results Table">
			<div class="overflow-x-auto p-4">
				<table class="w-full text-sm">
					<thead>
						<tr class="border-b">
							<th class="p-2 text-left">Noise</th>
							<th class="p-2 text-left">Closeness</th>
							<th class="p-2 text-left">Neighbour</th>
							<th class="p-2 text-left">Accept Rate</th>
							<th class="p-2 text-left">± StdDev</th>
							<th class="p-2 text-left">Nonzero Cells</th>
							<th class="p-2 text-left">± StdDev</th>
						</tr>
					</thead>
					<tbody>
						{#each sweepState.results as r}
							<tr class="hover:bg-surface-100 border-b">
								<td class="p-2 font-mono">{r.noise.toFixed(4)}</td>
								<td class="p-2 font-mono">{r.closeness.toFixed(2)}</td>
								<td class="p-2 font-mono">{r.neighbour}</td>
								<td class="p-2 font-mono">{(r.overall_accept_mean * 100).toFixed(2)}%</td>
								<td class="text-surface-content/50 p-2 font-mono"
									>±{(r.overall_accept_stddev * 100).toFixed(2)}%</td
								>
								<td class="p-2 font-mono">{r.nonzero_cells_mean.toFixed(0)}</td>
								<td class="text-surface-content/50 p-2 font-mono"
									>±{r.nonzero_cells_stddev.toFixed(0)}</td
								>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</Card>
	{/if}
</div>
