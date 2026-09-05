<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { resolve } from '$app/paths';
	import { createScene, getScene, getSites, updateScene, type Scene, type Site } from '$lib/api';
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
				published: form.published
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
