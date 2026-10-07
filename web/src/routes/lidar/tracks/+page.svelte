<script lang="ts">
	/**
	 * LiDAR Track Visualization - Main Page
	 *
	 * Two-pane layout for visualizing LiDAR tracking data:
	 * - Top pane (60%): Canvas-based map with background grid overlay
	 * - Bottom pane (40%): SVG timeline with playback controls
	 *
	 * Supports both historical playback (24-hour window) and live streaming.
	 * Includes clip/run selection and track labelling workflow.
	 */
	import { browser } from '$app/env';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import {
		getLabellingProgress,
		getLidarReplayCases,
		getLidarRun,
		getLidarRuns,
		getMissedRegions,
		getRunTracks,
		getTrackHistory,
		getTrackObservationsRange
	} from '#lib/api.js';
	import ScenePane from '#lib/components/lidar/ScenePane.svelte';
	import TimelinePane from '#lib/components/lidar/TimelinePane.svelte';
	import TrackList from '#lib/components/lidar/TrackList.svelte';
	import { unixNanosToMillis } from '#lib/dateUtils.js';
	import { clipQuery, runQuery } from '#lib/lidarLinks.js';
	import type {
		AnalysisRun,
		LabellingProgress,
		LidarReplayCase,
		MissedRegion,
		RunTrack,
		Track,
		TrackObservation
	} from '#lib/types/lidar.js';
	import { onDestroy, onMount, untrack } from 'svelte';
	import { SelectField } from 'svelte-ux';
	import { toStore } from 'svelte/store';

	// $app/state is runes-only: a legacy `$:` statement that reads page
	// directly runs once and never sees a later navigation. toStore makes
	// page.url a store again, so the statements below keep following it.
	const pageUrl = toStore(() => page.url);

	// Playback constants
	const PLAYBACK_UPDATE_INTERVAL_MS = 100; // Update playback position every 100ms

	// State
	let sensorId: string;
	// Reactive to URL changes - updates when user navigates with different sensor_id param
	$: sensorId = $pageUrl.searchParams.get('sensor_id') || 'hesai-pandar40p';
	let selectedTime = Date.now();
	let playbackSpeed = 1.0;
	let isPlaying = false;

	// Scene and run selection
	let scenes: LidarReplayCase[] = [];
	let selectedSceneId: string | null = null;
	let runs: AnalysisRun[] = [];
	let selectedRunId: string | null = null;
	let runTracks: RunTrack[] = [];
	let labellingProgress: LabellingProgress | null = null;
	let scenesLoading = false;
	let runsLoading = false;
	let mounted = false;
	let selectionSyncInFlight = false;
	let lastUrlSelectionKey: string | null = null;

	$: querySceneId = $pageUrl.searchParams.get('replay_case_id');
	$: queryRunId = $pageUrl.searchParams.get('run_id');

	// Derived state
	$: selectedScene = scenes.find((s) => s.replay_case_id === selectedSceneId) ?? null;
	$: selectedRun = runs.find((r) => r.run_id === selectedRunId) ?? null;

	$: if (browser && mounted) {
		void syncSelectionFromUrl(querySceneId, queryRunId);
	}

	function runTrackToDisplayTrack(runTrack: RunTrack): Track {
		const firstSeen = new Date(runTrack.start_unix_nanos / 1e6).toISOString();
		const lastSeen = new Date(runTrack.end_unix_nanos / 1e6).toISOString();
		return {
			track_id: runTrack.track_id,
			sensor_id: runTrack.sensor_id,
			state: (runTrack.track_state as Track['state']) || 'confirmed',
			position: { x: 0, y: 0, z: 0 },
			velocity: { vx: 0, vy: 0 },
			speed_mps: runTrack.max_speed_mps || runTrack.avg_speed_mps || 0,
			heading_rad: 0,
			object_class: runTrack.object_class as Track['object_class'],
			object_confidence: runTrack.object_confidence,
			observation_count: runTrack.observation_count,
			age_seconds: Math.max(0, (runTrack.end_unix_nanos - runTrack.start_unix_nanos) / 1e9),
			avg_speed_mps: runTrack.avg_speed_mps,
			max_speed_mps: runTrack.max_speed_mps,
			obb_heading_rad: 0,
			bounding_box: {
				length: runTrack.bounding_box_length_avg,
				width: runTrack.bounding_box_width_avg,
				height: runTrack.bounding_box_height_avg
			},
			first_seen: firstSeen,
			last_seen: lastSeen,
			history: []
		};
	}

	// Data
	let tracks: Track[] = [];
	let paginatedTracks: Track[] = []; // Tracks currently visible in the paginated list
	let selectedTrackId: string | null = null;
	// TODO: Add observationsLoading:boolean and observationsError:string|null state variables.
	// Display loading indicator in TrackList component when observationsLoading is true.
	// Show error banner above timeline when observationsError is set, with retry button.
	let foregroundObservations: TrackObservation[] = [];
	let foregroundLoading = false;
	let foregroundError: string | null = null;
	// Foreground observation viewport tracking (task 6.5).
	// When selectedTime drifts outside the last-queried window, reload.
	let fgWindowCentre = 0;
	const FG_RELOAD_DRIFT_MS = 20_000; // reload when playback drifts >20 s from centre

	// A run's missed regions, shown, never edited here
	let missedRegions: MissedRegion[] = [];

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
			selectedTrackId = null;
			selectedSceneId = null;
			selectedRunId = null;
			scenes = [];
			runs = [];
			runTracks = [];
			foregroundObservations = [];
			missedRegions = [];
			timeRange = null;
			// Reload data for new sensor
			void loadHistoricalData(); // eslint-disable-line svelte/infinite-reactive-loop
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
			console.error('[TrackHistory] Could not load historical data:', error);
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
			scenes = await getLidarReplayCases(sensorId); // eslint-disable-line svelte/infinite-reactive-loop
		} catch {
			scenes = []; // eslint-disable-line svelte/infinite-reactive-loop
		} finally {
			scenesLoading = false;
		}
	}

	// Load runs for selected scene's sensor
	async function loadRuns(scene: LidarReplayCase) {
		runsLoading = true;
		try {
			runs = await getLidarRuns({ sensor_id: scene.sensor_id });
			return runs;
		} catch {
			runs = [];
			return [];
		} finally {
			runsLoading = false;
		}
	}

	function clearRunSelectionState() {
		selectedRunId = null;
		runTracks = [];
		labellingProgress = null;
		selectedTrackId = null;
		missedRegions = [];
	}

	function findSceneForRun(run: AnalysisRun | null): LidarReplayCase | null {
		if (!run) return null;
		const byReference = scenes.find((scene) => scene.reference_run_id === run.run_id);
		if (byReference) return byReference;
		if (run.source_path) {
			return (
				scenes.find(
					(scene) => scene.pcap_file === run.source_path && scene.sensor_id === run.sensor_id
				) ?? null
			);
		}
		return null;
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

	// Reload lidar_tracks for the current run's time window.
	// PCAP replay tracks carry the PCAP capture timestamp, not wall-clock time,
	// so the default "last 1 hour" window misses them entirely.
	async function loadTracksForRunWindow() {
		if (runTracks.length === 0) return;
		const runStartNs = Math.min(...runTracks.map((rt) => rt.start_unix_nanos));
		const runEndNs = Math.max(...runTracks.map((rt) => rt.end_unix_nanos));
		try {
			// The run's own tracks are its run tracks. The history endpoint has
			// no run filter: over a run's window it returns up to 1,000 tracks
			// from every replay of that capture (9 MB for kirk0) and none of
			// this run's, so it is not fetched in run mode.
			tracks = [];
			timeRange = { start: runStartNs / 1e6, end: runEndNs / 1e6 };
			selectedTime = runStartNs / 1e6;
			// The timeline works in milliseconds, so the requested time is
			// converted before it is compared, never compared as nanoseconds.
			const requestedMs = unixNanosToMillis($pageUrl.searchParams.get('at_ns'));
			if (requestedMs !== null && requestedMs >= timeRange.start && requestedMs <= timeRange.end) {
				selectedTime = requestedMs;
			}
			loadForegroundObservations(timeRange.start, timeRange.end);
		} catch (error) {
			console.error('[TrackHistory] Could not load tracks for run window:', error);
		}
	}

	async function syncSelectionFromUrl(qsSceneId: string | null, qsRunId: string | null) {
		if (selectionSyncInFlight) return;
		const syncKey = `${sensorId}|${qsSceneId ?? ''}|${qsRunId ?? ''}`;
		if (syncKey === lastUrlSelectionKey) return;

		selectionSyncInFlight = true;
		try {
			if (!qsSceneId && !qsRunId) {
				if (selectedSceneId !== null || selectedRunId !== null || runs.length > 0) {
					selectedSceneId = null;
					runs = [];
					clearRunSelectionState();
					await loadHistoricalData();
				}
				return;
			}

			if (scenes.length === 0 && !scenesLoading) {
				await loadScenes();
			}

			let resolvedScene = qsSceneId
				? (scenes.find((scene) => scene.replay_case_id === qsSceneId) ?? null)
				: null;
			let resolvedRuns: AnalysisRun[] = [];

			if (resolvedScene) {
				selectedSceneId = resolvedScene.replay_case_id;
				resolvedRuns = await loadRuns(resolvedScene);
			} else if (qsRunId) {
				resolvedRuns = await getLidarRuns({ sensor_id: sensorId });
				runs = resolvedRuns;
				const resolvedRun = resolvedRuns.find((run) => run.run_id === qsRunId) ?? null;
				resolvedScene = findSceneForRun(resolvedRun);
				selectedSceneId = resolvedScene?.replay_case_id ?? null;
				if (resolvedScene) {
					resolvedRuns = await loadRuns(resolvedScene);
				}
			} else {
				selectedSceneId = null;
				runs = [];
			}

			if (!qsRunId) {
				clearRunSelectionState();
				return;
			}

			let resolvedRun = resolvedRuns.find((run) => run.run_id === qsRunId) ?? null;
			if (!resolvedRun) {
				// The run list is paged; a linked run older than it is fetched
				// directly, so a link from Runs or Clips still opens it.
				try {
					resolvedRun = await getLidarRun(qsRunId);
					runs = [resolvedRun, ...runs];
				} catch {
					clearRunSelectionState();
					return;
				}
			}

			selectedRunId = resolvedRun.run_id;
			selectedTrackId = null;
			await loadRunTracks();
			await loadTracksForRunWindow();
			await loadMissedRegions();
		} finally {
			lastUrlSelectionKey = syncKey;
			selectionSyncInFlight = false;
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
			const scene = scenes.find((s) => s.replay_case_id === selectedSceneId);
			if (scene) {
				loadRuns(scene);
			} else {
				runs = [];
			}
		} else {
			runs = [];
			missedRegions = [];
		}
	}

	// Handle run selection change
	function handleRunChange() {
		if (selectedRunId !== null) {
			// loadTracksForRunWindow uses runTracks state, so await loadRunTracks first
			loadRunTracks()
				.then(() => loadTracksForRunWindow())
				.catch((error) => console.error('[TrackHistory] handleRunChange pipeline failed:', error));
			loadMissedRegions();
		} else {
			runTracks = [];
			labellingProgress = null;
			missedRegions = [];
			// Reload the default window: tracks may have been scoped to the run's window
			void loadHistoricalData();
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
	$: detailedTrackIds = new Set(tracks.map((track) => track.track_id));
	$: runTrackGeometryLinked = runTracks.some((runTrack) => detailedTrackIds.has(runTrack.track_id));
	$: runDisplayTracks = runTracks.map(runTrackToDisplayTrack);
	// Each track's window in milliseconds, parsed once per load rather than
	// on every playback tick.
	$: runTrackWindows = runDisplayTracks.map((track) => ({
		track,
		from: Date.parse(track.first_seen),
		to: Date.parse(track.last_seen)
	}));
	$: trackWindows = tracks.map((track) => ({
		track,
		from: Date.parse(track.first_seen),
		to: Date.parse(track.last_seen)
	}));
	$: visibleRunTracks = runTrackWindows
		.filter((w) => selectedTime >= w.from && selectedTime <= w.to)
		.map((w) => w.track);
	$: displayTracks = selectedRunId ? runDisplayTracks : tracks;

	// Get tracks visible at current time, filtered by run if selected
	$: visibleTracks = trackWindows
		.filter(({ track, from, to }) => {
			if (runTrackMap && runTrackGeometryLinked) {
				const rt = runTrackMap.get(track.track_id);
				if (!rt) return false;
				// Use the run-track's own nanosecond time window for scoping
				const selectedTimeNs = selectedTime * 1e6;
				return selectedTimeNs >= rt.start_unix_nanos && selectedTimeNs <= rt.end_unix_nanos;
			}
			return selectedTime >= from && selectedTime <= to;
		})
		.map((w) => w.track);

	// Run-scoped foreground observations
	$: visibleForeground =
		runTrackMap && runTrackGeometryLinked
			? foregroundObservations.filter((obs) => runTrackMap.has(obs.track_id))
			: foregroundObservations;

	// Run-selected sidebar/timeline always use run-native summary tracks.
	$: listTracks = displayTracks;

	// Playback controls
	function handlePlay() {
		if (!browser || !timeRange) {
			console.log('[Playback] Cannot play - browser:', browser, 'timeRange:', timeRange);
			return;
		}

		isPlaying = true;
		playbackInterval = window.setInterval(() => {
			selectedTime += PLAYBACK_UPDATE_INTERVAL_MS * playbackSpeed;
			// Loop back to start if we reach the end
			if (selectedTime > timeRange!.end) selectedTime = timeRange!.start;
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
				error instanceof Error ? error.message : 'Could not load foreground observations.';
			foregroundObservations = []; // eslint-disable-line svelte/infinite-reactive-loop
		} finally {
			foregroundLoading = false; // eslint-disable-line svelte/infinite-reactive-loop
		}
	}

	// Reactive foreground reload when playback drifts outside queried window
	// (task 6.5). No longer gated on an overlay toggle: these observations are
	// what the scene draws, so not loading them would leave it empty.
	$: if (
		!foregroundLoading &&
		fgWindowCentre > 0 &&
		Math.abs(selectedTime - fgWindowCentre) > FG_RELOAD_DRIFT_MS
	) {
		loadForegroundObservations(); // eslint-disable-line svelte/infinite-reactive-loop
	}

	function handleTrackSelect(trackId: string) {
		if (selectedRunId && !displayTracks.some((track) => track.track_id === trackId)) {
			return;
		}
		selectedTrackId = trackId;
		// Jump to track start time when selected from list
		const track = displayTracks.find((t) => t.track_id === trackId);
		if (track) {
			selectedTime = new Date(track.first_seen).getTime();
		}
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
		await Promise.all([loadHistoricalData(), loadScenes()]);
		mounted = true;
		await syncSelectionFromUrl(querySceneId, queryRunId);
	});

	onDestroy(() => {
		if (playbackInterval !== null) {
			clearInterval(playbackInterval);
		}
	});
</script>

<svelte:head>
	<title>Tracks — velocity.report</title>
</svelte:head>

<main id="main-content" class="vr-page">
	<!-- Header -->
	<div class="vr-toolbar h-20">
		<div class="flex h-full items-center justify-between overflow-hidden">
			<div class="min-w-0 flex-1">
				<h1 class="text-surface-content truncate text-2xl font-semibold">LiDAR Tracks</h1>
				<!-- The clip and run lead, so the links survive the truncation a
				     narrow toolbar applies to the rest of the line. -->
				<p class="text-surface-content/60 mt-1 truncate text-sm">
					<!-- eslint-disable svelte/no-navigation-without-resolve -->
					{#if selectedScene}
						Clip:
						<a
							href={`${resolve('/lidar/replay-cases')}?${clipQuery(selectedScene.replay_case_id)}`}
							class="text-primary hover:underline"
							>{selectedScene.description || selectedScene.replay_case_id.substring(0, 8)}</a
						>
						•
					{/if}
					{#if selectedRun}
						Run:
						<a
							href={`${resolve('/lidar/runs')}?${runQuery(selectedRun.run_id)}`}
							class="text-primary font-mono hover:underline">{selectedRun.run_id.substring(0, 8)}</a
						>
						•
					{/if}
					<!-- eslint-enable svelte/no-navigation-without-resolve -->
					{selectedRunId ? visibleRunTracks.length : visibleTracks.length} tracks visible • Sensor: {sensorId}
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

				<!-- Clip selection -->
				<SelectField
					label="Clip"
					bind:value={selectedSceneId}
					on:change={handleSceneChange}
					options={[
						{ label: 'None (Historical)', value: null },
						...scenes.map((s) => ({
							label: s.description || s.replay_case_id,
							value: s.replay_case_id
						}))
					]}
					disabled={scenesLoading || scenes.length === 0}
					size="sm"
					class="w-56"
				/>

				<!-- Run selection (only shown when a clip is selected) -->
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

				<!-- Missed regions are shown read-only: like labels, they are a
				     reviewer's verdict, and the web is not where those are made. -->
				{#if selectedRunId && missedRegions.length > 0}
					<span
						class="text-surface-content/70 text-xs"
						title={missedRegions
							.map(
								(region) =>
									`${region.center_x.toFixed(1)}, ${region.center_y.toFixed(1)} · ${region.radius_m.toFixed(1)} m`
							)
							.join('\n')}
					>
						{missedRegions.length} missed region{missedRegions.length === 1 ? '' : 's'}
					</span>
				{/if}

				<!-- Observation load state. The layer toggles that used to live
				     here are now in the scene pane itself, next to the view
				     they affect; the manual overlay offset went with the flat
				     map it existed to align. -->
				<div class="text-surface-content flex items-center gap-3 text-xs">
					{#if foregroundLoading}
						<span class="text-surface-content/70">Loading observations…</span>
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
			<!-- The 3D scene view runs the same three.js player as the public
			     scenes, driven by a session built over these live
			     observations rather than a published export. -->
			<ScenePane
				observations={visibleForeground}
				{runTracks}
				{sensorId}
				title={selectedSceneId ?? 'Live run'}
				{missedRegions}
				onTrackSelect={handleTrackSelect}
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
					tracks={paginatedTracks.length > 0 ? paginatedTracks : displayTracks}
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
