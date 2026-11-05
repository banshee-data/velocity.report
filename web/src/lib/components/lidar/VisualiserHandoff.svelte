<script lang="ts">
	/**
	 * What to open in VelocityVisualiser to work an item: the pack directory
	 * on this machine, with the menu path, and the recording it was cut from.
	 * Mask, span and physical work happens there, not in the browser.
	 */
	import { handoff, type WorkspaceRow } from '#lib/labellingWorkspace.js';

	export let row: WorkspaceRow;
	export let compact = false;

	let copied = '';

	async function copy(text: string) {
		try {
			await navigator.clipboard.writeText(text);
			copied = text;
		} catch {
			copied = '';
		}
	}

	$: h = handoff(row);
</script>

<div class="rounded border border-sky-300 bg-sky-50 p-3 text-sm text-sky-950">
	<p class="font-medium">Open in VelocityVisualiser</p>
	{#if h.kind === 'none'}
		<p class="mt-1">{h.reason}</p>
	{:else}
		<ol class="mt-1 list-decimal space-y-1 pl-5">
			<li><b>Annotation → Open Annotation Window</b> (⇧⌘A).</li>
			<li>
				<b>Open Annotation Pack…</b> (⌘O) and choose this folder{h.shared
					? ' (each is one run of the same capture and shows the same labels; any will do)'
					: ''}:
				{#each h.packs as pack, i (pack.path)}
					{#if !compact || i === 0}
						<div class="mt-1 flex items-center gap-2">
							<code class="rounded bg-white px-1.5 py-0.5 text-xs break-all">{pack.path}</code>
							<button
								type="button"
								class="shrink-0 text-xs text-sky-800 underline"
								on:click={() => copy(pack.path)}>{copied === pack.path ? 'Copied' : 'Copy'}</button
							>
						</div>
					{/if}
				{/each}
			</li>
			{#if !compact && h.packs[0].vrlog}
				<li>
					To follow the same frames in the main window, replay the run's recording there:
					<div class="mt-1 flex items-center gap-2">
						<code class="rounded bg-white px-1.5 py-0.5 text-xs break-all">{h.packs[0].vrlog}</code>
						<button
							type="button"
							class="shrink-0 text-xs text-sky-800 underline"
							on:click={() => copy(h.packs[0].vrlog ?? '')}
							>{copied === h.packs[0].vrlog ? 'Copied' : 'Copy'}</button
						>
					</div>
				</li>
			{/if}
			<li>Save in the window, then <b>Refresh</b> here: progress is read from what was saved.</li>
		</ol>
	{/if}
</div>
