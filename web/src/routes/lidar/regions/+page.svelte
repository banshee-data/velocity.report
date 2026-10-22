<script lang="ts">
	/**
	 * Background Regions
	 *
	 * The regions L3 identifies while the grid settles, each with its own
	 * noise, neighbour and update parameters, drawn over the ring × azimuth
	 * grid. The grid is a raster, one block per cell (40 × 1800 on a Pandar40P),
	 * which is too many marks for SVG, so this page draws on a canvas.
	 */
	import {
		DEFAULT_LIDAR_SENSOR_ID,
		getBackgroundRegions,
		getLidarStatus,
		type BackgroundRegion,
		type BackgroundRegions
	} from '#lib/lidarControl.js';
	import {
		azimuthDegrees,
		cellAt,
		regionAt,
		regionColour,
		regionLegend,
		type GridCell
	} from '#lib/lidarRegions.js';
	import { onDestroy, onMount } from 'svelte';
	import { Button } from 'svelte-ux';

	// Poll while regions are still being identified, slower once they are.
	const PENDING_MS = 5000;
	const COMPLETE_MS = 30000;

	let sensorId = $state(DEFAULT_LIDAR_SENSOR_ID);
	let data = $state<BackgroundRegions | null>(null);
	let loaded = $state(false);
	let error = $state<string | null>(null);
	let selectedId = $state<number | null>(null);
	let hover = $state<{ cell: GridCell; region: BackgroundRegion; x: number; y: number } | null>(
		null
	);
	let canvas = $state<HTMLCanvasElement | null>(null);

	const legend = $derived(regionLegend(data));
	let timer: ReturnType<typeof setTimeout> | null = null;

	async function load() {
		try {
			data = await getBackgroundRegions(sensorId);
			error = null;
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			loaded = true;
		}
	}

	async function poll() {
		await load();
		timer = setTimeout(poll, data?.identification_complete ? COMPLETE_MS : PENDING_MS);
	}

	function draw(
		regions: BackgroundRegions | null,
		el: HTMLCanvasElement | null,
		selected: number | null
	) {
		if (!el) return;
		const ctx = el.getContext('2d');
		if (!ctx) return;
		ctx.clearRect(0, 0, el.width, el.height);
		if (!regions?.grid_mapping?.length || !regions.rings || !regions.azimuth_bins) return;
		const { rings, azimuth_bins: bins, grid_mapping: mapping } = regions;
		const cw = el.width / bins;
		const ch = el.height / rings;
		for (let ring = 0; ring < rings; ring++) {
			for (let bin = 0; bin < bins; bin++) {
				const id = mapping[ring * bins + bin];
				if (id == null || id < 0) continue;
				ctx.fillStyle = regionColour(id);
				ctx.globalAlpha = selected === null || selected === id ? 1 : 0.25;
				// Overdraw by a fraction so adjacent cells leave no seams.
				ctx.fillRect(bin * cw, ring * ch, cw + 0.5, ch + 0.5);
			}
		}
		ctx.globalAlpha = 1;
		ctx.strokeStyle = 'rgba(0, 0, 0, 0.15)';
		ctx.lineWidth = 0.5;
		for (let ring = 0; ring <= rings; ring += 5) {
			ctx.beginPath();
			ctx.moveTo(0, ring * ch);
			ctx.lineTo(el.width, ring * ch);
			ctx.stroke();
		}
	}

	$effect(() => draw(data, canvas, selectedId));

	function onPointerMove(e: PointerEvent) {
		if (!data || !canvas) return;
		const rect = canvas.getBoundingClientRect();
		const x = e.clientX - rect.left;
		const y = e.clientY - rect.top;
		const cell = cellAt(x, y, rect.width, rect.height, data.rings, data.azimuth_bins);
		const region = regionAt(data, cell);
		hover = cell && region ? { cell, region, x, y } : null;
	}

	function toggleRegion(id: number) {
		selectedId = selectedId === id ? null : id;
	}

	onMount(async () => {
		try {
			sensorId = (await getLidarStatus()).sensor_id || DEFAULT_LIDAR_SENSOR_ID;
		} catch {
			// Keep the default sensor; the region load reports its own error.
		}
		await poll();
	});

	onDestroy(() => {
		if (timer) clearTimeout(timer);
	});
</script>

<svelte:head>
	<title>Background Regions — velocity.report</title>
</svelte:head>

<main id="main-content" class="vr-page">
	<div class="vr-toolbar">
		<div class="flex items-center justify-between">
			<div>
				<h1 class="text-surface-content text-2xl font-semibold">Background Regions</h1>
				<p class="text-surface-content/60 mt-1 text-sm">
					Regions of the background grid with their own settling parameters, for
					<span class="font-mono">{sensorId}</span>.
				</p>
			</div>
			<Button variant="outline" on:click={load}>Refresh</Button>
		</div>
	</div>

	<div class="flex-1 overflow-y-auto p-6">
		<div class="space-y-4">
			{#if error}
				<div class="rounded bg-red-50 px-4 py-3 text-sm text-red-600">{error}</div>
			{/if}

			{#if !loaded}
				<p class="text-surface-content/50 text-sm">Loading…</p>
			{:else if !data}
				<p class="text-surface-content/50 text-sm">
					No background grid for this sensor yet. Regions appear once the grid starts settling.
				</p>
			{:else}
				<dl class="flex flex-wrap gap-x-8 gap-y-1 text-sm">
					<div>
						<dt class="text-surface-content/60 inline">Status</dt>
						<dd class="ml-1 inline font-medium">
							{data.identification_complete ? 'Identified' : 'Collecting…'}
						</dd>
					</div>
					<div>
						<dt class="text-surface-content/60 inline">Regions</dt>
						<dd class="ml-1 inline font-medium">{data.region_count}</dd>
					</div>
					<div>
						<dt class="text-surface-content/60 inline">Frames sampled</dt>
						<dd class="ml-1 inline font-medium">{data.frames_sampled.toLocaleString()}</dd>
					</div>
					<div>
						<dt class="text-surface-content/60 inline">Grid</dt>
						<dd class="ml-1 inline font-medium">
							{data.rings} rings × {data.azimuth_bins} azimuth bins
						</dd>
					</div>
				</dl>

				<section class="vr-chart-card">
					<div class="text-surface-content/50 mb-1 flex justify-between text-xs">
						<span>Ring 0 at top · azimuth 0° to 360° across</span>
						<span>Click a region in the legend to pick it out</span>
					</div>
					<div class="relative">
						<canvas
							bind:this={canvas}
							width="1800"
							height="400"
							class="border-surface-content/10 block h-[400px] w-full border"
							aria-label="Background regions over the ring and azimuth grid"
							onpointermove={onPointerMove}
							onpointerleave={() => (hover = null)}
						></canvas>
						{#if hover}
							<div
								class="bg-surface-100 border-surface-content/20 pointer-events-none absolute z-10 rounded border px-2 py-1 text-xs shadow"
								style:left="{hover.x + 12}px"
								style:top="{hover.y + 12}px"
							>
								<div class="flex items-center gap-2 font-semibold">
									<span
										class="inline-block h-3 w-3 rounded-sm"
										style:background-color={regionColour(hover.region.id)}
									></span>
									Region {hover.region.id}
								</div>
								<div>
									Ring {hover.cell.ring}, azimuth {azimuthDegrees(
										hover.cell.azimuthBin,
										data.azimuth_bins
									).toFixed(1)}°
								</div>
								<div>Variance {(hover.region.mean_variance || 0).toFixed(3)}</div>
								<div>
									Noise {(hover.region.params?.noise_relative_fraction || 0).toFixed(3)} · neighbours
									{hover.region.params?.neighbor_confirmation_count ?? 0} · update
									{(hover.region.params?.settle_update_fraction || 0).toFixed(3)}
								</div>
							</div>
						{/if}
					</div>
				</section>

				<section class="vr-chart-card">
					<h2 class="text-surface-content mb-2 text-sm font-semibold">Regions</h2>
					{#if legend.length === 0}
						<p class="text-surface-content/50 text-sm">No regions identified yet.</p>
					{:else}
						<ul class="grid gap-1 text-xs sm:grid-cols-2 lg:grid-cols-3">
							{#each legend as row (row.id)}
								<li>
									<button
										type="button"
										class="flex w-full items-center gap-2 rounded px-1 py-0.5 text-left {selectedId ===
										row.id
											? 'ring-primary ring-2'
											: 'hover:bg-surface-200'}"
										onclick={() => toggleRegion(row.id)}
									>
										<span
											class="inline-block h-3 w-3 flex-none rounded-sm"
											style:background-color={row.colour}
										></span>
										{row.text}
									</button>
								</li>
							{/each}
						</ul>
					{/if}
				</section>
			{/if}
		</div>
	</div>
</main>
