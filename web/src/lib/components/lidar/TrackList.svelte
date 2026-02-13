<script lang="ts">
	import type {
		Track,
		RunTrack,
		LabellingProgress,
		DetectionLabel,
		QualityLabel
	} from '$lib/types/lidar';
	import { TRACK_COLORS } from '$lib/types/lidar';
	import { updateTrackLabel, updateTrackFlags } from '$lib/api';
	import { Button } from 'svelte-ux';
	import { onMount, onDestroy } from 'svelte';
	import { SvelteSet } from 'svelte/reactivity';

	export let tracks: Track[] = [];
	export let selectedTrackId: string | null = null;
	export let onTrackSelect: (trackId: string) => void = () => {};
	// Callback to notify parent when paginated tracks change
	export let onPaginatedTracksChange: ((tracks: Track[]) => void) | null = null;

	// Phase 3: Labelling workflow props
	export let runId: string | null = null;
	export let runTracks: RunTrack[] = [];
	export let labellingProgress: LabellingProgress | null = null;

	// Phase 3.4: Bulk selection state
	let bulkSelectedTrackIds = new SvelteSet<string>();

	// Phase 3.5: Link mode state
	let linkMode = false;
	let linkSource: string | null = null;

	// Classification label options (single-select: what is the object?)
	const DETECTION_LABELS: { value: DetectionLabel; label: string; shortcut: string }[] = [
		{ value: 'car', label: 'Car', shortcut: '1' },
		{ value: 'ped', label: 'Pedestrian', shortcut: '2' },
		{ value: 'noise', label: 'Noise', shortcut: '3' }
	];

	// Quality flag options (multi-select: properties of the track)
	const QUALITY_LABELS: { value: QualityLabel; label: string; shortcut: string }[] = [
		{ value: 'good', label: 'Good', shortcut: '' },
		{ value: 'noisy', label: 'Noisy', shortcut: '' },
		{ value: 'jitter_velocity', label: 'Jitter Velocity', shortcut: '' },
		{ value: 'merge', label: 'Merge', shortcut: '' },
		{ value: 'split', label: 'Split', shortcut: '' },
		{ value: 'truncated', label: 'Truncated', shortcut: '' },
		{ value: 'disconnected', label: 'Disconnected', shortcut: '' }
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
				labeler_id: 'web-ui'
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

	// Phase 3: Toggle quality flag (multi-select)
	async function applyQualityLabel(trackId: string, label: QualityLabel) {
		if (!runId) return;

		isSavingLabel = true;
		labelError = null;

		try {
			// Parse current flags and toggle the selected one
			const runTrack = runTrackMap.get(trackId);
			const currentFlags = new Set(
				(runTrack?.quality_label ?? '')
					.split(',')
					.map((s) => s.trim())
					.filter((s) => s.length > 0)
			);

			if (currentFlags.has(label)) {
				currentFlags.delete(label);
			} else {
				currentFlags.add(label);
			}

			const newFlagsStr = [...currentFlags].sort().join(',');

			await updateTrackLabel(runId, trackId, {
				quality_label: newFlagsStr,
				labeler_id: 'web-ui'
			});

			// Update local state
			if (runTrack) {
				runTrack.quality_label = newFlagsStr;
				runTracks = [...runTracks]; // Trigger reactivity
			}

			console.log('[Label] Toggled quality flag', label, 'on track', trackId, '→', newFlagsStr);
		} catch (error) {
			console.error('[Label] Failed to apply quality label:', error);
			labelError = error instanceof Error ? error.message : 'Failed to apply label';
		} finally {
			isSavingLabel = false;
		}
	}

	// Phase 3.4: Apply bulk detection label
	async function applyBulkDetectionLabel(label: DetectionLabel) {
		if (!runId || bulkSelectedTrackIds.size === 0) return;

		isSavingLabel = true;
		labelError = null;

		try {
			// Apply label to all selected tracks
			await Promise.all(
				Array.from(bulkSelectedTrackIds).map((trackId) =>
					updateTrackLabel(runId, trackId, {
						user_label: label,
						labeler_id: 'web-ui'
					})
				)
			);

			// Update local state for all tracks
			bulkSelectedTrackIds.forEach((trackId) => {
				const runTrack = runTrackMap.get(trackId);
				if (runTrack) {
					runTrack.user_label = label;
				}
			});
			runTracks = [...runTracks]; // Trigger reactivity

			console.log(
				'[Label] Applied bulk detection label',
				label,
				'to',
				bulkSelectedTrackIds.size,
				'tracks'
			);
			bulkSelectedTrackIds.clear();
		} catch (error) {
			console.error('[Label] Failed to apply bulk detection label:', error);
			labelError = error instanceof Error ? error.message : 'Failed to apply bulk label';
		} finally {
			isSavingLabel = false;
		}
	}

	// Phase 3.4: Apply bulk quality label
	async function applyBulkQualityLabel(label: QualityLabel) {
		if (!runId || bulkSelectedTrackIds.size === 0) return;

		isSavingLabel = true;
		labelError = null;

		try {
			// Apply label to all selected tracks
			await Promise.all(
				Array.from(bulkSelectedTrackIds).map((trackId) =>
					updateTrackLabel(runId, trackId, {
						quality_label: label,
						labeler_id: 'web-ui'
					})
				)
			);

			// Update local state for all tracks
			bulkSelectedTrackIds.forEach((trackId) => {
				const runTrack = runTrackMap.get(trackId);
				if (runTrack) {
					runTrack.quality_label = label;
				}
			});
			runTracks = [...runTracks]; // Trigger reactivity

			console.log(
				'[Label] Applied bulk quality label',
				label,
				'to',
				bulkSelectedTrackIds.size,
				'tracks'
			);
			bulkSelectedTrackIds.clear();
		} catch (error) {
			console.error('[Label] Failed to apply bulk quality label:', error);
			labelError = error instanceof Error ? error.message : 'Failed to apply bulk label';
		} finally {
			isSavingLabel = false;
		}
	}

	// Phase 3.5: Link two tracks
	async function linkTracks(trackId1: string, trackId2: string) {
		if (!runId) return;

		isSavingLabel = true;
		labelError = null;

		try {
			// Link both tracks to each other
			await Promise.all([
				updateTrackFlags(runId, trackId1, {
					linked_track_ids: [trackId2],
					user_label: 'split'
				}),
				updateTrackFlags(runId, trackId2, {
					linked_track_ids: [trackId1],
					user_label: 'split'
				})
			]);

			// Update local state
			const runTrack1 = runTrackMap.get(trackId1);
			const runTrack2 = runTrackMap.get(trackId2);
			if (runTrack1) {
				runTrack1.linked_track_ids = [trackId2];
				runTrack1.user_label = 'split';
			}
			if (runTrack2) {
				runTrack2.linked_track_ids = [trackId1];
				runTrack2.user_label = 'split';
			}
			runTracks = [...runTracks]; // Trigger reactivity

			console.log('[Link] Linked tracks', trackId1, 'and', trackId2);
		} catch (error) {
			console.error('[Link] Failed to link tracks:', error);
			labelError = error instanceof Error ? error.message : 'Failed to link tracks';
		} finally {
			isSavingLabel = false;
		}
	}

	// Phase 3.5: Unlink a track
	async function unlinkTrack(trackId: string) {
		if (!runId) return;

		isSavingLabel = true;
		labelError = null;

		try {
			const runTrack = runTrackMap.get(trackId);
			if (!runTrack || !runTrack.linked_track_ids || runTrack.linked_track_ids.length === 0) {
				return;
			}

			// Unlink from all linked tracks
			const linkedIds = runTrack.linked_track_ids;
			await Promise.all([
				// Clear this track's links
				updateTrackFlags(runId, trackId, {
					linked_track_ids: []
				}),
				// Clear links from linked tracks
				...linkedIds.map((linkedId) =>
					updateTrackFlags(runId, linkedId, {
						linked_track_ids: []
					})
				)
			]);

			// Update local state
			runTrack.linked_track_ids = [];
			linkedIds.forEach((linkedId) => {
				const linkedTrack = runTrackMap.get(linkedId);
				if (linkedTrack) {
					linkedTrack.linked_track_ids = [];
				}
			});
			runTracks = [...runTracks]; // Trigger reactivity

			console.log('[Link] Unlinked track', trackId);
		} catch (error) {
			console.error('[Link] Failed to unlink track:', error);
			labelError = error instanceof Error ? error.message : 'Failed to unlink track';
		} finally {
			isSavingLabel = false;
		}
	}

	// Phase 3.4/3.5: Handle track click (with shift-click for multi-select and link mode)
	function handleTrackClick(trackId: string, event: MouseEvent) {
		if (!runId) {
			onTrackSelect(trackId);
			return;
		}

		// Phase 3.5: Link mode - clicking a track links it to the source
		if (linkMode) {
			if (!linkSource) {
				linkSource = trackId;
				console.log('[Link] Set link source:', trackId);
			} else {
				if (linkSource !== trackId) {
					linkTracks(linkSource, trackId);
				}
				linkMode = false;
				linkSource = null;
			}
			return;
		}

		// Phase 3.4: Shift-click for multi-select
		if (event.shiftKey) {
			event.preventDefault();
			if (bulkSelectedTrackIds.has(trackId)) {
				bulkSelectedTrackIds.delete(trackId);
			} else {
				bulkSelectedTrackIds.add(trackId);
			}
			return;
		}

		// Normal click: select single track
		onTrackSelect(trackId);
	}

	// Phase 3: Keyboard shortcuts for labelling
	function handleKeyPress(event: KeyboardEvent) {
		// Don't trigger if user is typing in an input field
		if (
			event.target instanceof HTMLInputElement ||
			event.target instanceof HTMLSelectElement ||
			event.target instanceof HTMLTextAreaElement ||
			(event.target instanceof HTMLElement && event.target.isContentEditable)
		) {
			return;
		}

		// Phase 3.4: Escape to clear multi-selection
		if (event.key === 'Escape') {
			if (bulkSelectedTrackIds.size > 0) {
				event.preventDefault();
				bulkSelectedTrackIds.clear();
				return;
			}
			if (linkMode) {
				event.preventDefault();
				linkMode = false;
				linkSource = null;
				return;
			}
		}

		if (!runId || !selectedTrackId) return;

		// Classification labels (1-3)
		if (!event.shiftKey && event.key >= '1' && event.key <= '3') {
			const index = parseInt(event.key) - 1;
			if (index < DETECTION_LABELS.length) {
				event.preventDefault();
				applyDetectionLabel(selectedTrackId, DETECTION_LABELS[index].value);
			}
		}

		// Quality flags (Shift+1 to Shift+7) — toggles
		if (event.shiftKey && event.key >= '1' && event.key <= '7') {
			const index = parseInt(event.key) - 1;
			if (index < QUALITY_LABELS.length) {
				event.preventDefault();
				applyQualityLabel(selectedTrackId, QUALITY_LABELS[index].value);
			}
		}
	}

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
		if (label === 'car') return 'bg-blue-100 text-blue-800';
		if (label === 'ped') return 'bg-green-100 text-green-800';
		if (label === 'noise') return 'bg-red-100 text-red-800';
		return 'bg-gray-100 text-gray-800';
	}

	// Phase 3: Get quality badge colour
	function getQualityColor(label: string): string {
		if (label === 'good') return 'bg-green-100 text-green-800';
		if (label === 'noisy') return 'bg-orange-100 text-orange-800';
		if (label === 'jitter_velocity') return 'bg-orange-100 text-orange-800';
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

		<!-- Phase 3.5: Link Mode Toggle -->
		{#if runId}
			<div class="mt-2 flex items-center gap-2">
				<Button
					size="sm"
					variant={linkMode ? 'fill' : 'outline'}
					color={linkMode ? 'primary' : 'neutral'}
					on:click={() => {
						linkMode = !linkMode;
						linkSource = null;
						bulkSelectedTrackIds.clear();
					}}
					class="text-xs"
					title="Enable to link two tracks (split/merge annotation)"
				>
					🔗 {linkMode ? 'Link Mode Active' : 'Link Tracks'}
				</Button>
				{#if linkMode && linkSource}
					<span class="text-surface-content/70 text-xs"
						>Source: {linkSource.substring(0, 8)}... (click another track to link)</span
					>
				{/if}
			</div>
		{/if}

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
					<option value="car">Car</option>
					<option value="ped">Pedestrian</option>
					<option value="noise">Noise</option>
					<option value="good">Good</option>
					<option value="noisy">Noisy</option>
					<option value="jitter_velocity">Jitter Velocity</option>
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
			{@const isMultiSelected = bulkSelectedTrackIds.has(track.track_id)}
			{@const isLinkSource = linkMode && linkSource === track.track_id}
			{@const color =
				track.object_class && track.object_class in TRACK_COLORS
					? TRACK_COLORS[track.object_class as keyof typeof TRACK_COLORS]
					: TRACK_COLORS.other}
			{@const runTrack = runId ? (runTrackMap.get(track.track_id) ?? null) : null}

			<button
				on:click={(e) => handleTrackClick(track.track_id, e)}
				class="border-surface-content/10 hover:bg-surface-200 w-full border-b px-4 py-3 text-left transition-colors {isSelected
					? 'border-l-primary bg-primary/10 border-l-4'
					: isMultiSelected || isLinkSource
						? 'border-l-accent bg-accent/10 border-l-4'
						: ''}"
			>
				<div class="flex items-start gap-3">
					<!-- Multi-select checkbox (Phase 3.4) -->
					{#if runId && !linkMode}
						<div class="flex-shrink-0">
							<input
								type="checkbox"
								checked={isMultiSelected}
								on:change={(e) => {
									e.stopPropagation();
									if (isMultiSelected) {
										bulkSelectedTrackIds.delete(track.track_id);
									} else {
										bulkSelectedTrackIds.add(track.track_id);
									}
								}}
								class="h-4 w-4"
							/>
						</div>
					{/if}

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
							<!-- Phase 3.5: Linked track indicator -->
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
									{#each runTrack.quality_label
										.split(',')
										.map((s) => s.trim())
										.filter((s) => s.length > 0) as flag}
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

	<!-- Phase 3: Labelling Controls (shown when track is selected in labelling mode) -->
	{#if !runId}
		<div class="border-surface-content/10 border-t px-4 py-3">
			<div class="text-surface-content/50 text-xs">
				<p class="font-medium">Labelling Mode</p>
				<p class="mt-1">
					Select a Scene and Run from the header dropdowns to enable track labelling.
				</p>
			</div>
		</div>
	{:else if bulkSelectedTrackIds.size > 0}
		<!-- Phase 3.4: Bulk Labelling Panel -->
		<div class="border-surface-content/10 space-y-3 border-t bg-blue-50 px-4 py-3 dark:bg-blue-950">
			<div class="flex items-center justify-between">
				<h4 class="text-surface-content text-sm font-semibold">
					Bulk Label ({bulkSelectedTrackIds.size} tracks)
				</h4>
				<Button
					size="sm"
					variant="outline"
					on:click={() => bulkSelectedTrackIds.clear()}
					class="text-xs"
				>
					Clear Selection
				</Button>
			</div>

			<!-- Detection Labels -->
			<div>
				<div class="text-surface-content/70 mb-1 block text-xs font-medium">Detection</div>
				<div class="grid grid-cols-2 gap-1">
					{#each DETECTION_LABELS as { value, label } (value)}
						<Button
							size="sm"
							variant="outline"
							color="neutral"
							on:click={() => applyBulkDetectionLabel(value)}
							disabled={isSavingLabel}
							class="text-xs"
						>
							{label}
						</Button>
					{/each}
				</div>
			</div>

			<!-- Quality Flags -->
			<div>
				<div class="text-surface-content/70 mb-1 block text-xs font-medium">Flags</div>
				<div class="grid grid-cols-2 gap-1">
					{#each QUALITY_LABELS as { value, label } (value)}
						<Button
							size="sm"
							variant="outline"
							color="neutral"
							on:click={() => applyBulkQualityLabel(value)}
							disabled={isSavingLabel}
							class="text-xs"
						>
							{label}
						</Button>
					{/each}
				</div>
			</div>
		</div>
	{:else if runId && !selectedTrackId}
		<div class="border-surface-content/10 border-t px-4 py-3">
			<div class="text-surface-content/50 text-xs">
				<p class="font-medium">Select a Track</p>
				<p class="mt-1">Click a track from the list above or on the map to start labelling.</p>
				<p class="mt-1">Keys 1-3: classification labels.</p>
				<p class="mt-1">Shift+click: multi-select for bulk labelling.</p>
			</div>
		</div>
	{:else if runId && selectedTrackId && selectedRunTrack}
		<div class="border-surface-content/10 space-y-3 border-t px-4 py-3">
			<div class="flex items-center justify-between">
				<h4 class="text-surface-content text-sm font-semibold">Label Track</h4>
				<!-- Phase 3.5: Unlink button -->
				{#if selectedRunTrack.linked_track_ids && selectedRunTrack.linked_track_ids.length > 0}
					<Button
						size="sm"
						variant="outline"
						color="danger"
						on:click={() => unlinkTrack(selectedTrackId)}
						disabled={isSavingLabel}
						class="text-xs"
						title="Remove link to other track(s)"
					>
						Unlink
					</Button>
				{/if}
			</div>

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

			<!-- Quality Flags (multi-select) -->
			<div>
				<div class="text-surface-content/70 mb-1 block text-xs font-medium">Flags</div>
				<div class="grid grid-cols-2 gap-1">
					{#each QUALITY_LABELS as { value, label, shortcut } (value)}
						{@const activeFlags = (selectedRunTrack.quality_label ?? '')
							.split(',')
							.map((s) => s.trim())}
						<Button
							size="sm"
							variant={activeFlags.includes(value) ? 'fill' : 'outline'}
							color={activeFlags.includes(value) ? 'primary' : 'neutral'}
							on:click={() => applyQualityLabel(selectedTrackId, value)}
							disabled={isSavingLabel}
							class="text-xs"
						>
							{label}
						</Button>
					{/each}
				</div>
			</div>

			<div class="text-surface-content/50 text-xs">
				<p>Use keyboard shortcuts for faster labelling:</p>
				<p>1-3: Classification labels</p>
				<p>Shift+click: Multi-select</p>
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
