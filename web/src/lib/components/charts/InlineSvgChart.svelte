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

	let svg = '';
	let loading = false;
	let error = '';
	let requestSerial = 0;
	let lastRequestedUrl = '';

	$: if (browser && url !== lastRequestedUrl) {
		lastRequestedUrl = url;
		void loadSvg(url);
	}

	async function loadSvg(nextUrl: string) {
		const serial = ++requestSerial;

		if (!nextUrl) {
			svg = '';
			loading = false;
			error = '';
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
			if (!response.ok) {
				throw new Error('Could not load chart preview.');
			}
			if (!isInlineSvgContentType(response.headers.get('content-type'))) {
				throw new Error('Could not load chart preview.');
			}
			const text = await response.text();
			if (serial !== requestSerial) {
				return;
			}
			svg = text;
		} catch {
			if (serial !== requestSerial) {
				return;
			}
			svg = '';
			error = 'Could not load chart preview.';
		} finally {
			if (serial === requestSerial) {
				loading = false;
			}
		}
	}
</script>

{#if url}
	<div
		class="chart-frame"
		style={`--chart-min-height: ${minHeight}px;`}
		role="img"
		aria-label={label}
		aria-busy={loading ? 'true' : 'false'}
	>
		{#if svg}
			<div class:chart-faded={loading}>
				<!--
					Deferred to v0.5.2 (BACKLOG.md, PR #455 comment 3139975818):
					replace {@html} with <img src> / <object data> or sanitise on
					injection. Safe today: same-origin only, server-rendered chart
					SVGs with no user-controlled script/handler attributes.
				-->
				<!-- eslint-disable-next-line svelte/no-at-html-tags -->
				{@html svg}
			</div>
		{/if}

		{#if loading || !svg}
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
