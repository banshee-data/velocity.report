<script lang="ts">
	/**
	 * Why a sweep scored as it did: the objective, the composite score, the
	 * top contributors, label coverage, and each component with its weight
	 * and its change from the previous sweep.
	 */
	import { parseScoreComponents, SCORE_COMPONENTS } from '#lib/sweepResults.js';

	interface Props {
		components: unknown;
		objectiveName?: string;
		objectiveVersion?: string;
		delta?: Record<string, unknown> | null;
	}

	let { components, objectiveName, objectiveVersion, delta = null }: Props = $props();

	const parsed = $derived(parseScoreComponents(components));
	const weights = $derived((parsed?.weights_used ?? {}) as Record<string, unknown>);
	const changes = $derived(
		(delta ?? (parsed?.delta_vs_previous as Record<string, unknown> | undefined) ?? null) as Record<
			string,
			unknown
		> | null
	);

	function fixed(v: unknown, digits: number): string {
		return typeof v === 'number' && Number.isFinite(v) ? v.toFixed(digits) : '—';
	}

	function signed(v: unknown): string {
		return typeof v === 'number' && Number.isFinite(v)
			? `${v >= 0 ? '+' : ''}${v.toFixed(4)}`
			: '—';
	}
</script>

{#if parsed}
	<div class="space-y-2 text-xs">
		{#if objectiveName}
			<p>
				<span class="text-surface-content/60">Objective</span>
				{objectiveName}{objectiveVersion ? ` (${objectiveVersion})` : ''}
			</p>
		{/if}
		<p>
			<span class="text-surface-content/60">Composite score</span>
			<strong>{fixed(parsed.composite_score, 4)}</strong>
		</p>
		{#if Array.isArray(parsed.top_contributors) && parsed.top_contributors.length > 0}
			<p>
				<span class="text-surface-content/60">Top contributors</span>
				{parsed.top_contributors.join(', ')}
			</p>
		{/if}
		{#if typeof parsed.label_coverage_confidence === 'number'}
			<p>
				<span class="text-surface-content/60">Label coverage</span>
				{(parsed.label_coverage_confidence * 100).toFixed(1)}%
			</p>
		{/if}
		<table class="w-full">
			<thead>
				<tr class="border-surface-content/10 border-b">
					<th class="text-surface-content/70 px-2 py-1 text-left">Component</th>
					<th class="text-surface-content/70 px-2 py-1 text-right">Value</th>
					<th class="text-surface-content/70 px-2 py-1 text-right">Weight</th>
					{#if changes}<th class="text-surface-content/70 px-2 py-1 text-right">Change</th>{/if}
				</tr>
			</thead>
			<tbody>
				{#each SCORE_COMPONENTS as [key, label] (key)}
					<tr class="border-surface-content/5 border-b">
						<td class="px-2 py-1">{label}</td>
						<td class="px-2 py-1 text-right font-mono">{fixed(parsed[key], 4)}</td>
						<td class="text-surface-content/60 px-2 py-1 text-right font-mono"
							>{fixed(weights[key], 2)}</td
						>
						{#if changes}<td class="px-2 py-1 text-right font-mono">{signed(changes[key])}</td>{/if}
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{:else}
	<p class="text-surface-content/50 text-xs">No score breakdown for this sweep.</p>
{/if}
