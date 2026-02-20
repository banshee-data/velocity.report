<script lang="ts">
	/**
	 * LiDAR Track Visualization - Main Page
	 *
	 * Two-pane layout for visualizing LiDAR tracking data:
	 * - Top pane (60%): Canvas-based map with background grid overlay
	 * - Bottom pane (40%): SVG timeline with playback controls
	 *
	 * Supports both historical playback (24-hour window) and live streaming.
	 * Includes scene/run selection and track labelling workflow.
	 */
	import { browser } from '$app/environment';
	import { page } from '$app/stores';
	import {
		createMissedRegion,
		deleteMissedRegion,
		getBackgroundGrid,
		getLabellingProgress,
		getLidarRuns,
		getLidarScenes,
		getMissedRegions,
		getRunTracks,
		getTrackHistory,
		getTrackObservations,
		getTrackObservationsRange
	} from '$lib/api';
	import MapPane from '$lib/components/lidar/MapPane.svelte';
	import TimelinePane from '$lib/components/lidar/TimelinePane.svelte';
	import TrackList from '$lib/components/lidar/TrackList.svelte';
	import type {
		AnalysisRun,
		BackgroundGrid,
		LabellingProgress,
		LidarScene,
		MissedRegion,
		RunTrack,
		Track,
		TrackObservation
	} from '$lib/types/lidar';
	import { onDestroy, onMount, untrack } from 'svelte';
	import { SelectField } from 'svelte-ux';

	// Playback constants
	const PLAYBACK_UPDATE_INTERVAL_MS = 100; // Update playback position every 100ms
	const PLAYBACK_UPDATE_FREQUENCY_HZ = 10; // 10Hz

	// State
	let sensorId: string;
	// Reactive to URL changes - updates when user navigates with different sensor_id param
	$: sensorId = $page.url.searchParams.get('sensor_id') || 'hesai-pandar40p';
	let selectedTime = Date.now();
	let playbackSpeed = 1.0;
	let isPlaying = false;

	// Scene and run selection
	let scenes: LidarScene[] = [];
	let selectedSceneId: string | null = null;
	let runs: AnalysisRun[] = [];
	let selectedRunId: string | null = null;
	let runTracks: RunTrack[] = [];
	let labellingProgress: LabellingProgress | null = null;
	let scenesLoading = false;
	let runsLoading = false;

	// Derived state
	$: selectedScene = scenes.find((s) => s.scene_id === selectedSceneId) ?? null;
	$: selectedRun = runs.find((r) => r.run_id === selectedRunId) ?? null;

	// Data
	let tracks: Track[] = [];
	let paginatedTracks: Track[] = []; // Tracks currently visible in the paginated list
	let backgroundGrid: BackgroundGrid | null = null;
	let selectedTrackId: string | null = null;
	let observationsByTrack: Record<string, TrackObservation[]> = {};
	let selectedTrackObservations: TrackObservation[] = [];
	let observationsRequestId = 0;
	// TODO: Add observationsLoading:boolean and observationsError:string|null state variables.
	// Display loading indicator in TrackList component when observationsLoading is true.
	// Show error banner above timeline when observationsError is set, with retry button.
	let foregroundObservations: TrackObservation[] = [];
	let foregroundLoading = false;
	let foregroundError: string | null = null;
	let showForeground = true;
	// Foreground observation viewport tracking (task 6.5).
	// When selectedTime drifts outside the last-queried window, reload.
	let fgWindowCentre = 0;
	const FG_RELOAD_DRIFT_MS = 20_000; // reload when playback drifts >20 s from centre
	let foregroundOffsetX = 0;
	let foregroundOffsetY = 0;
	let foregroundOffset = { x: 0, y: 0 };
	$: foregroundOffset = { x: foregroundOffsetX, y: foregroundOffsetY };

	// Missed regions state
	let missedRegions: MissedRegion[] = [];
	let markMissedMode = false;

	// Playback state
	let timeRange: { start: number; end: number } | null = null;
	let playbackInterval: number | null = null;

	// Resize handle state
	let topPaneHeight: number | null = null;
	let containerRef: HTMLDivElement;

	// Track sensorId changes and reload data when it changes (client-side navigation)
	// Guard to prevent duplicate loads on initial mount
	let previousSensorId: typeof sensorId | null = null;
	let hasSeenSensorIdOnce = false;
	$: if (browser) {
		// Use untrack for guard variables to prevent Svelte 5 infinite reactive loops.
		// Only sensorId should be a reactive dependency here.
		const seen = untrack(() => hasSeenSensorIdOnce);
		const prev = untrack(() => previousSensorId);
		if (!seen) {
			// Record the initial sensorId without triggering duplicate loads
			previousSensorId = sensorId;
			hasSeenSensorIdOnce = true;
		} else if (sensorId !== prev) {
			previousSensorId = sensorId;
			// Reset state
			tracks = [];
			backgroundGrid = null;
			selectedTrackId = null;
			selectedSceneId = null;
			selectedRunId = null;
			scenes = [];
			runs = [];
			runTracks = [];
			observationsByTrack = {};
			selectedTrackObservations = [];
			foregroundObservations = [];
			missedRegions = [];
			timeRange = null;
			// Reload data for new sensor
			void loadHistoricalData(); // eslint-disable-line svelte/infinite-reactive-loop
			void loadBackgroundGrid(); // eslint-disable-line svelte/infinite-reactive-loop
			void loadScenes(); // eslint-disable-line svelte/infinite-reactive-loop
		}
	}

	// Load historical data for playback
	async function loadHistoricalData() {
		console.log('[TrackHistory] Starting data load for sensor:', sensorId);
		try {
			// Query last 1 hour of historical data (bounded window).
			// Using epoch (startTime = 0) would load ALL data, causing excessive
			// load times, UI clutter, and exposure to historical artefacts.
			const endTime = Date.now() * 1e6; // Convert to nanoseconds
			const startTime = (Date.now() - 3_600_000) * 1e6; // Last 1 hour in nanoseconds

			console.log(
				'[TrackHistory] Querying tracks from',
				new Date(startTime / 1e6).toISOString(),
				'to',
				new Date(endTime / 1e6).toISOString(),
				'(startTime:',
				startTime,
				'endTime:',
				endTime,
				')'
			);

			console.log('[TrackHistory] Calling API...');
			const history = await getTrackHistory(sensorId, startTime, endTime, 1000);
			console.log('[TrackHistory] API response:', history);
			tracks = Array.isArray(history.tracks) ? history.tracks : []; // eslint-disable-line svelte/infinite-reactive-loop

			console.log('[TrackHistory] Loaded', tracks.length, 'tracks');

			if (tracks.length > 0) {
				// Sample first track for debugging
				const firstTrack = tracks[0];
				console.log('[TrackHistory] First track:', {
					track_id: firstTrack.track_id,
					first_seen: firstTrack.first_seen,
					last_seen: firstTrack.last_seen,
					position: firstTrack.position,
					history_length: firstTrack.history?.length || 0
				});

				// Check for invalid timestamps
				const lastSeenTimes = tracks.map((t) => new Date(t.last_seen).getTime());
				const validLastSeen = lastSeenTimes.filter((t) => t > 0 && t < Date.now());
				console.log(
					'[TrackHistory] last_seen times - total:',
					lastSeenTimes.length,
					'valid:',
					validLastSeen.length,
					'sample:',
					lastSeenTimes.slice(0, 3)
				);

				// eslint-disable-next-line svelte/infinite-reactive-loop
				timeRange = {
					start: Math.min(...tracks.map((t) => new Date(t.first_seen).getTime())),
					end: Math.max(...tracks.map((t) => new Date(t.last_seen).getTime()))
				};
				selectedTime = timeRange.start;

				console.log('[TrackHistory] Time range:', {
					start: new Date(timeRange.start).toISOString(),
					end: new Date(timeRange.end).toISOString(),
					startMs: timeRange.start,
					endMs: timeRange.end
				});
			} else {
				console.warn('[TrackHistory] No tracks loaded!');
			}

			// Load foreground observation overlay once we know the time window
			if (timeRange) {
				loadForegroundObservations(timeRange.start, timeRange.end); // eslint-disable-line svelte/infinite-reactive-loop
			}
		} catch (error) {
			console.error('[TrackHistory] Failed to load historical data:', error);
			if (error instanceof Error) {
				console.error('[TrackHistory] Error message:', error.message);
				console.error('[TrackHistory] Error stack:', error.stack);
			}
		}
	}

	// Load scenes for selected sensor
	async function loadScenes() {
		scenesLoading = true;
		try {
			scenes = await getLidarScenes(sensorId); // eslint-disable-line svelte/infinite-reactive-loop
		} catch {
			scenes = []; // eslint-disable-line svelte/infinite-reactive-loop
		} finally {
			scenesLoading = false;
		}
	}

	// Load runs for selected scene's sensor
	async function loadRuns(scene: LidarScene) {
		runsLoading = true;
		try {
			runs = await getLidarRuns({ sensor_id: scene.sensor_id });
		} catch {
			runs = [];
		} finally {
			runsLoading = false;
		}
	}

	// Load tracks for selected run
	async function loadRunTracks() {
		if (!selectedRunId) {
			runTracks = [];
			labellingProgress = null;
			return;
		}
		try {
			runTracks = await getRunTracks(selectedRunId);

			// Load labelling progress
			try {
				labellingProgress = await getLabellingProgress(selectedRunId);
			} catch {
				labellingProgress = null;
			}
		} catch {
			runTracks = [];
			labellingProgress = null;
		}
	}

	// Handle scene selection change
	// Looks up the scene directly from scenes array rather than relying on
	// derived $: selectedScene which may not have updated yet.
	function handleSceneChange() {
		selectedRunId = null;
		runTracks = [];
		labellingProgress = null;
		if (selectedSceneId !== null) {
			const scene = scenes.find((s) => s.scene_id === selectedSceneId);
			if (scene) {
				loadRuns(scene);
			} else {
				runs = [];
			}
		} else {
			runs = [];
			missedRegions = [];
			markMissedMode = false;
		}
	}

	// Handle run selection change
	function handleRunChange() {
		if (selectedRunId !== null) {
			loadRunTracks().then(() => {
				// Scope time range to this run's tracks
				if (runTracks.length > 0) {
					const runStart = Math.min(...runTracks.map((rt) => rt.start_unix_nanos / 1e6));
					const runEnd = Math.max(...runTracks.map((rt) => rt.end_unix_nanos / 1e6));
					timeRange = { start: runStart, end: runEnd };
					selectedTime = runStart;
				}
			});
			loadMissedRegions();
		} else {
			runTracks = [];
			labellingProgress = null;
			missedRegions = [];
			markMissedMode = false;
			// Restore full time range from all tracks
			if (tracks.length > 0) {
				timeRange = {
					start: Math.min(...tracks.map((t) => new Date(t.first_seen).getTime())),
					end: Math.max(...tracks.map((t) => new Date(t.last_seen).getTime()))
				};
				selectedTime = timeRange.start;
			}
		}
	}

	// Load background grid
	async function loadBackgroundGrid() {
		try {
			backgroundGrid = await getBackgroundGrid(sensorId); // eslint-disable-line svelte/infinite-reactive-loop
		} catch (error) {
			console.error('Failed to load background grid:', error);
		}
	}

	// Run-scoped track filtering: when a run is selected, build a Map keyed by
	// track_id so we can filter by both identity AND the run's own time window
	// (task 5.2). This avoids false positives from global ID membership alone.
	let runTrackMap: Map<string, RunTrack> | null = null;
	$: runTrackMap =
		selectedRunId && runTracks.length > 0
			? new Map(runTracks.map((rt) => [rt.track_id, rt]))
			: null;

	// Get tracks visible at current time, filtered by run if selected
	$: visibleTracks = tracks.filter((track) => {
		if (runTrackMap) {
			const rt = runTrackMap.get(track.track_id);
			if (!rt) return false;
			// Use the run-track's own nanosecond time window for scoping
			const selectedTimeNs = selectedTime * 1e6;
			return selectedTimeNs >= rt.start_unix_nanos && selectedTimeNs <= rt.end_unix_nanos;
		}
		const firstSeen = new Date(track.first_seen).getTime();
		const lastSeen = new Date(track.last_seen).getTime();
		return selectedTime >= firstSeen && selectedTime <= lastSeen;
	});

	// Run-scoped foreground observations
	$: visibleForeground = runTrackMap
		? foregroundObservations.filter((obs) => runTrackMap.has(obs.track_id))
		: foregroundObservations;

	// Run-scoped tracks for TrackList sidebar
	$: listTracks = runTrackMap ? tracks.filter((t) => runTrackMap.has(t.track_id)) : tracks;

	// Debug visible tracks changes
	let lastVisibleCount = -1;
	$: if (visibleTracks.length !== untrack(() => lastVisibleCount)) {
		console.log(
			'[VisibleTracks] At time',
			new Date(selectedTime).toISOString(),
			':',
			visibleTracks.length,
			'/',
			tracks.length,
			'tracks visible'
		);
		if (visibleTracks.length > 0) {
			console.log(
				'[VisibleTracks] Sample:',
				visibleTracks[0].track_id,
				'range:',
				visibleTracks[0].first_seen,
				'to',
				visibleTracks[0].last_seen
			);
		}
		lastVisibleCount = visibleTracks.length;
	}

	// Playback controls
	function handlePlay() {
		if (!browser || !timeRange) {
			console.log('[Playback] Cannot play - browser:', browser, 'timeRange:', timeRange);
			return;
		}

		console.log(
			'[Playback] Starting at',
			new Date(selectedTime).toISOString(),
			'speed:',
			playbackSpeed,
			'range:',
			new Date(timeRange!.start).toISOString(),
			'to',
			new Date(timeRange!.end).toISOString()
		);
		isPlaying = true;
		let tickCount = 0;
		playbackInterval = window.setInterval(() => {
			selectedTime += PLAYBACK_UPDATE_INTERVAL_MS * playbackSpeed;

			// Log every 10 ticks (1 second)
			if (++tickCount % PLAYBACK_UPDATE_FREQUENCY_HZ === 0) {
				console.log(
					'[Playback] Time:',
					new Date(selectedTime).toISOString(),
					'visible:',
					visibleTracks.length
				);
			}

			// Loop back to start if we reach the end
			if (selectedTime > timeRange!.end) {
				console.log('[Playback] Reached end, looping back');
				selectedTime = timeRange!.start;
				tickCount = 0;
			}
		}, PLAYBACK_UPDATE_INTERVAL_MS);
	}

	function handlePause() {
		if (!browser) return;
		isPlaying = false;
		if (playbackInterval !== null) {
			clearInterval(playbackInterval);
			playbackInterval = null;
		}
	}

	function handlePlaybackToggle() {
		if (isPlaying) {
			handlePause();
		} else {
			handlePlay();
		}
	}

	function handleTimeChange(newTime: number) {
		selectedTime = newTime;
	}

	function handleSpeedChange(speed: number) {
		playbackSpeed = speed;
		if (isPlaying) {
			handlePause();
			handlePlay();
		}
	}

	async function loadObservationsForTrack(trackId: string | null) {
		if (!trackId) {
			selectedTrackObservations = [];
			return;
		}

		// Concurrent request cancellation pattern: Each new request increments observationsRequestId.
		// Previous in-flight requests are cancelled by checking if their requestId still matches
		// the latest observationsRequestId before updating state.
		const requestId = ++observationsRequestId;

		try {
			if (!observationsByTrack[trackId]) {
				const obs = await getTrackObservations(trackId);
				observationsByTrack = { ...observationsByTrack, [trackId]: obs };
			}

			// Only update if this request is still the latest
			if (requestId === observationsRequestId) {
				selectedTrackObservations = observationsByTrack[trackId] ?? [];
			}
		} catch (error) {
			// Only handle error if this request is still the latest
			if (requestId === observationsRequestId) {
				// TODO: Set observationsError to error.message and display as:
				//   1. Error banner component above timeline with red background
				//   2. Include "Retry" button that calls loadObservationsForTrack(trackId) again
				//   3. Include "Dismiss" button that clears the error
				//   4. Auto-dismiss after 10 seconds
				// For now, errors are logged to console for debugging.
				console.error('[LiDAR] Failed to load track observations:', error);
				selectedTrackObservations = [];
			}
		}
		// Note: No finally block - loading state cleanup to be added later.
		// Trade-off: Users won't see loading indicators for track observations initially.
		// This is acceptable as the observations load quickly and users have visual feedback
		// from the track selection itself.
	}

	async function loadForegroundObservations(startMs?: number, endMs?: number) {
		if (!timeRange && (!startMs || !endMs)) return;

		// Scope the query to a ±30-second window around the current playback
		// position (task 6.5). This avoids sampling bias where a fixed limit
		// of 4 000 observations spread over the full time range under-
		// represents the visible viewport.
		const FG_WINDOW_MS = 30_000;
		const centre = selectedTime || ((startMs ?? timeRange!.start) + (endMs ?? timeRange!.end)) / 2;
		const windowStart = Math.max(startMs ?? timeRange!.start, centre - FG_WINDOW_MS);
		const windowEnd = Math.min(endMs ?? timeRange!.end, centre + FG_WINDOW_MS);
		fgWindowCentre = centre;

		foregroundLoading = true;
		foregroundError = null;

		try {
			const res = await getTrackObservationsRange(
				sensorId,
				Math.floor(windowStart * 1e6),
				Math.floor(windowEnd * 1e6),
				4000
			);
			foregroundObservations = res.observations ?? []; // eslint-disable-line svelte/infinite-reactive-loop
		} catch (error) {
			foregroundError =
				error instanceof Error ? error.message : 'Failed to load foreground observations';
			foregroundObservations = []; // eslint-disable-line svelte/infinite-reactive-loop
		} finally {
			foregroundLoading = false; // eslint-disable-line svelte/infinite-reactive-loop
		}
	}

	// Reactive foreground reload when playback drifts outside queried window (task 6.5)
	$: if (
		showForeground &&
		!foregroundLoading &&
		fgWindowCentre > 0 &&
		Math.abs(selectedTime - fgWindowCentre) > FG_RELOAD_DRIFT_MS
	) {
		loadForegroundObservations(); // eslint-disable-line svelte/infinite-reactive-loop
	}

	function handleTrackSelect(trackId: string) {
		selectedTrackId = trackId;
		// Jump to track start time when selected from list
		const track = tracks.find((t) => t.track_id === trackId);
		if (track) {
			selectedTime = new Date(track.first_seen).getTime();
		}
		loadObservationsForTrack(trackId);
	}

	// Load missed regions for current run
	async function loadMissedRegions() {
		if (!selectedRunId) {
			missedRegions = [];
			return;
		}
		try {
			missedRegions = await getMissedRegions(selectedRunId);
		} catch {
			missedRegions = [];
		}
	}

	// Handle map click in mark-missed mode
	async function handleMapClick(worldX: number, worldY: number) {
		if (!markMissedMode || !selectedRunId || !timeRange) return;

		try {
			const region = await createMissedRegion(selectedRunId, {
				center_x: worldX,
				center_y: worldY,
				radius_m: 3.0,
				time_start_ns: Math.floor(selectedTime * 1e6),
				time_end_ns: Math.floor(selectedTime * 1e6) + 5_000_000_000 // +5 seconds
			});
			missedRegions = [...missedRegions, region];
		} catch (error) {
			console.error('[MissedRegions] Failed to create missed region:', error);
		}
	}

	// Delete a missed region
	async function handleDeleteMissedRegion(regionId: string) {
		if (!selectedRunId) return;
		try {
			await deleteMissedRegion(selectedRunId, regionId);
			missedRegions = missedRegions.filter((r) => r.region_id !== regionId);
		} catch (error) {
			console.error('[MissedRegions] Failed to delete missed region:', error);
		}
	}

	function handleResizeStart(event: MouseEvent) {
		event.preventDefault();
		const onMouseMove = (e: MouseEvent) => {
			if (!containerRef) return;
			const rect = containerRef.getBoundingClientRect();
			const minH = 100;
			const maxH = rect.height - 150;
			topPaneHeight = Math.max(minH, Math.min(maxH, e.clientY - rect.top));
		};
		const onMouseUp = () => {
			window.removeEventListener('mousemove', onMouseMove);
			window.removeEventListener('mouseup', onMouseUp);
			document.body.style.cursor = '';
			document.body.style.userSelect = '';
		};
		document.body.style.cursor = 'row-resize';
		document.body.style.userSelect = 'none';
		window.addEventListener('mousemove', onMouseMove);
		window.addEventListener('mouseup', onMouseUp);
	}

	onMount(async () => {
		console.log('[Page] Component mounted, loading data...');
		loadHistoricalData();
		loadBackgroundGrid();

		// Load scenes and optionally pre-select from URL query params
		await loadScenes();
		const params = $page.url.searchParams;
		const qsSceneId = params.get('scene_id');
		const qsRunId = params.get('run_id');
		if (qsSceneId && scenes.find((s) => s.scene_id === qsSceneId)) {
			selectedSceneId = qsSceneId;
			const scene = scenes.find((s) => s.scene_id === qsSceneId);
			if (scene) {
				await loadRuns(scene);
				if (qsRunId && runs.find((r) => r.run_id === qsRunId)) {
					selectedRunId = qsRunId;
					loadRunTracks();
					loadMissedRegions();
				}
			}
		}
	});

	onDestroy(() => {
		if (playbackInterval !== null) {
			clearInterval(playbackInterval);
		}
	});
</script>

<main id="main-content" class="vr-page">
	<!-- Header -->
	<div class="vr-toolbar h-20">
		<div class="flex h-full items-center justify-between overflow-hidden">
			<div class="min-w-0 flex-1">
				<h1 class="text-surface-content truncate text-2xl font-semibold">
					LiDAR Track Visualization
				</h1>
				<p class="text-surface-content/60 mt-1 truncate text-sm">
					Sensor: {sensorId} • {visibleTracks.length} tracks visible
					{#if selectedRun}
						• Run: {selectedRun.run_id.substring(0, 8)}
					{/if}
				</p>
			</div>

			<div class="flex flex-none items-center gap-4 pl-4">
				<!-- Sensor Selection -->
				<SelectField
					label="Sensor"
					value={sensorId}
					options={[{ label: 'Hesai Pandar40P', value: 'hesai-pandar40p' }]}
					size="sm"
					class="w-48"
					disabled
				/>

				<!-- Scene Selection -->
				<SelectField
					label="Scene"
					bind:value={selectedSceneId}
					on:change={handleSceneChange}
					options={[
						{ label: 'None (Historical)', value: null },
						...scenes.map((s) => ({
							label: s.description || s.scene_id,
							value: s.scene_id
						}))
					]}
					disabled={scenesLoading || scenes.length === 0}
					size="sm"
					class="w-56"
				/>

				<!-- Run Selection (only shown when scene selected) -->
				{#if selectedScene}
					<SelectField
						label="Run"
						bind:value={selectedRunId}
						on:change={handleRunChange}
						options={[
							{ label: 'Select a run...', value: null },
							...runs.map((r) => ({
								label: `${r.run_id.substring(0, 8)} (${r.total_tracks} tracks)`,
								value: r.run_id
							}))
						]}
						disabled={runsLoading || runs.length === 0}
						size="sm"
						class="w-64"
					/>
				{/if}

				<!-- Mark Missed button (visible when run is selected) -->
				{#if selectedRunId}
					<button
						on:click={() => (markMissedMode = !markMissedMode)}
						class="rounded px-3 py-1.5 text-xs font-medium transition-colors {markMissedMode
							? 'bg-purple-600 text-white'
							: 'bg-surface-200 text-surface-content hover:bg-surface-300'}"
						title="Click on the map to mark areas where objects were missed"
					>
						{markMissedMode ? 'Stop Marking' : 'Mark Missed'}
						{#if missedRegions.length > 0}
							({missedRegions.length})
						{/if}
					</button>
				{/if}

				<!-- Foreground overlay controls -->
				<div class="text-surface-content flex items-center gap-3 text-xs">
					<label class="flex items-center gap-2">
						<input type="checkbox" bind:checked={showForeground} class="h-4 w-4" />
						<span>Foreground overlay</span>
					</label>
					<label class="flex items-center gap-1">
						<span>Offset X</span>
						<input
							type="number"
							step="0.25"
							bind:value={foregroundOffsetX}
							class="border-surface-content/30 bg-surface-50 w-20 rounded border px-2 py-1"
						/>
					</label>
					<label class="flex items-center gap-1">
						<span>Offset Y</span>
						<input
							type="number"
							step="0.25"
							bind:value={foregroundOffsetY}
							class="border-surface-content/30 bg-surface-50 w-20 rounded border px-2 py-1"
						/>
					</label>
					{#if foregroundLoading}
						<span class="text-surface-content/70">Loading…</span>
					{:else if foregroundError}
						<span class="text-error-500">{foregroundError}</span>
					{/if}
				</div>
			</div>
		</div>
	</div>

	<!-- Main Content: Two-Pane Layout -->
	<div class="flex flex-1 flex-col overflow-hidden" bind:this={containerRef}>
		<!-- Top Pane: Map Visualization -->
		<div
			class="border-surface-content/20 bg-surface-300 border-b"
			style={topPaneHeight !== null ? `height: ${topPaneHeight}px; flex-shrink: 0` : 'flex: 3'}
		>
			<MapPane
				tracks={visibleTracks}
				{selectedTrackId}
				{backgroundGrid}
				currentTime={selectedTime}
				observations={selectedTrackObservations}
				foreground={visibleForeground}
				foregroundEnabled={showForeground}
				{foregroundOffset}
				onTrackSelect={handleTrackSelect}
				{missedRegions}
				{markMissedMode}
				onMapClick={handleMapClick}
				onDeleteMissedRegion={handleDeleteMissedRegion}
			/>
		</div>

		<!-- Resize Handle -->
		<!-- svelte-ignore a11y-no-static-element-interactions -->
		<div
			class="bg-surface-200 hover:bg-primary/30 flex-none cursor-row-resize transition-colors select-none"
			style="height: 6px"
			on:mousedown={handleResizeStart}
		>
			<div class="bg-surface-content/20 mx-auto mt-[2px] h-[2px] w-8 rounded-full"></div>
		</div>

		<!-- Bottom Pane: Timeline -->
		<div class="flex flex-1 overflow-hidden">
			<!-- Timeline -->
			<div class="bg-surface-100 flex-1">
				<TimelinePane
					tracks={paginatedTracks.length > 0 ? paginatedTracks : tracks}
					currentTime={selectedTime}
					{timeRange}
					{isPlaying}
					{playbackSpeed}
					onTimeChange={handleTimeChange}
					onPlaybackToggle={handlePlaybackToggle}
					onSpeedChange={handleSpeedChange}
					{selectedTrackId}
					onTrackSelect={handleTrackSelect}
				/>
			</div>

			<!-- Track List Sidebar -->
			<div class="border-surface-content/20 bg-surface-100 w-[500px] overflow-hidden border-l">
				<TrackList
					tracks={listTracks}
					{selectedTrackId}
					onTrackSelect={handleTrackSelect}
					onPaginatedTracksChange={(newTracks) => (paginatedTracks = newTracks)}
					runId={selectedRunId}
					{runTracks}
					{labellingProgress}
				/>
			</div>
		</div>
	</div>
</main>
