<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { mdiDelete, mdiPencil, mdiPlus } from '@mdi/js';
	import { onMount } from 'svelte';
	import { Button, Card, Dialog, Header, Pagination, Table } from 'svelte-ux';
	import { derived, writable } from 'svelte/store';
	import { deleteSite, getSitesPaginated, type Site } from '../../lib/api';

	let sites = $state<Site[]>([]);
	let loading = $state(true);
	let error = $state('');

	// Pagination state
	const page = writable(1);
	const perPage = writable(5);
	const total = writable(0);
	const perPageOptions = [3, 5, 10];

	// Create pagination store compatible with Pagination component
	const pagination = derived([page, perPage, total], ([$page, $perPage, $total]) => {
		const totalPages = Math.ceil($total / $perPage);
		const from = ($page - 1) * $perPage + 1;
		const to = Math.min($page * $perPage, $total);
		return {
			page: $page,
			perPage: $perPage,
			total: $total,
			from,
			to,
			totalPages,
			isFirst: $page === 1,
			isLast: $page >= totalPages,
			hasPrevious: $page > 1,
			hasNext: $page < totalPages,
			slice: <T,>(data: T[]) => {
				const start = ($page - 1) * $perPage;
				const end = start + $perPage;
				return data.slice(start, end);
			}
		};
	});

	// Create methods for pagination store
	const paginationWithMethods = {
		subscribe: pagination.subscribe,
		nextPage: () => page.update((p) => p + 1),
		prevPage: () => page.update((p) => Math.max(1, p - 1)),
		firstPage: () => page.set(1),
		lastPage: () => {
			let currentTotal = 0;
			const unsubscribe = total.subscribe((t) => {
				currentTotal = t;
			});
			unsubscribe();
			let currentPerPage = 5;
			const unsubscribe2 = perPage.subscribe((pp) => {
				currentPerPage = pp;
			});
			unsubscribe2();
			const totalPages = Math.ceil(currentTotal / currentPerPage);
			page.set(totalPages);
		},
		setPage: (newPage: number) => page.set(newPage),
		setPerPage: (newPerPage: number) => {
			perPage.set(newPerPage);
			page.set(1);
		},
		setTotal: (newTotal: number) => total.set(newTotal)
	};

	// Subscribe to pagination changes
	let currentPage = $state(1);
	let currentPerPage = $state(5);
	pagination.subscribe((p) => {
		currentPage = p.page;
		currentPerPage = p.perPage;
	});

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
		void currentPerPage;

		if (!isInitialLoad) {
			loadSites();
		}
	});

	async function loadSites() {
		loading = true;
		error = '';
		try {
			const response = await getSitesPaginated(currentPage, currentPerPage);
			sites = response.sites || [];
			paginationWithMethods.setTotal(response.total || 0);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load sites';
			sites = []; // Ensure sites is always an array
			paginationWithMethods.setTotal(0);
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
	{:else if !sites || sites.length === 0}
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
				<div class="border-surface-content/10 p-4 flex items-center justify-center border-t">
					<Pagination pagination={paginationWithMethods} {perPageOptions} />
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
