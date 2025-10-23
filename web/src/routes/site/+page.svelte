<script lang="ts">
	import { goto } from '$app/navigation';
	import { mdiDelete, mdiPencil, mdiPlus } from '@mdi/js';
	import { onMount } from 'svelte';
	import { Button, Card, Dialog, Header, Table } from 'svelte-ux';
	import { deleteSite, getSites, type Site } from '../../lib/api';

	let sites: Site[] = [];
	let loading = true;
	let error = '';

	// Delete confirmation
	let showDeleteDialog = false;
	let deletingSite: Site | null = null;

	onMount(async () => {
		await loadSites();
	});

	async function loadSites() {
		loading = true;
		error = '';
		try {
			sites = await getSites();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load sites';
		} finally {
			loading = false;
		}
	}

	function handleCreate() {
		goto(`/app/site/new`);
	}

	function handleEdit(siteId: number) {
		goto(`/app/site/${siteId}`);
	}

	function openDeleteDialog(site: Site) {
		deletingSite = site;
		showDeleteDialog = true;
	}

	async function handleDelete() {
		if (!deletingSite) return;

		try {
			await deleteSite(deletingSite.id);
			showDeleteDialog = false;
			deletingSite = null;
			await loadSites();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to delete site';
			showDeleteDialog = false;
		}
	}
</script>

<svelte:head>
	<title>Site Management 🚴 velocity.report</title>
	<meta name="description" content="Manage radar survey site configurations and settings" />
</svelte:head>

<main id="main-content" class="space-y-6 p-4">
	<div class="flex items-center justify-between">
		<Header title="Site Management" subheading="Manage radar survey site configurations" />
		<Button on:click={handleCreate} icon={mdiPlus} variant="fill" color="primary">New Site</Button>
	</div>

	{#if loading}
		<div role="status" aria-live="polite">
			<p>Loading sites…</p>
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
	{:else if sites.length === 0}
		<Card>
			<div class="text-surface-content/60 p-8 text-center">
				<p class="mb-4 text-lg">No sites configured yet</p>
				<Button on:click={handleCreate} icon={mdiPlus} variant="fill" color="primary">
					Create Your First Site
				</Button>
			</div>
		</Card>
	{:else}
		<Card>
			<div class="px-4">
				<Table>
					<caption class="sr-only">List of configured radar survey sites</caption>
					<thead>
						<tr>
							<th scope="col" class="py-3">Name</th>
							<th scope="col" class="py-3">Location</th>
							<th scope="col" class="py-3">Cosine Angle</th>
							<th scope="col" class="py-3 text-right">Actions</th>
						</tr>
					</thead>
					<tbody>
						{#each sites as site}
							<tr class="border-surface-content/10 border-t">
								<th scope="row" class="py-4 font-medium">{site.name}</th>
								<td class="py-4">{site.location}</td>
								<td class="py-4">{site.cosine_error_angle}°</td>
								<td class="py-4 text-right">
									<div class="flex justify-end gap-2">
										<Button
											icon={mdiPencil}
											size="sm"
											variant="outline"
											on:click={() => handleEdit(site.id)}
											aria-label="Edit {site.name}"
										>
											Edit
										</Button>
										<Button
											icon={mdiDelete}
											size="sm"
											variant="outline"
											color="danger"
											on:click={() => openDeleteDialog(site)}
											aria-label="Delete {site.name}"
										>
											Delete
										</Button>
									</div>
								</td>
							</tr>
						{/each}
					</tbody>
				</Table>
			</div>
		</Card>
	{/if}
</main>

<!-- Delete Confirmation Dialog -->
<Dialog bind:open={showDeleteDialog} aria-modal="true" role="alertdialog">
	<div slot="title">Confirm Delete</div>

	<div class="space-y-4">
		<p>
			Are you sure you want to delete the site <strong>{deletingSite?.name}</strong>?
		</p>
		<p class="text-surface-content/60 text-sm">This action cannot be undone.</p>
	</div>

	<div slot="actions">
		<Button
			on:click={() => {
				showDeleteDialog = false;
			}}
			variant="outline"
		>
			Cancel
		</Button>
		<Button on:click={handleDelete} icon={mdiDelete} variant="fill" color="danger">Delete</Button>
	</div>
</Dialog>
