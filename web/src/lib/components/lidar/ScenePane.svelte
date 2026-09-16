<script lang="ts">
	/**
	 * ScenePane — the 3D scene view for the operator tools.
	 *
	 * This replaces the flat top-down canvas map with the same three.js player
	 * the public scenes run on. The player is imported from
	 * public_html/src/js, not copied: a copy would drift the moment either
	 * side fixed a camera, a trail or a colour.
	 *
	 * The data path is the only difference. A published scene reads static
	 * gzipped chunks; this mounts a session built over live observations from
	 * the database, so playback, seeking and trail reconstruction stay the
	 * shared implementation.
	 */
	import { mountScenePlayer } from '$scene/scene-player.js';
	import type { SceneInteraction } from '$scene/scene-reader.js';
	import { createLiveSceneSession } from '$lib/scene/liveSceneSource';
	import type { MissedRegion, RunTrack, TrackObservation } from '$lib/types/lidar';
	import { onDestroy } from 'svelte';

	export let observations: TrackObservation[] = [];
	export let runTracks: RunTrack[] = [];
	export let sensorId = 'hesai-pandar40p';
	export let title = 'Live run';

	/** Regions already marked, drawn as rings on the ground. */
	export let missedRegions: MissedRegion[] = [];
	/** When on, a click marks a region instead of selecting a track. */
	export let markMissedMode = false;
	export let onTrackSelect: (trackId: string) => void = () => {};
	/** Called with the ENU ground position of a click while marking. */
	export let onMapClick: ((worldX: number, worldY: number) => void) | null = null;
	/**
	 * The sensor's zero azimuth against this site's street grid, in degrees.
	 * Turning the reference grid by it lines the squares up with the kerbs; it
	 * changes nothing about the data.
	 */
	export let gridAzimuthDeg: number | undefined = undefined;
	/** The sensor's clockwise bearing from true north, where measured. */
	export let northAzimuthDeg: number | undefined = undefined;

	let canvas: HTMLCanvasElement;
	let labelLayer: HTMLDivElement;
	let presets: HTMLDivElement;
	let clock: HTMLSpanElement;
	let duration: HTMLSpanElement;
	let stats: HTMLSpanElement;
	let status: HTMLDivElement;
	let playToggle: HTMLButtonElement;
	let rate: HTMLSelectElement;
	let lidarToggle: HTMLInputElement;
	let boxesToggle: HTMLInputElement;
	let trailsToggle: HTMLInputElement;
	let gridToggle: HTMLInputElement;

	let mounted = false;
	let mountError: string | null = null;
	let skipped = 0;
	let frameCount = 0;
	let interaction: SceneInteraction | null = null;

	/**
	 * Remounting on every observation change would reset the camera mid-review,
	 * so the player is mounted once the first data arrives and the key is
	 * tracked to detect a genuinely different run.
	 */
	let mountedKey = '';

	$: sceneKey = `${sensorId}|${observations.length}|${observations[0]?.timestamp ?? ''}|${
		observations[observations.length - 1]?.timestamp ?? ''
	}`;

	$: if (canvas && observations.length > 0 && sceneKey !== mountedKey) {
		mountedKey = sceneKey;
		void mount();
	}

	async function mount() {
		mountError = null;
		try {
			const built = createLiveSceneSession({
				observations,
				runTracks,
				sensorId,
				title
			});
			skipped = built.skippedObservations;
			frameCount = built.frameCount;

			const session = await mountScenePlayer({
				canvas,
				session: built.session,
				ui: {
					labelLayer,
					presets,
					clock,
					duration,
					stats,
					status,
					playToggle,
					rate,
					lidarToggle,
					boxesToggle,
					trailsToggle,
					gridToggle,
					gridAzimuthDeg,
					northAzimuthDeg
				}
			});
			interaction = session.interaction ?? null;
			interaction?.setRegions(missedRegions);
			mounted = true;
		} catch (error) {
			mountError = error instanceof Error ? error.message : String(error);
		}
	}

	/**
	 * A click either marks a region or selects a track, never both: marking
	 * mode is explicit, so a click while it is on is unambiguous.
	 *
	 * The player answers where the click landed, because it owns the camera
	 * and the ENU-to-scene mapping.
	 */
	function handleCanvasClick(event: MouseEvent) {
		if (!interaction) return;

		if (markMissedMode) {
			const ground = interaction.groundAt(event.clientX, event.clientY);
			// A ray that misses the ground plane entirely — a click on the sky —
			// is not a position, so nothing is marked.
			if (ground && onMapClick) onMapClick(ground.x, ground.y);
			return;
		}

		const trackId = interaction.trackAt(event.clientX, event.clientY);
		if (trackId) onTrackSelect(trackId);
	}

	// Regions are pushed to the overlay whenever they change, not only at
	// mount, so a newly marked one appears without a remount.
	$: if (interaction) interaction.setRegions(missedRegions);

	onDestroy(() => {
		mounted = false;
		interaction = null;
	});
</script>

<div class="scene-pane">
	<canvas
		bind:this={canvas}
		class="scene-pane__canvas"
		class:scene-pane__canvas--marking={markMissedMode}
		on:click={handleCanvasClick}
	></canvas>
	<div bind:this={labelLayer} class="scene-pane__labels"></div>

	{#if markMissedMode}
		<p class="scene-pane__hint">Click the ground to mark a missed region</p>
	{/if}

	<div class="scene-pane__controls">
		<button bind:this={playToggle} type="button" class="scene-pane__button">Play</button>
		<select bind:this={rate} class="scene-pane__select" aria-label="Playback rate">
			<option value="1">1x</option>
			<option value="2">2x</option>
			<option value="4">4x</option>
			<option value="8">8x</option>
			<option value="16">16x</option>
		</select>
		<span bind:this={clock} class="scene-pane__clock">0:00</span>
		<span class="scene-pane__sep">/</span>
		<span bind:this={duration} class="scene-pane__clock">0:00</span>

		<label class="scene-pane__toggle">
			<input bind:this={lidarToggle} type="checkbox" checked /> Points
		</label>
		<label class="scene-pane__toggle">
			<input bind:this={boxesToggle} type="checkbox" checked /> Boxes
		</label>
		<label class="scene-pane__toggle">
			<input bind:this={trailsToggle} type="checkbox" checked /> Trails
		</label>
		<label class="scene-pane__toggle">
			<input bind:this={gridToggle} type="checkbox" checked /> Grid
		</label>

		<span bind:this={stats} class="scene-pane__stats"></span>
	</div>

	<!-- The player populates named viewpoints here when a scene has them. A
	     live run has none, so this stays empty rather than absent: the shared
	     code appends to it without checking. -->
	<div bind:this={presets} class="scene-pane__presets" hidden></div>

	<div bind:this={status} class="scene-pane__status" hidden></div>

	{#if mountError}
		<p class="scene-pane__error">Could not start the scene view: {mountError}</p>
	{:else if observations.length === 0}
		<p class="scene-pane__empty">No observations in this window.</p>
	{:else if mounted && skipped > 0}
		<!-- A partial load has to be visible: a dropped row would otherwise
		     make the frame count look complete. -->
		<p class="scene-pane__warning">
			{frameCount} frames · {skipped} observation{skipped === 1 ? '' : 's'} skipped for an unusable timestamp
		</p>
	{/if}
</div>

<style>
	.scene-pane {
		position: relative;
		display: flex;
		flex-direction: column;
		height: 100%;
		min-height: 0;
		background: #0b1418;
	}
	.scene-pane__canvas {
		flex: 1 1 auto;
		width: 100%;
		min-height: 0;
		display: block;
	}
	.scene-pane__canvas--marking {
		cursor: crosshair;
	}
	.scene-pane__labels {
		position: absolute;
		inset: 0;
		pointer-events: none;
	}
	.scene-pane__hint {
		position: absolute;
		top: 0.5rem;
		left: 50%;
		transform: translateX(-50%);
		margin: 0;
		padding: 0.25rem 0.6rem;
		font-size: 0.78rem;
		color: #f3e8ff;
		background: rgba(124, 58, 237, 0.85);
		border-radius: 3px;
		pointer-events: none;
	}
	.scene-pane__controls {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.5rem;
		padding: 0.4rem 0.6rem;
		font-size: 0.8rem;
		color: #d7e3e8;
		background: #12212a;
	}
	.scene-pane__button,
	.scene-pane__select {
		font: inherit;
		color: inherit;
		background: #1c313c;
		border: 1px solid #2c4552;
		border-radius: 3px;
		padding: 0.15rem 0.5rem;
		cursor: pointer;
	}
	.scene-pane__clock {
		font-variant-numeric: tabular-nums;
	}
	.scene-pane__sep {
		opacity: 0.5;
	}
	.scene-pane__toggle {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		cursor: pointer;
	}
	.scene-pane__stats {
		margin-left: auto;
		opacity: 0.75;
		font-variant-numeric: tabular-nums;
	}
	.scene-pane__status,
	.scene-pane__error,
	.scene-pane__empty,
	.scene-pane__warning {
		padding: 0.4rem 0.6rem;
		margin: 0;
		font-size: 0.78rem;
	}
	.scene-pane__error {
		color: #ff9a9a;
	}
	.scene-pane__empty {
		color: #90a4ae;
	}
	.scene-pane__warning {
		color: #ffcc80;
	}
</style>
