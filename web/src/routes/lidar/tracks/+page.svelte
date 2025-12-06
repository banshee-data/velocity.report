<script lang="ts">
	import { browser } from '$app/environment';
	import { getBackgroundGrid, getTrackHistory } from '$lib/api';
	import MapPane from '$lib/components/lidar/MapPane.svelte';
	import TimelinePane from '$lib/components/lidar/TimelinePane.svelte';
	import TrackList from '$lib/components/lidar/TrackList.svelte';
	import type { BackgroundGrid, Track } from '$lib/types/lidar';
	import { onDestroy, onMount } from 'svelte';
	import { SelectField, ToggleGroup, ToggleOption } from 'svelte-ux';

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

	// Playback state
	let timeRange: { start: number; end: number } | null = null;
	let playbackInterval: number | null = null;

	// Load historical data for playback
	async function loadHistoricalData() {
		try {
			// Query ALL historical data (use very wide time range)
			const endTime = Date.now() * 1e6; // Convert to nanoseconds
			const startTime = 0; // Start from epoch to capture all historical data

			console.log(
				'[TrackHistory] Querying tracks from',
				new Date(startTime / 1e6).toISOString(),
				'to',
				new Date(endTime / 1e6).toISOString()
			);

			const history = await getTrackHistory(sensorId, startTime, endTime);
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

				timeRange = {
					start: Math.min(...tracks.map((t) => new Date(t.first_seen).getTime())),
					end: Math.max(...tracks.map((t) => new Date(t.last_seen).getTime()))
				};
				selectedTime = timeRange.start;

				console.log('[TrackHistory] Time range:', {
					start: new Date(timeRange.start).toISOString(),
					end: new Date(timeRange.end).toISOString()
				});
			}
		} catch (error) {
			console.error('Failed to load historical data:', error);
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

	// Playback controls
	function handlePlay() {
		if (!browser || !timeRange) return;

		isPlaying = true;
		playbackInterval = window.setInterval(() => {
			selectedTime += 100 * playbackSpeed; // Advance by 100ms * speed

			// Loop back to start if we reach the end
			if (selectedTime > timeRange!.end) {
				selectedTime = timeRange!.start;
			}
		}, 100); // Update at 10Hz
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

	function handleTrackSelect(trackId: string) {
		selectedTrackId = trackId;
		// Jump to track start time when selected from list
		const track = tracks.find((t) => t.track_id === trackId);
		if (track) {
			selectedTime = new Date(track.first_seen).getTime();
		}
	}

	onMount(() => {
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
