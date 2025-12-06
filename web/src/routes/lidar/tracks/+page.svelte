<script lang="ts">
	import { browser } from '$app/environment';
	import { getBackgroundGrid, getTrackHistory } from '$lib/api';
	import MapPane from '$lib/components/lidar/MapPane.svelte';
	import TimelinePane from '$lib/components/lidar/TimelinePane.svelte';
	import TrackList from '$lib/components/lidar/TrackList.svelte';
	import type { BackgroundGrid, Track, TrackObservation } from '$lib/types/lidar';
	import { onDestroy, onMount } from 'svelte';
	import { Header } from 'svelte-ux';

	import { SelectField, ToggleGroup, ToggleOption } from 'svelte-ux';

	// State
	let sensorId = 'hesai-pandar40p';
	let mode: 'live' | 'playback' = 'playback';
	let selectedTime = Date.now();
	let playbackSpeed = 1.0;
	let isPlaying = false;

	// Data
	let tracks: Track[] = [];
	let observations: Record<string, TrackObservation[]> = {};
	let backgroundGrid: BackgroundGrid | null = null;
	let selectedTrackId: string | null = null;

	// Playback state
	let timeRange: { start: number; end: number } | null = null;
	let playbackInterval: number | null = null;

	// Load historical data for playback
	async function loadHistoricalData() {
		try {
			// Get last 24 hours of data
			const endTime = Date.now() * 1e6; // Convert to nanoseconds
			const startTime = endTime - 24 * 60 * 60 * 1e9; // 24 hours ago

			const history = await getTrackHistory(sensorId, startTime, endTime);
			tracks = history.tracks;
			observations = history.observations;

			if (tracks.length > 0) {
				timeRange = {
					start: Math.min(...tracks.map((t) => new Date(t.first_seen).getTime())),
					end: Math.max(...tracks.map((t) => new Date(t.last_seen).getTime()))
				};
				selectedTime = timeRange.start;
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

<main id="main-content" class="bg-gray-50 flex h-full flex-col">
	<!-- Header -->
	<div class="bg-white border-gray-200 px-6 py-4 border-b">
		<div class="flex items-center justify-between">
			<div>
				<Header
					title="LiDAR Track Visualization"
					subheading="Sensor: {sensorId} • {visibleTracks.length} tracks visible"
				/>
			</div>

			<div class="gap-4 flex items-center">
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
		<div class="border-gray-300 bg-gray-900 flex-[3] border-b">
			<MapPane
				tracks={visibleTracks}
				{selectedTrackId}
				{backgroundGrid}
				onTrackSelect={handleTrackSelect}
			/>
		</div>

		<!-- Bottom Pane: Timeline (40%) -->
		<div class="flex flex-[2] overflow-hidden">
			<!-- Timeline -->
			<div class="bg-white flex-1">
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
			<div class="w-80 border-gray-300 bg-white overflow-hidden border-l">
				<TrackList tracks={visibleTracks} {selectedTrackId} onTrackSelect={handleTrackSelect} />
			</div>
		</div>
	</div>
</main>
