<script lang="ts">
	import { goto } from '$app/navigation';
	import { mdiArrowLeft, mdiContentSave } from '@mdi/js';
	import { onMount } from 'svelte';
	import { Button, Card, Header, TextField, Toggle } from 'svelte-ux';
	import { createSite, getSite, updateSite } from '../../../lib/api';

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
		map_angle: null as number | null,
		include_map: true,
		site_description: '',
		speed_limit_note: ''
	};

	let formErrors: Record<string, string> = {};

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
				map_angle: site.map_angle || null,
				include_map: site.include_map,
				site_description: site.site_description || '',
				speed_limit_note: site.speed_limit_note || ''
			};
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load site';
		} finally {
			loading = false;
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
				map_angle: formData.map_angle,
				include_map: formData.include_map,
				site_description: formData.site_description || null,
				speed_limit_note: formData.speed_limit_note || null
			};

			if (isNewSite) {
				await createSite(siteData as any);
			} else if (siteId) {
				await updateSite(parseInt(siteId), siteData as any);
			}

			goto('/app/site');
		} catch (e) {
			saveError = e instanceof Error ? e.message : 'Failed to save site';
		}
	}

	function handleCancel() {
		goto('/app/site');
	}
</script>

<svelte:head>
	<title>{isNewSite ? 'New Site' : 'Edit Site'} 🚴 velocity.report</title>
</svelte:head>

<main class="space-y-6 p-4">
	<div class="flex items-center justify-between">
		<Header
			title={isNewSite ? 'Create New Site' : 'Edit Site'}
			subheading={isNewSite ? 'Add a new radar survey site' : 'Update site configuration'}
		/>
		<Button on:click={handleCancel} icon={mdiArrowLeft} variant="outline">Back to List</Button>
	</div>

	{#if loading}
		<p>Loading site…</p>
	{:else if error}
		<div class="rounded border border-red-300 bg-red-50 p-3 text-red-800">
			{error}
		</div>
	{:else}
		<div class="max-w-3xl space-y-6">
			{#if saveError}
				<div class="rounded border border-red-300 bg-red-50 p-3 text-red-800">
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

					<TextField bind:value={formData.address} label="Address" />
				</div>
			</Card>

			<!-- Map Settings -->
			<Card>
				<div class="space-y-4 p-6">
					<h3 class="text-lg font-semibold">Map Settings</h3>

					<div class="grid grid-cols-2 gap-4">
						<TextField bind:value={formData.latitude} label="Latitude" type="decimal" />

						<TextField bind:value={formData.longitude} label="Longitude" type="decimal" />
					</div>

					<TextField bind:value={formData.map_angle} label="Map Angle (degrees)" type="decimal" />

					<div class="flex items-center gap-2">
						<Toggle bind:on={formData.include_map} />
						<span>Include Map in Report</span>
					</div>
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
			<div class="flex justify-end gap-2">
				<Button on:click={handleCancel} variant="outline">Cancel</Button>
				<Button on:click={handleSave} icon={mdiContentSave} variant="fill" color="primary">
					{isNewSite ? 'Create Site' : 'Save Changes'}
				</Button>
			</div>
		</div>
	{/if}
</main>
