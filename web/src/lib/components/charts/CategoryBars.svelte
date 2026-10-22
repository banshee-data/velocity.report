<script lang="ts">
	/**
	 * Horizontal bars over category labels. Horizontal because the labels are
	 * parameter combinations or metric names, which read badly rotated under a
	 * vertical bar. The chart grows with its rows rather than squeezing them.
	 */
	import { BarChart } from 'layerchart/svg';
	import { BAR_COLOUR, formatValue, type BarDatum } from './chartData.js';

	interface Props {
		data: BarDatum[];
		valueLabel: string;
		colour?: string;
		/** Width reserved for the category labels. */
		labelWidth?: number;
		emptyMessage?: string;
	}

	let {
		data,
		valueLabel,
		colour = BAR_COLOUR,
		labelWidth = 160,
		emptyMessage = 'No values to show.'
	}: Props = $props();

	// A missing value is left out rather than drawn as zero.
	const rows = $derived(data.filter((d): d is { label: string; value: number } => d.value != null));
	const height = $derived(Math.max(160, rows.length * 22 + 56));
</script>

{#if rows.length === 0}
	<p class="text-surface-content/50 py-8 text-center text-sm">{emptyMessage}</p>
{:else}
	<div style:height="{height}px" class="w-full">
		<BarChart
			data={rows}
			x="value"
			y="label"
			orientation="horizontal"
			series={[{ key: 'value', label: valueLabel, color: colour }]}
			padding={{ top: 8, right: 24, bottom: 32, left: labelWidth }}
			props={{ xAxis: { format: (v: number) => formatValue(v, 2) } }}
		/>
	</div>
{/if}
