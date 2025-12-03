<script lang="ts">
	import { browser } from '$app/environment';
	import { onMount } from 'svelte';
	import { Button, Card, Header, TextField } from 'svelte-ux';
	import {
		createAnglePreset,
		deleteAnglePreset,
		getAnglePresets,
		updateAnglePreset,
		type AnglePreset
	} from '../../../lib/api';

	let presets: AnglePreset[] = [];
	let loading = true;
	let error = '';

	// Form state
	let newAngle = '';
	let newColor = '#3B82F6';
	let editingId: number | null = null;
	let editAngle = '';
	let editColor = '';

	// Constrained color palette (accessible Tailwind colors)
	const colorPalette = [
		{ hex: '#3B82F6', name: 'Blue' },
		{ hex: '#10B981', name: 'Green' },
		{ hex: '#F59E0B', name: 'Amber' },
		{ hex: '#EF4444', name: 'Red' },
		{ hex: '#8B5CF6', name: 'Purple' },
		{ hex: '#EC4899', name: 'Pink' },
		{ hex: '#14B8A6', name: 'Teal' },
		{ hex: '#F97316', name: 'Orange' },
		{ hex: '#6366F1', name: 'Indigo' },
		{ hex: '#84CC16', name: 'Lime' },
		{ hex: '#06B6D4', name: 'Cyan' },
		{ hex: '#A855F7', name: 'Violet' }
	];

	async function loadPresets() {
		loading = true;
		error = '';
		try {
			presets = await getAnglePresets();
		} catch (e) {
			error = `Failed to load angle presets: ${e}`;
			console.error(error);
		} finally {
			loading = false;
		}
	}

	async function handleCreate() {
		const angle = parseFloat(newAngle);
		if (isNaN(angle) || angle < 0 || angle > 90) {
			error = 'Angle must be a number between 0 and 90';
			return;
		}

		try {
			await createAnglePreset({ angle, color_hex: newColor });
			newAngle = '';
			newColor = '#3B82F6';
			error = '';
			await loadPresets();
		} catch (e) {
			error = `Failed to create preset: ${e}`;
			console.error(error);
		}
	}

	function startEdit(preset: AnglePreset) {
		editingId = preset.id;
		editAngle = preset.angle.toString();
		editColor = preset.color_hex;
	}

	function cancelEdit() {
		editingId = null;
		editAngle = '';
		editColor = '';
	}

	async function handleUpdate() {
		if (editingId === null) return;

		const angle = parseFloat(editAngle);
		if (isNaN(angle) || angle < 0 || angle > 90) {
			error = 'Angle must be a number between 0 and 90';
			return;
		}

		try {
			await updateAnglePreset(editingId, { angle, color_hex: editColor });
			editingId = null;
			editAngle = '';
			editColor = '';
			error = '';
			await loadPresets();
		} catch (e) {
			error = `Failed to update preset: ${e}`;
			console.error(error);
		}
	}

	async function handleDelete(id: number) {
		if (!confirm('Are you sure you want to delete this angle preset?')) {
			return;
		}

		try {
			await deleteAnglePreset(id);
			error = '';
			await loadPresets();
		} catch (e) {
			error = `Failed to delete preset: ${e}`;
			console.error(error);
		}
	}

	onMount(loadPresets);
</script>

<div class="p-6 max-w-5xl mx-auto">
	<Header title="Angle Presets" class="mb-6">
		<div slot="subheading" class="text-sm text-gray-600 mt-1">
			Manage cosine error angle presets with color coding for visual distinction
		</div>
	</Header>

	{#if error}
		<div class="bg-red-50 border-red-200 text-red-800 px-4 py-3 rounded mb-4 border" role="alert">
			{error}
		</div>
	{/if}

	{#if loading}
		<p class="text-gray-500 py-8 text-center">Loading angle presets...</p>
	{:else}
		<!-- Existing Presets Table -->
		<Card title="Current Presets" class="mb-6">
			<div class="overflow-x-auto">
				<table class="text-sm w-full text-left">
					<thead class="bg-gray-50 border-b">
						<tr>
							<th class="px-4 py-3 font-semibold text-gray-700">Angle (°)</th>
							<th class="px-4 py-3 font-semibold text-gray-700">Color</th>
							<th class="px-4 py-3 font-semibold text-gray-700">Type</th>
							<th class="px-4 py-3 font-semibold text-gray-700">Actions</th>
						</tr>
					</thead>
					<tbody class="divide-y">
						{#each presets as preset}
							<tr class="hover:bg-gray-50">
								<td class="px-4 py-3">
									{#if editingId === preset.id}
										<TextField
											bind:value={editAngle}
											type="number"
											min="0"
											max="90"
											step="0.1"
											classes={{ root: 'w-24' }}
										/>
									{:else}
										<span class="font-mono">{preset.angle.toFixed(1)}</span>
									{/if}
								</td>
								<td class="px-4 py-3">
									{#if editingId === preset.id}
										<div class="gap-2 flex items-center">
											<div
												class="w-8 h-8 rounded border-gray-300 border"
												style="background-color: {editColor}"
											></div>
											<select bind:value={editColor} class="rounded px-2 py-1 text-sm border">
												{#each colorPalette as color}
													<option value={color.hex}>{color.name}</option>
												{/each}
											</select>
										</div>
									{:else}
										<div class="gap-2 flex items-center">
											<div
												class="w-8 h-8 rounded border-gray-300 border"
												style="background-color: {preset.color_hex}"
											></div>
											<span class="font-mono text-xs text-gray-600">{preset.color_hex}</span>
										</div>
									{/if}
								</td>
								<td class="px-4 py-3">
									{#if preset.is_system}
										<span
											class="px-2 py-1 bg-blue-100 text-blue-800 text-xs rounded font-semibold inline-block"
										>
											System
										</span>
									{:else}
										<span class="px-2 py-1 bg-gray-100 text-gray-700 text-xs rounded inline-block">
											Custom
										</span>
									{/if}
								</td>
								<td class="px-4 py-3">
									{#if preset.is_system}
										<span class="text-xs text-gray-400">Protected</span>
									{:else if editingId === preset.id}
										<div class="gap-2 flex">
											<Button variant="fill" color="primary" on:click={handleUpdate} size="sm">
												Save
											</Button>
											<Button variant="outline" on:click={cancelEdit} size="sm">Cancel</Button>
										</div>
									{:else}
										<div class="gap-2 flex">
											<Button
												variant="outline"
												color="primary"
												on:click={() => startEdit(preset)}
												size="sm"
											>
												Edit
											</Button>
											<Button
												variant="outline"
												color="danger"
												on:click={() => handleDelete(preset.id)}
												size="sm"
											>
												Delete
											</Button>
										</div>
									{/if}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</Card>

		<!-- Add New Preset Form -->
		<Card title="Add New Preset">
			<div class="p-4">
				<div class="md:grid-cols-3 gap-4 grid grid-cols-1 items-end">
					<div>
						<label for="new-angle" class="text-sm font-medium text-gray-700 mb-1 block">
							Angle (°)
						</label>
						<TextField
							id="new-angle"
							bind:value={newAngle}
							type="number"
							min="0"
							max="90"
							step="0.1"
							placeholder="e.g., 25.5"
						/>
					</div>

					<div>
						<label for="new-color" class="text-sm font-medium text-gray-700 mb-1 block">
							Color
						</label>
						<div class="gap-2 flex items-center">
							<div
								class="w-10 h-10 rounded border-gray-300 flex-shrink-0 border"
								style="background-color: {newColor}"
							></div>
							<select id="new-color" bind:value={newColor} class="rounded px-3 py-2 flex-1 border">
								{#each colorPalette as color}
									<option value={color.hex}>{color.name}</option>
								{/each}
							</select>
						</div>
					</div>

					<div>
						<Button variant="fill" color="primary" on:click={handleCreate} class="w-full">
							Add Preset
						</Button>
					</div>
				</div>

				<p class="text-xs text-gray-500 mt-4">
					<strong>Note:</strong> System presets (5°, 15°, 30°, 45°, 60°) cannot be modified or deleted.
					Custom presets can be edited or removed at any time.
				</p>
			</div>
		</Card>
	{/if}
</div>
