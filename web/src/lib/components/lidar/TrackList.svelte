<script lang="ts">
	import type { Track } from '$lib/types/lidar';
	import { TRACK_COLORS } from '$lib/types/lidar';

	export let tracks: Track[] = [];
	export let selectedTrackId: string | null = null;
	export let onTrackSelect: (trackId: string) => void = () => {};

	// Filter and sort options
	let classFilter: string = 'all';
	let stateFilter: string = 'all';
	let sortBy: 'time' | 'speed' | 'duration' = 'time';

	// Filtered and sorted tracks
	$: filteredTracks = tracks
		.filter((track) => {
			if (classFilter !== 'all' && track.object_class !== classFilter) return false;
			if (stateFilter !== 'all' && track.state !== stateFilter) return false;
			return true;
		})
		.sort((a, b) => {
			switch (sortBy) {
				case 'speed':
					return b.avg_speed_mps - a.avg_speed_mps;
				case 'duration':
					return b.age_seconds - a.age_seconds;
				case 'time':
				default:
					return new Date(a.first_seen).getTime() - new Date(b.first_seen).getTime();
			}
		});

	// Get class icon
	function getClassIcon(track: Track): string {
		switch (track.object_class) {
			case 'pedestrian':
				return '🚶';
			case 'car':
				return '🚗';
			case 'bird':
				return '🦅';
			default:
				return '❓';
		}
	}

	// Format duration
	function formatDuration(seconds: number): string {
		if (seconds < 60) return `${seconds.toFixed(0)}s`;
		const minutes = Math.floor(seconds / 60);
		const secs = seconds % 60;
		return `${minutes}m ${secs.toFixed(0)}s`;
	}
</script>

<div class="flex h-full flex-col">
	<!-- Header -->
	<div class="border-b border-gray-200 px-4 py-3">
		<h3 class="font-semibold text-gray-900">Tracks ({filteredTracks.length})</h3>
	</div>

	<!-- Filters -->
	<div class="space-y-3 border-b border-gray-200 px-4 py-3">
		<!-- Class Filter -->
		<div>
			<label for="class-filter" class="mb-1 block text-xs font-medium text-gray-700">Class</label>
			<select
				id="class-filter"
				bind:value={classFilter}
				class="w-full rounded-md border-gray-300 text-sm shadow-sm focus:border-blue-500 focus:ring-blue-500"
			>
				<option value="all">All</option>
				<option value="pedestrian">Pedestrian</option>
				<option value="car">Car</option>
				<option value="bird">Bird</option>
				<option value="other">Other</option>
			</select>
		</div>

		<!-- State Filter -->
		<div>
			<label for="state-filter" class="mb-1 block text-xs font-medium text-gray-700">State</label>
			<select
				id="state-filter"
				bind:value={stateFilter}
				class="w-full rounded-md border-gray-300 text-sm shadow-sm focus:border-blue-500 focus:ring-blue-500"
			>
				<option value="all">All</option>
				<option value="confirmed">Confirmed</option>
				<option value="tentative">Tentative</option>
			</select>
		</div>

		<!-- Sort By -->
		<div>
			<label for="sort-by" class="mb-1 block text-xs font-medium text-gray-700">Sort By</label>
			<select
				id="sort-by"
				bind:value={sortBy}
				class="w-full rounded-md border-gray-300 text-sm shadow-sm focus:border-blue-500 focus:ring-blue-500"
			>
				<option value="time">Start Time</option>
				<option value="speed">Speed</option>
				<option value="duration">Duration</option>
			</select>
		</div>
	</div>

	<!-- Track List -->
	<div class="flex-1 overflow-y-auto">
		{#each filteredTracks as track}
			{@const isSelected = track.track_id === selectedTrackId}
			{@const color =
				track.object_class && track.object_class in TRACK_COLORS
					? TRACK_COLORS[track.object_class as keyof typeof TRACK_COLORS]
					: TRACK_COLORS.other}

			<button
				on:click={() => onTrackSelect(track.track_id)}
				class="w-full border-b border-gray-100 px-4 py-3 text-left transition-colors hover:bg-gray-50 {isSelected
					? 'border-l-4 border-l-blue-500 bg-blue-50'
					: ''}"
			>
				<div class="flex items-start gap-3">
					<!-- Icon -->
					<div class="flex-shrink-0 text-2xl">
						{getClassIcon(track)}
					</div>

					<!-- Content -->
					<div class="min-w-0 flex-1">
						<!-- Track ID -->
						<div class="truncate font-mono text-sm font-medium text-gray-900">
							{track.track_id}
						</div>

						<!-- Classification -->
						{#if track.object_class}
							<div class="mt-1 flex items-center gap-2">
								<span class="inline-block h-3 w-3 rounded-full" style="background-color: {color}"
								></span>
								<span class="text-xs text-gray-600 capitalize">
									{track.object_class}
									{#if track.object_confidence}
										({(track.object_confidence * 100).toFixed(0)}%)
									{/if}
								</span>
							</div>
						{/if}

						<!-- Stats -->
						<div class="mt-2 space-y-1 text-xs text-gray-600">
							<div class="flex justify-between">
								<span>Speed:</span>
								<span class="font-medium">{track.avg_speed_mps.toFixed(1)} m/s</span>
							</div>
							<div class="flex justify-between">
								<span>Duration:</span>
								<span class="font-medium">{formatDuration(track.age_seconds)}</span>
							</div>
							<div class="flex justify-between">
								<span>Observations:</span>
								<span class="font-medium">{track.observation_count}</span>
							</div>
						</div>

						<!-- State Badge -->
						{#if track.state === 'tentative'}
							<div class="mt-2">
								<span
									class="inline-flex items-center rounded bg-orange-100 px-2 py-0.5 text-xs font-medium text-orange-800"
								>
									Tentative
								</span>
							</div>
						{/if}
					</div>
				</div>
			</button>
		{/each}

		{#if filteredTracks.length === 0}
			<div class="px-4 py-8 text-center text-sm text-gray-500">No tracks found</div>
		{/if}
	</div>
</div>

<style>
	select {
		padding: 0.375rem 0.75rem;
		border: 1px solid #d1d5db;
		border-radius: 0.375rem;
		background-color: white;
		cursor: pointer;
	}

	select:focus {
		outline: none;
		border-color: #3b82f6;
		box-shadow: 0 0 0 1px #3b82f6;
	}
</style>
