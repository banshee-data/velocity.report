<script lang="ts">
	/**
	 * Scene map — every place captures have been taken.
	 *
	 * A site is an S2 L10 cell, roughly a kilometre across, so many visits to
	 * one junction collapse into a single entry however many replay cases they
	 * produced. Within a site, L13 counts deployments and L16 counts sensor
	 * positions: two sensors on opposite corners of one intersection fall in
	 * different L16 cells and are visibly two things.
	 *
	 * Canonical tokens are the identifiers. What is shown is the family
	 * display, which carries one hyphen at the family boundary and is
	 * presentation only — it is never sent back as a key.
	 */
	import { getSceneMap } from '$lib/api';
	import type { SceneMapResponse, SceneSite } from '$lib/types/captures';
	import { resolve } from '$app/paths';
	import { onMount } from 'svelte';
	import { Button } from 'svelte-ux';

	let map: SceneMapResponse | null = null;
	let loading = true;
	let error: string | null = null;
	let expanded: string | null = null;

	async function load() {
		loading = true;
		error = null;
		try {
			map = await getSceneMap();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load the scene map.';
		} finally {
			loading = false;
		}
	}

	function toggle(site: SceneSite) {
		expanded = expanded === site.s2_l10_token ? null : site.s2_l10_token;
	}

	/** A rough span for the site cell, for the operator's sense of scale. */
	function spanMetres(site: SceneSite): number {
		const latMetres = (site.ne_lat - site.sw_lat) * 111_320;
		return Math.round(latMetres);
	}

	function osmLink(site: SceneSite): string {
		return `https://www.openstreetmap.org/?mlat=${site.centre_lat}&mlon=${site.centre_lon}#map=17/${site.centre_lat}/${site.centre_lon}`;
	}

	onMount(load);
</script>

<svelte:head><title>Scene map — velocity.report</title></svelte:head>

<div class="p-4">
	<header class="mb-4 flex items-start justify-between gap-4">
		<div>
			<h1 class="text-surface-content text-2xl font-semibold">Scene map</h1>
			<p class="text-surface-content/60 mt-1 text-sm">
				Every place captures have been taken, grouped by S2 cell. A site is one L10 cell — about a
				kilometre across — so repeat visits to a junction are one entry.
			</p>
		</div>
		<a href={resolve('/lidar/captures')} class="text-primary text-sm hover:underline">Captures →</a>
	</header>

	{#if error}
		<div class="mb-4 rounded bg-red-50 px-3 py-2 text-sm text-red-600">{error}</div>
	{/if}

	{#if loading}
		<p class="text-surface-content/50 py-8 text-center text-sm">Loading the scene map…</p>
	{:else if !map || map.site_count === 0}
		<div class="border-surface-300 rounded border border-dashed p-8 text-center">
			<h2 class="text-surface-content mb-1 text-lg">No located sites yet</h2>
			<p class="text-surface-content/60 mx-auto max-w-lg text-sm">
				A site appears here once a replay case is labelled with where it was captured. Label one
				from the <a href={resolve('/lidar/captures')} class="text-primary hover:underline"
					>Captures</a
				> page, or from a case's own detail.
			</p>
		</div>
	{:else}
		<div class="text-surface-content/60 mb-3 text-sm">
			{map.site_count} site{map.site_count === 1 ? '' : 's'} · {map.case_count} located case{map.case_count ===
			1
				? ''
				: 's'}
			<span class="text-surface-content/40">
				· sites are L{map.coarse_level}, deployments L{map.fine_level}, sensor positions L{map.precise_level}
			</span>
		</div>

		<section class="border-surface-300 overflow-hidden rounded border">
			<div
				class="border-surface-300 bg-surface-200 text-surface-content/60 flex items-center gap-3 border-b px-3 py-2 text-xs"
			>
				<span class="w-32">Site</span>
				<span class="flex-1">Centre</span>
				<span class="w-20 text-right">Span</span>
				<span class="w-16 text-right">Cases</span>
				<span class="w-24 text-right">Deployments</span>
				<span class="w-20 text-right">Sensors</span>
			</div>

			{#each map.sites as site (site.s2_l10_token)}
				<div class="border-surface-300 border-b last:border-b-0">
					<button
						class="hover:bg-surface-100 flex w-full items-center gap-3 px-3 py-2 text-left text-sm"
						on:click={() => toggle(site)}
					>
						<span class="text-surface-content w-32 font-mono" title={site.s2_l10_token}>
							{site.s2_l10_display}
						</span>
						<span class="text-surface-content/70 flex-1 font-mono text-xs">
							{site.centre_lat.toFixed(5)}, {site.centre_lon.toFixed(5)}
						</span>
						<span class="text-surface-content/60 w-20 text-right text-xs">
							~{spanMetres(site).toLocaleString()} m
						</span>
						<span class="text-surface-content/70 w-16 text-right text-xs">{site.case_count}</span>
						<span class="text-surface-content/70 w-24 text-right text-xs">{site.l13_count}</span>
						<span class="text-surface-content/70 w-20 text-right text-xs">{site.l16_count}</span>
					</button>

					{#if expanded === site.s2_l10_token}
						<div class="border-surface-300 bg-surface-100 border-t px-3 py-3">
							<div class="text-surface-content/50 mb-2 flex flex-wrap gap-4 font-mono text-xs">
								<span>canonical <span class="text-surface-content">{site.s2_l10_token}</span></span>
								<span>
									bounds {site.sw_lat.toFixed(5)}, {site.sw_lon.toFixed(5)} → {site.ne_lat.toFixed(
										5
									)},
									{site.ne_lon.toFixed(5)}
								</span>
								<!-- eslint-disable svelte/no-navigation-without-resolve -->
								<a
									href={osmLink(site)}
									target="_blank"
									rel="noopener noreferrer"
									class="text-primary hover:underline">open in OpenStreetMap ↗</a
								>
								<!-- eslint-enable svelte/no-navigation-without-resolve -->
							</div>

							<div class="border-surface-300 overflow-hidden rounded border">
								{#each site.cases as c (c.replay_case_id)}
									<a
										href={resolve('/lidar/replay-cases')}
										class="border-surface-300 hover:bg-surface-200 flex items-center gap-3 border-b px-2 py-1.5 text-xs last:border-b-0"
									>
										<span class="text-surface-content flex-1 truncate">
											{c.description || c.replay_case_id}
										</span>
										<span class="text-surface-content/50 w-40 font-mono" title={c.s2_l13_token}>
											L13 {c.s2_l13_display}
										</span>
										<span class="text-surface-content/50 w-44 font-mono" title={c.s2_l16_token}>
											L16 {c.s2_l16_display}
										</span>
										<span class="text-surface-content/50 w-40 font-mono">
											{c.origin_lat.toFixed(5)}, {c.origin_lon.toFixed(5)}
										</span>
									</a>
								{/each}
							</div>
						</div>
					{/if}
				</div>
			{/each}
		</section>

		<p class="text-surface-content/40 mt-3 text-xs">
			Displayed values are family displays — the canonical token with one hyphen at the family
			boundary. Only the canonical token, shown on hover and when a site is expanded, is an
			identifier.
		</p>

		<div class="mt-4">
			<Button size="sm" variant="outline" on:click={load}>Refresh</Button>
		</div>
	{/if}
</div>
