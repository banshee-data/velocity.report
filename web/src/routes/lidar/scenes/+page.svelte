<script lang="ts">
	/**
	 * LiDAR Scene Management Page
	 *
	 * CRUD interface for managing LiDAR scenes — associating PCAP files,
	 * region maps, and background grids with a scene for ground truth labelling.
	 */
	import {
		createLidarScene,
		deleteLidarScene,
		getLidarRuns,
		getLidarScenes,
		updateLidarScene
	} from '$lib/api';
	import type { AnalysisRun, LidarScene } from '$lib/types/lidar';
	import { onMount } from 'svelte';
	import { Button, SelectField } from 'svelte-ux';

	// Scene list
	let scenes: LidarScene[] = [];
	let loading = true;
	let error: string | null = null;

	// Runs (for reference_run_id dropdown)
	let runs: AnalysisRun[] = [];

	// Selected scene for editing
	let selectedScene: LidarScene | null = null;

	// Create form state
	let showCreateForm = false;
	let newSensorId = 'hesai-pandar40p';
	let newPcapFile = '';
	let newDescription = '';
	let newPcapStartSecs = '';
	let newPcapDurationSecs = '';
	let creating = false;
	let createError: string | null = null;

	// Edit form state
	let editDescription = '';
	let editReferenceRunId: string | null = null;
	let editOptimalParams = '';
	let saving = false;
	let saveError: string | null = null;

	async function loadScenes() {
		loading = true;
		error = null;
		try {
			scenes = await getLidarScenes();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load scenes';
		} finally {
			loading = false;
		}
	}

	async function loadRuns() {
		try {
			runs = await getLidarRuns();
		} catch {
			runs = [];
		}
	}

	function selectScene(scene: LidarScene) {
		selectedScene = scene;
		editDescription = scene.description ?? '';
		editReferenceRunId = scene.reference_run_id ?? null;
		editOptimalParams = scene.optimal_params_json ?? '';
	}

	function deselectScene() {
		selectedScene = null;
		editDescription = '';
		editReferenceRunId = null;
		editOptimalParams = '';
		saveError = null;
	}

	async function handleCreate() {
		creating = true;
		createError = null;
		try {
			const scene = await createLidarScene({
				sensor_id: newSensorId,
				pcap_file: newPcapFile,
				description: newDescription || undefined,
				pcap_start_secs: newPcapStartSecs ? parseFloat(newPcapStartSecs) : undefined,
				pcap_duration_secs: newPcapDurationSecs ? parseFloat(newPcapDurationSecs) : undefined
			});
			scenes = [...scenes, scene];
			// Reset form
			newPcapFile = '';
			newDescription = '';
			newPcapStartSecs = '';
			newPcapDurationSecs = '';
			showCreateForm = false;
		} catch (e) {
			createError = e instanceof Error ? e.message : 'Failed to create scene';
		} finally {
			creating = false;
		}
	}

	async function handleUpdate() {
		if (!selectedScene) return;
		saving = true;
		saveError = null;
		try {
			const updated = await updateLidarScene(selectedScene.scene_id, {
				description: editDescription || undefined,
				reference_run_id: editReferenceRunId || undefined,
				optimal_params_json: editOptimalParams || undefined
			});
			// Update in list
			scenes = scenes.map((s) => (s.scene_id === updated.scene_id ? updated : s));
			selectedScene = updated;
		} catch (e) {
			saveError = e instanceof Error ? e.message : 'Failed to update scene';
		} finally {
			saving = false;
		}
	}

	async function handleDelete(sceneId: string) {
		if (!confirm('Delete this scene? This cannot be undone.')) return;
		try {
			await deleteLidarScene(sceneId);
			scenes = scenes.filter((s) => s.scene_id !== sceneId);
			if (selectedScene?.scene_id === sceneId) {
				deselectScene();
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to delete scene';
		}
	}

	function formatDate(ns: number): string {
		if (!ns) return '-';
		return new Date(ns / 1e6).toLocaleDateString();
	}

	onMount(() => {
		loadScenes();
		loadRuns();
	});
</script>

<main id="main-content" class="bg-surface-200 flex h-[calc(100vh-64px)] flex-col overflow-hidden">
	<!-- Header -->
	<div class="border-surface-content/10 bg-surface-100 flex-none border-b px-6 py-4">
		<div class="flex items-center justify-between">
			<div>
				<h1 class="text-surface-content text-2xl font-semibold">LiDAR Scenes</h1>
				<p class="text-surface-content/60 mt-1 text-sm">
					Manage scenes for ground truth labelling and parameter tuning
				</p>
			</div>
			<Button variant="fill" color="primary" on:click={() => (showCreateForm = !showCreateForm)}>
				{showCreateForm ? 'Cancel' : 'New Scene'}
			</Button>
		</div>
	</div>

	<div class="flex flex-1 overflow-hidden">
		<!-- Left: Scene List -->
		<div class="flex-1 overflow-y-auto p-6">
			{#if error}
				<div class="mb-4 rounded bg-red-50 px-4 py-3 text-sm text-red-600">
					{error}
					<button class="ml-2 underline" on:click={loadScenes}>Retry</button>
				</div>
			{/if}

			<!-- Create Form -->
			{#if showCreateForm}
				<div class="bg-surface-100 border-surface-content/10 mb-6 rounded-lg border p-6">
					<h2 class="text-surface-content mb-4 text-lg font-semibold">Create New Scene</h2>

					{#if createError}
						<div class="mb-4 rounded bg-red-50 px-3 py-2 text-sm text-red-600">
							{createError}
						</div>
					{/if}

					<div class="grid grid-cols-2 gap-4">
						<div>
							<label for="new-sensor" class="text-surface-content/70 mb-1 block text-sm font-medium"
								>Sensor</label
							>
							<SelectField
								label=""
								bind:value={newSensorId}
								options={[{ label: 'Hesai Pandar40P', value: 'hesai-pandar40p' }]}
								size="sm"
							/>
						</div>
						<div>
							<label for="new-pcap" class="text-surface-content/70 mb-1 block text-sm font-medium"
								>PCAP File</label
							>
							<input
								id="new-pcap"
								type="text"
								bind:value={newPcapFile}
								placeholder="path/to/capture.pcap"
								class="border-surface-content/20 bg-surface-50 w-full rounded border px-3 py-2 text-sm"
							/>
						</div>
						<div class="col-span-2">
							<label for="new-desc" class="text-surface-content/70 mb-1 block text-sm font-medium"
								>Description</label
							>
							<textarea
								id="new-desc"
								bind:value={newDescription}
								placeholder="Describe the scene environment..."
								rows="2"
								class="border-surface-content/20 bg-surface-50 w-full rounded border px-3 py-2 text-sm"
							></textarea>
						</div>
						<div>
							<label for="new-start" class="text-surface-content/70 mb-1 block text-sm font-medium"
								>Start Offset (seconds)</label
							>
							<input
								id="new-start"
								type="text"
								bind:value={newPcapStartSecs}
								placeholder="0"
								class="border-surface-content/20 bg-surface-50 w-full rounded border px-3 py-2 text-sm"
							/>
						</div>
						<div>
							<label
								for="new-duration"
								class="text-surface-content/70 mb-1 block text-sm font-medium"
								>Duration (seconds)</label
							>
							<input
								id="new-duration"
								type="text"
								bind:value={newPcapDurationSecs}
								placeholder="Full PCAP"
								class="border-surface-content/20 bg-surface-50 w-full rounded border px-3 py-2 text-sm"
							/>
						</div>
					</div>

					<div class="mt-4 flex justify-end gap-2">
						<Button variant="outline" on:click={() => (showCreateForm = false)} disabled={creating}>
							Cancel
						</Button>
						<Button
							variant="fill"
							color="primary"
							on:click={handleCreate}
							disabled={creating || !newPcapFile}
						>
							{creating ? 'Creating...' : 'Create Scene'}
						</Button>
					</div>
				</div>
			{/if}

			<!-- Scene Table -->
			{#if loading}
				<div class="text-surface-content/50 py-12 text-center">Loading scenes...</div>
			{:else if scenes.length === 0}
				<div class="text-surface-content/50 py-12 text-center">
					<p>No scenes yet.</p>
					<p class="mt-1 text-sm">Create a scene to start labelling tracks.</p>
				</div>
			{:else}
				<div class="bg-surface-100 border-surface-content/10 overflow-hidden rounded-lg border">
					<table class="w-full">
						<thead>
							<tr class="border-surface-content/10 border-b">
								<th class="text-surface-content/70 px-4 py-3 text-left text-sm font-medium"
									>Description</th
								>
								<th class="text-surface-content/70 px-4 py-3 text-left text-sm font-medium"
									>Sensor</th
								>
								<th class="text-surface-content/70 px-4 py-3 text-left text-sm font-medium"
									>PCAP File</th
								>
								<th class="text-surface-content/70 px-4 py-3 text-left text-sm font-medium"
									>Ref. Run</th
								>
								<th class="text-surface-content/70 px-4 py-3 text-left text-sm font-medium"
									>Created</th
								>
								<th class="text-surface-content/70 px-4 py-3 text-right text-sm font-medium"
									>Actions</th
								>
							</tr>
						</thead>
						<tbody>
							{#each scenes as scene (scene.scene_id)}
								{@const isSelected = selectedScene?.scene_id === scene.scene_id}
								<tr
									class="border-surface-content/10 hover:bg-surface-200/50 cursor-pointer border-b transition-colors last:border-b-0 {isSelected
										? 'bg-primary/5'
										: ''}"
									on:click={() => selectScene(scene)}
								>
									<td class="text-surface-content px-4 py-3 text-sm">
										{scene.description || scene.scene_id.substring(0, 8)}
									</td>
									<td class="text-surface-content/70 px-4 py-3 font-mono text-sm">
										{scene.sensor_id}
									</td>
									<td class="text-surface-content/70 max-w-[200px] truncate px-4 py-3 text-sm">
										{scene.pcap_file}
									</td>
									<td class="text-surface-content/70 px-4 py-3 font-mono text-sm">
										{scene.reference_run_id ? scene.reference_run_id.substring(0, 8) : '-'}
									</td>
									<td class="text-surface-content/70 px-4 py-3 text-sm">
										{formatDate(scene.created_at_ns)}
									</td>
									<td class="px-4 py-3 text-right">
										<button
											class="text-sm text-red-500 hover:text-red-700"
											on:click|stopPropagation={() => handleDelete(scene.scene_id)}
										>
											Delete
										</button>
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}
		</div>

		<!-- Right: Edit Panel -->
		{#if selectedScene}
			<div
				class="border-surface-content/10 bg-surface-100 w-[400px] flex-none overflow-y-auto border-l p-6"
			>
				<div class="mb-4 flex items-center justify-between">
					<h2 class="text-surface-content text-lg font-semibold">Edit Scene</h2>
					<button
						class="text-surface-content/50 hover:text-surface-content text-sm"
						on:click={deselectScene}
					>
						Close
					</button>
				</div>

				<div class="text-surface-content/50 mb-4 font-mono text-xs">
					{selectedScene.scene_id}
				</div>

				{#if saveError}
					<div class="mb-4 rounded bg-red-50 px-3 py-2 text-sm text-red-600">{saveError}</div>
				{/if}

				<div class="space-y-4">
					<div>
						<label for="edit-desc" class="text-surface-content/70 mb-1 block text-sm font-medium"
							>Description</label
						>
						<textarea
							id="edit-desc"
							bind:value={editDescription}
							rows="3"
							class="border-surface-content/20 bg-surface-50 w-full rounded border px-3 py-2 text-sm"
						></textarea>
					</div>

					<div>
						<label for="edit-ref-run" class="text-surface-content/70 mb-1 block text-sm font-medium"
							>Reference Run</label
						>
						<SelectField
							label=""
							bind:value={editReferenceRunId}
							options={[
								{ label: 'None', value: null },
								...runs.map((r) => ({
									label: `${r.run_id.substring(0, 8)} (${r.total_tracks} tracks)`,
									value: r.run_id
								}))
							]}
							size="sm"
						/>
						<p class="text-surface-content/40 mt-1 text-xs">
							The reference run contains ground truth labels for evaluation.
						</p>
					</div>

					<div>
						<label for="edit-pcap" class="text-surface-content/70 mb-1 block text-sm font-medium"
							>PCAP File</label
						>
						<div class="text-surface-content/60 bg-surface-200 rounded px-3 py-2 font-mono text-sm">
							{selectedScene.pcap_file}
						</div>
					</div>

					<div>
						<label for="edit-sensor" class="text-surface-content/70 mb-1 block text-sm font-medium"
							>Sensor</label
						>
						<div class="text-surface-content/60 bg-surface-200 rounded px-3 py-2 font-mono text-sm">
							{selectedScene.sensor_id}
						</div>
					</div>

					{#if selectedScene.pcap_start_secs !== undefined}
						<div class="grid grid-cols-2 gap-4">
							<div>
								<span class="text-surface-content/70 text-sm font-medium">Start</span>
								<div class="text-surface-content/60 text-sm">
									{selectedScene.pcap_start_secs}s
								</div>
							</div>
							<div>
								<span class="text-surface-content/70 text-sm font-medium">Duration</span>
								<div class="text-surface-content/60 text-sm">
									{selectedScene.pcap_duration_secs ?? 'Full'}s
								</div>
							</div>
						</div>
					{/if}

					<div>
						<label for="edit-params" class="text-surface-content/70 mb-1 block text-sm font-medium"
							>Optimal Parameters (JSON)</label
						>
						<textarea
							id="edit-params"
							bind:value={editOptimalParams}
							rows="6"
							placeholder={'{"background_threshold": 0.5, ...}'}
							class="border-surface-content/20 bg-surface-50 w-full rounded border px-3 py-2 font-mono text-xs"
						></textarea>
						<p class="text-surface-content/40 mt-1 text-xs">
							Best-known parameters for this scene, saved by auto-tuning.
						</p>
					</div>

					<div class="flex justify-end pt-2">
						<Button variant="fill" color="primary" on:click={handleUpdate} disabled={saving}>
							{saving ? 'Saving...' : 'Save Changes'}
						</Button>
					</div>
				</div>
			</div>
		{/if}
	</div>
</main>
