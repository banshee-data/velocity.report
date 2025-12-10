<script lang="ts">
	/**
	 * LiDAR Track Visualization - Main Page
	 *
	 * Two-pane layout for visualizing LiDAR tracking data:
	 * - Top pane (60%): Canvas-based map with background grid overlay
	 * - Bottom pane (40%): SVG timeline with playback controls
	 *
	 * Supports both historical playback (24-hour window) and live streaming (Phase 3).
	 */
	import { browser } from '$app/environment';
	import {
		getBackgroundGrid,
		getTrackHistory,
		getTrackObservations,
		getTrackObservationsRange
	} from '$lib/api';
	import MapPane from '$lib/components/lidar/MapPane.svelte';
	import TimelinePane from '$lib/components/lidar/TimelinePane.svelte';
	import TrackList from '$lib/components/lidar/TrackList.svelte';
	import type { BackgroundGrid, Track, TrackObservation } from '$lib/types/lidar';
	import { onDestroy, onMount } from 'svelte';
	import { SelectField, ToggleGroup, ToggleOption } from 'svelte-ux';

	// Playback constants
	const PLAYBACK_UPDATE_INTERVAL_MS = 100; // Update playback position every 100ms
	const PLAYBACK_UPDATE_FREQUENCY_HZ = 10; // 10Hz

	// State
	let sensorId = 'hesai-pandar40p';
	let mode: 'live' | 'playback' = 'playback';
	let selectedTime = Date.now();
	let playbackSpeed = 1.0;
	let isPlaying = false;

	// Data
	let tracks: Track[] = [];
	let backgroundGrid: BackgroundGrid | null = null;
	let selectedTrackId: string | null = null;
	let observationsByTrack: Record<string, TrackObservation[]> = {};
	let selectedTrackObservations: TrackObservation[] = [];
	let observationsLoading = false;
	let observationsError: string | null = null;
	let observationsRequestId = 0;
	let foregroundObservations: TrackObservation[] = [];
	let foregroundLoading = false;
	let foregroundError: string | null = null;
	let showForeground = true;
	let foregroundOffsetX = 0;
	let foregroundOffsetY = 0;
	let foregroundOffset = { x: 0, y: 0 };
	$: foregroundOffset = { x: foregroundOffsetX, y: foregroundOffsetY };

	// Playback state
	let timeRange: { start: number; end: number } | null = null;
	let playbackInterval: number | null = null;

	// Load historical data for playback
	async function loadHistoricalData() {
		console.log('[TrackHistory] Starting data load for sensor:', sensorId);
		try {
			// Query ALL historical data (use very wide time range)
			const endTime = Date.now() * 1e6; // Convert to nanoseconds
			const startTime = 0; // Start from epoch to capture all historical data

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
			const history = await getTrackHistory(sensorId, startTime, endTime);
			console.log('[TrackHistory] API response:', history);
			tracks = history.tracks;

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
				loadForegroundObservations(timeRange.start, timeRange.end);
			}
		} catch (error) {
			console.error('[TrackHistory] Failed to load historical data:', error);
			if (error instanceof Error) {
				console.error('[TrackHistory] Error message:', error.message);
				console.error('[TrackHistory] Error stack:', error.stack);
			}
		}
	}

	// Load background grid
	async function loadBackgroundGrid() {
		try {
			backgroundGrid = await getBackgroundGrid(sensorId);
		} catch (error) {
			console.error('Failed to load background grid:', error);
		}
	}

	// Get tracks visible at current time
	$: visibleTracks = tracks.filter((track) => {
		const firstSeen = new Date(track.first_seen).getTime();
		const lastSeen = new Date(track.last_seen).getTime();
		return selectedTime >= firstSeen && selectedTime <= lastSeen;
	});

	// Debug visible tracks changes
	let lastVisibleCount = -1;
	$: if (visibleTracks.length !== lastVisibleCount) {
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
			selectedTime += PLAYBACK_UPDATE_INTERVAL_MS * playbackSpeed; // Advance by interval * speed

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
		}, PLAYBACK_UPDATE_INTERVAL_MS); // Update at configured frequency
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

		const requestId = ++observationsRequestId;
		observationsLoading = true;
		observationsError = null;

		try {
			if (!observationsByTrack[trackId]) {
				const obs = await getTrackObservations(trackId);
				observationsByTrack = { ...observationsByTrack, [trackId]: obs };
			}

			if (requestId === observationsRequestId) {
				selectedTrackObservations = observationsByTrack[trackId] ?? [];
			}
		} catch (error) {
			if (requestId === observationsRequestId) {
				observationsError =
					error instanceof Error ? error.message : 'Failed to load track observations';
				selectedTrackObservations = [];
			}
		} finally {
			if (requestId === observationsRequestId) {
				observationsLoading = false;
			}
		}
	}

	async function loadForegroundObservations(startMs?: number, endMs?: number) {
		if (!timeRange && (!startMs || !endMs)) return;

		const windowStart = startMs ?? timeRange!.start;
		const windowEnd = endMs ?? timeRange!.end;

		foregroundLoading = true;
		foregroundError = null;

		try {
			const res = await getTrackObservationsRange(
				sensorId,
				Math.floor(windowStart * 1e6),
				Math.floor(windowEnd * 1e6),
				4000
			);
			foregroundObservations = res.observations ?? [];
		} catch (error) {
			foregroundError =
				error instanceof Error ? error.message : 'Failed to load foreground observations';
			foregroundObservations = [];
		} finally {
			foregroundLoading = false;
		}
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

	onMount(() => {
		console.log('[Page] Component mounted, loading data...');
		loadHistoricalData();
		loadBackgroundGrid();
	});

	onDestroy(() => {
		if (playbackInterval !== null) {
			clearInterval(playbackInterval);
		}
	});
</script>

<main id="main-content" class="bg-surface-200 flex h-full flex-col">
	<!-- Header -->
	<div class="border-surface-content/10 bg-surface-100 h-20 flex-none border-b px-6 py-4">
		<div class="flex h-full items-center justify-between overflow-hidden">
			<div class="min-w-0 flex-1">
				<h1 class="text-surface-content truncate text-2xl font-semibold">
					LiDAR Track Visualization
				</h1>
				<p class="text-surface-content/60 mt-1 truncate text-sm">
					Sensor: {sensorId} • {visibleTracks.length} tracks visible
				</p>
			</div>

			<div class="flex flex-none items-center gap-4 pl-4">
				<!-- Mode Toggle -->
				<ToggleGroup bind:value={mode} variant="outline" size="sm">
					<ToggleOption value="playback">Playback</ToggleOption>
					<ToggleOption value="live" disabled>Live (Coming Soon)</ToggleOption>
				</ToggleGroup>

				<!-- Sensor Selection -->
				<SelectField
					label="Sensor"
					bind:value={sensorId}
					options={[{ label: 'Hesai Pandar40P', value: 'hesai-pandar40p' }]}
					size="sm"
					class="w-48"
				/>

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
	<div class="flex flex-1 flex-col overflow-hidden">
		<!-- Top Pane: Map Visualization (60%) -->
		<div class="border-surface-content/20 bg-surface-300 border-b" style="flex: 3">
			<MapPane
				tracks={visibleTracks}
				{selectedTrackId}
				{backgroundGrid}
				observations={selectedTrackObservations}
				foreground={foregroundObservations}
				foregroundEnabled={showForeground}
				{foregroundOffset}
				onTrackSelect={handleTrackSelect}
			/>
		</div>

		<!-- Bottom Pane: Timeline (40%) -->
		<div class="flex overflow-hidden" style="flex: 2">
			<!-- Timeline -->
			<div class="bg-surface-100 flex-1">
				<TimelinePane
					{tracks}
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
			<div class="border-surface-content/20 bg-surface-100 w-80 overflow-hidden border-l">
				<TrackList {tracks} {selectedTrackId} onTrackSelect={handleTrackSelect} />
			</div>
		</div>
	</div>
</main>
