<script lang="ts">
	import type {
		Track,
		RunTrack,
		LabellingProgress,
		DetectionLabel,
		QualityLabel
	} from '$lib/types/lidar';
	import { TRACK_COLORS } from '$lib/types/lidar';
	import { updateTrackLabel } from '$lib/api';
	import { Button } from 'svelte-ux';

	export let tracks: Track[] = [];
	export let selectedTrackId: string | null = null;
	export let onTrackSelect: (trackId: string) => void = () => {};
	// Callback to notify parent when paginated tracks change
	export let onPaginatedTracksChange: ((tracks: Track[]) => void) | null = null;

	// Phase 3: Labelling workflow props
	export let runId: string | null = null;
	export let runTracks: RunTrack[] = [];
	export let labellingProgress: LabellingProgress | null = null;

	// Detection label options
	const DETECTION_LABELS: { value: DetectionLabel; label: string; shortcut: string }[] = [
		{ value: 'good_vehicle', label: 'Vehicle', shortcut: '1' },
		{ value: 'good_pedestrian', label: 'Pedestrian', shortcut: '2' },
		{ value: 'good_other', label: 'Other', shortcut: '3' },
		{ value: 'noise', label: 'Noise', shortcut: '4' },
		{ value: 'noise_flora', label: 'Flora', shortcut: '5' },
		{ value: 'split', label: 'Split', shortcut: '6' },
		{ value: 'merge', label: 'Merge', shortcut: '7' },
		{ value: 'missed', label: 'Missed', shortcut: '8' }
	];

	// Quality label options
	const QUALITY_LABELS: { value: QualityLabel; label: string; shortcut: string }[] = [
		{ value: 'perfect', label: 'Perfect', shortcut: 'Shift+1' },
		{ value: 'good', label: 'Good', shortcut: 'Shift+2' },
		{ value: 'truncated', label: 'Truncated', shortcut: 'Shift+3' },
		{ value: 'noisy_velocity', label: 'Noisy Vel', shortcut: 'Shift+4' },
		{ value: 'stopped_recovered', label: 'Stopped OK', shortcut: 'Shift+5' }
	];

	// Filter and sort options
	let classFilter: string = 'all';
	let stateFilter: string = 'all';
	let labelFilter: string = 'all'; // Phase 3: filter by label
	let sortBy: 'time' | 'speed' | 'duration' = 'time';
	let minObservations: number = 5; // Filter out tracks with fewer observations

	// Pagination
	const PAGE_SIZE = 50;
	let currentPage = 0;

	// Get run track for the selected track (if in labelling mode)
	$: selectedRunTrack =
		runId && selectedTrackId ? (runTrackMap.get(selectedTrackId) ?? null) : null;

	// Labelling state
	let isSavingLabel = false;
	let labelError: string | null = null;

	// Build a lookup map for run tracks (O(1) lookups instead of O(n²) find)
	$: runTrackMap = new Map(runTracks.map((rt) => [rt.track_id, rt]));

	// Filtered and sorted tracks
	$: filteredTracks = tracks
		.filter((track) => {
			// Filter by minimum observations (reduces noise from single-point tracks)
			if (track.observation_count < minObservations) return false;
			if (classFilter !== 'all' && track.object_class !== classFilter) return false;
			if (stateFilter !== 'all' && track.state !== stateFilter) return false;

			// Phase 3: Filter by label status
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

	// Phase 3: Apply detection label
	async function applyDetectionLabel(trackId: string, label: DetectionLabel) {
		if (!runId) return;

		isSavingLabel = true;
		labelError = null;

		try {
			await updateTrackLabel(runId, trackId, {
				user_label: label,
				labeler_id: 'web-ui' // TODO: Get from auth context
			});

			// Update local state
			const runTrack = runTrackMap.get(trackId);
			if (runTrack) {
				runTrack.user_label = label;
				runTracks = [...runTracks]; // Trigger reactivity
			}

			console.log('[Label] Applied detection label', label, 'to track', trackId);
		} catch (error) {
			console.error('[Label] Failed to apply detection label:', error);
			labelError = error instanceof Error ? error.message : 'Failed to apply label';
		} finally {
			isSavingLabel = false;
		}
	}

	// Phase 3: Apply quality label
	async function applyQualityLabel(trackId: string, label: QualityLabel) {
		if (!runId) return;

		isSavingLabel = true;
		labelError = null;

		try {
			await updateTrackLabel(runId, trackId, {
				quality_label: label,
				labeler_id: 'web-ui' // TODO: Get from auth context
			});

			// Update local state
			const runTrack = runTrackMap.get(trackId);
			if (runTrack) {
				runTrack.quality_label = label;
				runTracks = [...runTracks]; // Trigger reactivity
			}

			console.log('[Label] Applied quality label', label, 'to track', trackId);
		} catch (error) {
			console.error('[Label] Failed to apply quality label:', error);
			labelError = error instanceof Error ? error.message : 'Failed to apply label';
		} finally {
			isSavingLabel = false;
		}
	}

	// Phase 3: Keyboard shortcuts for labelling
	function handleKeyPress(event: KeyboardEvent) {
		if (!runId || !selectedTrackId) return;

		// Don't trigger if user is typing in an input field
		if (
			event.target instanceof HTMLInputElement ||
			event.target instanceof HTMLSelectElement ||
			event.target instanceof HTMLTextAreaElement ||
			(event.target instanceof HTMLElement && event.target.isContentEditable)
		) {
			return;
		}

		// Detection labels (1-8)
		if (!event.shiftKey && event.key >= '1' && event.key <= '8') {
			const index = parseInt(event.key) - 1;
			if (index < DETECTION_LABELS.length) {
				event.preventDefault();
				applyDetectionLabel(selectedTrackId, DETECTION_LABELS[index].value);
			}
		}

		// Quality labels (Shift+1 to Shift+5)
		if (event.shiftKey && event.key >= '1' && event.key <= '5') {
			const index = parseInt(event.key) - 1;
			if (index < QUALITY_LABELS.length) {
				event.preventDefault();
				applyQualityLabel(selectedTrackId, QUALITY_LABELS[index].value);
			}
		}
	}

	// Attach keyboard listener
	import { onMount, onDestroy } from 'svelte';

	onMount(() => {
		window.addEventListener('keydown', handleKeyPress);
	});

	onDestroy(() => {
		window.removeEventListener('keydown', handleKeyPress);
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

	// Phase 3: Get label badge colour
	function getLabelColor(label: string): string {
		if (label.startsWith('good_')) return 'bg-green-100 text-green-800';
		if (label === 'noise' || label === 'noise_flora') return 'bg-red-100 text-red-800';
		if (label === 'split' || label === 'merge') return 'bg-yellow-100 text-yellow-800';
		if (label === 'missed') return 'bg-purple-100 text-purple-800';
		return 'bg-gray-100 text-gray-800';
	}

	// Phase 3: Get quality badge colour
	function getQualityColor(label: string): string {
		if (label === 'perfect') return 'bg-green-100 text-green-800';
		if (label === 'good') return 'bg-blue-100 text-blue-800';
		if (label === 'truncated') return 'bg-yellow-100 text-yellow-800';
		if (label === 'noisy_velocity') return 'bg-orange-100 text-orange-800';
		if (label === 'stopped_recovered') return 'bg-purple-100 text-purple-800';
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

		<!-- Phase 3: Labelling Progress Bar -->
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

		<!-- Phase 3: Label error display -->
		{#if labelError}
			<div class="mt-2 rounded bg-red-50 px-2 py-1 text-xs text-red-600">
				{labelError}
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

		<!-- Phase 3: Label Filter (only show when in labelling mode) -->
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
					<option value="good_vehicle">Vehicle</option>
					<option value="good_pedestrian">Pedestrian</option>
					<option value="good_other">Other</option>
					<option value="noise">Noise</option>
					<option value="noise_flora">Flora</option>
					<option value="split">Split</option>
					<option value="merge">Merge</option>
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
					: TRACK_COLORS.other}
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

						<!-- Phase 3: Label Badges -->
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
									<span
										class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium {getQualityColor(
											runTrack.quality_label
										)}"
									>
										{runTrack.quality_label.replace('_', ' ')}
									</span>
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

	<!-- Phase 3: Labelling Controls (shown when track is selected in labelling mode) -->
	{#if runId && selectedTrackId && selectedRunTrack}
		<div class="border-surface-content/10 space-y-3 border-t px-4 py-3">
			<h4 class="text-surface-content text-sm font-semibold">Label Track</h4>

			<!-- Detection Labels -->
			<div>
				<div class="text-surface-content/70 mb-1 block text-xs font-medium">Detection</div>
				<div class="grid grid-cols-2 gap-1">
					{#each DETECTION_LABELS as { value, label, shortcut } (value)}
						<Button
							size="sm"
							variant={selectedRunTrack.user_label === value ? 'fill' : 'outline'}
							color={selectedRunTrack.user_label === value ? 'primary' : 'neutral'}
							on:click={() => applyDetectionLabel(selectedTrackId, value)}
							disabled={isSavingLabel}
							class="text-xs"
							title="Keyboard: {shortcut}"
						>
							{label}
						</Button>
					{/each}
				</div>
			</div>

			<!-- Quality Labels -->
			<div>
				<div class="text-surface-content/70 mb-1 block text-xs font-medium">Quality</div>
				<div class="grid grid-cols-2 gap-1">
					{#each QUALITY_LABELS as { value, label, shortcut } (value)}
						<Button
							size="sm"
							variant={selectedRunTrack.quality_label === value ? 'fill' : 'outline'}
							color={selectedRunTrack.quality_label === value ? 'primary' : 'neutral'}
							on:click={() => applyQualityLabel(selectedTrackId, value)}
							disabled={isSavingLabel}
							class="text-xs"
							title="Keyboard: {shortcut}"
						>
							{label}
						</Button>
					{/each}
				</div>
			</div>

			<div class="text-surface-content/50 text-xs">
				<p>Use keyboard shortcuts for faster labelling:</p>
				<p>1-8: Detection labels</p>
				<p>Shift+1-5: Quality labels</p>
			</div>
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
