<script lang="ts">
	/**
	 * ScenePane — the 3D scene view for the operator tools.
	 *
	 * This replaces the flat top-down canvas map with the same three.js player
	 * the public scenes run on. The player is imported from
	 * public_html/src/js, not copied: a copy would drift the moment either
	 * side fixed a camera, a trail or a colour.
	 *
	 * It plays one of two sources. A run with a recording plays that recording,
	 * exported by the server the way a published survey is (manifestURL), with
	 * the settled background it holds. Anything else plays observations read
	 * from the database through a session built over them; those carry no run,
	 * so for a replayed capture they mix every replay of it.
	 *
	 * With a recording, the player owns the clock: the page follows it through
	 * onClock and drives it through seekToMs, setPlaying and setRate.
	 */
	import { mountScenePlayer } from '$scene/scene-player.js';
	import type { SceneInteraction, SceneSession } from '$scene/scene-reader.js';
	import { createLiveSceneSession } from '#lib/scene/liveSceneSource.js';
	import { clockFollower, startMsOf, type PageClock } from '#lib/scene/sceneClock.js';
	import type { MissedRegion, RunTrack, TrackObservation } from '#lib/types/lidar.js';
	import { onDestroy } from 'svelte';

	export let observations: TrackObservation[] = [];
	export let runTracks: RunTrack[] = [];
	export let sensorId = 'hesai-pandar40p';
	export let title = 'Live run';
	/** A run's exported recording. When set, it is played instead of observations. */
	export let manifestURL: string | null = null;
	/** The recording's settled background, drawn behind the boxes. */
	export let backgroundURL: string | null = null;
	/** Called as a recording plays, about ten times a second, and on any control change. */
	export let onClock: ((clock: PageClock) => void) | null = null;

	/** A run's missed regions, drawn as rings on the ground. Shown, not edited. */
	export let missedRegions: MissedRegion[] = [];
	export let onTrackSelect: (trackId: string) => void = () => {};
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
	// The mounted player. A remount disposes it first: the next player takes
	// the same canvas and controls, and one left running would keep drawing
	// into the canvas and answering the Play button alongside it.
	let player: SceneSession | null = null;
	// Where the mounted recording starts, in Unix milliseconds.
	let startMs = 0;
	// Mounting is asynchronous, so a newer mount can start before an older one
	// returns; the older one's player is disposed as soon as it arrives.
	let mountSeq = 0;

	/**
	 * Remounting on every observation change would reset the camera mid-review,
	 * so the player is mounted once the first data arrives and the key is
	 * tracked to detect a genuinely different run. A recording mounts once.
	 */
	let mountedKey = '';

	$: sceneKey = manifestURL
		? `recording|${manifestURL}`
		: `${sensorId}|${observations.length}|${observations[0]?.timestamp ?? ''}|${
				observations[observations.length - 1]?.timestamp ?? ''
			}`;

	$: if (canvas && (manifestURL || observations.length > 0) && sceneKey !== mountedKey) {
		mountedKey = sceneKey;
		void mount();
	}

	/** Moves a recording's playhead to an absolute time. */
	export function seekToMs(ms: number) {
		player?.playback?.seek((ms - startMs) / 1000);
	}

	export function setPlaying(playing: boolean) {
		player?.playback?.setPlaying(playing);
	}

	export function setRate(value: number) {
		player?.playback?.setRate(value);
	}

	function disposePlayer() {
		player?.dispose?.();
		player = null;
		interaction = null;
	}

	async function mount() {
		mountError = null;
		const seq = ++mountSeq;
		disposePlayer();
		const ui = {
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
		};
		try {
			let session: SceneSession;
			if (manifestURL) {
				skipped = 0;
				frameCount = 0;
				session = await mountScenePlayer({
					canvas,
					manifestURL,
					ui: { ...ui, backgroundURL: backgroundURL ?? undefined }
				});
			} else {
				const built = createLiveSceneSession({
					observations,
					runTracks,
					sensorId,
					title
				});
				skipped = built.skippedObservations;
				frameCount = built.frameCount;
				session = await mountScenePlayer({ canvas, session: built.session, ui });
			}
			if (seq !== mountSeq) {
				session.dispose?.();
				return;
			}
			player = session;
			startMs = startMsOf(session.parts[0]?.startNs);
			if (manifestURL && onClock && session.playback) {
				session.playback.onChange(clockFollower(startMs, (c) => onClock?.(c)));
			}
			interaction = session.interaction ?? null;
			interaction?.setRegions(missedRegions);
			mounted = true;
		} catch (error) {
			mountError = error instanceof Error ? error.message : String(error);
		}
	}

	/**
	 * A click selects the track under it. The player answers which, because it
	 * owns the camera and the ENU-to-scene mapping.
	 */
	function handleCanvasClick(event: MouseEvent) {
		if (!interaction) return;
		const trackId = interaction.trackAt(event.clientX, event.clientY);
		if (trackId) onTrackSelect(trackId);
	}

	// Regions are pushed to the overlay whenever they change, not only at
	// mount, so a newly loaded set appears without a remount.
	$: if (interaction) interaction.setRegions(missedRegions);

	onDestroy(() => {
		mountSeq++;
		disposePlayer();
		mounted = false;
	});
</script>

<div class="scene-pane">
	<canvas bind:this={canvas} class="scene-pane__canvas" on:click={handleCanvasClick}></canvas>
	<div bind:this={labelLayer} class="scene-pane__labels"></div>

	<div class="scene-pane__controls">
		<button bind:this={playToggle} type="button" class="scene-pane__button">Play</button>
		<select bind:this={rate} class="scene-pane__select" aria-label="Playback rate">
			<!-- The Tracks page's speeds, so a rate it sets is one this can show. -->
			<option value="0.5">0.5x</option>
			<option value="1">1x</option>
			<option value="2">2x</option>
			<option value="5">5x</option>
			<option value="10">10x</option>
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
	{:else if !manifestURL && observations.length === 0}
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
	.scene-pane__labels {
		position: absolute;
		inset: 0;
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
