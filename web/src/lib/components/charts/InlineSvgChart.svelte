<script lang="ts">
	import { browser } from '$app/environment';
	import {
		buildInlineSvgChartBlobUrl,
		buildInlineSvgChartRequestUrl,
		detectInlineSvgChartTheme,
		type InlineSvgChartTheme,
		type InlineSvgChartThemeMode,
		isInlineSvgContentType,
		transformInlineSvgChartSvg
	} from '$lib/components/charts/inlineSvgChart';
	import { onMount } from 'svelte';

	export let url = '';
	export let label = 'Chart preview';
	export let loadingLabel = 'Loading chart…';
	export let minHeight = 320;
	export let themeMode: InlineSvgChartThemeMode = 'source';

	let imageUrl = '';
	let loading = false;
	let error = '';
	let sourceSvg = '';
	let activeTheme: InlineSvgChartTheme = 'light';
	let requestSerial = 0;
	let lastRequestedUrl = '';
	let imageKey = 0;
	let frameElement: HTMLDivElement | null = null;
	let themeObserver: MutationObserver | null = null;

	$: if (browser && url !== lastRequestedUrl) {
		lastRequestedUrl = url;
		void loadSvg(url);
	}

	onMount(() => {
		if (!browser) {
			return;
		}

		themeObserver = new MutationObserver(() => {
			if (themeMode !== 'source' && sourceSvg) {
				renderSvgForCurrentTheme();
			}
		});
		themeObserver.observe(document.documentElement, {
			attributes: true,
			attributeFilter: ['class', 'data-theme', 'style']
		});

		return () => {
			themeObserver?.disconnect();
			revokeImageUrl();
		};
	});

	$: if (browser && sourceSvg && themeMode !== 'source') {
		renderSvgForCurrentTheme();
	}

	async function loadSvg(nextUrl: string) {
		const serial = ++requestSerial;

		if (!nextUrl) {
			clearChart();
			loading = false;
			return;
		}

		loading = true;
		error = '';

		try {
			const requestUrl = buildInlineSvgChartRequestUrl(nextUrl, window.location.origin, Date.now());
			const response = await fetch(requestUrl, {
				cache: 'no-store',
				headers: {
					'Cache-Control': 'no-cache'
				}
			});
			if (!response.ok || !isInlineSvgContentType(response.headers.get('content-type'))) {
				throw new Error('Could not load chart preview.');
			}
			const text = await response.text();
			if (serial !== requestSerial) {
				return;
			}
			sourceSvg = text;
			renderSvgForCurrentTheme();
			loading = false;
		} catch {
			if (serial !== requestSerial) {
				return;
			}
			clearChart();
			error = 'Could not load chart preview.';
		}
	}

	function renderSvgForCurrentTheme() {
		if (!sourceSvg) {
			activeTheme = 'light';
			revokeImageUrl();
			return;
		}

		const theme = themeMode === 'dashboard' ? detectInlineSvgChartTheme(frameElement) : 'light';
		activeTheme = theme;
		const themedSvg =
			themeMode === 'dashboard' ? transformInlineSvgChartSvg(sourceSvg, theme) : sourceSvg;

		revokeImageUrl();
		imageUrl = buildInlineSvgChartBlobUrl(themedSvg);
		imageKey += 1;
	}

	function revokeImageUrl() {
		if (imageUrl.startsWith('blob:')) {
			URL.revokeObjectURL(imageUrl);
		}
		imageUrl = '';
	}

	function clearChart() {
		sourceSvg = '';
		activeTheme = 'light';
		revokeImageUrl();
		loading = false;
		error = '';
	}

	function handleImageLoad() {
		loading = false;
	}

	function handleImageError() {
		revokeImageUrl();
		loading = false;
		error = 'Could not load chart preview.';
	}
</script>

{#if url}
	<div
		bind:this={frameElement}
		class="chart-frame"
		class:chart-frame-dark={activeTheme === 'dark'}
		style={`--chart-min-height: ${minHeight}px;`}
		aria-busy={loading ? 'true' : 'false'}
	>
		{#if imageUrl}
			{#key imageKey}
				<img
					class:chart-faded={loading}
					src={imageUrl}
					alt={label}
					on:load={handleImageLoad}
					on:error={handleImageError}
				/>
			{/key}
		{/if}

		{#if loading || !imageUrl}
			<div
				class:chart-loading-dark={activeTheme === 'dark'}
				class="chart-loading"
				role="status"
				aria-live="polite"
			>
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

	.chart-frame-dark {
		background: #000;
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

	.chart-loading-dark {
		background: linear-gradient(180deg, rgba(0, 0, 0, 0.72), rgba(0, 0, 0, 0.82));
		color: rgba(255, 255, 255, 0.88);
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
