<script lang="ts">
	/**
	 * The motion/static timeline of a capture session, drawn as a strip.
	 *
	 * Static stretches are the deliverable ones — they feed replay and VRLOG
	 * recording — and motion stretches are where the sensor was being moved,
	 * which the SLAM work will want and replay will not. The colouring says
	 * which is which; the ticks say where one capture file ended and the next
	 * began, which is the thing a per-file view cannot show.
	 */
	import type { CaptureFile, MotionPeriod } from '$lib/types/captures';
	import { clockTicks, fileBoundaries, formatDuration, toBands } from '$lib/captures/timeline';

	export let periods: MotionPeriod[] = [];
	export let files: CaptureFile[] = [];
	export let startNs: number;
	export let endNs: number;
	export let height = 10;
	/** Show file-boundary ticks. Off for a dense row, on for a detail view. */
	export let showBoundaries = false;
	/** Show a wall-clock axis below the strip. Off for a dense row. */
	export let showClockAxis = false;
	/** Highlight one period, by label. */
	export let highlight: string | null = null;

	$: bands = toBands(periods, startNs, endNs);
	$: ticks = showBoundaries ? fileBoundaries(files, startNs, endNs) : [];
	$: clock = showClockAxis ? clockTicks(startNs, endNs) : [];
</script>

<div>
	<div
		class="bg-surface-200 relative w-full overflow-hidden rounded"
		style="height: {height}px"
		role="img"
		aria-label={bands.length
			? `Motion timeline: ${bands.length} periods`
			: 'Motion timeline not yet computed'}
	>
		{#each bands as band (band.label + band.startNs)}
			<div
				class="absolute top-0 bottom-0 {band.type === 'static'
					? 'bg-emerald-500'
					: 'bg-amber-500'} {highlight && highlight !== band.label ? 'opacity-40' : ''}"
				style="left: {band.left}%; width: {band.width}%"
				title="{band.label} · {formatDuration(band.durationSecs)}"
			></div>
		{/each}

		{#each ticks as tick (tick.relPath)}
			<div
				class="absolute top-0 bottom-0 w-px bg-black/30"
				style="left: {tick.at}%"
				title="capture boundary: {tick.relPath}"
			></div>
		{/each}

		{#if bands.length === 0}
			<div
				class="text-surface-content/40 absolute inset-0 flex items-center justify-center text-[10px]"
			>
				no motion pass yet
			</div>
		{/if}
	</div>

	{#if clock.length > 0}
		<div class="text-surface-content/50 relative mt-0.5 h-3.5 font-mono text-[10px]">
			{#each clock as tick, i (tick.at)}
				<span
					class="absolute top-0 whitespace-nowrap {i === 0
						? ''
						: i === clock.length - 1
							? '-translate-x-full'
							: '-translate-x-1/2'}"
					style="left: {tick.at}%"
				>
					{tick.label}
				</span>
			{/each}
		</div>
	{/if}
</div>
