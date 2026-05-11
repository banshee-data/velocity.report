<script lang="ts">
	import { browser } from '$app/environment';
	import {
		buildInlineSvgChartRequestUrl,
		isInlineSvgContentType
	} from '$lib/components/charts/inlineSvgChart';

	export let url = '';
	export let label = 'Chart preview';
	export let loadingLabel = 'Loading chart…';
	export let minHeight = 320;

	let requestUrl = '';
	let loading = false;
	let error = '';
	let lastRequestedUrl = '';
	let imageKey = 0;

	$: if (browser && url !== lastRequestedUrl) {
		lastRequestedUrl = url;
		loadSvg(url);
	}

	function loadSvg(nextUrl: string) {
		if (!nextUrl) {
			requestUrl = '';
			loading = false;
			error = '';
			return;
		}

		try {
			requestUrl = buildInlineSvgChartRequestUrl(
				nextUrl,
				window.location.origin,
				Date.now()
			).toString();
			imageKey += 1;
			loading = true;
			error = '';
		} catch {
			requestUrl = '';
			loading = false;
			error = 'Could not load chart preview.';
		}
	}

	async function verifySvgResponse(event: Event) {
		const image = event.currentTarget;
		if (!(image instanceof HTMLImageElement) || !image.currentSrc) {
			loading = false;
			error = 'Could not load chart preview.';
			return;
		}

		try {
			const response = await fetch(image.currentSrc, {
				cache: 'no-store',
				headers: {
					'Cache-Control': 'no-cache'
				}
			});
			if (!response.ok || !isInlineSvgContentType(response.headers.get('content-type'))) {
				throw new Error('Could not load chart preview.');
			}
			loading = false;
			error = '';
		} catch {
			requestUrl = '';
			loading = false;
			error = 'Could not load chart preview.';
		}
	}

	function handleImageError() {
		requestUrl = '';
		loading = false;
		error = 'Could not load chart preview.';
	}
</script>

{#if url}
	<div
		class="chart-frame"
		style={`--chart-min-height: ${minHeight}px;`}
		aria-busy={loading ? 'true' : 'false'}
	>
		{#if requestUrl}
			{#key imageKey}
				<img
					class:chart-faded={loading}
					src={requestUrl}
					alt={label}
					on:load={verifySvgResponse}
					on:error={handleImageError}
				/>
			{/key}
		{/if}

		{#if loading || !requestUrl}
			<div class="chart-loading" role="status" aria-live="polite">
				<div class="chart-loading__shimmer" aria-hidden="true"></div>
				<p>{loadingLabel}</p>
			</div>
		{/if}
	</div>

	{#if error}
		<div
			role="alert"
			aria-live="assertive"
			class="mt-3 rounded border border-red-300 bg-red-50 p-3 text-red-800 dark:border-red-700 dark:bg-red-950 dark:text-red-200"
		>
			{error}
		</div>
	{/if}
{/if}

<style>
	.chart-frame {
		position: relative;
		min-height: var(--chart-min-height);
		overflow: hidden;
		background: white;
	}

	.chart-frame :global(svg) {
		display: block;
		width: 100%;
		height: auto;
	}

	.chart-frame img {
		display: block;
		width: 100%;
		height: auto;
	}

	.chart-faded {
		opacity: 0.24;
		transition: opacity 140ms ease;
	}

	.chart-loading {
		position: absolute;
		inset: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		background: linear-gradient(180deg, rgba(255, 255, 255, 0.86), rgba(255, 255, 255, 0.92));
		color: rgba(17, 24, 39, 0.85);
		font-size: 0.95rem;
		font-weight: 500;
	}

	.chart-loading__shimmer {
		position: absolute;
		inset: 0;
		background: linear-gradient(
			100deg,
			rgba(255, 255, 255, 0) 0%,
			rgba(148, 163, 184, 0.12) 40%,
			rgba(148, 163, 184, 0.28) 50%,
			rgba(148, 163, 184, 0.12) 60%,
			rgba(255, 255, 255, 0) 100%
		);
		transform: translateX(-100%);
		animation: chart-shimmer 1.25s linear infinite;
	}

	.chart-loading p {
		position: relative;
		z-index: 1;
		margin: 0;
	}

	@keyframes chart-shimmer {
		to {
			transform: translateX(100%);
		}
	}
</style>
