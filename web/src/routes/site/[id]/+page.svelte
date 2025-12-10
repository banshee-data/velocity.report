<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { mdiArrowLeft, mdiCheck, mdiClose, mdiContentSave, mdiPencil, mdiPlus } from '@mdi/js';
	import { onMount } from 'svelte';
	import { Button, Card, Header, TextField } from 'svelte-ux';
	import {
		createSite,
		createSiteConfigPeriod,
		createSiteVariableConfig,
		getAnglePresets,
		getSite,
		getSiteConfigPeriodsForSite,
		updateSite,
		updateSiteConfigPeriod,
		type AnglePreset,
		type SiteConfigPeriod
	} from '../../../lib/api';

	let siteId: string | null = null;
	let isNewSite = false;
	let loading = true;
	let error = '';
	let saveError = '';

	// Form fields
	let formData = {
		name: '',
		location: '',
		description: '',
		cosine_error_angle: 21.0,
		speed_limit: 25,
		surveyor: '',
		contact: '',
		address: '',
		latitude: null as number | null,
		longitude: null as number | null,
		site_description: '',
		speed_limit_note: ''
	};

	let formErrors: Record<string, string> = {};

	// Config period management
	let configPeriods: SiteConfigPeriod[] = [];
	let anglePresets: AnglePreset[] = [];
	let loadingPeriods = false;
	let showAddPeriod = false;
	let editingPeriodId: number | null = null;

	// New period form
	let newPeriod = {
		angle: 0,
		start_date: '',
		end_date: '',
		notes: '',
		is_active: false
	};

	// Edit period form
	let editPeriod = {
		angle: 0,
		start_date: '',
		end_date: '',
		notes: '',
		is_active: false
	};

	onMount(async () => {
		// Get the site ID from the URL
		const pathParts = window.location.pathname.split('/');
		const id = pathParts[pathParts.length - 1];

		if (id === 'new') {
			isNewSite = true;
			loading = false;
		} else {
			siteId = id;
			await Promise.all([loadSite(), loadConfigPeriods(), loadAnglePresets()]);
		}
	});

	async function loadSite() {
		if (!siteId) return;

		loading = true;
		error = '';
		try {
			const site = await getSite(parseInt(siteId));
			formData = {
				name: site.name,
				location: site.location,
				description: site.description || '',
				cosine_error_angle: site.cosine_error_angle,
				speed_limit: site.speed_limit,
				surveyor: site.surveyor,
				contact: site.contact,
				address: site.address || '',
				latitude: site.latitude || null,
				longitude: site.longitude || null,
				site_description: site.site_description || '',
				speed_limit_note: site.speed_limit_note || ''
			};
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load site';
		} finally {
			loading = false;
		}
	}

	async function loadConfigPeriods() {
		if (!siteId) return;

		loadingPeriods = true;
		try {
			configPeriods = await getSiteConfigPeriodsForSite(parseInt(siteId));
		} catch (e) {
			console.error('Failed to load config periods:', e);
		} finally {
			loadingPeriods = false;
		}
	}

	async function loadAnglePresets() {
		try {
			anglePresets = await getAnglePresets();
		} catch (e) {
			console.error('Failed to load angle presets:', e);
		}
	}

	function formatDate(unixTimestamp: number): string {
		return new Date(unixTimestamp * 1000).toISOString().split('T')[0];
	}

	function parseDate(dateStr: string): number {
		return Math.floor(new Date(dateStr).getTime() / 1000);
	}

	function startAddPeriod() {
		newPeriod = {
			angle: anglePresets[0]?.angle || 0,
			start_date: new Date().toISOString().split('T')[0],
			end_date: '',
			notes: '',
			is_active: false
		};
		showAddPeriod = true;
	}

	async function handleAddPeriod() {
		if (!siteId) return;

		try {
			// Create variable config first
			const varConfig = await createSiteVariableConfig({
				cosine_error_angle: newPeriod.angle
			});

			// Create config period
			await createSiteConfigPeriod({
				site_id: parseInt(siteId),
				site_variable_config_id: varConfig.id,
				effective_start_unix: parseDate(newPeriod.start_date),
				effective_end_unix: newPeriod.end_date ? parseDate(newPeriod.end_date) : null,
				is_active: newPeriod.is_active,
				notes: newPeriod.notes
			});

			showAddPeriod = false;
			await loadConfigPeriods();
		} catch (e) {
			alert('Failed to add config period: ' + (e instanceof Error ? e.message : 'Unknown error'));
		}
	}

	function startEditPeriod(period: SiteConfigPeriod) {
		editingPeriodId = period.id;
		editPeriod = {
			angle: period.variable_config?.cosine_error_angle || 0,
			start_date: formatDate(period.effective_start_unix),
			end_date: period.effective_end_unix ? formatDate(period.effective_end_unix) : '',
			notes: period.notes || '',
			is_active: period.is_active
		};
	}

	async function handleSavePeriod(periodId: number) {
		try {
			// Create new variable config
			const varConfig = await createSiteVariableConfig({
				cosine_error_angle: editPeriod.angle
			});

			// Update period
			await updateSiteConfigPeriod(periodId, {
				site_variable_config_id: varConfig.id,
				effective_start_unix: parseDate(editPeriod.start_date),
				effective_end_unix: editPeriod.end_date ? parseDate(editPeriod.end_date) : null,
				is_active: editPeriod.is_active,
				notes: editPeriod.notes
			});

			editingPeriodId = null;
			await loadConfigPeriods();
		} catch (e) {
			alert(
				'Failed to update config period: ' + (e instanceof Error ? e.message : 'Unknown error')
			);
		}
	}

	function cancelEdit() {
		editingPeriodId = null;
	}

	function cancelAdd() {
		showAddPeriod = false;
	}

	function validateForm(): boolean {
		formErrors = {};

		if (!formData.name.trim()) {
			formErrors.name = 'Name is required';
		}
		if (!formData.location.trim()) {
			formErrors.location = 'Location is required';
		}
		if (!formData.surveyor.trim()) {
			formErrors.surveyor = 'Surveyor is required';
		}
		if (!formData.contact.trim()) {
			formErrors.contact = 'Contact is required';
		}
		if (formData.cosine_error_angle === null || formData.cosine_error_angle === undefined) {
			formErrors.cosine_error_angle = 'Cosine error angle is required';
		}

		return Object.keys(formErrors).length === 0;
	}

	async function handleSave() {
		if (!validateForm()) {
			return;
		}

		saveError = '';

		try {
			const siteData = {
				name: formData.name,
				location: formData.location,
				description: formData.description || null,
				cosine_error_angle: formData.cosine_error_angle,
				speed_limit: formData.speed_limit,
				surveyor: formData.surveyor,
				contact: formData.contact,
				address: formData.address || null,
				latitude: formData.latitude,
				longitude: formData.longitude,
				include_map: false, // Hardcoded to false
				site_description: formData.site_description || null,
				speed_limit_note: formData.speed_limit_note || null
			};

			if (isNewSite) {
				await createSite(siteData);
			} else if (siteId) {
				await updateSite(parseInt(siteId), siteData);
			}

			goto(resolve('/site'));
		} catch (e) {
			saveError = e instanceof Error ? e.message : 'Failed to save site';
		}
	}

	function handleCancel() {
		goto(resolve('/site'));
	}
</script>

<svelte:head>
	<title>{isNewSite ? 'New Site' : 'Edit Site'} 🚴 velocity.report</title>
	<meta
		name="description"
		content={isNewSite
			? 'Create a new radar survey site configuration'
			: 'Edit radar survey site configuration'}
	/>
</svelte:head>

<main id="main-content" class="space-y-6 p-4">
	<div class="flex items-center justify-between">
		<Header
			title={isNewSite ? 'Create New Site' : 'Edit Site'}
			subheading={isNewSite ? 'Add a new radar survey site' : 'Update site configuration'}
		/>
		<Button on:click={handleCancel} icon={mdiArrowLeft} variant="outline">Back to List</Button>
	</div>

	{#if loading}
		<div role="status" aria-live="polite">
			<p>Loading site…</p>
		</div>
	{:else if error}
		<div
			role="alert"
			aria-live="assertive"
			class="rounded border-red-300 bg-red-50 p-3 text-red-800 border"
		>
			<strong>Error:</strong>
			{error}
		</div>
	{:else}
		<div class="max-w-3xl space-y-6">
			{#if saveError}
				<div role="alert" class="rounded border-red-300 bg-red-50 p-3 text-red-800 border">
					<strong>Save Error:</strong>
					{saveError}
				</div>
			{/if}

			<!-- Basic Information -->
			<Card>
				<div class="space-y-4 p-6">
					<h3 class="text-lg font-semibold">Basic Information</h3>

					<TextField
						bind:value={formData.name}
						label="Site Name"
						required
						error={formErrors.name}
					/>

					<TextField
						bind:value={formData.location}
						label="Location"
						required
						error={formErrors.location}
					/>

					<TextField bind:value={formData.description} label="Description" />
				</div>
			</Card>

			<!-- Radar Configuration -->
			<Card>
				<div class="space-y-4 p-6">
					<h3 class="text-lg font-semibold">Radar Configuration</h3>

					<TextField
						bind:value={formData.cosine_error_angle}
						label="Cosine Error Angle (degrees)"
						type="decimal"
						required
						error={formErrors.cosine_error_angle}
					/>

					<TextField
						bind:value={formData.speed_limit}
						label="Speed Limit"
						type="integer"
						required
					/>
				</div>
			</Card>

			<!-- Configuration Periods -->
			{#if !isNewSite}
				<Card>
					<div class="space-y-4 p-6">
						<div class="flex items-center justify-between">
							<h3 class="text-lg font-semibold">Configuration Periods</h3>
							<Button
								on:click={startAddPeriod}
								icon={mdiPlus}
								size="sm"
								variant="fill"
								color="primary"
							>
								Add Period
							</Button>
						</div>

						<p class="text-gray-600 text-sm">
							Manage sensor angle configurations over different time periods. Each period can use a
							preset angle for cosine error correction.
						</p>

						{#if loadingPeriods}
							<div class="py-8 text-gray-500 text-center">Loading configuration periods...</div>
						{:else if configPeriods.length === 0 && !showAddPeriod}
							<div class="bg-gray-50 rounded-lg p-8 text-center">
								<p class="text-gray-600 mb-4">No configuration periods defined yet.</p>
								<Button on:click={startAddPeriod} icon={mdiPlus} variant="outline">
									Add First Period
								</Button>
							</div>
						{:else}
							<div class="overflow-x-auto">
								<table class="divide-gray-200 min-w-full divide-y">
									<thead class="bg-gray-50">
										<tr>
											<th class="px-4 py-3 text-xs font-medium text-gray-500 text-left uppercase">
												Angle
											</th>
											<th class="px-4 py-3 text-xs font-medium text-gray-500 text-left uppercase">
												Start Date
											</th>
											<th class="px-4 py-3 text-xs font-medium text-gray-500 text-left uppercase">
												End Date
											</th>
											<th class="px-4 py-3 text-xs font-medium text-gray-500 text-left uppercase">
												Status
											</th>
											<th class="px-4 py-3 text-xs font-medium text-gray-500 text-left uppercase">
												Notes
											</th>
											<th class="px-4 py-3 text-xs font-medium text-gray-500 text-left uppercase">
												Actions
											</th>
										</tr>
									</thead>
									<tbody class="divide-gray-200 bg-white divide-y">
										{#if showAddPeriod}
											<tr class="bg-blue-50">
												<td class="px-4 py-3">
													<select
														bind:value={newPeriod.angle}
														class="rounded border-gray-300 px-2 py-1 text-sm border"
													>
														{#each anglePresets as preset (preset.id)}
															<option value={preset.angle}>
																{preset.angle}° {preset.is_system ? '(System)' : ''}
															</option>
														{/each}
													</select>
												</td>
												<td class="px-4 py-3">
													<input
														type="date"
														bind:value={newPeriod.start_date}
														class="rounded border-gray-300 px-2 py-1 text-sm border"
													/>
												</td>
												<td class="px-4 py-3">
													<input
														type="date"
														bind:value={newPeriod.end_date}
														class="rounded border-gray-300 px-2 py-1 text-sm border"
													/>
												</td>
												<td class="px-4 py-3">
													<label class="gap-2 flex items-center">
														<input type="checkbox" bind:checked={newPeriod.is_active} />
														<span class="text-sm">Active</span>
													</label>
												</td>
												<td class="px-4 py-3">
													<input
														type="text"
														bind:value={newPeriod.notes}
														placeholder="Notes"
														class="rounded border-gray-300 px-2 py-1 text-sm w-full border"
													/>
												</td>
												<td class="px-4 py-3">
													<div class="gap-1 flex">
														<Button
															on:click={handleAddPeriod}
															icon={mdiCheck}
															size="sm"
															variant="fill"
															color="primary"
														>
															Save
														</Button>
														<Button
															on:click={cancelAdd}
															icon={mdiClose}
															size="sm"
															variant="outline"
														>
															Cancel
														</Button>
													</div>
												</td>
											</tr>
										{/if}
										{#each configPeriods as period (period.id)}
											<tr class:bg-green-50={period.is_active}>
												{#if editingPeriodId === period.id}
													<td class="px-4 py-3">
														<select
															bind:value={editPeriod.angle}
															class="rounded border-gray-300 px-2 py-1 text-sm border"
														>
															{#each anglePresets as preset (preset.id)}
																<option value={preset.angle}>
																	{preset.angle}° {preset.is_system ? '(System)' : ''}
																</option>
															{/each}
														</select>
													</td>
													<td class="px-4 py-3">
														<input
															type="date"
															bind:value={editPeriod.start_date}
															class="rounded border-gray-300 px-2 py-1 text-sm border"
														/>
													</td>
													<td class="px-4 py-3">
														<input
															type="date"
															bind:value={editPeriod.end_date}
															class="rounded border-gray-300 px-2 py-1 text-sm border"
														/>
													</td>
													<td class="px-4 py-3">
														<label class="gap-2 flex items-center">
															<input type="checkbox" bind:checked={editPeriod.is_active} />
															<span class="text-sm">Active</span>
														</label>
													</td>
													<td class="px-4 py-3">
														<input
															type="text"
															bind:value={editPeriod.notes}
															placeholder="Notes"
															class="rounded border-gray-300 px-2 py-1 text-sm w-full border"
														/>
													</td>
													<td class="px-4 py-3">
														<div class="gap-1 flex">
															<Button
																on:click={() => handleSavePeriod(period.id)}
																icon={mdiCheck}
																size="sm"
																variant="fill"
																color="primary"
															>
																Save
															</Button>
															<Button
																on:click={cancelEdit}
																icon={mdiClose}
																size="sm"
																variant="outline"
															>
																Cancel
															</Button>
														</div>
													</td>
												{:else}
													<td class="px-4 py-3">
														<div class="gap-2 flex items-center">
															<div
																class="h-4 w-4 rounded border"
																style="background-color: {anglePresets.find(
																	(p) => p.angle === period.variable_config?.cosine_error_angle
																)?.color_hex || '#gray'}"
															></div>
															<span class="font-medium">
																{period.variable_config?.cosine_error_angle || 0}°
															</span>
														</div>
													</td>
													<td class="px-4 py-3 text-sm">
														{formatDate(period.effective_start_unix)}
													</td>
													<td class="px-4 py-3 text-sm">
														{period.effective_end_unix
															? formatDate(period.effective_end_unix)
															: 'Ongoing'}
													</td>
													<td class="px-4 py-3">
														{#if period.is_active}
															<span
																class="bg-green-100 px-2 py-1 text-xs font-semibold text-green-800 inline-flex rounded-full"
															>
																Active
															</span>
														{:else}
															<span
																class="bg-gray-100 px-2 py-1 text-xs font-semibold text-gray-800 inline-flex rounded-full"
															>
																Inactive
															</span>
														{/if}
													</td>
													<td class="px-4 py-3 text-sm text-gray-600">
														{period.notes || '-'}
													</td>
													<td class="px-4 py-3">
														<Button
															on:click={() => startEditPeriod(period)}
															icon={mdiPencil}
															size="sm"
															variant="outline"
														>
															Edit
														</Button>
													</td>
												{/if}
											</tr>
										{/each}
									</tbody>
								</table>
							</div>
						{/if}
					</div>
				</Card>
			{/if}

			<!-- Contact Information -->
			<Card>
				<div class="space-y-4 p-6">
					<h3 class="text-lg font-semibold">Contact Information</h3>

					<TextField
						bind:value={formData.surveyor}
						label="Surveyor"
						required
						error={formErrors.surveyor}
					/>

					<TextField
						bind:value={formData.contact}
						label="Contact"
						required
						error={formErrors.contact}
					/>
				</div>
			</Card>

			<!-- Report Content -->
			<Card>
				<div class="space-y-4 p-6">
					<h3 class="text-lg font-semibold">Report Content</h3>

					<TextField
						bind:value={formData.site_description}
						label="Site Description (for report)"
						multiline
						rows={3}
					/>

					<TextField bind:value={formData.speed_limit_note} label="Speed Limit Note" />
				</div>
			</Card>

			<!-- Actions -->
			<div class="gap-2 flex justify-end">
				<Button on:click={handleCancel} variant="outline">Cancel</Button>
				<Button on:click={handleSave} icon={mdiContentSave} variant="fill" color="primary">
					{isNewSite ? 'Create Site' : 'Save Changes'}
				</Button>
			</div>
		</div>
	{/if}
</main>
