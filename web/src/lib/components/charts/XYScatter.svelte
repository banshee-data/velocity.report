<script lang="ts">
	/**
	 * XY scatter in sensor metres: equal, sensor-centred axes so a plan view
	 * keeps its shape. Each series is drawn in order, so put background before
	 * foreground. A point's own colour wins over its series colour.
	 */
	import { scaleOrdinal } from 'd3-scale';
	import { Axis, Chart, Layer, Points, Tooltip } from 'layerchart/svg';
	import { formatValue, symmetricDomain, type ScatterSeries, type XYPoint } from './chartData.js';

	interface Props {
		series: ScatterSeries[];
		/** Label for each point's value in the tooltip, if it has one. */
		valueLabel?: string;
		/** Point radius in pixels. */
		r?: number;
		height?: number;
		xLabel?: string;
		yLabel?: string;
		emptyMessage?: string;
	}

	let {
		series,
		valueLabel = 'Value',
		r = 2,
		height = 420,
		xLabel = 'X (m)',
		yLabel = 'Y (m)',
		emptyMessage = 'No points to show.'
	}: Props = $props();

	type Datum = XYPoint & { colour: string; seriesLabel: string };

	const data = $derived<Datum[]>(
		series.flatMap((s) =>
			s.points.map((p) => ({
				...p,
				colour: ('colour' in p ? p.colour : undefined) ?? s.colour ?? 'currentColor',
				seriesLabel: s.label
			}))
		)
	);
	const domain = $derived(symmetricDomain(...series.map((s) => s.points)));
	// An identity mapping: each point already carries its fill.
	const colours = $derived([...new Set(data.map((d) => d.colour))]);
</script>

{#if data.length === 0}
	<p class="text-surface-content/50 py-8 text-center text-sm" style:min-height="{height / 2}px">
		{emptyMessage}
	</p>
{:else}
	<div style:height="{height}px" class="w-full">
		<Chart
			{data}
			x="x"
			y="y"
			xDomain={domain}
			yDomain={domain}
			c="colour"
			cScale={scaleOrdinal()}
			cDomain={colours}
			cRange={colours}
			padding={{ top: 8, right: 16, bottom: 40, left: 52 }}
			tooltipContext={{ mode: 'quadtree' }}
		>
			<Layer>
				<Axis placement="left" grid rule label={yLabel} />
				<Axis placement="bottom" grid rule label={xLabel} />
				<Points {r} />
			</Layer>
			<Tooltip.Root>
				{#snippet children({ data: d }: { data: Datum })}
					<Tooltip.Header>{d.seriesLabel}</Tooltip.Header>
					<Tooltip.List>
						<Tooltip.Item label="X" value={formatValue(d.x, 2)} />
						<Tooltip.Item label="Y" value={formatValue(d.y, 2)} />
						{#if d.value != null}
							<Tooltip.Item label={valueLabel} value={formatValue(d.value, 2)} color={d.colour} />
						{/if}
					</Tooltip.List>
				{/snippet}
			</Tooltip.Root>
		</Chart>
	</div>
{/if}
