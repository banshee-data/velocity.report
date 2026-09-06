<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { resolve } from '$app/paths';
	import {
		createScene,
		getScene,
		getSites,
		updateScene,
		type Scene,
		type Site,
		type Vantage
	} from '$lib/api';
	import { mdiContentSave, mdiArrowLeft } from '@mdi/js';
	import { onMount } from 'svelte';
	import { Button } from 'svelte-ux';

	// The route always supplies an id, but the typed params allow undefined.
	$: sceneId = $page.params.id ?? 'new';
	$: isNew = sceneId === 'new';

	let loading = true;
	let saving = false;
	let error = '';
	let sites: Site[] = [];

	// Form state. Dates are held in the "YYYY-MM-DDTHH:MM" form a
	// datetime-local input produces; the server accepts that alongside RFC 3339.
	let form = {
		scene_id: '',
		title: '',
		description: '',
		site_id: '' as string,
		latitude: '' as string,
		longitude: '' as string,
		captured_start: '',
		captured_end: '',
		source_capture: '',
		source_vrlog_sha256: '',
		frame_count: '' as string,
		frame_stride: '' as string,
		asset_path: '',
		published: false
	};

	let resolvedPosition = '';

	// Named viewpoints. Kept as a working array so a row can be added, edited or
	// removed without round-tripping through JSON on every keystroke.
	let vantages: Vantage[] = [];
	let pasteText = '';
	let pasteError = '';

	function addVantage() {
		vantages = [
			...vantages,
			{ id: '', label: '', azimuth_deg: 0, polar_deg: 68, zoom: 0.8, offset_x: 0, offset_y: 0 }
		];
	}

	function removeVantage(index: number) {
		vantages = vantages.filter((_, i) => i !== index);
	}

	/**
	 * Accepts the numbers the public viewer's "Copy this view" button produces,
	 * so framing an angle by eye and recording it here is one paste rather than
	 * six transcribed decimals.
	 */
	function applyPastedView(index: number) {
		pasteError = '';
		try {
			const parsed = JSON.parse(pasteText);
			const row = vantages[index];
			vantages[index] = {
				...row,
				azimuth_deg: Number(parsed.azimuth_deg ?? row.azimuth_deg),
				polar_deg: Number(parsed.polar_deg ?? row.polar_deg),
				zoom: Number(parsed.zoom ?? row.zoom),
				offset_x: Number(parsed.offset_x ?? row.offset_x ?? 0),
				offset_y: Number(parsed.offset_y ?? row.offset_y ?? 0)
			};
			vantages = [...vantages];
			pasteText = '';
		} catch {
			pasteError = 'That is not the view JSON the viewer copies.';
		}
	}

	onMount(async () => {
		try {
			sites = await getSites();
		} catch {
			// A missing site list is not fatal: a scene can carry its own position.
			sites = [];
		}
		if (!isNew) {
			try {
				const scene = await getScene(sceneId);
				fillForm(scene);
			} catch (e) {
				error = e instanceof Error ? e.message : 'Could not load scene.';
			}
		}
		loading = false;
	});

	/**
	 * Renders an instant in the viewer's local time, which is what a
	 * datetime-local input shows and edits.
	 */
	function toLocalInput(value: string | null | undefined): string {
		if (!value) return '';
		const d = new Date(value);
		if (Number.isNaN(d.getTime())) return '';
		const pad = (n: number) => String(n).padStart(2, '0');
		// Seconds are kept: a capture window comes from a recording, and rounding
		// it to the minute on every save would quietly lose that precision.
		return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
	}

	function fillForm(scene: Scene) {
		form = {
			scene_id: scene.scene_id,
			title: scene.title,
			description: scene.description ?? '',
			site_id: scene.site_id == null ? '' : String(scene.site_id),
			latitude: scene.latitude == null ? '' : String(scene.latitude),
			longitude: scene.longitude == null ? '' : String(scene.longitude),
			captured_start: toLocalInput(scene.captured_start),
			captured_end: toLocalInput(scene.captured_end),
			source_capture: scene.source_capture ?? '',
			source_vrlog_sha256: scene.source_vrlog_sha256 ?? '',
			frame_count: scene.frame_count == null ? '' : String(scene.frame_count),
			frame_stride: scene.frame_stride == null ? '' : String(scene.frame_stride),
			asset_path: scene.asset_path ?? '',
			published: scene.published
		};
		vantages = scene.vantages ?? parseVantages(scene.vantages_json);
		resolvedPosition =
			scene.effective_latitude != null && scene.effective_longitude != null
				? `${scene.effective_latitude.toFixed(5)}, ${scene.effective_longitude.toFixed(5)} (${scene.position_source})`
				: 'No position resolved';
	}

	/**
	 * Converts a datetime-local value back to an explicit UTC instant.
	 *
	 * The input has no time zone, and a bare "YYYY-MM-DDTHH:MM" is read as UTC
	 * by the server. Sending the displayed local time unchanged would therefore
	 * shift the capture by the viewer's offset on every save, silently and
	 * cumulatively. Attaching the zone here keeps the instant fixed no matter
	 * where the person editing it happens to be.
	 */
	function toInstant(v: string): string | null {
		const t = v.trim();
		if (t === '') return null;
		const d = new Date(t);
		return Number.isNaN(d.getTime()) ? null : d.toISOString();
	}

	/** The server stores the list as a string; tolerate either shape. */
	function parseVantages(raw: string | null | undefined): Vantage[] {
		if (!raw) return [];
		try {
			const parsed = JSON.parse(raw);
			return Array.isArray(parsed) ? parsed : [];
		} catch {
			return [];
		}
	}

	function numberOrNull(v: string): number | null {
		const t = v.trim();
		if (t === '') return null;
		const n = Number(t);
		return Number.isFinite(n) ? n : null;
	}

	function textOrNull(v: string): string | null {
		const t = v.trim();
		return t === '' ? null : t;
	}

	async function handleSave() {
		error = '';
		saving = true;
		try {
			const payload: Partial<Scene> = {
				scene_id: form.scene_id.trim(),
				title: form.title.trim(),
				description: textOrNull(form.description),
				site_id: numberOrNull(form.site_id),
				latitude: numberOrNull(form.latitude),
				longitude: numberOrNull(form.longitude),
				captured_start: toInstant(form.captured_start),
				captured_end: toInstant(form.captured_end),
				source_capture: textOrNull(form.source_capture),
				source_vrlog_sha256: textOrNull(form.source_vrlog_sha256),
				frame_count: numberOrNull(form.frame_count),
				frame_stride: numberOrNull(form.frame_stride),
				asset_path: textOrNull(form.asset_path),
				published: form.published,
				// An empty list means "inherit from the recording", which differs
				// from a list the publisher deliberately emptied.
				vantages: vantages.length ? vantages : undefined
			};

			const saved = isNew ? await createScene(payload) : await updateScene(sceneId, payload);

			if (isNew) {
				goto(resolve(`/scene/${saved.scene_id}`));
			} else {
				fillForm(saved);
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not save scene.';
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head>
	<title>{isNew ? 'New scene' : form.title || 'Scene'}</title>
</svelte:head>

<div class="p-4">
	<div class="mb-4 flex items-center gap-3">
		<Button
			icon={mdiArrowLeft}
			on:click={() => goto(resolve('/scene'))}
			aria-label="Back to scenes"
		/>
		<h1 class="text-2xl font-semibold">{isNew ? 'New scene' : form.title || form.scene_id}</h1>
	</div>

	{#if error}
		<div class="border-danger/40 bg-danger/10 mb-4 rounded border p-3 text-sm" role="alert">
			{error}
		</div>
	{/if}

	{#if loading}
		<p class="text-surface-content/70">Loading…</p>
	{:else}
		<form class="max-w-3xl space-y-6" on:submit|preventDefault={handleSave}>
			<fieldset class="space-y-3">
				<legend class="mb-2 font-medium">Identity</legend>

				<label class="block">
					<span class="mb-1 block text-sm">Scene ID</span>
					<input
						class="border-surface-content/25 w-full rounded border bg-transparent px-3 py-2 font-mono text-sm"
						bind:value={form.scene_id}
						disabled={!isNew}
						placeholder="soma1"
						required
					/>
					<span class="text-surface-content/60 mt-1 block text-xs">
						Lowercase letters, digits, dashes and underscores. This also names the published asset
						directory, so it cannot be changed later.
					</span>
				</label>

				<label class="block">
					<span class="mb-1 block text-sm">Title</span>
					<input
						class="border-surface-content/25 w-full rounded border bg-transparent px-3 py-2 text-sm"
						bind:value={form.title}
						placeholder="SoMa 1"
						required
					/>
				</label>

				<label class="block">
					<span class="mb-1 block text-sm">Description</span>
					<textarea
						class="border-surface-content/25 w-full rounded border bg-transparent px-3 py-2 text-sm"
						rows="2"
						bind:value={form.description}></textarea>
				</label>
			</fieldset>

			<fieldset class="space-y-3">
				<legend class="mb-2 font-medium">Where</legend>

				<label class="block">
					<span class="mb-1 block text-sm">Site</span>
					<select
						class="border-surface-content/25 w-full rounded border bg-transparent px-3 py-2 text-sm"
						bind:value={form.site_id}
					>
						<option value="">No site</option>
						{#each sites as site (site.id)}
							<option value={String(site.id)}>{site.name} — {site.location}</option>
						{/each}
					</select>
					<span class="text-surface-content/60 mt-1 block text-xs">
						Leave latitude and longitude empty to inherit this site's position.
					</span>
				</label>

				<div class="grid grid-cols-2 gap-3">
					<label class="block">
						<span class="mb-1 block text-sm">Latitude</span>
						<input
							class="border-surface-content/25 w-full rounded border bg-transparent px-3 py-2 font-mono text-sm"
							bind:value={form.latitude}
							inputmode="decimal"
							placeholder="37.77493"
						/>
					</label>
					<label class="block">
						<span class="mb-1 block text-sm">Longitude</span>
						<input
							class="border-surface-content/25 w-full rounded border bg-transparent px-3 py-2 font-mono text-sm"
							bind:value={form.longitude}
							inputmode="decimal"
							placeholder="-122.41942"
						/>
					</label>
				</div>
				<p class="text-surface-content/60 text-xs">
					Give both or neither. Half a position places nothing.
					{#if !isNew && resolvedPosition}
						<br />Currently resolved to <span class="font-mono">{resolvedPosition}</span>.
					{/if}
				</p>
			</fieldset>

			<fieldset class="space-y-3">
				<legend class="mb-2 font-medium">When</legend>

				<div class="grid grid-cols-2 gap-3">
					<label class="block">
						<span class="mb-1 block text-sm">Capture start</span>
						<input
							type="datetime-local"
							step="1"
							class="border-surface-content/25 w-full rounded border bg-transparent px-3 py-2 text-sm"
							bind:value={form.captured_start}
						/>
					</label>
					<label class="block">
						<span class="mb-1 block text-sm">Capture end</span>
						<input
							type="datetime-local"
							step="1"
							class="border-surface-content/25 w-full rounded border bg-transparent px-3 py-2 text-sm"
							bind:value={form.captured_end}
						/>
					</label>
				</div>
				<p class="text-surface-content/60 text-xs">
					When the sensor recorded this, not when it was published. Duration is derived from the
					pair.
				</p>
			</fieldset>

			<fieldset class="space-y-3">
				<legend class="mb-2 font-medium">Provenance</legend>

				<label class="block">
					<span class="mb-1 block text-sm">Source capture</span>
					<input
						class="border-surface-content/25 w-full rounded border bg-transparent px-3 py-2 font-mono text-sm"
						bind:value={form.source_capture}
						placeholder="soma1-static-0.pcap"
					/>
				</label>

				<label class="block">
					<span class="mb-1 block text-sm">Source VRLOG fingerprint</span>
					<input
						class="border-surface-content/25 w-full rounded border bg-transparent px-3 py-2 font-mono text-xs"
						bind:value={form.source_vrlog_sha256}
					/>
				</label>

				<div class="grid grid-cols-2 gap-3">
					<label class="block">
						<span class="mb-1 block text-sm">Frame count</span>
						<input
							class="border-surface-content/25 w-full rounded border bg-transparent px-3 py-2 font-mono text-sm"
							bind:value={form.frame_count}
							inputmode="numeric"
						/>
					</label>
					<label class="block">
						<span class="mb-1 block text-sm">Frame stride</span>
						<input
							class="border-surface-content/25 w-full rounded border bg-transparent px-3 py-2 font-mono text-sm"
							bind:value={form.frame_stride}
							inputmode="numeric"
						/>
					</label>
				</div>
			</fieldset>

			<fieldset class="space-y-3">
				<legend class="mb-2 font-medium">Vantage points</legend>
				<p class="text-surface-content/60 text-xs">
					Named viewpoints the viewer offers. Label them for the street, not the compass:
					&ldquo;Eastbound Howard&rdquo; tells a reader what they are looking along; &ldquo;From
					east&rdquo; does not. Geometry is relative to the scene&rsquo;s own framing, so a saved
					viewpoint survives the export being regenerated. Leave the list empty to use whatever the
					recording carries.
				</p>

				{#if vantages.length}
					<label class="block">
						<span class="mb-1 block text-sm">Paste a view from the viewer</span>
						<div class="flex gap-2">
							<input
								class="border-surface-content/25 flex-1 rounded border bg-transparent px-3 py-2 font-mono text-xs"
								bind:value={pasteText}
								placeholder={'{"azimuth_deg":270,"polar_deg":72,"zoom":0.7,...}'}
							/>
						</div>
						{#if pasteError}
							<span class="text-danger mt-1 block text-xs">{pasteError}</span>
						{:else}
							<span class="text-surface-content/60 mt-1 block text-xs">
								Frame the angle in the public viewer, press &ldquo;Copy this view&rdquo;, paste
								here, then apply it to a row.
							</span>
						{/if}
					</label>
				{/if}

				{#each vantages as vantage, i (i)}
					<div class="border-surface-content/20 rounded border p-3">
						<div class="mb-2 grid grid-cols-2 gap-2">
							<label class="block">
								<span class="text-surface-content/70 mb-1 block text-xs">ID</span>
								<input
									class="border-surface-content/25 w-full rounded border bg-transparent px-2 py-1 font-mono text-xs"
									bind:value={vantage.id}
									placeholder="eb-howard"
								/>
							</label>
							<label class="block">
								<span class="text-surface-content/70 mb-1 block text-xs">Label</span>
								<input
									class="border-surface-content/25 w-full rounded border bg-transparent px-2 py-1 text-xs"
									bind:value={vantage.label}
									placeholder="Eastbound Howard"
								/>
							</label>
						</div>
						<div class="grid grid-cols-5 gap-2">
							<label class="block">
								<span class="text-surface-content/70 mb-1 block text-xs">Bearing&deg;</span>
								<input
									type="number"
									class="border-surface-content/25 w-full rounded border bg-transparent px-2 py-1 font-mono text-xs"
									bind:value={vantage.azimuth_deg}
								/>
							</label>
							<label class="block">
								<span class="text-surface-content/70 mb-1 block text-xs">Angle&deg;</span>
								<input
									type="number"
									min="0"
									max="90"
									class="border-surface-content/25 w-full rounded border bg-transparent px-2 py-1 font-mono text-xs"
									bind:value={vantage.polar_deg}
								/>
							</label>
							<label class="block">
								<span class="text-surface-content/70 mb-1 block text-xs">Zoom</span>
								<input
									type="number"
									step="0.05"
									class="border-surface-content/25 w-full rounded border bg-transparent px-2 py-1 font-mono text-xs"
									bind:value={vantage.zoom}
								/>
							</label>
							<label class="block">
								<span class="text-surface-content/70 mb-1 block text-xs">X&nbsp;m</span>
								<input
									type="number"
									step="0.5"
									class="border-surface-content/25 w-full rounded border bg-transparent px-2 py-1 font-mono text-xs"
									bind:value={vantage.offset_x}
								/>
							</label>
							<label class="block">
								<span class="text-surface-content/70 mb-1 block text-xs">Y&nbsp;m</span>
								<input
									type="number"
									step="0.5"
									class="border-surface-content/25 w-full rounded border bg-transparent px-2 py-1 font-mono text-xs"
									bind:value={vantage.offset_y}
								/>
							</label>
						</div>
						<div class="mt-2 flex gap-2">
							<Button size="sm" type="button" on:click={() => applyPastedView(i)}>
								Apply pasted view
							</Button>
							<Button size="sm" color="danger" type="button" on:click={() => removeVantage(i)}>
								Remove
							</Button>
						</div>
					</div>
				{/each}

				<Button size="sm" type="button" on:click={addVantage}>Add vantage point</Button>
			</fieldset>

			<fieldset class="space-y-3">
				<legend class="mb-2 font-medium">Publication</legend>

				<label class="block">
					<span class="mb-1 block text-sm">Asset path</span>
					<input
						class="border-surface-content/25 w-full rounded border bg-transparent px-3 py-2 font-mono text-sm"
						bind:value={form.asset_path}
						placeholder="/scenes/soma1/assets/manifest.json"
					/>
				</label>

				<label class="flex items-center gap-2 text-sm">
					<input type="checkbox" bind:checked={form.published} />
					Published
				</label>
			</fieldset>

			<div class="flex gap-2">
				<Button
					type="submit"
					variant="fill"
					color="primary"
					icon={mdiContentSave}
					disabled={saving}
				>
					{saving ? 'Saving…' : 'Save scene'}
				</Button>
				<Button type="button" on:click={() => goto(resolve('/scene'))}>Cancel</Button>
			</div>
		</form>
	{/if}
</div>
