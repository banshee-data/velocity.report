<script lang="ts">
	/**
	 * A sweep's results: the fixed chart set and the per-combination table.
	 * `columns` lays the charts out side by side on a wide page and stacks
	 * them in a narrow panel.
	 */
	import CategoryBars from '#lib/components/charts/CategoryBars.svelte';
	import CategoryHeatmap from '#lib/components/charts/CategoryHeatmap.svelte';
	import {
		defaultSweepCharts,
		metricLabel,
		resultColumns,
		type SweepResult
	} from '#lib/sweepResults.js';

	interface Props {
		results: SweepResult[];
		columns?: 1 | 2;
	}

	let { results, columns = 2 }: Props = $props();

	const charts = $derived(defaultSweepCharts(results));
	const tableColumns = $derived(resultColumns(results));
</script>

{#if results.length === 0}
	<p class="text-surface-content/50 text-sm">No results yet.</p>
{:else}
	<div class="grid gap-4 {columns === 2 ? 'lg:grid-cols-2' : ''}">
		{#each charts as chart (chart.id)}
			<section class="vr-chart-card">
				<h3 class="text-surface-content mb-2 text-sm font-semibold">{chart.title}</h3>
				{#if chart.kind === 'bars'}
					<CategoryBars
						data={chart.data}
						valueLabel={metricLabel(chart.metric)}
						labelWidth={columns === 2 ? 200 : 140}
					/>
				{:else}
					<CategoryHeatmap
						heatmap={chart.data}
						xLabel={metricLabel(chart.xKey)}
						yLabel={metricLabel(chart.yKey)}
						valueLabel={metricLabel(chart.metric)}
					/>
				{/if}
			</section>
		{/each}
	</div>

	<details class="mt-4" open={columns === 2}>
		<summary class="text-surface-content cursor-pointer text-sm font-semibold">
			Results ({results.length} combinations)
		</summary>
		<div class="mt-2 overflow-x-auto">
			<table class="w-full text-xs">
				<thead>
					<tr class="border-surface-content/10 border-b">
						{#each tableColumns as col, i (`${col.key}-${i}`)}
							<th class="text-surface-content/70 px-2 py-1 text-left whitespace-nowrap"
								>{col.header}</th
							>
						{/each}
					</tr>
				</thead>
				<tbody>
					{#each results as result, r (r)}
						<tr class="border-surface-content/5 border-b">
							{#each tableColumns as col, i (`${col.key}-${i}`)}
								<td class="px-2 py-1 font-mono whitespace-nowrap">{col.cell(result)}</td>
							{/each}
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	</details>
{/if}
