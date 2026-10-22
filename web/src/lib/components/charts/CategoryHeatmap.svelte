<script lang="ts">
	/**
	 * Heatmap over two category axes, such as two swept parameters, with a
	 * third value as colour. Cells are drawn from band scales; a combination
	 * that was not run is left blank.
	 */
	import { scaleBand } from 'd3-scale';
	import { Axis, Chart, Layer } from 'layerchart/svg';
	import { formatValue, VIRIDIS, type HeatmapData } from './chartData.js';

	interface Props {
		heatmap: HeatmapData;
		xLabel: string;
		yLabel: string;
		valueLabel: string;
		height?: number;
		emptyMessage?: string;
	}

	let {
		heatmap,
		xLabel,
		yLabel,
		valueLabel,
		height = 320,
		emptyMessage = 'No values to show.'
	}: Props = $props();
</script>

{#if heatmap.cells.length === 0}
	<p class="text-surface-content/50 py-8 text-center text-sm">{emptyMessage}</p>
{:else}
	<div style:height="{height}px" class="w-full">
		<Chart
			data={heatmap.cells}
			x="x"
			y="y"
			xScale={scaleBand().padding(0.04)}
			yScale={scaleBand().padding(0.04)}
			xDomain={heatmap.xValues}
			yDomain={heatmap.yValues}
			padding={{ top: 8, right: 16, bottom: 44, left: 64 }}
		>
			{#snippet children({ context })}
				<Layer>
					<Axis placement="left" label={yLabel} />
					<Axis placement="bottom" label={xLabel} />
					{#each heatmap.cells as cell (`${cell.x}|${cell.y}`)}
						<rect
							x={context.xScale(cell.x)}
							y={context.yScale(cell.y)}
							width={context.xScale.bandwidth?.() ?? 0}
							height={context.yScale.bandwidth?.() ?? 0}
							fill={cell.colour}
							><title
								>{xLabel}: {cell.x}, {yLabel}: {cell.y} — {valueLabel}: {formatValue(
									cell.value
								)}</title
							></rect
						>
					{/each}
				</Layer>
			{/snippet}
		</Chart>
	</div>
	<div class="text-surface-content/60 mt-1 flex items-center gap-2 text-xs">
		<span>{formatValue(heatmap.min)}</span>
		<span class="flex h-2 w-32">
			{#each VIRIDIS as stop (stop)}
				<span class="h-2 flex-1" style:background-color={stop}></span>
			{/each}
		</span>
		<span>{formatValue(heatmap.max)}</span>
		<span>{valueLabel}</span>
	</div>
{/if}
