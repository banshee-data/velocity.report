<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { deleteSite, getSites, type Site } from '$lib/api';
	import { mdiDelete, mdiPencil, mdiPlus } from '@mdi/js';
	import { onMount } from 'svelte';
	import { Button, Dialog } from 'svelte-ux';

	let sites: Site[] = [];
	let loading = true;
	let error = '';

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
			error = e instanceof Error ? e.message : 'Could not load sites.';
		} finally {
			loading = false;
		}
	}

	function handleCreate() {
		goto(resolve('/site/new'));
	}

	function handleEdit(siteId: number) {
		goto(resolve(`/site/${siteId}`));
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
			error = e instanceof Error ? e.message : 'Could not delete site.';
			showDeleteDialog = false;
		}
	}
</script>

<svelte:head>
	<title>Site Management 🚴 velocity.report</title>
	<meta name="description" content="Manage radar survey site configurations and settings" />
</svelte:head>

<main id="main-content" class="vr-page">
	<div class="vr-toolbar">
		<div class="flex items-center justify-between">
			<div>
				<h1 class="text-surface-content text-2xl font-semibold">Site Management</h1>
				<p class="text-surface-content/60 mt-1 text-sm">Manage radar survey site configurations</p>
			</div>
			<div class="flex gap-2">
				<Button variant="outline" on:click={loadSites} disabled={loading}>
					{loading ? 'Loading...' : 'Refresh'}
				</Button>
				<Button on:click={handleCreate} icon={mdiPlus} variant="fill" color="primary">
					New Site
				</Button>
			</div>
		</div>
	</div>

	<div class="flex flex-1 overflow-hidden">
		<div class="flex-1 overflow-y-auto p-6">
			<div class="vr-content-narrow space-y-4">
				{#if error}
					<div class="mb-4 rounded bg-red-50 px-4 py-3 text-sm text-red-600">
						{error}
						<button class="ml-2 underline" on:click={loadSites}>Retry</button>
					</div>
				{/if}

				{#if loading}
					<div class="text-surface-content/50 py-12 text-center">Loading sites...</div>
				{:else if sites.length === 0}
					<div class="text-surface-content/50 py-12 text-center">
						<p class="mb-4 text-lg">No sites configured yet.</p>
						<Button on:click={handleCreate} icon={mdiPlus} variant="fill" color="primary">
							Create your first site
						</Button>
					</div>
				{:else}
					<div class="overflow-x-auto">
						<table class="w-full border-collapse">
							<caption class="sr-only">List of configured radar survey sites</caption>
							<thead>
								<tr class="border-surface-content/10 border-b">
									<th scope="col" class="px-4 py-2 text-left font-semibold">Name</th>
									<th scope="col" class="hidden px-4 py-2 text-left font-semibold sm:table-cell">
										Location
									</th>
									<th scope="col" class="px-4 py-2 text-right font-semibold">Actions</th>
								</tr>
							</thead>
							<tbody>
								{#each sites as site (site.id)}
									<tr
										class="hover:bg-surface-50 border-surface-content/10 border-b transition-colors"
									>
										<th scope="row" class="px-4 py-2 text-left font-medium">{site.name}</th>
										<td class="hidden px-4 py-2 sm:table-cell">{site.location}</td>
										<td class="px-4 py-2 text-right">
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
						</table>
					</div>
				{/if}
			</div>
		</div>
	</div>
</main>

<!-- Delete Confirmation Dialog -->
<Dialog bind:open={showDeleteDialog} aria-modal="true" role="alertdialog">
	<div slot="title">Delete Site</div>

	<div class="space-y-4">
		<p>
			This will permanently delete <strong>{deletingSite?.name}</strong> and its configuration.
		</p>
		<p class="text-surface-content/60 text-sm">This cannot be undone.</p>
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
