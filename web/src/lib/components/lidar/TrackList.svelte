<script lang="ts">
	import type { Track } from '$lib/types/lidar';
	import { TRACK_COLORS } from '$lib/types/lidar';

	export let tracks: Track[] = [];
	export let selectedTrackId: string | null = null;
	export let onTrackSelect: (trackId: string) => void = () => {};
	// Callback to notify parent when paginated tracks change
	export let onPaginatedTracksChange: ((tracks: Track[]) => void) | null = null;

	// Filter and sort options
	let classFilter: string = 'all';
	let stateFilter: string = 'all';
	let sortBy: 'time' | 'speed' | 'duration' = 'time';
	let minObservations: number = 5; // Filter out tracks with fewer observations

	// Pagination
	const PAGE_SIZE = 50;
	let currentPage = 0;

	// Filtered and sorted tracks
	$: filteredTracks = tracks
		.filter((track) => {
			// Filter by minimum observations (reduces noise from single-point tracks)
			if (track.observation_count < minObservations) return false;
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

	// Pagination computed values
	$: totalPages = Math.ceil(filteredTracks.length / PAGE_SIZE);
	$: paginatedTracks = filteredTracks.slice(currentPage * PAGE_SIZE, (currentPage + 1) * PAGE_SIZE);

	// Notify parent when paginated tracks change
	$: if (onPaginatedTracksChange) {
		onPaginatedTracksChange(paginatedTracks);
	}

	// Reset to first page when filters change
	$: if (classFilter || stateFilter || sortBy || minObservations) {
		currentPage = 0;
	}

	function goToPage(page: number) {
		currentPage = Math.max(0, Math.min(page, totalPages - 1));
	}

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
	const SECONDS_PER_MINUTE = 60;
	function formatDuration(seconds: number): string {
		if (seconds < SECONDS_PER_MINUTE) return `${seconds.toFixed(0)}s`;
		const minutes = Math.floor(seconds / SECONDS_PER_MINUTE);
		const secs = seconds % SECONDS_PER_MINUTE;
		return `${minutes}m ${secs.toFixed(0)}s`;
	}
</script>

<div class="bg-surface-100 flex h-full flex-col">
	<!-- Header -->
	<div class="border-surface-content/10 border-b px-4 py-3">
		<h3 class="text-surface-content font-semibold">Tracks ({filteredTracks.length})</h3>
	</div>

	<!-- Filters -->
	<div class="border-surface-content/10 space-y-3 border-b px-4 py-3">
		<!-- Class Filter -->
		<div>
			<label for="class-filter" class="text-surface-content/70 mb-1 block text-xs font-medium"
				>Class</label
			>
			<select
				id="class-filter"
				bind:value={classFilter}
				class="border-surface-content/20 bg-surface-100 text-surface-content focus:border-primary focus:ring-primary w-full rounded-md text-sm shadow-sm"
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
			<label for="state-filter" class="text-surface-content/70 mb-1 block text-xs font-medium"
				>State</label
			>
			<select
				id="state-filter"
				bind:value={stateFilter}
				class="border-surface-content/20 bg-surface-100 text-surface-content focus:border-primary focus:ring-primary w-full rounded-md text-sm shadow-sm"
			>
				<option value="all">All</option>
				<option value="confirmed">Confirmed</option>
				<option value="tentative">Tentative</option>
			</select>
		</div>

		<!-- Minimum Observations Filter -->
		<div>
			<label for="min-obs" class="text-surface-content/70 mb-1 block text-xs font-medium"
				>Min Observations</label
			>
			<select
				id="min-obs"
				bind:value={minObservations}
				class="border-surface-content/20 bg-surface-100 text-surface-content focus:border-primary focus:ring-primary w-full rounded-md text-sm shadow-sm"
			>
				<option value={1}>1+ (all)</option>
				<option value={5}>5+ (default)</option>
				<option value={10}>10+</option>
				<option value={20}>20+</option>
				<option value={50}>50+</option>
			</select>
		</div>

		<!-- Sort By -->
		<div>
			<label for="sort-by" class="text-surface-content/70 mb-1 block text-xs font-medium"
				>Sort By</label
			>
			<select
				id="sort-by"
				bind:value={sortBy}
				class="border-surface-content/20 bg-surface-100 text-surface-content focus:border-primary focus:ring-primary w-full rounded-md text-sm shadow-sm"
			>
				<option value="time">Start Time</option>
				<option value="speed">Speed</option>
				<option value="duration">Duration</option>
			</select>
		</div>
	</div>

	<!-- Track List -->
	<div class="min-h-0 flex-1 overflow-y-auto">
		{#each paginatedTracks as track (track.track_id)}
			{@const isSelected = track.track_id === selectedTrackId}
			{@const color =
				track.object_class && track.object_class in TRACK_COLORS
					? TRACK_COLORS[track.object_class as keyof typeof TRACK_COLORS]
					: TRACK_COLORS.other}

			<button
				on:click={() => onTrackSelect(track.track_id)}
				class="border-surface-content/10 hover:bg-surface-200 w-full border-b px-4 py-3 text-left transition-colors {isSelected
					? 'border-l-primary bg-primary/10 border-l-4'
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
						<div class="text-surface-content truncate font-mono text-sm font-medium">
							{track.track_id}
						</div>

						<!-- Classification -->
						{#if track.object_class}
							<div class="mt-1 flex items-center gap-2">
								<span class="inline-block h-3 w-3 rounded-full" style="background-color: {color}"
								></span>
								<span class="text-surface-content/70 text-xs capitalize">
									{track.object_class}
									{#if track.object_confidence}
										({(track.object_confidence * 100).toFixed(0)}%)
									{/if}
								</span>
							</div>
						{/if}

						<!-- Stats -->
						<div class="text-surface-content/60 mt-2 space-y-1 text-xs">
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

		{#if paginatedTracks.length === 0}
			<div class="text-surface-content/50 px-4 py-8 text-center text-sm">No tracks found</div>
		{/if}
	</div>

	<!-- Pagination Controls -->
	{#if totalPages > 1}
		<div class="border-surface-content/10 flex items-center justify-between border-t px-4 py-2">
			<span class="text-surface-content/60 text-xs">
				Page {currentPage + 1} of {totalPages}
				<span class="text-surface-content/40">({filteredTracks.length} tracks)</span>
			</span>
			<div class="flex gap-1">
				<button
					on:click={() => goToPage(0)}
					disabled={currentPage === 0}
					class="hover:bg-surface-200 rounded px-2 py-1 text-xs disabled:opacity-30 disabled:hover:bg-transparent"
					title="First page"
				>
					⏮
				</button>
				<button
					on:click={() => goToPage(currentPage - 1)}
					disabled={currentPage === 0}
					class="hover:bg-surface-200 rounded px-2 py-1 text-xs disabled:opacity-30 disabled:hover:bg-transparent"
					title="Previous page"
				>
					◀
				</button>
				<button
					on:click={() => goToPage(currentPage + 1)}
					disabled={currentPage >= totalPages - 1}
					class="hover:bg-surface-200 rounded px-2 py-1 text-xs disabled:opacity-30 disabled:hover:bg-transparent"
					title="Next page"
				>
					▶
				</button>
				<button
					on:click={() => goToPage(totalPages - 1)}
					disabled={currentPage >= totalPages - 1}
					class="hover:bg-surface-200 rounded px-2 py-1 text-xs disabled:opacity-30 disabled:hover:bg-transparent"
					title="Last page"
				>
					⏭
				</button>
			</div>
		</div>
	{/if}
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
