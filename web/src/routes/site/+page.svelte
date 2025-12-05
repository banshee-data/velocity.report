<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { mdiChevronLeft, mdiChevronRight, mdiDelete, mdiPencil, mdiPlus } from '@mdi/js';
	import { onMount } from 'svelte';
	import { Button, Card, Dialog, Header, Menu, MenuItem, Table, Toggle } from 'svelte-ux';
	import { deleteSite, getSitesPaginated, type Site } from '../../lib/api';

	let sites = $state<Site[]>([]);
	let loading = $state(true);
	let error = $state('');
	let totalSites = $state(0);

	// Pagination state
	let currentPage = $state(1);
	let perPage = $state(5);
	const perPageOptions = [3, 5, 10];

	// Computed pagination values
	let totalPages = $derived(Math.ceil(totalSites / perPage));
	let from = $derived((currentPage - 1) * perPage + 1);
	let to = $derived(Math.min(currentPage * perPage, totalSites));
	let isFirstPage = $derived(currentPage === 1);
	let isLastPage = $derived(currentPage >= totalPages);

	// Delete confirmation
	let showDeleteDialog = $state(false);
	let deletingSite = $state<Site | null>(null);

	// Track if this is the first load to avoid double-loading
	let isInitialLoad = true;

	onMount(async () => {
		await loadSites();
		isInitialLoad = false;
	});

	// Watch pagination changes (but skip initial load)
	$effect(() => {
		// Access reactive values to trigger effect
		void currentPage;
		void perPage;

		if (!isInitialLoad) {
			loadSites();
		}
	});

	async function loadSites() {
		loading = true;
		error = '';
		try {
			const response = await getSitesPaginated(currentPage, perPage);
			sites = response.sites;
			totalSites = response.total;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load sites';
		} finally {
			loading = false;
		}
	}

	function nextPage() {
		if (!isLastPage) {
			currentPage++;
		}
	}

	function prevPage() {
		if (!isFirstPage) {
			currentPage--;
		}
	}

	function changePerPage(newPerPage: number) {
		perPage = newPerPage;
		// Reset to first page when changing perPage
		currentPage = 1;
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
			class="rounded border-red-300 bg-red-50 p-3 text-red-800 border"
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
						{#each sites as site (site.id)}
							<tr class="border-surface-content/10 border-t">
								<th scope="row" class="py-4 font-medium">{site.name}</th>
								<td class="py-4">{site.location}</td>
								<td class="py-4">{site.cosine_error_angle}°</td>
								<td class="py-4 text-right">
									<div class="gap-2 flex justify-end">
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
				<div class="border-surface-content/10 gap-2 p-4 flex items-center justify-center border-t">
					<!-- Per Page Selector -->
					<div class="text-surface-content/70 gap-2 text-sm flex items-center">
						<span>Per page:</span>
						<Toggle let:on={open} let:toggle let:toggleOff>
							<Button on:click={toggle} size="sm" variant="outline">
								{perPage}
							</Button>
							<Menu {open} on:close={toggleOff} offset={8}>
								{#each perPageOptions as option (option)}
									<MenuItem
										selected={perPage === option}
										on:click={() => {
											changePerPage(option);
											toggleOff();
										}}
									>
										{option}
									</MenuItem>
								{/each}
							</Menu>
						</Toggle>
					</div>

					<!-- Previous Page Button -->
					<Button
						icon={mdiChevronLeft}
						on:click={prevPage}
						disabled={isFirstPage}
						size="sm"
						variant="outline"
						aria-label="Previous page"
					/>

					<!-- Page Info -->
					<div class="text-sm tabular-nums">
						{from}-{to} of {totalSites}
					</div>

					<!-- Next Page Button -->
					<Button
						icon={mdiChevronRight}
						on:click={nextPage}
						disabled={isLastPage}
						size="sm"
						variant="outline"
						aria-label="Next page"
					/>
				</div>
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
