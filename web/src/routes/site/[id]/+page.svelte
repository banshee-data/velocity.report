<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { mdiArrowLeft, mdiContentSave } from '@mdi/js';
	import { onMount } from 'svelte';
	import { Button, Card, Header, TextField } from 'svelte-ux';
	import {
		createSite,
		getSite,
		updateSite,
		getSpeedLimitSchedulesForSite,
		createSpeedLimitSchedule,
		updateSpeedLimitSchedule,
		deleteSpeedLimitSchedule,
		type SpeedLimitSchedule
	} from '../../../lib/api';
	import SpeedLimitScheduleEditor from '../../../lib/components/SpeedLimitScheduleEditor.svelte';

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

	let speedLimitSchedules: SpeedLimitSchedule[] = [];
	let originalSchedules: SpeedLimitSchedule[] = [];

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
			await loadSchedules();
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

	async function loadSchedules() {
		if (!siteId) return;

		try {
			speedLimitSchedules = await getSpeedLimitSchedulesForSite(parseInt(siteId));
			originalSchedules = JSON.parse(JSON.stringify(speedLimitSchedules)); // Deep copy
		} catch (e) {
			console.error('Failed to load schedules:', e);
			// Don't fail the whole page if schedules fail to load
		}
	}

	function handleSchedulesChange(updatedSchedules: SpeedLimitSchedule[]) {
		speedLimitSchedules = updatedSchedules;
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

	async function saveSchedules(savedSiteId: number) {
		// Determine which schedules to create, update, or delete
		const schedulesToCreate = speedLimitSchedules.filter((s) => s.id < 0);
		const schedulesToUpdate = speedLimitSchedules.filter(
			(s) => s.id > 0 && originalSchedules.find((o) => o.id === s.id)
		);
		const schedulesToDelete = originalSchedules.filter(
			(o) => !speedLimitSchedules.find((s) => s.id === o.id)
		);

		// Create new schedules
		for (const schedule of schedulesToCreate) {
			await createSpeedLimitSchedule({
				site_id: savedSiteId,
				day_of_week: schedule.day_of_week,
				start_time: schedule.start_time,
				end_time: schedule.end_time,
				speed_limit: schedule.speed_limit
			});
		}

		// Update existing schedules
		for (const schedule of schedulesToUpdate) {
			await updateSpeedLimitSchedule(schedule.id, {
				site_id: savedSiteId,
				day_of_week: schedule.day_of_week,
				start_time: schedule.start_time,
				end_time: schedule.end_time,
				speed_limit: schedule.speed_limit
			});
		}

		// Delete removed schedules
		for (const schedule of schedulesToDelete) {
			await deleteSpeedLimitSchedule(schedule.id);
		}
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

			let savedSiteId: number;

			if (isNewSite) {
				const createdSite = await createSite(siteData);
				savedSiteId = createdSite.id;
			} else if (siteId) {
				await updateSite(parseInt(siteId), siteData);
				savedSiteId = parseInt(siteId);
			} else {
				throw new Error('No site ID available');
			}

			// Save speed limit schedules
			await saveSchedules(savedSiteId);

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

			<!-- Speed Limit Schedules -->
			{#if !isNewSite && siteId}
				<SpeedLimitScheduleEditor
					siteId={parseInt(siteId)}
					schedules={speedLimitSchedules}
					onSchedulesChange={handleSchedulesChange}
				/>
			{/if}

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
