<script lang="ts">
	import type { RadarStats } from '$lib/api';
	import { Axis, Bars, Chart, Grid, Layer, Points, Rule, Spline } from 'layerchart';
	import { scaleLinear } from 'd3-scale';

	export let data: RadarStats[] = [];
	export let group = '4h';
	export let speedUnits = 'mph';
	export let p98Reference = 0;

	type ChartDatum = RadarStats & { countScaled: number };
	type FixedInterval = { floor: (value: Date) => Date; offset: (value: Date) => Date };

	const COLORS = {
		p50: '#fbd92f',
		p85: '#f7b32b',
		p98: '#f25f5c',
		max: '#2d1e2f',
		count: '#2d1e2f'
	};

	function groupToMilliseconds(value: string): number | null {
		const match = /^(\d+)([hd])$/.exec(value);
		if (!match) return null;
		const amount = Number(match[1]);
		if (!Number.isFinite(amount) || amount <= 0) return null;
		return match[2] === 'h' ? amount * 60 * 60 * 1000 : amount * 24 * 60 * 60 * 1000;
	}

	function createFixedInterval(stepMs: number): FixedInterval {
		return {
			floor(value: Date): Date {
				return new Date(Math.floor(value.getTime() / stepMs) * stepMs);
			},
			offset(value: Date): Date {
				return new Date(value.getTime() + stepMs);
			}
		};
	}

	$: stepMs = groupToMilliseconds(group) ?? 4 * 60 * 60 * 1000;
	$: maxCount = Math.max(1, ...data.map((row) => Number(row.count || 0)));
	$: maxSpeedMetric = Math.max(
		1,
		p98Reference || 0,
		...data.flatMap((row) => [row.p50 || 0, row.p85 || 0, row.p98 || 0, row.max || 0])
	);
	$: speedDomainMax = maxSpeedMetric * 1.1;
	$: xInterval = createFixedInterval(stepMs);
	$: chartData = data.map(
		(row) =>
			({
				...row,
				countScaled: (Number(row.count || 0) / maxCount) * speedDomainMax
			}) as ChartDatum
	);

	$: xDomain = (() => {
		if (chartData.length === 0) return [new Date(), new Date()] as [Date, Date];
		const times = chartData.map((row) => row.date.getTime());
		const minTime = Math.min(...times);
		const maxTime = Math.max(...times);
		if (minTime === maxTime) {
			const halfWindowMs = Math.max(30 * 60 * 1000, stepMs / 2);
			return [new Date(minTime - halfWindowMs), new Date(maxTime + halfWindowMs)] as [Date, Date];
		}
		return [new Date(minTime), new Date(maxTime)] as [Date, Date];
	})();
</script>

{#if chartData.length === 0}
	<div
		class="text-surface-content/60 flex h-[360px] items-center justify-center rounded border p-4 text-sm"
	>
		No chart data available for the selected range.
	</div>
{:else}
	<div class="rounded border p-4">
		<div
			class="w-full"
			style="height: 360px;"
			role="img"
			aria-label="Vehicle count bars and percentile speed lines"
		>
			<Chart
				data={chartData}
				x="date"
				{xDomain}
				xInterval={xInterval as any}
				y="p98"
				yDomain={[0, speedDomainMax]}
				yNice={false}
				padding={{ top: 16, right: 58, bottom: 42, left: 56 }}
				tooltip={false}
			>
				{#snippet children({ context })}
					<Layer type="svg">
						<Grid x={false} />

						<Bars
							data={chartData}
							x="date"
							y="countScaled"
							fill={COLORS.count}
							fillOpacity={0.14}
							stroke="none"
						/>

						<Rule
							y={p98Reference}
							stroke={COLORS.p98}
							strokeWidth={1.5}
							style="stroke-dasharray: 6 4"
							opacity={0.9}
						/>

						<Spline data={chartData} x="date" y="p50" stroke={COLORS.p50} strokeWidth={2} />
						<Spline data={chartData} x="date" y="p85" stroke={COLORS.p85} strokeWidth={2} />
						<Spline data={chartData} x="date" y="p98" stroke={COLORS.p98} strokeWidth={2.25} />
						<Spline
							data={chartData}
							x="date"
							y="max"
							stroke={COLORS.max}
							strokeWidth={1.75}
							style="stroke-dasharray: 3 3"
						/>

						<Points
							data={chartData}
							x="date"
							y="p50"
							r={2.8}
							fill={COLORS.p50}
							stroke="white"
							strokeWidth={0.7}
						/>
						<Points
							data={chartData}
							x="date"
							y="p85"
							r={2.8}
							fill={COLORS.p85}
							stroke="white"
							strokeWidth={0.7}
						/>
						<Points
							data={chartData}
							x="date"
							y="p98"
							r={3.2}
							fill={COLORS.p98}
							stroke="white"
							strokeWidth={0.7}
						/>
						<Points
							data={chartData}
							x="date"
							y="max"
							r={2.6}
							fill={COLORS.max}
							stroke="white"
							strokeWidth={0.7}
						/>

						<Axis placement="left" label={`Velocity (${speedUnits})`} ticks={5} />
						{@const countScale = scaleLinear()
							.domain([0, maxCount])
							.range(context.yRange as [number, number])}
						<Axis placement="right" label="Count" scale={countScale} ticks={5} />
						<Axis placement="bottom" tickSpacing={120} />
					</Layer>
				{/snippet}
			</Chart>
		</div>

		<div class="text-surface-content/80 mt-3 flex flex-wrap gap-3 text-xs">
			<span class="inline-flex items-center gap-1">
				<span class="h-2 w-2 rounded-sm" style={`background:${COLORS.count}`}></span>
				Count
			</span>
			<span class="inline-flex items-center gap-1">
				<span class="h-2 w-2 rounded-full" style={`background:${COLORS.p50}`}></span>
				P50
			</span>
			<span class="inline-flex items-center gap-1">
				<span class="h-2 w-2 rounded-full" style={`background:${COLORS.p85}`}></span>
				P85
			</span>
			<span class="inline-flex items-center gap-1">
				<span class="h-2 w-2 rounded-full" style={`background:${COLORS.p98}`}></span>
				P98
			</span>
			<span class="inline-flex items-center gap-1">
				<span class="h-2 w-2 rounded-full" style={`background:${COLORS.max}`}></span>
				Max
			</span>
			<span class="inline-flex items-center gap-2">
				<span
					class="inline-block h-px w-5 border-t-2 border-dashed"
					style={`border-color:${COLORS.p98}`}
				></span>
				Headline P98
			</span>
		</div>
	</div>
{/if}
