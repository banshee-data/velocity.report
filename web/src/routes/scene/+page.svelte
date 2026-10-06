<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { deleteScene, getScenes, type Scene } from '$lib/api';
	import { mdiDelete, mdiPencil, mdiPlus } from '@mdi/js';
	import { onMount } from 'svelte';
	import { Button, Dialog } from 'svelte-ux';

	let scenes: Scene[] = [];
	let loading = true;
	let error = '';

	let showDeleteDialog = false;
	let deletingScene: Scene | null = null;

	onMount(loadScenes);

	async function loadScenes() {
		loading = true;
		error = '';
		try {
			scenes = await getScenes();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load scenes.';
		} finally {
			loading = false;
		}
	}

	function handleCreate() {
		goto(resolve('/scene/new'));
	}

	function handleEdit(sceneId: string) {
		goto(resolve(`/scene/${sceneId}`));
	}

	function openDeleteDialog(scene: Scene) {
		deletingScene = scene;
		showDeleteDialog = true;
	}

	async function handleDelete() {
		if (!deletingScene) return;
		try {
			await deleteScene(deletingScene.scene_id);
			showDeleteDialog = false;
			deletingScene = null;
			await loadScenes();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not delete scene.';
		}
	}

	/** Capture date and time, or a clear statement that it is not recorded. */
	function formatCaptured(scene: Scene): string {
		if (!scene.captured_start) return 'Not recorded';
		const d = new Date(scene.captured_start);
		if (Number.isNaN(d.getTime())) return 'Not recorded';
		return d.toLocaleString(undefined, {
			year: 'numeric',
			month: 'short',
			day: 'numeric',
			hour: '2-digit',
			minute: '2-digit'
		});
	}

	function formatDuration(secs: number | null): string {
		if (secs == null || secs <= 0) return '—';
		const m = Math.floor(secs / 60);
		const s = Math.round(secs % 60);
		return m > 0 ? `${m}m ${s}s` : `${s}s`;
	}

	/** Position plus where it came from, so an inherited one is never mistaken
	 * for one set on the scene itself. */
	function formatPosition(scene: Scene): string {
		const lat = scene.effective_latitude;
		const lng = scene.effective_longitude;
		if (lat == null || lng == null) return 'No position';
		const where = scene.position_source === 'site' ? ' (from site)' : '';
		return `${lat.toFixed(5)}, ${lng.toFixed(5)}${where}`;
	}
</script>

<svelte:head>
	<title>Scenes</title>
</svelte:head>

<div class="p-4">
	<div class="mb-4 flex items-center justify-between">
		<div>
			<h1 class="text-2xl font-semibold">Scenes</h1>
			<p class="text-surface-content/70 text-sm">
				Published LiDAR recordings, with where and when each was captured.
			</p>
		</div>
		<Button variant="fill" color="primary" icon={mdiPlus} on:click={handleCreate}>New scene</Button>
	</div>

	{#if error}
		<div class="border-danger/40 bg-danger/10 mb-4 rounded border p-3 text-sm" role="alert">
			{error}
		</div>
	{/if}

	{#if loading}
		<p class="text-surface-content/70">Loading scenes…</p>
	{:else if scenes.length === 0}
		<div class="border-surface-content/20 rounded border p-8 text-center">
			<p class="mb-2 font-medium">No scenes yet</p>
			<p class="text-surface-content/70 mb-4 text-sm">
				A scene records where and when a capture was made, so a published recording can be found on
				a map and in a timeline.
			</p>
			<Button variant="fill" color="primary" icon={mdiPlus} on:click={handleCreate}>
				Add the first scene
			</Button>
		</div>
	{:else}
		<div class="overflow-x-auto">
			<table class="w-full text-sm">
				<thead>
					<tr class="border-surface-content/20 border-b text-left">
						<th class="py-2 pr-4 font-medium">Scene</th>
						<th class="py-2 pr-4 font-medium">Captured</th>
						<th class="py-2 pr-4 font-medium">Duration</th>
						<th class="py-2 pr-4 font-medium">Position</th>
						<th class="py-2 pr-4 font-medium">Published</th>
						<th class="py-2 font-medium">Actions</th>
					</tr>
				</thead>
				<tbody>
					{#each scenes as scene (scene.scene_id)}
						<tr class="border-surface-content/10 border-b">
							<td class="py-2 pr-4">
								<div class="font-medium">{scene.title}</div>
								<div class="text-surface-content/60 font-mono text-xs">{scene.scene_id}</div>
							</td>
							<td class="py-2 pr-4 tabular-nums">{formatCaptured(scene)}</td>
							<td class="py-2 pr-4 tabular-nums">{formatDuration(scene.duration_secs)}</td>
							<td class="py-2 pr-4 font-mono text-xs">{formatPosition(scene)}</td>
							<td class="py-2 pr-4">
								{#if scene.published}
									<span class="bg-success/15 text-success rounded px-2 py-0.5 text-xs"
										>Published</span
									>
								{:else}
									<span class="text-surface-content/60 text-xs">Draft</span>
								{/if}
							</td>
							<td class="py-2">
								<Button
									icon={mdiPencil}
									size="sm"
									on:click={() => handleEdit(scene.scene_id)}
									aria-label="Edit {scene.title}"
								/>
								<Button
									icon={mdiDelete}
									size="sm"
									color="danger"
									on:click={() => openDeleteDialog(scene)}
									aria-label="Delete {scene.title}"
								/>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

<Dialog bind:open={showDeleteDialog}>
	<div slot="title">Delete this scene?</div>
	<div class="px-6 py-3 text-sm">
		{#if deletingScene}
			<p>
				<strong>{deletingScene.title}</strong> will be removed from the index.
			</p>
			<p class="text-surface-content/70 mt-2">
				This removes the index entry only. Any published asset files stay where they are.
			</p>
		{/if}
	</div>
	<div slot="actions">
		<Button variant="fill" color="danger" on:click={handleDelete}>Delete</Button>
		<Button on:click={() => (showDeleteDialog = false)}>Cancel</Button>
	</div>
</Dialog>
