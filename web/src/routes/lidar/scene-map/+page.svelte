<script lang="ts">
	/**
	 * Scene map — every place captures have been taken.
	 *
	 * Rows group by the S2 L10 cell, which is district-scale: about 12 km by
	 * 8 km at San Francisco's latitude. That is the archive-scale roll-up, not
	 * a junction. The junction is a site — the L16 cell, roughly 180 m across
	 * — so an area typically holds several, and this is where a site's
	 * canonical pose (the midpoint of the intersection, say) is set: distinct
	 * from any one case's own sensor pose, which is where the car was that day.
	 *
	 * Canonical tokens are the identifiers. What is shown is the family
	 * display, which carries one hyphen at the family boundary and is
	 * presentation only — it is never sent back as a key.
	 */
	import { getSceneMap, setSiteCanonicalPose } from '$lib/api';
	import type { LidarSite, SceneArea, SceneMapResponse, SiteSource } from '$lib/types/captures';
	import { resolve } from '$app/paths';
	import { onMount } from 'svelte';
	import { Button } from 'svelte-ux';

	let map: SceneMapResponse | null = null;
	let loading = true;
	let error: string | null = null;
	let expanded: string | null = null;

	// Canonical-pose edit drafts, keyed by site token so several can be open
	// (in principle) without clobbering each other.
	let poseDraft: Record<string, { lat: string; lon: string; source: SiteSource; label: string }> =
		{};
	let savingSite: string | null = null;
	let poseError: Record<string, string> = {};

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

	function toggle(area: SceneArea) {
		expanded = expanded === area.s2_l10_token ? null : area.s2_l10_token;
	}

	function startEditingPose(site: LidarSite) {
		poseDraft[site.s2_l16_token] = {
			lat: site.canonical_lat != null ? String(site.canonical_lat) : '',
			lon: site.canonical_lon != null ? String(site.canonical_lon) : '',
			source: (site.canonical_source as SiteSource) || 'surveyed',
			label: site.label ?? ''
		};
		poseDraft = poseDraft;
	}

	function cancelEditingPose(token: string) {
		const next = { ...poseDraft };
		delete next[token];
		poseDraft = next;
		poseError = { ...poseError, [token]: '' };
	}

	async function savePose(token: string) {
		const draft = poseDraft[token];
		if (!draft) return;
		const lat = Number(draft.lat);
		const lon = Number(draft.lon);
		if (!Number.isFinite(lat) || !Number.isFinite(lon)) {
			poseError = { ...poseError, [token]: 'Latitude and longitude must be numbers.' };
			return;
		}
		savingSite = token;
		poseError = { ...poseError, [token]: '' };
		try {
			const updated = await setSiteCanonicalPose(token, {
				canonical_lat: lat,
				canonical_lon: lon,
				canonical_source: draft.source,
				label: draft.label || undefined
			});
			if (map) {
				map = {
					...map,
					areas: map.areas.map((area) => ({
						...area,
						sites: area.sites.map((s) => (s.s2_l16_token === token ? updated : s))
					}))
				};
			}
			cancelEditingPose(token);
		} catch (e) {
			poseError = {
				...poseError,
				[token]: e instanceof Error ? e.message : "Could not save the site's pose."
			};
		} finally {
			savingSite = null;
		}
	}

	/** A rough north-south span for the area cell, for a sense of scale. */
	function spanMetres(area: SceneArea): number {
		const latMetres = (area.ne_lat - area.sw_lat) * 111_320;
		return Math.round(latMetres);
	}

	function osmLink(lat: number, lon: number): string {
		return `https://www.openstreetmap.org/?mlat=${lat}&mlon=${lon}#map=17/${lat}/${lon}`;
	}

	onMount(load);
</script>

<svelte:head><title>Scene map — velocity.report</title></svelte:head>

<main id="main-content" class="vr-page">
	<div class="vr-toolbar">
		<header class="flex items-start justify-between gap-4">
			<div>
				<h1 class="text-surface-content text-2xl font-semibold">Scene map</h1>
				<p class="text-surface-content/60 mt-1 text-sm">
					Every place captures have been taken, grouped by S2 area. An area is one L10 cell, about
					12 km across — the archive-scale roll-up. The sites inside it are L16 cells, roughly 180 m
					across: a junction and its approaches, and where a canonical pose — the midpoint of the
					intersection, say — can be set, apart from any one case's own sensor pose.
				</p>
			</div>
			<a href={resolve('/lidar/captures')} class="text-primary text-sm hover:underline"
				>Captures →</a
			>
		</header>
	</div>

	<div class="flex flex-1 overflow-hidden">
		<div class="flex-1 overflow-y-auto p-4">
			{#if error}
				<div class="mb-4 rounded bg-red-50 px-3 py-2 text-sm text-red-600">{error}</div>
			{/if}

			{#if loading}
				<p class="text-surface-content/50 py-8 text-center text-sm">Loading the scene map…</p>
			{:else if !map || map.area_count === 0}
				<div class="border-surface-300 rounded border border-dashed p-8 text-center">
					<h2 class="text-surface-content mb-1 text-lg">No located captures yet</h2>
					<p class="text-surface-content/60 mx-auto max-w-lg text-sm">
						A site appears here once a replay case is labelled with where it was captured. Label one
						from the <a href={resolve('/lidar/captures')} class="text-primary hover:underline"
							>Captures</a
						> page, or from a case's own detail.
					</p>
				</div>
			{:else}
				<div class="text-surface-content/60 mb-3 text-sm">
					{map.area_count} area{map.area_count === 1 ? '' : 's'} · {map.site_count} site{map.site_count ===
					1
						? ''
						: 's'} · {map.case_count} located case{map.case_count === 1 ? '' : 's'}
					<span class="text-surface-content/40">
						· areas are L{map.coarse_level}, neighbourhoods L{map.fine_level}, sites L{map.precise_level}
					</span>
				</div>

				<section class="border-surface-300 overflow-hidden rounded border">
					<div
						class="border-surface-300 bg-surface-200 text-surface-content/60 flex items-center gap-3 border-b px-3 py-2 text-xs"
					>
						<span class="w-32">Area</span>
						<span class="flex-1">Centre</span>
						<span class="w-20 text-right">Span</span>
						<span class="w-16 text-right">Cases</span>
						<span class="w-28 text-right">Neighbourhoods</span>
						<span class="w-16 text-right">Sites</span>
					</div>

					{#each map.areas as area (area.s2_l10_token)}
						<div class="border-surface-300 border-b last:border-b-0">
							<button
								class="hover:bg-surface-100 flex w-full items-center gap-3 px-3 py-2 text-left text-sm"
								on:click={() => toggle(area)}
							>
								<span class="text-surface-content w-32 font-mono" title={area.s2_l10_token}>
									{area.s2_l10_display}
								</span>
								<span class="text-surface-content/70 flex-1 font-mono text-xs">
									{area.centre_lat.toFixed(5)}, {area.centre_lon.toFixed(5)}
								</span>
								<span class="text-surface-content/60 w-20 text-right text-xs">
									~{spanMetres(area).toLocaleString()} m
								</span>
								<span class="text-surface-content/70 w-16 text-right text-xs"
									>{area.case_count}</span
								>
								<span class="text-surface-content/70 w-28 text-right text-xs"
									>{area.neighbourhood_count}</span
								>
								<span class="text-surface-content/70 w-16 text-right text-xs"
									>{area.sites.length}</span
								>
							</button>

							{#if expanded === area.s2_l10_token}
								<div class="border-surface-300 bg-surface-100 border-t px-3 py-3">
									<div class="text-surface-content/50 mb-3 flex flex-wrap gap-4 font-mono text-xs">
										<span
											>canonical <span class="text-surface-content">{area.s2_l10_token}</span></span
										>
										<span>
											bounds {area.sw_lat.toFixed(5)}, {area.sw_lon.toFixed(5)} → {area.ne_lat.toFixed(
												5
											)},
											{area.ne_lon.toFixed(5)}
										</span>
									</div>

									<div class="space-y-3">
										{#each area.sites as site (site.s2_l16_token)}
											<div class="border-surface-300 rounded border p-2">
												<div class="flex flex-wrap items-center gap-2">
													<span
														class="text-surface-content font-mono text-sm"
														title={site.s2_l16_token}
													>
														{site.label || `site ${site.s2_l16_token}`}
													</span>
													<span class="text-surface-content/40 text-xs"
														>{site.cases.length} capture{site.cases.length === 1 ? '' : 's'}</span
													>
													<span class="flex-1"></span>
													{#if site.canonical_lat != null && site.canonical_lon != null}
														<span class="text-surface-content/60 font-mono text-xs">
															{site.canonical_lat.toFixed(5)}, {site.canonical_lon.toFixed(5)}
															<span class="text-surface-content/40">({site.canonical_source})</span>
														</span>
														<!-- eslint-disable svelte/no-navigation-without-resolve -->
														<a
															href={osmLink(site.canonical_lat, site.canonical_lon)}
															target="_blank"
															rel="noopener noreferrer"
															class="text-primary text-xs hover:underline">map ↗</a
														>
														<!-- eslint-enable svelte/no-navigation-without-resolve -->
													{:else}
														<span class="text-surface-content/40 text-xs"
															>no canonical pose set</span
														>
													{/if}
													{#if !poseDraft[site.s2_l16_token]}
														<button
															class="text-primary text-xs hover:underline"
															on:click={() => startEditingPose(site)}
														>
															{site.canonical_lat != null ? 'edit' : 'set pose'}
														</button>
													{/if}
												</div>

												{#if poseDraft[site.s2_l16_token]}
													{@const draft = poseDraft[site.s2_l16_token]}
													<div class="mt-2 flex flex-wrap items-center gap-2">
														<input
															class="border-surface-300 bg-surface-100 w-24 rounded border px-2 py-1 font-mono text-xs"
															placeholder="name"
															bind:value={draft.label}
														/>
														<input
															class="border-surface-300 bg-surface-100 w-28 rounded border px-2 py-1 font-mono text-xs"
															placeholder="latitude"
															bind:value={draft.lat}
														/>
														<input
															class="border-surface-300 bg-surface-100 w-28 rounded border px-2 py-1 font-mono text-xs"
															placeholder="longitude"
															bind:value={draft.lon}
														/>
														<select
															class="border-surface-300 bg-surface-100 rounded border px-2 py-1 text-xs"
															bind:value={draft.source}
														>
															<option value="surveyed">surveyed</option>
															<option value="operator">entered by hand</option>
														</select>
														<Button
															size="sm"
															variant="fill"
															color="primary"
															disabled={savingSite === site.s2_l16_token}
															on:click={() => savePose(site.s2_l16_token)}
														>
															{savingSite === site.s2_l16_token ? 'Saving…' : 'Save'}
														</Button>
														<Button
															size="sm"
															variant="outline"
															on:click={() => cancelEditingPose(site.s2_l16_token)}
														>
															Cancel
														</Button>
													</div>
													{#if poseError[site.s2_l16_token]}
														<p class="mt-1 text-xs text-red-600">{poseError[site.s2_l16_token]}</p>
													{/if}
												{/if}

												<div class="border-surface-300 mt-2 overflow-hidden rounded border">
													{#each site.cases as c (c.replay_case_id)}
														<a
															href={resolve('/lidar/replay-cases')}
															class="border-surface-300 hover:bg-surface-200 flex items-center gap-3 border-b px-2 py-1.5 text-xs last:border-b-0"
														>
															<span class="text-surface-content flex-1 truncate">
																{c.description || c.replay_case_id}
															</span>
															<span class="text-surface-content/50 w-40 font-mono">
																sensor pose {c.origin_lat.toFixed(5)}, {c.origin_lon.toFixed(5)}
															</span>
														</a>
													{/each}
												</div>
											</div>
										{/each}
									</div>
								</div>
							{/if}
						</div>
					{/each}
				</section>

				<p class="text-surface-content/40 mt-3 text-xs">
					Displayed values are family displays — the canonical token with one hyphen at the family
					boundary. Only the canonical token, shown on hover and when an area is expanded, is an
					identifier.
				</p>

				<div class="mt-4">
					<Button size="sm" variant="outline" on:click={load}>Refresh</Button>
				</div>
			{/if}
		</div>
	</div>
</main>
