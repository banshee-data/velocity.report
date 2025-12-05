<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { Button, SelectField, ToggleGroup, ToggleOption } from 'svelte-ux';
	import MapPane from '$lib/components/lidar/MapPane.svelte';
	import TimelinePane from '$lib/components/lidar/TimelinePane.svelte';
	import TrackList from '$lib/components/lidar/TrackList.svelte';
	import { getActiveTracks, getTrackHistory, getBackgroundGrid } from '$lib/api';
	import type { Track, TrackObservation, BackgroundGrid } from '$lib/types/lidar';

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
		if (!timeRange) return;

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

<div class="h-screen flex flex-col bg-gray-50">
	<!-- Header -->
	<div class="bg-white border-b border-gray-200 px-6 py-4">
		<div class="flex items-center justify-between">
			<div>
				<h1 class="text-2xl font-semibold text-gray-900">LiDAR Track Visualization</h1>
				<p class="text-sm text-gray-600 mt-1">
					Sensor: {sensorId} • {visibleTracks.length} tracks visible
				</p>
			</div>

			<div class="flex items-center gap-4">
				<!-- Mode Toggle -->
				<ToggleGroup bind:value={mode} variant="outline" size="sm">
					<ToggleOption value="playback">Playback</ToggleOption>
					<ToggleOption value="live" disabled>Live (Coming Soon)</ToggleOption>
				</ToggleGroup>

				<!-- Sensor Selection -->
				<SelectField
					label="Sensor"
					bind:value={sensorId}
					options={[
						{ label: 'Hesai Pandar40P', value: 'hesai-pandar40p' }
					]}
					size="sm"
					class="w-48"
				/>
			</div>
		</div>
	</div>

	<!-- Main Content: Two-Pane Layout -->
	<div class="flex-1 flex flex-col overflow-hidden">
		<!-- Top Pane: Map Visualization (60%) -->
		<div class="flex-[3] border-b border-gray-300 bg-gray-900">
			<MapPane
				tracks={visibleTracks}
				{selectedTrackId}
				{backgroundGrid}
				onTrackSelect={handleTrackSelect}
			/>
		</div>

		<!-- Bottom Pane: Timeline (40%) -->
		<div class="flex-[2] flex overflow-hidden">
			<!-- Timeline -->
			<div class="flex-1 bg-white">
				<TimelinePane
					{tracks}
					{observations}
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
			<div class="w-80 border-l border-gray-300 bg-white overflow-hidden">
				<TrackList
					tracks={visibleTracks}
					{selectedTrackId}
					onTrackSelect={handleTrackSelect}
				/>
			</div>
		</div>
	</div>
</div>

<style>
	:global(body) {
		margin: 0;
		padding: 0;
		overflow: hidden;
	}
</style>
