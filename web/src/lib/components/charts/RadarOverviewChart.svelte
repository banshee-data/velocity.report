<script lang="ts">
	import type { RadarStats } from '$lib/api';
	import { PERCENTILE_COLOURS } from '$lib/palette';
	import { scaleLinear } from 'd3-scale';
	import type { TimeInterval } from 'd3-time';
	import { Axis, Bars, Chart, Grid, Layer, Points, Rule, Spline, Voronoi } from 'layerchart';
	import { SvelteMap } from 'svelte/reactivity';

	export let data: RadarStats[] = [];
	export let group = '4h';
	export let speedUnits = 'mph';
	export let p98Reference = 0;

	type MetricKey = 'p50' | 'p85' | 'p98' | 'max';
	type AxisTimeFormat = 'hour' | 'day' | 'week' | 'month-year' | 'quarter';
	type ChartDatum = RadarStats & {
		countScaled: number;
		lowSampleTop: number;
		dayKey: string;
	};
	type FixedInterval = { floor: (value: Date) => Date; offset: (value: Date) => Date };
	type TimeBand = { start: Date; end: Date };
	type DaySummary = {
		dayKey: string;
		date: Date;
		periods: number;
		countTotal: number;
		p50Avg: number;
		p85Avg: number;
		p98Avg: number;
		maxPeak: number;
	};

	const COLORS = {
		p50: PERCENTILE_COLOURS.p50,
		p85: PERCENTILE_COLOURS.p85,
		p98: PERCENTILE_COLOURS.p98,
		max: PERCENTILE_COLOURS.max,
		count: PERCENTILE_COLOURS.count_bar,
		lowSample: PERCENTILE_COLOURS.low_sample
	};

	const CHART_PADDING = { top: 16, right: 58, bottom: 42, left: 84 };
	const COUNT_AXIS_SCALE = 1.6;
	const LOW_SAMPLE_THRESHOLD = 50;
	const MISSING_COUNT_THRESHOLD = 5;
	const TARGET_MARKERS = 160;
	const AXIS_MAJOR_TICKS = 3;
	const HOUR_MS = 60 * 60 * 1000;
	const DAY_MS = 24 * HOUR_MS;
	const compactCountFormatter = new Intl.NumberFormat('en-US', {
		notation: 'compact',
		maximumFractionDigits: 1
	});

	let chartContainerEl: HTMLDivElement | undefined;
	let chartContainerWidth = 0;
	let chartContainerHeight = 0;
	let tooltipX = 0;
	let tooltipY = 0;
	let hoveredDayKey: string | null = null;

	function groupToMilliseconds(value: string): number | null {
		const match = /^(\d+)([hd])$/.exec(value);
		if (!match) return null;
		const amount = Number(match[1]);
		if (!Number.isFinite(amount) || amount <= 0) return null;
		return match[2] === 'h' ? amount * HOUR_MS : amount * DAY_MS;
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

	function isFiniteNumber(value: unknown): value is number {
		return typeof value === 'number' && Number.isFinite(value);
	}

	function dayKey(date: Date): string {
		return `${date.getFullYear()}-${date.getMonth()}-${date.getDate()}`;
	}

	function chooseAxisTimeFormat(rangeMs: number): AxisTimeFormat {
		if (rangeMs <= 4 * DAY_MS) return 'hour';
		if (rangeMs <= 90 * DAY_MS) return 'day';
		if (rangeMs <= 270 * DAY_MS) return 'week';
		if (rangeMs <= 900 * DAY_MS) return 'month-year';
		return 'quarter';
	}

	function buildAlternatingBands(domain: [Date, Date], count: number): TimeBand[] {
		if (!domain || count <= 0) return [];
		const start = domain[0].getTime();
		const end = domain[1].getTime();
		const total = end - start;
		if (!Number.isFinite(total) || total <= 0) return [];

		const step = total / count;
		const bands: TimeBand[] = [];
		for (let i = 0; i < count; i++) {
			const bandStart = start + i * step;
			const bandEnd = i === count - 1 ? end : start + (i + 1) * step;
			bands.push({ start: new Date(bandStart), end: new Date(bandEnd) });
		}
		return bands;
	}

	function isMetricRowValid(row: ChartDatum, metric: MetricKey): boolean {
		return row.count >= MISSING_COUNT_THRESHOLD && isFiniteNumber(row[metric]);
	}

	function buildSeriesSegments(
		rows: ChartDatum[],
		metric: MetricKey,
		gapBreakMs: number
	): ChartDatum[][] {
		const segments: ChartDatum[][] = [];
		let current: ChartDatum[] = [];

		for (const row of rows) {
			if (!isMetricRowValid(row, metric)) {
				if (current.length > 1) segments.push(current);
				current = [];
				continue;
			}

			const previous = current[current.length - 1];
			if (previous) {
				const gap = row.date.getTime() - previous.date.getTime();
				if (gap > gapBreakMs) {
					if (current.length > 1) segments.push(current);
					current = [];
				}
			}

			current.push(row);
		}

		if (current.length > 1) segments.push(current);
		return segments;
	}

	function buildMarkerData(rows: ChartDatum[], metric: MetricKey, stride: number): ChartDatum[] {
		if (rows.length === 0) return [];
		const clampedStride = Math.max(1, stride);
		return rows.filter((row, index) => {
			if (!isMetricRowValid(row, metric)) return false;
			return index === 0 || index === rows.length - 1 || index % clampedStride === 0;
		});
	}

	function clamp(value: number, min: number, max: number): number {
		return Math.min(max, Math.max(min, value));
	}

	function niceStep(maxValue: number, targetTicks: number): number {
		if (!Number.isFinite(maxValue) || maxValue <= 0) return 1;
		const raw = maxValue / Math.max(1, targetTicks);
		const power = Math.pow(10, Math.floor(Math.log10(raw)));
		const normalized = raw / power;
		const rounded =
			normalized <= 1
				? 1
				: normalized <= 2
					? 2
					: normalized <= 2.5
						? 2.5
						: normalized <= 5
							? 5
							: 10;
		return rounded * power;
	}

	function buildTickValues(maxValue: number, targetTicks: number): number[] {
		const step = niceStep(maxValue, targetTicks);
		const cappedMax = Math.max(step, Math.ceil(maxValue / step) * step);
		const ticks: number[] = [];
		for (let tick = 0; tick <= cappedMax + step / 2; tick += step) {
			ticks.push(Number(tick.toFixed(6)));
		}
		return ticks;
	}

	function buildTickValuesForRange(
		minValue: number,
		maxValue: number,
		targetTicks: number
	): number[] {
		const safeMin = Number.isFinite(minValue) ? minValue : 0;
		const safeMax = Number.isFinite(maxValue) ? maxValue : safeMin + 1;
		const span = Math.max(1, safeMax - safeMin);
		const step = niceStep(span, targetTicks);
		const start = Math.floor(safeMin / step) * step;
		const end = Math.max(start + step, Math.ceil(safeMax / step) * step);
		const ticks: number[] = [];
		for (let tick = start; tick <= end + step / 2; tick += step) {
			ticks.push(Number(tick.toFixed(6)));
		}
		return ticks;
	}

	function formatSpeedTick(value: number | string): string {
		const n = Number(value);
		if (!Number.isFinite(n)) return '';
		if (Math.abs(n) >= 10) return Math.round(n).toString();
		return n.toFixed(1).replace(/\.0$/, '');
	}

	function formatCountTick(value: number | string): string {
		const n = Number(value);
		if (!Number.isFinite(n)) return '';
		if (Math.abs(n) >= 1000) return compactCountFormatter.format(n);
		return Math.round(n).toLocaleString();
	}

	function buildDaySummaries(rows: ChartDatum[]): DaySummary[] {
		const buckets = new SvelteMap<
			string,
			{
				date: Date;
				periods: number;
				countTotal: number;
				p50Sum: number;
				p85Sum: number;
				p98Sum: number;
				maxPeak: number;
			}
		>();

		for (const row of rows) {
			const key = row.dayKey;
			const existing = buckets.get(key);
			if (!existing) {
				buckets.set(key, {
					date: new Date(row.date.getFullYear(), row.date.getMonth(), row.date.getDate()),
					periods: 1,
					countTotal: Math.max(0, Number(row.count || 0)),
					p50Sum: Number(row.p50 || 0),
					p85Sum: Number(row.p85 || 0),
					p98Sum: Number(row.p98 || 0),
					maxPeak: Number(row.max || 0)
				});
				continue;
			}

			existing.periods += 1;
			existing.countTotal += Math.max(0, Number(row.count || 0));
			existing.p50Sum += Number(row.p50 || 0);
			existing.p85Sum += Number(row.p85 || 0);
			existing.p98Sum += Number(row.p98 || 0);
			existing.maxPeak = Math.max(existing.maxPeak, Number(row.max || 0));
		}

		return [...buckets.entries()]
			.map(([key, value]) => ({
				dayKey: key,
				date: value.date,
				periods: value.periods,
				countTotal: value.countTotal,
				p50Avg: value.p50Sum / value.periods,
				p85Avg: value.p85Sum / value.periods,
				p98Avg: value.p98Sum / value.periods,
				maxPeak: value.maxPeak
			}))
			.sort((a, b) => a.date.getTime() - b.date.getTime());
	}

	function formatDayLabel(date: Date): string {
		return date.toLocaleDateString(undefined, {
			year: 'numeric',
			month: 'short',
			day: 'numeric'
		});
	}

	function formatMetric(value: number): string {
		if (!Number.isFinite(value)) return '-';
		return value.toFixed(1);
	}

	function handleVoronoiMove(event: PointerEvent, detail: { data: ChartDatum }) {
		if (!chartContainerEl) return;
		hoveredDayKey = detail.data.dayKey;
		const rect = chartContainerEl.getBoundingClientRect();
		tooltipX = event.clientX - rect.left + 12;
		tooltipY = event.clientY - rect.top + 12;
	}

	function hideTooltip() {
		hoveredDayKey = null;
	}

	$: stepMs = groupToMilliseconds(group) ?? 4 * HOUR_MS;
	$: sortedData = [...data]
		.filter((row) => row.date instanceof Date && !Number.isNaN(row.date.getTime()))
		.sort((a, b) => a.date.getTime() - b.date.getTime());
	$: maxCount = Math.max(1, ...sortedData.map((row) => Math.max(0, Number(row.count || 0))));
	$: countAxisTarget = Math.max(1, Math.ceil(maxCount * COUNT_AXIS_SCALE));
	$: speedSeriesValues = sortedData
		.flatMap((row) => [row.p50, row.p85, row.p98, row.max].map((value) => Number(value)))
		.filter((value) => Number.isFinite(value) && value >= 0) as number[];
	$: maxSpeedMetric = Math.max(1, p98Reference || 0, ...speedSeriesValues);
	$: minSpeedMetric = Math.min(
		p98Reference && Number.isFinite(p98Reference) ? p98Reference : Number.POSITIVE_INFINITY,
		...(speedSeriesValues.length ? speedSeriesValues : [0])
	);
	$: speedPadding = Math.max(2, (maxSpeedMetric - minSpeedMetric) * 0.18);
	$: speedDomainMinTarget = Math.max(0, minSpeedMetric - speedPadding);
	$: speedDomainMaxTarget = maxSpeedMetric + speedPadding;
	$: speedTicks = buildTickValuesForRange(
		speedDomainMinTarget,
		speedDomainMaxTarget,
		AXIS_MAJOR_TICKS
	);
	$: countTicks = buildTickValues(countAxisTarget, AXIS_MAJOR_TICKS);
	$: speedAxisMin = speedTicks[0] ?? 0;
	$: speedAxisMax = speedTicks[speedTicks.length - 1] ?? 1;
	$: speedAxisSpan = Math.max(1, speedAxisMax - speedAxisMin);
	$: countAxisMax = countTicks[countTicks.length - 1] ?? 1;
	$: gapBreakMs = Math.max(stepMs * 1.5, 30 * HOUR_MS);
	$: xInterval = createFixedInterval(stepMs);
	$: chartData = sortedData.map(
		(row) =>
			({
				...row,
				countScaled:
					speedAxisMin + (Math.max(0, Number(row.count || 0)) / countAxisMax) * speedAxisSpan,
				lowSampleTop: Math.max(0, Number(row.count || 0)) < LOW_SAMPLE_THRESHOLD ? speedAxisMax : 0,
				dayKey: dayKey(row.date)
			}) as ChartDatum
	);
	$: markerStride = Math.max(1, Math.ceil(chartData.length / TARGET_MARKERS));
	$: maxMarkerStride = Math.max(1, markerStride * 2);
	$: p50Segments = buildSeriesSegments(chartData, 'p50', gapBreakMs);
	$: p85Segments = buildSeriesSegments(chartData, 'p85', gapBreakMs);
	$: p98Segments = buildSeriesSegments(chartData, 'p98', gapBreakMs);
	$: maxSegments = buildSeriesSegments(chartData, 'max', gapBreakMs);
	$: p50Markers = buildMarkerData(chartData, 'p50', markerStride);
	$: p85Markers = buildMarkerData(chartData, 'p85', markerStride);
	$: p98Markers = buildMarkerData(chartData, 'p98', markerStride);
	$: maxMarkers = buildMarkerData(chartData, 'max', maxMarkerStride);

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

	$: xRangeMs = xDomain[1].getTime() - xDomain[0].getTime();
	$: axisTimeFormat = chooseAxisTimeFormat(xRangeMs);
	$: bandCount = clamp(Math.round((chartContainerWidth || 1400) / 130), 10, 15);
	$: alternatingBands = buildAlternatingBands(xDomain, bandCount);
	$: daySummaries = buildDaySummaries(chartData);
	$: daySummaryByKey = new SvelteMap(daySummaries.map((summary) => [summary.dayKey, summary]));
	$: hoveredDaySummary = hoveredDayKey ? daySummaryByKey.get(hoveredDayKey) : undefined;
	$: tooltipLeft = clamp(tooltipX, 8, Math.max(8, chartContainerWidth - 240));
	$: tooltipTop = clamp(tooltipY, 8, Math.max(8, chartContainerHeight - 170));
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
			class="radar-overview-chart relative w-full"
			role="img"
			aria-label="Vehicle count bars and percentile speed lines"
			bind:this={chartContainerEl}
			bind:clientWidth={chartContainerWidth}
			bind:clientHeight={chartContainerHeight}
		>
			<Chart
				data={chartData}
				x="date"
				{xDomain}
				xInterval={xInterval as unknown as TimeInterval}
				y="p98"
				yDomain={[speedAxisMin, speedAxisMax]}
				yNice={false}
				padding={CHART_PADDING}
				tooltip={false}
			>
				{#snippet children({ context })}
					<Layer type="svg">
						{#each alternatingBands as band, index (`band-${index}-${band.start.getTime()}`)}
							{#if index % 2 === 0}
								<rect
									x={context.xScale(band.start)}
									y={0}
									width={Math.max(0, context.xScale(band.end) - context.xScale(band.start))}
									height={context.height}
									fill="#0f172a"
									fill-opacity="0.03"
								/>
							{/if}
						{/each}

						<Grid x={false} />

						<Bars
							data={chartData}
							x="date"
							y="lowSampleTop"
							fill={COLORS.lowSample}
							fillOpacity={0.18}
							stroke="none"
						/>

						<Bars
							data={chartData}
							x="date"
							y="countScaled"
							fill={COLORS.count}
							fillOpacity={0.25}
							stroke="none"
						/>

						<Rule
							y={p98Reference}
							stroke={COLORS.p98}
							strokeWidth={1.5}
							style="stroke-dasharray: 6 4"
							opacity={0.9}
						/>

						{#each p50Segments as segment, idx (`p50-${idx}-${segment[0].date.getTime()}`)}
							<Spline
								data={segment}
								x="date"
								y="p50"
								fill="none"
								stroke={COLORS.p50}
								strokeWidth={1}
							/>
						{/each}
						{#each p85Segments as segment, idx (`p85-${idx}-${segment[0].date.getTime()}`)}
							<Spline
								data={segment}
								x="date"
								y="p85"
								fill="none"
								stroke={COLORS.p85}
								strokeWidth={1}
							/>
						{/each}
						{#each p98Segments as segment, idx (`p98-${idx}-${segment[0].date.getTime()}`)}
							<Spline
								data={segment}
								x="date"
								y="p98"
								fill="none"
								stroke={COLORS.p98}
								strokeWidth={1}
							/>
						{/each}
						{#each maxSegments as segment, idx (`max-${idx}-${segment[0].date.getTime()}`)}
							<Spline
								data={segment}
								x="date"
								y="max"
								fill="none"
								stroke={COLORS.max}
								strokeWidth={1}
								style="stroke-dasharray: 3 3"
							/>
						{/each}

						<Points
							data={p50Markers}
							x="date"
							y="p50"
							r={1.2}
							fill={COLORS.p50}
							stroke="white"
							strokeWidth={0.35}
						/>
						<Points
							data={p85Markers}
							x="date"
							y="p85"
							r={1.2}
							fill={COLORS.p85}
							stroke="white"
							strokeWidth={0.35}
						/>
						<Points
							data={p98Markers}
							x="date"
							y="p98"
							r={1.6}
							fill={COLORS.p98}
							stroke="white"
							strokeWidth={0.35}
						/>
						<Points
							data={maxMarkers}
							x="date"
							y="max"
							r={1.3}
							fill="none"
							stroke={COLORS.max}
							strokeWidth={0.6}
						/>

						{@const speedScale = scaleLinear()
							.domain([speedAxisMin, speedAxisMax])
							.range(context.yRange as [number, number])}
						<Axis
							placement="left"
							label={`Velocity (${speedUnits})`}
							scale={speedScale}
							ticks={speedTicks}
							format={formatSpeedTick}
							tickLabelProps={{
								fill: '#111827',
								stroke: 'none',
								strokeWidth: 0
							}}
							labelProps={{ fill: '#111827', stroke: 'none', strokeWidth: 0 }}
							rule={true}
						/>
						{@const countScale = scaleLinear()
							.domain([0, countAxisMax])
							.range(context.yRange as [number, number])}
						<Axis
							placement="right"
							label="Count"
							scale={countScale}
							ticks={countTicks}
							format={formatCountTick}
							tickLabelProps={{ fill: '#111827', stroke: 'none', strokeWidth: 0 }}
							labelProps={{ fill: '#111827', stroke: 'none', strokeWidth: 0 }}
							rule={true}
						/>
						<Axis
							placement="bottom"
							ticks={bandCount}
							format={axisTimeFormat}
							tickMultiline={true}
							rule={true}
						/>

						<Voronoi
							data={chartData}
							r={18}
							onpointerenter={handleVoronoiMove}
							onpointermove={handleVoronoiMove}
							onpointerleave={hideTooltip}
						/>
					</Layer>
				{/snippet}
			</Chart>

			{#if hoveredDaySummary}
				<div
					class="border-surface-300 bg-surface-100/98 text-surface-content pointer-events-none absolute z-30 w-[232px] rounded border px-3 py-2 text-xs shadow"
					style={`left:${tooltipLeft}px; top:${tooltipTop}px;`}
				>
					<p class="mb-1 text-sm font-semibold">{formatDayLabel(hoveredDaySummary.date)}</p>
					<p>Periods: {hoveredDaySummary.periods}</p>
					<p>Count: {hoveredDaySummary.countTotal.toLocaleString()}</p>
					<p>P50: {formatMetric(hoveredDaySummary.p50Avg)} {speedUnits}</p>
					<p>P85: {formatMetric(hoveredDaySummary.p85Avg)} {speedUnits}</p>
					<p>P98: {formatMetric(hoveredDaySummary.p98Avg)} {speedUnits}</p>
					<p>Max: {formatMetric(hoveredDaySummary.maxPeak)} {speedUnits}</p>
				</div>
			{/if}
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
			<span class="inline-flex items-center gap-1">
				<span class="h-2 w-2 rounded-sm" style={`background:${COLORS.lowSample}; opacity: 0.5;`}
				></span>
				Low-sample (&lt;{LOW_SAMPLE_THRESHOLD})
			</span>
		</div>
	</div>
{/if}

<style>
	.radar-overview-chart {
		height: 360px;
		min-height: 360px;
	}

	.radar-overview-chart :global(.lc-root-container) {
		height: 360px !important;
		min-height: 360px;
	}

	.radar-overview-chart :global(.lc-voronoi-path),
	.radar-overview-chart :global(.lc-voronoi-geo-path) {
		fill: transparent !important;
		stroke: transparent !important;
	}
</style>
