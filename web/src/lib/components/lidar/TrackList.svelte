<script lang="ts">
	/**
	 * The track list beside the scene player. Labels, quality flags and links
	 * are shown, never edited: the macOS visualiser is where tracks are
	 * labelled, so the web has no second writer to disagree with it.
	 */
	import type { LabellingProgress, RunTrack, Track } from '#lib/types/lidar.js';
	import { TRACK_COLORS } from '#lib/types/lidar.js';

	export let tracks: Track[] = [];
	export let selectedTrackId: string | null = null;
	export let onTrackSelect: (trackId: string) => void = () => {};
	// Callback to notify parent when paginated tracks change
	export let onPaginatedTracksChange: ((tracks: Track[]) => void) | null = null;

	// A run's labels, shown alongside its tracks
	export let runId: string | null = null;
	export let runTracks: RunTrack[] = [];
	export let labellingProgress: LabellingProgress | null = null;

	// Filter and sort options
	let classFilter: string = 'all';
	let stateFilter: string = 'all';
	let labelFilter: string = 'all'; // filter by label
	let sortBy: 'time' | 'speed' | 'duration' = 'time';
	let minObservations: number = 5; // Filter out tracks with fewer observations

	// Pagination
	const PAGE_SIZE = 50;
	let currentPage = 0;

	// The run track for the selected track, when a run is shown
	$: selectedRunTrack =
		runId && selectedTrackId ? (runTrackMap.get(selectedTrackId) ?? null) : null;

	// Build a lookup map for run tracks (O(1) lookups instead of O(n²) find)
	$: runTrackMap = new Map(runTracks.map((rt) => [rt.track_id, rt]));

	// Filtered and sorted tracks
	$: filteredTracks = tracks
		.filter((track) => {
			// Filter by minimum observations (reduces noise from single-point tracks)
			if (track.observation_count < minObservations) return false;
			if (classFilter !== 'all' && track.object_class !== classFilter) return false;
			if (stateFilter !== 'all' && track.state !== stateFilter) return false;

			// Filter by label status
			if (runId && labelFilter !== 'all') {
				const runTrack = runTrackMap.get(track.track_id);
				if (labelFilter === 'unlabelled' && runTrack?.user_label) return false;
				if (labelFilter === 'labelled' && !runTrack?.user_label) return false;
				if (labelFilter !== 'unlabelled' && labelFilter !== 'labelled') {
					if (runTrack?.user_label !== labelFilter) return false;
				}
			}

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
	$: if (classFilter || stateFilter || sortBy || minObservations || labelFilter) {
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
			case 'truck':
				return '🚚';
			case 'bus':
				return '🚌';
			case 'cyclist':
				return '🚲';
			case 'motorcyclist':
				return '🏍️';
			case 'bird':
				return '🦅';
			default:
				return '❓';
		}
	}

	// Get label badge colour
	function getLabelColor(label: string): string {
		if (label === 'car') return 'bg-blue-100 text-blue-800';
		if (label === 'truck') return 'bg-orange-100 text-orange-800';
		if (label === 'bus') return 'bg-purple-100 text-purple-800';
		if (label === 'pedestrian') return 'bg-green-100 text-green-800';
		if (label === 'cyclist') return 'bg-cyan-100 text-cyan-800';
		if (label === 'motorcyclist') return 'bg-pink-100 text-pink-800';
		if (label === 'noise') return 'bg-red-100 text-red-800';
		return 'bg-gray-100 text-gray-800';
	}

	// Get quality badge colour
	function getQualityColor(label: string): string {
		if (label === 'good') return 'bg-green-100 text-green-800';
		if (label === 'noisy') return 'bg-orange-100 text-orange-800';
		if (label === 'jitter_velocity') return 'bg-orange-100 text-orange-800';
		if (label === 'jitter_heading') return 'bg-orange-100 text-orange-800';
		if (label === 'merge') return 'bg-yellow-100 text-yellow-800';
		if (label === 'split') return 'bg-yellow-100 text-yellow-800';
		if (label === 'truncated') return 'bg-purple-100 text-purple-800';
		if (label === 'disconnected') return 'bg-purple-100 text-purple-800';
		return 'bg-gray-100 text-gray-800';
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

		<!-- Labelling Progress Bar -->
		{#if labellingProgress}
			<div class="mt-2">
				<div class="text-surface-content/70 mb-1 flex items-center justify-between text-xs">
					<span>Labelling Progress</span>
					<span
						>{labellingProgress.labelled} / {labellingProgress.total} ({labellingProgress.progress_pct.toFixed(
							0
						)}%)</span
					>
				</div>
				<div class="bg-surface-200 h-2 overflow-hidden rounded-full">
					<div
						class="bg-primary h-full transition-all duration-300"
						style="width: {labellingProgress.progress_pct}%"
					></div>
				</div>
			</div>
		{/if}
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
				<option value="bus">Bus</option>
				<option value="cyclist">Cyclist</option>
				<option value="bird">Bird</option>
				<option value="dynamic">Dynamic</option>
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

		<!-- Label Filter (only show when in labelling mode) -->
		{#if runId}
			<div>
				<label for="label-filter" class="text-surface-content/70 mb-1 block text-xs font-medium"
					>Label</label
				>
				<select
					id="label-filter"
					bind:value={labelFilter}
					class="border-surface-content/20 bg-surface-100 text-surface-content focus:border-primary focus:ring-primary w-full rounded-md text-sm shadow-sm"
				>
					<option value="all">All</option>
					<option value="unlabelled">Unlabelled</option>
					<option value="labelled">Labelled</option>
					<option value="car">Car</option>
					<option value="bus">Bus</option>
					<option value="pedestrian">Pedestrian</option>
					<option value="cyclist">Cyclist</option>
					<option value="bird">Bird</option>
					<option value="dynamic">Dynamic</option>
					<option value="noise">Noise</option>
					<option value="good">Good</option>
					<option value="noisy">Noisy</option>
					<option value="jitter_velocity">Jitter Velocity</option>
					<option value="jitter_heading">Jitter Heading</option>
					<option value="merge">Merge</option>
					<option value="split">Split</option>
					<option value="truncated">Truncated</option>
					<option value="disconnected">Disconnected</option>
				</select>
			</div>
		{/if}

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
					: TRACK_COLORS.dynamic}
			{@const runTrack = runId ? (runTrackMap.get(track.track_id) ?? null) : null}

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
						<div
							class="text-surface-content flex items-center gap-2 truncate font-mono text-sm font-medium"
						>
							{track.track_id}
							<!-- Linked track indicator -->
							{#if runTrack?.linked_track_ids && runTrack.linked_track_ids.length > 0}
								<span
									class="text-xs"
									title="Linked to {runTrack.linked_track_ids.length} other track(s)"
								>
									🔗
								</span>
							{/if}
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

						<!-- Label Badges -->
						{#if runTrack}
							<div class="mt-2 flex flex-wrap gap-1">
								{#if runTrack.user_label}
									<span
										class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium {getLabelColor(
											runTrack.user_label
										)}"
									>
										{runTrack.user_label.replace('_', ' ')}
									</span>
								{/if}
								{#if runTrack.quality_label}
									{#each runTrack.quality_label
										.split(',')
										.map((s) => s.trim())
										.filter((s) => s.length > 0) as flag (flag)}
										<span
											class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium {getQualityColor(
												flag
											)}"
										>
											{flag.replace('_', ' ')}
										</span>
									{/each}
								{/if}
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

	<!-- The selected track's labels, read-only: they are edited in the macOS app -->
	{#if !runId}
		<div class="border-surface-content/10 border-t px-4 py-3">
			<p class="text-surface-content/50 text-xs">
				Select a clip and run from the header to see the run's labels.
			</p>
		</div>
	{:else if selectedTrackId && selectedRunTrack}
		<div class="border-surface-content/10 space-y-2 border-t px-4 py-3 text-xs">
			<h4 class="text-surface-content text-sm font-semibold">Labels</h4>
			<dl class="space-y-1">
				<div class="flex justify-between gap-2">
					<dt class="text-surface-content/60">Class</dt>
					<dd class="text-surface-content">
						{selectedRunTrack.user_label ? selectedRunTrack.user_label.replace('_', ' ') : '—'}
					</dd>
				</div>
				<div class="flex justify-between gap-2">
					<dt class="text-surface-content/60">Flags</dt>
					<dd class="text-surface-content text-right">
						{selectedRunTrack.quality_label
							? selectedRunTrack.quality_label
									.split(',')
									.map((flag) => flag.trim().replace('_', ' '))
									.filter((flag) => flag.length > 0)
									.join(', ')
							: '—'}
					</dd>
				</div>
				{#if selectedRunTrack.linked_track_ids && selectedRunTrack.linked_track_ids.length > 0}
					<div class="flex justify-between gap-2">
						<dt class="text-surface-content/60">Linked to</dt>
						<dd class="text-surface-content text-right font-mono">
							{selectedRunTrack.linked_track_ids.join(', ')}
						</dd>
					</div>
				{/if}
			</dl>
			<p class="text-surface-content/50">Labels are edited in the macOS visualiser.</p>
		</div>
	{:else}
		<div class="border-surface-content/10 border-t px-4 py-3">
			<p class="text-surface-content/50 text-xs">
				Select a track to see its labels. Labels are edited in the macOS visualiser.
			</p>
		</div>
	{/if}

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
