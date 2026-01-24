<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import {
		createSite,
		getSite,
		getTimeline,
		listSiteConfigPeriods,
		updateSite,
		upsertSiteConfigPeriod,
		type SiteConfigPeriod
	} from '$lib/api';
	import { mdiArrowLeft, mdiContentSave } from '@mdi/js';
	import { onMount } from 'svelte';
	import { Button, Card, Header, TextField } from 'svelte-ux';

	let siteId: string | null = null;
	let isNewSite = false;
	let loading = true;
	let error = '';
	let saveError = '';
	let periodsError = '';
	let savingPeriod = false;
	let configPeriods: SiteConfigPeriod[] = [];
	let unconfiguredPeriods: Array<{ start_unix: number; end_unix: number }> = [];

	// Form fields
	let formData = {
		name: '',
		location: '',
		surveyor: '',
		contact: '',
		address: '',
		latitude: null as number | null,
		longitude: null as number | null,
		site_description: ''
	};

	let formErrors: Record<string, string> = {};
	let periodFormErrors: Record<string, string> = {};

	let periodForm = {
		id: null as number | null,
		start: '',
		end: '',
		angle: 0,
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
			await loadSite();
			await loadConfigPeriods();
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
				surveyor: site.surveyor,
				contact: site.contact,
				address: site.address || '',
				latitude: site.latitude || null,
				longitude: site.longitude || null,
				site_description: site.site_description || ''
			};
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load site';
		} finally {
			loading = false;
		}
	}

	async function loadConfigPeriods() {
		if (!siteId) return;
		periodsError = '';
		try {
			const siteNumericId = parseInt(siteId);
			configPeriods = await listSiteConfigPeriods(siteNumericId);
			const timeline = await getTimeline(siteNumericId);
			unconfiguredPeriods = timeline.unconfigured_periods ?? [];
		} catch (e) {
			periodsError = e instanceof Error ? e.message : 'Failed to load site configuration periods';
		}
	}

	function formatUnixSeconds(value: number | null | undefined): string {
		if (value === null || value === undefined) return '—';
		if (value === 0) return 'Initial';
		const date = new Date(value * 1000);
		if (Number.isNaN(date.getTime())) return '—';
		return date.toLocaleString();
	}

	function toUnixSeconds(value: string): number | null {
		if (!value) return null;
		const parsed = new Date(value).getTime();
		if (Number.isNaN(parsed)) return null;
		return Math.floor(parsed / 1000);
	}

	function editPeriod(period: SiteConfigPeriod) {
		periodForm = {
			id: period.id ?? null,
			start: period.effective_start_unix
				? new Date(period.effective_start_unix * 1000).toISOString().slice(0, 16)
				: '',
			end: period.effective_end_unix
				? new Date(period.effective_end_unix * 1000).toISOString().slice(0, 16)
				: '',
			angle: period.cosine_error_angle ?? 0,
			notes: period.notes ?? '',
			is_active: period.is_active
		};
	}

	function resetPeriodForm() {
		periodForm = {
			id: null,
			start: '',
			end: '',
			angle: 0,
			notes: '',
			is_active: false
		};
		periodFormErrors = {};
	}

	function validatePeriodForm(): boolean {
		periodFormErrors = {};
		const startUnix = toUnixSeconds(periodForm.start);
		const endUnix = toUnixSeconds(periodForm.end);
		const angleValue = Number(periodForm.angle);

		if (!startUnix) {
			periodFormErrors.start = 'Start time is required';
		}
		if (startUnix === 0 && !periodForm.id) {
			periodFormErrors.start = 'Start time must be after the initial configuration';
		}
		if (periodForm.end && !endUnix) {
			periodFormErrors.end = 'End time must be a valid date';
		}
		if (endUnix && startUnix && endUnix <= startUnix) {
			periodFormErrors.end = 'End time must be after the start time';
		}
		if (Number.isNaN(angleValue)) {
			periodFormErrors.angle = 'Cosine error angle must be a valid number';
		} else if (angleValue < 0.0 || angleValue > 80.0) {
			periodFormErrors.angle = 'Cosine error angle must be between 0 and 80 degrees';
		}

		return Object.keys(periodFormErrors).length === 0;
	}

	async function savePeriod() {
		if (!siteId || !validatePeriodForm()) {
			return;
		}
		savingPeriod = true;
		periodsError = '';
		try {
			const startUnix = toUnixSeconds(periodForm.start);
			const endUnix = toUnixSeconds(periodForm.end);
			const angleValue = Number(periodForm.angle);
			await upsertSiteConfigPeriod({
				id: periodForm.id ?? undefined,
				site_id: parseInt(siteId),
				effective_start_unix: startUnix,
				effective_end_unix: endUnix ?? null,
				is_active: periodForm.is_active,
				notes: periodForm.notes || null,
				cosine_error_angle: angleValue
			});
			resetPeriodForm();
			await loadConfigPeriods();
		} catch (e) {
			periodsError = e instanceof Error ? e.message : 'Failed to save site configuration period';
		} finally {
			savingPeriod = false;
		}
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
				surveyor: formData.surveyor,
				contact: formData.contact,
				address: formData.address || null,
				latitude: formData.latitude,
				longitude: formData.longitude,
				include_map: false, // Hardcoded to false
				site_description: formData.site_description || null
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

<div id="main-content" class="space-y-6 p-4">
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
			class="rounded border border-red-300 bg-red-50 p-3 text-red-800"
		>
			<strong>Error:</strong>
			{error}
		</div>
	{:else}
		<div class="max-w-3xl space-y-6">
			{#if saveError}
				<div role="alert" class="rounded border border-red-300 bg-red-50 p-3 text-red-800">
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
				</div>
			</Card>

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
						classes={{ input: 'h-40' }}
					/>
				</div>
			</Card>

			{#if !isNewSite}
				<Card>
					<div class="space-y-4 p-6">
						<h3 class="text-lg font-semibold">Configuration Periods</h3>
						<p class="text-surface-600-300-token text-sm">
							Define when cosine correction angles changed so reports apply the correct adjustments.
						</p>

						{#if periodsError}
							<div role="alert" class="rounded border border-red-300 bg-red-50 p-3 text-red-800">
								<strong>Error:</strong>
								{periodsError}
							</div>
						{/if}

						<div class="grid gap-4 md:grid-cols-2">
							<TextField
								bind:value={periodForm.start}
								label="Start time"
								type="datetime-local"
								required
								error={periodFormErrors.start}
							/>
							<TextField
								bind:value={periodForm.end}
								label="End time (optional)"
								type="datetime-local"
								error={periodFormErrors.end}
							/>
							<TextField
								bind:value={periodForm.angle}
								label="Cosine Error Angle (degrees)"
								type="number"
								step="0.1"
								required
								error={periodFormErrors.angle}
							/>
							<TextField bind:value={periodForm.notes} label="Notes" />
						</div>

						<label class="flex items-center gap-2 text-sm">
							<input type="checkbox" bind:checked={periodForm.is_active} />
							Active for new data
						</label>

						<div class="flex flex-wrap gap-3">
							<Button on:click={savePeriod} disabled={savingPeriod} icon={mdiContentSave}>
								{periodForm.id ? 'Update Period' : 'Add Period'}
							</Button>
							<Button on:click={resetPeriodForm} variant="outline">Reset</Button>
						</div>

						{#if configPeriods.length === 0}
							<p class="text-surface-600-300-token text-sm">No configuration periods yet.</p>
						{:else}
							<div class="overflow-x-auto">
								<table class="w-full text-sm">
									<thead>
										<tr class="border-b">
											<th class="px-2 py-2 text-left whitespace-nowrap">Start</th>
											<th class="px-2 py-2 text-left whitespace-nowrap">End</th>
											<th class="px-2 py-2 text-right">Angle</th>
											<th class="w-24 px-2 py-2 text-left">Notes</th>
											<th class="px-2 py-2 text-left">Active</th>
											<th class="px-2 py-2 text-left">Actions</th>
										</tr>
									</thead>
									<tbody>
										{#each configPeriods as period (period.id)}
											<tr class="border-b">
												<td class="px-2 py-2">{formatUnixSeconds(period.effective_start_unix)}</td>
												<td class="px-2 py-2">
													{period.effective_end_unix
														? formatUnixSeconds(period.effective_end_unix)
														: 'Open-ended'}
												</td>
												<td class="px-2 py-2 text-right">{period.cosine_error_angle}°</td>
												<td class="px-2 py-2">{period.notes || '—'}</td>
												<td class="px-2 py-2">{period.is_active ? 'Yes' : 'No'}</td>
												<td class="px-2 py-2">
													<Button size="sm" variant="outline" on:click={() => editPeriod(period)}>
														Edit
													</Button>
												</td>
											</tr>
										{/each}
									</tbody>
								</table>
							</div>
						{/if}

						{#if unconfiguredPeriods.length > 0}
							<div class="space-y-2 text-sm">
								<p class="font-semibold">Unconfigured data gaps</p>
								<ul class="list-disc pl-5">
									{#each unconfiguredPeriods as gap (gap.start_unix)}
										<li>
											{formatUnixSeconds(gap.start_unix)} → {formatUnixSeconds(gap.end_unix)}
										</li>
									{/each}
								</ul>
							</div>
						{/if}
					</div>
				</Card>
			{/if}

			<!-- Actions -->
			<div class="flex justify-end gap-2">
				<Button on:click={handleCancel} variant="outline">Cancel</Button>
				<Button on:click={handleSave} icon={mdiContentSave} variant="fill" color="primary">
					{isNewSite ? 'Create Site' : 'Save Changes'}
				</Button>
			</div>
		</div>
	{/if}
</div>
