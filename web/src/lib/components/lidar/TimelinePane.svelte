<script lang="ts">
	import { browser } from '$app/environment';
	import type { Track } from '$lib/types/lidar';
	import { TRACK_COLORS } from '$lib/types/lidar';
	import { scaleTime } from 'd3-scale';
	import { onMount } from 'svelte';
	import { Button } from 'svelte-ux';

	export let tracks: Track[] = [];
	export let currentTime: number;
	export let timeRange: { start: number; end: number } | null = null;
	export let isPlaying: boolean = false;
	export let playbackSpeed: number = 1.0;
	export let selectedTrackId: string | null = null;
	export let onTimeChange: (time: number) => void = () => {};
	export let onPlaybackToggle: () => void = () => {};
	export let onSpeedChange: (speed: number) => void = () => {};
	export let onTrackSelect: (trackId: string) => void = () => {};

	let svg: SVGSVGElement;
	let containerWidth = 800;
	let containerHeight = 300;
	let isDragging = false;

	// Constants
	const MARGIN = { top: 40, right: 20, bottom: 40, left: 100 };
	const TRACK_HEIGHT = 20;
	const TRACK_SPACING = 5;

	// Computed values
	$: width = containerWidth - MARGIN.left - MARGIN.right;
	$: contentHeight =
		MARGIN.top + sortedTracks.length * (TRACK_HEIGHT + TRACK_SPACING) + MARGIN.bottom;
	$: svgHeight = Math.max(containerHeight, contentHeight);
	$: height = svgHeight - MARGIN.top - MARGIN.bottom;

	// Time scale
	$: timeScale = timeRange
		? scaleTime()
				.domain([new Date(timeRange.start), new Date(timeRange.end)])
				.range([0, width])
		: null;

	// Visible tracks (sorted by start time)
	$: sortedTracks = [...tracks].sort(
		(a, b) => new Date(a.first_seen).getTime() - new Date(b.first_seen).getTime()
	);

	// Debug timeline data
	$: if (sortedTracks.length > 0) {
		console.log('[Timeline] Tracks:', sortedTracks.length, 'Time range:', timeRange);
		console.log('[Timeline] First track:', {
			id: sortedTracks[0].track_id,
			first_seen: sortedTracks[0].first_seen,
			last_seen: sortedTracks[0].last_seen
		});
		if (sortedTracks.length > 1) {
			console.log('[Timeline] Last track:', {
				id: sortedTracks[sortedTracks.length - 1].track_id,
				first_seen: sortedTracks[sortedTracks.length - 1].first_seen,
				last_seen: sortedTracks[sortedTracks.length - 1].last_seen
			});
		}
		if (timeScale) {
			const firstTrackStart = timeScale(new Date(sortedTracks[0].first_seen));
			const lastTrackEnd = timeScale(new Date(sortedTracks[sortedTracks.length - 1].last_seen));
			console.log(
				'[Timeline] Scale test - first track startX:',
				firstTrackStart,
				'last track endX:',
				lastTrackEnd,
				'width:',
				width
			);
		}
	}

	// Handle scrubber drag
	function handleScrubberMouseDown(e: MouseEvent) {
		isDragging = true;
		updateTimeFromMouse(e);
	}

	function handleMouseMove(e: MouseEvent) {
		if (isDragging) {
			updateTimeFromMouse(e);
		}
	}

	function handleMouseUp() {
		isDragging = false;
	}

	function updateTimeFromMouse(e: MouseEvent) {
		if (!svg || !timeScale || !timeRange) return;

		const rect = svg.getBoundingClientRect();
		const x = e.clientX - rect.left - MARGIN.left;
		const clampedX = Math.max(0, Math.min(width, x));

		const newTime = timeScale.invert(clampedX).getTime();
		onTimeChange(newTime);
	}

	// Handle track click
	function handleTrackClick(trackId: string) {
		onTrackSelect(trackId);
	}

	// Speed control options
	const speedOptions = [0.5, 1, 2, 5, 10];

	// Update container size
	function updateSize() {
		if (!svg) return;
		const container = svg.parentElement;
		if (container) {
			containerWidth = container.clientWidth;
			containerHeight = container.clientHeight;
		}
	}

	onMount(() => {
		if (!browser) return;
		updateSize();
		window.addEventListener('resize', updateSize);
		window.addEventListener('mousemove', handleMouseMove);
		window.addEventListener('mouseup', handleMouseUp);

		return () => {
			window.removeEventListener('resize', updateSize);
			window.removeEventListener('mousemove', handleMouseMove);
			window.removeEventListener('mouseup', handleMouseUp);
		};
	});

	// Format time for display
	function formatTime(ms: number): string {
		const date = new Date(ms);
		return date.toLocaleTimeString();
	}

	// Get track color
	function getTrackColor(track: Track): string {
		if (track.state === 'tentative') return TRACK_COLORS.tentative;
		if (track.state === 'deleted') return TRACK_COLORS.deleted;
		if (track.object_class && track.object_class in TRACK_COLORS) {
			return TRACK_COLORS[track.object_class as keyof typeof TRACK_COLORS];
		}
		return TRACK_COLORS.other;
	}
</script>

<div class="bg-surface-100 flex h-full flex-col">
	<!-- Controls Bar -->
	<div class="border-surface-content/10 flex items-center justify-between border-b px-4 py-3">
		<div class="flex items-center gap-3">
			<!-- Play/Pause -->
			<Button on:click={onPlaybackToggle} variant="outline" size="sm">
				{#if isPlaying}
					<svg class="h-4 w-4" fill="currentColor" viewBox="0 0 20 20">
						<path
							fill-rule="evenodd"
							d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zM7 8a1 1 0 012 0v4a1 1 0 11-2 0V8zm5-1a1 1 0 00-1 1v4a1 1 0 102 0V8a1 1 0 00-1-1z"
							clip-rule="evenodd"
						/>
					</svg>
					Pause
				{:else}
					<svg class="h-4 w-4" fill="currentColor" viewBox="0 0 20 20">
						<path
							fill-rule="evenodd"
							d="M10 18a8 8 0 100-16 8 8 0 000 16zM9.555 7.168A1 1 0 008 8v4a1 1 0 001.555.832l3-2a1 1 0 000-1.664l-3-2z"
							clip-rule="evenodd"
						/>
					</svg>
					Play
				{/if}
			</Button>

			<!-- Speed Control -->
			<div class="flex items-center gap-2">
				<span class="text-surface-content/60 text-sm">Speed:</span>
				{#each speedOptions as speed (speed)}
					<Button
						on:click={() => onSpeedChange(speed)}
						variant={playbackSpeed === speed ? 'fill' : 'outline'}
						size="sm"
						class="min-w-12"
					>
						{speed}x
					</Button>
				{/each}
			</div>
		</div>

		<!-- Time Display -->
		<div class="text-surface-content/70 font-mono text-sm">
			{formatTime(currentTime)}
		</div>
	</div>

	<!-- Timeline SVG -->
	<div class="flex-1 overflow-auto">
		<svg bind:this={svg} width={containerWidth} height={svgHeight} class="bg-surface-200">
			<g transform={`translate(${MARGIN.left}, ${MARGIN.top})`}>
				<!-- Time axis -->
				{#if timeScale}
					<line
						x1={0}
						y1={0}
						x2={width}
						y2={0}
						class="stroke-surface-content/20"
						stroke-width="2"
					/>

					<!-- Time ticks -->
					{#each timeScale.ticks(10) as tick (tick.getTime())}
						{@const x = timeScale(tick)}
						<g transform={`translate(${x}, 0)`}>
							<line y1={0} y2={5} class="stroke-surface-content/40" stroke-width="1" />
							<text y={20} text-anchor="middle" class="fill-surface-content/60 text-xs">
								{formatTime(tick.getTime())}
							</text>
						</g>
					{/each}
				{/if}

				<!-- Track bars -->
				{#each sortedTracks as track, i (track.track_id)}
					{@const y = 40 + i * (TRACK_HEIGHT + TRACK_SPACING)}
					{@const startX = timeScale ? timeScale(new Date(track.first_seen)) : 0}
					{@const endX = timeScale ? timeScale(new Date(track.last_seen)) : 0}
					{@const barWidth = endX - startX}
					{@const color = getTrackColor(track)}
					{@const isSelected = track.track_id === selectedTrackId}

					<g
						on:click={() => handleTrackClick(track.track_id)}
						on:keydown={(e) => e.key === 'Enter' && handleTrackClick(track.track_id)}
						role="button"
						tabindex="0"
						class="cursor-pointer hover:opacity-80"
					>
						<!-- Track label -->
						<text
							x={-10}
							y={y + TRACK_HEIGHT / 2}
							text-anchor="end"
							alignment-baseline="middle"
							class="fill-gray-700 font-mono text-xs"
						>
							{track.track_id.slice(-6)}
						</text>

						<!-- Track bar -->
						<rect
							x={startX}
							{y}
							width={barWidth}
							height={TRACK_HEIGHT}
							fill={color}
							fill-opacity={isSelected ? 0.8 : 0.5}
							stroke={color}
							stroke-width={isSelected ? 2 : 1}
						/>

						<!-- Speed indicator -->
						<text
							x={startX + barWidth / 2}
							y={y + TRACK_HEIGHT / 2}
							text-anchor="middle"
							alignment-baseline="middle"
							class="fill-white text-xs font-medium"
						>
							{track.avg_speed_mps.toFixed(1)} m/s
						</text>
					</g>
				{/each}

				<!-- Current time scrubber -->
				{#if timeScale}
					{@const scrubberX = timeScale(new Date(currentTime))}
					<g
						transform={`translate(${scrubberX}, 0)`}
						on:mousedown={handleScrubberMouseDown}
						role="slider"
						tabindex="0"
						aria-valuenow={currentTime}
						class="cursor-ew-resize"
					>
						<line y1={-10} y2={height} stroke="#ef4444" stroke-width="2" />
						<circle cy={-10} r={6} fill="#ef4444" stroke="white" stroke-width="2" />
					</g>
				{/if}
			</g>
		</svg>
	</div>
</div>

<style>
	svg {
		user-select: none;
	}
</style>
