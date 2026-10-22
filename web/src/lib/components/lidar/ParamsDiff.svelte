<script lang="ts">
	/**
	 * The running tuning parameters as JSON, with each line that differs from
	 * the built-in defaults highlighted, as the sweep dashboard showed them.
	 */
	import { paramDiffLines } from '#lib/sweepSetup.js';

	interface Props {
		current: Record<string, unknown> | null;
		defaults: Record<string, unknown> | null;
	}

	let { current, defaults }: Props = $props();

	const diff = $derived(current ? paramDiffLines(current, defaults) : null);
</script>

{#if !diff}
	<p class="text-surface-content/50 text-xs">Loading…</p>
{:else}
	<p class="text-surface-content/60 mb-1 text-xs">
		{diff.changedCount > 0
			? `${diff.changedCount} value${diff.changedCount === 1 ? '' : 's'} changed from default (highlighted).`
			: 'All values match the defaults.'}
	</p>
	<pre
		class="bg-surface-200 max-h-80 overflow-auto rounded p-2 text-xs">{#each diff.lines as line, i (i)}<span
				class={line.changed ? 'block bg-yellow-200/40 font-semibold' : 'block'}>{line.text}</span
			>{/each}</pre>
{/if}
